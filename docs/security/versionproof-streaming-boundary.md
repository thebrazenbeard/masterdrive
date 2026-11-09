# Streaming whole-file verification — source-only prototype

This stacked branch adds `VerifyStream` to `internal/versionproof`. It consumes at most the pinned size plus a one-byte EOF probe, computes SHA-256 incrementally, and rejects truncation, extra bytes, read errors, version mismatch, oversized policy claims and ambiguous EOF.

The pin MUST come from an independently authenticated source; `maxBytes` MUST be a local policy. A peer-supplied pin or limit is not sufficient. The EOF probe may block forever on a hostile reader without an externally enforced context/deadline; this function cannot impose transport deadlines. It is not suitable for unbounded streams or arbitrary partial-range integrity. No mount admission, pairing, grant, revocation, immutable snapshot, or tailnet policy integration is present.

Qualification requires exact-head Windows/Linux CI, independent hostile review, authenticated pin provenance, source-side immutable snapshots, and deadline/cancellation tests before integration. No real drives or host processes were used.
