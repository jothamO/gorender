package timeline

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrNoSlides     = errors.New("timeline requires at least one slide")
	ErrInvalidFPS   = errors.New("fps must be greater than zero")
	ErrInvalidDurMS = errors.New("slide duration must be greater than zero")
	ErrInvalidFrame = errors.New("frame must be non-negative")
)

// Timeline is a deterministic mapping between render-time and per-slide windows.
// It is intentionally pure and stdlib-only so it can be reused in future runtime
// implementations (CLI, server, and optional adapters).
type Timeline struct {
	durationsMs []int
	startMs     []int
	totalMs     int
}

// New constructs a validated timeline from per-slide durations in milliseconds.
func New(durationsMs []int) (*Timeline, error) {
	if len(durationsMs) == 0 {
		return nil, ErrNoSlides
	}
	start := make([]int, len(durationsMs))
	total := 0
	for i, d := range durationsMs {
		if d <= 0 {
			return nil, fmt.Errorf("slide %d: %w (%d)", i, ErrInvalidDurMS, d)
		}
		start[i] = total
		total += d
	}
	return &Timeline{
		durationsMs: append([]int(nil), durationsMs...),
		startMs:     start,
		totalMs:     total,
	}, nil
}

func (t *Timeline) TotalDurationMs() int {
	return t.totalMs
}

func (t *Timeline) SlideCount() int {
	return len(t.durationsMs)
}

func (t *Timeline) DurationsMs() []int {
	return append([]int(nil), t.durationsMs...)
}

// TotalFrames computes full-frame count for the whole timeline at fps.
func (t *Timeline) TotalFrames(fps int) (int, error) {
	if fps <= 0 {
		return 0, ErrInvalidFPS
	}
	frames := int(math.Ceil(float64(t.totalMs) * float64(fps) / 1000.0))
	if frames <= 0 {
		return 1, nil
	}
	return frames, nil
}

// FrameLocation describes where a frame lands in the timeline.
type FrameLocation struct {
	SlideIndex   int
	SlideStartMs int
	SlideDurMs   int
	InSlideMs    int
	SlideT       float64
	GlobalMs     int
}

// LocateFrame maps a frame number to its slide and local timing metrics.
func (t *Timeline) LocateFrame(frame int, fps int) (FrameLocation, error) {
	if frame < 0 {
		return FrameLocation{}, ErrInvalidFrame
	}
	if fps <= 0 {
		return FrameLocation{}, ErrInvalidFPS
	}

	globalMs := int(math.Round(float64(frame) * 1000.0 / float64(fps)))
	if globalMs < 0 {
		globalMs = 0
	}
	if globalMs >= t.totalMs {
		globalMs = t.totalMs - 1
	}

	i := 0
	for ; i < len(t.durationsMs)-1; i++ {
		if globalMs < t.startMs[i]+t.durationsMs[i] {
			break
		}
	}

	start := t.startMs[i]
	dur := t.durationsMs[i]
	in := globalMs - start
	if in < 0 {
		in = 0
	}
	denom := float64(dur)
	if denom <= 0 {
		denom = 1
	}
	tNorm := float64(in) / denom
	if tNorm < 0 {
		tNorm = 0
	}
	if tNorm > 1 {
		tNorm = 1
	}

	return FrameLocation{
		SlideIndex:   i,
		SlideStartMs: start,
		SlideDurMs:   dur,
		InSlideMs:    in,
		SlideT:       tNorm,
		GlobalMs:     globalMs,
	}, nil
}
