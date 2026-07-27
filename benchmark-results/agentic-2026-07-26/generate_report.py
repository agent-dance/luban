#!/usr/bin/env python3
"""Build the self-contained benchmark report and machine-readable summaries."""

from __future__ import annotations

import csv
import html
import json
import math
from collections import Counter
from datetime import datetime
from pathlib import Path


ROOT = Path(__file__).resolve().parent
RAW = ROOT / "raw"
AGENTS = ("codex", "luban")
TASKS = [
    ("danielmiessler__Fabric-2098", "Fabric / Codex 空响应", "Go"),
    ("openai__openai-agents-js-375", "OpenAI Agents JS / streaming agent_end", "TypeScript"),
    ("kubernetes__kube-state-metrics-2926", "kube-state-metrics / 旧 YAML 键兼容", "Go"),
    ("skim-rs__skim-1044", "Skim / --border none", "Rust"),
    ("include-what-you-use__include-what-you-use-1991", "IWYU / 显式特化依赖", "C++"),
]


def load(path: Path) -> dict | list:
    return json.loads(path.read_text(encoding="utf-8"))


def esc(value: object) -> str:
    return html.escape(str(value), quote=True)


def n(value: int | None) -> str:
    return "—" if value is None else f"{value:,}"


def sec(value: float) -> str:
    minutes, seconds = divmod(value, 60)
    if minutes >= 1:
        return f"{int(minutes)}m {seconds:04.1f}s"
    return f"{seconds:.1f}s"


def usd(value: float) -> str:
    return f"${value:,.4f}"


def pct(value: float) -> str:
    return f"{value * 100:.1f}%"


def wilson(successes: int, total: int, z: float = 1.959963984540054) -> tuple[float, float]:
    p = successes / total
    denominator = 1 + z * z / total
    center = (p + z * z / (2 * total)) / denominator
    margin = z * math.sqrt((p * (1 - p) + z * z / (4 * total)) / total) / denominator
    return center - margin, center + margin


metadata = {row["instance_id"]: row for row in load(RAW / "metadata" / "selected_instances.json")}
experiment = load(RAW / "metadata" / "experiment.json")

runs: dict[str, dict[str, dict]] = {}
evaluations: dict[str, dict[str, dict]] = {}
gold: dict[str, dict] = {}
for instance_id, _, _ in TASKS:
    runs[instance_id] = {}
    evaluations[instance_id] = {}
    for agent in AGENTS:
        runs[instance_id][agent] = load(RAW / "runs" / instance_id / agent / "summary.json")
        evaluations[instance_id][agent] = load(
            RAW / "evaluation" / instance_id / agent / "report.json"
        )
    gold[instance_id] = load(RAW / "evaluation" / instance_id / "gold" / "report.json")

diagnostics = {
    agent: load(
        RAW
        / "evaluation"
        / "skim-rs__skim-1044"
        / f"{agent}-diagnostic-production-only"
        / "report.json"
    )
    for agent in AGENTS
}


def aggregate(agent: str) -> dict:
    selected_runs = [runs[instance_id][agent] for instance_id, _, _ in TASKS]
    selected_evals = [evaluations[instance_id][agent] for instance_id, _, _ in TASKS]
    tool_types: Counter[str] = Counter()
    for run in selected_runs:
        tool_types.update(run["tool_calls_by_type"])
    total_input = sum(run["usage"]["input_tokens"] for run in selected_runs)
    cached_input = sum(run["usage"]["cached_input_tokens"] for run in selected_runs)
    reasoning_values = [run["usage"].get("reasoning_output_tokens") for run in selected_runs]
    return {
        "resolved": sum(bool(report["resolved"]) for report in selected_evals),
        "total_tasks": len(TASKS),
        "elapsed_seconds": round(sum(run["elapsed_seconds"] for run in selected_runs), 3),
        "mean_seconds": round(sum(run["elapsed_seconds"] for run in selected_runs) / len(TASKS), 3),
        "median_seconds": sorted(run["elapsed_seconds"] for run in selected_runs)[len(TASKS) // 2],
        "estimated_cost_usd": round(sum(run["estimated_cost_usd"] for run in selected_runs), 6),
        "tool_calls": sum(run["tool_calls"] for run in selected_runs),
        "tool_calls_by_type": dict(sorted(tool_types.items())),
        "input_tokens": total_input,
        "cached_input_tokens": cached_input,
        "uncached_input_tokens": total_input - cached_input,
        "cache_ratio": cached_input / total_input,
        "output_tokens": sum(run["usage"]["output_tokens"] for run in selected_runs),
        "reasoning_output_tokens": (
            sum(reasoning_values) if all(value is not None for value in reasoning_values) else None
        ),
        "files_changed_sum": sum(run["patch"]["files_changed"] for run in selected_runs),
        "additions": sum(run["patch"]["additions"] for run in selected_runs),
        "deletions": sum(run["patch"]["deletions"] for run in selected_runs),
        "evaluation_seconds": round(sum(report["elapsed_seconds"] for report in selected_evals), 3),
    }


aggregates = {agent: aggregate(agent) for agent in AGENTS}
strict_ci = {agent: wilson(aggregates[agent]["resolved"], len(TASKS)) for agent in AGENTS}

wxt_runs = {
    agent: load(RAW / "runs" / "wxt-dev__wxt-2267" / agent / "summary.json") for agent in AGENTS
}
wxt_evals = {
    agent: load(RAW / "evaluation" / "wxt-dev__wxt-2267" / agent / "report.json")
    for agent in AGENTS
}
counted_cost = sum(aggregates[agent]["estimated_cost_usd"] for agent in AGENTS)
excluded_agent_cost = sum(wxt_runs[agent]["estimated_cost_usd"] for agent in AGENTS)

environment = {
    "run_date": "2026-07-26",
    "host": "macOS 15.4.1 (24E263), arm64",
    "evaluation_vm": "Lima Debian Linux x86_64, 4 vCPU, 3.8 GiB RAM, no swap, 40 GiB disk",
    "codex_version": "codex-cli 0.144.6",
    "luban_version": "luban-code v0.1.0",
    "luban_commit": "7a8e62b317b632e0e11ed1f32ef660a20369678c",
    "model": experiment["model"],
    "reasoning_effort": experiment["reasoning_effort"],
    "agent_timeout_seconds": experiment["timeout_seconds"],
}

result_bundle = {
    "report_schema": "luban-agent-comparison/v1",
    "generated_at": datetime.now().astimezone().isoformat(timespec="seconds"),
    "environment": environment,
    "experiment": experiment,
    "task_order": [instance_id for instance_id, _, _ in TASKS],
    "runs": runs,
    "evaluations": evaluations,
    "gold": gold,
    "diagnostics_not_in_score": diagnostics,
    "aggregates": aggregates,
    "recorded_spend": {
        "counted_five_tasks_usd": round(counted_cost, 6),
        "excluded_wxt_runs_usd": round(excluded_agent_cost, 6),
        "all_recorded_agent_runs_usd": round(counted_cost + excluded_agent_cost, 6),
        "note": "Estimate from token events and the pinned local model catalog; excludes compute and network costs.",
    },
    "security_incident": {
        "observed": "The Fabric Luban trajectory ran env and captured inherited host credentials in tool output.",
        "impact": "Those values were included in provider requests to the configured model endpoint.",
        "remediation": "12 distinct values were redacted across 18 delivered files; rotate the exposed credentials. The delivered runner now uses an environment allowlist.",
    },
}
(ROOT / "results.json").write_text(
    json.dumps(result_bundle, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
)

csv_fields = [
    "instance_id",
    "language",
    "agent",
    "resolved",
    "elapsed_seconds",
    "evaluation_seconds",
    "estimated_cost_usd",
    "input_tokens",
    "cached_input_tokens",
    "output_tokens",
    "reasoning_output_tokens",
    "tool_calls",
    "tool_calls_by_type",
    "files_changed",
    "additions",
    "deletions",
    "fail_to_pass_passed",
    "fail_to_pass_expected",
    "pass_to_pass_passed",
    "pass_to_pass_expected",
]
with (ROOT / "runs.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=csv_fields)
    writer.writeheader()
    for instance_id, _, language in TASKS:
        for agent in AGENTS:
            run = runs[instance_id][agent]
            report = evaluations[instance_id][agent]
            writer.writerow(
                {
                    "instance_id": instance_id,
                    "language": language,
                    "agent": agent,
                    "resolved": report["resolved"],
                    "elapsed_seconds": run["elapsed_seconds"],
                    "evaluation_seconds": report["elapsed_seconds"],
                    "estimated_cost_usd": run["estimated_cost_usd"],
                    "input_tokens": run["usage"]["input_tokens"],
                    "cached_input_tokens": run["usage"]["cached_input_tokens"],
                    "output_tokens": run["usage"]["output_tokens"],
                    "reasoning_output_tokens": run["usage"].get("reasoning_output_tokens"),
                    "tool_calls": run["tool_calls"],
                    "tool_calls_by_type": json.dumps(run["tool_calls_by_type"], ensure_ascii=False),
                    "files_changed": run["patch"]["files_changed"],
                    "additions": run["patch"]["additions"],
                    "deletions": run["patch"]["deletions"],
                    "fail_to_pass_passed": len(report["FAIL_TO_PASS"]["passed"]),
                    "fail_to_pass_expected": report["FAIL_TO_PASS"]["expected"],
                    "pass_to_pass_passed": report["PASS_TO_PASS"]["passed_count"],
                    "pass_to_pass_expected": report["PASS_TO_PASS"]["expected"],
                }
            )


def raw_links(instance_id: str, agent: str) -> str:
    base = f"raw/runs/{instance_id}/{agent}"
    evaluation = f"raw/evaluation/{instance_id}/{agent}"
    links = [
        ("summary", f"{base}/summary.json"),
        ("events", f"{base}/events.jsonl"),
        ("patch", f"{base}/model.patch"),
        ("score", f"{evaluation}/report.json"),
    ]
    return " · ".join(f'<a href="{esc(path)}">{esc(label)}</a>' for label, path in links)


def status_badge(report: dict) -> str:
    if report["resolved"]:
        return '<span class="badge pass">PASS</span>'
    return '<span class="badge fail">FAIL</span>'


def metric_rows() -> str:
    rows = []
    for instance_id, title, language in TASKS:
        for index, agent in enumerate(AGENTS):
            run = runs[instance_id][agent]
            report = evaluations[instance_id][agent]
            usage = run["usage"]
            f2p = report["FAIL_TO_PASS"]
            p2p = report["PASS_TO_PASS"]
            task_cell = (
                f'<td rowspan="2"><strong>{esc(title)}</strong><br>'
                f'<code>{esc(instance_id)}</code><br><span class="muted">{esc(language)}</span></td>'
                if index == 0
                else ""
            )
            rows.append(
                "<tr>"
                + task_cell
                + f'<td><span class="agent {agent}">{esc(agent.title())}</span></td>'
                + f'<td>{status_badge(report)}<div class="tiny">F2P {len(f2p["passed"])}/{f2p["expected"]} · P2P {p2p["passed_count"]}/{p2p["expected"]}</div></td>'
                + f'<td>{sec(run["elapsed_seconds"])}<div class="tiny">评测 {sec(report["elapsed_seconds"])}</div></td>'
                + f'<td>{usd(run["estimated_cost_usd"])}</td>'
                + f'<td>{n(usage["input_tokens"])}<div class="tiny">缓存 {n(usage["cached_input_tokens"])}</div></td>'
                + f'<td>{n(usage["output_tokens"])}<div class="tiny">reasoning {n(usage.get("reasoning_output_tokens"))}</div></td>'
                + f'<td>{run["tool_calls"]}<div class="tiny">{esc(json.dumps(run["tool_calls_by_type"], ensure_ascii=False))}</div></td>'
                + f'<td>{run["patch"]["files_changed"]} files<div class="tiny">+{run["patch"]["additions"]} / −{run["patch"]["deletions"]}</div></td>'
                + f'<td class="links">{raw_links(instance_id, agent)}</td>'
                + "</tr>"
            )
    return "\n".join(rows)


def aggregate_table() -> str:
    fields = [
        ("官方严格分", lambda value: f'{value["resolved"]}/{value["total_tasks"]} ({pct(value["resolved"] / value["total_tasks"])})'),
        ("任务耗时", lambda value: sec(value["elapsed_seconds"])),
        ("平均 / 中位耗时", lambda value: f'{sec(value["mean_seconds"])} / {sec(value["median_seconds"])}'),
        ("估算模型费用", lambda value: usd(value["estimated_cost_usd"])),
        ("工具调用", lambda value: n(value["tool_calls"])),
        ("输入 token", lambda value: n(value["input_tokens"])),
        ("缓存输入 token", lambda value: f'{n(value["cached_input_tokens"])} ({pct(value["cache_ratio"])})'),
        ("非缓存输入 token", lambda value: n(value["uncached_input_tokens"])),
        ("输出 token", lambda value: n(value["output_tokens"])),
        ("reasoning token", lambda value: n(value["reasoning_output_tokens"])),
        ("补丁规模（求和）", lambda value: f'{value["files_changed_sum"]} files · +{value["additions"]} / −{value["deletions"]}'),
        ("正式评分容器耗时", lambda value: sec(value["evaluation_seconds"])),
    ]
    return "\n".join(
        f'<tr><th>{esc(label)}</th><td>{formatter(aggregates["codex"])}</td><td>{formatter(aggregates["luban"])}</td></tr>'
        for label, formatter in fields
    )


def tool_rows() -> str:
    names = sorted(
        set(aggregates["codex"]["tool_calls_by_type"])
        | set(aggregates["luban"]["tool_calls_by_type"])
    )
    return "\n".join(
        f'<tr><td>{esc(name)}</td><td>{aggregates["codex"]["tool_calls_by_type"].get(name, 0)}</td><td>{aggregates["luban"]["tool_calls_by_type"].get(name, 0)}</td></tr>'
        for name in names
    )


task_story = {
    "danielmiessler__Fabric-2098": "双方均正确处理 Responses API 的增量/最终响应；Luban 略慢但更便宜。",
    "openai__openai-agents-js-375": "双方均通过 streaming agent_end 生命周期回归；Codex 明显更快、调用更少。Codex 的候选补丁与官方测试补丁有可继续的 reject，但评分集合全部通过。",
    "kubernetes__kube-state-metrics-2926": "双方都只恢复旧 YAML tag，没有实现独立 deprecated alias 字段及规范键优先级；官方目标包无法编译，属于共同语义漏判。",
    "skim-rs__skim-1044": "双方核心实现均正确，但各自新增了与官方隐藏测试同名的 opt_border_none，合并后 Rust 重复定义；官方严格分记失败。去除 Agent 自增测试文件的对称诊断复评，两端均通过 1/1 + 587/587。",
    "include-what-you-use__include-what-you-use-1991": "双方均保持 228 个既有测试通过，但唯一目标回归失败；二者声称新增的 tests/bugs/1991 文件是未跟踪文件，未进入最终 git diff。",
}


def task_details() -> str:
    blocks = []
    for instance_id, title, language in TASKS:
        meta = metadata[instance_id]
        g = gold[instance_id]
        blocks.append(
            f'''<article class="task-card">
              <div class="task-title"><span>{esc(language)}</span><h3>{esc(title)}</h3></div>
              <p>{esc(task_story[instance_id])}</p>
              <dl class="facts">
                <div><dt>创建时间</dt><dd>{esc(meta["created_at"])}</dd></div>
                <div><dt>基础提交</dt><dd><code>{esc(meta["base_commit"])}</code></dd></div>
                <div><dt>金标准</dt><dd>{len(g["FAIL_TO_PASS"]["passed"])}/{g["FAIL_TO_PASS"]["expected"]} F2P · {g["PASS_TO_PASS"]["passed_count"]}/{g["PASS_TO_PASS"]["expected"]} P2P · {sec(g["elapsed_seconds"])}</dd></div>
                <div><dt>官方镜像</dt><dd><code>{esc(g["image"])}</code></dd></div>
              </dl>
              <p class="links"><a href="raw/evaluation/{esc(instance_id)}/gold/report.json">gold report</a> · <a href="raw/metadata/selected_instances_with_gold.json">公开 gold/test patch 快照</a></p>
            </article>'''
        )
    return "\n".join(blocks)


codex = aggregates["codex"]
luban = aggregates["luban"]
codex_time_advantage = (luban["elapsed_seconds"] - codex["elapsed_seconds"]) / luban["elapsed_seconds"]
codex_tool_advantage = (luban["tool_calls"] - codex["tool_calls"]) / luban["tool_calls"]
luban_cost_advantage = (codex["estimated_cost_usd"] - luban["estimated_cost_usd"]) / codex["estimated_cost_usd"]

report_html = f'''<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Codex vs Luban：Agentic Coding 五题实测</title>
<style>
:root {{ color-scheme: dark; --bg:#090d14; --panel:#111824; --panel2:#172131; --text:#eaf0f7; --muted:#98a6b8; --line:#2b384b; --blue:#63a7ff; --orange:#ffad66; --green:#4fd69c; --red:#ff6e7c; --gold:#ffd166; }}
* {{ box-sizing:border-box; }}
html {{ scroll-behavior:smooth; }}
body {{ margin:0; background:radial-gradient(circle at 20% 0,#17253d 0,transparent 34rem),var(--bg); color:var(--text); font:15px/1.65 ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; }}
a {{ color:#91c2ff; text-decoration:none; }} a:hover {{ text-decoration:underline; }}
code {{ font:12px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace; overflow-wrap:anywhere; color:#c9d7ea; }}
.wrap {{ max-width:1280px; margin:auto; padding:28px; }}
header {{ padding:56px 0 26px; }}
.eyebrow {{ color:var(--gold); text-transform:uppercase; letter-spacing:.14em; font-weight:700; font-size:12px; }}
h1 {{ margin:.2em 0; font-size:clamp(34px,6vw,70px); line-height:1.05; letter-spacing:-.04em; }}
h2 {{ margin:0 0 16px; font-size:27px; letter-spacing:-.02em; }} h3 {{ margin:.1em 0; }}
.lead {{ max-width:850px; color:#c6d1df; font-size:18px; }}
nav {{ position:sticky; top:0; z-index:5; background:#090d14e8; backdrop-filter:blur(12px); border-block:1px solid var(--line); }}
nav .wrap {{ padding-block:10px; display:flex; gap:18px; overflow:auto; white-space:nowrap; }}
section {{ padding:42px 0; border-bottom:1px solid var(--line); }}
.grid {{ display:grid; grid-template-columns:repeat(12,1fr); gap:16px; }}
.card,.task-card {{ background:linear-gradient(145deg,var(--panel2),var(--panel)); border:1px solid var(--line); border-radius:16px; padding:20px; }}
.metric {{ grid-column:span 3; min-height:150px; }}
.span5 {{ grid-column:span 5; }} .span6 {{ grid-column:span 6; }} .span7 {{ grid-column:span 7; }}
.metric .value {{ font-size:35px; font-weight:760; letter-spacing:-.04em; margin:5px 0; }}
.metric .label,.muted,.tiny {{ color:var(--muted); }} .tiny {{ font-size:11px; line-height:1.45; margin-top:3px; max-width:320px; overflow-wrap:anywhere; }}
.verdict {{ grid-column:span 8; }} .side {{ grid-column:span 4; }}
.agent {{ font-weight:750; }} .agent.codex {{ color:var(--blue); }} .agent.luban {{ color:var(--orange); }}
.badge {{ display:inline-block; border-radius:999px; padding:2px 9px; font-size:11px; font-weight:800; letter-spacing:.06em; }} .pass {{ background:#123c2d; color:var(--green); }} .fail {{ background:#451e27; color:var(--red); }}
.callout {{ border-left:3px solid var(--gold); padding:12px 16px; background:#ffd1660c; border-radius:0 10px 10px 0; }}
.table-wrap {{ overflow:auto; border:1px solid var(--line); border-radius:14px; }}
table {{ border-collapse:collapse; width:100%; min-width:760px; background:#0d131d; }} th,td {{ border-bottom:1px solid var(--line); padding:11px 12px; text-align:left; vertical-align:top; }} thead th {{ position:sticky; top:0; background:#172131; z-index:1; font-size:12px; text-transform:uppercase; letter-spacing:.05em; }} tbody tr:hover {{ background:#ffffff05; }}
.full-table {{ min-width:1500px; }} .links {{ font-size:12px; line-height:1.7; }}
.bar-group {{ display:grid; gap:12px; }} .bar-row {{ display:grid; grid-template-columns:95px 1fr 90px; align-items:center; gap:12px; }} .track {{ height:12px; background:#273347; border-radius:99px; overflow:hidden; }} .fill {{ height:100%; border-radius:99px; }} .fill.codex {{ background:var(--blue); }} .fill.luban {{ background:var(--orange); }}
.tasks {{ display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px; }} .task-title {{ display:flex; align-items:center; gap:10px; }} .task-title>span {{ background:#273347; border-radius:6px; padding:2px 7px; font-size:11px; }}
.facts {{ display:grid; grid-template-columns:1fr 1fr; gap:8px 16px; }} .facts div {{ min-width:0; }} dt {{ color:var(--muted); font-size:11px; text-transform:uppercase; letter-spacing:.08em; }} dd {{ margin:1px 0 0; overflow-wrap:anywhere; }}
.notes li {{ margin:.55em 0; }} details {{ border:1px solid var(--line); border-radius:12px; padding:12px 16px; margin:10px 0; background:#101721; }} summary {{ cursor:pointer; font-weight:700; }}
.foot {{ color:var(--muted); font-size:12px; padding:30px 0 70px; }}
@media(max-width:900px) {{ .metric {{ grid-column:span 6; }} .verdict,.side,.span5,.span6,.span7 {{ grid-column:span 12; }} .tasks {{ grid-template-columns:1fr; }} }}
@media(max-width:560px) {{ .wrap {{ padding-inline:16px; }} .metric {{ grid-column:span 12; }} .facts {{ grid-template-columns:1fr; }} }}
@media print {{ nav {{ display:none; }} body {{ background:white; color:#111; }} .card,.task-card,table {{ background:white; color:#111; }} a {{ color:#0645ad; }} }}
</style>
</head>
<body>
<header class="wrap">
  <div class="eyebrow">Controlled pilot · 2026-07-26 · pass@1</div>
  <h1>Codex vs Luban<br>Agentic Coding 实测</h1>
  <p class="lead">同一 <code>gpt-5.6-sol</code>、同一 <code>xhigh</code>、同一供应端点、同一公开 issue 与基础提交。五道 SWE-bench-Live MultiLang 代表题，完整记录分数、token、估算费用、耗时、工具调用、补丁和官方容器评分。</p>
</header>
<nav><div class="wrap"><a href="#verdict">结论</a><a href="#aggregate">总览</a><a href="#runs">10 次运行</a><a href="#tasks">逐题</a><a href="#method">方法</a><a href="#exceptions">异常与排除</a><a href="#limits">限制</a><a href="#raw">原始数据</a></div></nav>
<main class="wrap">
<section id="verdict">
  <div class="grid">
    <div class="card verdict">
      <div class="eyebrow">Executive verdict</div><h2>质量打平，Codex 明显更省时间与工具轮次；Luban 费用略低</h2>
      <p>官方严格主分：<span class="agent codex">Codex 2/5</span>，<span class="agent luban">Luban 2/5</span>。五题逐题结果完全一致，因此本样本没有显示任何质量领先者。两者在 Skim 的核心生产代码都正确，但都因新增测试与官方隐藏测试同名而被严格判失败；对称的生产代码诊断复评后，两者均相当于 3/5，仍打平。</p>
      <p>效率差异更清晰：Codex 总 Agent 时间少 {pct(codex_time_advantage)}，工具调用少 {pct(codex_tool_advantage)}；Luban 的估算模型费低 {pct(luban_cost_advantage)}。这说明相同模型下，真正拉开差距的是上下文组织、工具接口和行动策略，而非本次可观察到的修复成功率。</p>
      <div class="callout"><strong>不能外推为总体排行榜。</strong> 只有 5 题、每题单次运行；2/5 的 95% Wilson 区间对两端都是 {pct(strict_ci["codex"][0])}–{pct(strict_ci["codex"][1])}。当前结论适合做工程选型信号，不适合宣称统计显著的能力胜负。</div>
    </div>
    <div class="card side"><div class="label muted">官方严格分</div><div class="value" style="font-size:50px;font-weight:800">2–2</div><p>两端均通过 Fabric 与 OpenAI Agents JS；均未通过 kube-state-metrics、Skim（测试名碰撞）和 IWYU。</p></div>
    <div class="card metric"><div class="label">Codex 总耗时</div><div class="value agent codex">{sec(codex["elapsed_seconds"])}</div><div class="muted">均值 {sec(codex["mean_seconds"])} · 中位 {sec(codex["median_seconds"])}</div></div>
    <div class="card metric"><div class="label">Luban 总耗时</div><div class="value agent luban">{sec(luban["elapsed_seconds"])}</div><div class="muted">均值 {sec(luban["mean_seconds"])} · 中位 {sec(luban["median_seconds"])}</div></div>
    <div class="card metric"><div class="label">Codex 估算费用</div><div class="value agent codex">{usd(codex["estimated_cost_usd"])}</div><div class="muted">正式五题；非账单</div></div>
    <div class="card metric"><div class="label">Luban 估算费用</div><div class="value agent luban">{usd(luban["estimated_cost_usd"])}</div><div class="muted">正式五题；非账单</div></div>
  </div>
</section>

<section id="aggregate">
  <h2>聚合对比</h2>
  <div class="grid">
    <div class="card span7">
      <div class="table-wrap"><table><thead><tr><th>指标</th><th class="agent codex">Codex</th><th class="agent luban">Luban</th></tr></thead><tbody>{aggregate_table()}</tbody></table></div>
    </div>
    <div class="card span5">
      <h3>最直观的量级差</h3>
      <div class="bar-group">
        <div class="bar-row"><span>Codex 时间</span><div class="track"><div class="fill codex" style="width:{codex["elapsed_seconds"] / luban["elapsed_seconds"] * 100:.1f}%"></div></div><strong>{codex["elapsed_seconds"] / 60:.1f}m</strong></div>
        <div class="bar-row"><span>Luban 时间</span><div class="track"><div class="fill luban" style="width:100%"></div></div><strong>{luban["elapsed_seconds"] / 60:.1f}m</strong></div>
        <div class="bar-row"><span>Codex 工具</span><div class="track"><div class="fill codex" style="width:{codex["tool_calls"] / luban["tool_calls"] * 100:.1f}%"></div></div><strong>{codex["tool_calls"]}</strong></div>
        <div class="bar-row"><span>Luban 工具</span><div class="track"><div class="fill luban" style="width:100%"></div></div><strong>{luban["tool_calls"]}</strong></div>
        <div class="bar-row"><span>Codex 费用</span><div class="track"><div class="fill codex" style="width:100%"></div></div><strong>{usd(codex["estimated_cost_usd"])}</strong></div>
        <div class="bar-row"><span>Luban 费用</span><div class="track"><div class="fill luban" style="width:{luban["estimated_cost_usd"] / codex["estimated_cost_usd"] * 100:.1f}%"></div></div><strong>{usd(luban["estimated_cost_usd"])}</strong></div>
      </div>
      <p class="muted">工具调用是原生事件数，不代表等价工作量：Codex 的 <code>command_execution</code> 往往可包含复合 shell 操作，Luban 把 Glob/Grep/Read/Edit 分开暴露。</p>
    </div>
  </div>
  <h3 style="margin-top:22px">工具调用拆分</h3>
  <div class="table-wrap"><table><thead><tr><th>原生工具类型</th><th>Codex</th><th>Luban</th></tr></thead><tbody>{tool_rows()}</tbody></table></div>
</section>

<section id="runs">
  <h2>10 次正式 Agent 运行</h2>
  <p class="muted">“任务耗时”是 Agent CLI 从启动到退出的实际经过时间；“评测”是后续官方容器重建/测试时间，不计入任务耗时。reasoning token 对 Luban 为供应事件未提供，而非 0。点击链接可审计原始事件、补丁和测试输出。</p>
  <div class="table-wrap"><table class="full-table"><thead><tr><th>题目</th><th>Agent</th><th>结果</th><th>耗时</th><th>估算费用</th><th>输入</th><th>输出</th><th>工具调用</th><th>补丁</th><th>证据</th></tr></thead><tbody>{metric_rows()}</tbody></table></div>
</section>

<section id="tasks"><h2>逐题透视</h2><div class="tasks">{task_details()}</div></section>

<section id="method">
  <h2>方法与公平控制</h2>
  <div class="grid">
    <div class="card span6"><h3>固定条件</h3><ul class="notes">
      <li>Codex <code>{esc(environment["codex_version"])}</code>；Luban <code>{esc(environment["luban_version"])}</code>，源码提交 <code>{esc(environment["luban_commit"])}</code>。</li>
      <li>两端精确模型 ID 均为 <code>{esc(environment["model"])}</code>，reasoning effort 均为 <code>{esc(environment["reasoning_effort"])}</code>，走同一 OpenAI-compatible endpoint。Luban provider debug 请求逐次记录了两字段；Codex 通过 CLI 显式覆盖并忽略用户配置。</li>
      <li>每题使用相同 issue 文本、相同 base commit、独立浅克隆；remote 被移除，禁止网络、Web、子 Agent、远端历史及提交。</li>
      <li>每次单轮 pass@1，Agent 硬上限 {environment["agent_timeout_seconds"]} 秒；执行顺序交错，降低先后顺序偏差。</li>
    </ul></div>
    <div class="card span6"><h3>题目与评分</h3><ul class="notes">
      <li>来源是 <a href="https://swe-bench-live.github.io/">SWE-bench-Live</a> MultiLang，固定 Hugging Face revision <a href="https://huggingface.co/datasets/SWE-bench-Live/MultiLang/tree/{esc(experiment["dataset_revision"])}"><code>{esc(experiment["dataset_revision"][:12])}…</code></a>，而非滚动 latest。</li>
      <li>正式五题覆盖 5 个项目、4 种语言和 API/生命周期、配置兼容、CLI 选项、静态分析器语义；所有题先用 gold patch 通过完整评分集合。</li>
      <li>评分复现 <a href="https://github.com/microsoft/SWE-bench-Live">官方 SWE-bench-Live</a> / <a href="https://github.com/microsoft/RepoLaunch">RepoLaunch</a> 的 test patch、rebuild/test/print commands 与随题 log parser；resolved 要求全部 F2P 与 P2P 通过。</li>
      <li>没有采用 SWE-bench Pro 作为主集：<a href="https://openai.com/index/separating-signal-from-noise-coding-evaluations/">OpenAI 公开审计</a>报告其中约 30% 题目存在严重问题，pilot 更适合用新鲜且带可执行容器的 Live 快照。</li>
    </ul></div>
  </div>
  <h3 style="margin-top:22px">费用公式</h3>
  <p><code>(未缓存输入 × $5 + 缓存输入 × $0.50 + 输出 × $30) / 1,000,000</code>。单价固定自本仓库模型目录 <a href="../../provider/scripts/catalog_sync.py">catalog_sync.py</a> 与 <a href="../../cost/pricing.go">pricing.go</a>；这是基于 Agent token 事件的可比估算，不是 endpoint 账单。正式五题共 {usd(counted_cost)}；被排除 WXT 两次运行 {usd(excluded_agent_cost)}；所有已记录 Agent 运行合计 {usd(counted_cost + excluded_agent_cost)}。未货币化本机算力、镜像网络与评测 VM。</p>
  <h3>执行环境</h3><p>{esc(environment["host"])}；官方镜像在 {esc(environment["evaluation_vm"])} 上以 rootless nerdctl 运行。Agent 工作区在主机，评分工作区在隔离 Linux 容器。</p>
</section>

<section id="exceptions">
  <h2>异常、排除与替补</h2>
  <p class="callout">这里完整披露筛题过程。尤其 WXT 是在 Agent 运行后才排除，属于 post-hoc 决策，存在选择偏差风险；保留其全部成本与轨迹，便于读者自行做敏感性分析。</p>
  <details open><summary>安全事件：Fabric / Luban 工具输出捕获宿主凭据</summary><p>当时运行器把完整宿主环境传给两端进程；在 Fabric 轨迹中，Luban 明确执行了 <code>env</code>，使 API、Git、包仓库和发布类凭据进入工具结果，并随上下文发送到配置的模型端点。交付前已从全部文本工件识别并全局替换 12 个不同敏感值，共修改 18 个文件；二次扫描四类模式均为 0。<strong>脱敏不能撤销此前的端点传输，建议轮换当时环境中的相关凭据。</strong>交付版运行器已改为环境 allowlist，只保留基础运行变量与模型认证变量。<a href="redact_raw.py">脱敏脚本</a></p></details>
  <details open><summary>Defuddle / TypeScript：gold 性能阈值失败，未运行 Agent</summary><p>目标 1/1 通过，但既有 239 项中性能测试 <code>Performance parse time per fixture...</code> 在 4 vCPU VM 失败（238/239）。<a href="raw/evaluation/kepano__defuddle-243/gold/report.json">gold report</a></p></details>
  <details open><summary>WXT / TypeScript：外网型 P2P 波动，整题排除</summary><p>gold 为 2/2 + 799/799；Codex 2/2 + 799/799，Luban 2/2 + 798/799，唯一失败是远端 <code>url:*</code> 模块下载。两端核心补丁语义相同，故不把网络失败算作 Luban 能力差距。Codex：{sec(wxt_runs["codex"]["elapsed_seconds"])} / {usd(wxt_runs["codex"]["estimated_cost_usd"])} / {wxt_runs["codex"]["tool_calls"]} tools；Luban：{sec(wxt_runs["luban"]["elapsed_seconds"])} / {usd(wxt_runs["luban"]["estimated_cost_usd"])} / {wxt_runs["luban"]["tool_calls"]} tools。<a href="raw/evaluation/wxt-dev__wxt-2267/gold/report.json">gold</a> · <a href="raw/evaluation/wxt-dev__wxt-2267/codex/report.json">Codex</a> · <a href="raw/evaluation/wxt-dev__wxt-2267/luban/report.json">Luban</a></p></details>
  <details><summary>OpenDataLoader PDF / Java：JDK 在虚拟化 x86_64 上崩溃</summary><p>gold rebuild 先后出现 Temurin 21 SIGILL/SIGSEGV，目标测试未执行，排除 Java 题；这也是最终语言覆盖为 4 种而非 5 种的原因。<a href="raw/evaluation/opendataloader-project__opendataloader-pdf-383/gold/report.json">gold report</a></p></details>
  <details><summary>Hugo / Go 与 Biome / Rust：官方镜像拉取超过 1,800 秒</summary><p>两题都在 gold 之前因镜像拉取硬超时淘汰；没有模型调用。最初的宿主 timeout 未结束 VM 内 Biome pull，发现后按精确 PID 终止，并把评测器改为 VM 内 timeout；该适配器修复不改变 Agent 提示或评分。Hugo 由 kube-state-metrics 替补，Biome 由 Skim 替补。</p></details>
  <details><summary>Skim：官方严格失败与生产代码诊断通过</summary><p>双方都独立新增了同名 <code>opt_border_none</code> 测试，与官方 test patch 合并后重复定义，官方严格主分必须记失败。对称排除各自 <code>tests/options.rs</code> 后：<a href="raw/evaluation/skim-rs__skim-1044/codex-diagnostic-production-only/report.json">Codex diagnostic</a> 与 <a href="raw/evaluation/skim-rs__skim-1044/luban-diagnostic-production-only/report.json">Luban diagnostic</a> 均通过全部评分项；这不回写主分。</p></details>
</section>

<section id="limits">
  <h2>如何解读，不应如何解读</h2>
  <ul class="notes">
    <li><strong>样本太小：</strong>5 题、每题 1 次；没有估计随机采样方差，也无法证明质量等价。严格分 40%，95% Wilson 区间宽达 {pct(strict_ci["codex"][0])}–{pct(strict_ci["codex"][1])}。</li>
    <li><strong>公开题污染：</strong>公开 issue/patch 可能进入训练或检索语料；禁网只能阻止运行时查答案，不能证明模型未见过。</li>
    <li><strong>同模型不等于同请求：</strong>两款 Agent 的系统提示、工具 schema、上下文压缩和缓存策略不同，正是本实验测量对象；因此 token 不能解释为“模型思考量”的纯指标。</li>
    <li><strong>工具次数不是等价单位：</strong>Codex 的一条 command 可执行复合操作，Luban 使用更细粒度的 Read/Grep/Edit；总数适合看编排风格，不适合按次给工具定价。</li>
    <li><strong>费用是目录价估算：</strong>Luban 未返回 reasoning token；输出 token 已包含供应端计费事件中的输出总量。没有账单核对，也不含容器计算、网络或人工时间。</li>
    <li><strong>gold 门禁只执行一次：</strong>官方大规模流程建议多次 gold 运行过滤不稳定题；本 pilot 因资源限制只跑一次，并通过替补和异常披露降低风险。</li>
    <li><strong>基础设施限制：</strong>arm64 主机上的 x86_64 Lima VM、4GB 内存与 40GB 磁盘导致 Java JIT 崩溃和大镜像慢下载；这影响可选题分布，但正式五题的双方条件相同。</li>
  </ul>
</section>

<section id="raw">
  <h2>可复核交付</h2>
  <div class="grid">
    <div class="card metric"><div class="label">HTML 报告</div><div class="value" style="font-size:22px"><a href="report.html">report.html</a></div><div class="muted">本文件，可离线打开/打印</div></div>
    <div class="card metric"><div class="label">机器可读结果</div><div class="value" style="font-size:22px"><a href="results.json">results.json</a></div><div class="muted">10 次运行、评分、聚合、诊断</div></div>
    <div class="card metric"><div class="label">平面明细</div><div class="value" style="font-size:22px"><a href="runs.csv">runs.csv</a></div><div class="muted">适合 Excel / pandas</div></div>
    <div class="card metric"><div class="label">完整原始目录</div><div class="value" style="font-size:22px"><a href="raw/">raw/</a></div><div class="muted">事件、stderr、patch、测试与状态</div></div>
  </div>
  <p>复现入口：<a href="run_benchmark.py">run_benchmark.py</a>（Agent/模型锁定/计数）与 <a href="evaluate.py">evaluate.py</a>（官方镜像评分）。固定数据元信息：<a href="raw/metadata/experiment.json">experiment.json</a>、<a href="raw/metadata/selected_instances.json">selected_instances.json</a>、<a href="raw/metadata/selected_instances_with_gold.json">selected_instances_with_gold.json</a>。</p>
</section>
</main>
<footer class="wrap foot">Generated from local raw artifacts. Main score always uses unmodified Agent patch plus official test patch; diagnostic runs are explicitly separated.</footer>
</body></html>
'''

(ROOT / "report.html").write_text(report_html, encoding="utf-8")
print(f"wrote {ROOT / 'report.html'}")
print(f"wrote {ROOT / 'results.json'}")
print(f"wrote {ROOT / 'runs.csv'}")
