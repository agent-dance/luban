#!/usr/bin/env python3
"""
orchestration_metrics.py
────────────────────────
Go vs TypeScript 会话编排层功能覆盖率评估脚本

用法：
    python3 orchestration_metrics.py [--gosrc <path>] [--tssrc <path>] [--json]

选项：
    --gosrc   Go 源码根目录（默认：脚本所在目录的上上级）
    --tssrc   TS 源码根目录（默认：Go 根目录的上一级 src/）
    --json    以 JSON 格式输出（便于 CI 集成）

输出：
    - Hook 类型覆盖率表
    - Session 功能完整性表
    - Coordinator 功能完整性表
    - 总体覆盖率摘要
    - 快速修复建议
"""

import argparse
import json
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional


# ──────────────────────────────────────────────────────────────────────────────
# 数据模型
# ──────────────────────────────────────────────────────────────────────────────

@dataclass
class HookEntry:
    name: str
    ts_defined: bool
    go_defined: bool
    go_triggered: bool
    trigger_location: str = ""
    notes: str = ""

    @property
    def status(self) -> str:
        if self.go_triggered:
            return "✅ 已触发"
        if self.go_defined:
            return "⚠️  已定义未触发"
        return "❌ 未实现"

    @property
    def status_plain(self) -> str:
        if self.go_triggered:
            return "triggered"
        if self.go_defined:
            return "defined_only"
        return "missing"


@dataclass
class FeatureEntry:
    feature: str
    ts_support: str
    go_support: str
    completeness_pct: int
    notes: str = ""

    @property
    def bar(self) -> str:
        filled = self.completeness_pct // 10
        empty = 10 - filled
        return "█" * filled + "░" * empty


@dataclass
class MetricsReport:
    hooks: list = field(default_factory=list)
    session_features: list = field(default_factory=list)
    coordinator_features: list = field(default_factory=list)

    # computed
    ts_hook_count: int = 0
    go_hook_defined: int = 0
    go_hook_triggered: int = 0
    session_avg_pct: int = 0
    coordinator_avg_pct: int = 0

    quick_wins: list = field(default_factory=list)


# ──────────────────────────────────────────────────────────────────────────────
# 静态数据：TS 原版 hook 类型列表（来自 src/types/hooks.ts）
# ──────────────────────────────────────────────────────────────────────────────

TS_HOOK_TYPES = [
    "PreToolUse",
    "PostToolUse",
    "SubagentStart",
    "SubagentStop",
    "PermissionRequest",
    "FileChanged",
    "WorktreeCreate",
    "WorktreeRemove",
    "UserPromptSubmit",
    "SessionStart",
    "SessionEnd",
    "Stop",
    "Notification",
    "MCPToolStart",
    "MCPToolEnd",
    "PreCompact",
]

# Go 版中作为常量定义的 hook 类型（hooks/hooks.go）
GO_DEFINED_HOOK_TYPES = {
    "PreToolUse",
    "PostToolUse",
    "SessionStart",
    "SessionEnd",
    "UserPromptSubmit",
}

# Go 版实际在代码中调用 runner.Run() 的 hook 类型
# 通过扫描源码确认（loop/concurrent.go）
GO_TRIGGERED_HOOK_TYPES_STATIC = {
    "PreToolUse":  "loop/concurrent.go: runPreToolHooks()",
    "PostToolUse": "loop/concurrent.go: runPostToolHooks()",
}

# Session 功能列表
SESSION_FEATURES_STATIC = [
    FeatureEntry("消息持久化",      "SQLite",               "JSONL 原子写入",       80,  "功能等价，格式不同"),
    FeatureEntry("按 ID 查询会话",  "SQL WHERE",            "文件名映射",           90,  ""),
    FeatureEntry("会话列表",        "SQL SELECT",           "目录扫描",             70,  ""),
    FeatureEntry("会话删除",        "SQL DELETE",           "os.Remove",            90,  ""),
    FeatureEntry("三态 FSM",        "idle/running/action",  "无",                    0,  "重大缺失"),
    FeatureEntry("权限拦截",        "RequiresActionDetails","无",                    0,  "依赖 FSM"),
    FeatureEntry("SDK 事件发射",    "SessionExternalMetadata","无",                  0,  "外部集成需要"),
    FeatureEntry("记忆注入",        "无对应",               "MemoryStore（Go特有）",100, "Go 新增能力"),
]

# Coordinator 功能列表
COORDINATOR_FEATURES_STATIC = [
    FeatureEntry("多智能体协调",    "LLM 对话模式",         "程序化任务调度",       50,  "不同架构"),
    FeatureEntry("四阶段工作流",    "系统提示词内置",       "无",                    0,  "重大缺失"),
    FeatureEntry("任务通知 XML 协议","<task-notification>", "无",                    0,  ""),
    FeatureEntry("会话模式 resume", "matchSessionMode()",   "ResolveSession()",     70,  ""),
    FeatureEntry("DAG 依赖解析",    "无",                   "BlockedBy[]（Go特有）",100, "Go 超越 TS"),
    FeatureEntry("并发任务派发",    "无（LLM串行）",        "goroutine 并发",       100, "Go 超越 TS"),
    FeatureEntry("MessageBus",      "无",                   "缓冲 channel(32)",     100, "Go 超越 TS"),
]


# ──────────────────────────────────────────────────────────────────────────────
# 源码扫描
# ──────────────────────────────────────────────────────────────────────────────

def find_go_files(root: Path) -> list:
    """递归找出所有 .go 文件。"""
    return list(root.rglob("*.go"))


def scan_triggered_hooks(go_files: list) -> dict:
    """
    扫描 Go 源码，找出实际调用 runner.Run / hookRunner.Run 并传入特定 HookType 的位置。
    返回 {HookTypeName: "文件名:行号"} 字典。
    """
    triggered = {}
    pattern = re.compile(r'[Hh]ook(\w+)')

    for path in go_files:
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        lines = text.splitlines()
        for lineno, line in enumerate(lines, 1):
            # 只关注含 .Run( 的行（排除定义行）
            if ".Run(" not in line and "Run(ctx" not in line:
                continue
            for m in pattern.finditer(line):
                hook_name = m.group(1)
                if hook_name in TS_HOOK_TYPES and hook_name not in triggered:
                    triggered[hook_name] = f"{path.name}:{lineno}"
    return triggered


def scan_go_defined_hooks(go_files: list) -> set:
    """扫描 Go 源码中已定义为常量/变量的 HookType 名称。"""
    defined = set()
    pattern = re.compile(r'Hook(\w+)\s*(?:HookType\s*)?=')
    for path in go_files:
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        for m in pattern.finditer(text):
            name = m.group(1)
            if name in TS_HOOK_TYPES:
                defined.add(name)
    return defined


def scan_ts_hook_types(ts_src: Optional[Path]) -> list:
    """
    从 TS 源码中扫描 hookSpecificOutput 的 case 类型名称。
    若 ts_src 不存在，返回静态列表。
    """
    if ts_src is None or not ts_src.exists():
        return TS_HOOK_TYPES

    hooks_ts = ts_src / "types" / "hooks.ts"
    if not hooks_ts.exists():
        return TS_HOOK_TYPES

    try:
        text = hooks_ts.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return TS_HOOK_TYPES

    found = re.findall(r'hookEventName\s*:\s*["\'](\w+)["\']', text)
    if found:
        seen = set()
        result = []
        for name in found:
            if name not in seen:
                seen.add(name)
                result.append(name)
        return result
    return TS_HOOK_TYPES


# ──────────────────────────────────────────────────────────────────────────────
# 报告生成
# ──────────────────────────────────────────────────────────────────────────────

def build_report(go_src: Path, ts_src: Optional[Path]) -> MetricsReport:
    report = MetricsReport()

    go_files = find_go_files(go_src)

    dyn_triggered = scan_triggered_hooks(go_files)
    dyn_defined   = scan_go_defined_hooks(go_files)
    ts_hooks      = scan_ts_hook_types(ts_src)

    triggered_map = {**GO_TRIGGERED_HOOK_TYPES_STATIC, **dyn_triggered}
    defined_set   = GO_DEFINED_HOOK_TYPES | dyn_defined

    for hook_name in ts_hooks:
        is_defined   = hook_name in defined_set
        is_triggered = hook_name in triggered_map
        loc = triggered_map.get(hook_name, "")
        report.hooks.append(HookEntry(
            name=hook_name,
            ts_defined=True,
            go_defined=is_defined,
            go_triggered=is_triggered,
            trigger_location=loc,
        ))

    report.ts_hook_count     = len(ts_hooks)
    report.go_hook_defined   = sum(1 for h in report.hooks if h.go_defined)
    report.go_hook_triggered = sum(1 for h in report.hooks if h.go_triggered)

    report.session_features = SESSION_FEATURES_STATIC
    valid_pcts = [f.completeness_pct for f in report.session_features if f.ts_support != "无对应"]
    report.session_avg_pct = int(sum(valid_pcts) / len(valid_pcts)) if valid_pcts else 0

    report.coordinator_features = COORDINATOR_FEATURES_STATIC
    valid_pcts2 = [f.completeness_pct for f in report.coordinator_features if f.ts_support != "无"]
    report.coordinator_avg_pct = int(sum(valid_pcts2) / len(valid_pcts2)) if valid_pcts2 else 0

    not_triggered_but_defined = [h for h in report.hooks if h.go_defined and not h.go_triggered]
    for h in not_triggered_but_defined:
        report.quick_wins.append(f"触发已定义的 {h.name} hook（低风险，数小时工作量）")
    if any(h.name == "SubagentStart" for h in report.hooks if not h.go_triggered):
        report.quick_wins.append("实现 SubagentStart/Stop hook（Agent 工具子循环触发点）")
    report.quick_wins.append("修复 Coordinator Agent goroutine panic recover（1-2h）")
    report.quick_wins.append("为 Hook 触发覆盖率添加单元测试（防退化）")

    return report


# ──────────────────────────────────────────────────────────────────────────────
# 终端渲染
# ──────────────────────────────────────────────────────────────────────────────

COL_SEP = "  "
DIVIDER = "─"


def _col_widths(*rows):
    max_cols = max(len(r) for r in rows)
    widths = [0] * max_cols
    for row in rows:
        for i, cell in enumerate(row):
            widths[i] = max(widths[i], len(cell))
    return widths


def render_table(headers, rows, title="") -> str:
    all_rows = [headers] + rows
    widths = _col_widths(*all_rows)

    def fmt_row(row):
        parts = [cell.ljust(widths[i]) for i, cell in enumerate(row)]
        return COL_SEP.join(parts).rstrip()

    lines = []
    if title:
        lines.append(f"\n{'━' * 62}")
        lines.append(f"  {title}")
        lines.append(f"{'━' * 62}")

    header_line = fmt_row(headers)
    lines.append(header_line)
    lines.append(DIVIDER * len(header_line))
    for row in rows:
        lines.append(fmt_row(row))
    return "\n".join(lines)


def render_report(report: MetricsReport) -> str:
    out = []

    out.append("\n" + "═" * 62)
    out.append("  🔍  会话编排层覆盖率评估报告  (Go vs TypeScript)")
    out.append("═" * 62)

    # ── Hook 覆盖率 ──────────────────────────────────────────────
    hook_rows = []
    for h in report.hooks:
        hook_rows.append([
            h.name,
            "✅" if h.ts_defined else "—",
            "✅" if h.go_defined else "❌",
            h.status,
            h.trigger_location or "—",
        ])

    out.append(render_table(
        ["Hook 名称", "TS 定义", "Go 定义", "Go 触发状态", "触发位置"],
        hook_rows,
        title="① Hook 类型覆盖率",
    ))

    ts_count      = report.ts_hook_count or 1
    defined_pct   = int(report.go_hook_defined   / ts_count * 100)
    triggered_pct = int(report.go_hook_triggered / ts_count * 100)

    out.append(f"\n  TS 总数:    {report.ts_hook_count} 种")
    out.append(f"  Go 已定义:  {report.go_hook_defined} 种  ({defined_pct}%)")
    out.append(f"  Go 已触发:  {report.go_hook_triggered} 种  ({triggered_pct}%)")

    bar_def  = "█" * (defined_pct   // 5) + "░" * (20 - defined_pct   // 5)
    bar_trig = "█" * (triggered_pct // 5) + "░" * (20 - triggered_pct // 5)
    out.append(f"\n  定义覆盖率  [{bar_def}] {defined_pct}%")
    out.append(f"  触发覆盖率  [{bar_trig}] {triggered_pct}%")

    # ── Session 功能 ─────────────────────────────────────────────
    sess_rows = []
    for f in report.session_features:
        note = f"({f.notes})" if f.notes else ""
        sess_rows.append([f.feature, f.ts_support, f.go_support,
                          f"{f.bar} {f.completeness_pct}%", note])

    out.append(render_table(
        ["功能", "TS 原版", "Go 实现", "完整度", "备注"],
        sess_rows,
        title="② Session 模块功能完整性",
    ))
    out.append(f"\n  Session 平均完整度（排除 Go 特有项）：{report.session_avg_pct}%")

    # ── Coordinator 功能 ─────────────────────────────────────────
    coord_rows = []
    for f in report.coordinator_features:
        note = f"({f.notes})" if f.notes else ""
        coord_rows.append([f.feature, f.ts_support, f.go_support,
                           f"{f.bar} {f.completeness_pct}%", note])

    out.append(render_table(
        ["功能", "TS 原版", "Go 实现", "完整度", "备注"],
        coord_rows,
        title="③ Coordinator 模块功能完整性",
    ))
    out.append(f"\n  Coordinator 平均完整度（排除 Go 特有项）：{report.coordinator_avg_pct}%")

    # ── 总体摘要 ─────────────────────────────────────────────────
    out.append("\n" + "━" * 62)
    out.append("  📊  总体覆盖率摘要")
    out.append("━" * 62)
    summary_rows = [
        ["Hook 定义覆盖率",        f"{defined_pct}%   ({report.go_hook_defined}/{report.ts_hook_count})"],
        ["Hook 触发覆盖率",        f"{triggered_pct}%   ({report.go_hook_triggered}/{report.ts_hook_count})"],
        ["Session 平均完整度",     f"{report.session_avg_pct}%"],
        ["Coordinator 平均完整度", f"{report.coordinator_avg_pct}%"],
    ]
    widths = _col_widths(*summary_rows)
    for row in summary_rows:
        label = row[0].ljust(widths[0])
        out.append(f"  {label}  {row[1]}")

    # ── 快速修复建议 ──────────────────────────────────────────────
    if report.quick_wins:
        out.append("\n" + "━" * 62)
        out.append("  ⚡  快速修复建议（Quick Wins）")
        out.append("━" * 62)
        for i, win in enumerate(report.quick_wins, 1):
            out.append(f"  {i}. {win}")

    out.append("\n" + "═" * 62 + "\n")
    return "\n".join(out)


# ──────────────────────────────────────────────────────────────────────────────
# JSON 输出（CI 集成）
# ──────────────────────────────────────────────────────────────────────────────

def report_to_dict(report: MetricsReport) -> dict:
    ts_count = report.ts_hook_count or 1
    return {
        "hook_coverage": {
            "ts_total": report.ts_hook_count,
            "go_defined": report.go_hook_defined,
            "go_triggered": report.go_hook_triggered,
            "defined_pct": round(report.go_hook_defined   / ts_count * 100, 1),
            "triggered_pct": round(report.go_hook_triggered / ts_count * 100, 1),
            "hooks": [
                {
                    "name": h.name,
                    "go_defined": h.go_defined,
                    "go_triggered": h.go_triggered,
                    "status": h.status_plain,
                    "trigger_location": h.trigger_location,
                }
                for h in report.hooks
            ],
        },
        "session_completeness": {
            "average_pct": report.session_avg_pct,
            "features": [
                {
                    "feature": f.feature,
                    "ts_support": f.ts_support,
                    "go_support": f.go_support,
                    "completeness_pct": f.completeness_pct,
                    "notes": f.notes,
                }
                for f in report.session_features
            ],
        },
        "coordinator_completeness": {
            "average_pct": report.coordinator_avg_pct,
            "features": [
                {
                    "feature": f.feature,
                    "ts_support": f.ts_support,
                    "go_support": f.go_support,
                    "completeness_pct": f.completeness_pct,
                    "notes": f.notes,
                }
                for f in report.coordinator_features
            ],
        },
        "quick_wins": report.quick_wins,
    }


# ──────────────────────────────────────────────────────────────────────────────
# CLI 入口
# ──────────────────────────────────────────────────────────────────────────────

def main():
    script_dir = Path(__file__).resolve().parent

    # 默认路径：脚本在 gosrc/coordinator/scripts/，Go 根在 gosrc/
    default_gosrc = script_dir.parent.parent
    default_tssrc = default_gosrc.parent / "src"

    parser = argparse.ArgumentParser(
        description="Go vs TypeScript 会话编排层功能覆盖率评估",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--gosrc", default=str(default_gosrc),
                        help=f"Go 源码根目录 (default: {default_gosrc})")
    parser.add_argument("--tssrc", default=str(default_tssrc),
                        help=f"TS 源码根目录 (default: {default_tssrc})")
    parser.add_argument("--json", action="store_true",
                        help="以 JSON 格式输出（CI 集成用）")
    args = parser.parse_args()

    go_src = Path(args.gosrc)
    ts_src = Path(args.tssrc) if Path(args.tssrc).exists() else None

    if not go_src.exists():
        print(f"错误：Go 源码目录不存在: {go_src}", file=sys.stderr)
        sys.exit(1)

    report = build_report(go_src, ts_src)

    if args.json:
        print(json.dumps(report_to_dict(report), ensure_ascii=False, indent=2))
    else:
        print(render_report(report))


if __name__ == "__main__":
    main()
