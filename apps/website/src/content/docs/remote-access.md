---
title: Remote Access
description: Access gmux from your phone, tablet, or another machine over tailscale.
---

:::caution[Not a collaboration tool]
Remote access is designed for accessing **your own machine** from your other devices — your phone, tablet, or laptop. It is not intended for sharing terminal sessions with other people. Every connected user gets full, unrestricted shell access. See [Security](/security) for the full threat model.
:::

By default, gmux only listens on localhost. To access it from another device — your phone on the couch, a laptop in another room, or a tablet on the go — you can enable the built-in [tailscale](https://tailscale.com) listener.

## Why tailscale?

[Tailscale](https://tailscale.com) is a zero-config VPN built on [WireGuard](https://www.wireguard.com/). It creates a private network (a "tailnet") between your devices — your desktop, phone, laptop, servers — without opening ports or configuring firewalls. Devices find each other by name (e.g. `gmux.your-tailnet.ts.net`) and all traffic is end-to-end encrypted.

gmux uses tailscale because exposing terminal access to a network demands strong guarantees:

- **Encrypted transport** — WireGuard encrypts all traffic. No one can sniff your terminal sessions, even on public Wi-Fi.
- **Cryptographic identity** — every connection is authenticated by tailscale's key exchange. Peer identity can't be spoofed.
- **No ports to open** — tailscale punches through NATs. No firewall rules, no port forwarding, no dynamic DNS.
- **Automatic HTTPS** — tailscale provides valid TLS certificates via Let's Encrypt for `*.ts.net` hostnames.

gmux adds an identity-verified **allow list** on top — see [Security](/security) for how this works at a technical level.

## Setup

### 1. Set up tailscale

If you haven't used tailscale before:

1. [Create an account](https://login.tailscale.com/start) — free for personal use with up to 100 devices.
2. [Install tailscale](https://tailscale.com/download) on the machine running gmux.
3. Run `tailscale up` and sign in.
4. Install tailscale on the device you want to connect from (phone, laptop, etc.) and sign in with the same account.
5. Verify both devices can see each other: `tailscale status`.

If you already use tailscale, just make sure both devices are on the same tailnet.

### 2. Enable HTTPS certificates

gmux requires HTTPS. In the [tailscale admin console](https://login.tailscale.com/admin/dns):

1. Go to **DNS** → **HTTPS Certificates**.
2. Enable HTTPS certificates for your tailnet.

This lets tailscale issue valid TLS certificates for `*.your-tailnet.ts.net` hostnames.

### 3. Configure gmux

Create or edit `~/.config/gmux/config.toml`:

```toml
[tailscale]
enabled = true
```

That's it. Your own tailscale account is automatically whitelisted — gmuxd detects the node owner at startup. The hostname defaults to `gmuxd`, making it available at `https://gmux.your-tailnet.ts.net`.

| Field | Description |
|---|---|
| `enabled` | Start the tailscale listener. Default `false`. |
| `hostname` | The machine name on your tailnet. Default `gmux`, giving you `gmux.your-tailnet.ts.net`. Change this if you run gmux on multiple machines. |

### 4. Restart gmuxd

```bash
# If gmuxd is running, kill it — gmux will auto-start it next time
pkill gmuxd
gmux pi  # or any command — gmuxd starts automatically
```

Look for the log line:

```
tsauth: node owner you@github auto-whitelisted
tsauth: connected
```

### 5. Connect

On your other device, open:

```
https://gmux.your-tailnet.ts.net
```

The connection is HTTPS with a valid certificate. No certificate warnings, no HTTP fallback.

## What's checked on every request

1. The connection must come through tailscale (the listener only accepts tailnet traffic).
2. gmuxd calls tailscale's `WhoIs` API to identify the connecting peer's cryptographic identity.
3. The peer's **login name** is checked against the allow list.
4. If the login name doesn't match, the request gets a `403 Forbidden` and the attempt is logged.

This check runs on every HTTP request and WebSocket upgrade — there are no session cookies or tokens that could be stolen. For the full security design, see the [Security](/security) page.

## The localhost listener is unchanged

The tailscale listener is a second, independent listener. The localhost listener (`127.0.0.1:8790`) continues to work exactly as before, with no authentication. Local access is always available — you can't lock yourself out by misconfiguring tailscale.

## Adding other accounts

If you use multiple tailscale accounts (e.g. personal and work), add them to the allow list:

```toml
[tailscale]
enabled = true
allow = ["your-other-account@github"]
```

:::danger[Think twice before adding other people]
The `allow` list should ideally only contain accounts that belong to you. There are no permission levels — everyone on the list gets full shell access to your machine, can read all terminal output (including secrets), launch processes, and kill sessions. This is equivalent to giving someone your SSH key.
:::

## Troubleshooting

**"tsauth: could not determine node owner"** — gmuxd couldn't identify your tailscale account. Make sure tailscale is logged in (`tailscale status`) and try again.

**Can't reach the hostname** — Make sure both devices are on the same tailnet and that [MagicDNS](https://tailscale.com/kb/1081/magicdns) is enabled in your tailscale admin console.

**Certificate warning** — Make sure HTTPS certificates are enabled in your [tailscale DNS settings](https://login.tailscale.com/admin/dns). Tailscale issues valid Let's Encrypt certificates for `*.ts.net` hostnames automatically.

**First-time tailscale auth** — The first time gmuxd starts with tailscale enabled, tsnet may need to authenticate. Check the gmuxd logs for a login URL.
