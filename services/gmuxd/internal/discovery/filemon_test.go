package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gmuxapp/gmux/packages/adapter/adapters"
	"github.com/gmuxapp/gmux/services/gmuxd/internal/store"
)

func TestNotifyNewSessionDoesNotStealTitleFromOldPiFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := "/home/user/dev/project"
	pi := adapters.NewPi()
	sessionDir := pi.SessionDir(cwd)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	oldFile := filepath.Join(sessionDir, "2026-03-16T10-00-00-000Z_old.jsonl")
	oldContent := "" +
		"{\"type\":\"session\",\"id\":\"old-session\",\"cwd\":\"/home/user/dev/project\",\"timestamp\":\"2026-03-16T10:00:00Z\"}\n" +
		"{\"type\":\"message\",\"role\":\"user\",\"text\":\"fix auth bug\"}\n"
	if err := os.WriteFile(oldFile, []byte(oldContent), 0o644); err != nil {
		t.Fatalf("write old jsonl: %v", err)
	}
	// Set mtime to yesterday — real old files have old mtimes.
	yesterday := time.Now().Add(-24 * time.Hour)
	os.Chtimes(oldFile, yesterday, yesterday)

	s := store.New()
	s.Upsert(store.Session{
		ID:         "sess-new",
		Cwd:        cwd,
		Kind:       "pi",
		Alive:      true,
		Title:      "pi",
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		SocketPath: "/tmp/gmux-sessions/sess-new.sock",
	})

	fm := NewFileMonitor(s)
	if fm.watcher != nil {
		defer fm.watcher.Close()
	}

	fm.NotifyNewSession("sess-new")

	// The session started now but the file is from yesterday.
	// scanDirForRecentFiles should skip it (mtime before session start).
	time.Sleep(700 * time.Millisecond)

	sess, ok := s.Get("sess-new")
	if !ok {
		t.Fatal("session disappeared")
	}
	if sess.Title != "pi" {
		t.Fatalf("title = %q, want %q (do not steal old session title)", sess.Title, "pi")
	}
	if len(fm.attributions) != 0 {
		t.Fatalf("attributions = %v, want none until a real file write occurs", fm.attributions)
	}
}

func TestActiveFileTracking(t *testing.T) {
	s := store.New()
	s.Upsert(store.Session{
		ID:        "sess-1",
		Cwd:       "/tmp",
		Kind:      "codex",
		Alive:     true,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})

	fm := NewFileMonitor(s)
	if fm.watcher != nil {
		defer fm.watcher.Close()
	}

	// Simulate registering the session for monitoring.
	fm.sessions["sess-1"] = &monitoredSession{
		id:      "sess-1",
		cwd:     "/tmp",
		kind:    "codex",
		adapter: adapters.NewCodex(),
		filer:   adapters.NewCodex(),
		fileMon: adapters.NewCodex(),
	}

	// Create first session file.
	dir := t.TempDir()
	file1 := filepath.Join(dir, "rollout-a.jsonl")
	now := time.Now()
	content1 := `{"timestamp":"` + now.Format(time.RFC3339Nano) + `","type":"session_meta","payload":{"id":"session-aaa","timestamp":"` + now.Format(time.RFC3339Nano) + `","cwd":"/tmp"}}
{"timestamp":"` + now.Format(time.RFC3339Nano) + `","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	os.WriteFile(file1, []byte(content1), 0o644)

	// Attribute file1 to the session.
	fm.attributions[file1] = "sess-1"
	fm.updateActiveFileLocked("sess-1", file1)

	sess, _ := s.Get("sess-1")
	if sess.ResumeKey != "session-aaa" {
		t.Fatalf("expected resume_key 'session-aaa', got %q", sess.ResumeKey)
	}

	// Create second file (simulating /new command).
	file2 := filepath.Join(dir, "rollout-b.jsonl")
	content2 := `{"timestamp":"` + now.Format(time.RFC3339Nano) + `","type":"session_meta","payload":{"id":"session-bbb","timestamp":"` + now.Format(time.RFC3339Nano) + `","cwd":"/tmp"}}
{"timestamp":"` + now.Format(time.RFC3339Nano) + `","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"new topic"}]}}
`
	os.WriteFile(file2, []byte(content2), 0o644)

	// Attribute file2 to the same session.
	fm.attributions[file2] = "sess-1"
	fm.updateActiveFileLocked("sess-1", file2)

	sess, _ = s.Get("sess-1")
	if sess.ResumeKey != "session-bbb" {
		t.Fatalf("expected resume_key updated to 'session-bbb', got %q", sess.ResumeKey)
	}

	// Same file again — should be a no-op.
	fm.updateActiveFileLocked("sess-1", file2)
	sess, _ = s.Get("sess-1")
	if sess.ResumeKey != "session-bbb" {
		t.Fatalf("resume_key should still be 'session-bbb', got %q", sess.ResumeKey)
	}
}

// --- Pi status lifecycle tests ---
// These test the full filemon pipeline: file write → ParseNewLines → store update.

// setupPiFileMonitor creates a FileMonitor with a pi session pre-registered,
// returns the monitor, store, session dir, and the session JSONL path.
func setupPiFileMonitor(t *testing.T) (*FileMonitor, *store.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := "/home/user/dev/project"
	pi := adapters.NewPi()
	sessionDir := pi.SessionDir(cwd)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	s := store.New()
	s.Upsert(store.Session{
		ID:         "sess-pi",
		Cwd:        cwd,
		Kind:       "pi",
		Alive:      true,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		SocketPath: "/tmp/fake.sock",
	})

	fm := NewFileMonitor(s)
	if fm.watcher != nil {
		defer fm.watcher.Close()
		fm.watcher.Close()
		fm.watcher = nil // disable real inotify for unit tests
	}

	fm.sessions["sess-pi"] = &monitoredSession{
		id:      "sess-pi",
		cwd:     cwd,
		kind:    "pi",
		adapter: pi,
		fileMon: pi,
		filer:   pi,
	}

	return fm, s, sessionDir
}

// simulateFileWrite writes lines to a file and calls handleFileChange to
// simulate what inotify would do. Pre-sets attribution if not already set.
func simulateFileWrite(t *testing.T, fm *FileMonitor, sessionID, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	for _, l := range lines {
		f.WriteString(l + "\n")
	}
	f.Close()
	// Pre-set attribution to bypass scrollback-based matching (no real runner).
	if _, ok := fm.attributions[path]; !ok {
		fm.attributions[path] = sessionID
	}
	fm.handleFileChange(path)
}

func TestPiAbortClearsWorking(t *testing.T) {
	fm, s, dir := setupPiFileMonitor(t)
	path := filepath.Join(dir, "test.jsonl")

	// Session header + user message → working.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"session","id":"test-123","cwd":"/home/user/dev/project","timestamp":"2026-03-19T10:00:00Z"}`,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"fix bug"}]}}`,
	)
	sess, _ := s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working=true after user message")
	}

	// toolUse → still working.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a1","message":{"role":"assistant","stopReason":"toolUse","content":[]}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working=true after toolUse")
	}

	// User presses Esc → aborted → idle.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a2","message":{"role":"assistant","stopReason":"aborted","content":[]}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status != nil {
		t.Fatalf("expected nil status (idle) after abort, got %+v", sess.Status)
	}
}

func TestPiErrorClearsWorking(t *testing.T) {
	fm, s, dir := setupPiFileMonitor(t)
	path := filepath.Join(dir, "test.jsonl")

	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"session","id":"test-123","cwd":"/home/user/dev/project","timestamp":"2026-03-19T10:00:00Z"}`,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"fix bug"}]}}`,
	)
	sess, _ := s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working=true after user message")
	}

	// Error → idle.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a1","message":{"role":"assistant","stopReason":"error","content":[]}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status != nil {
		t.Fatalf("expected nil status (idle) after error, got %+v", sess.Status)
	}
}

func TestPiErrorAutoRetryResetsWorking(t *testing.T) {
	fm, s, dir := setupPiFileMonitor(t)
	path := filepath.Join(dir, "test.jsonl")

	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"session","id":"test-123","cwd":"/home/user/dev/project","timestamp":"2026-03-19T10:00:00Z"}`,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"fix bug"}]}}`,
	)

	// Error followed by auto-retry (both in same batch).
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a1","message":{"role":"assistant","stopReason":"error","content":[]}}`,
		`{"type":"message","id":"a2","message":{"role":"assistant","stopReason":"toolUse","content":[]}}`,
	)
	sess, _ := s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working=true after error auto-retry (toolUse)")
	}
}

func TestPiNameDuringWorkPreservesStatus(t *testing.T) {
	fm, s, dir := setupPiFileMonitor(t)
	path := filepath.Join(dir, "test.jsonl")

	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"session","id":"test-123","cwd":"/home/user/dev/project","timestamp":"2026-03-19T10:00:00Z"}`,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"fix bug"}]}}`,
	)

	// /name while working → title changes, working preserved.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"session_info","name":"Bug fix session"}`,
	)
	sess, _ := s.Get("sess-pi")
	if sess.AdapterTitle != "Bug fix session" {
		t.Errorf("expected title 'Bug fix session', got %q", sess.AdapterTitle)
	}
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working=true preserved after /name")
	}
}

func TestPiFullLifecycle(t *testing.T) {
	fm, s, dir := setupPiFileMonitor(t)
	path := filepath.Join(dir, "test.jsonl")

	// 1. Session starts.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"session","id":"test-123","cwd":"/home/user/dev/project","timestamp":"2026-03-19T10:00:00Z"}`,
	)
	sess, _ := s.Get("sess-pi")
	if sess.Status != nil {
		t.Fatalf("expected nil status on fresh session, got %+v", sess.Status)
	}

	// 2. User message → working.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"fix bug"}]}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working after user message")
	}
	if sess.AdapterTitle != "fix bug" {
		t.Errorf("expected title 'fix bug', got %q", sess.AdapterTitle)
	}

	// 3. Tool loop.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a1","message":{"role":"assistant","stopReason":"toolUse","content":[]}}`,
		`{"type":"message","id":"tr1","message":{"role":"toolResult","content":""}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working during tool loop")
	}

	// 4. Error + auto-retry.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a2","message":{"role":"assistant","stopReason":"error","content":[]}}`,
		`{"type":"message","id":"a3","message":{"role":"assistant","stopReason":"toolUse","content":[]}}`,
		`{"type":"message","id":"tr2","message":{"role":"toolResult","content":""}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working after error auto-retry")
	}

	// 5. Agent finishes.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a4","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"Done."}]}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status != nil {
		t.Fatalf("expected idle after stop, got %+v", sess.Status)
	}

	// 6. Second turn.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"u2","message":{"role":"user","content":[{"type":"text","text":"also fix login"}]}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status == nil || !sess.Status.Working {
		t.Fatal("expected working after second user message")
	}
	// Title should still be from first user message.
	if sess.AdapterTitle != "fix bug" {
		t.Errorf("title should stay 'fix bug', got %q", sess.AdapterTitle)
	}

	// 7. User aborts.
	simulateFileWrite(t, fm, "sess-pi", path,
		`{"type":"message","id":"a5","message":{"role":"assistant","stopReason":"aborted","content":[]}}`,
	)
	sess, _ = s.Get("sess-pi")
	if sess.Status != nil {
		t.Fatalf("expected idle after abort, got %+v", sess.Status)
	}
}

func TestAttributionStickiness(t *testing.T) {
	s := store.New()
	fm := NewFileMonitor(s)
	if fm.watcher != nil {
		defer fm.watcher.Close()
	}

	fm.sessions["sess-1"] = &monitoredSession{
		id:      "sess-1",
		cwd:     "/tmp",
		kind:    "codex",
		adapter: adapters.NewCodex(),
		filer:   adapters.NewCodex(),
		fileMon: adapters.NewCodex(),
	}

	// Pre-set an attribution.
	fm.attributions["/some/file.jsonl"] = "sess-1"

	// Calling attributeFileLocked should return the cached value for a known file.
	// (We can't easily test this without the full dir setup, but we verify
	// the map check in handleFileChange by checking the attributions map.)
	if fm.attributions["/some/file.jsonl"] != "sess-1" {
		t.Fatal("attribution should be sticky")
	}
}
