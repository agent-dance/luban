# 上下文压缩架构 — 设计参考文档

> 基于 Claude Code TypeScript 原版（`../src`）提炼，定义 Go 实现的目标架构。
> 包含原版设计分析 + Go 当前实现详情。

---

## 一、分层防御模型

原版实现了 **7 层**逐级防御，每层拦截上一层遗漏的内容。它们在请求生命周期的不同阶段执行：

```
工具执行时 ──────┐
                ├─ 第0层: 工具输出持久化（单工具 >50K字符 → 存磁盘）
                ├─ 第1层: 单消息聚合预算（聚合 >200K字符 → 持久化最大的）
                │
API调用前 ──────├─ 第2层: 微压缩（清除旧工具输出内容）
                ├─ 第3层: API级上下文管理（服务端处理，180K阈值）
                │
API响应后 ──────├─ 第4层: 会话记忆提取（异步后台代理）
                ├─ 第5层: 完整LLM摘要压缩
                │         ↳ 5a: 优先使用会话记忆压缩（无需LLM调用）
                │         ↳ 5b: 回退到LLM摘要（结构化9节）
                │
压缩失败时 ─────├─ 第6层: PTL重试（截断头部，重试压缩）
```

---

## 二、原版各层详情

### 第0层 — 工具输出持久化

**触发时机：** 每次工具执行完成后，结果进入消息前。  
**阈值：** `min(tool.maxResultSizeChars, 50,000)` 字符。  
**算法：**
1. 若 `内容大小 > 阈值` → 将完整结果写入磁盘 `{sessionDir}/tool-results/{toolUseId}.txt`
2. 生成 2KB 预览（在换行符边界截断）
3. 用 `<persisted-output>` XML 包装替换结果内容（含预览 + 文件路径引用）

**意义：** 防止一次 `cat huge-file.log` 吃掉 25% 上下文窗口。模型仍能看到有用的预览。

### 第1层 — 单消息工具输出预算

**触发时机：** API调用前，当轮所有工具执行完成后。  
**阈值：** 单条用户消息聚合 200,000 字符。  
**算法：**
1. 将连续 tool result 按 API 消息合并分组
2. 分为：`frozen`（历史已有，不动）、`fresh`（本轮新增）
3. 按大小降序排列 fresh 结果
4. 持久化最大的 fresh 结果，直到聚合量 < 预算
5. 在 `ContentReplacementState` 中记录决策，保证 prompt cache 稳定

**意义：** 并行工具调用（5个 Grep 各返回 50K）可在单消息产生 250K。第0层管单个结果；第1层管聚合。

### 第2层 — 微压缩（Microcompact）

**触发时机：** API调用前，第1层之后。  
**子策略：**

**2a — 基于时间：** 若闲置 > 60分钟 → 清除除最近5个外的所有可压缩工具结果（内容替换为 `[Old tool result content cleared]`）。理由：60分钟后 prompt cache 已过期。

**2b — 缓存感知：** 构建 `cache_edits` 指令供服务端清理。不修改本地消息——保留 prompt cache 前缀。

**可压缩工具：** FileRead、Shell、Grep、Glob、WebSearch、WebFetch、FileEdit、FileWrite。

### 第3层 — API级上下文管理

**触发时机：** 作为 API 请求负载的一部分（由服务端处理）。  
**策略：**
- `clear_tool_uses`：input_tokens > 180K 时 → 将工具结果清理到 40K 目标
- `clear_thinking`：闲置 > 1小时时清除 thinking 块

### 第4层 — 会话记忆提取

**触发时机：** 后采样钩子，每次 API 响应后异步执行。  
**触发条件：** tokens > 10K 初始化 且（距上次提取增长 5K + 3次工具调用）。  
**方式：** 派生一个 forked agent（共享 prompt cache），使用 FileEdit 将关键事实写入 `session_memory.md`。后台运行，不阻塞。

### 第5层 — 完整压缩

**触发时机：** API响应后，若 `tokenCount > 有效窗口 - 13K缓冲`。  
**两条路径：**

**5a — 会话记忆压缩（优先，无需LLM调用）：**
1. 加载预提取的 session_memory.md
2. 计算要保留的消息（最近消息周围 10K–40K token 窗口）
3. 用会话记忆内容替换旧消息作为摘要
4. 比 5b 快得多——不需要 LLM 调用

**5b — LLM 摘要（回退方案）：**
1. 剥离图片 → `[image]`，移除重注入的附件
2. 将整个对话连同摘要提示发送给 Claude
3. 提示要求结构化 9 节摘要：
   - 主要请求、关键技术概念、文件与代码
   - 错误与修复、问题解决、所有用户消息
   - 待办任务、当前工作、可选下一步
4. 压缩后：重新读取最近 5 个文件（每个 5K tokens，共 50K）
5. 重新挂载已调用的 skills（每个 5K，共 25K）
6. 重置所有缓存、分类器审批、系统提示段落

### 第6层 — PTL（提示过长）重试

**触发时机：** 压缩请求本身超出上下文窗口。  
**算法：** 按 API 轮次分组消息 → 丢弃最早的组 → 重试。  
**最后手段：** 连续失败 3 次后触发熔断器。

---

## 三、Token 计数

| 方法 | 使用场景 | 精度 |
|------|---------|------|
| API `usage.input_tokens` | 阈值比较 | 精确 |
| `anthropic.messages.countTokens()` | 预检查 | 精确 |
| Tiktoken cl100k_base | 估算/预算计算 | ~90% |
| `len(text) / 4` | 粗略回退 | ~70% |
| 文件类型差异化比率 | JSON=2, 其他=4 | ~85% |

原版的核心函数 `tokenCountWithEstimation()` 对已知前缀使用 API 报告的真实 token 数，仅对最近一次响应后新增的部分使用粗略估算。

---

## 四、常量参考

| 常量 | 值 | 用途 |
|------|---|------|
| `DEFAULT_MAX_RESULT_SIZE_CHARS` | 50,000 | 单工具持久化阈值 |
| `MAX_TOOL_RESULTS_PER_MESSAGE_CHARS` | 200,000 | 单消息聚合预算 |
| `PREVIEW_SIZE_BYTES` | 2,000 | 持久化结果预览大小 |
| `AUTOCOMPACT_BUFFER_TOKENS` | 13,000 | 上下文限制前的缓冲 |
| `COMPACT_MAX_OUTPUT_TOKENS` | 20,000 | 摘要输出最大 token 数 |
| `POST_COMPACT_MAX_FILES` | 5 | 压缩后重新读取的文件数 |
| `POST_COMPACT_TOKEN_BUDGET` | 50,000 | 文件重读的 token 预算 |
| `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES` | 3 | 熔断器限制 |

---

## 五、Go 实现现状

### 5.1 实现状态总览

| 层级 | 原版（TS） | Go 状态 | 实现文件 |
|------|-----------|---------|---------|
| 0 | 工具输出持久化 | ✅ 已实现 | `compact/resultstore.go` |
| 1 | 单消息聚合预算 | ✅ 已实现（简化版） | `compact/compact.go` → `ToolResultBudget` |
| 2 | 微压缩 | ✅ 已实现（简化版） | `compact/microcompact.go` |
| 3 | API级上下文管理 | ❌ 未实现 | Anthropic 私有协议，暂不支持 |
| 4 | 会话记忆提取 | ❌ 未实现 | 需要较大架构改动 |
| 5a | 会话记忆压缩 | ❌ 未实现 | 依赖第4层 |
| 5b | LLM 摘要压缩 | ✅ 已实现 | `compact/summarize.go` + `compact/prompt.go` |
| 6 | PTL 重试 | ❌ 未实现 | 不再以无语义裁剪伪装为 PTL 重试；超限时显式失败关闭 |
| — | Context Collapse | ⚠️ 仅 staged-message adapter | `compact/context_collapse.go` — 支持 exact-scope 的进程内显式 marker 投影/overflow drain；marker 必须在会话提交前消费，持久化层 fail-closed；不支持完整 TS store/subsystem |
| — | HistorySnip（Go独有） | ⚠️ 仅可作为显式独立策略 | `compact/compact.go` — `SummaryCompactor` 的旧兼容开关已废弃且无作用，summarizer 失败时始终 fail-closed |
| — | forceTruncate | ❌ 已移除 | `loop/query.go` 不再静默保留 head/tail 并删除中段 |
| — | 500条消息安全上限 | ✅ fail-closed | 超限返回 typed error，历史保持原样，要求先完成语义压缩 |

### 5.2 Go 数据流

```
工具执行完成 → toolResults[]
    │
    ▼ [第0层] ResultStore.ProcessResult()       compact/resultstore.go
    │  逻辑：遍历每个 toolResult
    │  若 len(content) > 50,000 字符：
    │    → 写入磁盘 {sessionDir}/tool-results/{toolUseID}.txt
    │    → 生成 2KB 预览（在换行符边界截断）
    │    → 用 <persisted-output> XML 包装替换内容
    │  接入位置：loop/query.go:270-275
    │
    ▼ [第1层] ToolResultBudget.Apply()           compact/compact.go
    │  逻辑：遍历消息中的所有 ToolResultBlock
    │  若 len(content) > 30,000 字符（MaxCharsPerResult）：
    │    → 截断到 30,000 字符
    │    → 追加 "\n\n... (truncated, N chars total)" 标记
    │  注意：与原版的聚合预算不同，Go版是逐个截断
    │  接入位置：loop/query.go:277-284
    │
    ▼ 追加到 q.messages（完整本地历史）
    │
    ━━━ 下一轮 API 调用前 ━━━
    │
    ▼ [第2层] Microcompact()                     compact/microcompact.go
    │  逻辑：
    │    1. 扫描所有消息，找出可压缩工具的 ToolResultBlock
    │       可压缩工具：Read, Bash, Grep, Glob, WebSearch, WebFetch, Write, Edit
    │    2. 保留最近 N 个（默认 KeepRecent=10）不动
    │    3. 更早的 → 内容替换为 "[Old tool result content cleared]"
    │  重要：创建消息副本 apiMessages，不修改 q.messages 本体
    │  接入位置：loop/query.go:214-218
    │
    ▼ [Go adapter] staged Context Collapse projection compact/context_collapse.go
    │  逻辑：
    │    1. 仅识别 `[context-collapse-staged]` 显式 marker
    │    2. 用 marker 内的 collapsed view 替换 marker 之前的消息
    │    3. 保留 marker 之后的 tail
    │  限制：Go 不实现完整 TS context-collapse store/subsystem
    │  接入位置：loop/context_prepare.go
    │
    ▼ AutoCompactIfNeeded()                      compact/auto_compact.go
    │  注意：HistorySnip 与 staged collapse projection 释放的 token delta
    │  会合并传入 auto-compact threshold 判断。
    │
    ▼ apiMessages 发送给 API
    │
    ▼ API 响应后
    │
    ▼ [第5b层] SummaryCompactor.Compact()        compact/compact.go + compact/summarize.go
    │  触发条件：ContextWindow.ShouldCompact()
    │    → UsedInput > MaxTokens × 0.8（默认 200,000 × 80% = 160,000）
    │  逻辑：
    │    1. 收集旧消息（保留最近 KeepRecent=20 条）
    │    2. 拼接旧消息文本，调用 SummarizeFunc
    │    3. SummarizeFunc（compact/summarize.go）：
    │       → 用 provider 发起流式 API 调用
    │       → 系统提示："You are a helpful AI assistant tasked with summarizing conversations."
    │       → 用户提示要求 6 节结构化摘要（compact/prompt.go）：
    │         主要请求、关键技术概念、文件与代码、
    │         错误与修复、当前工作、待办任务
    │       → MaxTokens: 4096
    │    4. 用摘要消息替换旧消息
    │    5. 失败 → 返回错误并保留原 view；始终不自动 HistorySnip
    │  接入位置：loop/query.go:295-310
    │
    ▼ [安全上限] MessageHistoryLimitError        loop/query.go
    │  触发条件：provider 调用前或响应合并后 len(messages) > 500
    │  逻辑：不发送、不裁剪、不改写历史；返回 typed error，要求先语义压缩
```

### 5.3 各组件文件说明

| 文件 | 行数 | 职责 |
|------|------|------|
| `compact/compact.go` | ~310 | 核心类型定义：TokenCounter、TiktokenCounter、CalibratedCounter、ContextWindow、Compactor接口、ToolResultBudget、HistorySnip、SummaryCompactor |
| `compact/prompt.go` | ~30 | LLM 摘要压缩的系统提示和用户提示模板 |
| `compact/summarize.go` | ~50 | `NewLLMSummarizeFunc()`：创建调用 provider 的摘要函数闭包 |
| `compact/resultstore.go` | ~60 | `ResultStore`：大工具输出持久化到磁盘 + 2KB 预览 |
| `compact/microcompact.go` | ~90 | `Microcompact()`：清理旧工具结果，保留最近 N 个 |
| `loop/query.go` | ~480 | 查询循环主逻辑，集成以上所有组件的接入点 |

### 5.4 Go 常量对照

| Go 常量 | 值 | 对应原版常量 | 位置 |
|---------|---|-------------|------|
| `maxResultSizeChars` | 50,000 | `DEFAULT_MAX_RESULT_SIZE_CHARS` | `compact/resultstore.go` |
| `previewSizeBytes` | 2,000 | `PREVIEW_SIZE_BYTES` | `compact/resultstore.go` |
| `MaxCharsPerResult` | 30,000 | 无直接对应（原版用聚合预算） | `compact/compact.go` |
| `KeepRecent`（微压缩） | 10 | 原版默认 5 | `compact/microcompact.go` |
| `KeepRecent`（摘要） | 20 | 原版按 token 窗口动态计算 | `loop/query.go` 初始化 |
| `Threshold`（压缩触发） | 0.80 | `effectiveWindow - 13K` | `compact/compact.go` |
| `maxMessagesHardLimit` | 500 | 无对应（原版靠多层避免到此） | `loop/query.go`，仅作 fail-closed 上限 |

### 5.5 Token 计数实现

| 组件 | 状态 | 说明 |
|------|------|------|
| `TiktokenCounter` | ✅ 活跃 | 使用 cl100k_base 编码，最接近 Claude tokenizer |
| `CalibratedCounter` | ⚠️ 未接入 | `Calibrate()` 只在测试中调用，设计是用 API usage 校准估算比率 |
| `ContextWindow.EstimateMessages()` | ✅ 活跃 | 用于压缩阈值、usage 与生命周期估算，不再驱动无语义 head/tail 裁剪 |
| `ContextWindow.UpdateUsage()` | ✅ 活跃 | 每轮 API 响应后更新，使用 API 返回的真实 token 数 |

---

## 六、与原版的差距及后续规划

### 6.1 已实现 vs 未实现

| 能力 | 原版 | Go 现状 | 差距评估 |
|------|------|---------|---------|
| 大工具输出持久化 | >50K → 存磁盘 + 2KB预览 | ✅ 已实现，逻辑一致 | 无 |
| 单结果截断 | 无独立层（由持久化覆盖） | ✅ ToolResultBudget >30K 截断 | Go 多了一层防御 |
| 旧结果清理 | 时间触发 + cache_edits | ✅ 简化版（保留最近10个） | 缺少时间触发和 cache 感知 |
| API级管理 | server-side 清理 | ❌ 未实现 | Anthropic 私有协议 |
| 会话记忆 | 后台异步提取 + 无LLM压缩 | ❌ 未实现 | 需要 forked agent 架构 |
| LLM 摘要 | 9节结构化 + 文件恢复 + skill恢复 | ✅ 已实现（6节，无文件恢复） | 缺少压缩后文件/skill 重挂载 |
| PTL 重试 | 按轮次分组砍头重试 | ❌ 未实现 | 当前显式失败关闭，不以静默裁剪部分模拟 |
| Prompt cache 感知 | 压缩保持 cache 前缀稳定 | ❌ 未实现 | 每次压缩废弃 cache |

### 6.2 后续优化方向（按优先级排序）

**P1 — 压缩后文件恢复（中等工作量）：**
- LLM 摘要完成后，重新读取最近 5 个文件（每个 5K tokens），追加到摘要后
- 防止压缩后模型丢失文件上下文
- 实现位置：`compact/summarize.go` 或新文件 `compact/post_compact.go`

**P2 — 接入 CalibratedCounter（低工作量）：**
- 每轮 API 响应后，用实际 usage 数据校准 token 估算比率
- 提高压缩阈值和 `EstimateMessages` 的精度
- 实现位置：`loop/query.go` 的 `UpdateUsage` 附近

**P3 — 时间触发微压缩（低工作量）：**
- 闲置 > 60 分钟时，清除除最近 5 个外的所有工具结果
- 实现位置：`compact/microcompact.go` 添加时间判断

**P4 — 会话记忆系统（高工作量）：**
- 后台 agent 异步提取关键信息到 `session_memory.md`
- 基于记忆的无 LLM 快速压缩
- 需要 forked agent 架构支持

**P5 — PTL 重试（中等工作量）：**
- 压缩请求本身超限时，按 API 轮次截断头部重试
- 连续失败 3 次触发熔断器
