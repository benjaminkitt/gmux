---
title: pi
description: How gmux works with the pi coding agent.
---

gmux has built-in support for [pi](https://github.com/mariozechner/pi-coding-agent). No configuration is needed — launch pi through gmux and everything works automatically.

## What you get

### Live status

The sidebar shows when pi is actively working. gmux detects pi's spinner animation and reports it as **working** (pulsing cyan dot). When the spinner stops, the dot disappears.

### Session titles from conversations

Instead of showing "pi" for every session, gmux reads pi's session files and extracts the first message you sent as the title:

```
▼ ~/dev/myapp
  ● Fix the auth bug in login.go
  ● Add pagination to the API
  ○ Refactor database layer
```

If you rename a session with pi's `/name` command, gmux picks up the new name automatically.

### Resumable sessions

When a pi session exits, it remains in the sidebar as a resumable entry. Click it to resume exactly where you left off — gmux launches `pi --session <path> -c` with the right session file.

Resumable sessions are deduplicated: if you're already running a session that matches a resumable entry, only the live one appears.

### Launch from the UI

Pi appears in the launch menu only when it is available on the current machine. `gmuxd` checks this at startup by running `pi --version`; if that fails, the pi launcher is omitted from the UI.

## How it works

### Detection

There are two separate pi checks:

- **availability discovery** in `gmuxd`: run `pi --version` at startup to see whether pi is installed and the pi launcher should be shown
- **runtime matching** in `gmux`: scan the launched command for a `pi` or `pi-coding-agent` binary name

The runtime matching works with direct invocation, full paths, `npx`, `nix run`, and other wrappers:

```bash
gmux pi                              # ✓ matched
gmux /home/user/.local/bin/pi        # ✓ matched
gmux npx pi                          # ✓ matched
gmux echo "not pi"                # ✗ not matched
```

If detection fails (e.g., an unusual wrapper), override it:

```bash
GMUX_ADAPTER=pi gmux my-pi-wrapper
```

### Session files

Pi stores conversations as JSONL files in `~/.pi/agent/sessions/`. Each working directory gets its own subfolder with an encoded name:

```
~/.pi/agent/sessions/
  --home-mg-dev-myapp--/
    2026-03-15T10-00-00-000Z_abc123.jsonl
    2026-03-15T11-30-00-000Z_def456.jsonl
```

gmuxd watches these directories and reads the files to populate the sidebar. The first line of each file is a session header with a UUID and timestamp. Message entries contain the conversation text used for titles.

### Session file attribution

When pi creates or updates a session file, gmuxd needs to figure out which running session it belongs to. For the common case (one pi session per directory), this is trivial. When multiple pi sessions share a directory, gmuxd uses content similarity matching — it compares text extracted from the file against each session's terminal scrollback to find the best match.

Attribution is sticky: once a file is matched to a session, it stays matched until a different file starts receiving writes (e.g., after using `/resume` or `/fork` in pi).

### Status detection

gmux watches pi's PTY output for its braille spinner characters (⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏) followed by "Working...". When detected, the session's status updates to `active` with the label "working". This is a lightweight heuristic — no parsing of pi's internal state, just pattern matching on the terminal output.

## Limitations

- **Status is spinner-only for now.** gmux doesn't yet distinguish between "thinking", "writing code", or "waiting for tool approval". It reports "working" for any active spinner.
- **Title appears after the first turn.** Pi creates the session file on launch but doesn't write conversation content until the first assistant response completes. The title (derived from your first message) appears once that write happens.
- **Multi-instance attribution needs content matching.** If you run two pi sessions in the same directory, gmux uses content similarity to attribute files. This works well in practice but has a one-write delay for initial attribution.
