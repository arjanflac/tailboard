#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
make -C "$PROJECT_DIR" tailboard-engine tailboard
"$PROJECT_DIR/scripts/install-macos-host-local.sh"
