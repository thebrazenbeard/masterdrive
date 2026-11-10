# Coordinated historical rollback counterexample

Evidence class: **source regression**, not protected authority qualification.

`TestAnchoredReadFloorAcceptsCoordinatedHistoricalRollback` provisions two independent **unprotected** filesystem journals with the same relationship at revision 1, advances both to revision 2, then restores both complete revision-1 byte snapshots. Reopening the double-snapshot gate returns revision 1 without an error. This is intentionally a *passing vulnerability demonstration*, not an assertion that rollback should be permitted.

The test isolates its fixtures under `t.TempDir()`. It does not access real drives, mount devices, invoke Tailscale, or exercise Windows host UI.

## Security interpretation

Two distinct files are not independent *trust domains* when the same adversary can replace both. Checksums, repeated reads, sidecar locks, and exact fingerprint agreement cannot establish freshness in that threat model. The authority needs independently protected monotonic state, authenticated provisioning, explicit revocation semantics, and fail-closed behavior on missing/contradictory authority. Do not wire this prototype to actual read admission or mounts.

## Remaining qualification

- Confirm exact-head CI on Windows and Linux; exercise crash/restart and cross-process fault injection.
- Hostile-review filesystem ownership, Windows reparse points, Linux handle-relative opening, and lock semantics.
- Implement an authenticated, independently protected authority, then replace this counterexample's expected acceptance with rejection under the protected implementation.
- Integrate pairing, per-read revocation and version-safe mount preconditions only after the authority is qualified.

No real host or tailnet authorization is implied by this source-only regression.
