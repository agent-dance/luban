"""CompositeVerifier registry for the shared verifier."""

from __future__ import annotations

from typing import Any

from workbuddy_bench.judge import PassRateScoringPolicy, VerifierRegistry
from workbuddy_bench.judge.runtime import HarborCommandExecutor
from workbuddy_bench.judge.runners.rule import (
    HarborScriptRuleJudgeRunner,
    ScriptRuleJudgeRunner,
)

from .items import build_plan
from .manifest import load_manifest
from .rule import JUDGE_TYPE, prepare_command


def build_registry(build_context: Any) -> VerifierRegistry:
    contract = build_context.contract
    manifest = load_manifest(contract.task_dir)
    runtime = build_context.runtime
    runner = (
        HarborScriptRuleJudgeRunner(runtime)
        if runtime is not None
        else ScriptRuleJudgeRunner()
    )

    async def prepare(context: Any) -> None:
        if runtime is None:
            return
        environment_dir = contract.task_dir / "environment"
        if (environment_dir / "scorer.py").is_file():
            await runtime.upload_dir(source_dir=environment_dir, target_dir="/environment")
        await HarborCommandExecutor(runtime.environment).run(
            prepare_command(),
            cwd="/workspace",
            env=context.env,
            timeout_sec=120,
        )

    def plan_builder(context: Any):
        return build_plan(
            dataset_id=contract.dataset_id,
            task_id=context.task_id,
            source_case=contract.source_case,
            manifest=manifest,
            timeout_sec=_task_timeout_sec(contract.task_config),
        )

    return VerifierRegistry(
        plan_builder=plan_builder,
        prepare=prepare,
        judge_runners={JUDGE_TYPE: runner},
        scoring_policy=PassRateScoringPolicy(),
    )


def _task_timeout_sec(task_config: dict[str, Any]) -> float | None:
    verifier = task_config.get("verifier") or {}
    if not isinstance(verifier, dict):
        return None
    value = verifier.get("timeout_sec")
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    return float(value)
