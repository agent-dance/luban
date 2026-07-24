#!/usr/bin/env python3
"""
Prompt Cache 命中率模拟器

基于数学模型模拟三种方案的缓存表现，输出逐轮明细和累计汇总。
可用于：
  - 预估不同对话长度下的缓存收益
  - 对比 Go 3断点 vs TS 完整版 vs 无缓存
  - 调整参数评估不同场景（大系统提示、多工具等）

用法:
  python3 scripts/cache_metrics.py                    # 默认参数
  python3 scripts/cache_metrics.py --turns 50         # 50轮对话
  python3 scripts/cache_metrics.py --system 12000     # 12K系统提示
  python3 scripts/cache_metrics.py --json             # JSON输出
  python3 scripts/cache_metrics.py --csv              # CSV输出
"""

import argparse
import json
import sys


def simulate_no_cache(S: int, T: int, D: int, N: int) -> list[dict]:
    """无缓存方案：每轮重新处理全部 input。"""
    rows = []
    for n in range(1, N + 1):
        total = S + T + n * D
        rows.append({
            "turn": n,
            "total_input": total,
            "cache_read": 0,
            "cache_creation": 0,
            "uncached": total,
            "hit_rate": 0.0,
            "eff_cost": 1.0,
        })
    return rows


def simulate_go_3bp(S: int, T: int, D: int, N: int) -> list[dict]:
    """Go 3断点方案：系统提示/工具/最后消息各一个断点。"""
    rows = []
    for n in range(1, N + 1):
        total = S + T + n * D
        if n == 1:
            creation = total
            read = 0
        else:
            # 前缀 = 上一轮的全部内容 → cache_read
            read = S + T + (n - 1) * D
            # 新增 = 本轮新消息 → cache_creation
            creation = D
        uncached = total - read - creation
        hit_rate = read / total if total > 0 else 0.0
        eff_cost = (uncached * 1.0 + creation * 1.25 + read * 0.1) / total if total > 0 else 0.0
        rows.append({
            "turn": n,
            "total_input": total,
            "cache_read": read,
            "cache_creation": creation,
            "uncached": max(0, uncached),
            "hit_rate": hit_rate,
            "eff_cost": eff_cost,
        })
    return rows


def simulate_ts_full(S: int, T: int, D: int, N: int,
                     mc_reduction: float = 0.15) -> list[dict]:
    """
    TS 完整版方案：基础3断点 + 微压缩减少消息体积。

    mc_reduction: 微压缩对旧消息的压缩比例（默认15%，即旧消息缩小15%）。
    从第5轮开始生效（前几轮消息不够多，微压缩无意义）。
    """
    rows = []
    for n in range(1, N + 1):
        raw_msg_tokens = n * D
        # 微压缩从第5轮开始，压缩旧消息（保留最近5个 tool result）
        if n >= 5:
            old_msg_tokens = (n - 5) * D
            msg_tokens = raw_msg_tokens - int(old_msg_tokens * mc_reduction)
        else:
            msg_tokens = raw_msg_tokens

        total = S + T + msg_tokens

        if n == 1:
            creation = total
            read = 0
        else:
            prev_total = rows[-1]["total_input"]
            read = prev_total  # 上一轮的全部内容
            creation = total - read
            if creation < 0:
                # 微压缩使本轮总量小于上轮缓存 → 全部 cache_read
                read = total
                creation = 0

        uncached = total - read - creation
        hit_rate = read / total if total > 0 else 0.0
        eff_cost = (uncached * 1.0 + creation * 1.25 + read * 0.1) / total if total > 0 else 0.0
        rows.append({
            "turn": n,
            "total_input": total,
            "cache_read": read,
            "cache_creation": max(0, creation),
            "uncached": max(0, uncached),
            "hit_rate": hit_rate,
            "eff_cost": eff_cost,
        })
    return rows


def summarize(rows: list[dict], label: str) -> dict:
    """汇总一组模拟结果。"""
    total_input = sum(r["total_input"] for r in rows)
    total_read = sum(r["cache_read"] for r in rows)
    total_creation = sum(r["cache_creation"] for r in rows)
    total_uncached = sum(r["uncached"] for r in rows)
    hit_rate = total_read / total_input if total_input > 0 else 0.0
    eff_cost = (
        (total_uncached * 1.0 + total_creation * 1.25 + total_read * 0.1) / total_input
        if total_input > 0 else 0.0
    )
    # Sonnet 4 pricing: $3/MTok input
    base_rate = 3.0 / 1_000_000
    dollar_cost = (total_uncached * 1.0 + total_creation * 1.25 + total_read * 0.1) * base_rate

    return {
        "label": label,
        "turns": len(rows),
        "total_input": total_input,
        "cache_read": total_read,
        "cache_creation": total_creation,
        "uncached": total_uncached,
        "hit_rate": hit_rate,
        "eff_cost": eff_cost,
        "savings": 1.0 - eff_cost,
        "dollar_cost": dollar_cost,
    }


def print_table(scenarios: list[tuple[str, list[dict]]]):
    """打印逐轮对比表。"""
    labels = [s[0] for s in scenarios]
    all_rows = [s[1] for s in scenarios]
    N = len(all_rows[0])

    # Header
    print(f"\n{'轮次':>4}", end="")
    for label in labels:
        print(f" │ {label:^36}", end="")
    print()

    print(f"{'':>4}", end="")
    for _ in labels:
        print(f" │ {'input':>8} {'read':>8} {'create':>8} {'命中率':>7}", end="")
    print()

    print("─" * (5 + 39 * len(labels)))

    # Rows (sample turns)
    sample_turns = sorted(set([1, 2, 3, 5, 10, 15, 20, 30, 40, 50, N]) & set(range(1, N + 1)))
    for n in sample_turns:
        print(f"{n:>4}", end="")
        for rows in all_rows:
            r = rows[n - 1]
            hr = f"{r['hit_rate']:.1%}"
            print(f" │ {r['total_input']:>8,} {r['cache_read']:>8,} {r['cache_creation']:>8,} {hr:>7}", end="")
        print()

    # Summary
    print("─" * (5 + 39 * len(labels)))
    print(f"\n{'指标':<24}", end="")
    for label in labels:
        print(f" │ {label:>14}", end="")
    print()
    print("─" * (25 + 17 * len(labels)))

    summaries = [summarize(rows, label) for (label, rows) in scenarios]
    metrics = [
        ("总 input tokens", "total_input", lambda v: f"{v:>14,}"),
        ("cache_read tokens", "cache_read", lambda v: f"{v:>14,}"),
        ("cache_creation tokens", "cache_creation", lambda v: f"{v:>14,}"),
        ("缓存命中率", "hit_rate", lambda v: f"{v:>13.1%}"),
        ("有效费率", "eff_cost", lambda v: f"{v:>13.3f}x"),
        ("节省率", "savings", lambda v: f"{v:>13.1%}"),
        ("费用 (Sonnet input)", "dollar_cost", lambda v: f"${v:>13.3f}"),
    ]
    for name, key, fmt in metrics:
        print(f"{name:<24}", end="")
        for s in summaries:
            print(f" │ {fmt(s[key])}", end="")
        print()


def main():
    parser = argparse.ArgumentParser(
        description="Prompt Cache 命中率模拟器 — 对比无缓存 / Go 3断点 / TS 完整版"
    )
    parser.add_argument("--system", "-s", type=int, default=8000,
                        help="系统提示 token 数 (默认 8000)")
    parser.add_argument("--tools", "-t", type=int, default=15000,
                        help="工具定义 token 数 (默认 15000)")
    parser.add_argument("--delta", "-d", type=int, default=2500,
                        help="每轮新增消息 token 数 (默认 2500)")
    parser.add_argument("--turns", "-n", type=int, default=20,
                        help="模拟对话轮数 (默认 20)")
    parser.add_argument("--mc-reduction", type=float, default=0.15,
                        help="TS微压缩对旧消息的压缩比 (默认 0.15)")
    parser.add_argument("--json", action="store_true",
                        help="输出 JSON 格式")
    parser.add_argument("--csv", action="store_true",
                        help="输出 CSV 格式")
    args = parser.parse_args()

    S, T, D, N = args.system, args.tools, args.delta, args.turns

    no_cache = simulate_no_cache(S, T, D, N)
    go_3bp = simulate_go_3bp(S, T, D, N)
    ts_full = simulate_ts_full(S, T, D, N, args.mc_reduction)

    scenarios = [
        ("无缓存", no_cache),
        ("Go 3断点", go_3bp),
        ("TS 完整版", ts_full),
    ]

    if args.json:
        output = {}
        for label, rows in scenarios:
            s = summarize(rows, label)
            s["per_turn"] = rows
            output[label] = s
        json.dump(output, sys.stdout, ensure_ascii=False, indent=2)
        print()
    elif args.csv:
        print("scenario,turn,total_input,cache_read,cache_creation,uncached,hit_rate,eff_cost")
        for label, rows in scenarios:
            for r in rows:
                print(f"{label},{r['turn']},{r['total_input']},{r['cache_read']},"
                      f"{r['cache_creation']},{r['uncached']},{r['hit_rate']:.4f},{r['eff_cost']:.4f}")
    else:
        print("=" * 70)
        print("  Prompt Cache 命中率模拟报告")
        print("=" * 70)
        print(f"\n参数: 系统提示={S:,} tokens, 工具={T:,} tokens, "
              f"每轮增量={D:,} tokens, 轮数={N}")
        print(f"      TS微压缩比={args.mc_reduction:.0%}, 定价基准=Sonnet 4 ($3/MTok)")
        print_table(scenarios)
        print()


if __name__ == "__main__":
    main()
