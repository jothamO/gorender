package timeline

import "testing"

func TestNewValidation(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("expected error for empty durations")
	}
	if _, err := New([]int{5000, 0}); err == nil {
		t.Fatal("expected error for non-positive duration")
	}
}

func TestTotalFrames(t *testing.T) {
	tl, err := New([]int{5000, 2500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := tl.TotalFrames(30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 7.5s * 30fps = 225 frames
	if got != 225 {
		t.Fatalf("expected 225 frames, got %d", got)
	}
}

func TestTotalFramesCeilRounding(t *testing.T) {
	tl, err := New([]int{1001})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := tl.TotalFrames(30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1.001s * 30fps = 30.03 -> ceil => 31
	if got != 31 {
		t.Fatalf("expected 31 frames, got %d", got)
	}
}

func TestLocateFrameAcrossSlides(t *testing.T) {
	tl, err := New([]int{5000, 2000, 3000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loc0, err := tl.LocateFrame(0, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc0.SlideIndex != 0 {
		t.Fatalf("expected slide 0, got %d", loc0.SlideIndex)
	}

	// 150 frames at 30fps -> 5000ms boundary, should enter slide 1.
	loc1, err := tl.LocateFrame(150, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc1.SlideIndex != 1 {
		t.Fatalf("expected slide 1, got %d", loc1.SlideIndex)
	}
	if loc1.InSlideMs != 0 {
		t.Fatalf("expected in-slide ms 0, got %d", loc1.InSlideMs)
	}

	// Last frame should clamp to the last slide range.
	last, err := tl.LocateFrame(10000, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if last.SlideIndex != 2 {
		t.Fatalf("expected slide 2, got %d", last.SlideIndex)
	}
	if last.InSlideMs < 0 || last.InSlideMs >= last.SlideDurMs {
		t.Fatalf("unexpected in-slide ms %d for dur %d", last.InSlideMs, last.SlideDurMs)
	}
}
