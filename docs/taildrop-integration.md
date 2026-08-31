# Taildrop integration direction

Status: proposed migration; the existing Tailboard transfer protocol remains
enabled until the mobile handoff flows have been verified on real devices.

## Decision

Tailboard should own clipboard synchronization and its native clipboard UI.
Tailscale Taildrop should own photos and arbitrary file transfer whenever the
Tailscale app is already installed.

This removes Tailboard's duplicate multi-gigabyte transfer spool, upload state
machine, integrity checks, and per-platform receiving code. Taildrop already
uses encrypted peer-to-peer Tailscale connections and the fastest available
path, with no third-party file host.

Tailcat is not the right transport for this product. Tailcat is a netcat-style
data plane for machines that do not share a Tailscale account or control plane.
Tailboard's devices already belong to one persistent tailnet, so embedding
Tailcat would add key exchange, discovery, retry, relay, and lifecycle work
without adding a user-facing capability.

## Platform handoff

### iPhone and iPad

The current Tailscale app publishes a `Send File` App Intent with two required
inputs: `Files` and `Destination`. A user Shortcut can therefore accept files
from the share sheet and pin `Destination` to the Mac. This is the cleanest
one-tap replacement for Tailboard's iOS share extension.

Tailboard cannot directly invoke another app's App Intent as a private API.
The supported boundary is the user-owned Shortcut. Tailboard can explain how
to create it and detect whether the legacy transfer feature is still enabled,
but it should not attempt to automate the Tailscale app UI.

### Android

Tailscale exports `ShareActivity` for `ACTION_SEND` and
`ACTION_SEND_MULTIPLE`, so Tailboard can forward Android content to the native
Taildrop screen or users can choose Tailscale directly in the system share
sheet.

The current activity accepts file URIs only. It does not define a supported
intent extra for a default destination, and its view model always presents the
eligible peer list. Tailboard therefore cannot safely provide a silent
default-Mac send on Android using the public integration surface.

The first-class fix is an upstream Tailscale Android preference or documented
intent parameter for a default Taildrop destination. Until that exists, use
the native target picker rather than relying on accessibility automation or a
fork of the Tailscale client.

### macOS

For Finder and drag-and-drop sends, Tailboard can call the installed, supported
CLI surface:

```sh
tailscale file cp <files...> <target>:
```

The target list is available through `tailscale file cp --targets`. Incoming
files are then handled by Tailscale's normal Taildrop receiver and Downloads
folder behavior, not Tailboard's embedded hub spool.

## Migration plan

1. Verify an iOS share-sheet Shortcut with the Mac fixed as its destination.
2. Verify the Tailscale share target on the Pixel and decide whether its one
   target-selection tap is acceptable.
3. Add a Tailboard setting that labels file transport as `Taildrop` or
   `Legacy`, with Taildrop recommended and new installs defaulting to it.
4. Stop accepting new legacy file transfers after all three devices use the
   Taildrop path. Keep existing spool metadata readable until pending transfers
   finish or expire.
5. Remove the legacy transfer server, mobile upload clients, and spool only in
   a later compatibility-breaking release.

Clipboard images remain clipboard items and continue through Tailboard. Photos
or files intentionally shared from Photos, Files, Finder, or an Android share
sheet go through Taildrop.

## Primary references

- [Taildrop documentation](https://tailscale.com/docs/features/taildrop)
- [Tailscale Android share activity](https://github.com/tailscale/tailscale-android/blob/main/android/src/main/java/com/tailscale/ipn/ShareActivity.kt)
- [Tailscale Android Taildrop view model](https://github.com/tailscale/tailscale-android/blob/main/android/src/main/java/com/tailscale/ipn/ui/viewModel/TaildropViewModel.kt)
- [Tailcat announcement](https://tailscale.com/blog/tailcat)
