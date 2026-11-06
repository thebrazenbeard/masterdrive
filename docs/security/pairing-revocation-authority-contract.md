# Pairing and revocation authority: implementation contract (proposal)

Status: **design-only, not implemented, not security-qualified**. Based on `main` at `d82e69f8c9c82e846bd9b118c7ffa8dc9ef2025b` and the experimental journal rollback demonstration in draft PR #32. Windows and Linux only. No automatic exports, tailnet ACL edits, mount activation, or paid infrastructure.

## Current source boundary

The README documents explicit opt-in exports, bearer credentials scoped to share IDs, a manual host catalog, and a controller that loads peer tokens at startup. Removing a host from the catalog **does not revoke the agent credential**. Existing grants are bearer secrets, **not authenticated device pairing**. The experimental generation journal in PRs #27–#32 is not on main; #32 demonstrates replay of a previously valid journal snapshot. It cannot authorize access or provide rollback-resistant revocation.

## Authority and threat model

* Source owner controls exports and grants. The source agent is the **enforcement point**. A controller, tailnet membership, DNS name, catalog entry, or local journal cannot grant access by itself.
* Every authorization is bound to an exact source identity, controller identity, share ID, permission (`read` only), grant ID, expiry and monotonic authority epoch. Never treat a display name, IP, hostname or Tailscale node address alone as a cryptographic identity.
* Pairing requires explicit source-owner consent and a mutually authenticated proof-of-possession exchange. Use audited standard cryptographic libraries and domain-separated signed messages; do not invent a bespoke signature or key agreement scheme.
* Trust bootstrap must bind the verified public-key fingerprint to the human-approved device and share. Out-of-band comparison or an equivalent authenticated owner-confirmation path is required; unverified key exchange is susceptible to active interception.
* No private signing keys, bearer tokens, or recovery secrets in the catalog, Git, logs, browser JS, or URL. Persist credentials in OS-appropriate protected storage with a documented headless fallback that fails closed when unavailable. Key replacement is a new consent event, not silent continuity.
* Source agent must consult its **own authoritative grant state** before serving metadata and before **each** staged range request. The client must check the pinned object version, source identity, share and epoch against the response. Fail closed if authority is unavailable, identity changes, version differs, epoch regresses, grant expires, or revocation is observed.
* A source-side revocation commits an epoch increase and denies new reads **before** acknowledging success. Concurrent in-flight reads must be cancelled or bounded; cancellation does not guarantee interrupting a blocked OS/network operation. Report this residual exposure. Do not promise remote clients erase bytes already received.
* Durability must survive crash/restart, restored local backups and simultaneous independent processes. A same-disk journal, unkeyed hash, owner marker or copied checkpoint is not an independent anti-rollback anchor. If a trusted monotonic anchor cannot be established, the system must not claim rollback-resistant revocation or authorize a native mount based on it.
* Offline policy: deny newly authorized reads when the source cannot validate the grant/epoch; never silently fall back to a cached allow decision. Explicitly bound stale UI inventory and do not represent it as authoritative availability.

## Proposed state machine

`UNPAIRED -> PENDING_OWNER_APPROVAL -> ACTIVE -> REVOKED`; timeout/denial returns to `UNPAIRED`. `REVOKED` cannot transition to `ACTIVE` under the same grant ID. Re-pairing creates a fresh grant ID and epoch. `SUSPENDED` (authority unavailable) is a deny state, not a permission state. Separate device-key rotation from share-grant rotation.

## Minimum version-safe read preconditions

A read request carries an authenticated source/consumer identity, grant ID, share ID, canonical relative path, requested byte range, pinned strong object version and observed authority epoch. The source checks path confinement, current grant and revocation epoch, immutable version equality and bounded range **before** reading. On mismatch, refuse the transfer and discard any staged partial output. The client commits bytes to a mount-visible cache only after complete length, version, epoch and authenticated peer checks. Never stitch bytes from different versions or authority epochs into one file view.

## Synthetic acceptance tests (not yet executed)

1. Owner approves exact identity/share; wrong identity, wrong share, expired invitation, replayed invitation and replaced key all fail.
2. Revoke while a ranged read is staged; no further authorized range starts, incomplete bytes never become mount-visible, and a blocked I/O path times out or reports bounded failure.
3. Crash immediately before and after revocation acknowledgement; restart must never restore an acknowledged grant. Restore earlier local state; absent independent trusted anchor, deny and report the gap.
4. Two independent source-agent processes contend over authority updates; exactly one durable ordering, no stale authorization success.
5. Re-pair after revoke produces a distinct grant ID and higher epoch; replay of the old signed approval remains denied.
6. Simulate offline source, DNS rebinding, changed Tailscale node identity, wrong TLS peer, token disclosure, and host catalog deletion. Catalog deletion alone must not be represented as revocation.
7. Change file contents between range requests and during a staged read; version mismatch discards the entire staged result, never mixes generations.
8. Execute on disposable Windows NTFS and Linux filesystem fixtures, including symlink/reparse attacks and process crash injection. Require exact-head CI plus independently recorded hostile review before any integration merge.

## Integration sequence and gates

1. Implement a **source-local, no-network** grant state machine with fake-clock tests and deterministic crash/fault injection in a separate PR based on current main. Keep the existing bearer route unchanged until migration is proven.
2. Specify and implement authenticated handshake with standard library primitives, human owner approval, and credential storage; separately review threat model and wire format.
3. Add source-enforced revocation and version-pinned staged reads, then cross-platform synthetic integration. Only then design a native read-only mount adapter.
4. Perform operator-authorized two-host tailnet qualification on non-sensitive disposable fixtures. A passing local CI suite is **not** proof of a live mesh, mount, pooling, or backup.

**Hostile-review hold:** This proposal does not solve trusted monotonic anchoring. An attacker able to roll back every local authority copy and its keys can make an old state appear internally valid. Until that threat is addressed by an independent trusted authority or an explicitly narrower security claim, keep rollback-resistant revocation and native mount qualification blocked.
