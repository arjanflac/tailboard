# Platform Support

See also: [Architecture](architecture.md), [Security & Privacy](security.md), [Known Limitations](limitations.md), [Roadmap](roadmap.md), [README](../README.md), [iOS setup](../ios/README.md)

## Canonical support statement

The main supported tg-clipboard sync path today is the desktop stack:

- `tg-clipboard` as the hub,
- `tg-clipd` as the clipboard-watching agent,
- `tg-clip` as the direct CLI,
- across macOS, Linux, and Windows.

An iOS companion app, keyboard extension, share extension, and widgets are maintained in CI as a supported companion. Its interaction model intentionally differs from desktop because iOS does not permit background clipboard monitoring.

## Desktop clipboard capability matrix

| Platform | `tg-clipd` status | `text/plain` | `text/html` | `image/png` | Notes |
| --- | --- | :---: | :---: | :---: | --- |
| macOS | supported | yes | yes | yes | Uses a cheap `NSPasteboard.changeCount` check before reading content. |
| Linux (Wayland) | supported | yes | yes | yes | Requires `wl-copy` and `wl-paste`; uses compositor-driven watch events. |
| Linux (X11) | supported | yes | yes | yes | Requires `xclip`. |
| Windows | supported | yes | yes | yes | Uses native Win32 clipboard formats and `AddClipboardFormatListener`; no PowerShell polling. |

## Component support by surface

| Surface | Current state | Notes |
| --- | --- | --- |
| `tg-clipboard` | supported on macOS, Linux, and Windows | Release archives cover Darwin AMD64/ARM64, Linux AMD64/ARM64, and Windows AMD64 and include service definitions. |
| `tg-clipd` | supported on macOS, Linux, and Windows | Rich clipboard parity plus a loopback companion opened with `--tray`; legacy transfers are CLI/compatibility functionality. |
| `tg-clip` | supported on macOS, Linux, and Windows | Includes clipboard commands plus device discovery and resumable, targeted file transfers. |
| iOS app + keyboard + share extension + widgets | supported companion; TestFlight is the intended distribution path | Requires iOS 17+, Tailscale connectivity, and Full Access for live keyboard networking. Simulator builds and tests run in CI. |

## iOS scope today

The iOS companion covers:

- a device-first container app with current clip, history, settings, widget, Control Center controls, and App Intents,
- a custom keyboard that can paste clips, copy image clips, and push the local clipboard,
- a share extension that sends selected text or links.

It is deliberately not a desktop background agent:

| Direction | Surface | Trigger |
| --- | --- | --- |
| Hub → iPhone paste | TailPaste keyboard | User opens the keyboard and taps a text clip |
| Hub → local clipboard | App, widget, or Shortcut | User taps Copy |
| iPhone → clipboard hub | Share extension, app, Shortcut, or keyboard push | User initiates the read/send |
| Device → iPhone files | Tailscale/Taildrop | User initiates or accepts through Tailscale |

## Choosing a deployment target

- For the common personal setup, run `tg-clipd --embed-hub --transfers off` on a frequently-on desktop. This provides the clipboard broker without a dedicated machine.
- Use standalone `tg-clipboard` when you want an independently managed, always-on broker.
- Use the desktop stack for automatic two-way clipboard monitoring.
- Use the iOS companion for explicit paste, copy, text/link share, Shortcut, and Control Center workflows.
- Use Taildrop for photos and files on every supported app platform.
- Keep the Tailscale app connected on iOS. tg-clipboard intentionally does not embed a second VPN tunnel.
