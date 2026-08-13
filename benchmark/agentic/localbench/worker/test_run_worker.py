import json
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import run_worker


class ResponseUsageCollectorTest(unittest.TestCase):
    def test_extracts_partial_sse_response_usage_without_content(self):
        collector = run_worker.ResponseUsageCollector()
        event = {
            "type": "response.completed",
            "response": {
                "model": "gpt-5.6-sol",
                "output": [{"content": "must not be retained"}],
                "usage": {
                    "input_tokens": 100,
                    "input_tokens_details": {"cached_tokens": 70, "cache_write_tokens": 5},
                    "output_tokens": 20,
                    "output_tokens_details": {"reasoning_tokens": 12},
                },
            },
        }
        payload = ("data: " + json.dumps(event) + "\n\n").encode()
        collector.feed(payload[:17])
        collector.feed(payload[17:])
        self.assertEqual(collector.finish(), {
            "input_tokens": 100,
            "cached_input_tokens": 70,
            "cache_creation_input_tokens": 5,
            "output_tokens": 20,
            "reasoning_output_tokens": 12,
            "served_model": "gpt-5.6-sol",
        })

    def test_provider_usage_sums_every_request(self):
        got = run_worker.provider_usage([
            {"input_tokens": 100, "cached_input_tokens": 70, "output_tokens": 20},
            {"input_tokens": 50, "cached_input_tokens": 10, "cache_creation_input_tokens": 5, "output_tokens": 4},
            {"status": 500},
        ])
        self.assertEqual(got, {
            "input_tokens": 150,
            "cached_input_tokens": 80,
            "cache_creation_input_tokens": 5,
            "output_tokens": 24,
            "reasoning_output_tokens": 0,
        })

    def test_context_metrics_capture_exact_turns(self):
        got = run_worker.context_metrics([
            {"type": "agentic_metrics", "metric": "context_projection", "turn_id": "session:query:turn-4",
             "context_projection": {"decision": "admit_net_savings", "projection_count": 2, "original_bytes": 1000, "projected_bytes": 200, "bytes_saved": 800}},
            {"type": "agentic_metrics", "metric": "context_compaction", "turn_count": 9},
        ])
        self.assertEqual(got["agent_turns"], 9)
        self.assertEqual(got["projection_turns"], [4])
        self.assertEqual(got["compaction_turns"], [9])
        self.assertEqual(got["first_compaction_turn"], 9)
        self.assertEqual(got["bytes_saved"], 800)
        self.assertEqual(got["tokens_saved"], 0)
        self.assertEqual(got["candidate_tools"], 2)
        self.assertEqual(got["projected_tools"], 2)
        self.assertEqual(got["context_update_proposals"], 0)
        self.assertEqual(got["projection_batches"], [{
            "turn": 4,
            "trigger": "",
            "applied": True,
            "projected_tools": 2,
            "rewritten_tools": 0,
            "indexed_tools": 0,
            "original_bytes": 1000,
            "projected_bytes": 200,
            "bytes_saved": 800,
            "original_tokens": 0,
            "projected_tokens": 0,
            "tokens_saved": 0,
            "decision": "admit_net_savings",
            "request_tokens_before": 0,
            "request_tokens_after": 0,
            "stable_prefix_tokens": 0,
            "invalidated_cached_tokens": 0,
            "cache_break_cost_usd": 0.0,
            "gross_cache_break_cost_usd": 0.0,
            "avoided_compact_input_cost_usd": 0.0,
            "estimated_net_savings_usd": 0.0,
            "avoids_immediate_compaction": False,
        }])

    def test_context_metrics_do_not_count_rejected_candidates_as_applied_savings(self):
        got = run_worker.context_metrics([
            {"type": "agentic_metrics", "metric": "context_projection", "turn_count": 6,
             "context_projection": {"decision": "keep_compact_threshold", "projection_count": 3,
                                    "tokens_saved": 12000, "bytes_saved": 27000}},
        ])
        self.assertEqual(got["candidate_tools"], 3)
        self.assertEqual(got["candidate_tokens_saved"], 12000)
        self.assertEqual(got["projected_tools"], 0)
        self.assertEqual(got["tokens_saved"], 0)
        self.assertEqual(got["bytes_saved"], 0)
        self.assertFalse(got["projection_batches"][0]["applied"])

    def test_context_metrics_do_not_count_pending_status_as_projection_attempt(self):
        got = run_worker.context_metrics([
            {"type": "agentic_metrics", "metric": "context_projection", "turn_count": 5,
             "context_projection": {"pending_only": True, "pending_tools": 2, "pending_tokens": 7000}},
        ])
        self.assertEqual(got["projection_turns"], [])
        self.assertEqual(got["projection_batches"], [])
        self.assertEqual(got["candidate_tools"], 0)

    def test_context_metrics_capture_context_update_shadow(self):
        got = run_worker.context_metrics([
            {"type": "agentic_metrics", "metric": "context_update", "turn_count": 3,
             "context_update": {"action": "REWRITE", "reason_code": "PARTIAL_VALUE",
                                "runtime_candidate": True, "applied": False}},
            {"type": "agentic_metrics", "metric": "context_update", "turn_count": 4,
             "context_update": {"action": "KEEP", "reason_code": "FAILED_DIAGNOSTIC",
                                "runtime_candidate": False, "applied": False}},
        ])
        self.assertEqual(got["context_update_proposals"], 2)
        self.assertEqual(got["context_update_runtime_candidates"], 1)
        self.assertEqual(got["context_update_actions"], {"REWRITE": 1, "KEEP": 1})
        self.assertEqual(got["context_update_reason_codes"], {
            "PARTIAL_VALUE": 1,
            "FAILED_DIAGNOSTIC": 1,
        })

    def test_estimated_cost_accepts_model_specific_pricing(self):
        usage = {"input_tokens": 1000, "cached_input_tokens": 800, "output_tokens": 100}
        got = run_worker.estimated_cost(usage, {
            "input": 0.14, "cached": 0.0028, "cache_write": 0.0, "output": 0.28,
        })
        self.assertAlmostEqual(got, 0.00005824)

    def test_luban_command_accepts_benchmark_provider_model_and_effort(self):
        command = run_worker.agent_command(
            "luban", "/tmp/luban", Path("/tmp/repo"), "prompt", Path("/tmp/debug"),
            "http://127.0.0.1:1234", provider_name="deepseek",
            model="deepseek-v4-flash", reasoning_effort="high",
        )
        self.assertEqual(command[command.index("--provider") + 1], "deepseek")
        self.assertEqual(command[command.index("--model") + 1], "deepseek-v4-flash")
        self.assertEqual(command[command.index("--reasoning-effort") + 1], "high")

    def test_luban_command_accepts_max_turns(self):
        command = run_worker.agent_command(
            "luban", "/tmp/luban", Path("/tmp/repo"), "prompt", Path("/tmp/debug"),
            "http://127.0.0.1:1234", max_turns=17,
        )
        self.assertEqual(command[command.index("--max-turns") + 1], "17")


if __name__ == "__main__":
    unittest.main()
