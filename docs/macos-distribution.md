# macOS signing and distribution

Tailboard's source remains MIT-licensed whether local builds are unsigned,
development-signed, or distributed as notarized Developer ID binaries. Apple
signing identifies the binary publisher; it does not change the source license
or require an App Store release. GitHub Releases can host the downloadable app,
so a separate website is optional.

## Service layout

The Mac build embeds these service-management resources inside the signed app:

- `Contents/Library/LoginItems/Tailboard Engine.app`

`Tailboard.app` registers the background-only engine bundle with
`SMAppService.loginItem` and registers itself for Open at Login. Engine
arguments are stored per user at
`~/Library/Application Support/tg-clipboard/engine-arguments.json`; the signed
application bundle is never modified after installation. On the first modern
launch, Tailboard migrates the old per-user LaunchAgent and restores it if the
new registration fails. The engine runtime is refreshed when either the
embedded executable or its bundle metadata changes, without discarding the
existing user approval.

Tailboard refreshes the outer and nested bundle locations with Launch Services
after an update. This is particularly important on macOS 27 development betas,
where background-task management can temporarily lose the parent path of a
correctly signed embedded item. If the approved item is not running, the menu
app starts its stable nested bundle directly without discarding that approval.

## Local development

`scripts/install-macos-local.sh` builds the engine, records the local runtime
arguments, signs nested code with the configured Apple Development identity,
builds the app with hardened runtime, verifies the result, installs it into
`/Applications`, and opens it. Copy `config.example.env` to the ignored
`config.local.env` before using the script.

The Go executable is wrapped in a minimal background-only login-item bundle so
macOS service management can retain its identity across app updates. It does
not add a Dock icon or a second user-facing app.

An Apple Development certificate is suitable for the owner's Macs and devices.
It is not a public distribution identity.

## Public GitHub binary

Before attaching a Mac archive to a GitHub release:

1. Create and install a **Developer ID Application** certificate for the Apple
   Developer team. Keep its private key outside the repository.
2. Archive a Release build with hardened runtime. Sign nested executables and
   extensions before signing the outer app, using a secure timestamp.
3. Verify the bundle with `codesign --verify --deep --strict --verbose=2`.
4. Store App Store Connect API credentials in the login keychain with
   `xcrun notarytool store-credentials`; never commit them or pass their values
   in repository scripts.
5. Submit the final ZIP or DMG with `xcrun notarytool submit --keychain-profile
   <profile> --wait`.
6. Staple and validate the ticket with `xcrun stapler staple Tailboard.app` and
   `xcrun stapler validate Tailboard.app`.
7. Test Gatekeeper assessment and a clean-user install/upgrade/uninstall before
   publishing the archive and its SHA-256 checksum.

A public Mac binary remains blocked until its release environment has a
Developer ID Application identity and notarization credential provisioned.

## Signer display name

macOS background-activity notifications take the developer name from the
signing certificate, not `CFBundleDisplayName`, copyright metadata, or the
license. If the certificate subject uses unexpected capitalization, update the
name in the Apple Developer membership details and have Apple approve it, then
issue new Apple Development and Developer ID Application certificates. Existing
certificates keep their original subject and cannot be relabeled by the app.

Do not switch to ad-hoc signing just to change this notification: that discards
the verified publisher identity and is unsuitable for public distribution.

## Release ownership

The public bundle IDs, app group, Android application ID, signing team, and
upgrade policy must be treated as durable release identifiers. Settle those
before the first supported binary because changing them later breaks normal
upgrade and shared-container continuity.
