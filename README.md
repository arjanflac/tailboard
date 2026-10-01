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
The engine waits quietly if Tailscale is disconnected and resumes automatically
when it reconnects, including after login, sleep, or a macOS update. It listens
only on the Mac's Tailscale address, never on a public or LAN interface.
On Android, open **Choose default Mac**, give your Mac a name, and enter its
Tailscale IP or MagicDNS hostname (for example, `100.x.x.x` or `macbook`). Tap
**Save as default Mac**, then
tap **Add Send Clipboard tile**. On older Android versions, add the tile using
the Quick Settings editor. If your phone stops receiving in the background,
check that its battery settings allow Tailboard to run.

You can save multiple named Macs and select one as the default. Selecting a saved
Mac and tapping **Save as default Mac** switches both sending and receiving to it.
Existing installations keep their saved Mac address when upgrading. The tile
shows the selected Mac and checks availability when you open Quick Settings.
A send reports **Sent to MacBook** only after that Mac confirms receipt; if it
cannot connect, it shows **MacBook unavailable. Check Tailscale and Tailboard.**
Tailboard cannot distinguish a powered-off Mac from a disconnected Tailscale app
or stopped Tailboard service. Sends time out after at most six seconds.

The phone sends only to its default Mac, never to every device on the tailnet.
That Mac shares the current text with phones connected to its Tailboard server.
No Tailscale API credentials or automatic peer discovery are required. Addresses
with custom ports are supported, including full `http://` or `https://` URLs.

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
The Mac installer uses the Xcode selected by `xcode-select` (or `DEVELOPER_DIR`)
and reuses the signing team of an existing Tailboard installation. Set `TAILBOARD_MAC_CODESIGN_IDENTITY` to
your certificate's SHA-1 hash if you need to choose a different identity.
Set `TAILBOARD_DEVELOPER_DIR` to override the selected Xcode.

Android checks run with `(cd android && ./gradlew test lint assembleDebug)`.
With an authorized phone attached, run `(cd android && ./gradlew connectedDebugAndroidTest)`
to verify saved-Mac upgrades, destination switching, and send failures on device.
These tests restore the phone's Tailboard settings and use only synthetic text.

For diagnostics, `make tailboard` builds the CLI:

```sh
bin/tailboard status
bin/tailboard get
bin/tailboard clear
```

If sync stops, check `tailscale status` and reconnect the existing account with
`tailscale up` or the Tailscale app. The engine stays running while disconnected;
`bin/tailboard status` becomes available again when Tailscale reconnects. If macOS
requests background-service approval, allow Tailboard in System Settings →
General → Login Items.

## Privacy and credit

Any device your Tailscale rules allow to reach port 9437 can read, replace, or
clear the shared text. Tailscale encrypts the connection; Tailboard has no
separate login. Mac clipboard items marked concealed or temporary, and copied
files, are ignored. Other clipboard managers can keep their own copies.

Derived from Thalys Guimarães's [tg-clipboard](https://github.com/thalysguimaraes/tg-clipboard).
MIT licensed; see [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md).
An independent project, not affiliated with Tailscale Inc.
