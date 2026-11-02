# MasterDrive distributed pooling — frozen research record

Date: 2026-11-02 (UTC). Scope: future G3 architecture, **not implementation**.

## Preregistration — frozen before external retrieval

Question: What minimal architecture and evidence gates would let MasterDrive legitimately evolve from read-only federation into a safe, writable, durable distributed storage pool across untrusted-to-fail Windows/Linux nodes?

Hypotheses:

- **H1 — distinction:** A unified browser/catalog cannot guarantee data survival after a disk failure. Prediction: established distributed filesystems document separate placement, replication/erasure, metadata consistency, and failure recovery mechanisms. Falsifier: an authoritative system contract demonstrates durability solely from a federated filename catalog.
- **H2 — CephFS:** A real POSIX-style distributed filesystem delegates file metadata and data placement to separate coordinated subsystems. Prediction: current official CephFS docs describe MDS and RADOS-backed data and Ceph CRUSH placement/failure domains. Falsifier: official docs specify an unmanaged collection of independent drives as sufficient for a durable writable pool.
- **H3 — JuiceFS:** A metadata service plus object storage can expose filesystem semantics, but the metadata transaction/failure model and object backend durability become critical. Prediction: official docs enumerate metadata engines, object storage, consistency and caching tradeoffs. Falsifier: no external metadata/storage dependency or transactional update is required.
- **H4 — SeaweedFS:** A different model can minimize metadata hotspots and optionally use erasure coding, but volume-level data protection and filer metadata remain explicit. Prediction: official docs separate volume server, master, filer, erasure coding/recovery. Falsifier: erasure/protection is automatic for every object without explicit topology/configuration.

Planned methods, all **confirmatory source inspections** (not live filesystem tests):

- **D1:** Retrieve current official CephFS and CRUSH placement/failure-domain documentation. Map mechanisms to MasterDrive requirements.
- **D2:** Retrieve current official JuiceFS architecture/consistency and recovery documentation. Map metadata and object dependencies.
- **D3:** Retrieve current official SeaweedFS architecture/erasure/recovery documentation. Map volume and filer risks.
- **L1:** Inspect MasterDrive local module and current architecture to establish existing behavior and dependency versions before applying external contracts.

Stopping rule: three bounded official-source lookup groups and one design ADR with falsifiable release gates; no live mounts, host mutation, network probe, dependency install, or unrequested implementation. Independent verifier verdict is required before calling research closed; until then status is **pending independent verification**.

## Evidence log (append-only)

- **E0, local, preregistration:** `masterdrive-build/go.mod` declares `github.com/thebrazenbeard/masterdrive`, `go 1.27.0`, no dependencies. Current source is a read-only agent/controller prototype, not a pooling runtime. Inspection command `sed -n '1,110p' masterdrive-build/go.mod` exit 0. Decisive lines: `module github.com/thebrazenbeard/masterdrive` and `go 1.27.0`. No external storage engine dependency to version-match.
- **E1, D1, official CephFS (retrieved 2026-11-02):** https://docs.ceph.com/en/latest/cephfs/ ; source `turn60view0` lines 11–14: POSIX-compliant CephFS sits on RADOS; metadata is in a separate RADOS pool served by MDS; client data I/O goes to RADOS; coordinated MDS metadata mutation is journaled to RADOS. The `latest` channel was current when retrieved but is not a pinned Ceph release.
- **E2, D1, official Ceph CRUSH (retrieved 2026-11-02):** https://docs.ceph.com/en/latest/rados/operations/crush-map/ ; source `turn62view0` lines 15, 593–599, 694–721: configured failure domains place replicated/EC shards across hosts/racks; EC profile `k`,`m` and failure-domain choices influence placement. The page explicitly states it is development-version documentation. No Ceph version installed in MasterDrive.
- **E3, D2, official JuiceFS architecture (retrieved 2026-11-02):** https://juicefs.com/docs/community/architecture/ ; source `turn60view2` lines 67–83, 93–103: metadata engine holds namespace, mapping, references and sessions; data blocks live separately in object storage, with clients processing chunks/slices; consistency policy interacts with cache. This is current Community documentation, not a pinned dependency.
- **E4, D2, official JuiceFS cache (retrieved 2026-11-02):** https://juicefs.com/docs/community/guide/cache/ ; source `turn58search1` and `turn60view3` lines 61–63 and 184–194: default metadata close-to-open; writeback mode can acknowledge metadata before asynchronous data upload to object store, creating explicit data-loss/availability tradeoffs. No JuiceFS dependency installed in MasterDrive.
- **E5, D3, official SeaweedFS README (retrieved 2026-11-02):** https://github.com/seaweedfs/seaweedfs/blob/master/README.md ; source `turn62view2` lines 1529–1536: master tracks volume placement, volume server stores and replicates/ECs blobs, filer maps files/directories via separate metadata store. README at `master` is moving head, not version-pinned.
- **E6, D3, official SeaweedFS design note (search description only, retrieved 2026-11-02):** https://github.com/seaweedfs/seaweedfs/wiki/SeaweedFS_Architecture.pdf ; source `turn58search36` describes replication and erasure coding. The PDF itself was **not independently retrievable**, so this evidence remains limited to search metadata; the independently retrieved current README E5 specifies warm-data erasure coding. Neither source is evidence of any MasterDrive cluster behavior.
- **E7, L1, local architecture:** `architecture-masterdrive-work/docs/architecture.md` inspected with `sed -n '1,58p' ...` exit 0 on 2026-11-02. Decisive sentence: “The storage fabric is federated access, not a distributed disk, RAID array, backup system, or pooled allocator.” The source architecture lists explicit opt-in exports and no data placement or replication. This is source-design evidence, not a runtime validation.

## Results and deviations

Confirmatory source inspections **D1/D2/D3 completed**. H1's prediction is supported by E1–E6 (each architecture separately addresses placement, metadata and data durability rather than treating a catalog as a pool). H2 is supported by E1/E2; H3 by E3/E4; H4 by E5/E6. No falsifier observed in the inspected primary materials. These results prove only the reference architectures' documented contracts, **not** their behavior in a MasterDrive deployment.

**Deviations:** Ceph's `latest` CRUSH page explicitly says development version; SeaweedFS `master` is a moving branch. Pinned stable release compatibility remains a future experiment once an actual backend/version is selected. No live experiments were run; no runtime result is being promoted to confirmation.

## Independent verification

Pending. No self-verification counts as closure.

### Independent verification addendum — 2026-11-02

- Reviewer: native `/root/docs_verifier`. Independently retrieved [CephFS](https://docs.ceph.com/en/latest/cephfs/) (separate RADOS metadata pool and MDS), [Ceph CRUSH](https://docs.ceph.com/en/latest/rados/operations/crush-map/) (failure-domain-aware replica placement), [JuiceFS architecture](https://juicefs.com/docs/community/architecture/) and [cache](https://juicefs.com/docs/community/guide/cache/) (separate metadata engine/object storage and writeback tradeoffs), and [SeaweedFS official README](https://github.com/seaweedfs/seaweedfs/blob/master/README.md) (volume servers and filer separation; hot-data replication and optional background warm-data erasure coding). These are current upstream, unpinned documentation pages; the Ceph `latest` pages are explicitly development-version docs.
- **Verdict:** `PARTIALLY CONFIRMED`: core conceptual architecture and explicit rejection of “catalog = durable pool” survive source falsification. Whether any candidate is compatible with intermittent home devices, or any actual pooling/consensus/durability behavior exists in MasterDrive, remains **UNTESTED**. No backend was selected, installed, run or benchmarked.
