#!/usr/bin/env bash
# Build gmuxd and gmuxr release binaries.
# Usage: ./scripts/build.sh [--skip-frontend]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin"
WEB_EMBED="$ROOT/services/gmuxd/cmd/gmuxd/web"

skip_frontend=false
for arg in "$@"; do
  case "$arg" in
    --skip-frontend) skip_frontend=true ;;
  esac
done

mkdir -p "$BIN"

# ── Frontend ──

if [ "$skip_frontend" = false ]; then
  echo "→ Building frontend…"
  (cd "$ROOT/apps/gmux-web" && npx vite build)

  # Copy dist into the go:embed directory
  rm -rf "$WEB_EMBED/assets" "$WEB_EMBED/favicon.svg" "$WEB_EMBED/manifest.json"
  cp -r "$ROOT/apps/gmux-web/dist/"* "$WEB_EMBED/"
  echo "  Embedded $(du -sh "$WEB_EMBED" | cut -f1) of frontend assets"
fi

# ── Go binaries ──

LDFLAGS="-s -w"

echo "→ Building gmuxd…"
(cd "$ROOT/services/gmuxd" && go build -ldflags "$LDFLAGS" -o "$BIN/gmuxd" ./cmd/gmuxd)

echo "→ Building gmuxr…"
(cd "$ROOT/cli/gmuxr" && go build -ldflags "$LDFLAGS" -o "$BIN/gmuxr" ./cmd/gmuxr)

echo ""
ls -lh "$BIN/gmuxd" "$BIN/gmuxr"
echo "✓ Build complete"
