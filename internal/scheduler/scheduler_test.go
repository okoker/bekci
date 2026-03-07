package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/bekci/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewScheduler(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if len(s.timers) != 0 {
		t.Fatalf("expected empty timers map, got %d", len(s.timers))
	}
	if s.eventCh == nil {
		t.Fatal("expected non-nil eventCh")
	}
}

func TestRunNowChannel(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	s.RunNow("check-123")
	select {
	case id := <-s.eventCh:
		if id != "check-123" {
			t.Fatalf("got %s, want check-123", id)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestStartStop(t *testing.T) {
	st := newTestStore(t)
	s := New(st)
	s.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}
