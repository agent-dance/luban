# 渐进式上下文压缩逐工具报告

日期：2026-08-11
结论：`Inspect` 进入生产 allowlist；`Run`、`ApplyPatch` 和 `ContextUpdate` 默认关闭；`DROP` 不开放。

## 1. 评测口径

所有正式数字来自本次运行保存的 OpenAI Responses provider request ledger，而不是 Luban 自报估算。固定模型为 `gpt-5.6-sol`、reasoning effort 为 `xhigh`、隐式 default service tier；正式长轨迹使用 60,000 token 实验窗口。价格固定为 input `$5/M`、cached input `$0.5/M`、cache write `$6.25/M`、output `$30/M`，来源为 [OpenAI GPT-5.6 Sol model page](https://developers.openai.com/api/docs/models/gpt-5.6-sol)。

实际费用按下式从每个成功 provider response 的 usage 重算：

```text
uncached = input - cached - cache_write
cost = uncached * $5/M + cached * $0.5/M
     + cache_write * $6.25/M + output * $30/M
```

质量使用同一冻结 evaluator：非空 patch 必须能应用，FAIL_TO_PASS 全部通过，PASS_TO_PASS 不退化。没有可固定的模型 seed，因此 A/B 的总 turns、output、费用和时延是“同 case 的真实观测”，不是精确因果均值。可直接归因的数字是 runtime gate、实际 replacement、continuation reset、缓存恢复和 semantic compact 触发记录。

## 2. 决策总表

| 对象 | 形式化实现 | 真实 provider 评测 | 质量 gate | 实际投影/提案 | 生产决策 |
|---|---:|---:|---:|---:|---|
| `Inspect` REWRITE/INDEX | 完成 | v13 正向 | 两侧 2/2 F2P、455/455 P2P | 6 个结果，节省 18,068 token | 默认开启 |
| `Run` REWRITE | 完成 | v7 无触发 | 2/2 F2P、455/455 P2P | 0 candidate / 0 applied | 关闭 |
| `ApplyPatch` receipt | 完成 | v7 无触发 | 2/2 F2P、455/455 P2P | 0 candidate / 0 applied | 关闭 |
| `ContextUpdate` v3 shadow | 完成 | v15/v16 无净值 | 两侧 1/1 F2P、635/635 P2P | 5 个有效 KEEP / 0 applied | 关闭 |
| `DROP` | fail-closed | 不具备放量前提 | 不适用 | 0 | 关闭 |

## 3. Inspect

### 3.1 冻结设置

- Luban binary SHA-256：`5073a288438967e8fad49bf908d3477d3923f265ba6a828661285957474ee4bd`
- case：`ninja-build__ninja-2749`
- Progressive：`Inspect` only，60k context，真实 token/cost gate
- Control：同一 binary、case、模型和窗口，progressive 关闭
- 工件：[Progressive summary](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/inspect-60k-v13/summary.json)、[Control summary](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/control-60k-v13/summary.json)
- evaluator：[Progressive](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/inspect-60k-v13/evaluation.json)、[Control](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/control-60k-v13/evaluation.json)

### 3.2 实际 A/B

| 指标 | Progressive | Control | 实际观测差值 |
|---|---:|---:|---:|
| Agent turns | 13 | 16 | -3（-18.8%） |
| provider calls（含 compact） | 13 | 22 | -9（-40.9%） |
| semantic compacts | 0 | 6（turn 11–16） | -6 |
| input token | 214,622 | 530,376 | -315,754（-59.5%） |
| cached input token | 119,296 | 196,608 | -77,312 |
| cached / input | 55.58% | 37.07% | +18.51 pp |
| output token | 12,077 | 53,930 | -41,853（-77.6%） |
| 实际费用 | `$0.898588` | `$3.385044` | `-$2.486456`（-73.5%） |
| provider request time | 327.170s | 1,146.401s | -819.231s（-71.5%） |
| wall time | 328.352s | 1,148.156s | -819.804s（-71.4%） |
| patch | 2 files，+41/-2 | 2 files，+48/-5 | 同一文件集合 |
| frozen evaluator | 2/2 F2P，455/455 P2P | 2/2 F2P，455/455 P2P | 质量 gate 等价 |

Control 的绝对 cached token 更高，是因为它发送了更多、更大的请求；这不能解释成缓存更好。归一化的 cached/input 从 37.07% 提升到 55.58%，且 Progressive 的总 input 和账单同时显著下降。

### 3.3 每批 token/cost gate

| turn | 结果 | 动作 | result token 前→后 | 请求 token 前→后 | cache-break cost | 保守直接净收益 |
|---:|---|---|---:|---:|---:|---:|
| 6 | admitted | 1 REWRITE + 3 INDEX | 13,318 → 3,637 | 26,866 → 17,185 | `$0` | `$0.0145425` |
| 7 | admitted | 1 REWRITE | 5,442 → 1,574 | 22,770 → 18,902 | `$0` | `$0.0182300` |
| 11 | rejected | 3 INDEX candidate | 14,479 → 1,293 | 22,643 → 9,457 | `$0.057068` | `-$0.0372890` |
| 12 | rejected | 2 INDEX candidate | 10,749 → 916 | 23,151 → 13,318 | `$0.090598` | `-$0.0758485` |
| 13 | admitted | 1 REWRITE | 5,385 → 866 | 25,686 → 21,167 | `$0` | `$0.0224615` |

实际提交总计：6 个结果、3 REWRITE、3 INDEX，24,145 → 6,077 result token，节省 18,068 token 和 45,506 bytes；不计任何“可能延迟 compact”的投机收益时，gate 合计保守直接净收益为 `$0.055234`。turn 11/12 说明它不会为了 byte/token 表面缩小而破坏仍然昂贵的缓存前缀。

### 3.4 结论

`Inspect` 的确定性 REWRITE/INDEX 在真实 provider 轨迹中同时满足质量、费用、缓存比例、compact 次数和时间的正向门槛，进入默认生产 allowlist。它仍只处理已被完整消费的旧结果，保留最新 1–2 个 source reads 全文，并保留可恢复路径、行范围、分页状态、proof、原文 SHA-256 与大小。

## 4. Run

- binary SHA-256：`e3c53b9378f54bc1b94df5d4f746612ad6caf3573e7cdad67f10d4064b817aba`
- case：`ninja-build__ninja-2749`，`Run` only，60k
- 实际结果：17 Agent turns、7 次 semantic compact、477,541 input、167,936 cached、51,948 output、`$3.190433`、1,294.415s provider time。
- 质量：2/2 F2P、455/455 P2P。
- 策略结果：0 candidate、0 applied。轨迹中的成功 Run 数量和大小没有越过“保留最近 2 个全文 + 2,000 token + cost-positive”门槛。
- 工件：[summary](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/run-only-60k-v7/summary.json)、[evaluation](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/run-only-60k-v7/evaluation.json)

结论：形式化 rewrite 保留在代码中供显式实验，但默认 allowlist 不包含 `Run`。本次独立轨迹与 Control 的 turns/费用差异来自模型采样，因为策略没有提交任何 replacement，不能算成策略收益或退化。

## 5. ApplyPatch

- binary SHA-256：`e3c53b9378f54bc1b94df5d4f746612ad6caf3573e7cdad67f10d4064b817aba`
- case：`ninja-build__ninja-2749`，`ApplyPatch` only，60k
- 实际结果：14 Agent turns、3 次 semantic compact、319,223 input、182,784 cached、29,510 output、`$1.658887`、759.693s provider time。
- 质量：2/2 F2P、455/455 P2P。
- 策略结果：0 candidate、0 applied。成功 receipt 太短且最近一次 mutation 必须保留全文，不足以偿还缓存断点。
- 工件：[summary](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/applypatch-only-60k-v7/summary.json)、[evaluation](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/applypatch-only-60k-v7/evaluation.json)

结论：默认关闭；只有未来观察到大量、冗长且已被后续 mutation 完全取代的 receipt，才值得重新评测。

## 6. ContextUpdate shadow

### 6.1 协议迭代事实

- v1 opaque ID：模型使用 Inspect 内部 request ID 或编造 `call_...`，真实 target 命中为 0，废弃。
- v2 filtered index：runtime 过滤了 ContextUpdate receipt，但模型按完整 batch 计数，位置漂移，废弃。
- v3 complete index：使用紧邻上一批完整 result 的 0-based 位置，加 `target_tool` 交叉校验；`functions.Inspect` 在边界规范化。要求只能与另一个工具动作并行，final answer 前省略。

### 6.2 v3 实际 A/B

- binary SHA-256：`3e47e7ccc3abd01b25a087088956747aa78cb6feb7d49da656ba4c70727f8a91`
- case：`charmbracelet__crush-766`
- Shadow 工件：[summary](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/context-update-shadow-short-v15/summary.json)
- 有效 Control 工件：[summary](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/context-update-control-short-v16/summary.json)
- evaluator：[Shadow](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/context-update-shadow-short-v15/evaluation.json)、[Control](../../benchmark-results/progressive-context-compaction-token-cost-2026-08-11/context-update-control-short-v16/evaluation.json)，两侧均为 1/1 F2P、635/635 P2P。
- 首个 Control 因单个上游请求卡满 900s、无完整 usage/patch 而排除，但原始失败工件保留在 `context-update-control-short-v15`。

| 指标 | v3 Shadow | Control | 实际观测差值 |
|---|---:|---:|---:|
| Agent turns / provider calls | 7 / 7 | 7 / 7 | 0 / 0 |
| ContextUpdate proposals | 5 | 0 | +5 |
| target found / runtime candidate | 5 / 5 | 0 / 0 | 协议命中 100% |
| actions | KEEP × 5 | 无 | 0 个压缩动作 |
| input token | 95,157 | 72,603 | +22,554（+31.1%） |
| cached/input | 75.33% | 57.12% | +18.21 pp |
| output token | 6,823 | 5,344 | +1,479（+27.7%） |
| 实际费用 | `$0.357915` | `$0.336711` | `+$0.021204`（+6.3%） |
| provider request time | 165.395s | 128.244s | +37.151s（+29.0%） |
| wall time | 166.759s | 126.241s | +40.518s（+32.1%） |

v3 已解决之前 turns 增加和 target 无法命中的提示词/协议问题：5 次提案全部与正常工具调用同行，没有专门评估 turn。可是本 case 中所有证据都仍活跃，模型正确选择 KEEP；新增 schema、arguments 和 receipts 没有带来任何可应用压缩，实际费用和时间反而增加。A/B 总差异仍受采样影响，但“0 applied、存在确定性协议开销”足以否决默认开启。

结论：实现和 telemetry 保留，只有显式 `shadow` 才注册该工具；生产默认不注册。确定性 runtime gate 继续独立工作，不依赖 ContextUpdate。

## 7. 最终 allowlist

```text
provider: openai
model:    gpt-5.6-sol*
tools:    Inspect
rollout:  100%（稳定 session hash；仍受 kill switch、预算和熔断约束）
```

`Run`、`ApplyPatch`、ContextUpdate application、自由文本 REWRITE 和 DROP 均不在生产默认路径。
