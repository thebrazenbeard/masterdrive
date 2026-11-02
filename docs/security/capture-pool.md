# G3 M0 capture-pool resource gate (source-only)

**Status: experimental library on a draft branch.** Not a mounted filesystem, live agent feature, deployed cache, user authorization service, or host-wide memory guarantee. This increment builds on the G3 sealed byte/range prototypes. It does not modify the G1 agent/controller, Tailscale, Taildrive, shares, or operating-system mounts.

## Contract

`versioncontract.NewCapturePool(maxBytes, maxSnapshots)` constructs a locally owned accounting pool. Its `Capture(ctx, reader, limit)` reserves **limit + 1 bytes** before invoking the existing `Capture` primitive, including the byte needed to detect oversized sources. It also reserves one snapshot slot. Admission is immediate: an exhausted pool returns `ErrBudgetExhausted` without reading from the source.

A successful capture returns a `Snapshot` whose `OpenRange` accepts only the exact captured ETag and valid byte bounds. Each opened reader holds a reservation lease. `Snapshot.Close` rejects new opens; **the reservation remains held until every previously opened reader is closed**. Calling Close twice or closing a reader twice must not double-release capacity. An unsuccessful capture releases both reservations. `Usage` reports the logical reservation count and bytes.

This design avoids treating closed-but-still-readable buffers as freed memory. It is intentionally fail-closed under pressure; it does not evict arbitrary snapshots or silently serve a different file version.

## Limits and unresolved hazards

- **Not a hard memory bound.** Go's allocator, `io.ReadAll` capacity growth, upstream readers, hash state, HTTP buffers, and independent pools can consume additional memory. Reservations are pessimistic logical payload allowances, not RSS measurements. There is no process-wide pool registry or per-share quota.
- **Not a revocation fence.** Closing a Snapshot stops new opens but does not interrupt readers already opened. A production handler needs independent durable grant state, request/stream authorization, cancellation and socket-close tests.
- **Not a coherent source snapshot.** A copy made during concurrent writes may be a mixed source revision even though the captured bytes become immutable.
- **No owner policy or identity.** The library does not open paths, accept HTTP requests, persist content, expose Taildrive, authorize a share, select a storage directory, or assign a source principal.
- **No automatic lifecycle management.** The caller must close snapshots and all readers. Missing Close calls exhaust the pool; they do not grant unbounded admissions. Context cancellation cannot forcibly interrupt a blocked upstream Read.

## Acceptance and next gates

Source tests cover invalid configurations, over-limit and canceled captures, admission exhaustion, failure rollback, stale validators, closed snapshots, double Close, readers surviving a snapshot Close without early quota release, and concurrent open/close. Windows and Linux CI and independent hostile review are required before integration.

Before a real mount or network endpoint: design and qualify process-wide and per-share quotas; use bounded allocator accounting or an OS-level memory limit; establish owner-local authenticated pairing, durable revocation, transport identity, and in-flight stream shutdown; validate real Windows/Linux filesystem behavior and two-host interoperability on disposable owner-approved shares.

See [M0 protocol](../protocol-m0.md), [pairing integration design](../security/identity.md), and [Taildrive interop](../interop/taildrive.md). No live storage was touched by this source experiment.
