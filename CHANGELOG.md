# Changelog

All notable changes to this project should be documented in this file.

This changelog is intentionally human-maintained. Update `## Unreleased` in the same pull request whenever a change is user-facing, operationally important, security-sensitive, or changes how contributors work with the repository.

Tailboard does not publish supported tagged releases yet. Until it does, keep entries under `## Unreleased`. Once tagged releases begin, move those entries into a versioned or dated heading and start a fresh `## Unreleased` section.

Suggested headings:

- `Added`
- `Changed`
- `Fixed`
- `Security`

## Unreleased

### Added

- Modern macOS background-service registration with the Go engine embedded in
  `Tailboard.app`, including migration and rollback from the legacy LaunchAgent.
- Standalone Tailboard repository preserving the upstream tg-clipboard Git
  history and MIT attribution.
- Android Clipboard, Devices, and Settings tabs with current clip, recent
  history, live device status, Quick Settings setup, and file-destination UX.
- Optional default file destinations on Android and iOS; fresh installs ask in
  the share sheet while configured destinations send immediately.
- Android CI alongside the existing Go and iOS validation.
- Governance baseline for contributors and maintainers, including contribution, security, and conduct documentation plus GitHub issue and pull request templates.
- Deterministic release automation that builds publishable archives, writes SHA-256 checksums, generates release notes, and records release metadata for GitHub releases.
- Package-manager release metadata generation for Homebrew, Scoop, and winget, driven by the published release manifest/checksum assets instead of rebuilding binaries.
- Opt-in clipboard privacy controls for app/process ignore lists, sensitive-content filtering (`secret`, `password-manager`, `otp`), and explicit `tg-clip clear` history wiping.
- Hub operability endpoints for liveness, readiness, and Prometheus-style metrics, plus reconnect/shutdown integration coverage and a `make test-race` workflow entrypoint.

### Changed

- macOS now presents the managed background component as Tailboard Engine under
  Tailboard instead of exposing the signing certificate holder as a standalone
  background item.
- Mac transfers now land directly in `~/Downloads`.
- iOS Control Center actions open Tailboard and complete pasteboard access in
  the foreground app process.
- The iOS app now waits for a real hub response before showing a connected
  state, reconnects as one foreground-only stream, and retires its misleading
  Live Activity control.
- Public release and TestFlight workflows are disabled pending the documented
  release-readiness gates.
- `make release` now emits publishable assets under `dist/release`, and `make release-verify` validates checksum and manifest consistency for dry runs and CI.
- `make release-package-managers` and `make release-package-managers-verify` now stage and validate the generated Homebrew/Scoop/winget definitions in CI and the tagged release workflow.
- Added raw blob upload/download endpoints, cursor-paged history responses, and typed HTTP error envelopes so large tg-clipboard API payloads no longer need to rely solely on base64-in-JSON workflows.

### Security

- Documented current privacy limitations, including plaintext-at-rest history storage and the scope of explicit clipboard clear behavior.

### Fixed

- Prefer `text/plain` over an accompanying browser/Notes HTML representation
  on Mac so Android and iPhone receive readable text instead of markup.
- Convert legacy HTML clips to readable plain text in the Android receive path.
- Shrink the Android adaptive foreground so the full Tailboard mark fits Pixel
  launcher's round icon mask.
- Respect Pixel system-bar insets and wait for launchd to finish unregistering
  the old Mac engine before reinstalling it.
- Track overlapping WebSocket replacements correctly so a stale iPhone socket
  cannot mark its live replacement offline.
