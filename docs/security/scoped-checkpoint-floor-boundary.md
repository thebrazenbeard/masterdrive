# Scoped checkpoint floor: source-only guard

The `ScopedCheckpointFloor` adapter pins an injected `CheckpointFingerprintFloorStore` to an explicitly provisioned source/audience/share triple. Mismatched or missing identity fields fail closed **before** calling the backing store. Zero-revision commits and ambiguous backing-store failures also deny admission. The adapter preserves the explicit equivocation error.

This is a defense against accidental cross-relationship store-handle reuse. It does **not** establish authenticated pairing, prevent malicious callers from creating their own adapter, or protect a backing store that incorrectly aliases different relationships. The backing store must independently partition and authorize each relationship, atomically compare revision+fingerprint, and durably commit before acknowledging. No persistent implementation is introduced here.

## Review holds

- The adapter is not wired into a mount or read path. Older admission APIs remain available.
- A caller may construct an adapter with false identities. Provisioning authority and source key provenance remain unimplemented.
- A malicious local process or rollback of persistent floor state remains out of scope.
- Revocation after admission still requires a per-read check or rigorously bounded lease.
- Synthetic tests cannot establish Windows/Linux filesystem semantics or multi-process safety.
- Exact-head CI, independent hostile review, and a durable store are required before merge.

No drives, network policies, hosts, mounts or credentials are accessed.
