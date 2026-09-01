# Tailboard for iOS

iOS companion for Tailboard: clipboard app, custom paste keyboard, text/link
share extension, widget, Control Center controls, and App Intents.

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
- **Tailboard**: device roster, current clip, history, settings, widget, controls, and Shortcuts actions
- **TGPasteKeyboard**: inserts text, copies image clips to the pasteboard, and can push the local clipboard
- **TGClipboardShare**: sends selected text or a link to the shared clipboard

## iOS interaction model

iOS does not allow background clipboard observation. Every local clipboard read is user initiated:

- Paste with the keyboard.
- Copy a hub clip from the app, widget deep link, or Shortcut.
- Send text from the share sheet, keyboard Push action, app, or Shortcut.
- iOS 18+ exposes **Send Clipboard** and **Receive Clipboard** controls. A
  UI-less extension cannot reliably use the system pasteboard, so each control
  opens Tailboard and completes its user-requested clipboard access in the
  foreground app. The last execution result appears in Settings. These controls
  do not keep Tailboard alive in the background.
- Use **Send via Tailscale** for photos and files. Tailboard deliberately does
  not appear as a photo/file share target and has no file destination setting.

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
