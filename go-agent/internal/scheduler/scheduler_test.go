package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSubmitWaitAndStats(t *testing.T) {
	s := New(1, 8, func(_ context.Context, job Job) (any, error) {
		time.Sleep(10 * time.Millisecond)
		return map[string]any{"method": job.Method, "ok": true}, nil
	})
	defer s.Stop()

	job, err := s.Submit("req-1", "sess-1", "agent", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	done, ok := s.Wait(context.Background(), job.ID, 3*time.Second)
	if !ok {
		t.Fatalf("wait returned missing job")
	}
	if done.State != JobSucceeded {
		t.Fatalf("expected succeeded, got %s", done.State)
	}
	if done.Error != "" {
		t.Fatalf("unexpected error: %s", done.Error)
	}

	stats := s.SnapshotStats()
	if stats.Succeeded < 1 {
		t.Fatalf("expected succeeded job count, got %+v", stats)
	}
}

func TestFailurePath(t *testing.T) {
	s := New(1, 8, func(_ context.Context, _ Job) (any, error) {
		return nil, errors.New("boom")
	})
	defer s.Stop()

	job, err := s.Submit("req-2", "sess-2", "send", nil)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	done, ok := s.Wait(context.Background(), job.ID, 3*time.Second)
	if !ok {
		t.Fatalf("wait returned missing job")
	}
	if done.State != JobFailed {
		t.Fatalf("expected failed, got %s", done.State)
	}
	if done.Error == "" {
		t.Fatalf("expected error text on failed job")
	}
}

func TestSubmitReturnsQueuedSnapshot(t *testing.T) {
	s := New(1, 128, func(_ context.Context, _ Job) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	defer s.Stop()

	for i := 0; i < 200; i++ {
		job, err := s.Submit("req-race", "sess-race", "agent", map[string]any{"i": i})
		if err != nil {
			t.Fatalf("submit failed at %d: %v", i, err)
		}
		if job.State != JobQueued {
			t.Fatalf("submit should return queued snapshot, got %s at %d", job.State, i)
		}
		if job.SubmittedAt.IsZero() {
			t.Fatalf("submit should return non-zero submittedAt at %d", i)
		}
	}
}
