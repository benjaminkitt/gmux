#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# shellcheck source=../scripts/worktree-env.sh
source "$ROOT/scripts/worktree-env.sh"
gmux_load_worktree_env "$ROOT"

log() {
  printf '[worktree-teardown] %s\n' "$*"
}

stop_if_running() {
  local bin_path=$1
  if [[ ! -x "$bin_path" ]]; then
    return 0
  fi
  XDG_CONFIG_HOME="$GMUX_DEV_STATE_DIR/config" \
  XDG_STATE_HOME="$GMUX_DEV_STATE_DIR/state" \
  "$bin_path" stop >/dev/null 2>&1 || true
}

if [[ "$GMUX_DEV_ISOLATED" == "1" ]]; then
  log "stopping isolated gmuxd dev daemon for $GMUX_DEV_INSTANCE_NAME"
  stop_if_running "$ROOT/bin/gmuxd-dev"

  if [[ -d "$GMUX_DEV_SOCKET_DIR" ]] && gmux_is_safe_dev_dir "$GMUX_DEV_SOCKET_DIR"; then
    log "removing dev socket dir $GMUX_DEV_SOCKET_DIR"
    rm -rf -- "$GMUX_DEV_SOCKET_DIR"
  fi

  if [[ -d "$GMUX_DEV_STATE_DIR" ]] && gmux_is_safe_dev_dir "$GMUX_DEV_STATE_DIR"; then
    log "removing isolated dev state dir $GMUX_DEV_STATE_DIR"
    rm -rf -- "$GMUX_DEV_STATE_DIR"
  fi
else
  log "main checkout detected; leaving shared gmux-dev state intact"
fi

log "done"
