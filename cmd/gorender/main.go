package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/makemoments/gorender"
	"github.com/makemoments/gorender/internal/composition"
	goffmpeg "github.com/makemoments/gorender/internal/ffmpeg"
	"github.com/makemoments/gorender/internal/presets"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

var (
	version = "0.1.0"
	commit  = "dev"
)

func main() {
	root := buildRoot()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gorender",
		Short: "Fast, framework-agnostic video renderer for web compositions",
	}
	cmd.AddCommand(buildRender())
	cmd.AddCommand(buildBench())
	cmd.AddCommand(buildParity())
	cmd.AddCommand(buildVersion())
	cmd.AddCommand(buildCheck())
	return cmd
}

func buildRender() *cobra.Command {
	var (
		workers          int
		tmpDir           string
		keepFrames       bool
		prefetch         []string
		prefetchFile     string
		chromeFlags      []string
		quick            bool
		noAudioDiscovery bool
		frameStep        int
		profile          string
		preset           string
		captureFormat    string
		jpegQuality      int
		durationSource   string
		slideDurationsMS string
		defaultSlideMS   int
		verbose          bool
		// Inline flags (alternative to a comp file)
		url             string
		frames          int
		fps             int
		slides          int
		secondsPerSlide float64
		width           int
		height          int
		upscaleWidth    int
		upscaleHeight   int
		noUpscale       bool
		output          string
	)

	cmd := &cobra.Command{
		Use:   "render [composition.json|composition.yaml]",
		Short: "Render a composition to video",
		Example: `  # From a composition file
  gorender render my-comp.json

  # Inline (no file needed)
  gorender render --url http://localhost:3000/comp --frames 300 --fps 30 --out output.mp4

  # High-quality render with 8 workers
  gorender render my-comp.json --workers 8`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(verbose)
			defer log.Sync()

			var comp *composition.Composition
			selectedPreset := choosePreset(preset, profile)
			presetCfg, hasPreset := presets.Resolve(selectedPreset)

			if len(args) == 1 {
				c, err := loadComposition(args[0])
				if err != nil {
					return fmt.Errorf("loading composition: %w", err)
				}
				comp = c
			} else {
				// Build inline composition from flags.
				if url == "" {
					return fmt.Errorf("either a composition file or --url is required")
				}
				if fps <= 0 {
					return fmt.Errorf("--fps must be > 0")
				}
				if quick {
					if frames <= 0 {
						frames = 60
					}
					if frames > 60 {
						frames = 60
					}
					if frameStep <= 1 {
						frameStep = 2
					}
					if profile == "" {
						profile = "fast"
					}
					if captureFormat == "" {
						captureFormat = "jpeg"
					}
				}
				if frames <= 0 && durationSource == string(composition.DurationSourceManual) && strings.TrimSpace(slideDurationsMS) == "" {
					return fmt.Errorf("manual duration source requires --slide-durations-ms")
				}
				inlineWidth := width
				inlineHeight := height
				inlineUpscaleW := upscaleWidth
				inlineUpscaleH := upscaleHeight
				if hasPreset {
					if !cmd.Flags().Changed("width") && presetCfg.DefaultWidth > 0 {
						inlineWidth = presetCfg.DefaultWidth
					}
					if !cmd.Flags().Changed("height") && presetCfg.DefaultHeight > 0 {
						inlineHeight = presetCfg.DefaultHeight
					}
					if !cmd.Flags().Changed("upscale-width") && presetCfg.DefaultUpscaleWidth > 0 {
						inlineUpscaleW = presetCfg.DefaultUpscaleWidth
					}
					if !cmd.Flags().Changed("upscale-height") && presetCfg.DefaultUpscaleHeight > 0 {
						inlineUpscaleH = presetCfg.DefaultUpscaleHeight
					}
				}
				if noUpscale {
					inlineUpscaleW = 0
					inlineUpscaleH = 0
				}
				comp = &composition.Composition{
					URL:            url,
					DurationFrames: frames,
					FPS:            fps,
					Width:          inlineWidth,
					Height:         inlineHeight,
					Output: composition.OutputConfig{
						Path:          output,
						UpscaleWidth:  inlineUpscaleW,
						UpscaleHeight: inlineUpscaleH,
					},
				}
			}
			if hasPreset {
				if !cmd.Flags().Changed("capture-format") && presetCfg.CaptureFormat != "" {
					captureFormat = presetCfg.CaptureFormat
				}
				if !cmd.Flags().Changed("jpeg-quality") && presetCfg.CaptureJPEGQuality > 0 {
					jpegQuality = presetCfg.CaptureJPEGQuality
				}
				if comp.Output.Preset == "" && presetCfg.EncoderPreset != "" {
					comp.Output.Preset = presetCfg.EncoderPreset
				}
				if comp.Output.CRF == 0 && presetCfg.CRF > 0 {
					comp.Output.CRF = presetCfg.CRF
				}
			}
			if noUpscale {
				comp.Output.UpscaleWidth = 0
				comp.Output.UpscaleHeight = 0
			}

			manualDurations, err := composition.ParseDurationsCSV(slideDurationsMS)
			if err != nil {
				return fmt.Errorf("parsing --slide-durations-ms: %w", err)
			}

			if prefetchFile != "" {
				loaded, err := loadPrefetchFile(prefetchFile)
				if err != nil {
					return fmt.Errorf("loading prefetch file: %w", err)
				}
				prefetch = append(prefetch, loaded...)
			}
			prefetch = uniqueStrings(prefetch)

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			startTime := time.Now()
			var lastPrint time.Time
			effectiveWorkers := workers
			if quick && effectiveWorkers == 0 {
				effectiveWorkers = 2
			}

			err = gorender.Render(ctx, comp, gorender.RenderOptions{
				Concurrency:          effectiveWorkers,
				TmpDir:               tmpDir,
				KeepFrames:           keepFrames,
				FrameStep:            frameStep,
				EncodingProfile:      profile,
				CaptureFormat:        captureFormat,
				CaptureJPEGQuality:   jpegQuality,
				ExperimentalPipeline: true,
				DurationSource:       composition.DurationSource(durationSource),
				SlideDurationsMs:     manualDurations,
				DefaultSlideMs:       defaultSlideMS,
				Slides:               slides,
				SecondsPerSlide:      secondsPerSlide,
				AutoDiscoverAudio:    !noAudioDiscovery,
				PrefetchAssets:       prefetch,
				ChromeFlags:          chromeFlags,
				OnProgress: func(done, total int, eta time.Duration) {
					if time.Since(lastPrint) > 2*time.Second {
						pct := float64(done) / float64(total) * 100
						etaLabel := "estimating"
						if eta > 0 && done >= 3 {
							etaLabel = eta.Round(time.Second).String()
						}
						fmt.Fprintf(os.Stderr, "\r  %.1f%% (%d/%d frames) ETA %s    ",
							pct, done, total, etaLabel)
						lastPrint = time.Now()
					}
				},
			}, log)

			fmt.Fprintln(os.Stderr) // newline after progress
			if err != nil {
				return err
			}

			fmt.Printf("[ok] Rendered %d frames in %s -> %s\n",
				comp.DurationFrames,
				time.Since(startTime).Round(time.Millisecond),
				comp.Output.Path,
			)
			return nil
		},
	}

	cmd.Flags().IntVarP(&workers, "workers", "w", 0, "number of parallel browser instances (default: CPU count)")
	cmd.Flags().StringVar(&tmpDir, "tmp-dir", "", "directory for intermediate frame files")
	cmd.Flags().BoolVar(&keepFrames, "keep-frames", false, "keep PNG frames after rendering (for debugging)")
	cmd.Flags().StringArrayVar(&prefetch, "prefetch", nil, "URLs to prefetch into the asset cache")
	cmd.Flags().StringVar(&prefetchFile, "prefetch-file", "", "path to newline-delimited URLs to prefetch into the asset cache")
	cmd.Flags().StringArrayVar(&chromeFlags, "chrome-flag", nil, "extra Chrome flags (e.g. --chrome-flag=disable-web-security)")
	cmd.Flags().BoolVar(&quick, "quick", false, "fast iteration mode: caps inline renders to 60 frames and defaults workers to 2")
	cmd.Flags().BoolVar(&noAudioDiscovery, "no-audio-discovery", false, "disable auto-discovery/mux of page audio tracks")
	cmd.Flags().IntVar(&frameStep, "frame-step", 1, "render every Nth frame and hold previous frame in-between (speed/quality tradeoff)")
	cmd.Flags().StringVar(&profile, "profile", "final", "encoding profile: final|fast")
	cmd.Flags().StringVar(&preset, "preset", "", "named render preset (overrides --profile aliasing)")
	cmd.Flags().StringVar(&captureFormat, "capture-format", "", "browser capture format override: png|jpeg (default: png, or jpeg in fast profile)")
	cmd.Flags().IntVar(&jpegQuality, "jpeg-quality", 0, "JPEG screenshot quality (1-100) when using jpeg capture")
	cmd.Flags().StringVar(&durationSource, "duration-source", string(composition.DurationSourceAuto), "duration source: auto|manual|fixed")
	cmd.Flags().StringVar(&slideDurationsMS, "slide-durations-ms", "", "comma-separated slide durations in milliseconds (manual duration source)")
	cmd.Flags().IntVar(&defaultSlideMS, "default-slide-ms", 5000, "default slide duration in milliseconds")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	// Inline flags
	cmd.Flags().StringVar(&url, "url", "", "composition URL (inline mode)")
	cmd.Flags().IntVar(&frames, "frames", 0, "total frame count (inline mode)")
	cmd.Flags().IntVar(&fps, "fps", 30, "frames per second (inline mode)")
	cmd.Flags().IntVar(&slides, "slides", 0, "number of slides (inline mode, computes frames from slides * seconds-per-slide * fps)")
	cmd.Flags().Float64Var(&secondsPerSlide, "seconds-per-slide", 5, "seconds per slide for --slides frame calculation")
	cmd.Flags().IntVar(&width, "width", 720, "render width (inline mode)")
	cmd.Flags().IntVar(&height, "height", 1280, "render height (inline mode)")
	cmd.Flags().IntVar(&upscaleWidth, "upscale-width", 1080, "output upscale width (inline mode)")
	cmd.Flags().IntVar(&upscaleHeight, "upscale-height", 1920, "output upscale height (inline mode)")
	cmd.Flags().BoolVar(&noUpscale, "no-upscale", false, "disable output upscale and keep native render resolution")
	cmd.Flags().StringVarP(&output, "out", "o", "output.mp4", "output video path (inline mode)")

	return cmd
}

func buildBench() *cobra.Command {
	var (
		runs          int
		workers       int
		tmpDir        string
		keepFrames    bool
		prefetch      []string
		chromeFlags   []string
		preset        string
		noAudioDiscovery bool
		durationSource   string
		slideDurationsMS string
		defaultSlideMS   int
		verbose       bool
		outputDir     string
		continueOnErr bool
		// Inline flags (alternative to a comp file)
		url             string
		frames          int
		fps             int
		width           int
		height          int
		slides          int
		secondsPerSlide float64
	)

	cmd := &cobra.Command{
		Use:   "bench [composition.json|composition.yaml]",
		Short: "Render the same composition repeatedly and report throughput",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runs <= 0 {
				return fmt.Errorf("--runs must be > 0")
			}

			log := buildLogger(verbose)
			defer log.Sync()
			selectedPreset := choosePreset(preset, "")
			presetCfg, hasPreset := presets.Resolve(selectedPreset)

			var baseComp *composition.Composition
			if len(args) == 1 {
				c, err := loadComposition(args[0])
				if err != nil {
					return fmt.Errorf("loading composition: %w", err)
				}
				baseComp = c
			} else {
				if url == "" {
					return fmt.Errorf("either a composition file or --url is required")
				}
				if fps <= 0 {
					return fmt.Errorf("--fps must be > 0")
				}
				if frames <= 0 && durationSource == string(composition.DurationSourceManual) && strings.TrimSpace(slideDurationsMS) == "" {
					return fmt.Errorf("manual duration source requires --slide-durations-ms")
				}
				baseComp = &composition.Composition{
					URL:            url,
					DurationFrames: frames,
					FPS:            fps,
					Width:          width,
					Height:         height,
				}
			}
			baseComp.Defaults()
			manualDurations, err := composition.ParseDurationsCSV(slideDurationsMS)
			if err != nil {
				return fmt.Errorf("parsing --slide-durations-ms: %w", err)
			}

			if outputDir == "" {
				outputDir = filepath.Join(".", "bench-output")
			}
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("creating bench output directory: %w", err)
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			var successes int
			var failures int
			var totalDuration time.Duration
			var minDuration time.Duration
			var maxDuration time.Duration

			for i := 1; i <= runs; i++ {
				compCopy := *baseComp
				compCopy.Output = baseComp.Output
				compCopy.Output.Path = filepath.Join(outputDir, fmt.Sprintf("run-%03d.mp4", i))
				if hasPreset {
					if compCopy.Output.Preset == "" && presetCfg.EncoderPreset != "" {
						compCopy.Output.Preset = presetCfg.EncoderPreset
					}
					if compCopy.Output.CRF == 0 && presetCfg.CRF > 0 {
						compCopy.Output.CRF = presetCfg.CRF
					}
				}

				started := time.Now()
				err := gorender.Render(ctx, &compCopy, gorender.RenderOptions{
					Concurrency:          workers,
					TmpDir:               tmpDir,
					KeepFrames:           keepFrames,
					AutoDiscoverAudio:    !noAudioDiscovery,
					PrefetchAssets:       prefetch,
					ChromeFlags:          chromeFlags,
					DurationSource:       composition.DurationSource(durationSource),
					SlideDurationsMs:     manualDurations,
					DefaultSlideMs:       defaultSlideMS,
					Slides:               slides,
					SecondsPerSlide:      secondsPerSlide,
					ExperimentalPipeline: true,
					CaptureFormat:        presetCfg.CaptureFormat,
					CaptureJPEGQuality:   presetCfg.CaptureJPEGQuality,
				}, log)
				elapsed := time.Since(started)

				if err != nil {
					failures++
					fmt.Printf("run %d/%d failed in %s: %v\n", i, runs, elapsed.Round(time.Millisecond), err)
					if !continueOnErr {
						break
					}
					continue
				}

				successes++
				totalDuration += elapsed
				if minDuration == 0 || elapsed < minDuration {
					minDuration = elapsed
				}
				if elapsed > maxDuration {
					maxDuration = elapsed
				}
				runFPS := float64(compCopy.DurationFrames) / elapsed.Seconds()
				fmt.Printf("run %d/%d ok in %s (%.2f fps) -> %s\n",
					i, runs, elapsed.Round(time.Millisecond), runFPS, compCopy.Output.Path)
			}

			fmt.Println("---- benchmark summary ----")
			fmt.Printf("runs requested: %d\n", runs)
			fmt.Printf("successes: %d\n", successes)
			fmt.Printf("failures: %d\n", failures)
			if successes > 0 {
				avg := totalDuration / time.Duration(successes)
				avgFPS := float64(baseComp.DurationFrames) / avg.Seconds()
				fmt.Printf("avg duration: %s\n", avg.Round(time.Millisecond))
				fmt.Printf("min duration: %s\n", minDuration.Round(time.Millisecond))
				fmt.Printf("max duration: %s\n", maxDuration.Round(time.Millisecond))
				fmt.Printf("avg throughput: %.2f fps\n", avgFPS)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&runs, "runs", 3, "number of benchmark runs")
	cmd.Flags().IntVarP(&workers, "workers", "w", 0, "number of parallel browser instances")
	cmd.Flags().StringVar(&tmpDir, "tmp-dir", "", "directory for intermediate frame files")
	cmd.Flags().BoolVar(&keepFrames, "keep-frames", false, "keep PNG frames after rendering")
	cmd.Flags().StringArrayVar(&prefetch, "prefetch", nil, "URLs to prefetch into the asset cache")
	cmd.Flags().StringArrayVar(&chromeFlags, "chrome-flag", nil, "extra Chrome flags")
	cmd.Flags().StringVar(&preset, "preset", "", "named render preset to benchmark")
	cmd.Flags().BoolVar(&noAudioDiscovery, "no-audio-discovery", true, "disable auto-discovery/mux of page audio tracks")
	cmd.Flags().StringVar(&durationSource, "duration-source", string(composition.DurationSourceAuto), "duration source: auto|manual|fixed")
	cmd.Flags().StringVar(&slideDurationsMS, "slide-durations-ms", "", "comma-separated slide durations in milliseconds (manual duration source)")
	cmd.Flags().IntVar(&defaultSlideMS, "default-slide-ms", 5000, "default slide duration in milliseconds")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "directory where benchmark outputs are written")
	cmd.Flags().BoolVar(&continueOnErr, "continue-on-error", false, "continue remaining runs even if one run fails")

	// Inline flags
	cmd.Flags().StringVar(&url, "url", "", "composition URL (inline mode)")
	cmd.Flags().IntVar(&frames, "frames", 0, "total frame count (inline mode)")
	cmd.Flags().IntVar(&fps, "fps", 30, "frames per second (inline mode)")
	cmd.Flags().IntVar(&slides, "slides", 0, "number of slides (inline mode, computes frames from slides * seconds-per-slide * fps)")
	cmd.Flags().Float64Var(&secondsPerSlide, "seconds-per-slide", 5, "seconds per slide for --slides frame calculation")
	cmd.Flags().IntVar(&width, "width", 1080, "output width (inline mode)")
	cmd.Flags().IntVar(&height, "height", 1920, "output height (inline mode)")

	return cmd
}

func buildParity() *cobra.Command {
	var (
		workers          int
		tmpDir           string
		chromeFlags      []string
		profile          string
		preset           string
		captureFormat    string
		jpegQuality      int
		durationSource   string
		slideDurationsMS string
		defaultSlideMS   int
		noAudioDiscovery bool
		verbose          bool
		keepOutputs      bool
		outputDir        string
		minSSIM          float64
		minPSNR          float64
		targetSpeedup    float64
		// Inline flags
		url             string
		frames          int
		fps             int
		slides          int
		secondsPerSlide float64
		width           int
		height          int
		upscaleWidth    int
		upscaleHeight   int
		noUpscale       bool
	)

	cmd := &cobra.Command{
		Use:   "parity [composition.json|composition.yaml]",
		Short: "Run baseline vs experimental render and validate speed + visual parity",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(verbose)
			defer log.Sync()
			selectedPreset := choosePreset(preset, profile)
			presetCfg, hasPreset := presets.Resolve(selectedPreset)

			var comp *composition.Composition
			if len(args) == 1 {
				c, err := loadComposition(args[0])
				if err != nil {
					return fmt.Errorf("loading composition: %w", err)
				}
				comp = c
			} else {
				if url == "" {
					return fmt.Errorf("either a composition file or --url is required")
				}
				if fps <= 0 {
					return fmt.Errorf("--fps must be > 0")
				}
				if frames <= 0 && durationSource == string(composition.DurationSourceManual) && strings.TrimSpace(slideDurationsMS) == "" {
					return fmt.Errorf("manual duration source requires --slide-durations-ms")
				}
				inlineWidth := width
				inlineHeight := height
				inlineUpscaleW := upscaleWidth
				inlineUpscaleH := upscaleHeight
				if hasPreset {
					if !cmd.Flags().Changed("width") && presetCfg.DefaultWidth > 0 {
						inlineWidth = presetCfg.DefaultWidth
					}
					if !cmd.Flags().Changed("height") && presetCfg.DefaultHeight > 0 {
						inlineHeight = presetCfg.DefaultHeight
					}
					if !cmd.Flags().Changed("upscale-width") && presetCfg.DefaultUpscaleWidth > 0 {
						inlineUpscaleW = presetCfg.DefaultUpscaleWidth
					}
					if !cmd.Flags().Changed("upscale-height") && presetCfg.DefaultUpscaleHeight > 0 {
						inlineUpscaleH = presetCfg.DefaultUpscaleHeight
					}
				}
				if noUpscale {
					inlineUpscaleW = 0
					inlineUpscaleH = 0
				}
				comp = &composition.Composition{
					URL:            url,
					DurationFrames: frames,
					FPS:            fps,
					Width:          inlineWidth,
					Height:         inlineHeight,
					Output: composition.OutputConfig{
						Path:          "output.mp4",
						UpscaleWidth:  inlineUpscaleW,
						UpscaleHeight: inlineUpscaleH,
					},
				}
			}
			comp.Defaults()
			if hasPreset {
				if !cmd.Flags().Changed("capture-format") && presetCfg.CaptureFormat != "" {
					captureFormat = presetCfg.CaptureFormat
				}
				if !cmd.Flags().Changed("jpeg-quality") && presetCfg.CaptureJPEGQuality > 0 {
					jpegQuality = presetCfg.CaptureJPEGQuality
				}
				if comp.Output.Preset == "" && presetCfg.EncoderPreset != "" {
					comp.Output.Preset = presetCfg.EncoderPreset
				}
				if comp.Output.CRF == 0 && presetCfg.CRF > 0 {
					comp.Output.CRF = presetCfg.CRF
				}
			}
			if noUpscale {
				comp.Output.UpscaleWidth = 0
				comp.Output.UpscaleHeight = 0
			}
			manualDurations, err := composition.ParseDurationsCSV(slideDurationsMS)
			if err != nil {
				return fmt.Errorf("parsing --slide-durations-ms: %w", err)
			}

			if outputDir == "" {
				outputDir = filepath.Join(".", "output", "parity-check")
			}
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("creating output dir: %w", err)
			}
			ts := time.Now().Format("20060102-150405")
			baseOut := filepath.Join(outputDir, fmt.Sprintf("baseline-%s.mp4", ts))
			expOut := filepath.Join(outputDir, fmt.Sprintf("experimental-%s.mp4", ts))

			baseComp := *comp
			baseComp.Output = comp.Output
			baseComp.Output.Path = baseOut

			expComp := *comp
			expComp.Output = comp.Output
			expComp.Output.Path = expOut

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			startBase := time.Now()
			if err := gorender.Render(ctx, &baseComp, gorender.RenderOptions{
				Concurrency:          workers,
				TmpDir:               tmpDir,
				FrameStep:            1,
				EncodingProfile:      profile,
				CaptureFormat:        captureFormat,
				CaptureJPEGQuality:   jpegQuality,
				ExperimentalPipeline: false,
				DurationSource:       composition.DurationSource(durationSource),
				SlideDurationsMs:     manualDurations,
				DefaultSlideMs:       defaultSlideMS,
				Slides:               slides,
				SecondsPerSlide:      secondsPerSlide,
				AutoDiscoverAudio:    !noAudioDiscovery,
				ChromeFlags:          chromeFlags,
			}, log); err != nil {
				return fmt.Errorf("baseline render failed: %w", err)
			}
			baseDur := time.Since(startBase)

			startExp := time.Now()
			if err := gorender.Render(ctx, &expComp, gorender.RenderOptions{
				Concurrency:          workers,
				TmpDir:               tmpDir,
				FrameStep:            1,
				EncodingProfile:      profile,
				CaptureFormat:        captureFormat,
				CaptureJPEGQuality:   jpegQuality,
				ExperimentalPipeline: true,
				DurationSource:       composition.DurationSource(durationSource),
				SlideDurationsMs:     manualDurations,
				DefaultSlideMs:       defaultSlideMS,
				Slides:               slides,
				SecondsPerSlide:      secondsPerSlide,
				AutoDiscoverAudio:    !noAudioDiscovery,
				ChromeFlags:          chromeFlags,
			}, log); err != nil {
				return fmt.Errorf("experimental render failed: %w", err)
			}
			expDur := time.Since(startExp)

			ssim, psnr, err := computeParityMetrics(ctx, baseOut, expOut)
			if err != nil {
				return fmt.Errorf("computing parity metrics: %w", err)
			}

			speedup := 0.0
			if baseDur > 0 {
				speedup = 1.0 - (float64(expDur) / float64(baseDur))
			}

			fmt.Printf("baseline:     %s\n", baseDur.Round(time.Millisecond))
			fmt.Printf("experimental: %s\n", expDur.Round(time.Millisecond))
			fmt.Printf("speedup:      %.2f%%\n", speedup*100)
			fmt.Printf("ssim(all):    %.6f\n", ssim)
			fmt.Printf("psnr(avg):    %.3f dB\n", psnr)
			fmt.Printf("baseline out: %s\n", baseOut)
			fmt.Printf("exper out:    %s\n", expOut)

			if !keepOutputs {
				defer os.Remove(baseOut)
				defer os.Remove(expOut)
			}

			if ssim < minSSIM {
				return fmt.Errorf("parity failed: ssim %.6f < min %.6f", ssim, minSSIM)
			}
			if psnr < minPSNR {
				return fmt.Errorf("parity failed: psnr %.3f < min %.3f", psnr, minPSNR)
			}
			if speedup < targetSpeedup {
				return fmt.Errorf("speed target failed: %.2f%% < target %.2f%%", speedup*100, targetSpeedup*100)
			}
			fmt.Println("[ok] parity and speed targets passed")
			return nil
		},
	}

	cmd.Flags().IntVarP(&workers, "workers", "w", 0, "number of parallel browser instances (default: CPU count)")
	cmd.Flags().StringVar(&tmpDir, "tmp-dir", "", "directory for intermediate frame files")
	cmd.Flags().StringArrayVar(&chromeFlags, "chrome-flag", nil, "extra Chrome flags")
	cmd.Flags().StringVar(&profile, "profile", "final", "encoding profile: final|fast")
	cmd.Flags().StringVar(&preset, "preset", "", "named render preset for parity runs")
	cmd.Flags().StringVar(&captureFormat, "capture-format", "", "browser capture format override: png|jpeg")
	cmd.Flags().IntVar(&jpegQuality, "jpeg-quality", 0, "JPEG screenshot quality (1-100) when using jpeg capture")
	cmd.Flags().StringVar(&durationSource, "duration-source", string(composition.DurationSourceAuto), "duration source: auto|manual|fixed")
	cmd.Flags().StringVar(&slideDurationsMS, "slide-durations-ms", "", "comma-separated slide durations in milliseconds (manual duration source)")
	cmd.Flags().IntVar(&defaultSlideMS, "default-slide-ms", 5000, "default slide duration in milliseconds")
	cmd.Flags().BoolVar(&noAudioDiscovery, "no-audio-discovery", false, "disable auto-discovery/mux of page audio tracks")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	cmd.Flags().BoolVar(&keepOutputs, "keep-outputs", true, "keep baseline/experimental output files")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "directory for parity outputs")
	cmd.Flags().Float64Var(&minSSIM, "min-ssim", 0.995, "minimum SSIM(all) threshold")
	cmd.Flags().Float64Var(&minPSNR, "min-psnr", 40.0, "minimum PSNR(avg) threshold in dB")
	cmd.Flags().Float64Var(&targetSpeedup, "target-speedup", 0.30, "minimum fractional speedup target for experimental run")

	cmd.Flags().StringVar(&url, "url", "", "composition URL (inline mode)")
	cmd.Flags().IntVar(&frames, "frames", 0, "total frame count (inline mode)")
	cmd.Flags().IntVar(&fps, "fps", 30, "frames per second (inline mode)")
	cmd.Flags().IntVar(&slides, "slides", 0, "number of slides (inline mode, computes frames from slides * seconds-per-slide * fps)")
	cmd.Flags().Float64Var(&secondsPerSlide, "seconds-per-slide", 5, "seconds per slide for --slides frame calculation")
	cmd.Flags().IntVar(&width, "width", 720, "render width (inline mode)")
	cmd.Flags().IntVar(&height, "height", 1280, "render height (inline mode)")
	cmd.Flags().IntVar(&upscaleWidth, "upscale-width", 1080, "output upscale width (inline mode)")
	cmd.Flags().IntVar(&upscaleHeight, "upscale-height", 1920, "output upscale height (inline mode)")
	cmd.Flags().BoolVar(&noUpscale, "no-upscale", false, "disable output upscale and keep native render resolution")
	return cmd
}

func buildVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gorender %s (%s)\n", version, commit)
		},
	}
}

func buildCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Verify that ffmpeg, ffprobe, and Chrome are available",
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := []struct {
				name string
				fn   func() error
			}{
				{"ffmpeg/ffprobe", func() error {
					return goffmpeg.Check()
				}},
				{"chrome/chromium", func() error {
					candidates := []string{"google-chrome", "chromium", "chromium-browser", "chrome", "chrome.exe", "msedge", "msedge.exe"}
					var found []string
					for _, bin := range candidates {
						if _, err := exec.LookPath(bin); err == nil {
							found = append(found, bin)
						}
					}
					if len(found) == 0 {
						return fmt.Errorf("no chrome/chromium binary found in PATH (tried: %s)", strings.Join(candidates, ", "))
					}
					return nil
				}},
			}
			allOK := true
			for _, c := range checks {
				if err := c.fn(); err != nil {
					fmt.Printf("  [x] %s: %v\n", c.name, err)
					allOK = false
				} else {
					fmt.Printf("  [ok] %s\n", c.name)
				}
			}
			if !allOK {
				return fmt.Errorf("some checks failed")
			}
			fmt.Println("All checks passed.")
			return nil
		},
	}
}

// loadComposition reads a JSON or YAML composition file.
func loadComposition(path string) (*composition.Composition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var comp composition.Composition
	switch {
	case hasExt(path, ".json"):
		if err := json.Unmarshal(data, &comp); err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
	case hasExt(path, ".yaml", ".yml"):
		if err := yaml.Unmarshal(data, &comp); err != nil {
			return nil, fmt.Errorf("parsing YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported file format (expected .json, .yaml, or .yml)")
	}

	return &comp, nil
}

func loadPrefetchFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var urls []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		v := strings.TrimSpace(s)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func hasExt(path string, exts ...string) bool {
	gotExt := strings.ToLower(filepath.Ext(path))
	for _, ext := range exts {
		if gotExt == strings.ToLower(ext) {
			return true
		}
	}
	return false
}

func computeParityMetrics(ctx context.Context, baselinePath string, experimentalPath string) (float64, float64, error) {
	args := []string{
		"-i", baselinePath,
		"-i", experimentalPath,
		"-lavfi", "[0:v][1:v]ssim;[0:v][1:v]psnr",
		"-f", "null",
		"-",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("ffmpeg metric run failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	ssimRe := regexp.MustCompile(`All:([0-9]*\.?[0-9]+)`)
	psnrRe := regexp.MustCompile(`average:([0-9]*\.?[0-9]+)`)
	text := string(out)
	ssimMatches := ssimRe.FindAllStringSubmatch(text, -1)
	psnrMatches := psnrRe.FindAllStringSubmatch(text, -1)
	if len(ssimMatches) == 0 || len(psnrMatches) == 0 {
		return 0, 0, fmt.Errorf("unable to parse ffmpeg SSIM/PSNR output")
	}

	ssim, serr := strconv.ParseFloat(ssimMatches[len(ssimMatches)-1][1], 64)
	if serr != nil {
		return 0, 0, fmt.Errorf("parsing ssim: %w", serr)
	}
	psnr, perr := strconv.ParseFloat(psnrMatches[len(psnrMatches)-1][1], 64)
	if perr != nil {
		return 0, 0, fmt.Errorf("parsing psnr: %w", perr)
	}
	return ssim, psnr, nil
}

func choosePreset(preset string, profile string) string {
	if strings.TrimSpace(preset) != "" {
		return strings.TrimSpace(preset)
	}
	alias := presets.AliasedProfile(profile)
	if alias != "" {
		return alias
	}
	return ""
}

func buildLogger(verbose bool) *zap.Logger {
	level := zapcore.InfoLevel
	if verbose {
		level = zapcore.DebugLevel
	}
	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	log, _ := cfg.Build()
	return log
}
