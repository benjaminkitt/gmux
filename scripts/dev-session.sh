#!/usr/bin/env bash
# Source this to get a gmux-dev command that launches sessions
# against this worktree's dev stack (started by dev-server.sh).
#
# Usage:
#   source scripts/dev-session.sh
#   gmux-dev bash

_GMUX_DEV_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# shellcheck source=./worktree-env.sh
source "$_GMUX_DEV_ROOT/scripts/worktree-env.sh"
gmux_load_worktree_env "$_GMUX_DEV_ROOT"

_GMUX_DEV_INSTANCE="$GMUX_DEV_INSTANCE_NAME"
_GMUX_DEV_PORT="$GMUX_DEV_PORT"
_GMUX_DEV_SOCKET_DIR="$GMUX_DEV_SOCKET_DIR"
_GMUX_DEV_STATE_DIR="$GMUX_DEV_STATE_DIR"

gmux-dev() {
  GMUX_SOCKET_DIR="$_GMUX_DEV_SOCKET_DIR" \
  XDG_STATE_HOME="$_GMUX_DEV_STATE_DIR/state" \
  PI_CODING_AGENT_DIR="$_GMUX_DEV_STATE_DIR/pi-agent" \
  "$_GMUX_DEV_ROOT/bin/gmux-dev" "$@"
}

echo "gmux-dev ($_GMUX_DEV_INSTANCE :$_GMUX_DEV_PORT) → gmux-dev <cmd>"
