#!/usr/bin/env bash
# Source this to get a gmux-dev command that launches sessions
# against the dev environment.
#
# Usage:
#   source scripts/dev-env.sh
#   gmux-dev pi
#   gmux-dev bash

gmux-dev() {
  GMUX_SOCKET_DIR=/tmp/gmux-dev-sessions \
  GMUXD_ADDR=http://localhost:8791 \
  "${GMUX_DEV_BIN:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/bin/gmux-dev}" "$@"
}

echo "gmux-dev command available. Usage: gmux-dev <cmd>"
