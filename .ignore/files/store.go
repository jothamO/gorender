package jobs

import (
	"fmt"
	"sync"
	"time"

	"github.com/makemoments/gorender/internal/composition"
)

// Status represents the lifecycle of a render job.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRendering Status = "rendering"
	StatusDone      Status = "done"
	StatusFailed    Status = "failed"
)

// Job represents a single render job.
type Job struct {
	ID        string     `json:"id"`
	Status    Status     `json:"status"`
	Progress  float64    `json:"progress"`  // 0.0 → 1.0
	ETA       string     `json:"eta"`       // human-readable, e.g. "14s"
	OutputPath string    `json:"outputPath,omitempty"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	DoneAt    *time.Time `json:"doneAt,omitempty"`

	// Internal — not serialized to API responses.
	Comp    *composition.Composition `json:"-"`
	retries int
}

// Duration returns how long the job has been running, or total time if done.
func (j *Job) Duration() time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	if j.DoneAt != nil {
		return j.DoneAt.Sub(*j.StartedAt)
	}
	return time.Since(*j.StartedAt)
}

// Store is a thread-safe in-memory job registry.
// Designed so the interface can be backed by Redis or SQLite later
// without changing any call sites.
type Store struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewStore creates an empty job store.
func NewStore() *Store {
	return &Store{jobs: make(map[string]*Job)}
}

// Put inserts or replaces a job.
func (s *Store) Put(job *Job) {
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
}

// Get retrieves a job by ID. Returns nil if not found.
func (s *Store) Get(id string) *Job {
	s.mu.RLock()
	j := s.jobs[id]
	s.mu.RUnlock()
	return j
}

// Update applies a mutation function to a job under the write lock.
// This is the safe way to transition job state.
func (s *Store) Update(id string, fn func(*Job)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	fn(j)
	return nil
}

// List returns all jobs ordered by creation time (newest first).
func (s *Store) List() []*Job {
	s.mu.RLock()
	out := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	s.mu.RUnlock()

	// Sort newest first.
	for i := 0; i < len(out)-1; i++ {
		for k := i + 1; k < len(out); k++ {
			if out[k].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[k] = out[k], out[i]
			}
		}
	}
	return out
}

// Prune deletes completed or failed jobs older than maxAge.
// Call this periodically to prevent unbounded memory growth.
func (s *Store) Prune(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	pruned := 0
	for id, j := range s.jobs {
		if (j.Status == StatusDone || j.Status == StatusFailed) &&
			j.CreatedAt.Before(cutoff) {
			delete(s.jobs, id)
			pruned++
		}
	}
	return pruned
}

// Counts returns a snapshot of job counts by status.
func (s *Store) Counts() map[Status]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[Status]int{
		StatusQueued:    0,
		StatusRendering: 0,
		StatusDone:      0,
		StatusFailed:    0,
	}
	for _, j := range s.jobs {
		counts[j.Status]++
	}
	return counts
}
