#!/usr/bin/env python3
"""Fast local five-task runner with content-free provider request metering."""

from __future__ import annotations

import hashlib
import http.client
import importlib.util
import json
import os
import ssl
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parent
LEGACY = ROOT.parent / "agentic-2026-07-26" / "run_benchmark.py"


def load_legacy():
    spec = importlib.util.spec_from_file_location("local5_legacy_runner", LEGACY)
    if spec is None or spec.loader is None:
        raise RuntimeError("cannot load the local benchmark runner")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.ROOT = ROOT
    module.RUNS_DIR = ROOT / "raw" / "runs"
    module.METADATA_DIR = ROOT / "raw" / "metadata"
    module.SAFE_CHILD_ENV_KEYS = set(module.SAFE_CHILD_ENV_KEYS) | {"OPENAI_BASE_URL"}
    module.SAFE_CHILD_ENV_KEYS.discard("CODEX_LB_API_KEY")
    return module


legacy = load_legacy()


class RequestMeter:
    def __init__(self, output: Path):
        upstream = urlsplit(os.environ.get("LOCAL5_UPSTREAM", "https://sub.blurooo.com"))
        if upstream.scheme != "https" or not upstream.hostname:
            raise RuntimeError("LOCAL5_UPSTREAM must be an HTTPS origin")
        self.upstream = upstream
        self.key = os.environ.pop("LOCAL5_UPSTREAM_KEY", "")
        if not self.key:
            raise RuntimeError("LOCAL5_UPSTREAM_KEY is required")
        self.output = output
        self.records: list[dict] = []
        self.lock = threading.Lock()
        self.sequence = 0
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), self._handler())
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)

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
                        timeout=3600,
                        context=ssl.create_default_context(),
                    )
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
                        chunk = response.read(65536)
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
                    with meter.lock:
                        meter.records.append(record)

        return Handler

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.server.server_port}"

    def start(self) -> None:
        self.thread.start()

    def stop(self) -> list[dict]:
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        self.key = ""
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
    return [
        os.environ["LOCAL5_LUBAN_BIN"],
        "--print", "--model", legacy.MODEL, "--provider", "custom-local-meter", "--api", "responses",
        "--reasoning-effort", legacy.EFFORT,
        "--pinned-model", "--no-model-fallback", "--output-format", "stream-json",
        "--allow-all", "--allowed-tools", "Inspect,ApplyPatch,Run",
        "--disallowed-tools", "WebSearch,WebFetch,Agent,Skill,TeamCreate,SendMessage",
        "--max-turns", "100", "--debug-file", str(debug_path), prompt,
    ]


legacy.codex_command = codex_command
legacy.luban_command = luban_command
original_run_agent = legacy.run_agent


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
    isolated_home = run_dir / "empty-home"
    isolated_home.mkdir(mode=0o700, exist_ok=True)
    luban_config = isolated_home / ".luban-code"
    luban_config.mkdir(mode=0o700, exist_ok=True)
    auth_path = luban_config / "auth.json"
    auth_path.write_text(
        json.dumps(
            {
                "entries": {
                    "custom-local-meter": {
                        "provider": "custom-local-meter",
                        "auth_method": "api_key",
                        "api_key": "local-benchmark-placeholder",
                        "expires_at": "0001-01-01T00:00:00Z",
                        "base_url": meter.base_url,
                        "api_style": "openai",
                        "display_name": "local-meter",
                        "user_defined": True,
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
    try:
        summary = original_run_agent(instance, agent, timeout)
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
    generations = [item for item in records if item["method"] == "POST" and item["endpoint"] == "responses"]
    summary["llm_calls"] = len(generations)
    summary["llm_successful_calls"] = sum(200 <= item["status"] < 300 for item in generations)
    summary["llm_failed_calls"] = sum(not (200 <= item["status"] < 300) for item in generations)
    summary["provider_request_seconds"] = round(sum(item["elapsed_seconds"] for item in generations), 6)
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
