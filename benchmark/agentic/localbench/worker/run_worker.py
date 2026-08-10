#!/usr/bin/env python3
"""Content-minimized local runner used by the Go benchmark orchestrator."""

from __future__ import annotations

import argparse
import fcntl
import hashlib
import http.client
import json
import os
import re
import shutil
import signal
import ssl
import subprocess
import threading
import time
import tomllib
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


MODEL = "gpt-5.6-sol"
EFFORT = "xhigh"
UPSTREAM_IDLE_TIMEOUT_SECONDS = max(
    1.0,
    float(os.environ.get("AGENTIC_BENCHMARK_UPSTREAM_IDLE_TIMEOUT_SECONDS", "90")),
)
PRICE = {"input": 5.0, "cached": 0.5, "cache_write": 6.25, "output": 30.0}
SAFE_ENV = {
    "PATH", "SHELL", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE",
    "SSL_CERT_FILE", "SSL_CERT_DIR", "NO_PROXY",
}


def load_catalog(path: Path) -> dict[str, dict]:
    values = json.loads(path.read_text(encoding="utf-8"))
    return {value["instance_id"]: value for value in values}


def codex_home() -> Path:
    configured = os.environ.get("CODEX_HOME")
    return Path(configured).expanduser() if configured else Path.home() / ".codex"


def provider_credentials() -> tuple[str, str]:
    root = codex_home()
    with (root / "config.toml").open("rb") as handle:
        config = tomllib.load(handle)
    provider_name = str(config.get("model_provider") or "openai")
    provider = (config.get("model_providers") or {}).get(provider_name) or {}
    upstream = str(provider.get("base_url") or "https://api.openai.com/v1").rstrip("/")
    parsed = urlsplit(upstream)
    if parsed.scheme != "https" or not parsed.hostname:
        raise RuntimeError("codex_provider_origin_invalid")
    auth = json.loads((root / "auth.json").read_text(encoding="utf-8"))
    key = str(auth.get("OPENAI_API_KEY") or "")
    if not key:
        raise RuntimeError("codex_api_key_missing")
    origin = f"{parsed.scheme}://{parsed.hostname}"
    if parsed.port:
        origin += f":{parsed.port}"
    return upstream, key


class RequestMeter:
    def __init__(self, upstream: str, key: str, output: Path):
        self.upstream = urlsplit(upstream)
        self.key = key
        self.output = output
        self.records: list[dict] = []
        self.lock = threading.Lock()
        self.connections: set[http.client.HTTPSConnection] = set()
        self.sequence = 0
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), self._handler())
        self.server.daemon_threads = True
        self.server.block_on_close = False
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.output.parent.mkdir(parents=True, exist_ok=True)
        self.output.write_text("", encoding="utf-8")

    def _record(self, record: dict) -> None:
        encoded = json.dumps(record, sort_keys=True, separators=(",", ":")) + "\n"
        with self.lock:
            self.records.append(record)
            with self.output.open("a", encoding="utf-8") as stream:
                stream.write(encoded)

    def _handler(self):
        meter = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *_args):
                return

            def do_GET(self):
                self._forward()

            def do_POST(self):
                self._forward()

            def _forward(self):
                started = time.monotonic()
                with meter.lock:
                    sequence = meter.sequence
                    meter.sequence += 1
                length = int(self.headers.get("Content-Length", "0") or "0")
                body = self.rfile.read(length) if length else None
                path = meter.upstream.path.rstrip("/") + self.path
                headers = {
                    name: value for name, value in self.headers.items()
                    if name.lower() not in {
                        "authorization", "host", "connection", "content-length",
                        "transfer-encoding", "accept-encoding",
                    }
                }
                headers["Authorization"] = "Bearer " + meter.key
                headers["Accept-Encoding"] = "identity"
                if body is not None:
                    headers["Content-Length"] = str(len(body))
                connection = None
                status = 502
                response_bytes = 0
                request_id_hash = ""
                error_class = ""
                try:
                    connection = http.client.HTTPSConnection(
                        meter.upstream.hostname, meter.upstream.port or 443,
                        timeout=UPSTREAM_IDLE_TIMEOUT_SECONDS, context=ssl.create_default_context(),
                    )
                    with meter.lock:
                        meter.connections.add(connection)
                    connection.request(self.command, path, body=body, headers=headers)
                    response = connection.getresponse()
                    status = response.status
                    request_id = response.getheader("x-request-id", "")
                    if request_id:
                        request_id_hash = hashlib.sha256(request_id.encode()).hexdigest()
                    self.send_response(status)
                    for name, value in response.getheaders():
                        if name.lower() in {"connection", "content-length", "transfer-encoding", "keep-alive"}:
                            continue
                        self.send_header(name, value)
                    self.send_header("Connection", "close")
                    self.end_headers()
                    while True:
                        # Responses is an SSE stream. read(amt) can wait for a
                        # full buffer and strand a small completed response.
                        chunk = response.read1(65536)
                        if not chunk:
                            break
                        response_bytes += len(chunk)
                        self.wfile.write(chunk)
                        self.wfile.flush()
                    self.close_connection = True
                except Exception as error:
                    error_class = type(error).__name__
                    try:
                        payload = b'{"error":{"message":"benchmark_proxy_failure"}}'
                        self.send_response(502)
                        self.send_header("Content-Type", "application/json")
                        self.send_header("Content-Length", str(len(payload)))
                        self.send_header("Connection", "close")
                        self.end_headers()
                        self.wfile.write(payload)
                    except Exception:
                        pass
                finally:
                    if connection is not None:
                        connection.close()
                    record = {
                        "sequence": sequence,
                        "method": self.command,
                        "endpoint": "responses" if path.rstrip("/").endswith("/responses") else "other",
                        "status": status,
                        "elapsed_seconds": round(time.monotonic() - started, 6),
                        "request_bytes": len(body or b""),
                        "response_bytes": response_bytes,
                        "request_id_sha256": request_id_hash,
                        "error_class": error_class,
                    }
                    meter._record(record)
                    if connection is not None:
                        with meter.lock:
                            meter.connections.discard(connection)

        return Handler

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.server.server_port}"

    def start(self) -> None:
        self.thread.start()

    def stop(self) -> list[dict]:
        with self.lock:
            connections = list(self.connections)
        for connection in connections:
            connection.close()
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            with self.lock:
                if not self.connections:
                    break
            time.sleep(0.01)
        self.key = ""
        with self.lock:
            records = sorted(self.records, key=lambda item: item["sequence"])
        self.output.write_text(
            "".join(json.dumps(item, sort_keys=True, separators=(",", ":")) + "\n" for item in records),
            encoding="utf-8",
        )
        return records


def run_quiet(args: list[str], cwd: Path | None = None, timeout: int = 1800) -> str:
    result = subprocess.run(
        args, cwd=cwd, text=True, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, timeout=timeout, check=True,
    )
    return result.stdout


def setup_repo(instance: dict, work_root: Path, agent: str) -> Path:
    canonical = work_root / "canonical" / instance["instance_id"]
    lock_path = work_root / "locks" / (instance["instance_id"] + ".lock")
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    with lock_path.open("w", encoding="utf-8") as lock:
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        if not (canonical / ".git").is_dir():
            canonical.parent.mkdir(parents=True, exist_ok=True)
            canonical.mkdir(parents=True, exist_ok=True)
            run_quiet(["git", "init", "-q"], cwd=canonical)
            run_quiet(["git", "remote", "add", "origin", f"https://github.com/{instance['repo']}.git"], cwd=canonical)
            run_quiet(["git", "fetch", "--depth", "1", "origin", instance["base_commit"]], cwd=canonical)
            run_quiet(["git", "checkout", "-q", "--detach", "FETCH_HEAD"], cwd=canonical)
            run_quiet(["git", "remote", "remove", "origin"], cwd=canonical)
    target = work_root / "repos" / instance["instance_id"] / agent
    if target.exists():
        shutil.rmtree(target)
    target.parent.mkdir(parents=True, exist_ok=True)
    run_quiet(["git", "clone", "-q", "--no-hardlinks", str(canonical), str(target)])
    run_quiet(["git", "checkout", "-q", "--detach", instance["base_commit"]], cwd=target)
    run_quiet(["git", "config", "user.email", "benchmark@example.invalid"], cwd=target)
    run_quiet(["git", "config", "user.name", "Benchmark Runner"], cwd=target)
    return target


def prompt_for(instance: dict) -> str:
    return (
        "Resolve the following software engineering issue in this repository. "
        "Work autonomously: inspect the code, edit the implementation, and run relevant tests when practical. "
        "Do not use the network, search the web, inspect remote repositories, inspect git history beyond HEAD, "
        "or access files outside this repository. Do not commit changes. "
        "Finish only after implementing the best complete fix you can.\n\nISSUE:\n"
        + instance["problem_statement"]
    )


def agent_command(agent: str, binary: str, repo: Path, prompt: str, debug_path: Path, base_url: str) -> list[str]:
    if agent == "codex":
        return [
            binary, "-a", "never", "exec", "--json", "--ephemeral", "--ignore-user-config",
            "--ignore-rules", "-C", str(repo), "-m", MODEL,
            "-c", 'model_reasoning_effort="xhigh"', "-c", 'service_tier="default"',
            "-c", 'model_provider="benchmark_meter"',
            "-c", 'model_providers.benchmark_meter.name="OpenAI"',
            "-c", f'model_providers.benchmark_meter.base_url="{base_url}"',
            "-c", "model_providers.benchmark_meter.requires_openai_auth=true",
            "-c", 'model_providers.benchmark_meter.wire_api="responses"',
            "-c", "model_providers.benchmark_meter.supports_websockets=false",
            "-c", "disable_response_storage=true", "-s", "workspace-write", prompt,
        ]
    return [
        binary, "--print", "--output-format", "stream-json", "--provider", "benchmark-meter",
        "--api", "responses", "--model", MODEL, "--reasoning-effort", EFFORT,
        "--service-tier", "default", "--pinned-model", "--no-model-fallback",
        "--allow-all", "--force-sandbox-tools", "--allowed-tools", "Inspect,ApplyPatch,Run",
        "--disallowed-tools", "WebSearch,WebFetch,Agent,Skill,TeamCreate,SendMessage",
        "--max-turns", "100", "--debug-file", str(debug_path), prompt,
    ]


def terminate_group(process: subprocess.Popen) -> None:
    try:
        os.killpg(process.pid, signal.SIGTERM)
        process.wait(timeout=10)
    except (ProcessLookupError, subprocess.TimeoutExpired):
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def parse_events(path: Path) -> list[dict]:
    values = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        try:
            value = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(value, dict):
            values.append(value)
    return values


def usage_and_tools(agent: str, events: list[dict]) -> tuple[dict, dict[str, int]]:
    tools: dict[str, int] = {}
    if agent == "codex":
        usage = {"input_tokens": 0, "cached_input_tokens": 0, "cache_creation_input_tokens": 0, "output_tokens": 0}
        seen: set[str] = set()
        for event in events:
            if event.get("type") == "turn.completed":
                latest = event.get("usage") or {}
                usage["input_tokens"] = int(latest.get("input_tokens") or 0)
                usage["cached_input_tokens"] = int(latest.get("cached_input_tokens") or 0)
                usage["output_tokens"] = int(latest.get("output_tokens") or 0)
                if latest.get("reasoning_output_tokens") is not None:
                    usage["reasoning_output_tokens"] = int(latest["reasoning_output_tokens"])
            item = event.get("item") or {}
            item_id = str(item.get("id") or "")
            item_type = str(item.get("type") or "")
            if event.get("type") == "item.completed" and item_type not in {"", "agent_message", "reasoning", "error", "todo_list"} and item_id not in seen:
                seen.add(item_id)
                tools[item_type] = tools.get(item_type, 0) + 1
        return usage, tools
    usage = {"input_tokens": 0, "cached_input_tokens": 0, "cache_creation_input_tokens": 0, "output_tokens": 0}
    seen = set()
    for event in events:
        if event.get("type") == "usage" and event.get("scope") == "last_request":
            usage["input_tokens"] += int(event.get("input_tokens") or 0)
            usage["output_tokens"] += int(event.get("output_tokens") or 0)
            usage["cached_input_tokens"] += int(event.get("cache_read_input_tokens") or 0)
            usage["cache_creation_input_tokens"] += int(event.get("cache_creation_input_tokens") or 0)
        if event.get("type") == "tool_use":
            tool_id = str(event.get("tool_use_id") or "")
            if tool_id not in seen:
                seen.add(tool_id)
                name = str(event.get("name") or "unknown")
                tools[name] = tools.get(name, 0) + 1
    return usage, tools


def estimated_cost(usage: dict) -> float:
    total = int(usage.get("input_tokens") or 0)
    cached = int(usage.get("cached_input_tokens") or 0)
    cache_write = int(usage.get("cache_creation_input_tokens") or 0)
    uncached = max(total - cached - cache_write, 0)
    output = int(usage.get("output_tokens") or 0)
    return (uncached * PRICE["input"] + cached * PRICE["cached"] + cache_write * PRICE["cache_write"] + output * PRICE["output"]) / 1_000_000


def patch_stats(patch: str) -> dict:
    files = re.findall(r"^diff --git a/(.+?) b/", patch, flags=re.MULTILINE)
    return {
        "files_changed": len(files), "files": files,
        "additions": len(re.findall(r"^\+(?!\+\+\+)", patch, flags=re.MULTILINE)),
        "deletions": len(re.findall(r"^-(?!---)", patch, flags=re.MULTILINE)),
    }


def run_agent(args, instance: dict, upstream: str, key: str) -> dict:
    result_root = Path(args.result_root).resolve()
    run_dir = result_root / "raw" / "runs" / instance["instance_id"] / args.agent
    run_dir.mkdir(parents=True, exist_ok=True)
    repo = setup_repo(instance, Path(args.work_root).resolve(), args.agent)
    meter = RequestMeter(upstream, key, run_dir / "provider-requests.jsonl")
    meter.start()
    isolated_home = run_dir / "empty-home"
    isolated_home.mkdir(mode=0o700, exist_ok=True)
    luban_config = isolated_home / ".luban-code"
    luban_config.mkdir(mode=0o700, exist_ok=True)
    (luban_config / "auth.json").write_text(json.dumps({
        "entries": {"benchmark-meter": {
            "provider": "benchmark-meter", "auth_method": "api_key",
            "api_key": "benchmark-placeholder", "expires_at": "0001-01-01T00:00:00Z",
            "base_url": meter.base_url, "api_style": "openai",
            "display_name": "benchmark-meter", "user_defined": True,
        }}
    }, separators=(",", ":")) + "\n", encoding="utf-8")
    (luban_config / "auth.json").chmod(0o600)
    (luban_config / "language.json").write_text('{"language":"en"}\n', encoding="utf-8")
    prompt = prompt_for(instance)
    binary = args.codex_bin if args.agent == "codex" else args.luban_bin
    command = agent_command(args.agent, binary, repo, prompt, run_dir / "provider-debug.log", meter.base_url)
    child_env = {name: value for name, value in os.environ.items() if name in SAFE_ENV}
    child_env.update({
        "HOME": str(isolated_home), "OPENAI_API_KEY": "benchmark-placeholder",
        "OPENAI_BASE_URL": meter.base_url, "NO_COLOR": "1", "TERM": "dumb",
        "LANG": "en_US.UTF-8", "LC_ALL": "en_US.UTF-8",
    })
    started = datetime.now(timezone.utc)
    started_mono = time.monotonic()
    process = subprocess.Popen(
        command, cwd=repo, env=child_env, text=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True,
    )
    timed_out = False
    try:
        stdout, stderr = process.communicate(timeout=args.timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        terminate_group(process)
        stdout, stderr = process.communicate()
    elapsed = time.monotonic() - started_mono
    records = meter.stop()
    (run_dir / "events.jsonl").write_text(stdout, encoding="utf-8")
    (run_dir / "stderr.log").write_text(stderr, encoding="utf-8")
    patch = run_quiet(["git", "--no-pager", "diff", "HEAD", "--text"], cwd=repo)
    (run_dir / "model.patch").write_text(patch, encoding="utf-8")
    events = parse_events(run_dir / "events.jsonl")
    usage, tools = usage_and_tools(args.agent, events)
    generations = [item for item in records if item["method"] == "POST" and item["endpoint"] == "responses"]
    identity = hashlib.sha256(Path(binary).read_bytes()).hexdigest()
    relative_root = Path("raw") / "runs" / instance["instance_id"] / args.agent
    return {
        "instance_id": instance["instance_id"], "language": instance["language"],
        "agent": args.agent, "model": MODEL, "reasoning_effort": EFFORT,
        "started_at": started.isoformat(), "elapsed_seconds": round(elapsed, 3),
        "timeout_seconds": args.timeout, "timed_out": timed_out, "exit_code": process.returncode,
        "usage": usage, "estimated_cost_usd": round(estimated_cost(usage), 6),
        "tool_events": sum(tools.values()), "tool_events_by_type": tools,
        "patch": patch_stats(patch), "llm_calls": len(generations),
        "llm_successful_calls": sum(200 <= item["status"] < 300 for item in generations),
        "llm_failed_calls": sum(not (200 <= item["status"] < 300) for item in generations),
        "provider_request_seconds": round(sum(item["elapsed_seconds"] for item in generations), 6),
        "binary": {"name": args.agent, "sha256": identity},
        "evidence_root": relative_root.as_posix(),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--preflight", action="store_true")
    parser.add_argument("--catalog")
    parser.add_argument("--result-root")
    parser.add_argument("--work-root")
    parser.add_argument("--task")
    parser.add_argument("--agent", choices=["codex", "luban"])
    parser.add_argument("--codex-bin")
    parser.add_argument("--luban-bin")
    parser.add_argument("--timeout", type=int, default=1800)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    upstream, key = provider_credentials()
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    if args.preflight:
        parsed = urlsplit(upstream)
        origin = f"{parsed.scheme}://{parsed.hostname}" + (f":{parsed.port}" if parsed.port else "")
        output.write_text(json.dumps({"gateway_origin": origin}, indent=2) + "\n", encoding="utf-8")
        return 0
    required = [args.catalog, args.result_root, args.work_root, args.task, args.agent, args.luban_bin]
    if args.agent == "codex":
        required.append(args.codex_bin)
    if any(value is None for value in required):
        raise RuntimeError("worker_configuration_incomplete")
    catalog = load_catalog(Path(args.catalog))
    instance = catalog[args.task]
    result = run_agent(args, instance, upstream, key)
    output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
