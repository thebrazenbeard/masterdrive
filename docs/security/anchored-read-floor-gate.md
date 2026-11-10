# External checkpoint floor read gate (prototype)

The read-only `AnchoredReadFloor` requires exact revision and fingerprint agreement between the local journal and an independently supplied authority. Missing authority, unavailable records, restored historical journal snapshots, and same-revision equivocation fail closed. The regression uses a **memory-only test oracle**; its state is neither durable nor protected.

**Security boundary:** This gate does not create, authenticate, protect, or advance the authority. A malicious actor able to roll back both inputs defeats it. It does not solve journal TOCTOU, crash recovery, cross-process locks, pairing, per-read revocation, or Windows reparse-point security. It is not wired into the real read/mount admission path and must not be represented as authorization-ready.

**Acceptance before integration:** independently protected monotonic authority with authenticated operator provisioning and revocation; atomic checkpoint publication and restart recovery; malicious replacement tests; Windows/Linux exact-head CI; independent hostile review; source-level integration proof; and explicit operator authorization for host qualification. No real shares, Tailscale policies, mounts, or host processes are touched.
