package jobs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/makemoments/gorender/internal/browser"
	"github.com/makemoments/gorender/internal/cache"
	"github.com/makemoments/gorender/internal/composition"
	goffmpeg "github.com/makemoments/gorender/internal/ffmpeg"
	"github.com/makemoments/gorender/internal/pipeline"
	"go.uber.org/zap"
)

const (
	maxRetries     = 3
	defaultWorkers = 4
)

// QueueOptions configures the job queue.
type QueueOptions struct {
	// MaxConcurrentJobs limits how many render jobs run simultaneously.
	// Prevents OOM on a constrained VPS. Defaults to 2.
	MaxConcurrentJobs int

	// WorkersPerJob is the browser pool size per job.
	// Total Chrome instances = MaxConcurrentJobs × WorkersPerJob.
	WorkersPerJob int

	// OutputDir is where finished MP4s are written.
	OutputDir string

	// TmpDir is where intermediate PNG frames land.
	TmpDir string

	// RetentionPeriod is how long finished MP4s are kept on disk.
	// The cleanup loop runs hourly. Defaults to 24h.
	RetentionPeriod time.Duration

	// PruneInterval controls how often the job store is pruned.
	PruneInterval time.Duration
}

func (o *QueueOptions) defaults() {
	if o.MaxConcurrentJobs == 0 {
		o.MaxConcurrentJobs = 2
	}
	if o.WorkersPerJob == 0 {
		o.WorkersPerJob = defaultWorkers
	}
	if o.OutputDir == "" {
		o.OutputDir = "./output"
	}
	if o.TmpDir == "" {
		o.TmpDir = os.TempDir()
	}
	if o.RetentionPeriod == 0 {
		o.RetentionPeriod = 24 * time.Hour
	}
	if o.PruneInterval == 0 {
		o.PruneInterval = time.Hour
	}
}

// Queue manages the lifecycle of render jobs.
// It owns the browser pool and dispatches jobs with concurrency control.
type Queue struct {
	store *Store
	pool  *browser.Pool
	cache *cache.AssetCache
	opts  QueueOptions
	log   *zap.Logger

	// sem limits concurrent render jobs (not concurrent frames).
	sem chan struct{}
	// incoming receives new job IDs to process.
	incoming chan string
}

// NewQueue creates a Queue and warms the browser pool.
func NewQueue(ctx context.Context, store *Store, opts QueueOptions, log *zap.Logger) (*Queue, error) {
	opts.defaults()

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	totalBrowsers := opts.MaxConcurrentJobs * opts.WorkersPerJob
	pool, err := browser.NewPool(ctx, browser.PoolOptions{
		Size:        totalBrowsers,
		HeadlessNew: true,
	}, log)
	if err != nil {
		return nil, fmt.Errorf("warming browser pool: %w", err)
	}

	assetCache := cache.New(log)

	q := &Queue{
		store:    store,
		pool:     pool,
		cache:    assetCache,
		opts:     opts,
		log:      log,
		sem:      make(chan struct{}, opts.MaxConcurrentJobs),
		incoming: make(chan string, 256),
	}

	// Enable shared asset interception on every warm browser instance once.
	for i := 0; i < q.pool.Len(); i++ {
		b, acqErr := q.pool.Acquire(ctx)
		if acqErr != nil {
			q.pool.Close()
			return nil, fmt.Errorf("acquiring browser for cache setup: %w", acqErr)
		}
		if setupErr := q.cache.EnableInterception(b.Context()); setupErr != nil {
			q.pool.Release(b)
			q.pool.Close()
			return nil, fmt.Errorf("enabling asset interception: %w", setupErr)
		}
		q.pool.Release(b)
	}

	return q, nil
}

// Start launches the dispatch loop and cleanup goroutines.
// Returns immediately; call Stop() to shut down.
func (q *Queue) Start(ctx context.Context) {
	go q.dispatchLoop(ctx)
	go q.cleanupLoop(ctx)
}

// Stop shuts down the browser pool. In-flight jobs will fail.
func (q *Queue) Stop() {
	q.pool.Close()
}

// Enqueue adds a job to the store and signals the dispatch loop.
func (q *Queue) Enqueue(job *Job) {
	q.store.Put(job)
	q.incoming <- job.ID
	q.log.Info("job enqueued", zap.String("id", job.ID), zap.String("url", job.Comp.URL))
}

// dispatchLoop processes incoming job IDs, respecting the concurrency semaphore.
func (q *Queue) dispatchLoop(ctx context.Context) {
	for {
		select {
		case id := <-q.incoming:
			// Acquire semaphore slot — blocks if MaxConcurrentJobs are active.
			select {
			case q.sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func(jobID string) {
				defer func() { <-q.sem }()
				q.runJob(ctx, jobID)
			}(id)

		case <-ctx.Done():
			return
		}
	}
}

// runJob executes a single render job with retry logic.
func (q *Queue) runJob(ctx context.Context, id string) {
	job := q.store.Get(id)
	if job == nil {
		q.log.Error("job not found in store", zap.String("id", id))
		return
	}

	now := time.Now()
	q.store.Update(id, func(j *Job) {
		j.Status = StatusRendering
		j.StartedAt = &now
	})

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			q.log.Warn("retrying job",
				zap.String("id", id),
				zap.Int("attempt", attempt),
				zap.Error(lastErr),
			)
			// Brief backoff between retries.
			select {
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			case <-ctx.Done():
				q.markFailed(id, ctx.Err())
				return
			}
		}

		lastErr = q.renderJob(ctx, job)
		if lastErr == nil {
			q.markDone(id, job.Comp.Output.Path)
			return
		}
	}

	q.markFailed(id, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr))
}

// renderJob does the actual work: sets up a tmp dir, runs the pipeline,
// and writes the final MP4.
func (q *Queue) renderJob(ctx context.Context, job *Job) error {
	comp := job.Comp
	comp.Defaults()

	// Each job gets its own tmp dir for frames.
	tmpDir := filepath.Join(q.opts.TmpDir, "gorender-"+job.ID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("creating tmp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set output path inside our managed output dir.
	comp.Output.Path = filepath.Join(q.opts.OutputDir, job.ID+".mp4")

	totalFrames := comp.DurationFrames

	// Build the render pipeline.
	rend := pipeline.NewRenderer(q.pool, comp, "png", q.log)
	rp := pipeline.NewRenderPipeline(rend, comp, tmpDir, false, 1, q.log)

	// Run rendering — get back an ordered frame channel.
	framesCh, errc := rp.Run(ctx, q.opts.WorkersPerJob)

	// Track progress via a counting wrapper around the frame channel.
	progressCh := q.trackProgress(job.ID, totalFrames, framesCh)

	// Stream into ffmpeg.
	writer := goffmpeg.NewWriter(comp, "png", q.log)
	if err := writer.Write(ctx, progressCh); err != nil {
		return fmt.Errorf("writing video: %w", err)
	}

	if err := <-errc; err != nil {
		return fmt.Errorf("render pipeline: %w", err)
	}

	return nil
}

// trackProgress wraps the frame channel, updating job progress as frames flow through.
func (q *Queue) trackProgress(jobID string, total int, in <-chan pipeline.FrameInOrder) <-chan pipeline.FrameInOrder {
	out := make(chan pipeline.FrameInOrder, cap(in))
	startTime := time.Now()
	done := 0

	go func() {
		defer close(out)
		for frame := range in {
			done++
			progress := float64(done) / float64(total)

			// Compute ETA.
			elapsed := time.Since(startTime)
			var eta time.Duration
			if done > 0 {
				perFrame := elapsed / time.Duration(done)
				eta = perFrame * time.Duration(total-done)
			}

			q.store.Update(jobID, func(j *Job) {
				j.Progress = progress
				j.ETA = eta.Round(time.Second).String()
			})

			out <- frame
		}
	}()

	return out
}

func (q *Queue) markDone(id, outputPath string) {
	now := time.Now()
	q.store.Update(id, func(j *Job) {
		j.Status = StatusDone
		j.Progress = 1.0
		j.ETA = "0s"
		j.OutputPath = outputPath
		j.DoneAt = &now
	})
	q.log.Info("job done", zap.String("id", id), zap.String("output", outputPath))
}

func (q *Queue) markFailed(id string, err error) {
	now := time.Now()
	q.store.Update(id, func(j *Job) {
		j.Status = StatusFailed
		j.Error = err.Error()
		j.DoneAt = &now
	})
	q.log.Error("job failed", zap.String("id", id), zap.Error(err))
}

// cleanupLoop periodically removes old MP4s from disk and prunes the job store.
func (q *Queue) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(q.opts.PruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			q.cleanup()
		case <-ctx.Done():
			return
		}
	}
}

func (q *Queue) cleanup() {
	// Prune job store.
	pruned := q.store.Prune(q.opts.RetentionPeriod)
	if pruned > 0 {
		q.log.Info("pruned old jobs", zap.Int("count", pruned))
	}

	// Remove MP4s older than retention period.
	cutoff := time.Now().Add(-q.opts.RetentionPeriod)
	entries, err := os.ReadDir(q.opts.OutputDir)
	if err != nil {
		return
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(q.opts.OutputDir, e.Name())
			os.Remove(path)
			removed++
		}
	}
	if removed > 0 {
		q.log.Info("cleaned up old MP4s", zap.Int("count", removed))
	}
}

// NewJob constructs a job from a composition, generating a unique ID.
func NewJob(comp *composition.Composition) *Job {
	return &Job{
		ID:        generateID(),
		Status:    StatusQueued,
		CreatedAt: time.Now(),
		Comp:      comp,
	}
}

// generateID produces a short, URL-safe unique ID.
func generateID() string {
	return fmt.Sprintf("j_%d", time.Now().UnixNano())
}
