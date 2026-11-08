# Read-only mount and pairing qualification gates (source baseline)

Status: **design/test contract, not an implemented mount or pairing feature**. Branch based on `main`, not on the experimental snapshot PR chain. Applies to Windows and Linux, with a $0 infrastructure budget. No real share or tailnet policy changes are authorized by this document.

## Verified baseline and boundaries

The main-branch `README.md`, `docs/architecture.md`, and `internal/agent/agent.go` describe an opt-in read-only agent, explicit exports, pinned `os.Root` directories, bearer-token share grants, a loopback controller, and manually registered HTTPS origins. The handler authenticates requests and scopes shares; it does **not** implement cryptographic peer enrollment or live grant revocation. `remove-host` changes the controller catalog, not the source agent's authority. The existing `/v1/download` endpoint must not be treated as a version-pinned random-access file.

Experimental PRs #39–#47 are **not main**. In particular, #39 demonstrates a synthetic mixed-version ranged download, #40 introduces bounded digest-tagged snapshot responses, #41 separately addresses duplicate conditional headers, and #42–#47 are a stacked experimental admission/test chain. None proves coherent source reads during concurrent in-place writes. Do not infer that an ETag of captured bytes is a stable source-version lease.

## Fail-closed mount preconditions

A mount adapter MUST remain disabled until all conditions below are implemented and tested against an exact source revision.

1. **Authority:** source-approved device identity is bound to a per-share grant; controller catalog membership, tailnet membership, or possession of a browser session is not sufficient. Enrollment requires explicit source-owner confirmation and a one-time, expiry-bounded challenge; replay and duplicate enrollment fail. The source records grant identity, share IDs, issuance, and revocation generation durably without plaintext reusable tokens in catalog or logs.
2. **Revocation:** source-side revocation survives restart and blocks *new* reads immediately. An active transfer is either cancelled by the source or has an explicitly documented, bounded expiry; test both. Catalog removal alone must never be advertised as revocation. A disconnected controller must not silently resurrect an old grant after reconnection.
3. **Version identity:** a read handle identifies one immutable byte representation, not just path/size/mtime. A snapshot may be created by bounded copy under a validated source-side protocol, or by a genuinely immutable filesystem version with platform-specific proof. If neither exists, return an explicit unsupported/precondition error and do not mount. A digest computed from a potentially torn read cannot prove coherence.
4. **Range semantics:** every partial read is bound to the *same* representation ID, total length and strong validator. Missing, stale, malformed, duplicated or conflicting conditional headers fail closed. A mismatch must never silently return bytes from a newer representation under an older handle.
5. **Lifetime:** impose bounded bytes, open descriptors, capture memory, duration, concurrent handles, and cancellation cleanup. Test actual OS handles in addition to admission-token counters; unauthorized requests must not monopolize file-open slots. A server restart invalidates volatile leases rather than guessing they remain valid.
6. **Offline behavior:** no fabricated zeros, stale data silently presented as current, or fallback to another host without a separately verified identical representation. Distinguish unavailable, revoked, changed, and permission-denied states. The adapter remains read-only; attempts to create, rename, delete, truncate, chmod or write fail locally and at the agent.
7. **Transport:** use only owner/operator-approved private TLS routing with certificate verification and redirect/token-leak protections. Tailscale Serve is an optional transport, not an identity substitute; never enable Funnel as a shortcut.

## Acceptance matrix (not yet executed)

| Case | Required observation |
| --- | --- |
| Synthetic file replaced between two range reads | Existing representation yields consistent old bytes or explicit invalidation; never a mixed old/new file |
| In-place rewrite preserving length and mtime | Immutable snapshot still consistent, or acquisition fails closed |
| Revocation before read / during read / after agent restart | No new authorized reads; active-stream policy enforced; no token resurrection |
| Duplicate `Range`, `If-Match`, `If-Range` | Explicit rejection before capture or descriptor admission |
| Invalid/unknown share under capacity pressure | Denied without exhausting authorized users' descriptor budget |
| Cancellation, 416, 412, 413, broken client, timeout | No leaked descriptors, capture buffers, or active leases |
| Network partition / controller restart | Offline/error state, no cross-version stitching or credential leakage |
| Windows junction/reparse and Linux symlink races | Confinement holds under the supported patched Go runtime |
| Mount read/write API fuzzing | No mutating operation reaches a source; deterministic read-only error |
| Windows + Linux real hosts with authorized private transport | Record exact binaries, OS/FS, configuration, negative tests, traces and owner approvals before qualification |

For each case capture the exact commit SHA, OS, Go runtime, test command, test output and evidence class. CI on disposable fixtures is **not** real-host qualification. Independent security review is required before any merge or mount enablement.

## Engineering order

Implement source-owned identity/grant custody and durable revocation separately from transport; then a bounded immutable representation/lease protocol with negative tests; then a read-only adapter behind an explicit feature gate. Integrate the duplicate-header rejection before versioned reads become eligible. Do not merge the experimental stacked snapshot PRs directly into main as if each were independent. Keep the existing local browser/read-only functionality intact.
