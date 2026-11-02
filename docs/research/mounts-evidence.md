# MasterDrive mounted-filesystem research record

Research date: 2026-11-02 UTC. Owner: mount_arch workstream. Scope: read-only mounting of the existing MasterDrive federation on Windows and Linux. No host mutation, network probing or actual mounts are authorized in this workstream.

## Frozen plan (written before retrieval)

**Question Q1:** What is the smallest defensible staged design for exposing the current opt-in, read-only MasterDrive peer namespace as a native Windows drive and Linux mount, with explicit permissions, reproducible tests, and honest offline/cache behavior?

**H1 — adapter boundary.** An optional filesystem adapter can remain outside the existing network/permission plane, translating filesystem operations into the local controller's authenticated read-only API. **Prediction:** WinFsp and FUSE provide read-only user-mode filesystem surfaces that allow separate adapter processes. **Falsifier:** official platform APIs require kernel extension or privileged write-capable service in a way that cannot preserve the read-only boundary.

**H2 — rclone as bootstrap.** A WebDAV bridge + `rclone mount` could provide a development-only mount with lower implementation cost than a custom filesystem adapter. **Prediction:** current rclone docs support mount over a read-only remote and explicitly document WinFsp/FUSE dependencies and cache semantics. **Falsifier:** supported platform requirements or semantics make the bridge irreconcilable with the approved MasterDrive export-and-identity model.

**H3 — consistency limit.** Current MasterDrive API cannot promise POSIX/Windows full filesystem semantics without versioned stable file identities, strong validators, and partial-I/O error behavior. **Prediction:** current source lacks mutating filesystem operations, strong ETags, rename support and stable directory snapshots. **Falsifier:** verified source already implements those contracts.

**Methods:**

- M1 (confirmatory, source inspection): read project manifest and API docs/source; record exact contracts and missing methods, no code execution.
- M2 (confirmatory, authoritative documentation): retrieve current WinFsp, FUSE/libfuse, and rclone mount/WebDAV reference pages, at most three grouped documentation queries. Log direct source URL, retrieval date and decisive excerpts or source-summary fingerprints.
- M3 (confirmatory, design reconciliation): map required filesystem operations to existing API routes, identify hazards and staged acceptance tests; mark every implemented-vs-proposed claim.
- M4 (exploratory): evaluate alternative approaches and implementation choices without declaring runtime feasibility confirmed.

**Stopping rule:** Stop once M1–M3 are recorded and a bounded read-only adapter recommendation and test matrix are written. Do not implement or run a filesystem mount. Independent verification is still required to close the research; if no verifier is available, report `inconclusive` rather than self-certifying.

## Evidence log (append-only)

- **E01 — local manifest, inspected 2026-11-02.** `masterdrive-build/go.mod`, SHA-256 `451426f8f8ca1aeb9eccdf705f78be1893f330bec9c2d41815280715b4e80042`: `go 1.27.0`; no third-party module required. There is no pinned WinFsp, rclone, libfuse, or WebDAV dependency in the manifest. Applies to M1 and version selection for M2.
- **E02 — local wire contract, inspected 2026-11-02.** `architecture-masterdrive-work/docs/protocol-v1.md`, SHA-256 `6c0af75e9b66183e2a0ee0d3169225cf47e5445c71e23df6d68a4abc04644a16`: agent shares/list/download and controller proxy/fragment-cookie endpoints; offset pagination is best effort, current source has no durable file-version contract. Corroborated with `agentauth-v1-work/internal/agent/agent.go` SHA-256 `0846d7062d5bb1fa9dcabf405975e3cec2dc424b1af25623f8b4e6e160f45d2f` and `integrity-work/internal/server/server.go` SHA-256 `c53e2cc88a35309bfa262e3cef8912a1c799e4ba0335da36572741187beab0c7` (bounded Range/If-Range guard but no source ETag). Applies to M1/M3.
- **E03 — WinFsp user-mode API, retrieved 2026-11-02.** https://winfsp.dev/doc/WinFsp-API-winfsp.h/ (`turn59view0`, `turn66view2`, `turn66view1`): lines 20–29 require user-mode interface, `FspFileSystemCreate` and dispatcher; lines 542–590 describe offset-based file read and directory read. Decisive phrase: “A user mode file system is a program that uses the WinFsp API.” Applies to H1/M2.
- **E04 — WinFsp native vs FUSE, retrieved 2026-11-02.** https://winfsp.dev/doc/Native-API-vs-FUSE/ (`turn65view1`) lines 21–25 establish two user-mode APIs; lines 35–52 contrast Windows ACLs, UTF-16 names, reparse handling with FUSE semantics. Applies to H1/M2.
- **E05 — WinFsp service/FAQ, retrieved 2026-11-02.** https://winfsp.dev/doc/WinFsp-Service-Architecture/ (`turn65view2`) lines 21, 26–54 warn services may run elevated and describe launcher registry/ACL concerns. https://winfsp.dev/doc/Frequently-Asked-Questions/ (`turn65view0`) lines 47–56 document Windows reparse/mountpoint behavior. Applies to the user-session, no silent service design.
- **E06 — Linux kernel FUSE contract, retrieved 2026-11-02.** https://docs.kernel.org/filesystems/fuse/fuse.html (`turn59view1`) lines 3–32 define user-mode daemon and non-privileged mount; lines 70–75 describe `default_permissions` and `allow_other`; lines 95–103 describe connection abort. Applies to H1/M2.
- **E07 — rclone mount, retrieved 2026-11-02.** https://rclone.org/commands/rclone_mount/ (`turn59view2`, `turn63view0`, `turn63view1`, `turn63view2`, `turn63view3`) lines 65–76 describe Linux/Windows FUSE mounting; lines 97–101 require WinFsp on Windows; lines 230–237 and 298–305 warn about filesystem reliability and caching; line 576 lists `--read-only`. Applies to H2/M2. No rclone version exists in MasterDrive's manifest; the website is a current documentation channel, not an installed-version match.
- **E08 — rclone WebDAV client, retrieved 2026-11-02.** https://rclone.org/webdav/ (`turn59view3`) lines 33–41 and 59–65 document a WebDAV remote configured with URL and user/password. The current MasterDrive `/api/v1` JSON endpoints are not such a WebDAV server (E02). Applies to H2/M3.
- **E09 — output artifact, 2026-11-02.** `mountarch-work/docs/mounts.md`, SHA-256 `d07faa641cc5045bd56c4f472349bc102d74af10a78b5a20126d9dd990f7bbea`: maps required filesystem ops, trust boundaries, stages and acceptance gates. This proves a design was written, not that runtime behavior works.
- **E10 — retrieval dead end, 2026-11-02.** Direct `open(https://www.rfc-editor.org/rfc/rfc4918)` returned a timeout (`turn66view0`); the WebDAV method subset is therefore an **exploratory design proposal** to be verified against RFC 4918 and selected clients before any implementation or compliance claim.

## Experiment/results ledger

| ID | Status | Evidence | Result / deviation |
| --- | --- | --- | --- |
| M1 | Open | — | — |
| M2 | Open | — | — |
| M3 | Open | — | — |
| M4 | Open | — | — |

### Closure entries (append-only; original planned rows retained)

- M1 — **Completed, confirmatory source inspection.** Read `go.mod`, `docs/protocol-v1.md` and current isolated agent/controller source; E01–E02. Outcome: no current mount adapter or source strong-validator/cursor contract. No code execution performed. No deviation.
- M2 — **Completed, confirmatory documentation lookup.** Current vendor primary documentation on WinFsp, Linux FUSE and rclone mount/WebDAV; E03–E08. WinFsp/rclone/libfuse *installed versions do not exist in this repository*, so there is no local release to version-match. One standards URL timed out (E10); did not infer RFC compliance. No other deviation.
- M3 — **Completed, confirmatory design reconciliation.** The operation mapping and acceptance matrix are in E09, supported by E02–E08. Outcome: read-only user-mode adapters are architecturally plausible; current API lacks production-mount guarantees. This is a design compatibility assessment, **not an implementation test**.
- M4 — **Closed, exploratory design judgment.** Favor a private loopback WebDAV+rclone pilot only after M0 prerequisites; later native WinFsp/FUSE callbacks use a shared typed backend. E02, E03, E06–E09. No host experiment was run and no pilot feasibility was established.

## Claim status (author assessment, subject to independent verification)

- **Supported by vendor contracts:** user-mode WinFsp/FUSE interfaces exist; rclone mount exposes WinFsp/FUSE platforms, `--read-only` and cache modes (E03–E08).
- **Supported by local source:** MasterDrive has read-only list/download and bounded range support, but lacks native mount, WebDAV, stable `stat`, directory snapshot cursors and source-issued strong ETags (E01–E02).
- **Unproven/exploratory:** the proposed WebDAV bridge's client interoperability, native OS error codes, mount permission behavior, driver deployment, timeout/caching/revocation handling and reliable file-version coherence (E09–E10). These require actual tests.
- **Hypotheses:** H1 supported at the external API level; H2 plausible but unconfirmed because the bridge is not built; H3 supported by current source. None proves a mount works.

## Independent verification

Pending independent verifier.

### Independent verification addendum — 2026-11-02

- Reviewer: native `/root/docs_verifier`. Fresh primary-source retrieval: [rclone mount](https://rclone.org/commands/rclone_mount/), [WinFsp licensing/APIs](https://winfsp.dev/src/), [libfuse licensing](https://github.com/libfuse/libfuse/blob/master/LICENSE); original research E03–E08 further anchors FUSE kernel and WinFsp user-mode APIs. The rclone mount page documents Windows WinFsp/FUSE requirements and an explicit VFS cache-off option. No stable installed rclone/WinFsp versions or client compatibility trials were available.
- **Verdict:** `PARTIALLY CONFIRMED`: upstream API viability and clear design-vs-implementation boundary survive; WebDAV interop, file-version coherence, mount isolation, cache/revocation semantics and both OS mounts are **UNTESTED**. No mount, disk write or driver installation occurred.
