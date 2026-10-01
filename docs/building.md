# Build options and troubleshooting

Run commands from the repository root unless shown otherwise. Use one Mac
installation method at a time.

## Mac engine

Requires Go 1.26.8+ and Xcode Command Line Tools or Xcode.

To run in a terminal instead of installing at login:

```sh
make tailboard-engine tailboard
"bin/Tailboard Engine"
```

Leave the terminal open; press Control-C to stop.

For automatic startup at login:

```sh
./scripts/install-macos-engine-local.sh --dry-run
./scripts/install-macos-engine-local.sh
```

This uses a local ad-hoc signature, with no Apple account. The engine is installed
in `~/.local/libexec/tailboard/`. Its login agent is
`~/Library/LaunchAgents/com.arjanflac.tailboard.engine.local.plist`.
Logs are in `~/Library/Logs/Tailboard/`; clipboard text is not logged.

Run the installer again to update. To stop and remove it:

```sh
./scripts/install-macos-engine-local.sh --uninstall
```

The engine waits for Tailscale and resumes when it reconnects. It listens on the
Mac's Tailscale address at port 9437. Do not use `--listen` to expose it publicly.
If several VPNs use Tailscale's IP range, automatic detection stops. Run the
engine with `--listen YOUR_TAILSCALE_IP:9437` to choose the correct address.
Clients connect directly to the selected Mac; HTTP redirects and system proxies
are disabled.

## Optional Mac app

The app registers the engine as a macOS background login item, then exits.
It requires Xcode, XcodeGen, and your own Apple Development signing identity.

```sh
./scripts/install-macos-local.sh
```

It installs `/Applications/Tailboard.app`. Open it to enable the background
service. Allow it in **System Settings → General → Login Items** if prompted.

The installer uses the selected Xcode and prefers the signing team of an existing
installation. Set `TAILBOARD_DEVELOPER_DIR` or `TAILBOARD_MAC_CODESIGN_IDENTITY`
to override these. This produces a local development build, not a notarized release.

## Android

Requires JDK 17 and Android SDK platform 37 with build-tools 36.1.0.
Set `JAVA_HOME` and `ANDROID_HOME` to your installation paths if needed.

```sh
(cd android && ./gradlew assembleDebug)
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

ADB requires an authorized USB or paired wireless debugging connection.
You can turn debugging off after installation. Tailboard needs no root or
accessibility access.

Keep your local Android debug keystore for future updates; an APK signed with a
different key cannot update the installed app.

## Connection problems

- Check that both devices are connected in Tailscale and allowed to reach port 9437.
- Check the default Mac's address in the phone's settings.
- If background receiving stops, allow Tailboard to run in the phone's battery settings.
- For the Mac app, reopen Tailboard and check its Login Items approval.
- For the engine-only install, check `~/Library/Logs/Tailboard/`.

The phone's unavailable message can mean the Mac is off, Tailscale is disconnected,
or Tailboard is stopped. A send times out within six seconds.

Build the command-line helper with `make tailboard`, then check:

```sh
bin/tailboard status
```

`bin/tailboard get` prints the current clipboard text. `bin/tailboard clear`
clears the shared text. Clearing does not erase other apps' clipboard history.

## Development checks

```sh
make test test-race lint
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
(cd android && ./gradlew test lint assembleDebug assembleDebugAndroidTest)
```

With an authorized phone connected:

```sh
(cd android && ./gradlew connectedDebugAndroidTest)
```

Device tests use synthetic text, restore settings, and leave the app installed.
