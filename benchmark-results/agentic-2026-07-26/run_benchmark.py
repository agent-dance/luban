#!/usr/bin/env python3
"""Reproducible Codex-vs-Luban runner for five SWE-bench-Live tasks."""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path

MODEL = "gpt-5.6-sol"
EFFORT = "xhigh"
DATASET_REVISION = "608f7ae9ab8ea1f9f0d030fe04562cf6bd1a0c8b"
PRICE = {
    "input_per_million_usd": 5.0,
    "cached_input_per_million_usd": 0.5,
    "output_per_million_usd": 30.0,
}
SAFE_CHILD_ENV_KEYS = {
    "HOME",
    "PATH",
    "SHELL",
    "TMPDIR",
    "LANG",
    "LC_ALL",
    "LC_CTYPE",
    "SSL_CERT_FILE",
    "SSL_CERT_DIR",
    "HTTP_PROXY",
    "HTTPS_PROXY",
    "NO_PROXY",
    "CODEX_LB_API_KEY",
    "OPENAI_API_KEY",
}
SELECTED = {
    "include-what-you-use__include-what-you-use-1991": "cpp",
    "danielmiessler__Fabric-2098": "go",
    "kubernetes__kube-state-metrics-2926": "go",
    "skim-rs__skim-1044": "rust",
    "openai__openai-agents-js-375": "ts",
}

ROOT = Path(__file__).resolve().parent
DATA_DIR = Path("/private/tmp/luban-agent-benchmark-data")
WORK_ROOT = Path("/private/tmp/luban-agent-benchmark-work")
RUNS_DIR = ROOT / "raw" / "runs"
METADATA_DIR = ROOT / "raw" / "metadata"


def run_quiet(args: list[str], cwd: Path | None = None, timeout: int = 600) -> str:
    result = subprocess.run(
        args,
        cwd=cwd,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=timeout,
        check=True,
    )
    return result.stdout


def load_instances() -> dict[str, dict]:
    import pyarrow.parquet as pq

    instances: dict[str, dict] = {}
    for instance_id, language in SELECTED.items():
        rows = pq.read_table(DATA_DIR / f"{language}.parquet").to_pylist()
        instance = next(row for row in rows if row["instance_id"] == instance_id)
        instance["language"] = language
        instances[instance_id] = instance
    return instances


def public_instance(instance: dict) -> dict:
    result = dict(instance)
    result.pop("patch", None)
    result.pop("test_patch", None)
    result.pop("hints_text", None)
    result.pop("all_hints_text", None)
    return result


def write_metadata(instances: dict[str, dict], include_gold: bool = False) -> None:
    METADATA_DIR.mkdir(parents=True, exist_ok=True)
    public = [public_instance(instances[key]) for key in SELECTED]
    (METADATA_DIR / "selected_instances.json").write_text(
        json.dumps(public, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    provenance = {
        "dataset": "SWE-bench-Live/MultiLang",
        "dataset_revision": DATASET_REVISION,
        "parquet_sha256": {
            "cpp": "5afc7db10f28232cc9c13de316ecec146f2da4c76de3bb460b934f5e271b0ec0",
            "go": "76d2b5dff0f3fac8303d30fa85495539e487d25974ad7c21cd21a545cb4756e2",
            "java": "75c8d2f925a83c1a5b7a7b267ccdc9db41f1dc7dd80fe641073aac577d13ecc4",
            "rust": "ea90be54a621c0c0280b77d5e2dee9650bc1d4ae087f9b9b06af821bcd8662d7",
            "ts": "2095a180276ed76a6f3063f3ca8d2d8d5d0fdb721e683023ebe240aa70fdfed9",
        },
        "model": MODEL,
        "reasoning_effort": EFFORT,
        "pricing_assumption": PRICE,
        "timeout_seconds": 1800,
    }
    (METADATA_DIR / "experiment.json").write_text(
        json.dumps(provenance, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    if include_gold:
        complete = [instances[key] for key in SELECTED]
        (METADATA_DIR / "selected_instances_with_gold.json").write_text(
            json.dumps(complete, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )


def canonical_repo(instance: dict) -> Path:
    return WORK_ROOT / "canonical" / instance["instance_id"]


def setup_canonical(instance: dict) -> Path:
    target = canonical_repo(instance)
    if (target / ".git").is_dir():
        return target
    target.parent.mkdir(parents=True, exist_ok=True)
    target.mkdir(parents=True, exist_ok=True)
    run_quiet(["git", "init", "-q"], cwd=target)
    run_quiet(["git", "remote", "add", "origin", f"https://github.com/{instance['repo']}.git"], cwd=target)
    run_quiet(["git", "fetch", "--depth", "1", "origin", instance["base_commit"]], cwd=target, timeout=1800)
    run_quiet(["git", "checkout", "-q", "--detach", "FETCH_HEAD"], cwd=target)
    run_quiet(["git", "remote", "remove", "origin"], cwd=target)
    run_quiet(["git", "config", "user.email", "benchmark@example.invalid"], cwd=target)
    run_quiet(["git", "config", "user.name", "Benchmark Runner"], cwd=target)
    return target


def setup_run_repo(instance: dict, agent: str) -> Path:
    source = setup_canonical(instance)
    target = WORK_ROOT / "repos" / instance["instance_id"] / agent
    if target.exists():
        shutil.rmtree(target)
    target.parent.mkdir(parents=True, exist_ok=True)
    run_quiet(["git", "clone", "-q", "--no-hardlinks", str(source), str(target)], timeout=1800)
    run_quiet(["git", "checkout", "-q", "--detach", instance["base_commit"]], cwd=target)
    return target


def prompt_for(instance: dict) -> str:
    return (
        "Resolve the following software engineering issue in this repository. "
        "Work autonomously: inspect the code, edit the implementation, and run relevant tests when practical. "
        "Do not use the network, search the web, inspect remote repositories, or inspect git history beyond HEAD. "
        "Do not access files outside this repository. Do not commit the changes. "
        "Finish only when you have implemented the best complete fix you can and briefly summarize it.\n\n"
        "ISSUE:\n"
        + instance["problem_statement"]
    )


def codex_command(repo: Path, prompt: str) -> list[str]:
    return [
        "codex",
        "-a",
        "never",
        "exec",
        "--json",
        "--ephemeral",
        "--ignore-user-config",
        "--ignore-rules",
        "-C",
        str(repo),
        "-m",
        MODEL,
        "-c",
        'model_reasoning_effort="xhigh"',
        "-c",
        'model_provider="custom"',
        "-c",
        'model_providers.custom.name="OpenAI"',
        "-c",
        'model_providers.custom.base_url="https://sub.blurooo.com"',
        "-c",
        "model_providers.custom.requires_openai_auth=true",
        "-c",
        'model_providers.custom.wire_api="responses"',
        "-c",
        "disable_response_storage=true",
        "-s",
        "workspace-write",
        prompt,
    ]


def luban_command(repo: Path, prompt: str, debug_path: Path) -> list[str]:
    return [
        "luban",
        "-p",
        "--model",
        MODEL,
        "--provider",
        "custom-sub.blurooo.com",
        "--output-format",
        "stream-json",
        "--allow-all",
        "--disallowed-tools",
        "WebSearch,WebFetch,Agent,TaskCreate,TeamCreate",
        "--max-turns",
        "100",
        "--debug-file",
        str(debug_path),
        prompt,
    ]


def terminate_process_group(process: subprocess.Popen) -> None:
    try:
        os.killpg(process.pid, signal.SIGTERM)
        process.wait(timeout=10)
    except (ProcessLookupError, subprocess.TimeoutExpired):
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def parse_events(path: Path) -> list[dict]:
    events = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            events.append(value)
    return events


def usage_and_tools(agent: str, events: list[dict]) -> tuple[dict, dict[str, int], str]:
    tools: dict[str, int] = {}
    final_text = ""
    if agent == "codex":
        usage = {"input_tokens": 0, "cached_input_tokens": 0, "output_tokens": 0, "reasoning_output_tokens": 0}
        seen: set[str] = set()
        for event in events:
            if event.get("type") == "turn.completed":
                usage.update(event.get("usage") or {})
            item = event.get("item") or {}
            if event.get("type") == "item.completed" and item.get("type") == "agent_message":
                final_text = item.get("text", "")
            item_type = item.get("type")
            item_id = item.get("id")
            if event.get("type") == "item.completed" and item_type not in {
                None,
                "agent_message",
                "reasoning",
                "error",
                "todo_list",
            } and item_id not in seen:
                seen.add(item_id)
                tools[item_type] = tools.get(item_type, 0) + 1
        usage["cache_read_input_tokens"] = usage.get("cached_input_tokens", 0)
        usage["cache_creation_input_tokens"] = 0
    else:
        usage = {
            "input_tokens": 0,
            "cached_input_tokens": 0,
            "cache_read_input_tokens": 0,
            "cache_creation_input_tokens": 0,
            "output_tokens": 0,
            "reasoning_output_tokens": None,
        }
        seen = set()
        texts = []
        for event in events:
            if event.get("type") == "usage" and event.get("scope") == "last_request":
                usage["input_tokens"] += int(event.get("input_tokens", 0))
                usage["output_tokens"] += int(event.get("output_tokens", 0))
                usage["cache_read_input_tokens"] += int(event.get("cache_read_input_tokens", 0))
                usage["cache_creation_input_tokens"] += int(event.get("cache_creation_input_tokens", 0))
            if event.get("type") == "tool_use":
                tool_id = event.get("tool_use_id")
                if tool_id not in seen:
                    seen.add(tool_id)
                    name = event.get("name", "unknown")
                    tools[name] = tools.get(name, 0) + 1
            if event.get("type") == "text":
                texts.append(event.get("content", ""))
        usage["cached_input_tokens"] = usage["cache_read_input_tokens"]
        final_text = texts[-1] if texts else ""
    return usage, tools, final_text


def estimate_cost(usage: dict) -> float:
    total_input = int(usage.get("input_tokens") or 0)
    cached = int(usage.get("cache_read_input_tokens") or usage.get("cached_input_tokens") or 0)
    uncached = max(total_input - cached, 0)
    output = int(usage.get("output_tokens") or 0)
    cache_creation = int(usage.get("cache_creation_input_tokens") or 0)
    return (
        uncached * PRICE["input_per_million_usd"]
        + cached * PRICE["cached_input_per_million_usd"]
        + cache_creation * 6.25
        + output * PRICE["output_per_million_usd"]
    ) / 1_000_000


def patch_stats(patch: str) -> dict:
    files = re.findall(r"^diff --git a/(.+?) b/", patch, flags=re.MULTILINE)
    additions = len(re.findall(r"^\+(?!\+\+\+)", patch, flags=re.MULTILINE))
    deletions = len(re.findall(r"^-(?!---)", patch, flags=re.MULTILINE))
    return {"files_changed": len(files), "files": files, "additions": additions, "deletions": deletions}


def run_agent(instance: dict, agent: str, timeout: int) -> dict:
    repo = setup_run_repo(instance, agent)
    run_dir = RUNS_DIR / instance["instance_id"] / agent
    run_dir.mkdir(parents=True, exist_ok=True)
    events_path = run_dir / "events.jsonl"
    stderr_path = run_dir / "stderr.log"
    debug_path = run_dir / "provider-debug.log"
    prompt = prompt_for(instance)
    command = codex_command(repo, prompt) if agent == "codex" else luban_command(repo, prompt, debug_path)
    # Do not expose unrelated host credentials to Agent shell tools. The
    # recorded 2026-07-26 runs predate this hardening and were redacted before
    # delivery; keep model authentication keys narrowly scoped and short-lived.
    env = {key: value for key, value in os.environ.items() if key in SAFE_CHILD_ENV_KEYS}
    env["NO_COLOR"] = "1"
    env["TERM"] = "dumb"
    if agent == "luban":
        env["OPENAI_REASONING_EFFORT"] = EFFORT
    started_wall = time.time()
    started_mono = time.monotonic()
    process = subprocess.Popen(
        command,
        cwd=repo,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        start_new_session=True,
    )
    timed_out = False
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        terminate_process_group(process)
        stdout, stderr = process.communicate()
    elapsed = time.monotonic() - started_mono
    events_path.write_text(stdout, encoding="utf-8")
    stderr_path.write_text(stderr, encoding="utf-8")
    patch = run_quiet(["git", "--no-pager", "diff", "HEAD", "--text"], cwd=repo)
    (run_dir / "model.patch").write_text(patch, encoding="utf-8")
    events = parse_events(events_path)
    usage, tools, final_text = usage_and_tools(agent, events)
    summary = {
        "instance_id": instance["instance_id"],
        "language": instance["language"],
        "agent": agent,
        "model": MODEL,
        "reasoning_effort": EFFORT,
        "started_at_unix": started_wall,
        "elapsed_seconds": round(elapsed, 3),
        "timeout_seconds": timeout,
        "timed_out": timed_out,
        "exit_code": process.returncode,
        "usage": usage,
        "estimated_cost_usd": round(estimate_cost(usage), 6),
        "tool_calls": sum(tools.values()),
        "tool_calls_by_type": tools,
        "patch": patch_stats(patch),
        "final_text": final_text,
        "command_public": {
            "executable": command[0],
            "model": MODEL,
            "reasoning_effort": EFFORT,
            "provider": "same custom OpenAI-compatible endpoint",
        },
    }
    (run_dir / "summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return summary


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--prepare", action="store_true")
    parser.add_argument("--agent", choices=["codex", "luban"])
    parser.add_argument("--instance")
    parser.add_argument("--timeout", type=int, default=1800)
    parser.add_argument("--include-gold", action="store_true")
    args = parser.parse_args()
    instances = load_instances()
    write_metadata(instances, include_gold=args.include_gold)
    if args.prepare:
        for key in SELECTED:
            print(f"Preparing {key}", flush=True)
            setup_canonical(instances[key])
        return 0
    if not args.agent or not args.instance:
        parser.error("--agent and --instance are required unless --prepare is used")
    if args.instance not in instances:
        parser.error(f"unknown instance: {args.instance}")
    print(f"START {args.agent} {args.instance}", flush=True)
    summary = run_agent(instances[args.instance], args.agent, args.timeout)
    print(json.dumps(summary, ensure_ascii=False), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
