package interpolate

import "math"

func Clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func Lerp(a, b, t float64) float64 {
	t = Clamp01(t)
	return a + (b-a)*t
}

func EaseInCubic(t float64) float64 {
	t = Clamp01(t)
	return t * t * t
}

func EaseOutCubic(t float64) float64 {
	t = Clamp01(t)
	u := 1 - t
	return 1 - (u * u * u)
}

func EaseInOutCubic(t float64) float64 {
	t = Clamp01(t)
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - math.Pow(u, 3)/2
}

func EaseInOutSine(t float64) float64 {
	t = Clamp01(t)
	return -(math.Cos(math.Pi*t)-1) / 2
}
