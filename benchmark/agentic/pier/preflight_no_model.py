"""Real Docker/Pier preflight for frozen agents without a provider model call."""

from __future__ import annotations

import argparse
import asyncio
import copy
import hashlib
import http.server
import json
import sys
import threading
from pathlib import Path

sys.dont_write_bytecode = True

from pier.models.task.task import Task
from pier.models.trial.paths import TrialPaths

from benchmark.agentic.pier.docker_environment import (
    AgenticBenchmarkDockerEnvironment,
)
from benchmark.agentic.pier.pinned_agent import (
    PinnedCLIAgent,
    _argv_sha256,
    _decode_sandbox_canary_v4,
    _formal_source_argv_tail,
    file_sha256,
)


class _HealthHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args) -> None:
        pass

    def do_GET(self) -> None:
        if self.path != "/healthz":
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Length", "2")
        self.end_headers()
        self.wfile.write(b"ok")


def _arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--task-dir", type=Path, required=True)
    parser.add_argument("--image", required=True)
    parser.add_argument("--bundle-root", type=Path, required=True)
    parser.add_argument("--bundle-manifest", type=Path, required=True)
    parser.add_argument("--codex-binary", type=Path, required=True)
    parser.add_argument("--luban-binary", type=Path, required=True)
    parser.add_argument("--egress-proxy-image", required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    return parser.parse_args()


async def _preflight_agent(
    *,
    kind: str,
    binary: Path,
    task: Task,
    image: str,
    bundle_root: Path,
    bundle_manifest: Path,
    bundle_tree_sha256: str,
    output_dir: Path,
    health_port: int,
    egress_proxy_image: str,
) -> dict:
    trial_paths = TrialPaths(output_dir / kind)
    trial_paths.mkdir()
    session_suffix = hashlib.sha256(str(output_dir).encode("utf-8")).hexdigest()[:8]
    base_commit = str(task.config.metadata["base_commit_hash"])
    command_argv = [str(binary.resolve()), *_formal_source_argv_tail(kind)]
    adapter_path = Path(__file__).with_name("pinned_agent.py").resolve()
    agent = PinnedCLIAgent(
        logs_dir=trial_paths.agent_dir,
        model_name="openai/gpt-5.6-sol",
        agent_kind=kind,
        binary_path=str(binary.resolve()),
        binary_sha256=file_sha256(binary),
        command_argv=command_argv,
        proxy_base_url=f"http://host.docker.internal:{health_port}/preflight/v1",
        proxy_health_url=f"http://host.docker.internal:{health_port}/healthz",
        proxy_host="host.docker.internal",
        reasoning_effort="xhigh",
        base_commit=base_commit,
        binary_bundle_root=str(bundle_root.resolve()),
        binary_bundle_manifest_path=str(bundle_manifest.resolve()),
        binary_bundle_tree_sha256=bundle_tree_sha256,
        binary_bundle_manifest_sha256=file_sha256(bundle_manifest),
        adapter_sha256=file_sha256(adapter_path),
        source_command_argv_sha256=_argv_sha256(command_argv),
        adapter_version="2.4.0",
    )
    task_environment = copy.deepcopy(task.config.environment)
    task_environment.docker_image = image
    environment = AgenticBenchmarkDockerEnvironment(
        environment_dir=task.paths.environment_dir,
        environment_name=f"agentic-preflight-{kind}-{session_suffix}",
        session_id=f"agentic-preflight-{kind}-{session_suffix}",
        trial_paths=trial_paths,
        task_env_config=task_environment,
        agent_install_spec=None,
        network_allowlist=agent.network_allowlist(),
        default_user=task.config.agent.user,
        private_proxy_port=health_port,
        egress_proxy_image=egress_proxy_image,
    )
    overlay = environment._agent_security_compose_path
    if overlay is None:
        raise RuntimeError("agent security overlay was not materialized")
    started = False
    try:
        await environment.start(force_build=False)
        started = True
        await agent.setup(environment)
        receipt = _decode_sandbox_canary_v4(
            (trial_paths.agent_dir / "sandbox-canary.json").read_bytes(),
            expected_agent_kind=kind,
            allow_pending_authority=True,
        )
        effective_raw = (trial_paths.agent_dir / "effective-argv.json").read_bytes()
        effective = json.loads(effective_raw)
        canonical_effective = json.dumps(
            effective, ensure_ascii=True, separators=(",", ":"), sort_keys=True
        ).encode("ascii")
        if hashlib.sha256(canonical_effective).hexdigest() != receipt.get(
            "effective_argv_receipt_sha256"
        ):
            raise RuntimeError("sandbox canary does not bind the effective argv")
        receipt["effective_argv_receipt"] = effective
        if kind == "luban":
            runtime = await environment.exec(
                command="command -v bwrap && bwrap --version && command -v rg && rg --version",
                cwd="/app",
                env=agent._process_env(environment),
                user=agent._execution_user(),
                timeout_sec=30,
            )
            if runtime.return_code != 0:
                raise RuntimeError("Luban runtime bwrap/rg preflight failed")
            receipt["luban_runtime_versions"] = (runtime.stdout or "").splitlines()
        receipt["overlay"] = json.loads(overlay.read_text(encoding="utf-8"))
        receipt["egress_proxy_image"] = egress_proxy_image
        return receipt
    finally:
        if started:
            await environment.stop(delete=False)


async def _main() -> None:
    args = _arguments()
    args.output_dir.mkdir(parents=True, exist_ok=True)
    AgenticBenchmarkDockerEnvironment.preflight()
    task = Task(args.task_dir)
    bundle = json.loads(args.bundle_manifest.read_text(encoding="utf-8"))
    tree_sha256 = bundle["tree_sha256"]

    server = http.server.HTTPServer(("0.0.0.0", 0), _HealthHandler)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    try:
        results = []
        for kind, binary in (
            ("codex", args.codex_binary),
            ("luban", args.luban_binary),
        ):
            results.append(
                await _preflight_agent(
                    kind=kind,
                    binary=binary,
                    task=task,
                    image=args.image,
                    bundle_root=args.bundle_root,
                    bundle_manifest=args.bundle_manifest,
                    bundle_tree_sha256=tree_sha256,
                    output_dir=args.output_dir,
                    health_port=server.server_port,
                    egress_proxy_image=args.egress_proxy_image,
                )
            )
    finally:
        server.shutdown()
        server.server_close()
        server_thread.join(timeout=5)

    verifier_paths = TrialPaths(args.output_dir / "verifier-shape")
    verifier_paths.mkdir()
    verifier_environment = copy.deepcopy(task.config.environment)
    verifier_environment.docker_image = args.image
    verifier = AgenticBenchmarkDockerEnvironment(
        environment_dir=task.paths.environment_dir,
        environment_name="agentic-preflight-verifier",
        session_id="agentic-preflight-verifier",
        trial_paths=verifier_paths,
        task_env_config=verifier_environment,
        agent_install_spec=None,
        network_allowlist=None,
        default_user=None,
    )
    if verifier._agent_security_compose_path is not None:
        raise RuntimeError("separate verifier unexpectedly received agent security opts")
    results.append(
        {
            "schema_version": "agentic-bench/no-model-preflight-v1",
            "verifier_security_overlay": False,
            "verifier_network_allowlist": [],
        }
    )
    print(json.dumps(results, indent=2, sort_keys=True))


if __name__ == "__main__":
    asyncio.run(_main())
