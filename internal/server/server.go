package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/makemoments/gorender/internal/composition"
	"github.com/makemoments/gorender/internal/jobs"
	"go.uber.org/zap"
)

// Server is the HTTP render server.
type Server struct {
	queue               *jobs.Queue
	store               *jobs.Store
	mux                 *http.ServeMux
	log                 *zap.Logger
	maxRequestBodyBytes int64
	apiKey              string // optional — empty means no auth
}

// Options configures the server.
type Options struct {
	// APIKey is a shared secret the caller must send as Bearer token.
	// Leave empty to disable auth (only do this on a private network).
	APIKey string

	// MaxRequestBodyBytes caps the JSON body size. Defaults to 1MB.
	MaxRequestBodyBytes int64
}

// New creates a Server and registers all routes.
func New(queue *jobs.Queue, store *jobs.Store, opts Options, log *zap.Logger) *Server {
	if opts.MaxRequestBodyBytes == 0 {
		opts.MaxRequestBodyBytes = 1 << 20 // 1MB
	}

	s := &Server{
		queue:               queue,
		store:               store,
		mux:                 http.NewServeMux(),
		log:                 log,
		apiKey:              opts.APIKey,
		maxRequestBodyBytes: opts.MaxRequestBodyBytes,
	}

	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	wrapped := &responseWriter{ResponseWriter: w, code: 200}
	s.mux.ServeHTTP(wrapped, r)
	s.log.Info("request",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.Int("status", wrapped.code),
		zap.Duration("elapsed", time.Since(start)),
	)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("POST /jobs", s.auth(s.handleCreateJob))
	s.mux.HandleFunc("GET /jobs", s.auth(s.handleListJobs))
	s.mux.HandleFunc("GET /jobs/{id}", s.auth(s.handleGetJob))
	s.mux.HandleFunc("GET /jobs/{id}/download", s.auth(s.handleDownload))
	s.mux.HandleFunc("DELETE /jobs/{id}", s.auth(s.handleDeleteJob))
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	counts := s.store.Counts()
	s.json(w, 200, map[string]any{
		"status": "ok",
		"jobs":   counts,
		"time":   time.Now().UTC(),
	})
}

// POST /jobs
//
// Body:
//
//	{
//	  "url": "https://makemoments.xyz/story/abc123",
//	  "durationFrames": 300,
//	  "fps": 30,
//	  "width": 1080,
//	  "height": 1920,
//	  "audio": [...],
//	  "output": { "crf": 20, "preset": "medium" }
//	}
func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	maxBody := s.maxRequestBodyBytes
	if maxBody <= 0 {
		maxBody = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	var req struct {
		URL            string                   `json:"url"`
		DurationFrames int                      `json:"durationFrames"`
		FPS            int                      `json:"fps"`
		Width          int                      `json:"width"`
		Height         int                      `json:"height"`
		SeekParam      string                   `json:"seekParam"`
		ReadySignal    string                   `json:"readySignal"`
		Audio          []composition.AudioTrack `json:"audio"`
		Output         composition.OutputConfig `json:"output"`
	}

	if err := s.decode(r, &req); err != nil {
		s.error(w, 400, "invalid request body: "+err.Error())
		return
	}

	if req.URL == "" {
		s.error(w, 400, "url is required")
		return
	}
	if req.DurationFrames <= 0 {
		s.error(w, 400, "durationFrames must be > 0")
		return
	}

	comp := &composition.Composition{
		URL:            req.URL,
		DurationFrames: req.DurationFrames,
		FPS:            req.FPS,
		Width:          req.Width,
		Height:         req.Height,
		SeekParam:      req.SeekParam,
		ReadySignal:    req.ReadySignal,
		Audio:          req.Audio,
		Output:         req.Output,
	}
	comp.Defaults()

	if s.queue == nil {
		s.error(w, 503, "render queue unavailable")
		return
	}

	job := jobs.NewJob(comp)
	s.queue.Enqueue(job)

	s.json(w, 202, jobResponse(job))
}

// GET /jobs
func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	all := s.store.List()
	resp := make([]jobResp, 0, len(all))
	for _, j := range all {
		resp = append(resp, jobResponse(j))
	}
	s.json(w, 200, map[string]any{"jobs": resp, "count": len(resp)})
}

// GET /jobs/{id}
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job := s.resolveJob(w, r)
	if job == nil {
		return
	}
	s.json(w, 200, jobResponse(job))
}

// GET /jobs/{id}/download
//
// Streams the MP4 directly. Returns 404 if not done, 202 if still rendering.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	job := s.resolveJob(w, r)
	if job == nil {
		return
	}

	switch job.Status {
	case jobs.StatusQueued, jobs.StatusRendering:
		s.json(w, 202, map[string]any{
			"status":   job.Status,
			"progress": job.Progress,
			"eta":      job.ETA,
		})
		return
	case jobs.StatusFailed:
		s.error(w, 422, "job failed: "+job.Error)
		return
	}

	// StatusDone — stream the file.
	f, err := os.Open(job.OutputPath)
	if err != nil {
		s.error(w, 404, "output file not found (may have been cleaned up)")
		return
	}
	defer f.Close()

	filename := filepath.Base(job.OutputPath)
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	stat, err := f.Stat()
	if err == nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}

	w.WriteHeader(200)
	io.Copy(w, f)
}

// DELETE /jobs/{id}
//
// Cancels a queued job or removes a completed one. Deletes the MP4 from disk.
func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	job := s.resolveJob(w, r)
	if job == nil {
		return
	}

	// Remove output file if it exists.
	if job.OutputPath != "" {
		os.Remove(job.OutputPath)
	}

	// Mark as failed (we don't support mid-render cancellation yet).
	s.store.Update(job.ID, func(j *jobs.Job) {
		j.Status = jobs.StatusFailed
		j.Error = "cancelled by client"
	})

	w.WriteHeader(204)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Server) resolveJob(w http.ResponseWriter, r *http.Request) *jobs.Job {
	id := r.PathValue("id")
	job := s.store.Get(id)
	if job == nil {
		s.error(w, 404, fmt.Sprintf("job %q not found", id))
		return nil
	}
	return job
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" {
			next(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != s.apiKey {
			s.error(w, 401, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) json(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func (s *Server) error(w http.ResponseWriter, code int, msg string) {
	s.json(w, code, map[string]string{"error": msg})
}

// ─── Response shapes ──────────────────────────────────────────────────────────

type jobResp struct {
	ID          string      `json:"id"`
	Status      jobs.Status `json:"status"`
	Progress    float64     `json:"progress"`
	ETA         string      `json:"eta,omitempty"`
	Error       string      `json:"error,omitempty"`
	DownloadURL string      `json:"downloadUrl,omitempty"`
	CreatedAt   time.Time   `json:"createdAt"`
	StartedAt   *time.Time  `json:"startedAt,omitempty"`
	DoneAt      *time.Time  `json:"doneAt,omitempty"`
	Duration    string      `json:"duration,omitempty"`
}

func jobResponse(j *jobs.Job) jobResp {
	r := jobResp{
		ID:        j.ID,
		Status:    j.Status,
		Progress:  j.Progress,
		ETA:       j.ETA,
		Error:     j.Error,
		CreatedAt: j.CreatedAt,
		StartedAt: j.StartedAt,
		DoneAt:    j.DoneAt,
	}
	if j.Duration() > 0 {
		r.Duration = j.Duration().Round(time.Millisecond).String()
	}
	if j.Status == jobs.StatusDone {
		r.DownloadURL = "/jobs/" + j.ID + "/download"
	}
	return r
}

// responseWriter wraps http.ResponseWriter to capture the status code for logging.
type responseWriter struct {
	http.ResponseWriter
	code int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.code = code
	rw.ResponseWriter.WriteHeader(code)
}
