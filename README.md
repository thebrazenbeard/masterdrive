# MasterDrive

**A private storage mesh for Windows and Linux.** Register your devices, explicitly share selected folders, and browse them through one unified interface—without copying everything to a central NAS.

Think *Tailscale for drives*, with a layer for storage visibility and management. MasterDrive does **not** implement its own VPN. It is designed to use an approved, encrypted private network such as Tailscale Serve, while keeping the storage layer transport-agnostic.

> **Project status:** early read-only prototype. This is not a RAID array, backup system, replication service, cloud synchronization tool, kernel filesystem, or production-hardened remote file server. It does not yet pool free capacity into a writable virtual disk. Files stay on their originating machines.

## Current vertical slice

- Opt-in node agent: publish only named read-only directories on a given Windows or Linux machine.
- Local controller: explicitly register peers, present their shares in a browser, list folders, and download individual files.
- Persistent host registry with no stored peer tokens; credentials are supplied to the controller at runtime.
- Peer traffic is expected over TLS on a private network. Local listeners bind to loopback by default; the application refuses public-interface listeners.
- No auto-discovery, destructive actions, upload, remote execution, replication, or background disk scans.

## Prerequisites

- Go 1.27 or newer, with a security-patched runtime for `os.Root` path confinement.
- Windows or Linux machines with the directories you intend to export.
- For access between devices: an existing private encrypted overlay or authenticated TLS reverse proxy. [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve) is one option; **use Serve, not Funnel**.

## Run a local smoke test

The examples use two terminals on one machine. Replace the exported directory with an existing absolute directory that contains files you're willing to expose read-only. Use a unique cryptographically random token of at least 32 characters; do not commit it or put it in shell-history examples.

1. In terminal A, set `MASTERDRIVE_AGENT_TOKEN` through your normal secret-management mechanism and start the node agent:

   ```sh
   go run ./cmd/masterdrive agent -export photos=/absolute/path/to/photos
   ```

   The agent listens on `127.0.0.1:8789` and requires `Authorization: Bearer <token>` on every route.

2. In terminal B, register this local peer:

   ```sh
   go run ./cmd/masterdrive add-host -name my-computer -url http://127.0.0.1:8789
   ```

   Copy the printed host ID. Set `MASTERDRIVE_PEER_TOKENS_JSON` to a JSON object mapping **that ID** to the *same* secret as terminal A, using a secret-safe environment mechanism. Then run:

   ```sh
   go run ./cmd/masterdrive controller
   ```

3. Open [http://127.0.0.1:8788](http://127.0.0.1:8788). The controller serves static files from `web/` by default; run it from the repository root or use `-web-dir`.

For actual remote access, run the agent on the other device, use a private HTTPS proxy to its loopback listener (for example `tailscale serve 8789` on that device), and register the HTTPS URL that the private network reports. Ensure tailnet policy limits access to the intended devices. MasterDrive still requires its independent bearer token; network membership alone is not file authorization.

### Commands

| Command | Purpose |
| --- | --- |
| `agent -export id=/absolute/path` | Opt in a local read-only directory; repeat `-export` for multiple shares |
| `add-host -name label -url https://private-peer` | Add or update a known peer in the local registry |
| `remove-host -id <id>` | Revoke a locally registered peer |
| `controller -listen 127.0.0.1:8788` | Serve the local GUI and read-only API |

Use `-data-dir` on the controller/registry commands to select a different local registry location. Never put secrets in repository files or URL query parameters.

## Architecture

```text
Windows / Linux device A                 Windows / Linux device B
  opt-in folders                           opt-in folders
        |                                        |
  MasterDrive node                        MasterDrive node
        |                                        |
        +-------- private TLS mesh --------------+
                         |
                   local controller
                    |          |
             host registry     web GUI
```

The controller is a **catalog and read-only gateway**, not a central data owner. Each source machine retains the authority to expose or withdraw its own directories. A disconnected source is unavailable; it is not silently replaced with stale data. Backend tokens stay server-side.

## Roadmap and safety gates

1. **MVP read-only federation:** secure listing and download, error states, tests, Windows/Linux runtime exercises.
2. **Private mesh onboarding:** identity-based pair/revoke, transport abstraction, explicit authorized discovery, persistent agent services and usability testing.
3. **Unified namespace and search:** cross-host metadata index with privacy settings and stale/offline markers, accurate capacity measurement, Taildrive/SMB adapters where appropriate.
4. **Writes and mounted drives:** only after authorization, collision/conflict, atomicity, lock, recovery, and rollback semantics are demonstrated on both platforms.
5. **Actual pooling:** optional placement, replication, erasure coding, checksums, durability proofs, rebuild and failure testing. This is a separate product milestone—not implied by the current GUI.

See `docs/` for the research comparison, protocol contract, and threat model as they land. [Taildrive](https://tailscale.com/docs/features/taildrive) already provides Tailscale folder sharing (currently alpha), so MasterDrive aims to differentiate through a broader, transport-agnostic storage control plane rather than duplicating that capability blindly.

## Development

```sh
go test -race ./...
go vet ./...
```

No third-party Go modules are needed in the initial build. A license has not yet been selected; reuse rights must not be assumed. Third-party projects and trademarks remain their owners' property.
