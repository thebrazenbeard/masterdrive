# Atomic checkpoint fingerprint floor: contract only

This change adds `VerifyAndCommitCheckpointFingerprint` and an injected `CheckpointFingerprintFloorStore`. The candidate checkpoint and read-only grant must pass source-key signature, identity, epoch, expiry and revision checks before the store can commit. At equal revision, a different canonical checkpoint fingerprint fails closed.

The store's commit method MUST perform the revision comparison and fingerprint comparison again under one atomic critical section, persist both values as one durable unit, and acknowledge success only after persistence. A stale concurrent writer or equal-revision fork must never be acknowledged. A commit error always denies admission, even if the disk outcome is ambiguous. An unprovisioned floor is not silently initialized from network evidence.

The included store is only a mutex-protected synthetic test double. It does not provide filesystem persistence, cross-process locking, power-loss recovery, rollback resistance, or an authenticated baseline. The older revision-only `VerifyAndCommitCheckpoint` remains present for compatibility and is NOT sufficient to detect equal-revision equivocation. Do not wire either prototype into a real mount or assume it authorizes subsequent reads.

## Hostile challenges / release holds

- A malicious peer can replay an old state directory. Local revision+hash persistence does not resist privileged rollback; an independently protected anchor is required.
- A correctly signed conflicting checkpoint does not reveal which version is current. Treat equivocation as a hard stop, not an automatic selection.
- A source key compromised after pairing can sign malicious evidence. Pairing, rotation and revocation authority remain unimplemented.
- Revocation immediately after admission still permits a race unless each read has independently enforced authorization or a rigorously bounded lease.
- Network deadlines must be enforced by the transport, not by a boolean or by checks between blocked reads.
- Windows and Linux disk semantics require independent crash/restart, directory-sync, ACL, atomic-replace, and concurrent-process qualification.

No real drives, Tailscale policies, mounts, network peers, or host processes are involved. The Go tests are synthetic source tests and must pass exact-head Windows/Linux CI before any merge consideration.
