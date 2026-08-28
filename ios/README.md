# Tailboard for iOS

iOS companion for Tailboard: device-first container app, custom paste keyboard,
share extension, widgets, Live Activity, and App Intents.

## Setup

### Generate Xcode project

Install [xcodegen](https://github.com/yonaskolb/XcodeGen) and run:

```bash
cd ios
xcodegen generate
open TGClipboard.xcodeproj
```

### Manual Xcode setup (alternative)

1. Create a new iOS App project named "tg-clipboard"
2. Add a Framework target "TGClipboardKit"
3. Add a Custom Keyboard Extension target "TGPasteKeyboard"
4. Add a Share Extension target "TGClipboardShare"
5. Enable App Groups (`group.com.arjanflac.tgclipboard`) on all 4 targets
6. Add all Swift source files to their respective targets
7. All 3 extension/app targets should embed TGClipboardKit

### Configuration

1. Set your Apple Developer Team ID in project.yml or Xcode signing settings
2. Build and run on your iPhone
3. Connect Tailscale, then configure the private hub from onboarding or the
   ignored `config.local.env` installer flow.
4. Enable the keyboard: Settings → General → Keyboard → Keyboards → Add → Tailboard Paste → Allow Full Access

Hub configuration is shared through Keychain. Clip previews remain cached in the app group so the keyboard can paste recent text while offline.

## Architecture

- **TGClipboardKit**: shared framework with REST client, WebSocket manager, models, storage
- **Tailboard**: device roster, transfer inbox, current clip, history, settings, widgets, and Shortcuts actions
- **TGPasteKeyboard**: inserts text, copies image clips to the pasteboard, and can push the local clipboard
- **TGClipboardShare**: sends text to the shared clipboard and waits for file uploads to reach the default Mac before reporting success

## iOS interaction model

iOS does not allow background clipboard observation. Every local clipboard read is user initiated:

- Paste with the keyboard.
- Copy a hub clip from the app, widget deep link, or Shortcut.
- Send from the share sheet, keyboard Push action, app, or Shortcut.
- The share extension asks for a file destination by default. Saving a default
  device in Settings restores one-tap automatic sends. The receiver must allow
  the iPhone's device ID.
- iOS 18+ exposes **Send Clipboard** and **Receive Clipboard** controls. A
  UI-less extension cannot reliably use the system pasteboard, so each control
  opens Tailboard and completes its user-requested clipboard access in the
  foreground app. The last execution result appears in Settings.
- While Tailboard is foregrounded, incoming Mac files are accepted automatically.
  Downloads live in Files → On My iPhone → Tailboard. Filename collisions are
  numbered and identical retries are deduplicated.

The keyboard needs Full Access for live networking. Without it, cached clips remain available.

## CI and distribution

The iOS CI job generates the Xcode project, builds the app and all extensions
for the simulator, and runs unit tests. Public/TestFlight publishing is
intentionally disabled until the repository's naming, bundle identifiers,
signing ownership, and release support policy are final. Local signing remains
available for contributors and personal deployments.

## Requirements

- iOS 17+
- Tailscale VPN active (to reach the hub on your tailnet)
- tg-clipboard hub running on your tailnet
