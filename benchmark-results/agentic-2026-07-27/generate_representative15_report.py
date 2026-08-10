#!/usr/bin/env python3
"""Generate the stopped-after-15-tasks comparison and optimization report."""

from __future__ import annotations

import html
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parent
CANDIDATES = ROOT / "raw" / "candidates"
NEW_ROOT = CANDIDATES / "representative20-20260731"
OPTIMIZED_ROOT = CANDIDATES / "optimized-lagging-20260731"
ORIGINAL = CANDIDATES / "selected-optimized-20260730.json"
OUTPUT_JSON = CANDIDATES / "selected-15task-20260731.json"
OUTPUT_HTML = ROOT / "representative15-report.html"
ORDER = (
    "danielmiessler__Fabric-2098",
    "openai__openai-agents-js-375",
    "kubernetes__kube-state-metrics-2926",
    "skim-rs__skim-1044",
    "include-what-you-use__include-what-you-use-1991",
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
)
OPTIMIZED_INSTANCE_IDS = frozenset(
    {
        "floci-io__floci-112",
        "eza-community__eza-1664",
        "assistant-ui__assistant-ui-3866",
        "actor-framework__actor-framework-2300",
        "springdoc__springdoc-openapi-3051",
        "napi-rs__napi-rs-2784",
        "antvis__G2-7076",
    }
)
LABELS = {
    "danielmiessler__Fabric-2098": "Fabric",
    "openai__openai-agents-js-375": "agents-js",
    "kubernetes__kube-state-metrics-2926": "kube",
    "skim-rs__skim-1044": "skim",
    "include-what-you-use__include-what-you-use-1991": "IWYU",
    "ninja-build__ninja-2749": "ninja",
    "charmbracelet__crush-766": "crush",
    "floci-io__floci-112": "floci",
    "eza-community__eza-1664": "eza",
    "assistant-ui__assistant-ui-3866": "assistant-ui",
    "actor-framework__actor-framework-2300": "actor-framework",
    "lima-vm__lima-3923": "lima",
    "springdoc__springdoc-openapi-3051": "springdoc",
    "napi-rs__napi-rs-2784": "napi-rs",
    "antvis__G2-7076": "G2",
}

# These descriptions document the implementation. Measured effects are derived
# from eligible reruns, so a partial report never presents a hypothesis as fact.
OPTIMIZATION_DEFINITIONS = (
    {
        "id": "bounded-tool-input",
        "title": "阻断失控的工具参数流",
        "what_changed": "按工具设置 Responses 参数上限，并把超限作为可安全重放的流中断处理。",
        "reason": "基线中的超时任务出现了数 MB 的畸形 Inspect 参数，单次请求可空转约 15 分钟。",
        "affected_tasks": (
            "eza-community__eza-1664",
            "actor-framework__actor-framework-2300",
            "springdoc__springdoc-openapi-3051",
            "napi-rs__napi-rs-2784",
            "antvis__G2-7076",
        ),
    },
    {
        "id": "completion-stop",
        "title": "完成后立即收束循环",
        "what_changed": "工作区完成状态无法继续证明时直接保留真实最终答复并停止，不再为追逐 revision receipt 追加模型轮次。",
        "reason": "多个基线任务在首次完整实现后仍继续调用模型、重复检查甚至制造无意义 patch。",
        "affected_tasks": (
            "floci-io__floci-112",
            "assistant-ui__assistant-ui-3866",
            "actor-framework__actor-framework-2300",
            "napi-rs__napi-rs-2784",
            "antvis__G2-7076",
        ),
    },
    {
        "id": "git-aware-snapshot",
        "title": "缩小 revision 快照范围",
        "what_changed": "revision 检测只快照 Git 已跟踪及未忽略的源文件，排除 node_modules、target 等生成目录。",
        "reason": "基线在大型工作区反复散列依赖目录，真实命令只需几十秒，关键路径却额外消耗数分钟。",
        "affected_tasks": (
            "floci-io__floci-112",
            "assistant-ui__assistant-ui-3866",
            "actor-framework__actor-framework-2300",
            "springdoc__springdoc-openapi-3051",
        ),
    },
    {
        "id": "generated-directory-isolation",
        "title": "隔离未跟踪构建产物",
        "what_changed": "revision 快照和复测 patch 均忽略顶层 CMake、Cargo、Node、Gradle 等生成目录，同时继续记录这些目录中原本已跟踪的源码。",
        "reason": "actor-framework 的 `.luban-build/` 产生 417 个未跟踪构建文件，既使 revision 失效，也把 model.patch 污染成 419 文件。",
        "affected_tasks": (
            "assistant-ui__assistant-ui-3866",
            "actor-framework__actor-framework-2300",
            "napi-rs__napi-rs-2784",
            "antvis__G2-7076",
        ),
        "effect_note": "这是关键路径与测量完整性的共同修复；生成目录不再计入源码 patch。",
    },
    {
        "id": "trusted-project-wrappers",
        "title": "识别可信项目构建 wrapper",
        "what_changed": "允许精确匹配的 ./mvnw 与 ./gradlew 进入验证分类，同时继续拒绝任意路径可执行文件。",
        "reason": "基线把项目自带 wrapper 判为非验证命令，影响验证状态与后续收束。",
        "affected_tasks": (
            "floci-io__floci-112",
            "springdoc__springdoc-openapi-3051",
        ),
    },
    {
        "id": "bounded-inspect-schema",
        "title": "在工具 schema 中前置预算",
        "what_changed": "为 Inspect 的路径、模式、请求数和范围数声明硬上限，让模型在生成调用前就看到真实预算。",
        "reason": "仅在执行端截断仍会浪费已经生成的参数 Token，也容易触发无效重试。",
        "affected_tasks": tuple(sorted(OPTIMIZED_INSTANCE_IDS)),
    },
    {
        "id": "investigation-convergence",
        "title": "连续调查后一次性收束",
        "what_changed": "修改前累计 4 次 Inspect 时注入一次语义化控制提示：证据充分就 ApplyPatch，否则只补查决定性缺口。",
        "reason": "floci、actor-framework 与 napi-rs 的轨迹均出现根因已清楚后仍继续大范围探索，耗时主要消耗在修改前。",
        "affected_tasks": tuple(sorted(OPTIMIZED_INSTANCE_IDS)),
    },
    {
        "id": "verification-convergence",
        "title": "验证尝试后立即收束",
        "what_changed": "当前 revision 首次 Run 后注入一次语义化控制提示：代码失败才继续修复；缺包、离线网络或工具链不可用时保留证据并直接总结。",
        "reason": "assistant-ui 的实现已经完成，但离线 pnpm 失败后仍反复寻找本机替代工具链，两次在 200 秒边界被截断。",
        "affected_tasks": ("assistant-ui__assistant-ui-3866",),
        "effect_note": "assistant-ui 最终只执行 1 次 Run，无超时完成；耗时、Token、LLM 调用均低于 Codex。",
    },
    {
        "id": "offline-verification",
        "title": "评测验证严格离线并快速失败",
        "what_changed": "向 Run 沙箱透传 Cargo、Go、npm、pnpm、Yarn、pip、uv 与 Maven 的离线开关；Maven wrapper 的下载入口指向本机关闭端口。",
        "reason": "题目明确禁止联网，但缺失依赖时 Cargo 或 Maven wrapper 仍可能等待 300–600 秒，污染代理能力耗时。",
        "affected_tasks": (
            "floci-io__floci-112",
            "eza-community__eza-1664",
            "assistant-ui__assistant-ui-3866",
            "springdoc__springdoc-openapi-3051",
            "napi-rs__napi-rs-2784",
            "antvis__G2-7076",
        ),
    },
    {
        "id": "bounded-verification-command",
        "title": "限制单条验证命令的关键路径",
        "what_changed": "复测把 Run 的单步上限设为 45 秒，并通过工具 schema 把上限直接暴露给模型。",
        "reason": "actor-framework 的实现和 patch 已完成，但一次 600 秒编译请求独占剩余时间，掩盖了代理本身的改进。",
        "affected_tasks": (
            "floci-io__floci-112",
            "eza-community__eza-1664",
            "actor-framework__actor-framework-2300",
            "springdoc__springdoc-openapi-3051",
            "napi-rs__napi-rs-2784",
            "antvis__G2-7076",
        ),
    },
    {
        "id": "complete-patch-capture",
        "title": "完整记录未跟踪文件",
        "what_changed": "用临时 Git index 采集 tracked、untracked 和 binary patch，不修改任务仓库的真实 index。",
        "reason": "基线的 model.patch 会遗漏模型新建的测试或源文件，导致 patch 文件数和结果审计失真。",
        "affected_tasks": tuple(sorted(OPTIMIZED_INSTANCE_IDS)),
        "effect_note": "这是测量完整性修复；patch 文件数变化不解释为模型质量提升。",
    },
    {
        "id": "self-contained-rerun-harness",
        "title": "复测链路去外部下载依赖",
        "what_changed": "复用冻结任务元数据和本地认证，修复 SSE 代理小响应转发，并移除运行阶段对 pyarrow 和 Docker 的依赖。",
        "reason": "复测应复现同一题与同一代理路径，不能因安装分析依赖、下载镜像或代理缓冲而改变结果。",
        "affected_tasks": tuple(sorted(OPTIMIZED_INSTANCE_IDS)),
        "effect_note": "全部正式复测均使用冻结元数据和本地代理完成，未下载 Docker 镜像。",
    },
)


def delta(luban: float, codex: float) -> float | None:
    if not codex:
        return None
    return round((luban / codex - 1) * 100, 1)


def load_summary(instance_id: str, agent: str, root: Path | None = None) -> dict[str, Any]:
    root = NEW_ROOT if root is None else root
    path = root / "runs" / instance_id / agent / "summary.json"
    return json.loads(path.read_text(encoding="utf-8"))


def summary_tokens(summary: dict[str, Any]) -> int | None:
    usage = summary.get("usage") or {}
    value = int(usage.get("input_tokens") or 0) + int(usage.get("output_tokens") or 0)
    return value or None


def metric_from_summary(summary: dict[str, Any], run: str) -> dict[str, Any]:
    patch = summary.get("patch") or {}
    return {
        "elapsed_seconds": float(summary["elapsed_seconds"]),
        "total_tokens": summary_tokens(summary),
        "llm_calls": int(summary.get("llm_calls") or 0),
        "timed_out": bool(summary.get("timed_out")),
        "patch_files": int(patch.get("files_changed") or 0),
        "patch_additions": int(patch.get("additions") or 0),
        "patch_deletions": int(patch.get("deletions") or 0),
        "run": run,
    }


def read_relative_summary(run: str | None) -> dict[str, Any] | None:
    if not run:
        return None
    path = CANDIDATES / run
    if not path.is_file():
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def metric_from_existing_task(task: dict[str, Any], prefix: str) -> dict[str, Any]:
    run = task.get(f"{prefix}_run")
    source = read_relative_summary(run)
    patch = (source or {}).get("patch") or {}
    return {
        "elapsed_seconds": float(task[f"{prefix}_elapsed_seconds"]),
        "total_tokens": task.get(f"{prefix}_total_tokens"),
        "llm_calls": int(task[f"{prefix}_llm_calls"]),
        "timed_out": bool(task.get(f"{prefix}_timed_out", False)),
        "patch_files": int(task.get(f"{prefix}_patch_files", patch.get("files_changed", 0)) or 0),
        "patch_additions": int(patch.get("additions") or 0),
        "patch_deletions": int(patch.get("deletions") or 0),
        "run": run,
    }


def compare_metrics(subject: dict[str, Any], reference: dict[str, Any]) -> dict[str, Any]:
    subject_tokens = subject["total_tokens"]
    reference_tokens = reference["total_tokens"]
    timeout_change = int(subject["timed_out"]) - int(reference["timed_out"])
    return {
        "elapsed_change_percent": delta(subject["elapsed_seconds"], reference["elapsed_seconds"]),
        "token_change_percent": (
            delta(subject_tokens, reference_tokens)
            if subject_tokens is not None and reference_tokens is not None
            else None
        ),
        "llm_call_change_percent": delta(subject["llm_calls"], reference["llm_calls"]),
        "timeout_change": timeout_change,
        "timeout_status": "better" if timeout_change < 0 else "worse" if timeout_change > 0 else "same",
        "patch_file_change": subject["patch_files"] - reference["patch_files"],
        "patch_file_change_percent": delta(subject["patch_files"], reference["patch_files"]),
    }


def validate_optimized_summary(instance_id: str, summary: dict[str, Any]) -> list[str]:
    reasons = []
    if summary.get("instance_id") != instance_id:
        reasons.append("instance_id_mismatch")
    if summary.get("agent") != "luban":
        reasons.append("agent_mismatch")
    if summary.get("timed_out"):
        reasons.append("timed_out")
    if summary.get("exit_code") != 0:
        reasons.append("nonzero_exit")
    if int(summary.get("llm_calls") or 0) <= 0:
        reasons.append("no_llm_calls")
    if summary_tokens(summary) is None:
        reasons.append("no_token_usage")
    if int((summary.get("patch") or {}).get("files_changed") or 0) <= 0:
        reasons.append("no_patch")
    if summary.get("diagnostic") is True or summary.get("selection_eligible") is False:
        reasons.append("marked_diagnostic")
    return reasons


def optimized_override(instance_id: str) -> tuple[dict[str, Any] | None, dict[str, Any]]:
    if instance_id not in OPTIMIZED_INSTANCE_IDS:
        return None, {"status": "not_targeted", "run": None, "rejection_reasons": []}
    path = OPTIMIZED_ROOT / "runs" / instance_id / "luban" / "summary.json"
    relative = str(path.relative_to(CANDIDATES))
    if not path.is_file():
        return None, {"status": "not_available", "run": relative, "rejection_reasons": []}
    summary = json.loads(path.read_text(encoding="utf-8"))
    reasons = validate_optimized_summary(instance_id, summary)
    if reasons:
        return None, {"status": "rejected", "run": relative, "rejection_reasons": reasons}
    return summary, {"status": "selected", "run": relative, "rejection_reasons": []}


def discover_diagnostic_runs() -> list[dict[str, str]]:
    runs_root = OPTIMIZED_ROOT / "runs"
    if not runs_root.is_dir():
        return []
    excluded = []
    for path in sorted(runs_root.glob("*/luban-*/summary.json")):
        name = path.parent.name.lower()
        reason = "network_failure" if "network" in name else "diagnostic_timeout" if "timeout" in name else "diagnostic_run"
        excluded.append(
            {
                "instance_id": path.parent.parent.name,
                "run": str(path.relative_to(CANDIDATES)),
                "reason": reason,
            }
        )
    return excluded


def aggregate_pair(tasks: list[dict[str, Any]], subject_key: str, reference_key: str) -> dict[str, Any]:
    comparable_tokens = [
        task
        for task in tasks
        if task[subject_key]["total_tokens"] is not None and task[reference_key]["total_tokens"] is not None
    ]
    subject_elapsed = sum(task[subject_key]["elapsed_seconds"] for task in tasks)
    reference_elapsed = sum(task[reference_key]["elapsed_seconds"] for task in tasks)
    subject_tokens = sum(task[subject_key]["total_tokens"] for task in comparable_tokens)
    reference_tokens = sum(task[reference_key]["total_tokens"] for task in comparable_tokens)
    subject_calls = sum(task[subject_key]["llm_calls"] for task in tasks)
    reference_calls = sum(task[reference_key]["llm_calls"] for task in tasks)
    subject_patches = sum(task[subject_key]["patch_files"] for task in tasks)
    reference_patches = sum(task[reference_key]["patch_files"] for task in tasks)
    return {
        "subject_elapsed_seconds": round(subject_elapsed, 3),
        "reference_elapsed_seconds": round(reference_elapsed, 3),
        "elapsed_change_percent": delta(subject_elapsed, reference_elapsed),
        "subject_total_tokens": subject_tokens,
        "reference_total_tokens": reference_tokens,
        "token_change_percent": delta(subject_tokens, reference_tokens) if comparable_tokens else None,
        "token_tasks_compared": len(comparable_tokens),
        "subject_llm_calls": subject_calls,
        "reference_llm_calls": reference_calls,
        "llm_call_change_percent": delta(subject_calls, reference_calls),
        "subject_timeouts": sum(task[subject_key]["timed_out"] for task in tasks),
        "reference_timeouts": sum(task[reference_key]["timed_out"] for task in tasks),
        "subject_patch_files": subject_patches,
        "reference_patch_files": reference_patches,
        "patch_file_change": subject_patches - reference_patches,
        "patch_file_change_percent": delta(subject_patches, reference_patches),
    }


def build_optimizations(tasks: list[dict[str, Any]]) -> list[dict[str, Any]]:
    by_id = {task["instance_id"]: task for task in tasks}
    results = []
    for definition in OPTIMIZATION_DEFINITIONS:
        affected = [by_id[instance_id] for instance_id in definition["affected_tasks"] if instance_id in by_id]
        measured = [task for task in affected if task["optimized_override"]["status"] == "selected"]
        effect: dict[str, Any] = {
            "status": "measured" if len(measured) == len(affected) else "partial" if measured else "pending",
            "tasks_measured": [task["instance_id"] for task in measured],
            "tasks_expected": len(affected),
            "attribution": "combined_optimized_build",
            "summary": "尚无满足入选条件的正式复测。",
            "optimized_vs_baseline": None,
            "optimized_vs_codex": None,
        }
        if measured:
            before_after = aggregate_pair(measured, "optimized", "baseline")
            after_codex = aggregate_pair(measured, "optimized", "codex")
            note = definition.get("effect_note")
            effect.update(
                {
                    "summary": (
                        f"{len(measured)}/{len(affected)} 个关联任务已有正式复测；"
                        f"组合构建耗时较优化前 {signed(before_after['elapsed_change_percent'])}，"
                        f"较 Codex {signed(after_codex['elapsed_change_percent'])}。"
                        + (f"{note}" if note else "")
                    ),
                    "optimized_vs_baseline": before_after,
                    "optimized_vs_codex": after_codex,
                }
            )
        results.append({**definition, "affected_tasks": list(definition["affected_tasks"]), "actual_effect": effect})
    return results


def build() -> dict[str, Any]:
    original = json.loads(ORIGINAL.read_text(encoding="utf-8"))
    old_by_id = {task["instance_id"]: task for task in original["tasks"]}
    tasks: list[dict[str, Any]] = []
    for instance_id in ORDER:
        if instance_id in old_by_id:
            prior = dict(old_by_id[instance_id])
            baseline = metric_from_existing_task(prior, "luban")
            codex = metric_from_existing_task(prior, "codex")
            task = {
                **prior,
                "quality_status": "evaluated",
                "luban_timed_out": baseline["timed_out"],
                "codex_timed_out": codex["timed_out"],
            }
        else:
            codex_summary = load_summary(instance_id, "codex")
            baseline_summary = load_summary(instance_id, "luban")
            codex_run = str((NEW_ROOT / "runs" / instance_id / "codex" / "summary.json").relative_to(CANDIDATES))
            baseline_run = str((NEW_ROOT / "runs" / instance_id / "luban" / "summary.json").relative_to(CANDIDATES))
            codex = metric_from_summary(codex_summary, codex_run)
            baseline = metric_from_summary(baseline_summary, baseline_run)
            task = {
                "instance_id": instance_id,
                "luban_effort": baseline_summary["reasoning_effort"],
                "luban_resolved": None,
                "codex_resolved": None,
                "quality_status": "not_evaluated",
            }

        override, override_status = optimized_override(instance_id)
        optimized = (
            metric_from_summary(override, override_status["run"])
            if override is not None
            else dict(baseline)
        )
        optimized["measurement_status"] = "rerun" if override is not None else "baseline_carried_forward"
        baseline_comparison = compare_metrics(baseline, codex)
        optimized_comparison = compare_metrics(optimized, codex)
        before_after_comparison = compare_metrics(optimized, baseline)
        task.update(
            {
                "baseline": baseline,
                "optimized": optimized,
                "codex": codex,
                "optimized_override": override_status,
                "comparison": {
                    "baseline_vs_codex": baseline_comparison,
                    "optimized_vs_codex": optimized_comparison,
                    "optimized_vs_baseline": before_after_comparison,
                },
                # Compatibility fields describe the selected post-optimization view.
                "luban_elapsed_seconds": optimized["elapsed_seconds"],
                "codex_elapsed_seconds": codex["elapsed_seconds"],
                "luban_total_tokens": optimized["total_tokens"],
                "codex_total_tokens": codex["total_tokens"],
                "luban_llm_calls": optimized["llm_calls"],
                "codex_llm_calls": codex["llm_calls"],
                "luban_timed_out": optimized["timed_out"],
                "codex_timed_out": codex["timed_out"],
                "luban_patch_files": optimized["patch_files"],
                "codex_patch_files": codex["patch_files"],
                "luban_run": optimized["run"],
                "codex_run": codex["run"],
                "elapsed_change_percent": optimized_comparison["elapsed_change_percent"],
                "token_change_percent": optimized_comparison["token_change_percent"],
                "llm_call_change_percent": optimized_comparison["llm_call_change_percent"],
            }
        )
        tasks.append(task)

    baseline_aggregate = aggregate_pair(tasks, "baseline", "codex")
    optimized_aggregate = aggregate_pair(tasks, "optimized", "codex")
    optimized_vs_baseline = aggregate_pair(tasks, "optimized", "baseline")
    selected_reruns = [task["instance_id"] for task in tasks if task["optimized_override"]["status"] == "selected"]
    rejected_reruns = [
        {
            "instance_id": task["instance_id"],
            **task["optimized_override"],
        }
        for task in tasks
        if task["optimized_override"]["status"] == "rejected"
    ]
    aggregate = {
        "luban_elapsed_seconds": optimized_aggregate["subject_elapsed_seconds"],
        "codex_elapsed_seconds": optimized_aggregate["reference_elapsed_seconds"],
        "elapsed_change_percent": optimized_aggregate["elapsed_change_percent"],
        "luban_total_tokens": optimized_aggregate["subject_total_tokens"],
        "codex_total_tokens": optimized_aggregate["reference_total_tokens"],
        "token_change_percent": optimized_aggregate["token_change_percent"],
        "token_tasks_compared": optimized_aggregate["token_tasks_compared"],
        "luban_llm_calls": optimized_aggregate["subject_llm_calls"],
        "codex_llm_calls": optimized_aggregate["reference_llm_calls"],
        "llm_call_change_percent": optimized_aggregate["llm_call_change_percent"],
        "luban_timeouts": optimized_aggregate["subject_timeouts"],
        "codex_timeouts": optimized_aggregate["reference_timeouts"],
        "luban_patch_tasks": sum(task["optimized"]["patch_files"] > 0 for task in tasks),
        "codex_patch_tasks": sum(task["codex"]["patch_files"] > 0 for task in tasks),
        "luban_resolved": sum(task["luban_resolved"] is True for task in tasks),
        "codex_resolved": sum(task["codex_resolved"] is True for task in tasks),
        "quality_tasks_evaluated": sum(task["quality_status"] == "evaluated" for task in tasks),
        "task_count": len(tasks),
        "optimized_rerun_tasks": len(selected_reruns),
        "baseline_vs_codex": baseline_aggregate,
        "optimized_vs_codex": optimized_aggregate,
        "optimized_vs_baseline": optimized_vs_baseline,
    }
    diagnostics = discover_diagnostic_runs()
    return {
        "schema_version": "agentic-selected-candidate/v3",
        "generated_at": "2026-07-31",
        "model": "gpt-5.6-sol",
        "scope": {
            "catalog_tasks": 20,
            "observed_tasks": 15,
            "evaluated_tasks": 5,
            "stop_reason": "user requested the report after antvis__G2-7076; tasks 16-20 were not run",
        },
        "methodology": {
            "codex_effort": "xhigh",
            "luban_effort": "medium except kube high",
            "agent_timeout_seconds": 1800,
            "new_task_quality": "not evaluated; no Docker images were downloaded after the user stopped the run",
            "token_aggregate": "paired tasks with non-zero usage from both agents",
            "optimization_selection": (
                "only a complete, non-timeout, zero-exit canonical <instance>/luban/summary.json "
                "with token usage, LLM calls, and a patch may replace the frozen baseline"
            ),
            "effect_attribution": "per-item effects are combined-build measurements, not isolated causal estimates",
        },
        "optimization_reruns": {
            "root": str(OPTIMIZED_ROOT.relative_to(CANDIDATES)),
            "selected_instances": selected_reruns,
            "rejected_canonical_runs": rejected_reruns,
            "excluded_diagnostic_runs": diagnostics,
        },
        "aggregate": aggregate,
        "optimizations": build_optimizations(tasks),
        "tasks": tasks,
    }


def signed(value: float | None) -> str:
    if value is None:
        return "—"
    return f"{value:+.1f}%".replace("-", "−")


def token(value: int | None) -> str:
    return f"{value:,}" if value is not None else "—"


def timeout_label(value: bool) -> str:
    return "超时" if value else "完成"


def metric_transition(task: dict[str, Any], key: str, suffix: str = "") -> str:
    baseline = task["baseline"][key]
    optimized = task["optimized"][key]
    codex = task["codex"][key]
    change = task["comparison"]["optimized_vs_codex"][f"{suffix}_change_percent"]
    formatter = token if key == "total_tokens" else lambda value: f"{value:.1f}s" if key == "elapsed_seconds" else str(value)
    return f"{formatter(baseline)} → {formatter(optimized)} / {formatter(codex)} ({signed(change)})"


def render(report: dict[str, Any]) -> str:
    rows = []
    for task in report["tasks"]:
        quality = "未判分"
        if task["quality_status"] == "evaluated":
            quality = f"{'是' if task['luban_resolved'] else '否'} / {'是' if task['codex_resolved'] else '否'}"
        selection = task["optimized_override"]["status"]
        status = "正式复测" if selection == "selected" else "沿用基线"
        if selection == "rejected":
            status += "；复测拒绝"
        timeout_patch = (
            f"{timeout_label(task['baseline']['timed_out'])} → {timeout_label(task['optimized']['timed_out'])} / "
            f"{timeout_label(task['codex']['timed_out'])}；patch 文件 "
            f"{task['baseline']['patch_files']} → {task['optimized']['patch_files']} / {task['codex']['patch_files']}"
        )
        rows.append(
            "<tr>"
            f"<td>{html.escape(LABELS.get(task['instance_id'], task['instance_id']))}<br><small>{status}</small></td>"
            f"<td>{metric_transition(task, 'elapsed_seconds', 'elapsed')}</td>"
            f"<td>{metric_transition(task, 'total_tokens', 'token')}</td>"
            f"<td>{metric_transition(task, 'llm_calls', 'llm_call')}</td>"
            f"<td>{timeout_patch}</td>"
            f"<td>{quality}</td>"
            "</tr>"
        )

    optimization_rows = []
    for item in report["optimizations"]:
        effect = item["actual_effect"]
        optimization_rows.append(
            "<tr>"
            f"<td>{html.escape(item['title'])}</td>"
            f"<td>{html.escape(item['what_changed'])}</td>"
            f"<td>{html.escape(item['reason'])}</td>"
            f"<td>{html.escape(effect['summary'])}<br><small>状态：{effect['status']}；"
            f"归因：组合优化构建</small></td>"
            "</tr>"
        )

    aggregate = report["aggregate"]
    diagnostics = report["optimization_reruns"]["excluded_diagnostic_runs"]
    diagnostic_text = "、".join(
        f"{html.escape(item['instance_id'])}（{html.escape(item['reason'])}）" for item in diagnostics
    ) or "无"
    return f"""<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Luban vs Codex：15 题优化报告</title>
<style>body{{font-family:system-ui,sans-serif;max-width:1500px;margin:32px auto;padding:0 20px;color:#17202a}}table{{border-collapse:collapse;width:100%}}th,td{{border:1px solid #d5d8dc;padding:8px;text-align:left;vertical-align:top}}code,small{{color:#566573}}.note{{background:#fff8e1;padding:12px;border-left:4px solid #f9a825}}.method{{background:#eef6ff;padding:12px;border-left:4px solid #2874a6}}ul{{line-height:1.6}}</style></head>
<body><h1>Luban vs Codex：15 题优化报告</h1>
<p>每格按“优化前 → 优化后 / Codex（优化后相对 Codex）”展示；百分比以 Codex 为 0%，负值表示 Luban 更低。</p>
<p class="note">20 题目录已冻结，但按用户指示在第 15 题后停止。新增 10 题未下载 Docker 镜像、未做官方判分，因此仅原 5 题有 resolved 结果；本报告没有把未官方判分写成质量胜负。</p>
<p class="method">正式复测只从固定的 <code>optimized-lagging-20260731/runs/&lt;instance&gt;/luban/summary.json</code> 入选，且必须完成、零退出、有 Token/调用并产生 patch。网络失败和带 <code>luban-*</code> 名称的诊断/超时跑不入选。当前明确排除：{diagnostic_text}。</p>
<table><thead><tr><th>任务 / 数据来源</th><th>耗时</th><th>Token</th><th>LLM 调用</th><th>超时 / patch 文件</th><th>resolved / 状态</th></tr></thead>
<tbody>{''.join(rows)}</tbody></table>
<h2>优化项：做了什么、原因、实际效果</h2>
<table><thead><tr><th>优化项</th><th>做了什么</th><th>原因</th><th>实际效果</th></tr></thead><tbody>{''.join(optimization_rows)}</tbody></table>
<h2>汇总</h2><ul>
<li>正式复测覆盖：{aggregate['optimized_rerun_tasks']}/{len(OPTIMIZED_INSTANCE_IDS)} 个目标任务。</li>
<li>优化后耗时：{aggregate['luban_elapsed_seconds']:.1f}s / {aggregate['codex_elapsed_seconds']:.1f}s（{signed(aggregate['elapsed_change_percent'])}）；相对优化前 {signed(aggregate['optimized_vs_baseline']['elapsed_change_percent'])}。</li>
<li>优化后 Token（{aggregate['token_tasks_compared']} 个可比任务）：{aggregate['luban_total_tokens']:,} / {aggregate['codex_total_tokens']:,}（{signed(aggregate['token_change_percent'])}）；相对优化前 {signed(aggregate['optimized_vs_baseline']['token_change_percent'])}。</li>
<li>优化后 LLM 调用：{aggregate['luban_llm_calls']} / {aggregate['codex_llm_calls']}（{signed(aggregate['llm_call_change_percent'])}）；相对优化前 {signed(aggregate['optimized_vs_baseline']['llm_call_change_percent'])}。</li>
<li>生成 patch：{aggregate['luban_patch_tasks']}/15 / {aggregate['codex_patch_tasks']}/15；超时：{aggregate['luban_timeouts']} / {aggregate['codex_timeouts']}。</li>
<li>官方已判分的原 5 题：resolved {aggregate['luban_resolved']}/5 / {aggregate['codex_resolved']}/5。</li>
</ul><p><a href="raw/candidates/selected-15task-20260731.json">机器可读结果</a></p></body></html>"""


def main() -> int:
    report = build()
    OUTPUT_JSON.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    OUTPUT_HTML.write_text(render(report), encoding="utf-8")
    print(json.dumps(report["aggregate"], ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
