#!/usr/bin/env python3
"""Run the archived SWE-bench-Live evaluator against this pilot's patches."""

from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent
REPOSITORY_ROOT = ROOT.parent.parent
REPRESENTATIVE20_CATALOG = REPOSITORY_ROOT / "benchmark" / "agentic" / "localbench" / "catalog" / "representative20.json"
SOURCE = ROOT.parent / "agentic-2026-07-26" / "evaluate.py"
spec = importlib.util.spec_from_file_location("local5_legacy_evaluator", SOURCE)
if spec is None or spec.loader is None:
    raise RuntimeError("cannot load the local benchmark evaluator")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
module.ROOT = ROOT
representative20 = json.loads(REPRESENTATIVE20_CATALOG.read_text(encoding="utf-8"))
module.SELECTED = {row["instance_id"]: row["language"] for row in representative20}

candidate_root_value = os.environ.get("LOCAL5_CANDIDATE_ROOT", "").strip()
if candidate_root_value:
    candidate_root = Path(candidate_root_value).resolve()
    original_load_instance = module.load_instance

    def load_candidate_instance(instance_id: str) -> dict:
        instance = original_load_instance(instance_id)
        # Metadata remains anchored to the archived pilot, while patches and
        # evaluation artifacts stay isolated under the candidate directory.
        module.ROOT = candidate_root
        return instance

    def candidate_patch_path(instance_id: str, agent: str) -> Path:
        return candidate_root / "runs" / instance_id / agent / "model.patch"

    module.load_instance = load_candidate_instance
    module.patch_path = candidate_patch_path


def local_lima(args: list[str], **kwargs) -> subprocess.CompletedProcess:
    """Run locally, using Docker on the roomy VM or nerdctl on the legacy VM."""
    machine = os.environ.get("LOCAL5_EVAL_LIMA", "agentic-deepswe-amd64")
    translated = args
    if machine == "agentic-deepswe-amd64":
        translated = ["docker" if value == "nerdctl" else value for value in args]
    return subprocess.run(
        ["limactl", "shell", machine, "--", *translated],
        **kwargs,
    )


module.lima = local_lima

if __name__ == "__main__":
    sys.exit(module.main())
