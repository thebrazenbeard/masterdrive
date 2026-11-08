# Read-only mesh transport boundary (experimental)

Source: `internal/readtransport/policy.go` on this branch, based on main `d82e69f8c9c82e846bd9b118c7ffa8dc9ef2025b`.

`NewRequest` requires an explicit exact HTTPS authority, rejects embedded credentials, URL queries/fragments, and creates a context-bound GET. `NewClient` disables redirect following, environment proxies, cookies, compression and keep-alives, and applies finite dial/TLS/header/total timeouts. These are **unwired primitives**, not a secure agent session or a mount.

**Critical limitation:** the client itself does not restrict destinations; callers must use `NewRequest` and must not pass untrusted URLs to `Client.Do` or `Client.Get`. Exact authority matching does not authenticate a device or prove that its DNS resolution/TLS identity is authorized. Host-owned grants, certificate validation, Tailscale peer identity, immutable snapshot provenance, per-range validation, bounded streaming, and digest verification remain separate mandatory gates. Never trust a peer-provided digest as independent evidence.

No real drive exports, host operations, tailnet ACL changes, or live mount qualification are implied. Synthetic tests cover malformed/misdirected endpoints, redirect suppression, and transport configuration. CI must qualify the exact PR head on Windows and Linux; independently hostile-review before merging. The current `main` branch is not protected by a required-check rule (GitHub branch metadata, 2026-11-08), so a green check alone does not authorize merge.
