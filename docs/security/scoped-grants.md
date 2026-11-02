# Scoped bearer grants: experimental source-handler seam

Status: **isolated source experiment, not integrated or deployed**. This
proposal extends MasterDrive's source agent so an individual bearer credential
can read only specific configured shares.

## The authorization boundary

The existing `NewHandler(exports, token)` deliberately gives one token access
to every configured export. Its source-side authorization is a useful first
gate, but rotating or distributing that token grants more access than many
operators intend.

The additive `NewHandlerWithGrants(exports, grants)` constructor takes grants
such as:

```go
agent.NewHandlerWithGrants(exports, []agent.AccessGrant{
    {BearerToken: tokenForControllerA, ShareIDs: []string{"photos"}},
    {BearerToken: tokenForControllerB, ShareIDs: []string{"documents"}},
})
```

Tokens are supplied through the embedding application's approved secret
mechanism. Neither the API nor this document defines a credential storage
file, command-line secret option, or pairing channel.

- Every valid token maps to a nonempty, explicit set of registered export IDs.
- Unknown tokens receive `401`; known tokens attempting an ungranted share
  receive `404`, the same as a nonexistent share.
- `GET /v1/shares` includes only the shares in that token's grant.
- The same share check occurs before opening a directory or file; listing and
  downloads cannot bypass inventory filtering.
- A duplicate token, unknown share ID, repeated share ID within one grant,
  weak token, empty grant set, or more than 256 grants fails handler creation.
- Responses retain `Cache-Control: no-store`; no token or local export path is
  emitted.

The original `NewHandler` signature and its all-shares semantics remain
unchanged for compatibility. Existing CLI `agent` configuration still uses
`MASTERDRIVE_AGENT_TOKEN` and the old constructor; therefore **the CLI does
not expose per-share credentials yet**. This change should not be described
as a shipped pairing or user-permission feature until a CLI/service
configuration and host test are integrated.

## Migration and revocation

1. Preserve the old handler for existing deployments and prove that all
   explicitly configured exports remain accessible with the old credential.
2. In a controlled host/service integration, migrate to the scoped constructor
   using newly generated independent tokens, each bound to an intended share
   set. Deliver those tokens only to approved controllers over a separate
   trusted channel; do not copy them into the host catalog.
3. Stop the agent listener, drain/cancel active requests, close the old
   handler, and start a new instance containing only approved grants. Revoke
   the corresponding tailnet/transport policy separately. **Merely deleting
   a controller catalog entry does not revoke an agent token.**
4. Validate from the previously authorized peer that the rotated token no
   longer works, the new token sees only the expected shares, and an in-flight
   download is properly terminated. These are live qualification steps, not
   verified properties of this isolated patch.

The experimental handler's grant map is immutable for its lifetime. It does
not include a network-accessible grant mutation API, unattended key rotation,
user identities, tamper-evident audit, expiration, one-time pairing, or
per-file permissions. A controller's `MASTERDRIVE_PEER_TOKENS_JSON` still
contains **one token per host ID**, so a controller needs a grant covering
every share it should access from that host.

## Threats and acceptance checks

- A stolen bearer token grants only its listed share set but remains reusable
  until the source service rotates it. A token is **not** an identity proof.
- TLS and a private, operator-approved transport remain mandatory between
  devices. Scoped tokens do not validate Tailscale users, tagged nodes,
  device grants, or gateway headers.
- Test all three routes for bad and limited tokens; verify that ungranted
  downloads never emit file bytes or local paths.
- Test that invalid or ambiguous grants prevent startup, and that the legacy
  handler still permits all explicitly configured exports.
- Test Windows reparse-point/symlink escape attempts, listener shutdown,
  token rotation, and overlapping in-flight downloads in an authorized,
  controlled multi-host environment before considering the feature released.

This is a least-privilege staging seam for [identity and pairing design](identity.md),
not the completed identity or revocation system.
