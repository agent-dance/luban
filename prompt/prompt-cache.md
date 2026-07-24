# Prompt Cache 架构 — 设计参考文档

> 基于 Claude Code TypeScript 原版（`../src`）提炼，定义 Go 实现的目标架构。
> 包含原版设计分析 + Go 当前实现详情 + 差距分析 + 修复方案。

---

## 一、概述

Prompt Cache 是 Anthropic API 的核心优化特性：通过在请求中标记 `cache_control: {type: "ephemeral"}` 断点，服务端会缓存断点前的所有内容（系统提示 + 工具定义 + 消息前缀）。后续请求只要前缀不变，就可以直接复用缓存，**节省 90% 以上的 input token 计费**。

原版实现了一套 **10步缓存流水线**，确保：
- 缓存命中率最大化
- 缓存前缀跨轮次稳定
- 压缩操作不破坏缓存
- 缓存中断可检测可归因

---

## 二、原版缓存流水线（10步）

### 步骤1：系统提示构建 — 静态/动态分界

**文件：** `src/constants/prompts.ts`、`src/utils/api.ts`

系统提示被一个边界标记 `__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__` 分为两部分：
- **静态内容**（边界前）：介绍、系统信息、任务指引、工具使用、语气风格 → **跨组织共享缓存**（`scope: 'global'`）
- **动态内容**（边界后）：会话特定指引、记忆、环境信息、MCP 指令 → **不缓存**

`splitSysPromptPrefix()` 将系统提示拆分为 `SystemPromptBlock[]`，每个块标注 `cacheScope`：
- `'global'`：跨组织共享（静态内容）
- `'org'`：组织级缓存（有 MCP 工具时）
- `null`：不缓存

### 步骤2：系统提示缓存标记

**文件：** `src/services/api/claude.ts` → `buildSystemPromptBlocks()`

根据 `cacheScope` 为系统提示块附加 `cache_control`：
```
{ type: 'ephemeral', ttl?: '1h', scope?: 'global' }
```
- 1小时 TTL：仅限付费用户（非超额使用）或 Anthropic 内部
- 全局作用域：仅限第一方 API

### 步骤3：工具定义缓存标记

**文件：** `src/utils/api.ts` → `toolToAPISchema`

- 工具 schema 在会话内一次计算并缓存，防止中途漂移
- **最后一个工具定义**附加 `cache_control: ephemeral`
- 确保 `系统提示 + 工具定义` 前缀被整体缓存

### 步骤4：消息标准化

**文件：** `src/utils/messages.ts` → `normalizeMessagesForAPI()`

- 合并连续同角色消息（提高缓存前缀稳定性）
- 过滤进度消息、系统消息、虚拟消息
- 标准化工具输入格式
- 保证跨轮次的消息序列化一致性

### 步骤5：工具输出替换稳定性

**文件：** `src/utils/toolResultStorage.ts` → `ContentReplacementState`

核心不变量：**一旦决定了某个 tool result 的命运（替换或保留），该决定永久冻结。**

三路分类：
- `mustReapply`：之前已替换 → 重新应用**完全相同的替换字符串**（字节级一致）
- `frozen`：之前已评估但未替换 → 永远不再替换
- `fresh`：首次评估 → 可做新决策

**意义：** 替换字符串不是重新生成的，而是从缓存中取出的原始字节。模板变更、路径变更不会悄悄破坏缓存。

### 步骤6：微压缩（缓存感知路径）

**文件：** `src/services/compact/microCompact.ts`

两条路径：
- **时间触发**：缓存已过期（>60分钟）→ 直接修改消息内容（安全）
- **缓存编辑**（`cache_edits`）：缓存未过期 → 不修改本地消息，而是构建 `cache_edits` 指令 + `cache_reference` 标记，让**服务端**在缓存层面执行删除

### 步骤7：缓存断点放置

**文件：** `src/services/api/claude.ts` → `addCacheBreakpoints()`

**策略：每个请求在消息数组上恰好放置一个 `cache_control` 标记。**

位置：倒数第1条消息（正常请求）或倒数第2条（fork 请求）。

**为什么只放一个？** Anthropic 的 Mycro KV-cache 内部机制：两个标记会导致倒数第二个位置的 local-attention KV 页被"保护"而无法释放。单标记让这些页在下一轮立即释放。

### 步骤8：会话稳定的请求参数

**文件：** `src/services/api/claude.ts`

关键的"锁存"（latch）机制：
- **Beta header 锁存**：`AFK_MODE`、`FAST_MODE`、`CACHE_EDITING` 一旦首次发送，整个会话保持不变。翻转 beta header 会导致 ~50-70K tokens 缓存失效。
- **1小时 TTL 资格锁存**：用户资格和配置在首次评估后冻结，防止中途超额状态变更翻转 cache TTL。
- **工具 schema 缓存**：每会话计算一次，不随 GrowthBook 配置漂移。

### 步骤9：缓存中断检测

**文件：** `src/services/api/promptCacheBreakDetection.ts`

两阶段检测：
- **调用前**（`recordPromptState`）：快照影响缓存键的所有因素——系统提示哈希、工具 schema 哈希、模型、beta headers 等
- **调用后**（`checkResponseForCacheBreak`）：比较 `cache_read_input_tokens`，若下降 >5% 且绝对值 >2K → 触发 `tengu_prompt_cache_break` 事件，并归因到具体原因（系统提示变更、工具增删、模型切换、TTL 过期等）

### 步骤10：压缩时缓存共享

**文件：** `src/services/compact/compact.ts`

压缩使用 `runForkedAgent` 与主会话共享缓存前缀：
- Fork 发送相同的系统提示、工具、模型和消息前缀
- 避免压缩时产生 ~50K+ tokens 的 `cache_creation` 费用
- 压缩完成后调用 `notifyCompaction()` 重置缓存基线

---

## 三、Go 当前实现状态

### 3.1 状态总览：**主动缓存已接入，仍是简化版**

Go 实现现在已经主动请求 Anthropic prompt cache，并能读取/显示多 provider 缓存指标：

- Anthropic：系统 blocks、最后一个工具 schema、最后一条消息的最后一个可缓存 content block 都可以携带 `cache_control: ephemeral`。
- OpenAI Responses：通过 `prompt_cache_key` 维持 cache affinity；`cached_tokens` 映射到统一 usage 字段。
- OpenAI Chat / OpenAI-compatible providers：读取后端返回的 cached token 指标；是否真实命中取决于后端 prefix cache 和路由稳定性。

仍未完全复刻的是原版更高级的缓存稳定性体系：server-side `cache_edits`、ContentReplacementState、beta/TTL latch、forked compaction cache sharing 等。

### 3.2 已有的缓存相关代码

| 位置 | 状态 | 说明 |
|------|------|------|
| `types/stream.go:75-76` | ✅ 字段存在 | `CacheCreationInputTokens`、`CacheReadInputTokens` 字段定义 |
| `provider/anthropic.go:120-121` | ✅ 数据填充 | 从 Anthropic API 响应中正确读取缓存指标 |
| `loop/query.go:371-372` | ✅ 数据传播 | 缓存指标通过 `EventTurnEnd` 事件传出 |
| `provider/provider.go` | ✅ block fallback | `SystemBlocks` > `SystemParts` > legacy `System` |
| `prompt/cache.go` | ✅ scope metadata | static/dynamic boundary、global/org scope、tool-marker fallback |
| `render.go` | ✅ 指标显示 | 有 read/create tokens 时显示统一 cache 行 |
| `compact/compact.go` | ✅ 指标追踪 | ContextWindow 记录 cache read/created；当前不驱动 compaction 决策 |

### 3.3 结构性缺陷

| 剩余缺口 | 详情 | 标记 |
|------|------|------|
| 顶层 CLI 默认仍可走 legacy string | provider/engine/loop 支持 blocks，但 main/session switcher 仍构造 legacy `System` 字符串 | remaining task: wire top-level prompt builder |
| 压缩不共享主会话缓存 | 没有原版 forked-agent cache sharing | remaining task: compaction/cache optimization |
| cache_edits | 未接 Anthropic 私有 server-side cache edit 协议 | rejected/out-of-scope |
| ContentReplacementState | 未声明原版字节级稳定 replacement state parity | remaining task if cache churn becomes material |
| Beta/TTL latch | 没有原版 GrowthBook/付费资格 latch 体系 | out-of-scope |

### 3.4 影响评估

假设 200K context window，系统提示 ~8K tokens，工具定义 ~15K tokens：
- **无缓存**：每轮 API 调用重新处理 ~23K tokens 系统前缀 + 全部历史消息
- **有缓存**：首次调用缓存前缀，后续调用仅处理增量 → **每轮节省 ~23K+ tokens**
- 10 轮对话：无缓存约 230K+ tokens 重复计费 vs 有缓存约 23K tokens 一次性 + 10× 增量
- **粗略估计：启用缓存可降低 60-80% 的 input token 费用**

---

## 四、修复方案（历史计划与当前状态）

本节保留最初的修复分解，便于理解演进过程；P0/P1 中的大部分项目已经落地，剩余项见 3.3 和 5.5。

### P0 — 基础缓存断点（高收益，中等工作量）

**改动1：系统提示支持分块缓存标记**
- ✅ `provider.Params` 已增加 `SystemBlocks`，并保留 `System` fallback
- ✅ Anthropic provider 按 system block metadata 设置 `cache_control: ephemeral`
- ✅ OpenAI/Responses provider join system blocks，忽略 Anthropic-specific cache metadata

**改动2：工具定义缓存标记**
- ✅ 在 `convertToAnthropicTools()` 中，为最后一个工具设置 `cache_control: ephemeral`
- 确保 `系统提示 + 工具定义` 前缀被整体缓存

**改动3：消息缓存断点**
- ✅ 在 `convertToAnthropicMessages()` 中，为最后一条消息的最后一个可缓存内容块设置 `cache_control: ephemeral`

### P1 — 缓存指标可观测（低工作量）

**改动4：显示缓存命中信息**
- ✅ 在 `EventTurnEnd` 处理中，打印缓存命中率
- 格式：`[cache: 45K read / 2K created / 12K uncached]`

**改动5：UpdateUsage 感知缓存**
- ✅ `ContextWindow` 记录 cache read/created；当前主要用于可观测性，不改变压缩阈值策略

### P2 — 压缩缓存稳定性（中等工作量）

**改动6：微压缩保持前缀稳定**
- 已实现的 `Microcompact()` 创建副本不改原始消息 → 已满足
- 但需确保微压缩后的 `apiMessages` 在连续轮次间保持稳定

### P3 — 高级优化（高工作量，可延后）

- 部分完成：系统提示静态/动态分界 + global/org scope metadata
- ✅ 缓存中断检测
- 剩余：工具输出替换稳定性（`ContentReplacementState`）
- Beta header / TTL 锁存

---

## 五、Go 实现现状

### 5.1 实现状态总览

| 能力 | 原版（TS） | Go 状态 | 实现文件 |
|------|-----------|---------|---------|
| 系统提示缓存断点 | ✅ 分块 + 作用域 | ✅ provider/loop 已支持；top-level CLI 默认仍可走 legacy fallback | `provider/provider.go`, `provider/anthropic.go`, `prompt/cache.go` |
| 工具定义缓存断点 | ✅ 最后一个工具 | ✅ 已实现 | `provider/anthropic.go:296` |
| 消息缓存断点 | ✅ 策略性放置 | ✅ 已实现 | `provider/anthropic.go:252` |
| 缓存指标字段 | ✅ | ✅ 存在并填充 | `types/stream.go:75-76` |
| 缓存指标显示 | ✅ 日志+分析 | ✅ REPL显示 | `render.go:100-104, 137-141` |
| ContextWindow缓存追踪 | ✅ | ✅ 字段存在 | `compact/compact.go:110-111` |
| 系统提示静态/动态分界 | ✅ 全局作用域 | ✅ metadata 已实现；入口接线仍有 legacy fallback | `prompt/cache.go`, `loop/query.go` |
| 工具输出替换稳定性 | ✅ ContentReplacementState | 未覆盖 | remaining task if needed |
| 缓存中断检测 | ✅ 两阶段检测 | ✅ 已实现 | `loop/cache_break.go` |
| Beta header/TTL 锁存 | ✅ 会话稳定 | out-of-scope | 原版产品/计费体系相关 |
| 压缩时缓存共享 | ✅ forked agent | 未覆盖 | remaining compaction/cache task |
| 缓存编辑（cache_edits） | ✅ server-side | rejected/out-of-scope | Anthropic 私有 |
| OpenAI 缓存指标 | ✅ 读取 CachedTokens | ✅ 读取 PromptTokensDetails.CachedTokens | `provider/openai.go:195-196` |
| Ollama/DeepSeek 缓存指标 | — | ✅ 走 OpenAI provider，同上 | `provider/openai.go` |
| 统一缓存指标显示 | ✅ | ✅ 所有 provider 共用 | `render.go:100-104, 137-141` |

### 5.2 缓存数据流（按 Provider 分）

**Anthropic Provider — 主动缓存（3断点）：**
```
构建请求 → 系统提示设 ephemeral     ← anthropic.go:67
         → 最后工具设 ephemeral     ← anthropic.go:296
         → 最后消息设 ephemeral     ← anthropic.go:249-256
                                      (跳过 ThinkingBlock 等不支持缓存的 block)
         → API 响应带 cache_read / cache_creation
         → 填充 Usage.CacheReadInputTokens / CacheCreationInputTokens
```

**OpenAI Provider — 被动指标（自动缓存）：**
```
构建请求 → 无需任何标记（OpenAI 2024年起自动缓存前缀）
         → API 响应带 PromptTokensDetails.CachedTokens
         → 映射到 Usage.CacheReadInputTokens              ← openai.go:195-196
         → OpenAI 不区分 cache_creation，故 CacheCreation=0
```

**Ollama / DeepSeek — 走 OpenAI Provider：**
```
同 OpenAI 路径。缓存支持取决于后端实现：
  - vLLM 支持 prefix caching（自动）
  - 原生 Ollama 无缓存（CachedTokens=0，指标显示不会出现）
  - DeepSeek API 支持自动缓存
```

**统一指标显示（render.go，所有 Provider 共用）：**
```
EventTurnEnd → 若 CacheRead > 0 或 CacheCreation > 0：
  打印 [cache: 42K read / 5K created / 3K uncached]
  对 OpenAI: [cache: 42K read / 0K created / 8K uncached]（无 creation 概念）
```

```
构建 API 请求（provider/anthropic.go:CreateStream）
    │
    ▼ 系统提示
    │  anthropic.NewTextBlock(params.System)
    │  └─ 设置 CacheControl = ephemeral              ← line 67
    │  效果：系统提示（~8K tokens）被缓存，后续轮次直接复用
    │
    ▼ 工具定义
    │  convertToAnthropicTools(params.Tools)
    │  └─ 最后一个工具设置 CacheControl = ephemeral   ← line 296
    │  效果：系统提示 + 工具定义（~23K tokens）整体缓存
    │
    ▼ 消息列表
    │  convertToAnthropicMessages(params.Messages)
    │  └─ 最后一条消息的最后一个 block 设置 ephemeral  ← line 252
    │  效果：整个消息前缀被缓存，下轮只需处理新增消息
    │
    ▼ API 响应
    │  Usage 中返回缓存指标：
    │  ├─ CacheReadInputTokens     → 从缓存复用的 token 数
    │  ├─ CacheCreationInputTokens → 新写入缓存的 token 数
    │  └─ InputTokens              → 总 input token 数（含缓存）
    │
    ▼ 指标显示（render.go）
    │  EventTurnEnd 时打印：
    │  [cache: 42K read / 5K created / 3K uncached]
    │
    ▼ 指标追踪（compact/compact.go）
       ContextWindow.CacheRead / CacheCreated 更新
       用于后续可观测性（当前不影响压缩决策）
```

### 5.3 实现细节

**系统提示缓存（`provider/anthropic.go:66-68`）：**
```go
sysBlock := anthropic.NewTextBlock(params.System)
sysBlock.CacheControl = anthropic.NewCacheControlEphemeralParam()
reqParams.System = []anthropic.TextBlockParam{sysBlock}
```
- 简化版：整个系统提示作为单一块缓存
- 与原版差异：原版分为静态（全局作用域）+ 动态（不缓存）两块

**工具定义缓存（`provider/anthropic.go:292-298`）：**
```go
if len(result) > 0 {
    last := &result[len(result)-1]
    if last.OfTool != nil {
        last.OfTool.CacheControl = anthropic.NewCacheControlEphemeralParam()
    }
}
```
- 仅最后一个工具设置断点，与原版策略一致
- 工具定义在会话内稳定 → 高缓存命中率

**消息缓存断点（`provider/anthropic.go:247-257`）：**
```go
// 从后往前找到第一个支持缓存的 block（跳过 ThinkingBlock 等）
if len(result) > 0 {
    lastMsg := &result[len(result)-1]
    for i := len(lastMsg.Content) - 1; i >= 0; i-- {
        block := &lastMsg.Content[i]
        if cc := block.GetCacheControl(); cc != nil {
            *cc = anthropic.NewCacheControlEphemeralParam()
            break
        }
    }
}
```
- 从后向前遍历，跳过不支持 `cache_control` 的 block 类型（ThinkingBlock、RedactedThinkingBlock 等）
- 避免了对 `GetCacheControl()` 返回 nil 时解引用导致的 panic
- 确保 extended thinking 模式下也能正确设置缓存断点

**缓存指标显示（`render.go:100-104, 137-141`）：**
```go
case loop.EventTurnEnd:
    if u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0 {
        cDim.Printf("[cache: %dK read / %dK created / %dK uncached]\n",
            u.CacheReadInputTokens/1000,
            u.CacheCreationInputTokens/1000,
            (u.InputTokens-u.CacheReadInputTokens-u.CacheCreationInputTokens)/1000)
    }
```
- 在 REPL 模式和 print 模式下均显示
- 仅在有缓存活动时显示（首次请求时全部为 creation）

### 5.4 预期效果

| 场景 | 无缓存（改前） | 有缓存（改后） | 节省 |
|------|--------------|--------------|------|
| 首次请求 | 全额计费 | cache_creation（1.25x 费率） | -25%（首次略贵） |
| 第2轮对话 | 全额重处理 ~23K 前缀 | cache_read（0.1x 费率） | ~90% 前缀费用 |
| 第10轮对话 | 全额重处理全部历史 | cache_read 大部分历史 | ~80% 总费用 |
| 长会话（50轮） | 累计重复处理数百万 tokens | 仅增量部分全额 | ~70-80% 总费用 |

### 5.5 与原版的剩余差距

| 能力 | 影响 | 优先级 | 说明 |
|------|------|--------|------|
| 系统提示静态/动态分界 | 中 | P2 | 动态部分变更不会废弃静态缓存 |
| ~~缓存中断检测~~ | ~~低~~ | ~~P3~~ | ✅ 已实现（`loop/cache_break.go`） |
| ContentReplacementState | 中 | P3 | 工具输出替换不破坏缓存前缀 |
| Beta/TTL 锁存 | 低 | P3 | Go版目前无 beta headers 翻转风险 |
| 压缩缓存共享 | 中 | P3 | 压缩时复用主会话缓存 |
| cache_edits | 低 | P4 | Anthropic 私有协议 |

当前实现覆盖了缓存收益的 **80/20**：3 个断点 + 指标显示 + 缓存中断检测 = 最大收益，最小改动。高级优化留作后续迭代。

---

## 六、codex-lb 缓存实测数据（2026-04-06）

### 6.1 什么是 codex-lb

codex-lb 是一个开源的 **ChatGPT 多账号负载均衡路由代理**（[GitHub](https://github.com/Soju06/codex-lb)），基于 FastAPI 构建。它不"实现"任何 API——而是将请求路由到 OpenAI 官方 API，核心能力包括：

- 多账号池管理（加密 token 存储、用量追踪、健康分层）
- 三层 sticky session 路由（turn-state header > session header > prompt_cache_key）
- 请求格式转换（V1 Chat 格式自动转 Responses 格式）

**缓存发生在 OpenAI 服务端，不在 codex-lb。** codex-lb 的 sticky routing 确保同一会话路由到同一账号，使 OpenAI 服务端的缓存得以命中。

### 6.2 测试环境

| 参数 | 值 |
|------|------|
| 代理 | codex-lb (自建) `http://192.168.31.83:2455` |
| 上游 | OpenAI 官方 API（通过 codex-lb 多账号路由） |
| 模型 | `gpt-5.4` |
| 测试日期 | 2026-04-06 |

### 6.3 三组对照实验

**实验设计：** 相同的多轮对话内容，三种不同的 API 配置，对比缓存命中率。

#### 实验 A：Chat Completions（`/v1/chat/completions`，无 sticky routing）

```
Round 1:    41 input,     0 cached,   0.0%
Round 2:  3993 input,     0 cached,   0.0%
Round 3:  9616 input,     0 cached,   0.0%
Round 4: 15031 input,     0 cached,   0.0%
Round 5: 21457 input,     0 cached,   0.0%
```

**结论：全程 0%。** codex-lb 对 Chat Completions 路径无 sticky routing，请求可能路由到不同账号，OpenAI 自动 prefix cache 无法命中。

#### 实验 B：Responses API（有 `prompt_cache_key`，无 `previous_response_id`）

```
Round 1:    36 input,     0 cached,   0.0%
Round 2:  4978 input,  4736 cached,  95.1%  ← 💥
```

**结论：Round 2 起 95% 命中。** `prompt_cache_key` 触发 codex-lb 的 sticky routing（`StickySessionKind.PROMPT_CACHE`），请求路由到同一账号 → OpenAI 自动 prefix cache 命中。

#### 实验 C：Responses API（有 `prompt_cache_key` + `previous_response_id`）

```
Round 1:    36 input,     0 cached,   0.0%
Round 2:  4848 input,  4608 cached,  95.0%  ← 💥
```

**结论：与实验 B 几乎相同。** `previous_response_id` 的额外价值不在缓存命中率（sticky routing 已经搞定），而在于**省网络带宽**（客户端只需发增量）和 **server-side state 管理**。

### 6.4 结论汇总

| API 路径 | `prompt_cache_key` | `previous_response_id` | Round 2 缓存 |
|----------|:-:|:-:|:-:|
| Chat Completions | ❌ | N/A | **0%** |
| Responses API | ✅ | ❌ | **95.1%** |
| Responses API | ✅ | ✅ | **95.0%** |

**三个关键发现：**

1. **`prompt_cache_key` 是缓存命中的关键**——它触发 codex-lb 的 sticky routing，确保同一会话路由到同一 OpenAI 账号。没有它 = 0% 缓存。

2. **`previous_response_id` 不影响缓存命中率，但有其他价值**——省网络带宽（只发增量）、server-side state 管理、容错（账号切换时 OpenAI 仍能查到历史）。

3. **Chat Completions 在 codex-lb 上永远 0% 缓存**——因为没有 sticky routing 机制。必须用 Responses API。

### 6.5 codex-lb sticky routing 机制（源码分析）

codex-lb 的 sticky routing 按优先级选择亲和策略：

```python
# app/modules/proxy/service.py — _sticky_key_for_responses_request()
PRIORITY 1: x-codex-turn-state header  → CODEX_SESSION 亲和
PRIORITY 2: session_id / x-codex-session-id header → CODEX_SESSION 亲和
PRIORITY 3: prompt_cache_key           → PROMPT_CACHE 亲和（TTL 30分钟）
PRIORITY 4: 无亲和                     → 随机负载均衡（= 0% 缓存）
```

`/v1/responses` 端点配置 `openai_cache_affinity=True`，启用 PRIORITY 3。
`/v1/chat/completions` 端点走独立路径，**无 sticky routing**。

### 6.6 Go SDK 配置方式（重构后）

API 格式（Chat Completions vs Responses）现在是独立于 Provider 的正交配置：

```bash
# 三个正交维度：
PROVIDER=openai                    # 谁提供服务
OPENAI_MODEL=gpt-5.4              # 用哪个模型
OPENAI_API=responses              # 用哪种 API 协议

# 等效的旧写法（仍兼容）：
OPENAI_USE_RESPONSES=1            # 兼容别名

# CLI flag：
claude-code-go --provider openai --api responses
```

### 6.7 代码路径验证

| 代码路径 | 文件 | 验证状态 |
|---------|------|---------|
| `prompt_cache_key` 传递 | `provider/responses.go` | ✅ 触发 codex-lb sticky routing |
| `previous_response_id` 链式传递 | `loop/query.go` → `Params.PreviousResponseID` | ✅ 已接通 |
| ResponseID 捕获 | `loop/query.go` processStream → `EventMessageStop.ResponseID` | ✅ |
| `cached_tokens` 读取 | `provider/responses.go` | ✅ 映射到 `CacheReadInputTokens` |
| Cache break 检测 | `loop/cache_break.go` | ✅ >5% 且 >2K token 下降时告警 |

### 6.8 实际使用预期

| 场景 | API | 预期缓存 | 原因 |
|------|-----|---------|------|
| codex-lb + Chat Completions | `/v1/chat/completions` | **0%** | 无 sticky routing |
| codex-lb + Responses API | `/v1/responses` | **95%+** (Round 2 起) | `prompt_cache_key` sticky routing |
| OpenAI 官方 + Chat Completions | `/v1/chat/completions` | **90%+** (≥1024 tokens) | OpenAI 自动 prefix cache |
| OpenAI 官方 + Responses API | `/v1/responses` | **95%+** | 自动 prefix cache + server-side state |
| Anthropic API | Messages API | **90%+** (Round 2 起) | 3 断点 `cache_control` |
| vLLM/SGLang 自建 | Chat Completions | **0-90%+** | 取决于 `--enable-prefix-caching` |
| Ollama | Chat Completions | **0%** | 无 prefix caching |
