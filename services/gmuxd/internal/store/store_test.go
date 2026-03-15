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
		ID:    "s1",
		Kind:  "pi",
		Alive: true,
		Title: "test",
	})

	got, ok := s.Get("s1")
	if !ok {
		t.Fatal("expected session to exist")
	}
	if !got.Alive {
		t.Fatal("expected alive")
	}
	if got.Title != "test" {
		t.Fatalf("expected title 'test', got %q", got.Title)
	}
}

func TestUpsertOverwrite(t *testing.T) {
	s := New()
	s.Upsert(Session{ID: "s1", Kind: "pi", Title: "v1"})
	s.Upsert(Session{ID: "s1", Kind: "pi", Title: "v2"})

	got, _ := s.Get("s1")
	if got.Title != "v2" {
		t.Fatalf("expected v2, got %q", got.Title)
	}
	if len(s.List()) != 1 {
		t.Fatal("expected 1 session after overwrite")
	}
}

func TestRemove(t *testing.T) {
	s := New()
	s.Upsert(Session{ID: "s1", Kind: "pi"})

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

	s.Upsert(Session{ID: "s1", Kind: "pi", Alive: true})

	select {
	case ev := <-ch:
		if ev.Type != "session-upsert" || ev.ID != "s1" {
			t.Fatalf("unexpected event: %+v", ev)
		}
		if ev.Session == nil || !ev.Session.Alive {
			t.Fatal("expected session in event")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}

	s.Remove("s1")

	select {
	case ev := <-ch:
		if ev.Type != "session-remove" || ev.ID != "s1" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for remove event")
	}
}

func TestSetTerminalSize(t *testing.T) {
	s := New()
	s.Upsert(Session{ID: "s1", Kind: "shell", Alive: true})

	if !s.SetTerminalSize("s1", 120, 40) {
		t.Fatal("expected terminal size update to succeed")
	}

	got, ok := s.Get("s1")
	if !ok {
		t.Fatal("expected session to exist")
	}
	if got.TerminalCols != 120 || got.TerminalRows != 40 {
		t.Fatalf("expected terminal size 120x40, got %dx%d", got.TerminalCols, got.TerminalRows)
	}
	if s.SetTerminalSize("missing", 120, 40) {
		t.Fatal("expected missing session update to fail")
	}
}
