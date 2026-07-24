#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


GOLD_FACTS: list[dict[str, Any]] = [
    {
        "id": "entry_and_modes",
        "groups": [["main"], ["run"], ["HiArgs"], ["Search", "Files", "Types", "Generate"]],
        "evidence": ["crates/core/main.rs", "main", "run", "HiArgs", "Mode::Search"],
    },
    {
        "id": "high_args",
        "groups": [["HiArgs"], ["from_low_args"], ["patterns", "pattern", "模式"], ["paths", "path", "路径"], ["globs", "types", "glob", "type"]],
        "evidence": ["crates/core/flags/hiargs.rs", "HiArgs", "from_low_args", "patterns", "paths", "globs", "types"],
    },
    {
        "id": "walk_and_ignore",
        "groups": [["walk_builder"], ["ignore"], ["globs"], ["types"], ["hidden"]],
        "evidence": ["crates/core/flags/hiargs.rs", "WalkBuilder", "overrides", "types", "git_ignore", "hidden"],
    },
    {
        "id": "haystack_filter",
        "groups": [["HaystackBuilder", "Haystack"], ["DirEntry"], ["explicit", "显式"], ["file", "search", "搜索"]],
        "evidence": ["crates/core/haystack.rs", "HaystackBuilder", "DirEntry", "is_explicit", "is_file"],
    },
    {
        "id": "matcher_build",
        "groups": [["RegexMatcherBuilder", "RegexMatcher", "PatternMatcher"], ["matcher"], ["patterns", "模式"], ["case", "unicode", "大小写", "HIR", "literal", "字面量"]],
        "evidence": ["crates/core/flags/hiargs.rs", "crates/regex/src/matcher.rs", "RegexMatcherBuilder", "build_many", "case"],
    },
    {
        "id": "search_worker",
        "groups": [["SearchWorker"], ["Searcher"], ["matcher"], ["printer"], ["search_path", "search_reader"]],
        "evidence": ["crates/core/search.rs", "SearchWorker", "Searcher", "search_path", "search_reader", "printer"],
    },
    {
        "id": "printer_output",
        "groups": [["Printer"], ["Standard", "JSON", "Summary"], ["StandardBuilder", "JSONBuilder", "SummaryBuilder", "格式"], ["output", "print", "输出"]],
        "evidence": ["crates/core/flags/hiargs.rs", "crates/printer/src/standard.rs", "Printer", "StandardBuilder", "JSONBuilder", "SummaryBuilder"],
    },
    {
        "id": "binary_preprocess",
        "groups": [["binary"], ["preprocessor", "decompress"], ["BinaryDetection", "二进制"], ["implicit", "explicit", "显式", "隐式", "特殊"]],
        "evidence": ["crates/core/search.rs", "crates/core/flags/hiargs.rs", "BinaryDetection", "preprocessor", "decompress"],
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
    text = norm(text)
    return any(norm(item) in text for item in group)


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
            if any(valid_evidence(repo, c, expected) for c in fact["evidence"]):
                return True
    return False


def citation_window(repo: Path, path: str, line: int) -> str | None:
    file_path = (repo / path).resolve()
    try:
        file_path.relative_to(repo.resolve())
    except ValueError:
        return None
    if not file_path.is_file():
        return None
    lines = file_path.read_text(errors="replace").splitlines()
    if line < 1 or line > len(lines):
        return None
    return "\n".join(lines[max(0, line - 4):min(len(lines), line + 3)])


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


def load_report(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text())
    except Exception as exc:
        return {"_load_error": str(exc)}
    return data if isinstance(data, dict) else {"_load_error": "report is not a JSON object"}


def score_report(repo: Path, report: Path) -> dict[str, Any]:
    data = load_report(report)
    if "_load_error" in data:
        return {
            "overall": 0.0,
            "fact_coverage": 0.0,
            "evidence_accuracy": 0.0,
            "test_pass_rate": 0.0,
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
    fact_hits = 0
    evidence_hits = 0
    details: dict[str, dict[str, Any]] = {}
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
    return {
        "overall": round(overall, 4),
        "fact_coverage": round(fact_coverage, 4),
        "evidence_accuracy": round(evidence_accuracy, 4),
        "summary_length_ok": summary_ok,
        "summary_length": summary_length,
        "test_pass_rate": round(overall, 4),
        "test_status": "full_pass" if overall >= 1 else "partial_pass" if overall > 0 else "no_pass",
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
