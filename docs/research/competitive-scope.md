# MasterDrive's scope relative to Taildrive and other storage tools

**Research snapshot:** 2026-11-02. **Evidence class:** exact-head MasterDrive source inspection plus current upstream documentation. This is a product/architecture comparison, **not** a benchmark, user-demand study, verified two-host test, mount qualification, or endorsement of a future design.

## The uncomfortable starting point

**Tailscale Taildrive already solves a large part of the literal “Tailscale for hard drives” idea.** It offers persistent folder sharing across a tailnet, per-share read-only or read/write grants, and a WebDAV access point at `100.100.100.100:8080` on the connected device. Tailscale calls Taildrive *alpha* in its current documentation. Simply building another private folder share over Tailscale would replicate existing capability; it is not a convincing differentiation. See the [official Taildrive explanation and policy examples](https://tailscale.com/docs/features/taildrive).

A proposed direction is an **experimental provider-neutral storage federation/catalog**, with a possible future owner-governed access and placement layer. **Today MasterDrive catalogs only manually registered native MasterDrive agents**; heterogeneous source adapters are not implemented. The broader positioning is a *product hypothesis*, not a proven gap in the market or an already delivered capability.

## What each tool actually addresses

| Tool | Officially documented primary problem | Important capability or boundary | Relation to MasterDrive |
| --- | --- | --- | --- |
| [Tailscale Taildrive](https://tailscale.com/docs/features/taildrive) | Shared folders within a tailnet | WebDAV namespace, persistent shares, policy-controlled `ro`/`rw` grants; presently documented as alpha | **Direct functional overlap.** MasterDrive currently adds its own manual agent/catalog UI and tightly bounded source filename search, but has no demonstrated advantage in connectivity, writes, or OS mounting. |
| [Syncthing](https://docs.syncthing.net/users/syncing) | Reconciling file copies across devices | Watches/scans roots, hashes blocks, exchanges versions and preserves conflict copies | A future optional **sync adapter or comparison point**, not a substitute for a remotely owned single file. MasterDrive currently has no synchronization. |
| [rclone mount](https://rclone.org/commands/rclone_mount/) | Exposing an existing remote as an OS filesystem | Virtual filesystem caching, configurable write-back and backend-dependent semantics | A **possible future mount client**, not proof that MasterDrive has suitable immutable reads, metadata, identities or locks. |
| [CephFS](https://docs.ceph.com/en/latest/cephfs/) | Coherent distributed POSIX filesystem atop RADOS | Metadata server cluster, separate metadata/data pools, direct client object I/O | A real **distributed filesystem** reference. A sum of remote drive free-space values is not a CephFS-like pool. |

The table does **not** assert that a listed tool cannot be combined with others, lacks every unlisted feature, or would be inferior to MasterDrive. Those broader comparisons need objective tests and user evaluation.

## MasterDrive exact-head truth

At [development commit `75ae4960`](https://github.com/thebrazenbeard/masterdrive/commit/75ae4960db8c7fb32d7be03474f9d5ca587c36c8), source inspection supports these narrower descriptions:

- **Implemented source:** an explicit read-only `-export ID=/absolute/directory` agent listening on loopback, with a legacy bearer token or optional per-share bearer grants; local browser controller, manually registered HTTPS origins, listing and downloads. See [agent CLI source](https://github.com/thebrazenbeard/masterdrive/blob/75ae4960db8c7fb32d7be03474f9d5ca587c36c8/cmd/masterdrive/agent.go) and [controller source](https://github.com/thebrazenbeard/masterdrive/blob/75ae4960db8c7fb32d7be03474f9d5ca587c36c8/internal/server/server.go).
- **Experimental G1 source:** immediate-children filename search across explicit host/share targets, tightly bounded fanout and pagination, with a single selected share/folder exposed in the UI. It is not an index, recursive crawl, sync service, or search across all attached drives. See [search contract](../search-v1.md).
- **Tests versus real systems:** source and CI tests are a distinct evidence class. At this research snapshot, a documented, owner-authorized real two-machine tailnet qualification had **not been performed**. See [two-host runbook](../testing/two-host.md).
- **Not implemented:** cryptographic device pairing with owner approval, effective durable revocation, automated drive discovery, native Explorer drive letter/Linux mount, stable immutable file-version reads, managed copying, synchronization, replication, cross-host write locks or allocatable pooled capacity. See [mount research](../mounts.md) and [pooling research](../pooling.md).

Those statements are about this exact candidate source. They are not proof of installed, released, tested-on-Lappy, or production behavior.

## A falsifiable product direction

The plausible distinguishing promise is **one explicitly authorized, inspectable view of files from heterogeneous sources without moving or duplicating their contents by default**, independent of whether the underlying private path uses Tailscale Serve, Taildrive/WebDAV, a local folder, or another approved adapter.

That sentence contains three unbuilt requirements:

1. **Source adapters:** a stable capability contract covering identity, enumeration, stat, version-pinned open and access denial; `unsupported` must stay an ordinary result instead of pretending all backends are equivalent.
2. **Owner-controlled governance:** enrollment with local confirmation, per-export permissions, transparent offline/stale markers, effective revocation and auditable capability use. Tailnet membership by itself is not MasterDrive share authorization.
3. **Cross-source catalog value:** useful explicit search/provenance across multiple authorized roots, with zero background scan until separately opted in, no deceptive “all capacity pooled” label, and measured latency/accuracy versus simply opening Taildrive or network shares.

**Pilot falsifier:** If two real Windows/Linux owners prefer Taildrive or existing shares for the same tasks, and cross-source catalog/search adds no independently measurable usability benefit, differentiation fails **for those participants**. Broader market demand remains untested; a larger study or a narrower integration path would then be more defensible than proclaiming superiority or novelty.

## Practical sequence, with no feature inflation

1. **Now — test the read-only prototype:** exact-head CI, adversarial source authorization tests, non-sensitive loopback fixture, and an owner-approved real two-host test. Mark these as source tests or real-host evidence separately.
2. **Next — identity and failure semantics:** local owner confirmation, identity-bound grants, revocation that halts subsequent requests, offline/stale truth, file identity/version metadata, and safe restart recovery. Avoid permanent tokens or silent whole-drive exports.
3. **Then — one read-only mount candidate:** only after the version-pinned read contract exists, measure rclone/WinFsp/Linux FUSE interoperability on disposable folders; preserve all unsupported operations as errors. No automatic privileged driver install.
4. **Separately — validate user demand for a multi-source catalog:** compare specified tasks against Taildrive, Syncthing and ordinary shares, including offline behavior, permissions, privacy, accuracy, and zero-cost operation.
5. **Much later, if justified — writes and pooling:** require an explicit durability model, metadata/placement quorum, transaction and recovery semantics, replication or coding policy, cross-platform filename/ACL plan, reserve calculations, and independent failure-injection tests. Do not describe remote browse capacity as writable pooled capacity.

**Constraints:** Windows and Linux only; owner opt-in exports; no default public exposure; no spending or vendor subscription required by MasterDrive's core source. Upstream service terms and platform capabilities may differ, and the project license choice remains unresolved; see [licensing research](../licensing.md). No installation or network-policy changes are authorized by this document.

## Traceability and unresolved questions

This analysis aligns with the earlier [architecture decisions](decisions.md) and [prior-art survey](prior-art.md). Each upstream claim above links to its provider's documentation, retrieved 2026-11-02. MasterDrive implementation claims point to an immutable development commit. The open questions are whether a provider-neutral catalog creates enough value, which transports can be supported without new authority assumptions, and which immutable-read contract can realistically be implemented before an OS mount.

**Verdict:** Existing-tool overlap **confirmed**; current MasterDrive source scope **confirmed at the inspected head**; differentiated demand and future reliability **UNKNOWN**. This is an internal source/documentation review pending separate independent verification, not market proof.
