# Source publication decision

Reviewed September 30, 2026. Recommended distribution: public MIT-licensed
source, with users building for their own Mac and Android phone. No maintainer
signed binaries, notarization service, release downloads, or automatic updates.
Changing GitHub visibility is a separate owner action.

The repository contains the upstream MIT attribution and dependency notices.
A redacted Gitleaks scan of all local Git history found no known secrets.
Tracked files and history were also checked for personal device addresses,
signing identities, local configuration, and compiled distribution artifacts.
Build products and local settings remain ignored. Git commit attribution is
part of the source history and remains public when the repository is public.

The installed personal Mac app uses an Apple Development identity; it is not a
notarized Developer ID distribution. CI uses no signing credentials and only
builds and tests. There are no GitHub Releases or uploaded binary assets as of
this review. The engine can run directly or as an ad-hoc-signed per-user
LaunchAgent, without an Apple account. The optional app requires the builder's
own development identity.

Tailboard trusts devices allowed to reach its private Tailscale listener on
port 9437. Those devices can read, replace, and clear its current clipboard.
The server rejects supplied browser Origin headers and requires JSON for
updates, but has no additional client authentication. Do not expose it through
a public proxy or a public network interface. These boundaries are documented
in the README rather than hidden by the installation flow.

Validation included Go tests, race checks and vet; Android unit tests, lint,
builds and device tests; a local Mac app build and install; and an isolated
ad-hoc-signed engine LaunchAgent started without a signing identity. The
engine-only installer also refused to conflict with the personal app service.
Android destination settings survive an upgrade and the installed phone can
reach the installed Mac. These checks cover the supported personal setup;
they are not a promise to support every macOS, Android, or tailnet policy.
