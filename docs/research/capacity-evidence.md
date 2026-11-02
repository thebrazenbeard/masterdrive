# MasterDrive capacity inventory: preregistered research

Date: 2026-11-02 UTC. Source manifest: `masterdrive-build/go.mod` declares Go 1.27.0 and no module dependencies.

## Frozen question

Can MasterDrive report useful, non-misleading read-only capacity for opt-in exports on Windows and Linux, without suggesting a pooled writable disk or leaking host-local paths?

## Hypotheses / falsifiers

- H1: Filesystem-level total/free can be obtained on both supported platforms without scanning files. Falsifier: no supported local filesystem API can provide it within the dependency/security limits.
- H2: Filesystem-level free space is not equivalent to free bytes allocatable to one share, particularly with quotas/mounts/network filesystems. Falsifier: platform docs guarantee identical semantics across these cases.
- H3: A conservative response can maintain per-share host identity, measurement timestamp and explicit unknown/unsupported states. Falsifier: current public API cannot add a new read-only route without breaking existing routes or secrets.

## Planned methods and stopping rules

- M1 confirmatory: inspect current agent/controller protocol and module version.
- M2 confirmatory: query official Go/Linux/Windows API docs for native filesystem space semantics (maximum three grouped documentation searches).
- M3 exploratory: design the minimal read-only `capacity` response and compile-targeted code slice with negative tests if API contracts permit; otherwise document why not.
- M4 confirmatory: verify static caller compatibility, then run any available test/build command. Report no runtime success if toolchain unavailable.

Stop after a bounded API recommendation and source/test deliverables; cross-platform runtime qualification and hostile mount reconfiguration remain separate gates. An independent verification pass is required before research can be called closed.

## Evidence log (append-only)

- E1 (M1, source inspection): `masterdrive-build/go.mod`, verbatim `go 1.27.0`; `agent_masterdrive_workspace/internal/agent/agent.go` defines opt-in roots and authenticated `GET /v1/shares`, `/v1/list`, `/v1/download`. No capacity endpoint. Inspected 2026-11-02.
- E2 (M2, Linux API): [statvfs(3), Linux man-pages](https://man7.org/linux/man-pages/man3/statvfs.3.html), fetched 2026-11-02. `f_blocks` is total fragments, `f_bavail` blocks available for unprivileged users, while `f_bfree` is raw free blocks. Some filesystems do not populate every field meaningfully. Linux `statfs`/`fstatfs` are backing kernel interfaces.
- E3 (M2, Windows API): [GetDiskFreeSpaceExW, Microsoft Learn](https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw), fetched 2026-11-02. Total and available-to-caller may be quota-adjusted; total free across volume is separately returned. Directory path is accepted, but symlinks follow their targets.
- E4 (M2, Windows library): [Go syscall API](https://pkg.go.dev/syscall?GOOS=windows), [Go stdlib Windows generated syscalls](https://go.dev/src/syscall/zsyscall_windows.go), and [vendored x/sys/windows implementation](https://go.dev/src/cmd/vendor/golang.org/x/sys/windows/zsyscall_windows.go), fetched 2026-11-02. Official Go `syscall.NewLazyDLL` warns of DLL preloading; Go standard syscall source has no `GetDiskFreeSpaceEx` wrapper, whereas the `golang.org/x/sys/windows` module provides it. Adding a vetted/pinned dependency is preferable to a bespoke unsafe dynamic call.
- E5 (M2, Go root boundary): [Go 1.27 `os.Root` docs](https://pkg.go.dev/os@go1.27.3), fetched 2026-11-02. Root constrains path escapes but does not forbid crossing mount boundaries. A pinned directory handle is therefore preferable to re-resolving a mutable original path for a metric on Linux; reporting underlying filesystem observations is not share-quota enforcement.

## Current analysis, not independent verdict

- H1 **partially supported** by E2/E3/E4: native APIs exist; a secure no-extra-dependency Windows binding is not present in current Go stdlib. No native code or runtime test yet.
- H2 **supported** by E2/E3/E5: the values are not a per-export entitlement, and Windows semantics are caller/quota-sensitive. Never label them `share_free_bytes` or add them across shares.
- H3 **source-plausible** from E1: a new bearer-protected read-only endpoint could be introduced without changing existing routes, but needs runtime tests and controller/UI contracts.
- Pivot P1: From direct cross-platform implementation to source-bound protocol design first, because the Go module currently has no external dependencies and low-level Windows system API binding risks should not be smuggled into v0.1. A future implementation should use reviewed `x/sys/windows` at a pinned version or another audited wrapper; no false free-space implementation is published here.
- M3 exploratory outcome: scoped protocol draft below, no compiled code. M4 status: not executed because the local toolchain is unavailable; CI on current repo was previously driven by the parent, not evidence for a nonexistent capacity implementation.

## Post-design implementation note (2026-11-02)

- P2: Added a **platform-neutral helper only** at `internal/capacity/metrics.go` and tests in `metrics_test.go` to reject overflow and construct exact decimal-string measurements. This does not query a disk, alter the MasterDrive agent/controller API, or establish Windows/Linux native measurement. The protocol remains a design.
- E6 (M4 diagnostics): `command -v go` exit 1, empty output; `command -v gofmt` exit 1, empty output; `rg --files capacity-v1-work` exit 0 and listed both Go files, the design document, and this log. The code is uncompiled and its tests unrun.
- M4 stopped as **unverified by execution**, not passed. No current `go.mod` change or external dependency was made.
- Independent verification is still **open**; this source-bound record is not a closed research verdict until a separate reviewer checks E1-E6 and the native API claims.

