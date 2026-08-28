#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
CONFIG_FILE=${TAILBOARD_CONFIG_FILE:-"$PROJECT_DIR/config.local.env"}
DERIVED_DATA="$PROJECT_DIR/.derived-data"
PRODUCT="$DERIVED_DATA/Build/Products/Release/Tailboard.app"
DESTINATION=/Applications/Tailboard.app
INSTALL_STAGING="$PROJECT_DIR/.tailboard-build/install"
STAGED_APP="$INSTALL_STAGING/Tailboard.app"

if [ -f "$CONFIG_FILE" ]; then
  # shellcheck disable=SC1090
  . "$CONFIG_FILE"
fi

DEVELOPER_DIR=${TAILBOARD_DEVELOPER_DIR:-/Applications/Xcode.app/Contents/Developer}
: "${TAILBOARD_MAC_CODESIGN_IDENTITY:?missing TAILBOARD_MAC_CODESIGN_IDENTITY in $CONFIG_FILE}"
: "${DEVELOPER_DIR:?missing stable Xcode developer directory}"
if [ ! -x "$DEVELOPER_DIR/usr/bin/xcodebuild" ]; then
  echo "stable Xcode not found at $DEVELOPER_DIR" >&2
  exit 1
fi
export DEVELOPER_DIR

make -C "$PROJECT_DIR" tailboard-engine tailboard
TAILBOARD_ENGINE_CODESIGN_IDENTITY="$TAILBOARD_MAC_CODESIGN_IDENTITY" \
  "$PROJECT_DIR/scripts/prepare-macos-engine-bundle.sh"
ENGINE_APP="$PROJECT_DIR/.tailboard-build/macos/Tailboard Engine.app"
codesign --verify --strict "$ENGINE_APP"

engine_team=$(codesign -dvv "$ENGINE_APP" 2>&1 \
  | sed -n 's/^TeamIdentifier=//p')
TAILBOARD_APPLE_DEVELOPMENT_TEAM=${TAILBOARD_APPLE_DEVELOPMENT_TEAM:-$engine_team}
: "${TAILBOARD_APPLE_DEVELOPMENT_TEAM:?could not determine Apple development team}"

xcodegen generate --spec "$PROJECT_DIR/ios/project.yml" \
  --project "$PROJECT_DIR/ios"
xcodebuild -quiet \
  -project "$PROJECT_DIR/ios/TGClipboard.xcodeproj" \
  -scheme TGClipboardMenuBar \
  -configuration Release \
  -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath "$DERIVED_DATA" \
  DEVELOPMENT_TEAM="$TAILBOARD_APPLE_DEVELOPMENT_TEAM" \
  CODE_SIGN_IDENTITY="Apple Development" \
  CODE_SIGN_STYLE=Automatic \
  build

codesign --verify --deep --strict "$PRODUCT"

case "$INSTALL_STAGING" in
  "$PROJECT_DIR/.tailboard-build/install") ;;
  *) echo "refusing unexpected staging path: $INSTALL_STAGING" >&2; exit 1 ;;
esac
rm -rf "$INSTALL_STAGING"
mkdir -p "$INSTALL_STAGING"
ditto "$PRODUCT" "$STAGED_APP"
codesign --verify --deep --strict "$STAGED_APP"

pkill -x Tailboard 2>/dev/null || true
# Unload pre-modern and pre-release engine jobs before the in-place update.
launchctl bootout "gui/$(id -u)/com.arjanflac.tailboard.engine.agent" 2>/dev/null || true
launchctl bootout "gui/$(id -u)/com.arjanflac.tailboard.engine" 2>/dev/null || true
sleep 1
if [ -d "$DESTINATION" ]; then
  # Keep the application URL continuously present. macOS 27 betas can lose an
  # SMAppService item's parent association if an updater moves the outer bundle
  # away, even when a valid replacement immediately takes the same path.
  rsync -aE --delete "$STAGED_APP/" "$DESTINATION/"
else
  ditto "$STAGED_APP" "$DESTINATION"
fi
codesign --verify --deep --strict "$DESTINATION"
LSREGISTER=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
"$LSREGISTER" -f "$DESTINATION"
"$LSREGISTER" -f "$DESTINATION/Contents/Library/LoginItems/Tailboard Engine.app"

# Remove the old AppleScript-created Open at Login entry. Tailboard now owns
# both main-app and engine registration through SMAppService.
osascript -e 'tell application "System Events" to if exists login item "Tailboard" then delete login item "Tailboard"' 2>/dev/null || true
open "$DESTINATION"

SHARE_EXTENSION="$DESTINATION/Contents/PlugIns/TailboardShare.appex"
if [ -d "$SHARE_EXTENSION" ]; then
  /usr/bin/pluginkit -a "$SHARE_EXTENSION" 2>/dev/null || true
  /usr/bin/pluginkit -e use -i com.arjanflac.tgclipboard.menubar.share 2>/dev/null || true
fi
