# Bounded HTTP range body staging (source-only)

**Status:** experimental, stacked on draft PR #10. Not wired to the agent, controller, pairing broker, WebDAV, Taildrive, a mount, or any live drive.

`internal/readcontract.ReadBoundedPartial` extends the header-only `ValidatePartial` preflight with a private in-memory body staging gate. It returns bytes only after exact advertised length and EOF, under an explicit maximum allocation (hard ceiling 16 MiB). Short, invalid, or non-progressing bodies fail without returning partial bytes; extra bytes fail **when the supplied Body exposes them** (a standard HTTP transport may stop exposing bytes at declared Content-Length). It closes the response body on completion or failure. The caller must supply a context with deadline and a non-nil closed-on-revocation channel; cancellation or observed revocation attempts to close the stream and suppress returned bytes.

**This is not an authorization or revocation system.** The supplied channel is only a cooperative notification input, not durable grant custody or verified identity. A malicious source can lie consistently about headers and content, and no cryptographic content hash is checked. A response body can still contain mixed source revisions. A generic `io.ReadCloser` is not guaranteed to unblock when closed, so caller-owned HTTP transport deadlines and a qualified network implementation remain mandatory. Revocation after successful return, or a race between the final grant check and the caller using bytes, requires a separate atomic authorization fence. Staged bytes are ordinary Go heap data, not securely zeroized or guaranteed to meet process RSS quotas.

A real mount adapter must additionally prove host/share/path identity, immutable source version or authenticated content digest, durable epoch-based revocation, atomic admission and delivery fences, offline/stale behavior, path safety, and actual Windows/Linux client interoperability. None is claimed here.

Local test command: `go test ./internal/readcontract -count=1`. Release requires exact-head Windows/Linux CI, independent hostile source review, and additional integration qualification. No real exports or Tailscale policy changes.


**Hostile review note (2026-11-02):** Aeonith Critic 0.1 (local Ollama) raised non-atomic grant/read races, which are real integration limitations rather than proof of an atomic revocation fence; its suggestion to add a pre-read gate was already present. Qwen2.5-Coder 1.5B Q8 (local Ollama) alleged unbounded reads, but the explicit 16 MiB cap and fixed-size buffer contradict that claim. Neither model supplies independent approval or replaces a human security review.
