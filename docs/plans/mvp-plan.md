# gmux MVP plan

## What we have (working end-to-end)

| Component | Status | Notes |
|-----------|--------|-------|
| **gmuxr** launches process, holds PTY | ✅ | Adapter system, env vars, silence detection |
| **gmuxr** serves WebSocket + HTTP on Unix socket | ✅ | `/meta`, `/events`, `/status`, `PATCH /meta`, WS terminal |
| **gmuxr** scrollback replay with DEC 2026 sync | ✅ | 128KB ring buffer, atomic BSU+reset+data+ESU |
| **gmuxr** registers with gmuxd | ✅ | Best-effort retry, deregister on exit |
| **gmuxd** socket discovery + registration | ✅ | Scan `*.sock`, `GET /meta`, periodic fallback |
| **gmuxd** session store with pub/sub | ✅ | Upsert/Remove/Subscribe, SSE fan-out |
| **gmuxd** WebSocket proxy | ✅ | `/ws/{sessionID}` → runner socket |
| **gmux-api** tRPC + SSE passthrough | ✅ | sessions.list, sessions.attach, `/api/events` |
| **gmux-web** sidebar with folder groups | ✅ | `groupByFolder()` by cwd, status priority sort |
| **gmux-web** session items with status dots | ✅ | 7 status states, pulse animations |
| **gmux-web** terminal with reconnecting WS | ✅ | Replay buffer, no-flicker switching, auto-focus |
| **gmux-web** SSE subscription | ✅ | session-update upsert, session-remove |
| **gmux-web** MainHeader (basic) | ✅ | Title, subtitle, status dot |
| **gmux-web** SessionDetail (dead sessions) | ✅ | Hero status, metadata grid |
| **gmux-web** mobile responsive CSS | ✅ | Slide-out sidebar, overlay, 640px breakpoint |
| **gmux-web** mock data mode | ✅ | `?mock` or `VITE_MOCK=1`, 9 sessions |
| **Dev tooling** | ✅ | `./dev` script, watchexec, CONTRIBUTING.md |
| **Protocol schemas** | ✅ | Zod-validated, session + events |
| **E2E integration test** | ✅ | Build → start → verify registration → PUT /status |

## What's broken or untested

| Issue | Risk | Notes |
|-------|------|-------|
| **Multi-session switching** | HIGH | Only tested with 1 session. Core use case. |
| **Session exit lifecycle** | HIGH | What happens in the UI when a session dies? Does gmuxd update? Does the dot go grey? |
| **SSE event flow under real conditions** | MEDIUM | SSE wiring exists but not verified with live session state changes |
| **Kill button is decorative** | MEDIUM | Button rendered but has no onClick handler |
| **gmuxd doesn't subscribe to runner /events** | HIGH | Discovery does `GET /meta` once. Status changes (active → attention) won't propagate unless re-scanned. This is a major gap. |
| **Folder names from cwd are full paths or wrong** | LOW | `groupByFolder` uses last path segment — should work, needs verification |
| **No loading/error states** | LOW | Blank screen while fetching, silent failures |

## MVP: what's needed to use this daily

### Tier 1: Must work (blocks daily use)

1. **gmuxd subscribes to runner `/events` SSE** — Without this, the sidebar is a snapshot, not a live view. When an adapter reports `attention` or a session exits, gmuxd needs to update its cache in real-time, which flows via SSE to the browser.

2. **Multi-session switching** — Launch 2+ gmuxr sessions, verify sidebar lists all, clicking switches terminal, scrollback replays correctly per session.

3. **Session exit lifecycle** — When a session's child process exits: gmuxr updates state, gmuxd sees it (via events subscription), sidebar shows dead status, terminal shows last output (frozen), SessionDetail shows exit code.

4. **Kill button works** — Wire onClick → `POST /v1/sessions/{id}/kill` → gmuxd → signal to runner. Session should transition to dead state via normal exit lifecycle.

5. **Folder grouping verified with real data** — Multiple sessions in same cwd group correctly. Sessions in different cwds create separate folders. Folder names are directory basenames, not full paths.

### Tier 2: Should work (daily use is painful without)

6. **Header bar enrichment** — Show cwd (shortened), adapter type, and kill button in the header bar (not just in SessionDetail). This is the primary action surface.

7. **Auto-select on session change** — When selected session dies, auto-select next alive session. When new session appears with attention status, maybe highlight it but don't steal selection.

8. **Basic error/loading states** — Show "connecting..." while fetching sessions. Show error if gmuxd is unreachable. Don't show blank white screen.

9. **URL param filtering** — `?project=gmux` or `?cwd=/home/mg/dev/gmux` filters the sidebar. This is how desktop users scope their browser tabs.

### Tier 3: Nice to have for MVP (do if time allows)

10. **Git probe** — Branch name + dirty indicator in folder heading. High-value, low-effort if the probe interface exists.

11. **Mobile polish** — Test on actual phone. Triage view should be genuinely usable for "check and steer" workflow.

12. **Sound/browser notification** — When a session transitions to `attention` state, browser Notification API ping. Critical for the "keep an eye on things" use case.

## Recommended order

```
1. gmuxd event subscription  ← unlocks live updates
   │
   ├─ 2. Multi-session verify  ← proves the core loop
   │
   ├─ 3. Session exit lifecycle ← proves the full lifecycle
   │
   └─ 4. Kill button wiring    ← first user action
       │
       5. Folder grouping verify ← visual correctness
       │
       6. Header bar enrichment  ← primary action surface
       │
       7. Auto-select logic      ← smooth UX
       │
       8. Error/loading states   ← robustness
       │
       9. URL param filtering    ← multi-tab workflow
       │
      10. Git probe              ← first probe, proves the interface
       │
      11. Mobile polish
       │
      12. Notifications
```

Items 1-5 are a tight dependency chain (each proves the next works). Items 6-9 are independent polish. Items 10-12 are enrichment.

**Estimated effort:**
- Tier 1 (items 1-5): ~1 session of focused work
- Tier 2 (items 6-9): ~1 session
- Tier 3 (items 10-12): ~1 session
