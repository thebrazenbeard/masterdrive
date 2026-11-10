# Current grant epoch consistency gate (unwired)

`VerifyCurrent` combines source-signed grant verification with exact comparison to a caller-provided current source/audience/share/key-version/grant-epoch record. It rejects revoked relationships, stale epochs, rotated keys, missing fields, bad signatures and expired grants.

**Critical trust boundary:** `CurrentAuthority` is a data structure, not a trusted authority. A malicious caller can forge a consistent record or replay an old record. The code does not fetch, authenticate, durably store or roll back-proof checkpoints; it does not prevent revocation between checks. A production caller must independently authenticate a monotonic checkpoint and enforce per-read freshness or a formally bounded lease, with transport deadlines and immutable snapshot proofs. This is not mount authorization.

Synthetic unit tests exercise invalid epochs, key rotations, revocation, identity mismatches and signatures. No host/drive/tailnet interaction is performed.
