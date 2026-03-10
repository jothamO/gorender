package preview

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewHandlerValidation(t *testing.T) {
	if _, err := NewHandler(Config{}); err == nil {
		t.Fatalf("expected error for missing base URL")
	}
	if _, err := NewHandler(Config{BaseURL: "localhost:8080/story"}); err == nil {
		t.Fatalf("expected error for relative base URL")
	}
	if _, err := NewHandler(Config{BaseURL: "ftp://localhost:8080/story"}); err == nil {
		t.Fatalf("expected error for non-http scheme")
	}
}

func TestNewHandlerRendersPage(t *testing.T) {
	h, err := NewHandler(Config{
		BaseURL: "http://localhost:8080/story/test",
		FPS:     30,
		Width:   720,
		Height:  1280,
		Params:  map[string]string{"foo": "bar"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "gorender preview") {
		t.Fatalf("expected preview page content")
	}
	if !strings.Contains(body, "http://localhost:8080/story/test") {
		t.Fatalf("expected base URL to be embedded")
	}
	if !strings.Contains(body, "\"foo\":\"bar\"") {
		t.Fatalf("expected params to be embedded")
	}
}

func TestNewHandlerNonRootPathNotFound(t *testing.T) {
	h, err := NewHandler(Config{
		BaseURL: "http://localhost:8080/story/test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/localhost:8080/story/test?frame=1", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestNewHandlerSanitizesQuotedBaseURL(t *testing.T) {
	cases := []string{
		"\"http://localhost:8080/story/test\"",
		"\\\"http://localhost:8080/story/test\\\"",
		"%22http://localhost:8080/story/test%22",
		"%2522http://localhost:8080/story/test%2522",
	}
	for _, in := range cases {
		got := sanitizeBaseURL(in)
		if got != "http://localhost:8080/story/test" {
			t.Fatalf("sanitizeBaseURL(%q) = %q", in, got)
		}
	}
}
