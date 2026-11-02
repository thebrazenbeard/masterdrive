# Local smoke tests (Windows and Linux)

These scripts are deliberately limited to **loopback** and to a new, uniquely named temporary fixture. They are integration checks, not installation, public serving, resilience qualification, or a test of real user drives.

## Requirements

- The integrated `masterdrive` source with `go.mod`, `internal/agent`, `internal/catalog`, `internal/server`, `cmd/masterdrive`, and `web/` present.
- A security-patched Go toolchain supported by the project's `go.mod` (currently Go 1.27 or newer).
- Windows: PowerShell 7 or newer. Linux: Bash, curl, and common POSIX utilities.
- Two unused local TCP ports, default 18788 (controller) and 18789 (agent).

Run from a clean repository checkout, without administrator/root elevation. Do not select directories containing private files: the scripts automatically create a harmless one-file export fixture.

Linux:

```sh
bash scripts/smoke-local.sh
```

Choose different loopback ports if those ports are occupied:

```sh
MASTERDRIVE_SMOKE_CONTROLLER_ADDR=127.0.0.1:28788 \
MASTERDRIVE_SMOKE_AGENT_ADDR=127.0.0.1:28789 \
bash scripts/smoke-local.sh
```

Windows PowerShell 7:

```powershell
pwsh -NoProfile -File ./scripts/smoke-local.ps1
```

Custom ports:

```powershell
pwsh -NoProfile -File ./scripts/smoke-local.ps1 -ControllerPort 28788 -AgentPort 28789
```

Each script:

1. Builds `masterdrive` into a private temporary directory.
2. Creates one small `hello.txt` fixture and a cryptographically random runtime bearer token (not printed or committed).
3. Starts an agent that exports only that temporary directory on `127.0.0.1`.
4. Explicitly registers the peer in a temporary catalog.
5. Starts the controller, captures its ephemeral session fragment in memory, and checks that unauthenticated API calls return HTTP 401.
6. Activates the session, lists hosts/shares/files, downloads the fixture and compares bytes.
7. Stops only the child processes it started and removes only its generated temporary directory, including the temporary cookie jar.

Expected terminal result on success:

```text
PASS: authenticated local agent, registry, controller, session, listing, and download.
```

If a test fails, examine the failure and fix the source before connecting real storage. The scripts intentionally do not perform uploads, deletes, repairs, background scans, remote connection attempts, or changes to firewall/Tailscale configuration.

## What these tests do **not** prove

- Remote network encryption, NAT traversal, tailnet grants, or peer identity beyond a test bearer token.
- Real Windows filesystem reparse points, SMB/NFS mounts, drive hot-unplug, heavy directories, or concurrent writes.
- Production-grade multi-user authorization, disaster recovery, or distributed-pool durability.

For manual remote tests, choose an explicitly approved test directory, run the agent loopback-only on a second device, and expose it through an authenticated private HTTPS reverse proxy. [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve) is one transport option and must be configured with appropriate tailnet grants. **Do not use Tailscale Funnel or expose the agent publicly.** Record exact host identity, toolchain, network path, test time, and logs without secrets; a passing local fixture is not equivalent to a verified remote storage session.
