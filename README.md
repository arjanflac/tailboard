# Tailboard

Copy text between a Mac and an Android phone over [Tailscale](https://tailscale.com).
Built for my Mac and Pixel; other Android phones should work too.

- Copy on the Mac and it appears on the phone's clipboard automatically.
- On Android, tap **Send Clipboard** in Quick Settings or use the app's send button.
- **Clear both clipboards** clears the phone, the Mac, and Tailboard's current text
  while they are connected. It does not erase Raycast or keyboard clipboard history.

Text only. Use [Taildrop](https://tailscale.com/kb/1106/taildrop) for files and photos.
Tailboard has no clipboard history, database, share extension, or menu bar icon.
The Mac keeps one text value in memory (up to 1 MiB); the phone stays connected
in the background.
Reconnecting does not copy the same update again. Android saves only the last
received update's random ID, never its text. Update both apps together.
There is no iOS app in this repo, and older Tailboard iOS builds are unsupported.

## Setup

Both devices need Tailscale installed, connected, and allowed to reach each other
on your tailnet. Tailboard uses the existing Tailscale apps.

After installing, open Tailboard on the Mac once to enable its background service.
On Android, enter `http://<your-mac-tailscale-ip>:9437` in **Edit connection**, then
tap **Add Send Clipboard tile**. On older Android versions, add the tile using
the Quick Settings editor. If your phone stops receiving in the background,
check that its battery settings allow Tailboard to run.

**No ADB permission grant, root, or accessibility service is needed.** Android
[requires focus to read the clipboard](https://developer.android.com/about/versions/10/privacy/changes#clipboard-data),
so the tile briefly opens a transparent activity to send your text. Permissions
previously granted to KDE Connect or another app are not needed by Tailboard.
USB or wireless debugging is only needed if you install the APK using ADB;
you can turn it off afterward.

## Build and install

This is currently a source project, without a packaged public release.
Requires macOS 14+, Android 10+, Go 1.26.1+, Xcode, XcodeGen, JDK 17, and the
Android SDK (platform 37). The Mac installer also requires an Apple Development
signing certificate in your keychain.

From the repo folder:

```sh
./scripts/install-macos-local.sh
(cd android && ./gradlew lint assembleDebug)
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

ADB needs an authorized USB connection or paired wireless debugging connection.
Alternatively, copy the APK to the phone and open it to install.
The Mac installer uses `/Applications/Xcode.app` and reuses the signing team of
an existing Tailboard installation. Set `TAILBOARD_MAC_CODESIGN_IDENTITY` to
your certificate's SHA-1 hash if you need to choose a different identity.

For diagnostics, `make tailboard` builds the CLI:

```sh
bin/tailboard status
bin/tailboard get
bin/tailboard clear
```

## Privacy and credit

Any device your Tailscale rules allow to reach port 9437 can read, replace, or
clear the shared text. Tailscale encrypts the connection; Tailboard has no
separate login. Mac clipboard items marked concealed or temporary, and copied
files, are ignored. Other clipboard managers can keep their own copies.

Derived from Thalys Guimarães's [tg-clipboard](https://github.com/thalysguimaraes/tg-clipboard).
MIT licensed; see [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md).
An independent project, not affiliated with Tailscale Inc.
