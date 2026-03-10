package pipeline

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/makemoments/gorender/internal/browser"
	"github.com/makemoments/gorender/internal/composition"
	"github.com/makemoments/gorender/internal/scheduler"
	"go.uber.org/zap"
)

// Renderer holds everything needed to render a single frame.
type Renderer struct {
	pool *browser.Pool
	comp *composition.Composition
	log  *zap.Logger
}

// NewRenderer creates a Renderer backed by the given pool.
func NewRenderer(pool *browser.Pool, comp *composition.Composition, log *zap.Logger) *Renderer {
	return &Renderer{pool: pool, comp: comp, log: log}
}

// RenderFrame is a WorkerFunc — acquires a browser, seeks to the frame,
// waits for the ready signal, and captures a PNG screenshot.
func (r *Renderer) RenderFrame(ctx context.Context, job scheduler.FrameJob) error {
	b, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire browser: %w", err)
	}
	defer r.pool.Release(b)

	frameURL := r.buildFrameURL(job.Frame)

	var screenshot []byte
	err = chromedp.Run(b.Context(),
		// Navigate to the composition URL for this frame.
		chromedp.Navigate(frameURL),

		// Wait for the ready signal with a timeout.
		waitForReady(r.comp.ReadySignal, r.comp.ReadyTimeout),

		// Set the exact viewport size.
		chromedp.EmulateViewport(int64(r.comp.Width), int64(r.comp.Height)),

		// Capture the full viewport as PNG.
		chromedp.FullScreenshot(&screenshot, 100),
	)

	if err != nil {
		r.pool.MarkUnhealthy(b)
		return fmt.Errorf("frame %d: %w", job.Frame, err)
	}

	if err := os.WriteFile(job.OutputPath, screenshot, 0644); err != nil {
		return fmt.Errorf("frame %d: writing png: %w", job.Frame, err)
	}

	return nil
}

// buildFrameURL injects the frame number as a query parameter.
func (r *Renderer) buildFrameURL(frame int) string {
	return fmt.Sprintf("%s?%s=%d", r.comp.URL, r.comp.SeekParam, frame)
}

// waitForReady polls a JS expression until it returns true.
func waitForReady(signal string, timeout time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			var ready bool
			if err := chromedp.Evaluate(signal, &ready).Do(ctx); err == nil && ready {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(16 * time.Millisecond): // ~1 frame at 60fps
			}
		}
		return fmt.Errorf("ready signal %q timed out after %s", signal, timeout)
	})
}

// ReorderBuffer accepts frames arriving out of order and emits them
// sequentially. This lets the FFmpeg pipe receive frames in order
// without stalling the render workers.
//
// Thread-safe: Push may be called from multiple goroutines concurrently.
type ReorderBuffer struct {
	mu       sync.Mutex
	buf      map[int]string // frame -> path
	next     int            // next expected frame index
	total    int
	out      chan FrameInOrder
	once     sync.Once // ensures out is closed exactly once
}

// FrameInOrder is a frame that has been sequenced by the ReorderBuffer.
type FrameInOrder struct {
	Frame int
	Path  string
}

// NewReorderBuffer creates a buffer for `total` frames.
// outBuffer controls how many sequenced frames can queue before the
// consumer (ffmpeg writer) must drain them.
func NewReorderBuffer(total, outBuffer int) *ReorderBuffer {
	if outBuffer == 0 {
		outBuffer = 64
	}
	return &ReorderBuffer{
		buf:   make(map[int]string, outBuffer),
		total: total,
		out:   make(chan FrameInOrder, outBuffer),
	}
}

// Push records a completed frame and flushes any newly contiguous run
// to the output channel. Safe to call from multiple goroutines.
func (rb *ReorderBuffer) Push(frame int, path string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.buf[frame] = path

	// Flush the contiguous run starting at rb.next.
	for {
		p, ok := rb.buf[rb.next]
		if !ok {
			break
		}
		rb.out <- FrameInOrder{Frame: rb.next, Path: p}
		delete(rb.buf, rb.next)
		rb.next++
	}

	// Close the output channel exactly once when all frames are delivered.
	if rb.next == rb.total {
		rb.once.Do(func() { close(rb.out) })
	}
}

// Out returns the channel of sequenced frames.
func (rb *ReorderBuffer) Out() <-chan FrameInOrder {
	return rb.out
}

// RenderPipeline orchestrates the full render: scheduler + reorder buffer.
type RenderPipeline struct {
	renderer *Renderer
	comp     *composition.Composition
	tmpDir   string
	log      *zap.Logger
}

// NewRenderPipeline creates a pipeline. tmpDir is where PNG frames are written.
func NewRenderPipeline(renderer *Renderer, comp *composition.Composition, tmpDir string, log *zap.Logger) *RenderPipeline {
	return &RenderPipeline{renderer: renderer, comp: comp, tmpDir: tmpDir, log: log}
}

// FramePath returns the expected PNG path for a given frame.
func (rp *RenderPipeline) FramePath(frame int) string {
	return fmt.Sprintf("%s/frame-%06d.png", rp.tmpDir, frame)
}

// Run renders all frames and returns a channel of sequenced frame paths.
// The caller should pipe these directly into the FFmpeg writer.
func (rp *RenderPipeline) Run(ctx context.Context, workers int) (<-chan FrameInOrder, <-chan error) {
	errc := make(chan error, 1)
	reorder := NewReorderBuffer(rp.comp.DurationFrames, workers*2)

	sched := scheduler.New(
		scheduler.Options{Workers: workers},
		rp.renderer.RenderFrame,
		rp.log,
	)

	jobs := scheduler.BuildJobs(rp.comp.DurationFrames, rp.FramePath)

	go func() {
		defer close(errc)

		if err := sched.Submit(ctx, jobs); err != nil {
			errc <- err
			return
		}
		sched.Close()

		results := sched.Run(ctx)
		progress := scheduler.NewProgress(rp.comp.DurationFrames)

		var firstErr error
		for result := range results {
			progress.Update(result)
			if result.Err != nil {
				if firstErr == nil {
					firstErr = result.Err
				}
				rp.log.Error("frame failed", zap.Int("frame", result.Frame), zap.Error(result.Err))
				continue
			}
			reorder.Push(result.Frame, result.OutputPath)

			if result.Frame%30 == 0 {
				rp.log.Info("render progress", zap.String("status", progress.String()))
			}
		}

		if firstErr != nil {
			errc <- firstErr
		}
	}()

	return reorder.Out(), errc
}
