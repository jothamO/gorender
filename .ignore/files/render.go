package gorender

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

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
}

// Render is the top-level function. Load a composition, render all frames,
// and produce the final video file.
func Render(ctx context.Context, comp *composition.Composition, opts RenderOptions, log *zap.Logger) error {
	comp.Defaults()

	if err := validateComposition(comp); err != nil {
		return fmt.Errorf("invalid composition: %w", err)
	}
	if err := goffmpeg.Check(); err != nil {
		return err
	}

	if opts.Concurrency == 0 {
		opts.Concurrency = min(runtime.NumCPU(), 8)
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
		zap.Int("fps", comp.FPS),
		zap.String("duration", comp.Duration().String()),
		zap.Int("workers", opts.Concurrency),
		zap.String("output", comp.Output.Path),
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

	// Wire renderer and pipeline.
	renderer := pipeline.NewRenderer(pool, comp, log)
	renderPipeline := pipeline.NewRenderPipeline(renderer, comp, tmpDir, log)

	// Run the render pipeline.
	start := time.Now()
	frames, errc := renderPipeline.Run(ctx, opts.Concurrency)
	if opts.OnProgress != nil {
		frames = withProgress(ctx, frames, comp.DurationFrames, opts.OnProgress)
	}

	// Stream frames into ffmpeg.
	writer := goffmpeg.NewWriter(comp, log)
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

	// Verify the output with ffprobe.
	if info, err := goffmpeg.Probe(comp.Output.Path); err == nil {
		log.Info("output verified",
			zap.String("duration", info["format.duration"]),
			zap.String("size", info["format.size"]),
		)
	}

	return nil
}

func withProgress(
	ctx context.Context,
	in <-chan pipeline.FrameInOrder,
	total int,
	onProgress func(done, total int, eta time.Duration),
) <-chan pipeline.FrameInOrder {
	out := make(chan pipeline.FrameInOrder, cap(in))
	started := time.Now()
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
				elapsed := time.Since(started)
				eta := time.Duration(0)
				if done > 0 {
					eta = (elapsed / time.Duration(done)) * time.Duration(total-done)
				}
				onProgress(done, total, eta)
				out <- frame
			}
		}
	}()

	return out
}

func validateComposition(comp *composition.Composition) error {
	if comp.URL == "" {
		return fmt.Errorf("url is required")
	}
	if comp.DurationFrames <= 0 {
		return fmt.Errorf("durationFrames must be > 0")
	}
	if comp.Output.Path == "" {
		return fmt.Errorf("output.path is required")
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
