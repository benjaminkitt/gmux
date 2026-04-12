#!/usr/bin/env bash

# Shared helpers for deriving per-worktree gmux dev settings.

# A linked git worktree has a .git file that points at the main repo's gitdir.
gmux_is_linked_worktree() {
  local root=${1:?root path required}
  [[ -f "$root/.git" ]] || return 1
  grep -Eq '^gitdir: ' "$root/.git"
}

gmux_slugify() {
  local value=${1:-worktree}
  value=${value,,}
  value=$(printf '%s' "$value" | tr -cs 'a-z0-9' '-')
  value=${value#-}
  value=${value%-}
  if [[ -z "$value" ]]; then
    value="worktree"
  fi
  printf '%s' "$value"
}

gmux_load_worktree_env() {
  local root=${1:?root path required}
  local state_base hash slug suffix

  state_base="${XDG_STATE_HOME:-$HOME/.local/state}"

  GMUX_DEV_ROOT="$root"
  GMUX_DEV_STATE_BASE="$state_base"

  if gmux_is_linked_worktree "$root"; then
    hash=$(printf '%s' "$root" | cksum | awk '{print $1}')
    slug=$(gmux_slugify "$(basename "$root")")
    slug=${slug:0:40}
    printf -v suffix '%05d' "$((hash % 100000))"

    GMUX_DEV_ISOLATED=1
    GMUX_DEV_INSTANCE_NAME="${slug}-${suffix}"
    GMUX_DEV_PORT=$((8800 + hash % 1000))
    GMUX_DEV_VITE_PORT=$((9800 + hash % 1000))
    GMUX_DEV_SOCKET_DIR="/tmp/gmux-dev-${GMUX_DEV_INSTANCE_NAME}"
    GMUX_DEV_STATE_DIR="$state_base/gmux-dev-${GMUX_DEV_INSTANCE_NAME}"
    GMUX_DEV_TAILSCALE_HOSTNAME="gmux-dev-${GMUX_DEV_INSTANCE_NAME}"
  else
    GMUX_DEV_ISOLATED=0
    GMUX_DEV_INSTANCE_NAME="gmux-dev"
    GMUX_DEV_PORT=8791
    GMUX_DEV_VITE_PORT=5173
    GMUX_DEV_SOCKET_DIR="/tmp/gmux-dev-sessions"
    GMUX_DEV_STATE_DIR="$state_base/gmux-dev"
    GMUX_DEV_TAILSCALE_HOSTNAME="gmux-dev"
  fi
}

gmux_is_safe_dev_dir() {
  local dir=${1:?dir path required}
  [[ "$dir" == /tmp/gmux-dev-* ]] || [[ "$dir" == "$GMUX_DEV_STATE_BASE"/gmux-dev-* ]]
}
