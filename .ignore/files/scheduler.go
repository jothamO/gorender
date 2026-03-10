package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// FrameJob is a single unit of render work.
type FrameJob struct {
	Frame      int
	OutputPath string // e.g. /tmp/gorender/frame-000042.png
}

// FrameResult is the outcome of rendering a single frame.
type FrameResult struct {
	Frame      int
	OutputPath string
	Duration   time.Duration
	Err        error
}

// WorkerFunc is the function called by each worker to render one frame.
// It must write a PNG to job.OutputPath and return any error.
type WorkerFunc func(ctx context.Context, job FrameJob) error

// Scheduler distributes frame jobs across a pool of workers.
// Uses a shared channel so fast workers naturally drain more work (work-stealing).
type Scheduler struct {
	jobs       chan FrameJob
	results    chan FrameResult
	workerFunc WorkerFunc
	workers    int
	log        *zap.Logger

	pending  atomic.Int64
	rendered atomic.Int64
	failed   atomic.Int64
}

// Options configures the scheduler.
type Options struct {
	// Workers is the number of concurrent render goroutines.
	// Should match your browser pool size.
	Workers int

	// QueueDepth is the job channel buffer size.
	// Defaults to Workers * 4.
	QueueDepth int
}

// New creates a Scheduler. Call Run() to start processing.
func New(opts Options, fn WorkerFunc, log *zap.Logger) *Scheduler {
	if opts.Workers == 0 {
		opts.Workers = 4
	}
	if opts.QueueDepth == 0 {
		opts.QueueDepth = opts.Workers * 4
	}
	return &Scheduler{
		jobs:       make(chan FrameJob, opts.QueueDepth),
		results:    make(chan FrameResult, opts.QueueDepth),
		workerFunc: fn,
		workers:    opts.Workers,
		log:        log,
	}
}

// Submit enqueues frame jobs. Returns an error if the scheduler is full
// and ctx is cancelled before a slot opens.
func (s *Scheduler) Submit(ctx context.Context, jobs []FrameJob) error {
	for _, job := range jobs {
		select {
		case s.jobs <- job:
			s.pending.Add(1)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Run starts all workers and processes jobs until Close() is called.
// Results are streamed to the returned channel.
// The caller must drain Results() to prevent blocking.
func (s *Scheduler) Run(ctx context.Context) <-chan FrameResult {
	var wg sync.WaitGroup

	for i := 0; i < s.workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			s.runWorker(ctx, workerID)
		}(i)
	}

	// Close results channel when all workers finish.
	go func() {
		wg.Wait()
		close(s.results)
	}()

	return s.results
}

// Close signals workers to stop after draining remaining jobs.
func (s *Scheduler) Close() {
	close(s.jobs)
}

// Stats returns current render progress counters.
func (s *Scheduler) Stats() (pending, rendered, failed int64) {
	return s.pending.Load(), s.rendered.Load(), s.failed.Load()
}

// runWorker processes jobs from the shared channel until it's closed.
func (s *Scheduler) runWorker(ctx context.Context, id int) {
	for job := range s.jobs {
		select {
		case <-ctx.Done():
			s.results <- FrameResult{Frame: job.Frame, Err: ctx.Err()}
			return
		default:
		}

		start := time.Now()
		err := s.workerFunc(ctx, job)
		elapsed := time.Since(start)

		result := FrameResult{
			Frame:      job.Frame,
			OutputPath: job.OutputPath,
			Duration:   elapsed,
			Err:        err,
		}

		if err != nil {
			s.failed.Add(1)
			s.log.Error("frame render failed",
				zap.Int("worker", id),
				zap.Int("frame", job.Frame),
				zap.Duration("elapsed", elapsed),
				zap.Error(err),
			)
		} else {
			s.rendered.Add(1)
			s.log.Debug("frame rendered",
				zap.Int("worker", id),
				zap.Int("frame", job.Frame),
				zap.Duration("elapsed", elapsed),
			)
		}

		s.pending.Add(-1)
		s.results <- result
	}
}

// BuildJobs generates the full list of frame jobs for a composition.
// framePaths is a function that returns the output path for a given frame number.
func BuildJobs(totalFrames int, framePath func(frame int) string) []FrameJob {
	jobs := make([]FrameJob, totalFrames)
	for i := 0; i < totalFrames; i++ {
		jobs[i] = FrameJob{
			Frame:      i,
			OutputPath: framePath(i),
		}
	}
	return jobs
}

// Progress tracks render progress and provides ETA estimates.
type Progress struct {
	Total    int
	Done     int
	Failed   int
	Started  time.Time
	mu       sync.Mutex
}

func NewProgress(total int) *Progress {
	return &Progress{Total: total, Started: time.Now()}
}

func (p *Progress) Update(result FrameResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Done++
	if result.Err != nil {
		p.Failed++
	}
}

func (p *Progress) ETA() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Done == 0 {
		return 0
	}
	elapsed := time.Since(p.Started)
	perFrame := elapsed / time.Duration(p.Done)
	remaining := p.Total - p.Done
	return perFrame * time.Duration(remaining)
}

func (p *Progress) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	pct := float64(p.Done) / float64(p.Total) * 100
	return fmt.Sprintf("%.1f%% (%d/%d frames, %d failed, ETA %s)",
		pct, p.Done, p.Total, p.Failed, p.ETA().Round(time.Second))
}
