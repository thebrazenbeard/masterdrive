# MasterDrive two-host qualification research record

## Frozen plan (2026-11-02 UTC, before external source checks)

**Question:** What safe, operator-controlled steps and negative tests can qualify a read-only MasterDrive source agent on one Windows/Linux host and a controller on a second host over private Tailscale Serve, without implying identity-bound pairing or a production release?

**Hypotheses and falsifiers**

- H1: Tailscale Serve can expose an agent bound to local loopback to authorized tailnet peers over a private HTTPS origin. Falsifier: current official Serve documentation does not support a private HTTPS proxy from local port 8789.
- H2: A narrow Tailscale *network* grant plus MasterDrive's independent bearer credential is required, and neither provides native per-share user identity in v0.1. Falsifier: current MasterDrive source verifies Tailscale peer identity and per-share grants automatically.
- H3: Revoking only a Tailscale grant is not equivalent to revoking the agent's shared bearer token; tests must distinguish transport loss, catalog removal, and source-token rotation. Falsifier: the current MasterDrive source automatically revokes all three together.
- H4: Host-only dry-run procedures can verify negative conditions without exporting any real drives. Falsifier: the prescribed workflow requires sharing a root or turning on public Funnel.

**Methods**

- M1 (confirmatory, source): inspect repository implementation for bind, tokens, local session, URL validation, catalog removal semantics.
- M2 (confirmatory, documentation): verify latest official Serve CLI syntax, HTTPS/MagicDNS prerequisites, private/public boundary, and teardown semantics.
- M3 (confirmatory, documentation): verify grants syntax, default policy and path-to-peer negative test expectations.
- M4 (design-only, no execution): write a step-by-step procedure requiring owner and admin approval and a temporary harmless fixture.
- M5 (independent verification): ask a separate reviewer to confirm documented claims and procedural safety before calling the research complete.

**Stop conditions:** At most three external documentation topic queries, no actual host commands or tailnet changes, no credentials collected, no real drive access. If syntax/permission semantics remain ambiguous, leave a version-dependent operator check rather than assuming.

## Evidence log

Entries will use E-IDs, source URL or local path, retrieval date, decisive observation, and what it supports. No live confirmatory runs are planned; all runtime qualification remains OPEN.

- **E1 (2026-11-02, code inspection):** `clireview-work/cmd/masterdrive/main.go`, `agent.go`, `integrity-work/internal/server/server.go`, `cisecfix-work/internal/catalog/store.go`. Decisive source symbols: `ValidateLoopbackListen`, `MASTERDRIVE_AGENT_TOKEN`, `MASTERDRIVE_PEER_TOKENS_JSON`, `net.Listen("tcp", *listen)`, `RequireSession`, `CanonicalEndpoint`. Supports local bind, separate credentials, controller browser session and origin-only remote HTTPS claims (M1).
- **E2 (2026-11-02, official Serve):** https://tailscale.com/docs/features/tailscale-serve — current docs state Serve proxies a loopback HTTP port via private tailnet HTTPS, requires HTTPS enablement, observes tailnet access policy, and foreground `tailscale serve 3000` ends on Ctrl+C. Supports H1 and M2. No local Serve runtime was inspected.
- **E3 (2026-11-02, official Serve CLI):** https://tailscale.com/docs/reference/tailscale-cli/serve — current docs show `tailscale serve [flags] <target>`, `status [--json]`, `--bg` persistence, `off` with original flags, and caution that `reset` removes all Serve configuration. Supports version-specific command/cleanup instructions (M2).
- **E4 (2026-11-02, official grants):** https://tailscale.com/docs/features/access-control/grants — official example `"ip": ["tcp:443", "udp:53"]`, selectors under `src`/`dst`, and network/application permissions are distinct. Supports H2/M3. A grant's existence alone cannot prove an effective deny if another rule allows the connection; this must be tested.
- **E5 (2026-11-02, official Serve identity notes):** https://tailscale.com/docs/features/tailscale-serve — user identity headers are not populated for tagged clients; current MasterDrive does not verify them, so they are explicitly excluded from qualification as authorization. Supports H2/M3.
- **E6 (2026-11-02, official Serve/Funnel):** https://tailscale.com/docs/features/tailscale-serve — Serve is tailnet-only; Funnel is explicitly for public Internet exposure. Supports H4/M2, and forbids Funnel in the procedure.
- **E7 (2026-11-02, repo research cross-check):** `taildrive-masterdrive-work/docs/interop/taildrive.md` cites https://tailscale.com/docs/reference/quad100 for the device-local `100.100.100.100` address and https://tailscale.com/docs/features/taildrive for WebDAV `:8080`. Supports the runbook's prohibition on treating that local Taildrive endpoint as the remote MasterDrive agent URL. This claim was inherited from the repo's separate Taildrive research, not newly tested.

No host-side command was executed. M4 is a proposed operator runbook, not an executed experiment. H1–H4 are supported by source/document contracts only; the runtime falsifiers remain untested.

## Status

Documentation source verification complete. Confirmatory *source/document* checks M1–M3 are closed; runtime qualification M4 remains an unexecuted procedure, not a confirmatory success. Independent verifier M5: **pending**. Research outcome: **provisional / not independently confirmed**.

### M5 — independent source-verifier addendum, 2026-11-02

- Reviewer: native `/root/docs_verifier`. Fresh official [Tailscale Serve CLI documentation](https://tailscale.com/docs/reference/tailscale-cli/serve) confirms `serve get-config --all` (specifically to enumerate hosted Services), foreground vs `--bg`, `status`, and the destructive full `reset` behavior. The runbook prohibits changing other people's configurations and distinguishes private Serve from Funnel.
- A documentation reproducibility issue was corrected in isolated `docs-verify-work/docs/testing/two-host.md`: its README reference now targets the **exact v0.1 commit** instead of mutable `main`. Its internal links resolve after integration at `docs/testing/two-host.md` if `docs/architecture.md`, `docs/protocol-v1.md`, `docs/security/acceptance.md`, and `docs/research/two-host-evidence.md` are staged in the corresponding locations.
- **Verdict:** `PARTIALLY CONFIRMED`: command syntax and separation of transport grant, agent secret and catalog removal survive. There was **NO** host-side execution, Tailscale admin-policy review, network denial proof, Windows/Linux qualification or revocation test. Two-host runtime gate remains **OPEN**.
