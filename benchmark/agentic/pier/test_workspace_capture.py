from __future__ import annotations

import hashlib
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from benchmark.agentic.pier import pinned_agent
from benchmark.agentic.pier.pinned_agent import PinnedCLIAgent


def _git(repo: Path, *args: str, input_bytes: bytes | None = None) -> bytes:
    return subprocess.run(
        ["git", *args],
        cwd=repo,
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=True,
    ).stdout


def _sha256(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


class WorkspaceCaptureTest(unittest.TestCase):
    def test_official_patch_is_committed_only_and_capture_preserves_real_git_state(
        self,
    ) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repo = root / "repo"
            logs = root / "logs"
            repo.mkdir()
            logs.mkdir()
            _git(repo, "init", "--quiet")
            _git(repo, "config", "user.name", "Agentic Capture Test")
            _git(repo, "config", "user.email", "capture@example.invalid")

            tracked = repo / "tracked.txt"
            tracked.write_text("base\n", encoding="utf-8")
            _git(repo, "add", "tracked.txt")
            _git(repo, "commit", "--quiet", "-m", "base")
            base_commit = _git(repo, "rev-parse", "HEAD").decode().strip()

            tracked.write_text("agent commit\n", encoding="utf-8")
            _git(repo, "add", "tracked.txt")
            _git(repo, "commit", "--quiet", "-m", "agent work")
            agent_head = _git(repo, "rev-parse", "HEAD").decode().strip()

            # These changes must be archived for audit, but DeepSWE's official
            # pre_artifacts hook intentionally excludes them from model.patch.
            tracked.write_text("uncommitted working tree\n", encoding="utf-8")
            (repo / "untracked.txt").write_text("audit only\n", encoding="utf-8")
            index_path = Path(
                _git(repo, "rev-parse", "--git-path", "index").decode().strip()
            )
            if not index_path.is_absolute():
                index_path = repo / index_path
            index_before = index_path.read_bytes()
            status_before = _git(repo, "status", "--porcelain=v1", "-z")
            expected_official = _git(
                repo, "diff", "--binary", base_commit, "HEAD", "--"
            )

            agent = object.__new__(PinnedCLIAgent)
            agent._base_commit = base_commit
            with mock.patch.object(pinned_agent, "_AGENT_LOGS", str(logs)):
                command = agent._capture_command()
            subprocess.run(["bash", "-c", command], cwd=repo, check=True)

            official = (logs / "committed-workspace.patch").read_bytes()
            audit = (logs / "full-workspace.patch").read_bytes()
            receipt = json.loads(
                (logs / "workspace-capture.json").read_text(encoding="utf-8")
            )

            self.assertEqual(official, expected_official)
            self.assertIn(b"agent commit", official)
            self.assertNotIn(b"uncommitted working tree", official)
            self.assertNotIn(b"untracked.txt", official)
            self.assertIn(b"uncommitted working tree", audit)
            self.assertIn(b"untracked.txt", audit)
            self.assertEqual(
                receipt,
                {
                    "schema_version": "agentic-bench/workspace-capture-v2",
                    "method": "official-git-diff+temporary-index-audit-v2",
                    "base_commit": base_commit,
                    "patch_sha256": _sha256(official),
                    "audit_patch_sha256": _sha256(audit),
                    "uncommitted_changes_present": True,
                    "includes_tracked": True,
                    "includes_untracked": True,
                    "includes_binary": True,
                },
            )

            self.assertEqual(_git(repo, "rev-parse", "HEAD").decode().strip(), agent_head)
            self.assertEqual(index_path.read_bytes(), index_before)
            self.assertEqual(_git(repo, "status", "--porcelain=v1", "-z"), status_before)


if __name__ == "__main__":
    unittest.main()
