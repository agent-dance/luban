"""Task-local verifier manifest parsing."""

from __future__ import annotations

import tomllib
from dataclasses import dataclass
from enum import StrEnum
from pathlib import Path
from typing import Any, Mapping


SCHEMA_VERSION = "workbuddy.code_lite.verifier.v1"
MANIFEST_NAME = "verifier.toml"


class VerifierFamily(StrEnum):
    SCRIPT_VERIFIER = "script_verifier"
    PYTEST_INJECTED = "pytest_injected"
    REPO_UNDERSTANDING = "repo_understanding"


@dataclass(frozen=True)
class VerifierManifest:
    family: VerifierFamily
    cwd: str = "/workspace"
    command: str = ""


def load_manifest(task_dir: Path) -> VerifierManifest:
    path = task_dir / "tests" / MANIFEST_NAME
    if not path.is_file():
        raise FileNotFoundError(f"task has no tests/{MANIFEST_NAME}: {task_dir}")
    try:
        data = tomllib.loads(path.read_text(encoding="utf-8"))
    except tomllib.TOMLDecodeError as exc:
        raise ValueError(f"malformed verifier manifest: {path}") from exc
    schema_version = str(data.get("schema_version") or "")
    if schema_version != SCHEMA_VERSION:
        raise ValueError(
            f"unsupported verifier manifest schema {schema_version!r}: {path}"
        )
    try:
        family = VerifierFamily(str(data.get("family") or ""))
    except ValueError as exc:
        raise ValueError(f"unsupported verifier family in {path}") from exc
    run = data.get("run") or {}
    if not isinstance(run, Mapping):
        raise ValueError(f"[run] must be a TOML table in {path}")
    command = _optional_str(run, "command")
    if family == VerifierFamily.PYTEST_INJECTED and not command:
        raise ValueError(f"pytest_injected manifest requires run.command: {path}")
    return VerifierManifest(
        family=family,
        cwd=_optional_str(run, "cwd") or "/workspace",
        command=command,
    )


def _optional_str(data: Mapping[str, Any], key: str) -> str:
    value = data.get(key)
    return "" if value is None else str(value).strip()
