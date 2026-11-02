# MasterDrive

**Your drives, wherever your computers are. One private place to browse and download the files you chose to share.**

MasterDrive is a publicly developed **Windows/Linux storage mesh prototype**: a lightweight agent exposes an explicitly approved folder from one computer, and a local browser controller can browse and download that folder alongside exports registered from other computers. Files remain on the machine that owns them.

Think **Tailscale's private-network idea applied to hard-drive access**. Tailscale Serve or another owner-approved private HTTPS gateway supplies secure connectivity; MasterDrive supplies the catalog, file-serving API, browser explorer, and access checks. **MasterDrive is not itself a VPN.**

> **Experimental, read-only developer prototype.** Local automated tests exercise the code. MasterDrive has **not** passed a documented real two-host tailnet qualification. It does not yet mount a virtual disk, combine capacities, synchronize or back up files, or provide production-ready device pairing. Do not rely on it for the only copy of anything important.

## Try MasterDrive safely in a few minutes

You need a **security-patched Go 1.27+** toolchain. Clone this repository and, from its root, run a disposable **loopback-only** test. No drive, tailnet, or administrator access is needed.

| System | Run from the repository root |
| --- | --- |
| Linux | `bash scripts/smoke-local.sh` |
| Windows, PowerShell 7 | `pwsh -File ./scripts/smoke-local.ps1` |
| Windows, PowerShell 5.1 | `powershell.exe -NoProfile -File .\scripts\smoke-local-winps51.ps1` |

The script creates a temporary fixture, builds the program, starts one test agent and one controller, checks authorization and a file download, and cleans up its own fixture and processes. It **does not** configure Tailscale or prove remote sharing.

Developers can run:

```sh
go test ./...
go vet ./...
```

[GitHub Actions](https://github.com/thebrazenbeard/masterdrive/actions) runs the Go checks on Windows and Ubuntu, Linux race tests, local-process smoke tests, and browser-code unit tests. Check the latest run for the current result; a green local CI run is not a live storage-mesh certification.

## What the code actually does

| Capability | Current state |
| --- | --- |
| Explicit exports | **Implemented:** read-only directories, each assigned a share ID; nothing auto-shared |
| Private agent | **Implemented:** `127.0.0.1:8789` by default, authenticated listing, file metadata, ranged downloads, path confinement |
| Local controller | **Implemented:** `127.0.0.1:8788` by default; browser session, registered host/share explorer, pagination, downloads |
| Host registry | **Implemented:** manually `add-host`/`remove-host`; registered HTTPS origins (HTTP only for loopback testing) |
| Scoped credentials | **Experimental G1:** optional bearer tokens limited to specifically named shares; legacy all-export token still supported |
| Filename search | **Experimental G1:** explicit search of immediate children in the selected share/folder, with bounded work and pagination; no background crawl |
| Remote operation | **Not qualified:** an approved private HTTPS bridge is required; no completed real multi-host proof |
| Explorer drive letter / Linux mount | **Not implemented:** no WinFsp, FUSE, SMB, or WebDAV-backed OS mount yet |
| Pooled/writable storage | **Not implemented:** no combined allocator, replication, parity, automatic placement, or offline file availability |

The UI's *Filter loaded files* box searches the entries already fetched into the browser. *Find names in this folder* makes a bounded source request; it is **not** recursive, full-text, or an index of every drive. A missing/offline host may leave results incomplete. Hosts are registered manually, not discovered or continuously monitored.

## Run your first approved folder

This is a manual, single-machine developer walkthrough. Use a non-sensitive test directory with only the files you approve. Never experiment by exporting an entire disk, home directory, NAS root, credentials folder, or an unrelated user's files.

**1. Start an agent** in a terminal on the computer that owns that directory. First provision `MASTERDRIVE_AGENT_TOKEN` with a **unique, cryptographically random token of at least 32 characters**, through your normal secret-management mechanism; don't put real secrets on a command line or in Git.

```sh
go run ./cmd/masterdrive agent -export demo=/absolute/path/to/approved-folder
```

`-export` is repeatable (`ID=/absolute/directory`), but each folder must be opted in. The agent listens on `127.0.0.1:8789` and serves read-only files after bearer authentication.

**2. Register that local agent** in another terminal:

```sh
go run ./cmd/masterdrive add-host -name demo-computer -url http://127.0.0.1:8789
```

Save the printed 32-character host ID. Supply the controller with `MASTERDRIVE_PEER_TOKENS_JSON`, a JSON object whose **key is the exact host ID** and whose **value is the agent token**. Inject it through your secret manager; it is a process environment setting, not a repository config file.

**3. Start the browser controller** from the checkout root:

```sh
go run ./cmd/masterdrive controller
```

Open the **exact** loopback URL printed by the controller, including its `#session=...` fragment. The fragment contains a sensitive **single-use** bootstrap secret: the first successful activation exchanges it for an HttpOnly browser cookie; the link cannot be redeemed again. If activation succeeds but its response is lost, restart the controller to get a new link. Existing session cookies work until that controller stops. Keep the URL private, and never expose the browser controller on a LAN or through a public tunnel.

The controller reads peer tokens at **startup**, not whenever its environment changes. The host catalog can be updated separately, but a newly registered host cannot be accessed until the controller is restarted with its token entry. `remove-host` removes a catalog record; it **does not revoke a source token**.

For Windows, see [setup, preflight, and conservative cleanup](docs/windows.md). For a second machine, require the permission of both device owners and the private-network administrator; read the [two-host qualification plan](docs/testing/two-host.md) and [security gates](docs/security/acceptance.md) first. Expose only the **agent**, over an access-controlled private HTTPS route such as [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve)—**never Funnel or a public port**. Network membership alone does not grant MasterDrive file access.

### Optional: restrict each credential to particular shares (G1)

Instead of the legacy `MASTERDRIVE_AGENT_TOKEN`, the source agent accepts `MASTERDRIVE_AGENT_GRANTS_JSON`. Set **exactly one** of those environment variables; completely **unset** the other. An illustrative JSON value is:

```json
[
  { "bearer_token": "<unique-random-secret-A>", "share_ids": ["documents"] },
  { "bearer_token": "<unique-random-secret-B>", "share_ids": ["photos"] }
]
```

Those are **placeholders**, not usable secrets. The granted `share_ids` must match explicit `-export` IDs. An authenticated token sees only its allowed shares; other share IDs return `404`. Configure the controller's host-token mapping with the appropriate single token for that source. These grants are **bearer credentials, not cryptographic device identity or an invitation/pairing system**. Revocation currently requires stopping transfers/listeners and restarting the agent without the old grant, updating/restarting controllers, and independently reviewing transport permissions. See [scoped grants](docs/security/scoped-grants-cli.md).

### Optional: search names, not disks (G1)

Select one share and folder in the UI, then use **Find names in this folder**. The search examines at most 256 immediate entries per request and returns a bounded page; the controller API can also query explicitly named host/share targets with strict fanout limits. Large or changing folders may return incomplete or duplicate results; no files are indexed in the background. Search terms are sent in GET query strings and may appear in HTTP access logs, so **do not search for secret names** on a route you do not control. Read the [search contract and limitations](docs/search-v1.md).

## What comes next

The storage mesh should progress through **owner-approved pairing and revocation**, verified private two-host transfers, honest per-device capacity reporting, and a native read-only mount before considering writable placement or pooling. An attractive unified file browser is not evidence that disconnected disks form a reliable single disk.

- [Architecture and current trust boundaries](docs/architecture.md)
- [Source and controller protocol](docs/protocol-v1.md)
- [Two-host Windows/Linux test runbook — **not an executed result**](docs/testing/two-host.md)
- [Read-only mount feasibility — **design only**](docs/mounts.md)
- [Capacity inventory — **not a live endpoint**](docs/capacity.md)
- [Pooling and replication alternatives — **research only**](docs/pooling.md)
- [Threat model](docs/security/threat-model.md) and [acceptance gates](docs/security/acceptance.md)
- [Licensing research and unresolved choice](docs/licensing.md)
- [Prior-art decisions](docs/research/decisions.md) and [Taildrive interoperability](docs/interop/taildrive.md)
- [Roadmap and issue tracker](https://github.com/thebrazenbeard/masterdrive/issues)

### Where Tailscale Taildrive fits

[Tailscale Taildrive](https://tailscale.com/docs/features/taildrive) already offers private folder sharing on a tailnet and is useful prior art, not a substitute for MasterDrive's desired transport-independent catalog. Its `100.100.100.100:8080` WebDAV endpoint is **local to each device**; do not register it as the HTTPS URL for a remote native MasterDrive agent. [Syncthing](https://docs.syncthing.net/users/syncing) focuses on file synchronization; [rclone mount](https://rclone.org/commands/rclone_mount/) and [WinFsp](https://winfsp.dev/doc/) inform future mounts; distributed filesystems like [CephFS](https://docs.ceph.com/en/latest/cephfs/) address a substantially different reliability problem.

**License:** No outbound license has yet been selected. A public repository is not automatically permission to redistribute, modify, or incorporate its code. See [licensing research](docs/licensing.md) before reuse. Windows and Linux are the target platforms; no macOS support is planned.
