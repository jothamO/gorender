package composition

import "testing"

func TestParseDurationsCSV(t *testing.T) {
	got, err := ParseDurationsCSV("5000, 7000,3000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 || got[0] != 5000 || got[1] != 7000 || got[2] != 3000 {
		t.Fatalf("unexpected durations: %#v", got)
	}
}

func TestParseDurationsCSVInvalid(t *testing.T) {
	if _, err := ParseDurationsCSV("5000,abc"); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestNormalizeDurationsMs(t *testing.T) {
	got, err := NormalizeDurationsMs([]int{5000, 0, -1, 3000}, 4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{5000, 4000, 4000, 3000}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected value at %d: got %d want %d", i, got[i], want[i])
		}
	}
}

func TestComputeTotalFramesFromDurationsMs(t *testing.T) {
	frames, err := ComputeTotalFramesFromDurationsMs([]int{5000, 2500}, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frames != 225 {
		t.Fatalf("unexpected frames: got %d want 225", frames)
	}
}

func TestComputeTotalFramesFromDurationsMsCeil(t *testing.T) {
	frames, err := ComputeTotalFramesFromDurationsMs([]int{1001}, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frames != 31 {
		t.Fatalf("unexpected frames: got %d want 31", frames)
	}
}
