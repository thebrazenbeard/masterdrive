# Native read-only source capacity probing — candidate G2 slice

Status: **isolated source candidate; not installed, executed, or reachable over HTTP**. It reports only the filesystem behind an explicitly opened export. It does not discover volumes, sum capacities, grant writes, or measure a quota reserved for MasterDrive.

`capacity.ProbePinnedRoot(shareID, *os.Root, time)` accepts an owner-authorized, already-open export root; it **does not accept a path**. On failure it returns an `unavailable` observation with a coarse reason, without raw OS errors, paths, or fabricated zero bytes. It has no server route and no background probe. The caller must hold the root open until the call completes.

## Platform semantics

- **Linux:** open `.` relative to the pinned `os.Root` and run `fstatfs` on that directory descriptor. Multiply `f_blocks` and **f_bavail** by positive `f_frsize`; reject overflow, nonsensical totals, or unknown fragment size. `f_bavail` excludes some reserved space; it is neither a full per-user quota calculation nor a write entitlement.
- **Windows (local volumes only):** open `.` relative to pinned `os.Root`, resolve that handle with `GetFinalPathNameByHandleW(VOLUME_NAME_GUID)`, strictly extract the volume GUID root, and query `GetDiskFreeSpaceExW` on that root. Return `lpFreeBytesAvailableToCaller`, **not** raw volume free bytes. UNC/mapped network shares and filesystems without stable Mount Manager GUIDs return `unavailable` rather than falling back to a race-prone configured path. Even the GUID-root result is underlying-volume information and can differ from per-directory policy/quotas.
- **Other platforms:** `unsupported`, no probe. Windows and Linux remain the only supported build targets.

This uses `golang.org/x/sys/windows` for secure system-DLL binding. The isolated module candidate pins `golang.org/x/sys v0.49.0`. **Before integration**, generate/check in its official `go.sum` using a trusted Go toolchain; never guess its digest. Run Windows/Linux `go test ./...`, `go vet ./...`, Linux race tests, and a disposable local filesystem probe. No native tests were executed in this scratch runtime, which has no Go installed.

## Fail-closed integration gates

1. Do not add `GET /v1/capacity` until bearer grant scope and unknown-share non-enumeration are tested. The handler must call the probe only after authentication, using its pinned `roots[share]`, with explicit rate limits and no host path from the request.
2. Confirm Windows handle-derived volume GUID behavior on NTFS junction/reparse replacement, UNC, mounted volumes without letters, and redirected folders. `GetDiskFreeSpaceExW` on the *configured path* is not acceptable because Windows follows changed links. If semantics remain ambiguous, return unknown.
3. Compare capacity values to actual native system tools on disposable shares; verify clock skew, quota effects, stale/offline states, and appropriate UI wording. Never sum two exports: they may refer to one filesystem.

Primary sources: [Linux fstatfs(2)](https://man7.org/linux/man-pages/man2/statfs.2.html), [Microsoft GetFinalPathNameByHandleW](https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-getfinalpathnamebyhandlew), [Microsoft GetDiskFreeSpaceExW](https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw), and [Go x/sys/windows API](https://pkg.go.dev/golang.org/x/sys/windows).
