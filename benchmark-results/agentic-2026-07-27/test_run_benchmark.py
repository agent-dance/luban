from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock


SCRIPT = Path(__file__).with_name("run_benchmark.py")


def load_runner():
    spec = importlib.util.spec_from_file_location("representative_runner", SCRIPT)
    if spec is None or spec.loader is None:
        raise RuntimeError("cannot load benchmark runner")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def git(repo: Path, *args: str) -> bytes:
    return subprocess.run(
        ["git", *args], cwd=repo, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True,
    ).stdout


class CaptureWorkspacePatchTest(unittest.TestCase):
    def test_offline_protocol_variables_are_forwarded_to_agent_processes(self) -> None:
        runner = load_runner()
        self.assertTrue(set(runner.BENCHMARK_OFFLINE_ENV).issubset(runner.legacy.SAFE_CHILD_ENV_KEYS))

    def test_upstream_key_can_be_loaded_from_an_explicit_auth_entry(self) -> None:
        runner = load_runner()
        with tempfile.TemporaryDirectory() as directory:
            auth_file = Path(directory) / "auth.json"
            auth_file.write_text(
                json.dumps({"entries": {"meter": {"api_key": "secret-value"}}}),
                encoding="utf-8",
            )
            with mock.patch.dict(os.environ, {
                "LOCAL5_UPSTREAM_KEY": "",
                "LOCAL5_UPSTREAM_AUTH_FILE": str(auth_file),
                "LOCAL5_UPSTREAM_AUTH_ENTRY": "meter",
            }, clear=False):
                self.assertEqual(runner.load_upstream_key(), "secret-value")

    def test_capture_includes_untracked_without_mutating_index(self) -> None:
        runner = load_runner()
        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory)
            git(repo, "init", "--quiet")
            git(repo, "config", "user.name", "Benchmark Test")
            git(repo, "config", "user.email", "benchmark@example.invalid")
            tracked = repo / "tracked.txt"
            tracked.write_text("base\n", encoding="utf-8")
            git(repo, "add", "tracked.txt")
            git(repo, "commit", "--quiet", "-m", "base")

            tracked.write_text("changed\n", encoding="utf-8")
            (repo / "new.txt").write_text("new\n", encoding="utf-8")
            generated = repo / ".luban-build" / "CMakeCache.txt"
            generated.parent.mkdir()
            generated.write_text("generated\n", encoding="utf-8")
            generated_variant = repo / "build-make" / "output.o"
            generated_variant.parent.mkdir()
            generated_variant.write_text("variant-generated\n", encoding="utf-8")
            index_path = Path(git(repo, "rev-parse", "--git-path", "index").decode().strip())
            if not index_path.is_absolute():
                index_path = repo / index_path
            index_before = index_path.read_bytes()
            status_before = git(repo, "status", "--porcelain=v1", "-z")

            patch = runner.capture_workspace_patch(repo)

            self.assertIn("tracked.txt", patch)
            self.assertIn("new.txt", patch)
            self.assertIn("changed", patch)
            self.assertIn("new", patch)
            self.assertNotIn(".luban-build", patch)
            self.assertNotIn("generated", patch)
            self.assertNotIn("build-make", patch)
            self.assertEqual(index_path.read_bytes(), index_before)
            self.assertEqual(git(repo, "status", "--porcelain=v1", "-z"), status_before)


if __name__ == "__main__":
    unittest.main()
