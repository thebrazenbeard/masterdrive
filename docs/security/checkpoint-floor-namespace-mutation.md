# Namespace mutation regression — evidence boundary

The synthetic tests in `checkpoint_floor_namespace_mutation_test.go` exercise concurrent revision advances for six independent source/audience/share triples, including concatenation-collision candidates and variations in each identity dimension. They verify final fingerprints and revisions remain separate, that stale revisions are rejected, and that empty identity components cannot mutate a valid relationship.

These tests exercise only `MemoryFingerprintFloors`, not a filesystem, multiple processes, trusted pairing, or any real read path. They do not establish durable isolation. A malicious or defective persistent implementation can pass this suite and still alias keys, lose records on restart, or accept rollback after restoring an old directory.

Before using a durable store for authorization: execute this suite against its own fixture, demonstrate cross-process contention and crash/restart recovery on Windows and Linux, protect state against rollback, prove operator-authorized provisioning, and revalidate revocation throughout reads. No mount or drive export is authorized by this change.
