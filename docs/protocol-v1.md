# MasterDrive v0.1 wire protocol

Status: **source-inspected draft**. The precise implemented behavior below corresponds to the initial Go packages in `internal/agent` and `internal/server` at the time of writing. It is not a claim of deployment, formal verification, or a stabilized public standard.

## Network boundaries

There are two distinct HTTP APIs:

- **Agent API** at the source host: bearer-authenticated, read-only, bound to loopback by the v0.1 CLI. It may be published to approved peers through an externally configured private HTTPS transport (e.g. operator-managed Tailscale Serve). Do not expose its loopback HTTP port to the public Internet or advertise arbitrary peer HTTP origins.
- **Controller API** on `127.0.0.1:8788` by default: browser cookie-authenticated; proxies requests to registered agent origins and attaches peer bearer tokens server-side. Browsers must not receive peer tokens. All controller API errors have a structured JSON object.

The controller uses `POST /api/v1/session` with an `X-MasterDrive-Session` secret supplied from the URL fragment, then a local `HttpOnly`, `SameSite=Strict` cookie for subsequent requests. All other v0.1 controller data routes are `GET`. The session bootstrap exception is not a remote device-enrollment API.

Paths are forward-slash-separated **relative file paths inside one export**, independent of Windows or Linux local syntax; the empty path addresses the share root for listings. Do not send absolute paths, Windows drive letters, `..`, backslashes or embedded NUL. Every path segment must be URL-encoded where used in a URI. Share IDs and host IDs are opaque identifiers; never substitute display names as authority.

## Agent routes

Every request requires exactly one `Authorization: Bearer <source-agent-token>` header. Bad/missing credentials produce `401`. The agent handler itself does not provide transport encryption.

| Method and path | Parameters | Response | Semantics |
| --- | --- | --- | --- |
| `GET /v1/shares` | none | `200` JSON `{ "shares": [ { "id": "docs", "name": "docs", "read_only": true } ] }` | Lists only explicitly configured exports; never reveals underlying absolute paths |
| `GET /v1/list` | `share`, `path` (optional root), `limit` (1–200, default 100), `offset` (0–10000) | `200` JSON `{ "share": "docs", "path": "", "entries": [ ... ], "next_offset": null }` | Lists immediate children; no recursive scan or unlimited enumeration |
| `GET /v1/download` | `share`, `path` (required file) | `200` or `206` octet stream with explicit length, Content-Disposition attachment; supports normal HTTP byte ranges via `http.ServeContent` | Regular files only; no source mutation |

Entry shape in the Go implementation:

```json
{
  "name": "report.pdf",
  "path": "folder/report.pdf",
  "type": "file",
  "size": 12345,
  "modified_at": "2026-11-01T12:00:00Z"
}
```

`type` may be `file`, `directory`, `symlink` or `other`; symlinks are described without their targets, and only regular files can be downloaded. Directory pagination is **best effort**, not a stable snapshot: concurrent filesystem changes can move or omit entries between offsets. The agent rejects invalid limit/offset/path or non-directory listing targets. Agent errors currently have the less-structured shape `{ "error": "message" }`. This difference from the controller error envelope is a known protocol cleanup target, not an implementation fact to conceal.

## Controller routes

| Method and path | Response / behavior |
| --- | --- |
| `POST /api/v1/session` | Local-session activation from header secret; `204` with session cookie |
| `GET /api/v1/health` | `{ "status": "ok", "mode": "controller" }`; proves only local HTTP readiness, **not peer connectivity** |
| `GET /api/v1/hosts` | `{ "hosts": [ { "id": "...", "name": "...", "status": "not_checked" } ] }` |
| `GET /api/v1/hosts/{host}/shares` | Forwards peer `/v1/shares`; same share schema |
| `GET /api/v1/hosts/{host}/shares/{share}/entries?path=...&limit=...&offset=...` | Forwards peer `/v1/list`; same listing schema |
| `GET /api/v1/hosts/{host}/shares/{share}/download?path=...` | Forwards peer `/v1/download`; may pass one `Range: bytes=...` header; streams with checked advertised length |

The controller explicitly refuses to proxy unregistered host IDs, and it does not expose `add-host`, `remove-host`, arbitrary URL fetch, uploads, or mutation over HTTP. Host add/remove are local CLI commands. The gateway reloads catalog changes from disk on API reads, but its in-memory peer token map is only read from `MASTERDRIVE_PEER_TOKENS_JSON` at startup. After adding a peer, restart the controller with its token or it will return `403 peer_token_missing`. Catalog removal is not remote credential revocation. `status: "not_checked"` is intentional: hosts are **not continuously probed**.

Controller errors use:

```json
{ "error": { "code": "peer_unavailable", "message": "peer could not be reached" } }
```

Likely error families include `session_required` (`401`), `invalid_host` (`403`), `unknown_host` (`404`), `peer_token_missing` (`403`), `peer_unavailable` (`502`), `peer_error` (`502`), `invalid_peer_response` (`502`), `invalid_range` (`400`), `invalid_if_range` (`400`), `range_not_pinned` (`428`), `bad_path` (`400`), `method_not_allowed` (`405`), and `not_found` (`404`). Exact status varies by route and underlying condition; clients should use codes and HTTP status rather than parse prose. Upstream agent error details are deliberately not blindly exposed to browsers.

## Transfer semantics

- Downloads are **read-through streaming** from one source. No replication, retry-across-hosts, cache guarantee or background sync occurs.
- The gateway disables HTTP redirects for its default client. It sets the peer token itself; browser requests cannot pick arbitrary peer origins.
- A missing upstream `Content-Length` is rejected before download headers. A premature EOF after headers causes the gateway to abort the stream, so consumers see an incomplete transfer; no checksum or end-to-end digest is specified yet.
- The controller forwards a single byte range **only** when the caller supplies exactly one syntactically strong If-Range ETag. Unpinned ranges fail with HTTP 428 (`range_not_pinned`); duplicate/invalid range or validator headers fail with HTTP 400. The peer must return HTTP 206, the exact strong ETag, and matching Content-Range/length. The current live-file agent does not issue immutable strong ETags, so its ranges cannot satisfy this precondition. See [gateway pinned-range boundary](security/gateway-pinned-range.md).
- A source modification during a transfer is not version-locked. The gateway validates response headers, not source coherence or content integrity. Consumers should verify their own content; future protocol versions need authenticated immutable representations, revocation fencing, and qualified resumable transfer design.

## Compatibility and future versions

The `v1` path segment identifies the current protocol family; it does not imply fully stabilized public compatibility. Future agents and controllers should negotiate capabilities and minimum/maximum supported protocol versions before enabling new effects. Unknown JSON fields should be ignored by readers where safe, while unknown write capabilities must be denied.

Planned extension points—**not v0.1 endpoints**—are authenticated peer enrollment, device identity attestation, key rotation and revocation, source-health probing, capacity and quota reporting, indexed search, SMB/NFS/WebDAV adapters, upload staging and atomic commit, move/rename/delete with scoped write grants, conflict/version preconditions, mount support, and optional real distributed placement/replication. Each needs a separate authorization and failure-semantics gate.

## Required interoperability and security checks

1. Agent rejects unauthenticated requests and bad tokens on **every** endpoint, including listings and downloads.
2. `/v1/list` and `/v1/download` reject `../`, encoded traversal, absolute paths, invalid separators and platform-specific special names; symlink/junction escapes cannot leave the pinned export root during concurrent mutation. Test on native Windows and Linux.
3. A new controller cannot register a device over HTTP; only local CLI enrollment is available. Remote non-TLS peer origins fail canonicalization; self-signed/incorrect-hostname certificates fail verification.
4. A source missing or offline produces a bounded gateway error. No catalog status is elevated from `not_checked` to `online` without a real probe.
5. Browser never sees peer tokens in HTML, JSON, URLs, console logs or download headers. Local session cookie is required; cross-origin and DNS-rebinding scenarios fail.
6. A transfer with truncated data or a malformed range does not silently become a successful full file. Test range boundaries and source changes during transfer.
7. Revocation tests distinguish catalog removal, controller token removal, source token rotation, and transport membership revocation; each is a separate effect.
8. Cross-build and native execution tests on Windows and Linux with an approved encrypted route precede any claim of real multi-host qualification.

References: [Tailscale Serve](https://tailscale.com/docs/features/tailscale-serve), [Tailscale grants](https://tailscale.com/docs/features/access-control/grants), [WireGuard protocol](https://www.wireguard.com/protocol/), [Microsoft SMB security](https://learn.microsoft.com/en-us/windows-server/storage/file-server/smb-security-hardening), [Linux FUSE](https://docs.kernel.org/filesystems/fuse/fuse.html), [rclone WebDAV server](https://rclone.org/commands/rclone_serve_webdav/).
