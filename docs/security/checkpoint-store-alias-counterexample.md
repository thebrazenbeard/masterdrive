# Checkpoint floor relationship alias — negative qualification evidence

Evidence class: synthetic source-level counterexample, not a storage implementation or security acceptance test.

`TestScopedFloorDoesNotPreventBackingStoreAlias` deliberately supplies two correctly provisioned `ScopedCheckpointFloor` adapters with a backend that ignores the source/audience/share arguments. A checkpoint committed through relationship A then appears as relationship B's floor. The test PASSES when this unsafe alias is reproduced. It must not be cited as proof of isolation.

A production store must derive collision-resistant, unambiguous namespace keys from the full provisioned identity tuple; bind the namespace to authenticated provisioning; reject missing or corrupt records; atomically compare-and-commit revision and fingerprint under concurrent processes; persist before acknowledging admission; and fail closed on ambiguous durability, rollback, replacement, symlink/reparse, permissions and locking errors. A store with a correct namespace but caller-writable state still permits rollback.

The current test uses only synthetic memory. It proves neither the Windows nor Linux filesystem semantics, disk durability, authenticated identity, Tailscale transport, revocation freshness, read cancellation, immutable snapshots, mount admission, nor any real-host qualification. Do not merge or wire the stacked grantproof prototype into a live read path without independent hostile review and exact-head Windows/Linux CI.
