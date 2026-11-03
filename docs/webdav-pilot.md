# MasterDrive WebDAV feasibility slice (source-only)

Status: **prototype, not a supported mount** (2026-11-02). This source does not start a listener, attach to the MasterDrive controller, install drivers, authenticate a real OS user, or mount a drive. It is a narrow protocol experiment for future M1 qualification; no external writes or host operations were performed.

## Tested design assumptions and prerequisites

`internal/webdavpilot` contains a Go-standard-library `http.Handler` that requires an injected version-aware backend and mandatory identity authorization callback. Paths have the explicit canonical form:

```
/dav/<32-lower-hex-host-id>/<agent-share-id>/<validated-relative-path>
```

This source pilot deliberately accepts only a conservative **ASCII filename subset**: it rejects percent signs, backslashes, slashes within components, Windows-invalid punctuation (including colons used for alternate data streams), reserved DOS device names (including extensions), trailing dots/spaces, controls, and non-ASCII. Decoded or double-encoded separators are rejected; the same restriction applies to incoming paths and backend listing names. This drops otherwise valid filenames rather than silently aliasing them. It is **not** a final cross-platform mapping: Unicode normalization/equivalence, case-folding collisions, 8.3 aliases, and reversible handling of excluded names still need a defined namespace contract before any mount.

The read-only handler covers `OPTIONS`, finite-depth `PROPFIND` (explicit 0/1 only), `GET` and `HEAD`, and rejects all mutators. It rejects non-loopback HTTP Host values, but **Host verification is not a substitute for binding the listener only to loopback or for authenticating the caller**. Production must enforce both. An injected `Authorize` callback must be a revocable per-user and per-mount capability check; using an unconditional `true` policy outside tests is insecure. HTTP loopback bearer credentials may leak to other processes running under the same user. Prefer a protected Unix socket/Windows named pipe for capability issuance and process binding, and test the selected WebDAV client's actual credential storage behavior.

The `Backend` contract requires:

- `Stat(ctx, key)` with stable metadata, the exact final name represented by the requested virtual path (share ID at the root), and a **source-issued strong ETag** for every file. A mismatched returned name fails closed instead of silently aliasing another path.
- `Children(ctx, key, limit)` with an explicitly bounded snapshot cursor and return values `(entries, complete, err)`. The handler requests **at most 200 entries**, rejects `complete=false` and refuses any backend response exceeding the supplied limit. The backend **must not** read and allocate every child before applying the limit; it must use a streaming or paginated cursor, examining at most `limit + 1` entries to determine completeness. An incomplete result cannot become a successful `207` listing. This source-only interface requirement is **not proof** that a future adapter honors its memory limit; an actual adapter needs limit-enforcement and memory-budget tests.
- `OpenSnapshot(ctx, key, expectedETag)` returning immutable seekable bytes pinned to exactly that version until close, or an explicit stale/offline error. `mtime + size` is not a substitute for strong file identity.

`GET` and `HEAD` use a source-issued strong ETag and a version-pinned read handle. HTTP single ranges on `GET` are delegated to Go's standard `http.ServeContent` only after the immutable handle's length is verified against metadata. `HEAD` ignores `Range` entirely. Multi-ranges on `GET` are rejected when their range precondition applies; if an `If-Range` ETag or HTTP-date validator fails, HTTP semantics require ignoring the `Range` and serving the full selected representation. Matching date-form `If-Range` is accepted. A missing source version, stale handle, source offline, or inconsistent body length fails closed.

### Known protocol limitations

- `PROPFIND` currently recognizes an empty body or DAV `propfind` XML root, then returns a fixed set of standard properties. It does **not** implement property-by-property selection (`prop`, `propname`, `allprop`) nor arbitrary custom properties. An interoperability test with the exact chosen client must decide whether this is adequate; passing unit tests does not establish rclone or Windows Explorer compatibility.
- The handler does not implement WebDAV locking, shared/exclusive caches, resumable multi-operation directory snapshots, FUSE inode handles, WinFsp file IDs, or live revocation teardown.
- The handler does not create an `http.Server`, protect its Unix socket/named pipe, ensure the operating-system transport peer is the same user, or enforce timeouts. The embedding process must enforce those separately.
- No buffering to local disk is implemented. A client (including rclone VFS, Explorer indexing, or OS page cache) may cache file content independently; eviction and revocation are unqualified.
- A `207` response with 200 children is complete **only if the backend's snapshot contract is true and it explicitly returns `complete=true`**. A remote paginated listing cannot be wrapped directly without version/cursor semantics. A backend ignoring the requested limit is rejected on oversized output, but preventing unbounded upstream allocation still depends on the backend's own implementation.

## Next acceptance sequence

1. Implement M0 on the *source* protocol and controller: authenticated per-object `stat`, consistent paginated directory cursor, strong version/immutable handle for ranges, offline/stale/revoked error differentiation. Qualify this on fixture sources with churn and forced disconnections.
2. Provide a per-user, mount-scoped local authorization broker. Separate it from browser sessions and the controller's peer credentials. Verify another local process/user cannot gain its authority.
3. Integrate the bridge with the new controller contract; add strict XML property selection if the selected WebDAV client requires it. Exercise `PROPFIND`, `GET`, `HEAD`, range and precondition logic with a real rclone WebDAV client against a disposable in-memory/local fixture.
4. Only after explicit host approval, run user-session Windows WinFsp / Linux FUSE mount qualification with `rclone mount --read-only`, and measure client cache/revocation, directory churn, stale reads, crash recovery and teardown. No automatic mounts or installation.

The local tests model authorizer rejection, traversal and encoded separator rejection, mutation denial, namespace/multistatus, finite-depth limits, immutable reads, ETags, HTTP ranges and offline/stale failures. The integration candidate also exercises sealed fixture bytes across independent range reads. Go tests have passed on the integration branch, but each subsequent head still requires its own CI/readback; neither tests nor CI qualify a live mount or production transport.

## Primary references

- [RFC 4918 — Web Distributed Authoring and Versioning](https://www.rfc-editor.org/rfc/rfc4918), especially `PROPFIND`, `Depth`, `multistatus`.
- [RFC 9110 — HTTP Semantics](https://www.rfc-editor.org/rfc/rfc9110), including strong validators and conditional range behavior.
- [rclone WebDAV remote](https://rclone.org/webdav/) and [rclone mount](https://rclone.org/commands/rclone_mount/) for client and cache semantics.
- [Linux FUSE kernel interface](https://docs.kernel.org/filesystems/fuse/fuse.html); [WinFsp API](https://winfsp.dev/doc/WinFsp-API-winfsp.h/) for later native qualification.
- Repository-local [mount design](mounts.md) documents the broader M0/M1/M2 gates and existing source gaps.
