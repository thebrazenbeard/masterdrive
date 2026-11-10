# Double-snapshot checkpoint consistency gate (prototype)

The read-only AnchoredReadFloor now obtains two sequential snapshots from each source and rejects any observed revision/fingerprint drift or read failure. Scripted readers test journal drift, same-revision authority mutation, a second-read failure, and stable agreement.

Evidence boundary: source-level mitigation and committed regression fixtures only until exact-head Windows/Linux CI is confirmed. These four reads are NOT atomic. An attacker can change a source after the final read, can roll back both sources together, or can substitute a coordinated pair of snapshots. A protected monotonic authority, authenticated pairing and revocation, cross-process durability, and read-admission integration remain mandatory. No mount or live share authorization is implied.

Host safety: no Windows host execution, Tailscale configuration change, drive export, or mount is authorized by this branch. The change is Go-only and uses no paid dependencies.
