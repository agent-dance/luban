# 渐进式上下文压缩全面应用报告

日期：2026-08-11
状态：生产可用；已对真实评测通过的 `openai/gpt-5.6-sol*` 与 `deepseek/deepseek-v4-flash*` 成对 scope 默认 100% 开启，工具策略仅开放 `Inspect`。

## 1. 最终范围

全面应用采用“机制全面可用，策略逐项证明”的定义，不等于把每个工具和危险动作全部打开：

- 默认开启：`Inspect` 的确定性 REWRITE/INDEX；
- 默认关闭：`Run`、`ApplyPatch`、ContextUpdate shadow/application、DROP；
- 未知 provider/model 组合、未知价格、缺 provider usage、估算不完整、媒体、persisted output、失败/取消/超时、缺 typed proof 全部 KEEP；
- 显式 settings 可以覆盖默认并可关闭；环境 kill switch 可以立即阻止新投影；
- 已提交 replacement 保持可恢复和字节稳定，kill switch 不破坏历史重放。

逐工具证据见[逐工具报告](progressive-context-compaction-tools-2026-08-11.md)，设计和不变量见[方案文档](../design/progressive-context-compaction.md)。

## 2. 已落地能力

### Token/cost gate

- 完整估算下一次 provider request，包含 system、tools/server schemas、messages、payload、媒体和 protocol framing；
- 使用 `P_next_est = max(0, P_prev + L_next - L_prev)` 校准 provider 实际输入与本地 delta；
- fresh resume/post-compact 在没有权威 provider usage 时自动 KEEP；
- 用实际模型价格、最少 token 节省、reuse horizon、两个冷请求缓存恢复成本和最小净收益共同 admission；
- 原始请求已越过 semantic threshold 时，若投影不能降回阈值以内则拒绝；
- 不把“可能延迟 semantic compact”计为投机美元收益。

### 缓存复用

- 不逐工具结果立即改写；只在已消费的 mutation boundary 或真实 pressure 下批处理；
- 选择能通过同一成本 gate 的最小原子批次；
- replacement 按 `tool_use_id` 冻结，后续请求不重复改写历史中部；
- 真正提交才切断 provider continuation，cache lineage 保持稳定；
- KEEP、rejection 和 shadow 不改变 continuation/cache prefix；
- session 上限为 24 个 projected tools / 48,000 projected token，避免频繁 top-up。

### 恢复与安全

- 原始 tool result 永久保存在会话历史；provider view 只应用私有 `ContentReplacementBlock`；
- resume、fork、subagent 和重启从 replacement records 重建相同 view 与预算；
- semantic compact boundary、cleanup/安装失败、post-compact usage、provider/continuation 失败均保持回滚或稳定重放语义；
- 多工具并行、媒体、persisted output、Inspect pagination/cursor、重复 mutation/verification 和失败 mutation fail-closed；
- 投影后估算不完整或 token 未下降会原子回滚；连续 3 次 anomaly 后当前 session 熔断；
- 动态 context window 同时驱动 pressure 和 semantic threshold，不保留静态旧阈值。

### 控制面与观测

- settings：enabled、shadow、killSwitch、rolloutPercent、provider/model/tool allowlists、token/cost 参数、session budgets、anomaly threshold；
- 稳定 FNV session rollout，resume/restart/fork 分配不漂移；
- `LUBAN_PROGRESSIVE_CONTEXT_COMPACTION_KILL_SWITCH=1` 动态 kill switch；
- content-free JSON telemetry 记录 candidate/applied、REWRITE/INDEX、token/byte、gate 前后请求、cache-break cost、净收益和 ContextUpdate shadow 聚合；
- benchmark worker 从真实 Responses SSE 提取逐请求 usage、served model、provider time，并将 rejected candidate 与 applied savings 分开。

## 3. 生产默认

没有显式 `progressiveContext` 配置时，应用使用以下内建策略：

```json
{
  "enabled": true,
  "shadow": false,
  "rolloutPercent": 100,
  "providerAllowlist": ["openai", "deepseek"],
  "modelAllowlist": ["gpt-5.6-sol", "deepseek-v4-flash"],
  "providerModelAllowlist": [
    "openai/gpt-5.6-sol",
    "deepseek/deepseek-v4-flash"
  ],
  "toolAllowlist": ["Inspect"],
  "imminentCompactProviderAllowlist": ["deepseek"],
  "benefitTrigger": true,
  "benefitTriggerProviderAllowlist": ["openai"],
  "benefitMinTokenSavings": 6000,
  "autoCompactKeepRecent": 1,
  "autoCompactMaxGrowthTokens": 4000,
  "autoCompactMinThresholdPercent": 100,
  "requireConsumedMutation": true,
  "flattenCompactInput": true,
  "conciseCompactSummary": true,
  "compactMaxOutputTokens": 4000,
  "minTokenSavings": 2000,
  "reuseHorizon": 3,
  "cacheRecoveryRequests": 2,
  "minNetSavingsUsd": 0,
  "maxProjectedTools": 24,
  "maxProjectedTokens": 48000,
  "maxConsecutiveAnomalies": 3
}
```

用户显式配置 `{"progressiveContext":{"enabled":false}}` 会完整保留关闭意图，不会被 production default 覆盖。

2026-08-13 起，OpenAI 的 Inspect 路径不再一律等待 mutation/pressure：已消费结果的确定性丰富转写累计达到 6K token 且不会破坏上一请求已命中的缓存前缀时，每 session 最多提前提交一次。DeepSeek 路径不变。完整阈值扫描和三次真实重复见 [6K 提前收益触发报告](progressive-context-compaction-benefit-trigger-2026-08-13.md)。

`providerModelAllowlist` 是最终 admission 条件，不会把两个独立 allowlist 展开成四个交叉组合。DeepSeek 专属 compact 参数也只有在 enabled、kill switch、stable rollout、成对 scope 与 `imminentCompactProviderAllowlist` 全部通过时生效；GPT 继续使用既有 structured history、九段式摘要、20k compact 上限与原阈值估算。

## 4. 全面应用后的真实结果

正式证据是 `ninja-build__ninja-2749`、`gpt-5.6-sol/xhigh`、60k 窗口、同一 binary SHA 的 Inspect v13 A/B：

| 指标 | Progressive | Control | 观测改善 |
|---|---:|---:|---:|
| 质量 | 2/2 F2P；455/455 P2P | 2/2 F2P；455/455 P2P | 等价 |
| Agent turns | 13 | 16 | -18.8% |
| semantic compact | 0 | 6 | -100% |
| input token | 214,622 | 530,376 | -59.5% |
| cached/input | 55.58% | 37.07% | +18.51 pp |
| output token | 12,077 | 53,930 | -77.6% |
| 实际费用 | `$0.898588` | `$3.385044` | -73.5% |
| provider time | 327.170s | 1,146.401s | -71.5% |
| wall time | 328.352s | 1,148.156s | -71.4% |

方案实际提交 6 个旧 Inspect 结果，result token 从 24,145 降至 6,077，节省 18,068 token；在不计避免 compact 的收益时，三个 admitted batch 的保守直接净收益合计 `$0.055234`。另有两个候选批次因为缓存恢复成本为负收益而被拒绝。

由于模型没有固定 seed，总账差额不能全部归因于投影；两边在投影前已出现采样分歧。但这组数据仍是最新实际 provider 账本，并且质量 gate、真实 replacement、无 compact、缓存恢复与 gate rejection 都可从工件独立复核。生产收益不依赖 ContextUpdate：它的 v3 shadow 虽修复 target/turn 行为，在有效 A/B 中仍为 5 KEEP、0 applied、费用 +6.3%，所以保持关闭。

### DeepSeek V4 Flash

`charmbracelet__crush-766`、`deepseek-v4-flash/high`、50k 压力窗口使用同一 binary SHA 的最终 A/B；Control 跑满 600 秒后超时，因此其总账是已实际发生的保守下界：

| 指标 | Progressive v11 | Control v11 | 观测改善 |
|---|---:|---:|---:|
| 质量 | F2P 1/1；P2P 635/635 | F2P 0/1；P2P 635/635 | Progressive 通过 |
| Agent turns | 14 | 29 | -51.72% |
| provider calls | 15 | 49 | -69.39% |
| semantic compact turns | 1 | 13 | -92.31% |
| input token | 281,234 | 704,702 | -60.09% |
| cached/input | 71.82% | 32.62% | +39.20 pp |
| output token | 23,386 | 104,638 | -77.65% |
| 实际费用 | `$0.018209` | `$0.096416` | -81.11% |
| provider time | 167.037s | 597.289s | -72.03% |

最终轨迹的 3 个 Inspect 候选均被 cost/anomaly gate 拒绝，实际投影为 0；这里没有把候选 token 当作收益。正向结果来自 provider usage 校准、推迟无经济价值的 compact、首次 mutation 消费保护、不可执行的扁平 compact transcript，以及一次紧凑的 4k 上限 handoff，最终避免了 Control 的 compact storm。完整失败轨迹和工件见 [DeepSeek V4 Flash 报告](progressive-context-compaction-deepseek-v4-flash-2026-08-11.md)。

### GPT-5.6 Sol 隔离回归

DeepSeek 策略合入后的独立真实回归仍通过 F2P 2/2、P2P 455/455；相对前一条已通过的 GPT progressive 轨迹，费用下降 19.14%、provider time 下降 1.58%、cache ratio 提升 25.06 pp，且 3 个 Inspect 投影实际节省 11,658 token。输入和 turns 因模型采样分别上升 18.92% 与 2 turns，但没有 semantic compact，也没有质量、费用或时延回退。工件见 [GPT regression summary](../../benchmark-results/progressive-context-compaction-deepseek-v4-flash-2026-08-11/gpt-regression-v12/summary.json) 与 [evaluator](../../benchmark-results/progressive-context-compaction-deepseek-v4-flash-2026-08-11/gpt-regression-v12/evaluation.json)。

## 5. 验证与发布门

实现完成时的验证门：

- `go test ./i18n`
- focused：`internal/runtime/compact`、`internal/runtime/loop`、`internal/tools/contextupdate`、`internal/app`、`internal/agent`、`internal/runtime/engine`、`registry`、terminal renderer
- `python3 -m unittest benchmark.agentic.localbench.worker.test_run_worker`
- `go test ./...`
- `git diff --check`

最终状态：上述 focused tests 与 `go test ./...` 全部通过；Python worker tests 通过；`git diff --check` 通过。`go test ./...` 的输出同时确认 `i18n`、`internal/runtime/compact`、`internal/runtime/loop`、`internal/runtime/engine`、`internal/agent`、`internal/app`、`internal/tools/contextupdate`、`internal/ui/terminal` 和 `registry` 通过。

## 6. 运行期回退标准

出现下列任一信号，立即启用 kill switch，并用原始历史继续：

- evaluator/线上回归显示任务质量下降；
- provider failures 或 anomaly circuit 显著增加；
- 相同工作负载下 semantic compact、实际 input/cost 或 provider time 系统性上升；
- 两个请求后缓存比例没有恢复，或实际缓存断点高于 gate 估计；
- replacement recovery、resume/fork 或动态 context window 出现不一致。

当前默认 scope 已是最小正值集合；关闭开关只阻止新投影，不需要迁移或删除任何原始会话数据。

## 7. 模型报告

[DeepSeek V4 Flash 真实 A/B 与优化报告](progressive-context-compaction-deepseek-v4-flash-2026-08-11.md)记录从负收益候选、错误早压缩、质量失败、compact 格式失败到最终正向策略的完整演进。生产只开放明确评测过的 `deepseek/deepseek-v4-flash*`，不会扩展到其他 DeepSeek family。
