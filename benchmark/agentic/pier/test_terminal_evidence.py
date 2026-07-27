from __future__ import annotations

import hashlib
import json
import unittest

from benchmark.agentic.pier.terminal_evidence import (
    TERMINAL_EVIDENCE_SCHEMA,
    TerminalEvidenceProtocolError,
    parse_terminal_evidence,
)


def event(value: object) -> bytes:
    return json.dumps(value, separators=(",", ":"), sort_keys=True).encode()


class TerminalEvidenceTest(unittest.TestCase):
    def test_codex_context_failure_uses_exact_structured_event_bytes(self) -> None:
        terminal = event(
            {
                "type": "turn.failed",
                "error": {
                    "code": "context_length_exceeded",
                    "message": "localized diagnostic is not authority",
                },
            }
        )
        evidence = parse_terminal_evidence(
            "codex", event({"type": "turn.started"}) + b"\n" + terminal + b"\n", b"exit\n", 1
        )
        self.assertEqual(evidence.schema_version, TERMINAL_EVIDENCE_SCHEMA)
        self.assertEqual(evidence.source, "provider_event")
        self.assertEqual(evidence.code, "context_length_exceeded")
        self.assertEqual(evidence.evidence_sha256, hashlib.sha256(terminal).hexdigest())

    def test_luban_context_failure_uses_runtime_semantic_code(self) -> None:
        terminal = event(
            {
                "type": "error",
                "schema_version": "runtime-event/v2",
                "kind": "runtime_error",
                "outcome": "failed",
                "code": "context_length_exceeded",
                "message": "redacted public copy",
            }
        )
        evidence = parse_terminal_evidence("luban", terminal + b"\n", b"exit\n", 17)
        self.assertEqual(evidence.source, "provider_event")
        self.assertEqual(evidence.code, "context_length_exceeded")
        self.assertEqual(evidence.evidence_sha256, hashlib.sha256(terminal).hexdigest())

    def test_raw_provider_response_failed_is_shared_by_both_agents(self) -> None:
        terminal = event(
            {
                "type": "response.failed",
                "response": {"error": {"code": "context_length_exceeded"}},
            }
        )
        for agent_kind in ("codex", "luban"):
            with self.subTest(agent_kind=agent_kind):
                evidence = parse_terminal_evidence(
                    agent_kind, terminal + b"\n", b"exit\n", 1
                )
                self.assertEqual(evidence.source, "provider_event")

    def test_message_and_stderr_like_text_never_classify_context(self) -> None:
        unstructured = event(
            {
                "type": "turn.failed",
                "error": {"message": "context_length_exceeded in stderr"},
            }
        )
        with self.assertRaisesRegex(
            TerminalEvidenceProtocolError, "lacks a supported structured code"
        ):
            parse_terminal_evidence("codex", unstructured + b"\n", b"exit\n", 1)

    def test_unknown_structured_provider_failure_is_protocol_invalid(self) -> None:
        unknown = event(
            {"type": "error", "error": {"code": "future_unknown_failure"}}
        )
        with self.assertRaisesRegex(
            TerminalEvidenceProtocolError, "unsupported structured provider failure"
        ):
            parse_terminal_evidence("codex", unknown + b"\n", b"exit\n", 1)

    def test_unstructured_or_malformed_records_are_protocol_invalid(self) -> None:
        for stream in (
            b'{"message":"no type"}\n',
            b'{"type":"text","type":"error"}\n',
            b'{"type":"text","value":NaN}\n',
            b'{"type":"text"}\r{"type":"text"}\n',
            b"not-json\n",
            b"\n",
        ):
            with self.subTest(stream=stream):
                with self.assertRaises(TerminalEvidenceProtocolError):
                    parse_terminal_evidence("luban", stream, b"exit\n", 1)

    def test_process_exit_receipt_is_authority_when_no_provider_failure_exists(self) -> None:
        receipt = b'{"schema_version":"agentic-bench/agent-exit-v1","exit_code":9}\n'
        evidence = parse_terminal_evidence(
            "luban", event({"type": "text", "content": "done"}) + b"\n", receipt, 9
        )
        self.assertEqual(evidence.source, "process_exit")
        self.assertEqual(evidence.code, "nonzero_exit")
        self.assertEqual(evidence.evidence_sha256, hashlib.sha256(receipt).hexdigest())

    def test_nonfatal_codex_item_error_does_not_override_completed_turn(self) -> None:
        stream = b"\n".join(
            (
                event(
                    {
                        "type": "item.completed",
                        "item": {"id": "item_1", "type": "error", "message": "warning"},
                    }
                ),
                event({"type": "turn.completed", "usage": {}}),
                b"",
            )
        )
        evidence = parse_terminal_evidence("codex", stream, b"exit-zero\n", 0)
        self.assertEqual(evidence.source, "process_exit")
        self.assertEqual(evidence.code, "completed")

    def test_conflicting_context_and_completion_is_protocol_invalid(self) -> None:
        stream = b"\n".join(
            (
                event(
                    {
                        "type": "turn.failed",
                        "error": {"code": "context_length_exceeded"},
                    }
                ),
                event({"type": "turn.completed", "usage": {}}),
                b"",
            )
        )
        with self.assertRaisesRegex(TerminalEvidenceProtocolError, "conflicts"):
            parse_terminal_evidence("codex", stream, b"exit\n", 1)


if __name__ == "__main__":
    unittest.main()
