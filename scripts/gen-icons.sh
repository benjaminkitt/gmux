#!/usr/bin/env bash
# Generate favicon and doc site icons from parameters.
# Usage: ./scripts/gen-icons.sh
#
# Outputs:
#   apps/gmux-web/public/favicon.svg       — app favicon (prompt icon)
#   apps/website/public/favicon.svg         — docs/landing favicon (prompt + border)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# ── Shared palette ──
BG="#0f141a"
ACCENT="#49b8b8"    # oklch(72% 0.1 195) — the app's --accent
STROKE_W="2.4"
RADIUS="6"

# ── Prompt geometry (shared between both icons) ──
# Chevron: 3 points forming ›
CHEV="8,10 16,16 8,22"
# Underscore: horizontal line
USCORE_X1="18"
USCORE_Y="22"
USCORE_X2="25"

# ── App favicon: prompt on solid bg ──
cat > "$ROOT/apps/gmux-web/public/favicon.svg" << SVG
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="$RADIUS" fill="$BG"/>
  <polyline points="$CHEV" fill="none" stroke="$ACCENT" stroke-width="$STROKE_W" stroke-linecap="round" stroke-linejoin="round"/>
  <line x1="$USCORE_X1" y1="$USCORE_Y" x2="$USCORE_X2" y2="$USCORE_Y" stroke="$ACCENT" stroke-width="$STROKE_W" stroke-linecap="round"/>
</svg>
SVG

# ── Docs favicon: same prompt with border ──
cat > "$ROOT/apps/website/public/favicon.svg" << SVG
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="$RADIUS" fill="$BG"/>
  <rect x="3.5" y="3.5" width="25" height="25" rx="3.5" fill="none" stroke="$ACCENT" stroke-width="1.5"/>
  <polyline points="$CHEV" fill="none" stroke="$ACCENT" stroke-width="$STROKE_W" stroke-linecap="round" stroke-linejoin="round"/>
  <line x1="$USCORE_X1" y1="$USCORE_Y" x2="$USCORE_X2" y2="$USCORE_Y" stroke="$ACCENT" stroke-width="$STROKE_W" stroke-linecap="round"/>
</svg>
SVG

echo "✓ Generated:"
echo "  apps/gmux-web/public/favicon.svg"
echo "  apps/website/public/favicon.svg"
