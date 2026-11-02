#!/usr/bin/env python3
"""Build reproducible *candidate* archives for the MasterDrive source prototype.

No releases are published. Artifacts are unsigned and not production-qualified.
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import zipfile


TARGETS = {
    "linux-amd64": ("linux", "amd64", "tar.gz"),
    "linux-arm64": ("linux", "arm64", "tar.gz"),
    "windows-amd64": ("windows", "amd64", "zip"),
    "windows-arm64": ("windows", "arm64", "zip"),
}

# An allowlist prevents packaging build caches, temporary files, secrets, or
# unrelated local content that happen to exist beside the source checkout.
RUNTIME_FILES = (
    "docs/architecture.md",
    "docs/security/acceptance.md",
    "web/index.html",
    "web/style.css",
    "web/app.js",
    "web/paths.mjs",
)
QUICKSTART = """# MasterDrive unsigned candidate: local quickstart

This archive contains a **read-only developer prototype**, not an installed or
production-qualified storage mesh. Its binaries are unsigned. Do not run them
as administrator/root, expose them publicly, or use them as a backup. There is
no native drive letter, distributed disk pool, upload, sync or replication.

The `web/` directory is required by the controller. Open a terminal **inside
this extracted archive's top-level folder**, which contains the executable
and `web/`. Nothing starts automatically.

## Local-only test using one harmless, explicitly approved folder

Create a temporary folder with a non-sensitive test file. Do not choose a
drive root, user profile, secrets directory, or irreplaceable files. Use
separate terminals with the **same current directory** and a random secret
of at least 32 characters supplied through your normal secret mechanism.
Never paste the secret in a command line or commit it to a file.

### Linux (compiled executable)

1. In terminal A, provision `MASTERDRIVE_AGENT_TOKEN` in the environment,
   then start `./masterdrive agent -export demo=/absolute/approved/test-folder`.
   This binds only `127.0.0.1:8789` and exports only that named directory.
2. In terminal B create a fresh, private demo catalog directory and register:

   ```sh
   DEMO_STATE="$(mktemp -d)"
   ./masterdrive add-host -data-dir "$DEMO_STATE" -name local-demo -url http://127.0.0.1:8789
   ```

3. Copy the printed host ID. Securely provision `MASTERDRIVE_PEER_TOKENS_JSON`
   as a JSON object mapping **that host ID** to **the same agent secret**.
   Then run `./masterdrive controller -data-dir "$DEMO_STATE" -web-dir web`
   in that same terminal B. Keep the directory path for cleanup.

### Windows PowerShell (compiled executable)

1. In terminal A, provision `MASTERDRIVE_AGENT_TOKEN` through an approved
   secret mechanism and start:

   ```powershell
   .\\masterdrive.exe agent -export demo=C:\\ApprovedTestFolder
   ```

2. In terminal B create a fresh, private demo catalog directory and register:

   ```powershell
   $demoState = Join-Path ([IO.Path]::GetTempPath()) ('masterdrive-demo-' + [Guid]::NewGuid().ToString('N'))
   [void][IO.Directory]::CreateDirectory($demoState)
   .\\masterdrive.exe add-host -data-dir $demoState -name local-demo -url http://127.0.0.1:8789
   ```

3. Copy the printed host ID. Securely provision `MASTERDRIVE_PEER_TOKENS_JSON`
   with that ID mapped to the agent's secret, then run:

   ```powershell
   .\\masterdrive.exe controller -data-dir $demoState -web-dir web
   ```

Both platforms: open the **exact local** `http://127.0.0.1:8788/#session=...`
link printed by the controller. The fragment is a sensitive, one-time browser
session secret. The browser never receives peer bearer tokens. Use Ctrl+C to
stop both processes when done. The controller reads peer token mappings only
at startup: after registering a new host or changing a token, restart it.
`remove-host -data-dir DEMO_DIRECTORY -id HOST_ID` only forgets an entry in
that chosen catalog; it does **not**
revoke or rotate a source token.

**Clean up after stopping both processes:** verify that the demo directory
recorded in terminal B is the *newly created* temporary path and contains
only the generated `hosts.json` metadata. Remove that exact file, then remove
the now-empty directory **without recursive deletion**. If anything unexpected
exists, leave it for inspection. Never recursively remove an unverified path
or touch your normal MasterDrive registry. The temporary catalog is separate
from your default user configuration by design.

## Remote access is NOT automatic

The example above is loopback-only. Access between two computers requires
separately approved, encrypted private HTTPS transport such as Tailscale Serve
for the **agent only** plus correctly restricted network grants. Do **not**
use public Tailscale Funnel, public port forwarding, plaintext remote HTTP,
or expose the browser controller. Tailscale Taildrive's local WebDAV endpoint
is a different service, not a remote MasterDrive peer URL.

This code has no owner-approved device pairing, per-share identity grants,
credential revocation UI, native host-service installer, verified cross-range
resume, or fully qualified Windows reparse-point safety. Review bundled
`docs/architecture.md` and `docs/security/acceptance.md` before any further
testing. The source repository's full README, code, and issue roadmap are at
https://github.com/thebrazenbeard/masterdrive .

No redistribution license has been selected. This candidate is **not** a
signed release, and SHA256SUMS alone does not authenticate its publisher.
"""
VERSION_RE = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}\Z")
REVISION_RE = re.compile(r"[a-f0-9]{40}\Z")
ZIP_TIMESTAMP = (1980, 1, 1, 0, 0, 0)


def validate_inputs(version: str, revision: str, target_names: list[str]) -> None:
    if not VERSION_RE.fullmatch(version):
        raise ValueError("version must be 1-64 safe letters, digits, dots, underscores, or hyphens")
    if not REVISION_RE.fullmatch(revision):
        raise ValueError("revision must be an exact lowercase 40-hex Git commit")
    if not target_names or len(set(target_names)) != len(target_names):
        raise ValueError("specify one or more distinct build targets")
    unknown = set(target_names) - TARGETS.keys()
    if unknown:
        raise ValueError(f"unsupported build target(s): {', '.join(sorted(unknown))}")


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def file_digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1 << 20), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def source_files(repo_root: Path) -> dict[str, bytes]:
    files = {}
    for name in RUNTIME_FILES:
        candidate = repo_root / name
        if candidate.is_symlink() or not candidate.is_file():
            raise ValueError(f"missing or symlinked required release file: {name}")
        # Check the checked-in parent directories as well. Links could escape
        # the checkout even when the final file is a normal file.
        relative = Path(name)
        for parent in relative.parents:
            if parent != Path(".") and (repo_root / parent).is_symlink():
                raise ValueError(f"symlinked release directory: {parent}")
        files[name] = candidate.read_bytes()
    return files


def verify_checkout(repo_root: Path, revision: str) -> None:
    """Do not label modified or unrelated code with a trusted-looking SHA."""
    def git(*args: str) -> str:
        return subprocess.run(["git", *args], cwd=repo_root, check=True,
                              capture_output=True, text=True).stdout.strip()

    checkout_root = Path(git("rev-parse", "--show-toplevel")).resolve()
    if checkout_root != repo_root.resolve():
        raise ValueError("package from the Git checkout root, not a nested/parent repository")
    actual = git("rev-parse", "HEAD")
    if actual != revision:
        raise ValueError(f"declared revision {revision} does not match checked-out HEAD {actual}")
    if git("status", "--porcelain=v1", "--untracked-files=all"):
        raise ValueError("source checkout is dirty; package only a clean, exact commit")


def package_members(
    *, binary: bytes, exe_name: str, runtime_files: dict[str, bytes],
    target: str, version: str, revision: str, go_version: str,
) -> dict[str, tuple[bytes, int]]:
    """Return stable archive member bytes and their POSIX permissions."""
    os_name, architecture, _ = TARGETS[target]
    metadata = {
        "artifact_kind": "unqualified-unsigned-developer-candidate",
        "binary_sha256": digest(binary),
        "binary_size": len(binary),
        "build_flags": ["-trimpath", "-buildvcs=false", "-mod=readonly", "-ldflags=-buildid="],
        "cgo_enabled": False,
        "go_arch": architecture,
        "go_os": os_name,
        "go_version": go_version,
        "source_revision": revision,
        "version_label": version,
    }
    manifest = (json.dumps(metadata, sort_keys=True, indent=2) + "\n").encode("utf-8")
    members = {exe_name: (binary, 0o755)}
    members.update({name: (content, 0o644) for name, content in runtime_files.items()})
    members["CANDIDATE-QUICKSTART.md"] = (QUICKSTART.encode("utf-8"), 0o644)
    members["BUILD-METADATA.json"] = (manifest, 0o644)
    return members


def write_archive(path: Path, prefix: str, members: dict[str, tuple[bytes, int]]) -> None:
    """Create deterministic ZIP or tar.gz from fixed names, bytes and metadata."""
    # Exclusive creation avoids silently replacing an earlier build or user file.
    with path.open("xb") as dest:
        if path.suffix == ".zip":
            with zipfile.ZipFile(dest, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
                for name, (content, mode) in sorted(members.items()):
                    info = zipfile.ZipInfo(f"{prefix}/{name}", date_time=ZIP_TIMESTAMP)
                    info.create_system = 3
                    info.external_attr = (0o100000 | mode) << 16
                    info.compress_type = zipfile.ZIP_DEFLATED
                    archive.writestr(info, content, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
        elif path.name.endswith(".tar.gz"):
            with gzip.GzipFile(fileobj=dest, filename="", mode="wb", mtime=0, compresslevel=9) as gz:
                with tarfile.open(fileobj=gz, mode="w") as archive:
                    for name, (content, mode) in sorted(members.items()):
                        info = tarfile.TarInfo(f"{prefix}/{name}")
                        info.size = len(content)
                        info.mode = mode
                        info.uid = info.gid = 0
                        info.uname = info.gname = ""
                        info.mtime = 0
                        archive.addfile(info, io.BytesIO(content))
        else:
            raise ValueError(f"unsupported archive extension: {path.name}")


def compile_target(repo_root: Path, destination: Path, go_os: str, go_arch: str) -> None:
    environment = os.environ.copy()
    environment.update({
        "CGO_ENABLED": "0",
        "GOOS": go_os,
        "GOARCH": go_arch,
        "GOFLAGS": "",
        "GOTOOLCHAIN": "local",
    })
    subprocess.run(
        ["go", "build", "-trimpath", "-buildvcs=false", "-mod=readonly",
         "-ldflags=-buildid=", "-o", str(destination), "./cmd/masterdrive"],
        cwd=repo_root, env=environment, check=True,
    )


def build(repo_root: Path, output_dir: Path, version: str, revision: str,
          target_names: list[str]) -> list[Path]:
    validate_inputs(version, revision, target_names)
    verify_checkout(repo_root, revision)
    # Validate all inputs and existing output files before the first build.
    if not (repo_root / "go.mod").is_file():
        raise ValueError("repository root has no go.mod")
    runtime_files = source_files(repo_root)
    output_dir.mkdir(parents=True, exist_ok=True)
    targets = sorted(target_names)
    outputs = [output_dir / f"masterdrive-{version}-{target}.{TARGETS[target][2]}"
               for target in targets]
    for path in [*outputs, output_dir / "SHA256SUMS"]:
        if path.exists() or path.is_symlink():
            raise FileExistsError(f"refusing to overwrite existing package output: {path}")
    go_version = subprocess.run(
        ["go", "env", "GOVERSION"], cwd=repo_root, check=True,
        capture_output=True, text=True, env={**os.environ, "GOTOOLCHAIN": "local"},
    ).stdout.strip()
    if not go_version.startswith("go"):
        raise RuntimeError("go env GOVERSION returned an unexpected toolchain")
    with tempfile.TemporaryDirectory(prefix="masterdrive-package-") as temporary:
        for target, output_path in zip(targets, outputs):
            go_os, go_arch, _ = TARGETS[target]
            exe_name = "masterdrive.exe" if go_os == "windows" else "masterdrive"
            binary_path = Path(temporary) / exe_name
            compile_target(repo_root, binary_path, go_os, go_arch)
            if not binary_path.is_file():
                raise RuntimeError(f"go build returned no executable for {target}")
            members = package_members(binary=binary_path.read_bytes(), exe_name=exe_name,
                                      runtime_files=runtime_files, target=target,
                                      version=version, revision=revision, go_version=go_version)
            write_archive(output_path, f"masterdrive-{version}-{target}", members)
    # A checksum list is written only after every selected target succeeds.
    lines = [f"{file_digest(path)}  {path.name}" for path in outputs]
    with (output_dir / "SHA256SUMS").open("x", encoding="utf-8", newline="\n") as checksums:
        checksums.write("\n".join(lines) + "\n")
    return outputs


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo-root", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--out", type=Path, default=Path("dist"))
    parser.add_argument("--version", required=True)
    parser.add_argument("--revision", required=True)
    parser.add_argument("--targets", nargs="+", choices=sorted(TARGETS), default=sorted(TARGETS))
    arguments = parser.parse_args()
    outputs = build(arguments.repo_root.resolve(), arguments.out.resolve(), arguments.version,
                    arguments.revision, arguments.targets)
    print(f"Built {len(outputs)} unsigned, unqualified candidate archive(s):")
    for output in outputs:
        print(f"  {output} ({file_digest(output)})")
    print("Verify SHA256SUMS before inspecting or running any archive.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
