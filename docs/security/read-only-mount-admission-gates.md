# Read-only mount admission: source-verified hold gates

Status: design and qualification checklist only. No mount implementation or host qualification is implied.

## Source evidence (2026-11-09)

- `go.mod` on `main` declares Go 1.27.0.
- Draft PR #64 at `05d01a9b24d90b2f2d88a3de0f50f337f035a446` implements `internal/versionproof.VerifyStream` on a stacked, **unmerged** branch. It compares a caller-supplied version and SHA-256 digest, checks bounded complete-file length, and probes EOF unless the final read returned data plus EOF. It does not authenticate the pin or reader.
- Issue #7 documents an intermittent Windows catalog write-lock AccessDenied, not a proven root cause.
- Issue #2 defines Stage 0 read-only mesh and explicitly reserves real pooling for Stage 3.

## Mandatory preconditions (all must pass, fail closed)

1. **Host identity**: authenticate peer device to a locally trusted identity; verify pairing, identity rotation and current revocation, with durable anti-rollback evidence. Tailnet connectivity alone is not authorization.
2. **Source consent**: bind each request to a source-owned, currently valid, least-privilege read grant for one share, identity, operation and expiry. Reject stale/ambiguous policy epochs and revoked credentials.
3. **Path confinement**: source must resolve beneath an explicitly opted-in root and reject symlink/reparse escapes and race-based path substitution. Test Windows and Linux separately.
4. **Representation**: pin digest, exact length and version to an independently authenticated source snapshot. A peer supplying both bytes and the expected digest is not integrity evidence. A whole-file digest does not validate arbitrary ranges.
5. **Transport**: use authenticated HTTPS identity and exact destination binding; disallow redirects, ambient proxies and credential leakage. Enforce connection, header, body, idle and total deadlines at the actual network reader. A synchronous `io.Reader.Read` cannot be cancelled by a wrapper that only checks context between reads.
6. **Mount semantics**: explicitly read-only; deny create/write/rename/delete and fallback-to-write. Define offline, timeout, stale-cache and revocation behavior before exposing filesystem operations. Do not claim a distributed pool.
7. **Evidence**: exact-head Windows/Linux CI, negative integration tests, independent security review, and separately authorized disposable-host runtime trials. Do not infer real drive qualification from unit tests.

## Adversarial acceptance matrix

| Fault injected | Expected behavior | Evidence required |
| --- | --- | --- |
| Revoked device reconnects with cached token | Reject before data access | Durable restart/rollback tests |
| Grant changed after directory listing | Reject stale read or reauthorize against current source policy | Concurrent source-agent integration |
| File changes between stat and read | Reject version mismatch; never return mixed-version success | Mutable-file race test |
| Peer sends correct bytes and its own forged digest | Reject untrusted pin | Trust-boundary integration |
| Partial range validated with whole-file digest | Reject as insufficient proof | Range proof negative test |
| Reader blocks forever inside `Read` | Transport deadline cancels and closes underlying stream | Real cancellable transport test |
| Reader returns `(n>0, io.EOF)` | Accept only exact pinned length and digest | PR #64 regression tests + exact-head CI |
| Windows reparse point changes during traversal | Reject root escape | Windows disposable NTFS test |
| Linux symlink changes during traversal | Reject root escape | Linux filesystem race test |
| Network drops after mount admission | Fail boundedly without write fallback | Mount fault-injection |
| Windows catalog lock returns AccessDenied | Preserve genuine ACL failure; retry only proven contention | Issue #7 native reproduction |

## Integration order

Pairing/revocation and source-owned grant authority precede authenticated immutable snapshots; snapshots precede verified reads; verified reads precede optional read-only mount admission. Each stage must reject missing upstream evidence. Keep PRs #60–#64 draft until their exact-head tests and independent reviews pass; stacking does not establish merge readiness.

## Host safety

Source-only work is allowed without host action. Never launch visible Windows consoles or steal foreground focus. A hidden-window flag alone is not evidence of noninterference. No drive exports, mount activation, Tailscale policy changes, or live host probes without fresh operator authorization.
