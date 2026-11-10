# Complete-journal rollback counterexample

The test `TestJournalCompleteRollbackCounterexample` records a valid revision-1 journal, commits revision 2, replaces the journal with the complete revision-1 bytes, and observes that a reopened store accepts revision 1.

This is a **passing test for an unresolved vulnerability**, not a regression asserting safe rollback resistance. SHA-256 record checksums detect corruption but are not keyed authentication and cannot distinguish a valid old snapshot from current authority. The cooperative sidecar lock does not protect against external filesystem modification.

Before journal floors can authorize any real read or mount, require a separately protected, monotonic revision/fingerprint authority; explicit authenticated pairing and revocation; per-read verification; and Windows/Linux crash, cross-process, hostile replacement, and filesystem-permission tests. The authority must fail closed when inaccessible or inconsistent. No anti-rollback protection, live mounting, drive exports, or host qualification is supplied by this test.

Test scope: disposable temporary files only. No Tailscale policy or host drive changes.
