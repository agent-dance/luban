from __future__ import annotations

import hashlib
import json
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any


TERMINAL_EVIDENCE_SCHEMA = "agentic-bench/terminal-evidence-v1"
_CONTEXT_FAILURE_CODE = "context_length_exceeded"


class TerminalEvidenceProtocolError(RuntimeError):
    """The machine stream cannot prove one unambiguous terminal outcome."""


@dataclass(frozen=True)
class TerminalEvidence:
    schema_version: str
    source: str
    code: str
    evidence_sha256: str


@dataclass(frozen=True)
class _TerminalEvent:
    kind: str
    code: str
    raw: bytes


def _sha256(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def _reject_duplicate_keys(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key {key!r}")
        result[key] = value
    return result


def _reject_nonfinite_constant(value: str) -> object:
    raise ValueError(f"non-finite JSON number {value!r}")


def _string_at(value: object, *path: str) -> str | None:
    current = value
    for component in path:
        if not isinstance(current, dict):
            return None
        current = current.get(component)
    return current if isinstance(current, str) and current else None


def _provider_failure_code(event: dict[str, Any]) -> str | None:
    event_type = event.get("type")
    if event_type == "response.failed":
        return _string_at(event, "response", "error", "code")
    if event_type == "turn.failed":
        return _string_at(event, "error", "code")
    if event_type != "error":
        return None

    # OpenAI provider error events use a top-level code. A terminal wrapper may
    # instead preserve the provider error object. Both shapes require a typed
    # code; a human-readable message is deliberately never inspected.
    return _string_at(event, "code") or _string_at(event, "error", "code")


def _terminal_event(agent_kind: str, event: dict[str, Any], raw: bytes) -> _TerminalEvent | None:
    event_type = event.get("type")
    if event_type == "turn.completed":
        if agent_kind != "codex":
            raise TerminalEvidenceProtocolError(
                "non-Codex stream emitted a Codex terminal event"
            )
        return _TerminalEvent("completed", "completed", raw)

    provider_code = _provider_failure_code(event)
    if event_type in {"response.failed", "turn.failed", "error"}:
        if provider_code is not None:
            return _TerminalEvent("provider_failure", provider_code, raw)

        # Luban's runtime-event projection is a structured terminal error only
        # when all authority fields are present. Its stable semantic code is
        # safe to inspect; localized message text is not.
        if (
            agent_kind == "luban"
            and event_type == "error"
            and event.get("schema_version") == "runtime-event/v2"
            and event.get("kind") == "runtime_error"
            and event.get("outcome") == "failed"
        ):
            code = event.get("code")
            if isinstance(code, str) and code:
                return _TerminalEvent("provider_failure", code, raw)

        raise TerminalEvidenceProtocolError(
            f"{agent_kind} terminal event lacks a supported structured code"
        )

    # Codex item errors are explicitly non-terminal and can coexist with a
    # later turn.completed. Tool failures are likewise operational events, not
    # evidence about the agent process terminal state.
    return None


def parse_terminal_evidence(
    agent_kind: str,
    stream_raw: bytes,
    exit_receipt_raw: bytes,
    exit_code: int,
) -> TerminalEvidence:
    """Derive sealed terminal evidence without reading stderr or error prose."""

    if agent_kind not in {"codex", "luban"}:
        raise TerminalEvidenceProtocolError(f"unsupported agent kind {agent_kind!r}")

    terminals: list[_TerminalEvent] = []
    # JSON Lines permits LF-delimited records, with an optional CR immediately
    # before the LF. Do not let Python's broader splitlines() semantics accept
    # bare CR, vertical-tab, or form-feed as record delimiters: the independent
    # Go verifier deliberately applies the same byte-level grammar.
    lines = stream_raw.split(b"\n")
    if lines and lines[-1] == b"":
        lines = lines[:-1]
    for line_number, line in enumerate(lines, 1):
        # The JSONL newline is a record delimiter, not part of the provider
        # event. Preserve every byte of the JSON value itself for the digest.
        raw = line.removesuffix(b"\r")
        if not raw:
            raise TerminalEvidenceProtocolError(
                f"machine stream contains an empty record at line {line_number}"
            )
        try:
            event = json.loads(
                raw,
                object_pairs_hook=_reject_duplicate_keys,
                parse_constant=_reject_nonfinite_constant,
            )
        except (UnicodeDecodeError, json.JSONDecodeError, ValueError) as error:
            raise TerminalEvidenceProtocolError(
                f"machine stream contains invalid JSON at line {line_number}"
            ) from error
        if not isinstance(event, dict) or not isinstance(event.get("type"), str):
            raise TerminalEvidenceProtocolError(
                f"machine stream contains an untyped event at line {line_number}"
            )
        terminal = _terminal_event(agent_kind, event, raw)
        if terminal is not None:
            terminals.append(terminal)

    context_events = [
        event
        for event in terminals
        if event.kind == "provider_failure" and event.code == _CONTEXT_FAILURE_CODE
    ]
    unknown_provider_events = [
        event
        for event in terminals
        if event.kind == "provider_failure" and event.code != _CONTEXT_FAILURE_CODE
    ]
    completed_events = [event for event in terminals if event.kind == "completed"]

    if unknown_provider_events:
        raise TerminalEvidenceProtocolError(
            "machine stream contains an unsupported structured provider failure"
        )
    if context_events:
        if len(context_events) != 1 or completed_events or exit_code == 0:
            raise TerminalEvidenceProtocolError(
                "context failure conflicts with another terminal outcome"
            )
        return TerminalEvidence(
            schema_version=TERMINAL_EVIDENCE_SCHEMA,
            source="provider_event",
            code=_CONTEXT_FAILURE_CODE,
            evidence_sha256=_sha256(context_events[0].raw),
        )
    if completed_events:
        if len(completed_events) != 1 or exit_code != 0:
            raise TerminalEvidenceProtocolError(
                "Codex turn completion conflicts with the process exit"
            )
    if exit_code == 0:
        code = "completed"
    else:
        code = "nonzero_exit"
    return TerminalEvidence(
        schema_version=TERMINAL_EVIDENCE_SCHEMA,
        source="process_exit",
        code=code,
        evidence_sha256=_sha256(exit_receipt_raw),
    )


def write_terminal_evidence(
    agent_kind: str,
    stream_path: Path,
    exit_receipt_path: Path,
    destination: Path,
    exit_code: int,
) -> TerminalEvidence:
    evidence = parse_terminal_evidence(
        agent_kind,
        stream_path.read_bytes(),
        exit_receipt_path.read_bytes(),
        exit_code,
    )
    payload = json.dumps(asdict(evidence), separators=(",", ":"), sort_keys=True)
    destination.write_text(payload + "\n", encoding="utf-8")
    return evidence
