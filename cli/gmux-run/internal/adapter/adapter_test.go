package adapter

import (
	"os"
	"testing"
	"time"
)

// --- Generic adapter tests ---

func TestGenericMatchAll(t *testing.T) {
	g := NewGeneric(0)
	if !g.Match([]string{"anything"}) {
		t.Fatal("generic should match any command")
	}
	if !g.Match([]string{}) {
		t.Fatal("generic should match empty command")
	}
}

func TestGenericName(t *testing.T) {
	g := NewGeneric(0)
	if g.Name() != "generic" {
		t.Fatalf("expected 'generic', got %q", g.Name())
	}
}

func TestGenericPreparePassthrough(t *testing.T) {
	g := NewGeneric(0)
	cmd, env := g.Prepare(PrepareContext{
		Command: []string{"echo", "hello"},
	})
	if len(cmd) != 2 || cmd[0] != "echo" || cmd[1] != "hello" {
		t.Fatalf("expected passthrough, got %v", cmd)
	}
	if len(env) != 0 {
		t.Fatalf("expected no env, got %v", env)
	}
}

func TestGenericMonitorFirstOutput(t *testing.T) {
	g := NewGeneric(time.Second)
	status := g.Monitor([]byte("hello"))
	if status == nil {
		t.Fatal("first output should produce status")
	}
	if status.State != "active" {
		t.Fatalf("expected 'active', got %q", status.State)
	}
	if status.Label != "running" {
		t.Fatalf("expected 'running', got %q", status.Label)
	}
}

func TestGenericMonitorSubsequentOutput(t *testing.T) {
	g := NewGeneric(time.Second)
	g.Monitor([]byte("first"))
	status := g.Monitor([]byte("second"))
	if status != nil {
		t.Fatal("subsequent output should not produce status (no change)")
	}
}

func TestGenericCheckSilence(t *testing.T) {
	g := NewGeneric(10 * time.Millisecond)

	// No output yet
	if s := g.CheckSilence(); s != nil {
		t.Fatal("no output yet, should return nil")
	}

	// Produce output
	g.Monitor([]byte("hello"))

	// Check immediately — should not be silent
	if s := g.CheckSilence(); s != nil {
		t.Fatal("just produced output, should not be silent")
	}

	// Wait for silence timeout
	time.Sleep(20 * time.Millisecond)
	status := g.CheckSilence()
	if status == nil {
		t.Fatal("should detect silence")
	}
	if status.State != "paused" {
		t.Fatalf("expected 'paused', got %q", status.State)
	}

	// After silence detected, next output should re-trigger active
	status = g.Monitor([]byte("more"))
	if status == nil || status.State != "active" {
		t.Fatal("output after silence should produce 'active'")
	}
}

// --- Registry tests ---

type testAdapter struct {
	name    string
	matches bool
}

func (a *testAdapter) Name() string                                  { return a.name }
func (a *testAdapter) Match(cmd []string) bool                       { return a.matches }
func (a *testAdapter) Prepare(ctx PrepareContext) ([]string, []string) { return ctx.Command, nil }
func (a *testAdapter) Monitor(output []byte) *Status                 { return nil }

func TestRegistryFallback(t *testing.T) {
	r := NewRegistry()
	a := r.Resolve([]string{"unknown"})
	if a.Name() != "generic" {
		t.Fatalf("expected 'generic' fallback, got %q", a.Name())
	}
}

func TestRegistryFirstMatch(t *testing.T) {
	r := NewRegistry()
	r.Register(&testAdapter{name: "pi", matches: true})
	r.Register(&testAdapter{name: "opencode", matches: true})
	a := r.Resolve([]string{"pi"})
	if a.Name() != "pi" {
		t.Fatalf("expected 'pi' (first match), got %q", a.Name())
	}
}

func TestRegistrySkipNonMatch(t *testing.T) {
	r := NewRegistry()
	r.Register(&testAdapter{name: "pi", matches: false})
	r.Register(&testAdapter{name: "pytest", matches: true})
	a := r.Resolve([]string{"pytest"})
	if a.Name() != "pytest" {
		t.Fatalf("expected 'pytest', got %q", a.Name())
	}
}

func TestRegistryEnvOverride(t *testing.T) {
	r := NewRegistry()
	r.Register(&testAdapter{name: "pi", matches: false})
	r.Register(&testAdapter{name: "pytest", matches: true})

	os.Setenv("GMUX_ADAPTER", "pi")
	defer os.Unsetenv("GMUX_ADAPTER")

	a := r.Resolve([]string{"anything"})
	if a.Name() != "pi" {
		t.Fatalf("expected 'pi' from env override, got %q", a.Name())
	}
}

func TestRegistryEnvOverrideUnknown(t *testing.T) {
	r := NewRegistry()
	r.Register(&testAdapter{name: "pi", matches: false})

	os.Setenv("GMUX_ADAPTER", "nonexistent")
	defer os.Unsetenv("GMUX_ADAPTER")

	// Should fall through to matching, then fallback
	a := r.Resolve([]string{"anything"})
	if a.Name() != "generic" {
		t.Fatalf("expected 'generic' fallback for unknown override, got %q", a.Name())
	}
}
