# MasterDrive

**One private storage mesh. Every authorized drive. One place to find your files.**

MasterDrive is a Windows/Linux storage-network project: connect your own computers and storage servers over a private network, explicitly share selected directories or drives, and browse those locations through a unified interface. The long-term goal is to make a collection of independent disks feel like one coherent storage environment, without concealing where data actually lives.

Tailscale or another encrypted private overlay can provide connectivity; MasterDrive provides the storage-specific catalog, permissions, browsing, health, search, and—later—placement and recovery policies. MasterDrive is intended to be transport-agnostic, not a new VPN.

## What MasterDrive is building

| Capability | Intended behavior | Stage |
| --- | --- | --- |
| Private device mesh | Connect explicitly approved Windows/Linux peers through an encrypted private network | Initial + pairing improvements |
| Opt-in exports | Each owner selects the exact folders/drives to make available; no automatic disk exposure | Initial |
| Unified browser | One responsive GUI for devices, shares, folders, and downloads | Initial |
| Cross-device inventory | Host and storage capacity, availability, offline/stale state, and permissions | Next |
| Search and virtual namespace | Find files across registered sources while retaining their host identity | Next |
| Mounted drive | Optional familiar filesystem view in Explorer or Linux file managers | Later |
| Writable distributed pool | Placement, redundancy, quotas, checksums, conflicts, and recovery | Research/qualification milestone |

## The architectural distinction

MasterDrive separates three layers:

1. **Connectivity and identity:** a private mesh or authenticated TLS transport connects authorized devices. Tailscale Serve is one possible starting point.
2. **Storage federation:** each device publishes only approved roots. A controller presents a combined namespace and forwards authorized operations. Files remain on the source device unless an explicit transfer is performed.
3. **Distributed pooling (future):** storage placement, replicas or erasure coding, consistency, and recovery across failure domains. A combined GUI does **not** by itself make a RAID array, a backup, or a durable distributed filesystem.

A disconnected device should be shown as unavailable or stale, never silently represented as a healthy replica. A file is not protected simply because it appears in the unified view.

## Security principles

- **Deny by default.** No drive is shared merely because a device joins a network.
- **Explicit sharing and revocation.** Device approval and per-share authorization must remain separate from network reachability.
- **Encrypted remote access.** Do not expose an agent or controller directly to the public internet or transmit bearer credentials over remote plaintext HTTP.
- **Read-only first.** Before uploads, deletes, or mounted writes, establish trustworthy path confinement, atomicity, collision handling, auditability, and rollback.
- **Honest availability.** Offline hosts, partial transfers, filesystem changes, and failed authorization must be visible and fail closed.

The initial implementation still needs full independent security review and live multi-host qualification. **Do not use it as the only access path or as a backup for irreplaceable data.**

## Current status

<small>Early development — source and tests are not an installed service.</small>

- The default branch currently contains the project definition and roadmap, **not yet a released or installed storage mesh**.
- [Build PR #1](https://github.com/thebrazenbeard/masterdrive/pull/1) contains the developing read-only controller, opt-in node agent, registry, CLI, and initial Windows/Linux tests. Review its latest head and CI before treating it as runnable.
- [Issue #2](https://github.com/thebrazenbeard/masterdrive/issues/2) tracks the staged acceptance criteria through actual pooling, including unresolved security and runtime gates.
- Research and independent architecture/security/UI contributions are being developed in separate branches before integration.

There is intentionally no copy-paste production installation command on this page yet. Once an integration release passes the checks and is merged, this README will include a tested Windows and Linux quickstart.

## Prior art and compatibility

[Tailscale Taildrive](https://tailscale.com/docs/features/taildrive) already shares folders across tailnet devices and is documented as an alpha feature. MasterDrive's intended added value is a broader, transport-independent storage-management experience—not pretending basic private folder sharing was invented here.

Research also covers [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), [rclone mount](https://rclone.org/commands/rclone_mount/), [WinFsp](https://winfsp.dev/doc/), [Syncthing](https://docs.syncthing.net/users/syncing), and [CephFS](https://docs.ceph.com/en/latest/cephfs/). Their distinct tradeoffs matter: remote access, synchronization, mounted filesystems, and true distributed storage are not interchangeable.

## Development and evidence

The initial backend is being written in Go with no third-party runtime modules, with tests targeting Windows and Linux. Evidence is tracked separately:

- **Source / test / CI:** checks of exact committed code.
- **Runtime probe:** a real agent and controller exchanging authorized data on identified hosts.
- **Security review:** hostile boundary testing, identity and authorization checks, and path confinement.
- **Product qualification:** recovery, performance, offline behavior, and user workflows on both platforms.

A green CI check is useful, but it does not establish that a distributed storage product is deployed or safe.

See the [roadmap](https://github.com/thebrazenbeard/masterdrive/issues/2) and [open development PRs](https://github.com/thebrazenbeard/masterdrive/pulls). License terms have not yet been selected; do not assume permission to redistribute the source.
