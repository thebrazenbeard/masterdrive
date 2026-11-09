# Canonical grant-ledger digest prototype (unwired)

Source: `internal/checkpointgate/grant_ledger.go` on draft PR #60. This is **not** a trusted checkpoint, a pairing protocol, or a mount admission mechanism.

`DigestGrantLedger` uses a domain-separated SHA-256 encoding with big-endian 64-bit length prefixes, a store identity, nonzero revision, sorted unique active grant records (grant ID, peer ID, share ID, epoch), and sorted unique revoked-grant tombstones (ID, epoch). It rejects missing fields, zero epochs, duplicates, oversize fields, and an ID present in both active and revoked sets. Sorting uses copies; caller input is unchanged.

## Hostile limitations

* The schema intentionally omits expiry, capabilities, device key material, key rotation, signature algorithm/version, principal identity, and policy provenance. Adding any such field **must** change the domain/version and update the tests. An omitted authorization field can change without affecting this digest.
* PeerID is opaque and is **not** verified against a live Tailscale node, device key, tailnet, or owner-approved share.
* A digest is not a signature, MAC, monotonic counter, or proof of durable atomic persistence. An attacker controlling both ledger and checkpoint can replace both.
* Neither this function nor the earlier tombstone-only `DigestRevocations` may be used to authorize reads, pairing, mounts, or revocation.
* The prototype rejects simultaneous active and revoked entries sharing an ID. A future schema must explicitly model grant generations and historical tombstones rather than silently dropping history.
* Even a complete signed ledger cannot alone prove an immutable source file version or correct HTTP range response.

Before integration: source-owned schema and threat model; independently authenticated checkpoint authority; rollback-resistant high-water mark; crash-recovery tests on Windows and Linux; identity/grant binding; concurrent revocation/read linearization; exact-head CI; independent security review. No real drive, policy, foreground UI, or paid host operation is authorized by this document.
