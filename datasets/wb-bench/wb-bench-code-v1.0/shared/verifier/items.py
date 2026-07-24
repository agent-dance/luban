"""Plan construction."""

from __future__ import annotations

from workbuddy_bench.judge import EvaluationItem, EvaluationPlan

from .manifest import VerifierManifest
from .rule import build_rule_judge


def build_plan(
    *,
    dataset_id: str,
    task_id: str,
    source_case: str,
    manifest: VerifierManifest,
    timeout_sec: float | None,
) -> EvaluationPlan:
    item_id = f"{source_case}:rule"
    return EvaluationPlan(
        dataset_id=dataset_id,
        task_id=task_id,
        items=[
            EvaluationItem(
                id=item_id,
                type="rule",
                label="Deterministic verifier",
                weight=1.0,
                metadata={"family": manifest.family.value},
            )
        ],
        judges=[build_rule_judge(item_id=item_id, manifest=manifest, timeout_sec=timeout_sec)],
        evidence_requirements=["agent-patch", "task-tests"],
        scoring_policy="pass_rate",
        metadata={"family": manifest.family.value, "source_case": source_case},
    )
