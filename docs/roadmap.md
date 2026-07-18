# Roadmap

See also: [Architecture](architecture.md), [Security & Privacy](security.md), [Known Limitations](limitations.md), [Platform Support](platform-support.md), [README](../README.md)

This roadmap focuses on the gaps that matter most for adoption. It is intentionally aligned with the current codebase rather than an aspirational rewrite.

## Shipped foundation

- Desktop clipboard parity for plain text, HTML, and PNG across macOS, Linux, and Windows.
- Event-driven Windows/Wayland watching and cheap change-sequence polling fallbacks.
- Stable device registry, protocol capabilities, privacy presets, detector reporting, and packaged background services.
- Maintained iOS app/keyboard/share/widgets/Intents surface with CI, foreground reconnect, Keychain configuration, transfer inbox, and background share uploads.
- Hub-spooled targeted transfers with consent policies, allowlists, integrity checks, quota/TTL, resumable CLI uploads, and deterministic directory manifests.

## Next product layer

- Ship the thin desktop tray companion over a local agent API: device drop targets, transfer progress/prompts, and clipboard pause/resume.
- Finish external distribution wiring: TestFlight signing plus authenticated pushes to the Homebrew tap and Scoop bucket.
- Add direct desktop-to-desktop fetch as a capability-gated optimization with transparent spool fallback.

## Deliberately deferred

- Targeted-transfer E2EE using registered device public keys remains an exploration; the schema reserves key material but the hub currently sees spool contents.
- Clipboard sync stays hub-mediated, last-write-wins, and broadcast.
- There is no Android client, stranger pairing, LAN mDNS discovery, or permanent transfer archive.
