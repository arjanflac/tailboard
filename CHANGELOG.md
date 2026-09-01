# Changelog

## Unreleased

- Converted the Mac app from a persistent menu-bar process into a one-shot
  signed host for a single headless `SMAppService` login item.
- Replaced repeated `osascript` pasteboard access with a direct AppKit bridge
  and reduced the change-count interval from 500 ms to 100 ms.
- Disabled and removed the unused desktop control surface.
- Removed Tailboard's entire file-transfer protocol and delegated photos and
  files to Tailscale Taildrop, including inherited PNG/blob clipboard routes,
  binary protocol fields, database storage, CLI flags, and mobile UI branches.
- Removed the standalone `tsnet` hub, Linux and Windows builds, public release
  automation, package-manager tooling, and public contribution templates.
- Kept Android automatic receive plus its user-initiated Quick Settings send
  action; reduced iOS to an optional foreground app by removing its keyboard,
  share, widget, Shortcut, Control Center, and Live Activity surfaces.
- Made the protocol plain-text-only and read the Mac pasteboard through its
  native string representation, so browser and Notes copies cannot arrive as
  literal HTML markup on mobile devices.
- Fixed the Pixel adaptive icon safe area.
- Replaced Mac-side clipboard history with a one-row, last-value relay and
  removed the history API and CLI command.
- Moved recent clips to fixed device-local mobile caches: 20 items for 24 hours
  on both Android and iOS.
