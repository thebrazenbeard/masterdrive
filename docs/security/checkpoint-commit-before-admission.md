# Checkpoint commit-before-admission boundary (unwired)

`VerifyAndCommitCheckpoint` requires an injected `CheckpointFloorStore`.
It rejects an absent or unprovisioned floor, verifies the source-signed
checkpoint and grant against the floor, and requires `CommitAtLeast` to
succeed before returning nil. The floor must be provisioned through a
separately authenticated process. It must never be supplied by the peer.

**The interface is a contract, not a durable store.** Implementations must
atomically and durably persist the maximum revision (including across
crashes, competing processes, and power loss), never decrease it, and
bind its namespace to independently paired identities. A store that
acknowledges writes before fsync or that can be rolled back by restoring
an older disk image is not qualified. The synthetic fake in tests
establishes control flow only, not crash safety.

A revision floor cannot prevent replay of a previously signed checkpoint
at the *same* revision; bounded age limits exposure but does not make
revocation instantaneous. Verification followed by a read is subject to
TOCTOU. No network, filesystem, mount, real drive, or tailnet integration
is provided. Clock skew, key compromise, key rotation, immutable snapshot
provenance, transport deadlines and per-read authority remain blockers.

Qualification: Go 1.27 Windows/Linux exact-head CI; independent hostile
review; separately reviewed durable-store implementation with fault
injection, crash/restart, concurrency, rollback and permission tests.
No real drives, host UI interaction or Tailscale policy changes.
