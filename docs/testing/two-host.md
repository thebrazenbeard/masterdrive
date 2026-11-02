# Two-host MasterDrive / Tailscale Serve qualification (operator runbook)

**Status:** procedure drafted 2026-11-02; **not executed** on any host. It is for the v0.1 read-only MasterDrive source at [commit `c20fee63`](https://github.com/thebrazenbeard/masterdrive/commit/c20fee63d176e69febb0c51afa0bf6a8bfb08143). It is **not** a deployment recipe, a security certification, or evidence of a working cross-host mesh. A successful two-host run would qualify one deliberately limited configuration, not general storage safety.

MasterDrive is a local browser/controller and a separately authenticated source agent. Tailscale Serve supplies **private connectivity**, not MasterDrive share authorization. All exported bytes remain on the source. Do not use real user drives, secrets folders, drive roots, home directories, mapped NAS volumes, or irreplaceable data in this test.

## Authorities and stop conditions

Do not begin unless all three parties explicitly approve: **source-device owner** (which temporary directory will be exported), **controller-device owner** (which controller/browser and catalog will run), and **tailnet administrator** (exact network grant, its scope, and rollback). Record the identities of those parties without recording secrets.

Immediately stop if:

- Any device is not positively identified, the HTTPS origin resolves to the wrong machine, a certificate is untrusted, or a source/port is already serving something unrelated.
- The source's `8789` agent port must be reachable from a public or LAN interface, or the procedure requires `tailscale funnel`, public port forwarding, an insecure TLS override, or administrator elevation merely to run MasterDrive.
- Tailscale requests automatic tailnet-policy changes that the administrator has not reviewed and approved.
- A negative test unexpectedly succeeds, or a source listing includes a path outside the explicitly generated fixture.
- The source/controller cannot be confined to ordinary user privileges and one harmless temporary fixture.

This procedure never configures another person's tailnet by itself. The commands below are *examples for approved operators*; they were **not run** for this document.

## Record a qualification manifest first

| Record (omit credentials) | Expected evidence |
| --- | --- |
| Source and controller | Hostnames, operating systems, responsible owners, Tailscale node IDs/tags if applicable |
| Runtime | Exact MasterDrive Git SHA, `go version`, `tailscale version`, PowerShell edition where applicable |
| Scope | One generated temporary source folder, its one harmless fixture file, and a temporary controller catalog path |
| Transport | Source's expected Tailscale FQDN, Serve configuration before/after, approved grant selector/port |
| Authorization | Which *test controller* should be allowed and which independent *test peer* should be denied |
| Test output | Timestamps, HTTP status/error category, fixture SHA-256, policy test outcomes, cleanup confirmation |

Do **not** record the agent bearer token, `MASTERDRIVE_PEER_TOKENS_JSON`, controller `#session=...` fragment, cookie, screenshots containing those values, or private full file paths in a shared report.

## Gate A — local, non-network readiness on both devices

1. Obtain a complete checkout at the same Git SHA and a current security-patched Go runtime supporting `os.Root`. Use a non-admin terminal. On Windows, confirm whether the shell is `powershell.exe` (5.1) or `pwsh` (7).
2. Run the disposable local tests **before** involving a tailnet:

   | Device shell | One-machine local test |
   | --- | --- |
   | Linux | `bash scripts/smoke-local.sh` |
   | Windows PowerShell 7 | `pwsh -NoProfile -File .\scripts\smoke-local.ps1` |
   | Windows PowerShell 5.1 | `powershell.exe -NoProfile -File .\scripts\smoke-local-winps51.ps1` |

3. Capture a pass/fail without exporting a real directory. These scripts generate and clean up their own fixtures; unexpected files/reparse points cause fail-closed cleanup rather than recursive deletion.
4. Check read-only Tailscale state on each device before changing anything:

   ```sh
   tailscale version
   tailscale status
   tailscale serve status
   tailscale serve get-config --all
   tailscale funnel status
   ```

   A pre-existing Serve, Services, or Funnel mapping on the candidate source is **not** yours to reset. `serve status` and `serve status --json` show different presentations, while `serve get-config --all` includes Services configuration; inspect the appropriate views before and after the test so no existing mapping is overwritten. Record it and ask its owner for a non-conflicting dedicated test configuration. Do not run `tailscale serve reset`.

## Gate B — the tailnet administrator approves a narrow transport rule

Tailscale's current grants model separates selectors (`src`, `dst`) from network permissions (`ip`). The *shape* of a dedicated test rule is:

```json
{
  "src": ["<admin-verified controller-device selector>"],
  "dst": ["<admin-verified source-device selector>"],
  "ip": ["tcp:443"]
}
```

This is an **illustrative rule object**, **not a drop-in policy**: the administrator must determine real selectors and place a rule within an existing, reviewed policy. An email-based `src` can cover multiple devices belonging to one user; a tag has ownership/key-expiry implications. A broad existing allow-all grant can still permit a supposedly denied peer. Consequently, a rule's presence is not proof of enforcement: a distinct, unapproved test peer must fail the network reachability test below.

The owner records a reversible policy change and rollback plan. Do not replace the whole tailnet policy with a demonstration JSON snippet. This v0.1 application does **not** consume Tailscale Serve identity headers or grants as share-level authorization; tagged-client user headers may be absent, and localhost programs can present their own headers. Only the independent agent token authorizes an export.

## Gate C — source-device agent and foreground Serve

Create an **empty, uniquely named temporary directory** owned by the test operator, then put a harmless `hello.txt` fixture inside. Record its SHA-256 locally. The path must be absolute and must contain **only** the approved fixture. Avoid generating a shared token in a command line or committing it; inject a unique 256-bit random secret into this agent process through an approved secret mechanism as `MASTERDRIVE_AGENT_TOKEN`.

From the source checkout's root:

```sh
# Linux — variable already injected securely; replace the fixture path.
go run ./cmd/masterdrive agent -listen 127.0.0.1:8789 -export test=/absolute/path/to/temporary-fixture
```

```powershell
# Windows PowerShell — secret already in this process environment.
go run ./cmd/masterdrive agent -listen 127.0.0.1:8789 -export 'test=C:\Temp\YourUniqueApprovedFixture'
```

The agent must report readiness only after binding `127.0.0.1:8789`. On the source device, confirm unauthenticated access is denied:

```sh
curl --noproxy '*' -i http://127.0.0.1:8789/v1/shares
```

On Windows use `curl.exe` rather than PowerShell's historical `curl` alias. Expect HTTP **401**, not share names or file paths. Never start an agent with `-listen 0.0.0.0:8789`.

The tailnet administrator then confirms Serve/HTTPS prerequisites. Recheck `tailscale serve status` and `tailscale serve get-config --all` for an existing service on this origin. In a **separate foreground terminal on the source**, after inspecting existing mappings and obtaining authorization:

```sh
tailscale serve 8789
```

Current Tailscale Serve documentation describes an HTTPS FQDN for the local loopback proxy and foreground Ctrl+C teardown. Copy only the **HTTPS origin** printed by this *source device* (for example `https://source-name.tailnet-name.ts.net`) into the manifest. Verify that it is the intended source identity and certificate; never substitute the device-local Taildrive WebDAV address `100.100.100.100:8080`, public Funnel, or `https+insecure`.

## Gate D — controller registration and browser path

On the separate controller host, use a **new, operator-owned temporary catalog directory**. Keep it distinct from a normal MasterDrive registry. From the same source revision:

```sh
go run ./cmd/masterdrive add-host -data-dir /absolute/path/to/temporary-catalog -name approved-test-source -url https://source-name.tailnet-name.ts.net
```

For Windows, pass a Windows absolute path after `-data-dir` instead. Record the 32-hex-character host ID displayed by `add-host`.

Transfer the source's agent bearer secret through an already approved credential channel—not email, issue comments, a URL, or a shell command literal. Inject `MASTERDRIVE_PEER_TOKENS_JSON` into the controller process as a JSON map of **host ID to that same secret**, then start:

```sh
go run ./cmd/masterdrive controller -data-dir /absolute/path/to/temporary-catalog -listen 127.0.0.1:8788 -web-dir web
```

Open the **exact local** `http://127.0.0.1:8788/#session=...` URL printed by the controller, on that controller machine only. The fragment activates a separate HttpOnly browser cookie. Do not forward or screenshot the session URL. The controller reads peer-token JSON at **startup**; after any token change, stop/restart the controller with a new map and open its new link.

**Positive criterion:** the browser lists exactly one registered source and its `test` share, lists the approved `hello.txt`, and downloads bytes matching the source's recorded SHA-256. No source full path or peer token appears in browser responses.

## Gate E — negative and revocation matrix

These are explicit acceptance tests. Do not pass a check merely because the UI displays an error: distinguish the **network boundary** from the **agent boundary**.

From the *approved controller*, this harmless wrong-token probe is safe to log because the value is deliberately invalid; it should reach the agent through Serve and receive HTTP 401:

```sh
curl --noproxy '*' -i -H 'Authorization: Bearer deliberately-invalid-test-token' https://source-name.tailnet-name.ts.net/v1/shares
```

Use `curl.exe --noproxy '*'` on Windows PowerShell, and check system/user proxy environment variables so the probe demonstrably uses the tailnet route. For the unapproved peer test, omit the Authorization header entirely and observe whether the **HTTPS connection itself** is denied; an HTTP 401 is evidence that some tailnet route was still permitted, not a pass for the transport-deny test. A successful Tailscale ping alone does not establish that the specific HTTPS grant is allowed or denied.

| Probe (owner-controlled) | Expected result | Failure condition |
| --- | --- | --- |
| Distinct unapproved tailnet test peer tries source's Serve HTTPS origin | Cannot establish authorized tailnet request under the reviewed grant | Any reachable HTTP endpoint, **including 401**, means the supposed network deny did not hold (other grants may allow it) |
| Approved controller uses wrong/empty bearer token against `/v1/shares` | Agent returns HTTP **401** with no listing | Share names or file bytes leak |
| Controller tries `add-host -url http://source-name.tailnet-name.ts.net` or non-loopback IP HTTP | CLI rejects plaintext remote origin | Remote cleartext origin is accepted |
| CI/local unit test calls `ValidateLoopbackListen("0.0.0.0:8789")` without opening a socket | Validation rejects the public bind | It is accepted; **do not** attempt a potentially public bind with a live agent |
| Authenticated client attempts `../` traversal or another unexported share | HTTP error; no source sentinel bytes or absolute paths | Data outside `test` returns |
| Authenticated client sends `PUT`, `POST`, or `DELETE` to file route | Non-2xx, and original fixture hash unchanged | A file is modified |
| Source agent stops while Serve remains configured | Controller shows bounded peer error/unavailable, never stale success | Reads silently succeed from unrelated data |
| Admin removes *only the dedicated temporary transport grant* after confirming no wider grant applies | A **fresh connection** from the previously allowed controller loses Serve reachability even with known token | A new client can still reach Serve under supposedly revoked network policy; in-flight streams/keepalive teardown remain a separate unverified behavior |
| Controller `remove-host` with same temporary catalog | Host disappears from controller; **agent token is not revoked** | Product implies source credential was rotated/revoked |
| Agent owner restarts agent with a new random token and retires old process | Old token returns HTTP 401; new configured controller token works only after controller restart | Old token still works with old live agent process, or old stream is misdescribed as revoked |

**Expected gateway behavior:** a failed source request often surfaces as HTTP **502** from the controller because it deliberately hides remote error detail; the source itself should distinguish 401 for a wrong token. Mark each layer's observed status separately. MasterDrive v0.1 has no native node pairing, effective live token rotation API, per-share grants, or in-flight stream revocation. A network grant change can disrupt other users if it was not truly test-scoped; if the administrator cannot isolate it, mark the revocation test **blocked**, not passed.

## Gate F — teardown and evidence

1. In the three terminals under your control, stop only the test controller, agent, and foreground Serve session using Ctrl+C. Confirm with `tailscale serve status` and `tailscale serve get-config --all` that the **test mapping** is no longer active while preexisting mappings are unchanged. Do not reset unrelated Serve configuration.
2. The tailnet administrator restores or removes **only** the test-specific grant after checking the original policy. Verify again that an unapproved peer cannot reach the source.
3. If the temporary catalog still contains the test registration, remove exactly that host ID with `masterdrive remove-host -data-dir <temporary-catalog> -id <host-id>`. This changes local metadata only; it is not credential revocation.
4. Retire the fixture token in the approved secret channel, clear process environment variables from the temporary shells, and verify test processes exited. Do not leave `-bg` Serve configuration, public port forwarding, or a saved browser session link.
5. Inspect the exact temporary fixture/catalog paths and remove only files created for this test using your normal recoverable cleanup process. Never run an unbounded recursive cleanup against a drive root or wildcard.
6. Record pass/fail for every row, exact host/runtime/commit/policy identity, hashes (not bytes of private content), and blocked tests. Any unexpected exposure is a release blocker requiring investigation.

## What this test does *not* prove

Even if every row passes, this is one controlled two-device read-only trial. It does not certify Windows NTFS junction/reparse races, mutable-file cross-range consistency, automatic pairing/revocation, secure credential vaults, large-file resiliency, multi-user ACLs, mounted drive semantics, replication, placement, data integrity across crashes, or a writable storage pool. Those require separate tests in [security acceptance gates](../security/acceptance.md).

### Primary sources

- [Tailscale Serve behavior and HTTPS prerequisites](https://tailscale.com/docs/features/tailscale-serve)
- [Current Serve CLI syntax, status, off and foreground/background behavior](https://tailscale.com/docs/reference/tailscale-cli/serve)
- [Tailscale grants](https://tailscale.com/docs/features/access-control/grants)
- [Tailscale grants examples](https://tailscale.com/docs/reference/examples/grants)
- [MasterDrive v0.1 README at the exact tested source revision](https://github.com/thebrazenbeard/masterdrive/blob/c20fee63d176e69febb0c51afa0bf6a8bfb08143/README.md)
- [MasterDrive architecture](../architecture.md) and [protocol](../protocol-v1.md)

Research provenance and unresolved independent verification: [two-host evidence record](../research/two-host-evidence.md), to be published alongside this runbook by the integrator.
