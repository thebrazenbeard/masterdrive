# Version proof: complete-file integrity gate (source-only)

`internal/versionproof` compares an independently authenticated complete-file SHA-256/size/version pin with received bytes. Its range check only rejects invalid arithmetic; it cannot validate partial content against a complete-file digest. No mount or transport integration is present.

Do not trust a pin supplied in the same response as the bytes. A compromised peer could substitute both. Before mounting or reading ranges, require independently authenticated source-owned grant and peer identity, revocation high-water evidence, an immutable snapshot/handle, bounded transfer, and per-range integrity proofs. An ETag is not a cryptographic digest. This branch does not provide these prerequisites or any real host qualification.

Local isolated Go package tests are not Windows/Linux matrix CI. No real drive, ACL, Tailscale configuration, or host UI changes are required for these tests.
