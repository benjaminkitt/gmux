#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

log() {
  printf '[worktree-setup] %s\n' "$*"
}

if [[ ! -f package.json ]]; then
  log "package.json not found, skipping setup"
  exit 0
fi

if ! command -v pnpm >/dev/null 2>&1; then
  log "pnpm is not installed; skipping dependency install"
  exit 0
fi

if ! command -v go >/dev/null 2>&1; then
  log "go is not installed; Go commands will run once Go is available"
fi

log "installing workspace dependencies with pnpm"
pnpm install --frozen-lockfile

cat <<'EOF'

Worktree setup complete.
Useful commands:
  pnpm lint
  pnpm test
  source scripts/dev-session.sh && gmux-dev <cmd>
  ./scripts/dev-server.sh
EOF
