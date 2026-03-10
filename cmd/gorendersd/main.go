package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/makemoments/gorender/internal/jobs"
	"github.com/makemoments/gorender/internal/server"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	if err := buildRoot().Execute(); err != nil {
		os.Exit(1)
	}
}

func buildRoot() *cobra.Command {
	var (
		addr              string
		apiKey            string
		maxConcurrentJobs int
		workersPerJob     int
		outputDir         string
		tmpDir            string
		retentionHours    int
		verbose           bool
	)

	cmd := &cobra.Command{
		Use:   "gorendersd",
		Short: "gorender HTTP render server",
		Long: `gorendersd is a persistent HTTP server that accepts render jobs,
manages a warm browser pool, and produces MP4 files on disk.

Your MakeMoments backend POSTs to /jobs, polls /jobs/{id}, and
downloads the result from /jobs/{id}/download.`,
		Example: `  # Start with defaults
  gorendersd --addr :8080 --output-dir ./output

  # Production
  gorendersd \
    --addr :8080 \
    --api-key $RENDER_API_KEY \
    --max-jobs 2 \
    --workers 4 \
    --output-dir /var/gorender/output \
    --retention 24`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(verbose)
			defer log.Sync()

			ctx, cancel := signal.NotifyContext(context.Background(),
				syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			return run(ctx, runConfig{
				addr:    addr,
				apiKey:  apiKey,
				queueOpts: jobs.QueueOptions{
					MaxConcurrentJobs: maxConcurrentJobs,
					WorkersPerJob:     workersPerJob,
					OutputDir:         outputDir,
					TmpDir:            tmpDir,
					RetentionPeriod:   time.Duration(retentionHours) * time.Hour,
				},
				verbose: verbose,
			}, log)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "address to listen on")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Bearer token for API auth (leave empty to disable)")
	cmd.Flags().IntVar(&maxConcurrentJobs, "max-jobs", 2, "max simultaneous render jobs")
	cmd.Flags().IntVar(&workersPerJob, "workers", 4, "browser pool size per job")
	cmd.Flags().StringVar(&outputDir, "output-dir", "./output", "directory for finished MP4s")
	cmd.Flags().StringVar(&tmpDir, "tmp-dir", os.TempDir(), "directory for intermediate PNG frames")
	cmd.Flags().IntVar(&retentionHours, "retention", 24, "hours to keep finished MP4s on disk")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	return cmd
}

type runConfig struct {
	addr      string
	apiKey    string
	queueOpts jobs.QueueOptions
	verbose   bool
}

func run(ctx context.Context, cfg runConfig, log *zap.Logger) error {
	log.Info("starting gorendersd",
		zap.String("addr", cfg.addr),
		zap.Int("maxJobs", cfg.queueOpts.MaxConcurrentJobs),
		zap.Int("workersPerJob", cfg.queueOpts.WorkersPerJob),
		zap.String("outputDir", cfg.queueOpts.OutputDir),
	)

	store := jobs.NewStore()

	log.Info("warming browser pool...")
	queue, err := jobs.NewQueue(ctx, store, cfg.queueOpts, log)
	if err != nil {
		return fmt.Errorf("creating job queue: %w", err)
	}
	defer queue.Stop()

	queue.Start(ctx)
	log.Info("browser pool ready")

	srv := server.New(queue, store, server.Options{
		APIKey: cfg.apiKey,
	}, log)

	httpServer := &http.Server{
		Addr:         cfg.addr,
		Handler:      srv,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute, // long for /download streaming
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", zap.String("addr", cfg.addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or server error.
	select {
	case <-ctx.Done():
		log.Info("shutting down...")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutCancel()
		if err := httpServer.Shutdown(shutCtx); err != nil {
			log.Error("shutdown error", zap.Error(err))
		}
		log.Info("shutdown complete")
		return nil
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}

func buildLogger(verbose bool) *zap.Logger {
	level := zapcore.InfoLevel
	if verbose {
		level = zapcore.DebugLevel
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	log, _ := cfg.Build()
	return log
}
