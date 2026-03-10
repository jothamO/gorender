package jobs

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestStore_PutAndGet(t *testing.T) {
	s := NewStore()
	job := &Job{ID: "j_1", Status: StatusQueued, CreatedAt: time.Now()}
	s.Put(job)

	got := s.Get("j_1")
	if got == nil || got.ID != "j_1" {
		t.Fatal("expected to get job j_1")
	}
	if s.Get("j_nope") != nil {
		t.Fatal("expected nil for missing job")
	}
}

func TestStore_Update(t *testing.T) {
	s := NewStore()
	s.Put(&Job{ID: "j_2", Status: StatusQueued, CreatedAt: time.Now()})

	err := s.Update("j_2", func(j *Job) {
		j.Status = StatusRendering
		j.Progress = 0.5
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got := s.Get("j_2")
	if got.Status != StatusRendering {
		t.Errorf("expected rendering, got %s", got.Status)
	}
	if got.Progress != 0.5 {
		t.Errorf("expected 0.5, got %f", got.Progress)
	}
}

func TestStore_Update_MissingJob(t *testing.T) {
	s := NewStore()
	err := s.Update("j_nope", func(j *Job) {})
	if err == nil {
		t.Fatal("expected error for missing job")
	}
}

func TestStore_List_OrderedNewestFirst(t *testing.T) {
	s := NewStore()
	for i := 0; i < 5; i++ {
		s.Put(&Job{
			ID:        fmt.Sprintf("j_%d", i),
			Status:    StatusQueued,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	list := s.List()
	if len(list) != 5 {
		t.Fatalf("expected 5, got %d", len(list))
	}
	// j_4 should be first (newest).
	if list[0].ID != "j_4" {
		t.Errorf("expected j_4 first, got %s", list[0].ID)
	}
}

func TestStore_Prune(t *testing.T) {
	s := NewStore()
	old := &Job{
		ID:        "old",
		Status:    StatusDone,
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}
	fresh := &Job{
		ID:        "fresh",
		Status:    StatusDone,
		CreatedAt: time.Now(),
	}
	queued := &Job{
		ID:        "queued",
		Status:    StatusQueued,
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}
	s.Put(old)
	s.Put(fresh)
	s.Put(queued)

	pruned := s.Prune(24 * time.Hour)
	if pruned != 1 {
		t.Errorf("expected 1 pruned, got %d", pruned)
	}
	if s.Get("old") != nil {
		t.Error("old job should be pruned")
	}
	if s.Get("fresh") == nil {
		t.Error("fresh job should remain")
	}
	if s.Get("queued") == nil {
		t.Error("queued job (non-terminal) should remain")
	}
}

func TestStore_Counts(t *testing.T) {
	s := NewStore()
	s.Put(&Job{ID: "a", Status: StatusQueued, CreatedAt: time.Now()})
	s.Put(&Job{ID: "b", Status: StatusQueued, CreatedAt: time.Now()})
	s.Put(&Job{ID: "c", Status: StatusRendering, CreatedAt: time.Now()})
	s.Put(&Job{ID: "d", Status: StatusDone, CreatedAt: time.Now()})
	s.Put(&Job{ID: "e", Status: StatusFailed, CreatedAt: time.Now()})

	counts := s.Counts()
	if counts[StatusQueued] != 2 {
		t.Errorf("queued: expected 2, got %d", counts[StatusQueued])
	}
	if counts[StatusRendering] != 1 {
		t.Errorf("rendering: expected 1, got %d", counts[StatusRendering])
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup

	// Concurrent puts.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Put(&Job{ID: fmt.Sprintf("j_%d", n), Status: StatusQueued, CreatedAt: time.Now()})
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Get(fmt.Sprintf("j_%d", n))
			s.List()
			s.Counts()
		}(i)
	}

	wg.Wait()
}
