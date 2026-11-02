# MasterDrive identity integration research record

Research date: 2026-11-02 (UTC)

## Frozen question and plan (pre-registered)

Question: For a Windows/Linux, opt-in, read-only MasterDrive agent reachable privately through Tailscale, what is a practical secure device pairing, caller identity, authorization, and revocation architecture that does not mistake Taildrive's local WebDAV endpoint for a remote endpoint?

Hypotheses:

- H1: An agent behind private Tailscale Serve, bound only to loopback, can authenticate an incoming user identity through Serve-generated headers, but the loopback trust boundary alone is insufficient against locally untrusted processes. Prediction: official docs explicitly discuss forwarded identity headers and their conditions. Falsifier: official docs deny or change the identity header contract.
- H2: Embedding a `tsnet` node offers a stronger programmatic identity and per-capability authorization mechanism via `WhoIs`, at the cost of new dependencies and host state lifecycle. Prediction: current docs describe `LocalClient.WhoIs` and capability grants for tsnet. Falsifier: documented API lacks that mechanism or Windows deployment is unsupported.
- H3: Tailscale grants plus MasterDrive's own pair approval and per-device/share policy can fail closed on revocation while bearer-token fallback is explicitly confined to loopback tests. Prediction: docs establish a network-grant primitive and changing policy can remove network access; local code currently uses a static token map. Falsifier: official grants do not gate communication or MasterDrive already persists a live credential-independent pair identity.

Methods:

- M1, source inspection, confirmatory: inspect current CLI, HTTP gateway/agent, session, manifests, protocols for actual guarantees and limits.
- M2, docs verification, confirmatory: consult current official Tailscale Serve, tsnet/WhoIs, grants/policy docs (at most three independent documentation queries) and record retrieved evidence with date.
- M3, threat reasoning, confirmatory: draw pairing/revocation state flow, explicitly distinguish transport identity, app authorization, and local OS trust; enumerate tests that would falsify security claims.

Stopping rules: Deliver a source-cited design and executable acceptance gates; do not claim a live mesh qualification. Stop before host changes, Tailscale configuration, credential generation on real systems, or inserting unreviewed transport-auth code into the build branch.

## Evidence log (append only)

- E1 (2026-11-02, source inspection): `masterdrive-build/go.mod` (`sha256:451426f8f8ca1aeb9eccdf705f78be1893f330bec9c2d41815280715b4e80042`) declares Go 1.27.0 and has no `tailscale.com` Go module. No third-party identity SDK version is installed in this source snapshot. M1 confirmed.
- E2 (2026-11-02, source inspection): `masterdrive-build/cmd/masterdrive/main.go` (`sha256:41dcf68960e1dca1d5c73323864615475108e40f65aa59f6eeeb1384888127a5`) reads `MASTERDRIVE_PEER_TOKENS_JSON` at controller startup into an in-memory map, using a local session token for browser API. `masterdrive-build/cmd/masterdrive/agent.go` (`sha256:140c3b5c7c0d9b1d234fc59dd6159a50e7e65e1ad6d2165d93fc0f24a7796c84`) insists on loopback binding and one `MASTERDRIVE_AGENT_TOKEN`. M1 confirmed.
- E3 (2026-11-02, source inspection): `agent_masterdrive_workspace/internal/agent/agent.go` `NewHandler` accepts only one bearer secret, stores a SHA-256 digest, checks auth on every route and accepts all explicit shares after auth. This is a host-wide proof-of-concept secret, not per-controller/per-share pairing or live revocation. M1 confirmed.
- E4 (2026-11-02, docs lookup #1, Tailscale Serve): [https://tailscale.com/docs/features/tailscale-serve](https://tailscale.com/docs/features/tailscale-serve), retrieved through official-web search `turn29view0`. Decisive text: Serve identity headers omit tagged devices, include accepted external device shares, and require backend localhost to prevent direct spoofing; app capability forwarding `--accept-app-caps` requires Tailscale v1.92+. M2 confirmed H1 with the documented caveat.
- E5 (2026-11-02, docs lookup #2, Tailscale tsnet/identity): [https://tailscale.com/docs/features/tsnet/how-to/create-basic-tsnet-app](https://tailscale.com/docs/features/tsnet/how-to/create-basic-tsnet-app), `turn29view2`, and [https://tailscale.com/docs/concepts/tailscale-identity](https://tailscale.com/docs/concepts/tailscale-identity), `turn28search1`. `tsnet` offers a distinct node identity with a persistent state directory and `LocalClient.WhoIs`; Tailscale LocalAPI maps a connecting node to user/tags and capabilities. Source doesn't establish MasterDrive's runtime integration or Windows-specific compatibility. M2 confirms contract, not deployment.
- E6 (2026-11-02, docs lookup #3, grants/app capabilities): [https://tailscale.com/docs/reference/syntax/grants](https://tailscale.com/docs/reference/syntax/grants) (`turn29view3`) and [https://tailscale.com/docs/features/access-control/grants/grants-app-capabilities](https://tailscale.com/docs/features/access-control/grants/grants-app-capabilities) (`turn29view1`). Grants can specify source/destination and `tcp:443`; app capabilities are only effective alongside network access, and custom app capability namespace must use a domain the operator controls (Tailscale domains reserved). M2 confirms H3 grant layer and rejects the claim that grant alone authenticates app shares.
- E7 (2026-11-02, parallel project research): `taildrive-masterdrive-work/docs/interop/taildrive.md` cites [https://tailscale.com/docs/reference/quad100](https://tailscale.com/docs/reference/quad100), establishing Taildrive WebDAV's `100.100.100.100` as device-local, not a remote agent endpoint. Independent document is unexecuted design evidence only.

## Bounded checks and deviation log

- M1 source inspection completed; no Go compiler or local Tailscale runtime tests were run under this task. A distinct `tailscale.com` Go module version could not be established because the project currently has no dependency on it (E1).
- M2 three grouped official-documentation queries completed and inspected 2026-11-02; no version-specific binary release was available to match against a running host (E4–E6).
- M3 is a desk-based threat analysis, not an executable confirmation. Its outputs are in `docs/security/identity.md` and require independent code review plus live Windows/Linux qualification before any security verdict beyond documentation.

## Verdict and unresolved

- **Partially confirmed (source contract only):** H1 and H2 have direct official documentation support; H3's network-grant claim is documented, but effective pair/revoke depends on code not implemented in this snapshot. E1–E7.
- **Unanswered:** actual host Tailscale binary version; whether Serve has `--accept-app-caps` at the target device; Windows service lifecycle and localhost principal isolation; propagation latency of changed grants; exact grant/revocation behavior during an existing long download.
- **No independent verifier was assigned to this isolated document before delivery.** This report is a proposed, source-bound design, not a completed verified integration. No evidence of live pairing/revocation was produced.

