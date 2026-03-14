# Adapters

Adapters teach gmux how to work with specific tools. When gmuxr launches
a process, the adapter recognizes the command, optionally modifies the
launch, and monitors PTY output to report status to the sidebar.

## Where adapters live

Adapter logic is split across two components based on what each needs
to see:

| Concern | Component | Why |
|---------|-----------|-----|
| Command matching | **gmuxr** | Runs once at launch, needs the command |
| Launch preparation | **gmuxr** | Modifies command/env before exec |
| PTY output monitoring | **gmuxr** | Sees raw terminal output in real-time |
| Launcher registration | **gmuxr** | Declares how to start new sessions |
| Session file attribution | **gmuxd** | Needs global view of all sessions |
| Resumable session discovery | **gmuxd** | Scans disk, not tied to a running process |
| Session directory watching | **gmuxd** | One watcher per cwd, shared across sessions |

The split exists because gmuxr is per-session (sees one process) while
gmuxd is per-machine (sees everything). Attribution and resumability
require the global view.

## The Adapter interface (gmuxr)

```go
type Adapter interface {
    Name() string
    Match(command []string) bool
    Env(ctx EnvContext) []string
    Monitor(output []byte) *Status
}
```

### Name

Returns the adapter identifier: `"shell"`, `"pi"`, etc. Used in session
metadata (`kind` field) and for `GMUX_ADAPTER` env override matching.

### Match

Called once at launch. Receives the full command array. Returns true if
this adapter handles the command. Adapters are tried in registration
order; first match wins.

Matching should be cheap — no shelling out, no file I/O. Match on binary
base name and argument patterns:

```go
// Pi matches "pi" or "pi-coding-agent" anywhere in the command,
// stopping at "--". This handles direct invocation, npx wrappers,
// nix run, env wrappers, and full paths.
func (p *Pi) Match(cmd []string) bool {
    for _, arg := range cmd {
        base := filepath.Base(arg)
        if base == "pi" || base == "pi-coding-agent" {
            return true
        }
        if arg == "--" { break }
    }
    return false
}
```

The cost of a false negative is low — the shell adapter catches
everything, and `GMUX_ADAPTER=pi` overrides matching entirely.

### Env

Returns adapter-specific environment variables for the child process.
The runner automatically sets `GMUX`, `GMUX_SOCKET`, `GMUX_SESSION_ID`,
`GMUX_ADAPTER`, and `GMUX_VERSION` — `Env()` adds anything beyond that.

Most adapters return nil:

```go
func (p *Pi) Env(ctx EnvContext) []string { return nil }
```

**The command is never modified by adapters.** What the user (or
launcher) specified is exactly what runs. This ensures:
- Session metadata matches the actual command
- Resumability doesn't need to distinguish "original" vs "prepared"
- No surprising flag injection or wrapper rewriting
- The adapter's Match() output stays consistent with what runs

If a tool needs special flags, that belongs in the launcher's `Command`
definition or in the user's shell alias — not in adapter mutation.

### Monitor

Called on **every PTY read** with raw bytes. Must be very cheap — no
allocations, no regex compilation per call. Returns nil when there's
nothing to report.

When it returns a `Status`, the runner propagates it to the session
state, which flows through SSE to the sidebar.

## Status

```go
type Status struct {
    Label string  // Short text: "working", "3/10 passed"
    State string  // Visual treatment: active|attention|success|error|paused|info
    Icon  string  // Optional icon hint (emoji)
    Title string  // If set, updates the session's display title
}
```

### Status states

| State | Meaning | Sidebar indicator |
|-------|---------|-------------------|
| `active` | Working, processing | Green pulsing dot |
| `attention` | Needs user input | Orange/amber dot |
| `success` | Completed successfully | Green check |
| `error` | Something went wrong | Red dot |
| `paused` | Idle but resumable | Grey dot |
| `info` | Informational | Blue dot |

A session with no reported status shows a dim green dot (alive, quiet).

### Title updates

If `Status.Title` is set, it replaces the session's display title in the
sidebar. The shell adapter uses this to propagate OSC 0/2 terminal title
sequences (e.g., fish/zsh set the title on directory change).

## Adapter resolution

When gmuxr launches a command:

1. **`GMUX_ADAPTER` env override** — if set, use that adapter directly.
   Escape hatch for wrappers and aliases where binary name matching fails.
2. **Walk registered adapters** — call `Match()` in registration order.
   First match wins.
3. **Shell fallback** — always matches, always last.

## Built-in adapters

### Shell (fallback)

- **Matches**: everything (catch-all)
- **Monitor**: parses OSC 0/2 title sequences for live sidebar titles
- **Status**: none — shells don't report activity states
- **Launcher**: always added by gmuxd as the default "new session" option.
  Not in the `Launchers` slice (it's not an opt-in adapter)

### Pi (coding agent)

- **Matches**: `pi` or `pi-coding-agent` as base name in any arg position
- **Monitor**: detects braille spinner + "Working..." → `active` status
- **Status**: `{label: "working", state: "active"}` during agent turns
- **Launcher**: `{id: "pi", label: "pi", command: ["pi"]}`
- **Session files**: `~/.pi/agent/sessions/--<cwd-encoded>--/*.jsonl`
- **Resume**: via `pi --session <path> -c` (handled by gmuxd, see below)

## Adding an adapter

One file per adapter in `cli/gmuxr/internal/adapter/adapters/`. The file
registers itself via `init()`:

```go
package adapters

func init() {
    // Register launcher (appears in UI "new session" menu)
    Launchers = append(Launchers, Launcher{
        ID:      "myapp",
        Label:   "MyApp",
        Command: []string{"myapp"},
    })
    // Register adapter instance (used for matching)
    All = append(All, NewMyApp())
}

type MyApp struct{}

func NewMyApp() *MyApp { return &MyApp{} }
func (m *MyApp) Name() string { return "myapp" }
func (m *MyApp) Match(cmd []string) bool { /* ... */ }
func (m *MyApp) Env(ctx adapter.EnvContext) []string { return nil }
func (m *MyApp) Monitor(output []byte) *adapter.Status { /* ... */ }
```

No other files need to be touched. The registry iterates `All` at
startup; `Launchers` is queried via `gmuxr adapters` subcommand.

## gmuxd responsibilities

### Launcher discovery

At startup, gmuxd runs `gmuxr adapters` which outputs the `Launchers`
slice as JSON. gmuxd prepends shell as the default and serves the full
list via `GET /v1/config`. The UI's "new session" menu is built from
this.

### Session file attribution (ADR-0009)

gmuxd watches session directories with inotify (one watcher per unique
cwd that has live sessions). When a file is written:

1. **Single session in cwd** → trivially attribute the file to that session
2. **Multiple sessions** → content similarity match: extract text from the
   file, compare against each session's scrollback tail, best match wins

Attribution sets the session's `resume_key` — the unique identifier from
the application's session file (e.g., pi's session UUID). This key
is used for deduplication with resumable entries.

Attribution is **sticky** — once set, it holds until a different file
starts receiving writes (e.g., after `/resume` or `/fork` in pi).
Re-attribution happens naturally on the next write, no command detection
needed.

See [ADR-0009](adr/0009-session-file-attribution.md) for the full design.

### Resumable session discovery

gmuxd periodically scans for sessions that can be resumed. For pi,
this means listing `.jsonl` files in session directories and reading
their headers for UUID, cwd, and timestamp.

Resumable sessions are pushed through the same SSE stream as live
sessions, with `alive: false` and `resumable: true`. The frontend
deduplicates: if a live session's `resume_key` matches a resumable
entry's key (same adapter), the resumable entry is hidden.

### Reverse channel (gmuxd → gmuxr)

After attribution, gmuxd can notify the runner which file is "theirs"
(via a POST to the runner's socket). This enables the runner to do
richer per-file monitoring — e.g., watching specific JSONL entries for
conversation state. This is optional; attribution works entirely in
gmuxd without it.

## Session states from the UI's perspective

| State | Indicator | Terminal | Close button | Source |
|-------|-----------|----------|--------------|--------|
| Alive, no status | Dim green dot | Interactive | × or − | Process running |
| Alive, active | Green pulsing | Interactive | × or − | Monitor() |
| Alive, attention | Orange dot | Interactive | × or − | Monitor() or child PUT /status |
| Alive, error | Red dot | Interactive | × or − | Monitor() or child PUT /status |
| Resumable | Hollow dot | None | × (forget) | gmuxd scan |
| Gone | Not shown | None | — | Dismissed or exited (no resume) |

The close button shape depends on whether the adapter supports resume:
- **×** — session will be forgotten (shell, or dismissing a resumable entry)
- **−** — session will be closed but entry remains as resumable (pi)

Since no adapter implements resume yet, all sessions show × today.
When pi gets resume support, its sessions automatically show −.

See [ADR-0007](adr/0007-session-lifecycle-and-close-semantics.md).

## Child awareness protocol

Any process running inside gmuxr can detect and communicate with gmux
through environment variables set automatically by the runner:

| Variable | Value | Purpose |
|----------|-------|---------|
| `GMUX` | `1` | Detection flag |
| `GMUX_SOCKET` | `/tmp/gmux-sessions/sess-abc.sock` | Communication channel |
| `GMUX_SESSION_ID` | `sess-abc` | Session identity |
| `GMUX_ADAPTER` | `pi` | Which adapter matched |
| `GMUX_VERSION` | `0.1.0` | Protocol version |

Children can optionally call back to the runner:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/status` | PUT | Set application status (highest priority) |
| `/meta` | PATCH | Update title, subtitle |

Priority: child self-report > adapter Monitor() > process defaults.

## Testing

Integration tests live alongside the adapter code with a `//go:build
integration` tag:

```bash
go test -tags integration -v -timeout 120s \
  ./cli/gmuxr/internal/adapter/adapters/
```

These launch real processes through PTYs and verify adapter behavior.
They require the target binaries to be installed (e.g., `pi`) and are
skipped if not found. Tests document observable behavior patterns (output
timing, file creation lifecycle) that the attribution logic depends on.
