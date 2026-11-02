# Capacity inventory without pretending to pool disks

Status: **proposed G1 protocol and implementation plan** (2026-11-02), not an implemented MasterDrive endpoint or live host qualification. This design deliberately does not create a single writable capacity number.

## Meaning of a capacity observation

An exported folder is a *view of a backing filesystem*, not a separately provisioned disk or quota. Two exports may share one volume; a bind mount, Windows reparse point, mount namespace, network filesystem or storage quota can change the relationship. A successful capacity probe says nothing about the existence, freshness, permissions, retrievability, or integrity of individual files.

Native platform APIs are not semantically identical:

| Platform | Proposed native source | Total | Available | Interpretation caveat |
| --- | --- | --- | --- | --- |
| Linux | `fstatfs` on a directory handle opened beneath the export's pinned `os.Root` | `Blocks × fragment size` | `Bavail × fragment size` | `Bavail` excludes some reserved blocks for unprivileged users; it is not a complete per-identity quota calculation. |
| Windows | `GetDiskFreeSpaceExW` on the explicitly approved export directory, via a vetted `golang.org/x/sys/windows` binding | `lpTotalNumberOfBytes` | `lpFreeBytesAvailableToCaller` | Both may be quota-adjusted for the calling user. A path-based probe may follow a changed symlink/junction. |

The third Windows return value, `lpTotalNumberOfFreeBytes`, is raw volume free space, *not* how much this agent can allocate. It should not replace available-to-caller. Linux `Bfree` has a similar distinction. Different shares on the same backing volume can report identical totals and available bytes; **do not sum them into an aggregate pool**.

Official references: [Linux statvfs(3)](https://man7.org/linux/man-pages/man3/statvfs.3.html), [Microsoft GetDiskFreeSpaceExW](https://learn.microsoft.com/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw), [Go 1.27 os.Root](https://pkg.go.dev/os@go1.27.3), [Go Windows syscall caveat](https://pkg.go.dev/syscall?GOOS=windows), [Go x/sys/windows](https://pkg.go.dev/golang.org/x/sys/windows).

## Proposed read-only v1 extension

`GET /v1/capacity?share=<opaque-share-id>` is a **future source-agent route**, requiring the existing bearer token. It must not take a raw filesystem path or enumerate drives. Only shares explicitly present in the live agent config may be queried. Unknown IDs return `404`; unauthenticated queries return `401`, before probing any filesystem.

Example successful response (illustrative values, not a live measurement):

```json
{
  "share": "docs",
  "status": "measured",
  "scope": "backing_filesystem",
  "total_reported_bytes": "1000000000000",
  "available_reported_bytes": "200000000000",
  "availability_semantics": "unprivileged",
  "measured_at": "2026-11-02T00:00:00Z",
  "read_only_export": true
}
```

Fields:

- `status`: `measured`, `unavailable`, or `unsupported`. Only `measured` responses contain numeric fields; unavailable/unsupported values omit numbers rather than coercing to zero. A controller that has not queried a share displays **Not checked** instead of inventing its status.
- `scope`: always `backing_filesystem` for this API. Never `share_quota`, `virtual_pool`, or `available_for_masterdrive_writes`.
- `total_reported_bytes`, `available_reported_bytes`: unsigned **decimal strings**, not JSON numbers. This avoids JavaScript's 53-bit integer precision limit and allows `uint64` values to be represented exactly. Conversion from filesystem blocks must check multiplication overflow; impossible/ambiguous readings become `unavailable`.
- `availability_semantics`: `unprivileged` on Linux; `caller_quota` on Windows. The label is essential when comparing hosts. Do not infer filesystem access to any other identity.
- `measured_at`: UTC timestamp from the source host; not an assertion that the peer remains online or that the numbers remain accurate. Display age and an expiry badge, treating stale observations as stale rather than zero or healthy.
- `read_only_export`: always `true` in v0.1/G1; native free space does **not** confer a MasterDrive write permission.

For `unavailable` or `unsupported`, include a coarse reason code such as `probe_failed` or `platform_not_supported` but **never** underlying directory paths or raw OS errors. A failed probe must not take down unrelated host/share listing.

## Gateway and UI

The controller may add `GET /api/v1/hosts/{host}/shares/{share}/capacity` as a **pass-through to the configured agent** with existing peer credentials, same strict origin and redirect rules, same local browser session; it must not ask arbitrary remote URLs for capacity. Apply a bounded response size and JSON schema validation. A responding source is evidence of a successful capacity probe **at that moment**, not of a globally healthy mount or owner authorization.

The UI should show a host/share-specific card such as “200 GB reported available on source filesystem · measured 42s ago,” with caveats and an explicit unknown state. Never aggregate the bytes by addition; even attempting volume-based de-duplication requires a separate privacy-preserving, stable identity scheme and still does not create allocatable pool capacity. Do not calculate “percent used by this share” from filesystem totals. Do not trigger background recursive directory traversal.

## Implementation plan (not completed)

1. Add a narrowly scoped capacity probe interface under the existing agent handler. Keep the `os.Root` export pinning; on Linux prefer `fstatfs` of a directory opened using that root so a rename cannot redirect the metric. Handle file-descriptor lifetime and `uint64` overflow defensively.
2. On Windows, use a reviewed pinned `golang.org/x/sys/windows` dependency (not an ad-hoc `syscall.NewLazyDLL` whose search behavior can create DLL preloading risk), document quota sensitivity, and test reparse replacement before any claim that the returned metric refers to the originally approved backing volume. If that guarantee cannot be established, report `unavailable` on changed paths rather than presenting a misleading value.
3. Add authentication and unknown-share tests plus platform-native temporary-volume tests. Probe errors, unmounted shares, directory renames, junction/bind-mount changes, network shares and quota-limited users must fail visibly without exposing paths.
4. Add the controller proxy and UI only after agent tests. Keep transport availability, capacity freshness, and share authorization as **three distinct states**. Avoid a global “total free” banner.
5. Qualify on real Windows/Linux hosts with timestamps, current code revision, expected filesystem/identity, exact API outputs and permission policies **without logging tokens or exporting personal drives**.

## Acceptance gates

- [ ] Wrong/missing bearer token cannot measure *any* configured share.
- [ ] Unknown share ID cannot probe an arbitrary host path or enumerate drive letters/mounts.
- [ ] Native Linux/Windows readings match their documented APIs under ordinary local test volumes; quota and reserved-block cases are labeled accurately.
- [ ] No cross-share/host totals are summed; two shares on the same volume never masquerade as twice the capacity.
- [ ] Values that overflow, are nonsensical, or are unavailable render as **unknown**, never as zero or healthy.
- [ ] Concurrent mount swaps/reparse changes and offline/network volumes fail closed within an agreed operational timeout; no unbounded background scan is performed.
- [ ] Windows/Linux CI and native host tests prove the exact integrated code, separately from the design above.

