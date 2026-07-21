# tg-clipboard iOS

iOS companion for tg-clipboard: device-first container app, tg-paste keyboard, share extension, widgets, Live Activity, and App Intents.

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
5. Enable App Groups (`group.com.thalys.cliphub`) on all 4 targets
6. Add all Swift source files to their respective targets
7. All 3 extension/app targets should embed TGClipboardKit

### Configuration

1. Set your Apple Developer Team ID in project.yml or Xcode signing settings
2. Build and run on your iPhone
3. Connect the Tailscale app, then enter the HTTPS hub URL shown by your tg-clipboard deployment.
4. Enable the keyboard: Settings → General → Keyboard → Keyboards → Add → tg-clipboard → Allow Full Access

Hub configuration is shared through Keychain. Clip previews remain cached in the app group so the keyboard can paste recent text while offline.

## Architecture

- **TGClipboardKit**: shared framework with REST client, WebSocket manager, models, storage
- **tg-clipboard**: device roster, transfer inbox, current clip, history, settings, widgets, and Shortcuts actions
- **TGPasteKeyboard**: inserts text, copies image clips to the pasteboard, and can push the local clipboard
- **TGClipboardShare**: sends text/images to the clipboard hub or file sets to a chosen device using background uploads

## iOS interaction model

iOS does not allow background clipboard observation. Every local clipboard read is user initiated:

- Paste with the keyboard.
- Copy a hub clip from the app, widget deep link, or Shortcut.
- Send from the share sheet, keyboard Push action, app, or Shortcut.
- Accept incoming files from the foreground app transfer inbox; downloaded files live in the app's Documents container and are visible through Files.

The keyboard needs Full Access for live networking. Without it, cached clips remain available.

## CI and distribution

The iOS CI job generates the Xcode project, builds the app and all extensions for the simulator, and runs unit tests. `.github/workflows/testflight.yml` archives and uploads on an `ios-v*` tag or manual dispatch when these repository secrets are configured:

- `APP_STORE_CONNECT_API_KEY_ID`
- `APP_STORE_CONNECT_API_ISSUER_ID`
- `APP_STORE_CONNECT_API_KEY_BASE64`
- `APPLE_TEAM_ID`

The API key needs App Manager access so Xcode can manage automatic signing for the app and embedded extensions. Local signing remains available for contributors and personal deployments.

## Requirements

- iOS 17+
- Tailscale VPN active (to reach the hub on your tailnet)
- tg-clipboard hub running on your tailnet
