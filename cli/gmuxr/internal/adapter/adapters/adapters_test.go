package adapters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gmuxapp/gmux/cli/gmuxr/internal/adapter"
)

// --- Generic adapter tests ---

func TestGenericMatchAll(t *testing.T) {
	g := NewShell()
	if !g.Match([]string{"anything"}) {
		t.Fatal("shell should match any command")
	}
	if !g.Match([]string{}) {
		t.Fatal("shell should match empty command")
	}
}

func TestGenericName(t *testing.T) {
	g := NewShell()
	if g.Name() != "shell" {
		t.Fatalf("expected 'shell', got %q", g.Name())
	}
}

func TestShellEnvNil(t *testing.T) {
	g := NewShell()
	env := g.Env(adapter.EnvContext{})
	if env != nil {
		t.Fatalf("expected nil env, got %v", env)
	}
}

func TestShellMonitorPlainOutput(t *testing.T) {
	g := NewShell()
	status := g.Monitor([]byte("hello"))
	if status != nil {
		t.Fatal("shell should not report status for plain output")
	}
}

func TestShellCheckSilenceNoop(t *testing.T) {
	g := NewShell()
	g.Monitor([]byte("hello"))
	if s := g.CheckSilence(); s != nil {
		t.Fatal("shell should never report silence status")
	}
}

// --- OSC title parsing tests ---

func TestParseOSCTitleBEL(t *testing.T) {
	// ESC ] 0 ; hello BEL
	data := []byte("\x1b]0;my title\x07 more data")
	title := parseOSCTitle(data)
	if title != "my title" {
		t.Fatalf("expected 'my title', got %q", title)
	}
}

func TestParseOSCTitleST(t *testing.T) {
	// ESC ] 2 ; hello ESC backslash
	data := []byte("\x1b]2;window title\x1b\\ more")
	title := parseOSCTitle(data)
	if title != "window title" {
		t.Fatalf("expected 'window title', got %q", title)
	}
}

func TestParseOSCTitleNone(t *testing.T) {
	data := []byte("hello world no escape here")
	title := parseOSCTitle(data)
	if title != "" {
		t.Fatalf("expected empty, got %q", title)
	}
}

func TestParseOSCTitleEmbedded(t *testing.T) {
	// Title buried in other output
	data := []byte("some output\r\n\x1b]0;~/dev/gmux\x07prompt $ ")
	title := parseOSCTitle(data)
	if title != "~/dev/gmux" {
		t.Fatalf("expected '~/dev/gmux', got %q", title)
	}
}

func TestShellMonitorTitleUpdate(t *testing.T) {
	g := NewShell()
	status := g.Monitor([]byte("\x1b]0;fish: ~/dev\x07"))
	if status == nil {
		t.Fatal("should return status")
	}
	if status.Title != "fish: ~/dev" {
		t.Fatalf("expected title 'fish: ~/dev', got %q", status.Title)
	}
	if status.State != "" {
		t.Fatalf("expected no state, got %q", status.State)
	}
}

// --- Pi adapter tests ---

func TestPiName(t *testing.T) {
	p := NewPi()
	if p.Name() != "pi" {
		t.Fatalf("expected 'pi', got %q", p.Name())
	}
}

func TestPiMatchDirect(t *testing.T) {
	p := NewPi()
	if !p.Match([]string{"pi"}) {
		t.Fatal("should match 'pi'")
	}
	if !p.Match([]string{"pi-coding-agent"}) {
		t.Fatal("should match 'pi-coding-agent'")
	}
}

func TestPiMatchWrapped(t *testing.T) {
	p := NewPi()
	if !p.Match([]string{"npx", "pi"}) {
		t.Fatal("should match 'npx pi'")
	}
	if !p.Match([]string{"env", "pi", "--flag"}) {
		t.Fatal("should match 'env pi --flag'")
	}
	if !p.Match([]string{"/home/user/.local/bin/pi"}) {
		t.Fatal("should match full path")
	}
}

func TestPiMatchStopsAtDoubleDash(t *testing.T) {
	p := NewPi()
	if p.Match([]string{"echo", "--", "pi"}) {
		t.Fatal("should not match 'pi' after '--'")
	}
}

func TestPiNoMatchOther(t *testing.T) {
	p := NewPi()
	if p.Match([]string{"pytest", "tests/"}) {
		t.Fatal("should not match pytest")
	}
	if p.Match([]string{"pipeline"}) {
		t.Fatal("should not match 'pipeline' (contains 'pi' but base name is 'pipeline')")
	}
}

func TestPiEnvNil(t *testing.T) {
	p := NewPi()
	env := p.Env(adapter.EnvContext{SessionID: "sess-test"})
	if env != nil {
		t.Fatalf("expected nil env, got %v", env)
	}
}

func TestPiMonitorPlainOutput(t *testing.T) {
	p := NewPi()
	if s := p.Monitor([]byte("some output")); s != nil {
		t.Fatal("should return nil for non-spinner output")
	}
}

func TestPiMonitorSpinner(t *testing.T) {
	p := NewPi()
	s := p.Monitor([]byte("⠋ Working..."))
	if s == nil {
		t.Fatal("should detect spinner")
	}
	if s.State != "active" || s.Label != "working" {
		t.Fatalf("expected active/working, got %s/%s", s.State, s.Label)
	}
}

// --- Pi session info tests ---

func writeTempJSONL(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test-session.jsonl")
	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadPiSessionInfoFirstUserMessage(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"session","version":3,"id":"abc-123","timestamp":"2026-03-15T10:00:00Z","cwd":"/tmp/test"}`,
		`{"type":"model_change","id":"m1","timestamp":"2026-03-15T10:00:00Z"}`,
		`{"type":"message","id":"u1","timestamp":"2026-03-15T10:01:00Z","message":{"role":"user","content":[{"type":"text","text":"Fix the auth bug in login.go"}]}}`,
		`{"type":"message","id":"a1","timestamp":"2026-03-15T10:01:05Z","message":{"role":"assistant","content":[{"type":"text","text":"I'll fix that for you."}]}}`,
	)

	info, err := ReadPiSessionInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "abc-123" {
		t.Errorf("expected id abc-123, got %s", info.ID)
	}
	if info.Cwd != "/tmp/test" {
		t.Errorf("expected cwd /tmp/test, got %s", info.Cwd)
	}
	if info.Title != "Fix the auth bug in login.go" {
		t.Errorf("expected first user msg as title, got %q", info.Title)
	}
	if info.MessageCount != 2 {
		t.Errorf("expected 2 messages, got %d", info.MessageCount)
	}
}

func TestReadPiSessionInfoNameOverridesFirstMessage(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"session","version":3,"id":"abc-456","timestamp":"2026-03-15T10:00:00Z","cwd":"/tmp/test"}`,
		`{"type":"message","id":"u1","timestamp":"2026-03-15T10:01:00Z","message":{"role":"user","content":[{"type":"text","text":"Fix the auth bug"}]}}`,
		`{"type":"session_info","name":"  Auth refactor  "}`,
		`{"type":"message","id":"a1","timestamp":"2026-03-15T10:01:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`,
	)

	info, err := ReadPiSessionInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "Auth refactor" {
		t.Errorf("expected session_info name as title, got %q", info.Title)
	}
}

func TestReadPiSessionInfoNoMessages(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"session","version":3,"id":"abc-789","timestamp":"2026-03-15T10:00:00Z","cwd":"/tmp/test"}`,
	)

	info, err := ReadPiSessionInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "(no messages)" {
		t.Errorf("expected fallback title, got %q", info.Title)
	}
	if info.MessageCount != 0 {
		t.Errorf("expected 0 messages, got %d", info.MessageCount)
	}
}

func TestReadPiSessionInfoLongTitleTruncated(t *testing.T) {
	long := "Please help me with this very long request that goes on and on about many different things and really should be truncated for the sidebar"
	path := writeTempJSONL(t,
		`{"type":"session","version":3,"id":"abc-long","timestamp":"2026-03-15T10:00:00Z","cwd":"/tmp/test"}`,
		`{"type":"message","id":"u1","timestamp":"2026-03-15T10:01:00Z","message":{"role":"user","content":[{"type":"text","text":"`+long+`"}]}}`,
	)

	info, err := ReadPiSessionInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Title) > 85 { // 80 + "…"
		t.Errorf("title too long (%d chars): %q", len(info.Title), info.Title)
	}
	if info.Title[len(info.Title)-3:] != "…" {
		t.Errorf("expected truncation marker, got %q", info.Title)
	}
}

func TestReadPiSessionInfoStringContent(t *testing.T) {
	// Some older formats use plain string content instead of array
	path := writeTempJSONL(t,
		`{"type":"session","version":3,"id":"abc-str","timestamp":"2026-03-15T10:00:00Z","cwd":"/tmp/test"}`,
		`{"type":"message","id":"u1","timestamp":"2026-03-15T10:01:00Z","message":{"role":"user","content":"Help me debug this"}}`,
	)

	info, err := ReadPiSessionInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "Help me debug this" {
		t.Errorf("expected string content as title, got %q", info.Title)
	}
}

func TestPiSessionDirEncoding(t *testing.T) {
	// Can't test the full path (depends on $HOME) but test the encoding logic
	dir := PiSessionDir("/home/mg/dev/gmux")
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %s", dir)
	}
	base := filepath.Base(dir)
	if base != "--home-mg-dev-gmux--" {
		t.Errorf("expected --home-mg-dev-gmux--, got %s", base)
	}
}

func TestListSessionFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "b.jsonl"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not a session"), 0644)

	files := ListSessionFiles(dir)
	if len(files) != 2 {
		t.Errorf("expected 2 jsonl files, got %d", len(files))
	}
}
