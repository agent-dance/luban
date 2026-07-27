"""Pier agent adapter for the formal Agentic Coding comparison.

The host-side Go backend owns the real provider credential and the evidence
proxy.  This adapter receives only a dummy token and an unguessable proxy URL.
It uploads the exact frozen Linux binary, runs a zero-model network-isolation
canary, executes the CLI, and captures the complete Git workspace before Pier
starts its physically separate verifier.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import posixpath
import shlex
import stat
import sys
from dataclasses import dataclass
from pathlib import Path
from pathlib import PurePosixPath
from urllib.parse import urlparse

sys.dont_write_bytecode = True

from pier.agents.base import BaseAgent
from pier.agents.installed.base import NonZeroAgentExitCodeError
from pier.environments.base import BaseEnvironment
from pier.models.agent.context import AgentContext
from pier.models.agent.network import NetworkAllowlist

from benchmark.agentic.pier import terminal_evidence


_REMOTE_SINGLE_BINARY = "/opt/agentic-bench/agent"
_REMOTE_VENDOR_ROOT = "/opt/agentic-bench/vendor"
_LUBAN_RUNTIME_BWRAP = "x86_64-unknown-linux-musl/codex-resources/bwrap"
_LUBAN_RUNTIME_RG = "x86_64-unknown-linux-musl/codex-path/rg"
_LUBAN_RUNTIME_FILES = frozenset({_LUBAN_RUNTIME_BWRAP, _LUBAN_RUNTIME_RG})
_REMOTE_HOME = "/tmp/agentic-bench-home"
_WORKSPACE = "/app"
_AGENT_LOGS = "/logs/agent"
_BUNDLE_SCHEMA = "agentic-bench/codex-vendor-bundle-v2"
_TERMINAL_EVIDENCE_PARSER_SHA256 = (
    "bd41c29fa0115d6aa3da8fd0173643a4602cf7685f51ce72e92cefcde6525ddc"
)
_EFFECTIVE_ARGV_SCHEMA = "agentic-bench/effective-argv-v2"
_SANDBOX_CANARY_V4_SCHEMA = "agentic-bench/sandbox-canary-v4"
_CACHE_WIRE_SCHEMA = "agentic-bench/content-free-cache-wire-v1"
_HTTP_INFERENCE_TRANSPORT = "responses-http-inference-required"
_HTTP_INFERENCE_REQUIREMENT = "http_inference_required"
# A real zero-model v8 receipt must be generated and content-addressed before
# this can be promoted to ``verified_formal``.  The Go authority resolver rejects
# ``pending_repin`` in production; keeping the placeholder explicit prevents a
# synthetic or merely structural fixture from becoming formal authority.
_CODEX_V8_CANONICAL_AUTHORITY_SCOPE = "pending_repin"
_CODEX_HTTP_PROVIDER = "agentic_http"
_CODEX_HTTP_PROVIDER_CONFIG_TOKEN = (
    'model_providers.agentic_http={name="OpenAI",base_url="{provider_base_url}",'
    'wire_api="responses",requires_openai_auth=true,supports_websockets=false}'
)
_LUBAN_DISALLOWED_TOOLS = "WebSearch,WebFetch,Agent,Skill,TeamCreate,SendMessage"
_CODEX_WEB_SEARCH_CANARY_CATALOG_REMOTE = (
    "/tmp/agentic-bench-codex-web-search-models.json"
)
_CODEX_WEB_SEARCH_DISABLED_CONFIG = 'web_search="disabled"'
_CODEX_AGENTS_DISABLED_CONFIG = "agents.enabled=false"
_CODEX_SERVICE_TIER_DEFAULT_CONFIG = 'service_tier="default"'
_CODEX_SERVICE_TIER_PRIORITY_CONFIG = 'service_tier="priority"'
_CODEX_WEB_SEARCH_CANARY_CATALOG = {
    "models": [
        {
            "slug": "gpt-5.6-sol",
            "display_name": "GPT-5.6-Sol",
            "description": "Agentic benchmark web-search configuration canary",
            "default_reasoning_level": "xhigh",
            "supported_reasoning_levels": [
                {"effort": "xhigh", "description": "Extra high reasoning"}
            ],
            "shell_type": "shell_command",
            "visibility": "list",
            "supported_in_api": True,
            "priority": 1,
            "availability_nux": None,
            "upgrade": None,
            "base_instructions": "You are Codex.",
            "support_verbosity": False,
            "default_verbosity": None,
            "apply_patch_tool_type": None,
            "truncation_policy": {"mode": "bytes", "limit": 10_000},
            "supports_parallel_tool_calls": True,
            "experimental_supported_tools": [],
            "service_tiers": [
                {
                    "id": "priority",
                    "name": "Fast",
                    "description": "Priority processing",
                }
            ],
            "default_service_tier": None,
            # The frozen formal model uses Responses Lite, which deliberately
            # omits hosted tools.  This diagnostic catalog selects the public
            # Responses envelope for the same model ID so the config's actual
            # wire effect is observable without a paid provider request.
            "use_responses_lite": False,
        }
    ]
}
_EXPECTED_CODEX_PACKAGE = {
    "name": "@openai/codex",
    "version": "0.145.0-linux-x64",
    "runtime_version": "0.145.0",
    "target": "x86_64-unknown-linux-musl",
    "source_url": "https://registry.npmjs.org/@openai/codex/-/codex-0.145.0-linux-x64.tgz",
    "dist_integrity": "sha512-u8w8LLv3DvsfrDCoswLIemZ0SoNEXyi511WsfFsSiYUazk9qMsB/NtU8N9vhAfN7mZAxLFoMex4v66JjHuZWwA==",
    "tarball_sha256": "11239480f8e3efd1430f23bbe91c1a397856b8bbe6185ccbaee2382d25e03df2",
}
_EXPECTED_CODEX_REGISTRY_SNAPSHOT = {
    "fetched_at": "2026-07-26T09:49:26Z",
    "package_metadata_url": "https://registry.npmjs.org/@openai%2fcodex",
    "version_metadata_url": "https://registry.npmjs.org/@openai%2fcodex/0.145.0-linux-x64",
    "dist_tags_url": "https://registry.npmjs.org/-/package/@openai%2fcodex/dist-tags",
    "latest_version": "0.145.0",
    "linux_x64_version": "0.145.0-linux-x64",
    "published_at": "2026-07-21T18:21:50.929Z",
    "registry_modified_at": "2026-07-25T20:33:52.658Z",
    "dist_shasum": "ff7b16287345f0dc9d087002dfd0aafe280b01a7",
    "tarball_size": 135637111,
    "dist_file_count": 8,
    "dist_unpacked_size": 363710778,
    "dist_signature_keyid": "SHA256:DhQ8wR5APBvFHLF/+Tc+AYvPOdTpcIDqOhxsBHRwC7U",
    "dist_signature": "MEYCIQC0FjMiAzCjgGQdi6PX3Cr/H+hs5baEiRdFeqdqNBLhZAIhAKbzR4enAHr2kA0gb8bnEXotrW5oCluk9WfF3v4wQz1U",
    "dist_attestation_url": "https://registry.npmjs.org/-/npm/v1/attestations/@openai%2fcodex@0.145.0-linux-x64",
    "dist_attestation_predicate_type": "https://slsa.dev/provenance/v1",
    "npm_audit_signatures_version": "11.12.1",
    "npm_audit_signatures_verified": True,
    "package_file_count": 8,
    "package_tree_sha256": "68e64b834dee9d80f5df3de9dd5f1217e8cd3c0173323d7153ed882f0b6b3429",
}


@dataclass(frozen=True)
class BundleFile:
    path: str
    mode: str
    size: int
    sha256: str


@dataclass(frozen=True)
class BundleManifest:
    files: tuple[BundleFile, ...]
    binary_path: str
    tree_sha256: str


def _is_sha256(value: object) -> bool:
    return (
        isinstance(value, str)
        and len(value) == 64
        and all(character in "0123456789abcdef" for character in value)
    )


def _reject_duplicate_json_keys(pairs: list[tuple[str, object]]) -> dict:
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def _require_exact_keys(value: object, keys: set[str], label: str) -> dict:
    if not isinstance(value, dict) or set(value) != keys:
        raise ValueError(f"{label} has unexpected fields")
    return value


def _strict_json_object(raw: str | bytes, label: str) -> dict:
    """Decode one exact JSON object while rejecting duplicate/trailing data."""

    if isinstance(raw, bytes):
        try:
            source = raw.decode("utf-8")
        except UnicodeDecodeError as error:
            raise ValueError(f"{label} is not UTF-8 JSON") from error
    elif isinstance(raw, str):
        source = raw
    else:
        raise ValueError(f"{label} is not JSON text")
    decoder = json.JSONDecoder(object_pairs_hook=_reject_duplicate_json_keys)
    try:
        value, end = decoder.raw_decode(source)
    except (TypeError, json.JSONDecodeError) as error:
        raise ValueError(f"{label} is not strict JSON") from error
    if source[end:].strip():
        raise ValueError(f"{label} contains trailing JSON")
    if not isinstance(value, dict):
        raise ValueError(f"{label} is not a JSON object")
    return value


_CACHE_POLICY_FIELDS = {
    "observed",
    "shape_valid",
    "prompt_cache_key_present",
    "prompt_cache_key_sha256",
    "prompt_cache_options_present",
    "prompt_cache_options_mode",
    "prompt_cache_options_ttl_present",
    "prompt_cache_options_ttl",
    "prompt_cache_options_ttl_seconds",
    "prompt_cache_options_ttl_seconds",
    "prompt_cache_retention_present",
    "prompt_cache_retention",
    "prompt_cache_breakpoint_count",
    "prompt_cache_breakpoint_position_hashes",
}


def _validate_content_free_cache_policy(value: object, label: str) -> dict:
    policy = _require_exact_keys(value, _CACHE_POLICY_FIELDS, label)
    for field in (
        "observed",
        "shape_valid",
        "prompt_cache_key_present",
        "prompt_cache_options_present",
        "prompt_cache_options_ttl_present",
        "prompt_cache_retention_present",
    ):
        if type(policy[field]) is not bool:
            raise ValueError(f"{label}.{field} is not boolean")
    for field in (
        "prompt_cache_key_sha256",
        "prompt_cache_options_mode",
        "prompt_cache_options_ttl",
        "prompt_cache_retention",
    ):
        if not isinstance(policy[field], str):
            raise ValueError(f"{label}.{field} is not text")
    if type(policy["prompt_cache_breakpoint_count"]) is not int or policy[
        "prompt_cache_breakpoint_count"
    ] < 0:
        raise ValueError(f"{label} has an invalid breakpoint count")
    positions = policy["prompt_cache_breakpoint_position_hashes"]
    if (
        not isinstance(positions, list)
        or len(positions) != policy["prompt_cache_breakpoint_count"]
        or any(not _is_sha256(position) for position in positions)
        or positions != sorted(positions)
        or len(set(positions)) != len(positions)
    ):
        raise ValueError(f"{label} has invalid breakpoint-position hashes")
    if policy["prompt_cache_key_present"] != _is_sha256(
        policy["prompt_cache_key_sha256"]
    ):
        raise ValueError(f"{label} has inconsistent cache-key evidence")
    if not policy["prompt_cache_key_present"] and policy[
        "prompt_cache_key_sha256"
    ]:
        raise ValueError(f"{label} retained an unbound cache-key digest")
    ttl_seconds = policy["prompt_cache_options_ttl_seconds"]
    if policy["prompt_cache_options_ttl_present"]:
        if (
            not isinstance(policy["prompt_cache_options_ttl"], str)
            or not policy["prompt_cache_options_ttl"]
            or type(ttl_seconds) is not int
            or ttl_seconds <= 0
        ):
            raise ValueError(f"{label} has invalid cache TTL evidence")
    elif policy["prompt_cache_options_ttl"] or ttl_seconds is not None:
        raise ValueError(f"{label} synthesized an omitted cache TTL")
    if not policy["prompt_cache_options_present"] and (
        policy["prompt_cache_options_mode"]
        or policy["prompt_cache_options_ttl_present"]
    ):
        raise ValueError(f"{label} synthesized omitted cache options")
    if policy["prompt_cache_retention_present"] != bool(
        policy["prompt_cache_retention"]
    ):
        raise ValueError(f"{label} has inconsistent cache-retention evidence")
    if not policy["observed"] or not policy["shape_valid"]:
        raise ValueError(f"{label} did not observe a valid cache-policy shape")
    return policy


def _cache_evidence_sha256(domain: str, value: str) -> str:
    if domain not in {"prompt-cache-key", "prompt-cache-breakpoint-position"}:
        raise ValueError("cache-evidence hash domain is invalid")
    return hashlib.sha256(
        b"agentic-bench/cache-evidence/v1\0"
        + domain.encode("ascii")
        + b"\0"
        + value.encode("utf-8")
    ).hexdigest()


def _go_canonical_json(value: object, *, sort_map_keys: bool) -> bytes:
    """Mirror encoding/json for the bounded JSON shapes used by tool evidence."""

    try:
        source = json.dumps(
            value,
            ensure_ascii=False,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=sort_map_keys,
        )
    except (TypeError, ValueError) as error:
        raise ValueError("tool definition is not canonical JSON") from error
    # Go's encoding/json leaves ordinary UTF-8 intact but HTML-escapes these
    # code points by default.  StableToolCatalogSHA256 uses that encoder.
    source = (
        source.replace("&", "\\u0026")
        .replace("<", "\\u003c")
        .replace(">", "\\u003e")
        .replace("\u2028", "\\u2028")
        .replace("\u2029", "\\u2029")
    )
    return source.encode("utf-8")


def _content_free_tool_catalog_evidence(definitions: object) -> dict:
    if not isinstance(definitions, list) or not definitions:
        raise ValueError("tool catalog is not a non-empty JSON array")
    public_tools: list[dict] = []
    semantic_projection: list[dict] = []
    canonical_bytes = 0
    for raw_definition in definitions:
        if not isinstance(raw_definition, dict):
            raise ValueError("tool catalog contains a non-object definition")
        kind = raw_definition.get("type")
        name = raw_definition.get("name")
        if kind not in {"function", "custom"} or not isinstance(name, str) or not name:
            raise ValueError("tool catalog contains an unsupported identity")
        if "allowed_callers" in raw_definition:
            raise ValueError("tool catalog contains a provider-owned definition")
        strict: bool | None = None
        if "strict" in raw_definition:
            if type(raw_definition["strict"]) is not bool:
                raise ValueError("tool catalog contains a non-boolean strict flag")
            strict = raw_definition["strict"]
        description_sha256 = ""
        description_bytes = 0
        if "description" in raw_definition:
            description = raw_definition["description"]
            if not isinstance(description, str):
                raise ValueError("tool catalog contains a non-text description")
            if description:
                encoded_description = description.encode("utf-8")
                description_bytes = len(encoded_description)
                description_sha256 = hashlib.sha256(encoded_description).hexdigest()
        schema: object | None = None
        for field in ("parameters", "input_schema", "schema", "format"):
            if field in raw_definition:
                schema = raw_definition[field]
                break
        schema_sha256 = ""
        schema_bytes = 0
        if schema is not None:
            canonical_schema = _go_canonical_json(schema, sort_map_keys=True)
            schema_bytes = len(canonical_schema)
            schema_sha256 = hashlib.sha256(canonical_schema).hexdigest()
        canonical_definition = _go_canonical_json(
            raw_definition, sort_map_keys=True
        )
        definition_sha256 = hashlib.sha256(canonical_definition).hexdigest()
        definition_bytes = len(canonical_definition)
        canonical_bytes += definition_bytes
        public_tools.append(
            {
                "type": kind,
                "name": name,
                "definition_sha256": definition_sha256,
                "definition_bytes": definition_bytes,
            }
        )
        # Dict insertion order deliberately mirrors the Go struct declaration
        # in harness.stableToolDefinitionProjection.
        projection = {
            "type": kind,
            "name": name,
            "billing_owner": "client",
        }
        if strict is not None:
            projection["strict"] = strict
        if schema_sha256:
            projection["schema_sha256"] = schema_sha256
        projection["schema_bytes"] = schema_bytes
        if description_sha256:
            projection["description_sha256"] = description_sha256
        projection["description_bytes"] = description_bytes
        projection["definition_sha256"] = definition_sha256
        projection["definition_bytes"] = definition_bytes
        semantic_projection.append(projection)
    semantic = _go_canonical_json(semantic_projection, sort_map_keys=False)
    return {
        "tool_definitions": public_tools,
        "tool_catalog_semantic_sha256": hashlib.sha256(semantic).hexdigest(),
        "tool_catalog_canonical_bytes": canonical_bytes,
    }


def _validate_content_free_tool_catalog_requests(
    requests: list[dict], expected: list[tuple[str, str]]
) -> dict:
    stable: dict | None = None
    for index, request in enumerate(requests):
        definitions = request.get("tool_definitions")
        if not isinstance(definitions, list) or len(definitions) != len(expected):
            raise ValueError(f"tool-wire request {index} has an invalid catalog")
        for tool_index, (definition, identity) in enumerate(zip(definitions, expected)):
            tool = _require_exact_keys(
                definition,
                {"type", "name", "definition_sha256", "definition_bytes"},
                f"tool-wire request {index} definition {tool_index}",
            )
            if (
                (tool["type"], tool["name"]) != identity
                or not _is_sha256(tool["definition_sha256"])
                or type(tool["definition_bytes"]) is not int
                or tool["definition_bytes"] <= 0
            ):
                raise ValueError(f"tool-wire request {index} changed its catalog")
        semantic = request.get("tool_catalog_semantic_sha256")
        canonical_bytes = request.get("tool_catalog_canonical_bytes")
        if (
            not _is_sha256(semantic)
            or type(canonical_bytes) is not int
            or canonical_bytes
            != sum(definition["definition_bytes"] for definition in definitions)
        ):
            raise ValueError(f"tool-wire request {index} has invalid catalog evidence")
        evidence = {
            "tool_definitions": definitions,
            "tool_catalog_semantic_sha256": semantic,
            "tool_catalog_canonical_bytes": canonical_bytes,
        }
        if stable is None:
            stable = evidence
        elif evidence != stable:
            raise ValueError("tool-wire catalog changed between inference requests")
    if stable is None:
        raise ValueError("tool-wire canary captured no catalog")
    return stable


def _summarize_content_free_cache_requests(requests: list[dict]) -> dict:
    if not requests:
        raise ValueError("cache-wire canary captured no inference requests")
    policies = [
        _validate_content_free_cache_policy(
            request.get("cache_policy"), f"cache-wire request {index}"
        )
        for index, request in enumerate(requests)
    ]
    key_hashes = [
        policy["prompt_cache_key_sha256"]
        for policy in policies
        if policy["prompt_cache_key_present"]
    ]
    transitions = sum(
        previous != current
        for previous, current in zip(key_hashes, key_hashes[1:])
    )
    summary = {
        "schema_version": _CACHE_WIRE_SCHEMA,
        "content_retained": False,
        "observed_requests": len(policies),
        "shape_valid_requests": sum(1 for policy in policies if policy["shape_valid"]),
        "key_present_requests": len(key_hashes),
        "unique_key_count": len(set(key_hashes)),
        "key_transitions": transitions,
        "first_key_sha256": key_hashes[0] if key_hashes else "",
        "stable": len(key_hashes) == len(policies) and len(set(key_hashes)) == 1,
        "prompt_cache_options_modes": [
            policy["prompt_cache_options_mode"] for policy in policies
        ],
        "prompt_cache_options_ttls": [
            policy["prompt_cache_options_ttl"] for policy in policies
        ],
        "prompt_cache_options_ttl_seconds": [
            policy["prompt_cache_options_ttl_seconds"] for policy in policies
        ],
        "prompt_cache_retentions": [
            policy["prompt_cache_retention"] for policy in policies
        ],
        "breakpoint_counts": [
            policy["prompt_cache_breakpoint_count"] for policy in policies
        ],
        "breakpoint_position_hashes": [
            policy["prompt_cache_breakpoint_position_hashes"]
            for policy in policies
        ],
    }
    if not summary["stable"]:
        raise ValueError("cache-wire canary did not prove one stable hashed lineage")
    return summary


def _decode_sandbox_canary_v4(
    raw: str | bytes,
    *,
    expected_agent_kind: str,
    allow_pending_authority: bool = False,
) -> dict:
    if expected_agent_kind not in {"codex", "luban"}:
        raise ValueError("sandbox canary agent kind is invalid")
    fields = {
        "schema_version",
        "agent_kind",
        "binary_sha256",
        "base_commit",
        "controller_proxy_reachable",
        "tool_proxy_reachable",
        "credential_in_agent",
        "adapter_sha256",
        "bundle_manifest_sha256",
        "effective_argv_receipt_sha256",
        "source_bundle_tree_sha256",
        "runtime_payload_tree_sha256",
        "provider_canary_requests",
        "provider_canary_transport",
        "http_transport",
        "cache_wire",
    }
    if expected_agent_kind == "codex":
        fields |= {
            "canonical_authority",
            "sandbox_negative_control",
            "web_search_configuration_canary",
            "workspace_state",
        }
    receipt = _require_exact_keys(
        _strict_json_object(raw, "sandbox canary"), fields, "sandbox canary"
    )
    if (
        receipt["schema_version"] != _SANDBOX_CANARY_V4_SCHEMA
        or receipt["agent_kind"] != expected_agent_kind
        or receipt["provider_canary_transport"] != _HTTP_INFERENCE_TRANSPORT
        or type(receipt["controller_proxy_reachable"]) is not bool
        or receipt["controller_proxy_reachable"] is not True
        or receipt["tool_proxy_reachable"] is not False
        or receipt["credential_in_agent"] is not False
    ):
        raise ValueError("sandbox canary violates the v4 HTTP contract")
    for digest_field in (
        "binary_sha256",
        "adapter_sha256",
        "bundle_manifest_sha256",
        "effective_argv_receipt_sha256",
        "source_bundle_tree_sha256",
        "runtime_payload_tree_sha256",
    ):
        if not _is_sha256(receipt[digest_field]):
            raise ValueError(f"sandbox canary has invalid {digest_field}")
    if (
        not isinstance(receipt["base_commit"], str)
        or len(receipt["base_commit"]) != 40
        or any(character not in "0123456789abcdef" for character in receipt["base_commit"])
    ):
        raise ValueError("sandbox canary has invalid base_commit")
    requests = receipt["provider_canary_requests"]
    expected_request_count = 2 if expected_agent_kind == "codex" else 3
    if (
        not isinstance(requests, list)
        or len(requests) != expected_request_count
        or any(not isinstance(request, dict) for request in requests)
        or any(request.get("transport") != "http_sse" for request in requests)
        or any(request.get("prewarm_requested") is not False for request in requests)
        or any(request.get("websocket_upgrade_count_before_request") != 0 for request in requests)
    ):
        raise ValueError("sandbox canary contains non-HTTP inference")
    expected_transport = {
        "schema_version": "agentic-bench/http-inference-transport-v1",
        "requirement": _HTTP_INFERENCE_REQUIREMENT,
        "http_inference_request_count": expected_request_count,
        "websocket_upgrade_request_count": 0,
        "websocket_generation_frame_count": 0,
        "prewarm_request_count": 0,
    }
    if receipt["http_transport"] != expected_transport:
        raise ValueError("sandbox canary transport summary is invalid")
    expected_tools = (
        [
            ("custom", "exec"),
            ("function", "wait"),
            ("function", "request_user_input"),
        ]
        if expected_agent_kind == "codex"
        else [
            ("function", "Inspect"),
            ("function", "ApplyPatch"),
            ("function", "Run"),
        ]
    )
    _validate_content_free_tool_catalog_requests(requests, expected_tools)
    if receipt["cache_wire"] != _summarize_content_free_cache_requests(requests):
        raise ValueError("sandbox canary cache summary differs from request evidence")
    if expected_agent_kind == "codex":
        authority = _require_exact_keys(
            receipt["canonical_authority"],
            {
                "generation",
                "authority_scope",
                "responses_transport_requirement",
            },
            "Codex canonical authority",
        )
        allowed_scope = {"verified_formal"}
        if allow_pending_authority:
            allowed_scope.add("pending_repin")
        if (
            authority["generation"] != "v8"
            or authority["authority_scope"] not in allowed_scope
            or authority["responses_transport_requirement"]
            != _HTTP_INFERENCE_REQUIREMENT
        ):
            raise ValueError("Codex canonical authority is not formal")
    return receipt


def _canonical_bundle_tree(files: tuple[BundleFile, ...]) -> str:
    digest = hashlib.sha256()
    for entry in files:
        digest.update(
            (
                entry.path
                + "\0"
                + entry.mode
                + "\0"
                + str(entry.size)
                + "\0"
                + entry.sha256
                + "\n"
            ).encode("utf-8")
        )
    return digest.hexdigest()


def load_bundle_manifest(
    manifest_path: Path,
    bundle_root: Path,
    binary_path: Path | None,
    binary_sha256: str | None,
    expected_tree_sha256: str,
) -> BundleManifest:
    """Validate a complete, regular-file-only vendor tree before upload."""

    if not manifest_path.is_absolute() or not bundle_root.is_absolute():
        raise ValueError("bundle manifest and root must be absolute")
    if bundle_root.is_symlink() or not bundle_root.is_dir():
        raise ValueError("bundle root must be a real directory")
    try:
        source = json.loads(
            manifest_path.read_text(encoding="utf-8"),
            object_pairs_hook=_reject_duplicate_json_keys,
        )
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError("cannot read Codex bundle manifest") from error
    source = _require_exact_keys(
        source,
        {
            "schema_version",
            "package",
            "registry_snapshot",
            "binary_path",
            "tree_sha256",
            "files",
        },
        "bundle manifest",
    )
    if source["schema_version"] != _BUNDLE_SCHEMA:
        raise ValueError("unsupported Codex bundle manifest schema")
    package = _require_exact_keys(
        source["package"], set(_EXPECTED_CODEX_PACKAGE), "bundle package"
    )
    if package != _EXPECTED_CODEX_PACKAGE:
        raise ValueError("Codex bundle package provenance differs from 0.145.0")
    registry_snapshot = _require_exact_keys(
        source["registry_snapshot"],
        set(_EXPECTED_CODEX_REGISTRY_SNAPSHOT),
        "Codex registry snapshot",
    )
    if json.dumps(
        registry_snapshot, sort_keys=True, separators=(",", ":")
    ) != json.dumps(
        _EXPECTED_CODEX_REGISTRY_SNAPSHOT,
        sort_keys=True,
        separators=(",", ":"),
    ):
        raise ValueError("Codex registry snapshot differs from the frozen release")
    if source["tree_sha256"] != expected_tree_sha256 or not _is_sha256(
        expected_tree_sha256
    ):
        raise ValueError("Codex bundle tree hash differs from the frozen value")
    if not isinstance(source["files"], list) or not source["files"]:
        raise ValueError("Codex bundle manifest has no files")

    entries: list[BundleFile] = []
    previous = ""
    for index, raw_entry in enumerate(source["files"]):
        raw_entry = _require_exact_keys(
            raw_entry, {"path", "mode", "size", "sha256"}, f"bundle file {index}"
        )
        relative = raw_entry["path"]
        mode = raw_entry["mode"]
        size = raw_entry["size"]
        sha256 = raw_entry["sha256"]
        if (
            not isinstance(relative, str)
            or not relative
            or "\\" in relative
            or relative.startswith("/")
            or posixpath.normpath(relative) != relative
            or any(part in {"", ".", ".."} for part in PurePosixPath(relative).parts)
        ):
            raise ValueError("bundle file path is not canonical and relative")
        if previous and relative <= previous:
            raise ValueError("bundle files must be uniquely sorted by path")
        if (
            not isinstance(mode, str)
            or len(mode) != 4
            or mode[0] != "0"
            or any(character not in "01234567" for character in mode)
        ):
            raise ValueError("bundle file mode must be four-digit octal")
        if not isinstance(size, int) or isinstance(size, bool) or size < 0:
            raise ValueError("bundle file size must be non-negative")
        if not _is_sha256(sha256):
            raise ValueError("bundle file hash must be lowercase SHA-256")
        entries.append(BundleFile(relative, mode, size, sha256))
        previous = relative

    manifest_binary_path = source["binary_path"]
    if (
        not isinstance(manifest_binary_path, str)
        or not manifest_binary_path
        or "\\" in manifest_binary_path
        or manifest_binary_path.startswith("/")
        or posixpath.normpath(manifest_binary_path) != manifest_binary_path
    ):
        raise ValueError("bundle binary path is not canonical and relative")
    frozen = BundleManifest(
        files=tuple(entries),
        binary_path=manifest_binary_path,
        tree_sha256=source["tree_sha256"],
    )
    if frozen.binary_path not in {entry.path for entry in frozen.files}:
        raise ValueError("bundle binary path is absent from its file manifest")
    binary_entry = next(
        entry for entry in frozen.files if entry.path == frozen.binary_path
    )
    if binary_path is not None or binary_sha256 is not None:
        if binary_path is None or binary_sha256 is None:
            raise ValueError("bundle binary path and hash must be supplied together")
        if binary_entry.sha256 != binary_sha256:
            raise ValueError("Codex binary hash differs from its bundle manifest")
        expected_binary = bundle_root.joinpath(
            *PurePosixPath(frozen.binary_path).parts
        )
        if os.path.abspath(binary_path) != os.path.abspath(expected_binary):
            raise ValueError("Codex binary is not at its original vendor path")

    expected_files = {entry.path: entry for entry in frozen.files}
    expected_dirs = set()
    for relative in expected_files:
        parts = PurePosixPath(relative).parts
        for length in range(1, len(parts)):
            expected_dirs.add(PurePosixPath(*parts[:length]).as_posix())
    seen: set[str] = set()
    for current, directories, filenames in os.walk(bundle_root, followlinks=False):
        current_path = Path(current)
        for name in directories:
            path = current_path / name
            if path.is_symlink():
                raise ValueError("Codex bundle contains a directory symlink")
            relative = path.relative_to(bundle_root).as_posix()
            if relative not in expected_dirs:
                raise ValueError("Codex bundle contains an unexpected directory")
        for name in filenames:
            path = current_path / name
            relative = path.relative_to(bundle_root).as_posix()
            entry = expected_files.get(relative)
            if entry is None:
                raise ValueError("Codex bundle contains an unexpected file")
            metadata = path.lstat()
            if not stat.S_ISREG(metadata.st_mode):
                raise ValueError("Codex bundle contains a non-regular file")
            if f"{stat.S_IMODE(metadata.st_mode):04o}" != entry.mode:
                raise ValueError("Codex bundle file mode differs from its manifest")
            if metadata.st_size != entry.size or file_sha256(path) != entry.sha256:
                raise ValueError("Codex bundle file content differs from its manifest")
            seen.add(relative)
    if seen != set(expected_files):
        raise ValueError("Codex bundle is incomplete")
    if _canonical_bundle_tree(frozen.files) != frozen.tree_sha256:
        raise ValueError("Codex bundle canonical tree hash is invalid")
    return frozen


def _canonical_argv_json(argv: list[str]) -> bytes:
    """Return the one cross-runtime representation used by every argv hash.

    Formal argv is deliberately printable ASCII.  Apart from making Python
    and Go JSON encoding byte-identical, this prevents terminal controls,
    instruction content, or opaque credential bytes from entering provenance.
    """

    if not isinstance(argv, list) or not argv or len(argv) > 64:
        raise ValueError("effective argv must be a non-empty bounded JSON array")
    for value in argv:
        if (
            not isinstance(value, str)
            or not value
            or len(value) > 4096
            or any(ord(character) < 0x20 or ord(character) > 0x7E for character in value)
        ):
            raise ValueError("effective argv must contain printable ASCII strings")
    return json.dumps(argv, ensure_ascii=True, separators=(",", ":")).encode("ascii")


def _argv_sha256(argv: list[str]) -> str:
    return hashlib.sha256(_canonical_argv_json(argv)).hexdigest()


def _formal_source_argv_tail(agent_kind: str) -> list[str]:
    if agent_kind == "codex":
        return [
            "--ask-for-approval",
            "never",
            "--sandbox",
            "workspace-write",
            "exec",
            "--json",
            "--ephemeral",
            "--ignore-user-config",
            "--model",
            "gpt-5.6-sol",
            "--config",
            "model_reasoning_effort=xhigh",
            "--config",
            _CODEX_SERVICE_TIER_DEFAULT_CONFIG,
            "--config",
            'web_search="disabled"',
            "--config",
            _CODEX_AGENTS_DISABLED_CONFIG,
            "--config",
            f'model_provider="{_CODEX_HTTP_PROVIDER}"',
            "--config",
            _CODEX_HTTP_PROVIDER_CONFIG_TOKEN,
            "{instruction_path}",
        ]
    if agent_kind == "luban":
        return [
            "--print",
            "--output-format",
            "stream-json",
            "--provider",
            "openai",
            "--api",
            "responses",
            "--model",
            "gpt-5.6-sol",
            "--reasoning-effort",
            "xhigh",
            "--service-tier",
            "default",
            "--pinned-model",
            "--no-model-fallback",
            "--allow-all",
            "--force-sandbox-tools",
            "{instruction_path}",
        ]
    raise ValueError("unsupported formal agent kind")


def _effective_semantic_projection(
    agent_kind: str, argv: list[str]
) -> dict[str, object]:
    def one_value(flag: str) -> str:
        indexes = [index for index, value in enumerate(argv) if value == flag]
        if len(indexes) != 1 or indexes[0] + 1 >= len(argv):
            raise ValueError(f"effective argv must contain exactly one {flag}")
        return argv[indexes[0] + 1]

    if "--search" in argv:
        raise ValueError("effective argv enabled web search")
    common: dict[str, object] = {
        "api": "responses",
        "instruction_transport": "stdin",
        "provider": "openai",
        # Both zero-model canaries and every metered provider request attest
        # this wire property.  Codex 0.145 has no valid storage CLI override.
        "response_storage": False,
    }
    if agent_kind == "codex":
        configs = [
            argv[index + 1]
            for index, value in enumerate(argv[:-1])
            if value == "--config"
        ]
        provider_prefix, provider_suffix = _CODEX_HTTP_PROVIDER_CONFIG_TOKEN.split(
            "{provider_base_url}", 1
        )
        bound_provider_configs = [
            value
            for value in configs
            if value.startswith(provider_prefix)
            and value.endswith(provider_suffix)
            and value[len(provider_prefix) : len(value) - len(provider_suffix)]
            != "{provider_base_url}"
        ]
        if (
            configs.count("model_reasoning_effort=xhigh") != 1
            or configs.count(_CODEX_SERVICE_TIER_DEFAULT_CONFIG) != 1
            or configs.count('web_search="disabled"') != 1
            or configs.count(_CODEX_AGENTS_DISABLED_CONFIG) != 1
            or configs.count(f'model_provider="{_CODEX_HTTP_PROVIDER}"') != 1
            or len(bound_provider_configs) != 1
        ):
            raise ValueError(
                "Codex effective config drifted in reasoning, service tier, search, or agents"
            )
        common.update(
            {
                "agents_enabled": False,
                "approval_policy": one_value("--ask-for-approval"),
                "model": one_value("--model"),
                "model_fallback": False,
                "provider_endpoint": "private-proxy",
                "provider_transport": "responses-http",
                "reasoning_effort": "xhigh",
                "responses_api_profile": "codex_lite",
                "responses_lite": True,
                "responses_transport_requirement": _HTTP_INFERENCE_REQUIREMENT,
                "sandbox_policy": one_value("--sandbox"),
                "service_tier": "default",
                "user_config": "ignored",
                "web_search": False,
            }
        )
    elif agent_kind == "luban":
        if "--responses-websocket" in argv:
            raise ValueError("Luban effective argv enabled WebSocket inference")
        disallowed = one_value("--disallowed-tools").split(",")
        required_disabled = {
            "WebSearch",
            "WebFetch",
            "Agent",
            "TeamCreate",
            "SendMessage",
        }
        if not required_disabled.issubset(disallowed):
            raise ValueError("Luban effective argv does not disable web or agent tools")
        service_tier = one_value("--service-tier")
        if service_tier != "default":
            raise ValueError("Luban effective argv service tier is not default")
        common.update(
            {
                "agents_enabled": False,
                "approval_policy": "never" if "--allow-all" in argv else "prompt",
                "model": one_value("--model"),
                "model_fallback": not (
                    "--pinned-model" in argv and "--no-model-fallback" in argv
                ),
                "provider_endpoint": "private-proxy-env",
                "provider_transport": "responses-http",
                "reasoning_effort": one_value("--reasoning-effort"),
                "responses_api_profile": "openai_public",
                "responses_lite": False,
                "responses_transport_requirement": _HTTP_INFERENCE_REQUIREMENT,
                "sandbox_policy": (
                    "forced-tools" if "--force-sandbox-tools" in argv else "unforced"
                ),
                "service_tier": service_tier,
                "user_config": "empty-home",
                "web_search": False,
            }
        )
    else:
        raise ValueError("unsupported formal agent kind")
    return common


def _content_safe_argv(agent_kind: str, argv: list[str], proxy_base_url: str) -> list[str]:
    safe = list(argv)
    dynamic = _CODEX_HTTP_PROVIDER_CONFIG_TOKEN.replace(
        "{provider_base_url}", proxy_base_url
    )
    matches = [index for index, value in enumerate(safe) if value == dynamic]
    if agent_kind == "codex":
        if len(matches) != 1:
            raise ValueError("Codex effective argv lacks one private proxy binding")
        safe[matches[0]] = _CODEX_HTTP_PROVIDER_CONFIG_TOKEN
    elif matches:
        raise ValueError("Luban effective argv unexpectedly contains a proxy argument")
    if proxy_base_url in _canonical_argv_json(safe).decode("ascii"):
        raise ValueError("content-safe effective argv leaked the private proxy URL")
    return safe


def _effective_argv_receipt(
    *,
    agent_kind: str,
    argv: list[str],
    proxy_base_url: str,
    adapter_version: str,
    adapter_sha256: str,
    source_command_argv_sha256: str,
    bundle_manifest_sha256: str,
    bundle_tree_sha256: str,
) -> dict[str, object]:
    safe = _content_safe_argv(agent_kind, argv, proxy_base_url)
    projection = _effective_semantic_projection(agent_kind, argv)
    return {
        "schema_version": _EFFECTIVE_ARGV_SCHEMA,
        "agent_kind": agent_kind,
        "adapter_version": adapter_version,
        "adapter_sha256": adapter_sha256,
        "source_command_argv_sha256": source_command_argv_sha256,
        "bundle_manifest_sha256": bundle_manifest_sha256,
        "bundle_tree_sha256": bundle_tree_sha256,
        "effective_argv": safe,
        "effective_argv_sha256": _argv_sha256(safe),
        # The unhashed value is never serialized.  The backend independently
        # reconstructs it from its ephemeral authority and the safe argv.
        "execution_argv_sha256": _argv_sha256(argv),
        "private_proxy_base_url_sha256": hashlib.sha256(
            proxy_base_url.encode("ascii")
        ).hexdigest(),
        "semantic_projection": projection,
        "semantic_projection_sha256": hashlib.sha256(
            json.dumps(
                projection, ensure_ascii=True, separators=(",", ":"), sort_keys=True
            ).encode("ascii")
        ).hexdigest(),
    }


def codex_canary_server_source() -> str:
    """Return a content-free Responses API for the real Codex exec canary.

    Codex 0.145.0 presents its coding surface through Responses Lite.  The
    model-visible ``exec`` custom tool is the orchestrator for the nested
    ``exec_command`` shell tool, so the canary exercises the same route as a
    formal task rather than invoking the sandbox helper out of band.
    """

    return """
import hashlib,http.server,json,pathlib,re,sys,time
tool_command=sys.argv[1]
expected_exit=int(sys.argv[2])
ready=pathlib.Path(sys.argv[3])
audit=pathlib.Path(sys.argv[4])
expected_tools=[('custom','exec'),('function','wait'),('function','request_user_input')]
response_usage={'input_tokens':11,'input_tokens_details':{'cached_tokens':3,'cache_write_tokens':2},'output_tokens':5,'output_tokens_details':{'reasoning_tokens':1},'total_tokens':16}
def cache_hash(domain,value):
    return hashlib.sha256(b'agentic-bench/cache-evidence/v1\\0'+domain.encode()+b'\\0'+value.encode()).hexdigest()
def cache_policy(request):
    key=request.get('prompt_cache_key')
    options=request.get('prompt_cache_options') if isinstance(request.get('prompt_cache_options'),dict) else {}
    ttl=options.get('ttl','')
    match=re.fullmatch(r'([1-9][0-9]*)(s|m|h)',ttl) if isinstance(ttl,str) else None
    positions=[]
    def walk(value,pointer=''):
        if isinstance(value,dict):
            for name in sorted(value):
                child=pointer+'/'+name.replace('~','~0').replace('/','~1')
                if name == 'prompt_cache_breakpoint':
                    positions.append(cache_hash('prompt-cache-breakpoint-position',child))
                walk(value[name],child)
        elif isinstance(value,list):
            for index,item in enumerate(value): walk(item,pointer+'/'+str(index))
    walk(request)
    multiplier={'s':1,'m':60,'h':3600}
    return {'observed':True,'shape_valid':('prompt_cache_key' not in request or isinstance(key,str) and bool(key)) and ('prompt_cache_options' not in request or isinstance(request.get('prompt_cache_options'),dict)),'prompt_cache_key_present':isinstance(key,str) and bool(key),'prompt_cache_key_sha256':cache_hash('prompt-cache-key',key) if isinstance(key,str) and key else '','prompt_cache_options_present':'prompt_cache_options' in request,'prompt_cache_options_mode':options.get('mode','') if isinstance(options.get('mode',''),str) else '','prompt_cache_options_ttl_present':'ttl' in options,'prompt_cache_options_ttl':ttl if match else '','prompt_cache_options_ttl_seconds':int(match.group(1))*multiplier[match.group(2)] if match else None,'prompt_cache_retention_present':'prompt_cache_retention' in request,'prompt_cache_retention':request.get('prompt_cache_retention','') if isinstance(request.get('prompt_cache_retention',''),str) else '','prompt_cache_breakpoint_count':len(positions),'prompt_cache_breakpoint_position_hashes':sorted(positions)}
def canonical_json(value,sort_keys):
    source=json.dumps(value,ensure_ascii=False,allow_nan=False,separators=(',',':'),sort_keys=sort_keys)
    slash=chr(92)
    for character,escape in (('&','u0026'),('<','u003c'),('>','u003e'),(chr(0x2028),'u2028'),(chr(0x2029),'u2029')): source=source.replace(character,slash+escape)
    return source.encode()
def tool_catalog_evidence(definitions):
    public=[]
    projections=[]
    total=0
    for definition in definitions:
        kind=definition.get('type')
        name=definition.get('name')
        if kind not in ('function','custom') or not isinstance(name,str) or not name or 'allowed_callers' in definition: raise ValueError('invalid tool identity')
        strict_present='strict' in definition
        strict=definition.get('strict')
        if strict_present and not isinstance(strict,bool): raise ValueError('invalid strict flag')
        description=definition.get('description','')
        if not isinstance(description,str): raise ValueError('invalid description')
        description_raw=description.encode()
        schema=None
        for field in ('parameters','input_schema','schema','format'):
            if field in definition:
                schema=definition[field]
                break
        schema_raw=canonical_json(schema,True) if schema is not None else b''
        definition_raw=canonical_json(definition,True)
        definition_hash=hashlib.sha256(definition_raw).hexdigest()
        item={'type':kind,'name':name,'definition_sha256':definition_hash,'definition_bytes':len(definition_raw)}
        public.append(item)
        projection={'type':kind,'name':name,'billing_owner':'client'}
        if strict_present: projection['strict']=strict
        if schema_raw: projection['schema_sha256']=hashlib.sha256(schema_raw).hexdigest()
        projection['schema_bytes']=len(schema_raw)
        if description_raw: projection['description_sha256']=hashlib.sha256(description_raw).hexdigest()
        projection['description_bytes']=len(description_raw)
        projection['definition_sha256']=definition_hash
        projection['definition_bytes']=len(definition_raw)
        projections.append(projection)
        total+=len(definition_raw)
    return {'tool_definitions':public,'tool_catalog_semantic_sha256':hashlib.sha256(canonical_json(projections,False)).hexdigest(),'tool_catalog_canonical_bytes':total}
class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version='HTTP/1.1'
    request_index=0
    websocket_upgrade_count=0
    websocket_upgrade_header_present=False
    websocket_key_header_present=False
    def log_message(self,*args):
        pass
    def reject(self,message):
        payload=json.dumps({'error':{'message':message,'type':'invalid_request_error'}},separators=(',',':')).encode()
        self.send_response(422)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
    def do_GET(self):
        if self.path != '/v1/responses':
            self.send_error(404)
            return
        Handler.websocket_upgrade_count+=1
        Handler.websocket_upgrade_header_present=self.headers.get('upgrade','').lower() == 'websocket' and 'upgrade' in self.headers.get('connection','').lower()
        Handler.websocket_key_header_present=self.headers.get('sec-websocket-key') is not None
        self.send_response(426)
        self.send_header('Content-Length','0')
        self.send_header('Connection','close')
        self.end_headers()
    def do_POST(self):
        if self.path != '/v1/responses':
            self.send_error(404)
            return
        try:
            request=json.loads(self.rfile.read(int(self.headers.get('content-length','0'))))
        except Exception:
            self.reject('canary request is not JSON')
            return
        inputs=request.get('input') if isinstance(request.get('input'),list) else []
        prefixes=[item for item in inputs if isinstance(item,dict) and item.get('type') == 'additional_tools']
        prefix_tools=prefixes[0].get('tools') if len(prefixes) == 1 and isinstance(prefixes[0].get('tools'),list) else []
        catalog=[(tool.get('type'),tool.get('name')) for tool in prefix_tools if isinstance(tool,dict)]
        top_tools=request.get('tools') if isinstance(request.get('tools'),list) else []
        all_tools=prefix_tools+top_tools
        try:
            tool_wire=tool_catalog_evidence(prefix_tools)
        except Exception:
            self.reject('Codex canary tool definitions are not canonical')
            return
        web_search_tools=[tool for tool in all_tools if isinstance(tool,dict) and ('web_search' in str(tool.get('type','')).lower() or str(tool.get('name','')).lower() == 'web_search' or (tool.get('type') == 'namespace' and tool.get('name') == 'web'))]
        subagent_tools=[tool for tool in all_tools if isinstance(tool,dict) and (tool.get('type') == 'namespace' and tool.get('name') == 'collaboration' or str(tool.get('name','')).lower() in {'spawn_agent','wait_agent','send_message','followup_task','interrupt_agent','list_agents'})]
        reasoning=request.get('reasoning') if isinstance(request.get('reasoning'),dict) else {}
        outputs=[item for item in inputs if isinstance(item,dict) and item.get('type') == 'custom_tool_call_output']
        output_exit=None
        if outputs:
            fragments=[]
            raw_output=outputs[0].get('output')
            if isinstance(raw_output,str):
                fragments.append(raw_output)
            elif isinstance(raw_output,list):
                fragments.extend(item.get('text','') for item in raw_output if isinstance(item,dict) and isinstance(item.get('text'),str))
            compact=''.join(fragments).replace(' ','')
            for candidate in (0,91):
                if '\"exit_code\":'+str(candidate) in compact:
                    output_exit=candidate
                    break
        audit_entry={
            'request_index':Handler.request_index,
            'model':request.get('model'),
            'store':request.get('store'),
            'reasoning_effort':reasoning.get('effort'),
            'reasoning_context':reasoning.get('context'),
            'include_encrypted_reasoning':request.get('include') == ['reasoning.encrypted_content'],
            'stream':request.get('stream'),
            'transport':'http_sse',
            'prewarm_requested':request.get('generate') is False,
            'request_service_tier_present':'service_tier' in request,
            'request_service_tier':request.get('service_tier'),
            'request_service_tier_canonical':request.get('service_tier') or 'default',
            'request_service_tier_source':'wire_explicit' if 'service_tier' in request else 'client_canonicalized_default',
            'top_level_tool_count':len(top_tools),
            'tool_catalog':[{'type':kind,'name':name} for kind,name in catalog],
            'web_search_tool_present':bool(web_search_tools),
            'web_search_tool_count':len(web_search_tools),
            'collaboration_namespace_present':any(tool.get('type') == 'namespace' and tool.get('name') == 'collaboration' for tool in all_tools if isinstance(tool,dict)),
            'subagent_tool_present':bool(subagent_tools),
            'exec_cell_wait_present':('function','wait') in catalog,
            'websocket_upgrade_count_before_request':Handler.websocket_upgrade_count,
            'websocket_upgrade_header_present':Handler.websocket_upgrade_header_present,
            'websocket_key_header_present':Handler.websocket_key_header_present,
            'responses_lite_header_present':self.headers.get('x-openai-internal-codex-responses-lite') is not None,
            'authorization_header_present':self.headers.get('authorization') is not None,
            'originator':self.headers.get('originator'),
            'user_agent_present':self.headers.get('user-agent') is not None,
            'previous_response_id_present':'previous_response_id' in request,
            'custom_tool_output_count':len(outputs),
            'tool_output_exit_code':output_exit,
            'response_model':'gpt-5.6-sol',
            'response_service_tier':'default',
            'response_service_tier_canonical':'default',
            'response_request_id_present':True,
            'response_usage':{'input_tokens':11,'cached_input_tokens':3,'cache_write_input_tokens':2,'output_tokens':5,'reasoning_output_tokens':1},
            'cache_policy':cache_policy(request),
        }
        audit_entry.update(tool_wire)
        with audit.open('a',encoding='utf-8') as handle:
            handle.write(json.dumps(audit_entry,separators=(',',':'),sort_keys=True)+'\\n')
        if len(prefixes) != 1 or catalog != expected_tools or top_tools or web_search_tools or subagent_tools:
            self.reject('Codex canary tool catalog differs from frozen Responses Lite')
            return
        if self.headers.get('x-openai-internal-codex-responses-lite') != 'true':
            self.reject('Codex canary lacks the Responses Lite header')
            return
        if self.headers.get('authorization') is None or self.headers.get('originator') != 'codex_exec':
            self.reject('Codex canary lacks controller identity headers')
            return
        if request.get('model') != 'gpt-5.6-sol':
            self.reject('Codex canary model is not pinned to gpt-5.6-sol')
            return
        if reasoning.get('effort') != 'xhigh' or reasoning.get('context') != 'all_turns':
            self.reject('Codex canary reasoning contract differs from xhigh/all_turns')
            return
        if request.get('store') is not False or request.get('include') != ['reasoning.encrypted_content']:
            self.reject('Codex canary is not stateless with encrypted reasoning')
            return
        if request.get('stream') is not True or 'previous_response_id' in request or 'service_tier' in request:
            self.reject('Codex canary Responses transport is not a stateless stream')
            return
        if Handler.websocket_upgrade_count != 0 or Handler.websocket_upgrade_header_present or Handler.websocket_key_header_present:
            self.reject('Codex HTTP canary unexpectedly attempted WebSocket')
            return
        index=Handler.request_index
        Handler.request_index+=1
        if index == 0:
            if outputs:
                self.reject('first Codex canary request unexpectedly contains tool output')
                return
            arguments={'cmd':tool_command,'workdir':'/app','login':False,'yield_time_ms':10000,'max_output_tokens':1000}
            tool_input='const r=await tools.exec_command('+json.dumps(arguments,separators=(',',':'))+');text(JSON.stringify({exit_code:r.exit_code,output:r.output}));'
            item={'type':'custom_tool_call','id':'ctc_canary','status':'completed','call_id':'call_canary','name':'exec','input':tool_input}
            response_id='resp_canary_tool'
        else:
            if len(outputs) != 1 or outputs[0].get('call_id') != 'call_canary' or output_exit != expected_exit:
                self.reject('second Codex canary request lacks the expected exec result')
                return
            item={'type':'message','id':'msg_canary','role':'assistant','content':[{'type':'output_text','text':'sandbox canary complete'}]}
            response_id='resp_canary_done'
        events=[
            {'type':'response.created','response':{'id':response_id,'model':'gpt-5.6-sol','service_tier':'default'}},
            {'type':'response.output_item.done','item':item},
            {'type':'response.completed','response':{'id':response_id,'model':'gpt-5.6-sol','service_tier':'default','usage':response_usage}},
        ]
        payload=''.join('event: '+event['type']+'\\ndata: '+json.dumps(event,separators=(',',':'))+'\\n\\n' for event in events).encode()
        self.send_response(200)
        self.send_header('Content-Type','text/event-stream')
        self.send_header('openai-model','gpt-5.6-sol')
        self.send_header('x-request-id','req_agentic_canary_'+str(index))
        self.send_header('Content-Length',str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
server=http.server.HTTPServer(('127.0.0.1',0),Handler)
ready.write_text(str(server.server_port),encoding='ascii')
deadline=time.monotonic()+45
while Handler.request_index < 2 and time.monotonic() < deadline:
    server.timeout=max(0.1,deadline-time.monotonic())
    server.handle_request()
server.server_close()
if Handler.request_index != 2 or Handler.websocket_upgrade_count != 0:
    raise SystemExit(1)
""".strip()


def codex_web_search_canary_server_source() -> str:
    """Expose the frozen CLI's public Responses web-search config on wire."""

    return """
import hashlib,http.server,json,pathlib,re,sys
ready=pathlib.Path(sys.argv[1])
audit=pathlib.Path(sys.argv[2])
class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version='HTTP/1.1'
    request_index=0
    def log_message(self,*args):
        pass
    def reject(self,message):
        payload=json.dumps({'error':{'message':message,'type':'invalid_request_error'}},separators=(',',':')).encode()
        self.send_response(422)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
    def do_POST(self):
        if self.path != '/v1/responses':
            self.send_error(404)
            return
        try:
            request=json.loads(self.rfile.read(int(self.headers.get('content-length','0'))))
        except Exception:
            self.reject('web-search canary request is not JSON')
            return
        tools=request.get('tools') if isinstance(request.get('tools'),list) else []
        catalog=[{'type':tool.get('type'),'name':tool.get('name')} for tool in tools if isinstance(tool,dict)]
        search_tools=[tool for tool in tools if isinstance(tool,dict) and 'web_search' in str(tool.get('type','')).lower()]
        subagent_tools=[tool for tool in tools if isinstance(tool,dict) and (tool.get('type') == 'namespace' and (tool.get('name') == 'collaboration' or str(tool.get('name','')).startswith('multi_agent')) or str(tool.get('name','')).lower() in {'spawn_agent','wait_agent','send_message','followup_task','interrupt_agent','list_agents'})]
        reasoning=request.get('reasoning') if isinstance(request.get('reasoning'),dict) else {}
        accepted=not search_tools and not subagent_tools and 'service_tier' not in request
        index=Handler.request_index
        Handler.request_index+=1
        entry={
            'request_index':index,
            'model':request.get('model'),
            'store':request.get('store'),
            'reasoning_effort':reasoning.get('effort'),
            'include_encrypted_reasoning':request.get('include') == ['reasoning.encrypted_content'],
            'stream':request.get('stream'),
            'responses_lite_header_present':self.headers.get('x-openai-internal-codex-responses-lite') is not None,
            'authorization_header_present':self.headers.get('authorization') is not None,
            'originator':self.headers.get('originator'),
            'request_service_tier_present':'service_tier' in request,
            'request_service_tier':request.get('service_tier'),
            'request_service_tier_canonical':request.get('service_tier') or 'default',
            'request_service_tier_source':'wire_explicit' if 'service_tier' in request else 'client_canonicalized_default',
            'ordered_tool_catalog':catalog,
            'web_search_tool_count':len(search_tools),
            'web_search_external_access':[tool.get('external_web_access') for tool in search_tools],
            'collaboration_namespace_present':any(tool.get('type') == 'namespace' and tool.get('name') == 'collaboration' for tool in tools if isinstance(tool,dict)),
            'multi_agent_namespace_present':any(tool.get('type') == 'namespace' and str(tool.get('name','')).startswith('multi_agent') for tool in tools if isinstance(tool,dict)),
            'subagent_tool_present':bool(subagent_tools),
            'configuration_accepted':accepted,
            'response_model':'gpt-5.6-sol',
            'response_service_tier':'default',
            'response_service_tier_canonical':'default',
            'response_request_id_present':True,
            'response_usage':{'input_tokens':7,'cached_input_tokens':2,'cache_write_input_tokens':1,'output_tokens':3,'reasoning_output_tokens':1},
        }
        with audit.open('a',encoding='utf-8') as handle:
            handle.write(json.dumps(entry,separators=(',',':'),sort_keys=True)+'\\n')
        if request.get('model') != 'gpt-5.6-sol' or reasoning.get('effort') != 'xhigh':
            self.reject('web-search canary model or effort drifted')
            return
        if request.get('store') is not False or request.get('stream') is not True:
            self.reject('web-search canary transport drifted')
            return
        if request.get('include') != ['reasoning.encrypted_content']:
            self.reject('web-search canary encrypted reasoning drifted')
            return
        if 'service_tier' in request:
            self.reject('web-search canary service tier drifted')
            return
        if self.headers.get('x-openai-internal-codex-responses-lite') is not None:
            self.reject('web-search config canary unexpectedly used Responses Lite')
            return
        if self.headers.get('authorization') is None or self.headers.get('originator') != 'codex_exec':
            self.reject('web-search canary lacks controller identity headers')
            return
        if search_tools or subagent_tools:
            self.reject('disabled search or agent config is absent or ineffective')
            return
        response_id='resp_web_search_canary'
        usage={'input_tokens':7,'input_tokens_details':{'cached_tokens':2,'cache_write_tokens':1},'output_tokens':3,'output_tokens_details':{'reasoning_tokens':1},'total_tokens':10}
        item={'type':'message','id':'msg_web_search_canary','role':'assistant','content':[{'type':'output_text','text':'web search canary complete'}]}
        events=[
            {'type':'response.created','response':{'id':response_id,'model':'gpt-5.6-sol','service_tier':'default'}},
            {'type':'response.output_item.done','item':item},
            {'type':'response.completed','response':{'id':response_id,'model':'gpt-5.6-sol','service_tier':'default','usage':usage}},
        ]
        payload=''.join('event: '+event['type']+'\\ndata: '+json.dumps(event,separators=(',',':'))+'\\n\\n' for event in events).encode()
        self.send_response(200)
        self.send_header('Content-Type','text/event-stream')
        self.send_header('openai-model','gpt-5.6-sol')
        self.send_header('x-request-id','req_agentic_web_search_canary_'+str(index))
        self.send_header('Content-Length',str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
server=http.server.HTTPServer(('127.0.0.1',0),Handler)
ready.write_text(str(server.server_port),encoding='ascii')
server.timeout=45
server.handle_request()
server.handle_request()
server.handle_request()
server.handle_request()
server.server_close()
""".strip()


def luban_canary_server_source() -> str:
    """Return a three-response API that verifies and exercises Agentic V2."""

    return """
import hashlib,http.server,json,pathlib,re,sys
tool_command=sys.argv[1]
ready=pathlib.Path(sys.argv[2])
audit=pathlib.Path(sys.argv[3])
expected_tools=['Inspect','ApplyPatch','Run']
def cache_hash(domain,value):
    return hashlib.sha256(b'agentic-bench/cache-evidence/v1\\0'+domain.encode()+b'\\0'+value.encode()).hexdigest()
def cache_policy(request):
    key=request.get('prompt_cache_key')
    options=request.get('prompt_cache_options') if isinstance(request.get('prompt_cache_options'),dict) else {}
    ttl=options.get('ttl','')
    match=re.fullmatch(r'([1-9][0-9]*)(s|m|h)',ttl) if isinstance(ttl,str) else None
    positions=[]
    def walk(value,pointer=''):
        if isinstance(value,dict):
            for name in sorted(value):
                child=pointer+'/'+name.replace('~','~0').replace('/','~1')
                if name == 'prompt_cache_breakpoint': positions.append(cache_hash('prompt-cache-breakpoint-position',child))
                walk(value[name],child)
        elif isinstance(value,list):
            for index,item in enumerate(value): walk(item,pointer+'/'+str(index))
    walk(request)
    multiplier={'s':1,'m':60,'h':3600}
    return {'observed':True,'shape_valid':('prompt_cache_key' not in request or isinstance(key,str) and bool(key)) and ('prompt_cache_options' not in request or isinstance(request.get('prompt_cache_options'),dict)),'prompt_cache_key_present':isinstance(key,str) and bool(key),'prompt_cache_key_sha256':cache_hash('prompt-cache-key',key) if isinstance(key,str) and key else '','prompt_cache_options_present':'prompt_cache_options' in request,'prompt_cache_options_mode':options.get('mode','') if isinstance(options.get('mode',''),str) else '','prompt_cache_options_ttl_present':'ttl' in options,'prompt_cache_options_ttl':ttl if match else '','prompt_cache_options_ttl_seconds':int(match.group(1))*multiplier[match.group(2)] if match else None,'prompt_cache_retention_present':'prompt_cache_retention' in request,'prompt_cache_retention':request.get('prompt_cache_retention','') if isinstance(request.get('prompt_cache_retention',''),str) else '','prompt_cache_breakpoint_count':len(positions),'prompt_cache_breakpoint_position_hashes':sorted(positions)}
def canonical_json(value,sort_keys):
    source=json.dumps(value,ensure_ascii=False,allow_nan=False,separators=(',',':'),sort_keys=sort_keys)
    slash=chr(92)
    for character,escape in (('&','u0026'),('<','u003c'),('>','u003e'),(chr(0x2028),'u2028'),(chr(0x2029),'u2029')): source=source.replace(character,slash+escape)
    return source.encode()
def tool_catalog_evidence(definitions):
    public=[]
    projections=[]
    total=0
    for definition in definitions:
        kind=definition.get('type')
        name=definition.get('name')
        if kind not in ('function','custom') or not isinstance(name,str) or not name or 'allowed_callers' in definition: raise ValueError('invalid tool identity')
        strict_present='strict' in definition
        strict=definition.get('strict')
        if strict_present and not isinstance(strict,bool): raise ValueError('invalid strict flag')
        description=definition.get('description','')
        if not isinstance(description,str): raise ValueError('invalid description')
        description_raw=description.encode()
        schema=None
        for field in ('parameters','input_schema','schema','format'):
            if field in definition:
                schema=definition[field]
                break
        schema_raw=canonical_json(schema,True) if schema is not None else b''
        definition_raw=canonical_json(definition,True)
        definition_hash=hashlib.sha256(definition_raw).hexdigest()
        item={'type':kind,'name':name,'definition_sha256':definition_hash,'definition_bytes':len(definition_raw)}
        public.append(item)
        projection={'type':kind,'name':name,'billing_owner':'client'}
        if strict_present: projection['strict']=strict
        if schema_raw: projection['schema_sha256']=hashlib.sha256(schema_raw).hexdigest()
        projection['schema_bytes']=len(schema_raw)
        if description_raw: projection['description_sha256']=hashlib.sha256(description_raw).hexdigest()
        projection['description_bytes']=len(description_raw)
        projection['definition_sha256']=definition_hash
        projection['definition_bytes']=len(definition_raw)
        projections.append(projection)
        total+=len(definition_raw)
    return {'tool_definitions':public,'tool_catalog_semantic_sha256':hashlib.sha256(canonical_json(projections,False)).hexdigest(),'tool_catalog_canonical_bytes':total}
class Handler(http.server.BaseHTTPRequestHandler):
    request_index=0
    websocket_upgrade_count=0
    def log_message(self,*args):
        pass
    def reject(self,message):
        payload=json.dumps({'error':{'message':message,'type':'invalid_request_error'}},separators=(',',':')).encode()
        self.send_response(422)
        self.send_header('Content-Type','application/json')
        self.send_header('Content-Length',str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
    def do_GET(self):
        if self.path != '/v1/responses':
            self.send_error(404)
            return
        Handler.websocket_upgrade_count+=1
        self.send_response(426)
        self.send_header('Content-Length','0')
        self.send_header('Connection','close')
        self.end_headers()
    def do_POST(self):
        if self.path != '/v1/responses':
            self.send_error(404)
            return
        try:
            request=json.loads(self.rfile.read(int(self.headers.get('content-length','0'))))
        except Exception:
            self.reject('canary request is not JSON')
            return
        prefixes=[item for item in request.get('input',[]) if isinstance(item,dict) and item.get('type') == 'additional_tools']
        tools=request.get('tools')
        names=[tool.get('name') for tool in tools] if isinstance(tools,list) and all(isinstance(tool,dict) for tool in tools) else []
        try:
            tool_wire=tool_catalog_evidence(tools)
        except Exception:
            self.reject('Luban canary tool definitions are not canonical')
            return
        reasoning=request.get('reasoning') if isinstance(request.get('reasoning'),dict) else {}
        audit_entry={'request_index':Handler.request_index,'transport':'http_sse','prewarm_requested':request.get('generate') is False,'websocket_upgrade_count_before_request':Handler.websocket_upgrade_count,'model':request.get('model'),'store':request.get('store'),'reasoning_effort':reasoning.get('effort'),'reasoning_context':reasoning.get('context'),'request_service_tier_present':'service_tier' in request,'request_service_tier':request.get('service_tier'),'request_service_tier_canonical':'default','request_service_tier_source':'wire_explicit_default','tool_names':names,'responses_lite_header':self.headers.get('x-openai-internal-codex-responses-lite'),'additional_tools_prefixes':len(prefixes),'previous_response_id_present':'previous_response_id' in request,'response_model':'gpt-5.6-sol','response_service_tier':'default','response_service_tier_canonical':'default','response_request_id_present':True,'cache_policy':cache_policy(request)}
        audit_entry.update(tool_wire)
        with audit.open('a',encoding='utf-8') as handle:
            handle.write(json.dumps(audit_entry,separators=(',',':'),sort_keys=True)+'\\n')
        if prefixes or not isinstance(tools,list):
            self.reject('canary request does not use the frozen public Responses tool envelope')
            return
        if names != expected_tools or any(tool.get('type') != 'function' for tool in tools):
            self.reject('Agentic V2 tool catalog is not exactly Inspect, ApplyPatch, Run')
            return
        if self.headers.get('x-openai-internal-codex-responses-lite') is not None:
            self.reject('public Responses canary carried a private Lite header')
            return
        if request.get('model') != 'gpt-5.6-sol':
            self.reject('canary model is not pinned to gpt-5.6-sol')
            return
        if reasoning.get('effort') != 'xhigh' or reasoning.get('context') not in (None,'all_turns'):
            self.reject('canary reasoning effort is not xhigh')
            return
        if request.get('store') is not False:
            self.reject('canary request is not stateless')
            return
        if request.get('service_tier') != 'default':
            self.reject('canary service tier is not explicitly default')
            return
        if request.get('include') != ['reasoning.encrypted_content']:
            self.reject('canary request does not include encrypted reasoning')
            return
        index=Handler.request_index
        Handler.request_index+=1
        if index == 0:
            patch='*** Begin Patch\\n*** Add File: .agentic-bench-luban-sandbox-canary\\n+agentic-bench-sandbox-write\\n*** End Patch'
            patch_arguments=json.dumps({'patch':patch},separators=(',',':'))
            run_arguments=json.dumps({'steps':[{'id':'sandbox-canary','shell_script':tool_command}],'fail_fast':True,'requires_patch_commit':True},separators=(',',':'))
            completed_patch={'type':'function_call','id':'fc_patch','call_id':'call_patch','name':'ApplyPatch','status':'completed','arguments':patch_arguments}
            completed_run={'type':'function_call','id':'fc_canary','call_id':'call_canary','name':'Run','status':'completed','arguments':run_arguments}
            events=[
                ('response.created',{'response':{'id':'resp_canary_tool','model':'gpt-5.6-sol','service_tier':'default','status':'in_progress'}}),
                ('response.output_item.added',{'output_index':0,'item':{'type':'function_call','id':'fc_patch','call_id':'call_patch','name':'ApplyPatch','status':'in_progress'}}),
                ('response.function_call_arguments.delta',{'output_index':0,'delta':patch_arguments}),
                ('response.function_call_arguments.done',{'output_index':0,'arguments':patch_arguments}),
                ('response.output_item.done',{'output_index':0,'item':completed_patch}),
                ('response.output_item.added',{'output_index':1,'item':{'type':'function_call','id':'fc_canary','call_id':'call_canary','name':'Run','status':'in_progress'}}),
                ('response.function_call_arguments.delta',{'output_index':1,'delta':run_arguments}),
                ('response.function_call_arguments.done',{'output_index':1,'arguments':run_arguments}),
                ('response.output_item.done',{'output_index':1,'item':completed_run}),
                ('response.completed',{'response':{'id':'resp_canary_tool','model':'gpt-5.6-sol','service_tier':'default','status':'completed','usage':{'input_tokens':1,'output_tokens':1},'output':[completed_patch,completed_run]}}),
            ]
        elif index == 1:
            outputs=[item for item in request.get('input',[]) if isinstance(item,dict) and item.get('type') == 'function_call_output']
            if {item.get('call_id') for item in outputs} != {'call_patch','call_canary'}:
                self.reject('second canary request lacks the ApplyPatch and Run results')
                return
            verify_arguments=json.dumps({'steps':[{'id':'sandbox-static-analysis','argv':['git','diff','--check']}],'fail_fast':True,'requires_patch_commit':True},separators=(',',':'))
            completed_verify={'type':'function_call','id':'fc_verify','call_id':'call_verify','name':'Run','status':'completed','arguments':verify_arguments}
            events=[
                ('response.created',{'response':{'id':'resp_canary_verify','model':'gpt-5.6-sol','service_tier':'default','status':'in_progress'}}),
                ('response.output_item.added',{'output_index':0,'item':{'type':'function_call','id':'fc_verify','call_id':'call_verify','name':'Run','status':'in_progress'}}),
                ('response.function_call_arguments.delta',{'output_index':0,'delta':verify_arguments}),
                ('response.function_call_arguments.done',{'output_index':0,'arguments':verify_arguments}),
                ('response.output_item.done',{'output_index':0,'item':completed_verify}),
                ('response.completed',{'response':{'id':'resp_canary_verify','model':'gpt-5.6-sol','service_tier':'default','status':'completed','usage':{'input_tokens':1,'output_tokens':1},'output':[completed_verify]}}),
            ]
        elif index == 2:
            outputs=[item for item in request.get('input',[]) if isinstance(item,dict) and item.get('type') == 'function_call_output']
            if {item.get('call_id') for item in outputs} != {'call_patch','call_canary','call_verify'}:
                self.reject('third canary request lacks the revision-bound verification result')
                return
            completed_message={'type':'message','id':'msg_canary','role':'assistant','status':'completed','content':[{'type':'output_text','text':'sandbox canary complete','annotations':[]}]}
            events=[
                ('response.created',{'response':{'id':'resp_canary_done','model':'gpt-5.6-sol','service_tier':'default','status':'in_progress'}}),
                ('response.output_item.added',{'output_index':0,'item':{'type':'message','id':'msg_canary'}}),
                ('response.content_part.added',{'output_index':0,'content_index':0,'part':{'type':'output_text','text':''}}),
                ('response.output_text.delta',{'output_index':0,'content_index':0,'delta':'sandbox canary complete'}),
                ('response.content_part.done',{'output_index':0,'content_index':0,'part':completed_message['content'][0]}),
                ('response.output_item.done',{'output_index':0,'item':completed_message}),
                ('response.completed',{'response':{'id':'resp_canary_done','model':'gpt-5.6-sol','service_tier':'default','status':'completed','usage':{'input_tokens':1,'output_tokens':1},'output':[completed_message]}}),
            ]
        else:
            self.reject('Luban canary received an unexpected extra request')
            return
        payload=''.join('event: '+kind+'\\ndata: '+json.dumps(data,separators=(',',':'))+'\\n\\n' for kind,data in events).encode()
        self.send_response(200)
        self.send_header('Content-Type','text/event-stream')
        self.send_header('x-request-id','req_agentic_luban_canary_'+str(index))
        self.send_header('Content-Length',str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)
server=http.server.HTTPServer(('127.0.0.1',0),Handler)
ready.write_text(str(server.server_port),encoding='ascii')
server.timeout=45
server.handle_request()
server.handle_request()
server.handle_request()
server.server_close()
if Handler.request_index != 3 or Handler.websocket_upgrade_count != 0:
    raise SystemExit(1)
""".strip()


def _write_adapter_terminal_evidence(
    agent_kind: str,
    stream_path: Path,
    exit_receipt_path: Path,
    destination: Path,
    exit_code: int,
) -> bool:
    try:
        terminal_evidence.write_terminal_evidence(
            agent_kind,
            stream_path,
            exit_receipt_path,
            destination,
            exit_code,
        )
    except terminal_evidence.TerminalEvidenceProtocolError:
        # Codex exec 0.145 projects ThreadErrorEvent.message but drops the
        # provider's structured error code. On a nonzero exit, deliberately
        # leave the adapter receipt absent: the host may classify context only
        # from the last inference round's sealed evidenceproxy response.failed
        # code, and separately requires exactly one failed Codex turn.
        if agent_kind == "codex" and exit_code != 0:
            return False
        raise
    return True


class PinnedCLIAgent(BaseAgent):
    """Run a caller-pinned Codex or Luban binary without installing anything."""

    SUPPORTS_ATIF = False
    SUPPORTS_WINDOWS = False

    def __init__(
        self,
        logs_dir: Path,
        model_name: str | None = None,
        *,
        agent_kind: str,
        binary_path: str,
        binary_sha256: str,
        command_argv: list[str],
        proxy_base_url: str,
        proxy_health_url: str,
        proxy_host: str,
        reasoning_effort: str,
        base_commit: str,
        binary_bundle_root: str | None = None,
        binary_bundle_manifest_path: str | None = None,
        binary_bundle_tree_sha256: str | None = None,
        binary_bundle_manifest_sha256: str | None = None,
        adapter_sha256: str | None = None,
        source_command_argv_sha256: str | None = None,
        adapter_version: str = "2.4.0",
        extra_env: dict[str, str] | None = None,
        **kwargs,
    ):
        super().__init__(logs_dir=logs_dir, model_name=model_name, **kwargs)
        if agent_kind not in {"codex", "luban"}:
            raise ValueError("agent_kind must be codex or luban")
        if not Path(binary_path).is_absolute():
            raise ValueError("binary_path must be absolute")
        if len(binary_sha256) != 64 or any(c not in "0123456789abcdef" for c in binary_sha256):
            raise ValueError("binary_sha256 must be lowercase SHA-256")
        if len(base_commit) != 40 or any(c not in "0123456789abcdef" for c in base_commit):
            raise ValueError("base_commit must be a full lowercase Git SHA")
        parsed_proxy = urlparse(proxy_base_url)
        parsed_health = urlparse(proxy_health_url)
        if (
            parsed_proxy.scheme != "http"
            or parsed_health.scheme != "http"
            or parsed_proxy.hostname != proxy_host
            or parsed_health.hostname != proxy_host
            or not parsed_proxy.path.endswith("/v1")
            or parsed_health.path != "/healthz"
        ):
            raise ValueError("proxy URLs do not match the pinned private transport contract")
        canonical_source_argv = _canonical_argv_json(command_argv)
        if command_argv[0] != str(Path(binary_path)) or command_argv[
            1:
        ] != _formal_source_argv_tail(agent_kind):
            raise ValueError("command_argv differs from the frozen formal command")
        if not _is_sha256(source_command_argv_sha256) or hashlib.sha256(
            canonical_source_argv
        ).hexdigest() != source_command_argv_sha256:
            raise ValueError("command_argv differs from its benchmark manifest hash")
        if adapter_version != "2.4.0" or not _is_sha256(adapter_sha256):
            raise ValueError("adapter identity is not pinned to v2.4.0")
        if file_sha256(Path(__file__).resolve()) != adapter_sha256:
            raise ValueError("loaded pinned_agent.py differs from the preflight adapter hash")
        if (
            file_sha256(Path(terminal_evidence.__file__).resolve())
            != _TERMINAL_EVIDENCE_PARSER_SHA256
        ):
            raise ValueError("terminal evidence parser differs from its pinned hash")

        self._agent_kind = agent_kind
        self._binary_path = Path(binary_path)
        self._binary_sha256 = binary_sha256
        if not all(
            (
                binary_bundle_root,
                binary_bundle_manifest_path,
                binary_bundle_tree_sha256,
                binary_bundle_manifest_sha256,
            )
        ):
            raise ValueError("formal agents require the frozen Codex runtime bundle")
        if not _is_sha256(binary_bundle_manifest_sha256) or file_sha256(
            Path(binary_bundle_manifest_path)
        ) != binary_bundle_manifest_sha256:
            raise ValueError("Codex bundle manifest differs from its byte hash")
        self._bundle_root = Path(binary_bundle_root)
        self._bundle = load_bundle_manifest(
            Path(binary_bundle_manifest_path),
            self._bundle_root,
            self._binary_path if agent_kind == "codex" else None,
            binary_sha256 if agent_kind == "codex" else None,
            binary_bundle_tree_sha256,
        )
        if agent_kind == "codex":
            self._uploaded_bundle_files = self._bundle.files
            self._remote_binary = (
                _REMOTE_VENDOR_ROOT + "/" + self._bundle.binary_path
            )
        else:
            self._uploaded_bundle_files = tuple(
                entry
                for entry in self._bundle.files
                if entry.path in _LUBAN_RUNTIME_FILES
            )
            if {entry.path for entry in self._uploaded_bundle_files} != set(
                _LUBAN_RUNTIME_FILES
            ):
                raise ValueError("Codex runtime bundle lacks Luban bwrap or rg")
            self._remote_binary = _REMOTE_SINGLE_BINARY
        self._command_argv = list(command_argv)
        self._proxy_base_url = proxy_base_url
        self._proxy_health_url = proxy_health_url
        self._proxy_host = proxy_host
        self._reasoning_effort = reasoning_effort
        self._base_commit = base_commit
        self._adapter_version = adapter_version
        self._adapter_sha256 = adapter_sha256
        self._source_command_argv_sha256 = source_command_argv_sha256
        self._bundle_manifest_sha256 = binary_bundle_manifest_sha256
        # Pier resolves this mapping from the CLI.  It must contain only the
        # dummy token and non-secret provider configuration.
        self._extra_env = dict(extra_env or {})
        protected_env = {
            "CODEX_HOME",
            "HOME",
            "OPENAI_API",
            "OPENAI_API_KEY",
            "OPENAI_BASE_URL",
            "OPENAI_REASONING_EFFORT",
            "PATH",
            "PROVIDER",
        }
        if protected_env.intersection(self._extra_env):
            raise ValueError("extra_env cannot override the private transport contract")

    @staticmethod
    def name() -> str:
        return "agentic-bench-pinned-cli"

    def version(self) -> str:
        return self._adapter_version

    def network_allowlist(self) -> NetworkAllowlist:
        # DeepSWE declares no-network. Pier's filtered egress proxy grants the
        # controller exactly this one host and nothing else.
        return NetworkAllowlist(domains=[self._proxy_host])

    async def setup(self, environment: BaseEnvironment) -> None:
        initialized = await environment.exec(
            command="rm -rf /opt/agentic-bench && mkdir -p /opt/agentic-bench /logs/agent && chmod 0777 /logs/agent && rm -rf "
            + shlex.quote(_REMOTE_HOME)
            + " && mkdir -p "
            + shlex.quote(_REMOTE_HOME + "/codex")
            + " && chmod 0777 "
            + " ".join(
                map(shlex.quote, (_REMOTE_HOME, _REMOTE_HOME + "/codex"))
            ),
            user="root",
        )
        if initialized.return_code != 0:
            raise RuntimeError("cannot initialize the frozen agent payload")
        if self._agent_kind == "luban":
            await environment.upload_file(self._binary_path, self._remote_binary)
        directories = sorted(
            {
                str(
                    PurePosixPath(_REMOTE_VENDOR_ROOT)
                    / PurePosixPath(entry.path).parent
                )
                for entry in self._uploaded_bundle_files
            }
        )
        created = await environment.exec(
            command="mkdir -p " + " ".join(map(shlex.quote, directories)),
            user="root",
        )
        if created.return_code != 0:
            raise RuntimeError("cannot create the frozen Codex vendor layout")
        for entry in self._uploaded_bundle_files:
            source = self._bundle_root.joinpath(*PurePosixPath(entry.path).parts)
            destination = _REMOTE_VENDOR_ROOT + "/" + entry.path
            await environment.upload_file(source, destination)
        prepared = await environment.exec(
            command=self._payload_verification_command(),
            user="root",
        )
        if prepared.return_code != 0:
            raise RuntimeError("frozen binary or base commit verification failed")

        effective_receipt = self._make_effective_argv_receipt()
        effective_receipt_json = json.dumps(
            effective_receipt, ensure_ascii=True, separators=(",", ":"), sort_keys=True
        )
        if self._proxy_base_url in effective_receipt_json:
            raise RuntimeError("effective argv receipt leaked the private proxy URL")
        (self.logs_dir / "effective-argv.json").write_text(
            effective_receipt_json + "\n", encoding="ascii"
        )
        effective_receipt_sha256 = hashlib.sha256(
            effective_receipt_json.encode("ascii")
        ).hexdigest()

        process_env = self._process_env(environment)
        if self._agent_kind == "codex":
            search_catalog_json = json.dumps(
                _CODEX_WEB_SEARCH_CANARY_CATALOG,
                ensure_ascii=True,
                separators=(",", ":"),
                sort_keys=True,
            )
            search_catalog_path = (
                self.logs_dir / "codex-web-search-canary-model-catalog.json"
            )
            search_catalog_path.write_text(
                search_catalog_json + "\n", encoding="ascii"
            )
            await environment.upload_file(
                search_catalog_path, _CODEX_WEB_SEARCH_CANARY_CATALOG_REMOTE
            )
            self._codex_web_search_catalog_sha256 = hashlib.sha256(
                (search_catalog_json + "\n").encode("ascii")
            ).hexdigest()
        if self._agent_kind == "luban":
            git_ready = await environment.exec(
                command="git config --global --add safe.directory /app",
                cwd=_WORKSPACE,
                env=process_env,
                user=self._execution_user(),
                timeout_sec=15,
            )
            if git_ready.return_code != 0:
                raise RuntimeError("cannot bind Luban's non-root Git workspace")
        await self._require_controller_network(environment, process_env)
        await self._require_tool_network_denial(environment, process_env)

        receipt = {
            "schema_version": _SANDBOX_CANARY_V4_SCHEMA,
            "agent_kind": self._agent_kind,
            "binary_sha256": self._binary_sha256,
            "base_commit": self._base_commit,
            "controller_proxy_reachable": True,
            "tool_proxy_reachable": False,
            "credential_in_agent": False,
            "adapter_sha256": self._adapter_sha256,
            "bundle_manifest_sha256": self._bundle_manifest_sha256,
            "effective_argv_receipt_sha256": effective_receipt_sha256,
        }
        receipt["source_bundle_tree_sha256"] = self._bundle.tree_sha256
        receipt["runtime_payload_tree_sha256"] = _canonical_bundle_tree(
            self._uploaded_bundle_files
        )
        audit_name = (
            "codex-sandbox-canary-request.jsonl"
            if self._agent_kind == "codex"
            else "luban-sandbox-canary-request.jsonl"
        )
        audit_path = self.logs_dir / audit_name
        provider_requests = [
            _strict_json_object(line, f"{self._agent_kind} provider canary request")
            for line in audit_path.read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]
        expected_provider_request_count = 2 if self._agent_kind == "codex" else 3
        if (
            len(provider_requests) != expected_provider_request_count
            or any(request.get("transport") != "http_sse" for request in provider_requests)
            or any(request.get("prewarm_requested") is not False for request in provider_requests)
            or any(request.get("websocket_upgrade_count_before_request") != 0 for request in provider_requests)
        ):
            raise RuntimeError("provider canary is not HTTP-inference-only")
        receipt["provider_canary_requests"] = provider_requests
        receipt["provider_canary_transport"] = _HTTP_INFERENCE_TRANSPORT
        receipt["http_transport"] = {
            "schema_version": "agentic-bench/http-inference-transport-v1",
            "requirement": _HTTP_INFERENCE_REQUIREMENT,
            "http_inference_request_count": len(provider_requests),
            "websocket_upgrade_request_count": 0,
            "websocket_generation_frame_count": 0,
            "prewarm_request_count": 0,
        }
        receipt["cache_wire"] = _summarize_content_free_cache_requests(
            provider_requests
        )
        if self._agent_kind == "codex":
            negative_audit_path = (
                self.logs_dir
                / "codex-sandbox-negative-control-request.jsonl"
            )
            negative_receipt_path = (
                self.logs_dir / "codex-sandbox-negative-control.json"
            )
            if (
                [
                    request.get("custom_tool_output_count")
                    for request in provider_requests
                ]
                != [0, 1]
            ):
                raise RuntimeError("Codex HTTP canary duplicated a generation")
            receipt["sandbox_negative_control"] = {
                **_strict_json_object(
                    negative_receipt_path.read_bytes(),
                    "Codex sandbox negative-control receipt",
                ),
                "provider_canary_requests": [
                    _strict_json_object(
                        line, "Codex sandbox negative-control provider request"
                    )
                    for line in negative_audit_path.read_text(
                        encoding="utf-8"
                    ).splitlines()
                    if line.strip()
                ],
            }
            receipt["canonical_authority"] = {
                "generation": "v8",
                "authority_scope": _CODEX_V8_CANONICAL_AUTHORITY_SCOPE,
                "responses_transport_requirement": _HTTP_INFERENCE_REQUIREMENT,
            }
            receipt["web_search_configuration_canary"] = _strict_json_object(
                (
                    self.logs_dir
                    / "codex-web-search-configuration-canary.json"
                ).read_bytes(),
                "Codex web-search configuration canary",
            )
            receipt["workspace_state"] = _strict_json_object(
                (
                    self.logs_dir / "codex-sandbox-workspace-state.json"
                ).read_bytes(),
                "Codex sandbox workspace state",
            )
        encoded_receipt = json.dumps(
            receipt, ensure_ascii=True, separators=(",", ":"), sort_keys=True
        )
        _decode_sandbox_canary_v4(
            encoded_receipt,
            expected_agent_kind=self._agent_kind,
            allow_pending_authority=True,
        )
        (self.logs_dir / "sandbox-canary.json").write_text(
            encoded_receipt + "\n", encoding="ascii"
        )

    def _payload_verification_command(self) -> str:
        commands = ["set -eu;"]
        if self._agent_kind == "luban":
            commands.extend(
                [
                    "chown 0:0 " + shlex.quote(self._remote_binary) + ";",
                    "chmod 0555 " + shlex.quote(self._remote_binary) + ";",
                    "test \"$(sha256sum "
                    + shlex.quote(self._remote_binary)
                    + " | cut -d' ' -f1)\" = "
                    + shlex.quote(self._binary_sha256)
                    + ";",
                ]
            )
        for entry in self._uploaded_bundle_files:
            remote = _REMOTE_VENDOR_ROOT + "/" + entry.path
            commands.extend(
                [
                    "chown 0:0 " + shlex.quote(remote) + ";",
                    "chmod " + entry.mode + " " + shlex.quote(remote) + ";",
                    "test ! -L " + shlex.quote(remote) + ";",
                    "test -f " + shlex.quote(remote) + ";",
                    "test \"$(stat -c %s "
                    + shlex.quote(remote)
                    + ")\" = "
                    + str(entry.size)
                    + ";",
                    "test \"$(stat -c %a "
                    + shlex.quote(remote)
                    + ")\" = "
                    + shlex.quote(entry.mode[1:])
                    + ";",
                    "test \"$(stat -c %u:%g "
                    + shlex.quote(remote)
                    + ")\" = 0:0;",
                    "test \"$(sha256sum "
                    + shlex.quote(remote)
                    + " | cut -d' ' -f1)\" = "
                    + shlex.quote(entry.sha256)
                    + ";",
                ]
            )
        commands.extend(
            [
                "test \"$(find "
                + shlex.quote(_REMOTE_VENDOR_ROOT)
                + " -type f | wc -l)\" -eq "
                + str(len(self._uploaded_bundle_files))
                + ";",
                "test \"$(find "
                + shlex.quote(_REMOTE_VENDOR_ROOT)
                + " -type l | wc -l)\" -eq 0;",
                "tree=$({",
            ]
        )
        for entry in self._uploaded_bundle_files:
            commands.append(
                "printf '%s\\000%s\\000%s\\000%s\\n' "
                + " ".join(
                    map(
                        shlex.quote,
                        (
                            entry.path,
                            entry.mode,
                            str(entry.size),
                            entry.sha256,
                        ),
                    )
                )
                + ";"
            )
        commands.extend(
            [
                "} | sha256sum | cut -d' ' -f1);",
                "test \"$tree\" = "
                + shlex.quote(_canonical_bundle_tree(self._uploaded_bundle_files))
                + ";",
            ]
        )
        if self._agent_kind == "luban":
            commands.extend(
                [
                    shlex.quote(_REMOTE_VENDOR_ROOT + "/" + _LUBAN_RUNTIME_RG)
                    + " --version >/dev/null;",
                    shlex.quote(
                        _REMOTE_VENDOR_ROOT + "/" + _LUBAN_RUNTIME_BWRAP
                    )
                    + " --version >/dev/null;",
                ]
            )
        commands.extend(
            [
                "chmod 0777 " + shlex.quote(_REMOTE_HOME) + ";",
                "test \"$(git -C /app rev-parse HEAD)\" = "
                + shlex.quote(self._base_commit),
            ]
        )
        return " ".join(commands)

    def _process_env(self, environment: BaseEnvironment) -> dict[str, str]:
        env = {
            "HOME": _REMOTE_HOME,
            "CODEX_HOME": f"{_REMOTE_HOME}/codex",
            "OPENAI_API_KEY": "agentic-bench-dummy-token",
            "OPENAI_BASE_URL": self._proxy_base_url,
            "OPENAI_API": "responses",
            "OPENAI_REASONING_EFFORT": self._reasoning_effort,
            "PROVIDER": "openai",
            "NO_COLOR": "1",
            **self._extra_env,
        }
        if self._agent_kind == "luban":
            env["PATH"] = ":".join(
                [
                    _REMOTE_VENDOR_ROOT
                    + "/"
                    + str(PurePosixPath(_LUBAN_RUNTIME_BWRAP).parent),
                    _REMOTE_VENDOR_ROOT
                    + "/"
                    + str(PurePosixPath(_LUBAN_RUNTIME_RG).parent),
                    "/usr/local/sbin",
                    "/usr/local/bin",
                    "/usr/sbin",
                    "/usr/bin",
                    "/sbin",
                    "/bin",
                ]
            )
        # Pier adds its ephemeral HTTP(S)_PROXY credential here. This channel
        # is required by the controller but the nested tool sandbox removes
        # network access entirely.
        return environment.agent_process_env(env) or env

    def _execution_user(self) -> str | None:
        # Luban's immutable executable authority intentionally rejects bwrap
        # when the caller itself can rewrite root-owned system paths.
        return "65534:65534" if self._agent_kind == "luban" else None

    @staticmethod
    def _network_probe(url: str, *, expect_failure: bool) -> str:
        source = (
            "import sys,urllib.request;"
            "u=sys.argv[1];"
            "urllib.request.urlopen(u,timeout=5).read();"
            "print('reachable')"
        )
        command = "python3 -c " + shlex.quote(source) + " " + shlex.quote(url)
        if expect_failure:
            return "if " + command + "; then exit 91; else exit 0; fi"
        return command

    async def _require_controller_network(
        self, environment: BaseEnvironment, process_env: dict[str, str]
    ) -> None:
        result = await environment.exec(
            command=self._network_probe(self._proxy_health_url, expect_failure=False),
            cwd=_WORKSPACE,
            env=process_env,
            user=self._execution_user(),
            timeout_sec=15,
        )
        if result.return_code != 0:
            raise RuntimeError("agent controller cannot reach the private evidence proxy")

    async def _require_tool_network_denial(
        self, environment: BaseEnvironment, process_env: dict[str, str]
    ) -> None:
        if self._agent_kind == "codex":
            await self._require_codex_exec_sandbox(environment, process_env)
            return
        await self._require_luban_tool_sandbox(environment, process_env)

    def _codex_canary_argv(self, port: int, sandbox_mode: str) -> list[str]:
        if not 1 <= port <= 65535:
            raise ValueError("Codex canary port is invalid")
        if sandbox_mode not in {"workspace-write", "danger-full-access"}:
            raise ValueError("Codex canary sandbox mode is invalid")
        provider = _CODEX_HTTP_PROVIDER
        return [
            self._remote_binary,
            "--strict-config",
            "--ask-for-approval",
            "never",
            "--sandbox",
            sandbox_mode,
            "exec",
            "--json",
            "--ephemeral",
            "--ignore-user-config",
            "--model",
            "gpt-5.6-sol",
            "--config",
            "model_reasoning_effort=xhigh",
            "--config",
            _CODEX_SERVICE_TIER_DEFAULT_CONFIG,
            "--config",
            _CODEX_WEB_SEARCH_DISABLED_CONFIG,
            "--config",
            _CODEX_AGENTS_DISABLED_CONFIG,
            "--config",
            f'model_provider="{provider}"',
            "--config",
            _CODEX_HTTP_PROVIDER_CONFIG_TOKEN.replace(
                "{provider_base_url}", f"http://127.0.0.1:{port}/v1"
            ),
            "--config",
            f"model_providers.{provider}.request_max_retries=0",
            "--config",
            f"model_providers.{provider}.stream_max_retries=0",
            "--cd",
            _WORKSPACE,
            "agentic-bench sandbox canary",
        ]

    def _codex_web_search_canary_argv(
        self,
        port: int,
        *,
        web_search_disabled: bool,
        agents_disabled: bool,
        service_tier: str,
    ) -> list[str]:
        if not 1 <= port <= 65535:
            raise ValueError("Codex web-search canary port is invalid")
        service_tier_configs = {
            "default": _CODEX_SERVICE_TIER_DEFAULT_CONFIG,
            "priority": _CODEX_SERVICE_TIER_PRIORITY_CONFIG,
        }
        if service_tier not in service_tier_configs:
            raise ValueError("Codex web-search canary service tier is invalid")
        provider = "agentic-canary"
        argv = [
            self._remote_binary,
            "--strict-config",
            "--ask-for-approval",
            "never",
            "--sandbox",
            "workspace-write",
            "exec",
            "--json",
            "--ephemeral",
            "--ignore-user-config",
            "--model",
            "gpt-5.6-sol",
            "--config",
            "model_reasoning_effort=xhigh",
        ]
        argv.extend(["--config", service_tier_configs[service_tier]])
        if web_search_disabled:
            argv.extend(["--config", _CODEX_WEB_SEARCH_DISABLED_CONFIG])
        if agents_disabled:
            argv.extend(["--config", _CODEX_AGENTS_DISABLED_CONFIG])
        argv.extend(
            [
                "--config",
                f'model_catalog_json="{_CODEX_WEB_SEARCH_CANARY_CATALOG_REMOTE}"',
                "--config",
                f'model_provider="{provider}"',
                "--config",
                f'model_providers.{provider}.name="OpenAI"',
                "--config",
                f'model_providers.{provider}.base_url="http://127.0.0.1:{port}/v1"',
                "--config",
                f'model_providers.{provider}.env_key="OPENAI_API_KEY"',
                "--config",
                f'model_providers.{provider}.wire_api="responses"',
                "--config",
                f"model_providers.{provider}.supports_websockets=false",
                "--config",
                f"model_providers.{provider}.request_max_retries=0",
                "--config",
                f"model_providers.{provider}.stream_max_retries=0",
                "--cd",
                _WORKSPACE,
                "agentic-bench web search fairness canary",
            ]
        )
        return argv

    async def _require_codex_web_search_disabled(
        self, environment: BaseEnvironment, process_env: dict[str, str]
    ) -> None:
        ready = "/tmp/agentic-bench-codex-web-search-canary.port"
        pid = "/tmp/agentic-bench-codex-web-search-canary.pid"
        request_audit = (
            f"{_AGENT_LOGS}/codex-web-search-configuration-canary-request.jsonl"
        )
        server_log = f"{_AGENT_LOGS}/codex-web-search-configuration-canary-server.log"
        positive_stream = (
            f"{_AGENT_LOGS}/codex-web-search-configuration-canary.stream.jsonl"
        )
        positive_stderr = (
            f"{_AGENT_LOGS}/codex-web-search-configuration-canary.stderr.log"
        )
        negative_stream = (
            f"{_AGENT_LOGS}/codex-web-search-negative-control.stream.jsonl"
        )
        negative_stderr = (
            f"{_AGENT_LOGS}/codex-web-search-negative-control.stderr.log"
        )
        agents_negative_stream = (
            f"{_AGENT_LOGS}/codex-agents-negative-control.stream.jsonl"
        )
        agents_negative_stderr = (
            f"{_AGENT_LOGS}/codex-agents-negative-control.stderr.log"
        )
        service_negative_stream = (
            f"{_AGENT_LOGS}/codex-service-tier-negative-control.stream.jsonl"
        )
        service_negative_stderr = (
            f"{_AGENT_LOGS}/codex-service-tier-negative-control.stderr.log"
        )
        start_server = (
            "rm -f "
            + " ".join(map(shlex.quote, (ready, pid, request_audit)))
            + "; python3 -c "
            + shlex.quote(codex_web_search_canary_server_source())
            + " "
            + shlex.quote(ready)
            + " "
            + shlex.quote(request_audit)
            + " >"
            + shlex.quote(server_log)
            + " 2>&1 & echo $! >"
            + shlex.quote(pid)
        )
        started = await environment.exec(
            command=start_server,
            cwd=_WORKSPACE,
            env=process_env,
            timeout_sec=15,
        )
        if started.return_code != 0:
            raise RuntimeError("cannot start the Codex web-search config canary")
        try:
            port_result = await environment.exec(
                command=(
                    "i=0; while [ $i -lt 100 ]; do if test -s "
                    + shlex.quote(ready)
                    + "; then cat "
                    + shlex.quote(ready)
                    + "; exit 0; fi; i=$((i+1)); sleep 0.05; done; exit 1"
                ),
                timeout_sec=10,
            )
            port_text = (port_result.stdout or "").strip()
            if port_result.return_code != 0 or not port_text.isdigit():
                raise RuntimeError("Codex web-search config canary did not start")
            port = int(port_text)
            canary_env = dict(process_env)
            canary_env["NO_PROXY"] = "127.0.0.1,localhost"
            canary_env["no_proxy"] = "127.0.0.1,localhost"
            positive_argv = self._codex_web_search_canary_argv(
                port,
                web_search_disabled=True,
                agents_disabled=True,
                service_tier="default",
            )
            negative_argv = self._codex_web_search_canary_argv(
                port,
                web_search_disabled=False,
                agents_disabled=True,
                service_tier="default",
            )
            agents_negative_argv = self._codex_web_search_canary_argv(
                port,
                web_search_disabled=True,
                agents_disabled=False,
                service_tier="default",
            )
            service_negative_argv = self._codex_web_search_canary_argv(
                port,
                web_search_disabled=True,
                agents_disabled=True,
                service_tier="priority",
            )
            counterfactual_argv = list(positive_argv)
            config_index = counterfactual_argv.index(
                _CODEX_WEB_SEARCH_DISABLED_CONFIG
            )
            if config_index == 0 or counterfactual_argv[config_index - 1] != "--config":
                raise RuntimeError("Codex web-search config is not a formal argv pair")
            del counterfactual_argv[config_index - 1 : config_index + 1]
            if counterfactual_argv != negative_argv:
                raise RuntimeError(
                    "Codex web-search counterfactual changed more than one config"
                )
            agents_counterfactual_argv = list(positive_argv)
            agents_config_index = agents_counterfactual_argv.index(
                _CODEX_AGENTS_DISABLED_CONFIG
            )
            if (
                agents_config_index == 0
                or agents_counterfactual_argv[agents_config_index - 1] != "--config"
            ):
                raise RuntimeError("Codex agents config is not a formal argv pair")
            del agents_counterfactual_argv[
                agents_config_index - 1 : agents_config_index + 1
            ]
            if agents_counterfactual_argv != agents_negative_argv:
                raise RuntimeError(
                    "Codex agents counterfactual changed more than one config"
                )
            service_counterfactual_argv = list(positive_argv)
            service_config_index = service_counterfactual_argv.index(
                _CODEX_SERVICE_TIER_DEFAULT_CONFIG
            )
            if (
                service_config_index == 0
                or service_counterfactual_argv[service_config_index - 1]
                != "--config"
            ):
                raise RuntimeError("Codex service tier is not a formal argv pair")
            service_counterfactual_argv[
                service_config_index
            ] = _CODEX_SERVICE_TIER_PRIORITY_CONFIG
            if service_counterfactual_argv != service_negative_argv:
                raise RuntimeError(
                    "Codex service-tier counterfactual changed more than one value"
                )

            positive_result = await environment.exec(
                command=(
                    shlex.join(positive_argv)
                    + " >"
                    + shlex.quote(positive_stream)
                    + " 2>"
                    + shlex.quote(positive_stderr)
                ),
                cwd=_WORKSPACE,
                env=canary_env,
                timeout_sec=90,
            )
            if positive_result.return_code != 0:
                raise RuntimeError("Codex rejected web_search disabled config")
            negative_result = await environment.exec(
                command=(
                    shlex.join(negative_argv)
                    + " >"
                    + shlex.quote(negative_stream)
                    + " 2>"
                    + shlex.quote(negative_stderr)
                ),
                cwd=_WORKSPACE,
                env=canary_env,
                timeout_sec=90,
            )
            if negative_result.return_code == 0:
                raise RuntimeError(
                    "Codex web-search config negative control did not fail closed"
                )
            agents_negative_result = await environment.exec(
                command=(
                    shlex.join(agents_negative_argv)
                    + " >"
                    + shlex.quote(agents_negative_stream)
                    + " 2>"
                    + shlex.quote(agents_negative_stderr)
                ),
                cwd=_WORKSPACE,
                env=canary_env,
                timeout_sec=90,
            )
            if agents_negative_result.return_code == 0:
                raise RuntimeError(
                    "Codex agents config negative control did not fail closed"
                )
            service_negative_result = await environment.exec(
                command=(
                    shlex.join(service_negative_argv)
                    + " >"
                    + shlex.quote(service_negative_stream)
                    + " 2>"
                    + shlex.quote(service_negative_stderr)
                ),
                cwd=_WORKSPACE,
                env=canary_env,
                timeout_sec=90,
            )
            if service_negative_result.return_code == 0:
                raise RuntimeError(
                    "Codex service-tier counterfactual did not fail closed"
                )
            audited = await environment.exec(
                command=(
                    "test -f "
                    + shlex.quote(request_audit)
                    + " && test \"$(wc -l < "
                    + shlex.quote(request_audit)
                    + ")\" -eq 4"
                ),
                timeout_sec=15,
            )
            if audited.return_code != 0:
                raise RuntimeError("Codex web-search request audit is incomplete")
        finally:
            await environment.exec(
                command=(
                    "if test -s "
                    + shlex.quote(pid)
                    + "; then kill \"$(cat "
                    + shlex.quote(pid)
                    + ")\" 2>/dev/null || true; fi; rm -f "
                    + " ".join(map(shlex.quote, (ready, pid)))
                ),
                user="root",
                timeout_sec=15,
            )

        requests = [
            json.loads(line)
            for line in (
                self.logs_dir
                / "codex-web-search-configuration-canary-request.jsonl"
            )
            .read_text(encoding="utf-8")
            .splitlines()
            if line.strip()
        ]
        if (
            len(requests) != 4
            or requests[0].get("request_index") != 0
            or requests[0].get("web_search_tool_count") != 0
            or requests[0].get("subagent_tool_present") is not False
            or requests[0].get("request_service_tier_present") is not False
            or requests[0].get("request_service_tier") is not None
            or requests[0].get("configuration_accepted") is not True
            or requests[1].get("request_index") != 1
            or requests[1].get("web_search_tool_count") != 1
            or requests[1].get("web_search_external_access") != [False]
            or requests[1].get("subagent_tool_present") is not False
            or requests[1].get("request_service_tier_present") is not False
            or requests[1].get("request_service_tier") is not None
            or requests[1].get("configuration_accepted") is not False
            or requests[2].get("request_index") != 2
            or requests[2].get("web_search_tool_count") != 0
            or requests[2].get("collaboration_namespace_present") is not False
            or requests[2].get("multi_agent_namespace_present") is not True
            or requests[2].get("subagent_tool_present") is not True
            or requests[2].get("request_service_tier_present") is not False
            or requests[2].get("request_service_tier") is not None
            or requests[2].get("configuration_accepted") is not False
            or requests[3].get("request_index") != 3
            or requests[3].get("web_search_tool_count") != 0
            or requests[3].get("subagent_tool_present") is not False
            or requests[3].get("request_service_tier_present") is not True
            or requests[3].get("request_service_tier") != "priority"
            or requests[3].get("configuration_accepted") is not False
        ):
            raise RuntimeError("Codex fairness-config wire counterfactual is invalid")
        receipt = {
            "schema_version": "agentic-bench/fairness-configuration-canary-v2",
            "provider_transport": "responses-http-sse-standard-diagnostic",
            "model": "gpt-5.6-sol",
            "reasoning_effort": "xhigh",
            "effective_config": _CODEX_WEB_SEARCH_DISABLED_CONFIG,
            "agents_effective_config": _CODEX_AGENTS_DISABLED_CONFIG,
            "service_tier_effective_config": _CODEX_SERVICE_TIER_DEFAULT_CONFIG,
            "service_tier_default_wire_encoding": "omitted",
            "service_tier_default_source": "client_canonicalized_default",
            "model_catalog_sha256": self._codex_web_search_catalog_sha256,
            "positive": {
                "effective_argv_sha256": _argv_sha256(positive_argv),
                "expected_cli_exit_code": 0,
                "actual_cli_exit_code": positive_result.return_code,
                "valid_receipt_emitted": True,
                "request": requests[0],
            },
            "negative_control": {
                "config_removed": _CODEX_WEB_SEARCH_DISABLED_CONFIG,
                "only_removed_config": True,
                "effective_argv_sha256": _argv_sha256(negative_argv),
                "counterfactual_argv_sha256": _argv_sha256(counterfactual_argv),
                "expected_cli_exit_code": "nonzero",
                "actual_cli_exit_code": negative_result.return_code,
                "valid_receipt_emitted": False,
                "request": requests[1],
            },
            "agents_negative_control": {
                "config_removed": _CODEX_AGENTS_DISABLED_CONFIG,
                "only_removed_config": True,
                "effective_argv_sha256": _argv_sha256(agents_negative_argv),
                "counterfactual_argv_sha256": _argv_sha256(
                    agents_counterfactual_argv
                ),
                "expected_cli_exit_code": "nonzero",
                "actual_cli_exit_code": agents_negative_result.return_code,
                "valid_receipt_emitted": False,
                "request": requests[2],
            },
            "service_tier_negative_control": {
                "config_replaced": _CODEX_SERVICE_TIER_DEFAULT_CONFIG,
                "replacement_config": _CODEX_SERVICE_TIER_PRIORITY_CONFIG,
                "only_replaced_config_value": True,
                "effective_argv_sha256": _argv_sha256(service_negative_argv),
                "counterfactual_argv_sha256": _argv_sha256(
                    service_counterfactual_argv
                ),
                "expected_cli_exit_code": "nonzero",
                "actual_cli_exit_code": service_negative_result.return_code,
                "valid_receipt_emitted": False,
                "request": requests[3],
            },
        }
        (self.logs_dir / "codex-web-search-configuration-canary.json").write_text(
            json.dumps(receipt, sort_keys=True) + "\n", encoding="utf-8"
        )

    async def _run_codex_exec_canary(
        self,
        environment: BaseEnvironment,
        process_env: dict[str, str],
        *,
        label: str,
        marker: str,
        sandbox_mode: str,
        expected_exit: int,
    ) -> None:
        ready = f"/tmp/agentic-bench-{label}.port"
        pid = f"/tmp/agentic-bench-{label}.pid"
        request_audit = f"{_AGENT_LOGS}/{label}-request.jsonl"
        server_log = f"{_AGENT_LOGS}/{label}-server.log"
        stream_log = f"{_AGENT_LOGS}/{label}.stream.jsonl"
        stderr_log = f"{_AGENT_LOGS}/{label}.stderr.log"
        network_probe = self._network_probe(
            self._proxy_health_url, expect_failure=False
        )
        tool_command = (
            "if "
            + network_probe
            + " >/dev/null 2>&1; then exit 91; fi; printf '%s\\n' "
            + shlex.quote("agentic-bench-sandbox-write")
            + " > "
            + shlex.quote(marker)
        )
        start_server = (
            "rm -f "
            + " ".join(map(shlex.quote, (ready, pid, marker, request_audit)))
            + "; python3 -c "
            + shlex.quote(codex_canary_server_source())
            + " "
            + shlex.quote(tool_command)
            + " "
            + str(expected_exit)
            + " "
            + shlex.quote(ready)
            + " "
            + shlex.quote(request_audit)
            + " >"
            + shlex.quote(server_log)
            + " 2>&1 & echo $! >"
            + shlex.quote(pid)
        )
        started = await environment.exec(
            command=start_server,
            cwd=_WORKSPACE,
            env=process_env,
            timeout_sec=15,
        )
        if started.return_code != 0:
            raise RuntimeError("cannot start the local Codex sandbox canary provider")
        try:
            port_result = await environment.exec(
                command=(
                    "i=0; while [ $i -lt 100 ]; do if test -s "
                    + shlex.quote(ready)
                    + "; then cat "
                    + shlex.quote(ready)
                    + "; exit 0; fi; i=$((i+1)); sleep 0.05; done; exit 1"
                ),
                timeout_sec=10,
            )
            port_text = (port_result.stdout or "").strip()
            if port_result.return_code != 0 or not port_text.isdigit():
                raise RuntimeError("local Codex sandbox canary provider did not start")
            port = int(port_text)
            canary_env = dict(process_env)
            # The controller reaches the loopback fake provider directly.  A
            # danger-full-access tool retains Pier's HTTP proxy and therefore
            # reaches the private health endpoint; workspace-write must not.
            canary_env["NO_PROXY"] = "127.0.0.1,localhost"
            canary_env["no_proxy"] = "127.0.0.1,localhost"
            canary_result = await environment.exec(
                command=(
                    shlex.join(self._codex_canary_argv(port, sandbox_mode))
                    + " >"
                    + shlex.quote(stream_log)
                    + " 2>"
                    + shlex.quote(stderr_log)
                ),
                cwd=_WORKSPACE,
                env=canary_env,
                timeout_sec=90,
            )
            if canary_result.return_code != 0:
                raise RuntimeError("Codex failed its real exec sandbox canary")
            audited = await environment.exec(
                command=(
                    "test -f "
                    + shlex.quote(request_audit)
                    + " && test \"$(wc -l < "
                    + shlex.quote(request_audit)
                    + ")\" -eq 2"
                ),
                timeout_sec=15,
            )
            if audited.return_code != 0:
                raise RuntimeError("Codex sandbox canary request audit is incomplete")
        finally:
            await environment.exec(
                command=(
                    "if test -s "
                    + shlex.quote(pid)
                    + "; then kill \"$(cat "
                    + shlex.quote(pid)
                    + ")\" 2>/dev/null || true; fi; rm -f "
                    + " ".join(map(shlex.quote, (ready, pid)))
                ),
                user="root",
                timeout_sec=15,
            )

    async def _require_codex_exec_sandbox(
        self, environment: BaseEnvironment, process_env: dict[str, str]
    ) -> None:
        positive_marker = f"{_WORKSPACE}/.agentic-bench-codex-sandbox-canary"
        negative_marker = f"{_WORKSPACE}/.agentic-bench-codex-negative-canary"
        await self._run_codex_exec_canary(
            environment,
            process_env,
            label="codex-sandbox-canary",
            marker=positive_marker,
            sandbox_mode="workspace-write",
            expected_exit=0,
        )
        positive = await environment.exec(
            command=(
                "test \"$(cat "
                + shlex.quote(positive_marker)
                + ")\" = agentic-bench-sandbox-write && rm -f "
                + shlex.quote(positive_marker)
                + " && git diff --quiet && test -z \"$(git ls-files --others --exclude-standard)\""
            ),
            cwd=_WORKSPACE,
            user="root",
            timeout_sec=15,
        )
        if positive.return_code != 0:
            raise RuntimeError("Codex sandbox canary did not prove an isolated write")

        # Fail-closed counterfactual: the identical generated shell command
        # runs with the sandbox disabled.  It must reach the controller-only
        # endpoint, exit 91 before the write, and never produce a valid marker.
        await self._run_codex_exec_canary(
            environment,
            process_env,
            label="codex-sandbox-negative-control",
            marker=negative_marker,
            sandbox_mode="danger-full-access",
            expected_exit=91,
        )
        negative = await environment.exec(
            command=(
                "test ! -e "
                + shlex.quote(negative_marker)
                + " && git diff --quiet && test -z \"$(git ls-files --others --exclude-standard)\""
            ),
            cwd=_WORKSPACE,
            user="root",
            timeout_sec=15,
        )
        if negative.return_code != 0:
            raise RuntimeError("Codex sandbox negative control did not fail closed")
        (self.logs_dir / "codex-sandbox-negative-control.json").write_text(
            json.dumps(
                {
                    "schema_version": "agentic-bench/sandbox-negative-control-v1",
                    "sandbox_policy": "danger-full-access",
                    "expected_tool_exit_code": 91,
                    "marker_written": False,
                    "valid_sandbox_receipt_emitted": False,
                },
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )
        await self._require_codex_web_search_disabled(environment, process_env)

        workspace_state = await environment.exec(
            command=(
                "set -eu; "
                "test ! -e "
                + shlex.quote(positive_marker)
                + "; test ! -e "
                + shlex.quote(negative_marker)
                + "; git diff --quiet; git diff --cached --quiet; "
                "test \"$(git status --porcelain=v1 --untracked-files=all | wc -c)\" -eq 0; "
                "git rev-parse HEAD; git ls-files --stage | sha256sum | cut -d' ' -f1; "
                "git status --porcelain=v1 -z --untracked-files=all | sha256sum | cut -d' ' -f1"
            ),
            cwd=_WORKSPACE,
            user="root",
            timeout_sec=15,
        )
        state_lines = (workspace_state.stdout or "").splitlines()
        if (
            workspace_state.return_code != 0
            or len(state_lines) != 3
            or state_lines[0] != self._base_commit
            or not _is_sha256(state_lines[1])
            or state_lines[2] != hashlib.sha256(b"").hexdigest()
        ):
            raise RuntimeError("Codex canaries changed the frozen Git workspace")
        (self.logs_dir / "codex-sandbox-workspace-state.json").write_text(
            json.dumps(
                {
                    "schema_version": "agentic-bench/sandbox-workspace-state-v1",
                    "head": state_lines[0],
                    "expected_base_commit": self._base_commit,
                    "head_matches_base_commit": True,
                    "index_entries_sha256": state_lines[1],
                    "index_matches_head": True,
                    "tracked_worktree_matches_index": True,
                    "status_porcelain_v1_z_sha256": state_lines[2],
                    "status_entry_count": 0,
                    "positive_marker_absent": True,
                    "negative_marker_absent": True,
                },
                sort_keys=True,
            )
            + "\n",
            encoding="utf-8",
        )

    async def _require_luban_tool_sandbox(
        self,
        environment: BaseEnvironment,
        process_env: dict[str, str],
    ) -> None:
        security_help = await environment.exec(
            command=shlex.quote(self._remote_binary) + " --help",
            env=process_env,
            user=self._execution_user(),
            timeout_sec=15,
        )
        if security_help.return_code != 0 or "force-sandbox-tools" not in (
            (security_help.stdout or "") + (security_help.stderr or "")
        ):
            raise RuntimeError("Luban binary lacks --force-sandbox-tools")

        marker = f"{_WORKSPACE}/.agentic-bench-luban-sandbox-canary"
        ready = "/tmp/agentic-bench-luban-canary.port"
        pid = "/tmp/agentic-bench-luban-canary.pid"
        server_log = f"{_AGENT_LOGS}/luban-sandbox-canary-server.log"
        request_audit = f"{_AGENT_LOGS}/luban-sandbox-canary-request.jsonl"
        proxy = urlparse(self._proxy_health_url)
        if proxy.hostname is None or proxy.port is None:
            raise RuntimeError("Luban sandbox canary proxy endpoint is incomplete")
        tcp_probe = f"/dev/tcp/{proxy.hostname}/{proxy.port}"
        tool_command = (
            "grep -qx agentic-bench-sandbox-write "
            + shlex.quote(marker)
            + " && ! head -c 0 < "
            + shlex.quote(tcp_probe)
        )
        start_server = (
            "rm -f "
            + " ".join(map(shlex.quote, (ready, pid, marker, request_audit)))
            + "; python3 -c "
            + shlex.quote(luban_canary_server_source())
            + " "
            + shlex.quote(tool_command)
            + " "
            + shlex.quote(ready)
            + " "
            + shlex.quote(request_audit)
            + " >"
            + shlex.quote(server_log)
            + " 2>&1 & echo $! >"
            + shlex.quote(pid)
        )
        started = await environment.exec(
            command=start_server,
            cwd=_WORKSPACE,
            env=process_env,
            user=self._execution_user(),
            timeout_sec=15,
        )
        if started.return_code != 0:
            raise RuntimeError("cannot start the local Luban sandbox canary provider")
        try:
            port_result = await environment.exec(
                command=(
                    "i=0; while [ $i -lt 100 ]; do "
                    + "if test -s "
                    + shlex.quote(ready)
                    + "; then cat "
                    + shlex.quote(ready)
                    + "; exit 0; fi; i=$((i+1)); sleep 0.05; done; exit 1"
                ),
                user=self._execution_user(),
                timeout_sec=10,
            )
            port = (port_result.stdout or "").strip()
            if port_result.return_code != 0 or not port.isdigit():
                raise RuntimeError("local Luban sandbox canary provider did not start")
            canary_env = dict(process_env)
            canary_env["OPENAI_BASE_URL"] = f"http://127.0.0.1:{port}/v1"
            for name in (
                "HTTP_PROXY",
                "HTTPS_PROXY",
                "ALL_PROXY",
                "http_proxy",
                "https_proxy",
                "all_proxy",
            ):
                canary_env[name] = ""
            canary_env["NO_PROXY"] = "127.0.0.1,localhost"
            canary_env["no_proxy"] = "127.0.0.1,localhost"
            canary_argv = [
                self._remote_binary,
                "--print",
                "--output-format",
                "stream-json",
                "--provider",
                "openai",
                "--api",
                "responses",
                "--model",
                "gpt-5.6-sol",
                "--reasoning-effort",
                self._reasoning_effort,
                "--service-tier",
                "default",
                "--pinned-model",
                "--no-model-fallback",
                "--allow-all",
                "--force-sandbox-tools",
                "--max-turns",
                "3",
                "agentic-bench sandbox canary",
            ]
            canary_result = await environment.exec(
                command=shlex.join(canary_argv)
                + " >"
                + shlex.quote(f"{_AGENT_LOGS}/luban-sandbox-canary.stream.jsonl")
                + " 2>"
                + shlex.quote(f"{_AGENT_LOGS}/luban-sandbox-canary.stderr.log"),
                cwd=_WORKSPACE,
                env=canary_env,
                user=self._execution_user(),
                timeout_sec=90,
            )
            if canary_result.return_code != 0:
                raise RuntimeError("Luban failed its real force-sandbox-tools canary")
            verified = await environment.exec(
                command=(
                    "test \"$(cat "
                    + shlex.quote(marker)
                    + ")\" = agentic-bench-sandbox-write && rm -f "
                    + shlex.quote(marker)
                    + " && git diff --quiet && test -z \"$(git ls-files --others --exclude-standard)\""
                ),
                cwd=_WORKSPACE,
                user="root",
                timeout_sec=15,
            )
            if verified.return_code != 0:
                raise RuntimeError(
                    "Luban sandbox canary did not prove isolated workspace write"
                )
        finally:
            await environment.exec(
                command=(
                    "if test -s "
                    + shlex.quote(pid)
                    + "; then kill \"$(cat "
                    + shlex.quote(pid)
                    + ")\" 2>/dev/null || true; fi; rm -f "
                    + " ".join(map(shlex.quote, (ready, pid)))
                ),
                user="root",
                timeout_sec=15,
            )

    def _resolved_argv(self) -> list[str]:
        if self._command_argv[1:] != _formal_source_argv_tail(self._agent_kind):
            raise RuntimeError("frozen source command changed after adapter initialization")
        argv = [self._remote_binary, *self._command_argv[1:]]
        if self._agent_kind == "codex":
            bound_provider = _CODEX_HTTP_PROVIDER_CONFIG_TOKEN.replace(
                "{provider_base_url}", self._proxy_base_url
            )
            argv = [
                "-"
                if value == "{instruction_path}"
                else bound_provider
                if value == _CODEX_HTTP_PROVIDER_CONFIG_TOKEN
                else value
                for value in argv
            ]
            exec_index = argv.index("exec")
            argv[exec_index + 1 : exec_index + 1] = ["--cd", _WORKSPACE]
        else:
            # Luban reads the query from stdin when there is no positional
            # query. Never give either CLI a host instruction path.
            argv = [value for value in argv if value != "{instruction_path}"]
            argv[1:1] = ["--disallowed-tools", _LUBAN_DISALLOWED_TOOLS]
        _canonical_argv_json(argv)
        return argv

    def _make_effective_argv_receipt(self) -> dict[str, object]:
        return _effective_argv_receipt(
            agent_kind=self._agent_kind,
            argv=self._resolved_argv(),
            proxy_base_url=self._proxy_base_url,
            adapter_version=self._adapter_version,
            adapter_sha256=self._adapter_sha256,
            source_command_argv_sha256=self._source_command_argv_sha256,
            bundle_manifest_sha256=self._bundle_manifest_sha256,
            bundle_tree_sha256=self._bundle.tree_sha256,
        )

    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        self.logs_dir.mkdir(parents=True, exist_ok=True)
        instruction_path = self.logs_dir / "instruction.txt"
        instruction_path.write_text(instruction, encoding="utf-8")
        os.chmod(instruction_path, 0o644 if self._agent_kind == "luban" else 0o600)

        argv = self._resolved_argv()
        expected_effective_receipt = self._make_effective_argv_receipt()
        try:
            archived_effective_receipt = json.loads(
                (self.logs_dir / "effective-argv.json").read_text(encoding="ascii"),
                object_pairs_hook=_reject_duplicate_json_keys,
            )
        except (OSError, json.JSONDecodeError) as error:
            raise RuntimeError("effective argv setup receipt is unavailable") from error
        if archived_effective_receipt != expected_effective_receipt:
            raise RuntimeError("effective argv changed between setup and execution")
        context.metadata = {
            "binary_sha256": self._binary_sha256,
            "base_commit": self._base_commit,
            "workspace_capture": "temporary-git-index-v1",
            "provider_meter": "host-content-free-v2",
            "adapter_sha256": self._adapter_sha256,
            "effective_argv_sha256": expected_effective_receipt[
                "effective_argv_sha256"
            ],
            "execution_argv_sha256": expected_effective_receipt[
                "execution_argv_sha256"
            ],
            "terminal_evidence_parser_sha256": _TERMINAL_EVIDENCE_PARSER_SHA256,
        }
        context.metadata["source_bundle_tree_sha256"] = self._bundle.tree_sha256
        context.metadata["runtime_payload_tree_sha256"] = _canonical_bundle_tree(
            self._uploaded_bundle_files
        )
        stream_path = f"{_AGENT_LOGS}/stream.jsonl"
        stderr_path = f"{_AGENT_LOGS}/stderr.log"
        command = (
            shlex.join(argv)
            + " < "
            + shlex.quote(f"{_AGENT_LOGS}/instruction.txt")
            + " > "
            + shlex.quote(stream_path)
            + " 2> "
            + shlex.quote(stderr_path)
        )
        started = await environment.exec(
            command="date -u +%Y-%m-%dT%H:%M:%S.%NZ",
            cwd=_WORKSPACE,
        )
        result = None
        cancelled = False
        try:
            result = await environment.exec(
                command=command,
                cwd=_WORKSPACE,
                env=self._process_env(environment),
                user=self._execution_user(),
            )
        except asyncio.CancelledError:
            # Pier implements the task's exact agent timeout with
            # asyncio.wait_for.  Its cancellation stops the CLI exec first;
            # the finally block must still make the timed-out workspace the
            # verifier input instead of silently losing partial work.
            cancelled = True
            raise
        finally:
            capture_result = await environment.exec(
                command=self._capture_command(),
                cwd=_WORKSPACE,
                timeout_sec=120,
            )
            if capture_result.return_code != 0:
                raise RuntimeError("complete workspace capture failed")
            exit_code = (
                result.return_code if result is not None else 124 if cancelled else 125
            )
            exit_receipt = {
                "schema_version": "agentic-bench/agent-exit-v1",
                "exit_code": exit_code,
                "started_at": (started.stdout or "").strip(),
            }
            exit_receipt_path = self.logs_dir / "exit.json"
            exit_receipt_path.write_text(
                json.dumps(exit_receipt, sort_keys=True) + "\n", encoding="utf-8"
            )
            if not cancelled:
                _write_adapter_terminal_evidence(
                    self._agent_kind,
                    self.logs_dir / "stream.jsonl",
                    exit_receipt_path,
                    self.logs_dir / "terminal-evidence.json",
                    exit_code,
                )

        assert result is not None
        if result.return_code != 0:
            raise NonZeroAgentExitCodeError(
                f"pinned {self._agent_kind} exited with code {result.return_code}"
            )

    def _capture_command(self) -> str:
        patch = f"{_AGENT_LOGS}/full-workspace.patch"
        committed_patch = f"{_AGENT_LOGS}/committed-workspace.patch"
        receipt = f"{_AGENT_LOGS}/workspace-capture.json"
        return " ".join(
            [
                "set -eu;",
                "git diff --binary",
                shlex.quote(self._base_commit),
                "HEAD -- >",
                shlex.quote(committed_patch) + ";",
                "idx=$(mktemp);",
                "rm -f \"$idx\";",
                "trap 'rm -f \"$idx\"' EXIT;",
                "export GIT_INDEX_FILE=\"$idx\";",
                "git read-tree",
                shlex.quote(self._base_commit) + ";",
                "git add -A -- .;",
                "git diff --cached --binary",
                shlex.quote(self._base_commit),
                "-- >",
                shlex.quote(patch) + ";",
                "audit_sha=$(sha256sum",
                shlex.quote(patch),
                "| cut -d' ' -f1);",
                "official_sha=$(sha256sum",
                shlex.quote(committed_patch),
                "| cut -d' ' -f1);",
                "uncommitted=false;",
                "cmp -s",
                shlex.quote(committed_patch),
                shlex.quote(patch),
                "|| uncommitted=true;",
                "printf",
                shlex.quote(
                    '{"schema_version":"agentic-bench/workspace-capture-v2",'
                    '"method":"official-git-diff+temporary-index-audit-v2",'
                    f'"base_commit":"{self._base_commit}",'
                    '"patch_sha256":"%s","audit_patch_sha256":"%s",'
                    '"uncommitted_changes_present":%s,'
                    '"includes_tracked":true,"includes_untracked":true,'
                    '"includes_binary":true}\n'
                ),
                '"$official_sha" "$audit_sha" "$uncommitted" >',
                shlex.quote(receipt) + ";",
                "unset GIT_INDEX_FILE;",
                "test \"$(git rev-parse",
                shlex.quote(self._base_commit) + ")\" =",
                shlex.quote(self._base_commit),
            ]
        )


def file_sha256(path: Path) -> str:
    """Small host-side helper used by adapter conformance tests."""

    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()
