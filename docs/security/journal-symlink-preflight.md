# Journal symlink preflight: limited defense in depth

This change rejects a pre-existing symbolic link at the journal or sidecar-lock path before the first open, and repeats the journal check before append. It adds disposable-file tests for both paths. The tests skip on hosts that cannot create symbolic links (notably some Windows environments).

**Security boundary:** Lstat-to-OpenFile remains a time-of-check/time-of-use race. Parent directory symlinks, junctions, Windows reparse points, malicious concurrent replacement, lock spoofing, rollback, partial writes, and power-loss durability are NOT solved. This must not be used for mount admission or live drive exports. Qualification requires protected parent directory ownership, handle-relative no-follow opens or equivalent OS-specific primitives, and cross-process adversarial tests on Windows and Linux.

Evidence: source change and committed tests only until exact-head CI confirms execution. No host actions or Tailscale policy changes.
