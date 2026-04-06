---
bump: minor
---

### Tailscale auto-discovery

- **Zero-config peer discovery on Tailscale.** When `tailscale.enabled`
  is true, gmuxd subscribes to tailnet changes via `WatchIPNBus` and
  reacts immediately when devices come online. Each new device is probed
  with `/v1/health` to confirm it's running gmux, then registered as a
  peer. No manual `[[peers]]` configuration or token exchange needed.

- **Discovery is event-driven, not polling.** Changes are detected
  within seconds instead of the 30-second polling window. Non-gmux
  devices are re-probed at most once per 5 minutes in case gmux was
  installed later.

- **Discovery cache survives restarts.** Known gmux peers are
  re-registered immediately on daemon start without re-probing. The
  cache is stored at `~/.local/state/gmux/tailscale-discovery.json`.

- **Offline peers visible on the home page.** Tailnet devices whose
  hostname matches the tsnet prefix (e.g. `gmux-dev`) appear as dimmed
  cards with "offline" status, even if they've never been probed. Once
  online, they're confirmed and promoted to full peers.
