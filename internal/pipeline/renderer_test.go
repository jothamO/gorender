package pipeline

import (
	"strings"
	"testing"

	"github.com/makemoments/gorender/internal/composition"
)

func TestBuildFrameURL_NoExistingQuery(t *testing.T) {
	r := &Renderer{
		comp: &composition.Composition{
			URL:       "http://localhost:3000/story",
			SeekParam: "frame",
		},
	}
	got := r.buildFrameURL(42)
	if got != "http://localhost:3000/story?frame=42" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func TestBuildFrameURL_WithExistingQuery(t *testing.T) {
	r := &Renderer{
		comp: &composition.Composition{
			URL:       "http://localhost:3000/story?foo=bar",
			SeekParam: "frame",
		},
	}
	got := r.buildFrameURL(7)
	if !strings.Contains(got, "foo=bar") {
		t.Fatalf("expected existing query param to remain, got: %s", got)
	}
	if !strings.Contains(got, "frame=7") {
		t.Fatalf("expected frame query param to be added, got: %s", got)
	}
}

func TestBuildFrameURL_WithTimelineQueryHints(t *testing.T) {
	r := &Renderer{
		comp: &composition.Composition{
			URL:               "http://localhost:3000/story",
			SeekParam:         "frame",
			FPS:               30,
			EmitTimelineQuery: true,
			SlideDurationsMs:  []int{5000, 2000},
		},
	}
	got := r.buildFrameURL(151) // 5033ms -> slide 1
	if !strings.Contains(got, "gr_slide=1") {
		t.Fatalf("expected gr_slide hint, got: %s", got)
	}
	if !strings.Contains(got, "gr_in_slide_ms=") {
		t.Fatalf("expected gr_in_slide_ms hint, got: %s", got)
	}
	if !strings.Contains(got, "gr_slide_ms=2000") {
		t.Fatalf("expected gr_slide_ms hint, got: %s", got)
	}
	if !strings.Contains(got, "gr_t=") {
		t.Fatalf("expected gr_t hint, got: %s", got)
	}
}

func TestFrameTimeline_GuardedOff(t *testing.T) {
	r := &Renderer{
		comp: &composition.Composition{
			FPS:               30,
			EmitTimelineQuery: false,
			SlideDurationsMs:  []int{5000, 2000},
		},
	}
	if _, ok := r.frameTimeline(10); ok {
		t.Fatalf("expected frame timeline to be disabled when guard is off")
	}
}
