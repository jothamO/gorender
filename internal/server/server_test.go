package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/makemoments/gorender/internal/jobs"
	"go.uber.org/zap"
)

// mockQueue satisfies the interface enough for handler tests.
type mockQueue struct {
	enqueued []*jobs.Job
}

func (m *mockQueue) Enqueue(job *jobs.Job) {
	m.enqueued = append(m.enqueued, job)
}

func newTestServer(t *testing.T) (*Server, *jobs.Store, *mockQueue) {
	t.Helper()
	store := jobs.NewStore()
	mq := &mockQueue{}
	// We can't use NewQueue in unit tests (requires Chrome).
	// So we test the server layer directly with a mock queue.
	s := &Server{
		store:  store,
		mux:    http.NewServeMux(),
		log:    zap.NewNop(),
		apiKey: "",
	}
	// Wire handlers manually since we bypass NewQueue.
	s.routes()
	return s, store, mq
}

func TestHandleHealth(t *testing.T) {
	s, _, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}

func TestHandleGetJob_NotFound(t *testing.T) {
	s, _, _ := newTestServer(t)
	req := httptest.NewRequest("GET", "/jobs/j_nope", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleGetJob_Found(t *testing.T) {
	s, store, _ := newTestServer(t)

	now := time.Now()
	store.Put(&jobs.Job{
		ID:        "j_test",
		Status:    jobs.StatusQueued,
		CreatedAt: now,
	})

	req := httptest.NewRequest("GET", "/jobs/j_test", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["id"] != "j_test" {
		t.Errorf("expected id j_test, got %v", body["id"])
	}
	if body["status"] != "queued" {
		t.Errorf("expected queued, got %v", body["status"])
	}
}

func TestHandleDownload_StillRendering(t *testing.T) {
	s, store, _ := newTestServer(t)
	store.Put(&jobs.Job{
		ID:        "j_busy",
		Status:    jobs.StatusRendering,
		Progress:  0.3,
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/jobs/j_busy/download", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	// Should return 202 Accepted, not 200.
	if w.Code != 202 {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

func TestHandleDownload_Failed(t *testing.T) {
	s, store, _ := newTestServer(t)
	store.Put(&jobs.Job{
		ID:        "j_fail",
		Status:    jobs.StatusFailed,
		Error:     "chrome crashed",
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("GET", "/jobs/j_fail/download", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != 422 {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleListJobs(t *testing.T) {
	s, store, _ := newTestServer(t)
	for i := 0; i < 3; i++ {
		store.Put(&jobs.Job{
			ID:        fmt.Sprintf("j_test_%d", i),
			Status:    jobs.StatusDone,
			CreatedAt: time.Now(),
		})
	}

	req := httptest.NewRequest("GET", "/jobs", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if int(body["count"].(float64)) < 3 {
		t.Errorf("expected at least 3 jobs")
	}
}

func TestAuth_Blocks_WithKey(t *testing.T) {
	store := jobs.NewStore()
	s := &Server{
		store:  store,
		mux:    http.NewServeMux(),
		log:    zap.NewNop(),
		apiKey: "secret-key",
	}
	s.routes()

	req := httptest.NewRequest("GET", "/jobs", nil)
	// No auth header.
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("expected 401 without key, got %d", w.Code)
	}

	// With correct key.
	req2 := httptest.NewRequest("GET", "/jobs", nil)
	req2.Header.Set("Authorization", "Bearer secret-key")
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Errorf("expected 200 with key, got %d", w2.Code)
	}
}

func TestHandleCreateJob_Validation(t *testing.T) {
	s, _, _ := newTestServer(t)

	// Missing url.
	body := `{"durationFrames": 100, "fps": 30}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400 for missing url, got %d", w.Code)
	}

	// Missing durationFrames.
	body2 := `{"url": "http://localhost:3000/comp", "fps": 30}`
	req2 := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	s.ServeHTTP(w2, req2)
	if w2.Code != 400 {
		t.Errorf("expected 400 for missing durationFrames, got %d", w2.Code)
	}
}

func TestHandleCreateJob_QueueUnavailable(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := `{"url":"http://localhost:3000/comp","durationFrames":100,"fps":30}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Errorf("expected 503 when queue is unavailable, got %d", w.Code)
	}
}

func TestHandleCreateJob_OversizedBody(t *testing.T) {
	s, _, _ := newTestServer(t)
	s.maxRequestBodyBytes = 16
	body := `{"url":"http://localhost:3000/comp","durationFrames":100,"fps":30}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("expected 400 for oversized body, got %d", w.Code)
	}
}

func TestHandleCreateJob_UnknownFieldRejected(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := `{"url":"http://localhost:3000/comp","durationFrames":100,"fps":30,"bogus":1}`
	req := httptest.NewRequest("POST", "/jobs", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 for unknown field, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "unknown field") {
		t.Errorf("expected unknown field error, got %q", w.Body.String())
	}
}
