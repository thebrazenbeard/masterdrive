# MasterDrive candidate packaging (not a release)

**Status:** developer-only packaging proposal. These archives remain **unsigned,
unqualified, and not production deployment artifacts**. Nothing here installs a
service, configures Tailscale, exposes a share, creates a GitHub Release, or
grants a redistribution license. The repository currently has no selected
license; the repository owner must decide redistribution terms before an
actual public release.

## Scope

`scripts/package.py` cross-compiles the Go command using a locally installed,
patched toolchain and creates deterministic-format candidate archives:

| Target | Archive |
| --- | --- |
| Linux AMD64 | `masterdrive-<label>-linux-amd64.tar.gz` |
| Linux ARM64 | `masterdrive-<label>-linux-arm64.tar.gz` |
| Windows AMD64 | `masterdrive-<label>-windows-amd64.zip` |
| Windows ARM64 | `masterdrive-<label>-windows-arm64.zip` |

Each archive contains `masterdrive` or `masterdrive.exe`, the browser's
**required** `web/` assets, a compiled-binary-specific
`CANDIDATE-QUICKSTART.md`, architecture/security acceptance docs,
and a `BUILD-METADATA.json` file describing the exact source SHA, Go toolchain,
target and executable SHA-256. `SHA256SUMS` outside the archives covers the
archive bytes. This avoids the easy packaging mistake of shipping only the
controller executable, which requires a `web/` directory by default.

Artifacts are **neither a mounted drive nor an installer**. After extracting,
run from the top-level extracted directory and follow
`CANDIDATE-QUICKSTART.md` for commands invoking the **compiled executable**.
Its sample `add-host` and `controller` commands share one freshly generated
temporary `-data-dir`, rather than writing into the operator's default host
registry. After stopping both processes, verify the exact demo path and
remove only the known catalog file and empty directory non-recursively.
The source repository's README is deliberately *not* bundled because it
describes source-checkout workflows such as `go run`, which are inappropriate
for binary-only archives. The source checkout's Windows and Linux local smoke
tests must pass before using any real folder. Cross-host use still needs a separately approved
encrypted route and security qualification.

## Build locally

From a clean checkout of the exact commit you wish to test:

```sh
go test ./...
python3 -m unittest discover -s scripts -p 'test_package.py' -v
python3 scripts/package.py \
  --version dev-1 \
  --revision "$(git rev-parse HEAD)" \
  --out dist
(cd dist && sha256sum -c SHA256SUMS)
```

Prerequisites: Python 3.12+ (stdlib only), the pinned Go version from `go.mod`,
and Git for resolving the source revision. The packager refuses a dirty checkout
or a source tree whose `HEAD` differs from the declared revision. `GOTOOLCHAIN=local` prevents automatic
toolchain downloads during each build, so the installed toolchain must be
sufficient. The Go module currently uses no third-party runtime modules.

The label accepts only alphanumerics, dots, underscores and hyphens; the
revision must be an exact 40-character lowercase SHA. Output paths are
created exclusively: an existing archive or checksum list is **never
overwritten**. Use a new, empty output directory for another run.

**Partial failure handling:** if a build fails after producing one or more
archives, the script may leave those exact incomplete or intermediate
candidate files in the requested output directory. In that case no complete
`SHA256SUMS` index is written; **do not distribute or run partial artifacts**.
Inspect the failure, then use a fresh empty output directory for retry. If
cleaning the failed attempt, remove only the specific filenames that the
packager reported/created after verifying the directory; the packager itself
will not recursively delete output or overwrite existing files. A successful
manifest must cover **every requested target**.

To verify on Windows, use `Get-FileHash -Algorithm SHA256` for each archive and
compare its value to `SHA256SUMS`. Checksums catch accidental corruption, but
are not authenticity signatures; an attacker who can replace an archive can
also replace an adjacent checksum file.

## Manual CI operation

The optional `.github/workflows/package-candidate.yml` responds **only to an
explicit `workflow_dispatch` invocation**. It first runs source and packager
tests, then produces the four candidate archives on a GitHub-hosted Linux
runner, verifies checksums, and uploads a **7-day Actions workflow artifact**.
It has `contents: read` permission and contains no `release`, `push`,
`pull_request` or tag-publishing trigger. This is not an automatic deployment
or GitHub Release.

## Reproducibility and security limits

- `go build` uses `-trimpath`, `-buildvcs=false`, a zero linker build ID,
  `CGO_ENABLED=0`, and an exact target environment. It does not embed local
  source checkout paths or mutable Git metadata in executable content.
- ZIP and tar.gz entries have sorted names, fixed timestamps, owner IDs and
  permissions, and no symlink targets. Gzip has `mtime=0` and no source
  filename. Source assets come from a fixed allowlist, not an unbounded file
  glob that might accidentally package local secrets or build output.
- The Python unit tests prove deterministic **archive assembly** with fixed
  input bytes. They do not prove byte-for-byte identical Go binaries across
  different compiler releases, architectures or operating systems. Verify
  twice with an identical toolchain before claiming reproducible binaries.
- The build manifest and SHA256SUMS are not a signed provenance attestation.
  Code-signing, trusted identity, full SBOM, vulnerability scanning, artifact
  attestations, supply-chain review and release approval are separate gates.
- A successful candidate build says nothing about safe Tailscale pairing,
  per-share authorization, Windows reparse-point confinement, cryptographic
  resume integrity, actual storage pooling, or backup durability. See
  `docs/security/acceptance.md` and the issue roadmap.

Primary references: [Go build flags](https://pkg.go.dev/cmd/go),
[Python gzip deterministic timestamp](https://docs.python.org/3/library/gzip.html),
[GitHub manual workflows](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow),
[GitHub artifact retention](https://docs.github.com/en/actions/using-workflows/storing-workflow-data-as-artifacts).
