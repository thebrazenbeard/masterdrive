# Taildrive interoperability assessment (optional, experimental)

Research snapshot: **2026-11-02**. This is a design assessment, **not** a claim that MasterDrive currently implements a Taildrive adapter. All behavior below attributed to Tailscale is based on its public documentation; product-specific assumptions requiring a live compatibility test are explicitly marked **UNKNOWN**.

## Conclusion

**Do not implement a second Taildrive.** Tailscale already exposes shares through a WebDAV namespace and handles its own tailnet sharing policy. MasterDrive can offer an *opt-in, read-only WebDAV source adapter* for users who have already configured Taildrive. Treat that source as an external filesystem with its own authority, not as a MasterDrive-managed disk, and do not silently bridge its contents to a broader audience.

Keep a separate, straightforward option for a **native MasterDrive agent behind Tailscale Serve**. It uses Tailscale for private network transport to MasterDrive's service, but is **not** Taildrive interoperability. The two options have distinct authentication and authorization boundaries.

## Verified protocol and reachability facts

| Subject | Verified fact | Implication |
| --- | --- | --- |
| Taildrive state | Tailscale calls Taildrive **alpha**; 1.64.0+ includes a WebDAV server while connected. | Pin tested client versions; expect changes and feature gating. |
| WebDAV entry | URL `http://100.100.100.100:8080`; share paths `/<tailnet>/<machine>/<share>`. | Read the path as a discovered namespace, not a user-authored arbitrary filesystem path. |
| Quad100 scope | `100.100.100.100` is Tailscale's device-local Quad100 service address; other devices **cannot access that device through Quad100**. It is comparable to loopback in locality, but **is not** the literal `127.0.0.1` address. | `http://100.100.100.100:8080` must be accessed **from the machine running the adapter and Tailscale**. Never publish it as a globally routable origin or assume a browser on another host will reach the same namespace. |
| Sharing | Administrators enable `drive:share` and/or `drive:access` in `nodeAttrs`; grants use `tailscale.com/cap/drive` with share selectors and `ro`/`rw`. | A MasterDrive adapter must defer permission decisions to Taildrive's current source principal and policy; an ordinary network grant is not a substitute for Taildrive's per-share capability. |
| UI client | Tailscale's macOS instructions connect to the local HTTP WebDAV service as **Guest**; the documentation states the connection is encrypted across the Tailscale WireGuard tunnels. | Do **not** invent a Taildrive username/password or demand an API token; “Guest” here does **not** mean all remote shares are publicly readable. Preserve the local security boundary. |
| Device shares | Linux, macOS and Windows can provide Taildrive shares; iOS and Android are clients only. Devices shared from one tailnet into another cannot access those Taildrive folders. | Do not promise cross-tailnet mounting or mobile-origin share publishing. |
| WebDAV protocol | RFC 4918 specifies `PROPFIND` with `Depth: 0` or `1`, XML/`207 Multi-Status`, property-level statuses, and HTTP/WebDAV operation semantics. | Parse namespace-aware XML and `propstat` failures, use bounded requests, and differentiate “unavailable”, “forbidden” and “not found”. |

Sources: [Tailscale Taildrive](https://tailscale.com/docs/features/taildrive), [Quad100 technical reference](https://tailscale.com/docs/reference/quad100), [Tailscale application capabilities](https://tailscale.com/docs/features/access-control/grants/grants-app-capabilities), [Tailscale grants](https://tailscale.com/docs/features/access-control/grants), [RFC 4918](https://www.rfc-editor.org/rfc/rfc4918).

## Path A — consume Taildrive as a locally authorized WebDAV source

The trust boundary is **the Tailscale client on the adapter host**. Mounting or indexing the namespace in a headless *shared* MasterDrive controller may result in every MasterDrive user seeing files reachable by the controller's Tailscale identity; that would be an authorization-confused deputy unless MasterDrive separately enforces equally restrictive per-user access. Prefer a per-user, on-device adapter initially.

Proposed minimal contract (design, **not yet implemented**):

1. Explicitly enable the experimental integration on a device with Tailscale connected and Taildrive enabled. Default **off**; require a clear display of which local device's Taildrive access will be used.
2. Bound the server address to the exact fixed origin `http://100.100.100.100:8080`. Do not accept arbitrary URLs, redirect targets, localhost aliases or cross-origin credentials. Never let untrusted WebDAV `href` values turn into arbitrary outbound HTTP requests; normalize and constrain paths to the selected root.
3. Discover accessible shares with `PROPFIND Depth: 1` and `DAV:` XML properties such as `resourcetype`, `displayname`, `getcontentlength`, `getlastmodified`, and `getetag` where available. Handle missing properties and `207 Multi-Status` per-resource/per-property errors. Do not send recursive `Depth: infinity` scans.
4. Browse a user-selected `/<tailnet>/<machine>/<share>` root with capped pagination or bounded directory enumeration, timeouts, response size limits, secure XML parsing, and cancellation. Cache metadata with a short TTL; expiry or disconnect removes *visibility*, not the user's disk data.
5. Read files with streamed GETs. Check optional range support by actual responses; validate lengths and status codes and reject HTML/login/error bodies masquerading as files. Never report hash verification unless MasterDrive actually computes and checks an independent expected digest.
6. Surface the source as `taildrive:read-only` with prominent provenance and policy-change warnings. Exclude it from MasterDrive re-sharing, upload, delete, rename, automatic mirroring, dedup, and background replication by default. No secret-bearing headers, cookies, bearer tokens or URL user-info are logged, stored, or forwarded.
7. On permission revocation, disconnect, HTTP error, missing share, timeout, or identity switch: fail closed, invalidate stale directory views, and return a clear error. Do not serve previously cached file *contents* once access changes.

Important **UNKNOWNs requiring an integration test**:

- Which subset of RFC 4918 methods, properties, conditional GET, byte ranges, stable ETags and large-directory behaviors Tailscale's current alpha implementation supports across target OSes.
- Whether Taildrive's folder enumeration exposes all inaccessible share names or filters them; do not infer policy from directory names.
- What exact status codes are returned for denied access, disconnected Tailscale, or a revoked share, and whether reauthentication/restarting changes behavior.
- How client upgrade/downgrade affects namespace naming and consistency; whether a directory may mutate between `PROPFIND` and GET.
- Whether noninteractive service processes can use the device-local endpoint on each target OS and installation variant. The docs establish the endpoint's existence, **not** these embedding guarantees.

## Path B — expose MasterDrive's own agent through Tailscale Serve

Serve can proxy a service bound on `127.0.0.1:<agent-port>` to an HTTPS URL accessible *within the tailnet*, subject to tailnet network rules. It requires HTTPS certificates to be enabled. This is a transport route to the **MasterDrive protocol**, not to Taildrive's `100.100.100.100:8080` share interface, and does not confer Taildrive's `tailscale.com/cap/drive` permissions. Do **not** use Funnel for private-only access: Funnel is intended to expose services to the public internet. [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), [Serve command](https://tailscale.com/docs/reference/tailscale-cli/serve), [Tailscale Funnel](https://tailscale.com/docs/features/tailscale-funnel).

Serve adds `Tailscale-User-*` identity headers for some tailnet traffic and removes spoofed incoming instances. However, the docs say identity headers are **not populated for tagged-device traffic**, and externally shared device access can also carry identity headers. An app must still implement authorization, including for empty/unexpected identity. Serve's optional `Tailscale-App-Capabilities` forwarding requires Tailscale **1.92+**, explicit `--accept-app-caps`, and backend validation; it does **not** automatically add file permissions. [Tailscale Serve identity and app capability headers](https://tailscale.com/docs/features/tailscale-serve).

For any service trusting Serve-injected headers, bind its HTTP listener to loopback and ensure there is no direct LAN/tailnet route bypassing the proxy. The official Serve guidance warns direct callers could otherwise spoof the identity/capability headers. Use an explicit independent MasterDrive authorization policy, never network membership alone.

## Acceptance gates before offering a Taildrive adapter

These are proposed testable gates for a later implementation; passing protocol unit tests alone would **not** establish compatibility or safety.

1. **Device-locality:** two connected machines each query their own Quad100; confirm a remote connection to machine A's Quad100 cannot be used to access A's endpoint, and UI text never misrepresents the origin as a remote host.
2. **Policy matrix:** account A with `ro`, account B with `rw`, account C denied, and revoked/retagged shares; verify only A/B can read and the adapter cannot elevate A or C. Include source identities on tagged devices and shared devices where relevant.
3. **Isolation:** an account lacking a grant cannot obtain directory entries, file bytes, caches, or stale data from a previously authorized account; concurrent sessions must not share a single privileged adapter principal.
4. **WebDAV fixtures:** valid `207` mixed `propstat` results, namespaces, escaping, unicode, percent-encoded paths, nested collections, empty collections, partial GET, status mismatches, huge XML, slow streams, redirects, and unexpected HTML.
5. **Fault injection:** Tailscale stopped/restarted, drive attribute disabled, grants revoked during GET, share renamed/deleted, host offline, file changed midstream, disk full, and client upgrade. Confirm graceful errors, zero corrupted “successful” downloads and no unauthorized cached bytes.
6. **No mutation/no secrets:** assert no PUT/MKCOL/DELETE/MOVE/COPY/PROPPATCH or credential propagation by default; inspect structured logs and telemetry to ensure they contain neither private paths nor tokens/cookies. Explicit user opt-in is preserved across no permission-sensitive account switch.
7. **Support range:** run on at least supported Linux/Windows versions and a macOS Tailscale installation variant, documenting tested Tailscale versions, changed behavior and how to disable the adapter without data loss.

## Licensing and positioning

Use ordinary standards-compliant WebDAV client code and public documented endpoints rather than copying Tailscale implementation internals. RFC 4918 is an interoperable protocol; Tailscale's core daemon code is open source, but that **does not license the Tailscale name/trademark, guarantee hosted service rights, or guarantee alpha compatibility**. If any Tailscale source code or `tailscale/xnet/webdav` code is imported, inspect the exact per-file and dependency licenses and preserve notices (the Tailscale source tree shows BSD-3-Clause SPDX identifiers, and its dependency inventory identifies `xnet/webdav` as BSD-3-Clause). This is an engineering caveat, not legal advice. [Tailscale open source](https://tailscale.com/opensource), [Tailscale source license inventory](https://github.com/tailscale/tailscale/blob/main/licenses/tailscale.md), [Tailscale terms](https://tailscale.com/terms).

Product copy: **“MasterDrive can optionally browse shares you already have access to through Tailscale Taildrive (experimental).”** Avoid “official Tailscale integration,” “compatible with all Taildrive versions,” “federated Taildrive,” or “Tailscale-backed access control for all MasterDrive exports” absent an agreement and verification. Clearly attribute Taildrive and Tailscale to Tailscale Inc.

## Recommendation

Ship the **native agent + optional Tailscale Serve** route first as a documented private transport scenario; it need not wait for Taildrive alpha stability. Keep **Taildrive read-only import/browse** as a separate feature flag with the acceptance gates above. Never place the device-local WebDAV endpoint behind an unguarded, multi-user MasterDrive API; doing so would turn one user's/share principal's permissions into everyone else's.
