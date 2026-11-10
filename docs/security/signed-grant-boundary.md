# Source-signed read-only grant prototype (unwired)

`internal/grantproof` verifies Ed25519 signatures over domain-separated, length-delimited grant fields: source identity, audience identity, share identity, snapshot identity, source key version, grant epoch, validity interval, and read-only capability. It compares the caller's expected audience/share/snapshot and a separately pinned source public key. Expiration uses an externally trusted clock. It rejects missing fields and any non-read-only grant.

This package **does not** create trusted keys, authenticate initial pairing, persist grants, establish a revocation authority, check whether an epoch is current, protect against replay of an old still-valid grant, or authenticate the snapshot digest. It does not implement a mount or network endpoint. A compromised source private key can sign fraudulent grants. Time-of-check/time-of-use revocation races remain unresolved. The caller must not interpret a passing signature check as mount authorization.

Admission must separately require an independently authenticated and rollback-resistant source key and revocation checkpoint, source-owned path confinement, immutable version and digest provenance, transport deadlines, and per-read reauthorization or a formally bounded lease policy. Tailscale connectivity alone is not proof of these properties.

Tests use deterministic synthetic keys only. The standalone package was exercised with Go 1.23.2 on Linux in GOPATH mode; repository-targeted Go 1.27 Windows/Linux CI and independent review are required. No real drives, credentials, hosts, or Tailscale policy are involved.
