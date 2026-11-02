# MasterDrive licensing and upstream-integration decision memo

Status: **research proposal, not a license selection or legal opinion** (2026-11-02). The repository is public but currently has no checked-in `LICENSE`. Its v0.1 Go module requires no third-party modules; Tailscale Serve, WinFsp, and Linux FUSE are future/optional integrations, not bundled runtime dependencies. Source-level verification only; see the [licensing evidence record](research/licensing-evidence.md) for the frozen question and sources, if published alongside this memo.

## The decision the owner needs to make

The project should not silently remain unlicensed if the goal is outside contributions, redistribution, binaries, or integrations. GitHub explains that without a license, ordinary copyright restrictions remain; public visibility alone is not permission for others to reproduce, modify, or redistribute the code. [GitHub licensing guidance](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/licensing-a-repository).

The strongest first choice depends on the owner's intended policy:

| Choice | Good fit if the owner wants… | Tradeoff to accept |
| --- | --- | --- |
| **Apache-2.0** | Broad personal, commercial, NAS-vendor and third-party adoption, while using a well-defined contributor patent grant | More notice/redistribution details than MIT; others can publish proprietary additions |
| **MIT** | Very simple permissive licensing and low friction for tools, package managers and integrations | Does not contain Apache-2.0's comparably explicit patent-grant structure; proprietary forks possible |
| **AGPL-3.0-only** | A reciprocal code-sharing policy for modified network-facing deployments, including modified software offered as a service | Higher compliance/integration friction; a deliberate reciprocal ecosystem choice, not a security mechanism |
| **Keep unlicensed for now** | Deliberately reserve all rights while deciding business/ownership model | Inhibits legitimate community redistribution and makes unsigned candidate binaries difficult for third parties to reuse responsibly |

All three named licenses are OSI-approved, but their conditions differ materially. The [MIT text](https://opensource.org/license/mit) broadly grants use, modification and distribution with notice retention. [Apache-2.0](https://opensource.org/license/apache-2.0) adds explicit patent-license terms and does not grant trademark rights. [AGPL-3.0 section 13](https://opensource.org/license/agpl-3-0) addresses source availability for remote users of modified network-interactive software. A license choice does not substitute for secure pairing, encryption, reliable storage, or a warranty.

**Conditional recommendation:** if MasterDrive is intended as an easy-to-integrate personal-storage tool with eventual community and commercial adapters, choose **Apache-2.0** after confirming all contributors' authority to license the code. If the principal requirement is that improvements to network-hosted derivative services remain available to users, evaluate **AGPL-3.0** instead, with compatibility advice. MIT is a reasonable simplicity-first option. This recommendation is a product-policy judgment, not a requirement imposed by current Go source.

## Tailscale: source-code rights are not service rights

The open [Tailscale client repository](https://github.com/tailscale/tailscale/blob/main/LICENSE) uses **BSD-3-Clause** for its primary source. Its license requires preserving notices on redistribution and prohibits using the copyright owners' names to imply endorsement. The repository also [lists component licenses](https://github.com/tailscale/tailscale/blob/main/licenses/tailscale.md); importing `tsnet` or redistributing binaries needs a complete dependency/notice audit for the pinned version, not a one-line assumption that every linked file is BSD-3-Clause.

Separately, Tailscale's [Terms of Service](https://tailscale.com/terms) (last updated August 25, 2026) contain section 2.3 restrictions on using the Services for competitive analysis, benchmarking, and development/commercialization of technology that could compete with the Services. **That service agreement is distinct from the BSD source license.** Its applicability to a storage-specific MasterDrive integration depends on use, account terms and circumstances; it cannot be resolved by code inspection. Before using Tailscale Services for competitive analysis, benchmarking, or development of technology that could compete with those Services—even if the project is noncommercial—get appropriate legal review or written authorization where needed. This is a potential terms issue, not a conclusion that MasterDrive competes with Tailscale.

The current architecture merely allows an operator to configure an *external* private HTTPS route such as Tailscale Serve. That is not the same as embedding Tailscale's code, selling Tailscale access, or claiming a Tailscale partnership. Avoid brand language suggesting endorsement, certification, official Taildrive integration, or that MasterDrive owns Tailscale marks. Factual documentation such as “supports a manually configured Tailscale Serve route” is more precise than naming MasterDrive “Tailscale for disks” as its formal brand.

## WinFsp: review the FLOSS exception before any Windows mount

The official [WinFsp license](https://github.com/winfsp/winfsp/blob/master/License.txt) is **GPLv3 with a special FLOSS exception**. It permits specified linking to platform WinFsp DLLs and redistribution of unmodified official installers under conditions, including:

- The integrating software is distributed under a qualifying Free Software/Open Source Definition license.
- Required WinFsp copyright and repository attribution appears in the user interface and user-facing documentation.
- The exception does not authorize mixing the WinFsp-linked system with proprietary non-FLOSS software.

Commercial licensing is an alternative from the WinFsp author. This is a notable constraint if MasterDrive later adopts a proprietary or mixed commercial model. MIT, Apache-2.0, and AGPL-3.0 are OSI-approved, but compliance still depends on the actual code, distribution and notices; have a knowledgeable reviewer confirm the specific planned DLL, binding, installer and packaging layout. The exception is not a general grant to relicense the WinFsp code itself.

## Linux FUSE: the exact component matters

The official [libfuse license file](https://github.com/libfuse/libfuse/blob/master/LICENSE) distinguishes source files: `include/`, `lib/`, and `meson.build` may be used under **LGPL-2.1**, while other files in the repository are under **GPL-2.0**. A later Go FUSE binding may have its own license independent of libfuse and may use a kernel API directly. Record the exact chosen library/module version and how it is linked/distributed before promising compatible static binaries or commercial bundling. Do not bundle other libfuse repository utilities assuming they share the library's LGPL terms.

## Suggested action before public binaries

1. **Owner chooses a policy** (permissive Apache/MIT, reciprocal AGPL, or deliberate proprietary) and confirms contributor copyright authorization. No assistant should choose or commit this unilaterally.
2. Add the complete, unmodified canonical license in the repository root plus an appropriate SPDX identifier in source headers/package metadata. If copyright notices are needed, use the actual rightsholder(s), not an invented company.
3. Inventory source and binary dependencies at pinned versions; include third-party copyright notices and license texts when redistribution requires them. Generated checksums alone are not a software license, provenance signature, or SBOM.
4. Before native mounts, review WinFsp and Linux FUSE adapter-specific linking, distribution, attribution, and license compatibility. Keep proposed adapters optional until permission and platform security are verified.
5. Before Tailscale-Service-backed competitive analysis, benchmarking or potentially competitive development (whether commercial or not), resolve separate hosted-service terms and brand/endorsement questions; record any authorization and use the vendor's current terms, not just the repository BSD license.
6. Update the README and candidate-archive notices *after* a licensing decision. Until then, the existing warning that public source does not confer general reuse rights is appropriate.

## Evidence and unresolved questions

Primary-source evidence is recorded in the accompanying [licensing evidence record](research/licensing-evidence.md): official GitHub no-license guidance, OSI license texts, Tailscale source license and service terms, and WinFsp/libfuse license files. The current decision cannot be made solely from evidence because it depends on the owner's goals, contributor rights, whether commercial distribution is intended, and which future native mount libraries are actually chosen. No license was added, no hosted service used, and no distribution was performed during this research.
