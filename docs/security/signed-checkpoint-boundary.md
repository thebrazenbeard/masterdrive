# Signed grant checkpoint boundary (prototype; not wired)

This package verifies a source-signed, domain-separated Ed25519 checkpoint
alongside the previously signed read-only grant. The checkpoint binds source,
audience, share, signing-key version, grant epoch, revocation bit, revision,
issue time, and expiry. A caller-provided monotonic revision floor and bounded
age are required. It fails closed on absent floor, stale or expired checkpoints,
signature mismatch, revoked grants, and mismatched epochs.

## What it does NOT establish

- The public key is independently paired and protected.
- The revision floor is authenticated, persisted atomically, durable against
  rollback, or updated before a read begins. A peer-controlled floor defeats
  rollback detection. The caller MUST persist the maximum accepted revision
  before admitting a read; if persistence fails, deny.
- A malicious source signing key cannot forge an apparently current checkpoint.
- Wall-clock accuracy or bounded skew is established.
- Revocation cannot occur immediately after verification. Per-read fresh
  authorization or a separately reviewed bounded lease remains necessary.
- A file snapshot is immutable or its bytes are integrity-protected.
- TLS peer identity, Tailscale Serve configuration, transport deadlines,
  path confinement, or an actual mount is qualified.

This source is an isolated building block, not an authority provider.
It must not be wired into mount admission without end-to-end review.

## Qualification

Synthetic tests: `go test ./internal/grantproof` on Windows and Linux, then
`go test -race ./internal/grantproof` on supported Linux runners.
Require independent hostile review of rollback, replay, key rotation,
clock skew, revision persistence crashes, and TOCTOU. Use disposable fixtures.
No real drive exports, host launches, tailnet policy edits, or paid services.
