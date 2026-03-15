# ADR-0010: Adapter interface v2 — shared package, opt-in capabilities

- Status: Proposed
- Date: 2026-03-15

## Context

The adapter interface (ADR-0005) currently lives in
`cli/gmuxr/internal/adapter/` and only covers gmuxr's needs: Match,
Env, Monitor. But gmuxd also has adapter-specific behavior:

- **Session file discovery**: pi stores files in `~/.pi/agent/sessions/`
  with a specific directory encoding. Other tools will have different
  conventions.
- **Session file parsing**: extracting title, UUID, message count from
  pi's JSONL format. Other tools will have different formats.
- **Live file monitoring**: when an attributed file gets new writes,
  extracting meaningful events (name changes, status transitions).
- **Resume command**: `pi --session <path> -c` vs whatever another tool
  needs.
- **Resumable discovery**: listing files that represent resumable
  sessions.
- **Launch menu metadata**: deciding which adapters should appear in the
  UI launch menu, and with which default commands.

This is all adapter-specific knowledge that shouldn't be hardcoded in
gmuxd. But gmuxd can't import gmuxr's internal packages.

## Decision

### Move adapter package to a shared location

```
packages/adapter/          # shared Go module
  adapter.go               # core interfaces + types
  capabilities.go          # optional capability interfaces
  adapters/                # built-in implementations + registry helpers
    shell.go
    pi.go
```

Both `cli/gmuxr` and `services/gmuxd` import `packages/adapter`.
Add `./packages/adapter` to `go.work`.

### Core interface (required, all adapters)

```go
type Adapter interface {
    // Identity + machine availability
    Name() string
    Discover() bool
    Match(command []string) bool

    // gmuxr hooks
    Env(ctx EnvContext) []string
    Monitor(output []byte) *Status
}
```

This is the base. Every adapter must implement the base interface.
`Discover()` is required so gmuxd can probe which adapter-backed tools
are actually available on the current machine.

### Opt-in capability interfaces

Additional interfaces that adapters implement when they support richer
behavior. Components check with type assertions:

```go
if sf, ok := adapter.(SessionFiler); ok {
    dir := sf.SessionDir(cwd)
    // ...
}
```

#### Launchable — UI launch menu support

```go
// Launchable is implemented by adapters that want to expose one or more
// launch presets in the UI.
type Launchable interface {
    Launchers() []Launcher
}

type Launcher struct {
    ID          string
    Label       string
    Command     []string
    Description string
}
```

Called by gmuxd when building the launch menu it serves to the UI.
Launchers are derived from the compiled adapter set by checking which
adapters implement `Launchable`, then filtered by each adapter's
`Discover()` result.

This keeps launch-menu support optional while keeping launch availability
explicit:
- adapters can expose zero launchers by not implementing the interface
- one adapter can expose multiple launch presets
- every adapter must still implement `Discover()` to report availability
- shell can participate without needing a separate special registry

#### SessionFiler — session file discovery and parsing

```go
// SessionFiler is implemented by adapters whose tools write session
// files to disk (pi, claude-code, etc).
type SessionFiler interface {
    // SessionRootDir returns the parent directory containing all per-cwd
    // session subdirectories.
    SessionRootDir() string

    // SessionDir returns the directory where this tool stores session
    // files for the given cwd. Returns "" if unknown or not applicable.
    SessionDir(cwd string) string

    // ParseSessionFile reads a session file and returns display metadata.
    // Called by gmuxd for resumable discovery and live file monitoring.
    ParseSessionFile(path string) (*SessionFileInfo, error)
}

type SessionFileInfo struct {
    ID           string    // Tool's session identifier (becomes resume_key)
    Title        string    // Display title for sidebar
    Cwd          string    // Working directory
    Created      time.Time // When the session was created
    MessageCount int       // Number of messages/interactions
    FilePath     string    // Absolute path to the file
}
```

Called by gmuxd during:
- **Resumable discovery**: scan `SessionRootDir()` / `SessionDir()` for files, parse each
- **Attribution**: after matching a file to a session, parse for resume_key

#### FileMonitor — live file event extraction

```go
// FileMonitor is implemented by adapters that want to react to changes
// in their attributed session file. gmuxd calls ParseNewLines when
// inotify fires on an attributed file.
type FileMonitor interface {
    // ParseNewLines receives lines appended since the last read.
    // Returns events that should update the session's state.
    // Called by gmuxd on IN_CLOSE_WRITE for the attributed file.
    ParseNewLines(lines []string) []FileEvent
}

type FileEvent struct {
    Title  string  // If non-empty, update session title
    Status *Status // If non-nil, update session status
    // Future: subtitle, metadata, etc.
}
```

Called by gmuxd when:
- an attributed file gets new writes
- meaningful changes need to be extracted from appended content

For pi: parse new JSONL lines, look for `session_info` entries with
name changes, and possibly message-role transitions for richer status.

#### Resumer — session resume support

```go
// Resumer is implemented by adapters whose sessions can be resumed
// after the process exits.
type Resumer interface {
    // ResumeCommand returns the command to resume the given session.
    ResumeCommand(info *SessionFileInfo) []string

    // CanResume returns whether a session file represents a resumable
    // session (vs a corrupted/empty/incompatible one).
    CanResume(path string) bool
}
```

Called by gmuxd when:
- user clicks a resumable session in the UI
- gmuxd needs to filter which files are actually resumable

For pi: `ResumeCommand` returns `[]string{"pi", "--session", info.FilePath, "-c"}`.
`CanResume` checks the file has a valid header and at least one message.

### What each component uses

```
gmuxr:
  Adapter.Name()          — session metadata (kind field)
  Adapter.Match()         — adapter resolution at launch
  Adapter.Env()           — child environment setup
  Adapter.Monitor()       — PTY output parsing

gmuxd:
  Adapter.Name()          — identify adapters for store/config
  Launchable              — launch menu discovery from compiled adapters
  SessionFiler            — resumable discovery, attribution parsing
  FileMonitor             — live file event extraction
  Resumer                 — resume command generation, filtering
```

### Capability composition

```
Shell:     Adapter + Launchable
Pi:        Adapter + Launchable + SessionFiler + FileMonitor + Resumer
Opencode:  Adapter + Launchable + SessionFiler + Resumer  (hypothetical)
Pytest:    Adapter                                         (hypothetical, no files)
```

The shell adapter is still the simplest runtime adapter, but it also
contributes the default shell launcher. Pi is the richest built-in.
New adapters pick only the capabilities they need.

### Interface detection pattern

```go
// In gmuxd config code:
launchers := adapters.AllLaunchers()

// In gmuxd discovery code:
for _, a := range adapters.All {
    sf, ok := a.(adapter.SessionFiler)
    if !ok {
        continue // this adapter doesn't have session files
    }
    dir := sf.SessionDir(cwd)
    if dir == "" {
        continue
    }
    // scan dir for resumable sessions...
    files := listFiles(dir)
    for _, f := range files {
        info, err := sf.ParseSessionFile(f)
        // ...
    }
}
```

No special launcher registry, no separate launcher subprocess protocol,
and no hardcoded per-tool maps. Capabilities are discovered from the
adapter instances themselves.

## Package layout

```
packages/
  adapter/
    go.mod                    # github.com/gmuxapp/gmux/packages/adapter
    adapter.go                # Adapter interface, Status, EnvContext, Launcher
    capabilities.go           # Launchable, SessionFiler, FileMonitor, Resumer
    adapters/
      shell.go                # Shell: Adapter + Launchable + fallback
      pi.go                   # Pi: Adapter + Launchable + SessionFiler + FileMonitor + Resumer
```

The current implementation keeps pi in a single file rather than a
separate `pi_files.go`. That is an organizational choice, not a model
constraint.

## Consequences

### Positive
- Clean separation: each interface is a single concern
- Opt-in: adapters only implement what they support
- Shared: both gmuxr and gmuxd import the same package
- Discoverable: type assertion pattern is idiomatic Go
- Extensible: new capabilities = new interfaces, no breaking changes
- Unified launch discovery: no parallel launcher registry to maintain

### Negative
- Package move: adapter code moves from gmuxr internal to shared
- New Go module in workspace: minor build complexity
- Pi adapter grows: several interfaces beyond base, though each remains small

### Migration
1. Create `packages/adapter/` with new module
2. Move existing types + interfaces
3. Add capability interfaces, including `Launchable`
4. Move pi/shell adapter implementations
5. Update gmuxr and gmuxd imports
6. Derive launchers from compiled adapters instead of a separate registry
