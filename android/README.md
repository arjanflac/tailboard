# Tailboard for Android

Tailboard's Android companion connects to a private Tailboard hub over
Tailscale:

- Desktop → Android clipboard updates arrive automatically through a foreground
  connection service.
- Android → other devices: copy text, then tap the **Send Clipboard** Quick
  Settings tile.
- Android share sheet: choose **Tailboard** to send text, photos, or files. A
  fresh install asks for the file destination; Settings can save a default for
  immediate one-shot sends.
- The old Direct Share shortcut was removed so the share sheet has one clear
  destination: **Tailboard**.
- Incoming files from registered macOS and iOS devices are accepted automatically,
  verified with SHA-256, and published under `Downloads/Tailboard`.

Android 10+ only lets the focused app or default keyboard read clipboard
contents. The Quick Settings tile therefore uses a 1×1 transparent activity to
perform the user-requested read. Android's own pasteboard privacy notice cannot
be renamed by the app. Notification permission is intentionally not requested,
which hides the foreground-service notification. File shares show an immediate
"Sending…" toast from the share surface and a final "File sent to MacBook"
confirmation.

The main app mirrors the iOS information architecture with Clipboard, Devices,
and Settings tabs. It shows the current clip, recent text history, live device
status, file destination, and Quick Settings setup. Legacy HTML clipboard
payloads are converted to readable plain text on receipt.

## Build

```sh
export JAVA_HOME=/path/to/jdk-17
./gradlew assembleDebug
```

## Install

Connect the Pixel through USB or Android's Wireless debugging, then run:

```sh
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

Open **Tailboard** once, give it unrestricted battery access, and tap
**Add Send Clipboard tile**. Keep notification access disabled if the silent,
notification-free background connection is preferred.
