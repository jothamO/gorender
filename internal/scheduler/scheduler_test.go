package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestScheduler_AllFramesRendered(t *testing.T) {
	total := 50
	var rendered atomic.Int64

	worker := func(ctx context.Context, job FrameJob) error {
		rendered.Add(1)
		return nil
	}

	sched := New(Options{Workers: 4}, worker, zap.NewNop())
	jobs := BuildJobs(total, func(f int) string {
		return fmt.Sprintf("/tmp/frame-%06d.png", f)
	})

	ctx := context.Background()
	results := sched.Run(ctx)
	go func() {
		_ = sched.Submit(ctx, jobs)
		sched.Close()
	}()

	for range results {
	}

	if rendered.Load() != int64(total) {
		t.Errorf("expected %d renders, got %d", total, rendered.Load())
	}
}

func TestScheduler_ErrorsPropagated(t *testing.T) {
	boom := fmt.Errorf("render exploded")
	worker := func(ctx context.Context, job FrameJob) error {
		if job.Frame == 5 {
			return boom
		}
		return nil
	}

	sched := New(Options{Workers: 2}, worker, zap.NewNop())
	jobs := BuildJobs(10, func(f int) string { return fmt.Sprintf("f%d.png", f) })

	ctx := context.Background()
	results := sched.Run(ctx)
	go func() {
		_ = sched.Submit(ctx, jobs)
		sched.Close()
	}()

	var errCount int
	for r := range results {
		if r.Err != nil {
			errCount++
		}
	}

	if errCount != 1 {
		t.Errorf("expected 1 error, got %d", errCount)
	}
}

func TestScheduler_RespectsCancellation(t *testing.T) {
	blocked := make(chan struct{})
	worker := func(ctx context.Context, job FrameJob) error {
		select {
		case <-blocked:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	sched := New(Options{Workers: 2, QueueDepth: 100}, worker, zap.NewNop())
	jobs := BuildJobs(20, func(f int) string { return fmt.Sprintf("f%d.png", f) })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resultsCh := sched.Run(ctx)
	go func() {
		_ = sched.Submit(ctx, jobs)
		sched.Close()
	}()

	var results []FrameResult
	for r := range resultsCh {
		results = append(results, r)
	}

	// Some frames will have ctx.Err() — just check we don't hang.
	t.Logf("got %d results before cancellation", len(results))
}

func TestScheduler_WorkStealing(t *testing.T) {
	// Workers should drain all jobs even if counts are uneven.
	// This tests that the channel-based work-stealing is actually working.
	total := 37 // odd number intentionally
	var count atomic.Int64

	worker := func(ctx context.Context, job FrameJob) error {
		count.Add(1)
		return nil
	}

	sched := New(Options{Workers: 3}, worker, zap.NewNop())
	jobs := BuildJobs(total, func(f int) string { return fmt.Sprintf("f%d.png", f) })

	ctx := context.Background()
	results := sched.Run(ctx)
	go func() {
		_ = sched.Submit(ctx, jobs)
		sched.Close()
	}()

	for range results {
	}

	if count.Load() != int64(total) {
		t.Errorf("work stealing: expected %d, got %d", total, count.Load())
	}
}

func TestBuildJobs(t *testing.T) {
	jobs := BuildJobs(5, func(f int) string {
		return fmt.Sprintf("/tmp/frame-%06d.png", f)
	})
	if len(jobs) != 5 {
		t.Fatalf("expected 5 jobs, got %d", len(jobs))
	}
	for i, j := range jobs {
		if j.Frame != i {
			t.Errorf("job %d: wrong frame %d", i, j.Frame)
		}
		expected := fmt.Sprintf("/tmp/frame-%06d.png", i)
		if j.OutputPath != expected {
			t.Errorf("job %d: wrong path %q", i, j.OutputPath)
		}
	}
}

func TestProgress_ETA(t *testing.T) {
	p := NewProgress(100)
	// Simulate 50 frames done in ~0 time — ETA should be near zero.
	for i := 0; i < 50; i++ {
		p.Update(FrameResult{Frame: i})
	}
	eta := p.ETA()
	if eta < 0 {
		t.Errorf("negative ETA: %s", eta)
	}
}
