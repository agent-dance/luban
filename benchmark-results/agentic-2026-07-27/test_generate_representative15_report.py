#!/usr/bin/env python3

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("generate_representative15_report.py")


def load_module():
    spec = importlib.util.spec_from_file_location("representative15_report", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


reporter = load_module()


def summary(
    instance_id,
    *,
    elapsed,
    tokens,
    calls,
    timed_out=False,
    exit_code=0,
    patch_files=1,
):
    return {
        "instance_id": instance_id,
        "agent": "luban",
        "reasoning_effort": "medium",
        "elapsed_seconds": elapsed,
        "timed_out": timed_out,
        "exit_code": exit_code,
        "usage": {"input_tokens": tokens, "output_tokens": 0},
        "llm_calls": calls,
        "patch": {
            "files_changed": patch_files,
            "additions": patch_files * 2,
            "deletions": patch_files,
        },
    }


class Representative15OptimizationReportTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.candidates = self.root / "candidates"
        self.baseline_root = self.candidates / "baseline"
        self.optimized_root = self.candidates / "optimized"
        self.original = self.candidates / "original.json"
        self.original.parent.mkdir(parents=True)
        self.original.write_text(json.dumps({"tasks": []}), encoding="utf-8")

        self._write_baseline("task-a", "luban", summary("task-a", elapsed=100, tokens=1000, calls=10, patch_files=2))
        codex_a = summary("task-a", elapsed=80, tokens=800, calls=8, patch_files=2)
        codex_a["agent"] = "codex"
        self._write_baseline("task-a", "codex", codex_a)
        self._write_baseline(
            "task-b",
            "luban",
            summary("task-b", elapsed=180, tokens=0, calls=2, timed_out=True, patch_files=0),
        )
        codex_b = summary("task-b", elapsed=100, tokens=500, calls=5, patch_files=1)
        codex_b["agent"] = "codex"
        self._write_baseline("task-b", "codex", codex_b)

        self._write_optimized("task-a", "luban", summary("task-a", elapsed=70, tokens=700, calls=7, patch_files=3))
        self._write_optimized(
            "task-b",
            "luban",
            summary("task-b", elapsed=90, tokens=600, calls=6, timed_out=True, patch_files=3),
        )
        self._write_optimized(
            "task-a",
            "luban-network-failed",
            summary("task-a", elapsed=10, tokens=100, calls=1, exit_code=1, patch_files=0),
        )

        self.patches = [
            mock.patch.object(reporter, "CANDIDATES", self.candidates),
            mock.patch.object(reporter, "NEW_ROOT", self.baseline_root),
            mock.patch.object(reporter, "OPTIMIZED_ROOT", self.optimized_root),
            mock.patch.object(reporter, "ORIGINAL", self.original),
            mock.patch.object(reporter, "ORDER", ("task-a", "task-b")),
            mock.patch.object(reporter, "OPTIMIZED_INSTANCE_IDS", frozenset({"task-a", "task-b"})),
            mock.patch.object(reporter, "LABELS", {"task-a": "A", "task-b": "B"}),
            mock.patch.object(
                reporter,
                "OPTIMIZATION_DEFINITIONS",
                (
                    {
                        "id": "test-change",
                        "title": "测试优化",
                        "what_changed": "缩短循环。",
                        "reason": "避免空转。",
                        "affected_tasks": ("task-a", "task-b"),
                    },
                ),
            ),
        ]
        for patch in self.patches:
            patch.start()

    def tearDown(self):
        for patch in reversed(self.patches):
            patch.stop()
        self.temp.cleanup()

    def _write_baseline(self, instance_id, agent, value):
        path = self.baseline_root / "runs" / instance_id / agent / "summary.json"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value), encoding="utf-8")

    def _write_optimized(self, instance_id, agent, value):
        path = self.optimized_root / "runs" / instance_id / agent / "summary.json"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value), encoding="utf-8")

    def test_build_preserves_baseline_and_selects_only_eligible_canonical_reruns(self):
        report = reporter.build()
        by_id = {task["instance_id"]: task for task in report["tasks"]}

        selected = by_id["task-a"]
        self.assertEqual(selected["baseline"]["elapsed_seconds"], 100)
        self.assertEqual(selected["optimized"]["elapsed_seconds"], 70)
        self.assertEqual(selected["optimized"]["measurement_status"], "rerun")
        self.assertEqual(selected["luban_elapsed_seconds"], 70)
        self.assertEqual(selected["comparison"]["baseline_vs_codex"]["elapsed_change_percent"], 25.0)
        self.assertEqual(selected["comparison"]["optimized_vs_codex"]["elapsed_change_percent"], -12.5)
        self.assertEqual(selected["comparison"]["optimized_vs_baseline"]["elapsed_change_percent"], -30.0)
        self.assertEqual(selected["comparison"]["optimized_vs_codex"]["patch_file_change"], 1)

        rejected = by_id["task-b"]
        self.assertEqual(rejected["optimized_override"]["status"], "rejected")
        self.assertIn("timed_out", rejected["optimized_override"]["rejection_reasons"])
        self.assertEqual(rejected["optimized"]["elapsed_seconds"], 180)
        self.assertEqual(rejected["optimized"]["measurement_status"], "baseline_carried_forward")
        self.assertIsNone(rejected["optimized"]["total_tokens"])

        self.assertEqual(report["optimization_reruns"]["selected_instances"], ["task-a"])
        self.assertEqual(len(report["optimization_reruns"]["excluded_diagnostic_runs"]), 1)
        self.assertEqual(
            report["optimization_reruns"]["excluded_diagnostic_runs"][0]["reason"],
            "network_failure",
        )
        effect = report["optimizations"][0]["actual_effect"]
        self.assertEqual(effect["status"], "partial")
        self.assertEqual(effect["tasks_measured"], ["task-a"])
        self.assertEqual(effect["optimized_vs_baseline"]["elapsed_change_percent"], -30.0)

    def test_render_explains_metrics_effects_and_exclusions(self):
        rendered = reporter.render(reporter.build())

        self.assertIn("优化前 → 优化后 / Codex", rendered)
        self.assertIn("做了什么", rendered)
        self.assertIn("原因", rendered)
        self.assertIn("实际效果", rendered)
        self.assertIn("网络失败", rendered)
        self.assertIn("诊断/超时跑不入选", rendered)
        self.assertIn("未下载 Docker 镜像", rendered)
        self.assertIn("未做官方判分", rendered)
        self.assertIn("100.0s → 70.0s / 80.0s (−12.5%)", rendered)

    def test_canonical_rerun_without_patch_is_rejected(self):
        no_patch = summary("task-a", elapsed=60, tokens=600, calls=6, patch_files=0)
        self._write_optimized("task-a", "luban", no_patch)

        report = reporter.build()
        task = report["tasks"][0]
        self.assertEqual(task["optimized_override"]["status"], "rejected")
        self.assertIn("no_patch", task["optimized_override"]["rejection_reasons"])
        self.assertEqual(task["optimized"]["elapsed_seconds"], 100)


if __name__ == "__main__":
    unittest.main()
