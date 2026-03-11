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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/makemoments/gorender"
	"github.com/makemoments/gorender/internal/composition"
	"github.com/makemoments/gorender/internal/distributed"
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
	cmd.AddCommand(buildShardPlan())
	cmd.AddCommand(buildConcat())
	cmd.AddCommand(buildExport())
	cmd.AddCommand(buildWarmup())
	cmd.AddCommand(buildBench())
	cmd.AddCommand(buildParity())
	cmd.AddCommand(buildVersion())
	cmd.AddCommand(buildCheck())
	return cmd
}

func buildRender() *cobra.Command {
	var (
		workers            int
		tmpDir             string
		keepFrames         bool
		prefetch           []string
		prefetchFile       string
		chromeFlags        []string
		quick              bool
		noAudioDiscovery   bool
		frameStep          int
		profile            string
		preset             string
		captureFormat      string
		jpegQuality        int
		durationSource     string
		slideDurationsMS   string
		defaultSlideMS     int
		doWarmup           bool
		warmupFrame        int
		warmupReadyTimeout time.Duration
		warmupNoReadyCheck bool
		timelineResolver   bool
		frameOffset        int
		verbose            bool
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
			if doWarmup {
				warmComp := &composition.Composition{
					URL:          comp.URL,
					FPS:          comp.FPS,
					Width:        comp.Width,
					Height:       comp.Height,
					SeekParam:    comp.SeekParam,
					ReadySignal:  comp.ReadySignal,
					ReadyTimeout: warmupReadyTimeout,
				}
				if err := gorender.Warmup(ctx, warmComp, gorender.WarmupOptions{
					Concurrency:    effectiveWorkers,
					PrefetchAssets: prefetch,
					ChromeFlags:    chromeFlags,
					Frame:          warmupFrame,
					SkipReadyCheck: warmupNoReadyCheck,
				}, log); err != nil {
					return fmt.Errorf("warmup failed: %w", err)
				}
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
				UseTimelineResolver:  timelineResolver,
				FrameOffset:          frameOffset,
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
	cmd.Flags().BoolVar(&doWarmup, "warmup", false, "run a browser/asset warmup pass before rendering")
	cmd.Flags().IntVar(&warmupFrame, "warmup-frame", 0, "frame index used for warmup probe")
	cmd.Flags().DurationVar(&warmupReadyTimeout, "warmup-ready-timeout", 20*time.Second, "ready signal timeout during render warmup")
	cmd.Flags().BoolVar(&warmupNoReadyCheck, "warmup-no-ready-check", false, "skip waiting for ready signal during render warmup")
	cmd.Flags().BoolVar(&timelineResolver, "timeline-resolver", false, "enable guarded deterministic timeline query hints (gr_slide/gr_t) in frame URLs")
	cmd.Flags().IntVar(&frameOffset, "frame-offset", 0, "absolute frame offset applied to frontend frame query (distributed shard rendering)")
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

func buildShardPlan() *cobra.Command {
	var (
		frames int
		shards int
		output string
		pretty bool
	)
	cmd := &cobra.Command{
		Use:   "shard-plan",
		Short: "Build contiguous frame shards for distributed rendering",
		RunE: func(cmd *cobra.Command, args []string) error {
			if frames <= 0 {
				return fmt.Errorf("--frames must be > 0")
			}
			if shards <= 0 {
				return fmt.Errorf("--shards must be > 0")
			}
			plan, err := distributed.BuildShards(frames, shards)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"totalFrames": frames,
				"shards":      len(plan),
				"ranges":      plan,
			}
			var b []byte
			if pretty {
				b, err = json.MarshalIndent(payload, "", "  ")
			} else {
				b, err = json.Marshal(payload)
			}
			if err != nil {
				return fmt.Errorf("encoding shard plan: %w", err)
			}
			if output == "" {
				fmt.Println(string(b))
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}
			if err := os.WriteFile(output, b, 0644); err != nil {
				return fmt.Errorf("writing shard plan: %w", err)
			}
			fmt.Printf("[ok] wrote shard plan -> %s\n", output)
			return nil
		},
	}
	cmd.Flags().IntVar(&frames, "frames", 0, "total frame count")
	cmd.Flags().IntVar(&shards, "shards", 0, "number of shards to produce")
	cmd.Flags().StringVar(&output, "out", "", "optional path to write plan JSON")
	cmd.Flags().BoolVar(&pretty, "pretty", true, "pretty-print JSON output")
	return cmd
}

func buildConcat() *cobra.Command {
	var (
		inputs  []string
		listIn  string
		output  string
		verbose bool
	)
	cmd := &cobra.Command{
		Use:   "concat",
		Short: "Concat shard videos into a final output MP4",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := goffmpeg.Check(); err != nil {
				return err
			}
			if output == "" {
				return fmt.Errorf("--out is required")
			}
			if listIn != "" {
				loaded, err := loadPrefetchFile(listIn)
				if err != nil {
					return fmt.Errorf("loading --list file: %w", err)
				}
				inputs = append(inputs, loaded...)
			}
			inputs = uniqueStrings(inputs)
			if len(inputs) < 2 {
				return fmt.Errorf("at least two inputs are required")
			}

			for _, p := range inputs {
				if _, err := os.Stat(p); err != nil {
					return fmt.Errorf("input missing %q: %w", p, err)
				}
			}
			if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				return fmt.Errorf("creating output dir: %w", err)
			}

			tmpList := filepath.Join(os.TempDir(), fmt.Sprintf("gorender-concat-%d.txt", time.Now().UnixNano()))
			if err := writeConcatList(tmpList, inputs); err != nil {
				return fmt.Errorf("writing concat list: %w", err)
			}
			defer os.Remove(tmpList)

			ffArgs := []string{
				"-y",
				"-f", "concat",
				"-safe", "0",
				"-i", tmpList,
				"-c", "copy",
				output,
			}
			ff := exec.Command("ffmpeg", ffArgs...)
			if verbose {
				ff.Stdout = os.Stdout
				ff.Stderr = os.Stderr
			}
			if err := ff.Run(); err != nil {
				return fmt.Errorf("ffmpeg concat failed: %w", err)
			}

			fmt.Printf("[ok] concatenated %d shard videos -> %s\n", len(inputs), output)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&inputs, "input", nil, "input shard video path (repeatable, in order)")
	cmd.Flags().StringVar(&listIn, "list", "", "newline-delimited input list file (in order)")
	cmd.Flags().StringVarP(&output, "out", "o", "", "final output file path")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show ffmpeg output")
	return cmd
}

type exportCommonFlags struct {
	url              string
	frames           int
	fps              int
	slides           int
	secondsPerSlide  float64
	width            int
	height           int
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
	timelineResolver bool
	verbose          bool
}

func buildExport() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export output variants (still, sequence, gif, audio-only)",
	}
	cmd.AddCommand(buildExportStill())
	cmd.AddCommand(buildExportSequence())
	cmd.AddCommand(buildExportGIF())
	cmd.AddCommand(buildExportAudio())
	return cmd
}

func buildExportStill() *cobra.Command {
	var c exportCommonFlags
	var (
		out         string
		frameNumber int
	)
	cmd := &cobra.Command{
		Use:   "still",
		Short: "Render and export a still image from a selected frame",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(out) == "" {
				return fmt.Errorf("--out is required")
			}
			if frameNumber < 0 {
				return fmt.Errorf("--frame must be >= 0")
			}
			log := buildLogger(c.verbose)
			defer log.Sync()
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			tempVideo, err := renderVariantTempVideo(ctx, &c, log)
			if err != nil {
				return err
			}
			defer os.Remove(tempVideo)

			if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
				return fmt.Errorf("creating output dir: %w", err)
			}
			filter := fmt.Sprintf("select=eq(n\\,%d)", frameNumber)
			ffArgs := []string{"-y", "-i", tempVideo, "-vf", filter, "-vframes", "1", out}
			if err := runFFmpeg(ctx, ffArgs, c.verbose); err != nil {
				return err
			}
			fmt.Printf("[ok] still exported -> %s\n", out)
			return nil
		},
	}
	applyExportCommonFlags(cmd, &c)
	cmd.Flags().StringVarP(&out, "out", "o", "", "output image path (.png/.jpg)")
	cmd.Flags().IntVar(&frameNumber, "frame", 0, "absolute frame number to export from rendered output")
	return cmd
}

func buildExportSequence() *cobra.Command {
	var c exportCommonFlags
	var (
		outDir  string
		pattern string
	)
	cmd := &cobra.Command{
		Use:   "sequence",
		Short: "Render and export an image sequence",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(outDir) == "" {
				return fmt.Errorf("--out-dir is required")
			}
			log := buildLogger(c.verbose)
			defer log.Sync()
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			tempVideo, err := renderVariantTempVideo(ctx, &c, log)
			if err != nil {
				return err
			}
			defer os.Remove(tempVideo)

			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("creating output dir: %w", err)
			}
			outPattern := filepath.Join(outDir, pattern)
			ffArgs := []string{"-y", "-i", tempVideo, outPattern}
			if err := runFFmpeg(ctx, ffArgs, c.verbose); err != nil {
				return err
			}
			fmt.Printf("[ok] image sequence exported -> %s\n", outPattern)
			return nil
		},
	}
	applyExportCommonFlags(cmd, &c)
	cmd.Flags().StringVar(&outDir, "out-dir", "", "output directory for images")
	cmd.Flags().StringVar(&pattern, "pattern", "frame-%06d.png", "image filename pattern")
	return cmd
}

func buildExportGIF() *cobra.Command {
	var c exportCommonFlags
	var (
		out    string
		gifFPS int
	)
	cmd := &cobra.Command{
		Use:   "gif",
		Short: "Render and export GIF",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(out) == "" {
				return fmt.Errorf("--out is required")
			}
			if gifFPS <= 0 {
				return fmt.Errorf("--gif-fps must be > 0")
			}
			log := buildLogger(c.verbose)
			defer log.Sync()
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			tempVideo, err := renderVariantTempVideo(ctx, &c, log)
			if err != nil {
				return err
			}
			defer os.Remove(tempVideo)

			if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
				return fmt.Errorf("creating output dir: %w", err)
			}
			ffArgs := []string{"-y", "-i", tempVideo, "-vf", fmt.Sprintf("fps=%d", gifFPS), out}
			if err := runFFmpeg(ctx, ffArgs, c.verbose); err != nil {
				return err
			}
			fmt.Printf("[ok] gif exported -> %s\n", out)
			return nil
		},
	}
	applyExportCommonFlags(cmd, &c)
	cmd.Flags().StringVarP(&out, "out", "o", "", "output gif path")
	cmd.Flags().IntVar(&gifFPS, "gif-fps", 15, "output gif fps")
	return cmd
}

func buildExportAudio() *cobra.Command {
	var c exportCommonFlags
	var (
		out   string
		codec string
	)
	cmd := &cobra.Command{
		Use:   "audio",
		Short: "Render and export audio-only output",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(out) == "" {
				return fmt.Errorf("--out is required")
			}
			switch codec {
			case "mp3", "aac":
			default:
				return fmt.Errorf("--codec must be mp3 or aac")
			}
			log := buildLogger(c.verbose)
			defer log.Sync()
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			tempVideo, err := renderVariantTempVideo(ctx, &c, log)
			if err != nil {
				return err
			}
			defer os.Remove(tempVideo)

			if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
				return fmt.Errorf("creating output dir: %w", err)
			}
			ffArgs := []string{"-y", "-i", tempVideo, "-vn"}
			if codec == "mp3" {
				ffArgs = append(ffArgs, "-c:a", "libmp3lame")
			} else {
				ffArgs = append(ffArgs, "-c:a", "aac")
			}
			ffArgs = append(ffArgs, out)
			if err := runFFmpeg(ctx, ffArgs, c.verbose); err != nil {
				return err
			}
			fmt.Printf("[ok] audio exported -> %s\n", out)
			return nil
		},
	}
	applyExportCommonFlags(cmd, &c)
	cmd.Flags().StringVarP(&out, "out", "o", "", "output audio path (.mp3/.m4a)")
	cmd.Flags().StringVar(&codec, "codec", "mp3", "audio codec: mp3|aac")
	return cmd
}

func applyExportCommonFlags(cmd *cobra.Command, c *exportCommonFlags) {
	cmd.Flags().StringVar(&c.url, "url", "", "composition URL")
	cmd.Flags().IntVar(&c.frames, "frames", 0, "total frame count (optional when duration-source resolves it)")
	cmd.Flags().IntVar(&c.fps, "fps", 30, "frames per second")
	cmd.Flags().IntVar(&c.slides, "slides", 0, "number of slides for fixed fallback")
	cmd.Flags().Float64Var(&c.secondsPerSlide, "seconds-per-slide", 5, "seconds per slide for fixed fallback")
	cmd.Flags().IntVar(&c.width, "width", 720, "render width")
	cmd.Flags().IntVar(&c.height, "height", 1280, "render height")
	cmd.Flags().IntVarP(&c.workers, "workers", "w", 0, "number of parallel browser instances")
	cmd.Flags().StringVar(&c.tmpDir, "tmp-dir", "", "directory for intermediate frame files")
	cmd.Flags().StringArrayVar(&c.chromeFlags, "chrome-flag", nil, "extra Chrome flags")
	cmd.Flags().StringVar(&c.profile, "profile", "final", "encoding profile: final|fast")
	cmd.Flags().StringVar(&c.preset, "preset", "", "named render preset")
	cmd.Flags().StringVar(&c.captureFormat, "capture-format", "", "browser capture format override: png|jpeg")
	cmd.Flags().IntVar(&c.jpegQuality, "jpeg-quality", 0, "JPEG screenshot quality (1-100)")
	cmd.Flags().StringVar(&c.durationSource, "duration-source", string(composition.DurationSourceAuto), "duration source: auto|manual|fixed")
	cmd.Flags().StringVar(&c.slideDurationsMS, "slide-durations-ms", "", "comma-separated slide durations in milliseconds")
	cmd.Flags().IntVar(&c.defaultSlideMS, "default-slide-ms", 5000, "default slide duration in milliseconds")
	cmd.Flags().BoolVar(&c.noAudioDiscovery, "no-audio-discovery", false, "disable auto-discovery/mux of page audio tracks")
	cmd.Flags().BoolVar(&c.timelineResolver, "timeline-resolver", false, "enable guarded deterministic timeline query hints")
	cmd.Flags().BoolVarP(&c.verbose, "verbose", "v", false, "enable debug logging")
}

func renderVariantTempVideo(ctx context.Context, c *exportCommonFlags, log *zap.Logger) (string, error) {
	if strings.TrimSpace(c.url) == "" {
		return "", fmt.Errorf("--url is required")
	}
	if c.fps <= 0 {
		return "", fmt.Errorf("--fps must be > 0")
	}
	manualDurations, err := composition.ParseDurationsCSV(c.slideDurationsMS)
	if err != nil {
		return "", fmt.Errorf("parsing --slide-durations-ms: %w", err)
	}

	comp := &composition.Composition{
		URL:            c.url,
		DurationFrames: c.frames,
		FPS:            c.fps,
		Width:          c.width,
		Height:         c.height,
		Output: composition.OutputConfig{
			Path: filepath.Join(os.TempDir(), fmt.Sprintf("gorender-export-%d.mp4", time.Now().UnixNano())),
		},
	}
	comp.Defaults()

	selectedPreset := choosePreset(c.preset, c.profile)
	presetCfg, hasPreset := presets.Resolve(selectedPreset)
	if hasPreset {
		if c.width == 720 && presetCfg.DefaultWidth > 0 {
			comp.Width = presetCfg.DefaultWidth
		}
		if c.height == 1280 && presetCfg.DefaultHeight > 0 {
			comp.Height = presetCfg.DefaultHeight
		}
		if c.captureFormat == "" && presetCfg.CaptureFormat != "" {
			c.captureFormat = presetCfg.CaptureFormat
		}
		if c.jpegQuality == 0 && presetCfg.CaptureJPEGQuality > 0 {
			c.jpegQuality = presetCfg.CaptureJPEGQuality
		}
		if comp.Output.Preset == "" && presetCfg.EncoderPreset != "" {
			comp.Output.Preset = presetCfg.EncoderPreset
		}
		if comp.Output.CRF == 0 && presetCfg.CRF > 0 {
			comp.Output.CRF = presetCfg.CRF
		}
	}

	if err := gorender.Render(ctx, comp, gorender.RenderOptions{
		Concurrency:          c.workers,
		TmpDir:               c.tmpDir,
		FrameStep:            1,
		EncodingProfile:      c.profile,
		CaptureFormat:        c.captureFormat,
		CaptureJPEGQuality:   c.jpegQuality,
		ExperimentalPipeline: true,
		DurationSource:       composition.DurationSource(c.durationSource),
		SlideDurationsMs:     manualDurations,
		DefaultSlideMs:       c.defaultSlideMS,
		Slides:               c.slides,
		SecondsPerSlide:      c.secondsPerSlide,
		AutoDiscoverAudio:    !c.noAudioDiscovery,
		ChromeFlags:          c.chromeFlags,
		UseTimelineResolver:  c.timelineResolver,
	}, log); err != nil {
		return "", err
	}
	return comp.Output.Path, nil
}

func runFFmpeg(ctx context.Context, args []string, verbose bool) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ffmpeg failed: %w", err)
		}
		return nil
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("ffmpeg failed: %w", err)
		}
		return fmt.Errorf("ffmpeg failed: %w (%s)", err, msg)
	}
	return nil
}

func buildWarmup() *cobra.Command {
	var (
		workers      int
		prefetch     []string
		prefetchFile string
		chromeFlags  []string
		url          string
		fps          int
		width        int
		height       int
		frame        int
		readyTimeout time.Duration
		noReadyCheck bool
		verbose      bool
	)

	cmd := &cobra.Command{
		Use:   "warmup",
		Short: "Warm browser workers and asset cache for faster subsequent renders",
		RunE: func(cmd *cobra.Command, args []string) error {
			if url == "" {
				return fmt.Errorf("--url is required")
			}
			if fps <= 0 {
				return fmt.Errorf("--fps must be > 0")
			}
			log := buildLogger(verbose)
			defer log.Sync()

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

			comp := &composition.Composition{
				URL:          url,
				FPS:          fps,
				Width:        width,
				Height:       height,
				ReadyTimeout: readyTimeout,
			}

			return gorender.Warmup(ctx, comp, gorender.WarmupOptions{
				Concurrency:    workers,
				PrefetchAssets: prefetch,
				ChromeFlags:    chromeFlags,
				Frame:          frame,
				SkipReadyCheck: noReadyCheck,
			}, log)
		},
	}

	cmd.Flags().IntVarP(&workers, "workers", "w", 0, "number of parallel browser instances (default: CPU count)")
	cmd.Flags().StringArrayVar(&prefetch, "prefetch", nil, "URLs to prefetch into the asset cache")
	cmd.Flags().StringVar(&prefetchFile, "prefetch-file", "", "path to newline-delimited URLs to prefetch into the asset cache")
	cmd.Flags().StringArrayVar(&chromeFlags, "chrome-flag", nil, "extra Chrome flags")
	cmd.Flags().StringVar(&url, "url", "", "composition URL to warm")
	cmd.Flags().IntVar(&fps, "fps", 30, "frames per second used for frame URL generation")
	cmd.Flags().IntVar(&width, "width", 720, "render width")
	cmd.Flags().IntVar(&height, "height", 1280, "render height")
	cmd.Flags().IntVar(&frame, "frame", 0, "frame index to probe during warmup")
	cmd.Flags().DurationVar(&readyTimeout, "ready-timeout", 20*time.Second, "ready signal timeout during warmup")
	cmd.Flags().BoolVar(&noReadyCheck, "no-ready-check", false, "skip waiting for ready signal (navigation + cache warm only)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	return cmd
}

func buildBench() *cobra.Command {
	var (
		runs             int
		workers          int
		tmpDir           string
		keepFrames       bool
		prefetch         []string
		chromeFlags      []string
		preset           string
		noAudioDiscovery bool
		durationSource   string
		slideDurationsMS string
		defaultSlideMS   int
		timelineResolver bool
		verbose          bool
		outputDir        string
		continueOnErr    bool
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
					UseTimelineResolver:  timelineResolver,
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
	cmd.Flags().BoolVar(&timelineResolver, "timeline-resolver", false, "enable guarded deterministic timeline query hints (gr_slide/gr_t) in frame URLs")
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
		expEncodePreset  string
		timelineResolver bool
		parityRuns       int
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
			if parityRuns <= 0 {
				parityRuns = 1
			}
			ts := time.Now().Format("20060102-150405")

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			baseDurations := make([]time.Duration, 0, parityRuns)
			expDurations := make([]time.Duration, 0, parityRuns)
			ssimValues := make([]float64, 0, parityRuns)
			psnrValues := make([]float64, 0, parityRuns)
			baseOutputs := make([]string, 0, parityRuns)
			expOutputs := make([]string, 0, parityRuns)

			for run := 1; run <= parityRuns; run++ {
				baseOut := filepath.Join(outputDir, fmt.Sprintf("baseline-%s-r%02d.mp4", ts, run))
				expOut := filepath.Join(outputDir, fmt.Sprintf("experimental-%s-r%02d.mp4", ts, run))

				baseComp := *comp
				baseComp.Output = comp.Output
				baseComp.Output.Path = baseOut

				expComp := *comp
				expComp.Output = comp.Output
				expComp.Output.Path = expOut
				if strings.TrimSpace(expEncodePreset) != "" {
					expComp.Output.Preset = strings.TrimSpace(expEncodePreset)
					// Keep CRF identical to baseline while changing encoder speed preset.
					expComp.Output.CRF = baseComp.Output.CRF
				}

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
					UseTimelineResolver:  timelineResolver,
				}, log); err != nil {
					return fmt.Errorf("baseline render failed on run %d: %w", run, err)
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
					UseTimelineResolver:  timelineResolver,
				}, log); err != nil {
					return fmt.Errorf("experimental render failed on run %d: %w", run, err)
				}
				expDur := time.Since(startExp)

				ssim, psnr, err := computeParityMetrics(ctx, baseOut, expOut)
				if err != nil {
					return fmt.Errorf("computing parity metrics on run %d: %w", run, err)
				}

				baseDurations = append(baseDurations, baseDur)
				expDurations = append(expDurations, expDur)
				ssimValues = append(ssimValues, ssim)
				psnrValues = append(psnrValues, psnr)
				baseOutputs = append(baseOutputs, baseOut)
				expOutputs = append(expOutputs, expOut)

				runSpeedup := 0.0
				if baseDur > 0 {
					runSpeedup = 1.0 - (float64(expDur) / float64(baseDur))
				}
				fmt.Printf("run %d/%d: baseline=%s experimental=%s speedup=%.2f%% ssim=%.6f psnr=%.3f dB\n",
					run, parityRuns, baseDur.Round(time.Millisecond), expDur.Round(time.Millisecond), runSpeedup*100, ssim, psnr)
			}

			medianBase := medianDuration(baseDurations)
			medianExp := medianDuration(expDurations)
			worstSSIM := minFloat(ssimValues)
			worstPSNR := minFloat(psnrValues)

			speedup := 0.0
			if medianBase > 0 {
				speedup = 1.0 - (float64(medianExp) / float64(medianBase))
			}

			fmt.Printf("baseline (median):     %s\n", medianBase.Round(time.Millisecond))
			fmt.Printf("experimental (median): %s\n", medianExp.Round(time.Millisecond))
			fmt.Printf("speedup (median):      %.2f%%\n", speedup*100)
			fmt.Printf("ssim(all, worst):      %.6f\n", worstSSIM)
			fmt.Printf("psnr(avg, worst):      %.3f dB\n", worstPSNR)
			if len(baseOutputs) > 0 {
				fmt.Printf("baseline out: %s\n", baseOutputs[len(baseOutputs)-1])
			}
			if len(expOutputs) > 0 {
				fmt.Printf("exper out:    %s\n", expOutputs[len(expOutputs)-1])
			}

			if !keepOutputs {
				for _, p := range baseOutputs {
					defer os.Remove(p)
				}
				for _, p := range expOutputs {
					defer os.Remove(p)
				}
			}

			if worstSSIM < minSSIM {
				return fmt.Errorf("parity failed: worst ssim %.6f < min %.6f", worstSSIM, minSSIM)
			}
			if worstPSNR < minPSNR {
				return fmt.Errorf("parity failed: worst psnr %.3f < min %.3f", worstPSNR, minPSNR)
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
	cmd.Flags().IntVar(&parityRuns, "parity-runs", 1, "number of baseline/experimental run pairs; median speedup and worst-case quality are enforced")
	cmd.Flags().StringVar(&expEncodePreset, "exp-encode-preset", "", "override experimental encode preset only (e.g. veryfast, superfast); CRF remains same as baseline")
	cmd.Flags().BoolVar(&timelineResolver, "timeline-resolver", false, "enable guarded deterministic timeline query hints (gr_slide/gr_t) in frame URLs")

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

func writeConcatList(path string, inputs []string) error {
	var b strings.Builder
	for _, in := range inputs {
		p := strings.ReplaceAll(in, "'", "'\\''")
		b.WriteString("file '")
		b.WriteString(p)
		b.WriteString("'\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
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

func medianDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func minFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
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
