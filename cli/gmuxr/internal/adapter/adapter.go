// Package adapter defines the interface for teaching gmux-run how to
// launch and monitor specific tools. Adapters are matched by command
// and produce Status events for the sidebar.
package adapter

import "path/filepath"

// Status represents an application-reported status for the sidebar.
type Status struct {
	Label string `json:"label"`           // Short text: "thinking", "3/10 passed"
	State string `json:"state"`           // active|attention|success|error|paused|info
	Icon  string `json:"icon,omitempty"`  // Optional icon hint
	Title string `json:"title,omitempty"` // If set, updates the session title
}

// Adapter teaches gmux-run how to work with a specific child process.
type Adapter interface {
	// Name returns the adapter identifier (e.g. "pi", "shell").
	Name() string

	// Match returns true if this adapter handles the given command.
	Match(command []string) bool

	// Env returns adapter-specific environment variables for the child.
	// Common GMUX_* vars are set automatically by the runner.
	// Return nil if no extra env is needed.
	Env(ctx EnvContext) []string

	// Monitor receives PTY output and optionally produces a Status.
	// Called on every PTY read with raw bytes. Must be cheap — no
	// allocations or regex compilation per call. Return nil for no change.
	Monitor(output []byte) *Status
}

// EnvContext provides launch context to Env().
type EnvContext struct {
	Cwd        string
	SessionID  string
	SocketPath string
}

// BaseName extracts the base name of a command argument, stripping path.
func BaseName(arg string) string {
	return filepath.Base(arg)
}
