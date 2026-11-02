# Exact HTTP range-response preflight (source-only)

**Status:** experimental pure library, not wired to an agent, controller, WebDAV bridge, mount, or live drive. No remote service is configured or authorized by this code.

The future read-only mount adapter must not combine bytes from different source versions. The `internal/readcontract` package adds a deliberately conservative consumer-side metadata gate. Given a trusted, independently authorized file size, source-issued **strong** ETag, and requested offset/length, `ValidatePartial` accepts only a single `206 Partial Content` response with the exact ETag, `Content-Range`, and `Content-Length`. It rejects weak/missing/multiple validators, changed total size, incorrect byte windows, ambiguous lengths, compression and transfer encodings, invalid arithmetic, and a full-body `200` fallback.

This is **not** a source-issued version protocol. A malicious peer can lie consistently about an ETag. A mutable file may change after the response header. A strong ETag only helps when its generation and backing bytes are trustworthy. The caller must still authenticate the source and per-share grant, bind a stable host/share/path, pin an immutable representation or verified source version, enforce exact body byte count and cancellation, and fail closed on revocation, timeout, EOF, or offline peer. The validator does not read a body, authorize a mount, install a driver, or enforce transport identity.

The strict metadata contract is intentionally narrower than generic HTTP range support; clients requiring compressed or chunked responses need a separately reviewed and tested protocol. An empty file is not represented by a positive-length 206 range; metadata and zero-byte opens require a distinct contract.

Tests: `go test ./internal/readcontract -count=1`; full Windows/Linux CI and independent hostile review remain release gates. No drive export or Tailscale policy change is part of this source-only increment.

See [mount design](../mounts.md), [protocol v1](../protocol-v1.md), and [security acceptance](acceptance.md).
