# Admission evidence consistency gate (unwired)

This isolated pure-Go predicate checks whether a supplied set of security evidence is internally consistent and fresh at a caller-provided time. It does not authenticate that evidence, acquire a trusted clock, authorize a share, inspect a filesystem, contact Tailscale, or mount anything.

The caller MUST independently authenticate peer pairing and key versions, obtain source-owned current grants and revocation epochs from rollback-resistant storage, establish confined immutable snapshots and independently authenticated pins, and enforce deadlines in the actual transport. Boolean evidence fields are assertions, not proofs. Never expose this predicate alone as a mount authorization decision.

The grant expiry comparison is strict (equal time is expired). Missing/zero digests, stale policy epochs, identity mismatches, altered snapshot metadata, absent read-only grant, and absent transport confinement/liveness assertions fail closed.

## Evidence

Synthetic isolated test: `GO111MODULE=off go test -count=1 -v` on Go 1.23.2/Linux passed locally for the valid fixture and sixteen negative cases, before the same source was committed. This is not Go 1.27 CI, Windows qualification, independent review, integration testing, or live-host verification.

## Hostile limitations

A malicious caller can set every Boolean to true and forge consistent fields. Current policy epoch is not proven durable or monotonic. Revocation may race after this check. The snapshot may mutate after checking. A transport can still block inside Read despite a Boolean claiming deadline enforcement. These are mandatory upstream authority and integration gaps, not solved by this package.

No real drives, credentials, Tailscale policy, mounts, Lappy processes, or paid services are involved.
