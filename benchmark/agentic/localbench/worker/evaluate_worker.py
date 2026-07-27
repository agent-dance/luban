#!/usr/bin/env python3
"""Direct-Docker evaluator for the frozen local representative task catalog."""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import time
import uuid
from pathlib import Path


def load_catalog(path: Path) -> dict[str, dict]:
    values = json.loads(path.read_text(encoding="utf-8"))
    return {value["instance_id"]: value for value in values}


DOCKER_PREFIX: list[str] | None = None


def select_engine() -> tuple[list[str], str]:
    direct = subprocess.run(
        ["docker", "info"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        timeout=30,
    ) if shutil.which("docker") else None
    if direct is not None and direct.returncode == 0:
        return ["docker"], "local-docker"
    machine = os.environ.get("LUBAN_BENCHMARK_LIMA", "agentic-deepswe-amd64")
    if shutil.which("limactl"):
        prefix = ["limactl", "shell", machine, "--", "docker"]
        probe = subprocess.run(
            [*prefix, "info"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
            timeout=60,
        )
        if probe.returncode == 0:
            return prefix, "lima:" + machine + "/docker"
    raise RuntimeError("container_engine_unavailable")


def docker(args: list[str], **kwargs) -> subprocess.CompletedProcess:
    global DOCKER_PREFIX
    if DOCKER_PREFIX is None:
        DOCKER_PREFIX, _ = select_engine()
    return subprocess.run([*DOCKER_PREFIX, *args], **kwargs)


def apply_patch(container: str, patch: str) -> subprocess.CompletedProcess:
    command = (
        'root=/testbed; if [ ! -d "$root/.git" ]; then '
        'g=$(find "$root" -maxdepth 2 -mindepth 2 -type d -name .git -print -quit); '
        '[ -n "$g" ] && root=${g%/.git}; fi; '
        'cd "$root" && git apply --reject --whitespace=nowarn -'
    )
    return docker(
        ["exec", "-i", container, "bash", "-c", command], input=patch,
        text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=300,
    )


def exec_in_repo(container: str, command: str, timeout: int) -> subprocess.CompletedProcess:
    wrapper = (
        'root=/testbed; if [ ! -d "$root/.git" ]; then '
        'g=$(find "$root" -maxdepth 2 -mindepth 2 -type d -name .git -print -quit); '
        '[ -n "$g" ] && root=${g%/.git}; fi; cd "$root"; ' + command
    )
    return docker(
        ["exec", container, "bash", "-c", wrapper], text=True,
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout,
    )


def parse_log(source: str, log: str) -> dict[str, str]:
    namespace: dict = {}
    exec(source, namespace)
    parser = namespace.get("parser")
    if not callable(parser):
        raise RuntimeError("dataset_log_parser_missing")
    result = parser(log)
    if not isinstance(result, dict):
        raise RuntimeError("dataset_log_parser_invalid")
    return result


def evaluate(args, instance: dict) -> dict:
    result_root = Path(args.result_root).resolve()
    output_root = result_root / "raw" / "evaluation" / instance["instance_id"] / args.agent
    output_root.mkdir(parents=True, exist_ok=True)
    if args.agent == "gold":
        solution_patch = instance["patch"]
    else:
        solution_patch = (result_root / "raw" / "runs" / instance["instance_id"] / args.agent / "model.patch").read_text(encoding="utf-8")
    container = "lubanbench-" + re.sub(r"[^a-z0-9_.-]", "-", f"{args.agent}-{instance['instance_id']}".lower())[:72] + "-" + uuid.uuid4().hex[:8]
    image = instance["docker_image"]
    if not image.startswith("docker.io/"):
        image = "docker.io/" + image
    started = time.monotonic()
    inspect = docker(["image", "inspect", image], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
    if inspect.returncode != 0:
        pull = docker(["pull", "--platform", "linux/amd64", image], text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=1820)
        (output_root / "image-pull.log").write_text(pull.stdout, encoding="utf-8")
        if pull.returncode != 0:
            raise RuntimeError("image_pull_failed")
    create = docker(
        ["run", "-d", "--platform", "linux/amd64", "--network", "none", "--name", container, image, "sleep", "infinity"],
        text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=300,
    )
    if create.returncode != 0:
        (output_root / "container-create.log").write_text(create.stdout, encoding="utf-8")
        raise RuntimeError("container_start_failed")
    try:
        test_apply = apply_patch(container, instance["test_patch"])
        solution_apply = apply_patch(container, solution_patch)
        (output_root / "test-patch-apply.log").write_text(test_apply.stdout, encoding="utf-8")
        (output_root / "solution-patch-apply.log").write_text(solution_apply.stdout, encoding="utf-8")
        rebuild = exec_in_repo(container, " ; ".join(instance.get("rebuild_cmds") or []) or "true", args.timeout)
        tests = exec_in_repo(container, " ; ".join(instance.get("test_cmds") or []), args.timeout)
        printed = exec_in_repo(container, " ; ".join(instance.get("print_cmds") or []), min(args.timeout, 600))
        (output_root / "rebuild.log").write_text(rebuild.stdout, encoding="utf-8")
        (output_root / "test-command.log").write_text(tests.stdout, encoding="utf-8")
        (output_root / "post-patch.log").write_text(printed.stdout, encoding="utf-8")
        status = parse_log(instance["log_parser"], printed.stdout)
        (output_root / "status.json").write_text(json.dumps(status, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        passed = {name for name, value in status.items() if "pass" in value.lower()}
        failed = {name for name, value in status.items() if "fail" in value.lower()}
        f2p = set(instance["FAIL_TO_PASS"])
        p2p = set(instance["PASS_TO_PASS"])
        resolved = f2p.issubset(passed) and not (failed & f2p) and not (failed & p2p) and not (p2p - passed - failed)
        relative_root = Path("raw") / "evaluation" / instance["instance_id"] / args.agent
        return {
            "instance_id": instance["instance_id"], "language": instance["language"],
            "agent": args.agent, "resolved": resolved,
            "elapsed_seconds": round(time.monotonic() - started, 3),
            "FAIL_TO_PASS": {
                "expected": len(f2p), "passed": sorted(passed & f2p),
                "failed": sorted(failed & f2p), "missing": sorted(f2p - passed - failed),
            },
            "PASS_TO_PASS": {
                "expected": len(p2p), "passed_count": len(passed & p2p),
                "failed": sorted(failed & p2p), "missing_count": len(p2p - passed - failed),
            },
            "evidence_root": relative_root.as_posix(),
        }
    finally:
        docker(["rm", "-f", container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=300)


def main() -> int:
    global DOCKER_PREFIX
    parser = argparse.ArgumentParser()
    parser.add_argument("--preflight", action="store_true")
    parser.add_argument("--catalog")
    parser.add_argument("--result-root")
    parser.add_argument("--task")
    parser.add_argument("--agent", choices=["gold", "codex", "luban"])
    parser.add_argument("--timeout", type=int, default=2700)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    prefix, engine = select_engine()
    DOCKER_PREFIX = prefix
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    if args.preflight:
        output.write_text(json.dumps({"engine": engine}, indent=2) + "\n", encoding="utf-8")
        return 0
    if any(value is None for value in [args.catalog, args.result_root, args.task, args.agent]):
        raise RuntimeError("evaluator_configuration_incomplete")
    instance = load_catalog(Path(args.catalog))[args.task]
    result = evaluate(args, instance)
    output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
