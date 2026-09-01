# Tailboard

Tailboard is a self-hosted clipboard and file bridge for macOS, Android, and
iPhone over a private Tailscale tailnet. One Mac can carry the hub, clipboard
engine, menu-bar app, and Finder share extension; the mobile apps provide
explicit clipboard actions and one-shot photo or file drops.

> [!IMPORTANT]
> Tailboard is pre-release source code. There are no signed public binaries or
> supported package feeds yet. The display name also needs a final naming and
> trademark review before this repository is made public.

## Why it exists

- Copy ordinary text on a Mac and receive clean text on Android or iPhone.
- Send the Android clipboard from a Quick Settings tile.
- Share photos and files from either mobile share sheet to a chosen device.
- Save an optional default destination for immediate, one-shot file sends; a
  fresh install asks every time.
- Receive verified mobile files directly in `~/Downloads` on the Mac.
- Keep the transport private to devices already trusted on a Tailscale tailnet.

## Platform behavior

| Direction | Clipboard | Files |
| --- | --- | --- |
| Mac → Android | Automatic | Automatic, SHA-256 verified, saved under `Downloads/Tailboard` |
| Android → Mac | Quick Settings or app action | Android share target, saved under `~/Downloads` |
| iPhone → Mac | Foreground app, keyboard, Shortcut, or control | iOS share extension, saved under `~/Downloads` |
| Mac → iPhone | Foreground app/keyboard workflow | Accepted while Tailboard is open; saved in Files → On My iPhone → Tailboard |
| iPhone ↔ Android | Shared clipboard while iOS is active | Select the other phone in the share sheet or save it as the default |

iOS does not permit continuous clipboard observation. Tailboard keeps iPhone
clipboard reads user initiated. Its iOS 18 Control Center buttons open the app
to complete clipboard access in the foreground.

## Components

- **Tailboard Engine** — Go clipboard agent with an optional embedded hub,
  transfer receiver, and loopback-only desktop API.
- **macOS menu-bar app and Finder Share extension** — native Swift surfaces
  over an engine embedded and managed through Apple's modern service API.
- **Android app** — current clip and history, device roster, settings, foreground
  sync connection, Quick Settings tile, and share-sheet destination.
- **iOS app** — current clip and history, devices, transfers, settings, share
  extension, keyboard, widgets, Shortcuts, and Control Center.
- **CLI** — `bin/tailboard` plus compatibility binaries inherited from
  tg-clipboard.

## Build from source

Prerequisites are Go 1.26+, Tailscale, Xcode/XcodeGen for Apple targets, and
JDK 17 plus an Android SDK for Android.

```sh
git clone https://github.com/arjanflac/tailboard.git
cd tailboard

# Go engine, embedded hub, and CLI
make all
go test ./...
go vet ./...

# Android
cd android
./gradlew test lint assembleDebug

# iOS and macOS project
cd ../ios
xcodegen generate
```

Apple contributors must select their own development team and use bundle/app
group identifiers they control before device signing. See [ios/README.md](ios/README.md)
and [android/README.md](android/README.md) for platform details. Developer ID
signing and notarization are only needed when distributing a downloadable Mac
app; they do not change the MIT license. See
[docs/macos-distribution.md](docs/macos-distribution.md).

For a local personal deployment, copy the example configuration and keep the
result ignored:

```sh
cp config.example.env config.local.env
./scripts/install-macos-local.sh
./scripts/configure-local-devices.sh
```

## Security and privacy boundary

Tailboard relies on Tailscale membership and OS disk encryption. Clipboard
history is stored locally by the hub and mobile previews are cached locally.
File payloads are SHA-256 verified but are not additionally end-to-end
encrypted at the application layer. Privacy filters are available but opt-in.
Do not sync secrets unless every participating device is trusted for them.

See [docs/security.md](docs/security.md) and [SECURITY.md](SECURITY.md) for the
full threat model and reporting process.

## Relationship to tg-clipboard

Tailboard is derived from the MIT-licensed
[`thalysguimaraes/tg-clipboard`](https://github.com/thalysguimaraes/tg-clipboard)
project and preserves its Git history and copyright notice. The upstream
project already supplies the cross-platform Go clipboard core, iOS companion,
and initial macOS menu-bar surface. Tailboard adds the Android client, integrated
cross-mobile file transfers, embedded personal Mac workflow, Finder sharing,
Tailboard branding, and product-specific UX. See [NOTICE.md](NOTICE.md) for the
precise provenance statement.

## Release status

CI covers Go on macOS, Linux, and Windows, plus Android and iOS builds. Public
release and TestFlight workflows are intentionally disabled until the naming,
bundle ownership, authentication model, privacy defaults, and distribution
checklist are resolved. See [docs/public-release-checklist.md](docs/public-release-checklist.md).

## License and upstream credit

MIT. Tailboard uses the same permissive license as its upstream and retains the
upstream copyright and permission notice. See [LICENSE](LICENSE) and
[NOTICE.md](NOTICE.md).
