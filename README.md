<img src="assets/branding/tailboard-app-icon-macos-1024.png" alt="Tailboard icon" width="96" height="96">

# Tailboard

Copy text between a Mac and an Android phone over [Tailscale](https://tailscale.com).

A small personal utility, shared as-is. Build it for your own devices. No prebuilt
apps or promised support.

Tailboard uses the native Tailscale apps already installed and connected on your
devices. It doesn't bundle Tailscale or add another device to your tailnet.

## What it does

- **Mac → phone:** copied text arrives automatically.
- **Phone → Mac:** tap **Send Clipboard** in Quick Settings or in the app.
- **Clear both clipboards:** clear the shared text on both connected devices.

Text only, with no clipboard history. Use [Taildrop](https://tailscale.com/kb/1106/taildrop)
for files and photos.

## Install

Requires macOS 14+ and Android 10+.
Clone this repository and run these commands from its root.

### Mac

Install Go 1.26.8+ and Xcode Command Line Tools, then run:

```sh
./scripts/install-macos-engine-local.sh
```

This builds the engine and starts it at login. No Mac app or Apple account is
needed. Run the same command to update; add `--uninstall` to remove it.

### Android

Install JDK 17 and the Android SDK (platform 37, build-tools 36.1.0), then build:

```sh
cd android
./gradlew assembleDebug
```

Copy `android/app/build/outputs/apk/debug/app-debug.apk` from the repository to
your phone and open it to install. You can also [install with ADB](docs/building.md#android).

### Connect

1. On the phone, open **Choose default Mac**.
2. Enter a name and the Mac's Tailscale IP or hostname, then tap **Save as default Mac**.
3. Tap **Add Send Clipboard tile** to add it to Quick Settings.

You can save several Macs and choose a default. The phone sends only to that
Mac and tells you if it is unavailable. Keep Tailboard and Tailscale running on
both devices.

See [build options and troubleshooting](docs/building.md) for the optional Mac app,
running the engine in a terminal, and connection checks.

## Privacy

Tailscale encrypts the connection. Tailboard keeps only the current text in
memory and has no separate login. Any device allowed to reach port 9437 can read,
replace, or clear that text. Allow only trusted devices and keep the service private.

## License

[MIT](LICENSE). Derived from Thalys Guimarães's
[tg-clipboard](https://github.com/thalysguimaraes/tg-clipboard).
See [credits](NOTICE.md) and [dependency notices](THIRD_PARTY_NOTICES.md).
Independent of Tailscale Inc.
