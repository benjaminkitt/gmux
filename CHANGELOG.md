# Changelog

## v0.2.1

### State management rearchitecture

Rewrote how session state flows between the daemon and the web UI to eliminate a class of race conditions in the resume lifecycle.

**Backend:** Register is now the single entry point for creating live sessions. Discovery delegates to it instead of having its own creation path. The resume handler no longer sets `alive=true` optimistically — the session stays resumable until the runner actually registers.

**Frontend:** Zero optimistic updates. The session list is a pure projection of backend state, written only by SSE events. UI-only state (`selectedId`, `resumingId`) is kept separate from session state.

### Bugfixes

- **Resume races eliminated** — fixed duplicate sessions from discovery/register race, stale socket causing session to die again, and premature terminal attach to non-existent socket.
- **Dismissed sessions stay dismissed** — dismissed resume keys are tracked so the periodic file scanner doesn't re-add them.
- **Exit status no longer clobbers resume transition** — clean exits show no status label (the dead state is visible from the dot).
- **Stale selection cleared** — selecting a session that dies or gets dismissed now correctly deselects. Auto-select only picks sessions with a valid socket.
- **Discovery won't delete brand-new sockets** — sockets must be >10s old before cleanup, preventing deletion during runner startup.
- **Cross-platform Chrome app-mode launch** — macOS no longer uses `open -a` (which silently drops `--args` when Chrome is running). Calls the binary inside the `.app` bundle directly.

### UI

- **Status labels are null by default** — only shown when informative (e.g. `exited (1)`). Removed redundant labels like "working", "starting", "completed".
- **Kind label removed from sidebar** — "claude", "codex" etc. no longer shown as text next to sessions.
- **Dead sessions not auto-selected on page load.**

### Docs

- **State Management** page — comprehensive documentation of session lifecycle, owner table, derived fields, and frontend architecture.
- **Session Schema** added to sidebar navigation.
- Updated session-schema, adapter-architecture, writing-adapters, using-the-ui, and remote-access docs.

### Internal

- `resumableKinds` derived from adapter set instead of hardcoded negative list.
- `PendingResumes.Has()` removed (unused after Scan delegates to Register).
- Auto-select consolidated to single effect.

## v0.2.0

### New adapters

- **Claude Code** — full integration with session file parsing, live status from JSONL, title extraction, and resume via `claude --resume <id>`.
- **Codex** — full integration with date-nested session storage (`~/.codex/sessions/YYYY/MM/DD/`), live status, title extraction, and resume via `codex resume <id>`.

### Resumable sessions

Sessions from Claude Code, Codex, and pi now transition seamlessly between alive and resumable states:

- **No "exited" limbo** — when a session exits and has an attributed file, it becomes resumable immediately. No intermediate dead state.
- **Minimize button (−)** appears on sessions that have a session file. Killing these keeps them in the sidebar as resumable.
- **Dismiss button (×)** appears on sessions without a file, or on dead resumable sessions you want to remove.
- Sessions opened and closed without interaction are correctly treated as non-resumable.

### UI improvements

- **Empty state redesign** — launcher buttons as the hero element.
- **No auto-select of dead sessions** — page reload with only dead sessions shows the empty state instead of highlighting a random session.
- **Selection tracks the terminal** — when a session dies, the selection clears instead of leaving a highlighted row with no terminal.
- **"Starting" interstitial** — shown briefly while a resumed session's runner is registering.
- **Kind labels removed** — "claude", "codex" labels no longer clutter the sidebar and header.
- **Status labels cleaned up** — `null` by default. The pulsing dot is enough for "working"; labels reserved for genuinely informative states like `exited (1)`.

### Browser launch

- **Fixed Chrome app-mode on macOS** — `open -a Chrome --args` silently drops flags when Chrome is already running. Now calls the binary inside the `.app` bundle directly.
- **Default browser detection** — checks if the default browser is Chromium-based before falling back to any installed Chromium, then system default.
- Supports Chrome, Chromium, Arc, Brave, and Edge on macOS.

### Bugfixes

- **Resume race conditions** — cleared stale socket path on resume; discovery now skips sockets with pending resumes; Register cleans up duplicates from the race window.
- **Dismissed resumables reappearing** — scanner no longer re-adds sessions the user dismissed.
- **Double-click resume prevention** — serialized with mutex; second click rejected while first is in flight.
- **Exit status clobbering** — `OnExit` hook return value now correctly prevents "exited (N)" from overwriting a clean resumable transition.
- **Stale socket crash path** — discovery resolves the resume command before cleaning up file monitor state.

### Internal

- `Resumable` and `CloseAction` are derived fields, computed in `Upsert()` from `!alive + resumeKind + hasFile + hasCommand`. No manual state management.
- `resumableKinds` built from the compiled adapter set at startup (which adapters implement `Resumer`), not a hardcoded list.
- `SessionFileLister` interface for adapters with non-standard directory layouts (Codex).
- `PendingResumes.Has()` for non-destructive checks by the discovery loop.
- Dismissed resume keys tracked in-memory to prevent scanner re-adding.

### Docs

- Integration pages for Claude Code and Codex.
- Session schema updated with Resume fields (`resumable`, `resume_key`, `close_action`).
- Adapter architecture docs cover live→resumable transition flow.
- Status design principle documented: null by default, labels only when informative.
- Remote access docs explain `hostname` for multi-machine tailnets.

## v0.1.0

Initial release.
