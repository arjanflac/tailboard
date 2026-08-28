#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
DERIVED_DATA="$PROJECT_DIR/.derived-data"
PRODUCT="$DERIVED_DATA/Build/Products/Release/Tailboard.app"
DESTINATION=/Applications/Tailboard.app

xcodegen generate --spec "$PROJECT_DIR/ios/project.yml" \
  --project "$PROJECT_DIR/ios"
xcodebuild -quiet \
  -project "$PROJECT_DIR/ios/TGClipboard.xcodeproj" \
  -scheme TGClipboardMenuBar \
  -configuration Release \
  -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath "$DERIVED_DATA" \
  build

pkill -x Tailboard 2>/dev/null || true
ditto "$PRODUCT" "$DESTINATION"
open "$DESTINATION"

SHARE_EXTENSION="$DESTINATION/Contents/PlugIns/TailboardShare.appex"
if [ -d "$SHARE_EXTENSION" ]; then
  /usr/bin/pluginkit -a "$SHARE_EXTENSION" 2>/dev/null || true
  /usr/bin/pluginkit -e use -i com.arjanflac.tgclipboard.menubar.share 2>/dev/null || true
fi

if ! osascript -e 'tell application "System Events" to get the name of every login item' \
  | tr ',' '\n' | sed 's/^ *//' | grep -qx Tailboard; then
  osascript -e 'tell application "System Events" to make login item at end with properties {name:"Tailboard", path:"/Applications/Tailboard.app", hidden:false}'
fi
