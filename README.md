# Tailboard

Tailboard synchronizes plain text between a Mac and an Android phone over an
existing [Tailscale](https://tailscale.com) network.

- Mac → Android is automatic.
- Android → Mac uses the **Send Clipboard** Quick Settings tile or app button,
  because Android does not allow ordinary background clipboard reads.
- Files, photos, file URLs, images, HTML payloads, and clipboard history are not
  part of Tailboard. Use [Taildrop](https://tailscale.com/kb/1106/taildrop) for
  files.
- **Clear both clipboards** removes the current value from Android, the Mac, and
  Tailboard's memory.

The Mac runs one signed background process. It binds only to the Mac's
Tailscale IPv4 address, polls `NSPasteboard.changeCount`, and keeps at most one
text value in memory. The Android app keeps one WebSocket connection open and
does not maintain its own history. Tailboard creates no separate Tailscale node
and stores no clipboard database.

## Build

Requirements: Go 1.26+, the Tailscale Mac app, stable Xcode, XcodeGen, JDK 17,
and the Android SDK.

```sh
./scripts/install-macos-local.sh

cd android
./gradlew test lint assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

The Mac installer reuses the signing team of an existing installation. On a
first install with multiple Apple Development teams, set
`TAILBOARD_MAC_CODESIGN_IDENTITY` to the intended certificate's SHA-1 hash.

Open the Android app and set the Mac URL to
`http://<your-mac-tailscale-ip>:9437`. Add its Quick Settings tile for fast
phone-to-Mac sends.

For local diagnostics, `make tailboard` builds a small CLI:

```sh
bin/tailboard status
bin/tailboard get
bin/tailboard clear
```

## Privacy and scope

Tailboard trusts devices admitted to your tailnet. Text is protected in transit
by Tailscale but is not additionally encrypted by Tailboard. Pasteboard entries
marked concealed/transient and any entry carrying file semantics are ignored.
System clipboard managers can independently retain copied text.

Tailboard is an independent project and is not affiliated with Tailscale Inc.
It is derived from Thalys Guimarães's MIT-licensed
[`tg-clipboard`](https://github.com/thalysguimaraes/tg-clipboard); see
[LICENSE](LICENSE) and [NOTICE.md](NOTICE.md).
