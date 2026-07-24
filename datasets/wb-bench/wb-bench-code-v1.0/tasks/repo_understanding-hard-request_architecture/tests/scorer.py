#!/usr/bin/env python3
"""Structured-report scorer for HTTPX repo-understanding task.

This intentionally avoids LLM judging. It scores two machine-checkable signals:

1. Whether the submitted facts mention the expected architectural concepts.
2. Whether each matched fact includes at least one citation pointing at a real
   repository line that contains supporting code nearby.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


GOLD_FACTS: list[dict[str, Any]] = [
    {
        "id": "top_level_api",
        "groups": [
            ["_api.py", "top-level", "top level", "顶层"],
            ["Client"],
            ["client.request", "request"],
        ],
        "evidence": ["httpx/_api.py", "Client(", "client.request", "def request"],
    },
    {
        "id": "sync_client_flow",
        "groups": [
            ["Client"],
            ["send"],
            ["_send_handling_auth"],
            ["_send_handling_redirects"],
            ["_send_single_request"],
        ],
        "evidence": [
            "httpx/_client.py",
            "def send",
            "_send_handling_auth",
            "_send_handling_redirects",
            "_send_single_request",
        ],
    },
    {
        "id": "async_client_flow",
        "groups": [
            ["AsyncClient"],
            ["async"],
            ["handle_async_request"],
        ],
        "evidence": [
            "httpx/_client.py",
            "class AsyncClient",
            "handle_async_request",
            "async def",
        ],
    },
    {
        "id": "transport_boundary",
        "groups": [
            ["BaseTransport"],
            ["AsyncBaseTransport"],
            ["handle_request"],
            ["handle_async_request"],
        ],
        "evidence": [
            "httpx/_transports/base.py",
            "BaseTransport",
            "AsyncBaseTransport",
            "handle_request",
            "handle_async_request",
        ],
    },
    {
        "id": "httpcore_adapter",
        "groups": [
            ["HTTPTransport"],
            ["httpcore"],
            ["ConnectionPool", "HTTPProxy", "SOCKSProxy"],
            ["handle_request", "handle_async_request", "map_httpcore_exceptions", "_pool"],
        ],
        "evidence": [
            "httpx/_transports/default.py",
            "HTTPTransport",
            "httpcore",
            "ConnectionPool",
            "map_httpcore_exceptions",
        ],
    },
    {
        "id": "proxy_mount_routing",
        "groups": [
            ["URLPattern"],
            ["_mounts"],
            ["_transport_for_url"],
            ["proxy", "mount"],
        ],
        "evidence": [
            "httpx/_client.py",
            "httpx/_utils.py",
            "URLPattern",
            "_mounts",
            "_transport_for_url",
        ],
    },
    {
        "id": "auth_flow",
        "groups": [
            ["Auth"],
            ["auth_flow"],
            ["sync_auth_flow", "async_auth_flow", "_send_handling_auth", "生成器", "generator"],
            ["response"],
        ],
        "evidence": [
            "httpx/_auth.py",
            "Auth",
            "auth_flow",
            "sync_auth_flow",
            "async_auth_flow",
            "response",
        ],
    },
    {
        "id": "cli_entry",
        "groups": [
            ["_main.py"],
            ["main"],
            ["Client"],
            ["client.stream"],
        ],
        "evidence": [
            "httpx/_main.py",
            "def main",
            "Client(",
            "client.stream",
        ],
    },
]


def norm(value: Any) -> str:
    return str(value).lower()


def stringify(value: Any) -> str:
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return " ".join(stringify(item) for item in value)
    if isinstance(value, dict):
        return " ".join(f"{key} {stringify(item)}" for key, item in value.items())
    return str(value)


def group_hit(text: str, group: list[str]) -> bool:
    normalized = norm(text)
    return any(norm(item) in normalized for item in group)


def find_matching_facts(facts: list[Any], gold: dict[str, Any]) -> tuple[list[Any], str]:
    groups = gold["groups"]
    for fact in facts:
        if all(group_hit(stringify(fact), group) for group in groups):
            return [fact], "single_fact"

    # Allow nearby facts to jointly support one architectural point, but do not
    # let scattered terminology across a long report accidentally combine.
    window_size = 3
    for start in range(len(facts)):
        window = facts[start:start + window_size]
        text = " ".join(stringify(fact) for fact in window)
        if all(group_hit(text, group) for group in groups):
            related = [
                fact
                for fact in window
                if any(group_hit(stringify(fact), group) for group in groups)
            ]
            return related, "nearby_facts"
    return [], "missing"


def claims_for_details(matches: list[Any]) -> str:
    claims: list[str] = []
    for fact in matches[:5]:
        if isinstance(fact, dict):
            claim = fact.get("claim", "")
            if claim:
                claims.append(str(claim))
    return " | ".join(claims)


def evidence_from_matches(repo: Path, matches: list[Any], expected: list[str]) -> bool:
    for fact in matches:
        if isinstance(fact, dict) and isinstance(fact.get("evidence"), list):
            if any(valid_evidence(repo, citation, expected) for citation in fact["evidence"]):
                return True
    return False


def citation_window(repo: Path, path: str, line: int) -> str | None:
    file_path = (repo / path).resolve()
    try:
        repo_root = repo.resolve()
        file_path.relative_to(repo_root)
    except ValueError:
        return None
    if not file_path.exists() or not file_path.is_file():
        return None
    lines = file_path.read_text(errors="replace").splitlines()
    if line < 1 or line > len(lines):
        return None
    start = max(0, line - 4)
    end = min(len(lines), line + 3)
    return "\n".join(lines[start:end])


def valid_evidence(repo: Path, citation: Any, expected: list[str]) -> bool:
    if not isinstance(citation, dict):
        return False
    path = citation.get("path")
    line = citation.get("line")
    if not isinstance(path, str) or not isinstance(line, int):
        return False
    window = citation_window(repo, path, line)
    if window is None:
        return False

    normalized_path = norm(path)
    path_hints = [
        norm(item)
        for item in expected
        if "/" in norm(item) or norm(item).endswith((".py", ".js", ".go", ".rs", ".rb", ".java"))
    ]
    content_hints = [item for item in expected if norm(item) not in path_hints]
    path_ok = not path_hints or any(hint in normalized_path for hint in path_hints)
    content_ok = any(norm(item) in norm(window) for item in content_hints)
    return path_ok and content_ok


def load_report(report_path: Path) -> dict[str, Any]:
    try:
        data = json.loads(report_path.read_text())
    except Exception as exc:
        return {"_load_error": str(exc)}
    return data if isinstance(data, dict) else {"_load_error": "report is not a JSON object"}


def score_report(repo: Path, report_path: Path) -> dict[str, Any]:
    data = load_report(report_path)
    if "_load_error" in data:
        return {
            "overall": 0.0,
            "fact_coverage": 0.0,
            "evidence_accuracy": 0.0,
            "test_status": "build_error",
            "error": data["_load_error"],
            "tests_passed": 0,
            "tests_total": len(GOLD_FACTS) * 2 + 1,
            "details": {},
        }

    facts = data.get("facts", [])
    if not isinstance(facts, list):
        facts = []
    summary = data.get("summary", "")
    summary_ok = isinstance(summary, str) and len(summary) <= 200
    summary_length = len(summary) if isinstance(summary, str) else None

    details: dict[str, dict[str, Any]] = {}
    fact_hits = 0
    evidence_hits = 0

    for gold in GOLD_FACTS:
        matches, match_type = find_matching_facts(facts, gold)
        has_fact = bool(matches)
        has_evidence = evidence_from_matches(repo, matches, gold["evidence"]) if has_fact else False
        fact_hits += int(has_fact)
        evidence_hits += int(has_evidence)
        details[gold["id"]] = {
            "fact": has_fact,
            "evidence": has_evidence,
            "match_type": match_type,
            "matched_claim": claims_for_details(matches),
        }

    fact_coverage = fact_hits / len(GOLD_FACTS)
    evidence_accuracy = evidence_hits / len(GOLD_FACTS)
    tests_passed = fact_hits + evidence_hits + int(summary_ok)
    tests_total = len(GOLD_FACTS) * 2 + 1
    overall = tests_passed / tests_total
    details["summary_length"] = {
        "pass": summary_ok,
        "length": summary_length,
        "limit": 200,
    }
    if overall >= 1.0:
        test_status = "full_pass"
    elif overall > 0:
        test_status = "partial_pass"
    else:
        test_status = "no_pass"

    return {
        "overall": round(overall, 4),
        "fact_coverage": round(fact_coverage, 4),
        "evidence_accuracy": round(evidence_accuracy, 4),
        "summary_length_ok": summary_ok,
        "summary_length": summary_length,
        "test_pass_rate": round(overall, 4),
        "test_status": test_status,
        "tests_passed": tests_passed,
        "tests_total": tests_total,
        "details": details,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", default="/workspace")
    parser.add_argument("--report", default="/workspace/analysis.json")
    parser.add_argument("--output", default="/logs/verifier/reward.json")
    args = parser.parse_args()

    result = score_report(Path(args.repo), Path(args.report))
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result, indent=2, ensure_ascii=False))
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
