# 渐进式上下文压缩

状态：生产可用；`openai/gpt-5.6-sol*` 与 `deepseek/deepseek-v4-flash*` 经真实评测后按 provider/model 成对默认 100% 开启，工具策略仅开放 `Inspect`
更新：2026-08-13

## 1. 目标与非目标

工具结果在第一次被模型消费之后仍会反复进入后续请求。大结果会增加输入 token、费用和时延，并更早触发损失更大的整段语义 compact。本方案在不改写原始会话历史的前提下，为 provider 构造一个低损、可恢复、缓存友好的视图。

每个已消费结果只有四种语义动作：

- `KEEP`：后续仍需要全文或无法证明安全，保留原文；
- `REWRITE`：保留可验证事实和有界原文片段；
- `INDEX`：价值不确定，只保留可恢复位置、范围、分页状态和 proof；
- `DROP`：只有 runtime 能证明结果已被更强的确定性 receipt 完全取代时才可能采用。

当前不追求为“Agent turns 变化”和单次冻结评测建立过度严谨的因果模型。质量 gate 只要求基本反映事实：相同冻结 evaluator、非空有效 patch、无新增 provider 失败。token、缓存、费用和时延则必须来自实际请求账本，并区分候选收益与真正应用收益。

## 2. 核心不变量

1. 结果至少被一次后续 assistant 决策完整消费后才有资格投影；当前尚未消费的结果永远保留全文。
2. 持久化会话保留原始结果；投影只改变 provider-bound view。
3. `tool_use_id` 的 replacement 一旦提交便字节稳定，不在后续轮次重复改写缓存前缀。
4. 未知工具、非法结构、媒体、persisted output、失败/取消/超时、缺 proof 和 mutation 失败全部 `KEEP`。
5. OpenAI 的普通收益触发只选最新的安全后缀并使用丰富 `REWRITE`；真实 context pressure 下保留最近 1 个已消费结果全文并可把更早结果降为可恢复 `INDEX`。最近 2 个 `Run`、1 个 `ApplyPatch` 始终保留全文。
6. replacement 保留原文 SHA-256、原始大小、typed `agentic-proof/v2` 以及该工具恢复所需的事实。
7. 只有 token/cost gate 判断净值为正的原子批次才能提交；byte 只作为观测指标。
8. 普通收益触发不得改写上一请求已命中的缓存后缀，并且每个 session 最多触发一次；压力兜底不受这个次数限制。
9. 投影提交后使 Responses continuation 失效，但保持稳定的 cache lineage；同一冻结 replacement 后续不再变化。
10. semantic compact 仍是压力兜底。渐进投影异常会回滚，连续 3 次异常后本 session 自动熔断。
11. resume、fork 和进程重启从私有 replacement records 重建完全相同的 provider view 与 session 预算。

## 3. 数据与恢复模型

```text
不可变会话历史（原始 tool result）
             |
             +--> 私有 ContentReplacementBlock 账本
             |             tool_use_id -> frozen replacement
             |
             +--> provider view
                         |
                         +--> 已冻结 REWRITE / INDEX / receipt
                         +--> 最近工作集全文
                         +--> semantic compact boundary（最终兜底）
```

`ContentReplacementBlock` 在 estimator、budgeter、hook、provider 和 UI 之前被剥离。resume 时 `ApplyToolResultBudget` 重放 replacement；原始内容仍可审计和恢复。投影事务若出现重估不完整、投影后 token 未下降或安装失败，会恢复 replacement state、continuation fence 和 compaction tracker。

## 4. Token/cost gate

### 4.1 完整请求估算

gate 使用与下一次 provider 请求相同的 `provider.Params`：system prompt、可见 tool/server schemas、messages、tool payload、媒体预算和 protocol framing。估算不完整或模型价格未知时失败关闭。

provider continuation 可能使通用 message estimator 存在稳定的绝对偏差。runtime 保存上一次请求的完整本地估算 `L_prev` 和 provider 实际输入 `P_prev`，下一次使用：

```text
P_next_est = max(0, P_prev + L_next - L_prev)
```

这保留了 provider 的权威基线，同时用本地 delta 反映新消息、tool result replacement 和 schema 变化。没有可比较的完整基线时，回退到本地值与 provider 值中的较大者。该校准同时供 progressive pressure、auto-compact 和投影后 anomaly check 使用，避免两套水位判断漂移。

成本 gate 还必须存在上一请求的 provider usage。fresh resume 虽然能恢复完整本地历史，却不知道 provider 端 cache 是否仍可复用；把未知状态当成 `cache_read=0` 会乐观放行并破坏可能存在的远端缓存。因此首个 resumed request 自动 `KEEP`，等拿到权威 usage baseline 后再评估。

### 4.2 Admission

默认参数：

```text
min_token_savings        = 2,000
benefit_min_token_savings = 6,000（OpenAI 生产收益触发）
reuse_horizon            = 3 requests
cache_recovery_requests  = 2
min_net_savings_usd      = 0
max_projected_tools      = 24 / session
max_projected_tokens     = 48,000 / session
max_consecutive_anomaly  = 3
protected_working_set    = Inspect 2（pressure 时 1）, Run 2, ApplyPatch 1
```

普通收益触发先定位候选中最靠后的第一处改写，并估算该位置之前的稳定请求前缀。若上一请求的 `cache_read` 已越过改写位置，则说明提交会让已命中的缓存后缀失效，直接拒绝：

```text
stable_cached            = min(previous_cache_read, stable_prefix)
invalidated_cached       = previous_cache_read - stable_cached
gross_cache_break_cost   = invalidated_cached
                         * (input_price - cached_input_price)
                         * cache_recovery_requests
admit_benefit            = invalidated_cached == 0
                         && saved_tokens >= 6,000
                         && current_savings + future_savings >= min_net_savings
```

因此普通收益触发不是用估计美元收益交换一个已知缓存断点；只有改写位置尚未进入缓存命中前缀时才可能提交。收益门槛使用 Luban 现有模型价格和 provider usage 校准，不依赖 byte。

若原始请求已经超过 semantic compact threshold，压力批次必须把预计请求降回阈值以内；否则拒绝，不能先破坏缓存再立刻做整段 compact。压力路径沿用保守收益：

```text
cache_break_cost = 2 * max(projected_cold_input_cost - current_mixed_cache_cost, 0)
direct_net       = current_request_savings
                 + reuse_horizon * saved_tokens * cached_input_price
                 - cache_break_cost
```

仅推迟 semantic compact 不计美元收益，因为任务可能在数轮后再次跨过阈值。无论是否降回阈值，replacement 自身在 `reuse_horizon` 内的直接 token 收益都必须偿还两个冷请求的缓存恢复成本并达到 `min_net_savings_usd`。

普通收益选择器从最新候选向前扩展，寻找通过 gate 的最小安全后缀，使第一处变化尽量靠后；它只使用确定性 `REWRITE`。压力选择器仍先寻找最小确定性 `REWRITE` 前缀；如果完整可用前缀仍不足以避免 compact，才从最老结果开始逐个降级到 `INDEX`。

## 5. 缓存策略

每次工具调用都做价值评估，但不意味着每次都提交压缩。逐结果提交会反复改变历史中部、清空 continuation，并让短结果的 token 节省无法偿还缓存断点。生产路径采用：

1. 结果完整消费一次；
2. OpenAI 一旦安全候选累计节省达到 6,000 token，立即尝试最新安全后缀，不等待 mutation 或压力水位；
3. 若改写位置会破坏任何已命中 cached suffix，则继续等待，不提交；
4. 普通收益路径每 session 最多一次 continuation reset，replacement 冻结后不再 top-up；
5. mutation boundary 与接近 compact 的压力路径继续作为兜底；
6. session 工具/token 预算阻止压力路径无限累积。

压力 gate 在 semantic threshold 前 8,000 token 打开，提供约两个普通 coding turns 的处理余量。只有真正提交时才推进 continuation epoch；KEEP/shadow 不改 provider lineage。

## 6. 逐工具策略

### Inspect

`REWRITE` 使用 `progressive-inspect-rewrite/v1`，保留：

- 每个 request 的 ID、kind、path、截断原因；
- search 每个路径的命中数、首末行；
- evidence 的仓库路径、行范围和有界精确头尾片段；
- cursor、`has_more_view`、`source_truncated`；
- 原文 SHA-256/bytes 与完整 `agentic-proof/v2`。

单结果最多 6 KiB。若丰富转写仍无法避免 semantic compact，可用 `progressive-inspect-index/v1`：保留所有路径、chunk 行范围、search cardinality、分页状态和 proof，不保留源码正文，模型可按精确索引重新读取。

### Run

`progressive-run-rewrite/v1` 仅接受所有 step 都真正 invoked、status succeeded、exit code 0、logical execution committed 且 verification 状态安全的结果。保留 SHA-256/bytes、proof 和最多 768 bytes 的精确 head/tail。任何失败、超时、取消、未执行 step、schedule reason 或 revision 异常保持全文。

### ApplyPatch

仅接受 typed proof 表明 CAS `committed`、无 failure reason 的成功 mutation。旧的冗长 receipt 可替换为完整 `agentic-proof/v2`；最近一次 patch receipt 仍保留全文。失败、冲突和未知提交状态保持全文。

Run 与 ApplyPatch 的策略已经有形式化与集成覆盖，但默认 allowlist 只有 `Inspect`。真实轨迹显示它们通常太短；只有逐工具实际评测证明应用频率和净收益后才会放量。

## 7. ContextUpdate shadow

Agentic V2 catalog 可稳定增加第四个 `ContextUpdate` schema。v1 使用 opaque `tool_use_id`，真实模型既会误用 Inspect 内部 request ID，也会生成看似合理但不存在的 `call_...`，因此已废弃；v2 的 filtered index 与模型观察到的完整 batch 顺序不一致。v3 不要求复制 provider 生成的标识，并直接使用完整 batch 位置：

```json
{
  "target_index": 0,
  "target_tool": "Inspect",
  "action": "KEEP | REWRITE | INDEX | DROP",
  "reason_code": "stable_protocol_code",
  "confidence": 0.0
}
```

`target_index` 是紧邻上一批完整 tool-result 中的 0-based 位置，指向 ContextUpdate 自身的位置无效；`target_tool` 是交叉校验，OpenAI 常见的 `functions.Inspect` 在协议边界规范化为 `Inspect`。模型只做价值分类，runtime 自己复算确定性 REWRITE/INDEX，避免未采用的自由文本 rewrite 反而进入后续上下文。ContextUpdate 只能与另一个工具动作并行；下一步若是 final answer 则省略，禁止为评估单独增加 turn。

模型建议是不可信输入。shadow 阶段只验证 target 是否存在、工具/结果状态是否属于 runtime 候选，并输出不含原始内容的聚合 telemetry；不会修改历史。ContextUpdate-only batch 不能回指更早结果，自身 receipt 也永远不是 target。catalog 在整个 session 内保持稳定，不能按轮次动态增删 schema。

正式应用顺序为 `KEEP` → runtime 自己可复算的 `REWRITE` → 可恢复 `INDEX`。自由文本 rewrite 和 `DROP` 不因模型高置信度直接获准；`DROP` 只有 runtime 已持有更强确定性 receipt 时才可能单独评审。

## 8. 配置与控制面

`settings.json` / `settings.local.json`：

```json
{
  "progressiveContext": {
    "enabled": true,
    "shadow": false,
    "killSwitch": false,
    "rolloutPercent": 5,
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
}
```

rollout assignment 使用稳定 session FNV hash，resume/fork/restart 不漂移。provider 精确匹配，model 采用显式前缀匹配以覆盖已审阅 family 的 dated revision；生产策略再用 `providerModelAllowlist` 对已审阅组合成对约束，避免独立 allowlist 形成未验证的交叉组合。动态环境变量 `LUBAN_PROGRESSIVE_CONTEXT_COMPACTION_KILL_SWITCH=1` 可在不重启配置系统的情况下立即阻止新投影；已冻结 replacement 仍可确定性重放。

新工具/模型/provider 的阶段建议：shadow → 1% → 5% → 25% → 100%。`openai/gpt-5.6-sol*` 的 Inspect 投影和 6K 提前收益触发，以及 `deepseek/deepseek-v4-flash*` 的 token 校准、质量 guard 与 semantic compact 策略均已通过冻结质量 gate 和真实 A/B，进入生产默认 100%；显式设置仍可关闭。提前收益触发只在 OpenAI provider scope 生效，不改变 DeepSeek 的已验证路径。DeepSeek 的专属 compact 参数只有在 enabled、kill switch、stable rollout、成对 scope 和 provider allowlist 全部通过时才生效，关闭 progressive 会完整恢复 legacy compactor。任一 scope 出现质量失败率上升、provider failures、新增 semantic compact、实际费用/输入系统性回归或 anomaly 熔断异常，立即 kill switch 并保留原始历史恢复。

## 9. 验证矩阵

自动化覆盖必须包括：

- session resume、replacement/budget 重建和崩溃恢复；
- semantic compact boundary 安装、失败回滚和 post-compact 恢复；
- provider/continuation 失败、projection continuation reset 与 cache lineage；
- 多工具并行、subagent/fork、主 Agent scope；
- persisted output、媒体、分页/cursor；
- 重复 mutation/verification、失败 mutation、失败 Inspect；
- 动态 context window、价格未知、估算不完整、session budget；
- anomaly circuit、feature flag、allowlist、stable rollout 和 kill switch；
- ContextUpdate 非法 schema、缺失 target、失败/媒体 target 的 fail-closed 行为。

每次代码变更至少运行 focused packages、`go test ./i18n` 和最终 `go test ./...`。

## 10. 真实评测口径

每个工具独立报告：

- exact Luban binary SHA、case、模型、reasoning effort、service tier、context budget；
- frozen evaluator 通过数、patch 文件/行数、provider failures；
- agent turns、首次/总 semantic compactions；
- 实际 request input、cached input、cache write、output、cache ratio；
- provider calls、provider time、wall time；
- 候选批次数/候选 token 节省；
- 真正 `applied=true` 的工具数、REWRITE/INDEX 数、token/byte 节省；
- 每批 gate 前后 token、cache-break cost、avoided-compact 下界和估计净收益；
- 按 Luban 现有价格表重算的实际 USD。

本轮直接固定 Luban benchmark/cost catalog 已有的 `gpt-5.6-sol` 价格：input `$5/M`、cached input `$0.5/M`、output `$30/M`，cache write `$6.25/M`。

实际总费用：

```text
uncached = input - cached - cache_write
cost = uncached * $5/M
     + cached * $0.5/M
     + cache_write * $6.25/M
     + output * $30/M
```

所有 provider 请求都计费，包括普通 turns、semantic compaction、重试和 ContextUpdate shadow 所在的正常响应。没有可固定 seed 时，A/B 的 turns、输出和总费用不能被当作精确因果均值；报告必须披露投影前采样分歧。可直接归因的是 gate 决策、实际 replacement、continuation/cache 变化和同一轨迹中是否避免紧随其后的 compact。

## 11. 全面应用定义

“全面应用”不是把所有动作和工具默认打开，而是：

1. 机制、恢复、控制面、观测和 kill switch 可生产使用；
2. 每个正式 allowlist 工具都有真实质量等价且净值为正的证据；
3. 未证明价值的工具明确保持 KEEP/关闭，而不是伪造收益；
4. 100% rollout 仍受 provider/model/tool allowlist、session budget、anomaly circuit 和动态 kill switch 约束；
5. 逐工具报告与整体报告能从保留的 request ledger 和 evaluator 工件独立复算。

## 12. 实施与评测报告

- [逐工具真实评测](../reports/progressive-context-compaction-tools-2026-08-11.md)
- [全面应用与发布报告](../reports/progressive-context-compaction-rollout-2026-08-11.md)
- [DeepSeek V4 Flash 真实 A/B 与优化报告](../reports/progressive-context-compaction-deepseek-v4-flash-2026-08-11.md)
- [6K 提前收益触发真实 A/B 与方案收敛报告](../reports/progressive-context-compaction-benefit-trigger-2026-08-13.md)
