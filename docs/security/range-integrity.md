# MasterDrive range/version integrity (design checkpoint)

Status: controller guard implemented in this branch; **strong end-to-end resumption is not yet supported by the native agent**.

## Why a byte range is not a file version

RFC 9110 requires a **strong validator** when a client wants to combine partial responses into a coherent representation. `Content-Range: bytes 1024-2047/4096` and a matching byte count prove only the location and length of one piece, not that different requests read the same file content. A size and modification-time-derived value can miss rewrites and must **not** be advertised as a strong ETag. Go's `http.ServeContent` supports conditional range requests when the origin supplies an ETag, but does not invent one.

Primary references:

- [RFC 9110 §8.8.1, §8.8.3, §13.1.5 and §14](https://www.rfc-editor.org/rfc/rfc9110.html): strong/weak validators and `If-Range`.
- [Go `net/http.ServeContent`](https://pkg.go.dev/net/http#ServeContent): conditional range and ETag behavior.

## Current controller guard

- Unconditional `Range: bytes=...` remains available for single-request partial reads. **Do not advertise it as verified multi-request resume.**
- `Range` with `If-Range` accepts one syntactically strong, quoted ETag only. Weak ETags, dates, malformed tags and multiple validators are rejected before a peer request.
- It forwards the validator to the peer. If the representation changed and the peer returns `200`, the controller responds with `412 representation_changed` instead of accidentally delivering a full file to a range-assembling client.
- If a peer returns `206` without the **exact same strong ETag** and valid `Content-Range`, the controller returns `502`; no bytes/attachment headers are forwarded.
- Only valid strong ETags from a trusted peer are forwarded to the browser. The controller never creates a faux strong validator from size/modtime.
- If the agent does not send a strong ETag (currently the case), guarded resume is intentionally rejected. HTTP `If-Range` normally allows falling back to full `200` content; the gateway's `412` is a deliberate stricter protocol policy for storage safety, and clients must handle it by discarding cached ranges and downloading from byte zero.

## Gate before enabling verified resume

1. Native agent must serve **immutable snapshots**, not the current mutable file descriptor without isolation. A reliable implementation could use a snapshot-capable filesystem or content-addressed immutable backing. Hashing a mutable file and then rereading it is **not** sufficient to guarantee the bytes served match the hash.
2. The agent should derive a strong ETag from exact snapshot bytes, retain that snapshot for subsequent conditional requests (with bounded retention/revocation), and atomically enforce `If-Range` against its snapshot identity.
3. Controller should verify the peer's provenance over authenticated encrypted mesh transport; an ETag asserted by an untrusted peer does not prove correctness.
4. Test a rename/write/truncate during and between two range GETs, mtime-preserving modification, validator reuse, stale snapshot expiration, and peer revocation; ensure no mixed content is marked complete.
5. For very large files, define storage/bandwidth cost and retention bounds; avoid an unconditional SHA-256 pass and temp duplicate per request.

Until all gates pass, release notes and UI must describe transfers as read-only downloads and byte-range fetches, not cryptographically verified resume.
