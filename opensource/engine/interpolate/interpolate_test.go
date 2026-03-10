package interpolate

import "testing"

func almost(a, b, eps float64) bool {
	if a > b {
		return a-b <= eps
	}
	return b-a <= eps
}

func TestClampAndLerp(t *testing.T) {
	if Clamp01(-1) != 0 {
		t.Fatal("expected clamp to 0")
	}
	if Clamp01(2) != 1 {
		t.Fatal("expected clamp to 1")
	}
	if !almost(Lerp(10, 20, 0.5), 15, 1e-9) {
		t.Fatal("expected midpoint lerp")
	}
}

func TestCubicCurvesBounded(t *testing.T) {
	cases := []struct {
		name string
		fn   func(float64) float64
	}{
		{"in", EaseInCubic},
		{"out", EaseOutCubic},
		{"inout", EaseInOutCubic},
		{"sine", EaseInOutSine},
	}
	for _, tc := range cases {
		a := tc.fn(0)
		b := tc.fn(1)
		if !almost(a, 0, 1e-9) {
			t.Fatalf("%s: expected f(0)=0, got %f", tc.name, a)
		}
		if !almost(b, 1, 1e-9) {
			t.Fatalf("%s: expected f(1)=1, got %f", tc.name, b)
		}
		m := tc.fn(0.5)
		if m <= 0 || m >= 1 {
			t.Fatalf("%s: expected 0<f(0.5)<1, got %f", tc.name, m)
		}
	}
}
