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

