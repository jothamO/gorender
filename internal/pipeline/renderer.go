package pipeline

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/makemoments/gorender/internal/browser"
	"github.com/makemoments/gorender/internal/composition"
	"go.uber.org/zap"
)

// Renderer holds everything needed to render a single frame.
type Renderer struct {
	pool *browser.Pool
	comp *composition.Composition
	log  *zap.Logger
	mu   sync.Mutex
	// captureFormat is "png" (default) or "jpeg" for faster, lower-quality capture.
	captureFormat string
	jpegQuality   int
	// Sticky browser per worker to keep page state hot.
	sessions map[int]*workerSession
	stats    renderStats
	// frameOffset maps local render frames to absolute timeline frames.
	frameOffset int
}

type workerSession struct {
	b           *browser.Browser
	seekReady   bool
	viewportSet bool
	warmed      bool
}

type renderStats struct {
	frames          atomic.Int64
	seekNanos       atomic.Int64
	readyNanos      atomic.Int64
	screenshotNanos atomic.Int64
}

type StatsSnapshot struct {
	Frames             int64
	SeekDuration       time.Duration
	ReadyDuration      time.Duration
	ScreenshotDuration time.Duration
}

// NewRenderer creates a Renderer backed by the given pool.
func NewRenderer(pool *browser.Pool, comp *composition.Composition, captureFormat string, log *zap.Logger) *Renderer {
	if captureFormat == "" {
		captureFormat = "png"
	}
	return &Renderer{pool: pool, comp: comp, captureFormat: captureFormat, jpegQuality: 82, log: log, sessions: make(map[int]*workerSession)}
}

func (r *Renderer) SetJPEGQuality(quality int) {
	if quality <= 0 || quality > 100 {
		return
	}
	r.jpegQuality = quality
}

func (r *Renderer) SetFrameOffset(offset int) {
	if offset < 0 {
		return
	}
	r.frameOffset = offset
}

// RenderFrame acquires a worker-scoped browser, seeks to frame,
// waits for the ready signal, and captures a PNG screenshot.
func (r *Renderer) RenderFrame(ctx context.Context, workerID int, frame int) ([]byte, error) {
	s, err := r.ensureSession(ctx, workerID)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}

	absoluteFrame := frame + r.frameOffset

	seekStart := time.Now()
	if err := r.seekOrNavigate(s, absoluteFrame); err != nil {
		r.pool.MarkUnhealthy(s.b)
		r.dropSession(workerID)
		return nil, fmt.Errorf("frame %d (abs %d): %w", frame, absoluteFrame, err)
	}
	seekDur := time.Since(seekStart)

	// Guarded promotion path: expose deterministic timeline context directly in
	// runtime JS for frontends that consume per-frame slide timing metadata.
	if err := r.setRuntimeTimelineContext(s.b.Context(), absoluteFrame); err != nil {
		r.log.Debug("timeline runtime context skipped", zap.Int("frame", frame), zap.Error(err))
	}

	readyStart := time.Now()
	readyTimeout := r.comp.ReadyTimeout
	if !s.warmed && readyTimeout < 20*time.Second {
		readyTimeout = 20 * time.Second
	}
	err = chromedp.Run(s.b.Context(), waitForFrameReady(absoluteFrame, r.comp.ReadySignal, readyTimeout))
	readyDur := time.Since(readyStart)
	if err != nil {
		r.pool.MarkUnhealthy(s.b)
		r.dropSession(workerID)
		return nil, fmt.Errorf("frame %d (abs %d): %w", frame, absoluteFrame, err)
	}

	shotStart := time.Now()
	screenshot, err := r.captureFrame(s.b.Context())
	shotDur := time.Since(shotStart)
	if err != nil {
		r.pool.MarkUnhealthy(s.b)
		r.dropSession(workerID)
		return nil, fmt.Errorf("frame %d (abs %d): %w", frame, absoluteFrame, err)
	}

	s.warmed = true
	r.stats.frames.Add(1)
	r.stats.seekNanos.Add(seekDur.Nanoseconds())
	r.stats.readyNanos.Add(readyDur.Nanoseconds())
	r.stats.screenshotNanos.Add(shotDur.Nanoseconds())
	return screenshot, nil
}

func (r *Renderer) captureFrame(ctx context.Context) ([]byte, error) {
	if r.captureFormat == "jpeg" {
		var shot []byte
		err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			data, err := page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(int64(r.jpegQuality)).
				Do(ctx)
			if err != nil {
				return err
			}
			shot = data
			return nil
		}))
		if err != nil {
			return nil, err
		}
		return shot, nil
	}
	var screenshot []byte
	if err := chromedp.Run(ctx, chromedp.FullScreenshot(&screenshot, 100)); err != nil {
		return nil, err
	}
	return screenshot, nil
}

func (r *Renderer) seekOrNavigate(s *workerSession, absoluteFrame int) error {
	frameURL := r.buildFrameURL(absoluteFrame)

	if !s.seekReady {
		return chromedp.Run(s.b.Context(), chromedp.Navigate(frameURL))
	}

	var usedSeek bool
	seekScript := fmt.Sprintf(`(function() {
		try {
			window.__READY__ = false;
			if (typeof window.__GORENDER_SEEK_FRAME__ === "function") {
				window.__GORENDER_SEEK_FRAME__(%d);
				return true;
			}
			return false;
		} catch (e) {
			return false;
		}
	})()`, absoluteFrame)
	if err := chromedp.Run(s.b.Context(), chromedp.Evaluate(seekScript, &usedSeek)); err != nil {
		return err
	}
	if usedSeek {
		return nil
	}
	return chromedp.Run(s.b.Context(), chromedp.Navigate(frameURL))
}

func (r *Renderer) ensureSession(ctx context.Context, workerID int) (*workerSession, error) {
	r.mu.Lock()
	if s, ok := r.sessions[workerID]; ok && s.b != nil && s.b.IsHealthy() {
		r.mu.Unlock()
		return s, nil
	}
	r.mu.Unlock()

	b, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	s := &workerSession{b: b, seekReady: true}
	if err := chromedp.Run(s.b.Context(), chromedp.EmulateViewport(int64(r.comp.Width), int64(r.comp.Height))); err != nil {
		r.pool.Release(b)
		return nil, fmt.Errorf("set viewport: %w", err)
	}
	s.viewportSet = true
	r.mu.Lock()
	r.sessions[workerID] = s
	r.mu.Unlock()
	return s, nil
}

func (r *Renderer) dropSession(workerID int) {
	r.mu.Lock()
	s, ok := r.sessions[workerID]
	if ok {
		delete(r.sessions, workerID)
	}
	r.mu.Unlock()
	if ok && s.b != nil {
		r.pool.Release(s.b)
	}
}

func (r *Renderer) Close() {
	r.mu.Lock()
	sessions := make([]*workerSession, 0, len(r.sessions))
	for id, s := range r.sessions {
		delete(r.sessions, id)
		sessions = append(sessions, s)
	}
	r.mu.Unlock()
	for _, s := range sessions {
		if s != nil && s.b != nil {
			r.pool.Release(s.b)
		}
	}
}

func (r *Renderer) Stats() StatsSnapshot {
	return StatsSnapshot{
		Frames:             r.stats.frames.Load(),
		SeekDuration:       time.Duration(r.stats.seekNanos.Load()),
		ReadyDuration:      time.Duration(r.stats.readyNanos.Load()),
		ScreenshotDuration: time.Duration(r.stats.screenshotNanos.Load()),
	}
}

// buildFrameURL injects the frame number as a query parameter.
func (r *Renderer) buildFrameURL(frame int) string {
	u, err := url.Parse(r.comp.URL)
	if err != nil {
		if r.comp.FPS > 0 {
			return fmt.Sprintf("%s?%s=%d&fps=%d", r.comp.URL, r.comp.SeekParam, frame, r.comp.FPS)
		}
		return fmt.Sprintf("%s?%s=%d", r.comp.URL, r.comp.SeekParam, frame)
	}
	q := u.Query()
	q.Set(r.comp.SeekParam, strconv.Itoa(frame))
	if r.comp.FPS > 0 {
		q.Set("fps", strconv.Itoa(r.comp.FPS))
	}
	if loc, ok := r.frameTimeline(frame); ok {
		q.Set("gr_slide", strconv.Itoa(loc.SlideIndex))
		q.Set("gr_in_slide_ms", strconv.Itoa(loc.InSlideMs))
		q.Set("gr_slide_ms", strconv.Itoa(loc.SlideDurMs))
		q.Set("gr_t", strconv.FormatFloat(loc.SlideT, 'f', 6, 64))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (r *Renderer) frameTimeline(frame int) (composition.FrameLocation, bool) {
	if !r.comp.EmitTimelineQuery || len(r.comp.SlideDurationsMs) == 0 || r.comp.FPS <= 0 {
		return composition.FrameLocation{}, false
	}
	loc, err := composition.LocateFrameInDurations(r.comp.SlideDurationsMs, r.comp.FPS, frame)
	if err != nil {
		return composition.FrameLocation{}, false
	}
	return loc, true
}

func (r *Renderer) setRuntimeTimelineContext(ctx context.Context, frame int) error {
	loc, ok := r.frameTimeline(frame)
	if !ok {
		return nil
	}
	script := fmt.Sprintf(`(function () {
		window.__GORENDER_TIMELINE__ = {
			frame: %d,
			fps: %d,
			globalMs: %d,
			slide: %d,
			slideStartMs: %d,
			inSlideMs: %d,
			slideMs: %d,
			t: %.6f
		};
		return true;
	})()`,
		frame,
		r.comp.FPS,
		loc.GlobalMs,
		loc.SlideIndex,
		loc.SlideStartMs,
		loc.InSlideMs,
		loc.SlideDurMs,
		loc.SlideT,
	)
	var okEval bool
	return chromedp.Run(ctx, chromedp.Evaluate(script, &okEval))
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
			case <-time.After(16 * time.Millisecond):
			}
		}
		return fmt.Errorf("ready signal %q timed out after %s", signal, timeout)
	})
}

func waitForFrameReady(frame int, signal string, timeout time.Duration) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			var frameReady bool
			if err := chromedp.Evaluate(fmt.Sprintf("window.__FRAME_READY__ === %d", frame), &frameReady).Do(ctx); err == nil && frameReady {
				return nil
			}

			var ready bool
			if err := chromedp.Evaluate(signal, &ready).Do(ctx); err == nil && ready {
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(16 * time.Millisecond):
			}
		}
		return fmt.Errorf("frame %d ready timed out after %s", frame, timeout)
	})
}

// ReorderBuffer accepts frames arriving out of order and emits them sequentially.
type ReorderBuffer struct {
	mu     sync.Mutex
	buf    map[int]FrameInOrder // seq -> payload
	next   int
	total  int
	out    chan FrameInOrder
	closed bool
	clone  bool
}

// FrameInOrder is a frame sequenced for the writer.
type FrameInOrder struct {
	Frame int
	Path  string
	Bytes []byte
}

func NewReorderBuffer(total, outBuffer int) *ReorderBuffer {
	return NewReorderBufferWithClone(total, outBuffer, true)
}

func NewReorderBufferWithClone(total, outBuffer int, clone bool) *ReorderBuffer {
	if outBuffer == 0 {
		outBuffer = 64
	}
	return &ReorderBuffer{
		buf:   make(map[int]FrameInOrder, outBuffer),
		total: total,
		out:   make(chan FrameInOrder, outBuffer),
		clone: clone,
	}
}

func (rb *ReorderBuffer) Push(seq int, frame int, path string, data []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.closed {
		return
	}

	payload := data
	if rb.clone {
		cloned := make([]byte, len(data))
		copy(cloned, data)
		payload = cloned
	}
	rb.buf[seq] = FrameInOrder{Frame: frame, Path: path, Bytes: payload}

	for {
		item, ok := rb.buf[rb.next]
		if !ok {
			break
		}
		rb.out <- item
		delete(rb.buf, rb.next)
		rb.next++
	}

	if rb.next == rb.total {
		rb.closed = true
		close(rb.out)
	}
}

func (rb *ReorderBuffer) Out() <-chan FrameInOrder { return rb.out }

func (rb *ReorderBuffer) ForceClose() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.closed {
		return
	}
	rb.closed = true
	close(rb.out)
}

// RenderPipeline orchestrates worker rendering and sequencing.
type RenderPipeline struct {
	renderer   *Renderer
	comp       *composition.Composition
	tmpDir     string
	log        *zap.Logger
	keepFrames bool
	frameStep  int
	// experimental is opt-in and keeps stable defaults untouched.
	experimental bool
}

func NewRenderPipeline(renderer *Renderer, comp *composition.Composition, tmpDir string, keepFrames bool, frameStep int, log *zap.Logger) *RenderPipeline {
	if frameStep <= 0 {
		frameStep = 1
	}
	return &RenderPipeline{renderer: renderer, comp: comp, tmpDir: tmpDir, keepFrames: keepFrames, frameStep: frameStep, log: log}
}

func (rp *RenderPipeline) SetExperimental(enabled bool) {
	rp.experimental = enabled
}

func (rp *RenderPipeline) FramePath(frame int) string {
	return fmt.Sprintf("%s/frame-%06d.png", rp.tmpDir, frame)
}

func (rp *RenderPipeline) Run(ctx context.Context, workers int) (<-chan FrameInOrder, <-chan error) {
	errc := make(chan error, 1)
	if workers <= 0 {
		workers = 1
	}
	totalJobs := countFramesToRender(rp.comp.DurationFrames, rp.frameStep)
	reorder := NewReorderBufferWithClone(totalJobs, workers*2, !rp.experimental)
	renderCtx, cancel := context.WithCancel(ctx)
	chunks, offsets := buildWorkerChunks(rp.comp.DurationFrames, rp.frameStep, workers)

	go func() {
		defer close(errc)
		defer cancel()
		defer rp.renderer.Close()

		type result struct {
			seq   int
			frame int
			path  string
			data  []byte
			err   error
		}

		results := make(chan result, workers*2)
		var wg sync.WaitGroup
		for workerID, chunk := range chunks {
			if len(chunk) == 0 {
				continue
			}
			wg.Add(1)
			go func(workerID int, frames []int, baseSeq int) {
				defer wg.Done()
				for localSeq, frame := range frames {
					select {
					case <-renderCtx.Done():
						return
					default:
					}
					data, err := rp.renderer.RenderFrame(renderCtx, workerID, frame)
					path := rp.FramePath(frame)
					if err == nil && rp.keepFrames {
						if werr := os.WriteFile(path, data, 0644); werr != nil {
							err = fmt.Errorf("frame %d: writing png: %w", frame, werr)
						}
					}
					results <- result{seq: baseSeq + localSeq, frame: frame, path: path, data: data, err: err}
					if err != nil {
						return
					}
				}
			}(workerID, chunk, offsets[workerID])
		}
		go func() {
			wg.Wait()
			close(results)
		}()

		done := 0
		var firstErr error
		for res := range results {
			done++
			if res.err != nil {
				if firstErr == nil {
					firstErr = res.err
					rp.log.Error("frame failed", zap.Int("frame", res.frame), zap.Error(res.err))
					cancel()
					reorder.ForceClose()
				}
				continue
			}
			reorder.Push(res.seq, res.frame, res.path, res.data)

			if done%30 == 0 {
				pct := float64(done) / float64(totalJobs) * 100
				rp.log.Info("render progress", zap.String("status", fmt.Sprintf("%.1f%% (%d/%d frames)", pct, done, totalJobs)))
			}
		}

		if firstErr != nil {
			errc <- firstErr
		}
	}()

	return reorder.Out(), errc
}

func countFramesToRender(totalFrames int, step int) int {
	if totalFrames <= 0 {
		return 0
	}
	if step <= 1 {
		return totalFrames
	}
	n := 0
	for i := 0; i < totalFrames; i += step {
		n++
	}
	if (totalFrames-1)%step != 0 {
		n++
	}
	return n
}

func buildWorkerChunks(totalFrames int, step int, workers int) ([][]int, []int) {
	if workers <= 0 {
		workers = 1
	}
	frames := make([]int, 0, countFramesToRender(totalFrames, step))
	for i := 0; i < totalFrames; i += step {
		frames = append(frames, i)
	}
	if totalFrames > 0 && len(frames) > 0 && frames[len(frames)-1] != totalFrames-1 {
		frames = append(frames, totalFrames-1)
	}

	chunks := make([][]int, workers)
	offsets := make([]int, workers)
	if len(frames) == 0 {
		return chunks, offsets
	}
	base := len(frames) / workers
	extra := len(frames) % workers
	idx := 0
	seq := 0
	for w := 0; w < workers; w++ {
		size := base
		if w < extra {
			size++
		}
		offsets[w] = seq
		if size == 0 {
			continue
		}
		chunks[w] = append(chunks[w], frames[idx:idx+size]...)
		idx += size
		seq += size
	}
	return chunks, offsets
}
