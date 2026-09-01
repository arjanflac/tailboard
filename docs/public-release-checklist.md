# Public release checklist

The repository is safe to keep private and iterate on now. Do not switch it to
public or publish binaries until every blocking item below is resolved.

## Blocking

- [ ] Complete a naming and trademark review. “Tailboard” is already used by
  multiple software products and a live 2026 U.S. software-services trademark
  application exists for the word mark.
- [ ] Decide whether Tailscale membership alone is the intended authorization
  boundary for a public audience, or add application-level enrollment/auth.
- [ ] Choose public Apple bundle IDs, app-group IDs, Android application ID,
  signing ownership, and upgrade/migration behavior.
- [ ] Replace personal deployment defaults with a documented onboarding flow
  that works on another person's tailnet.
- [ ] Run the complete device matrix on at least one physical Mac, Pixel, and
  iPhone, including offline/reconnect, clipboard privacy prompts,
  background/foreground transitions, text share targets, and a documented
  handoff to Taildrop for photos and files.
- [ ] Decide which artifacts are supported. Public release and TestFlight
  workflows remain disabled until signing, package names, and rollback are set.

## Before first public commit

- [ ] Re-run secret scanning across the full Git history and current tree.
- [ ] Confirm `config.local.env`, device IDs, hostnames, signing identities,
  screenshots, APKs/IPAs, databases, and logs are untracked.
- [ ] Review `SECURITY.md`, `CODE_OF_CONDUCT.md`, and maintainer contact paths.
- [ ] Confirm the upstream MIT license and `NOTICE.md` remain intact.
- [ ] Add screenshots that contain no personal device names or clipboard data.
- [ ] Define supported versions and a vulnerability-response expectation.

## Release evidence

- [ ] `go test ./...`, `go vet ./...`, and the race suite pass.
- [ ] Android `test`, `lint`, and release build pass with a production signing
  configuration.
- [ ] iOS simulator tests and signed physical-device archive pass.
- [ ] macOS engine/menu-bar signing, installation, upgrade, and uninstall are
  tested on a clean user account.
- [ ] Installation, update, uninstall, data retention, and recovery behavior are
  documented for every supported platform.

## Completed foundation

- [x] Preserve the upstream Git history, MIT notice, and explicit attribution.
- [x] Embed the Mac engine as a background-only login item in `Tailboard.app`
  and manage it with `SMAppService`, including legacy LaunchAgent migration and
  a rollback path.
- [x] Sign local Mac development builds with hardened runtime and verify nested
  code before installation.
- [x] Document the Developer ID and notarization path without committing signing
  credentials.
