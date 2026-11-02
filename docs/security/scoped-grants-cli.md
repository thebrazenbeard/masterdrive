# Source agent CLI: scoped bearer grants (G1 experimental)

This is a **read-only, opt-in, development-stage configuration**, not an automatic pairing or identity system. The source agent is still bound to loopback and requires an independently approved private HTTPS transport for another machine.

## Select exactly one secret mode

The original mode remains supported without changes:

- `MASTERDRIVE_AGENT_TOKEN`: one cryptographically random, at-least-32-character token authorizes **all** exports that this agent process explicitly configured.

To instead restrict different bearer credentials to named shares, provide a JSON array in `MASTERDRIVE_AGENT_GRANTS_JSON`, with this shape:

```json
[
  { "bearer_token": "<unique-random-secret-for-controller-A>", "share_ids": ["documents"] },
  { "bearer_token": "<different-random-secret-for-controller-B>", "share_ids": ["photos"] }
]
```

The bracketed strings above are placeholders, **not usable secrets**. Provision the real values through an approved secret-management mechanism into the agent's **process environment**. Do not put them in CLI arguments, checked-in `.env` files, shell history, shared logs, screenshots, issue comments, or URLs.

When using `MASTERDRIVE_AGENT_GRANTS_JSON`, ensure `MASTERDRIVE_AGENT_TOKEN` is completely **unset**, not just set to an empty string. Conversely, leave the grants variable unset in legacy mode. The source refuses to start when both or neither variables are present, and rejects empty/malformed/unknown-field/oversized grants JSON.

Configure the ordinary, repeated opt-in exports as before:

```sh
go run ./cmd/masterdrive agent -export documents=/absolute/approved/documents -export photos=/absolute/approved/photos
```

Each grant's `share_ids` must refer to existing exported IDs. Each bearer token must be unique, have at least 32 non-whitespace characters, and enumerate at least one distinct share. Maximum: 256 grants, with a 1 MiB environment-JSON size limit. No grant may implicitly expose every disk or unknown share. Credentials are not stored in MasterDrive's host registry.

## Observable authorization

- The authorized `GET /v1/shares` inventory contains only shares listed in the caller's grant.
- Listing, shallow search, and downloads for an ungranted share return the same generic `404` as an unknown share, without opening the source path.
- Unknown/incorrect bearer credentials receive `401` on all source endpoints.
- A controller continues to use one peer token per catalog host in `MASTERDRIVE_PEER_TOKENS_JSON`, so its configured token must grant all source shares that controller should access. A source may provision different tokens to different approved controllers.

The CLI still listens on `127.0.0.1:8789` by default; it never publishes a public listener, enables Tailscale Serve/Funnel, or enrolls devices. It prints only the number of configured shares and local listen address. The JSON is parsed strictly and credential material is not echoed in its error messages.

## Rotation and revocation limitations

Source grants are fixed for the lifetime of an agent handler. To remove or rotate a grant, stop its listener and active transfers, update the secret mechanism, and restart the agent; update each relevant controller's token map and restart that controller. Independently revoke the peer's private transport permissions. Catalog `remove-host` only removes a local metadata entry; it does **not** invalidate an agent bearer token.

This mechanism does **not** provide Tailscale node identity, authenticated pairing invitations, approval workflow, expiry, audited revocation, per-file ACLs, or in-flight stream cancellation by itself. Those remain [security acceptance gates](../security/acceptance.md). Before production use, qualify scoped grants on real Windows and Linux devices over an authorized private route, including malicious requests and active download cancellation.
