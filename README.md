# Tailboard

Tailboard is a self-hosted clipboard bridge for macOS, Android, and iPhone over
a private Tailscale tailnet. One Mac carries the hub, lightweight clipboard
engine, and menu-bar app. Tailscale's native Taildrop owns photo and file
delivery.

> [!IMPORTANT]
> Tailboard is pre-release source code. There are no signed public binaries or
> supported package feeds yet. The display name also needs a final naming and
> trademark review before this repository is made public.

## Why it exists

- Copy ordinary text on a Mac and receive clean text on Android or iPhone.
- Send the Android clipboard from a Quick Settings tile.
- Share selected text or links to Tailboard from the Android or iOS share sheet.
- Send photos and files with the Tailscale share target already available on
  macOS, Android, and iOS.
- Keep the transport private to devices already trusted on a Tailscale tailnet.

## Platform behavior

| Direction | Tailboard clipboard | Photos and files |
| --- | --- | --- |
| Mac → Android | Automatic | Tailscale/Taildrop |
| Android → Mac | Quick Settings, app action, or text share | Tailscale/Taildrop |
| iPhone → Mac | Foreground app, keyboard, Shortcut, control, or text share | Tailscale/Taildrop |
| Mac → iPhone | Foreground app/keyboard workflow | Tailscale/Taildrop |
| iPhone ↔ Android | Shared clipboard while iOS is active | Tailscale/Taildrop |

iOS does not permit continuous clipboard observation. Tailboard keeps iPhone
clipboard reads user initiated. Its iOS 18 Control Center buttons open the app
to complete clipboard access in the foreground.

## Components

- **Tailboard Engine** — Go clipboard agent with an optional embedded hub and
  loopback-only desktop API. The Mac app runs it with legacy transfers off.
- **macOS menu-bar app** — a native Swift surface over an engine embedded and
  managed through Apple's modern service API.
- **Android app** — current clip and history, device roster, settings, foreground
  sync connection, Quick Settings tile, and text/link share target.
- **iOS app** — current clip and history, devices, settings, text/link share
  extension, keyboard, widget, Shortcuts, and Control Center controls.
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
Privacy filters are available but opt-in. Do not sync secrets unless every
participating device is trusted for them. Files sent through Taildrop use
Tailscale's encrypted peer-to-peer transport and are outside Tailboard's data
plane.

See [docs/security.md](docs/security.md) and [SECURITY.md](SECURITY.md) for the
full threat model and reporting process.

## Relationship to tg-clipboard

Tailboard is derived from the MIT-licensed
[`thalysguimaraes/tg-clipboard`](https://github.com/thalysguimaraes/tg-clipboard)
project and preserves its Git history and copyright notice. The upstream
project already supplies the cross-platform Go clipboard core, iOS companion,
initial macOS menu-bar surface, and CLI transfer machinery. Tailboard adds the
Android client, embedded personal Mac workflow, modern Apple service management,
Tailboard branding, and product-specific UX. Tailboard briefly productized the
inherited transfer protocol, then retired that app UI in favor of Taildrop. See
[NOTICE.md](NOTICE.md) for the precise provenance statement.

## Release status

CI covers Go on macOS, Linux, and Windows, plus Android and iOS builds. Public
release and TestFlight workflows are intentionally disabled until the naming,
bundle ownership, authentication model, privacy defaults, and distribution
checklist are resolved. See [docs/public-release-checklist.md](docs/public-release-checklist.md).

## License and upstream credit

MIT. Tailboard uses the same permissive license as its upstream and retains the
upstream copyright and permission notice. See [LICENSE](LICENSE) and
[NOTICE.md](NOTICE.md).
