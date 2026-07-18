# Known Limitations

See also: [Architecture](architecture.md), [Security & Privacy](security.md), [Platform Support](platform-support.md), [Roadmap](roadmap.md), [README](../README.md)

ClipHub is intentionally small and opinionated. The current behavior favors predictable sync over platform-perfect fidelity.

## Behavior and consistency

- Sync is last-write-wins. Two devices copying at nearly the same time will not merge content.
- Windows and Wayland use event-driven clipboard notifications. macOS polls only the cheap pasteboard change counter because the OS exposes no notification API; X11 retains a sequence-aware polling fallback.
- Reconnect recovery is sequence-based. Clients catch up from `since_seq`, but there is no multi-version history or conflict resolution model.
- Remote writes are read back after apply to handle platform conversions, which means the stored representation can differ from what the source client originally wrote.

## Content-type limitations

- The protocol caps individual clipboard payloads at 10 MiB.
- Files use a separate, targeted transfer primitive. Hub-spooled transfers are resumable and asynchronous; direct peer fetch is not implemented yet.
- Rich content support is platform-dependent:
  - macOS, Linux, and Windows exchange `text/plain`, `text/html`, and `image/png`.
  - OS-native format conversion can still alter HTML or image representation.
- Clipboard format conversion can degrade content. For example, a platform may round-trip HTML as plain text.

## Platform and packaging limitations

- Linux requires either `wl-copy`/`wl-paste` or `xclip`.
- The current cross-platform desktop companion is browser-backed and launched with `clipd --tray`; it does not yet install a native menu-bar icon.
- iOS is built and tested in CI, but TestFlight/App Store delivery still depends on signing and external Apple release configuration.
- The iOS experience is not equivalent to `clipd` on desktop. The app/keyboard/share extension can read from or send to the hub, but there is no always-on iOS background clipboard watcher.
- Release archives cover the desktop matrix, including Linux ARM64. Publishing Homebrew/Scoop repositories still requires their external repository credentials.

## Security and policy limitations

- The hub is trusted with raw clipboard contents.
- Privacy controls exist, but they are opt-in and local to `clipd`; the hub does not centrally enforce them for every client.
- Ignore-list behavior is best-effort because it depends on foreground-context detection. Native Hyprland/Sway and pure-Go X11 EWMH paths are preferred; `xdotool` remains a fallback.
- There is still no per-device permission model, selective sync, or application-layer history encryption.
- Hub-spooled file contents are not end-to-end encrypted.
- Development mode is not a hardened network deployment path.

## Operational limitations

- Auto-discovery depends on Tailscale metadata. If discovery fails, clients fall back to localhost-oriented behavior unless you set an explicit hub URL.
- History retention is short by design. ClipHub is a sync tool, not a long-term clipboard archive.
- Transfer spooling is bounded by quota and TTL. A transfer larger than available hub capacity fails; direct-fetch fallback is future work.
- `tailclip clear` and `tailclip clear --local` help with cleanup, but they do not retroactively wipe clipboard contents that were already written to other devices or offline caches.

The planned work to address the biggest gaps is tracked in [Roadmap](roadmap.md).
