# 基础设施层架构 — 设计参考文档

> 基于 Claude Code TypeScript 原版（`../src`）提炼，定义 Go 实现的目标架构。
> 覆盖四个核心模块：**types（类型系统）、permissions（权限控制）、render（终端渲染）、cli（命令行入口）**，
> 以及根目录的启动编排文件（main.go / repl_tui.go / repl_common.go / printmode.go / signals.go）。

---

## 一、概述

基础设施层是整个 LUBAN Code 的地基，承担四项核心职责：

| 职责 | 模块 | 说明 |
|------|------|------|
| **类型系统** | `types/` | 定义消息、内容块、工具、流式事件等全局数据结构，供所有模块共享 |
| **权限控制** | `permissions/` | 三模式（AllowAll / AskAlways / RuleBased）权限检查，带会话缓存和并发安全 |
| **终端渲染** | `render/` + `render.go` | Markdown → ANSI 彩色终端输出，含工具调用预览和结果格式化 |
| **CLI 入口** | `cli/` + `main.go` + `repl_tui.go` | 命令行参数解析、启动模式分发（Print / TUI）、信号处理 |

这四个模块构成"零依赖基底"——它们互不依赖，而是被上层模块（`loop`、`tools`、`session` 等）所引用。

---

## 二、原版（TS）设计详情

### 2.1 类型系统 — `src/types/`

TypeScript 原版的类型系统散布于多个文件，核心结构如下：

#### 消息与内容块

原版依赖 Anthropic SDK（`@anthropic-ai/sdk`）提供的类型，同时自行扩展：

| TS 类型 | 说明 |
|---------|------|
| `MessageParam` | SDK 消息参数，含 `role` 和 `content` |
| `ContentBlockParam` | 联合类型：`TextBlockParam \| ToolUseBlockParam \| ToolResultBlockParam \| ImageBlockParam` |
| `TextBlock` | `{type:'text', text:string}` |
| `ToolUseBlock` | `{type:'tool_use', id, name, input}` |
| `ToolResultBlock` | `{type:'tool_result', tool_use_id, content, is_error?}` |
| `ThinkingBlock` | `{type:'thinking', thinking, signature?}` — 扩展类型 |
| `ImageBlockParam` | `{type:'image', source:{type,media_type,data}}` |

内容块的 JSON 反序列化依赖 Zod schema 进行运行时验证（`lazySchema.ts`），比 Go 的静态类型切换更灵活但开销更高。

#### 权限类型 — `src/types/permissions.ts`

原版权限系统在类型层面极为复杂，远超 Go 实现：

| TS 类型 | 说明 |
|---------|------|
| `PermissionMode` | `'acceptEdits' \| 'bypassPermissions' \| 'default' \| 'dontAsk' \| 'plan' \| 'auto' \| 'bubble'` |
| `PermissionBehavior` | `'allow' \| 'deny' \| 'ask'` |
| `PermissionRule` | 含 `source`（来源追踪）+ `ruleBehavior` + `ruleValue` |
| `PermissionRuleSource` | `'userSettings' \| 'projectSettings' \| 'session' \| 'cliArg' \| ...` （7种来源） |
| `PermissionDecision` | 三路联合：`PermissionAllowDecision \| PermissionAskDecision \| PermissionDenyDecision` |
| `PermissionDecisionReason` | 9种决策原因：`rule / mode / hook / classifier / sandboxOverride / ...` |
| `ToolPermissionContext` | 只读上下文，含模式、额外工作目录、allow/deny/ask 规则集 |
| `ClassifierResult` | AI 分类器结果（2阶段：fast→thinking），含置信度、请求ID、耗时 |
| `YoloClassifierResult` | bypassPermissions 模式下的分类器结果 |

**原版特有机制（Go 未实现）：**
- **多源规则**：同一条工具规则可来自 settings.json、CLI 参数、会话或命令——有优先级链
- **AI 分类器**：`auto` 模式下，由 LLM 分类器异步评估危险命令，可在用户点击前自动审批
- **Sandbox**：macOS 沙盒集成，危险操作自动隔离
- **Permission prompt tool**：通过 MCP 工具协议让外部系统参与权限决策
- **Hooks 集成**：`PreToolUse` 钩子可动态覆盖权限结果

#### 会话 ID 类型 — `src/types/ids.ts`

```typescript
type SessionId = string & { readonly __brand: 'SessionId' }
type AgentId   = string & { readonly __brand: 'AgentId' }
```

通过 Branded Types 在编译期防止 SessionId 与 AgentId 混用。Go 版本使用普通 `string`，无此静态保障。

### 2.2 渲染层 — `src/components/` (Ink)

原版使用 [Ink](https://github.com/vadimdemedes/ink)（React for terminals）构建整个 TUI：

| Ink 组件 | 功能 |
|----------|------|
| `<App>` | 顶层组件，管理全局状态 |
| `<Messages>` | 消息历史渲染，含虚拟化滚动 |
| `<ToolCall>` | 工具调用展示（折叠/展开、实时流式） |
| `<PermissionPrompt>` | 权限确认 UI（键盘导航、yes/no/always） |
| `<StatusBar>` | 底部状态栏（token 用量、模型名、spinner） |
| `<ThinkingBlock>` | 思考过程折叠展示 |

Markdown 渲染使用 `marked` + `terminal-link` 库，支持：
- 语法高亮（通过 `highlight.js`）
- 超链接（通过 OSC 8 转义序列）
- 表格渲染
- 图片内联显示（iTerm2 协议）

**Ink 渲染模型的核心特点：**
1. **增量更新**：React 差量 diff → 只重绘变化部分，无闪烁
2. **状态驱动**：通过 React state/hooks 管理渲染状态
3. **全宽布局**：实时获取终端宽度，自动 reflow
4. **鼠标支持**：可选的鼠标事件处理

### 2.3 CLI 入口 — `src/cli/`

原版 CLI 通过 Commander.js 解析，参数远多于 Go 版本：

| 参数 | 说明 |
|------|------|
| `--print / -p` | 单次查询模式 |
| `--model` | 指定模型 |
| `--permission-mode` | 权限模式（default/acceptEdits/bypassPermissions/...） |
| `--allowed-tools` | 工具白名单（逗号分隔） |
| `--disallowed-tools` | 工具黑名单 |
| `--max-turns` | 最大轮次 |
| `--system-prompt` | 覆盖系统提示 |
| `--resume` | 恢复上次会话 |
| `--session-id` | 指定会话 ID |
| `--mcp-server` | 注册 MCP 服务器 |
| `--add-dir` | 添加工作目录 |
| `--output-format` | 输出格式（text/json/stream-json） |
| `--verbose` | 详细日志 |
| `--no-update` | 禁用自动更新 |

**特殊 CLI 功能（Go 未实现）：**
- `--output-format json` → JSON lines 输出（供外部程序解析）
- `--mcp-server` → 连接 MCP 服务器（stdio/SSE/HTTP）
- 自动更新检测与安装
- 远程 IO（headless 模式，接受来自 stdin 的 NDJSON 指令）

---

## 三、Go 实现现状

### 3.1 types 模块

**文件：** `types/messages.go`, `types/tools.go`, `types/stream.go`

#### 核心类型定义

```
types/
├── messages.go   内容块类型 + Message + 构造函数 + JSON 编解码
├── tools.go      Tool 接口 + ToolDefinition + ToolResult + 辅助函数
└── stream.go     StreamEvent + ContentDelta + APIMessage + Usage + 请求类型
```

**消息与内容块（messages.go）**

| Go 类型 | 对应 TS 类型 | 状态 |
|---------|-------------|------|
| `ContentBlock` (interface) | `ContentBlockParam` | ✅ 已实现 |
| `TextBlock` | `TextBlockParam` | ✅ 已实现 |
| `ThinkingBlock` | `ThinkingBlock` | ✅ 已实现（含 Signature） |
| `ToolUseBlock` | `ToolUseBlockParam` | ✅ 已实现 |
| `ToolResultBlock` | `ToolResultBlockParam` | ✅ 已实现 |
| `ImageBlock` + `ImageSource` | `ImageBlockParam` | ✅ 已实现 |
| `UnknownBlock` | N/A（原版用 Zod catch-all） | ✅ Go 独有（防数据丢失） |
| `Message` | `MessageParam` | ✅ 已实现 |
| `Role` (`user`/`assistant`) | `Role` | ✅ 已实现 |
| `StopReason` | `StopReason` | ✅ 已实现 |

**自定义 JSON 序列化：** `Message.MarshalJSON()` 和 `Message.UnmarshalJSON()` 处理 `ContentBlock` 接口的多态序列化——对已知类型按 `type` 字段路由，对未知类型保留 `UnknownBlock.Raw`（`json.RawMessage`），避免数据丢失。

**工具接口（tools.go）**

```go
type Tool interface {
    Name()        string
    Description() string
    Schema()      JSONSchema
    Execute(ctx context.Context, input map[string]any) (ToolResult, error)
}
```

**错误语义（Go 特有的两级错误模型）：**
- `(ToolResult{IsError: true}, nil)` → 业务层错误，LLM 可见，继续对话
- `(ToolResult{}, err)` → 基础设施错误，中断当前轮次

**流式事件（stream.go）**

| Go 类型 | 说明 |
|---------|------|
| `StreamEvent` | SSE 流中的单条事件 |
| `StreamEventType` | 8 种事件类型常量 |
| `ContentDelta` | 增量内容（文本/工具输入/思考） |
| `APIMessage` | API 顶层响应 |
| `Usage` | Token 用量（含缓存字段） |
| `APIError` | API 错误，实现 `error` 接口 |
| `CreateMessageRequest` | 请求体（含 Temperature/TopP/TopK/Metadata） |

#### 类型测试覆盖

| 测试 | 文件 |
|------|------|
| `TestUserMessage` / `TestAssistantMessageFunc` | messages_test.go |
| `TestToolResultMessage` | messages_test.go |
| `TestHasToolUse` / `GetToolUses` | messages_test.go |
| `TestMessageMarshalJSON` | messages_test.go |
| `TestMessageWithToolUseMarshalRoundTrip` | messages_test.go |
| Tool interface 一致性 | tools_test.go |
| Stream event 序列化 | （暂无独立测试） |

---

### 3.2 permissions 模块

**文件：** `permissions/permissions.go`, `permissions/prompt.go`

#### 架构

```
permissions/
├── permissions.go   Checker + Rule + Mode + Decision + 缓存 + 规则匹配
└── prompt.go        交互式提示函数（终端 yes/no/always/once 提示）
```

#### 三模式权限系统

```
ModeAllowAll  → 直接返回 DecisionAllow（跳过所有检查）
ModeAskAlways → askOrCache()：检查会话缓存 → 有缓存则复用 → 无缓存则调用 promptFunc
ModeRuleBased → evaluateRules()：按顺序匹配规则 → 无匹配则 fallthrough 到 askOrCache()
```

**四种决策值：**

| 决策 | 含义 | 缓存 |
|------|------|------|
| `DecisionAllow` | 永久允许 | ✅ 存入 sessionCache |
| `DecisionDeny` | 拒绝 | ❌ 不缓存（每次重新询问） |
| `DecisionAsk` | 需要询问 | — |
| `DecisionAllowOnce` | 允许本次但不缓存 | ❌ 不缓存 |

**会话缓存键生成策略（cacheKey）：**

| 工具 | 键格式 | 安全性 |
|------|--------|--------|
| `Bash` | `Bash:<sha256[:8]>` | 按完整命令哈希，防止 "git status" 缓存绕过 "git push --force" |
| `Write/Edit/Read` | `{Tool}:{cleaned_path}` | 按绝对路径，防止目录级权限穿透 |
| 其他 | `{toolName}` | 按工具名 |

**规则匹配（evaluateRules）：**
- `Tool` 字段支持 glob 模式（`filepath.Match`），`*` 匹配所有工具
- `Pattern` 字段按字段类型匹配：`file_path` → glob，`command` → 前缀匹配，其他 → 精确匹配
- 失败闭合（fail-closed）：模式语法错误 → 视为匹配，确保 deny 规则生效

**并发安全：** `sessionCache` 由 `sync.RWMutex` 保护，支持高并发工具调用。

#### 测试覆盖

| 测试 | 文件 |
|------|------|
| `TestModeAllowAll` | permissions_test.go |
| `TestRuleBasedAllow` / `Wildcard` | permissions_test.go |
| `TestAskAlwaysWithoutPromptFuncDenies` | permissions_test.go |
| `TestSessionCache`（缓存命中/不同命令不共享） | permissions_test.go |
| `TestRuleBasedFallthroughToAsk` | boundary_test.go |
| `TestDenyDecisionNotCached` | boundary_test.go |
| `TestCacheKeyFilePathExact`（文件粒度缓存） | boundary_test.go |
| `TestCacheKeyUnknownTool` | boundary_test.go |
| `TestPermissionsConcurrentCheck`（race detector） | boundary_test.go |

---

### 3.3 render 模块

**文件：** `render/markdown.go`, `render/markdown_test.go`, `render.go`（根目录）

#### 架构

```
render/
└── markdown.go     Markdown → ANSI 渲染（fatih/color 驱动）

render.go（package main）
    ├── formatToolInput()    工具调用单行预览（含截断）
    ├── formatToolResult()   工具结果多行展示（含截断，默认20行）
    ├── makeEventHandler()   print 模式事件回调
    └── makeREPLEventHandler() REPL 模式事件回调
```

#### Markdown 渲染（render/markdown.go）

**实现方式：** 自研行扫描器，使用 `fatih/color` 输出 ANSI 颜色码。无外部 Markdown 库依赖。

| 语法元素 | 实现方式 | 颜色/样式 |
|----------|----------|----------|
| `# H1` | 前缀匹配 | Bold + Cyan |
| `## H2` | 前缀匹配 | Bold + Blue |
| `### H3` | 前缀匹配 | Bold |
| `- item` / `* item` | 前缀匹配 | 黄色 `•` 符号 |
| `1. item` | 正则匹配 | 黄色数字 |
| `> quote` | 前缀匹配 | 绿色 `│` 边框 |
| ` ``` code ``` ` | 状态机（inCodeBlock） | 灰色 `│` 前缀，语言标签 |
| `**bold**` | 正则替换 | Bold |
| `*italic*` | 正则替换 | Italic |
| `` `code` `` | 正则替换 | Cyan |
| `[text](url)` | 正则替换 | Underline + 暗色 URL |
| `~~strike~~` | 正则替换 | CrossedOut |
| `---` / `***` / `___` | 字符检测 | 暗色 `────` 横线 |

**处理顺序（inline 格式化）：**
1. 行内代码（先处理，保护内容不被后续规则二次替换）
2. 加粗 → 斜体 → 链接 → 删除线

#### render.go — 事件到终端的适配

`makeEventHandler` / `makeREPLEventHandler` 将 `loop.Event` 类型映射到终端输出：

| 事件类型 | 终端输出 |
|----------|----------|
| `EventText` | 直接 `fmt.Print`（流式输出） |
| `EventThinking` | 暗色文本（print 模式需 `--verbose`，REPL 模式始终显示） |
| `EventToolUse` | 黄色 `⚡ ToolName` + 暗色参数预览 |
| `EventToolResult` | 暗色 `↳` 或 `✗` 前缀，20行截断 |
| `EventTurnEnd` | 暗色 token 用量（仅缓存相关字段有值时显示） |
| `EventError` | 红色 `Error: ...` |

**工具输入预览（formatToolInput）：**

| 工具 | 预览格式 | 截断 |
|------|----------|------|
| `Bash` | `` ` cmd` `` | 100字符 |
| `Read/Write/Edit` | 文件路径 | — |
| `Glob` | pattern 字符串 | — |
| `Grep` | `/pattern/` 格式 | — |
| `Agent` | `"prompt..."` | 80字符 |

---

### 3.4 cli 模块

**文件：** `cli/cli.go`

#### 设计原则

> 仅使用标准库 `flag` 包，零额外依赖。

```go
type Options struct {
    Model        string   // --model / -m
    Provider     string   // --provider
    Print        bool     // -p
    Resume       bool     // --resume
    SessionID    string   // --session-id（含路径遍历防护）
    MaxTurns     int      // --max-turns（默认100）
    SystemPrompt string   // --system-prompt
    AllowedDirs  []string // --allowed-dir（可重复）
    AllowAll     bool     // --allow-all
    Version      bool     // --version / -v
    Help         bool     // --help / -h
    Verbose      bool     // --verbose
    Args         []string // 位置参数（-p 模式的查询）
}
```

**特殊实现细节：**
- `--help / -h`：在 `flag.Parse` 前拦截，输出到 stdout（而非 stderr），exit 0
- `--version`：解析完成后立即打印并退出
- `--allowed-dir`：`multiString` 实现 `flag.Value`，支持多次重复传入
- Session ID 校验：`^[a-zA-Z0-9_T:.\-]+$` + 禁止 `..`（防路径遍历攻击）

---

### 3.5 根目录文件

#### main.go — 启动编排

`main()` 是整个应用的编排入口，依次完成：

```
1. cli.Parse()                  → 解析 CLI 参数
2. provider.NewFromEnvWithOverrides() → 创建 AI 提供商（Anthropic/OpenAI/Ollama/DeepSeek）
3. os.Getwd() + allowedDirs     → 确定工作目录 + 权限白名单
4. SetupRegistry()              → 注册所有工具（含 Agent、Team 工具）
5. prompt.BuildSystemPrompt()   → 读取 CLAUDE.md + git 上下文，构建系统提示
6. loop.New()                   → 创建查询循环（含压缩配置）
7. hooks.LoadFromSettings()     → 加载 .claude/settings.json 中的钩子
```

然后按 CLI 参数分叉：
- `-p` → `RunPrintMode()`（单次查询，完成后退出）
- 否则 → `RunTUIREPL()`（默认交互式 TUI）

**颜色变量（package-level，供 render.go 共用）：**

```go
var (
    cGreen    = color.New(color.FgGreen)      // readline 提示符
    cYellow   = color.New(color.FgYellow)     // 工具名
    cDim      = color.New(color.Faint)        // 次要信息
    cRed      = color.New(color.FgRed)        // 错误
    cBoldCyan = color.New(color.Bold, color.FgCyan) // 启动 Banner
)
```

#### repl_tui.go — 交互式 TUI 循环

`RunTUIREPL` 实现完整的 TUI 读取-执行-存储循环：

```
TUI TextArea submit → TrimSpace → 空输入跳过
    ├── "exit"/"quit" → 退出
    ├── /command → cmdReg.Parse() → cmd.Execute()
    │       └── ErrExit → 退出
    └── 普通文本 → context.WithCancel() → ql.Run()
                    └── Store.Save()（自动存档）
```

**命令注册：** `commands.NewRegistry()` + `RegisterBuiltins()` 支持斜杠命令（`/help`、`/history` 等）。

**会话适配器（`sessionStoreAdapter`）：** 将 `session.FileStore` 的接口桥接到 `commands.SessionStore`，解耦会话存储实现。

#### printmode.go — 单次查询模式

`RunPrintMode` 是无状态的单次执行路径：

```
→ context.WithCancel()
→ signal.Notify(sigCh, SIGINT, SIGTERM)（独立 goroutine）
→ ql.Run(ctx, query, makeEventHandler(verbose))
→ fmt.Println()
→ return exitCode（0 成功，1 失败/取消）
```

#### signals.go — 两级信号处理

`SignalHandler` 实现了两层 SIGINT 语义：

```
SIGINT + 有活跃查询 → queryCancelFn()（取消当前查询，不退出）
SIGINT + 无活跃查询 → globalCancel()（退出程序）
SIGTERM             → globalCancel()（始终退出程序）
```

并发安全：`queryCancelFn` 读写由 `sync.Mutex` 保护。

---

### 3.6 实现状态总览

| 模块/特性 | 原版 TS | Go 实现 | 完整度 |
|-----------|---------|---------|--------|
| **types — 基础消息类型** | SDK 类型 + Zod 验证 | 原生 Go struct + 自定义 JSON | ✅ 95% |
| types — UnknownBlock（防数据丢失） | Zod catch-all | `UnknownBlock{Raw: json.RawMessage}` | ✅ Go 特有 |
| types — Branded ID 类型 | `SessionId`/`AgentId` branded string | 普通 `string` | ❌ 无类型保障 |
| types — 流式事件 | SDK `MessageStreamEvent` | `StreamEvent` + `ContentDelta` | ✅ 90% |
| **permissions — 三模式** | 5+ 模式（含 auto/bubble） | AllowAll/AskAlways/RuleBased | ⚠️ 60% |
| permissions — 规则匹配 | 多源优先级链 | 顺序规则数组 | ⚠️ 简化版 |
| permissions — 会话缓存 | session-scoped cache | `sessionCache map` + `RWMutex` | ✅ 已实现 |
| permissions — AI 分类器 | 两阶段 LLM 分类 | ❌ 未实现 | ❌ |
| permissions — Sandbox | macOS sandbox-exec | ❌ 未实现 | ❌ |
| permissions — Hooks 集成 | PreToolUse 覆盖权限 | 部分（HookRunner 存在但未接入） | ⚠️ |
| **render — Markdown** | marked + highlight.js + Ink | 自研行扫描器 + fatih/color | ⚠️ 75% |
| render — 代码语法高亮 | highlight.js（多语言） | 无高亮（仅暗色显示） | ❌ |
| render — 超链接（OSC 8） | terminal-link | ❌ 未实现 | ❌ |
| render — 表格渲染 | Ink `<Box>` 布局 | ❌ 未实现 | ❌ |
| render — 图片内联 | iTerm2 协议 | ❌ 未实现 | ❌ |
| render — 增量刷新 | React diff | 简单 `fmt.Print` 流式 | ⚠️ 功能等价 |
| **cli — 基础参数** | Commander.js | 标准库 `flag` | ✅ 90% |
| cli — MCP 服务器参数 | `--mcp-server` | ❌ 未实现 | ❌ |
| cli — 输出格式 | `--output-format json` | ❌ 未实现 | ❌ |
| cli — 权限模式参数 | `--permission-mode` | `--allow-all`（简化） | ⚠️ |
| **main — 启动编排** | Commander + React | main() 顺序编排 | ✅ 90% |
| **signals — 两级 SIGINT** | readline + process 管理 | `SignalHandler` goroutine | ✅ 已实现 |
| **interactive UI — 交互循环** | Ink | `go-tui` + `RunTUIREPL` | ✅ 90% |

---

### 3.7 启动流程图（ASCII）

```
main()
  │
  ├─ cli.Parse()
  │      └─ flag 解析 → Options{Model, Provider, Print, MaxTurns, AllowedDirs, ...}
  │
  ├─ provider.NewFromEnvWithOverrides(opts.Provider, opts.Model)
  │      └─ 检测 ANTHROPIC_API_KEY / OPENAI_API_KEY / OLLAMA_HOST 等环境变量
  │         返回实现 Provider 接口的具体实现
  │
  ├─ os.Getwd() → cwd
  │
  ├─ SetupRegistry(p, cwd, allowedDirs)
  │      ├─ 注册内置工具：Bash, Read, Write, Edit, Glob, Grep, Agent, ...
  │      └─ 返回 deps{Registry, AgentTool, TeamManager, CronStore}
  │
  ├─ prompt.BuildSystemPrompt()
  │      ├─ prompt.DiscoverClaudeMD(cwd)  → 读取 CLAUDE.md
  │      └─ prompt.LoadGitContext(cwd)    → git branch/status 等
  │
  ├─ loop.New(p, deps.Registry, Config{System, MaxTurns, MaxTokens, HookRunner})
  │      └─ 创建 QueryLoop，含 compact.SummaryCompactor 等
  │
  ├─ hooks.LoadFromSettings() + LoadFromDir()
  │      └─ 合并 .claude/settings.json 和 .claude/hooks/ 中的钩子
  │
  ├─ [opts.Print == true]
  │      └─ RunPrintMode(ql, query, verbose)
  │              ├─ context.WithCancel()
  │              ├─ signal.Notify(SIGINT/SIGTERM)
  │              ├─ ql.Run(ctx, query, makeEventHandler)
  │              └─ os.Exit(exitCode)
  │
  └─ [交互模式]
         ├─ session.NewFileStore() → store
         ├─ ResolveSession()       → sessionID（新建或恢复）
         ├─ compact.NewResultStore() → resultStore（工具输出持久化）
         ├─ context.WithCancel()   → globalCtx
         ├─ NewSignalHandler()     → sigHandler
         └─ RunTUIREPL(globalCtx, TUIREPLConfig, sigHandler)
                 │
                 └─ loop:
                       TUI input submit
                         │
                         ├─ /cmd → commands.Execute()
                         └─ text → context.WithCancel()
                                      → ql.Run(queryCtx, input, onEvent)
                                      → queryCancel()
                                      → store.Save(sessionID, messages)
```

---

## 四、关键知识背景

### 4.1 Go 类型系统设计选择

**接口驱动多态（vs TS 联合类型）**

TypeScript 使用联合类型（`TextBlock | ToolUseBlock | ...`）+类型守卫（`if (block.type === 'tool_use')`）。Go 使用接口（`ContentBlock interface`）+ 类型断言（`if tb, ok := block.(TextBlock); ok`）。

两者功能等价，但 Go 方案在反序列化时需要自定义 `UnmarshalJSON` 实现分发逻辑（`tools.go` 中的 switch 块）。这部分是 Go 版本的核心复杂度所在。

**UnknownBlock 的价值**

原版依赖 Zod 的 `.passthrough()` 或 catch-all 保留未知字段；Go 版通过 `UnknownBlock{Raw: json.RawMessage}` 实现等价效果——任何未知 `type` 的内容块以原始 JSON 保留，不丢失数据，且可在序列化时原样输出。

**两级错误语义（Tool.Execute）**

```
(ToolResult{IsError: true}, nil)  → 业务错误：LLM 可见，对话继续
(ToolResult{}, err)               → 基础设施错误：中断当前轮次，向上传播
```

这是 Go 惯用的多返回值错误处理的有意设计，比原版的 `throws` + try/catch 更清晰。

### 4.2 终端渲染技术

**fatih/color vs Ink**

| 维度 | fatih/color（Go） | Ink（TS） |
|------|-------------------|-----------|
| 模型 | 命令式，逐行写入 | 声明式 React 组件树 |
| 更新 | 无法局部更新（除非使用 ANSI 光标控制） | React diff，局部更新 |
| 复杂度 | 简单，无运行时开销 | 复杂，需要 fiber reconciler |
| 适用场景 | 流式输出、简单格式化 | TUI 应用、复杂布局 |

Go 的流式输出模型（`fmt.Print` 逐字符打印 LLM 输出）实际上与终端渲染解耦，在大多数使用场景下体验与 Ink 等价。差距主要体现在：交互式权限提示、折叠/展开控件、状态栏。

**ANSI 转义序列支持**

`fatih/color` 自动处理：
- `NO_COLOR` 环境变量（禁用颜色）
- 非 TTY 检测（管道输出时禁用颜色）
- Windows 控制台（通过 `mattn/go-colorable`）

### 4.3 CLI 框架对比

| 特性 | Commander.js（原版） | Go flag 标准库（Go版） |
|------|---------------------|----------------------|
| 子命令 | ✅ 支持 | ❌ 不支持 |
| 自动帮助 | ✅ 自动生成 | ⚠️ 需自定义 |
| 参数验证 | ✅ 内置类型检查 | ⚠️ 需手动 |
| 可重复参数 | ✅ 内置 | 需实现 `flag.Value` |
| 依赖 | npm 包 | 零依赖 |

Go 版通过 `multiString` 类型（实现 `flag.Value` 接口）支持 `--allowed-dir` 的重复传入，这是标准库实现可重复参数的惯用模式。

### 4.4 信号处理模式

Go 版的两级 SIGINT 处理（query-cancel vs global-cancel）是终端应用的标准模式：

```
第一次 Ctrl-C（有活跃查询） → 仅取消当前查询，回到提示符
第一次 Ctrl-C（无活跃查询） → 退出程序
SIGTERM                      → 始终退出程序（用于 kill 命令）
```

这与 bash 等 shell 的行为一致，避免用户误操作退出程序。

---

## 五、评估指标

### 5.1 核心类型覆盖率

| 类型类别 | TS 类型数 | Go 类型数 | 覆盖率 |
|----------|-----------|-----------|--------|
| 内容块类型 | 5（text/tool_use/tool_result/thinking/image） | 6（+UnknownBlock） | ✅ 120%（Go 超集） |
| 流式事件类型 | 8 | 8 | ✅ 100% |
| Message 构造函数 | 3（user/assistant/toolResult） | 3 | ✅ 100% |
| 请求参数字段 | 15+ | 12 | ⚠️ 80% |
| 权限模式 | 7（含 auto/bubble） | 3 | ❌ 43% |
| 权限决策原因 | 9 种 | 4 种（Allow/Deny/Ask/AllowOnce） | ⚠️ 44% |

### 5.2 权限模型完整度

| 功能 | 原版 | Go | 完整度 |
|------|------|----|--------|
| AllowAll 模式 | ✅ | ✅ | 100% |
| 规则匹配（glob + 前缀） | ✅ | ✅ | 90% |
| 会话级缓存 | ✅ | ✅ | 100% |
| AllowOnce（不缓存） | ✅ | ✅ | 100% |
| 多来源规则（settings/cli/session） | ✅ | ❌ | 0% |
| 目录白名单 | ✅ | ✅（allowedDirs） | 80% |
| AI 分类器（auto 模式） | ✅ | ❌ | 0% |
| Hooks 覆盖权限 | ✅ | ❌ | 0% |
| macOS Sandbox | ✅ | ❌ | 0% |
| **综合完整度** | — | — | **~45%** |

### 5.3 渲染特性支持度

| 渲染特性 | 原版 | Go | 支持度 |
|----------|------|----|--------|
| 标题（H1-H3） | ✅ | ✅ | 100% |
| 粗体/斜体/删除线 | ✅ | ✅ | 100% |
| 行内代码 | ✅ | ✅ | 100% |
| 围栏代码块 | ✅ | ✅（无高亮） | 70% |
| 有序/无序列表 | ✅ | ✅ | 100% |
| 引用块 | ✅ | ✅ | 100% |
| 水平分割线 | ✅ | ✅ | 100% |
| 链接（文本显示） | ✅ | ✅（无 OSC 8） | 80% |
| 代码语法高亮 | ✅ | ❌ | 0% |
| 表格 | ✅ | ❌ | 0% |
| 图片内联 | ✅ | ❌ | 0% |
| 工具调用预览 | ✅ | ✅ | 90% |
| **综合支持度** | — | — | **~75%** |

### 5.4 CLI 参数兼容性

| 参数 | 原版 | Go | 状态 |
|------|------|----|------|
| `-p` / `--print` | ✅ | ✅ | ✅ 兼容 |
| `--model` / `-m` | ✅ | ✅ | ✅ 兼容 |
| `--max-turns` | ✅ | ✅ | ✅ 兼容 |
| `--system-prompt` | ✅ | ✅ | ✅ 兼容 |
| `--resume` | ✅ | ✅ | ✅ 兼容 |
| `--session-id` | ✅ | ✅ | ✅ 兼容（含安全验证） |
| `--verbose` | ✅ | ✅ | ✅ 兼容 |
| `--version` / `-v` | ✅ | ✅ | ✅ 兼容 |
| `--allowed-dir` | `--add-dir` | `--allowed-dir` | ⚠️ 名称不同 |
| `--allow-all` | `--dangerously-skip-permissions` | `--allow-all` | ⚠️ 名称不同 |
| `--permission-mode` | ✅ | ❌ | ❌ |
| `--mcp-server` | ✅ | ❌ | ❌ |
| `--output-format` | ✅ | ❌ | ❌ |
| `--provider`（Go 独有） | ❌ | ✅ | — |
| **参数兼容性** | — | — | **~65%** |

### 5.5 启动流程完整度

| 启动步骤 | 原版 | Go | 完整度 |
|----------|------|----|--------|
| CLI 参数解析 | ✅ | ✅ | 90% |
| Provider 初始化 | ✅ | ✅ | 95% |
| 工具注册 | ✅ | ✅ | 80% |
| 系统提示构建（含 CLAUDE.md） | ✅ | ✅ | 85% |
| 会话加载/恢复 | ✅ | ✅ | 80% |
| Hooks 加载 | ✅ | ✅ | 70% |
| 查询循环创建 | ✅ | ✅ | 85% |
| 信号处理 | ✅ | ✅ | 95% |
| Print 模式 | ✅ | ✅ | 90% |
| REPL 模式 | ✅ | ✅ | 85% |
| 自动更新检测 | ✅ | ❌ | 0% |
| MCP 服务器连接 | ✅ | ❌ | 0% |
| **综合完整度** | — | — | **~72%** |

---

## 六、与原版的差距及后续规划

### 6.1 差距汇总

#### 类型系统（差距：小）

| 差距项 | 优先级 | 工作量 |
|--------|--------|--------|
| `SessionId`/`AgentId` Branded Types（编译期类型安全） | P3 | 低（Go 可用 `type SessionId string` + 构造函数实现） |
| `CreateMessageRequest` 缺少部分字段（如 `betas`、`thinking`） | P2 | 低 |
| 流式事件未覆盖所有 SDK 事件类型 | P2 | 低 |

#### 权限系统（差距：大）

| 差距项 | 优先级 | 工作量 |
|--------|--------|--------|
| 多来源规则合并（settings.json / CLI / 会话） | P1 | 中 |
| `--permission-mode` CLI 参数（acceptEdits/plan 等） | P1 | 低 |
| Hooks 集成（PreToolUse 覆盖权限） | P2 | 中 |
| AI 分类器（auto 模式）| P3 | 高 |
| macOS Sandbox | P4 | 高 |

#### 渲染层（差距：中）

| 差距项 | 优先级 | 工作量 |
|--------|--------|--------|
| 代码块语法高亮（[chroma](https://github.com/alecthomas/chroma) 库可选） | P2 | 低 |
| OSC 8 超链接（终端超链接转义序列） | P3 | 低 |
| 表格渲染 | P3 | 中 |
| 交互式权限提示 UI（当前为终端 yes/no） | P2 | 中 |

#### CLI / 启动（差距：中）

| 差距项 | 优先级 | 工作量 |
|--------|--------|--------|
| `--output-format json` JSON lines 输出（供脚本集成） | P1 | 中 |
| `--allowed-tools` / `--disallowed-tools` 工具过滤 | P2 | 低 |
| 自动更新检测（GitHub Releases API） | P3 | 中 |
| MCP 服务器参数（已有 MCP 模块，需串联） | P2 | 中 |

### 6.2 后续优化路线（按优先级）

**P1 — 高影响、低风险（2周内可完成）**

1. **`--output-format json`**：在 print 模式输出 NDJSON，每行一个事件对象。这是让 LUBAN Code 可嵌入脚本/工具链的关键特性。实现位置：`printmode.go` + 新文件 `cli/jsonoutput.go`。

2. **多源权限规则**：将 `permissions.Rule` 扩展为包含 `source` 字段，在 `main.go` 中从 settings.json、CLI 参数分别加载并合并规则集。实现位置：`permissions/permissions.go` + `registry_setup.go`。

3. **`--permission-mode` 参数**：添加 `--permission-mode acceptEdits|default|bypassPermissions` CLI 参数，映射到现有的三种 `Mode`。

**P2 — 中等影响、中等工作量（1个月内可完成）**

4. **代码语法高亮**：引入 `alecthomas/chroma` 库，在 `render/markdown.go` 的代码块渲染中增加语言感知高亮。可选功能，不影响正确性。

5. **工具过滤参数**：`--allowed-tools`/`--disallowed-tools` 在 `registry_setup.go` 中过滤工具注册。

6. **交互式权限提示 UI 改进**：在 `permissions/prompt.go` 中使用 `readline` 提供更友好的 yes/no/always/once 提示，含默认值和快捷键提示。

**P3 — 完整性提升（中长期）**

7. **Branded Types for Session/Agent ID**：Go 中可用 `type SessionId string` + `func NewSessionId() SessionId` 实现轻量级 branded types。

8. **OSC 8 超链接**：在 `render/markdown.go` 的链接渲染中使用 `\033]8;;URL\033\\text\033]8;;\033\\` 转义序列，支持 iTerm2/kitty/现代终端。

9. **MCP 参数串联**：`--mcp-server` 参数已有 `mcp` 模块支持，需在 CLI 解析和 `registry_setup.go` 中串联。
