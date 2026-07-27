#!/usr/bin/env python3
"""Run the archived SWE-bench-Live evaluator against this pilot's patches."""

from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parent
SOURCE = ROOT.parent / "agentic-2026-07-26" / "evaluate.py"
spec = importlib.util.spec_from_file_location("local5_legacy_evaluator", SOURCE)
if spec is None or spec.loader is None:
    raise RuntimeError("cannot load the local benchmark evaluator")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
module.ROOT = ROOT


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
