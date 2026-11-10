# Namespaced in-memory checkpoint floor: reference implementation only

Evidence: source implementation plus committed tests; not durable storage or production qualification.

`MemoryFingerprintFloors` implements `CheckpointFingerprintFloorStore` with exact three-field relationship keys and one mutex protecting lookup and compare/commit. Provisioning is explicit and cannot reset existing state. Missing identities, unknown relationships, revision rollback, and same-revision fingerprint conflicts fail closed. This directly contrasts with the intentionally defective aliasing backend in PR #76.

**Security hold:** memory is lost on restart. An attacker or crash can reset the floor, so this implementation MUST NOT be wired into real read admission, mount preconditions, or device pairing. `Provision` is not an authenticated operator enrollment protocol. A process-local mutex does not protect other processes. No disk fsync, cross-process lock, reparse/symlink confinement, authenticated identity, freshness lease, or revocation-to-read cancellation is provided.

The committed tests exercise relationship isolation, reprovision refusal, stale and equivocal commits, and concurrent revision advancement. Passing these tests would not prove durability. Require a protected durable store with authenticated provisioning, atomic compare+commit, restart and power-loss fault injection, Windows/Linux CI and independent hostile review before production wiring.
