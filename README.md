# Tailboard

Tailboard is Arjan's private clipboard bridge between one Mac, one Pixel, and
an optional iPhone client over an existing Tailscale tailnet. It is deliberately
not a public product, package, or upstream contribution.

## What owns each job

| Job | Owner |
| --- | --- |
| Mac → Pixel text | Tailboard, automatic |
| Pixel → Mac text | Tailboard's **Send Clipboard** Quick Settings tile |
| Mac ↔ iPhone text | Apple Universal Clipboard |
| Pixel ↔ iPhone text | Tailboard iOS app when a direct bridge is needed |
| Photos and files | Tailscale Taildrop |

Android 10 and later do not let an ordinary background app read the clipboard.
The Pixel tile briefly brings Tailboard into the foreground, reads the current
text, sends it, and disappears. Replacing Gboard, adding an accessibility
service, or running a privileged Shizuku process would automate that direction
at the cost of a heavier and more fragile system; Tailboard intentionally does
none of those things.

## Runtime

The Mac install has two bundles but one persistent process:

- `Tailboard.app` is a signed, one-shot service host. Opening it registers or
  updates the nested login item and exits.
- `Tailboard Engine` is the only always-on Mac process. It watches the native
  pasteboard, carries the small embedded hub, and has no menu, window, Dock
  icon, control server, or pause-file polling.
- The Android foreground service keeps one WebSocket open so inbound text is
  applied immediately.
- The iOS app is optional and foreground-only, with no keyboard, share, widget,
  Shortcut, Control Center, or Live Activity extensions. It may remain closed
  when Apple Universal Clipboard is enough.

No standalone hub, `tsnet` node, file-transfer protocol, public release
pipeline, Linux build, or Windows build is part of this repository.
The clipboard protocol itself is text-only; there are no binary payload or
blob endpoints hiding behind the UI.

## Local setup

Prerequisites are Go 1.26+, Tailscale, stable Xcode plus XcodeGen, JDK 17, and
an Android SDK.

```sh
cp config.example.env config.local.env
./scripts/install-macos-local.sh
./scripts/configure-local-devices.sh
```

The local Mac installer uses the signing identity from `config.local.env` and
stable `/Applications/Xcode.app`. An Apple Development signature is sufficient
for this personal install. It is not the same as a notarized Developer ID build;
notarization is only useful if the app is packaged for distribution to other
Macs.

Validation:

```sh
go test ./...
go vet ./...
go test -race ./...

cd android
./gradlew test lint assembleDebug
```

## Privacy

Tailboard trusts the devices already admitted to the tailnet. The Mac relay
persists exactly one current clipboard value in SQLite. Each mobile app keeps at
most 20 recent text clips locally for 24 hours. Clipboard text is not
end-to-end encrypted above Tailscale
and may also be retained by system clipboard managers. See
[docs/security.md](docs/security.md).

## Provenance

Tailboard is derived from the MIT-licensed
[`thalysguimaraes/tg-clipboard`](https://github.com/thalysguimaraes/tg-clipboard)
project. Its Git history, license, and attribution are retained even though this
fork remains private. See [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md).
