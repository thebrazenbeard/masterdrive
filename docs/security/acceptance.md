# MasterDrive acceptance gates — security, correctness, recovery

Status: **proposed executable qualification plan**, not a record of passing tests. Target: exact integrated commit + built Windows/Linux binaries, Go compiler patch level, Tailscale/WireGuard and host configuration recorded for every run. A CI green badge is insufficient when the test did not exercise a live mesh or filesystem race.

## Gate 0 — product honesty and source provenance

1. Exact Git SHA and CI run recorded; developer-generated artifacts and `go version` recorded. Go compiler must exclude GO-2026-4970 affected versions (Unix `os.Root` path escape): use a patched version such as Go 1.26.5+ or appropriate patched newer release.
2. CLI/README/UI consistently say **read-only virtual federation** (no RAID, parity, replication, automatic placement, native OS mount, guaranteed capacity aggregate, offline availability, or backup claim).
3. No automatic scan or exposure of home directories, devices, NAS volumes, SMB shares, `D:\\`, `/`, or other roots. Explicit export-ID-to-directory configuration is mandatory.
4. Every remote service/listener is loopback by default; no public network bind or Tailscale Funnel. `tailscale serve status --json` (or equivalent) and a network probe show the intended private-mesh route only.

## Gate 1 — file confinement (block release on any failure)

Run each test on Linux and Windows, including NTFS junctions/reparse points, without skipping tests merely because creating links requires elevation. Use an appropriately provisioned test host or explicitly mark platform incomplete.

| Case | Expected result |
| --- | --- |
| Export absent, relative or duplicate; empty share list | Refuse invalid config; no default drive export |
| `../`, `a/../../`, percent encoded slash/backslash, double encoding, NUL, UNC/drive-letter/device names, Windows `CON`/`NUL`/`COM1` | Reject or never leave exported root; no private bytes returned |
| Symlink/junction inside root to outside; symlink as final element with trailing `/` | Reject; outside sentinel must never appear (includes GO-2026-4970 regression) |
| Swap intermediate directory to external symlink between validation and file open (repeated concurrent race) | Never read sentinel outside root |
| Rename configured export directory after agent startup, replace old path with sensitive bait | Continue serving pinned original directory or fail closed; never follow replacement |
| Named pipe, Unix socket, block/character device, `/proc` subtree, nested mount | No special-file download; document mount traversal policy and verify refusal where configured |
| Attempt HTTP `PUT`, `POST`, `DELETE`, `PATCH`, WebDAV write commands | No write; no local content mutated |
| Hidden files and nested data chosen within root | Only visible to explicitly authorized peer; clarify hidden-file policy |

## Gate 2 — peer identity, consent, and revocation (block broad sharing)

1. New mesh-connected device has **zero** shares until the owner explicitly exports a root, pairs that peer and assigns read permission to the share. Tailnet membership or a network grant is insufficient by itself.
2. Unknown, wrong, empty, duplicate, replayed/retired credentials cannot list or download shares; return consistent 401/403 without information leakage. Compare secrets without timing-sensitive early exits.
3. Use peer identity authenticated by Tailscale Serve/LocalAPI *or* mutually authenticated TLS with explicit trust roots, and bind it to locally stored share authorization. If using Serve identity/app-capability headers, enforce backend loopback and reject spoofed headers on a direct path.
4. Revoke a paired peer and read back failure for its **previously valid** credential. Stop an export and verify cached metadata disappears and remote GET fails. A tailnet device removal alone must not be silently reported as share-level revocation.
5. Disconnect and rejoin tailnet, rotate node key/cert, change DNS target, and verify identity continuity or re-pair requirements without silently attaching the wrong host's storage.
6. Verify Tailscale Serve is private and Funnel is disabled. Unauthorized tailnet member and public Internet probes both fail, including direct-IP and DNS rebinding attempts.

## Gate 3 — controller/browser authorization (block release)

1. Local controller rejects non-loopback bind and hostile `Host` values. Browser requests without valid session cookie receive 401, including shares/downloads; unrelated websites cannot activate session via CSRF or DNS rebinding.
2. Session bootstrap token is generated with cryptographic randomness; UI consumes fragment and removes it from URL history before normal navigation; session cookie is `HttpOnly`, `SameSite=Strict`, path-limited, ephemeral, and is `Secure` when served over HTTPS.
3. Browser cannot discover peer bearer tokens, raw local filesystem export paths, OS identities or hidden controller secrets in page HTML, API bodies, localStorage/sessionStorage, logs, or download URLs.
4. Remote file name `"><img src=x onerror=...>` and `<script>...` stays inert text in host/share/tree views; no untrusted HTML insertion. Downloads use `attachment` and safe filename handling.
5. Add/remove-host works as documented *while controller runs* or explicitly requires restart; two concurrent CLI processes cannot silently lose a registered host. Registry is intact after crash during atomic write.

## Gate 4 — network proxy and file integrity

1. Invalid/unregistered endpoint, URL containing user-info/query/path, remote plain HTTP, invalid TLS certificate/hostname, redirects (301/302/307/308), slow server, oversized JSON and unexpected content encoding all fail closed; no bearer token is sent to a redirect destination.
2. Proxy allows only configured peer paths. Overlong input, parameter injection, path traversal and query encoding do not alter the outgoing authority; invalid share IDs are rejected.
3. Download passes with known-length file; peer early EOF, disconnect and declared-length mismatch result in a failed client transfer, not a successful truncated file.
4. Single-range `206` test checks start/end/total/byte count, suffix, open-ended, invalid, multiple-range and unsatisfiable requests. Do not splice responses from different file revisions: require strong ETag/If-Range or content hash when resuming; otherwise disable resume and label limitations.
5. Under bandwidth cap and client cancellation, HTTP handlers release descriptors and goroutines. One offline/slow host does not block unrelated hosts or exhaust all sockets.
6. End-to-end file bytes match independently computed SHA-256 across Linux→Windows, Windows→Linux, and same-OS peer pairs for small, Unicode-name, sparse, and multi-GB files. Never infer integrity from `Content-Length` alone.

## Gate 5 — real-system mesh qualification

Required evidence matrix (each entry needs timestamp, exact binaries/Git SHA, endpoint identities, redacted configuration, command, result, logs, and any waiver):

| Environment | Positive test | Negative/failure test |
| --- | --- | --- |
| Windows agent → Windows controller | Browse consented export, stream bytes and compare SHA-256 | Directory junction escape; controller session spoof |
| Linux agent → Windows controller | Browse and transfer over encrypted private mesh | Symlink swap / patched `os.Root` regression; invalid cert |
| Windows agent → Linux controller | Browse and compare bytes | NTFS reparse path/rename; revoke peer |
| Linux agent → Linux controller | Browse/search with 10k+ entries, pagination | Offline peer, interrupted transfer, concurrent catalog edit |
| Multi-host pool (three or more) | GUI displays distinct host ownership and accurate independent capacities if implemented | One host offline without false unified availability or data loss |

## Blocking verdict template

For each gate record `PASS`, `PARTIAL`, `FAIL`, or `UNKNOWN` and attach evidence. `UNKNOWN` does not authorize remote deployment or data access. Proposed initial build remains **SOURCE_READY / LIVE_MESH_UNQUALIFIED** until Gate 5 is completed. There is no need to suppress useful local prototype work while preventing unsupported production claims.

Sources: [Go os.Root](https://go.dev/blog/osroot), [GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970), [Tailscale Taildrive](https://tailscale.com/docs/features/taildrive), [Tailscale Grants](https://tailscale.com/docs/features/access-control/grants), [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), [Tailscale security best practices](https://tailscale.com/docs/reference/best-practices/security), [OWASP SSRF prevention](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html), [OWASP CSRF prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html).
