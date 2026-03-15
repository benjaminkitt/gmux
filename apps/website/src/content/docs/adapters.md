---
title: Adapters
description: How gmux understands what your process is doing.
---

Adapters teach gmux how to interpret specific tools. When you launch a session, gmux automatically detects what you're running and activates the right adapter. On the `gmuxd` side, gmux also checks which adapter-backed tools are actually installed on the current machine so the launch UI only shows integrations that are available there.

This page is the high-level overview. For implementation details, see [Writing an Adapter](/develop/writing-adapters). For the runtime model behind `gmuxr`, `gmuxd`, session files, and resume, see [Adapter Architecture](/develop/adapter-architecture).

## What adapters do

An adapter watches the terminal output of your process and reports structured status to the sidebar:

- **Active** — the tool is working (green pulsing dot)
- **Attention** — the tool needs your input (orange dot)
- **Error** — something went wrong (red dot)

Without an adapter, gmux still tracks whether the process is alive — but with one, you get meaningful at-a-glance status.

## Automatic detection

You don't configure adapters. gmux recognizes tools by their command name:

```bash
gmuxr pi            # → pi adapter (spinner detection, session resume)
gmuxr bash          # → shell adapter (terminal title tracking)
gmuxr -- make build # → shell adapter
```

If no specific adapter matches, the **shell** adapter takes over. It tracks terminal title changes so your shell's working directory appears in the sidebar, but it doesn't report rich activity status.

For launchers, gmuxd also probes each compiled adapter at startup. Built-in examples:

- **shell** is always available
- **pi** is only available when `pi --version` succeeds on that machine

That lets the UI hide adapters that gmux knows about in principle but that are not usable on the current host.

## Beyond status: session files

Some adapters understand more than just terminal output. The **pi** adapter knows where pi stores its session files, how to extract conversation titles from them, and how to resume previous sessions. This means:

- Resumable sessions appear in the sidebar even when nothing is running
- Session titles show the first message you sent, not just `pi`
- Renaming a session with `/name` updates the sidebar in real time

See [Integrations → pi](/integrations/pi) for the concrete behavior.

## Self-reporting

Any process can report its own status without a custom adapter. `gmuxr` sets `$GMUX_SOCKET` in the child's environment:

```bash
curl -X PUT --unix-socket "$GMUX_SOCKET" \
  http://localhost/status \
  -d '{"label": "building", "state": "active"}'
```

Self-reported status takes priority over adapter-detected status. This lets tools integrate with gmux directly, without changes to gmux itself.

## Learn more

- Want to add support for a new tool? See [Writing an Adapter](/develop/writing-adapters).
- Want to understand how `gmuxr`, `gmuxd`, session files, and resume fit together? See [Adapter Architecture](/develop/adapter-architecture).
