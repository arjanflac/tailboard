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
received update's random ID, never its text.
There is no iOS app in this repo, and older Tailboard iOS builds are unsupported.

## Setup

Both devices need Tailscale installed, connected, and allowed to reach each other
on your tailnet. Tailboard uses the existing Tailscale apps.

Start the Mac engine using one of the build options below. If you choose the
app-based install, open Tailboard once to enable its background service.
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

## Build from source

Tailboard is an MIT-licensed source project for macOS 14+ and Android 10+.
There are no official prebuilt apps, APKs, DMGs, automatic updates, or notarized
releases. GitHub hosts the code; CI runs tests and builds without distributing
artifacts or using the maintainer's signing credentials.

### Mac: run just the engine

The engine does all clipboard and network work. `Tailboard.app` is an optional
launcher that registers it with macOS as a background login item and then exits.
You do not need the app, an Apple ID, or a signing certificate to build and run
the engine locally. Install Go 1.26.1+ and Xcode Command Line Tools (or Xcode),
then run:

```sh
make tailboard-engine tailboard
"bin/Tailboard Engine"
```

Leave that terminal running; press Control-C to stop. The engine waits for
Tailscale to connect, listens on its IPv4 address at port 9437, and recovers after
disconnects. Do not use `--listen` to expose it outside a trusted tailnet.

For automatic startup at your own macOS login, install the optional per-user
LaunchAgent instead:

```sh
./scripts/install-macos-engine-local.sh --dry-run
./scripts/install-macos-engine-local.sh
```

This builds the engine and applies a local ad-hoc signature, without contacting
Apple or using a signing account. It copies the engine into
`~/.local/libexec/tailboard/`, registers
`~/Library/LaunchAgents/com.arjanflac.tailboard.engine.local.plist`, and logs
startup/error metadata to `~/Library/Logs/Tailboard/`. Clipboard text is not
logged. Run the installer again to update it; stop and remove it with:

```sh
./scripts/install-macos-engine-local.sh --uninstall
```

Use one Mac installation method at a time. The engine-only installer refuses
to replace an existing `Tailboard.app` background service or another server
using port 9437. A LaunchAgent runs in your logged-in desktop session, not before
you log in.

### Mac: optionally build the app

The app route additionally needs Xcode, XcodeGen, and **your own** Apple
Development signing identity in your keychain:

```sh
./scripts/install-macos-local.sh
```

It installs `/Applications/Tailboard.app`. Open it once to register the embedded
engine; allow it in System Settings → General → Login Items if macOS asks.
The installer uses the Xcode selected by `xcode-select` (or `DEVELOPER_DIR`) and
reuses the signing team of an existing installation. Override those choices
with `TAILBOARD_DEVELOPER_DIR` and `TAILBOARD_MAC_CODESIGN_IDENTITY` as needed.
This is a local development build, not a notarized distribution build. CI also
checks the app with `CODE_SIGNING_ALLOWED=NO`; that build check is not an
installation method.

The maintainer does not sign or notarize other users' builds. A downloadable Mac
app would need a separate distribution-signing/notarization process; see
[Apple's Developer ID guidance](https://developer.apple.com/developer-id/).
Making the repository public does not start that process or automatically
publish a DMG. GitHub's automatic release archives contain source; binary assets
are uploaded separately ([GitHub releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases)).

### Android: build your own APK

Install JDK 17 and the Android SDK with platform 37 and build-tools 36.1.0. Set
`JAVA_HOME` and `ANDROID_HOME` for your installation, then run:

```sh
(cd android && ./gradlew test lint assembleDebug)
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

ADB needs an authorized USB or paired wireless debugging connection.
Alternatively, copy the APK to the phone and open it to install. Keep your local
Android debug keystore for subsequent updates: an APK signed by someone else's
key cannot update your installation. A production Android distribution would
need its own stable release-signing key, independent of Apple signing.

With an authorized phone attached, `(cd android && ./gradlew connectedDebugAndroidTest)`
checks saved-Mac upgrades, destination switching, and send failures. Tests use
synthetic text, restore Tailboard settings, and leave the app installed.
`make test test-race lint` checks the Mac engine.

### Diagnostics

```sh
bin/tailboard status
bin/tailboard get
bin/tailboard clear
```

`get` prints the current clipboard text; use `status` when sharing diagnostics.
If sync stops, check `tailscale status` and reconnect with `tailscale up` or the
Tailscale app. The engine keeps running while disconnected; `status` becomes
available again after reconnection. For the app-based install, reopen Tailboard
or check its Login Items approval. For the engine-only agent, inspect its logs.

## Maintenance

This is a small personal utility shared as-is. There is no promised support,
release schedule, or compatibility with older implementations. Build it for
your own devices and keep both clients on compatible versions. Publishing the
source does not enroll it in an app store or commit the maintainer to signing
and distributing binaries.

## Privacy and credit

Any device your Tailscale rules allow to reach port 9437 can read, replace, or
clear the shared text. Restrict that port to trusted devices using your Tailscale
access rules; do not forward it publicly or put it behind a public proxy.
Tailboard accepts native clients, rejects browser-origin requests, and requires
JSON for text updates. Those checks protect against browser requests; they do
not authenticate a device already allowed onto the service. Tailscale encrypts
the connection; Tailboard has no separate login. Mac clipboard items marked
concealed or temporary, and copied
files, are ignored. Other clipboard managers can keep their own copies.

Derived from Thalys Guimarães's [tg-clipboard](https://github.com/thalysguimaraes/tg-clipboard).
MIT licensed; see [LICENSE](LICENSE), [NOTICE.md](NOTICE.md), and
[third-party notices](THIRD_PARTY_NOTICES.md).
An independent project, not affiliated with Tailscale Inc.
