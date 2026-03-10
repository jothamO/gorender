package composition

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type DurationSource string

const (
	DurationSourceAuto   DurationSource = "auto"
	DurationSourceManual DurationSource = "manual"
	DurationSourceFixed  DurationSource = "fixed"
)

func ParseDurationsCSV(csv string) ([]int, error) {
	raw := strings.TrimSpace(csv)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid duration %q: %w", v, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func NormalizeDurationsMs(raw []int, fallback int) ([]int, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if fallback <= 0 {
		fallback = 5000
	}
	out := make([]int, 0, len(raw))
	for i, v := range raw {
		if v > 0 {
			out = append(out, v)
			continue
		}
		if fallback <= 0 {
			return nil, fmt.Errorf("duration at index %d must be > 0", i)
		}
		out = append(out, fallback)
	}
	return out, nil
}

func ComputeTotalFramesFromDurationsMs(durations []int, fps int) (int, error) {
	if fps <= 0 {
		return 0, fmt.Errorf("fps must be > 0")
	}
	if len(durations) == 0 {
		return 0, fmt.Errorf("durations must be non-empty")
	}
	totalMs := 0
	for i, d := range durations {
		if d <= 0 {
			return 0, fmt.Errorf("duration at index %d must be > 0", i)
		}
		totalMs += d
	}
	frames := int(math.Ceil((float64(totalMs) / 1000.0) * float64(fps)))
	if frames <= 0 {
		return 0, fmt.Errorf("computed frames must be > 0")
	}
	return frames, nil
}
