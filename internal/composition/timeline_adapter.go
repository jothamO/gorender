package composition

import enginetimeline "github.com/makemoments/gorender/opensource/engine/timeline"

// FrameLocation is the normalized frame position within a slide timeline.
// It is an alias of the isolated engine contract for promotion-safe reuse.
type FrameLocation = enginetimeline.FrameLocation

// LocateFrameInDurations resolves where a given frame sits across per-slide
// durations. This is a promotion adapter over opensource engine timeline logic.
func LocateFrameInDurations(durations []int, fps int, frame int) (FrameLocation, error) {
	tl, err := enginetimeline.New(durations)
	if err != nil {
		return FrameLocation{}, err
	}
	return tl.LocateFrame(frame, fps)
}
