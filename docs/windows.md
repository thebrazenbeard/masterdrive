# Windows 11: safe MasterDrive setup and manual mesh qualification

Status: **developer/operator guide for the read-only prototype**, updated 2026-11-02. This is not a packaged installer, a background Windows service, an automatic Tailscale mesh, or a production deployment recipe. Nothing in the steps below silently grants access to a drive or changes tailnet policy. MasterDrive has no device-pairing UI yet.

## First, run the disposable local test

Use a regular, non-administrator PowerShell window in a complete checkout of this repository. The required source directories are `cmd/masterdrive`, `internal/agent`, `internal/catalog`, `internal/server`, and `web`.

1. Install a **current security-patched Go 1.27 or later** toolchain from the [official Go downloads](https://go.dev/dl/) and verify `go version`. A stale Go runtime can invalidate the source agent's `os.Root` path-containment assumptions.
2. Open the repository folder, then run:

   ```powershell
   # PowerShell 5.1 and PowerShell 7: inspection only, no administrator rights.
   powershell.exe -NoProfile -File .\scripts\preflight-windows.ps1
   ```

3. Run the actual loopback smoke fixture:

   ```powershell
   # Use the PowerShell edition supported by the checked-in smoke script.
   pwsh -NoProfile -File .\scripts\smoke-local.ps1
   ```

   The original `smoke-local.ps1` requires PowerShell 7 (`pwsh`) and builds a temporary executable, generates a random token, creates a one-file export, starts two loopback child processes, checks browser-session isolation and listing/download, and removes its own temporary files. **Windows PowerShell 5.1 (`powershell.exe`) cannot run that original script.** Use the dedicated `scripts/smoke-local-winps51.ps1` runner instead. Do not bypass the `#Requires -Version 7.0` line.

   ```powershell
   powershell.exe -NoProfile -File .\scripts\smoke-local-winps51.ps1
   ```

The test should print `PASS: authenticated local agent, registry, controller, session, listing, and download.` It does **not** prove any two-device network route or any actual Windows drive permission boundary. Choose alternate unused local test ports if 18788 or 18789 is already taken:

```powershell
powershell.exe -NoProfile -File .\scripts\preflight-windows.ps1 -ControllerPort 28788 -AgentPort 28789
pwsh -NoProfile -File .\scripts\smoke-local.ps1 -ControllerPort 28788 -AgentPort 28789
```

Do not run from an arbitrary working directory if the checkout is incomplete. Avoid administrator elevation and never choose an existing personal folder for a smoke fixture; the runner is designed to use only its generated temporary directory.

## Understand the four commands before using real data

| CLI command | Scope and expected behavior |
| --- | --- |
| `masterdrive agent -export photos=C:\ApprovedTestFiles` | Bind a read-only agent to `127.0.0.1:8789`, serving **only** the explicitly selected existing directory. Requires `MASTERDRIVE_AGENT_TOKEN`. |
| `masterdrive add-host -name my-pc -url https://machine.tailnet.ts.net` | Add a vetted HTTPS endpoint to the local JSON catalog. **No auto-discovery**. Plain HTTP is accepted only for loopback testing. |
| `masterdrive controller -web-dir web` | Local-only UI/API on `127.0.0.1:8788`; consumes `MASTERDRIVE_PEER_TOKENS_JSON` at **startup** and prints a secret-bearing one-time browser URL fragment. |
| `masterdrive remove-host -id <host-id>` | Remove the local registry entry. This **does not** rotate a remote agent token or revoke Tailscale grants. |

Build once from the repository root, without installing a Windows service:

```powershell
go test ./...
go vet ./...
go build -o .\masterdrive.exe .\cmd\masterdrive
```

This leaves one local `masterdrive.exe` in the checkout; remove it manually when no longer needed. You can run `go run ./cmd/masterdrive` instead if you do not want a persistent executable. The agent currently requires an explicit absolute export path; **never export `C:\`, a user profile root, secrets/config folders, or an entire mounted NAS** merely to test discovery.

## Optional: tightly scoped two-Windows-device test

Proceed only with authorization from the owners of both Windows devices and the tailnet administrator. Make a harmless test directory containing a non-sensitive fixture on the source device. Keep the controller on the operator's local desktop. This is an operator **qualification procedure**, not an automated deployment and not proof of safe multi-user sharing.

### 1. Source Windows device

On the source device, use a non-admin terminal and deliberately set a long random token in **the agent process environment** from your trusted secret mechanism. It must have at least 32 non-whitespace characters. Do not embed the token in arguments, JSON files, scripts, commits, URLs, clipboard screenshots, or terminal transcript captures. A terminal environment variable is temporary process state, **not** an OS credential vault; use a dedicated restricted session and clear it afterward.

```powershell
# C:\MasterDriveDemo must already exist and contain only approved test files.
# Before this command, have your secret manager set MASTERDRIVE_AGENT_TOKEN
# in this terminal's environment (value omitted intentionally).
.\masterdrive.exe agent -listen 127.0.0.1:8789 -export demo=C:\MasterDriveDemo
```

Do not type the literal token in a command that will be recorded in history. The CLI refuses to start without the token or the export, and the listener refuses non-loopback bindings. Confirm on the source machine that only the intended directory was opened. Start and stop this process under the interactive account that already has access to that directory.

### 2. Source-device private transport

On the same source device, have the tailnet owner verify the intended source/controller identities, HTTPS availability, grants, and that port 443 of the source device is private-only. Inspect existing Serve configuration first so another application is not displaced:

```powershell
tailscale status
tailscale serve status
```

If approved, in a **separate foreground terminal**:

```powershell
tailscale serve 8789
```

The output supplies a private `https://<machine>.<tailnet>.ts.net` URL. Use exactly that HTTPS origin, after checking it is the expected device and TLS certificate is trusted. In a new Tailscale CLI (v1.52+), `tailscale serve 8789` proxies `127.0.0.1:8789` and `Ctrl+C` stops the foreground Serve session. Do **not** use `tailscale funnel`, public port forwarding, an insecure certificate override, or `--bg` during this limited test. Do not expose the browser controller through Serve. Tailscale network access alone does **not** authorize file access: MasterDrive still requires its independent bearer token.

> Taildrive is a different Tailscale feature. Its `http://100.100.100.100:8080` WebDAV endpoint is device-local; it is **not** the remote URL to register for a native MasterDrive agent.

### 3. Controller Windows device

On the controller device, install/build the same source revision. Verify an approved encrypted tailnet route to the source and register the exact HTTPS origin printed by Serve:

```powershell
.\masterdrive.exe add-host -name approved-demo-source -url https://SOURCE-NAME.TAILNET.ts.net
```

Record the 32-hex-character **host ID** printed by `add-host`. The controller must receive the source's bearer token over a separate approved secret channel. Populate its process environment with a JSON object where the key is **that exact host ID** and the value is **that exact agent token**. For example, after your secret manager has supplied `$agentSecret` without echoing it:

```powershell
$hostId = '<host-id-from-add-host>'
$peerTokens = @{}
$peerTokens[$hostId] = $agentSecret
$env:MASTERDRIVE_PEER_TOKENS_JSON = ConvertTo-Json -InputObject $peerTokens -Compress
.\masterdrive.exe controller -listen 127.0.0.1:8788 -web-dir web
```

The controller prints a local URL containing `#session=...`; open that **exact one-time link in a browser on the controller machine**, rather than the bare `http://127.0.0.1:8788` URL. The fragment is an authentication secret until consumed. Do not forward, photograph, or publish it. Browse only the test share and verify that the file bytes match the source fixture. Verify separately that an unapproved tailnet device and a wrong bearer token are both denied; a successful browsing session alone is not a security qualification.

**Process snapshot caveat:** the controller reads `MASTERDRIVE_PEER_TOKENS_JSON` when it starts. If you add a host or rotate a token, restart the controller with an updated token map; merely editing the environment in another shell will not reconfigure an existing controller process. The controller reloads catalog changes made by separate `add-host`/`remove-host` processes; that does **not** reload credentials. Catalog writes use a bounded exclusive lock; a process crash may leave a stale lock requiring careful manual recovery. Do not share the catalog directory over an unqualified network filesystem.

### 4. Tear down and review

1. Stop the controller and source agent with `Ctrl+C` in their own terminals; shut down the foreground `tailscale serve` terminal with `Ctrl+C`. Verify `tailscale serve status` contains no unintended active mapping. Do **not** run `tailscale serve reset` merely to clean up a test: it can erase other configured Serve endpoints.
2. Remove only the test host from the controller catalog with `masterdrive remove-host -id <host-id>` when appropriate. This is catalog cleanup, not remote credential revocation.
3. Clear test secrets from their shells using `Remove-Item Env:\MASTERDRIVE_AGENT_TOKEN -ErrorAction SilentlyContinue`, `Remove-Item Env:\MASTERDRIVE_PEER_TOKENS_JSON -ErrorAction SilentlyContinue`, and `$agentSecret = $null` in any controller session that used those variables. Dispose of secrets in your approved store and rotate them if distributed more widely than intended. Clearing environment variables does not revoke secrets already copied into another process.
4. Record Windows and Go versions, code revision, Tailscale version, explicit test export, host ID, expected policy principal, source and controller identities, transport URL, and pass/fail outcomes **without logging tokens or session fragments**.

## Current limitations and release gates

- There is no unattended Windows service installer, elevation strategy, persistent secure key storage, approval/pairing UI, device identity binding, source-side key rotation API, share-level ACL, or audited revocation flow.
- Browser session is local-only; do not expose its API through a LAN, reverse proxy, Taildrive share, or public tunnel.
- The read-only export uses Go's `os.Root`, but needs Windows-specific NTFS junction/reparse point, symlink, share-ACL, hot-unplug, file mutation, and concurrent download tests before real user drives are recommended.
- Large/ranged downloads need file-version integrity validation to support safe resumes across mutations; no production backup, sync, disk pooling, writable drive, availability, or durability promise exists.
- Windows PowerShell 5.1 may be installed by default alongside PowerShell 7; check `$PSVersionTable.PSVersion` and invoke `powershell.exe` or `pwsh` explicitly. Do **not** globally lower execution policy to run a project script. Follow device policy and review scripts before execution.

Primary references: [Go Windows installation](https://go.dev/doc/install), [Go releases](https://go.dev/dl/), [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), [Serve CLI including foreground/background and disable semantics](https://tailscale.com/docs/reference/tailscale-cli/serve), [Tailscale grants](https://tailscale.com/docs/features/access-control/grants), [Taildrive](https://tailscale.com/docs/features/taildrive), [PowerShell execution policies](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_execution_policies?view=powershell-5.1).
