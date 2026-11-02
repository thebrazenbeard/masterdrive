# MasterDrive prior art and differentiation

Research snapshot: 2026-11-01 (America/Indiana/Indianapolis). Sources below are project-maintained documentation, protocol specifications, or official implementation repositories, not independent product certification. Feature support may change.

## Scope of Patrick's idea

MasterDrive aspires to make the storage exported by multiple authorized Windows/Linux machines appear in one interface, with an eventual logical storage pool. These are different engineering claims:

- **Federated namespace**: display distinct host/share roots (`host/share/path`) together; files remain owned by their original machine. Cross-host browsing does *not* aggregate free space into a fault-tolerant disk and does not make an offline host accessible.
- **Synchronization**: copies file replicas to designated machines and reconciles divergence. Capacity is consumed on each replica, and concurrent modification creates conflicts.
- **Distributed filesystem/object pool**: chooses placement and redundancy of data and metadata across hosts, with documented quorum, consistency, rebuild, failure and recovery behavior. Merely summing host free bytes does not deliver this.

## Most important overlap: Taildrive exists

Tailscale's **Taildrive** is already a persistent cross-device folder-sharing product. Its official documentation describes a WebDAV server at `100.100.100.100:8080`, a namespace `/<tailnet>/<machine>/<share>`, per-share grants including read/write controls, Windows and Linux sharing, and Windows Explorer mapped-drive support. Taildrive is currently labeled **alpha**. [Taildrive](https://tailscale.com/docs/features/taildrive); [Taildrive application capabilities](https://tailscale.com/docs/features/access-control/grants/grants-app-capabilities). This substantially overlaps with a literal 'Tailscale but for hard drives' interpretation; describing simple cross-device folder sharing as unprecedented would be false.

Tailscale already supplies cryptographic device identity and authenticated overlay connectivity via node keys, and authorization via grants. [Node keys](https://tailscale.com/docs/concepts/node-keys); [Grants](https://tailscale.com/docs/features/access-control/grants). Its [Taildrop](https://tailscale.com/docs/features/taildrop) sends files rather than presenting a persistent namespace. Taildrop and Taildrive should not be conflated. MasterDrive must not claim to have built WireGuard or encrypted peer-to-peer overlay networking when relying on existing Tailscale connectivity.

**Potential differentiation (hypotheses to validate):** aggregate Taildrive/SMB/NFS/rclone-backed roots in one inventory and GUI; cross-host capacity accounting and forecasts; indexed search with disclosure boundaries; explicit transfer/copy/sync plans with budgets and checksums; later policy-driven placement, replication, and machine-failure repair. Taildrive itself may change, so verify current features before claiming any gap. An optional adapter built on official interfaces may be less brittle than a duplicate transport stack.

## Primary-source comparison

| System | Model and documented capability | Relevance / caution |
| --- | --- | --- |
| [Tailscale Taildrive](https://tailscale.com/docs/features/taildrive) | Persistent tailnet file sharing via WebDAV; per-share controls; Windows/Linux share providers; alpha | Direct overlap for remote shares. Can be a connectivity/share adapter; not evidence of a capacity-aggregating placement engine. |
| [Tailscale grants](https://tailscale.com/docs/features/access-control/grants), [node keys](https://tailscale.com/docs/concepts/node-keys) | Device identity and network/app permissions | Reuse conceptually or integrate legitimately; never treat reachability as file authorization. |
| [SMB 2/3](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-smb2/) | Stateful network file sharing; Windows SMB 3.1.1 supports signing/encryption and Multichannel in applicable configurations [Microsoft feature descriptions](https://learn.microsoft.com/en-us/windows-server/storage/file-server/smb-feature-descriptions) | Native interoperability, but account mapping, locking, open handles, dialect negotiation, security policy and offline behavior remain host-specific. |
| [NFSv4](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/10/html/configuring_and_using_network_file_services/deploying-an-nfs-server) | Linux-oriented network filesystem with NFSv4 security and locking semantics | Not an implicit secure Windows/Linux common denominator. Identity mapping, permissions, firewall and mount behavior need explicit integration tests. |
| [WebDAV RFC 4918](https://datatracker.ietf.org/doc/rfc4918/) | HTTP-based distributed authoring, directory discovery via PROPFIND and optional locking | Useful interchange protocol. A WebDAV URL alone is not secure sharing; require TLS, authentication and authorization, validate locking/version semantics for writes. |
| [rclone serve](https://rclone.org/commands/rclone_serve/) and [mount](https://rclone.org/commands/rclone_mount/) | Exposes many storage backends via HTTP/WebDAV/SFTP/S3 and mounts remotes through FUSE-related mechanisms | Useful backend adapter or delegated transport. `rclone serve http` defaults to localhost and advises authentication when network-exposed [docs](https://rclone.org/commands/rclone_serve_http/). Do not assume all backends supply identical metadata or atomic operations. |
| [Syncthing](https://docs.syncthing.net/users/syncing) | Peer folder synchronization; documents conflict detection and case-collision behavior | A reference for replication/conflict UX, **not** on-demand federation of unsynchronized disks. [Device setup](https://docs.syncthing.net/intro/getting-started) uses cryptographic device IDs. |
| [SeaweedFS](https://github.com/seaweedfs/seaweedfs) | Master/volume/filer architecture with replication and erasure coding; multiple access protocols | Evidence that true pooling requires metadata ownership, placement, recovery, and protocol gateways; far more machinery than a GUI. |
| [JuiceFS](https://juicefs.com/docs/community/architecture/) | Filesystem with object storage for data and separate metadata engine; mounts and caching | Useful reference for separated metadata/data and consistency tradeoffs; [cache docs](https://juicefs.com/docs/community/guide/cache/). |
| [CephFS](https://docs.ceph.com/en/latest/cephfs/) | POSIX-style distributed filesystem atop RADOS with metadata servers | True distributed filesystem example, with [disaster-recovery complexity](https://docs.ceph.com/en/latest/cephfs/disaster-recovery/). Not a sensible minimal dependency for a casual peer-drive mesh. |
| [Tahoe-LAFS](https://www.tahoe-lafs.org/trac/tahoe-lafs/wiki/FAQ) | Capability-oriented encrypted erasure-coded distributed storage | Inspires confidentiality and failure-tolerance design, but does not automatically solve ordinary OS mount semantics or write conflicts. |
| [WinFsp](https://winfsp.dev/doc/) / [Linux FUSE](https://docs.kernel.org/filesystems/fuse/fuse.html) | User-mode filesystem integration into Windows/Linux | Relevant to future unified OS drive mount. Driver installation, OS permissions, correct caching and reentrancy substantially enlarge risk. |

## Licensing and provenance boundary

Links are references, **not permission to copy their source** into MasterDrive. Before vendoring or linking any implementation, inspect the *exact pinned release's* LICENSE, notice files, transitive dependencies and distribution obligations. SeaweedFS' repository declares Apache-2.0 in its [README](https://github.com/seaweedfs/seaweedfs); that fact is insufficient to assess other projects' licenses. For now, rely on standards and documented protocols; avoid copied source and architecture diagrams. MasterDrive currently needs its own explicit license decision before external reuse claims.

## Direct falsifiers for the concept

> If Taildrive already meets the user's desired multi-host file access with a mapped drive, MasterDrive's simple sharing proposition has no differentiation. Investigate real gaps before adding a parallel authentication or VPN stack.

> If a node can disappear while a remote file is opened, any claim of a seamless local disk is false unless errors, retries and possibly replicated availability are defined and tested.

> If free space is merely added across nodes, calling it a reliable pooled filesystem is misleading. A 100 GB file may not fit despite 300 GB aggregate free space if no eligible single target has space, and losing one machine still loses its unique files.

> If ACL checks live only in the GUI, direct requests to the underlying agent can bypass them. Enforcement must happen at each data authority boundary.

## Research gaps that remain open

- Independent performance and security tests against Windows and Linux hosts; none were run in this research lane.
- Live Taildrive interoperability and API stability across versions; its alpha label argues for an optional, replaceable adapter.
- A complete threat model for identity, peer enrollment, key rotation, compromised hosts, symlink/junction path escapes, SSRF, and token disclosure.
- Recovery semantics for duplicate host IDs, case-insensitive filenames, rename races, clock skew, interrupted transfers and offline edits.
- Validation of licenses at exact dependency versions before packaging.
