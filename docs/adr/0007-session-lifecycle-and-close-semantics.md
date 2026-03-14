# ADR-0007: Session lifecycle and close semantics

- Status: Proposed
- Date: 2026-03-15

## Context

Sessions in gmux can be alive (running), dead (child exited), or — in the
future — resumable (not running but recoverable from disk). The sidebar
needs to show what's relevant without accumulating clutter, and the user
needs a single, obvious gesture to close/dismiss sessions.

Key tensions:
- **Shell sessions** are disposable. When closed, they're gone forever.
- **Agent sessions** (pi) leave session files on disk. When closed, they
  become resumable — the entry should stay in the sidebar.
- **Task-runner sessions** (future) may benefit from keeping their terminal
  output visible after exit. This is deferred.
- **gmuxr lingering** (keeping the process alive to serve scrollback after
  child exit) adds complexity and accumulates zombie processes.

## Decision

### Lifecycle states

From the user's perspective, a session is in one of three states:

| State | Meaning | Terminal | In sidebar |
|-------|---------|----------|------------|
| **Alive** | Child process running | Interactive | Yes |
| **Resumable** | Not running, but can be resumed | None | Yes |
| **Gone** | Fully dismissed | None | No |

"Dead" is a transient internal state: when a child exits, the session
either becomes **resumable** (if the adapter supports resume) or **gone**
(if it doesn't). There is no visible "dead with frozen terminal" state
in the MVP.

### gmuxr exits immediately on child death

gmuxr does not linger after the child process exits. The socket disappears,
and gmuxd detects this via its next scan or the SSE connection dropping.

This eliminates process accumulation entirely. Scrollback is lost when the
session exits — this is acceptable because:
- Shell sessions are disposable
- Agent sessions have their own persistence (session files, conversation
  history) that matters more than raw terminal output
- The `linger` policy (below) is available for future adapters that need
  post-exit scrollback

### Close button

Every session item in the sidebar shows a close button on hover.

**Button shape is derived from resume support:**
- **`×`** — session will be forgotten (gone). Used when the adapter does
  not support resume.
- **`−`** — session will be closed but the entry remains as resumable.
  Used when the adapter supports resume.

Since no adapter implements resume yet, all sessions show `×` today.
When the pi adapter gains resume support, its sessions automatically
show `−` instead.

**Button behavior:**
- On an **alive** session: sends kill signal to the child process.
  - If adapter has no resume → gmuxr exits → session gone from sidebar.
  - If adapter has resume → gmuxr exits → session becomes resumable.
- On a **resumable** session: forgets it (removes from sidebar, does NOT
  delete session files on disk).

### Selection after close

When the currently selected session is closed, the selection clears and
the main area shows the empty state. No auto-select to another session —
that would be surprising.

### Linger policy (deferred)

Some future adapters (task runners, build tools) may want to keep terminal
output visible after the child exits. For these, gmuxr would stay alive
to serve the scrollback via WebSocket.

This is modeled as an adapter-level trait, not a global setting:

```go
type Lingerer interface {
    // LingerAfterExit returns true if gmuxr should stay alive after
    // the child exits to continue serving scrollback.
    LingerAfterExit() bool
}
```

If an adapter implements `Lingerer` and returns true:
- gmuxr stays alive after child exit, serving read-only scrollback
- The session shows as dead in the sidebar with frozen terminal
- The close button (`×`) shuts down gmuxr explicitly via `POST /shutdown`
- No timeout — user explicitly dismisses when done

This is NOT built until an adapter needs it.

### Bulk cleanup

The sidebar header includes a "Clear dead" action (icon or menu) that
dismisses all non-alive, non-resumable sessions at once. For the linger
case (future), this would also shut down lingering gmuxr processes.

### SessionDetail page: removed

Dead/resumable sessions do not show a metadata detail page. When a
resumable session is selected, the main area shows a minimal resume
prompt (session info + resume button). When a gone session disappears,
selection clears to empty state.

## Consequences

### Positive
- **No zombie processes** — gmuxr always exits when child exits (MVP)
- **No timeout tuning** — no linger duration to configure
- **Single gesture** — one button to close, shape communicates intent
- **Forward compatible** — linger and resume are clean extensions
- **No clutter** — shell sessions vanish on close, only resumable
  sessions persist in sidebar

### Negative
- **Scrollback lost on exit** — can't review terminal output after
  session dies (until linger is implemented for specific adapters)
- **Two-click for resume-capable sessions** — close (becomes resumable)
  then forget (gone) — but this is intentional, not accidental

### Neutral
- Resume support determines button shape — no explicit user config
- "Clear dead" is a convenience, not a necessity (sessions clean up
  on their own since gmuxr exits)
