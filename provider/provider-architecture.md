# Provider 模块架构文档

> 本文档是 `gosrc/provider/` 模块的设计参考，对照原版 TypeScript 实现说明 Go 复现的现状、差距与后续规划。
> 生成日期：2026-04-05

---

## 一、概述

`provider` 模块是整个 Go 版 Claude Code 的 **LLM 通信层**，承担以下核心职责：

| 职责 | 说明 |
|------|------|
| 统一接口抽象 | 通过 `Provider` 接口屏蔽底层 API 差异，上层逻辑无需关心具体 LLM 服务商 |
| 流式事件转换 | 将各服务商的 SSE 原始帧统一转换为 `types.StreamEvent` 协议事件 |
| 多服务商支持 | 支持 Anthropic 官方 API 及所有 OpenAI 兼容 API（OpenAI、Ollama、DeepSeek 等） |
| Prompt 缓存 | 在 Anthropic 请求上自动注入缓存断点（system / tools / messages 末尾） |
| 环境变量路由 | `NewFromEnv()` 工厂函数根据 `PROVIDER` 等环境变量自动选择并配置 Provider |
| 上下文取消 | 所有流式 goroutine 均尊重 `context.Context` 取消信号 |

### 模块文件清单

```
gosrc/provider/
├── provider.go          # Provider 接口 + Params/Config 结构体定义
├── anthropic.go         # Anthropic 官方 SDK 封装
├── openai.go            # OpenAI 兼容 API 封装（含 Ollama/DeepSeek）
├── env.go               # 环境变量工厂函数
├── provider_test.go     # 单元测试（682 行）
└── scripts/
    └── provider_metrics.py  # Go vs TS 覆盖率评估脚本
```

---

## 二、原版 TypeScript 设计详情

### 2.1 客户端工厂：`src/services/api/client.ts`

原版通过 `getAnthropicClient()` 工厂函数创建 Anthropic SDK 客户端实例，支持四种部署形态：

| 部署形态 | 客户端类 | 认证方式 |
|----------|----------|----------|
| Direct API（默认） | `Anthropic` | `ANTHROPIC_API_KEY` |
| AWS Bedrock | `AnthropicBedrock` | AWS 凭证 + 自动刷新 |
| Azure AI Foundry | `AnthropicFoundry` | Azure AD / Bearer token |
| Google Vertex AI | `AnthropicVertex` | GoogleAuth OAuth 令牌 |

**额外能力（Go 未实现）**：
- `ANTHROPIC_CUSTOM_HEADERS` 环境变量：解析 curl 风格的自定义请求头（`HeaderName: Value` 格式，换行分隔）
- `x-client-request-id` 注入：每次请求附加唯一 ID 用于超时追踪
- Proxy fetch 支持（代理环境下的 `fetchOptions`）
- Session ID header 传递（`x-session-id`）

### 2.2 主调用逻辑：`src/services/api/claude.ts`

`claude.ts` 中的 `runClaudeStreaming()` 构建请求参数，支持以下字段（Go 部分缺失）：

| 参数 | TS 支持 | Go 支持 | 说明 |
|------|---------|---------|------|
| `model` | ✅ | ✅ | 模型 ID |
| `max_tokens` | ✅ | ✅ | 最大 token 数 |
| `system` | ✅ | ✅ | 系统提示词 |
| `messages` | ✅ | ✅ | 对话历史 |
| `tools` | ✅ | ✅ | 工具定义 |
| `thinking` | ✅ | ❌ | Extended Thinking 配置（type/budget_tokens） |
| `effort` | ✅ | ❌ | 推理努力程度（`low`/`medium`/`high`） |
| `tool_choice` | ✅ | ❌ | 工具选择策略（auto/any/tool） |
| `betas` | ✅ | ❌ | Beta 功能头（见下表） |
| `stream` | ✅ | ✅ | 强制流式响应 |

**Beta 功能头（Go 全部未实现）**：

```typescript
AFK_MODE_BETA_HEADER           // 无人值守模式
CONTEXT_1M_BETA_HEADER         // 100万 token 上下文
EFFORT_BETA_HEADER             // 推理努力控制
FAST_MODE_BETA_HEADER          // 快速模式（降低延迟）
PROMPT_CACHING_SCOPE_BETA_HEADER  // 缓存范围控制
REDACT_THINKING_BETA_HEADER    // 隐藏思维过程
STRUCTURED_OUTPUTS_BETA_HEADER // 结构化输出
TASK_BUDGETS_BETA_HEADER       // 任务 token 预算
```

### 2.3 重试机制：`src/services/api/withRetry.ts`

原版实现了完整的重试/退避逻辑，Go 版本**完全未实现**：

```
重试参数：
  DEFAULT_MAX_RETRIES = 10
  MAX_529_RETRIES     = 3
  BASE_DELAY_MS       = 500 ms
  退避公式：500 * 2^(attempt-1) + 25% jitter（最大 ~8 分钟）

可重试状态码：
  429 (Rate Limit)  → 指数退避，读取 Retry-After 响应头
  529 (Overloaded)  → 前台来源重试，后台来源立即丢弃
  5xx              → 通用服务器错误退避

特殊处理：
  Fast Mode: Retry-After 短 → 启用 fast_mode 重试；长 → 进入冷却期
  Persistent Retry: CLAUDE_CODE_UNATTENDED_RETRY=1 时 429/529 无限重试，每 30s yield heartbeat
  OAuth 401    → handleOAuth401Error() 刷新令牌
  Bedrock 403  → 清空 AWS 凭证缓存
  Vertex 401   → 清空 GCP 凭证缓存
  ECONNRESET   → 禁用 keep-alive 重连
  400 + context overflow → 调整 max_tokens 后重试
```

### 2.4 Prompt 缓存断点检测：`src/services/api/promptCacheBreakDetection.ts`

原版实现两阶段缓存断点检测，Go 版本**完全未实现**：

```
阶段一（请求前）：
  计算 hash(system + tools + model + betas + fastMode + effort + extraBody)
  存储到 per-source 状态映射中

阶段二（响应后）：
  比较 cache_read_input_tokens；
  若读取量下降 >5% 且绝对值 >2000 tokens → 缓存断点触发
  触发 analytics 事件：tengu_prompt_cache_break
  写入调试 diff 文件
  分析原因：模型变更 / 系统提示变更 / 工具变更 / TTL 过期 / 服务端清除
```

### 2.5 内容块类型支持

| 块类型 | TS 支持 | Go 支持 | 说明 |
|--------|---------|---------|------|
| `text` | ✅ | ✅ | 纯文本 |
| `tool_use` | ✅ | ✅ | 工具调用 |
| `tool_result` | ✅ | ✅ | 工具结果 |
| `thinking` | ✅ | ✅ | Extended Thinking（Anthropic 流） |
| `redacted_thinking` | ✅ | ❌ | 被隐藏的思维内容 |
| `image` | ✅ | ❌ | 图片输入 |
| `document` | ✅ | ❌ | 文档输入（PDF 等） |

---

## 三、Go 实现现状

### 3.1 文件级状态表

| 文件 | 行数 | 状态 | 核心功能 |
|------|------|------|----------|
| `provider.go` | 39 | ✅ 完整 | Provider 接口、Params、Config 定义 |
| `anthropic.go` | 304 | ✅ 核心完整 | Anthropic SDK 封装、SSE 流、缓存断点 |
| `openai.go` | 422 | ✅ 核心完整 | OpenAI 兼容封装、事件合成、工具转换 |
| `env.go` | 88 | ✅ 完整 | 环境变量路由工厂 |
| `provider_test.go` | 682 | ✅ 完整 | 单元测试全覆盖 |

### 3.2 功能覆盖率状态表

| 功能域 | 功能点 | Go 实现状态 | 备注 |
|--------|--------|------------|------|
| **Provider 抽象** | Provider 接口 | ✅ 已实现 | |
| | Params/Config 结构体 | ✅ 已实现 | |
| | ModelID() 方法 | ✅ 已实现 | |
| **Anthropic 提供商** | Direct API 连接 | ✅ 已实现 | |
| | SSE 流式接收 | ✅ 已实现 | 6 种事件类型全覆盖 |
| | Extended Thinking | ✅ 已实现 | thinking/thinking_delta |
| | 工具调用流 | ✅ 已实现 | InputJSONDelta |
| | System 缓存断点 | ✅ 已实现 | CacheControlEphemeral |
| | Tools 缓存断点 | ✅ 已实现 | 末尾 tool 注入 |
| | Messages 缓存断点 | ✅ 已实现 | 末尾消息末尾块注入 |
| | AWS Bedrock | ❌ 未实现 | |
| | Google Vertex AI | ❌ 未实现 | |
| | Azure Foundry | ❌ 未实现 | |
| | Thinking Config | ❌ 未实现 | budget_tokens, type |
| | Effort 参数 | ❌ 未实现 | |
| | Beta 功能头 | ❌ 未实现 | 8 个 beta header |
| | Tool Choice | ❌ 未实现 | auto/any/tool |
| | Structured Outputs | ❌ 未实现 | |
| | OAuth 支持 | ❌ 未实现 | |
| | 图片块 | ❌ 未实现 | |
| | 文档块 | ❌ 未实现 | |
| | RedactedThinking 块 | ❌ 未实现 | |
| **OpenAI 兼容层** | OpenAI Direct | ✅ 已实现 | |
| | Ollama 本地 | ✅ 已实现 | noAuthTransport |
| | DeepSeek | ✅ 已实现 | 硬编码 base URL |
| | 自定义 BaseURL | ✅ 已实现 | |
| | 自定义 Headers | ✅ 已实现 | headerTransport |
| | 超时控制 | ✅ 已实现 | 默认 600s |
| | 事件合成 | ✅ 已实现 | OpenAI→Anthropic 协议 |
| | IncludeUsage | ✅ 已实现 | 末尾 usage chunk |
| | 工具调用流 | ✅ 已实现 | per-index 跟踪 |
| | Thinking 块透传 | ❌ 不适用 | OpenAI 不支持，已静默丢弃 |
| | Azure OpenAI | ⚠️ 部分支持 | 可通过 BaseURL+Headers 配置，无原生支持 |
| | vLLM/LiteLLM | ⚠️ 兼容支持 | 可通过 BaseURL 配置 |
| **错误处理** | 流错误事件 | ✅ 已实现 | EventError |
| | APIError 映射 | ✅ 已实现 | openai.APIError |
| | 上下文取消 | ✅ 已实现 | ctx.Done() |
| | 重试机制 | ❌ 未实现 | TS 有 10 次重试 |
| | 指数退避 | ❌ 未实现 | |
| | 429/529 区分处理 | ❌ 未实现 | |
| **缓存管理** | 请求侧断点注入 | ✅ 已实现 | |
| | 响应侧断点检测 | ❌ 未实现 | TS 有两阶段检测 |
| | Analytics 事件 | ❌ 未实现 | |
| **工厂/路由** | NewFromEnv | ✅ 已实现 | 4 种 provider |
| | NewFromEnvWithOverrides | ✅ 已实现 | |
| | 环境变量文档 | ✅ 已实现 | |
| | 模型回退 | ❌ 未实现 | 529 时降级模型 |

### 3.3 数据流图

#### 3.3.1 Anthropic Provider 数据流

```
调用方 (loop.go / agent.go)
        │
        │  CreateStream(ctx, Params{System, Messages, Tools, ...})
        ▼
AnthropicProvider.CreateStream()
        │
        ├─ convertToAnthropicMessages(params.Messages)
        │       └─ 对最后一条消息的最后一个兼容块注入 CacheControlEphemeral
        │
        ├─ convertToAnthropicTools(params.Tools)
        │       └─ 对最后一个 tool 注入 CacheControlEphemeral
        │
        ├─ System prompt → TextBlockParam + CacheControlEphemeral
        │
        └─ client.Messages.NewStreaming(ctx, reqParams)
                │
                │  [goroutine]
                ▼
        processAnthropicStream(ctx, stream, ch)
                │
                ├─ MessageStartEvent     → EventMessageStart + Usage
                ├─ ContentBlockStartEvent
                │       ├─ TextBlock     → EventContentBlockStart{type:text}
                │       ├─ ToolUseBlock  → EventContentBlockStart{type:tool_use, id, name}
                │       └─ ThinkingBlock → EventContentBlockStart{type:thinking, signature}
                ├─ ContentBlockDeltaEvent
                │       ├─ TextDelta        → EventContentBlockDelta{text_delta}
                │       ├─ InputJSONDelta   → EventContentBlockDelta{input_json_delta}
                │       └─ ThinkingDelta    → EventContentBlockDelta{thinking_delta}
                ├─ ContentBlockStopEvent → EventContentBlockStop
                ├─ MessageDeltaEvent    → EventMessageDelta{StopReason, Usage}
                └─ MessageStopEvent     → EventMessageStop
                         │
                         ▼
               <-chan types.StreamEvent (buffered 64)
                         │
                         ▼
                   调用方消费事件
```

#### 3.3.2 OpenAI Provider 数据流

```
调用方
        │
        │  CreateStream(ctx, Params)
        ▼
OpenAIProvider.CreateStream()
        │
        ├─ convertMessagesToOpenAI(params)
        │       ├─ System → ChatMessageRoleSystem
        │       ├─ User messages → ChatMessageRoleUser
        │       │       └─ ToolResultBlock → ChatMessageRoleTool (单独消息)
        │       └─ Assistant messages → ChatMessageRoleAssistant + ToolCalls[]
        │
        ├─ convertToolsToOpenAI(params.Tools)
        │       └─ 确保 object schema 有 properties 字段
        │
        └─ client.CreateChatCompletionStream(ctx, req)
                │
                │  [goroutine]
                ▼
        processStream(ctx, stream, ch)
                │
                ├─ 合成 EventMessageStart
                │
                ├─ delta.Content != ""
                │       ├─ 首次 → 合成 EventContentBlockStart{text, index:0}
                │       └─ EventContentBlockDelta{text_delta, index:0}
                │
                ├─ delta.ToolCalls[]
                │       ├─ 首次(idx) → 关闭 text block → EventContentBlockStart{tool_use, index:idx+1}
                │       └─ EventContentBlockDelta{input_json_delta, index:idx+1}
                │
                ├─ resp.Usage != nil → EventMessageDelta{Usage}（含 CachedTokens）
                │
                └─ choice.FinishReason != ""
                        ├─ 关闭所有 open blocks → EventContentBlockStop
                        ├─ 映射 StopReason（tool_calls/length/其他）
                        └─ EventMessageStop
                                 │
                                 ▼
                       <-chan types.StreamEvent
```

#### 3.3.3 环境变量路由流

```
NewFromEnv() / NewFromEnvWithOverrides(provider, model)
        │
        ├─ PROVIDER="anthropic" (默认)
        │       └─ ANTHROPIC_API_KEY 必须非空
        │               └─ NewAnthropic(Config{...})
        │
        ├─ PROVIDER="openai"
        │       └─ OPENAI_API_KEY 必须非空
        │               └─ NewOpenAI(Config{BaseURL: "https://api.openai.com/v1"})
        │
        ├─ PROVIDER="ollama"
        │       └─ 无需 API Key（noAuthTransport 自动剥离 Authorization）
        │               └─ NewOpenAI(Config{BaseURL: "http://localhost:11434/v1", Model: "llama3.1"})
        │
        └─ PROVIDER="deepseek"
                └─ DEEPSEEK_API_KEY 必须非空
                        └─ NewOpenAI(Config{BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-v4-flash"})
```

### 3.4 Anthropic vs OpenAI Provider 对比

| 维度 | AnthropicProvider | OpenAIProvider |
|------|-------------------|----------------|
| 底层 SDK | `anthropic-sdk-go` | `go-openai` |
| 默认模型 | `claude-sonnet-4-20250514` | `gpt-4o` |
| 默认 MaxTokens | 16384 | 16384 |
| 默认超时 | SDK 默认（~10分钟） | 600s |
| 流式接收 | 原生 SSE（SDK 封装） | `CreateChatCompletionStream` |
| 事件协议 | 原生 Anthropic 事件 | 合成 Anthropic 协议事件 |
| Extended Thinking | ✅ 原生支持 | ❌ 静默丢弃 |
| 工具参数流 | `InputJSONDelta` | `Function.Arguments` 分片 |
| 缓存断点注入 | ✅ 三处（system/tools/messages） | ❌ OpenAI 不支持 |
| 自动缓存（响应端） | Anthropic 服务端自动 | OpenAI 自动缓存（只读，不可控） |
| CachedTokens 读取 | ✅ `CacheReadInputTokens` | ✅ `PromptTokensDetails.CachedTokens` |
| 自定义认证 | 无额外配置 | `noAuthTransport`（Ollama） |
| 自定义请求头 | N/A | `headerTransport` |

### 3.5 API 参数映射表（Go ↔ TS ↔ 原始 API）

#### Params → Anthropic API

| Go Params 字段 | TS claude.ts 字段 | Anthropic API 字段 | 映射状态 |
|----------------|-------------------|--------------------|----------|
| `Model` | `model` | `model` | ✅ 直接映射 |
| `MaxTokens` | `max_tokens` | `max_tokens` | ✅ 直接映射 |
| `System` | `system` | `system[]` (TextBlockParam+cache) | ✅ 含缓存断点 |
| `Messages` | `messages` | `messages[]` | ✅ 含缓存断点 |
| `Tools` | `tools` | `tools[]` | ✅ 含缓存断点 |
| `—` | `thinking` | `thinking{type, budget_tokens}` | ❌ 未映射 |
| `—` | `effort` | `effort` | ❌ 未映射 |
| `—` | `tool_choice` | `tool_choice` | ❌ 未映射 |
| `—` | `betas` | `anthropic-beta` header | ❌ 未映射 |
| `—` | `stream_options` | N/A（Anthropic 原生流） | N/A |

#### Params → OpenAI API

| Go Params 字段 | OpenAI API 字段 | 映射状态 |
|----------------|-----------------|----------|
| `Model` | `model` | ✅ 直接映射 |
| `MaxTokens` | `max_completion_tokens` | ✅ 直接映射 |
| `System` | `messages[0].role=system` | ✅ 转换 |
| `Messages` | `messages[]` | ✅ 转换 |
| `Tools` | `tools[]` | ✅ 转换 |
| `—` | `stream_options.include_usage` | ✅ 固定 true |
| `—` | `tool_choice` | ❌ 未映射 |
| `—` | `response_format` | ❌ 未映射 |
| `—` | `temperature` | ❌ 未映射 |

---

## 四、关键知识背景

### 4.1 Anthropic Messages API（SSE 流协议）

Anthropic 流式响应遵循严格的事件序列：

```
message_start          ← 包含 usage.input_tokens 等初始统计
  content_block_start  ← index=N, type=text|tool_use|thinking
    content_block_delta (重复)  ← text_delta / input_json_delta / thinking_delta
  content_block_stop
  ...（多个内容块）
message_delta          ← stop_reason + usage.output_tokens
message_stop           ← 流结束标志
```

**关键约束**：
- 内容块是有序的，index 从 0 开始单调递增
- 同一 index 的所有 delta 必须在 `content_block_stop` 前到达
- `message_delta` 中的 `stop_reason` 取值：`end_turn` / `tool_use` / `max_tokens` / `stop_sequence`

### 4.2 OpenAI Chat Completions SSE

OpenAI 流式协议与 Anthropic 差异显著：

```
data: {"choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}
data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"x","function":{"name":"f","arguments":""}}]},"finish_reason":null}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"k\":"}}]},"finish_reason":null}]}
data: {"choices":[{"finish_reason":"tool_calls"}], "usage":{...}}  ← 仅当 stream_options.include_usage=true
data: [DONE]
```

**OpenAI→Anthropic 协议映射关键点**：
- `delta.content` → `text_delta`（index 固定为 0）
- `delta.tool_calls[i]` → `input_json_delta`（index = i+1，留 0 给可能的 text）
- `finish_reason:"tool_calls"` → `StopReasonToolUse`
- `finish_reason:"length"` → `StopReasonMaxTokens`
- 必须手动合成 `message_start` / `content_block_start` / `content_block_stop` 事件

### 4.3 Prompt 缓存机制

Anthropic 的 Prompt 缓存通过 `cache_control: {type: "ephemeral"}` 标注启用：

- **生存期**：5 分钟（TTL）
- **最小缓存单元**：2048 tokens（某些模型）
- **可缓存位置**：`system[]` 内容块、`tools[]` 列表末尾、`messages[]` 中的内容块
- **缓存命中**：响应头中 `cache_read_input_tokens > 0`
- **缓存创建**：`cache_creation_input_tokens > 0`

Go 实现策略：**总是在最后一个兼容位置注入断点**，让 Anthropic 服务端决定是否实际缓存。

### 4.4 Extended Thinking（扩展思维）

Extended Thinking 让模型在生成最终答案前进行隐式推理：

```
content_block_start  {type:"thinking", signature:"sig_xxx"}
  content_block_delta {type:"thinking_delta", thinking:"...推理过程..."}
content_block_stop
content_block_start  {type:"text"}
  content_block_delta {type:"text_delta", text:"...答案..."}
content_block_stop
```

**重要**：`signature` 字段是服务端验证机制，在多轮对话中必须将 `ThinkingBlock`（含 signature）原样回传。Go 的 `convertToAnthropicMessages` 正确处理了这一点（通过 `anthropic.NewThinkingBlock(signature, thinking)`）。

### 4.5 Go HTTP Transport 链

Go 实现通过组合 `http.RoundTripper` 实现请求拦截：

```
请求 → headerTransport.RoundTrip()
           └─ noAuthTransport.RoundTrip()  (如果启用)
                   └─ http.DefaultTransport.RoundTrip()
                           └─ 实际网络请求
```

这种链式设计与 TS 中的 `fetchOptions` 中间件模式等价，但 Go 的实现更轻量、零依赖。

---

## 五、评估指标

以下指标均可通过 `scripts/provider_metrics.py` 自动计算。

### 5.1 Provider 功能覆盖率

**计算方法**：已实现功能点 / 总功能点（参见 3.2 节状态表）

| 功能域 | 总功能点 | 已实现 | 覆盖率 |
|--------|---------|--------|--------|
| Provider 抽象 | 3 | 3 | 100% |
| Anthropic 核心功能 | 8 | 8 | 100% |
| Anthropic 高级功能 | 9 | 0 | 0% |
| OpenAI 兼容层 | 11 | 9 | 82% |
| 错误处理 | 6 | 3 | 50% |
| 缓存管理 | 4 | 1 | 25% |
| 工厂/路由 | 5 | 4 | 80% |
| **合计** | **46** | **28** | **60.9%** |

### 5.2 流式事件正确性

**Anthropic Provider** — 事件类型全覆盖：

| 事件类型 | 实现状态 | 验证方式 |
|----------|---------|----------|
| `message_start` | ✅ | `TestConvertToAnthropicMessages_RoundTrip` |
| `content_block_start` (text) | ✅ | `TestProcessStream_TextOnly` |
| `content_block_start` (tool_use) | ✅ | `TestProcessStream_ToolUse` |
| `content_block_start` (thinking) | ✅ | `TestProcessStream_ThinkingBlock` |
| `content_block_delta` (text_delta) | ✅ | `TestProcessStream_TextOnly` |
| `content_block_delta` (input_json_delta) | ✅ | `TestProcessStream_ToolUse` |
| `content_block_delta` (thinking_delta) | ✅ | `TestProcessStream_ThinkingBlock` |
| `content_block_stop` | ✅ | 多个测试 |
| `message_delta` (stop_reason) | ✅ | 流结束处理 |
| `message_delta` (usage) | ✅ | `TestProcessStream_TextOnly` |
| `message_stop` | ✅ | 所有流测试 |
| `error` | ✅ | `TestProcessStream_ErrorEvent` |

**OpenAI→Anthropic 事件合成正确性**：

| 合成场景 | 实现状态 | 潜在问题 |
|----------|---------|----------|
| 纯文本流 | ✅ | |
| 工具调用流 | ✅ | |
| 文本+工具混合 | ✅ | index 偏移逻辑正确 |
| 多工具并发 | ✅ | per-index map 跟踪 |
| Usage chunk 处理 | ✅ | `IncludeUsage: true` |
| FinishReason 映射 | ✅ | 3 种 case 全覆盖 |
| 空 FinishReason 跳过 | ✅ | `"null"` 字符串处理 |

### 5.3 错误处理覆盖率

| 错误场景 | TS 处理 | Go 处理 | 覆盖状态 |
|----------|---------|---------|----------|
| `io.EOF` 正常结束 | N/A | ✅ break | ✅ |
| OpenAI APIError | 映射到类型 | ✅ `openai.APIError` | ✅ |
| 通用流错误 | 重试 | ✅ `EventError` 事件 | ⚠️ 无重试 |
| 上下文取消 | N/A | ✅ `ctx.Done()` | ✅ |
| 429 Rate Limit | ✅ 重试退避 | ❌ 直接报错 | ❌ |
| 529 Overloaded | ✅ 重试退避 | ❌ 直接报错 | ❌ |
| 5xx 服务器错误 | ✅ 重试 | ❌ 直接报错 | ❌ |
| OAuth 401 | ✅ 令牌刷新 | ❌ 不支持 OAuth | ❌ |
| 网络 ECONNRESET | ✅ 禁用 keep-alive | ❌ 未处理 | ❌ |
| 400 + context overflow | ✅ 调整 max_tokens | ❌ 未处理 | ❌ |
| Anthropic SDK 流错误 | N/A | ✅ `stream.Err()` | ✅ |

错误处理覆盖率：**5/11 = 45%**

### 5.4 缓存断点放置正确性

| 断点位置 | TS 行为 | Go 行为 | 正确性 |
|----------|---------|---------|--------|
| System prompt 末尾 | 注入 ephemeral | ✅ 注入 ephemeral | ✅ |
| Tools 列表末尾 | 注入 ephemeral | ✅ 注入 ephemeral | ✅ |
| Messages 最后一条末尾块 | 注入 ephemeral | ✅ 向后查找兼容块 | ✅ |
| ThinkingBlock 跳过 | 不支持 cache_control | ✅ 向后跳过 | ✅ |
| 无 messages 时 | 不注入 | ✅ `len(result) > 0` 保护 | ✅ |

缓存断点正确率：**5/5 = 100%**

### 5.5 Token 计数准确性

| Token 计数字段 | Anthropic 来源 | Go 映射 | 准确性 |
|----------------|----------------|---------|--------|
| `InputTokens` | `message.usage.input_tokens` | ✅ 直接映射 | ✅ |
| `OutputTokens` | `message_delta.usage.output_tokens` | ✅ 直接映射 | ✅ |
| `CacheCreationInputTokens` | `message.usage.cache_creation_input_tokens` | ✅ 直接映射 | ✅ |
| `CacheReadInputTokens` | `message.usage.cache_read_input_tokens` | ✅ 直接映射 | ✅ |
| OpenAI CachedTokens | `usage.prompt_tokens_details.cached_tokens` | ✅ `CacheReadInputTokens` | ✅ |
| OpenAI CacheCreation | 不区分 | ⚠️ 无法区分（注释说明） | ⚠️ |

---

## 六、与原版差距及后续规划

### 6.1 差距汇总

```
优先级 P0（核心功能，影响生产稳定性）
────────────────────────────────────────
❌ 重试机制（withRetry.ts）
   影响：任何 429/529/5xx 错误直接失败，生产环境无法长时间稳定运行
   规划：实现指数退避 + jitter，支持 Retry-After 响应头解析
   参考：DEFAULT_MAX_RETRIES=10, BASE_DELAY_MS=500

优先级 P1（高价值功能，扩大适用场景）
────────────────────────────────────────
❌ AWS Bedrock 支持
   影响：无法在 AWS 生产环境使用 Claude
   规划：集成 AnthropicBedrock SDK，支持 AWS 凭证刷新

❌ Google Vertex AI 支持
   影响：无法在 GCP 生产环境使用 Claude
   规划：集成 AnthropicVertex SDK，支持 GoogleAuth OAuth

❌ Tool Choice 参数
   影响：无法强制指定工具调用策略
   规划：在 Params 中添加 ToolChoice 字段，映射到 API 参数

优先级 P2（功能完善，提升体验）
────────────────────────────────────────
❌ Extended Thinking 配置（thinking config + effort）
   影响：无法控制思考模式和推理预算
   规划：在 Params 中添加 Thinking/Effort 字段

❌ Beta 功能头支持
   影响：无法使用 1M context、structured outputs 等 beta 功能
   规划：在 Params 或 Config 中添加 BetaHeaders []string

❌ 图片/文档输入块
   影响：无法处理多模态输入
   规划：在 types.ContentBlock 中添加 ImageBlock/DocumentBlock

优先级 P3（观测性，提升可维护性）
────────────────────────────────────────
❌ Prompt 缓存断点检测（promptCacheBreakDetection.ts）
   影响：无法检测缓存失效原因，难以调试缓存问题
   规划：两阶段 hash 检测，输出调试信息

❌ 请求 ID 注入（x-client-request-id）
   影响：无法关联超时日志与具体请求
   规划：在 headerTransport 中自动注入 UUID

❌ Azure Foundry 原生支持
   影响：Azure 部署需手动配置 BaseURL+Headers
   规划：在 env.go 中添加 "azure" provider 路由
```

### 6.2 实施路线图

```
阶段一（稳定性）     目标：生产可用
├─ 实现重试机制（P0）
│   ├─ 基础指数退避（500ms * 2^n + jitter）
│   ├─ 429/529 状态码处理
│   └─ 可配置最大重试次数（环境变量）
└─ 基础错误分类（network/auth/rate-limit/server）

阶段二（功能扩展）   目标：多云支持
├─ AWS Bedrock Provider（P1）
├─ Google Vertex AI Provider（P1）
├─ Tool Choice 参数（P1）
└─ Azure Foundry Provider（P3）

阶段三（高级特性）   目标：对齐原版
├─ Extended Thinking Config（P2）
├─ Beta Headers 支持（P2）
├─ 图片/文档块支持（P2）
└─ Structured Outputs（P2）

阶段四（观测性）     目标：生产可观测
├─ Prompt 缓存断点检测（P3）
├─ 请求 ID 注入（P3）
└─ Metrics/Tracing 接入点
```

### 6.3 架构扩展建议

当实现重试机制时，建议在 `Provider` 接口层之上增加装饰器模式，而非修改各个 Provider 实现：

```go
// 建议新增：RetryProvider 装饰器
type RetryProvider struct {
    inner      Provider
    maxRetries int
    baseDelay  time.Duration
}

func (r *RetryProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
    for attempt := 0; attempt <= r.maxRetries; attempt++ {
        ch, err := r.inner.CreateStream(ctx, params)
        if err == nil {
            return ch, nil
        }
        if !isRetryable(err) || attempt == r.maxRetries {
            return nil, err
        }
        delay := r.baseDelay * (1 << attempt)
        delay = addJitter(delay, 0.25)
        select {
        case <-time.After(delay):
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
    panic("unreachable")
}

// 使用方式
p := NewRetryProvider(NewAnthropic(cfg), RetryConfig{MaxRetries: 10})
```

这与 TS 中 `withRetry.ts` 包装 API 调用的设计思路一致，且不破坏现有 Provider 接口契约。

---

*文档版本：v1.0 | 对应源码版本：参见 `gosrc/go.mod` | 评估脚本：`scripts/provider_metrics.py`*
