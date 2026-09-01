# Roadmap

See also: [Architecture](architecture.md), [Security & Privacy](security.md), [Known Limitations](limitations.md), [Platform Support](platform-support.md), [README](../README.md)

This roadmap focuses on the gaps that matter most for adoption. It is intentionally aligned with the current codebase rather than an aspirational rewrite.

## Shipped foundation

- Desktop clipboard parity for plain text, HTML, and PNG across macOS, Linux, and Windows.
- Event-driven Windows/Wayland watching and cheap change-sequence polling fallbacks.
- Stable device registry, protocol capabilities, privacy presets, detector reporting, and packaged background services.
- Maintained iOS app/keyboard/text-share/widget/controls/Intents surface with CI, foreground reconnect, and Keychain configuration.
- Maintained Android app with foreground sync, text sharing, and Quick Settings clipboard action.
- Hub-spooled targeted transfers with consent policies, allowlists, integrity checks, quota/TTL, resumable CLI uploads, and deterministic directory manifests.
- Native macOS menu-bar status and clipboard pause/resume over the loopback control surface.
- Taildrop handoff for photos/files across macOS, Android, and iOS.
- Capability-gated direct desktop fetch with scoped bearer serving, range support, integrity verification, receipt waiting, and transparent spool fallback.

## Next product layer

- Add optional native menu-bar/taskbar wrappers around the shipped browser-backed desktop companion.
- Finish external distribution wiring: TestFlight signing plus authenticated pushes to the Homebrew tap and Scoop bucket.
- Remove the legacy transfer server/CLI after a documented compatibility window.

## Deliberately deferred

- Targeted-transfer E2EE has an accepted transfers-first design and downgrade rule, but implementation awaits cross-platform key custody, shared test vectors, and cryptographic review; the hub currently sees spool contents.
- Clipboard sync stays hub-mediated, last-write-wins, and broadcast.
- There is no stranger pairing, LAN mDNS discovery, or permanent archive.
