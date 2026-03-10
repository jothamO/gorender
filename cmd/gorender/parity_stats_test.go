package main

import (
	"testing"
	"time"
)

func TestMedianDuration(t *testing.T) {
	if got := medianDuration(nil); got != 0 {
		t.Fatalf("expected 0 for empty slice, got %s", got)
	}
	gotOdd := medianDuration([]time.Duration{5 * time.Second, 1 * time.Second, 3 * time.Second})
	if gotOdd != 3*time.Second {
		t.Fatalf("expected odd median 3s, got %s", gotOdd)
	}
	gotEven := medianDuration([]time.Duration{4 * time.Second, 2 * time.Second, 6 * time.Second, 8 * time.Second})
	if gotEven != 5*time.Second {
		t.Fatalf("expected even median 5s, got %s", gotEven)
	}
}

func TestMinFloat(t *testing.T) {
	if got := minFloat(nil); got != 0 {
		t.Fatalf("expected 0 for empty slice, got %f", got)
	}
	got := minFloat([]float64{0.998, 0.997, 0.999})
	if got != 0.997 {
		t.Fatalf("expected min 0.997, got %f", got)
	}
}
