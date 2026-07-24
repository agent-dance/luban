# Tools 模块设计参考文档

> 版本：2026-04-05 | 覆盖范围：`gosrc/tools/` 全部源文件 vs `src/` TypeScript 原版

---

## 一、概述

`tools` 模块是 Claude Code Go 复刻版的**核心执行层**，负责将模型输出的 `tool_use` 块转化为真实的系统操作，再将操作结果打包为 `tool_result` 块返回给模型。

### 1.1 核心职责

| 职责 | 描述 |
|------|------|
| **工具注册与发现** | 统一接口注册所有内置工具，供 `loop` 层在对话开始时装配 |
| **工具调用分发** | 根据模型返回的 `tool_use.name` 路由到对应实现 |
| **参数解析与验证** | 将 JSON 原始输入强类型化，校验必填字段 |
| **权限检查** | 在执行前通过 `permissions.Checker` 判断操作是否被允许 |
| **危险命令拦截** | 双通道检测 shell 危险指令（正则 + AST） |
| **安全文件操作** | 原子写入、路径沙箱、符号链接解析防 TOCTOU |
| **MCP 桥接** | 透明转发 JSON-RPC 调用到外部 MCP 服务进程 |
| **LSP 集成** | 持久化 LSP 服务进程，提供 9 种代码智能操作 |
| **多 Agent 协调** | 通过 TeamManager 创建并调度子 Agent |
| **定时任务** | 内置 CronStore 实现 30s 精度的周期/单次任务 |

### 1.2 模块边界

```
┌──────────────────────────────────────────────────────────────┐
│                         loop/query.go                        │
│  assembleTools() → []Tool  ←→  ExecuteTools(toolUseBlock)   │
└─────────────────────────┬────────────────────────────────────┘
                          │  []types.Tool
          ┌───────────────▼────────────────┐
          │         tools 模块              │
          │  ┌──────────┐  ┌────────────┐  │
          │  │ 内置工具  │  │ MCP工具    │  │
          │  │ files    │  │ MCPManager │  │
          │  │ search   │  │ MCPTool    │  │
          │  │ lsp      │  └────────────┘  │
          │  │ tasks    │  ┌────────────┐  │
          │  │ team     │  │ Skill工具  │  │
          │  │ cron     │  │ SkillMgr  │  │
          │  │ ...      │  └────────────┘  │
          │  └──────────┘                  │
          └───────────────┬────────────────┘
                          │
          ┌───────────────▼────────────────┐
          │     permissions.Checker         │
          │     (allow / ask / deny)        │
          └────────────────────────────────┘
```

---

## 二、原版（TS）设计详情

### 2.1 Tool 接口（TypeScript）

原版 `src/Tool.ts` 定义了一个约含 **40 个属性/方法** 的富接口：

```typescript
interface Tool<Input, Output extends ToolOutput, P extends ToolPermission> {
  // 身份
  name: string
  description: string
  inputSchema: ZodType<Input>        // Zod 运行时校验

  // 执行
  call(input: Input, ctx: ToolUseContext): Promise<Output>
  checkPermissions(input: Input, ctx: ToolUseContext): Promise<PermissionResult>
  validateInput(input: Input): string | null

  // 渲染
  renderToolResultMessage(output: Output, ...): ReactNode
  renderToolUseMessage(input: Input, ...): ReactNode

  // 元数据
  isReadOnly(): boolean
  isDestructive(): boolean
  isConcurrencySafe(): boolean
  shouldDefer: boolean
  alwaysLoad: boolean
  maxResultSizeChars: number
  interruptBehavior(): InterruptBehavior

  // 权限
  userFacingName(): string
  getInputContext(input: Input): string
  backfillObservableInput(input: Input): Input
  toAutoClassifierInput(input: Input): unknown
}
```

**`buildTool()`** 为每个工具注入以下默认值（`TOOL_DEFAULTS`）：

| 属性 | 默认值 | 说明 |
|------|--------|------|
| `maxResultSizeChars` | `200_000` | 超限时截断 |
| `shouldDefer` | `false` | 是否延迟到用户确认 |
| `alwaysLoad` | `false` | 是否强制加入工具池 |
| `isConcurrencySafe()` | `false` | 是否允许并行执行 |
| `isReadOnly()` | `false` | 只读工具豁免权限弹窗 |
| `isDestructive()` | `false` | 破坏性操作需要特殊授权 |

### 2.2 权限模型

```
ToolUseContext
  ├── alwaysAllowRules   (自动批准，不弹窗)
  ├── alwaysDenyRules    (自动拒绝，无法覆盖)
  ├── alwaysAskRules     (每次弹窗询问)
  └── permissionMode     (default | acceptEdits | bypassPermissions | plan)

checkPermissions(input, ctx)
  └─→ "allow" | "ask" | "deny"
         └─→ deny: 返回 PermissionError，不执行 call()
         └─→ ask:  通过 IPC 弹出用户确认对话框
         └─→ allow: 直接执行
```

### 2.3 工具注册表（TS）

`src/tools.ts` 中的 `getAllBaseTools()` 返回约 **50 个**工具，其中部分被特性标志门控：

```typescript
// 工具池组装流程
getAllBaseTools()           // 50+ 工具
  → getTools(ctx)          // 按权限上下文过滤
  → assembleToolPool()     // 合并 MCP 工具，按名排序（提示缓存稳定性）
  → filterToolsByDenyRules() // 移除 deny 规则命中的工具
```

### 2.4 tool_use / tool_result 协议

```json
// 模型输出 → tool_use
{
  "type": "tool_use",
  "id": "toolu_01XYZ",
  "name": "Bash",
  "input": { "command": "ls -la" }
}

// 执行结果 → tool_result（返回给模型）
{
  "type": "tool_result",
  "tool_use_id": "toolu_01XYZ",
  "content": "total 48\ndrwxr-xr-x ...",
  "is_error": false
}
```

### 2.5 结果大小管理

TS 版每个工具声明 `maxResultSizeChars`，`call()` 返回后由框架自动截断。Go 版当前无统一截断机制，由各工具自行使用 `truncate()` 辅助函数处理。

---

## 三、Go 实现现状

### 3.1 Tool 接口（Go）

```go
// types/types.go
type Tool interface {
    Name()        string
    Description() string
    Schema()      JSONSchema
    Execute(ctx context.Context, input map[string]any) (ToolResult, error)
}

// 可选扩展（类型断言检测）
type ConcurrentTool interface {
    IsConcurrentSafe() bool
}
```

相比 TS 的 40 个属性，Go 接口仅暴露 **4 个核心方法**，其余能力通过独立机制（`permissions.Checker`、`dangerous.go`、`parse.go`）实现。

### 3.2 工具清单对照表

| TS 工具名 | Go 工具名 | 实现文件 | 状态 |
|-----------|-----------|----------|------|
| `bash` | `BashTool` | `files.go` | ✅ |
| `read_file` (FileRead) | `FileReadTool` | `files.go` | ✅ |
| `write_file` (FileWrite) | `FileWriteTool` | `files.go` | ✅ |
| `edit_file` (FileEdit) | `FileEditTool` | `files.go` | ✅ |
| `glob` | `GlobTool` | `search.go` | ✅ |
| `grep` | `GrepTool` | `search.go` | ✅ |
| `agent` / `computer_use` | `AgentTool` | `agent.go` | ✅ |
| `notebook_edit` | `NotebookEditTool` | `notebook.go` | ✅ |
| `notebook_read` | `NotebookReadTool` | `notebook.go` | ✅ |
| `todo_write` | `TodoWriteTool` | `todowrite.go` | ✅ |
| `web_search` | `WebSearchTool` | `web.go` | ✅ |
| `web_fetch` | `WebFetchTool` | `web.go` | ✅ |
| `mcp_tool` | `MCPTool` | `mcp_tools.go` | ✅ |
| `list_mcp_resources` | `ListMcpResourcesTool` | `mcp_tools.go` | ✅ |
| `read_mcp_resource` | `ReadMcpResourceTool` | `mcp_tools.go` | ✅ |
| `skill` (slash cmd) | `SkillTool` | `skill.go` | ✅ |
| `lsp_*` (9 ops) | `LSPTool` | `lsp.go` | ✅ |
| `worktree_enter` | `EnterWorktreeTool` | `worktree.go` | ✅ |
| `worktree_exit` | `ExitWorktreeTool` | `worktree.go` | ✅ |
| `config` | `ConfigTool` | `config.go` | ✅ |
| `plan_mode_enter` | `EnterPlanModeTool` | `planmode.go` | ✅ |
| `plan_mode_exit` | `ExitPlanModeTool` | `planmode.go` | ✅ |
| `task_create` | `TaskCreateTool` | `tasks.go` | ✅ |
| `task_get` | `TaskGetTool` | `tasks.go` | ✅ |
| `task_update` | `TaskUpdateTool` | `tasks.go` | ✅ |
| `task_stop` | `TaskStopTool` | `tasks.go` | ✅ |
| `task_output` | `TaskOutputTool` | `tasks.go` | ✅ |
| `cron_create` | `CronCreateTool` | `cron.go` | ✅ |
| `team_create` | `TeamCreateTool` | `team.go` | ✅ (Go 独有) |
| `team_delete` | `TeamDeleteTool` | `team.go` | ✅ (Go 独有) |
| `team_dispatch` | `TeamDispatchTool` | `team.go` | ✅ (Go 独有) |
| `send_message` | `SendMessageTool` | `team.go` | ✅ (Go 独有) |
| `tool_search` | — | — | ❌ 未实现 |
| `repl` | — | — | ❌ (内部工具) |
| `sleep` | — | — | ❌ 未实现 |
| `web_browser` | — | — | ❌ 未实现 |
| `powershell` | — | — | ❌ 未实现 |
| `push_notification` | — | — | ❌ 未实现 |
| `subscribe_pr` | — | — | ❌ 未实现 |
| `snip` | — | — | ❌ 未实现 |
| `workflow` | — | — | ❌ 未实现 |
| `verify_plan_execution` | — | — | ❌ 未实现 |

**覆盖率统计**：32 / 42 核心工具 ≈ **76%**（Go 独有工具 4 个另计）

### 3.3 工具分类

```
内置工具 (Built-in)
├── 文件系统      BashTool, FileReadTool, FileWriteTool, FileEditTool
├── 搜索          GlobTool, GrepTool
├── Web           WebFetchTool, WebSearchTool
├── 代码智能      LSPTool (9 operations)
├── Notebook      NotebookEditTool, NotebookReadTool
├── 任务管理      TaskCreate/Get/Update/Stop/Output, TodoWriteTool
├── 定时任务      CronCreateTool
├── 多Agent       TeamCreate/Delete/Dispatch, SendMessageTool, AgentTool
├── 工作区        EnterWorktreeTool, ExitWorktreeTool
├── 计划模式      EnterPlanModeTool, ExitPlanModeTool
└── 配置          ConfigTool

扩展工具 (Extension)
├── MCP           MCPTool, ListMcpResourcesTool, ReadMcpResourceTool
└── Skill         SkillTool
```

### 3.4 参数解析机制

```go
// parse.go — 泛型 JSON 往返强类型化
func parseInput[T any](input map[string]any) (T, error) {
    data, _ := json.Marshal(input)
    var result T
    err := json.Unmarshal(data, &result)
    return result, err
}

func parseInputOrError[T any](input map[string]any) (*T, *types.ToolResult) {
    result, err := parseInput[T](input)
    if err != nil {
        toolErr := types.ToolResult{Content: "Error: " + err.Error(), IsError: true}
        return nil, &toolErr
    }
    return &result, nil
}
```

所有工具 `Execute()` 入口均通过此模式完成参数绑定，避免手动类型断言。

### 3.5 主要工具参数对照

#### BashTool

| 参数 | TS 版 | Go 版 | 状态 |
|------|-------|-------|------|
| `command` | string (required) | string (required) | ✅ |
| `timeout` | number? | number? | ✅ |
| `description` | string? | — | ❌ 缺失 |
| `restart` | bool? | — | ❌ 缺失 |

#### FileReadTool

| 参数 | TS 版 | Go 版 | 状态 |
|------|-------|-------|------|
| `file_path` | string (required) | string (required) | ✅ |
| `offset` | number? | number? | ✅ |
| `limit` | number? | number? | ✅ |
| `pages` (PDF) | string? | — | ❌ 缺失 |
| 图像支持 | base64 data URI | — | ❌ 缺失 |

#### GrepTool

| 参数 | TS 版 | Go 版 | 状态 |
|------|-------|-------|------|
| `pattern` | required | required | ✅ |
| `path` | optional | optional | ✅ |
| `glob` | optional | optional | ✅ |
| `output_mode` | enum | enum | ✅ |
| `-i` | boolean | boolean | ✅ |
| `-C` | number | number | ✅ |
| `head_limit` | number | number | ✅ |
| `-A` / `-B` | number | — | ❌ 缺失 |
| `multiline` | boolean | — | ❌ 缺失 |
| `type` | string | — | ❌ 缺失 |
| `offset` | number | — | ❌ 缺失 |
| `-n` | boolean | — | ❌ 缺失 |

#### LSPTool

| 参数 | TS 版 | Go 版 | 状态 |
|------|-------|-------|------|
| `operation` | enum(20+) | enum(9 ops) | ⚠️ 部分 |
| `filePath` | required | required | ✅ |
| `line` | number (1-based) | number (1-based) | ✅ |
| `character` | number (0-based) | number (0-based) | ✅ |
| `newName` | rename only | rename only | ✅ |
| `query` | symbol search | symbol search | ✅ |
| `directory` | diag only | diag only | ✅ |

### 3.6 安全实现

Go 版在安全方面有超越 TS 的独立实现：

#### 危险命令双通道检测（`dangerous.go`）

```
DetectDangerousCommand(command string) (bool, string)
  │
  ├── Pass 1: 正则快速通道
  │     ├── rm -rf /  |  mkfs.*  |  dd if=
  │     ├── curl.*|.*bash  |  wget.*|.*bash
  │     ├── base64.*|.*bash  |  反弹 Shell 模式
  │     └── fork 炸弹  |  内核模块加载
  │
  └── Pass 2: Shell AST (mvdan.cc/sh/v3/syntax)
        ├── checkCallExpr  — 危险命令调用
        ├── checkPipeChain — curl|bash 等管道链
        ├── checkRedirects — 重定向到 /dev/sda 等
        └── checkScriptOneLiners — 压缩脚本检测
```

#### 文件路径沙箱（`files.go`）

```go
func checkAllowedPath(path string, allowedDirs []string) error {
    real, _ := filepath.EvalSymlinks(path)  // 解析符号链接防 escape
    for _, dir := range allowedDirs {
        realDir, _ := filepath.EvalSymlinks(dir)
        if strings.HasPrefix(real, realDir+"/") {
            return nil
        }
    }
    return ErrPathNotAllowed
}
```

#### 原子写入（`files.go`）

```go
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
    tmp := path + ".tmp." + randomSuffix()
    os.WriteFile(tmp, data, perm)
    f.Sync()              // fsync 防断电丢失
    os.Rename(tmp, path)  // 原子 rename
}
```

---

## 四、关键知识背景

### 4.1 Anthropic tool_use 协议

Claude 模型支持 Function Calling，通过在 `messages` API 的 `tools` 字段声明 JSON Schema，模型在需要时输出 `tool_use` 内容块：

```
对话轮次流程：

User message
    └─→ API (tools=[...])
          └─→ Assistant: [TextBlock?, ToolUseBlock+]
                └─→ stop_reason = "tool_use"
                      └─→ 执行工具 → ToolResult
                            └─→ User: [ToolResultBlock+]
                                  └─→ 继续对话...
```

`stop_reason = "end_turn"` 时对话结束，不再有工具调用。

### 4.2 JSON Schema 工具声明

每个工具通过 `Schema()` 暴露 JSON Schema，框架在每轮请求中将所有已注册工具的 Schema 发送给模型：

```go
type JSONSchema struct {
    Type        string         `json:"type"`
    Properties  map[string]any `json:"properties,omitempty"`
    Required    []string       `json:"required,omitempty"`
    Description string         `json:"description,omitempty"`
}
```

工具声明数量影响**提示缓存命中率**——TS 版通过 `assembleToolPool()` 按名排序确保工具列表稳定，保持缓存 key 不变。

### 4.3 权限模型

```
Go 权限检查链：

Execute(input)
  └─→ permissions.Checker.Check(op, path)
        ├── AllowList 匹配 → allow
        ├── DenyList 匹配  → deny (返回 ToolResult{IsError: true})
        └── 默认           → ask (CLI 交互确认)

TS 权限检查链：

call(input, ctx)
  └─→ checkPermissions(input, ctx)
        ├── alwaysAllow 规则  → allow
        ├── alwaysDeny 规则   → deny
        ├── alwaysAsk 规则    → ask
        └── PermissionMode 判断
              ├── bypassPermissions → allow all
              ├── plan              → deny destructive
              └── default           → 走 rules
```

### 4.4 MCP（Model Context Protocol）

MCP 基于 **JSON-RPC 2.0 over stdio**，允许外部进程动态注册工具：

```
Claude Code
  └─→ MCPManager.Connect(serverName, cmd)
        └─→ 启动子进程（30s 超时握手）
              └─→ stdio JSON-RPC
                    ├── initialize / initialized
                    ├── tools/list
                    ├── tools/call
                    └── resources/list, resources/read
```

Go 版 `MCPTool` 将工具调用透明转发，`ListMcpResourcesTool` 和 `ReadMcpResourceTool` 管理资源访问。

`ListMcpResourcesTool` 通过 services MCP manager 的连接状态快照工作：只处理
已经连接且声明 `capabilities.resources` 的服务器，不会从工具调用中启动 pending、
failed、needs-auth 或 disabled 服务器。资源目录按服务器预热/缓存，在连接关闭和
`notifications/resources/list_changed` 时失效；单服务器失败只记录到结果 metadata，
不会中断聚合。模型结果是严格的扁平数组（`uri`、`name`、可选 `mimeType`/
`description`、`server`），空结果使用 TS 固定提示文本。该工具同时通过可选工具契约
声明 read-only、并发安全、deferred search hint、严格输入/输出 schema 和 100000 字符
结果预算。

### 4.5 LSP 集成

Go 版维护持久化 LSP 服务进程，按语言隔离：

| 语言 | LSP 服务 | 命令 |
|------|----------|------|
| Go | gopls | `gopls serve` |
| TypeScript | typescript-language-server | `typescript-language-server --stdio` |
| JavaScript | typescript-language-server | `typescript-language-server --stdio` |

支持的 9 种操作：`hover`、`goto_definition`、`find_references`、`document_symbols`、`workspace_symbols`、`rename`、`prepare_rename`、`code_actions`、`diagnostics`

LRU 文档缓存上限：`maxOpenDocs = 200`

### 4.6 SkillManager 发现机制

```
SkillManager.Get(name)
  └─→ 扫描目录（优先级从高到低）：
        1. .claude/skills/{name}/
        2. ~/.claude/skills/{name}/
  └─→ loadSkillDir()
        ├── skill.json 存在 → Command 类型 (bash 执行，SKILL_ARGS 环境变量传参)
        └── SKILL.md 存在   → Prompt 类型 (返回文件内容 + args)
```

---

## 五、评估指标

### 5.1 工具覆盖率

| 分类 | TS 工具数 | Go 已实现 | 覆盖率 |
|------|-----------|-----------|--------|
| 文件系统 | 4 | 4 | 100% |
| 搜索 | 2 | 2 | 100% |
| Web | 2 | 2 | 100% |
| LSP | 1 (多操作) | 1 (9 ops) | ~60% ops |
| Notebook | 2 | 2 | 100% |
| 任务管理 | 5 | 6 (含TodoWrite) | 100%+ |
| MCP | 3 | 3 | 100% |
| Skill | 1 | 1 | 100% |
| 多Agent | 1 | 4 (Go 扩展) | 400% |
| 其他 | 10+ | 0–3 | < 30% |
| **合计** | **~42** | **32** | **~76%** |

### 5.2 参数 Schema 完整性

| 工具 | TS 参数数 | Go 参数数 | 缺失参数 | 完整度 |
|------|-----------|-----------|----------|--------|
| BashTool | 4 | 2 | description, restart | 50% |
| FileReadTool | 5 | 3 | pages, image | 60% |
| FileWriteTool | 2 | 2 | — | 100% |
| FileEditTool | 4 | 4 | — | 100% |
| GlobTool | 2 | 2 | — | 100% |
| GrepTool | 10 | 7 | -A, -B, multiline, type, offset, -n | 70% |
| WebFetchTool | 2 | 2 | — | 100% |
| WebSearchTool | 3 | 3 | — | 100% |
| NotebookEditTool | 5 | 5 | — | 100% |
| MCPTool | 3 | 3 | — | 100% |
| LSPTool | 7 | 7 | (ops 数量差距) | 85% |
| SkillTool | 2 | 2 | — | 100% |
| **平均** | — | — | — | **~88%** |

### 5.3 接口能力对照

| 能力 | TS Tool 接口 | Go Tool 接口 | 实现方式 |
|------|--------------|--------------|----------|
| 名称/描述/Schema | ✅ | ✅ | 接口方法 |
| 执行入口 | `call()` | `Execute()` | 接口方法 |
| 参数验证 | `validateInput()` (Zod) | `parseInputOrError()` | 独立函数 |
| 权限检查 | `checkPermissions()` | `permissions.Checker` | 独立模块 |
| 并发安全标记 | `isConcurrencySafe()` | `IsConcurrentSafe()` | 可选接口 |
| 只读标记 | `isReadOnly()` | `ReadOnlyTool` / `ToolMetadata` | ✅ 可选接口与契约元数据 |
| 破坏性标记 | `isDestructive()` | — | ❌ 未实现 |
| 结果大小限制 | `maxResultSizeChars` | `ToolContract.MaxResultSizeChars` | ✅ 统一结果预算元数据 |
| 结果渲染 | `renderToolResultMessage()` | — | ❌ (CLI 不需要) |
| 工具 UI 渲染 | `renderToolUseMessage()` | — | ❌ (CLI 不需要) |
| 延迟执行 | `shouldDefer` | `registry.DiscoveryMetadata` | ✅ 延迟发现 |
| 强制加载 | `alwaysLoad` | — | ❌ 未实现 |
| 中断行为 | `interruptBehavior()` | — | ❌ 未实现 |

### 5.4 安全机制对比

| 安全特性 | TS 版 | Go 版 |
|----------|-------|-------|
| 危险命令检测 | 基础正则 | ✅ 双通道（正则 + AST） |
| 路径沙箱 | allowedDirectories 检查 | ✅ + 符号链接解析 |
| 原子写入 | 无 | ✅ `atomicWriteFile()` |
| TOCTOU 防护 | 无 | ✅ `verifyOpenFd()` |
| SSRF 防护 | 无 | ✅ `validateURL()` |
| Shell 注入防护 | 无 | ✅ `SKILL_ARGS` 环境变量隔离 |

### 5.5 错误处理完整度

| 场景 | Go 处理 | 完整度 |
|------|---------|--------|
| 参数缺失/类型错误 | `parseInputOrError` 统一返回 `ToolResult{IsError}` | ✅ |
| 文件不存在 | `os.Stat` 错误传播 | ✅ |
| 权限拒绝 | `permissions.Checker` 返回 deny | ✅ |
| 命令执行超时 | `context.WithTimeout` | ✅ |
| 命令非零退出 | 捕获 stderr，`IsError: true` | ✅ |
| 网络超时 | 30s/15s context 控制 | ✅ |
| MCP 连接失败 | 30s 握手超时，返回错误 | ✅ |
| 结果超大截断 | 各工具独立截断（不统一） | ⚠️ |
| 上下文取消传播 | `ctx.Err()` 检查 | ✅ |

---

## 六、与原版的差距及后续规划

### 6.1 架构层面差距

| 差距 | 影响程度 | 说明 |
|------|----------|------|
| Tool 核心接口保持精简 | 🟢 低 | Go 通过 `ToolContractProvider`、read-only/并发可选接口和 registry discovery metadata 承载差异化能力 |
| 结果预算与工具自有截断并存 | 🟢 低 | `maxResultSizeChars` 由统一契约传递，特殊输出仍可保留工具级截断策略 |
| 无 Zod 等价物 | 🟡 中 | Go 的 JSON 往返解析缺少字段级别的自定义校验消息 |
| 无 UI 渲染接口 | 🟢 低 | CLI 场景不需要 React 渲染，影响有限 |

### 6.2 工具层面差距

| 缺失工具 | 优先级 | 补充理由 |
|----------|--------|----------|
| `BashTool.description` 参数 | 🔴 P0 | TS 用于区分人类可读说明与实际命令，影响权限审查 |
| `FileReadTool` 图像支持 | 🟡 P1 | 视觉任务需要读取图像文件 |
| `FileReadTool` PDF pages 参数 | 🟡 P1 | 大 PDF 分页读取 |
| `GrepTool` `-A`/`-B`/`multiline` | 🟡 P1 | 常用 grep 参数，影响使用体验 |
| `LSPTool` 操作集扩展（9→20+） | 🟡 P1 | code_action_resolve、inlay_hints 等高价值操作缺失 |
| `SleepTool` | 🟢 P2 | 简单，用于等待异步操作 |
| `ToolSearchTool` | 🟢 P2 | 工具自发现，大工具集时有用 |

### 6.3 后续规划路线图

```
Phase 1（接口强化）— 1-2 周
  ├── Tool 接口添加 IsReadOnly() / IsDestructive() 方法
  ├── 框架层实现统一 maxResultSizeChars 截断
  └── BashTool 补充 description 参数

Phase 2（工具补全）— 2-4 周
  ├── FileReadTool 支持图像（base64）和 PDF pages
  ├── GrepTool 补充 -A/-B/multiline/type/-n/offset
  ├── LSPTool 扩展到 15+ 操作（code_action_resolve, inlay_hints 等）
  └── SleepTool、ToolSearchTool 实现

Phase 3（能力扩展）— 4-8 周
  ├── WebBrowserTool（Playwright/CDP 集成）
  ├── PowerShellTool（Windows 支持）
  └── 权限模型增强（alwaysLoad、shouldDefer 支持）

Phase 4（质量保证）
  ├── 工具集成测试（工具调用 → API 端到端验证）
  ├── Schema 兼容性自动测试（Go JSON Schema vs TS Zod）
  └── 持续覆盖率监控（tools_metrics.py CI 集成）
```

### 6.4 Go 独有优势（不需追平）

以下 Go 特性已超越 TS 原版，应予保留：

- ✅ **危险命令 AST 检测** — 正则 + `mvdan.cc/sh/v3/syntax` 双通道，精度更高
- ✅ **原子文件写入** — `fsync` + `rename` 防止断电数据损坏
- ✅ **TOCTOU 保护** — `verifyOpenFd()` 防止文件路径替换攻击
- ✅ **SSRF 防护** — `validateURL()` 阻止服务端请求伪造
- ✅ **Team/Multi-Agent 工具集** — Go 版原生多 Agent 编排，TS 版无直接对应
- ✅ **CronStore** — 内置定时任务调度，TS 版依赖外部机制
