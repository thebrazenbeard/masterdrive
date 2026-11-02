# MasterDrive licensing research record

## Frozen research plan — 2026-11-02 UTC (before external lookup)

**Question:** Given a public but unlicensed Go source repository for a Windows/Linux storage-federation prototype, what licensing choice should the owner consider, and what upstream dependency/trademark conditions matter for optional Tailscale, WinFsp, and Linux FUSE integrations?

**Local baseline:** `masterdrive-build/go.mod` has `module github.com/thebrazenbeard/masterdrive`, `go 1.27.0`, and no `require` dependencies. The latest authored README explicitly says no license selected. Current source does not vendor Tailscale, WinFsp, or FUSE code. Confirm against GitHub before final verdict.

**H1 (permissive):** MIT or Apache-2.0 would allow broad use and redistribution, whereas no license leaves third parties without default reuse permission. **Falsifier:** current official license sources say those licenses do not grant broad reuse or GitHub no-license documentation says reuse is permitted by default.

**H2 (copyleft):** AGPL-3.0 is a possible deliberate choice if the owner wants to condition network service deployment on sharing source; it carries more compliance friction for integrators. **Falsifier:** official license text/OSI sources do not support network source provisions.

**H3 (upstream integration):** Using vendor interfaces (external Tailscale Serve / kernel-mounted FUSE) without copying code is legally/technically different from incorporating third-party code; chosen binding packages may add obligations, and product naming must not imply vendor affiliation. **Falsifier:** project licensing docs expressly prohibit all use via interfaces or grant unconditional trademark permission.

**M1 confirmatory:** Inspect official GitHub, OSI/SPDX license guidance and text on MIT/Apache/AGPL/no-license.

**M2 confirmatory:** Inspect official Tailscale repository license/trademark guidance; distinguish app's calling an installed product from importing `tsnet` code.

**M3 confirmatory:** Inspect official WinFsp and libfuse license documents; identify compatibility decision points for distribution.

**Stopping rules:** Conclude after M1–M3 if primary docs are sufficient, otherwise mark gaps. No license will be written to GitHub or selected on the user's behalf. Findings need independent verification before definitive closure.

## Evidence log (append-only)

Pending primary-source lookup.

### E1 — GitHub default copyright, M1, 2026-11-02

- Source: https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/licensing-a-repository (GitHub current documentation; search result was Enterprise Cloud variant).
- Decisive phrase fingerprint: “without a license, the default copyright laws apply”.
- Observation: a public repository is viewable/forkable through GitHub but does not automatically grant a general right to redistribute or create derivatives. H1 remains plausible.

### E2 — OSI license text, M1, 2026-11-02

- Sources: https://opensource.org/license/mit, https://opensource.org/license/apache-2.0, https://opensource.org/license/agpl-3-0.
- Fingerprints: MIT broadly grants use/copy/modification/redistribution subject to notice retention; Apache-2.0 section 3 grants a patent license with patent-litigation termination, section 6 excludes trademark rights; AGPL-3.0 section 13 covers source availability for users remotely interacting with modified programs.
- Observation: H1 and H2 supported. The owner must choose policy; “open source” alone does not determine reciprocity.

### E3 — Tailscale source vs service terms, M2, 2026-11-02

- Sources: https://github.com/tailscale/tailscale/blob/main/LICENSE and https://tailscale.com/terms (ToS updated 2026-08-25), plus https://github.com/tailscale/tailscale/blob/main/licenses/tailscale.md for component notices.
- Fingerprints: source `BSD 3-Clause License` and condition 3 banning implying endorsement by copyright holders; Tailscale terms section 2.3 includes competitive-product, benchmarking, and competitive-analysis restrictions for customers using Services.
- Observation: software license and service agreement grant different rights. Whether a storage management adapter triggers the section 2.3 clause cannot be resolved from source reading; request permission/legal review for commercial or competitive use, and avoid implying official affiliation.

### E4 — WinFsp license, M3, 2026-11-02

- Source: https://github.com/winfsp/winfsp/blob/master/License.txt (official repository file), corroborated by https://github.com/winfsp/winfsp.
- Fingerprint: GPLv3 with a FLOSS linking/distribution exception, requiring an OSI/FSD-compatible license, specified attribution in UI and user-facing docs, and excluding proprietary mixing; separate commercial licensing is available.
- Observation: candidate WinFsp mount plans require compliance review before linking or bundling drivers. An OSI-approved MasterDrive license is a likely prerequisite for the FLOSS exception, not the entire compliance work.

### E5 — libfuse license split, M3, 2026-11-02

- Source: https://github.com/libfuse/libfuse/blob/master/LICENSE (official repository, read via GitHub resource fetch).
- Fingerprint: `include/` and `lib/` files under LGPL-2.1; other repository files under GPL-2.0. Exact file scope matters.
- Observation: do not label the whole libfuse source tree one license or assume a future Go binding inherits exactly that license. Pin chosen adapter and inspect its own LICENSE and transitive notices.

### D1 — retrieval dead end and pivot, 2026-11-02

- Web open of raw WinFsp and libfuse LICENSE URLs returned a retrieval DisabledError. Pivoted to direct GitHub-approved file fetch; retrieved authoritative license texts from their official repositories. This was a documentation lookup, not runtime experiment.

### Confirmatory method status

- M1: completed source comparison; supported H1/H2.
- M2: completed source/service distinction; H3 partially supported, actual service-term applicability unresolved.
- M3: completed license-file inspection; H3 supported as a compliance distinction.
- No runtime or legal adjudication experiment was planned or performed. No independent verifier assigned yet; verdict is **provisionally partially-confirmed, pending independent review**.

### E0 — exact current repository license status, 2026-11-02

- Source: GitHub connected `fetch_file` for `thebrazenbeard/masterdrive`, ref `main`, path `LICENSE`.
- Fingerprint: API `404 NOT_FOUND`. Local `masterdrive-build/go.mod` contains only module path and `go 1.27.0`, without `require` blocks. The current README's license paragraph says none selected.
- Observation: the conventional root `LICENSE` path does not exist at inspected revision. A full root tree audit would be necessary before claiming no license-like file exists under *any* alternative name; all findings are scoped to conventional root license and manifest.

### E6 — independent primary-source cross-check and correction, 2026-11-02

- Independent verifier: native reviewer `/root/docs_verifier`; method: fresh direct retrieval of [WinFsp's official license](https://github.com/winfsp/winfsp/blob/master/License.txt), [libfuse's official license](https://github.com/libfuse/libfuse/blob/master/LICENSE), and [Tailscale's current hosted-service terms](https://tailscale.com/legal/terms). All three were retrieved as documentation/source content only; no legal ruling or runtime probe occurred.
- WinFsp special exception expressly names `winfsp-a64.dll`, `winfsp-x64.dll`, `winfsp-x86.dll`, and `winfsp-msil.dll`; requires qualifying FLOSS licensing, specified notice in UI and user-facing docs, and no proprietary mixing. Prior E4 is confirmed for the license-file version retrieved.
- **E5 correction:** libfuse `LICENSE` specifically lists `meson.build` alongside `include/` and `lib/` for the LGPL-2.1 option; the unqualified assertion that *all* other files are GPL-2.0 was overbroad. Corrected in isolated `docs-verify-work/docs/licensing.md` without altering original evidence.
- Tailscale restrictions described in section 2.3 are not universally conditioned on commercial sale; the proposal's legal-review trigger was broadened to potentially competitive analysis/benchmarking/development whether commercial or not, while leaving applicability to MasterDrive **UNKNOWN**.
- **Verdict:** `PARTIALLY CONFIRMED` (official licensing facts independently verified, but selection, applicability to an actual integration, permissions and distribution obligations remain owner/legal-review questions). No root license, bundled dependency, or hosted-service use is proven by this source review.
