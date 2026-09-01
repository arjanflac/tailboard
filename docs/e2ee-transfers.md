# Transfer E2EE Decision

> [!NOTE]
> Historical design record for the inherited transfer protocol. Native
> Tailboard apps now use Taildrop for photos/files; this proposal is not on the
> active app roadmap.

> [!NOTE]
> Historical design record for the inherited transfer protocol. Native
> Tailboard apps now use Taildrop for photos/files; this proposal is not on the
> active app roadmap.

Status: design accepted for a future protocol revision; implementation deferred

## Decision

If tg-clipboard adds application-layer end-to-end encryption, targeted transfers ship first. Clipboard broadcast remains unchanged until a separate multi-recipient/key-rotation design exists.

The current transfer protocol deliberately stays plaintext inside the trusted tailnet: the hub can read spool contents and direct-fetch metadata. The device registry already reserves `public_key`, but no client advertises an encryption capability and no UI claims encrypted delivery.

## Proposed `transfer_e2ee_v1`

1. Each install generates an X25519 identity keypair. The private key stays in the OS credential store (Keychain, DPAPI-backed storage, or Secret Service); registration publishes the encoded public key.
2. The sender generates a random 256-bit content key and nonce base for each transfer.
3. Each file is encrypted as independently authenticated, fixed-size chunks using XChaCha20-Poly1305. Chunk boundaries align with resumable upload/range offsets so retries do not require re-encrypting the full file.
4. The content key is wrapped for the target device with an HPKE profile based on its registered X25519 key. The hub stores only the wrapped key, ciphertext manifest, and ciphertext chunks.
5. The receiver unwraps locally, verifies every AEAD tag, reconstructs the plaintext, and finally verifies the existing declared plaintext SHA-256 before exposing the file.
6. Direct fetch serves the same ciphertext representation. Switching between spool and direct transport therefore does not change cryptographic state.

Negotiation is explicit through both hub feature `transfer_e2ee_v1` and device capability `transfer-e2ee-v1`. A sender must never silently downgrade an E2EE-requested transfer; it either encrypts for the selected target or reports that the target is incompatible.

## Metadata that remains visible

E2EE would hide file bytes and plaintext hashes from the hub. It would not hide sender/receiver device IDs, timing, transfer state, file count, ciphertext sizes, or expiry. Filenames should be moved inside the encrypted manifest; the hub can use opaque file indices.

## Key lifecycle requirements before implementation

- Detect public-key changes and require an explicit trust/reset confirmation before sending encrypted files.
- Define device removal and lost-device recovery. There is intentionally no server-side escrow.
- Store an identity-key fingerprint in transfer audit output so recipients can diagnose mismatches.
- Add deterministic test vectors shared by Go and Swift before enabling the feature bit.
- Complete an external cryptographic review. Do not design a custom key-wrap construction.

## Why this is deferred

Integrity, consent, path safety, tailnet transport encryption, and short-lived scoped direct tokens are implemented now. E2EE adds cross-platform secure-key storage, Swift/Go interoperability, rotation UX, and irreversible downgrade semantics. Shipping those pieces halfway would be worse than the current blunt trust statement. The reserved registry column and backward-compatible protocol fields keep this path open without making an unverified security promise.
