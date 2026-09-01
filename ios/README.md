# Tailboard for iPhone

The iPhone app is an optional foreground client for direct Pixel/iPhone text
clipboard work. Apple Universal Clipboard already owns normal Mac/iPhone
continuity, so Tailboard does not attempt to stay alive in the background.

The target contains only the app and its shared code framework. The former
keyboard, share, widget, App Intent, Control Center, and Live Activity surfaces
were removed because they duplicated the app's Send and Copy buttons without
providing a reliable background path.

## Build

```sh
cd ios
xcodegen generate
open TGClipboard.xcodeproj
```

Choose the local Apple development team, run on the iPhone, keep Tailscale
connected, and configure the Mac hub. The app opens a WebSocket only while its
scene is active and closes it when backgrounded.

Requirements: iOS 17+, Tailscale, and a reachable Tailboard Mac hub.
