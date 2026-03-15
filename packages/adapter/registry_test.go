package adapter

import (
	"os"
	"testing"
)

type testAdapter struct {
	name    string
	matches bool
}

func (a *testAdapter) Name() string              { return a.name }
func (a *testAdapter) Discover() bool            { return true }
func (a *testAdapter) Match(_ []string) bool     { return a.matches }
func (a *testAdapter) Env(_ EnvContext) []string { return nil }
func (a *testAdapter) Monitor(_ []byte) *Status  { return nil }

func TestRegistryFallback(t *testing.T) {
	r := NewRegistry()
	r.SetFallback(&testAdapter{name: "shell", matches: true})
	a := r.Resolve([]string{"unknown"})
	if a.Name() != "shell" {
		t.Fatalf("expected 'shell' fallback, got %q", a.Name())
	}
}

func TestRegistryFirstMatch(t *testing.T) {
	r := NewRegistry()
	r.SetFallback(&testAdapter{name: "shell", matches: true})
	r.Register(&testAdapter{name: "pi", matches: true})
	r.Register(&testAdapter{name: "opencode", matches: true})
	a := r.Resolve([]string{"pi"})
	if a.Name() != "pi" {
		t.Fatalf("expected 'pi' (first match), got %q", a.Name())
	}
}

func TestRegistrySkipNonMatch(t *testing.T) {
	r := NewRegistry()
	r.SetFallback(&testAdapter{name: "shell", matches: true})
	r.Register(&testAdapter{name: "pi", matches: false})
	r.Register(&testAdapter{name: "pytest", matches: true})
	a := r.Resolve([]string{"pytest"})
	if a.Name() != "pytest" {
		t.Fatalf("expected 'pytest', got %q", a.Name())
	}
}

func TestRegistryEnvOverride(t *testing.T) {
	r := NewRegistry()
	r.SetFallback(&testAdapter{name: "shell", matches: true})
	r.Register(&testAdapter{name: "pi", matches: false})

	os.Setenv("GMUX_ADAPTER", "pi")
	defer os.Unsetenv("GMUX_ADAPTER")

	if a := r.Resolve([]string{"anything"}); a.Name() != "pi" {
		t.Fatalf("expected 'pi' from env override, got %q", a.Name())
	}
}

func TestRegistryEnvOverrideUnknown(t *testing.T) {
	r := NewRegistry()
	r.SetFallback(&testAdapter{name: "shell", matches: true})
	r.Register(&testAdapter{name: "pi", matches: false})

	os.Setenv("GMUX_ADAPTER", "nonexistent")
	defer os.Unsetenv("GMUX_ADAPTER")

	if a := r.Resolve([]string{"anything"}); a.Name() != "shell" {
		t.Fatalf("expected 'shell' fallback for unknown override, got %q", a.Name())
	}
}

func TestRegistryAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&testAdapter{name: "pi"})
	r.Register(&testAdapter{name: "pytest"})
	if len(r.All()) != 2 {
		t.Fatalf("expected 2, got %d", len(r.All()))
	}
}
