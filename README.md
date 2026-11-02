# MasterDrive

**Your computers have drives. MasterDrive gives their approved files one place to browse.**

MasterDrive is an experimental, publicly developed storage federation for Windows and Linux. You choose folders to expose on each computer; a local controller shows those shares in one browser and lets you download files. The files stay on the computer that owns them.

Use [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve) or another approved private HTTPS route to connect computers. MasterDrive is the **storage layer**, not a VPN. It is not yet a single writable virtual disk.

> **Status: read-only developer prototype, not a production storage service.** Source and automated tests exist; secure two-device onboarding, identity-based sharing/revocation, and real multi-host qualification are still required. Don't use it as a backup or your only way to access important files.

## Try it without sharing a real drive

Prerequisite: **Go 1.27 or newer, with current security patches**. Clone this repository, then use its disposable smoke test:

| Platform | Command from repository root |
| --- | --- |
| Linux | `bash scripts/smoke-local.sh` |
| Windows PowerShell 7 | `pwsh -File ./scripts/smoke-local.ps1` |
| Windows PowerShell 5.1 | `powershell.exe -NoProfile -File .\scripts\smoke-local-winps51.ps1` |

The smoke test creates a temporary test folder, builds MasterDrive, starts a local agent and controller, verifies authentication and a file download, then stops its processes and removes its own test data. It does **not** share a real drive, configure Tailscale, or qualify remote access.

Run the development checks with:

```sh
go test ./...
go vet ./...
```

GitHub Actions also runs tests on [Windows and Ubuntu](https://github.com/thebrazenbeard/masterdrive/actions), with Linux race detection and real local-process smoke tests. The status of the latest workflow run—not this paragraph—is the current test result.

## What works today

| Component | Current behavior |
| --- | --- |
| Source agent | Explicit, named, **read-only** directory exports; bearer token required on every request |
| Controller | Browser dashboard, host/share listing, paginated folders, and file downloads |
| Host catalog | Local `add-host` / `remove-host` commands; validated persisted registration without storing peer secrets |
| Transfer | Single-file streaming, one byte range, bad-range rejection, truncation detection; no verified resume across file changes |
| Network | Loopback listeners by default; only HTTPS is accepted for registered remote origins (loopback HTTP allowed for tests) |

Hosts are **not** automatically discovered or continuously health-checked. The UI is an inventory of configured sources, not evidence that all their disks are online.

## Run with a folder you explicitly approve

This is a manual developer workflow. Start with a single machine; only move to a second machine after you have an approved private transport and have reviewed the [security gates](docs/security/acceptance.md).

1. On the computer owning the folder, choose a directory containing only files you mean to share. Provision a unique, random token of **at least 32 characters** in `MASTERDRIVE_AGENT_TOKEN` using your normal secrets mechanism. Run:

   ```sh
   go run ./cmd/masterdrive agent -export documents=/absolute/path/to/approved-folder
   ```

   The agent listens on `127.0.0.1:8789`. It serves no folders unless you explicitly name an export.

2. In another terminal on that same computer, register the agent:

   ```sh
   go run ./cmd/masterdrive add-host -name local-computer -url http://127.0.0.1:8789
   ```

   Copy the 32-character **host ID** printed by the command. Configure `MASTERDRIVE_PEER_TOKENS_JSON` for the controller as a JSON object mapping this ID to the **same agent token**. Inject this secret securely; don't paste live credentials into Git, logs, screenshots, or shared shell history.

3. Start the controller from the repository root:

   ```sh
   go run ./cmd/masterdrive controller
   ```

   Open the exact `http://127.0.0.1:8788/#session=...` address printed on startup. The URL fragment activates a local, HttpOnly browser session; peer tokens stay on the controller.

For another computer, run its agent there, put **only that agent** behind a private, access-controlled HTTPS route such as Tailscale Serve (never Funnel), and register the route's HTTPS origin. See the [Windows guide](docs/windows.md) and [architecture](docs/architecture.md) before any two-computer trial. Do not open agent ports to the public internet.

**Operational limitations:** The controller reads its peer-token map **once at startup**. Newly added hosts appear in the catalog, but their shares return `peer_token_missing` until you restart the controller with their credentials. `remove-host` removes a catalog entry; it does **not** revoke a remote token. Source tokens are currently agent-wide, not per-share identity grants.

## What MasterDrive is not yet

- **Not disk pooling:** no combined free-space allocator, RAID, erasure coding, replication, placement, or automatic recovery.
- **Not a mounted filesystem:** no Explorer drive letter or FUSE/WinFsp mount yet.
- **Not sync or backup:** no mirroring, offline copies, or durability guarantee.
- **Not fully paired:** no automatic discovery, owner-approved per-device pairing, share-level grants, effective credential rotation, or native tailnet identity verification yet.
- **Not production-qualified:** no completed end-to-end encrypted two-host Windows/Linux trial, reparse-point hostile testing, or resilience benchmark.

Those are distinct milestones—not features that a unified file list magically provides. Follow [the roadmap and acceptance gates](https://github.com/thebrazenbeard/masterdrive/issues/2).

## Tailscale and other prior art

[Tailscale Taildrive](https://tailscale.com/docs/features/taildrive) already provides private folder sharing over a tailnet, currently documented as alpha. MasterDrive aims to add a transport-independent storage catalog and eventually search, policy, mount and placement capabilities. Taildrive's `100.100.100.100:8080` endpoint is **device-local**; it is not a remote peer URL to paste into MasterDrive. The [interoperability notes](docs/interop/taildrive.md) distinguish a possible Taildrive browser from native MasterDrive agents through Serve.

Research also considers [rclone mount](https://rclone.org/commands/rclone_mount/), [WinFsp](https://winfsp.dev/doc/), [Syncthing](https://docs.syncthing.net/users/syncing), and [CephFS](https://docs.ceph.com/en/latest/cephfs/), which solve different storage problems.

## Development and security

- [Protocol and boundaries](docs/protocol-v1.md)
- [Threat model](docs/security/threat-model.md)
- [Acceptance and live-test gates](docs/security/acceptance.md)
- [Windows setup and teardown](docs/windows.md)
- [Research and design decisions](docs/research/decisions.md)

All automated tests are **local or simulated** unless explicitly identified otherwise. Real private-network identity, permissions, revocation, and native filesystem edge cases remain release blockers. [Open an issue](https://github.com/thebrazenbeard/masterdrive/issues) to help test or discuss the design.

**License:** No license has been selected. Public source availability does not by itself grant redistribution or derivative-work rights.
