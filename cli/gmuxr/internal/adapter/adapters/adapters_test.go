package adapters

import (
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
