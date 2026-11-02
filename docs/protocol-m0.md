# M0: immutable-representation contract for read-only MasterDrive mounts

**Status (2026-11-02): SOURCE-ONLY DESIGN + BOUNDED BYTE-SEAL EXPERIMENT. NOT a working snapshot API, native mount, durable cache, content-integrity qualification, or deployed protocol.** This proposal targets Windows and Linux only. It extends the experimental G1 agent/controller and the G2 `webdavpilot` interface without silently changing their live behavior.

## Literal failure being addressed

G1 uses `os.Root` to keep a request inside an explicitly exported tree. Its download handler opens a regular file, calls `f.Stat()`, then passes the **live, mutable file handle** to `http.ServeContent`. A concurrent writer can change bytes, length, or identity during a long transfer. A controller can verify a single range's status/length but cannot prove two separate ranges came from one representation. `size + modified_at`, file IDs, and an `ETag` invented by a gateway are **not** proof of content immutability.

The G2 read-only WebDAV pilot correctly requires a future backend `Stat` / `Children` / `OpenSnapshot(ctx,key,strongETag)`. The backend is not implemented. Returning a mutable source `*os.File` from `OpenSnapshot` would violate the intended contract even if `Stat` and `Open` happen to agree once.

> **Hostile challenge — TOCTOU:** A file can be replaced, truncated, rewritten in place, or mutated twice between metadata observations. A consistent file ID and unchanged size/mtime do not prove a fixed byte sequence. Hashing a source and then reopening it for transfer also creates a race. A mount that assembles range reads under that assumption can silently corrupt data.

> **Hostile challenge — resource pressure:** Making a private copy of every source file can require space comparable to the data being served. This is not free, even if the software itself has no license fee. On a memory-constrained machine or nearly full disk, implicit caching is an unacceptable default.

## M0 non-negotiable invariants

1. **Owner scope first.** An authenticated, specifically approved `(source node, share, portable relative path)` is checked before any metadata, content opening, or snapshot creation. No host/drive discovery, implicit share, or absolute source path on the wire.
2. **Immutable representation.** Successful `OpenSnapshot` yields only a byte sequence that cannot change for the lifetime of its reader. The exact byte sequence is associated with a strong version token and a fixed `size`.
3. **Version check is source-owned.** The source that owns a sealed snapshot issues its strong ETag; a controller cannot invent or upgrade one from `(name,size,mtime)` or a client-provided value. Different snapshot bytes must not share a version token.
4. **No stale fallback.** If a snapshot is missing, altered, revoked, expired, offline, or cannot be proved, return a distinguishable failure. Never silently substitute a new live file when a client asks for an old version.
5. **No unauthorized persistence.** Snapshot copying is off by default; any cache budget, TTL, location, encryption, filesystem ACL, cleanup, share scope, and memory/disk quota require an explicit owner policy. No permanent copies on Lappy by assumption.
6. **Revocation is independent.** M0 versions do not grant access. Every new open and transfer must re-evaluate the caller's active share grant; running reads need a tested cancellation and source shutdown guarantee. Cache access must fail after revocation even when bytes remain stored.
7. **No writes.** M0 does not authorize PUT, DELETE, MOVE, rename, allocation, pooling, replica placement or background indexing.

## Proposed protocol objects — not existing routes

`host_id`, `share_id`, `path` and `source_epoch` are authority selectors, not display labels. `snapshot_id` is an opaque, source-generated capability identifier **not sufficient without authorization**. Expose only sanitized relative names, never source filesystem paths.

```json
{
  "schema": "masterdrive.m0.representation.v1",
  "host_id": "<registered-host-id>",
  "share_id": "approved-share",
  "path": "project/report.pdf",
  "source_epoch": "<opaque-source-generation>",
  "snapshot_id": "<opaque-unpredictable-id>",
  "strong_etag": "\"m0-opaque-version\"",
  "size_bytes": "123456789",
  "captured_at": "2026-11-02T20:00:00Z",
  "expires_at": "2026-11-02T20:05:00Z",
  "snapshot_semantics": "sealed-copy",
  "complete": true
}
```

`size_bytes` is a decimal string (not a JavaScript floating-point number) to preserve large exact lengths, aligned with `docs/capacity.md`. Times are informational TTL/provenance, **not strong validators**. A directory snapshot would need a separate versioned child list with deterministic sort, bounded pagination, stable cursor and explicit collision handling; an offset into a live directory is not a snapshot.

Suggested future endpoints, explicitly **unimplemented**:

| Operation | Requirement | Failure |
| --- | --- | --- |
| `POST /v2/snapshots` | Opt-in, share-scoped, max size/quota, authenticated operation intent; creates a bounded sealed representation | `403/404` unauthorized, `413/507` quota, `503` capture unavailable |
| `GET /v2/snapshots/{id}/metadata` | Reauthorize, return immutable metadata and strong validator | `404` missing/expired, `410` revoked version (if safe to distinguish) |
| `GET /v2/snapshots/{id}/bytes` | Reauthorize, exact selected version; single checked range and `If-Range` strong match | `206` exact range; `412` version mismatch; `416` invalid range; `503` offline |
| `DELETE /v2/snapshots/{id}` | Local owner authorization to discard only cache bytes, not source files | Never delete source path |

The v2 shape is illustrative, not a commitment to `POST` being remotely exposed. A local broker may have to create snapshots instead, with no network creation API. Existence and error-disclosure semantics need threat review. Do not add unauthenticated endpoints or let a snapshot ID act as a standalone bearer credential.

## HTTP conditional-range semantics

RFC 9110 §13.1.5 ordinarily allows `If-Range`: send the range if the strong validator matches; otherwise send a full `200` representation. M0 intentionally adds a **fail-closed mount adapter** contract to avoid mixing full and partial segments: when a client demands a particular version, a mismatch must be detected and handled as an error (e.g. `412` for the version-pinned API) before streaming bytes. The general HTTP `If-Range` semantics and this stricter versioned API must not be conflated.

- Never attach a weak `W/` tag, modification date, guessed ETag, or clock-based validator to a range assembly.
- Limit requests to a single syntactically checked byte range, bound to the immutable size. Verify `Content-Range` start/end/total and transferred exact byte count; detect clean premature EOF.
- If an upstream returns unexpected `200` for a versioned range, the controller **fails** rather than appending it to an existing buffer. A regular browser download without a version pin may still use the existing v1 read-through path with its honest consistency limitation.
- Always validate the upstream's **source-issued** strong ETag on each response and its binding to the chosen `host/share/path/snapshot`. A strong ETag applies to one representation, not universally across a share or host.
- Cache and proxy behavior must be audited: `no-store`, no cross-user content sharing, redirects forbidden, credential confinement, bounded timeouts and response size.

## Filesystem capture choices and honest guarantees

| Candidate | What it can truthfully guarantee | What it cannot guarantee without more work |
| --- | --- | --- |
| **Small in-memory sealed copy** (experimental `internal/versioncontract`) | Once captured successfully, repeated reads return exactly the owned bytes; SHA-256 strong tag matches those bytes. At most 16 MiB per capture; no disk copy | That the bytes came from a single point in time on a concurrently written source, safe host-wide memory quotas, per-user access, cache lifetime or external URL integration |
| **Authorized copy to sealed temporary artifact** | Stable bytes after flush/close, hash/verify, immutable/open-by-handle policy and quota enforcement | Source point-in-time consistency; disk availability, durable deletion, encryption, crash cleanup, symlink/reparse and permission correctness |
| **Filesystem-native snapshot** (VSS, Btrfs/ZFS/LVM as applicable) | Potential filesystem-coherent frozen view at source-specific snapshot boundary | Generic Windows/Linux support, admin-independent setup, remote filesystem/UNC support, semantics across mounted filesystems and failure modes |
| **Live open file handle + stat pre/post** | Detects **some** modifications | Cannot establish immutable bytes or a safe strong ETag; must fail the M0 gate |

**One important nuance:** an in-memory copy captured while another process writes is a stable *captured byte sequence*, but may contain pieces from different source revisions. It is not a point-in-time filesystem snapshot. If the product promises users they are viewing one coherent historical source version, this candidate **fails** that stronger requirement; use an authenticated filesystem snapshot facility or immutable source-file/version provider instead.

## Portable file identity and confinement

- `os.Root.Open`/`OpenFile` confines path resolution beneath its root, including symlink escape prevention on supported platforms, but does **not** automatically block crossing filesystem boundaries, Linux bind mounts, `/proc` special files or device files. Continue checking final handles are ordinary files. A Go patch-level requirement is mandatory: `os.Root` has had published Unix symlink-boundary vulnerabilities in older 1.25/1.26/1.27 prerelease versions (Go advisory GO-2026-4970).
- Windows `FILE_ID_INFO` combines a volume serial and 128-bit file ID to distinguish open handles; IDs are useful provenance, not cryptographic content validators. `CreateFile` with `FILE_FLAG_OPEN_REPARSE_POINT` changes whether the reparse point itself or its target is opened. Test NTFS junctions and file replacement at actual native Windows runtime; never infer safety from Unix-only tests.
- On Linux, `(st_dev, st_ino)` and `statx` mount ID can help detect handle/namespace changes, not establish content immutability; inodes can be reused. Bind mounts and other filesystems may require explicit owner policy. Keep the authorized open handle while reading, no string-based path reopen after validating metadata.
- Filesystems with aliases, hardlinks, case folding, Unicode normalization, ADS, reserved device names, sparse/compressed files and remote mounts need platform-specific collision and resource tests. The G2 WebDAV pilot's conservative ASCII-only namespace is **intentional**, not full filesystem compatibility.

## Minimal source-only test primitive (not integrated)

`internal/versioncontract.Capture(ctx, reader, limit)` is a bounded 16 MiB in-memory capture with no disk writes. It owns copied bytes, returns an exact SHA-256-derived strong tag and independent seekable readers only for the matching tag. It has tests for caller buffer mutation, second reader independence, stale tags, too-large/invalid limits, cancellation, and upstream I/O failure. This demonstrates only **stable bytes after capture**, not coherent source snapshots. No source-file opening, agent route, controller route, WebDAV handler, encryption, owner ACL, global quota or cleanup service is implemented. Its unsalted content hash may leak file equality/known-content clues; a real protocol needs a privacy-aware tag strategy.

## Qualification gates before any mount integration

1. **Semantics:** Owners select a capture policy and size/quota limits. Distinguish source-coherent snapshots from merely sealed read results in every API and UI claim.
2. **Source authorization:** Per-share identity and pending revocation checked on capture, metadata, each open, each stream. Confirm that disallowed shares and stale snapshots are indistinguishable where required to prevent enumeration.
3. **Mutator race:** Race heavy overwrite/truncate/rename/reparse/hardlink and filesystem snapshot operations, then verify output digest, version, size and response codes. No success may contain bytes from a different declared sealed snapshot.
4. **Cross-platform runtime:** Native Windows NTFS/ReFS and Linux ext4/Btrfs test matrices for files, junctions/symlinks, bind mounts, UNC/NAS, sparse/large files, empty files, changed files, case/Unicode collisions, disk quota and abrupt source restarts.
5. **Transport:** Two owner-approved hosts over verified private HTTPS; incorrect certificate, forged identity, revoked grant, redirect, replay, off-tailnet probe and partial disconnect tests with genuine negative readback.
6. **Client interoperability:** Actual rclone WebDAV and WinFsp/FUSE read-only mounts on disposable shares; crash/restart, repeated range, concurrent readers, directory snapshots, stale cache invalidation and OS-user local broker isolation.
7. **Resource control:** Global and per-share memory/disk budgets, capture concurrency bounds, timeout cancellation, close-on-revocation, zero unintended persistence, explicit owner cleanup and recoverability.
8. **Independent evidence:** Go unit/race/vet and source review are necessary but insufficient; retain exact Git SHA, Windows/Linux versions, filesystem types, transport state, owner approval and observed test statuses.

### Primary sources

- [RFC 9110 HTTP semantics §§8.8, 13.1.5, 14](https://www.rfc-editor.org/rfc/rfc9110.html) — validators, `If-Range`, byte ranges.
- [Go `os.Root` package documentation](https://pkg.go.dev/os#Root) — confined root behavior and boundaries.
- [Go security advisory GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970) — prior `os.Root` symlink escapes fixed by patched Go releases.
- [Microsoft `FILE_ID_INFO`](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_id_info) — volume + identifier for handle identity.
- [Microsoft `CreateFile` / reparse semantics](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew) — reparse flags and open behavior.
- [Microsoft NTFS change journal records](https://learn.microsoft.com/en-us/windows/win32/fileio/change-journal-records) — change detection is not byte reconstruction.
- [Linux `statx(2)`](https://man7.org/linux/man-pages/man2/statx.2.html) — inode/mount metadata and limitations.
- [Linux FUSE documentation](https://docs.kernel.org/filesystems/fuse/fuse.html) — userspace filesystem boundary.

### Range gate prototype (source-only)

The experimental `Sealed.OpenRange(expectedETag, offset, length)` rejects stale
versions before checking range bounds and returns an exact bounded reader of
already captured bytes. It accepts an empty read at EOF but rejects negative,
overflowing, or out-of-bounds ranges. Concurrent readers have independent state.
`Capture` also rejects a source reader that repeatedly returns `(0,nil)`
instead of spinning indefinitely. Neither primitive authorizes a share, pins a
real filesystem snapshot, enforces global memory budgets, cancels blocked
readers, or serves HTTP. Every open/stream still needs a durable authorization
and revocation fence before mount use. No native mount is enabled.
