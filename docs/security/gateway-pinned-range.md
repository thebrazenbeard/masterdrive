# Gateway pinned-range precondition (source-only)

Status: experimental source hardening; not a versioned mount, paired mesh, immutable source, or deployment. This document describes internal/server/server.go and internal/server/gateway_range_precondition_test.go on the candidate branch.

A browser/controller caller requesting a partial download through the gateway must provide exactly one byte Range and exactly one syntactically strong If-Range ETag. Missing If-Range returns HTTP 428 before contacting the peer. Duplicate/empty ranges, weak/date/multiple validators, and an orphan If-Range return HTTP 400 before contacting the peer. A peer must return HTTP 206 with the exact same strong ETag, valid Content-Range, and known byte count. The gateway advertises Accept-Ranges only when forwarding a syntactically strong peer ETag.

This is deliberately stricter than ordinary HTTP range behavior. The current live-file agent uses http.ServeContent and does not issue an immutable content-derived strong ETag. Therefore partial downloads from that agent cannot satisfy this gateway gate. Unconditional full downloads still work, but are not immutable snapshots or resumable transfers.

Tests: go test ./internal/server -run TestGateway -count=20. Negative cases assert that the peer was not contacted and file headers were not leaked. Existing positive and mismatched-peer tests now use a matching strong fixture validator.

Hostile challenge: A malicious peer can lie consistently about a strong ETag and return mixed bytes. Syntactic validation does not authenticate the peer, prove content coherence, or prevent revocation races.

Compatibility challenge: Clients that issue Range without If-Range now receive HTTP 428. This intentional safety trade-off needs real client interoperability testing before any mount release.

No real drives, Tailscale grants, Serve configuration, remote hosts, operating-system mounts, or storage pools are involved. Next gates: independently trusted immutable-version issuance, authenticated pairing and durable revocation, body verification, isolated Windows/Linux client tests, and independent hostile review. The separate draft readcontract experiments in PRs #10-#11 do not automatically integrate with this gateway.
