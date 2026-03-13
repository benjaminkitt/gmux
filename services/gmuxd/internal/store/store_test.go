package store

import (
	"testing"
	"time"
)

func TestListEmpty(t *testing.T) {
	s := New()
	if len(s.List()) != 0 {
		t.Fatal("expected empty list")
	}
}

func TestUpsertAndGet(t *testing.T) {
	s := New()
	s.Upsert(Session{
		SessionID:  "s1",
		AbducoName: "pi:test:1",
		Kind:       "pi",
		State:      "running",
	})

	got, ok := s.Get("s1")
	if !ok {
		t.Fatal("expected session to exist")
	}
	if got.State != "running" {
		t.Fatalf("expected running, got %s", got.State)
	}
}

func TestSetState(t *testing.T) {
	s := New()
	s.Upsert(Session{SessionID: "s1", Kind: "pi", State: "running"})

	updated, ok := s.SetState("s1", "waiting")
	if !ok {
		t.Fatal("expected success")
	}
	if updated.State != "waiting" {
		t.Fatalf("expected waiting, got %s", updated.State)
	}

	_, ok = s.SetState("nonexistent", "running")
	if ok {
		t.Fatal("expected failure for nonexistent session")
	}
}

func TestRemove(t *testing.T) {
	s := New()
	s.Upsert(Session{SessionID: "s1", Kind: "pi", State: "running"})

	if !s.Remove("s1") {
		t.Fatal("expected remove to succeed")
	}
	if s.Remove("s1") {
		t.Fatal("expected second remove to return false")
	}
	if len(s.List()) != 0 {
		t.Fatal("expected empty list after remove")
	}
}

func TestSubscribe(t *testing.T) {
	s := New()
	ch, cancel := s.Subscribe()
	defer cancel()

	s.Upsert(Session{SessionID: "s1", Kind: "pi", State: "running"})

	select {
	case ev := <-ch:
		if ev.Type != "session-upsert" || ev.SessionID != "s1" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}

	s.SetState("s1", "waiting")

	select {
	case ev := <-ch:
		if ev.Type != "session-state" || ev.State != "waiting" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for state event")
	}
}

func TestSeeds(t *testing.T) {
	s := NewWithSeeds()
	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 seeded sessions, got %d", len(list))
	}
}
