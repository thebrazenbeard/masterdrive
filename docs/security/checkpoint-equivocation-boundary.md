# Equal-revision checkpoint equivocation boundary

An authenticated source can sign two different checkpoint messages for the same source/audience/share and revision. A revision-only floor cannot distinguish these messages. In particular, an older non-revoked checkpoint may remain usable after a conflicting revoked checkpoint is issued at the same revision.

`CheckpointFingerprint` computes SHA-256 over the canonical, domain-separated checkpoint message. `RejectSameRevisionFork` rejects differing fingerprints at the same relationship and revision. This is a **pure unwired predicate**, not an admission fix.

## Required before using this for authorization

- Authenticate the signing key through explicit pairing and rotation rules.
- Provision a trusted checkpoint baseline and persist **revision plus fingerprint** atomically and durably, scoped to the source/audience/share.
- Reject same-revision forks even if both signatures verify; do not silently choose the non-revoked message.
- Ensure the source increments revisions for every authority change, including revocation, and recheck authorization throughout reads.
- Test crash/restart, concurrent processes, rollback of the entire state directory, and Windows/Linux filesystem semantics. Local disk alone does not establish rollback resistance against a privileged attacker.
- Establish bounded network deadlines and immutable source-owned snapshot provenance before mount admission.

No actual store, network read, pairing, mount, Tailscale policy, or host execution is provided here. The synthetic tests exercise only this comparison contract.
