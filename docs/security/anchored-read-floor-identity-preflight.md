# Anchored checkpoint gate: identity preflight

The read-only `AnchoredReadFloor` now applies the existing relationship-key contract (nonempty source, audience and share) before calling either journal or authority adapter. This prevents a permissive or faulty adapter from turning an empty identity into a plausible floor and avoids invoking adapters for malformed requests.

The regression checks all three empty identity components, zero returned state, no adapter calls, and the unchanged valid-identity double-snapshot path.

**Evidence:** source and committed Go regression only until exact-head Windows/Ubuntu CI completes. This does not make the identity cryptographically authenticated: arbitrary nonempty strings still pass preflight. It does not establish protected monotonic state, atomic snapshots, per-read revocation, host identity pairing, or permission to mount or export a drive. The gate is not wired to real read admission.

**Host boundary:** no Windows host process, foreground UI action, drive export, Tailscale policy edit, mount, or paid dependency is required by this change.
