# G2 pairing and revocation: deliberately unintegrated source prototype

**Status: SOURCE / UNIT-TEST CANDIDATE, not a deployed G2 feature.** This document accompanies `internal/pairing/registry.go`. It is subject to the existing `docs/security/identity.md` trust boundary and is deliberately not wired to HTTP, the CLI, `tsnet`, shared disks, or the currently separate G1 grant environment configuration.

## Owner-approved state transitions

1. A trusted **local owner** chooses the exact controller Ed25519 public key and allowed share IDs. `IssueLocalInvite` creates a random 256-bit, single-use, at-most-five-minute invitation. Only its SHA-256 digest remains in the registry. Out-of-band delivery is the integrator's responsibility.
2. The intended controller signs the domain-separated invitation challenge with its private key. `RedeemInvite` atomically verifies and consumes the invitation, creating a **pending** pair with a fresh **five-minute confirmation deadline**. A failed signature does not consume a still-valid invitation.
3. The local owner independently verifies the **full** SHA-256 fingerprint of the controller key and explicitly calls `ConfirmLocal` before that deadline. Until then, *all reads fail closed*. A remote controller is not allowed to confirm itself. Expired pending pairs are removed when a local pairing operation next runs and cannot be confirmed afterward.
4. A confirmed controller signs each read operation over pair ID, permission epoch, operation, share, logical path, time and a 128-bit nonce. The agent checks the exact key, approved share, timestamp window, nonce reuse and the current permission epoch. **Only `list` and full-file `download` are modeled.** Search is explicitly rejected even when correctly signed; query text is not in the proof schema.
5. A trusted **local owner** calls `RevokeLocal`, which advances that pair's epoch, denies any *new* authorizations and signals cancellation to active `AuthorizeStream` leases. The consumer **must** stop streaming when its context is canceled and release each lease. A restarted in-memory registry forgets all pairs and denies new reads.

These methods are a low-level policy primitive. Local owner authority and independently verified key possession **are integration obligations**, not facts inferable from an arbitrary incoming HTTP request. There is intentionally no `POST /pair`, anonymous enrollment listener, remote confirmation, bearer-admin endpoint, or remotely callable `RevokeLocal`.

### Single key identity and bounded pending state

- **One key, one pair:** The registry rejects a controller public key already appearing in an unexpired invitation, pending pair, confirmed pair, or revoked pair tombstone. This prevents two independent PairIDs for one key from being separately revoked while the other remains active. To grant multiple shares, the owner selects their complete share set in one invitation (up to 64 share IDs); later permission changes require a designed local reauthorization/rotation workflow, not a parallel overlapping PairID.
- **Key rotation:** Revoking a pair retains a tombstone for its old public key, so the owner must invite a **new controller public key** to reconnect. The UI must clearly say this revokes the identity, not just one folder or session. The controller must prove possession of the new key and pass a fresh independent fingerprint confirmation. This registry does not implement an atomic old-to-new permission migration.
- **Bounds:** At most 128 unredeemed invitations, 128 unconfirmed pending pairs, and 1,024 total pair records (confirmed plus revoked tombstones included). Expired invitations and pending pairs are garbage-collected on local pairing calls, reducing abandoned pending-state exhaustion. Revoked tombstones are intentionally **not** garbage-collected; reaching the total cap fails closed pending a durable migration or owner-authorized administrative lifecycle.
- **Existing data:** This prototype has no persistence or upgrade path. If future storage already contains duplicate PairIDs sharing the same public key, do not load it without reconciliation. A production revocation path must then fan out across all such identities or reject that data; the empty in-memory registry's uniqueness invariant does not retroactively fix an existing deployment.

## Request-side security gates required at integration

- Authenticate the **actual inbound peer** using the chosen qualified Tailscale transport and bind it to the pinned controller identity. A signed proof cannot by itself establish that a request arrived from the intended network node. Prefer `tsnet.Server.LocalClient().WhoIs(r.Context(), r.RemoteAddr)` where supported and proven; reject unknown, tagged, inconsistent, or unmapped identities according to the selected policy. Never assume raw client-supplied `Tailscale-User-*` headers prove the caller.
- Invoke `Authorize` **on every supported** file request, before file open. Canonicalize the precise operation, share ID and logical path used by the handler *before* verifying the signed proof. The signed fields and the resource actually accessed must match exactly. `os.Root` or equivalent real filesystem confinement remains required; signature validation is not path confinement.
- **Fail closed on unsigned request semantics:** This proof does **not** bind HTTP search query terms, `Range`/`If-Range` headers, sort/pagination/filter parameters, or other response-changing URL query and header fields. Do not authorize search or partial/conditional downloads using it. The handler must reject those variants, including unsigned search parameters appended to an otherwise valid listing request. A later `search` or ranged-download protocol must sign and verify a canonical intent including all applicable semantic parameters; updating an HTTP route alone is insufficient.
- Resolve owner shares from trusted local configuration, not controller-provided directories. Do not allow mutating actions via signed read proofs. Reject downloads whose token authorizes a different path; do not let frontend/cache/search paths bypass checks.
- Secure invite delivery, private-key storage, operator confirmation and controller-key rotation. A public key must be provisioned through a trusted local action. If a key is lost, create a **new pair** after local approval rather than silently replacing an existing key.
- Preserve an owner-controlled audit log of invitations, confirmations, failed authorizations and revocations without logging invitation secrets, private keys or bearer tokens.

## Proof and failure properties of this code

| Claim | Evidence level | Important limit |
| --- | --- | --- |
| Uninvited remote controller cannot create an active pair with this library | Source structure; intended negative unit tests | Local method call authority must still be isolated |
| Wrong key fails redemption, pending fails authorization, invite is consumed once | Unit tests supplied, **not executed in this environment** | DoS or UI social engineering outside scope |
| Pending confirmation expires after five minutes and abandoned records are bounded | Source plus intended expiry/cap tests | Lazy cleanup runs on local enrollment/confirmation actions, not a background timer |
| Same controller key cannot hold multiple independently revocable PairIDs | Source uniqueness scan plus intended negative tests | No persisted migration; UI must grant all chosen shares under one key |
| Revoked tombstones count toward the 1,024-record ceiling | Source and synthetic-boundary regression test | No durable administrative tombstone lifecycle; reaching cap denies enrollment |
| Only signed listing and full-file download intents are permitted | Source and signed-search rejection regression test | HTTP handler must reject unsigned query/Range variants; no middleware is wired |
| Signed read proofs are share/operation/path/epoch bound | Source structure; intended unit tests | Transport/node binding and handler path matching not implemented |
| Same nonce cannot be accepted twice in its 30-second timestamp-valid window | Source; test covers future-dated replay after 35 seconds | Each process holds an ephemeral cache only; multi-process use unsupported |
| Revocation fences new authorizations and cancels tracked stream contexts in this process | Source under a mutex; intended unit tests | Does **not** forcibly close an already running socket/write or revoke Tailscale membership; use `AuthorizeStream` for tracked downloads |
| Authorization survives restart and replicated agents | **False / not implemented** | Restart loses pairs and denies access; durable signed/audited state and cross-process epochs are required |

The nonce cache is bounded (1,024 unexpired entries per pair). Overflow fails closed, which may impact high-throughput workloads; do not mistake it for a production-sized admission policy. The prototype uses Ed25519 signatures with no plaintext private key maintained by the registry.

### Material hostile challenges

> **FAILS as immediate byte-level revocation:** `RevokeLocal` signals cancellation to registered stream leases, but can return while an earlier request still writes bytes. The existing HTTP handler must honor the lease context and pass interruption tests before claiming stream abortion. The one-shot `Authorize` method does not track a stream. Go's `http.Server.Shutdown` does not forcibly interrupt all active connections.

> **FAILS as transport identity:** An Ed25519 proof binds the controller key but not its current Tailscale node. A stolen key could be used from another node unless network identity is independently pinned and verified per request. Serve-provided identity headers have documented caveats, especially tagged callers and local backends.

> **UNKNOWN as durable permission control:** Persisted pairs need crash-safe, tamper-resistant owner custody, monotonic permission epochs, safe restore/backup semantics and tests across daemon restarts. Do not copy this in-memory registry into a distributed service without durable replay/revocation coordination.

> **REVIEW — UX/semantic risk:** One-key/one-pair prevents ambiguous revocation, but revoking a key now removes all its shares and requires a new key and explicit owner confirmation. A UI that labels this action "remove one folder" would be incorrect and potentially disruptive. Explicit per-share revocation needs a separate design preserving the single identity and monotonic permission epochs.

> **FAILS for unmodeled intent:** A valid signature on `download` says nothing about an HTTP `Range` or `If-Range` header; a signature on `list` says nothing about additional search/filter parameters. No HTTP adapter is present to reject these today. Enabling such routes without new canonical signed fields would misstate the authorization evidence.

## Production admission gates

1. Integrate explicit owner-local enrollment UI/CLI and secure controller public-key provisioning. No remote auto-approval or self-enrollment.
2. Qualify inbound Tailscale node identity; inspect Serve, `tsnet.WhoIs`, tagged devices, and localhost spoofing on Windows and Linux with live two-device tests.
3. Run `go test -race ./internal/pairing` and negative integration tests of every file route; the supplied unit tests are source-only until executed in an actual Go environment.
4. Add durable atomic revocation/epoch custody and enforce it across all serving processes and restarts. Rotate/reissue credentials after revocation; don't reuse a revoked pair ID.
5. Wire download streams through `AuthorizeStream` and test stop-on-cancel and hard interruption of blocked writes. Until independently tested, describe revocation as **new-read fencing plus cooperative cancellation signaling**, not guaranteed cessation of bytes.
6. Test rejected owner operations against unauthenticated and self-enrolled peers, including wrong keys, replay, stolen invite, controller key replacement, stale epochs, path tampering, and process crash/restart.
7. Test pending TTL, 128/1,024 capacity limits, expiry cleanup, single-key uniqueness and revoked-key rotation on both supported platforms. Decide the production policy for per-share removal, historical duplicate-key migration and full-record tombstone retention.
8. Before any search or Range support, design a versioned canonical request envelope binding query/pagination or byte offsets and content identity as appropriate, then negative-test tampering and signature replay across semantic variants.

## Primary technical sources

- [Tailscale Serve documentation](https://tailscale.com/docs/features/tailscale-serve) — Serve identity headers, tagged-peer caveats, and local-backend trust boundary.
- [Tailscale `tsnet` API](https://pkg.go.dev/tailscale.com/tsnet) and [tsnet server API reference](https://tailscale.com/docs/reference/tsnet-server-api) — embedded tailnet server, local client and `WhoIs` support.
- [Tailscale identity concepts](https://tailscale.com/docs/concepts/tailscale-identity) — authenticated tailnet device and user identity.
- [Go `crypto/ed25519`](https://pkg.go.dev/crypto/ed25519) — signature generation and verification.
- [Go `net/http`](https://pkg.go.dev/net/http) — request cancellation and server shutdown behavior.

No external code was copied; the registry and tests are original source for this project.
