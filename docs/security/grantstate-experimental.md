# Source-local grant decision seam (experimental)

Evidence scope: source and Windows Go tests only. This package is deliberately **not wired to HTTP handlers**, not a pairing protocol, not a durable grant database, and not a mount authority.

`internal/grantstate` models unique grant IDs, pending owner approval, active grants, irreversible in-process revocation, expiry, exact share and consumer-key fingerprint matching, monotonically increasing in-process epochs, and fail-closed unavailability. Approval is an explicit method intended for a future independently authenticated owner-consent boundary. Fingerprints are opaque strings; the package does **not** verify cryptographic possession. Re-pairing requires a fresh grant ID. Any mutation or read decision is serialized by a mutex.

## Non-negotiable limitations

* Restarting creates a fresh empty store. No persistence, trusted clock, inter-process coordination, authenticated handshake, protected key storage, rollback resistance or crash-safe revocation acknowledgment exists. **Never use this package as a production authorization authority.**
* `Revoke` changes process memory only. It cannot honestly acknowledge durable revocation; integration must supply durable source-owned commit and independently anchored monotonic state before responding success.
* `Authorize` returns a point-in-time decision, not a lease. A concurrent revoke may happen immediately after success. A real reader must enforce revocation and pinned object-version checks at the source before each bounded range, and discard staged bytes if authorization changes before publication.
* `SetAvailable` is a testable simulation of missing authority, not an actual health or trust probe. This implementation does not authenticate Tailscale identity or alter any tailnet policy.
* A stale epoch is denied, including when an unrelated grant is approved or revoked. This conservative global epoch policy can cause availability churn and should be challenged before network integration.

## Reproducible checks

`go test -count=1 ./internal/grantstate`; `go test -count=1 ./...`; `go vet ./...`. The tests cover pending denial, approval, identity/share mismatch, stale epoch, expiry, unavailability, revocation and non-reuse, and concurrent revocation. They do **not** cover persistent crash recovery, Windows reparse paths, Linux symlinks, cryptographic pairing, or mount behavior.

Design context: draft PR #33 `docs/security/pairing-revocation-authority-contract.md`. This implementation is an independently based mainline experiment, not a claim that PR #33 was merged.

## In-process clock rollback latch (experimental)

The grant state machine now observes wall-clock time under its store mutex and
latches an unavailable state if a subsequent observation goes backwards. This
prevents a previously expired grant from becoming eligible again within that
process following an observed clock rollback. A later clock correction does not
clear the latch. The regression TestClockRollbackPoisonsGrantAuthority
exercises both denial and persistence of the latch.

This is deliberately conservative: legitimate NTP/manual clock corrections can
disable this experimental authority until process replacement. It is NOT
a trusted time source, does not survive restart, does not prove an attacker
cannot roll back the clock before observation, and does not protect persisted
grant state. It must not be used to authorize real shares or mounts. Production
design still requires authenticated owner consent, durable monotonic authority,
peer identity binding, version-pinned source reads, and crash/restart testing.
