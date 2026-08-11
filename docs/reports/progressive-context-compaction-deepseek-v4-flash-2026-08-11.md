# DeepSeek V4 Flash 渐进式上下文压缩验证

日期：2026-08-11
结论：最终策略已通过真实同 binary A/B 和冻结 evaluator，进入生产 `deepseek/deepseek-v4-flash*` 成对 allowlist；DeepSeek 专属策略不会作用于 GPT-5.6 Sol。

## 1. 最终评测设置

- 模型/API：`deepseek-v4-flash` / DeepSeek Responses API
- reasoning effort：`high`
- case：`charmbracelet__crush-766`
- 压力窗口：50,000 token；两侧 timeout：600 秒
- A/B binary SHA-256：`2aba8ddf93e7f548407730ccc87eda7b8316bcb669ae6abc8acfec10f9041185`
- 最终 release-candidate binary SHA-256：`cb8a7e9f8f3abc55e8e39888e058f205408c2fa59881af6d1c1d70c83c5e1423`
- 固定价格：input `$0.14/M`、cache read `$0.0028/M`、output `$0.28/M`、cache write `$0/M`
- 价格与能力来源：[DeepSeek pricing](https://api-docs.deepseek.com/quick_start/pricing/)、[Responses API](https://api-docs.deepseek.com/guides/responses_api/)

两侧使用同一二进制、同一任务、同一模型、同一 effort、同一窗口和 evaluator。模型没有可固定 seed，因此这不是统计显著性结论；它是针对已复现 compact storm 的真实 provider 行为回归。Control 跑满 600 秒后超时，以下 Control 总账是已经实际发生的下界，不外推未运行部分。

A/B 后的 release candidate 只增加 provider-specific compact policy 的完整 enabled、kill switch、stable rollout 与成对 provider/model scope 检查；被测的已启用 `deepseek/deepseek-v4-flash*` 路径取值不变。该收紧由 focused 与全仓测试覆盖，没有再花费 provider token 重跑行为等价的 600 秒 Control。

工件：

- [Progressive summary](../../benchmark-results/progressive-context-compaction-deepseek-v4-flash-2026-08-11/progressive-v11/summary.json)
- [Progressive evaluator](../../benchmark-results/progressive-context-compaction-deepseek-v4-flash-2026-08-11/progressive-v11/evaluation.json)
- [Control summary](../../benchmark-results/progressive-context-compaction-deepseek-v4-flash-2026-08-11/control-v11/summary.json)
- [Control evaluator](../../benchmark-results/progressive-context-compaction-deepseek-v4-flash-2026-08-11/control-v11/evaluation.json)

## 2. 最终真实结果

| 指标 | Progressive v11 | Control v11 | 变化 |
|---|---:|---:|---:|
| 运行结果 | 自然完成 | 600s timeout | Progressive 完成 |
| Agent turns | 14 | 29 | -51.72% |
| provider calls | 15 | 49 | -69.39% |
| 成功 semantic compact turns | 1 | 13 | -92.31% |
| 实际 compact requests | 1 | 20 | -95.00% |
| input token | 281,234 | 704,702 | -60.09% |
| cached input token | 201,984 | 229,888 | -12.14% |
| cached/input | 71.82% | 32.62% | +39.20pp |
| output token | 23,386 | 104,638 | -77.65% |
| 实际费用 | `$0.018209` | `$0.096416` | -81.11% |
| provider time | 167.037s | 597.289s | -72.03% |
| 最终 patch | 3 files，+70/-12 | 空 | Progressive 有效 |
| frozen evaluator | F2P 1/1；P2P 635/635 | F2P 0/1；P2P 635/635 | 质量提升 |

Progressive 的 Inspect 候选在最终轨迹中全部被 cost/anomaly gate 拒绝，实际投影工具数为 0。这一点很重要：最终收益不是通过强行减少 DeepSeek 的 cached token 得到的，而是通过正确的 provider token 触发、延迟无经济价值的 semantic compact、一次成功的紧凑摘要和避免 compact storm 得到的。

## 3. 失败轨迹与逐轮修正

早期结果不能进入生产：

1. v1 的 48 个候选、79,162 候选 token 全部为负净值；DeepSeek cache read 仅为普通 input 价格的 2%，破坏缓存通常不划算。
2. v6 在 turn 2 即压缩。本地 tokenizer/schema 估算约 28k，而 provider 实测约 8k，错误触发连续 semantic compact。
3. v8 将阈值推迟后成本和时间显著下降，但首次 mutation 前压缩旧 Inspect 源码读，模型随后只能拿到 `seen` 索引，最终无 patch、质量失败。
4. v9 增加“成功 mutation 被后续决策消费前禁止投影”，冻结质量恢复；但 DeepSeek 把 semantic compact 当成继续工具循环，3 次 compact request 均未安装边界。
5. v10 将 compact 历史扁平为一个明确的不可信 JSON transcript，首次真实 compact 即返回合法 `compact-summary/v2`，冻结质量通过；九段式 GPT 摘要仍过长。
6. v11 使用 DeepSeek 专属 concise handoff 和 4k 输出上限，一次 compact 完成并取得最终 A/B 结果。

这些失败没有被丢弃或平均掉：v8 证明“便宜但质量失败”不可接受，直接产生 `requireConsumedMutation`；v9 证明只延迟 compact 不够，直接产生 flattened compact input；v10 证明格式正确仍不等于经济，直接产生 concise summary。

## 4. 最终策略

DeepSeek 专属生产参数：

```json
{
  "providerModelAllowlist": ["deepseek/deepseek-v4-flash"],
  "imminentCompactProviderAllowlist": ["deepseek"],
  "autoCompactKeepRecent": 1,
  "autoCompactMaxGrowthTokens": 4000,
  "autoCompactMinThresholdPercent": 100,
  "requireConsumedMutation": true,
  "flattenCompactInput": true,
  "conciseCompactSummary": true,
  "compactMaxOutputTokens": 4000
}
```

- 自动 compact 决策以最近 provider 实测 input 为基线，本地单轮增长最多计 4k；provider 实测本身超过阈值时不会被隐藏。
- 触发阈值只允许延后：在小压力窗口至少到有效输入窗口 100%，大窗口仍受原动态窗口规则约束。
- 首次成功 ApplyPatch 被后续 assistant 消费前，旧 Inspect 源码读保持 lossless。
- semantic compact 使用单一不可执行 JSON transcript、concise coding handoff 和 4k 上限。
- 原始历史、持久化 transcript、compact boundary 和恢复路径保持不变。

## 5. GPT 隔离与生产判断

生产配置使用 `provider/model-prefix` 成对 allowlist，只允许：

- `openai/gpt-5.6-sol*`
- `deepseek/deepseek-v4-flash*`

独立 provider/model allowlist 的交叉组合会 fail-closed。所有 DeepSeek compact 参数还必须同时通过 feature enabled、kill switch、stable rollout、成对 scope 和 `imminentCompactProviderAllowlist`，显式关闭 progressive 会完整恢复 legacy compactor。

最终判断：DeepSeek V4 Flash 可以进入生产 100% reviewed scope。这里的“100%”仍受 paired allowlist、session budget、anomaly circuit 和 kill switch 约束；不是对未知 DeepSeek 模型或其他工具动作的通配开启。
