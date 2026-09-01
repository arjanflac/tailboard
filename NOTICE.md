# Attribution and provenance

Tailboard is a derivative of
[`thalysguimaraes/tg-clipboard`](https://github.com/thalysguimaraes/tg-clipboard),
originally created by Thalys Guimarães and distributed under the MIT License.

This repository was created from upstream commit
`2cfad69b9b13fdde26ac2b616e87390a84a711f1` and retains the upstream Git
history. The original copyright and permission notice remain in `LICENSE`.

Tailboard-specific work includes:

- the Android application, foreground sync service, Quick Settings action,
  adaptive launcher assets, and text share-sheet flow;
- the embedded personal Mac hub/engine workflow;
- Tailboard product branding and the expanded mobile/desktop UX;
- local deployment and configuration scripts.

The repository still contains the earlier transfer engine for compatibility and
historical attribution, but current Tailboard apps delegate photos and files to
Tailscale's Taildrop instead of exposing a parallel transfer interface.

Subsequent upstream and Tailboard development may overlap. This notice is an
attribution record, not a claim that every modified line is unique to Tailboard.
