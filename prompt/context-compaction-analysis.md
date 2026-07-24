# Context Compaction: TS 原版 vs Go Fork 完整差异分析

> 生成时间: 2026-04-13
> 目的: 诊断 Go fork 上下文窗口溢出问题 (200.9K/200.0K) 并指导完整修复

---

## 一、架构概览

### TS 原版 (src/services/compact/)

6 层压缩体系，约 4000+ 行代码：

| 层级 | 文件 | 职责 |
|------|------|------|
| L1: 阈值与触发 | `autoCompact.ts` (352行) | 计算 effectiveContextWindow、autoCompactThreshold、断路器、会话递归守卫 |
| L2: 主压缩流程 | `compact.ts` (1706行) | 全量/部分压缩、图片剥离、PTL重试(groupMessagesByApiRound)、8+种 post-compact 附件恢复、pre/post hooks |
| L3: Prompt 工程 | `prompt.ts` (375行) | 3种 prompt 变体(BASE/PARTIAL_FROM/PARTIAL_UP_TO)、`<analysis>` 草稿本 + formatCompactSummary() 剥离、NO_TOOLS 强制约束 |
| L4: 微压缩 | `microCompact.ts` (531行) | 3层架构：time-based MC + cached MC (cache_edits API, ant-only) + legacy removed |
| L5: 工具结果持久化 | `toolResultStorage.ts` (1041行) | per-tool 持久化 + per-message 200K 聚合预算 + ContentReplacementState 3态管理 |
| L6: Session Memory | `sessionMemoryCompact.ts` (631行) | 实验性 session memory 替代 LLM summary，minTokens=10K / maxTokens=40K / minTextBlockMessages=5 |

### Go Fork (gosrc/compact/)

4 层压缩体系，约 1000+ 行代码：

| 层级 | 文件 | 职责 |
|------|------|------|
| L1: 阈值与触发 | `compact.go` (457行) | ContextWindow、ShouldCompact()、断路器、CalibratedCounter |
| L2: 主压缩流程 | `compact.go` (SummaryCompactor) | LLM summarize + PTL 重试 + HistorySnip fallback + post-compact 文件恢复 |
| L3: Prompt 工程 | `prompt.go` (18行) | 简化 6 段 prompt，无 `<analysis>` 草稿，无 NO_TOOLS 约束 |
| L4: 微压缩 | `microcompact.go` (137行) | 基本 time-based + idle 策略 |
| 持久化 | `resultstore.go` (67行) | per-tool >50K 持久化到磁盘，无 per-message 聚合预算 |

---

## 二、已修复的关键缺陷

### P0-1: ResultStore 未注入 (已修复 ✅)

**问题**: `resultstore.go` 已实现完整的 >50K chars 持久化逻辑，但 `engine/core.go` 的 `buildConv()` 没有调用 `ql.SetResultStore()`。导致所有大工具结果直接堆积在内存中。

**修复**: 在 `buildConv()` 中注入:
```go
sessionDir := filepath.Join(session.DefaultDir(), sessionID)
ql.SetResultStore(compact.NewResultStore(sessionDir))
```

### P0-2: 压缩阈值公式错误 (已修复 ✅)

**问题**: Go 使用简单的 `0.80 * MaxTokens` 作为阈值。对于 200K 窗口，阈值 = 160K。
TS 使用: `effectiveContextWindow = contextWindow - min(maxOutputTokens, 20000)`，然后 `threshold = effectiveContextWindow - 13000`。
对于 200K 窗口 + 16384 maxOutput: effective = 183616, threshold = 170616。
**差距**: Go 在 160K 就触发（过早），但更重要的是 `Remaining()` 也用错误公式导致 UI 显示不准确。

**修复**: 添加 TS 等价常量和方法:
```go
const (
    AutocompactBufferTokens          = 13000
    WarningThresholdBufferTokens     = 20000
    MaxConsecutiveAutocompactFailures = 3
)

func (cw *ContextWindow) effectiveContextWindowSize() int {
    outputReserve := cw.MaxOutputTokens
    if outputReserve <= 0 || outputReserve > WarningThresholdBufferTokens {
        outputReserve = WarningThresholdBufferTokens
    }
    return cw.MaxTokens - outputReserve
}

func (cw *ContextWindow) autoCompactThreshold() int {
    return cw.effectiveContextWindowSize() - AutocompactBufferTokens
}
```

### P0-3: 断路器缺失 (已修复 ✅)

**问题**: TS 有 `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES = 3`，连续失败 3 次后停止重试。Go 无此机制，会无限循环尝试压缩。

**修复**: 在 `ContextWindow` 中添加:
```go
func (cw *ContextWindow) RecordCompactSuccess() { cw.consecutiveFailures = 0 }
func (cw *ContextWindow) RecordCompactFailure()  { cw.consecutiveFailures++ }
func (cw *ContextWindow) ShouldCompact() bool {
    if cw.consecutiveFailures >= MaxConsecutiveAutocompactFailures {
        return false  // 断路器跳闸
    }
    return cw.UsedInput > cw.autoCompactThreshold()
}
```

### P1-1: `/compact` 命令使用简单截断 (已修复 ✅)

**问题**: `/compact` 命令直接截断保留最后 4 条消息，不调用 LLM summarize。

**修复**: 在 `commands/commands.go` 添加 `CompactFunc` 回调，`/compact` 优先使用 LLM compaction + fallback 截断。

### P2: MaxContextTokens 自动检测 (已修复 ✅)

**问题**: `main.go` 硬编码 `MaxContextTokens: 200000`，对于其他模型不准确。

**修复**: `buildConv()` 中当 `MaxContextTokens <= 0` 时调用 `provider.LookupMaxContext(model)` 自动检测。

---

## 三、待修复的差距

### 3.1 Prompt 质量差距 (优先级: HIGH)

#### TS 的 9 段式 Prompt (prompt.ts)

TS 定义了 3 种 prompt 变体和多个辅助机制：

**NO_TOOLS_PREAMBLE** — 强制禁止工具调用:
```
CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.
- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.
```

**`<analysis>` 草稿本** — Chain-of-thought 提升总结质量:
```
Before providing your final summary, wrap your analysis in <analysis> tags...
1. Chronologically analyze each message and section of the conversation...
   - The user's explicit requests and intents
   - Your approach to addressing the user's requests
   - Key decisions, technical concepts and code patterns
   - Specific details like: file names, full code snippets, function signatures, file edits
   - Errors that you ran into and how you fixed them
   - Pay special attention to specific user feedback...
2. Double-check for technical accuracy and completeness
```

**9 个 Summary 段**:
1. **Primary Request and Intent** — 用户所有明确请求的详细描述
2. **Key Technical Concepts** — 重要技术概念、框架
3. **Files and Code Sections** — 文件路径 + 代码片段 + 变更原因
4. **Errors and Fixes** — 错误及修复方法，特别是用户反馈
5. **Problem Solving** — 已解决的问题和持续排查
6. **All User Messages** — 列出所有非 tool_result 的用户消息（关键!）
7. **Pending Tasks** — 待完成任务
8. **Current Work** — 当前正在进行的工作（最近消息详述）
9. **Optional Next Step** — 下一步（必须与用户最近明确请求直接相关）

**NO_TOOLS_TRAILER** — 末尾再次强化:
```
REMINDER: Do NOT call any tools. Respond with plain text only —
an <analysis> block followed by a <summary> block.
Tool calls will be rejected and you will fail the task.
```

**formatCompactSummary()** — 后处理剥离 `<analysis>` 标签:
```typescript
function formatCompactSummary(summary: string): string {
    // Strip analysis section — it's a drafting scratchpad
    formattedSummary = formattedSummary.replace(/<analysis>[\s\S]*?<\/analysis>/, '')
    // Extract and format summary section
    const summaryMatch = formattedSummary.match(/<summary>([\s\S]*?)<\/summary>/)
    if (summaryMatch) {
        formattedSummary = `Summary:\n${content.trim()}`
    }
    return formattedSummary.trim()
}
```

#### Go 当前的 6 段 Prompt (prompt.go)

```go
const CompactSystemPrompt = "You are a helpful AI assistant tasked with summarizing conversations."

const CompactUserPrompt = `Analyze the conversation and produce a structured summary covering:

1. **Primary Request**: What the user originally asked for
2. **Key Technical Concepts**: Important technical details, patterns, or decisions
3. **Files and Code**: Files read, written, or modified (with paths)
4. **Errors and Fixes**: Problems encountered and how they were resolved
5. **Current Work**: What was being worked on most recently
6. **Pending Tasks**: Any unfinished work or known remaining issues

Keep the summary concise but comprehensive...
`
```

**差距**:
- ❌ 缺少 NO_TOOLS 保护（模型可能尝试调用工具，浪费 turn）
- ❌ 缺少 `<analysis>` Chain-of-thought 草稿（降低总结质量）
- ❌ 只有 6 段，缺少: Problem Solving, All User Messages, Optional Next Step
- ❌ 缺少 `formatCompactSummary()` 后处理（`<analysis>` 标签会残留在上下文中）
- ❌ 缺少详细的段落说明和示例结构

### 3.2 图片/文档剥离 (优先级: MEDIUM)

#### TS (compact.ts:145-200)
```typescript
export function stripImagesFromMessages(messages: Message[]): Message[] {
    // 将 image 块替换为 [image] 文本标记
    // 将 document 块替换为 [document] 文本标记
    // 递归处理嵌套在 tool_result 中的图片/文档
}
```

在发送给 summarizer LLM 之前调用，防止图片占用 compaction API 本身的 context window。

#### Go 当前状态
❌ 完全缺失。如果用户在会话中附带了图片，图片 base64 数据会被发送给 summarizer，浪费大量 token。

### 3.3 Per-Message 聚合工具结果预算 (优先级: MEDIUM)

#### TS (toolLimits.ts + toolResultStorage.ts)

```typescript
export const MAX_TOOL_RESULTS_PER_MESSAGE_CHARS = 200_000
```

当单条 user message 中的所有 tool_result 块总计超过 200K chars 时，最大的块会被持久化到磁盘并替换为预览。这防止 N 个并行工具各产生 40K 结果、合计 400K 的情况。

**3态管理** (`ContentReplacementState`):
- `mustReapply`: 重新应用持久化（内容已改变）
- `frozen`: 不变（缓存命中）
- `fresh`: 新结果，首次评估

#### Go 当前状态
`resultstore.go` 只有 per-tool 的 >50K chars 持久化。❌ 缺少 per-message 的聚合预算检查。

### 3.4 Post-Compact 附件恢复 (优先级: LOW)

#### TS (compact.ts:518-594) — 8+ 种附件恢复

压缩后恢复的上下文（按重要性排序）:

| 附件类型 | Go 状态 | 说明 |
|---------|---------|------|
| **文件恢复** (readFileState) | ✅ 已有 | `post_compact.go` 恢复最近 5 个文件 |
| **Plan 附件** | ❌ 缺失 | 重新注入当前 plan 状态 |
| **Skill 附件** | ❌ 缺失 | 重新注入已加载的 skill 内容 |
| **Deferred Tools Delta** | ❌ 缺失 | 重新公告延迟加载的工具 |
| **Agent Listing Delta** | ❌ 缺失 | 重新公告 agent 列表 |
| **MCP Instructions Delta** | ❌ 缺失 | 重新公告 MCP 指令 |
| **Async Agent 附件** | ❌ 缺失 | 重新注入异步 agent 状态 |
| **Session Start Hooks** | ❌ 缺失 | 压缩后执行 session_start hooks |
| **Post-Compact Hooks** | ❌ 缺失 | 执行 post_compact hooks |

> 注: Go fork 当前不支持 Plan、Skill、MCP、Async Agent 等特性，因此这些附件恢复暂不需要实现。但文件恢复已正确实现。

### 3.5 Partial Compact (优先级: LOW)

#### TS (compact.ts:772-1040)

支持双向部分压缩:
- `'from'`: 从 pivot 之后开始总结，保留之前的（缓存命中）
- `'up_to'`: 总结 pivot 之前的，保留之后的

带有 `preservedSegment` 元数据用于消息 UUID 链接修补。

#### Go 当前状态
❌ 完全缺失。当前只支持全量压缩。

### 3.6 Session Memory Compact (优先级: LOW)

#### TS (sessionMemoryCompact.ts)

实验性功能：使用 session memory 替代 LLM summary。配置: `minTokens=10K, maxTokens=40K, minTextBlockMessages=5`。

#### Go 当前状态
❌ 完全缺失。

### 3.7 API-Round Grouping for PTL Retry (优先级: LOW)

#### TS (grouping.ts)

`groupMessagesByApiRound()` 按 assistant message ID 边界分组，允许更精细的 PTL 重试（丢弃最早的完整 API round 而非简单 turn）。

#### Go 当前状态
Go 的 `retryWithTruncation()` 使用简单的 `findNextTurnEnd()`（找到第一个 assistant 消息）。功能等价但粒度更粗。

### 3.8 API-Level Microcompact (优先级: SKIP)

#### TS (apiMicrocompact.ts)

`clear_tool_uses_20250919` 和 `clear_thinking_20251015` — Anthropic 内部 API 功能（`USER_TYPE === 'ant'`），通过特殊 API 参数让服务端清除旧 tool_use/thinking 块。

#### Go 当前状态
❌ 缺失，但 **不需要实现** — 这是 Anthropic 内部 API 特性，不适用于 OpenAI 兼容的 provider。

---

## 四、优先级排序与实施计划

### Phase 1: 立即修复 (影响上下文溢出的核心问题)

| # | 修复项 | 优先级 | 预计改动 |
|---|--------|--------|----------|
| 1 | **增强 prompt.go** — 9段 + `<analysis>` + NO_TOOLS + formatCompactSummary() | HIGH | ~200行 |
| 2 | **增强 summarize.go** — 使用新 prompt + `<analysis>` 剥离 | HIGH | ~30行 |
| 3 | **添加图片剥离** — stripImagesFromMessages 在压缩前调用 | MEDIUM | ~60行 |
| 4 | **Per-message 聚合预算** — 200K chars/message 的工具结果限制 | MEDIUM | ~100行 |

### Phase 2: 后续增强 (不直接影响溢出，但提升质量)

| # | 修复项 | 优先级 | 说明 |
|---|--------|--------|------|
| 5 | Partial Compact | LOW | Go fork 暂不需要消息选择器 UI |
| 6 | Session Memory Compact | LOW | 实验性功能，需要 session memory 基础设施 |
| 7 | API-Round grouping | LOW | 当前 turn-based PTL 重试已足够 |
| 8 | Post-compact 附件恢复增强 | LOW | 等 Plan/Skill/MCP 等特性实现后再添加 |

---

## 五、文件清单

### 已修改的文件
- `gosrc/compact/compact.go` — 阈值公式、断路器、常量
- `gosrc/engine/core.go` — ResultStore 注入、MaxContextTokens 自动检测、MaxOutputTokens 传递
- `gosrc/loop/query.go` — Config 增加 MaxOutputTokens、断路器调用
- `gosrc/commands/builtins.go` — `/compact` LLM 摘要优先
- `gosrc/commands/commands.go` — CompactFunc 回调
- `gosrc/repl.go` + `gosrc/repl_tui.go` — CompactFunc 注入
- `gosrc/compact/boundary_test.go` — 更新测试

### 待修改的文件 (Phase 1)
- `gosrc/compact/prompt.go` — 9段 prompt + NO_TOOLS + 格式化函数
- `gosrc/compact/summarize.go` — 使用新 prompt + `<analysis>` 剥离
- `gosrc/compact/compact.go` — 添加 stripImagesFromMessages + per-message budget

---

## 六、根因总结

**上下文溢出 (200.9K/200.0K) 的根本原因**:

1. **ResultStore 未注入** (P0-1) — 大工具结果 (>50K chars) 没有被持久化到磁盘，全部堆积在内存中
2. **阈值公式错误** (P0-2) — 使用简单 80% 而非 TS 的 `effectiveContextWindow - 13K buffer` 公式
3. **无断路器** (P0-3) — 压缩失败后无限重试，浪费 API 调用
4. **Prompt 过于简化** — 6段 vs 9段，缺少 `<analysis>` chain-of-thought，总结质量不够好导致压缩后上下文仍然偏大
5. **无图片剥离** — 图片数据发送给 summarizer 浪费 token
6. **无 per-message 聚合预算** — 并行工具可以在单条消息中产生过多结果

其中 1-3 已修复，4-6 为待修复项。
