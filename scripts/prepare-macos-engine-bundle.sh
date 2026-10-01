#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
ENGINE="$PROJECT_DIR/bin/Tailboard Engine"
OUTPUT_ROOT="$PROJECT_DIR/.tailboard-build/macos"
ENGINE_APP="$OUTPUT_ROOT/Tailboard Engine.app"
INFO_PLIST="$ENGINE_APP/Contents/Info.plist"
EXECUTABLE="$ENGINE_APP/Contents/MacOS/Tailboard Engine"

if [ ! -x "$ENGINE" ]; then
  echo "missing engine executable: $ENGINE" >&2
  exit 1
fi

case "$OUTPUT_ROOT" in
  "$PROJECT_DIR/.tailboard-build/macos") ;;
  *) echo "refusing unexpected output path: $OUTPUT_ROOT" >&2; exit 1 ;;
esac

rm -rf "$ENGINE_APP"
mkdir -p "$ENGINE_APP/Contents/MacOS"
ditto "$ENGINE" "$EXECUTABLE"

plutil -create xml1 "$INFO_PLIST"
plutil -insert CFBundleDevelopmentRegion -string en "$INFO_PLIST"
plutil -insert CFBundleExecutable -string 'Tailboard Engine' "$INFO_PLIST"
plutil -insert CFBundleIdentifier -string com.arjanflac.tailboard.engine.background "$INFO_PLIST"
plutil -insert CFBundleInfoDictionaryVersion -string '6.0' "$INFO_PLIST"
plutil -insert CFBundleName -string 'Tailboard Engine' "$INFO_PLIST"
plutil -insert CFBundlePackageType -string APPL "$INFO_PLIST"
plutil -insert CFBundleShortVersionString -string 2.0 "$INFO_PLIST"
plutil -insert CFBundleVersion -string 13 "$INFO_PLIST"
plutil -insert LSBackgroundOnly -bool true "$INFO_PLIST"
plutil -insert LSMinimumSystemVersion -string 14.0 "$INFO_PLIST"

if [ -n "${TAILBOARD_ENGINE_CODESIGN_IDENTITY:-}" ]; then
  codesign --force \
    --sign "$TAILBOARD_ENGINE_CODESIGN_IDENTITY" \
    --options runtime \
    --timestamp=none \
    "$ENGINE_APP"
fi
