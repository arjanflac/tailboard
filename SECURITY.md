# Security Policy

Tailboard is still pre-release and does not publish supported binaries yet.
Security fixes are made against the latest commit on `main`.

## Supported versions

| Version | Supported |
| --- | --- |
| `main` | Yes |
| Tagged releases | Not yet applicable |

## Reporting a vulnerability

Please do not report security issues in public GitHub issues, pull requests, or discussions.

Use the repository's private **Report a vulnerability** flow under GitHub's
Security tab. Do not send a sensitive report to the upstream tg-clipboard
maintainer unless the issue also affects the upstream project. Include:

- the affected component (Tailboard Engine, hub, CLI, Android app, Apple app or
  extension, or docs/setup),
- the commit, branch, or binary build you tested,
- clear reproduction steps or a proof of concept,
- the impact you expect if the issue is exploitable,
- any logs, screenshots, or packet captures that help explain the report.

You should receive an acknowledgment within 5 business days. The maintainer will keep the report private while confirming impact, preparing a fix, and coordinating disclosure.

If a report is confirmed, the project will use the private GitHub security
advisory to coordinate the fix and disclosure timeline.

## What counts as a security issue

Examples include:

- bypassing the expected Tailscale-only trust boundary,
- exposing clipboard contents or clipboard history to unauthorized parties,
- code execution, injection, or arbitrary file access triggered by clipboard content,
- privilege escalation in the desktop agents, iOS extensions, or local tooling.

If you are not sure whether something is security-sensitive, report it privately anyway.

## Operational privacy limitations

tg-clipboard now exposes opt-in privacy controls for ignore lists, sensitive-content filtering, and explicit clear behavior, but a few limitations remain important:

- Privacy filters are not enabled by default. Operators must opt in with `tg-clipd` flags or environment variables.
- Clipboard history on the hub is stored as plain SQLite, and the iOS cache is stored as plain JSON. tg-clipboard does not yet add application-layer encryption on top of OS disk encryption and file permissions.
- App/process ignore rules are best-effort because they rely on foreground-window detection. On Linux, that currently requires `xdotool` to resolve the active process.
- `tg-clip clear` clears hub state and persisted history, and `tg-clip clear --local` also clears the invoking machine's system clipboard. This does not retroactively wipe clipboard contents already written to other devices or offline caches.

## Non-security bugs

If the issue does not need private handling, please use the public GitHub issue templates so the report can be triaged in the open.
