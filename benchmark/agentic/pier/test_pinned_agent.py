from __future__ import annotations

import copy
import hashlib
import inspect
import json
import os
import subprocess
import sys
import tempfile
import time
import unittest
import urllib.error
import urllib.request
from pathlib import Path

import benchmark.agentic.pier.pinned_agent as pinned_agent_module

from benchmark.agentic.pier.pinned_agent import (
    _BUNDLE_SCHEMA,
    _CACHE_WIRE_SCHEMA,
    _CODEX_HTTP_PROVIDER_CONFIG_TOKEN,
    _CODEX_WEB_SEARCH_CANARY_CATALOG,
    _CODEX_AGENTS_DISABLED_CONFIG,
    _CODEX_SERVICE_TIER_DEFAULT_CONFIG,
    _CODEX_SERVICE_TIER_PRIORITY_CONFIG,
    _CODEX_WEB_SEARCH_DISABLED_CONFIG,
    _EXPECTED_CODEX_PACKAGE,
    _EXPECTED_CODEX_REGISTRY_SNAPSHOT,
    _HTTP_INFERENCE_REQUIREMENT,
    _HTTP_INFERENCE_TRANSPORT,
    _SANDBOX_CANARY_V4_SCHEMA,
    _TERMINAL_EVIDENCE_PARSER_SHA256,
    BundleFile,
    PinnedCLIAgent,
    _write_adapter_terminal_evidence,
    _argv_sha256,
    _cache_evidence_sha256,
    _content_free_tool_catalog_evidence,
    _canonical_bundle_tree,
    _effective_argv_receipt,
    _effective_semantic_projection,
    _formal_source_argv_tail,
    _decode_sandbox_canary_v4,
    _strict_json_object,
    _summarize_content_free_cache_requests,
    _validate_content_free_tool_catalog_requests,
    codex_canary_server_source,
    codex_web_search_canary_server_source,
    load_bundle_manifest,
    luban_canary_server_source,
)


def write_test_bundle(directory: Path) -> tuple[Path, Path, str, str]:
    root = directory / "vendor"
    binary = root / "x86_64-unknown-linux-musl" / "bin" / "codex"
    binary.parent.mkdir(parents=True)
    binary.write_bytes(b"frozen-test-codex")
    os.chmod(binary, 0o755)
    binary_hash = hashlib.sha256(binary.read_bytes()).hexdigest()
    entry = BundleFile(
        "x86_64-unknown-linux-musl/bin/codex",
        "0755",
        binary.stat().st_size,
        binary_hash,
    )
    tree_hash = _canonical_bundle_tree((entry,))
    manifest = directory / "bundle.json"
    manifest.write_text(
        json.dumps(
            {
                "schema_version": _BUNDLE_SCHEMA,
                "package": _EXPECTED_CODEX_PACKAGE,
                "registry_snapshot": _EXPECTED_CODEX_REGISTRY_SNAPSHOT,
                "binary_path": entry.path,
                "tree_sha256": tree_hash,
                "files": [
                    {
                        "path": entry.path,
                        "mode": entry.mode,
                        "size": entry.size,
                        "sha256": entry.sha256,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )
    return manifest, root, binary_hash, tree_hash


class BundleManifestTest(unittest.TestCase):
    def test_frozen_release_manifest_has_verified_registry_and_tree_identity(self) -> None:
        manifest_path = Path(__file__).with_name(
            "codex-0.145.0-linux-x64.bundle.json"
        )
        source = json.loads(manifest_path.read_text(encoding="utf-8"))
        self.assertEqual(source["schema_version"], _BUNDLE_SCHEMA)
        self.assertEqual(source["package"], _EXPECTED_CODEX_PACKAGE)
        self.assertEqual(
            source["registry_snapshot"], _EXPECTED_CODEX_REGISTRY_SNAPSHOT
        )
        entries = tuple(
            BundleFile(
                entry["path"], entry["mode"], entry["size"], entry["sha256"]
            )
            for entry in source["files"]
        )
        self.assertEqual(len(entries), 6)
        self.assertEqual(_canonical_bundle_tree(entries), source["tree_sha256"])
        self.assertIn(
            "x86_64-unknown-linux-musl/bin/codex-code-mode-host",
            {entry.path for entry in entries},
        )

    def test_terminal_evidence_parser_is_bound_into_adapter_identity(self) -> None:
        parser = Path(__file__).with_name("terminal_evidence.py")
        self.assertEqual(
            hashlib.sha256(parser.read_bytes()).hexdigest(),
            _TERMINAL_EVIDENCE_PARSER_SHA256,
        )

    def test_complete_regular_tree_is_content_addressed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            manifest, root, binary_hash, tree_hash = write_test_bundle(
                Path(directory)
            )
            loaded = load_bundle_manifest(
                manifest,
                root,
                root / "x86_64-unknown-linux-musl/bin/codex",
                binary_hash,
                tree_hash,
            )
            self.assertEqual(loaded.tree_sha256, tree_hash)
            self.assertEqual(len(loaded.files), 1)

    def test_unexpected_file_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            manifest, root, binary_hash, tree_hash = write_test_bundle(
                Path(directory)
            )
            (root / "unexpected").write_text("surprise", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "unexpected file"):
                load_bundle_manifest(
                    manifest,
                    root,
                    root / "x86_64-unknown-linux-musl/bin/codex",
                    binary_hash,
                    tree_hash,
                )

    def test_registry_snapshot_rejects_json_type_substitution(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            manifest, root, binary_hash, tree_hash = write_test_bundle(
                Path(directory)
            )
            source = json.loads(manifest.read_text(encoding="utf-8"))
            source["registry_snapshot"]["npm_audit_signatures_verified"] = 1
            manifest.write_text(json.dumps(source), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "registry snapshot"):
                load_bundle_manifest(
                    manifest,
                    root,
                    root / "x86_64-unknown-linux-musl/bin/codex",
                    binary_hash,
                    tree_hash,
                )

    def test_symlink_substitution_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            manifest, root, binary_hash, tree_hash = write_test_bundle(
                Path(directory)
            )
            binary = root / "x86_64-unknown-linux-musl/bin/codex"
            target = Path(directory) / "replacement"
            target.write_bytes(binary.read_bytes())
            os.chmod(target, 0o755)
            binary.unlink()
            binary.symlink_to(target)
            with self.assertRaisesRegex(ValueError, "non-regular"):
                load_bundle_manifest(
                    manifest, root, binary, binary_hash, tree_hash
                )


class AdapterTerminalEvidenceFallbackTest(unittest.TestCase):
    def test_nonzero_codex_without_public_error_code_defers_to_host_seal(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stream = root / "stream.jsonl"
            exit_receipt = root / "exit.json"
            destination = root / "terminal-evidence.json"
            stream.write_text(
                json.dumps(
                    {
                        "type": "turn.failed",
                        "error": {"message": "diagnostic text is not authority"},
                    }
                )
                + "\n",
                encoding="utf-8",
            )
            exit_receipt.write_text('{"exit_code":1}\n', encoding="utf-8")

            self.assertFalse(
                _write_adapter_terminal_evidence(
                    "codex", stream, exit_receipt, destination, 1
                )
            )
            self.assertFalse(destination.exists())

    def test_deferral_is_not_available_to_luban_or_a_zero_codex_exit(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stream = root / "stream.jsonl"
            exit_receipt = root / "exit.json"
            destination = root / "terminal-evidence.json"
            stream.write_text(
                '{"type":"turn.failed","error":{"message":"opaque"}}\n',
                encoding="utf-8",
            )
            exit_receipt.write_text('{"exit_code":1}\n', encoding="utf-8")

            for agent_kind, exit_code in (("luban", 1), ("codex", 0)):
                with self.subTest(agent_kind=agent_kind, exit_code=exit_code):
                    with self.assertRaises(
                        pinned_agent_module.terminal_evidence.TerminalEvidenceProtocolError
                    ):
                        _write_adapter_terminal_evidence(
                            agent_kind,
                            stream,
                            exit_receipt,
                            destination,
                            exit_code,
                        )


class SandboxCanaryContractTest(unittest.TestCase):
    def test_codex_canary_argv_is_pinned_to_custom_http_provider(self) -> None:
        agent = object.__new__(PinnedCLIAgent)
        agent._remote_binary = "/opt/agentic-bench/vendor/x/bin/codex"
        positive = agent._codex_canary_argv(43123, "workspace-write")
        negative = agent._codex_canary_argv(43123, "danger-full-access")
        self.assertEqual(positive[positive.index("--sandbox") + 1], "workspace-write")
        self.assertEqual(
            negative[negative.index("--sandbox") + 1], "danger-full-access"
        )
        for argv in (positive, negative):
            self.assertIn("--ask-for-approval", argv)
            self.assertIn("never", argv)
            self.assertIn("--json", argv)
            self.assertIn("--ephemeral", argv)
            self.assertIn("--ignore-user-config", argv)
            self.assertIn("gpt-5.6-sol", argv)
            self.assertIn("model_reasoning_effort=xhigh", argv)
            self.assertEqual(argv.count(_CODEX_SERVICE_TIER_DEFAULT_CONFIG), 1)
            self.assertIn(_CODEX_WEB_SEARCH_DISABLED_CONFIG, argv)
            self.assertIn(_CODEX_AGENTS_DISABLED_CONFIG, argv)
            self.assertIn(
                'model_provider="agentic_http"',
                argv,
            )
            self.assertIn(
                _CODEX_HTTP_PROVIDER_CONFIG_TOKEN.replace(
                    "{provider_base_url}", "http://127.0.0.1:43123/v1"
                ),
                argv,
            )
            self.assertIn(
                "model_providers.agentic_http.request_max_retries=0", argv
            )
            self.assertIn(
                "model_providers.agentic_http.stream_max_retries=0", argv
            )
            self.assertNotIn("--responses-websocket", argv)
            self.assertNotIn("disable_response_storage=true", argv)

    def test_codex_fake_responses_is_content_free_and_exercises_exec(self) -> None:
        source = codex_canary_server_source()
        compile(source, "<codex-canary>", "exec")
        expected_tools = [
            {"type": "custom", "name": "exec"},
            {"type": "function", "name": "wait"},
            {"type": "function", "name": "request_user_input"},
        ]
        secret = "SECRET-PROMPT-MUST-NOT-ENTER-AUDIT"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ready = root / "port"
            audit = root / "audit.jsonl"
            process = subprocess.Popen(
                [
                    sys.executable,
                    "-c",
                    source,
                    "printf canary",
                    "0",
                    str(ready),
                    str(audit),
                ],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            try:
                deadline = time.monotonic() + 5
                while (
                    (not ready.exists() or ready.stat().st_size == 0)
                    and time.monotonic() < deadline
                ):
                    time.sleep(0.01)
                self.assertTrue(ready.exists() and ready.stat().st_size > 0)
                endpoint = f"http://127.0.0.1:{ready.read_text()}/v1/responses"
                prefix = {
                    "type": "additional_tools",
                    "role": "developer",
                    "tools": expected_tools,
                }
                common = {
                    "model": "gpt-5.6-sol",
                    "reasoning": {"effort": "xhigh", "context": "all_turns"},
                    "store": False,
                    "include": ["reasoning.encrypted_content"],
                    "stream": True,
                    "prompt_cache_key": "stable-secret-cache-key",
                }
                headers = {
                    "Authorization": "Bearer SECRET-CREDENTIAL",
                    "Originator": "codex_exec",
                    "x-openai-internal-codex-responses-lite": "true",
                    "Content-Type": "application/json",
                }
                first = {
                    **common,
                    "input": [
                        prefix,
                        {
                            "type": "message",
                            "role": "user",
                            "content": [{"type": "input_text", "text": secret}],
                        },
                    ],
                }
                response = urllib.request.urlopen(
                    urllib.request.Request(
                        endpoint,
                        data=json.dumps(first).encode(),
                        headers=headers,
                        method="POST",
                    ),
                    timeout=5,
                ).read()
                self.assertIn(b'"type":"custom_tool_call"', response)
                self.assertIn(b'"name":"exec"', response)
                self.assertIn(b"tools.exec_command", response)
                self.assertIn(b'"cache_write_tokens":2', response)
                self.assertIn(b'"service_tier":"default"', response)

                second = {
                    **common,
                    "input": [
                        prefix,
                        {
                            "type": "custom_tool_call_output",
                            "call_id": "call_canary",
                            "output": [
                                {
                                    "type": "input_text",
                                    "text": '{"exit_code":0,"output":"' + secret + '"}',
                                }
                            ],
                        },
                    ],
                }
                urllib.request.urlopen(
                    urllib.request.Request(
                        endpoint,
                        data=json.dumps(second).encode(),
                        headers=headers,
                        method="POST",
                    ),
                    timeout=5,
                ).read()
                self.assertEqual(process.wait(timeout=5), 0)
                raw_audit = audit.read_text(encoding="utf-8")
                self.assertNotIn(secret, raw_audit)
                self.assertNotIn("SECRET-CREDENTIAL", raw_audit)
                entries = [json.loads(line) for line in raw_audit.splitlines()]
                self.assertEqual(len(entries), 2)
                self.assertEqual(entries[0]["tool_catalog"], expected_tools)
                self.assertIs(entries[0]["store"], False)
                self.assertEqual(entries[0]["reasoning_effort"], "xhigh")
                self.assertFalse(entries[0]["request_service_tier_present"])
                self.assertIsNone(entries[0]["request_service_tier"])
                self.assertEqual(
                    entries[0]["request_service_tier_canonical"], "default"
                )
                self.assertEqual(
                    entries[0]["request_service_tier_source"],
                    "client_canonicalized_default",
                )
                self.assertFalse(entries[0]["web_search_tool_present"])
                self.assertEqual(entries[0]["web_search_tool_count"], 0)
                self.assertFalse(entries[0]["collaboration_namespace_present"])
                self.assertFalse(entries[0]["subagent_tool_present"])
                self.assertTrue(entries[0]["exec_cell_wait_present"])
                self.assertEqual(entries[0]["transport"], "http_sse")
                self.assertFalse(entries[0]["prewarm_requested"])
                self.assertEqual(entries[0]["websocket_upgrade_count_before_request"], 0)
                self.assertFalse(entries[0]["websocket_upgrade_header_present"])
                self.assertFalse(entries[0]["websocket_key_header_present"])
                cache_summary = _summarize_content_free_cache_requests(entries)
                self.assertEqual(cache_summary["schema_version"], _CACHE_WIRE_SCHEMA)
                self.assertTrue(cache_summary["stable"])
                self.assertNotIn("stable-secret-cache-key", json.dumps(entries))
                catalog = _validate_content_free_tool_catalog_requests(
                    entries,
                    [
                        ("custom", "exec"),
                        ("function", "wait"),
                        ("function", "request_user_input"),
                    ],
                )
                self.assertEqual(
                    catalog["tool_catalog_canonical_bytes"],
                    sum(
                        tool["definition_bytes"]
                        for tool in catalog["tool_definitions"]
                    ),
                )
                self.assertEqual(
                    entries[0]["response_usage"],
                    {
                        "input_tokens": 11,
                        "cached_input_tokens": 3,
                        "cache_write_input_tokens": 2,
                        "output_tokens": 5,
                        "reasoning_output_tokens": 1,
                    },
                )
                self.assertTrue(entries[0]["response_request_id_present"])
                self.assertEqual(entries[0]["response_model"], "gpt-5.6-sol")
                self.assertEqual(entries[0]["response_service_tier"], "default")
                self.assertEqual(
                    entries[0]["response_service_tier_canonical"], "default"
                )
                self.assertEqual(entries[1]["tool_output_exit_code"], 0)
                self.assertTrue(entries[0]["authorization_header_present"])
                self.assertTrue(entries[0]["responses_lite_header_present"])
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait(timeout=5)
                if process.stderr is not None:
                    process.stderr.close()

    def test_codex_web_search_config_has_wire_visible_counterfactual(self) -> None:
        source = codex_web_search_canary_server_source()
        compile(source, "<codex-web-search-canary>", "exec")
        self.assertFalse(
            _CODEX_WEB_SEARCH_CANARY_CATALOG["models"][0]["use_responses_lite"]
        )
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            ready = root / "port"
            audit = root / "audit.jsonl"
            process = subprocess.Popen(
                [sys.executable, "-c", source, str(ready), str(audit)],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
                text=True,
            )
            try:
                deadline = time.monotonic() + 5
                while (
                    (not ready.exists() or ready.stat().st_size == 0)
                    and time.monotonic() < deadline
                ):
                    time.sleep(0.01)
                self.assertTrue(ready.exists() and ready.stat().st_size > 0)
                endpoint = f"http://127.0.0.1:{ready.read_text()}/v1/responses"
                headers = {
                    "Authorization": "Bearer SECRET-CREDENTIAL",
                    "Originator": "codex_exec",
                    "Content-Type": "application/json",
                }
                common = {
                    "model": "gpt-5.6-sol",
                    "reasoning": {"effort": "xhigh"},
                    "store": False,
                    "include": ["reasoning.encrypted_content"],
                    "stream": True,
                    "input": [
                        {
                            "type": "message",
                            "role": "user",
                            "content": [
                                {"type": "input_text", "text": "SECRET-PROMPT"}
                            ],
                        }
                    ],
                }
                positive = {
                    **common,
                    "tools": [
                        {"type": "function", "name": "shell_command"}
                    ],
                }
                response = None
                for _attempt in range(20):
                    try:
                        response = urllib.request.urlopen(
                            urllib.request.Request(
                                endpoint,
                                data=json.dumps(positive).encode(),
                                headers=headers,
                                method="POST",
                            ),
                            timeout=5,
                        )
                        break
                    except urllib.error.URLError:
                        if process.poll() is not None:
                            raise
                        time.sleep(0.01)
                self.assertIsNotNone(response)
                assert response is not None
                self.assertEqual(response.headers["openai-model"], "gpt-5.6-sol")
                self.assertEqual(
                    response.headers["x-request-id"],
                    "req_agentic_web_search_canary_0",
                )
                response.read()

                negative = {
                    **common,
                    "tools": [
                        {"type": "function", "name": "shell_command"},
                        {
                            "type": "web_search",
                            "external_web_access": False,
                        },
                    ],
                }
                with self.assertRaises(urllib.error.HTTPError) as rejected:
                    urllib.request.urlopen(
                        urllib.request.Request(
                            endpoint,
                            data=json.dumps(negative).encode(),
                            headers=headers,
                            method="POST",
                        ),
                        timeout=5,
                    )
                self.assertEqual(rejected.exception.code, 422)

                agents_negative = {
                    **common,
                    "tools": [
                        {"type": "function", "name": "shell_command"},
                        {"type": "namespace", "name": "multi_agent_v1"},
                    ],
                }
                with self.assertRaises(urllib.error.HTTPError) as rejected_agents:
                    urllib.request.urlopen(
                        urllib.request.Request(
                            endpoint,
                            data=json.dumps(agents_negative).encode(),
                            headers=headers,
                            method="POST",
                        ),
                        timeout=5,
                    )
                self.assertEqual(rejected_agents.exception.code, 422)

                service_negative = {
                    **positive,
                    "service_tier": "priority",
                }
                with self.assertRaises(urllib.error.HTTPError) as rejected_service:
                    urllib.request.urlopen(
                        urllib.request.Request(
                            endpoint,
                            data=json.dumps(service_negative).encode(),
                            headers=headers,
                            method="POST",
                        ),
                        timeout=5,
                    )
                self.assertEqual(rejected_service.exception.code, 422)
                self.assertEqual(process.wait(timeout=5), 0)
                raw_audit = audit.read_text(encoding="utf-8")
                self.assertNotIn("SECRET-PROMPT", raw_audit)
                self.assertNotIn("SECRET-CREDENTIAL", raw_audit)
                entries = [json.loads(line) for line in raw_audit.splitlines()]
                self.assertEqual(entries[0]["web_search_tool_count"], 0)
                self.assertTrue(entries[0]["configuration_accepted"])
                for entry in entries[:3]:
                    self.assertFalse(entry["request_service_tier_present"])
                    self.assertIsNone(entry["request_service_tier"])
                    self.assertEqual(
                        entry["request_service_tier_canonical"], "default"
                    )
                    self.assertEqual(
                        entry["request_service_tier_source"],
                        "client_canonicalized_default",
                    )
                    self.assertEqual(entry["response_service_tier"], "default")
                    self.assertEqual(
                        entry["response_service_tier_canonical"], "default"
                    )
                self.assertEqual(entries[1]["web_search_tool_count"], 1)
                self.assertEqual(
                    entries[1]["web_search_external_access"], [False]
                )
                self.assertFalse(entries[1]["configuration_accepted"])
                self.assertFalse(entries[1]["subagent_tool_present"])
                self.assertEqual(entries[2]["web_search_tool_count"], 0)
                self.assertFalse(entries[2]["collaboration_namespace_present"])
                self.assertTrue(entries[2]["multi_agent_namespace_present"])
                self.assertTrue(entries[2]["subagent_tool_present"])
                self.assertFalse(entries[2]["configuration_accepted"])
                self.assertTrue(entries[3]["request_service_tier_present"])
                self.assertEqual(entries[3]["request_service_tier"], "priority")
                self.assertEqual(
                    entries[3]["request_service_tier_canonical"], "priority"
                )
                self.assertEqual(
                    entries[3]["request_service_tier_source"], "wire_explicit"
                )
                self.assertFalse(entries[3]["configuration_accepted"])
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait(timeout=5)
                if process.stderr is not None:
                    process.stderr.close()

    def test_codex_web_search_negative_argv_only_removes_disabled_config(self) -> None:
        agent = object.__new__(PinnedCLIAgent)
        agent._remote_binary = "/opt/agentic-bench/vendor/x/bin/codex"
        positive = agent._codex_web_search_canary_argv(
            43123,
            web_search_disabled=True,
            agents_disabled=True,
            service_tier="default",
        )
        negative = agent._codex_web_search_canary_argv(
            43123,
            web_search_disabled=False,
            agents_disabled=True,
            service_tier="default",
        )
        agents_negative = agent._codex_web_search_canary_argv(
            43123,
            web_search_disabled=True,
            agents_disabled=False,
            service_tier="default",
        )
        service_negative = agent._codex_web_search_canary_argv(
            43123,
            web_search_disabled=True,
            agents_disabled=True,
            service_tier="priority",
        )
        self.assertIn(_CODEX_WEB_SEARCH_DISABLED_CONFIG, positive)
        self.assertNotIn(_CODEX_WEB_SEARCH_DISABLED_CONFIG, negative)
        self.assertIn(_CODEX_AGENTS_DISABLED_CONFIG, positive)
        self.assertNotIn(_CODEX_AGENTS_DISABLED_CONFIG, agents_negative)
        for argv in (positive, negative, agents_negative):
            self.assertEqual(argv.count(_CODEX_SERVICE_TIER_DEFAULT_CONFIG), 1)
        self.assertNotIn(_CODEX_SERVICE_TIER_DEFAULT_CONFIG, service_negative)
        self.assertEqual(
            service_negative.count(_CODEX_SERVICE_TIER_PRIORITY_CONFIG), 1
        )
        masked = list(positive)
        index = masked.index(_CODEX_WEB_SEARCH_DISABLED_CONFIG)
        self.assertEqual(masked[index - 1], "--config")
        del masked[index - 1 : index + 1]
        self.assertEqual(masked, negative)
        agents_masked = list(positive)
        agents_index = agents_masked.index(_CODEX_AGENTS_DISABLED_CONFIG)
        self.assertEqual(agents_masked[agents_index - 1], "--config")
        del agents_masked[agents_index - 1 : agents_index + 1]
        self.assertEqual(agents_masked, agents_negative)
        service_masked = list(positive)
        service_index = service_masked.index(_CODEX_SERVICE_TIER_DEFAULT_CONFIG)
        self.assertEqual(service_masked[service_index - 1], "--config")
        service_masked[service_index] = _CODEX_SERVICE_TIER_PRIORITY_CONFIG
        self.assertEqual(service_masked, service_negative)
        self.assertIn(
            'model_catalog_json="/tmp/agentic-bench-codex-web-search-models.json"',
            positive,
        )

    def test_codex_negative_control_disables_sandbox_and_cannot_emit_marker(self) -> None:
        source = inspect.getsource(PinnedCLIAgent._require_codex_exec_sandbox)
        self.assertIn('sandbox_mode="danger-full-access"', source)
        self.assertIn("expected_exit=91", source)
        self.assertIn('"valid_sandbox_receipt_emitted": False', source)
        self.assertIn('"test ! -e "', source)

    def test_luban_fake_provider_source_enforces_agentic_v2_run(self) -> None:
        source = luban_canary_server_source()
        compile(source, "<luban-canary>", "exec")
        self.assertIn("expected_tools=['Inspect','ApplyPatch','Run']", source)
        self.assertIn("item.get('type') == 'additional_tools'", source)
        self.assertIn("x-openai-internal-codex-responses-lite", source)
        self.assertIn("'name':'Run'", source)
        self.assertNotIn("'name':'Bash'", source)
        self.assertIn("'name':'ApplyPatch'", source)
        self.assertIn("'requires_patch_commit':True", source)
        self.assertIn("'call_id':'call_verify'", source)
        self.assertIn("'argv':['git','diff','--check']", source)
        self.assertIn("Handler.request_index != 3", source)
        self.assertIn('"--max-turns",\n                "3",', inspect.getsource(PinnedCLIAgent._require_luban_tool_sandbox))
        self.assertIn("request.get('model') != 'gpt-5.6-sol'", source)
        self.assertIn("reasoning.get('effort') != 'xhigh'", source)
        self.assertIn("reasoning.get('context') not in (None,'all_turns')", source)
        self.assertIn('"--service-tier",', inspect.getsource(PinnedCLIAgent._require_luban_tool_sandbox))
        self.assertIn("request.get('store') is not False", source)
        self.assertIn("request.get('service_tier') != 'default'", source)
        self.assertIn("'response_service_tier':'default'", source)
        self.assertIn("['reasoning.encrypted_content']", source)
        self.assertIn("'transport':'http_sse'", source)
        self.assertIn("Handler.websocket_upgrade_count != 0", source)
        self.assertIn("'cache_policy':cache_policy(request)", source)
        self.assertIn("tool_catalog_semantic_sha256", source)

    def test_luban_fake_provider_accepts_the_manifest_tool_order(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            ready = root / "ready"
            audit = root / "audit.jsonl"
            process = subprocess.Popen(
                [
                    sys.executable,
                    "-c",
                    luban_canary_server_source(),
                    "true",
                    str(ready),
                    str(audit),
                ],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            try:
                deadline = time.monotonic() + 5
                while not ready.exists() and time.monotonic() < deadline:
                    if process.poll() is not None:
                        self.fail("fake provider exited before publishing its port")
                    time.sleep(0.01)
                self.assertTrue(ready.exists(), "fake provider did not publish its port")
                tools = [
                    {
                        "type": "function",
                        "name": name,
                        "description": "content-free fixture",
                        "strict": True,
                        "parameters": {
                            "type": "object",
                            "properties": {},
                            "additionalProperties": False,
                        },
                    }
                    for name in ("Inspect", "ApplyPatch", "Run")
                ]
                body = json.dumps(
                    {
                        "model": "gpt-5.6-sol",
                        "reasoning": {"effort": "xhigh"},
                        "store": False,
                        "service_tier": "default",
                        "include": ["reasoning.encrypted_content"],
                        "input": [],
                        "tools": tools,
                    }
                ).encode()
                request = urllib.request.Request(
                    f"http://127.0.0.1:{ready.read_text(encoding='ascii')}/v1/responses",
                    data=body,
                    headers={"Content-Type": "application/json"},
                    method="POST",
                )
                with urllib.request.urlopen(request, timeout=5) as response:
                    self.assertEqual(response.status, 200)
                    self.assertEqual(response.headers.get_content_type(), "text/event-stream")
                entry = json.loads(audit.read_text(encoding="utf-8").splitlines()[0])
                self.assertEqual(
                    entry["tool_names"], ["Inspect", "ApplyPatch", "Run"]
                )
            finally:
                process.terminate()
                try:
                    process.wait(timeout=2)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=2)

    def test_luban_canary_does_not_narrow_the_visible_catalog(self) -> None:
        source = inspect.getsource(PinnedCLIAgent._require_luban_tool_sandbox)
        self.assertIn('"--allow-all"', source)
        self.assertNotIn('"--allowed-tools"', source)

    def test_luban_benchmark_uses_default_coding_surface(self) -> None:
        class FakeEnvironment:
            @staticmethod
            def agent_process_env(environment: dict[str, str]) -> dict[str, str]:
                return environment

        agent = object.__new__(PinnedCLIAgent)
        agent._agent_kind = "luban"
        agent._proxy_base_url = "http://host.docker.internal:43123/v1"
        agent._reasoning_effort = "xhigh"
        agent._extra_env = {}
        environment = agent._process_env(FakeEnvironment())
        self.assertNotIn("LUBAN_CODE_EXPERIMENTAL_AGENTIC_V2", environment)


class SandboxCanaryV4ReceiptTest(unittest.TestCase):
    _RAW_CACHE_KEY = "never-retain-this-cache-key"
    _RAW_BREAKPOINT = "/input/0/prompt_cache_breakpoint"

    @classmethod
    def cache_policy(cls) -> dict[str, object]:
        return {
            "observed": True,
            "shape_valid": True,
            "prompt_cache_key_present": True,
            "prompt_cache_key_sha256": _cache_evidence_sha256(
                "prompt-cache-key", cls._RAW_CACHE_KEY
            ),
            "prompt_cache_options_present": True,
            "prompt_cache_options_mode": "automatic",
            "prompt_cache_options_ttl_present": True,
            "prompt_cache_options_ttl": "1h",
            "prompt_cache_options_ttl_seconds": 3600,
            "prompt_cache_retention_present": False,
            "prompt_cache_retention": "",
            "prompt_cache_breakpoint_count": 1,
            "prompt_cache_breakpoint_position_hashes": [
                _cache_evidence_sha256(
                    "prompt-cache-breakpoint-position", cls._RAW_BREAKPOINT
                )
            ],
        }

    @classmethod
    def receipt(cls, kind: str, *, authority: str = "verified_formal") -> dict:
        identities = (
            [("custom", "exec"), ("function", "wait"), ("function", "request_user_input")]
            if kind == "codex"
            else [("function", "Inspect"), ("function", "ApplyPatch"), ("function", "Run")]
        )
        tool_evidence = _content_free_tool_catalog_evidence(
            [
                {
                    "type": tool_type,
                    "name": name,
                    "description": "content-free fixture " + name,
                    "parameters": {
                        "type": "object",
                        "properties": {"value": {"type": "string"}},
                    },
                    "strict": True,
                }
                for tool_type, name in identities
            ]
        )
        request_count = 2 if kind == "codex" else 3
        requests = [
            {
                "request_index": index,
                "transport": "http_sse",
                "prewarm_requested": False,
                "websocket_upgrade_count_before_request": 0,
                "cache_policy": cls.cache_policy(),
                **copy.deepcopy(tool_evidence),
            }
            for index in range(request_count)
        ]
        receipt = {
            "schema_version": _SANDBOX_CANARY_V4_SCHEMA,
            "agent_kind": kind,
            "binary_sha256": "a" * 64,
            "base_commit": "b" * 40,
            "controller_proxy_reachable": True,
            "tool_proxy_reachable": False,
            "credential_in_agent": False,
            "adapter_sha256": "c" * 64,
            "bundle_manifest_sha256": "d" * 64,
            "effective_argv_receipt_sha256": "e" * 64,
            "source_bundle_tree_sha256": "f" * 64,
            "runtime_payload_tree_sha256": "0" * 64,
            "provider_canary_requests": requests,
            "provider_canary_transport": _HTTP_INFERENCE_TRANSPORT,
            "http_transport": {
                "schema_version": "agentic-bench/http-inference-transport-v1",
                "requirement": _HTTP_INFERENCE_REQUIREMENT,
                "http_inference_request_count": request_count,
                "websocket_upgrade_request_count": 0,
                "websocket_generation_frame_count": 0,
                "prewarm_request_count": 0,
            },
            "cache_wire": _summarize_content_free_cache_requests(requests),
        }
        if kind == "codex":
            receipt.update(
                {
                    "canonical_authority": {
                        "generation": "v8",
                        "authority_scope": authority,
                        "responses_transport_requirement": _HTTP_INFERENCE_REQUIREMENT,
                    },
                    "sandbox_negative_control": {},
                    "web_search_configuration_canary": {},
                    "workspace_state": {},
                }
            )
        return receipt

    @staticmethod
    def encode(receipt: dict) -> str:
        return json.dumps(receipt, ensure_ascii=True, separators=(",", ":"), sort_keys=True)

    def test_domain_separated_cache_evidence_is_content_free_and_stable(self) -> None:
        requests = [
            {"cache_policy": self.cache_policy()},
            {"cache_policy": self.cache_policy()},
        ]
        summary = _summarize_content_free_cache_requests(requests)
        expected_key_hash = hashlib.sha256(
            b"agentic-bench/cache-evidence/v1\0prompt-cache-key\0"
            + self._RAW_CACHE_KEY.encode()
        ).hexdigest()
        self.assertEqual(summary["first_key_sha256"], expected_key_hash)
        self.assertTrue(summary["stable"])
        encoded = self.encode(summary)
        self.assertNotIn(self._RAW_CACHE_KEY, encoded)
        self.assertNotIn(self._RAW_BREAKPOINT, encoded)

    def test_tool_catalog_evidence_matches_wire_order_without_raw_schema(self) -> None:
        raw_description = "do <private> & deterministic work\u2028"
        definitions = [
            {
                "type": "function",
                "name": "Inspect",
                "description": raw_description,
                "strict": True,
                "parameters": {
                    "type": "object",
                    "properties": {"secret_field": {"type": "string"}},
                },
            },
            {"type": "function", "name": "ApplyPatch", "parameters": {}},
            {"type": "function", "name": "Run", "parameters": {}},
        ]
        evidence = _content_free_tool_catalog_evidence(definitions)
        encoded = self.encode(evidence)
        self.assertNotIn(raw_description, encoded)
        self.assertNotIn("secret_field", encoded)
        self.assertEqual(
            [tool["name"] for tool in evidence["tool_definitions"]],
            ["Inspect", "ApplyPatch", "Run"],
        )
        self.assertEqual(
            _validate_content_free_tool_catalog_requests(
                [evidence, evidence],
                [
                    ("function", "Inspect"),
                    ("function", "ApplyPatch"),
                    ("function", "Run"),
                ],
            ),
            evidence,
        )

    def test_strict_json_rejects_duplicate_keys_and_trailing_values(self) -> None:
        with self.assertRaisesRegex(ValueError, "duplicate JSON key"):
            _strict_json_object('{"a":1,"a":2}', "fixture")
        with self.assertRaisesRegex(ValueError, "trailing JSON"):
            _strict_json_object('{"a":1} {}', "fixture")

    def test_both_http_receipts_decode_and_codex_pending_is_not_production(self) -> None:
        for kind in ("codex", "luban"):
            with self.subTest(kind=kind):
                receipt = self.receipt(kind)
                decoded = _decode_sandbox_canary_v4(
                    self.encode(receipt), expected_agent_kind=kind
                )
                self.assertEqual(decoded, receipt)

        pending = self.receipt("codex", authority="pending_repin")
        with self.assertRaisesRegex(ValueError, "authority is not formal"):
            _decode_sandbox_canary_v4(
                self.encode(pending), expected_agent_kind="codex"
            )
        self.assertEqual(
            _decode_sandbox_canary_v4(
                self.encode(pending),
                expected_agent_kind="codex",
                allow_pending_authority=True,
            ),
            pending,
        )

    def test_v4_decoder_rejects_transport_cache_and_shape_drift(self) -> None:
        mutations = {
            "unknown top-level": lambda receipt: receipt.__setitem__("extra", True),
            "missing top-level": lambda receipt: receipt.pop("cache_wire"),
            "wrong agent": lambda receipt: receipt.__setitem__("agent_kind", "codex"),
            "bad base": lambda receipt: receipt.__setitem__("base_commit", "B" * 40),
            "one request": lambda receipt: receipt["provider_canary_requests"].pop(),
            "WebSocket request": lambda receipt: receipt["provider_canary_requests"][0].__setitem__(
                "websocket_upgrade_count_before_request", 1
            ),
            "prewarm request": lambda receipt: receipt["provider_canary_requests"][0].__setitem__(
                "prewarm_requested", True
            ),
            "transport summary": lambda receipt: receipt["http_transport"].__setitem__(
                "websocket_generation_frame_count", 1
            ),
            "raw cache key": lambda receipt: receipt["provider_canary_requests"][0][
                "cache_policy"
            ].__setitem__("raw_prompt_cache_key", self._RAW_CACHE_KEY),
            "tool definition hash": lambda receipt: receipt[
                "provider_canary_requests"
            ][0]["tool_definitions"][0].__setitem__("definition_sha256", "9" * 64),
            "tool semantic hash": lambda receipt: receipt[
                "provider_canary_requests"
            ][0].__setitem__("tool_catalog_semantic_sha256", "8" * 64),
            "tool canonical bytes": lambda receipt: receipt[
                "provider_canary_requests"
            ][0].__setitem__("tool_catalog_canonical_bytes", 1),
        }
        for name, mutate in mutations.items():
            with self.subTest(name=name):
                receipt = self.receipt("luban")
                mutate(receipt)
                with self.assertRaises(ValueError):
                    _decode_sandbox_canary_v4(
                        self.encode(receipt), expected_agent_kind="luban"
                    )


class EffectiveArgvContractTest(unittest.TestCase):
    _PROXY = "http://host.docker.internal:43123/unguessable-run/v1"

    @staticmethod
    def agent(kind: str) -> PinnedCLIAgent:
        agent = object.__new__(PinnedCLIAgent)
        agent._agent_kind = kind
        agent._remote_binary = (
            "/opt/agentic-bench/vendor/x86_64-unknown-linux-musl/bin/codex"
            if kind == "codex"
            else "/opt/agentic-bench/agent"
        )
        agent._command_argv = ["/host/frozen-agent", *_formal_source_argv_tail(kind)]
        agent._proxy_base_url = EffectiveArgvContractTest._PROXY
        return agent

    def test_codex_receipt_binds_actual_argv_without_leaking_proxy_authority(self) -> None:
        agent = self.agent("codex")
        actual = agent._resolved_argv()
        receipt = _effective_argv_receipt(
            agent_kind="codex",
            argv=actual,
            proxy_base_url=self._PROXY,
            adapter_version="2.4.0",
            adapter_sha256="a" * 64,
            source_command_argv_sha256=_argv_sha256(agent._command_argv),
            bundle_manifest_sha256="b" * 64,
            bundle_tree_sha256="c" * 64,
        )
        raw = json.dumps(receipt, sort_keys=True, separators=(",", ":"))
        self.assertNotIn(self._PROXY, raw)
        self.assertIn(_CODEX_HTTP_PROVIDER_CONFIG_TOKEN, receipt["effective_argv"])
        self.assertNotEqual(
            receipt["effective_argv_sha256"], receipt["execution_argv_sha256"]
        )
        self.assertEqual(receipt["semantic_projection"]["model"], "gpt-5.6-sol")
        self.assertEqual(receipt["semantic_projection"]["reasoning_effort"], "xhigh")
        self.assertEqual(receipt["semantic_projection"]["service_tier"], "default")
        self.assertIs(receipt["semantic_projection"]["agents_enabled"], False)
        self.assertIs(receipt["semantic_projection"]["response_storage"], False)
        self.assertIs(receipt["semantic_projection"]["web_search"], False)
        self.assertEqual(
            receipt["semantic_projection"]["responses_transport_requirement"],
            _HTTP_INFERENCE_REQUIREMENT,
        )
        self.assertEqual(
            receipt["semantic_projection"]["provider_transport"], "responses-http"
        )
        self.assertEqual(
            receipt["semantic_projection"]["responses_api_profile"], "codex_lite"
        )
        self.assertIs(receipt["semantic_projection"]["responses_lite"], True)
        self.assertIn("--cd", actual)
        self.assertIn("/app", actual)
        self.assertEqual(actual.count('web_search="disabled"'), 1)
        self.assertEqual(actual.count(_CODEX_AGENTS_DISABLED_CONFIG), 1)
        self.assertEqual(actual.count(_CODEX_SERVICE_TIER_DEFAULT_CONFIG), 1)

    def test_codex_source_drift_is_rejected_before_execution(self) -> None:
        mutations = {
            "search": lambda argv: argv.insert(1, "--search"),
            "sandbox": lambda argv: argv.__setitem__(argv.index("workspace-write"), "danger-full-access"),
            "approval": lambda argv: argv.__setitem__(argv.index("never"), "on-request"),
            "model": lambda argv: argv.__setitem__(argv.index("gpt-5.6-sol"), "gpt-5.6"),
            "effort": lambda argv: argv.__setitem__(argv.index("model_reasoning_effort=xhigh"), "model_reasoning_effort=high"),
            "service_tier": lambda argv: argv.__setitem__(
                argv.index(_CODEX_SERVICE_TIER_DEFAULT_CONFIG), 'service_tier="auto"'
            ),
            "web_search": lambda argv: argv.__delitem__(
                slice(
                    argv.index(_CODEX_WEB_SEARCH_DISABLED_CONFIG) - 1,
                    argv.index(_CODEX_WEB_SEARCH_DISABLED_CONFIG) + 1,
                )
            ),
            "agents": lambda argv: argv.__setitem__(
                argv.index(_CODEX_AGENTS_DISABLED_CONFIG), "agents.enabled=true"
            ),
            "store": lambda argv: argv.extend(["--config", "disable_response_storage=false"]),
        }
        for name, mutate in mutations.items():
            with self.subTest(name=name):
                agent = self.agent("codex")
                mutate(agent._command_argv)
                with self.assertRaisesRegex(RuntimeError, "source command changed"):
                    agent._resolved_argv()

    def test_luban_fallback_and_sandbox_drift_are_rejected(self) -> None:
        for value in (
            "--service-tier",
            "--no-model-fallback",
            "--pinned-model",
            "--force-sandbox-tools",
        ):
            with self.subTest(value=value):
                agent = self.agent("luban")
                agent._command_argv.remove(value)
                with self.assertRaisesRegex(RuntimeError, "source command changed"):
                    agent._resolved_argv()

    def test_luban_effective_projection_requires_default_service_tier(self) -> None:
        actual = self.agent("luban")._resolved_argv()
        projection = _effective_semantic_projection("luban", actual)
        self.assertEqual(projection["service_tier"], "default")
        self.assertEqual(projection["provider_transport"], "responses-http")
        self.assertEqual(projection["responses_api_profile"], "openai_public")
        self.assertIs(projection["responses_lite"], False)
        self.assertEqual(
            projection["responses_transport_requirement"],
            _HTTP_INFERENCE_REQUIREMENT,
        )

        actual[actual.index("default")] = "auto"
        with self.assertRaisesRegex(ValueError, "service tier is not default"):
            _effective_semantic_projection("luban", actual)

    def test_luban_websocket_inference_flag_is_rejected(self) -> None:
        actual = self.agent("luban")._resolved_argv()
        actual.insert(1, "--responses-websocket")
        with self.assertRaisesRegex(ValueError, "WebSocket inference"):
            _effective_semantic_projection("luban", actual)

    def test_version_contract_default_is_2_4_0(self) -> None:
        self.assertEqual(
            inspect.signature(PinnedCLIAgent.__init__).parameters[
                "adapter_version"
            ].default,
            "2.4.0",
        )


class SubmissionCaptureContractTest(unittest.TestCase):
    def test_capture_is_audit_only_and_never_commits_agent_work(self) -> None:
        agent = object.__new__(PinnedCLIAgent)
        agent._base_commit = "a" * 40
        command = agent._capture_command()
        self.assertIn("git diff --binary", command)
        self.assertIn("committed-workspace.patch", command)
        self.assertIn("full-workspace.patch", command)
        self.assertIn("GIT_INDEX_FILE", command)
        self.assertIn("git add -A -- .", command)
        self.assertNotIn("agentic-bench-submission", command)
        self.assertNotIn("commit -q", command)
        self.assertEqual(command.count("git add -A -- ."), 1)

    def test_committed_submission_and_full_audit_are_distinct(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository = root / "repository"
            logs = root / "logs"
            repository.mkdir()
            logs.mkdir()

            def git(*args: str) -> str:
                result = subprocess.run(
                    ["git", *args], cwd=repository, check=True,
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                )
                return result.stdout

            git("init")
            git("config", "user.email", "benchmark@example.invalid")
            git("config", "user.name", "Benchmark")
            (repository / "committed.txt").write_text("base\n", encoding="utf-8")
            (repository / "staged.txt").write_text("base\n", encoding="utf-8")
            (repository / "unstaged.txt").write_text("base\n", encoding="utf-8")
            git("add", ".")
            git("commit", "-m", "base")
            base = git("rev-parse", "HEAD").strip()

            (repository / "committed.txt").write_text("committed\n", encoding="utf-8")
            git("add", "committed.txt")
            git("commit", "-m", "agent committed change")
            head_before_capture = git("rev-parse", "HEAD").strip()
            (repository / "staged.txt").write_text("staged only\n", encoding="utf-8")
            git("add", "staged.txt")
            (repository / "unstaged.txt").write_text("unstaged only\n", encoding="utf-8")
            (repository / "untracked.txt").write_text("untracked only\n", encoding="utf-8")

            agent = object.__new__(PinnedCLIAgent)
            agent._base_commit = base
            original_logs = pinned_agent_module._AGENT_LOGS
            pinned_agent_module._AGENT_LOGS = str(logs)
            try:
                subprocess.run(
                    ["bash", "-c", agent._capture_command()], cwd=repository,
                    check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                )
            finally:
                pinned_agent_module._AGENT_LOGS = original_logs

            self.assertEqual(git("rev-parse", "HEAD").strip(), head_before_capture)
            official = subprocess.run(
                ["git", "diff", "--binary", base, "HEAD", "--"],
                cwd=repository, check=True, stdout=subprocess.PIPE,
            ).stdout
            committed_capture = (logs / "committed-workspace.patch").read_bytes()
            audit_capture = (logs / "full-workspace.patch").read_bytes()
            self.assertEqual(committed_capture, official)
            self.assertNotEqual(audit_capture, official)
            self.assertIn(b"committed.txt", official)
            for name in (b"staged.txt", b"unstaged.txt", b"untracked.txt"):
                self.assertNotIn(name, official)
                self.assertIn(name, audit_capture)
            receipt = json.loads((logs / "workspace-capture.json").read_text())
            self.assertEqual(receipt["schema_version"], "agentic-bench/workspace-capture-v2")
            self.assertTrue(receipt["uncommitted_changes_present"])


if __name__ == "__main__":
    unittest.main()
