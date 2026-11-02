# MasterDrive on Windows PowerShell 5.1: local integration smoke

Windows 10/11 commonly include **Windows PowerShell 5.1** (`powershell.exe`), even
when PowerShell 7 (`pwsh.exe`) is not installed. MasterDrive provides two
equivalent local fixture tests; use the variant matching your installed shell.

## Native Windows PowerShell 5.1

From the root of an integrated MasterDrive source checkout:

```powershell
powershell.exe -NoProfile -File .\scripts\smoke-local-winps51.ps1
```

Use different ports when the default loopback ports are already occupied:

```powershell
powershell.exe -NoProfile -File .\scripts\smoke-local-winps51.ps1 -ControllerPort 28788 -AgentPort 28789
```

The script needs an installed Go toolchain compatible with `go.mod` and the
repository's `web/` directory. It invokes the native Go compiler but installs
nothing and does not need elevation. It respects the machine's configured
PowerShell script-execution policy; resolve any policy restriction through your
organization's normal process rather than changing the machine policy for the
test.

PowerShell 7 users may continue to run the existing `scripts/smoke-local.ps1`
with `pwsh -NoProfile -File .\scripts\smoke-local.ps1`.

## Scope and assertions

The PowerShell 5.1 script:

1. Checks that two specified TCP ports are available on **127.0.0.1**.
2. Creates a uniquely named directory under the OS temporary directory, builds
   a temporary `masterdrive.exe`, and writes one harmless fixture file there.
3. Generates a random 256-bit bearer secret in memory, starts an agent that
   exports **only** the temporary fixture directory, and checks its authenticated
   shares endpoint.
4. Registers the agent into a **temporary** controller catalog.
5. Starts a loopback-only controller and captures its random local browser
   session fragment from a bounded async stdout read (never prints the secret).
6. Verifies an unauthenticated API request receives **HTTP 401**, then exchanges
   the session fragment for a cookie, lists hosts/shares/entries, downloads the
   fixture, and compares SHA-256 hashes.
7. Stops only child processes started by this invocation; deletes only named
   generated files and then their empty directories, **without recursive
   deletion**. Unexpected contents or reparse points halt cleanup instead of
   traversing outside the verified temporary path. Existing listeners and
   user directories are never killed, scanned, or removed.

Expected success output:

```text
PASS: WinPS5.1 loopback agent, registration, session cookie, listing, and download.
```

Startup probes and HTTP requests have finite timeouts; test failure will not
leave a `ReadLine()` call waiting indefinitely. The script deliberately uses
Windows PowerShell 5.1/.NET Framework APIs (`ProcessStartInfo.Arguments`,
`EnvironmentVariables`, `HttpWebRequest`, and `ReadLineAsync`) and does not
require PowerShell 7-specific `Invoke-WebRequest` options or .NET 6 APIs.

## What a green result does *not* mean

This validates one machine's local, read-only control and data path. It is
**not** verification of Tailscale grants, remote TLS termination, multi-device
reachability, real-drive safety, reparse points, SMB/NFS shares, heavy loads,
or production deployment. The script never modifies Tailscale, Windows
Firewall, actual drive mappings, or host settings. Do not run it with an export
pointed at a real user directory; it supplies its own fixture automatically.

In Windows CI, run the following on a `windows-latest` runner with Go installed,
after all source components have been integrated:

```powershell
powershell.exe -NoProfile -File .\scripts\smoke-local-winps51.ps1
```

A CI pass is evidence for that runner only, not for a separately connected
laptop.
