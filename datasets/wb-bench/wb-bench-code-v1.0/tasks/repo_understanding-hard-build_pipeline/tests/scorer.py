#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


GOLD_FACTS: list[dict[str, Any]] = [
    {
        "id": "command_config_site",
        "groups": [["Build"], ["configuration_from_options", "configuration"], ["Jekyll::Site", "Site.new"], ["build", "process_site"]],
        "evidence": ["lib/jekyll/commands/build.rb", "lib/jekyll/command.rb", "lib/jekyll.rb", "configuration_from_options", "Site.new", "process_site"],
    },
    {
        "id": "site_process_stages",
        "groups": [["Site#process", "process"], ["reset"], ["read"], ["generate"], ["render"], ["cleanup"], ["write"]],
        "evidence": ["lib/jekyll/site.rb", "reset", "read", "generate", "render", "cleanup", "write"],
    },
    {
        "id": "reader_ingest",
        "groups": [["Reader"], ["layouts"], ["pages"], ["posts"], ["collections"], ["static", "static_files"], ["data"]],
        "evidence": ["lib/jekyll/reader.rb", "LayoutReader", "CollectionReader", "read_directories", "read_data", "retrieve_posts", "retrieve_pages", "retrieve_static_files"],
    },
    {
        "id": "collection_document_read",
        "groups": [["Collection"], ["Document"], ["read_document", "read"], ["front matter", "yaml", "data"], ["published", "static"]],
        "evidence": ["lib/jekyll/collection.rb", "lib/jekyll/document.rb", "read_document", "Document.new", "Utils.has_yaml_header", "published"],
    },
    {
        "id": "plugins_generators_converters",
        "groups": [["plugin", "PluginManager"], ["conscientious_require", "setup", "加载"], ["Generator", "generators"], ["Converter", "converters"], ["instantiate_subclasses", "实例化"]],
        "evidence": ["lib/jekyll/site.rb", "lib/jekyll/plugin_manager.rb", "lib/jekyll/generator.rb", "lib/jekyll/converter.rb", "conscientious_require", "instantiate_subclasses"],
    },
    {
        "id": "render_liquid_convert_layout",
        "groups": [["Renderer"], ["Liquid"], ["convert", "Converter"], ["layout"], ["payload"], ["post_convert", "pre_render"]],
        "evidence": ["lib/jekyll/renderer.rb", "render_liquid", "convert", "place_in_layouts", "trigger_hooks", "payload"],
    },
    {
        "id": "cleanup_write_hooks",
        "groups": [["cleanup", "Cleaner"], ["write"], ["destination", "写入"], ["regenerator", "metadata"], ["Hooks", "hook"], ["post_write", "on_obsolete"]],
        "evidence": ["lib/jekyll/site.rb", "lib/jekyll/cleaner.rb", "lib/jekyll/document.rb", "lib/jekyll/convertible.rb", "lib/jekyll/static_file.rb", "lib/jekyll/hooks.rb", "cleanup", "write", "post_write", "on_obsolete"],
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
