#!/usr/bin/env python3
"""Redact credentials accidentally captured in benchmark text artifacts."""

from __future__ import annotations

import re
from pathlib import Path


ROOT = Path(__file__).resolve().parent
TEXT_SUFFIXES = {".json", ".jsonl", ".log", ".txt", ".patch", ".csv", ".html", ".py"}

SENSITIVE_NAME = re.compile(
    r"(?P<name>[A-Za-z][A-Za-z0-9_]*(?:API_KEY|PRIVATE_TOKEN|AUTH_TOKEN|ACCESS_TOKEN|"
    r"PASSWORD|SECRET_KEY|SCAFFOLD_SECRET|ARCUS_SECRET|USER_KEY|GIT_TOKEN|RELEASE_TOKEN))"
    r"=(?P<value>[^\\\r\n\"\s]+)",
    re.IGNORECASE,
)
SPECIAL_ASSIGNMENT = re.compile(
    r"(?P<name>CODE_PUSH_REMOTE_URL|mirrors)=(?P<value>[^\\\r\n\"\s]+)",
    re.IGNORECASE,
)
JSON_SECRET = re.compile(
    r'(?P<prefix>\"(?:api[_-]?key|authorization|token|password|secret)\"\s*:\s*\")'
    r'(?P<value>[^\"]+)(?P<suffix>\")',
    re.IGNORECASE,
)
SECRET_PATTERNS = [
    re.compile(r"sk-[A-Za-z0-9_-]{12,}"),
    re.compile(r"Bearer\s+[A-Za-z0-9._~+/-]{12,}", re.IGNORECASE),
    re.compile(r"(?P<scheme>https?://)[^:/\\\s\"]+:[^@\\\s\"]+@", re.IGNORECASE),
]


def text_files() -> list[Path]:
    return [
        path
        for path in ROOT.rglob("*")
        if path.is_file() and path.suffix.lower() in TEXT_SUFFIXES and path.name != "redact_raw.py"
    ]


paths = text_files()
values: set[str] = set()
for path in paths:
    content = path.read_text(encoding="utf-8", errors="replace")
    for pattern in (SENSITIVE_NAME, SPECIAL_ASSIGNMENT):
        for match in pattern.finditer(content):
            value = match.group("value")
            if len(value) >= 8 and value != "[REDACTED]":
                values.add(value)
    for match in JSON_SECRET.finditer(content):
        value = match.group("value")
        if len(value) >= 8 and value != "[REDACTED]":
            values.add(value)

changed = 0
for path in paths:
    content = path.read_text(encoding="utf-8", errors="replace")
    redacted = SENSITIVE_NAME.sub(lambda m: f'{m.group("name")}=[REDACTED]', content)
    redacted = SPECIAL_ASSIGNMENT.sub(lambda m: f'{m.group("name")}=[REDACTED]', redacted)
    redacted = JSON_SECRET.sub(
        lambda m: f'{m.group("prefix")}[REDACTED]{m.group("suffix")}', redacted
    )
    for value in sorted(values, key=len, reverse=True):
        redacted = redacted.replace(value, "[REDACTED]")
    for pattern in SECRET_PATTERNS:
        if "scheme" in pattern.groupindex:
            redacted = pattern.sub(lambda m: f'{m.group("scheme")}[REDACTED]@', redacted)
        else:
            redacted = pattern.sub("[REDACTED]", redacted)
    if redacted != content:
        path.write_text(redacted, encoding="utf-8")
        changed += 1

print(f"redacted {len(values)} distinct values across {changed} files")
