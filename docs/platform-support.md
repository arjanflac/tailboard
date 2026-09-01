# Platform Support

See also: [Architecture](architecture.md), [Known Limitations](limitations.md),
[README](../README.md), and [iOS setup](../ios/README.md).

## Clipboard engine

| Platform | Automatic watcher | `text/plain` | `text/html` | `image/png` | Notes |
| --- | :---: | :---: | :---: | :---: | --- |
| macOS | yes | yes | yes | yes | Uses `NSPasteboard.changeCount` before reading. |
| Linux Wayland | yes | yes | yes | yes | Requires `wl-copy` and `wl-paste`. |
| Linux X11 | polling | yes | yes | yes | Requires `xclip`. |
| Windows | yes | yes | yes | yes | Uses native Win32 clipboard notifications and formats. |

## Native apps

| Surface | Current scope |
| --- | --- |
| macOS | Embedded engine, menu-bar status, device roster, current clip, and pause/resume. |
| Android | Automatic receive, foreground sync service, current clip/history, text share target, and Quick Settings send action. |
| iOS | Foreground app, keyboard, text/link share target, widget, Shortcuts, and Control Center controls. |

iOS is intentionally not a desktop-style background agent:

| Direction | Trigger |
| --- | --- |
| Hub → iPhone clipboard | User taps in the app, keyboard, widget, Shortcut, or Control Center. |
| iPhone → hub | User invokes the app, keyboard, text share target, Shortcut, or Control Center. |
| Photos/files | User chooses Tailscale in the native share sheet. |

## Deployment

- The personal setup runs `tg-clipd --embed-hub` on a frequently available
  Mac.
- `tg-clipboard` remains available for an independently managed hub.
- `tg-clip` provides clipboard, history, status, device, pause, resume, and
  clear commands.
- Linux and Windows are supported by the Go tools but do not have Tailboard
  native GUIs.
- Every client requires reachability through the same Tailscale tailnet.
