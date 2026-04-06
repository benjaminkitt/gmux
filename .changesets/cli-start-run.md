---
bump: minor
---

### CLI lifecycle

- **`gmuxd start` backgrounds the daemon.** Logs go to
  `~/.local/state/gmux/gmuxd.log`. Prints the PID on success and waits
  up to 3 seconds for the health check to confirm the daemon is ready.
  Replaces any running instance automatically.

- **`gmuxd run` is the foreground mode.** Use it for systemd units,
  Docker containers, or interactive debugging. Same as `start` but
  blocks until interrupted.

- **`gmuxd restart` is an alias for `start`.** Since `start` already
  replaces a running instance, a separate restart command is unnecessary
  but kept for discoverability.

- **`gmuxd status` shows session counts and peer health.** Displays
  alive sessions (local vs. remote split), dead session count, and
  per-peer connection state with session counts or error reasons.
