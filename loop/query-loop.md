# Loop 模块设计参考文档

> 本文档面向 Claude Code Go 复刻版贡献者，系统描述 `loop` 包的设计意图、与 TypeScript 原版的对应关系、当前实现状态及后续演进规划。

---

## 一、概述

`loop` 包是 Claude Code 的**代理执行核心**，负责驱动"用户消息 → LLM 流式响应 → 工具调用 → 工具结果回填 → 再次调用 LLM"这一完整的 Agentic Loop，直到模型不再发出工具调用或达到最大轮次上限为止。

### 核心职责

| 职责 | 说明 |
|------|------|
| **消息管理** | 维护完整会话历史，追加用户消息、助手消息、工具结果消息 |
| **流式处理** | 解析 SSE 流事件，按块（Block）累积文本/思考/工具调用内容 |
| **工具分派** | 将并发安全工具并行执行，非并发工具串行执行，保序回填结果 |
| **上下文压缩** | 集成 compact 包；每轮前 Microcompact 裁剪旧工具结果，超阈值时触发 Auto-compact |
| **Hook 集成** | 每次工具执行前后运行 PreToolUse/PostToolUse hooks，注入 system-reminder 消息 |
| **事件发布** | 通过回调 `onEvent` 向调用方实时推送 text/thinking/tool\_use/tool\_result/turn\_end/error 事件 |
| **硬限制防护** | 消息数超过 500 条时强制截断，防止无限增长 |

---

## 二、原版（TS）设计详情

TypeScript 原版 `src/query.ts`（约 1730 行）+ `src/query/` 子目录实现了一套远比 Go 复杂的状态机。

### 2.1 整体架构：异步生成器 + 可变 State

```
query(prompt) ──► queryLoop(state)
                    │
                    ▼
             async function* queryLoop()
             ├── while(true) 无限循环
             ├── State 结构（可变，跨迭代传递）
             │     messages, toolUseContext, autoCompactTracking,
             │     maxOutputTokensRecoveryCount, hasAttemptedReactiveCompact,
             │     maxOutputTokensOverride, pendingToolUseSummary,
             │     stopHookActive, turnCount, transition
             └── yield 各类 AssistantMessage / ToolUseMessage / SystemMessage
```

`State` 不是函数返回值，而是在每次迭代开始前通过 `transition` 字段决定下一步行为，实现**续传**（continuation-passing）而非递归。

### 2.2 QueryConfig：不可变快照

每次 `query()` 调用入口，立即从环境/Statsig 拍摄一份不可变快照 `QueryConfig`，所有后续逻辑均使用该快照，避免运行时配置变化导致行为不一致：

```typescript
type QueryConfig = {
  sessionId: string
  streamingToolExecution: boolean   // 是否启用流式工具执行
  emitToolUseSummaries: boolean      // 是否生成工具批次摘要
  isAnt: boolean                     // 内部账户标志
  fastModeEnabled: boolean           // 快速模式
}
```

### 2.3 流式处理

原版使用 `streamResponse()` 异步生成器，每次从 Anthropic SDK 拿到 SSE chunk 即 yield：

- `text` chunk → 立即 yield TextBlock，同时推送 UI 事件
- `input_json_delta` → 累积 toolInput buffer
- `content_block_stop` → 完成 ToolUseBlock 并（如开启 `streamingToolExecution`）立即启动工具执行
- `message_stop` → 收集最终 Usage，结束流

**StreamingToolExecutor**（TS 独有）：在流仍在传输时，已停止的 ToolUseBlock 即被并发启动执行，而不等待整个 message 结束。这可以大幅降低多工具批次的总延迟。

### 2.4 工具调用协议

```
assistantMsg (含 ToolUseBlock[])
       │
       ▼
StreamingToolExecutor.startTool(toolUse)
       │  ← 流式执行，工具可能在下一个 ToolUseBlock 到达前已完成
       ▼
toolResults[]  ──► ToolResultMessage
       │
       ▼
（如开启 emitToolUseSummaries）
  Haiku 异步调用生成工具批次摘要
  pendingToolUseSummary = Promise<ToolUseSummaryMessage>
       │  ← 在下一轮 model stream 期间解析，作为 context 追加
       ▼
（继续下一轮循环）
```

### 2.5 上下文管理（7 层防御）

| 层级 | 策略 | 触发条件 |
|------|------|----------|
| L1 | Microcompact（清除旧工具结果） | 每轮 API 调用前 |
| L2 | ToolResultBudget（截断超大结果） | 工具结果入 history 前 |
| L3 | ResultStore（持久化超大结果） | 结果超阈值时落盘 |
| L4 | Snip Compact（消息片段压缩） | 上下文接近容量 |
| L5 | Reactive Compact（触发式摘要） | 发现 context_window_full 错误 |
| L6 | Auto-compact（定时摘要） | `ShouldCompact()` 返回 true |
| L7 | Force Truncate（强制截断，最后手段） | 压缩失败或消息数超限 |

### 2.6 Token Budget 与 Task Budget

**Token Budget**（`src/query/tokenBudget.ts`）：
- `COMPLETION_THRESHOLD = 0.9`：输出 token 超过最大输出的 90% 时触发 continue
- `DIMINISHING_THRESHOLD = 500`：连续 continue 收益递减阈值
- `BudgetTracker` 跟踪 `continuationCount` 和 `lastDeltaTokens`
- `checkTokenBudget()` 返回 continue / stop 决策，模型被注入剩余 token 提示

**Task Budget**（API 级别 `output_config`）：独立于 Token Budget，控制模型输出的绝对上限。

### 2.7 错误恢复与续传路径（7+ 条）

| 转换名称 | 触发条件 | 行为 |
|---------|---------|------|
| `collapse_drain_retry` | 流中断，context collapse | 压缩消息后重试 |
| `reactive_compact_retry` | `context_window_full` API 错误 | 触发摘要压缩后重试 |
| `max_output_tokens_escalate` | 输出达到 max_tokens | 升级 max_tokens 参数继续 |
| `max_output_tokens_recovery` | 多次 escalate 后仍未完成 | 降级恢复策略 |
| `stop_hook_blocking` | Stop hook 返回阻塞 | 循环等待 hook 释放 |
| `token_budget_continuation` | Token budget 续传 | 注入 token 余量提示后继续 |
| `next_turn` | 正常工具结果回填 | 标准下一轮 |

### 2.8 模型回退（FallbackTriggeredError）

当流式响应中发生 `FallbackTriggeredError`（如 Claude 3 Haiku → Sonnet 降级）：
1. 当前 assistant 消息被作为**墓碑消息（tombstone）** yield 给调用方
2. 剥离 thinking signature（避免跨模型校验失败）
3. 使用新模型重新发起请求

### 2.9 Stop Hooks / TeammateIdle / TaskCompleted

每轮无工具调用的最终响应后，原版会运行一套后处理 hooks（`src/query/stopHooks.ts`，474 行）：

- **Stop hooks**：可阻塞循环、注入 reminder、触发新轮
- **TeammateIdle hooks**：多代理协作时通知队友空闲
- **TaskCompleted hooks**：任务完成时触发
- **副作用（fire-and-forget）**：记忆提取、auto-dream、prompt suggestion

### 2.10 后采样 Hooks 与预取

- **Post-sampling hooks**：模型输出后、工具执行前触发，可修改输出
- **Memory prefetch**：查询开始时后台预取相关记忆
- **Skill discovery prefetch**：后台发现可用 skill

---

## 三、Go 实现现状

### 3.1 能力对照表

| 能力 | 原版 TS | Go 状态 | 实现文件 |
|------|---------|---------|---------|
| 基本 Agentic Loop（消息 → LLM → 工具 → 回填） | ✅ | ✅ | `query.go` |
| SSE 流式解析（per-block 状态机） | ✅ | ✅ | `query.go` |
| 交错工具调用（index-keyed block map） | ✅ | ✅ | `query.go` |
| 并发安全工具并行执行 | ✅ | ✅ | `concurrent.go` |
| PreToolUse / PostToolUse hooks | ✅ | ✅ | `concurrent.go` |
| System reminder 注入 | ✅ | ✅ | `query.go` |
| Microcompact（旧工具结果清除） | ✅ | ✅ | `query.go` + `compact` |
| ToolResultBudget（超大结果截断） | ✅ | ✅ | `query.go` + `compact` |
| ResultStore（超大结果落盘） | ✅ | ✅ | `query.go` + `compact` |
| Auto-compact（定时摘要压缩） | ✅ | ✅ | `query.go` + `compact` |
| Force truncate（最后手段截断） | ✅ | ✅ | `query.go` |
| 消息数硬限制（500条） | ✅ | ✅ | `query.go` |
| ThinkingBlock 支持（签名保留） | ✅ | ✅ | `query.go` |
| 畸形 JSON 降级为 TextBlock | ✅ | ✅ | `query.go` |
| OpenAI 兼容重复 JSON 解析 | ✅ | ✅ | `query.go` |
| QueryConfig 不可变快照 | ✅ | ❌ | — |
| StreamingToolExecutor | ✅ | ❌ | — |
| Token Budget（90% 续传） | ✅ | ❌ | — |
| Task Budget（API 级输出控制） | ✅ | ❌ | — |
| 模型回退 + Tombstone | ✅ | ❌ | — |
| Reactive Compact（API 错误触发） | ✅ | ❌ | — |
| Snip Compact（片段压缩） | ✅ | ❌ | — |
| Stop hooks | ✅ | ❌ | — |
| TeammateIdle hooks | ✅ | ❌ | — |
| TaskCompleted hooks | ✅ | ❌ | — |
| Post-sampling hooks | ✅ | ❌ | — |
| Memory prefetch | ✅ | ❌ | — |
| Skill discovery prefetch | ✅ | ❌ | — |
| 工具批次摘要生成 | ✅ | ❌ | — |
| collapse_drain_retry 恢复路径 | ✅ | ❌ | — |

### 3.2 数据流图（Go 现状）

```
Run(ctx, userMessage, onEvent)
│
├── append UserMessage
│
└── for turnCount < MaxTurns
    │
    ├── [L1] Microcompact(messages) ──► apiMessages（裁剪旧工具结果）
    │
    ├── provider.CreateStream(params)
    │        │
    │        ▼
    │   processStream()
    │   ┌─────────────────────────────────────────┐
    │   │  blocks map[int]*blockState              │
    │   │                                          │
    │   │  EventContentBlockStart                  │
    │   │    └─► blocks[idx] = &blockState{...}   │
    │   │                                          │
    │   │  EventContentBlockDelta                  │
    │   │    ├─► text_delta    → text.Builder      │
    │   │    ├─► thinking_delta→ text.Builder      │
    │   │    └─► input_json_delta→toolInput.Builder│
    │   │                                          │
    │   │  EventContentBlockStop                   │
    │   │    ├─► toolName≠""  → ToolUseBlock       │
    │   │    ├─► Thinking     → ThinkingBlock      │
    │   │    └─► default      → TextBlock          │
    │   │                                          │
    │   │  EventMessageDelta → usage.OutputTokens  │
    │   └─────────────────────────────────────────┘
    │        │
    │        ▼ assistantMsg, usage
    │
    ├── ctxWindow.UpdateUsage(usage)
    ├── append assistantMsg
    │
    ├── GetToolUses() == 0 ?
    │   └─► onEvent(TurnEnd) → return nil ✓
    │
    ├── executeToolsConcurrently()
    │   ┌───────────────────────────────────────────┐
    │   │  partition: concurrentIndices / sequential │
    │   │                                            │
    │   │  concurrent ──► goroutines (parallel)      │
    │   │    ├─ PreToolUse hook                       │
    │   │    ├─ reg.ExecuteToolWithError()            │
    │   │    ├─ PostToolUse hook                      │
    │   │    └─ callbackCh (序列化回调)               │
    │   │                                            │
    │   │  sequential ──► for loop (in-order)        │
    │   │    ├─ PreToolUse hook                       │
    │   │    ├─ reg.ExecuteToolWithError()            │
    │   │    └─ PostToolUse hook                      │
    │   └───────────────────────────────────────────┘
    │        │ toolResults, reminders
    │
    ├── [L3] resultStore.ProcessResult() （可选）
    ├── [L2] toolBudget.Apply()
    ├── append ToolResultMessage
    ├── append reminders as UserMessage（<system-reminder>）
    │
    ├── onEvent(TurnEnd)
    │
    ├── [L6] ctxWindow.ShouldCompact() ?
    │   ├─ YES → compactor.Compact() ──► q.messages
    │   │         └─ 失败 → forceTruncate() [L7]
    │   └─ NO  → 继续
    │
    └── len(messages) > 500 ? → forceTruncate() [L7]

return fmt.Errorf("max turns exceeded")
```

### 3.3 组件文件一览

| 文件 | 行数 | 职责 |
|------|------|------|
| `query.go` | 532 | QueryLoop 结构体、Run()、processStream()、forceTruncate()、parseToolInputJSON() |
| `concurrent.go` | 277 | executeToolsConcurrently()、isConcurrentSafe()、runPreToolHooks()、runPostToolHooks() |
| `query_test.go` | 354 | processStream() 单元测试（8 个用例） |
| `concurrent_test.go` | — | executeToolsConcurrently() 单元测试 |
| `integration_test.go` | — | 端到端集成测试（mockProvider） |

### 3.4 常量与配置对照

| 常量/配置项 | TS 原版 | Go 实现 | 备注 |
|------------|---------|---------|------|
| 最大轮次 | `MAX_TURNS`（动态，约100） | `defaultMaxTurns = 100` | 相同 |
| 消息数硬限制 | 依赖压缩策略 | `maxMessagesHardLimit = 500` | Go 新增保险 |
| Token budget 阈值 | `COMPLETION_THRESHOLD = 0.9` | ❌ 未实现 | — |
| 续传收益递减阈值 | `DIMINISHING_THRESHOLD = 500` | ❌ 未实现 | — |
| Microcompact 保留轮数 | 动态 | `DefaultMicrocompactConfig()` | 见 compact 包 |
| Auto-compact 保留消息数 | 20 | `KeepRecent: 20` | 相同 |
| 并发安全工具（默认） | Read/Glob/Grep/WebFetch | Read/Glob/Grep | WebFetch 未列入 |

---

## 四、关键知识背景

### 4.1 SSE 流式处理协议

Anthropic API 基于 Server-Sent Events（SSE）推送流式响应，事件类型按顺序为：

```
message_start          → 包含初始 usage（InputTokens）
  content_block_start  → 开始新内容块（text / tool_use / thinking）
    content_block_delta → 增量内容（text_delta / input_json_delta / thinking_delta）
    ...
  content_block_stop   → 块结束，触发完整 Block 构建
  message_delta        → 包含最终 OutputTokens
message_stop           → 消息结束
```

**关键实现要点**：
- 同一消息中多个工具调用的 `content_block_delta` 可能**交错到达**（OpenAI 兼容接口尤为常见），必须用 `map[int]*blockState` 按 `Index` 分别累积，不能使用单一变量
- `input_json_delta` 为 JSON 字符串的分片，需完整拼接后才能解析
- OpenAI 兼容代理（vLLM 等）可能在分片后再次发送完整 JSON，导致 `{"a":1}{"a":1}` 格式；须用 `json.Decoder` 只解析第一个对象

### 4.2 工具调用消息格式规范

```
Assistant Message:
  Content: [
    TextBlock{Text: "I'll help you..."},
    ToolUseBlock{ID: "toolu_01", Name: "Bash", Input: {"command": "ls -la"}},
    ToolUseBlock{ID: "toolu_02", Name: "Read", Input: {"file_path": "/tmp/x"}}
  ]

User Message（工具结果）:
  Content: [
    ToolResultBlock{ToolUseID: "toolu_01", Content: "file1\nfile2", IsError: false},
    ToolResultBlock{ToolUseID: "toolu_02", Content: "...", IsError: false}
  ]
```

**严格约束**：
- `ToolResultBlock.ToolUseID` 必须与对应 `ToolUseBlock.ID` 精确匹配，否则 API 返回 400
- 工具结果消息必须作为 `user` 角色消息发送
- 助手消息中的 `ToolUseBlock` 与后续 `ToolResultBlock` 必须成对出现，不能被截断分割

### 4.3 ConcurrentSafe 接口

```go
// 工具实现此接口以声明并发安全
type ConcurrentSafe interface {
    IsConcurrentSafe() bool
}

// 默认安全工具（无需实现接口）
switch toolName {
case "Read", "Glob", "Grep":
    return true
}
```

并发执行时通过 `callbackCh` channel 序列化 `onResult` 回调，避免 UI 输出交错。基础设施错误（如 context 取消）通过 `sync.Once` + `toolCancel()` 传播给所有 goroutine。

### 4.4 System Reminder 注入机制

Hooks 可返回 `SystemReminder` 字符串。Loop 在工具结果消息之后插入：

```go
q.messages = append(q.messages, types.UserMessage(
    "<system-reminder>\n" + combined + "\n</system-reminder>",
))
```

这保证了 LLM 在下一轮能看到 hook 注入的上下文，而不污染正式的工具结果结构。

---

## 五、评估指标

### 5.1 延迟类指标

| 指标 | 定义 | 目标值 | 测量方法 |
|------|------|------|---------|
| **TTFT**（首 token 延迟） | `Run()` 调用到第一个 `EventText` 事件的时间 | < 500ms（本地模拟） | `time.Now()` 打点 |
| **单轮总延迟** | 从 API 调用到 `EventTurnEnd` 的时间 | < 2s（模拟） | 轮次级计时 |
| **工具执行延迟** | `executeToolsConcurrently()` 的总耗时 | 取决于工具 | 函数级打点 |
| **流处理开销** | `processStream()` 的 CPU 时间 | < 1ms/event | benchmark |

### 5.2 可靠性类指标

| 指标 | 定义 | 目标值 |
|------|------|------|
| **工具调用成功率** | 成功执行 / 总工具调用次数 | > 99% |
| **错误恢复率** | 非 context cancel 错误中成功恢复的比例 | > 95% |
| **畸形 JSON 降级率** | 触发 TextBlock 降级的工具调用比例 | < 0.1% |
| **并发执行正确率** | 并发工具结果与预期完全匹配的比例 | 100% |

### 5.3 吞吐量类指标

| 指标 | 定义 | 目标值 |
|------|------|------|
| **消息处理吞吐量** | 每秒处理的 SSE 事件数 | > 10,000 events/s |
| **轮次吞吐量** | 每分钟完成的 Loop 轮次数 | 取决于 LLM 延迟 |
| **并发工具加速比** | 串行耗时 / 并发耗时 | > N×0.8（N=工具数） |

### 5.4 功能覆盖度指标

| 维度 | 能力总项 | Go 已实现 | 覆盖率 |
|------|---------|----------|------|
| 核心循环 | 8 | 8 | 100% |
| 流式处理 | 6 | 6 | 100% |
| 工具执行 | 5 | 5 | 100% |
| 上下文管理 | 7 | 5 | 71% |
| 错误恢复 | 7 | 1 | 14% |
| Hook 系统 | 5 | 2 | 40% |
| 高级特性 | 8 | 0 | 0% |
| **总计** | **46** | **27** | **59%** |

---

## 六、与原版的差距及后续规划

### P0 — 核心可靠性（建议尽快实现）

**1. Token Budget 续传机制**
- 原版：输出 token 达到 `max_tokens` 的 90% 时，注入剩余 token 提示并自动续传
- Go 现状：达到 `max_tokens` 时直接截断，长任务可能不完整
- 实现建议：在 `Run()` 检测 `usage.OutputTokens / config.MaxTokens > 0.9` 时注入提示继续循环

**2. Reactive Compact（API 错误触发式压缩）**
- 原版：收到 `context_window_full` API 错误时触发摘要压缩后重试
- Go 现状：API 错误直接返回给调用方，无法自愈
- 实现建议：在 `provider.CreateStream()` 错误处理中检测错误类型，触发 compact 后重试

### P1 — 性能与体验提升（中期规划）

**3. StreamingToolExecutor（流中并发工具启动）**
- 原版：工具调用 block 完成即启动执行，不等 message_stop
- Go 现状：等待 `processStream()` 完全返回后才执行工具，多工具批次延迟更高
- 实现建议：`processStream()` 改为 channel 输出，`Run()` 并发消费 ToolUseBlock

**4. Snip Compact（片段级压缩）**
- 原版：上下文接近容量时可仅压缩中间片段，保留头尾
- Go 现状：仅有整体摘要压缩，压缩粒度粗
- 实现建议：在 compact 包新增 SnipCompactor，loop 集成

**5. 模型回退与 Tombstone**
- 原版：`FallbackTriggeredError` 时切换模型重试，yield tombstone
- Go 现状：模型错误直接失败
- 实现建议：provider 层暴露 FallbackError 类型，loop 层处理回退

### P2 — 完整性补全（长期规划）

**6. Stop hooks / TeammateIdle / TaskCompleted**
- 原版：无工具调用的最终响应后运行后处理 hooks，支持阻塞/继续/注入 reminder
- Go 现状：Hook 系统仅覆盖 PreToolUse/PostToolUse
- 实现建议：在 hooks 包新增 StopHook 类型，loop 在无工具调用时触发

**7. 工具批次摘要（ToolUseSummary）**
- 原版：用 Haiku 异步生成工具批次摘要，作为压缩上下文追加
- Go 现状：无此机制
- 实现建议：在 compact 包新增 ToolSummarizer，loop 中 fire-and-forget 调用

**8. QueryConfig 不可变快照**
- 原版：查询入口拍摄环境快照，全程使用不变配置
- Go 现状：Config 在 Run() 期间可被 SetModel() 修改
- 实现建议：Run() 开始时复制一份 config 快照，后续仅使用快照

**9. Post-sampling hooks**
- 原版：模型输出后、工具执行前可拦截并修改输出
- Go 现状：无此生命周期点
- 实现建议：在 processStream() 返回后、executeToolsConcurrently() 前插入 hook 点

**10. Memory / Skill Prefetch**
- 原版：查询开始时后台预取记忆和 skill，降低首轮延迟
- Go 现状：无后台预取
- 实现建议：Run() 开始时 goroutine 预取，结果注入 system prompt

### 差距优先级汇总

```
P0（核心可靠）  ────────────► Token Budget 续传、Reactive Compact
                              预计工作量：中（各 1-2 天）

P1（性能体验）  ────────────► StreamingToolExecutor、Snip Compact、模型回退
                              预计工作量：中-大（各 2-5 天）

P2（完整性）    ────────────► Stop hooks、ToolUseSummary、QueryConfig 快照
                              预计工作量：大（各 3-7 天）
```

---

*文档生成时间：2026-04-05*
*参考源文件：`gosrc/loop/query.go`（532行）、`gosrc/loop/concurrent.go`（277行）、`src/query.ts`（1730行）、`src/query/tokenBudget.ts`、`src/query/stopHooks.ts`（474行）、`src/query/config.ts`*
