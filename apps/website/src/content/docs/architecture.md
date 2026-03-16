---
title: Architecture
description: "Runtime structure: runner, daemon, and embedded web UI."
---

## Runtime pieces

### `gmuxr` — session runner

One per session. It:

- Launches the child process under a PTY
- Owns the live session state (title, status, working flag)
- Exposes the session on a Unix socket (metadata, events, terminal attach)
- Runs adapter logic over child output

`gmuxr` is the source of truth for a live session.

### `gmuxd` — machine daemon

One per machine. It:

- Discovers live runner sockets (`/tmp/gmux-sessions/*.sock`)
- Subscribes to runner events for live updates
- Watches adapter session files (e.g. pi's JSONL conversations)
- Serves the REST API, SSE event stream, and WebSocket proxy
- Serves the embedded web frontend as a SPA
- Manages session launch, kill, dismiss, and resume

`gmuxd` is stateless — if it restarts, it rediscovers running sessions. On startup it hashes the `gmuxr` binary it ships with; sessions running a different build are marked **stale** so the UI can flag them.

`gmuxr` auto-starts `gmuxd` if it isn't already running.

Configuration lives in `~/.config/gmux/config.toml`. See [Security](/security) and [Remote Access](/remote-access) for details.

### Web UI

The frontend is built with Preact and xterm.js, compiled into a static bundle, and embedded into the `gmuxd` binary via `go:embed`. No separate web server or Node.js runtime is needed.

## Data flow

```
gmuxr ──Unix socket──→ gmuxd ──HTTP/SSE/WS──→ browser
```

1. `gmuxr` launches a session and exposes it on a Unix socket
2. `gmuxd` discovers the socket and reads session metadata
3. `gmuxd` subscribes to the runner's SSE event stream for live updates
4. The browser fetches sessions from `GET /v1/sessions` and subscribes to `GET /v1/events`
5. When you click a session, the browser opens a WebSocket to `/ws/{id}` — gmuxd proxies this to the runner's socket
6. Terminal I/O flows directly between browser and runner through the proxy

## API surface

All served by `gmuxd` on a single port (default `:8790`):

| Endpoint | Purpose |
|---|---|
| `GET /v1/sessions` | List all sessions |
| `GET /v1/config` | Launcher configuration |
| `POST /v1/launch` | Launch a new session |
| `POST /v1/sessions/{id}/kill` | Kill a session |
| `POST /v1/sessions/{id}/dismiss` | Kill + remove |
| `POST /v1/sessions/{id}/resume` | Resume a resumable session |
| `GET /v1/events` | SSE stream of session changes |
| `WS /ws/{id}` | Terminal WebSocket proxy |
| `GET /` | Embedded web UI (SPA) |

## Repo layout

| Path | Language | Purpose |
|---|---|---|
| `cli/gmuxr` | Go | Session launcher — PTY, WebSocket, adapters |
| `services/gmuxd` | Go | Daemon — discovery, proxy, embedded web UI |
| `apps/gmux-web` | TypeScript/Preact | Browser UI — sidebar, terminal, header |
| `packages/protocol` | TypeScript | Shared schemas (zod-validated) |
| `packages/adapter` | Go | Adapter interfaces and built-in adapters |
