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


class ResponseUsageCollector:
    """Extracts only billable usage numbers from Responses JSON/SSE payloads."""

    def __init__(self):
        self.buffer = b""
        self.usage: dict[str, int | str] | None = None

    def feed(self, chunk: bytes) -> None:
        self.buffer += chunk
        while b"\n" in self.buffer:
            line, self.buffer = self.buffer.split(b"\n", 1)
            self._consume(line)

    def finish(self) -> dict[str, int | str] | None:
        if self.buffer.strip():
            self._consume(self.buffer)
        self.buffer = b""
        return self.usage

    def _consume(self, line: bytes) -> None:
        line = line.strip()
        if not line:
            return
        if line.startswith(b"data:"):
            line = line[5:].strip()
        if line == b"[DONE]":
            return
        try:
            payload = json.loads(line)
        except (json.JSONDecodeError, UnicodeDecodeError):
            return
        if not isinstance(payload, dict):
            return
        response = payload.get("response")
        usage = response.get("usage") if isinstance(response, dict) else None
        if not isinstance(usage, dict):
            usage = payload.get("usage")
        if not isinstance(usage, dict):
            return
        input_details = usage.get("input_tokens_details") or {}
        output_details = usage.get("output_tokens_details") or {}
        if not isinstance(input_details, dict):
            input_details = {}
        if not isinstance(output_details, dict):
            output_details = {}
        self.usage = {
            "input_tokens": int(usage.get("input_tokens") or usage.get("prompt_tokens") or 0),
            "cached_input_tokens": int(
                input_details.get("cached_tokens")
                or usage.get("cache_read_input_tokens")
                or 0
            ),
            "cache_creation_input_tokens": int(
                input_details.get("cache_write_tokens")
                or usage.get("cache_creation_input_tokens")
                or 0
            ),
            "output_tokens": int(usage.get("output_tokens") or usage.get("completion_tokens") or 0),
            "reasoning_output_tokens": int(
                output_details.get("reasoning_tokens")
                or usage.get("reasoning_output_tokens")
                or 0
            ),
        }
        model = response.get("model") if isinstance(response, dict) else payload.get("model")
        if isinstance(model, str) and model.strip():
            self.usage["served_model"] = model.strip()


def load_catalog(path: Path) -> dict[str, dict]:
    values = json.loads(path.read_text(encoding="utf-8"))
    return {value["instance_id"]: value for value in values}


def codex_home() -> Path:
    configured = os.environ.get("CODEX_HOME")
    return Path(configured).expanduser() if configured else Path.home() / ".codex"


def provider_credentials(args) -> tuple[str, str]:
    if args.upstream_luban_provider:
        root = Path.home() / ".luban-code"
        auth = json.loads((root / "auth.json").read_text(encoding="utf-8"))
        entry = (auth.get("entries") or {}).get(args.upstream_luban_provider) or {}
        key = str(entry.get("api_key") or "")
        upstream = str(args.upstream_base_url or entry.get("base_url") or "").rstrip("/")
        if not key:
            raise RuntimeError("luban_provider_api_key_missing")
        parsed = urlsplit(upstream)
        if parsed.scheme != "https" or not parsed.hostname:
            raise RuntimeError("luban_provider_origin_invalid")
        return upstream, key
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
                usage_collector = ResponseUsageCollector()
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
                        usage_collector.feed(chunk)
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
                    usage = usage_collector.finish()
                    if usage is not None:
                        record.update(usage)
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


def agent_command(
    agent: str, binary: str, repo: Path, prompt: str, debug_path: Path, base_url: str,
    pinned_model: bool = True, explicit_default_service_tier: bool = True,
    context_update_shadow: bool = False, provider_name: str = "benchmark-meter",
    model: str = MODEL, reasoning_effort: str = EFFORT, max_turns: int = 100,
) -> list[str]:
    if agent == "codex":
        return [
            binary, "-a", "never", "exec", "--json", "--ephemeral", "--ignore-user-config",
            "--ignore-rules", "-C", str(repo), "-m", model,
            "-c", f'model_reasoning_effort="{reasoning_effort}"', "-c", 'service_tier="default"',
            "-c", 'model_provider="benchmark_meter"',
            "-c", 'model_providers.benchmark_meter.name="OpenAI"',
            "-c", f'model_providers.benchmark_meter.base_url="{base_url}"',
            "-c", "model_providers.benchmark_meter.requires_openai_auth=true",
            "-c", 'model_providers.benchmark_meter.wire_api="responses"',
            "-c", "model_providers.benchmark_meter.supports_websockets=false",
            "-c", "disable_response_storage=true", "-s", "workspace-write", prompt,
        ]
    allowed_tools = "Inspect,ApplyPatch,Run"
    if context_update_shadow:
        allowed_tools += ",ContextUpdate"
    command = [
        binary, "--print", "--output-format", "stream-json", "--provider", provider_name,
        "--api", "responses", "--model", model, "--reasoning-effort", reasoning_effort,
        "--no-model-fallback",
        "--allow-all", "--force-sandbox-tools", "--allowed-tools", allowed_tools,
        "--disallowed-tools", "WebSearch,WebFetch,Agent,Skill,TeamCreate,SendMessage",
        "--max-turns", str(max_turns), "--debug-file", str(debug_path), prompt,
    ]
    if pinned_model:
        command.insert(command.index("--no-model-fallback"), "--pinned-model")
    if explicit_default_service_tier:
        command[command.index("--no-model-fallback"):command.index("--no-model-fallback")] = ["--service-tier", "default"]
    return command


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


def provider_usage(records: list[dict]) -> dict:
    usage = {
        "input_tokens": 0,
        "cached_input_tokens": 0,
        "cache_creation_input_tokens": 0,
        "output_tokens": 0,
        "reasoning_output_tokens": 0,
    }
    observed = False
    for record in records:
        if "input_tokens" not in record:
            continue
        observed = True
        for key in usage:
            usage[key] += int(record.get(key) or 0)
    return usage if observed else {}


def context_metrics(events: list[dict]) -> dict:
    agent_turns = 0
    compaction_turns: list[int] = []
    projection_turns: list[int] = []
    projection_batches: list[dict] = []
    projected_tools = 0
    rewritten_tools = 0
    indexed_tools = 0
    candidate_tools = 0
    candidate_tokens_saved = 0
    original_bytes = 0
    projected_bytes = 0
    bytes_saved = 0
    original_tokens = 0
    projected_tokens = 0
    tokens_saved = 0
    estimated_net_savings_usd = 0.0
    context_update_proposals = 0
    context_update_runtime_candidates = 0
    context_update_actions: dict[str, int] = {}
    context_update_reason_codes: dict[str, int] = {}
    for event in events:
        turn = int(event.get("turn_count") or 0)
        if turn <= 0:
            match = re.search(r":turn-(\d+)$", str(event.get("turn_id") or ""))
            if match:
                turn = int(match.group(1))
        agent_turns = max(agent_turns, turn)
        if event.get("type") != "agentic_metrics":
            continue
        metric = event.get("metric")
        if metric == "context_compaction":
            if turn > 0:
                compaction_turns.append(turn)
        elif metric == "context_projection":
            projection = event.get("context_projection") or {}
            if turn > 0:
                projection_turns.append(turn)
            decision = str(projection.get("decision") or "")
            applied = projection.get("applied") is True or decision.startswith("admit_")
            batch_tools = int(projection.get("projection_count") or 0)
            batch_tokens_saved = int(projection.get("tokens_saved") or 0)
            candidate_tools += batch_tools
            candidate_tokens_saved += batch_tokens_saved
            if applied:
                projected_tools += batch_tools
                rewritten_tools += int(projection.get("rewrite_count") or 0)
                indexed_tools += int(projection.get("index_count") or 0)
                original_bytes += int(projection.get("original_bytes") or 0)
                projected_bytes += int(projection.get("projected_bytes") or 0)
                bytes_saved += int(projection.get("bytes_saved") or 0)
                original_tokens += int(projection.get("original_tokens") or 0)
                projected_tokens += int(projection.get("projected_tokens") or 0)
                tokens_saved += batch_tokens_saved
                estimated_net_savings_usd += float(projection.get("estimated_net_savings_usd") or 0)
            projection_batches.append({
                "turn": turn,
                "trigger": str(projection.get("trigger") or ""),
                "applied": applied,
                "projected_tools": batch_tools,
                "rewritten_tools": int(projection.get("rewrite_count") or 0),
                "indexed_tools": int(projection.get("index_count") or 0),
                "original_bytes": int(projection.get("original_bytes") or 0),
                "projected_bytes": int(projection.get("projected_bytes") or 0),
                "bytes_saved": int(projection.get("bytes_saved") or 0),
                "original_tokens": int(projection.get("original_tokens") or 0),
                "projected_tokens": int(projection.get("projected_tokens") or 0),
                "tokens_saved": int(projection.get("tokens_saved") or 0),
                "decision": decision,
                "request_tokens_before": int(projection.get("request_tokens_before") or 0),
                "request_tokens_after": int(projection.get("request_tokens_after") or 0),
                "cache_break_cost_usd": float(projection.get("cache_break_cost_usd") or 0),
                "gross_cache_break_cost_usd": float(projection.get("gross_cache_break_cost_usd") or 0),
                "avoided_compact_input_cost_usd": float(projection.get("avoided_compact_input_cost_usd") or 0),
                "estimated_net_savings_usd": float(projection.get("estimated_net_savings_usd") or 0),
                "avoids_immediate_compaction": projection.get("avoids_immediate_compaction") is True,
            })
        elif metric == "context_update":
            update = event.get("context_update") or {}
            context_update_proposals += 1
            if update.get("runtime_candidate") is True:
                context_update_runtime_candidates += 1
            action = str(update.get("action") or "")
            if action:
                context_update_actions[action] = context_update_actions.get(action, 0) + 1
            reason_code = str(update.get("reason_code") or "")
            if reason_code:
                context_update_reason_codes[reason_code] = context_update_reason_codes.get(reason_code, 0) + 1
    return {
        "agent_turns": agent_turns,
        "compaction_turns": sorted(set(compaction_turns)),
        "first_compaction_turn": min(compaction_turns) if compaction_turns else None,
        "projection_turns": sorted(set(projection_turns)),
        "projection_batches": projection_batches,
        "projected_tools": projected_tools,
        "rewritten_tools": rewritten_tools,
        "indexed_tools": indexed_tools,
        "candidate_tools": candidate_tools,
        "candidate_tokens_saved": candidate_tokens_saved,
        "original_bytes": original_bytes,
        "projected_bytes": projected_bytes,
        "bytes_saved": bytes_saved,
        "original_tokens": original_tokens,
        "projected_tokens": projected_tokens,
        "tokens_saved": tokens_saved,
        "estimated_net_savings_usd": round(estimated_net_savings_usd, 9),
        "context_update_proposals": context_update_proposals,
        "context_update_runtime_candidates": context_update_runtime_candidates,
        "context_update_actions": context_update_actions,
        "context_update_reason_codes": context_update_reason_codes,
    }


def estimated_cost(usage: dict, price: dict[str, float] = PRICE) -> float:
    total = int(usage.get("input_tokens") or 0)
    cached = int(usage.get("cached_input_tokens") or 0)
    cache_write = int(usage.get("cache_creation_input_tokens") or 0)
    uncached = max(total - cached - cache_write, 0)
    output = int(usage.get("output_tokens") or 0)
    return (uncached * price["input"] + cached * price["cached"] + cache_write * price["cache_write"] + output * price["output"]) / 1_000_000


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
    provider_entry = {
            "provider": args.benchmark_provider, "auth_method": "api_key",
            "api_key": "benchmark-placeholder", "expires_at": "0001-01-01T00:00:00Z",
            "base_url": meter.base_url,
    }
    if args.benchmark_provider == "benchmark-meter":
        provider_entry.update({"api_style": "openai", "display_name": "benchmark-meter", "user_defined": True})
    (luban_config / "auth.json").write_text(json.dumps({
        "entries": {args.benchmark_provider: provider_entry}
    }, separators=(",", ":")) + "\n", encoding="utf-8")
    (luban_config / "auth.json").chmod(0o600)
    (luban_config / "language.json").write_text('{"language":"en"}\n', encoding="utf-8")
    if args.progressive_tools:
        progressive_tools = [value.strip() for value in args.progressive_tools.split(",") if value.strip()]
        progressive_context = {
            "enabled": True,
            "toolAllowlist": progressive_tools,
            "providerAllowlist": [args.benchmark_provider],
            "modelAllowlist": [args.benchmark_model],
        }
        if args.progressive_imminent_compact_counterfactual:
            progressive_context["imminentCompactProviderAllowlist"] = [args.benchmark_provider]
        if args.progressive_auto_compact_keep_recent is not None:
            progressive_context["autoCompactKeepRecent"] = args.progressive_auto_compact_keep_recent
        if args.progressive_auto_compact_max_growth_tokens is not None:
            progressive_context["autoCompactMaxGrowthTokens"] = args.progressive_auto_compact_max_growth_tokens
        if args.progressive_auto_compact_min_threshold_percent is not None:
            progressive_context["autoCompactMinThresholdPercent"] = args.progressive_auto_compact_min_threshold_percent
        if args.progressive_require_consumed_mutation:
            progressive_context["requireConsumedMutation"] = True
        if args.progressive_flatten_compact_input:
            progressive_context["flattenCompactInput"] = True
        if args.progressive_concise_compact_summary:
            progressive_context["conciseCompactSummary"] = True
        if args.progressive_compact_max_output_tokens is not None:
            progressive_context["compactMaxOutputTokens"] = args.progressive_compact_max_output_tokens
        (luban_config / "settings.json").write_text(json.dumps({
            "progressiveContext": progressive_context
        }, separators=(",", ":")) + "\n", encoding="utf-8")
    prompt = prompt_for(instance)
    binary = args.codex_bin if args.agent == "codex" else args.luban_bin
    command = agent_command(
        args.agent, binary, repo, prompt, run_dir / "provider-debug.log", meter.base_url,
        pinned_model=not args.unpinned_model,
        explicit_default_service_tier=not args.implicit_default_service_tier,
        context_update_shadow=args.context_update_shadow,
        provider_name=args.benchmark_provider,
        model=args.benchmark_model,
        reasoning_effort=args.benchmark_reasoning_effort,
        max_turns=args.max_turns,
    )
    child_env = {name: value for name, value in os.environ.items() if name in SAFE_ENV}
    child_env.update({
        "HOME": str(isolated_home), "OPENAI_API_KEY": "benchmark-placeholder",
        "OPENAI_BASE_URL": meter.base_url, "NO_COLOR": "1", "TERM": "dumb",
        "LANG": "en_US.UTF-8", "LC_ALL": "en_US.UTF-8",
    })
    if args.experiment_max_context_tokens:
        child_env["LUBAN_EXPERIMENT_MAX_CONTEXT_TOKENS"] = str(args.experiment_max_context_tokens)
    if args.progressive_context_compaction:
        child_env["LUBAN_PROGRESSIVE_CONTEXT_COMPACTION"] = "1"
    if args.context_update_shadow:
        child_env["LUBAN_CONTEXT_UPDATE_SHADOW"] = "1"
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
    event_usage, tools = usage_and_tools(args.agent, events)
    measured_provider_usage = provider_usage(records)
    usage = measured_provider_usage or event_usage
    context = context_metrics(events)
    served_models = sorted({str(item.get("served_model")) for item in records if item.get("served_model")})
    generations = [item for item in records if item["method"] == "POST" and item["endpoint"] == "responses"]
    identity = hashlib.sha256(Path(binary).read_bytes()).hexdigest()
    relative_root = Path("raw") / "runs" / instance["instance_id"] / args.agent
    return {
        "instance_id": instance["instance_id"], "language": instance["language"],
        "agent": args.agent, "model": args.benchmark_model, "reasoning_effort": args.benchmark_reasoning_effort,
        "started_at": started.isoformat(), "elapsed_seconds": round(elapsed, 3),
        "timeout_seconds": args.timeout, "timed_out": timed_out, "exit_code": process.returncode,
        "usage": usage, "estimated_cost_usd": round(estimated_cost(usage, args.price), 6),
        "usage_source": "provider_responses" if measured_provider_usage else "stream_events",
        "stream_event_usage": event_usage,
        "context_metrics": context,
        "served_models": served_models,
        "experiment": {
            "max_context_tokens": args.experiment_max_context_tokens,
            "progressive_context_compaction": bool(args.progressive_context_compaction),
            "context_update_shadow": bool(args.context_update_shadow),
            "progressive_tools": [value.strip() for value in (args.progressive_tools or "").split(",") if value.strip()],
            "progressive_imminent_compact_counterfactual": bool(args.progressive_imminent_compact_counterfactual),
            "progressive_auto_compact_keep_recent": args.progressive_auto_compact_keep_recent,
            "progressive_auto_compact_max_growth_tokens": args.progressive_auto_compact_max_growth_tokens,
            "progressive_auto_compact_min_threshold_percent": args.progressive_auto_compact_min_threshold_percent,
            "progressive_require_consumed_mutation": bool(args.progressive_require_consumed_mutation),
            "progressive_flatten_compact_input": bool(args.progressive_flatten_compact_input),
            "progressive_concise_compact_summary": bool(args.progressive_concise_compact_summary),
            "progressive_compact_max_output_tokens": args.progressive_compact_max_output_tokens,
            "pinned_model_capability_required": not args.unpinned_model,
            "service_tier": "default_implicit" if args.implicit_default_service_tier else "default_explicit",
        },
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
    parser.add_argument("--experiment-max-context-tokens", type=int)
    parser.add_argument("--progressive-context-compaction", action="store_true")
    parser.add_argument("--context-update-shadow", action="store_true")
    parser.add_argument("--progressive-tools")
    parser.add_argument("--progressive-imminent-compact-counterfactual", action="store_true")
    parser.add_argument("--progressive-auto-compact-keep-recent", type=int)
    parser.add_argument("--progressive-auto-compact-max-growth-tokens", type=int)
    parser.add_argument("--progressive-auto-compact-min-threshold-percent", type=int)
    parser.add_argument("--progressive-require-consumed-mutation", action="store_true")
    parser.add_argument("--progressive-flatten-compact-input", action="store_true")
    parser.add_argument("--progressive-concise-compact-summary", action="store_true")
    parser.add_argument("--progressive-compact-max-output-tokens", type=int)
    parser.add_argument("--max-turns", type=int, default=100)
    parser.add_argument("--benchmark-provider", default="benchmark-meter")
    parser.add_argument("--benchmark-model", default=MODEL)
    parser.add_argument("--benchmark-reasoning-effort", default=EFFORT)
    parser.add_argument("--upstream-luban-provider")
    parser.add_argument("--upstream-base-url")
    parser.add_argument("--price-input", type=float, default=PRICE["input"])
    parser.add_argument("--price-cached", type=float, default=PRICE["cached"])
    parser.add_argument("--price-cache-write", type=float, default=PRICE["cache_write"])
    parser.add_argument("--price-output", type=float, default=PRICE["output"])
    parser.add_argument("--unpinned-model", action="store_true")
    parser.add_argument("--implicit-default-service-tier", action="store_true")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    args.price = {"input": args.price_input, "cached": args.price_cached, "cache_write": args.price_cache_write, "output": args.price_output}
    upstream, key = provider_credentials(args)
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
