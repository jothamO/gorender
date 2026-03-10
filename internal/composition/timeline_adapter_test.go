package composition

import "testing"

func TestLocateFrameInDurations(t *testing.T) {
	durations := []int{5000, 2000, 3000}

	loc0, err := LocateFrameInDurations(durations, 30, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc0.SlideIndex != 0 {
		t.Fatalf("expected slide 0, got %d", loc0.SlideIndex)
	}

	// 150 frames @ 30fps => 5000ms, first frame of slide 1.
	loc1, err := LocateFrameInDurations(durations, 30, 150)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc1.SlideIndex != 1 {
		t.Fatalf("expected slide 1, got %d", loc1.SlideIndex)
	}
	if loc1.InSlideMs != 0 {
		t.Fatalf("expected in-slide ms 0, got %d", loc1.InSlideMs)
	}
}

func TestLocateFrameInDurationsInvalid(t *testing.T) {
	if _, err := LocateFrameInDurations([]int{}, 30, 0); err == nil {
		t.Fatalf("expected error for empty durations")
	}
	if _, err := LocateFrameInDurations([]int{5000}, 0, 0); err == nil {
		t.Fatalf("expected error for invalid fps")
	}
	if _, err := LocateFrameInDurations([]int{5000}, 30, -1); err == nil {
		t.Fatalf("expected error for invalid frame")
	}
}
