#!/usr/bin/env bash
# Generate favicon and doc site icons from parameters.
# Usage: ./scripts/gen-icons.sh
#
# Outputs:
#   apps/gmux-web/public/favicon.svg       — app favicon (prompt icon)
#   apps/website/public/favicon.svg         — docs/landing favicon (terminal icon)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# ── Shared palette ──
BG="#0f141a"
ACCENT="#49b8b8"    # oklch(72% 0.1 195) — the app's --accent
STROKE_W="2.4"
RADIUS="6"

# ── App favicon: chevron + underscore (prompt) ──
cat > "$ROOT/apps/gmux-web/public/favicon.svg" << SVG
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="$RADIUS" fill="$BG"/>
  <!-- chevron › -->
  <polyline points="10,10 18,16 10,22" fill="none" stroke="$ACCENT" stroke-width="$STROKE_W" stroke-linecap="round" stroke-linejoin="round"/>
  <!-- underscore _ -->
  <line x1="20" y1="22" x2="26" y2="22" stroke="$ACCENT" stroke-width="$STROKE_W" stroke-linecap="round"/>
</svg>
SVG

# ── Docs favicon: terminal window (box + prompt) ──
cat > "$ROOT/apps/website/public/favicon.svg" << SVG
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="$RADIUS" fill="$BG"/>
  <!-- terminal box outline -->
  <rect x="4" y="6" width="24" height="20" rx="3" fill="none" stroke="$ACCENT" stroke-width="$STROKE_W"/>
  <!-- title bar dots -->
  <circle cx="9" cy="10.5" r="1.2" fill="$ACCENT" opacity="0.5"/>
  <circle cx="13" cy="10.5" r="1.2" fill="$ACCENT" opacity="0.5"/>
  <!-- divider below title bar -->
  <line x1="4" y1="14" x2="28" y2="14" stroke="$ACCENT" stroke-width="0.8" opacity="0.3"/>
  <!-- small chevron prompt inside -->
  <polyline points="9,18 13,21 9,24" fill="none" stroke="$ACCENT" stroke-width="$STROKE_W" stroke-linecap="round" stroke-linejoin="round"/>
</svg>
SVG

echo "✓ Generated:"
echo "  apps/gmux-web/public/favicon.svg"
echo "  apps/website/public/favicon.svg"
