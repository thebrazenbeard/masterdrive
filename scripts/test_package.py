"""Standard-library tests for deterministic candidate package assembly."""

from pathlib import Path
import tarfile
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import zipfile

import package as candidate


REVISION = "a" * 40


class PackageTests(unittest.TestCase):
    def fixture_repo(self, root: Path) -> None:
        (root / "go.mod").write_text("module github.com/thebrazenbeard/masterdrive\ngo 1.27.0\n")
        for name in candidate.RUNTIME_FILES:
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(("fixture: " + name + "\n").encode())

    def test_input_validation_rejects_path_and_revision_injection(self):
        candidate.validate_inputs("dev-1", REVISION, ["linux-amd64"])
        for version in ["", "../bad", "with space", "-option", "x" * 65]:
            with self.subTest(version=version), self.assertRaises(ValueError):
                candidate.validate_inputs(version, REVISION, ["linux-amd64"])
        for revision in ["", "ABC" * 13 + "A", "a" * 39, "a" * 39 + "/"]:
            with self.subTest(revision=revision), self.assertRaises(ValueError):
                candidate.validate_inputs("dev", revision, ["linux-amd64"])
        for targets in [[], ["bogus"], ["linux-amd64", "linux-amd64"]]:
            with self.subTest(targets=targets), self.assertRaises(ValueError):
                candidate.validate_inputs("dev", REVISION, targets)

    def test_archive_metadata_and_repeated_bytes_are_deterministic(self):
        runtime = {"web/index.html": b"<p>safe</p>"}
        for target, extension in [("windows-amd64", "zip"), ("linux-arm64", "tar.gz")]:
            with self.subTest(target=target), tempfile.TemporaryDirectory() as root:
                directory = Path(root)
                name = f"masterdrive-dev-{target}"
                exe = "masterdrive.exe" if target.startswith("windows") else "masterdrive"
                members = candidate.package_members(
                    binary=b"test-binary-bytes", exe_name=exe, runtime_files=runtime,
                    target=target, version="dev", revision=REVISION, go_version="go1.27.0",
                )
                first = directory / f"one.{extension}"
                second = directory / f"two.{extension}"
                candidate.write_archive(first, name, members)
                candidate.write_archive(second, name, members)
                self.assertEqual(first.read_bytes(), second.read_bytes())
                if target.startswith("windows"):
                    with zipfile.ZipFile(first) as archive:
                        self.assertEqual(archive.read(f"{name}/{exe}"), b"test-binary-bytes")
                        self.assertEqual(archive.read(f"{name}/web/index.html"), b"<p>safe</p>")
                        guide = archive.read(f"{name}/CANDIDATE-QUICKSTART.md").decode()
                        self.assertIn(".\\masterdrive.exe controller", guide)
                        self.assertIn(".\\masterdrive.exe add-host -data-dir $demoState", guide)
                        self.assertIn("controller -data-dir $demoState", guide)
                        self.assertNotIn("go run", guide)
                        for entry in archive.infolist():
                            self.assertEqual(entry.date_time, candidate.ZIP_TIMESTAMP)
                else:
                    with first.open("rb") as f:
                        self.assertEqual(f.read(8)[4:8], b"\x00\x00\x00\x00")
                    with tarfile.open(first, "r:gz") as archive:
                        entries = archive.getmembers()
                        self.assertTrue(all(m.mtime == 0 and m.uid == 0 and m.gid == 0 for m in entries))
                        self.assertEqual(archive.extractfile(f"{name}/{exe}").read(), b"test-binary-bytes")
                        guide = archive.extractfile(f"{name}/CANDIDATE-QUICKSTART.md").read().decode()
                        self.assertIn("./masterdrive controller", guide)
                        self.assertIn('add-host -data-dir "$DEMO_STATE"', guide)
                        self.assertIn('controller -data-dir "$DEMO_STATE"', guide)
                        self.assertIn("without recursive deletion", guide)
                        self.assertNotIn("go run", guide)
                if first.exists():
                    with self.assertRaises(FileExistsError):
                        candidate.write_archive(first, name, members)

    def test_pipeline_builds_both_formats_and_checksum_index(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "source"
            root.mkdir()
            self.fixture_repo(root)
            results = []
            def fake_compile(_repo, destination, go_os, architecture):
                destination.write_bytes(f"{go_os}/{architecture} binary".encode())
            with patch.object(candidate, "verify_checkout"), patch.object(
                candidate, "compile_target", side_effect=fake_compile), patch.object(
                candidate.subprocess, "run", return_value=SimpleNamespace(stdout="go1.27.0\n")
            ):
                for index in (1, 2):
                    output = Path(temporary) / f"dist{index}"
                    archives = candidate.build(root, output, "dev-7", REVISION,
                                               ["windows-amd64", "linux-amd64"])
                    self.assertEqual(len(archives), 2)
                    self.assertEqual(len((output / "SHA256SUMS").read_text().splitlines()), 2)
                    for archive in archives:
                        checksum = candidate.file_digest(archive)
                        self.assertIn(f"{checksum}  {archive.name}",
                                      (output / "SHA256SUMS").read_text())
                    results.append({archive.name: archive.read_bytes() for archive in archives})
                self.assertEqual(results[0], results[1])
                with self.assertRaises(FileExistsError):
                    candidate.build(root, Path(temporary) / "dist1", "dev-7", REVISION,
                                    ["linux-amd64"])

    def test_checkout_validation_rejects_wrong_revision_or_dirty_state(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            command_outputs = iter([str(root), "b" * 40, ""])
            with patch.object(candidate.subprocess, "run", side_effect=lambda *a, **kw: SimpleNamespace(stdout=next(command_outputs))):
                with self.assertRaisesRegex(ValueError, "does not match"):
                    candidate.verify_checkout(root, REVISION)
            command_outputs = iter([str(root), REVISION, " M web/app.js"])
            with patch.object(candidate.subprocess, "run", side_effect=lambda *a, **kw: SimpleNamespace(stdout=next(command_outputs))):
                with self.assertRaisesRegex(ValueError, "dirty"):
                    candidate.verify_checkout(root, REVISION)

    def test_required_runtime_asset_is_missing_or_symlinked(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.fixture_repo(root)
            (root / "web" / "app.js").unlink()
            with self.assertRaisesRegex(ValueError, "missing or symlinked"):
                candidate.source_files(root)
            (root / "web" / "app.js").symlink_to(root / "go.mod")
            with self.assertRaisesRegex(ValueError, "missing or symlinked"):
                candidate.source_files(root)


if __name__ == "__main__":
    unittest.main()
