#!/usr/bin/env python3
"""
loop_metrics.py — Query Loop 评估脚本

可重复运行，用于衡量 Go QueryLoop 与 TypeScript 原版之间的关键指标：
  - 延迟模拟（TTFT、单轮延迟、工具执行延迟）
  - 功能覆盖率（Go vs TS）
  - 错误恢复能力对比
  - 并发工具加速比

用法:
    python3 loop_metrics.py                   # 运行全部基准
    python3 loop_metrics.py --suite latency   # 只运行延迟基准
    python3 loop_metrics.py --suite coverage  # 只运行覆盖率分析
    python3 loop_metrics.py --suite recovery  # 只运行恢复能力分析
    python3 loop_metrics.py --suite concurrent # 只运行并发加速比
    python3 loop_metrics.py --json            # 以 JSON 格式输出结果
"""

import argparse
import json
import math
import random
import statistics
import sys
import time
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Optional

# ─────────────────────────────────────────────────────────────────────────────
# 数据结构
# ─────────────────────────────────────────────────────────────────────────────

@dataclass
class LatencyResult:
    name: str
    samples: list[float]
    unit: str = "ms"

    @property
    def mean(self) -> float:
        return statistics.mean(self.samples)

    @property
    def median(self) -> float:
        return statistics.median(self.samples)

    @property
    def p95(self) -> float:
        s = sorted(self.samples)
        idx = math.ceil(0.95 * len(s)) - 1
        return s[max(0, idx)]

    @property
    def p99(self) -> float:
        s = sorted(self.samples)
        idx = math.ceil(0.99 * len(s)) - 1
        return s[max(0, idx)]

    @property
    def stdev(self) -> float:
        return statistics.stdev(self.samples) if len(self.samples) > 1 else 0.0


@dataclass
class FeatureEntry:
    category: str
    feature: str
    ts_status: str    # ✅ / ❌
    go_status: str    # ✅ / ❌ / ⚠️
    go_file: str
    priority: str     # P0 / P1 / P2 / -


@dataclass
class RecoveryEntry:
    path: str
    ts_support: bool
    go_support: bool
    description: str


@dataclass
class ConcurrentResult:
    n_tools: int
    serial_ms: float
    parallel_ms: float

    @property
    def speedup(self) -> float:
        if self.parallel_ms == 0:
            return float("inf")
        return self.serial_ms / self.parallel_ms

    @property
    def efficiency(self) -> float:
        return self.speedup / self.n_tools


# ─────────────────────────────────────────────────────────────────────────────
# 功能覆盖率数据（基于 query-loop.md 第三节）
# ─────────────────────────────────────────────────────────────────────────────

FEATURE_MATRIX: list[FeatureEntry] = [
    # 核心循环
    FeatureEntry("核心循环", "基本 Agentic Loop（消息→LLM→工具→回填）", "✅", "✅", "query.go", "-"),
    FeatureEntry("核心循环", "最大轮次限制（MaxTurns=100）", "✅", "✅", "query.go", "-"),
    FeatureEntry("核心循环", "消息数硬限制（500条）", "✅", "✅", "query.go", "-"),
    FeatureEntry("核心循环", "System Reminder 注入", "✅", "✅", "query.go", "-"),
    FeatureEntry("核心循环", "Context 取消传播", "✅", "✅", "query.go", "-"),
    FeatureEntry("核心循环", "事件回调（onEvent）", "✅", "✅", "query.go", "-"),
    FeatureEntry("核心循环", "QueryConfig 不可变快照", "✅", "❌", "-", "P2"),
    FeatureEntry("核心循环", "模型动态切换（SetModel）", "✅", "✅", "query.go", "-"),

    # 流式处理
    FeatureEntry("流式处理", "SSE per-block 状态机", "✅", "✅", "query.go", "-"),
    FeatureEntry("流式处理", "交错工具调用（index-keyed map）", "✅", "✅", "query.go", "-"),
    FeatureEntry("流式处理", "ThinkingBlock 支持（签名保留）", "✅", "✅", "query.go", "-"),
    FeatureEntry("流式处理", "畸形 JSON 降级为 TextBlock", "✅", "✅", "query.go", "-"),
    FeatureEntry("流式处理", "OpenAI 兼容重复 JSON 解析", "✅", "✅", "query.go", "-"),
    FeatureEntry("流式处理", "StreamingToolExecutor（流中并发启动）", "✅", "❌", "-", "P1"),

    # 工具执行
    FeatureEntry("工具执行", "并发安全工具并行执行", "✅", "✅", "concurrent.go", "-"),
    FeatureEntry("工具执行", "非并发工具串行执行（保序）", "✅", "✅", "concurrent.go", "-"),
    FeatureEntry("工具执行", "PreToolUse hooks", "✅", "✅", "concurrent.go", "-"),
    FeatureEntry("工具执行", "PostToolUse hooks", "✅", "✅", "concurrent.go", "-"),
    FeatureEntry("工具执行", "工具阻塞（blocked by hook）", "✅", "✅", "concurrent.go", "-"),

    # 上下文管理
    FeatureEntry("上下文管理", "Microcompact（旧工具结果清除）", "✅", "✅", "query.go+compact", "-"),
    FeatureEntry("上下文管理", "ToolResultBudget（超大结果截断）", "✅", "✅", "query.go+compact", "-"),
    FeatureEntry("上下文管理", "ResultStore（超大结果落盘）", "✅", "✅", "query.go+compact", "-"),
    FeatureEntry("上下文管理", "Auto-compact（定时摘要压缩）", "✅", "✅", "query.go+compact", "-"),
    FeatureEntry("上下文管理", "Force truncate（强制截断）", "✅", "✅", "query.go", "-"),
    FeatureEntry("上下文管理", "Snip Compact（片段压缩）", "✅", "❌", "-", "P1"),
    FeatureEntry("上下文管理", "Reactive Compact（API 错误触发）", "✅", "❌", "-", "P0"),

    # 错误恢复
    FeatureEntry("错误恢复", "Token Budget 续传（90% 阈值）", "✅", "❌", "-", "P0"),
    FeatureEntry("错误恢复", "Task Budget（API 级输出控制）", "✅", "❌", "-", "P0"),
    FeatureEntry("错误恢复", "模型回退 + Tombstone", "✅", "❌", "-", "P1"),
    FeatureEntry("错误恢复", "collapse_drain_retry", "✅", "❌", "-", "P1"),
    FeatureEntry("错误恢复", "max_output_tokens_escalate", "✅", "❌", "-", "P0"),
    FeatureEntry("错误恢复", "stop_hook_blocking", "✅", "❌", "-", "P2"),
    FeatureEntry("错误恢复", "token_budget_continuation", "✅", "❌", "-", "P0"),

    # Hook 系统
    FeatureEntry("Hook 系统", "Stop hooks", "✅", "❌", "-", "P2"),
    FeatureEntry("Hook 系统", "TeammateIdle hooks", "✅", "❌", "-", "P2"),
    FeatureEntry("Hook 系统", "TaskCompleted hooks", "✅", "❌", "-", "P2"),
    FeatureEntry("Hook 系统", "Post-sampling hooks", "✅", "❌", "-", "P2"),
    FeatureEntry("Hook 系统", "Hook 输入修改（modifiedInput）", "✅", "✅", "concurrent.go", "-"),

    # 高级特性
    FeatureEntry("高级特性", "Memory prefetch（后台预取记忆）", "✅", "❌", "-", "P2"),
    FeatureEntry("高级特性", "Skill discovery prefetch", "✅", "❌", "-", "P2"),
    FeatureEntry("高级特性", "工具批次摘要（ToolUseSummary）", "✅", "❌", "-", "P2"),
    FeatureEntry("高级特性", "Tool use summary 异步解析", "✅", "❌", "-", "P2"),
    FeatureEntry("高级特性", "auto-dream / 记忆提取（fire-and-forget）", "✅", "❌", "-", "P2"),
]

RECOVERY_MATRIX: list[RecoveryEntry] = [
    RecoveryEntry("collapse_drain_retry", True, False, "流中断，context collapse 后重试"),
    RecoveryEntry("reactive_compact_retry", True, False, "context_window_full API 错误，摘要压缩后重试"),
    RecoveryEntry("max_output_tokens_escalate", True, False, "输出达 max_tokens，升级参数后继续"),
    RecoveryEntry("max_output_tokens_recovery", True, False, "多次 escalate 后降级恢复"),
    RecoveryEntry("stop_hook_blocking", True, False, "Stop hook 阻塞，循环等待释放"),
    RecoveryEntry("token_budget_continuation", True, False, "Token budget 续传，注入余量提示"),
    RecoveryEntry("next_turn (normal)", True, True, "正常工具结果回填，标准下一轮"),
    RecoveryEntry("force_truncate_fallback", True, True, "压缩失败后强制截断（最后手段）"),
]


# ─────────────────────────────────────────────────────────────────────────────
# Parity fixture coverage
# ─────────────────────────────────────────────────────────────────────────────

SCRIPT_DIR = Path(__file__).resolve().parent
LOOP_DIR = SCRIPT_DIR.parent
PARITY_DIR = LOOP_DIR / "testdata" / "parity"
TASKS_DIR = LOOP_DIR / "parity_tasks"


def _load_json(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def load_parity_fixtures(parity_dir: Path = PARITY_DIR) -> list[dict]:
    if not parity_dir.exists():
        return []
    fixtures = []
    for path in sorted(parity_dir.glob("*.json")):
        if path.name == "coverage_manifest.json":
            continue
        data = _load_json(path)
        data["_path"] = str(path.relative_to(LOOP_DIR))
        fixtures.append(data)
    return fixtures


def load_parity_manifest(parity_dir: Path = PARITY_DIR) -> dict:
    path = parity_dir / "coverage_manifest.json"
    if not path.exists():
        return {"schema_version": "1", "updated_at": None, "tasks": []}
    return _load_json(path)


def load_task_ids(tasks_dir: Path = TASKS_DIR) -> list[str]:
    task_ids = []
    if not tasks_dir.exists():
        return task_ids
    for path in sorted(tasks_dir.glob("task_*.json")):
        data = _load_json(path)
        task_id = data.get("id") or path.stem
        task_ids.append(task_id)
    return task_ids


def run_parity_suite() -> dict:
    fixtures = load_parity_fixtures()
    manifest = load_parity_manifest()
    task_ids = load_task_ids()

    fixture_by_id = {f["id"]: f for f in fixtures}
    tag_index: dict[str, dict] = {}
    for fixture in fixtures:
        status = fixture.get("status", "active")
        updated_at = fixture.get("updated_at")
        for tag in fixture.get("coverage_tags", []):
            entry = tag_index.setdefault(tag, {
                "tag": tag,
                "status": "pending",
                "fixtures": [],
                "updated_at": updated_at,
            })
            entry["fixtures"].append(fixture["id"])
            if status == "active":
                entry["status"] = "covered"
            if updated_at and (entry.get("updated_at") is None or updated_at > entry["updated_at"]):
                entry["updated_at"] = updated_at

    tasks = []
    manifest_task_ids = set()
    for item in manifest.get("tasks", []):
        task_id = item["task_id"]
        manifest_task_ids.add(task_id)
        fixtures_for_task = item.get("fixtures", [])
        active_fixtures = [
            fid for fid in fixtures_for_task
            if fixture_by_id.get(fid, {}).get("status") == "active"
        ]
        status = item.get("status", "pending")
        if active_fixtures and status != "pending":
            status = "covered"
        updated_candidates = [manifest.get("updated_at")]
        updated_candidates.extend(
            fixture_by_id[fid].get("updated_at") for fid in fixtures_for_task if fid in fixture_by_id
        )
        updated_candidates = [value for value in updated_candidates if value]
        tasks.append({
            "task_id": task_id,
            "status": status,
            "coverage_tags": item.get("coverage_tags", []),
            "fixtures": fixtures_for_task,
            "active_fixtures": active_fixtures,
            "updated_at": max(updated_candidates) if updated_candidates else None,
            "notes": item.get("notes", ""),
        })
        for tag in item.get("coverage_tags", []):
            entry = tag_index.setdefault(tag, {
                "tag": tag,
                "status": "pending",
                "fixtures": [],
                "updated_at": manifest.get("updated_at"),
            })
            if status == "covered" and entry["status"] != "covered":
                entry["status"] = "pending"
            if manifest.get("updated_at") and (entry.get("updated_at") is None or manifest["updated_at"] > entry["updated_at"]):
                entry["updated_at"] = manifest["updated_at"]

    missing_manifest_tasks = [task_id for task_id in task_ids if task_id not in manifest_task_ids]

    active_count = sum(1 for f in fixtures if f.get("status") == "active")
    pending_count = sum(1 for f in fixtures if f.get("status") in ("pending", "expected_failure"))
    covered_tasks = sum(1 for t in tasks if t["status"] == "covered")

    return {
        "updated_at": manifest.get("updated_at"),
        "fixture_count": len(fixtures),
        "active_fixture_count": active_count,
        "pending_fixture_count": pending_count,
        "task_count": len(task_ids),
        "covered_task_count": covered_tasks,
        "pending_task_count": len(tasks) - covered_tasks,
        "missing_manifest_tasks": missing_manifest_tasks,
        "fixtures": [
            {
                "id": f.get("id"),
                "status": f.get("status", "active"),
                "updated_at": f.get("updated_at"),
                "coverage_tags": f.get("coverage_tags", []),
                "parity_tasks": f.get("parity_tasks", []),
                "path": f.get("_path"),
            }
            for f in fixtures
        ],
        "tasks": tasks,
        "feature_coverage": sorted(tag_index.values(), key=lambda e: e["tag"]),
    }


# ─────────────────────────────────────────────────────────────────────────────
# 延迟模拟
# ─────────────────────────────────────────────────────────────────────────────

def simulate_stream_processing(n_events: int = 100, noise_pct: float = 0.1) -> float:
    """
    模拟 processStream() 的处理时间（µs 级，无真实 I/O）。
    返回毫秒数。
    """
    # 每个事件约 0.5µs 处理开销（模拟 map lookup + builder write）
    base_us = n_events * 0.5
    noise = base_us * noise_pct * (random.random() * 2 - 1)
    time.sleep((base_us + noise) * 1e-6)
    return (base_us + noise) / 1000.0


def simulate_ttft(api_latency_ms: float = 150.0, noise_pct: float = 0.2) -> float:
    """模拟首 token 延迟（ms）"""
    noise = api_latency_ms * noise_pct * random.random()
    return api_latency_ms + noise


def simulate_tool_execution(n_tools: int, tool_latency_ms: float = 50.0,
                            parallel: bool = False) -> float:
    """
    模拟工具执行总耗时（ms）。
    parallel=True 时取最大值（并发），parallel=False 时取总和（串行）。
    """
    latencies = [tool_latency_ms * (0.8 + 0.4 * random.random()) for _ in range(n_tools)]
    if parallel:
        return max(latencies)
    return sum(latencies)


def run_latency_suite(n_samples: int = 50) -> list[LatencyResult]:
    """运行延迟基准，返回各项 LatencyResult"""
    results = []
    rng = random.Random(42)  # 固定种子，可重复

    # TTFT 模拟
    ttft_samples = []
    for _ in range(n_samples):
        api_ms = rng.gauss(150, 20)  # 平均 150ms，σ=20ms
        ttft_samples.append(simulate_ttft(api_latency_ms=max(50, api_ms)))
    results.append(LatencyResult("TTFT (首 token 延迟)", ttft_samples))

    # processStream 开销（100 events/turn）
    stream_samples = []
    for _ in range(n_samples):
        n_ev = rng.randint(50, 300)
        stream_samples.append(simulate_stream_processing(n_ev))
    results.append(LatencyResult("processStream() 处理开销", stream_samples))

    # 单轮总延迟（API + stream + 1 tool）
    turn_samples = []
    for _ in range(n_samples):
        api_ms = max(50, rng.gauss(150, 20))
        stream_ms = simulate_stream_processing(rng.randint(50, 300))
        tool_ms = simulate_tool_execution(1, tool_latency_ms=max(10, rng.gauss(50, 15)))
        turn_samples.append(api_ms + stream_ms + tool_ms)
    results.append(LatencyResult("单轮总延迟（API+流+1工具）", turn_samples))

    # 工具执行延迟：串行 vs 并发（4 工具）
    serial_samples = []
    parallel_samples = []
    for _ in range(n_samples):
        serial_samples.append(simulate_tool_execution(4, parallel=False))
        parallel_samples.append(simulate_tool_execution(4, parallel=True))
    results.append(LatencyResult("工具执行（4工具，串行）", serial_samples))
    results.append(LatencyResult("工具执行（4工具，并发）", parallel_samples))

    return results


# ─────────────────────────────────────────────────────────────────────────────
# 覆盖率分析
# ─────────────────────────────────────────────────────────────────────────────

def run_coverage_suite() -> dict:
    """计算各维度功能覆盖率"""
    categories: dict[str, dict] = {}
    for entry in FEATURE_MATRIX:
        cat = entry.category
        if cat not in categories:
            categories[cat] = {"total": 0, "go_impl": 0, "features": []}
        categories[cat]["total"] += 1
        if entry.go_status == "✅":
            categories[cat]["go_impl"] += 1
        categories[cat]["features"].append(entry)

    total = sum(c["total"] for c in categories.values())
    total_impl = sum(c["go_impl"] for c in categories.values())

    return {
        "categories": categories,
        "total": total,
        "total_impl": total_impl,
        "overall_pct": total_impl / total * 100 if total > 0 else 0,
    }


# ─────────────────────────────────────────────────────────────────────────────
# 恢复能力分析
# ─────────────────────────────────────────────────────────────────────────────

def run_recovery_suite() -> dict:
    ts_count = sum(1 for r in RECOVERY_MATRIX if r.ts_support)
    go_count = sum(1 for r in RECOVERY_MATRIX if r.go_support)
    return {
        "paths": RECOVERY_MATRIX,
        "ts_count": ts_count,
        "go_count": go_count,
        "coverage_pct": go_count / ts_count * 100 if ts_count > 0 else 0,
    }


# ─────────────────────────────────────────────────────────────────────────────
# 并发加速比
# ─────────────────────────────────────────────────────────────────────────────

def run_concurrent_suite(tool_latency_ms: float = 50.0) -> list[ConcurrentResult]:
    results = []
    rng = random.Random(42)
    for n in [2, 4, 6, 8]:
        serial_times = []
        parallel_times = []
        for _ in range(20):
            latencies = [max(10, rng.gauss(tool_latency_ms, tool_latency_ms * 0.2))
                         for _ in range(n)]
            serial_times.append(sum(latencies))
            parallel_times.append(max(latencies))
        results.append(ConcurrentResult(
            n_tools=n,
            serial_ms=statistics.mean(serial_times),
            parallel_ms=statistics.mean(parallel_times),
        ))
    return results


# ─────────────────────────────────────────────────────────────────────────────
# 输出格式化
# ─────────────────────────────────────────────────────────────────────────────

BOLD = "\033[1m"
GREEN = "\033[92m"
RED = "\033[91m"
YELLOW = "\033[93m"
CYAN = "\033[96m"
RESET = "\033[0m"
DIM = "\033[2m"


def _color(text: str, color: str, use_color: bool) -> str:
    return f"{color}{text}{RESET}" if use_color else text


def print_header(title: str, use_color: bool = True) -> None:
    width = 72
    line = "─" * width
    print()
    print(_color(line, CYAN, use_color))
    print(_color(f"  {title}", BOLD, use_color))
    print(_color(line, CYAN, use_color))


def print_latency_table(results: list[LatencyResult], use_color: bool = True) -> None:
    print_header("延迟基准结果", use_color)
    header = f"{'指标':<35} {'均值':>8} {'P50':>8} {'P95':>8} {'P99':>8} {'σ':>8}"
    print(_color(header, DIM, use_color))
    print(_color("─" * 72, DIM, use_color))
    for r in results:
        row = (f"{r.name:<35} {r.mean:>7.2f}{r.unit[0]}"
               f" {r.median:>7.2f}{r.unit[0]}"
               f" {r.p95:>7.2f}{r.unit[0]}"
               f" {r.p99:>7.2f}{r.unit[0]}"
               f" {r.stdev:>7.2f}{r.unit[0]}")
        print(row)


def print_coverage_table(data: dict, use_color: bool = True) -> None:
    print_header("功能覆盖率（Go vs TypeScript 原版）", use_color)
    header = f"{'维度':<14} {'TS':>4} {'Go':>4} {'覆盖率':>8}"
    print(_color(header, DIM, use_color))
    print(_color("─" * 35, DIM, use_color))
    for cat, info in data["categories"].items():
        pct = info["go_impl"] / info["total"] * 100 if info["total"] > 0 else 0
        if pct == 100:
            pct_str = _color(f"{pct:6.0f}%", GREEN, use_color)
        elif pct == 0:
            pct_str = _color(f"{pct:6.0f}%", RED, use_color)
        else:
            pct_str = _color(f"{pct:6.0f}%", YELLOW, use_color)
        print(f"  {cat:<14} {info['total']:>3}  {info['go_impl']:>3}  {pct_str}")
    print(_color("─" * 35, DIM, use_color))
    overall = data["overall_pct"]
    overall_str = _color(f"{overall:6.1f}%", GREEN if overall >= 80 else YELLOW if overall >= 50 else RED, use_color)
    print(f"  {'总计':<14} {data['total']:>3}  {data['total_impl']:>3}  {overall_str}")

    # 未实现的 P0 特性
    p0_missing = [e for e in FEATURE_MATRIX if e.go_status != "✅" and e.priority == "P0"]
    if p0_missing:
        print()
        print(_color("  ⚠  P0 未实现特性（建议尽快补齐）:", YELLOW, use_color))
        for e in p0_missing:
            print(f"     • [{e.category}] {e.feature}")


def print_recovery_table(data: dict, use_color: bool = True) -> None:
    print_header("错误恢复路径对比", use_color)
    header = f"  {'恢复路径':<34} {'TS':^5} {'Go':^5}"
    print(_color(header, DIM, use_color))
    print(_color("  " + "─" * 46, DIM, use_color))
    for r in data["paths"]:
        ts = _color("  ✅", GREEN, use_color) if r.ts_support else _color("  ❌", RED, use_color)
        go = _color("  ✅", GREEN, use_color) if r.go_support else _color("  ❌", RED, use_color)
        print(f"  {r.path:<34}{ts}   {go}")
    print(_color("  " + "─" * 46, DIM, use_color))
    pct = data["coverage_pct"]
    pct_str = _color(f"{pct:.0f}%", GREEN if pct >= 80 else YELLOW if pct >= 40 else RED, use_color)
    print(f"  Go 恢复路径覆盖率：{pct_str}（{data['go_count']}/{data['ts_count']}）")


def print_parity_table(data: dict, use_color: bool = True) -> None:
    print_header("Parity Fixture 覆盖状态", use_color)
    print(f"  Fixtures: active={data['active_fixture_count']} pending={data['pending_fixture_count']} total={data['fixture_count']}")
    print(f"  Tasks: covered={data['covered_task_count']} pending={data['pending_task_count']} total={data['task_count']}")
    print(f"  Last updated: {data.get('updated_at') or '-'}")
    if data["missing_manifest_tasks"]:
        print(_color("  Missing manifest entries: " + ", ".join(data["missing_manifest_tasks"]), RED, use_color))

    print()
    header = f"  {'Task':<8} {'Status':<9} {'Updated':<12} {'Fixtures'}"
    print(_color(header, DIM, use_color))
    print(_color("  " + "─" * 66, DIM, use_color))
    for task in data["tasks"]:
        status = task["status"]
        color = GREEN if status == "covered" else YELLOW
        fixtures = ", ".join(task["fixtures"]) if task["fixtures"] else "-"
        print(f"  {task['task_id']:<8} {_color(status, color, use_color):<9} {task.get('updated_at') or '-':<12} {fixtures}")


def print_concurrent_table(results: list[ConcurrentResult], use_color: bool = True) -> None:
    print_header("并发工具执行加速比", use_color)
    header = f"  {'工具数':>5} {'串行(ms)':>10} {'并发(ms)':>10} {'加速比':>8} {'效率':>8}"
    print(_color(header, DIM, use_color))
    print(_color("  " + "─" * 46, DIM, use_color))
    for r in results:
        speedup_str = f"{r.speedup:.2f}x"
        eff_pct = r.efficiency * 100
        eff_color = GREEN if eff_pct >= 70 else YELLOW if eff_pct >= 50 else RED
        eff_str = _color(f"{eff_pct:.0f}%", eff_color, use_color)
        print(f"  {r.n_tools:>5} {r.serial_ms:>10.1f} {r.parallel_ms:>10.1f} "
              f"{speedup_str:>8} {eff_str:>8}")


def print_summary(cov: dict, rec: dict, use_color: bool = True) -> None:
    print_header("综合评估摘要", use_color)
    overall = cov["overall_pct"]
    rec_pct = rec["coverage_pct"]

    def grade(pct: float) -> str:
        if pct >= 90: return _color("优秀", GREEN, use_color)
        if pct >= 70: return _color("良好", GREEN, use_color)
        if pct >= 50: return _color("待改善", YELLOW, use_color)
        return _color("需重点投入", RED, use_color)

    print(f"  功能覆盖率：{overall:.1f}%  → {grade(overall)}")
    print(f"  恢复路径覆盖：{rec_pct:.0f}%  → {grade(rec_pct)}")

    p0 = sum(1 for e in FEATURE_MATRIX if e.go_status != "✅" and e.priority == "P0")
    p1 = sum(1 for e in FEATURE_MATRIX if e.go_status != "✅" and e.priority == "P1")
    p2 = sum(1 for e in FEATURE_MATRIX if e.go_status != "✅" and e.priority == "P2")
    print(f"  待实现特性：P0={p0}  P1={p1}  P2={p2}")
    print()
    print("  建议行动项：")
    if p0 > 0:
        print(_color(f"    1. 优先实现 {p0} 项 P0 特性（Token Budget 续传、Reactive Compact）", YELLOW, use_color))
    if p1 > 0:
        print(f"    2. 规划 {p1} 项 P1 特性（StreamingToolExecutor、模型回退）")
    if p2 > 0:
        print(f"    3. 长期补全 {p2} 项 P2 特性（Stop hooks、工具摘要等）")
    print()


# ─────────────────────────────────────────────────────────────────────────────
# 主入口
# ─────────────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(
        description="Query Loop 评估脚本 — 衡量 Go QueryLoop 与 TS 原版的关键指标"
    )
    parser.add_argument(
        "--suite",
        choices=["latency", "coverage", "recovery", "concurrent", "parity", "all"],
        default="all",
        help="指定要运行的测试套件（默认：all）",
    )
    parser.add_argument(
        "--samples",
        type=int,
        default=50,
        help="延迟基准采样次数（默认：50）",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="以 JSON 格式输出结果（适合 CI 集成）",
    )
    parser.add_argument(
        "--no-color",
        action="store_true",
        help="禁用 ANSI 颜色输出",
    )
    args = parser.parse_args()

    use_color = not args.no_color and sys.stdout.isatty()
    suite = args.suite

    if args.json:
        # JSON 模式：静默收集，最终输出 JSON
        output: dict = {"generated_at": time.strftime("%Y-%m-%dT%H:%M:%S")}

        if suite in ("latency", "all"):
            latency_results = run_latency_suite(args.samples)
            output["latency"] = [
                {
                    "name": r.name,
                    "mean_ms": round(r.mean, 3),
                    "median_ms": round(r.median, 3),
                    "p95_ms": round(r.p95, 3),
                    "p99_ms": round(r.p99, 3),
                    "stdev_ms": round(r.stdev, 3),
                }
                for r in latency_results
            ]

        if suite in ("coverage", "all"):
            cov = run_coverage_suite()
            parity = run_parity_suite()
            output["coverage"] = {
                "overall_pct": round(cov["overall_pct"], 1),
                "total": cov["total"],
                "implemented": cov["total_impl"],
                "by_category": {
                    cat: {
                        "total": info["total"],
                        "implemented": info["go_impl"],
                        "pct": round(info["go_impl"] / info["total"] * 100 if info["total"] else 0, 1),
                    }
                    for cat, info in cov["categories"].items()
                },
            }
            output["parity"] = parity

        if suite == "parity":
            output["parity"] = run_parity_suite()

        if suite in ("recovery", "all"):
            rec = run_recovery_suite()
            output["recovery"] = {
                "coverage_pct": round(rec["coverage_pct"], 1),
                "ts_count": rec["ts_count"],
                "go_count": rec["go_count"],
                "paths": [
                    {"name": r.path, "ts": r.ts_support, "go": r.go_support}
                    for r in rec["paths"]
                ],
            }

        if suite in ("concurrent", "all"):
            conc = run_concurrent_suite()
            output["concurrent"] = [
                {
                    "n_tools": r.n_tools,
                    "serial_ms": round(r.serial_ms, 2),
                    "parallel_ms": round(r.parallel_ms, 2),
                    "speedup": round(r.speedup, 2),
                    "efficiency_pct": round(r.efficiency * 100, 1),
                }
                for r in conc
            ]

        print(json.dumps(output, ensure_ascii=False, indent=2))
        return

    # 人类可读模式
    print(_color("\n╔══════════════════════════════════════════════════════════════════════╗", CYAN, use_color))
    print(_color("║        Query Loop 评估报告 — Go vs TypeScript 原版                   ║", CYAN, use_color))
    print(_color("╚══════════════════════════════════════════════════════════════════════╝", CYAN, use_color))

    cov = None
    rec = None

    if suite in ("latency", "all"):
        latency_results = run_latency_suite(args.samples)
        print_latency_table(latency_results, use_color)

    if suite in ("coverage", "all"):
        cov = run_coverage_suite()
        print_coverage_table(cov, use_color)
        print_parity_table(run_parity_suite(), use_color)

    if suite == "parity":
        print_parity_table(run_parity_suite(), use_color)

    if suite in ("recovery", "all"):
        rec = run_recovery_suite()
        print_recovery_table(rec, use_color)

    if suite in ("concurrent", "all"):
        conc = run_concurrent_suite()
        print_concurrent_table(conc, use_color)

    if suite == "all" and cov is not None and rec is not None:
        print_summary(cov, rec, use_color)


if __name__ == "__main__":
    main()
