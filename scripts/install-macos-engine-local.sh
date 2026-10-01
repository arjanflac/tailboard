#!/bin/sh
# Optional per-user LaunchAgent for a locally built engine, without an Apple ID.
set -eu
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")
LABEL=com.arjanflac.tailboard.engine.local
DOMAIN="gui/$(id -u)"
INSTALL_DIR="$HOME/.local/libexec/tailboard"
EXECUTABLE="$INSTALL_DIR/tailboard-engine"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG_DIR="$HOME/Library/Logs/Tailboard"
MODE=${1:-install}

case "$MODE" in
  install|--dry-run|--uninstall) ;;
  *) echo "Usage: $0 [install|--dry-run|--uninstall]" >&2; exit 2 ;;
esac
[ "$(uname -s)" = Darwin ] || { echo "This installer requires macOS" >&2; exit 1; }

if [ -f "$PLIST" ]; then
  if [ "$(plutil -extract Label raw "$PLIST")" != "$LABEL" ] || \
     [ "$(plutil -extract ProgramArguments.0 raw "$PLIST")" != "$EXECUTABLE" ]; then
    echo "Refusing to replace a LaunchAgent not managed by this installer" >&2
    exit 1
  fi
fi

if [ "$MODE" = --uninstall ]; then
  launchctl bootout "$DOMAIN/$LABEL" >/dev/null 2>&1 || true
  rm -f "$PLIST" "$EXECUTABLE"
  echo "Removed the engine-only login agent. Logs remain at $LOG_DIR."
  exit 0
fi

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT HUP INT TERM
STAGED_PLIST="$TEMP_DIR/engine.plist"
plutil -create xml1 "$STAGED_PLIST"
plutil -insert Label -string "$LABEL" "$STAGED_PLIST"
plutil -insert ProgramArguments -json '[]' "$STAGED_PLIST"
plutil -insert ProgramArguments.0 -string "$EXECUTABLE" "$STAGED_PLIST"
plutil -insert RunAtLoad -bool true "$STAGED_PLIST"
plutil -insert KeepAlive -bool true "$STAGED_PLIST"
plutil -insert ThrottleInterval -integer 10 "$STAGED_PLIST"
plutil -insert ProcessType -string Background "$STAGED_PLIST"
plutil -insert LimitLoadToSessionType -string Aqua "$STAGED_PLIST"
plutil -insert StandardOutPath -string "$LOG_DIR/engine.stdout.log" "$STAGED_PLIST"
plutil -insert StandardErrorPath -string "$LOG_DIR/engine.stderr.log" "$STAGED_PLIST"
plutil -lint "$STAGED_PLIST" >/dev/null

if [ "$MODE" = --dry-run ]; then
  cat "$STAGED_PLIST"
  exit 0
fi

if launchctl print "$DOMAIN/com.arjanflac.tailboard.engine.background" >/dev/null 2>&1; then
  echo "Tailboard.app already owns a Mac background service; use one installation method." >&2
  exit 1
fi
if ! launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1 && \
   lsof -nP -iTCP:9437 -sTCP:LISTEN >/dev/null 2>&1; then
  echo "Port 9437 is already in use; stop the other engine before installing." >&2
  exit 1
fi

make -C "$PROJECT_DIR" tailboard-engine tailboard
mkdir -p "$INSTALL_DIR" "$(dirname "$PLIST")" "$LOG_DIR"
ditto "$PROJECT_DIR/bin/Tailboard Engine" "$INSTALL_DIR/tailboard-engine.new"
codesign --force --sign - "$INSTALL_DIR/tailboard-engine.new"
codesign --verify --strict "$INSTALL_DIR/tailboard-engine.new"
launchctl bootout "$DOMAIN/$LABEL" >/dev/null 2>&1 || true
mv -f "$INSTALL_DIR/tailboard-engine.new" "$EXECUTABLE"
cp "$STAGED_PLIST" "$PLIST"
chmod 600 "$PLIST"
launchctl enable "$DOMAIN/$LABEL"
launchctl bootstrap "$DOMAIN" "$PLIST"
launchctl kickstart "$DOMAIN/$LABEL"
attempt=0
engine_pid=
while [ "$attempt" -lt 20 ]; do
  engine_pid=$(launchctl print "$DOMAIN/$LABEL" | awk '/^[[:space:]]*pid = [0-9]+/ {print $3; exit}')
  if [ -n "$engine_pid" ] && kill -0 "$engine_pid" 2>/dev/null; then
    # A successful launch request alone does not mean the process stayed up.
    sleep 1
    if kill -0 "$engine_pid" 2>/dev/null; then
      break
    fi
    engine_pid=
  fi
  sleep 0.5
  attempt=$((attempt + 1))
done
if [ -z "$engine_pid" ] || ! kill -0 "$engine_pid" 2>/dev/null; then
  echo "The engine did not remain running. Check $LOG_DIR/engine.stderr.log." >&2
  exit 1
fi
printf 'Installed the engine-only login agent. Logs: %s\n' "$LOG_DIR"
