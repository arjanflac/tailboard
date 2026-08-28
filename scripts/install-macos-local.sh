#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
CONFIG_FILE=${TAILBOARD_CONFIG_FILE:-"$PROJECT_DIR/config.local.env"}

if [ ! -f "$CONFIG_FILE" ]; then
  echo "Missing $CONFIG_FILE; copy config.example.env and fill in local values." >&2
  exit 1
fi

# shellcheck disable=SC1090
. "$CONFIG_FILE"

: "${TAILBOARD_DESKTOP_NAME:?missing TAILBOARD_DESKTOP_NAME}"
: "${TAILBOARD_AUTO_ACCEPT_DEVICE_IDS:?missing TAILBOARD_AUTO_ACCEPT_DEVICE_IDS}"

make -C "$PROJECT_DIR" tailboard-engine tailboard

"$PROJECT_DIR/bin/Tailboard Engine" write-config \
  --embed-hub \
  --node "$TAILBOARD_DESKTOP_NAME" \
  --transfers accept \
  --transfer-allow "$TAILBOARD_AUTO_ACCEPT_DEVICE_IDS"

"$PROJECT_DIR/scripts/install-menubar-local.sh"
