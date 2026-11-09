# Revocation checkpoint gate: source-only boundary

This package is a **pure comparison helper**, not a durable revocation ledger or a source of trusted identity. It is not wired to agent authorization, pairing, transport, or mount admission.

`VerifyExact(trusted, observed)` requires a nonempty matching store identity, a nonzero matching revision, and a matching 32-byte SHA-256 digest. It rejects a missing checkpoint, revision rollback or advance, and digest divergence. The digest must cover a deterministic domain-separated serialization of the **entire** ledger, including all revocation tombstones. The caller must obtain `trusted` from an independently authenticated, rollback-resistant authority, not from the same writable directory or the peer being checked.

## Unresolved requirements

1. Define a canonical ledger encoding and source-owned signature or MAC, with key rotation and replay controls.
2. Persist a monotonic trusted high-water mark outside the mutable ledger, with atomic durability and crash recovery on Windows and Linux.
3. Bind checkpoint identity and grant epoch to the actual authenticated Tailscale device and per-share grant.
4. Reject stale or missing evidence during every authorization decision, not only process startup; serialize reads with revocation updates.
5. Adversarially test deletion of a tombstone, restoration of an old complete ledger, partial writes, concurrent opens, and loss of the independent authority.
6. Require independent security review and exact-head Windows/Linux CI before integration or merge.

A local attacker who can rewrite both the ledger and its alleged trusted checkpoint can pass this comparison. A matching checkpoint cannot prove an immutable file snapshot, safe mount, or running mesh. No host action or policy change is authorized by this document.
