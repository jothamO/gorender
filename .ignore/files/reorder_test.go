package pipeline

import (
	"fmt"
	"sync"
	"testing"
)

func TestReorderBuffer_InOrder(t *testing.T) {
	total := 10
	rb := NewReorderBuffer(total, 16)

	for i := 0; i < total; i++ {
		rb.Push(i, fmt.Sprintf("frame-%d.png", i))
	}

	var got []int
	for f := range rb.Out() {
		got = append(got, f.Frame)
	}

	if len(got) != total {
		t.Fatalf("expected %d frames, got %d", total, len(got))
	}
	for i, v := range got {
		if v != i {
			t.Errorf("position %d: expected frame %d, got %d", i, i, v)
		}
	}
}

func TestReorderBuffer_OutOfOrder(t *testing.T) {
	total := 5
	rb := NewReorderBuffer(total, 16)

	// Push in reverse order.
	for i := total - 1; i >= 0; i-- {
		rb.Push(i, fmt.Sprintf("frame-%d.png", i))
	}

	var got []int
	for f := range rb.Out() {
		got = append(got, f.Frame)
	}

	if len(got) != total {
		t.Fatalf("expected %d frames, got %d", total, len(got))
	}
	for i, v := range got {
		if v != i {
			t.Errorf("out-of-order: position %d expected frame %d, got %d", i, i, v)
		}
	}
}

func TestReorderBuffer_ConcurrentPush(t *testing.T) {
	total := 100
	rb := NewReorderBuffer(total, 32)

	var wg sync.WaitGroup
	// Simulate N concurrent workers each pushing one frame.
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(frame int) {
			defer wg.Done()
			rb.Push(frame, fmt.Sprintf("frame-%06d.png", frame))
		}(i)
	}

	// Drain concurrently with pushes.
	var got []int
	doneCh := make(chan struct{})
	go func() {
		for f := range rb.Out() {
			got = append(got, f.Frame)
		}
		close(doneCh)
	}()

	wg.Wait()
	<-doneCh

	if len(got) != total {
		t.Fatalf("concurrent: expected %d frames, got %d", total, len(got))
	}

	// Verify sequential ordering.
	for i, v := range got {
		if v != i {
			t.Errorf("concurrent: position %d expected frame %d, got %d", i, i, v)
		}
	}
}

func TestReorderBuffer_ChannelClosed(t *testing.T) {
	rb := NewReorderBuffer(3, 8)
	rb.Push(0, "a.png")
	rb.Push(1, "b.png")
	rb.Push(2, "c.png")

	count := 0
	for range rb.Out() {
		count++
	}
	// Channel should be closed after all frames delivered.
	if count != 3 {
		t.Errorf("expected 3 frames, got %d", count)
	}
}

func TestReorderBuffer_SingleFrame(t *testing.T) {
	rb := NewReorderBuffer(1, 4)
	rb.Push(0, "only.png")

	frames := make([]FrameInOrder, 0)
	for f := range rb.Out() {
		frames = append(frames, f)
	}

	if len(frames) != 1 || frames[0].Frame != 0 || frames[0].Path != "only.png" {
		t.Errorf("single frame: unexpected result %+v", frames)
	}
}
