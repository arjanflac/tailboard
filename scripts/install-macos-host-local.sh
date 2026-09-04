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
if [ -z "${TAILBOARD_MAC_CODESIGN_IDENTITY:-}" ]; then
  tailboard_preferred_team=${TAILBOARD_APPLE_DEVELOPMENT_TEAM:-}
  if [ -z "$tailboard_preferred_team" ] && [ -d "$DESTINATION" ]; then
    tailboard_preferred_team=$(codesign -dvv "$DESTINATION" 2>&1 \
      | sed -n 's/^TeamIdentifier=//p')
  fi
  tailboard_identities=$(security find-identity -v -p codesigning \
    | sed -n '/Apple Development/p')
  if [ -n "$tailboard_preferred_team" ]; then
    while IFS= read -r tailboard_identity_line; do
      tailboard_identity_hash=$(printf '%s\n' "$tailboard_identity_line" \
        | sed -n 's/.*\([0-9A-F]\{40\}\).*/\1/p')
      tailboard_identity_name=$(printf '%s\n' "$tailboard_identity_line" \
        | sed -n 's/.*"\(.*\)".*/\1/p')
      tailboard_identity_team=$(security find-certificate -a -c "$tailboard_identity_name" -p 2>/dev/null \
        | openssl x509 -noout -subject 2>/dev/null \
        | sed -n 's/.*OU=\([^,]*\).*/\1/p' \
        | head -1)
      if [ "$tailboard_identity_team" = "$tailboard_preferred_team" ]; then
        TAILBOARD_MAC_CODESIGN_IDENTITY=$tailboard_identity_hash
        break
      fi
    done <<EOF
$tailboard_identities
EOF
  fi
  if [ -z "${TAILBOARD_MAC_CODESIGN_IDENTITY:-}" ]; then
    TAILBOARD_MAC_CODESIGN_IDENTITY=$(printf '%s\n' "$tailboard_identities" \
      | sed -n 's/.*\([0-9A-F]\{40\}\).*/\1/p' \
      | head -1)
  fi
fi
: "${TAILBOARD_MAC_CODESIGN_IDENTITY:?no Apple Development signing identity found}"
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

xcodegen generate --spec "$PROJECT_DIR/macos/project.yml" \
  --project "$PROJECT_DIR/macos"
xcodebuild -quiet \
  -project "$PROJECT_DIR/macos/Tailboard.xcodeproj" \
  -scheme Tailboard \
  -configuration Release \
  -destination 'platform=macOS,arch=arm64' \
  -derivedDataPath "$DERIVED_DATA" \
  DEVELOPMENT_TEAM="$TAILBOARD_APPLE_DEVELOPMENT_TEAM" \
  CODE_SIGN_IDENTITY="$TAILBOARD_MAC_CODESIGN_IDENTITY" \
  CODE_SIGN_STYLE=Manual \
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
# The modern login item can be restarted through Launch Services during an
# in-place update, so it may no longer be attached to a printable launchctl
# job even though SMAppService still owns its registration. Stop that process
# explicitly before replacing the nested signed bundle.
# macOS 27 may expose an SMAppService login item under its bundle identifier
# rather than its executable name.
pkill -f '^com\.arjanflac\.tailboard\.engine\.background$' 2>/dev/null || true
pkill -f '^/Applications/Tailboard[.]app/Contents/Library/LoginItems/Tailboard Engine[.]app/Contents/MacOS/Tailboard Engine$' 2>/dev/null || true
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

open -W "$DESTINATION"

ENGINE_DESTINATION="$DESTINATION/Contents/Library/LoginItems/Tailboard Engine.app"
ENGINE_SERVICE="gui/$(id -u)/com.arjanflac.tailboard.engine.background"
engine_running() {
  pgrep -f '^(com[.]arjanflac[.]tailboard[.]engine[.]background|/Applications/Tailboard[.]app/Contents/Library/LoginItems/Tailboard Engine[.]app/Contents/MacOS/Tailboard Engine)$' >/dev/null
}

# Wait for the one-shot host to finish registration, remove any direct
# Launch Services instance it used while recovering, then start the registered
# SMAppService job itself. `kickstart` is needed on macOS 27 betas when the job
# remains enabled but records the replaced executable as a successful exit.
pkill -f '^/Applications/Tailboard[.]app/Contents/Library/LoginItems/Tailboard Engine[.]app/Contents/MacOS/Tailboard Engine$' 2>/dev/null || true
launchctl kickstart -kp "$ENGINE_SERVICE" >/dev/null 2>&1 || true

launch_attempt=0
while [ "$launch_attempt" -lt 20 ] && ! engine_running; do
  sleep 0.5
  launch_attempt=$((launch_attempt + 1))
done

if ! engine_running; then
  # Last-resort recovery for systems where the service job is still importing.
  open -gj "$ENGINE_DESTINATION"
  launch_attempt=0
  while [ "$launch_attempt" -lt 20 ] && ! engine_running; do
    sleep 0.5
    launch_attempt=$((launch_attempt + 1))
  done
fi

if ! engine_running; then
  echo "Tailboard Engine did not start after installation" >&2
  exit 1
fi
