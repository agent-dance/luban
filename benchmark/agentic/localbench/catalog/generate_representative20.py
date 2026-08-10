#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["pyarrow==21.0.0"]
# ///
"""Rebuild the frozen 20-task local benchmark catalog from pinned parquet files."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

import pyarrow.parquet as parquet


DATASET = "SWE-bench-Live/MultiLang"
REVISION = "608f7ae9ab8ea1f9f0d030fe04562cf6bd1a0c8b"
SEED = "representative20-v1"
LANGUAGES = ("cpp", "go", "java", "rust", "ts")
TARGET_PER_LANGUAGE = 4
MAX_FAIL_TO_PASS = 50
MAX_PASS_TO_PASS = 1000
PARQUET_SHA256 = {
    "cpp": "5afc7db10f28232cc9c13de316ecec146f2da4c76de3bb460b934f5e271b0ec0",
    "go": "76d2b5dff0f3fac8303d30fa85495539e487d25974ad7c21cd21a545cb4756e2",
    "java": "cc04473f299dbdbbb6c4061da3c68367cd460e28e40c04234f4887e0fc234220",
    "rust": "ea90be54a621c0c0280b77d5e2dee9650bc1d4ae087f9b9b06af821bcd8662d7",
    "ts": "7e23783e27230c9cfab1035690035c25523043d6af635bc78da3fd2010c32714",
}
PREFIX = (
    "danielmiessler__Fabric-2098",
    "openai__openai-agents-js-375",
    "kubernetes__kube-state-metrics-2926",
    "skim-rs__skim-1044",
    "include-what-you-use__include-what-you-use-1991",
)
EXPECTED_ORDER = PREFIX + (
    "ninja-build__ninja-2749",
    "charmbracelet__crush-766",
    "floci-io__floci-112",
    "eza-community__eza-1664",
    "assistant-ui__assistant-ui-3866",
    "actor-framework__actor-framework-2300",
    "lima-vm__lima-3923",
    "springdoc__springdoc-openapi-3051",
    "napi-rs__napi-rs-2784",
    "antvis__G2-7076",
    "apache__kvrocks-3084",
    "gitlab4j__gitlab4j-api-1266",
    "biomejs__biome-9995",
    "mikro-orm__mikro-orm-7464",
    "opendataloader-project__opendataloader-pdf-383",
)
CATALOG_FIELDS = (
    "repo",
    "base_commit",
    "patch",
    "test_patch",
    "problem_statement",
    "rebuild_cmds",
    "test_cmds",
    "print_cmds",
    "log_parser",
    "FAIL_TO_PASS",
    "PASS_TO_PASS",
    "docker_image",
)


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        while chunk := stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


def rank(language: str, instance_id: str) -> str:
    value = "\0".join((SEED, REVISION, language, instance_id))
    return hashlib.sha256(value.encode()).hexdigest()


def catalog_row(row: dict, language: str) -> dict:
    result = {"instance_id": row["instance_id"], "language": language}
    result.update({field: row[field] for field in CATALOG_FIELDS})
    return result


def eligible(row: dict) -> bool:
    required = ("instance_id", "repo", "problem_statement", "patch", "test_patch", "docker_image")
    return (
        all(row.get(field) for field in required)
        and 1 <= len(row.get("FAIL_TO_PASS") or ()) <= MAX_FAIL_TO_PASS
        and 1 <= len(row.get("PASS_TO_PASS") or ()) <= MAX_PASS_TO_PASS
    )


def load_rows(data_dir: Path) -> tuple[dict[str, tuple[str, dict]], dict[str, list[dict]]]:
    by_id: dict[str, tuple[str, dict]] = {}
    by_language: dict[str, list[dict]] = {}
    for language in LANGUAGES:
        path = data_dir / f"{language}.parquet"
        actual = sha256_file(path)
        if actual != PARQUET_SHA256[language]:
            raise RuntimeError(f"{path}: sha256 {actual}, want {PARQUET_SHA256[language]}")
        rows = parquet.read_table(path).to_pylist()
        by_language[language] = rows
        for row in rows:
            instance_id = row["instance_id"]
            if instance_id in by_id:
                raise RuntimeError(f"duplicate instance_id: {instance_id}")
            by_id[instance_id] = (language, row)
    return by_id, by_language


def select(by_id: dict[str, tuple[str, dict]], by_language: dict[str, list[dict]]) -> tuple[list[dict], dict[str, str]]:
    selected: dict[str, list[dict]] = {language: [] for language in LANGUAGES}
    used_repositories: set[str] = set()
    prefix_rows: list[dict] = []
    for instance_id in PREFIX:
        language, row = by_id[instance_id]
        selected[language].append(row)
        used_repositories.add(row["repo"].lower())
        prefix_rows.append(catalog_row(row, language))

    ranks: dict[str, str] = {}
    additions: dict[str, list[dict]] = {language: [] for language in LANGUAGES}
    for language in LANGUAGES:
        candidates = sorted(
            (row for row in by_language[language] if eligible(row)),
            key=lambda row: rank(language, row["instance_id"]),
        )
        needed = TARGET_PER_LANGUAGE - len(selected[language])
        for row in candidates:
            repository = row["repo"].lower()
            if row["instance_id"] in PREFIX or repository in used_repositories:
                continue
            additions[language].append(row)
            used_repositories.add(repository)
            ranks[row["instance_id"]] = rank(language, row["instance_id"])
            if len(additions[language]) == needed:
                break
        if len(additions[language]) != needed:
            raise RuntimeError(f"not enough eligible {language} tasks")

    ordered = prefix_rows
    for index in range(max(map(len, additions.values()))):
        for language in LANGUAGES:
            if index < len(additions[language]):
                ordered.append(catalog_row(additions[language][index], language))
    actual_order = tuple(row["instance_id"] for row in ordered)
    if actual_order != EXPECTED_ORDER:
        raise RuntimeError(f"selection drifted:\nactual={actual_order!r}\nwant={EXPECTED_ORDER!r}")
    return ordered, ranks


def verify_prefix(prefix_path: Path, catalog: list[dict]) -> None:
    frozen = {row["instance_id"]: row for row in json.loads(prefix_path.read_text(encoding="utf-8"))}
    for row in catalog[: len(PREFIX)]:
        if row != frozen.get(row["instance_id"]):
            raise RuntimeError(f"frozen prefix drifted: {row['instance_id']}")


def write_json(path: Path, value: object) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    directory = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser()
    parser.add_argument("--data-dir", type=Path, required=True)
    parser.add_argument("--prefix", type=Path, default=directory / "representative5.json")
    parser.add_argument("--output", type=Path, default=directory / "representative20.json")
    parser.add_argument("--manifest", type=Path, default=directory / "representative20.selection.json")
    args = parser.parse_args()

    by_id, by_language = load_rows(args.data_dir)
    catalog, ranks = select(by_id, by_language)
    verify_prefix(args.prefix, catalog)
    encoded = json.dumps(catalog, ensure_ascii=False, indent=2) + "\n"
    args.output.write_text(encoded, encoding="utf-8")
    manifest = {
        "schema_version": "agentic-local-selection/v1",
        "dataset": DATASET,
        "dataset_revision": REVISION,
        "seed": SEED,
        "parquet_sha256": PARQUET_SHA256,
        "rules": {
            "preserved_prefix_count": len(PREFIX),
            "target_per_language": TARGET_PER_LANGUAGE,
            "minimum_FAIL_TO_PASS": 1,
            "maximum_FAIL_TO_PASS": MAX_FAIL_TO_PASS,
            "minimum_PASS_TO_PASS": 1,
            "maximum_PASS_TO_PASS": MAX_PASS_TO_PASS,
            "required_nonempty_fields": [
                "instance_id", "repo", "problem_statement", "patch", "test_patch", "docker_image"
            ],
            "unique_repository": True,
            "ranking": "sha256(seed + NUL + revision + NUL + language + NUL + instance_id)",
            "addition_order": "round-robin: cpp, go, java, rust, ts",
        },
        "ordered_instance_ids": list(EXPECTED_ORDER),
        "addition_ranks": {instance_id: ranks[instance_id] for instance_id in EXPECTED_ORDER if instance_id in ranks},
        "catalog_sha256": hashlib.sha256(encoded.encode()).hexdigest(),
    }
    write_json(args.manifest, manifest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
