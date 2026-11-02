# Catalog concurrency and recovery (v0.1)

The controller reads public peer metadata from `<data-dir>/hosts.json`. The
registry intentionally stores **no peer access tokens**. CLI host mutations
update this file. This procedure concerns only catalog metadata; deleting a
host entry does not revoke a credential or delete data on another machine.

## Concurrent CLI commands

Each `add-host`, `remove-host`, or health mutation creates an exclusive
`hosts.json.lock`, re-reads the current catalog while holding that lock, then
commits a replacement. This prevents two independently started CLI processes
from silently overwriting each other's registered hosts. Waiters retry for up
to five seconds; if busy, the CLI returns an error and does not commit its
proposed change. Re-run it after confirming that the other writer finished.

The lock contains only a diagnostic PID and UTC timestamp. It is intentionally
not removed based on age or PID alone: killing a live writer's lock can allow
two writers to commit competing snapshots. This mechanism is designed for
the same local filesystem, not unqualified network shares or synchronized
folders.

## Recovering a stale lock after a crash

1. Stop MasterDrive CLI writer processes (the serving controller only reads).
2. Verify no MasterDrive process still owns an in-flight catalog write.
3. Make a backup copy of `hosts.json` in a private location, if it exists.
4. Only then manually remove `hosts.json.lock`; start one writer and verify
   the catalog list. Do **not** remove `hosts.json` to clear a lock.

If catalog JSON is missing after it was established, corrupt, invalid, or
oversized, the program fails closed rather than replacing it with an empty
registry. Restore the last valid backup through normal filesystem recovery;
the agent must never invent peer grants or new credentials as recovery.

## Platform limits

The serialized-writer guarantee does not automatically imply power-loss or
crash-safe persistence on every filesystem. Go explicitly notes that
[`os.Rename`](https://pkg.go.dev/os#Rename) is not guaranteed atomic on
non-Unix platforms. Windows crash/restart, unexpected process termination
during rename, filesystem durability and data-directory permissions must be
qualified before production use. The exclusive creation contract uses Go's
[`os.O_EXCL`](https://pkg.go.dev/os#pkg-constants) with `O_CREATE`.
