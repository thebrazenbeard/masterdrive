# Bounded journal replay (prototype only)

Journal replay now uses a 64 KiB buffered `ReadSlice` per newline-delimited JSON record. An overlong record produces `bufio.ErrBufferFull`, which fails closed as `ErrFloorUnavailable` without proceeding to the commit callback. The regression appends a 70 KiB malformed record, verifies that read and commit fail, and checks that the rejected commit does not change journal size.

This is a memory-exhaustion mitigation, not a complete resource budget: the journal can still grow without bound across many small records, and replay remains O(total journal size). It does not establish anti-rollback, protected filesystem ownership, safe stale-lock recovery, crash consistency, Windows/Linux host qualification, or read/mount admission. No real drives or tailnet policy are touched.
