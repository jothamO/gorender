package gorender

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/makemoments/gorender/internal/browser"
	"github.com/makemoments/gorender/internal/cache"
	"github.com/makemoments/gorender/internal/composition"
	goffmpeg "github.com/makemoments/gorender/internal/ffmpeg"
	"github.com/makemoments/gorender/internal/pipeline"
	"go.uber.org/zap"
)

// RenderOptions configures a render job.
type RenderOptions struct {
	// Concurrency is the number of parallel browser instances.
	// Defaults to min(CPU count, 8).
	Concurrency int

	// TmpDir is where PNG frames are written during rendering.
	// Defaults to os.TempDir()/gorender-<timestamp>.
	TmpDir string

	// KeepFrames prevents cleanup of PNG frames after rendering.
	// Useful for debugging.
	KeepFrames bool

	// PrefetchAssets is a list of URLs to pre-warm the asset cache.
	PrefetchAssets []string

	// ChromeFlags are additional flags passed to Chrome.
	ChromeFlags []string

	// OnProgress is called after each frame is rendered.
	OnProgress func(done, total int, eta time.Duration)

	// FrameStep renders every Nth frame and fills gaps by frame hold.
	// 1 means render every frame.
	FrameStep int

	// EncodingProfile tunes ffmpeg defaults: "fast" or "final".
	EncodingProfile string

	// AutoDiscoverAudio probes the page for HTMLAudioElement src URLs and
	// auto-muxes the first track when comp.Audio is empty.
	AutoDiscoverAudio bool

	// CaptureFormat controls screenshot encoding from Chrome: "png" or "jpeg".
	CaptureFormat string

	// CaptureJPEGQuality controls JPEG screenshot quality when capture format is jpeg.
	// Range 1-100. Defaults to 82, or 90 for experimental final-profile renders.
	CaptureJPEGQuality int

	// ExperimentalPipeline enables opt-in rendering optimizations for A/B testing.
	// Defaults to false to preserve current stable behavior.
	ExperimentalPipeline bool

	// DurationSource controls how frames are derived when DurationFrames is not set.
	// Supported: "auto", "manual", "fixed".
	DurationSource composition.DurationSource
	// SlideDurationsMs supports manual duration-source mode.
	SlideDurationsMs []int
	// DefaultSlideMs is used as fallback when per-slide durations are missing.
	DefaultSlideMs int
	// Slides and SecondsPerSlide are used for fixed mode and auto fallback.
	Slides          int
	SecondsPerSlide float64
}

// Render is the top-level function. Load a composition, render all frames,
// and produce the final video file.
func Render(ctx context.Context, comp *composition.Composition, opts RenderOptions, log *zap.Logger) error {
	origPreset := comp.Output.Preset
	origCRF := comp.Output.CRF
	comp.Defaults()
	applyEncodingProfile(comp, opts.EncodingProfile, opts.ExperimentalPipeline, origPreset, origCRF)
	if opts.CaptureFormat == "" {
		opts.CaptureFormat = "png"
	}
	if opts.EncodingProfile == "fast" && opts.CaptureFormat == "png" {
		opts.CaptureFormat = "jpeg"
	}
	if opts.ExperimentalPipeline && opts.CaptureFormat == "png" && opts.EncodingProfile == "final" {
		// Experimental take: JPEG capture in final mode for faster capture throughput.
		opts.CaptureFormat = "jpeg"
	}
	if opts.CaptureJPEGQuality <= 0 || opts.CaptureJPEGQuality > 100 {
		opts.CaptureJPEGQuality = 82
	}
	if opts.ExperimentalPipeline && opts.CaptureFormat == "jpeg" && opts.EncodingProfile == "final" && opts.CaptureJPEGQuality < 90 {
		opts.CaptureJPEGQuality = 90
	}

	if opts.DurationSource == "" {
		opts.DurationSource = composition.DurationSourceAuto
	}
	if opts.DefaultSlideMs <= 0 {
		opts.DefaultSlideMs = 5000
	}
	if opts.SecondsPerSlide <= 0 {
		opts.SecondsPerSlide = float64(opts.DefaultSlideMs) / 1000.0
	}

	if err := validateCompositionBase(comp); err != nil {
		return fmt.Errorf("invalid composition: %w", err)
	}
	if err := goffmpeg.Check(); err != nil {
		return err
	}

	if opts.Concurrency == 0 {
		opts.Concurrency = min(runtime.NumCPU(), 8)
	}
	if opts.FrameStep <= 0 {
		opts.FrameStep = 1
	}
	if err := resolveDurationFrames(ctx, comp, opts, log); err != nil {
		return fmt.Errorf("resolving frame duration: %w", err)
	}
	if comp.DurationFrames <= 0 {
		return fmt.Errorf("durationFrames must be > 0")
	}

	// Set up temp directory for frames.
	tmpDir := opts.TmpDir
	if tmpDir == "" {
		tmpDir = filepath.Join(os.TempDir(), fmt.Sprintf("gorender-%d", time.Now().UnixMilli()))
	}
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("creating tmp dir: %w", err)
	}
	if !opts.KeepFrames {
		defer os.RemoveAll(tmpDir)
	}

	log.Info("starting render",
		zap.String("url", comp.URL),
		zap.Int("frames", comp.DurationFrames),
		zap.Int("frameStep", opts.FrameStep),
		zap.Int("fps", comp.FPS),
		zap.String("duration", comp.Duration().String()),
		zap.Int("workers", opts.Concurrency),
		zap.String("output", comp.Output.Path),
		zap.String("profile", opts.EncodingProfile),
		zap.String("captureFormat", opts.CaptureFormat),
		zap.Int("jpegQuality", opts.CaptureJPEGQuality),
		zap.Bool("experimentalPipeline", opts.ExperimentalPipeline),
		zap.String("durationSource", string(opts.DurationSource)),
	)

	// Ensure output directory exists.
	if err := goffmpeg.EnsureOutputDir(comp.Output.Path); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// Build shared asset cache.
	assetCache := cache.New(log)

	if len(opts.PrefetchAssets) > 0 {
		log.Info("prefetching assets", zap.Int("count", len(opts.PrefetchAssets)))
		if err := assetCache.Prefetch(ctx, opts.PrefetchAssets); err != nil {
			log.Warn("some assets failed to prefetch", zap.Error(err))
		}
	}

	// Launch browser pool.
	pool, err := browser.NewPool(ctx, browser.PoolOptions{
		Size:         opts.Concurrency,
		ChromeFlags:  opts.ChromeFlags,
		HeadlessNew:  true,
		WindowWidth:  comp.Width,
		WindowHeight: comp.Height,
	}, log)
	if err != nil {
		return fmt.Errorf("creating browser pool: %w", err)
	}
	defer pool.Close()

	// Enable asset interception on all browsers.
	// We acquire each browser just to set up interception, then release it.
	for i := 0; i < pool.Len(); i++ {
		b, err := pool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("acquiring browser for setup: %w", err)
		}
		if err := assetCache.EnableInterception(b.Context()); err != nil {
			pool.Release(b)
			return fmt.Errorf("enabling asset interception: %w", err)
		}
		pool.Release(b)
	}

	if opts.AutoDiscoverAudio && len(comp.Audio) == 0 {
		if tracks, derr := discoverAudioTracks(ctx, pool, comp); derr != nil {
			log.Warn("audio discovery failed", zap.Error(derr))
		} else if len(tracks) > 0 {
			comp.Audio = append(comp.Audio, tracks...)
			log.Info("audio discovered", zap.Int("tracks", len(tracks)), zap.String("src", tracks[0].Src))
		}
	}

	// Wire renderer and pipeline.
	renderer := pipeline.NewRenderer(pool, comp, opts.CaptureFormat, log)
	renderer.SetJPEGQuality(opts.CaptureJPEGQuality)
	renderPipeline := pipeline.NewRenderPipeline(renderer, comp, tmpDir, opts.KeepFrames, opts.FrameStep, log)
	renderPipeline.SetExperimental(opts.ExperimentalPipeline)

	// Run the render pipeline.
	start := time.Now()
	frames, errc := renderPipeline.Run(ctx, opts.Concurrency)
	if opts.OnProgress != nil {
		frames = withProgress(ctx, frames, comp.DurationFrames, opts.OnProgress)
	}

	// Stream frames into ffmpeg.
	writer := goffmpeg.NewWriter(comp, opts.CaptureFormat, log, opts.ExperimentalPipeline)
	if err := writer.Write(ctx, frames); err != nil {
		return fmt.Errorf("writing video: %w", err)
	}

	// Check for any render errors.
	if err := <-errc; err != nil {
		return fmt.Errorf("render pipeline: %w", err)
	}

	elapsed := time.Since(start)
	fps := float64(comp.DurationFrames) / elapsed.Seconds()

	log.Info("render complete",
		zap.String("output", comp.Output.Path),
		zap.Duration("elapsed", elapsed.Round(time.Millisecond)),
		zap.Float64("avg_fps", fps),
	)
	stats := renderer.Stats()
	if stats.Frames > 0 {
		log.Info("render timing split",
			zap.Int64("frames", stats.Frames),
			zap.Duration("browser_seek_total", stats.SeekDuration),
			zap.Duration("browser_ready_total", stats.ReadyDuration),
			zap.Duration("browser_screenshot_total", stats.ScreenshotDuration),
			zap.Duration("browser_seek_avg", stats.SeekDuration/time.Duration(stats.Frames)),
			zap.Duration("browser_ready_avg", stats.ReadyDuration/time.Duration(stats.Frames)),
			zap.Duration("browser_screenshot_avg", stats.ScreenshotDuration/time.Duration(stats.Frames)),
		)
	}

	// Verify the output with ffprobe.
	if info, err := goffmpeg.Probe(comp.Output.Path); err == nil {
		log.Info("output verified",
			zap.String("duration", info["format.duration"]),
			zap.String("size", info["format.size"]),
		)
	}

	return nil
}

func applyEncodingProfile(comp *composition.Composition, profile string, experimental bool, origPreset string, origCRF int) {
	switch profile {
	case "fast":
		if origPreset == "" {
			comp.Output.Preset = "veryfast"
		}
		if origCRF == 0 {
			comp.Output.CRF = 24
		}
	case "final", "":
		// Keep defaults / explicit composition settings.
	default:
		// Unknown profile: ignore, keep existing settings.
	}
	if experimental && origPreset == "" && profile != "fast" {
		// Isolated experiment: faster encoder preset with same CRF for speed A/B tests.
		// This is opt-in and never changes default stable behavior.
		comp.Output.Preset = "veryfast"
	}
}

func discoverAudioTracks(ctx context.Context, pool *browser.Pool, comp *composition.Composition) ([]composition.AudioTrack, error) {
	b, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer pool.Release(b)

	frameURL := buildFrameURL(comp.URL, comp.SeekParam, comp.FPS, 0)
	if err := chromedp.Run(
		b.Context(),
		chromedp.Navigate(frameURL),
		chromedp.EmulateViewport(int64(comp.Width), int64(comp.Height)),
	); err != nil {
		return nil, err
	}
	if err := waitForReadyExpr(b.Context(), comp.ReadySignal, comp.ReadyTimeout); err != nil {
		return nil, err
	}

	var urls []string
	js := `(function(){
		const out = [];
		for (const el of Array.from(document.querySelectorAll('audio'))) {
			const src = el.currentSrc || el.src || '';
			if (src) out.push(src);
		}
		return Array.from(new Set(out));
	})()`
	if err := chromedp.Run(b.Context(), chromedp.Evaluate(js, &urls)); err != nil {
		return nil, err
	}
	if len(urls) == 0 {
		return nil, nil
	}
	return []composition.AudioTrack{{Src: urls[0], StartFrame: 0, Volume: 1}}, nil
}

func waitForReadyExpr(ctx context.Context, signal string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var ready bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(signal, &ready)); err == nil && ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(16 * time.Millisecond):
		}
	}
	return fmt.Errorf("ready signal %q timed out after %s", signal, timeout)
}

func buildFrameURL(raw string, seekParam string, fps int, frame int) string {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Sprintf("%s?%s=%d&fps=%d", raw, seekParam, frame, fps)
	}
	q := u.Query()
	q.Set(seekParam, strconv.Itoa(frame))
	q.Set("fps", strconv.Itoa(fps))
	u.RawQuery = q.Encode()
	return u.String()
}

func withProgress(
	ctx context.Context,
	in <-chan pipeline.FrameInOrder,
	total int,
	onProgress func(done, total int, eta time.Duration),
) <-chan pipeline.FrameInOrder {
	out := make(chan pipeline.FrameInOrder, cap(in))
	const window = 30
	recent := make([]time.Time, 0, window+1)
	done := 0

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-in:
				if !ok {
					return
				}
				done++
				eta := time.Duration(0)
				now := time.Now()
				recent = append(recent, now)
				if len(recent) > window+1 {
					recent = recent[len(recent)-(window+1):]
				}
				if done >= 2 && len(recent) >= 2 {
					framesInWindow := len(recent) - 1
					windowElapsed := recent[len(recent)-1].Sub(recent[0])
					if framesInWindow > 0 && windowElapsed > 0 {
						eta = (windowElapsed / time.Duration(framesInWindow)) * time.Duration(total-done)
					}
				}
				onProgress(done, total, eta)
				out <- frame
			}
		}
	}()

	return out
}

func validateCompositionBase(comp *composition.Composition) error {
	if comp.URL == "" {
		return fmt.Errorf("url is required")
	}
	if comp.Output.Path == "" {
		return fmt.Errorf("output.path is required")
	}
	return nil
}

func resolveDurationFrames(ctx context.Context, comp *composition.Composition, opts RenderOptions, log *zap.Logger) error {
	if comp.DurationFrames > 0 {
		return nil
	}

	switch opts.DurationSource {
	case composition.DurationSourceManual:
		if len(opts.SlideDurationsMs) == 0 {
			return fmt.Errorf("manual duration source requires slideDurationsMs")
		}
		durations, err := composition.NormalizeDurationsMs(opts.SlideDurationsMs, opts.DefaultSlideMs)
		if err != nil {
			return err
		}
		frames, err := composition.ComputeTotalFramesFromDurationsMs(durations, comp.FPS)
		if err != nil {
			return err
		}
		comp.DurationFrames = frames
		log.Info("resolved duration frames", zap.String("source", "manual"), zap.Int("frames", frames), zap.Int("slides", len(durations)))
		return nil

	case composition.DurationSourceFixed:
		if opts.Slides <= 0 {
			return fmt.Errorf("fixed duration source requires slides > 0")
		}
		frames := int(math.Round(float64(opts.Slides) * opts.SecondsPerSlide * float64(comp.FPS)))
		if frames <= 0 {
			return fmt.Errorf("fixed duration computed non-positive frame count")
		}
		comp.DurationFrames = frames
		log.Info("resolved duration frames", zap.String("source", "fixed"), zap.Int("frames", frames), zap.Int("slides", opts.Slides))
		return nil

	case composition.DurationSourceAuto:
		durations, totalMs, err := discoverRenderMeta(ctx, comp, opts.ChromeFlags)
		if err != nil {
			log.Warn("auto duration discovery failed", zap.Error(err))
		}
		if len(durations) > 0 {
			norm, nerr := composition.NormalizeDurationsMs(durations, opts.DefaultSlideMs)
			if nerr != nil {
				log.Warn("auto duration normalization failed", zap.Error(nerr))
			} else {
				frames, ferr := composition.ComputeTotalFramesFromDurationsMs(norm, comp.FPS)
				if ferr == nil && frames > 0 {
					comp.DurationFrames = frames
					log.Info("resolved duration frames", zap.String("source", "auto_meta"), zap.Int("frames", frames), zap.Int("slides", len(norm)))
					return nil
				}
			}
		}
		if totalMs > 0 {
			frames := int(math.Ceil((float64(totalMs) / 1000.0) * float64(comp.FPS)))
			if frames > 0 {
				comp.DurationFrames = frames
				log.Info("resolved duration frames", zap.String("source", "auto_total"), zap.Int("frames", frames), zap.Int("totalMs", totalMs))
				return nil
			}
		}
		if opts.Slides > 0 {
			frames := int(math.Round(float64(opts.Slides) * opts.SecondsPerSlide * float64(comp.FPS)))
			if frames > 0 {
				comp.DurationFrames = frames
				log.Info("resolved duration frames", zap.String("source", "auto_fixed_fallback"), zap.Int("frames", frames), zap.Int("slides", opts.Slides))
				return nil
			}
		}
		return fmt.Errorf("auto duration source could not resolve frames")
	default:
		return fmt.Errorf("unsupported duration source %q", opts.DurationSource)
	}
}

func discoverRenderMeta(ctx context.Context, comp *composition.Composition, chromeFlags []string) ([]int, int, error) {
	log := zap.NewNop()
	pool, err := browser.NewPool(ctx, browser.PoolOptions{
		Size:         1,
		ChromeFlags:  chromeFlags,
		HeadlessNew:  true,
		WindowWidth:  comp.Width,
		WindowHeight: comp.Height,
	}, log)
	if err != nil {
		return nil, 0, err
	}
	defer pool.Close()

	b, err := pool.Acquire(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer pool.Release(b)

	if err := chromedp.Run(
		b.Context(),
		chromedp.Navigate(buildFrameURL(comp.URL, comp.SeekParam, comp.FPS, 0)),
		chromedp.EmulateViewport(int64(comp.Width), int64(comp.Height)),
	); err != nil {
		return nil, 0, err
	}

	waitTimeout := comp.ReadyTimeout
	if waitTimeout < 20*time.Second {
		waitTimeout = 20 * time.Second
	}
	// Metadata may become available before __READY__ flips; poll for either.
	deadline := time.Now().Add(waitTimeout)
	readyOrMeta := false
	for time.Now().Before(deadline) {
		var hasMeta bool
		if err := chromedp.Run(b.Context(), chromedp.Evaluate(`(function () {
			const m = window.__GORENDER_META__;
			return !!(m && typeof m === "object" && (Array.isArray(m.slideDurationsMs) || Number.isFinite(m.totalDurationMs)));
		})()`, &hasMeta)); err == nil && hasMeta {
			readyOrMeta = true
			break
		}
		var ready bool
		if err := chromedp.Run(b.Context(), chromedp.Evaluate(comp.ReadySignal, &ready)); err == nil && ready {
			readyOrMeta = true
			break
		}
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !readyOrMeta {
		var pageText string
		_ = chromedp.Run(b.Context(), chromedp.Evaluate(`(document && document.body && document.body.innerText) ? document.body.innerText : ""`, &pageText))
		if strings.Contains(strings.ToLower(pageText), "celebration not found") {
			return nil, 0, fmt.Errorf("render page reported: celebration not found (check slug URL)")
		}
		return nil, 0, fmt.Errorf("metadata/ready timed out after %s", waitTimeout)
	}

	var meta struct {
		Status           string    `json:"status"`
		SlideDurationsMs []float64 `json:"slideDurationsMs"`
		TotalDurationMs  float64   `json:"totalDurationMs"`
	}
	js := `(function () {
		const m = window.__GORENDER_META__;
		if (!m || typeof m !== "object") return { status: "", slideDurationsMs: [], totalDurationMs: 0 };
		const out = { status: "", slideDurationsMs: [], totalDurationMs: 0 };
		if (typeof m.status === "string") {
			out.status = m.status;
		}
		if (Array.isArray(m.slideDurationsMs)) {
			out.slideDurationsMs = m.slideDurationsMs.map((n) => Number(n)).filter((n) => Number.isFinite(n));
		}
		if (Number.isFinite(m.totalDurationMs)) {
			out.totalDurationMs = Number(m.totalDurationMs);
		}
		return out;
	})()`
	if err := chromedp.Run(b.Context(), chromedp.Evaluate(js, &meta)); err != nil {
		return nil, 0, err
	}
	durations := make([]int, 0, len(meta.SlideDurationsMs))
	for _, d := range meta.SlideDurationsMs {
		di := int(math.Round(d))
		if di > 0 {
			durations = append(durations, di)
		}
	}
	totalMs := int(math.Round(meta.TotalDurationMs))
	if strings.EqualFold(strings.TrimSpace(meta.Status), "not_found") {
		return nil, 0, fmt.Errorf("render page reported: celebration not found (check slug URL)")
	}
	if len(durations) == 0 && totalMs <= 0 {
		var pageText string
		_ = chromedp.Run(b.Context(), chromedp.Evaluate(`(document && document.body && document.body.innerText) ? document.body.innerText : ""`, &pageText))
		if strings.Contains(strings.ToLower(pageText), "celebration not found") {
			return nil, 0, fmt.Errorf("render page reported: celebration not found (check slug URL)")
		}
	}
	return durations, totalMs, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
