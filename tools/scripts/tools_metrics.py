#!/usr/bin/env python3
"""
Deprecated compatibility wrapper.

The old tools_metrics.py was a hand-maintained registry and drifted badly.
Keep the entrypoint, but delegate to the source-driven parity audit instead.
"""

from __future__ import annotations

import runpy
import sys
from pathlib import Path


def main() -> int:
    target = Path(__file__).with_name("tool_parity_audit.py")
    sys.argv[0] = str(target)
    runpy.run_path(str(target), run_name="__main__")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
