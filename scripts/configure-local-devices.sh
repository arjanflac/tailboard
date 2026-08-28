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

: "${TAILBOARD_HUB_URL:?missing TAILBOARD_HUB_URL}"
: "${TAILBOARD_DESKTOP_NAME:?missing TAILBOARD_DESKTOP_NAME}"
: "${TAILBOARD_ANDROID_NAME:?missing TAILBOARD_ANDROID_NAME}"
: "${TAILBOARD_IOS_NAME:?missing TAILBOARD_IOS_NAME}"
TAILBOARD_DEFAULT_FILE_TARGET=${TAILBOARD_DEFAULT_FILE_TARGET:-}

if [ -n "${TAILBOARD_ANDROID_ADB_SERIAL:-}" ]; then
  adb -s "$TAILBOARD_ANDROID_ADB_SERIAL" shell am start -W \
    -a com.arjanflac.tgclipboard.DEBUG_CONFIGURE \
    -n com.arjanflac.tgclipboard/.MainActivity \
    --es hub_url "$TAILBOARD_HUB_URL" \
    --es device_name "$TAILBOARD_ANDROID_NAME" \
    --es default_target "$TAILBOARD_DEFAULT_FILE_TARGET"
fi

if [ -n "${TAILBOARD_IOS_COREDEVICE_ID:-}" ]; then
  xcrun devicectl device process launch \
    --device "$TAILBOARD_IOS_COREDEVICE_ID" \
    --terminate-existing \
    com.arjanflac.tgclipboard \
    "--tailboard-hub=$TAILBOARD_HUB_URL" \
    "--tailboard-device-name=$TAILBOARD_IOS_NAME" \
    "--tailboard-default-target=$TAILBOARD_DEFAULT_FILE_TARGET"
fi
