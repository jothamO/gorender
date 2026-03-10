package integration

// Integration tests require a running MakeMoments staging environment.
// Run with: go test ./... -tags=integration -v
//
// These tests are intentionally NOT run in CI by default — they need
// a real Chrome instance, ffmpeg, and a live URL.
//
// Usage:
//   GORENDER_TEST_URL=https://staging.makemoments.xyz/story/test-fixture \
//   go test ./internal/integration/... -tags=integration -v -timeout 120s

//go:build integration

import (
	"context"
	"os"
	"testing"
	"time"

	gorender "github.com/makemoments/gorender"
	"github.com/makemoments/gorender/internal/composition"
	goffmpeg "github.com/makemoments/gorender/internal/ffmpeg"
	"go.uber.org/zap"
)

const (
	testFixtureFrames = 30  // 1 second at 30fps — fast to iterate
	testFPS           = 30
)

func TestStoryViewerRenders(t *testing.T) {
	url := os.Getenv("GORENDER_TEST_URL")
	if url == "" {
		t.Skip("GORENDER_TEST_URL not set — skipping integration test")
	}

	outPath := t.TempDir() + "/test-output.mp4"

	comp := &composition.Composition{
		URL:            url,
		DurationFrames: testFixtureFrames,
		FPS:            testFPS,
		Width:          1080,
		Height:         1920,
		Output: composition.OutputConfig{
			Path:   outPath,
			Format: "mp4",
			CRF:    23, // slightly lower quality for faster test render
		},
	}

	log, _ := zap.NewDevelopment()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	err := gorender.Render(ctx, comp, gorender.RenderOptions{
		Concurrency: 2,
	}, log)

	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	elapsed := time.Since(start)
	t.Logf("rendered %d frames in %s (%.1f fps)",
		testFixtureFrames, elapsed.Round(time.Millisecond),
		float64(testFixtureFrames)/elapsed.Seconds(),
	)

	// Verify output file exists and is non-empty.
	stat, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("output file missing: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatal("output file is empty")
	}
	t.Logf("output size: %d bytes", stat.Size())

	// Verify with ffprobe.
	info, err := goffmpeg.Probe(outPath)
	if err != nil {
		t.Fatalf("ffprobe failed: %v", err)
	}

	// Check duration matches expectation within 100ms.
	expectedDuration := float64(testFixtureFrames) / float64(testFPS) // 1.0s
	t.Logf("ffprobe: duration=%s codec=%s size=%s",
		info["format.duration"],
		info["streams.stream.0.codec_name"],
		info["format.size"],
	)
	_ = expectedDuration // TODO: parse and assert

	// Check codec.
	codec := info["streams.stream.0.codec_name"]
	if codec != "h264" {
		t.Errorf("expected h264 codec, got %q", codec)
	}
}

func TestStoryViewerRenders_MultipleWorkers(t *testing.T) {
	url := os.Getenv("GORENDER_TEST_URL")
	if url == "" {
		t.Skip("GORENDER_TEST_URL not set")
	}

	log, _ := zap.NewDevelopment()
	ctx := context.Background()

	workerCounts := []int{1, 2, 4}
	for _, workers := range workerCounts {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			outPath := t.TempDir() + "/output.mp4"
			comp := &composition.Composition{
				URL:            url,
				DurationFrames: 15, // half second
				FPS:            testFPS,
				Width:          1080,
				Height:         1920,
				Output:         composition.OutputConfig{Path: outPath},
			}
			start := time.Now()
			if err := gorender.Render(ctx, comp, gorender.RenderOptions{
				Concurrency: workers,
			}, log); err != nil {
				t.Fatalf("workers=%d: %v", workers, err)
			}
			t.Logf("workers=%d: %.1f fps",
				workers,
				float64(15)/time.Since(start).Seconds(),
			)
		})
	}
}
