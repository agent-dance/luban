#!/usr/bin/env python3
"""Fast local five-task runner with content-free provider request metering."""

from __future__ import annotations

import hashlib
import http.client
import importlib.util
import json
import os
import ssl
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parent
REPOSITORY_ROOT = ROOT.parent.parent
REPRESENTATIVE20_CATALOG = REPOSITORY_ROOT / "benchmark" / "agentic" / "localbench" / "catalog" / "representative20.json"
FROZEN_INSTANCES = ROOT / "raw" / "candidates" / "representative20-20260731" / "metadata" / "selected_instances.json"
UPSTREAM_IDLE_TIMEOUT_SECONDS = max(
    1.0,
    float(os.environ.get("LOCAL5_UPSTREAM_IDLE_TIMEOUT_SECONDS", "90")),
)
LEGACY = ROOT.parent / "agentic-2026-07-26" / "run_benchmark.py"
BENCHMARK_OFFLINE_ENV = {
    "CARGO_NET_OFFLINE": "true",
    "GOPROXY": "off",
    "GOSUMDB": "off",
    # Keep a single verification command from consuming most of the agent's
    # wall-clock budget. The Run schema exposes this cap to the model.
    "LUBAN_CODE_BASH_MAX_TIMEOUT_MS": "45000",
    # Maven's shell-only wrapper downloads the Maven distribution before the
    # Maven offline flag can take effect. Point that bootstrap at a closed
    # loopback port so benchmark verification fails fast without network I/O.
    "MVNW_REPOURL": "http://127.0.0.1:9",
    "MAVEN_ARGS": "--offline",
    "NPM_CONFIG_OFFLINE": "true",
    "PNPM_CONFIG_OFFLINE": "true",
    "YARN_ENABLE_NETWORK": "0",
    "PIP_NO_INDEX": "1",
    "UV_OFFLINE": "1",
}
GENERATED_TOP_LEVEL_DIRECTORIES = (
    ".gradle", ".luban-build", ".next", "build", "coverage",
    "dist", "node_modules", "out", "target",
)
GENERATED_TOP_LEVEL_PATTERNS = (".luban-build-*", "build-*", "build_*")


def load_upstream_key() -> str:
    key = os.environ.pop("LOCAL5_UPSTREAM_KEY", "")
    if key:
        return key
    auth_file = os.environ.get("LOCAL5_UPSTREAM_AUTH_FILE", "").strip()
    entry_name = os.environ.get("LOCAL5_UPSTREAM_AUTH_ENTRY", "").strip()
    if not auth_file or not entry_name:
        return ""
    document = json.loads(Path(auth_file).expanduser().read_text(encoding="utf-8"))
    entry = (document.get("entries") or {}).get(entry_name) or {}
    return str(entry.get("api_key") or "")


def load_legacy():
    spec = importlib.util.spec_from_file_location("local5_legacy_runner", LEGACY)
    if spec is None or spec.loader is None:
        raise RuntimeError("cannot load the local benchmark runner")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.ROOT = ROOT
    module.RUNS_DIR = Path(
        os.environ.get("LOCAL5_RUNS_DIR", ROOT / "raw" / "runs")
    ).resolve()
    module.METADATA_DIR = Path(
        os.environ.get("LOCAL5_METADATA_DIR", ROOT / "raw" / "metadata")
    ).resolve()
    module.SAFE_CHILD_ENV_KEYS = set(module.SAFE_CHILD_ENV_KEYS) | {"OPENAI_BASE_URL"} | set(BENCHMARK_OFFLINE_ENV)
    module.SAFE_CHILD_ENV_KEYS.discard("CODEX_LB_API_KEY")
    return module


legacy = load_legacy()
representative20 = json.loads(REPRESENTATIVE20_CATALOG.read_text(encoding="utf-8"))
legacy.SELECTED = {row["instance_id"]: row["language"] for row in representative20}


def load_frozen_instances() -> dict[str, dict]:
    rows = json.loads(FROZEN_INSTANCES.read_text(encoding="utf-8"))
    by_id = {row["instance_id"]: row for row in rows}
    missing = sorted(set(legacy.SELECTED) - set(by_id))
    if missing:
        raise RuntimeError(f"frozen benchmark metadata is missing: {', '.join(missing)}")
    return {instance_id: dict(by_id[instance_id]) for instance_id in legacy.SELECTED}


legacy.load_instances = load_frozen_instances
original_write_metadata = legacy.write_metadata


def write_representative20_metadata(instances: dict[str, dict], include_gold: bool = False) -> None:
    original_write_metadata(instances, include_gold=include_gold)
    experiment_path = legacy.METADATA_DIR / "experiment.json"
    experiment = json.loads(experiment_path.read_text(encoding="utf-8"))
    experiment["parquet_sha256"] = {
        "cpp": "5afc7db10f28232cc9c13de316ecec146f2da4c76de3bb460b934f5e271b0ec0",
        "go": "76d2b5dff0f3fac8303d30fa85495539e487d25974ad7c21cd21a545cb4756e2",
        "java": "cc04473f299dbdbbb6c4061da3c68367cd460e28e40c04234f4887e0fc234220",
        "rust": "ea90be54a621c0c0280b77d5e2dee9650bc1d4ae087f9b9b06af821bcd8662d7",
        "ts": "7e23783e27230c9cfab1035690035c25523043d6af635bc78da3fd2010c32714",
    }
    experiment["selection_manifest"] = str(REPRESENTATIVE20_CATALOG.with_suffix(".selection.json").relative_to(REPOSITORY_ROOT))
    experiment["task_count"] = len(legacy.SELECTED)
    experiment_path.write_text(json.dumps(experiment, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


legacy.write_metadata = write_representative20_metadata


class RequestMeter:
    def __init__(self, output: Path):
        upstream = urlsplit(os.environ.get("LOCAL5_UPSTREAM", "https://sub.blurooo.com"))
        if upstream.scheme != "https" or not upstream.hostname:
            raise RuntimeError("LOCAL5_UPSTREAM must be an HTTPS origin")
        self.upstream = upstream
        self.key = load_upstream_key()
        if not self.key:
            raise RuntimeError("LOCAL5_UPSTREAM_KEY is required")
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
                prefix = meter.upstream.path.rstrip("/")
                path = prefix + self.path
                headers = {
                    name: value
                    for name, value in self.headers.items()
                    if name.lower()
                    not in {
                        "authorization",
                        "host",
                        "connection",
                        "content-length",
                        "transfer-encoding",
                        "accept-encoding",
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
                        meter.upstream.hostname,
                        meter.upstream.port or 443,
                        timeout=UPSTREAM_IDLE_TIMEOUT_SECONDS,
                        context=ssl.create_default_context(),
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
                        if name.lower() in {
                            "connection",
                            "content-length",
                            "transfer-encoding",
                            "keep-alive",
                        }:
                            continue
                        self.send_header(name, value)
                    self.send_header("Connection", "close")
                    self.end_headers()
                    while True:
                        # Responses is an SSE stream. read(amt) may wait until
                        # amt bytes arrive, which deadlocks small completed
                        # responses behind this proxy; read1 forwards each
                        # available upstream chunk immediately.
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
                        payload = b'{"error":{"message":"local benchmark proxy failure"}}'
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


_proxy_base = ""


def codex_command(repo: Path, prompt: str) -> list[str]:
    return [
        os.environ["LOCAL5_CODEX_BIN"],
        "-a", "never", "exec", "--json", "--ephemeral", "--ignore-user-config",
        "--ignore-rules", "-C", str(repo), "-m", legacy.MODEL,
        "-c", 'model_reasoning_effort="xhigh"',
        "-c", 'model_provider="local_meter"',
        "-c", 'model_providers.local_meter.name="OpenAI"',
        "-c", f'model_providers.local_meter.base_url="{_proxy_base}"',
        "-c", "model_providers.local_meter.requires_openai_auth=true",
        "-c", 'model_providers.local_meter.wire_api="responses"',
        "-c", "disable_response_storage=true",
        "-s", "workspace-write", prompt,
    ]


def luban_command(repo: Path, prompt: str, debug_path: Path) -> list[str]:
    effort = os.environ.get("LOCAL5_LUBAN_EFFORT", legacy.EFFORT).strip() or legacy.EFFORT
    return [
        os.environ["LOCAL5_LUBAN_BIN"],
        "--print", "--model", legacy.MODEL, "--provider", "openai", "--api", "responses",
        "--reasoning-effort", effort,
        "--pinned-model", "--no-model-fallback", "--output-format", "stream-json",
        "--allow-all", "--allowed-tools", "Inspect,ApplyPatch,Run",
        "--disallowed-tools", "WebSearch,WebFetch,Agent,Skill,TeamCreate,SendMessage",
        "--max-turns", "100", "--debug-file", str(debug_path), prompt,
    ]


legacy.codex_command = codex_command
legacy.luban_command = luban_command
original_run_agent = legacy.run_agent


def capture_workspace_patch(repo: Path) -> str:
    """Capture tracked and non-ignored untracked changes without touching the real index."""
    with tempfile.TemporaryDirectory(prefix="luban-benchmark-index-") as directory:
        index_path = Path(directory) / "index"
        env = os.environ.copy()
        env["GIT_INDEX_FILE"] = str(index_path)
        git = [
            "git", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false",
            "-c", "submodule.recurse=false",
        ]
        subprocess.run(
            [*git, "read-tree", "HEAD"], cwd=repo, env=env,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True,
        )
        generated_excludes = [
            pathspec
            for directory in GENERATED_TOP_LEVEL_DIRECTORIES
            for pathspec in (f":(exclude){directory}", f":(exclude){directory}/**")
        ]
        generated_excludes.extend(
            pathspec
            for pattern in GENERATED_TOP_LEVEL_PATTERNS
            for pathspec in (f":(exclude,glob){pattern}", f":(exclude,glob){pattern}/**")
        )
        subprocess.run(
            [*git, "add", "-A", "--", ".", *generated_excludes], cwd=repo, env=env,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True,
        )
        result = subprocess.run(
            [*git, "--no-pager", "diff", "--cached", "--binary", "HEAD", "--"],
            cwd=repo, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True,
        )
    return result.stdout.decode("utf-8")


def metered_run_agent(instance: dict, agent: str, timeout: int) -> dict:
    global _proxy_base
    run_dir = legacy.RUNS_DIR / instance["instance_id"] / agent
    if run_dir.exists():
        import shutil
        shutil.rmtree(run_dir)
    run_dir.mkdir(parents=True, exist_ok=True)
    meter = RequestMeter(run_dir / "provider-requests.jsonl")
    meter.start()
    _proxy_base = meter.base_url
    prior_base = os.environ.get("OPENAI_BASE_URL")
    prior_key = os.environ.get("OPENAI_API_KEY")
    prior_home = os.environ.get("HOME")
    prior_lang = os.environ.get("LANG")
    prior_lc_all = os.environ.get("LC_ALL")
    prior_offline = {name: os.environ.get(name) for name in BENCHMARK_OFFLINE_ENV}
    isolated_home = run_dir / "empty-home"
    isolated_home.mkdir(mode=0o700, exist_ok=True)
    luban_config = isolated_home / ".luban-code"
    luban_config.mkdir(mode=0o700, exist_ok=True)
    auth_path = luban_config / "auth.json"
    auth_path.write_text(
        json.dumps(
            {
                "entries": {
                    "openai": {
                        "provider": "openai",
                        "auth_method": "api_key",
                        "api_key": "local-benchmark-placeholder",
                        "expires_at": "0001-01-01T00:00:00Z",
                        "base_url": meter.base_url,
                        "api_style": "openai",
                        "api_format": "responses",
                        "disable_strict_tools": True,
                        "disable_prompt_cache_options": True,
                        "display_name": "local-meter",
                    }
                }
            },
            sort_keys=True,
            separators=(",", ":"),
        )
        + "\n",
        encoding="utf-8",
    )
    auth_path.chmod(0o600)
    (luban_config / "language.json").write_text(
        '{"language":"en"}\n', encoding="utf-8"
    )
    os.environ["OPENAI_BASE_URL"] = meter.base_url
    os.environ["OPENAI_API_KEY"] = "local-benchmark-placeholder"
    os.environ["HOME"] = str(isolated_home)
    if agent == "luban":
        os.environ["LANG"] = "en_US.UTF-8"
        os.environ["LC_ALL"] = "en_US.UTF-8"
    os.environ.update(BENCHMARK_OFFLINE_ENV)
    try:
        summary = original_run_agent(instance, agent, timeout)
        repo = legacy.WORK_ROOT / "repos" / instance["instance_id"] / agent
        patch = capture_workspace_patch(repo)
        (run_dir / "model.patch").write_text(patch, encoding="utf-8")
        summary["patch"] = legacy.patch_stats(patch)
    finally:
        records = meter.stop()
        if prior_base is None:
            os.environ.pop("OPENAI_BASE_URL", None)
        else:
            os.environ["OPENAI_BASE_URL"] = prior_base
        if prior_key is None:
            os.environ.pop("OPENAI_API_KEY", None)
        else:
            os.environ["OPENAI_API_KEY"] = prior_key
        if prior_home is None:
            os.environ.pop("HOME", None)
        else:
            os.environ["HOME"] = prior_home
        if prior_lang is None:
            os.environ.pop("LANG", None)
        else:
            os.environ["LANG"] = prior_lang
        if prior_lc_all is None:
            os.environ.pop("LC_ALL", None)
        else:
            os.environ["LC_ALL"] = prior_lc_all
        for name, value in prior_offline.items():
            if value is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = value
    generations = [item for item in records if item["method"] == "POST" and item["endpoint"] == "responses"]
    summary["llm_calls"] = len(generations)
    summary["llm_successful_calls"] = sum(200 <= item["status"] < 300 for item in generations)
    summary["llm_failed_calls"] = sum(not (200 <= item["status"] < 300) for item in generations)
    summary["provider_request_seconds"] = round(sum(item["elapsed_seconds"] for item in generations), 6)
    if agent == "luban":
        effort = os.environ.get("LOCAL5_LUBAN_EFFORT", legacy.EFFORT).strip() or legacy.EFFORT
        summary["reasoning_effort"] = effort
        summary["command_public"]["reasoning_effort"] = effort
    summary["binary"] = {
        "path": os.environ["LOCAL5_CODEX_BIN"] if agent == "codex" else os.environ["LOCAL5_LUBAN_BIN"],
        "sha256": hashlib.sha256(
            Path(os.environ["LOCAL5_CODEX_BIN"] if agent == "codex" else os.environ["LOCAL5_LUBAN_BIN"]).read_bytes()
        ).hexdigest(),
    }
    (run_dir / "summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return summary


legacy.run_agent = metered_run_agent


if __name__ == "__main__":
    sys.exit(legacy.main())
