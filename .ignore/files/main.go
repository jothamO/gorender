package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/makemoments/gorender"
	"github.com/makemoments/gorender/internal/composition"
	goffmpeg "github.com/makemoments/gorender/internal/ffmpeg"
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
	cmd.AddCommand(buildVersion())
	cmd.AddCommand(buildCheck())
	return cmd
}

func buildRender() *cobra.Command {
	var (
		workers        int
		tmpDir         string
		keepFrames     bool
		prefetch       []string
		chromeFlags    []string
		verbose        bool
		// Inline flags (alternative to a comp file)
		url            string
		frames         int
		fps            int
		width          int
		height         int
		output         string
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
				comp = &composition.Composition{
					URL:            url,
					DurationFrames: frames,
					FPS:            fps,
					Width:          width,
					Height:         height,
					Output:         composition.OutputConfig{Path: output},
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			startTime := time.Now()
			var lastPrint time.Time

			err := gorender.Render(ctx, comp, gorender.RenderOptions{
				Concurrency:    workers,
				TmpDir:         tmpDir,
				KeepFrames:     keepFrames,
				PrefetchAssets: prefetch,
				ChromeFlags:    chromeFlags,
				OnProgress: func(done, total int, eta time.Duration) {
					if time.Since(lastPrint) > 2*time.Second {
						pct := float64(done) / float64(total) * 100
						fmt.Fprintf(os.Stderr, "\r  %.1f%% (%d/%d frames) ETA %s    ",
							pct, done, total, eta.Round(time.Second))
						lastPrint = time.Now()
					}
				},
			}, log)

			fmt.Fprintln(os.Stderr) // newline after progress
			if err != nil {
				return err
			}

			fmt.Printf("✓ Rendered %d frames in %s → %s\n",
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
	cmd.Flags().StringArrayVar(&chromeFlags, "chrome-flag", nil, "extra Chrome flags (e.g. --chrome-flag=disable-web-security)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	// Inline flags
	cmd.Flags().StringVar(&url, "url", "", "composition URL (inline mode)")
	cmd.Flags().IntVar(&frames, "frames", 0, "total frame count (inline mode)")
	cmd.Flags().IntVar(&fps, "fps", 30, "frames per second (inline mode)")
	cmd.Flags().IntVar(&width, "width", 1080, "output width (inline mode)")
	cmd.Flags().IntVar(&height, "height", 1920, "output height (inline mode)")
	cmd.Flags().StringVarP(&output, "out", "o", "output.mp4", "output video path (inline mode)")

	return cmd
}

func buildBench() *cobra.Command {
	var (
		runs         int
		workers      int
		tmpDir       string
		keepFrames   bool
		prefetch     []string
		chromeFlags  []string
		verbose      bool
		outputDir    string
		continueOnErr bool
		// Inline flags (alternative to a comp file)
		url          string
		frames       int
		fps          int
		width        int
		height       int
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
				baseComp = &composition.Composition{
					URL:            url,
					DurationFrames: frames,
					FPS:            fps,
					Width:          width,
					Height:         height,
				}
			}
			baseComp.Defaults()

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

				started := time.Now()
				err := gorender.Render(ctx, &compCopy, gorender.RenderOptions{
					Concurrency:    workers,
					TmpDir:         tmpDir,
					KeepFrames:     keepFrames,
					PrefetchAssets: prefetch,
					ChromeFlags:    chromeFlags,
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
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "directory where benchmark outputs are written")
	cmd.Flags().BoolVar(&continueOnErr, "continue-on-error", false, "continue remaining runs even if one run fails")

	// Inline flags
	cmd.Flags().StringVar(&url, "url", "", "composition URL (inline mode)")
	cmd.Flags().IntVar(&frames, "frames", 0, "total frame count (inline mode)")
	cmd.Flags().IntVar(&fps, "fps", 30, "frames per second (inline mode)")
	cmd.Flags().IntVar(&width, "width", 1080, "output width (inline mode)")
	cmd.Flags().IntVar(&height, "height", 1920, "output height (inline mode)")

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
					candidates := []string{"google-chrome", "chromium", "chromium-browser", "chrome"}
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
					fmt.Printf("  ✗ %s: %v\n", c.name, err)
					allOK = false
				} else {
					fmt.Printf("  ✓ %s\n", c.name)
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

func hasExt(path string, exts ...string) bool {
	for _, ext := range exts {
		if len(path) >= len(ext) && path[len(path)-len(ext):] == ext {
			return true
		}
	}
	return false
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
