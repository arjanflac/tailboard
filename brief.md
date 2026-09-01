# Tailboard brief

Tailboard is a private clipboard bridge focused on text for Arjan's Mac and Pixel,
with an optional foreground iPhone client. It uses the existing Tailscale
network; Taildrop owns every photo and file transfer.

The Mac runs one headless `Tailboard Engine` process. A signed one-shot
`Tailboard.app` exists only to install and update that engine with Apple's
modern service API. Android receives automatically and sends through a Quick
Settings tile because background clipboard reads are restricted by the OS.
Apple Universal Clipboard owns normal Mac/iPhone clipboard continuity.

The repository stays private. It will not publish releases or send changes
upstream. MIT licensing and the upstream notice remain for correct provenance.
