# Identity, pairing, authorization, and revocation design

Status: **proposed architecture, not implemented or live qualified**. Research date: 2026-11-02. The current MasterDrive prototype has a loopback agent, a single process-start bearer secret, a controller env-token map, and a read-only browser UI. Its current authentication is **not** a device pairing or revocation system.

## Decision

Use Tailscale as an **optional private transport and network identity provider**, while keeping MasterDrive's **separate, explicit, per-share permission and pairing** system. Implement this in stages:

1. **Transport beta:** preserve the loopback-only MasterDrive agent and access it through private Tailscale Serve HTTPS. Require Tailscale grants **and** the existing high-entropy bearer secret; document manual out-of-band token configuration as a development bridge, not as completed pairing. No Funnel or public listener.
2. **Pairing milestone:** replace one global agent token and the controller's static JSON env map with persistent controller/agent key identities, an owner-approved invitation, per-pair/per-share grants, durable credential storage and immediate local revocation. Build a protocol with a scripted end-to-end test before exposing UI pairing.
3. **Identity-aware milestone:** bind a pair to verified Tailscale node identity (prefer an embedded `tsnet` node with `WhoIs` for a dedicated program identity; alternatively use Serve-forwarded application capabilities on a sufficiently recent daemon **plus** a separate pair credential). Deny all claims if the identity provider is unavailable, inconsistent, or revoked; retain MasterDrive's own share decisions.

Tailscale network authorization alone is insufficient: membership in a tailnet, a matching user email, or access to a host HTTPS port does not automatically entitle a caller to exported disk contents. Taildrive's policy `tailscale.com/cap/drive` governs Taildrive itself, **not** a custom MasterDrive export. [Tailscale identity](https://tailscale.com/docs/concepts/tailscale-identity), [grants](https://tailscale.com/docs/reference/syntax/grants), [Taildrive](https://tailscale.com/docs/features/taildrive).

## Trust boundaries

| Boundary | Authority | What MasterDrive must verify |
| --- | --- | --- |
| Local browser → controller | Current localhost-only session fragment converted to HttpOnly cookie | A browser session, no peer secrets in client JavaScript; browser cannot add export roots or change pair permissions |
| Controller → Tailscale → agent | Tailnet route/TLS, grants and the receiving node's authenticated Tailscale identity | Exact expected device or tag, HTTPS-only remote endpoint, no redirect following, and a separate approved MasterDrive pair |
| Serve reverse proxy → agent | Tailscale's forwarded headers, from a proxy on the **same device** | The backend listens only on loopback and must not treat arbitrary client-provided `Tailscale-User-*` as authentic unless it can establish proxy provenance; independent per-pair authorization remains required |
| Agent → disk | Local device owner's opt-in export, pinned `os.Root` | Share membership, read-only operations, path confinement, quotas and source-change handling; a Tailscale grant must not add exports |

`Tailscale-User-Login` is **absent for tagged callers**. It is also not a unique *device* identifier for a person with multiple nodes. Serve strips incoming identity headers and sets trusted ones on proxied tailnet traffic, but its official threat model warns that another **local process** can spoof headers to a loopback backend. Never interpret the presence of `Tailscale-User-Login` as sufficient by itself. [Serve identity documentation](https://tailscale.com/docs/features/tailscale-serve).

For a server tagged as `tag:masterdrive-agent` and a dedicated controller tagged `tag:masterdrive-controller`, custom app capability forwarding through Serve requires Tailscale **v1.92+** and an explicit `--accept-app-caps` configuration. The backend must parse the capability as untrusted input until it verifies the request came through its trusted local proxy, and still independently check an approved pair. A fallback to *allow* when the header is missing is forbidden. [Serve capabilities](https://tailscale.com/docs/features/tailscale-serve), [app capability integration](https://tailscale.com/docs/features/access-control/grants/grants-app-capabilities).

## Proposed pair state machine

| State | Action | Persistent effect |
| --- | --- | --- |
| `unpaired` | Device owner explicitly opts in a directory; its path stays local | Export handle can be opened, but no remote peer can enumerate/download |
| `invited` | Owner starts local pair wizard, selects share IDs, controller identity and short expiration | One-use, cryptographically random invitation secret, stored hashed; TTL ≤ 5 minutes |
| `pending-confirmation` | Controller proves possession of its private key over HTTPS and uses the invitation secret; both ends display the same fingerprint/short authentication string | No file access yet; invite is consumed atomically, replay denied |
| `paired` | Owner confirms on the agent's **local** UI/CLI; controller confirms fingerprint | Pair record maps immutable controller public key and expected Tailscale identity to named shares and `read` only |
| `suspended` | Expected tailnet identity, endpoint cert, grant, credential or policy is unavailable/changed | Agent returns denial without file content; records remain for recovery |
| `revoked` | Owner revokes locally, removes export, rotates key, or deletes pair | Secret version and permission epoch increment atomically; in-flight requests cancelled best-effort; future requests denied immediately |

The invitation must never be a guessable six-digit code accepted as sole proof. Generate at least 256 bits of entropy and deliver out of band to the intended controller while presenting a human-verifiable fingerprint. Bind the invite to the proposed controller public key and its approved share scope. If a browser-based wizard is added later, its one-time invitation material must not appear in query strings, referrer URLs, logs or telemetry. One attempt per short time window, rate limit, and audit rejected attempts without logging secrets.

### V1 local record shape (illustrative)

```json
{
  "pair_id": "opaque-random-id",
  "controller_pubkey_ed25519": "base64-public-key",
  "tailscale_node_id": "verified-node-id-or-null-until-qualified",
  "shares": ["documents"],
  "capabilities": ["list", "read"],
  "credential_version": 3,
  "permission_epoch": 9,
  "status": "paired",
  "expires_at": null
}
```

This is a **schema sketch**—not a claim that those fields exist in current code. Never persist a raw invitation, pairing token or private key in this JSON record. On Windows use a narrowly scoped Windows credential vault/DPAPI-backed implementation with documented service-account ownership; on Linux use a protected secret store or a permission-checked (`0600`, owned) file under a private data directory. Prove backup/restore semantics and key rotation before treating a storage backend as secure. Do not rely on cross-platform filesystem mode bits alone for Windows. These storage choices are engineering recommendations requiring implementation review.

## Authorization request path

1. Controller connects only to the manually registered HTTPS peer (not arbitrary browser-provided URL), through a deliberately configured private Serve route. Disable HTTP proxy environment overrides for an identity-sensitive peer transport unless the proxy is explicitly trusted. Never follow redirects, including from a compromised peer.
2. Agent establishes the caller's Tailscale identity: if using `tsnet`, resolve `WhoIs` on the actual inbound connection, not a forwarded user-selected IP; if using Serve, require a trusted local proxy boundary, validated app capability headers on supported versions, and the independently held pair credential. Reject empty/mismatched identities and tagged/user mismatch.
3. Agent checks pair record status, controller key fingerprint, expected node/tag, expiry, current permission epoch, share allowlist, method and `read` scope **on every operation**, including metadata. Do not allow a valid pair for one share to access another.
4. Agent keeps the `os.Root` confinement and read-only GET surface. No operator permission prompt or auth downgrade occurs during a network request. Listing and transfers have bounded resource consumption.
5. On a failed policy check return a generic denial with a machine-readable reason to the trusted controller; do not disclose which directories or pair IDs exist to unauthenticated callers. Never surface bearer tokens or private keys in browser data.

Request signing with Ed25519 (method + canonical path/query + timestamp + random nonce + pair ID + body hash) is one way to prevent replay after the initial out-of-band approval, but its implementation must specify a canonicalization format and a bounded replay cache. An alternative is mutually authenticated TLS with pinned client certificate and a separate share policy. **Do not combine half-implemented variants.** A protocol review must settle one concrete scheme before coding.

## Revocation: two independent fences

- **MasterDrive local fence:** modifying an agent's pair database atomically increments `permission_epoch` and invalidates its credential/key immediately. A new GET, list, range continuation or status request must fail. Cancel or truncate in-flight streams on revocation if implementation can prove it; otherwise document that already-open streams may continue until termination. Never claim immediate cessation without a test.
- **Tailnet fence:** remove the applicable Tailscale grant, change tag assignment, or deauthorize/delete the node as an operator action. Because clients cache distributed policy, treat propagation as a defense-in-depth control with measurable delay; do not depend on it as the only instantaneous revocation. [Grants policy compilation/caching](https://tailscale.com/docs/reference/syntax/grants).
- **Recovery:** rotate a leaked pair credential or key; do not silently re-authorize a new node with the same display name or MagicDNS hostname. If a node is reinstalled or loses its `tsnet.Server.Dir`, treat it as a new identity that requires an explicit new approval. [tsnet persistent state](https://tailscale.com/docs/features/tsnet/how-to/create-basic-tsnet-app).

## Illustrative tailnet policy (NOT a copy/paste production default)

```json
{
  "tagOwners": {
    "tag:masterdrive-controller": ["autogroup:admin"],
    "tag:masterdrive-agent": ["autogroup:admin"]
  },
  "grants": [
    {
      "src": ["tag:masterdrive-controller"],
      "dst": ["tag:masterdrive-agent"],
      "ip": ["tcp:443"],
      "app": {
        "example.com/cap/masterdrive": [
          {"shares": ["documents"], "read": true}
        ]
      }
    }
  ]
}
```

Replace `example.com/cap/masterdrive` with a namespace under a **domain controlled by the operator**. It is not yet a registered MasterDrive capability and the sample has not been applied or validated in a live tailnet. Tailscale reserves its own domains for built-in capabilities. Narrow the source and destination selectors as much as the real topology permits. Existing broad/default-allow grants may still authorize port access because grants are additive, so audit the *whole* policy. [Grant syntax](https://tailscale.com/docs/reference/syntax/grants), [custom capability namespace](https://tailscale.com/docs/features/access-control/grants/grants-app-capabilities).

## Windows/Linux qualification gates

All are **not yet passed**; a green localhost CI smoke does not count as a two-node mesh test.

1. **Versions:** record actual Go, Windows/Linux OS and Tailscale CLI/daemon versions. Require v1.92+ if Serve capability headers are chosen; block startup or explicitly disable identity-aware Serve mode on older versions. Confirm `Serve` is private and not `Funnel` on the intended port.
2. **Identity matrix:** user device, tagged controller node, externally shared node, missing/garbled identity header, spoofed localhost header, proxy-bypass attempt, stale/renamed/reinstalled node. Each unauthorized case must deny bytes and metadata.
3. **Permission matrix:** same controller granted share A but not B; removed share A; requested unknown share; removed grant while logged in; deleted/expired pair; rotated key; restarted agent; changed source OS account. Require exact deny behavior, not UI-only hiding.
4. **Pair protocol:** wrong out-of-band code, expired invite, replay, parallel redemption, mismatched Ed25519 fingerprint, clock skew, TLS cert change, request reordering, simultaneous revoke and download, agent restart mid-pair; assert atomic transitions and no share leak.
5. **Host deployment:** real two-machine Windows↔Linux private transport, tailnet offline/reconnect, Windows boot/service account/credential vault behavior, Linux file permissions, Tailscale startup race, DNS churn, port conflicts, and local firewall rules. The backend must remain bound to loopback for the Serve approach.
6. **Observability:** audit meaningful pair create/use/revoke events without paths, tokens or invitation material. Maintain a human-visible local kill switch disabling every remote share without deleting user files.

## Open implementation decisions

- **Serve vs. tsnet:** Serve minimizes dependencies and can be used manually now, but authentication via forwarded headers has a localhost-process spoofing caveat and tagged nodes need app-cap forwarding on v1.92+. `tsnet` offers a distinct node identity and `WhoIs`, but adds a significant dependency and a new persistent node enrollment/state lifecycle; Windows compatibility and packaging must be integration-tested rather than assumed. Prefer `tsnet` for strict per-device identity after a spike, retain Serve+bearer as explicit prototype mode.
- **Pair signing vs. mTLS:** choose one wire-level credential design via a versioned protocol ADR before implementation.
- **Multi-writer catalog and key vault:** the current host catalog cannot be treated as a transactional pair database across concurrent CLI/controller processes; implement single-writer transactions, file locking or an embedded transactional store before storing revocation-sensitive state.
- **Long streams and offline revocation:** define a bounded stream lifetime and a cancellation signal; no network isolation mechanism can erase file bytes already delivered to an authorized client.

## Evidence and references

This design is traceable to the local `identity-work/research.md` evidence log and project source inspection; the log can be published separately under `docs/research/identity-evidence.md`. Primary references: [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), [Tailscale identity](https://tailscale.com/docs/concepts/tailscale-identity), [grants syntax](https://tailscale.com/docs/reference/syntax/grants), [custom app capabilities](https://tailscale.com/docs/features/access-control/grants/grants-app-capabilities), [tsnet how-to](https://tailscale.com/docs/features/tsnet/how-to/create-basic-tsnet-app), and [Taildrive/Quad100](https://tailscale.com/docs/reference/quad100). No live pairing, node authorization, revocation, or two-host transport experiment was performed for this document.
