#!/usr/bin/env python3
"""Evaluate gold or agent patches with SWE-bench-Live's official images and rubric."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import time
import uuid
from pathlib import Path

import pyarrow.parquet as pq


ROOT = Path(__file__).resolve().parent
DATA_DIR = Path("/private/tmp/luban-agent-benchmark-data")
SELECTED = {
    "include-what-you-use__include-what-you-use-1991": "cpp",
    "danielmiessler__Fabric-2098": "go",
    "kubernetes__kube-state-metrics-2926": "go",
    "skim-rs__skim-1044": "rust",
    "openai__openai-agents-js-375": "ts",
}


def load_instance(instance_id: str) -> dict:
    language = SELECTED[instance_id]
    rows = pq.read_table(DATA_DIR / f"{language}.parquet").to_pylist()
    instance = next(row for row in rows if row["instance_id"] == instance_id)
    instance["language"] = language
    return instance


def lima(args: list[str], **kwargs) -> subprocess.CompletedProcess:
    return subprocess.run(["limactl", "shell", "ultrawork-amd64", "--", *args], **kwargs)


def apply_patch(container: str, patch: str) -> dict:
    command = (
        'source ~/.bashrc 2>/dev/null || true; '
        'root=/testbed; if [ ! -d "$root/.git" ]; then '
        'g=$(find "$root" -maxdepth 2 -mindepth 2 -type d -name .git -print -quit); '
        '[ -n "$g" ] && root=${g%/.git}; fi; '
        'cd "$root" && git apply --reject --whitespace=nowarn -'
    )
    result = lima(
        ["nerdctl", "exec", "-i", container, "bash", "-c", command],
        input=patch,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=300,
    )
    # RepoLaunch deliberately continues after partial application and leaves
    # *.rej evidence. Match that behavior, then let the tests decide.
    return {"exit_code": result.returncode, "output": result.stdout}


def exec_in_repo(container: str, command: str, timeout: int) -> subprocess.CompletedProcess:
    wrapper = (
        'source ~/.bashrc 2>/dev/null || true; '
        'root=/testbed; if [ ! -d "$root/.git" ]; then '
        'g=$(find "$root" -maxdepth 2 -mindepth 2 -type d -name .git -print -quit); '
        '[ -n "$g" ] && root=${g%/.git}; fi; '
        'cd "$root"; ' + command
    )
    return lima(
        ["nerdctl", "exec", container, "bash", "-c", wrapper],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=timeout,
    )


def parse_log(parser_source: str, log: str) -> dict[str, str]:
    namespace: dict = {}
    exec(parser_source, namespace)
    parser = namespace.get("parser")
    if not callable(parser):
        raise RuntimeError("dataset log_parser did not define parser(log)")
    result = parser(log)
    if not isinstance(result, dict):
        raise RuntimeError("dataset log_parser returned a non-dictionary")
    return result


def patch_path(instance_id: str, agent: str) -> Path:
    return ROOT / "raw" / "runs" / instance_id / agent / "model.patch"


def exclude_patch_paths(patch: str, excluded_paths: set[str]) -> str:
    if not excluded_paths:
        return patch
    sections = re.split(r"(?=^diff --git )", patch, flags=re.MULTILINE)
    kept = []
    for section in sections:
        match = re.match(r"diff --git a/(\S+) b/(\S+)", section)
        if match and (match.group(1) in excluded_paths or match.group(2) in excluded_paths):
            continue
        kept.append(section)
    return "".join(kept)


def evaluate(
    instance: dict,
    agent: str,
    timeout: int,
    excluded_paths: set[str] | None = None,
) -> dict:
    excluded_paths = excluded_paths or set()
    output_agent = agent if not excluded_paths else f"{agent}-diagnostic-production-only"
    if agent == "gold":
        solution_patch = instance["patch"]
    else:
        solution_patch = patch_path(instance["instance_id"], agent).read_text(encoding="utf-8")
    solution_patch = exclude_patch_paths(solution_patch, excluded_paths)
    output_dir = ROOT / "raw" / "evaluation" / instance["instance_id"] / output_agent
    output_dir.mkdir(parents=True, exist_ok=True)
    container = "lubanbench-" + re.sub(r"[^a-z0-9_.-]", "-", f"{output_agent}-{instance['instance_id']}".lower())[:80]
    container += "-" + uuid.uuid4().hex[:8]
    image = "docker.io/" + instance["docker_image"]
    started = time.monotonic()
    cached = lima(
        ["nerdctl", "image", "inspect", image],
        text=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        timeout=60,
    )
    pull = cached if cached.returncode == 0 else lima(
        [
            "timeout",
            "--signal=TERM",
            "--kill-after=10s",
            "1800s",
            "nerdctl",
            "pull",
            "--quiet",
            image,
        ],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=1820,
    )
    if cached.returncode == 0:
        pull.stdout = "cached\n"
    (output_dir / "image-pull.log").write_text(pull.stdout, encoding="utf-8")
    if pull.returncode != 0:
        raise RuntimeError(f"image pull failed: {pull.stdout}")
    create = lima(
        ["nerdctl", "run", "-d", "--name", container, image, "sleep", "infinity"],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=300,
    )
    if create.returncode != 0:
        raise RuntimeError(f"container start failed: {create.stdout}")
    try:
        test_patch_apply = apply_patch(container, instance["test_patch"])
        solution_patch_apply = apply_patch(container, solution_patch)
        (output_dir / "test-patch-apply.log").write_text(test_patch_apply["output"], encoding="utf-8")
        (output_dir / "solution-patch-apply.log").write_text(solution_patch_apply["output"], encoding="utf-8")
        rebuild_cmd = " ; ".join(instance.get("rebuild_cmds") or [])
        test_cmd = " ; ".join(instance.get("test_cmds") or [])
        print_cmd = " ; ".join(instance.get("print_cmds") or [])
        rebuild = exec_in_repo(container, rebuild_cmd or "true", timeout)
        (output_dir / "rebuild.log").write_text(rebuild.stdout, encoding="utf-8")
        test = exec_in_repo(container, test_cmd, timeout)
        (output_dir / "test-command.log").write_text(test.stdout, encoding="utf-8")
        printed = exec_in_repo(container, print_cmd, 600)
        (output_dir / "post-patch.log").write_text(printed.stdout, encoding="utf-8")
        status = parse_log(instance["log_parser"], printed.stdout)
        (output_dir / "status.json").write_text(
            json.dumps(status, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        passed = {name for name, value in status.items() if "pass" in value.lower()}
        failed = {name for name, value in status.items() if "fail" in value.lower()}
        f2p = set(instance["FAIL_TO_PASS"])
        p2p = set(instance["PASS_TO_PASS"])
        report = {
            "instance_id": instance["instance_id"],
            "language": instance["language"],
            "agent": output_agent,
            "diagnostic_excluded_paths": sorted(excluded_paths),
            "resolved": f2p.issubset(passed) and not (failed & f2p) and not (failed & p2p),
            "elapsed_seconds": round(time.monotonic() - started, 3),
            "image": instance["docker_image"],
            "image_pull_exit_code": pull.returncode,
            "test_patch_apply_exit_code": test_patch_apply["exit_code"],
            "solution_patch_apply_exit_code": solution_patch_apply["exit_code"],
            "rebuild_exit_code": rebuild.returncode,
            "test_exit_code": test.returncode,
            "print_exit_code": printed.returncode,
            "parsed_tests": len(status),
            "FAIL_TO_PASS": {
                "expected": len(f2p),
                "passed": sorted(passed & f2p),
                "failed": sorted(failed & f2p),
                "missing": sorted(f2p - passed - failed),
            },
            "PASS_TO_PASS": {
                "expected": len(p2p),
                "passed_count": len(passed & p2p),
                "failed": sorted(failed & p2p),
                "missing_count": len(p2p - passed - failed),
            },
        }
        (output_dir / "report.json").write_text(
            json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )
        return report
    finally:
        lima(
            ["nerdctl", "rm", "-f", container],
            text=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=300,
        )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--instance", required=True, choices=SELECTED.keys())
    parser.add_argument("--agent", required=True, choices=["gold", "codex", "luban"])
    parser.add_argument("--timeout", type=int, default=2700)
    parser.add_argument("--exclude-path", action="append", default=[])
    args = parser.parse_args()
    instance = load_instance(args.instance)
    print(f"EVAL START {args.agent} {args.instance}", flush=True)
    report = evaluate(instance, args.agent, args.timeout, set(args.exclude_path))
    print(json.dumps(report, ensure_ascii=False), flush=True)
    return 0 if report["resolved"] else 1


if __name__ == "__main__":
    sys.exit(main())
