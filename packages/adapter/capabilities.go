package adapter

import "time"

// SessionFileInfo holds metadata extracted from a tool's session file.
type SessionFileInfo struct {
	ID           string
	Title        string
	Cwd          string
	Created      time.Time
	MessageCount int
	FilePath     string
}

// FileEvent represents a meaningful change extracted from new file content.
type FileEvent struct {
	Title  string
	Status *Status
}

// Launchable is implemented by adapters that want to expose one or more
// launch presets in the UI.
type Launchable interface {
	// Launchers returns the launch presets this adapter contributes.
	// Adapters may return zero, one, or many presets.
	Launchers() []Launcher
}

// SessionFiler is implemented by adapters whose tools write session
// files to disk (pi, claude-code, etc). Used by gmuxd for resumable
// session discovery and session file attribution.
type SessionFiler interface {
	// SessionRootDir returns the parent directory containing all per-cwd
	// session subdirectories (e.g. ~/.pi/agent/sessions/).
	// Used by the scanner to enumerate all known working directories.
	SessionRootDir() string

	// SessionDir returns the directory where this tool stores session
	// files for the given cwd. Returns "" if not applicable.
	SessionDir(cwd string) string

	// ParseSessionFile reads a session file and returns display metadata.
	// Called by gmuxd for resumable discovery and live file monitoring.
	ParseSessionFile(path string) (*SessionFileInfo, error)
}

// FileMonitor is implemented by adapters that want to react to changes
// in their attributed session file. gmuxd calls ParseNewLines when
// inotify fires on an attributed file.
type FileMonitor interface {
	// ParseNewLines receives lines appended since the last read.
	// Returns events that should update the session's state.
	ParseNewLines(lines []string) []FileEvent
}

// SessionFileLister is an optional extension of SessionFiler for adapters
// whose session files aren't organized as direct children of per-cwd
// subdirectories. When implemented, the scanner uses ListSessionFiles
// instead of the default one-level directory listing.
type SessionFileLister interface {
	// ListSessionFiles returns all session file paths under the root.
	// Called instead of the default per-subdirectory listing.
	ListSessionFiles() []string
}

// Resumer is implemented by adapters whose sessions can be resumed
// after the process exits.
type Resumer interface {
	// ResumeCommand returns the command to resume the given session.
	ResumeCommand(info *SessionFileInfo) []string

	// CanResume returns whether a session file represents a resumable
	// session (vs a corrupted/empty/incompatible one).
	CanResume(path string) bool
}
