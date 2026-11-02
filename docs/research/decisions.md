# MasterDrive architecture decisions and qualification gates

Status: **research recommendation**, not an approved architecture, implemented feature, or live deployment. Snapshot 2026-11-01, America/Indiana/Indianapolis. Cross-reference [prior-art.md](prior-art.md).

## ADR-001: Distinguish a storage federation from a storage pool

**Decision proposed:** Ship a read-only federated file browser first. Exported roots remain physically resident on their hosts; present them as distinct namespace segments. Report each host's capacity separately, plus a clearly labeled aggregate *inventory* (not a guaranteed allocatable or fault-tolerant virtual volume). Later stages may introduce managed copy/sync, placement, and redundancy behind separate versioned contracts.

**Why:** A usable two-host vertical slice can be checked without pretending to have distributed metadata, consensus, file locking or replication. Genuine pooling follows a different risk/qualification path illustrated by [SeaweedFS](https://github.com/seaweedfs/seaweedfs), [CephFS](https://docs.ceph.com/en/latest/cephfs/), and [JuiceFS](https://juicefs.com/docs/community/architecture/).

**Rejected at v0.1:** Automatic whole-drive sharing, implicit full-filesystem write permissions, silent rebalancing, automatic deletions, Windows driver installation, cryptographic VPN reimplementation, POSIX correctness claims, RAID/erasure-code reliability claims.

## ADR-002: Treat connectivity and authorization as independent controls

**Decision proposed:** Pair nodes explicitly, require positive authorization on every share and operation, deny by default; do not mistake being on a LAN/tailnet for file authorization. Prefer an existing authenticated tunnel/overlay, with an optional Taildrive adapter, over inventing encrypted networking. If a custom application protocol is used, require authenticated transport (e.g. HTTPS with validated peer identity) and least-privilege short-lived credentials. Do not expose an unauthenticated agent on `0.0.0.0`.

**Rationale:** [Tailscale node keys](https://tailscale.com/docs/concepts/node-keys) provide device identity while [grants](https://tailscale.com/docs/features/access-control/grants) authorize network/application capabilities. [Taildrive](https://tailscale.com/docs/features/taildrive) already integrates such controls. MasterDrive must define its own file authority if it runs a separate agent.

**Open design choice:** Whether first production integration is built around Taildrive/WebDAV, an opt-in local MasterDrive agent over an existing overlay, or adapters for both. Do not silently bind to an external SaaS requirement without a user product decision.

## ADR-003: Keep filesystem and protocol adapters replaceable

**Decision proposed:** Internal logical identity `(peer-id, export-id, relative-path)` with stable opaque IDs; a capability interface for list/stat/open/download and optional write methods. Map SMB/NFS/WebDAV/native-agent source semantics into this limited interface with explicit unsupported-operation errors. Never conflate filesystem case-folding, symlink traversal, UID/GID and Windows ACL behavior.

**Why:** [SMB2/3](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/), [NFSv4](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/10/html/configuring_and_using_network_file_services/deploying-an-nfs-server), [WebDAV](https://datatracker.ietf.org/doc/rfc4918/), and [rclone](https://rclone.org/commands/rclone_serve/) make different guarantees. Adapters cannot promise stronger guarantees than their backend.

## v0.1 acceptance contract (falsifiable)

1. Install and launch on Windows and Linux without privileged filesystem drivers; a local GUI displays explicit paired hosts and explicit exported roots.
2. Two separate test hosts expose isolated test directories. One GUI lists both; each returned file records its owning host/share. A disconnected host is marked unavailable without hiding other hosts or displaying stale inventory as current.
3. A non-exported sibling file, `../` traversal, encoded traversal, symlink/junction escape and unauthorized direct API request must all be denied, including when the GUI is bypassed. No host root/home directory is shared by default.
4. File listing and downloads are read-only. A downloaded file's cryptographic checksum matches its source; interruption and host-offline behavior produce explicit errors. No copy, rename, deletion, move or sync effect occurs.
5. A credential or node revocation blocks subsequent requests; logs omit secrets and sensitive paths by default. A service refuses unsafe remote plaintext deployment.
6. Cross-platform build, unit and integration tests pass from exact reported revision; at least one Windows and one Linux real-host probe is needed before claiming cross-platform runtime qualification. CI compile alone is insufficient.

## Progression gates

| Gate | Entry criteria | Required proof before advancing |
| --- | --- | --- |
| G0: federation prototype | v0.1 contract implemented; no write APIs | Unit traversal/auth tests, two-host integration, failure simulation, explicit secure-network constraints |
| G1: durable federation | G0; user consent for persistent config | Restart/recovery, stable peer IDs, revocation, audit, discovery controls, negative security testing |
| G2: managed transfers | G1; explicit source and destination permission | Checksums, atomic temp-to-final promotion, resumability, quota/space, cancellation, rollback, crash and duplicate-request tests |
| G3: synchronization | G2; selected roots and conflict policy | Version vectors/causal identity or defined equivalent, conflict replicas, clock-skew and rename/case-fold tests; never silent last-writer-wins |
| G4: managed virtual pool | G3 or independent validated design; formal placement and durability specification | Replication/erasure coding tests, data/metadata failure matrix, node partitions, capacity reservation, repair, disaster recovery, backup/export; independent hostile review |
| G5: OS-native mount | Stable backend semantics; explicit privilege grant | Windows WinFsp/Linux FUSE integration, application compatibility, locking/cache semantics, crash/disconnect stress, uninstall/rollback |

These gates are **not milestones already achieved**. G2/G3/G4/G5 may be reordered if evidence warrants, but neither a beautiful GUI nor green compilation can waive their independent qualification.

## Hostile challenges and possible refutations

> **Overlap risk:** Taildrive already supplies persistent sharing, per-share permissions and Windows drive access. A shallow clone has weak reason to exist. Validate aggregated multi-provider management and placement needs with users first; see [official feature scope](https://tailscale.com/docs/features/taildrive).

> **Safety risk:** A broad bearer token plus directory-tree traversal can turn a misconfigured agent into a remote filesystem exfiltration endpoint. Make every export and operation explicit, constrain paths at the OS boundary, rotate/revoke credentials, and test races around symlinks/junctions.

> **Truth-in-naming risk:** Aggregate byte counts are not a writable pool, and copies are not shared-disk semantics. The UI must show exactly what is local, remote, replicated, stale, or unavailable.

> **Portability risk:** Windows and Linux differ in filenames, ACLs, link types and locking. A passing Go/Linux unit suite is not evidence of Windows operation; later native mounts require [WinFsp](https://winfsp.dev/doc/) and [FUSE](https://docs.kernel.org/filesystems/fuse/fuse.html) qualification.

## Evidence types

This document is **RESEARCH / DESIGN REVIEW** based on linked upstream documentation. It is not source inspection of MasterDrive's future implementation, a build, test, runtime probe, security audit, formal verification, or a human acceptance decision. A future integrator should update status with exact Git head and test receipts, without promoting this research to runtime proof.
