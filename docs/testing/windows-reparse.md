# Windows filesystem boundary qualification

Status: source-level regression suite staged for review. **Not yet executed on a native Windows host** by this subagent. It adds tests only; no exported real drive, machine policy, Tailscale configuration, or production service is changed.

## Risk being tested

NTFS junctions are reparse points that redirect a directory path to another directory. A malicious or compromised local user with write access under an approved export could introduce a junction that points outside it. MasterDrive must refuse to list or download from that outside target even if the HTTP caller has the agent bearer token. String-prefix checks and an initial check of the path are insufficient because the filesystem changes after registration.

The current source agent opens `os.Root` for the approved export and performs each requested file open through that pinned root. Go documents `os.Root` as traversal-resistant, including symlink constraints and Windows reserved device-name handling. Its Windows directory handle also prevents renaming/deletion of the pinned root while open; however, writable child directories can still change. Tests must exercise the real HTTP handler as well as individual path validation.

## Tests added

`internal/agent/agent_reparse_test.go` adds three tests, all skipped when `runtime.GOOS != "windows"`:

1. **Existing junction escape:** a genuine Windows junction inside an opt-in temporary export points to a sibling temporary directory containing a secret sentinel. Authenticated listing and download through the junction must fail, while an ordinary approved file must still download successfully.
2. **Post-registration insertion:** after the handler has opened the export root, a junction is created under it. Both direct listing and download attempts must fail without disclosing sentinel bytes.
3. **Reserved device names:** `NUL`, `CON`, `PRN`, `AUX`, `COM1`, and `LPT1` must fail `localPath` and the authenticated download route.

The fixtures use `t.TempDir` for *both* the approved and outside-sentinel directories. PowerShell receives the path values through process environment variables (not interpolated PowerShell source), and `New-Item -ItemType Junction` creates the genuine reparse point. The test registers non-recursive `os.Remove` cleanup of the junction before `t.TempDir` cleanup. It never executes junction creation or file deletion on a user's existing path. An inability to create a junction on native Windows is a test failure, not a silently ignored qualification.

## How to verify after integration

On an explicitly authorized, throwaway Windows CI runner with a current patched Go toolchain and Windows PowerShell available:

```powershell
go test -v ./internal/agent -run 'TestWindows(Junction|ReservedDevice)'
go test ./...
```

Check the `go test -v` output: tests must actually execute on `GOOS=windows`, not merely compile or report `SKIP`. The workflow's ordinary `windows-latest` `go test ./...` step will include the new tests once integrated. Record Windows build, Go patch version, filesystem (NTFS vs alternatives), and result before treating this as a security gate.

## What a pass would **not** prove

- These are deterministic junction escapes, not a continuous adversarial create/delete/rename race under high concurrency. Such a race, ACL changes, and live reparse-tag mutation require a separately owned Windows security testbed.
- They do not cover all reparse tags, volume mount points, SMB shares, remote filesystem behavior, symlink privilege variations, file identity changes across ranged requests, or Windows crash/power-loss recovery.
- Tests use disposable local directories, not a real user's data or a connected tailnet. They do not establish identity-based pairing, share revocation, or private-network policy.
- The source agent's reliance on `os.Root` requires a Go runtime with all relevant security fixes. In particular, [GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970) documents a separate Unix path/symlink escape in older Go versions, fixed by current releases. This Windows suite does not replace an up-to-date toolchain or Unix tests.

## Primary references

- [Go: Traversal-resistant file APIs](https://go.dev/blog/osroot) — `os.Root` guarantees, Windows semantics, and filesystem boundary limitations.
- [Go `os.Root` package documentation](https://pkg.go.dev/os#Root) — relative path confinement and Windows reserved device-name restrictions.
- [Microsoft: Reparse points](https://learn.microsoft.com/en-us/windows/win32/fileio/reparse-points) — kernel/filesystem indirection mechanism.
- [Microsoft: Hard links and junctions](https://learn.microsoft.com/en-us/windows/win32/fileio/hard-links-and-junctions) — NTFS junction behavior.
- [Microsoft: PowerShell 5.1 `New-Item`](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/new-item?view=powershell-5.1) — `Junction` item type and creation semantics.
