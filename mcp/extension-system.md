# 扩展系统层 — 设计参考文档

> 基于 Claude Code TypeScript 原版（`../../src`）提炼，定义 Go 实现的目标架构。  
> 覆盖四个子模块：**MCP 协议集成、Skills 插件系统、Registry 能力注册、Commands 命令系统**。

---

## 一、概述

扩展系统层是 Claude Code 架构的"外骨骼"，负责将 LLM 核心能力向外延伸：

```
用户输入
   │
   ├── /cmd  ──►  Commands（斜杠命令系统）
   │                  └── REPL 控制、会话管理、模型切换…
   │
   ├── 工具调用  ──►  Registry（能力注册中心）
   │                  ├── 内置工具（Bash、FileRead、Agent…）
   │                  ├── MCP 工具（外部服务器动态注入）
   │                  └── Skill 工具（插件提示模板）
   │
   └── 触发词  ──►  Skills（插件系统）
                      ├── 文件型 (.md / SKILL.md)
                      ├── 内置捆绑 (bundled)
                      └── MCP 发现 (feature-flagged)
```

| 模块 | 核心职责 | Go 包路径 | TS 对应路径 |
|------|---------|-----------|------------|
| MCP | 子进程 JSON-RPC 2.0 客户端，工具/资源发现 | `gosrc/mcp/` | `src/services/mcp/` |
| Skills | Markdown 插件加载、触发词检测、模板执行 | `gosrc/skills/` | `src/skills/` |
| Registry | 线程安全工具注册表、执行调度 | `gosrc/registry/` | 内嵌于 `src/Tool.ts` |
| Commands | 斜杠命令解析、REPL 控制指令 | `gosrc/commands/` | `src/commands/` + `src/commands.ts` |

---

## 二、原版（TS）设计详情

### 2.1 MCP 架构

原版使用官方 `@modelcontextprotocol/sdk` 包，完整实现 MCP 规范（协议版本 `2024-11-05`）。

**连接管理层（`src/services/mcp/`）**

```
useManageMCPConnections (React Hook)
   │
   ├── MCPConnectionManager (Context Provider)
   │     ├── reconnectMcpServer()
   │     └── toggleMcpServer()       ← 支持运行时开关单个 server
   │
   ├── client.ts
   │     ├── getMcpToolsCommandsAndResources()
   │     ├── fetchToolsForClient()
   │     ├── fetchCommandsForClient()   ← MCP 服务器还能贡献斜杠命令！
   │     ├── fetchResourcesForClient()
   │     └── reconnectMcpServerImpl()
   │
   └── config.ts
         ├── filterMcpServersByPolicy()
         ├── doesEnterpriseMcpConfigExist()
         ├── dedupClaudeAiMcpServers()
         └── isMcpServerDisabled()
```

**关键特性（原版有，Go 未实现）**：

| 特性 | 说明 |
|------|------|
| 动态连接管理 | 运行时添加/删除/重连 MCP 服务器，无需重启 |
| 服务器开关 | `toggleMcpServer()` 持久化 enable/disable 状态到配置 |
| MCP 命令 | 服务器可以贡献斜杠命令（`fetchCommandsForClient`） |
| MCP Prompts | `PromptListChangedNotification` 动态更新 |
| MCP Skills | `feature('MCP_SKILLS')` 使服务器可发布 Skill 提示 |
| 变更通知 | 监听 `ToolListChangedNotification`、`ResourceListChangedNotification` |
| 权限继电 | `ChannelPermissionNotification` 跨 channel 权限中继 |
| 企业配置 | 企业级 MCP 策略过滤，支持 strict 模式 |
| 严格模式 | `isStrictMcpConfig` 限制服务器范围 |
| 分析埋点 | 每次连接/重连记录 analytics 事件 |

**TS MCP 数据流**：

```
配置文件 (.claude/settings.json)
   │
   ▼
useManageMCPConnections
   │── 启动子进程 / stdio / SSE
   │── initialize() 握手
   │── tools/list → Tool[] → 注册到全局工具池
   │── resources/list → Resource[]
   │── fetchCommandsForClient → Command[] → 注册到命令系统
   │── notifications/initialized
   │
   ▼（运行时）
ToolListChangedNotification → 重新 fetchTools → 更新工具池
ResourceListChangedNotification → 重新 fetchResources
PromptListChangedNotification → 重新 fetchPrompts → 更新 Skills
```

### 2.2 Skills 架构

原版 Skills 分三大来源，接口统一为 `Command` 类型（复用斜杠命令体系）：

**来源一：Disk-based Skills（`src/skills/loadSkillsDir.ts`）**

```
扫描目录 (~/.claude/skills/, .claude/skills/, project/.claude/skills/)
   │
   ▼
parseFrontmatter() — YAML frontmatter 解析
   ├── name, description, aliases
   ├── allowed-tools: [Bash, FileRead, ...]   ← 限制可用工具子集
   ├── model: claude-opus-4-5                 ← 每个 skill 可指定模型
   ├── context: 'inline' | 'fork'             ← 执行模式
   ├── agent: <name>                          ← 可委托给特定 agent
   ├── effort: 1-5                            ← 推理深度
   ├── triggers: [keyword1, keyword2]         ← 自动触发词
   └── shell: !`cmd`                          ← 支持 shell 插值
   │
   ▼
createSkillCommand() → Command 对象
```

完整 Frontmatter 字段（Go 缺失）：

```yaml
---
name: my-skill
description: Does something useful
aliases: [ms, my_s]
allowed-tools: [Bash, FileRead, Grep]
model: claude-opus-4-5
context: fork          # 在子 agent 中执行，不污染主对话
agent: executor        # 委托给具名 agent
effort: 3              # 推理努力等级 1-5
triggers: [keyword, /regex/]
hooks:
  PreToolUse:
    - "echo before"
  PostToolUse:
    - "echo after"
---
Skill 提示内容，支持 $ARGUMENTS 变量替换
```

**来源二：Bundled Skills（`src/skills/bundled/`）**

通过 `registerBundledSkill()` 在代码中注册，16 个内置 Skill：

| Skill 名 | 功能 |
|----------|------|
| batch | 批量操作封装 |
| claudeApi | Claude API 调用辅助 |
| claudeApiContent | API 内容处理 |
| claudeInChrome | Chrome 集成 |
| debug | 调试辅助 |
| keybindings | 快捷键管理 |
| loop | 循环执行 (`/loop 5m /cmd`) |
| loremIpsum | 示例文本生成 |
| remember | 记忆持久化 |
| scheduleRemoteAgents | 远程 Agent 调度 |
| simplify | 代码简化 |
| skillify | 技能化封装 |
| stuck | 卡住恢复助手 |
| updateConfig | 配置更新 |
| verify | 验证工具 |
| verifyContent | 内容验证 |

**来源三：MCP Skills（`src/skills/mcpSkills.ts`，`feature('MCP_SKILLS')`）**

```
MCP 服务器 prompts/list RPC
   │
   ▼
fetchMcpSkillsForClient()
   │── parseSkillFrontmatterFields()   ← 解析服务器返回的 frontmatter
   │── createSkillCommand()            ← 转为 Command 对象
   ▼
注册到全局命令注册表（与 disk/bundled skill 同等地位）
```

**Skill 锁文件（`gosrc/skills-lock.json`）**

```json
{
  "version": 1,
  "skills": {
    "skill-name": {
      "source": "owner/repo",
      "sourceType": "github",
      "computedHash": "<sha256>"
    }
  }
}
```

类似 `package-lock.json`，记录从外部源安装的 skill 的版本哈希，防止篡改。当前锁文件仅有 1 个来自 GitHub 的 skill（`gangjing`）。

### 2.3 Registry 架构（TS）

TS 原版没有独立的 Registry 模块，工具管理分散于：
- `src/Tool.ts` — Tool 接口定义
- `src/tools/` — 各工具实现
- `src/utils/api.ts` — 请求时动态组装工具定义列表
- MCP client 连接后直接 push 进全局工具数组

**工具集（原版，约 40+ 个）**：

| 分类 | 工具 |
|------|------|
| 文件系统 | Read, Write, Edit, MultiEdit, Glob, Grep, LS |
| Shell | Bash |
| Agent | Agent（子 agent 调度） |
| 网络 | WebFetch, WebSearch |
| 任务 | TodoWrite, TodoRead |
| Notebook | NotebookRead, NotebookEdit |
| 记忆 | Remember（通过 skill 实现） |
| MCP | 动态注入（每个 server 的工具） |

### 2.4 Commands 架构（TS）

原版命令系统规模庞大，约 **85 个**命令（含目录式和文件式）：

**目录式命令（`src/commands/<name>/index.ts`）共 ~68 个**

核心命令分组：

| 分组 | 命令 |
|------|------|
| 会话管理 | clear, compact, context, resume, session, rewind |
| 认证 | login, logout |
| 配置 | config, model, output-style, theme, vim, color |
| 开发工具 | mcp, hooks, permissions, plan, files, diff |
| 工作流 | commit, branch, review, tasks, pr_comments |
| Skills | skills, keybindings |
| 帮助/信息 | help, cost, usage, status, stats, version |
| 系统 | exit, doctor, upgrade, terminalSetup |
| 实验性 | effort, passes, thinkback, voice, chrome |
| 团队协作 | agents, btw, share |

**文件式命令（`src/commands/*.ts`）约 15 个**：
`commit`, `commit-push-pr`, `review`, `security-review`, `init`, `version`, `advisor`, `brief` 等。

**命令 vs. Skill 区别**：
- **Command**：REPL 控制指令，以 `/` 开头，直接执行 JS 逻辑
- **Skill**：注入提示模板，触发 LLM 对话流，以 `/` 调用但通过 AI 执行

---

## 三、Go 实现现状

### 3.1 MCP 模块（`gosrc/mcp/mcp.go`）

**已实现**：

完整实现了 MCP 客户端侧核心协议，质量较高：

```go
// 核心结构
type ServerConfig struct { Command, Args, Env }
type MCPTool    { ToolName, OriginalName, ToolDesc, InputSchema, client }
type MCPResource { URI, Name, Description, MimeType }
type Client     { jc *jrpc2.Client, cmd *exec.Cmd, name string }
```

| 功能 | 实现状态 | 说明 |
|------|---------|------|
| 子进程启动 | ✅ 完整 | `exec.Command` + stdin/stdout pipe |
| Line 帧 JSON-RPC | ✅ 完整 | `channel.Line(stdout, stdin)` |
| initialize 握手 | ✅ 完整 | 含 `notifications/initialized` notify |
| 协议版本 | ✅ 完整 | `2024-11-05` |
| tools/list 分页 | ✅ 完整 | cursor-based 分页循环 |
| 工具名前缀 | ✅ 完整 | `mcp__{server}__{tool}` 命名，server/tool 两段按 TypeScript `normalizeNameForMCP` 规则规范化 |
| tools/call | ✅ 完整 | 含 `isError` 错误路径 |
| 多内容块合并 | ✅ 完整 | 仅合并 text 类型，忽略 image 等 |
| resources/list | ✅ 完整 | |
| resources/read | ✅ 完整 | 含多 content 合并 |
| 优雅关闭 | ✅ 完整 | 5s 超时后 Kill |
| 测试覆盖 | ✅ 完整 | 18 个测试用例，含 in-process jrpc2 server |
| in-process 测试模式 | ✅ 完整 | `NewClientFromChannel` |

**未实现（与原版差距）**：

| 功能 | 原版 | Go 状态 |
|------|------|---------|
| SSE 传输 | ✅ 支持 HTTP/SSE 服务器 | ❌ 仅 stdio |
| 动态重连 | ✅ `reconnectMcpServerImpl` | ❌ 无 |
| 服务器开关 | ✅ `toggleMcpServer` | ❌ 无 |
| MCP 命令 | ✅ 服务器贡献 `/cmd` | ❌ 无 |
| MCP Prompts | ✅ `prompts/list` | ❌ 无 |
| MCP Skills | ✅ feature-flagged | ❌ 无 |
| 变更通知 | ✅ `ToolListChangedNotification` 等 | ❌ 无 |
| 企业策略 | ✅ 多级过滤 | ❌ 无 |
| 权限中继 | ✅ `ChannelPermission` | ❌ 无 |
| 分析埋点 | ✅ analytics events | ❌ 无 |
| MCPManager 高层封装 | ✅ | ⚠️ 在 `tools/` 包中有 MCPManager |

**数据流（Go 当前）**：

```
.claude/settings.json
   │  (由 tools.MCPManager.LoadFromSettings 加载)
   ▼
mcp.NewClient(name, ServerConfig)
   │── exec.Command(config.Command, config.Args...)
   │── cmd.StdinPipe() + cmd.StdoutPipe()
   │── channel.Line(stdout, stdin)  ← newline-framed JSON-RPC 2.0
   │── initialize() ──► {protocolVersion, capabilities, clientInfo}
   │── notifications/initialized (fire-and-forget)
   ▼
client.ListTools() ──► []*MCPTool
   │── tools/list (cursor pagination)
   │── 每个 tool: ToolName = "mcp__{normalized-server}__{normalized-tool}"
   ▼
registry.Register(mcpTool)  ← 与内置工具同等注册
   ▼
LLM 工具调用 ──► MCPTool.Execute()
   │── client.CallTool(ctx, originalName, input)
   │── tools/call RPC
   ▼
types.ToolResult{Content: textContent}
```

### 3.2 Skills 模块（`gosrc/skills/skills.go`）

**已实现**：

```go
type Skill struct {
    Name, Description, Prompt string
    Triggers      []string
    RequiredTools []string
    FilePath      string
    compiledTriggers []*regexp.Regexp
}
type Loader struct { dirs []string, skills map[string]*Skill }
type SkillTool struct { Loader *Loader }  // 实现 types.Tool
```

| 功能 | 实现状态 | 说明 |
|------|---------|------|
| 文件型 skill (.md) | ✅ 完整 | 扫描目录 |
| 目录型 skill (SKILL.md) | ✅ 完整 | 含子目录 |
| 描述提取 | ✅ 完整 | 解析 `# Title` 行 |
| 触发词解析 | ✅ 完整 | `triggers: a, b` 行 |
| 正则触发 | ✅ 完整 | 预编译 `(?i)pattern` |
| 大小写不敏感匹配 | ✅ 完整 | |
| 模板替换 | ✅ 完整 | `{{ARGS}}`, `{{PROMPT}}` |
| SkillTool 工具接口 | ✅ 完整 | LLM 可通过工具调用 skill |
| 不存在目录静默跳过 | ✅ 完整 | |
| 测试覆盖 | ✅ 完整 | 17 个测试用例 |

**未实现（与原版差距）**：

| 功能 | 原版 | Go 状态 |
|------|------|---------|
| YAML frontmatter | ✅ 完整 YAML 解析 | ❌ 仅简单行解析 |
| `allowed-tools` 字段 | ✅ 限制工具子集 | ❌ 无 |
| `model` 字段 | ✅ per-skill 模型 | ❌ 无 |
| `context: fork` | ✅ 子 agent 隔离执行 | ❌ 无 |
| `agent:` 委托 | ✅ 委托给具名 agent | ❌ 无 |
| `effort:` 推理等级 | ✅ 1-5 | ❌ 无 |
| `aliases:` 别名 | ✅ 多别名触发 | ❌ 无 |
| `hooks:` 钩子 | ✅ Pre/PostToolUse | ❌ 无 |
| Shell 插值 | ✅ `!`cmd`` | ❌ 无 |
| $ARGUMENTS 替换 | ✅ 命名参数 | ⚠️ 仅 `{{ARGS}}`/`{{PROMPT}}` |
| 内置 Bundled Skills | ✅ 16 个 | ❌ 无 |
| MCP Skills 发现 | ✅ feature-flagged | ❌ 无 |
| 锁文件校验 | ✅ hash 验证 | ❌ 仅有锁文件格式 |
| Skill 搜索索引 | ✅ `EXPERIMENTAL_SKILL_SEARCH` | ❌ 无 |
| 多目录优先级 | ✅ 项目 > 用户 > 全局 | ⚠️ 支持多目录但无优先级 |

**数据流（Go 当前）**：

```
NewLoader(dirs...)
   ▼
loader.Load()
   │── os.ReadDir(dir)
   │── .md 文件 → parseSkillFile(path, name)
   │── 子目录/SKILL.md → parseSkillFile(skillFile, dirName)
   ▼
parseSkillFile(path, name)
   │── os.ReadFile(path)
   │── 扫描行: "# " → Description
   │── 扫描行: "triggers:" → Triggers
   │── regexp.Compile("(?i)" + trigger) → compiledTriggers
   ▼
loader.skills[name] = &Skill{...}

触发检测:
loader.DetectTrigger(input)
   │── strings.Contains(lower, pattern)  // 快速路径
   └── compiledTriggers[i].MatchString   // 慢速路径

SkillTool.Execute(ctx, {"skill": "name", "args": "..."})
   │── loader.Get(skillName)
   │── strings.ReplaceAll(prompt, "{{ARGS}}", args)
   └── ToolResult{Content: expandedPrompt}
```

### 3.3 Registry 模块（`gosrc/registry/registry.go` + `registry_setup.go`）

**已实现（最完整的模块）**：

```go
type Registry struct {
    mu    sync.RWMutex
    tools map[string]types.Tool
    order []string
}
```

| 功能 | 实现状态 | 说明 |
|------|---------|------|
| 线程安全注册/读取 | ✅ 完整 | RWMutex |
| 保序迭代 | ✅ 完整 | `order []string` 维护插入顺序 |
| 重复注册覆盖 | ✅ 完整 | 不 panic，静默替换（用于 AgentTool 深度递增）|
| 工具执行 | ✅ 完整 | `ExecuteTool` + `ExecuteToolWithError` |
| 业务错误 vs 基础设施错误区分 | ✅ 完整 | `ExecuteToolWithError` |
| API 定义生成 | ✅ 完整 | `Definitions()` |
| 克隆 | ✅ 完整 | `Clone()` 浅拷贝，支持子 agent 替换 AgentTool |
| 并发安全测试 | ✅ 完整 | 10读+10写并发测试 |
| 测试覆盖 | ✅ 完整 | 15 个测试用例 |

**`registry_setup.go` 注册的工具清单**（约 38 个）：

| 分类 | 工具 | 数量 |
|------|------|------|
| 文件/Shell | Bash, FileRead, FileWrite, FileEdit, Glob, Grep | 6 |
| Agent | AgentTool | 1 |
| 任务 | TaskCreate/List/Update/Get/Stop/Output, TodoWrite | 7 |
| 计划 | EnterPlanMode, ExitPlanMode | 2 |
| 用户交互 | AskUserQuestion | 1 |
| 网络 | WebFetch, WebSearch | 2 |
| Cron | CronCreate, CronDelete, CronList | 3 |
| Git Worktree | EnterWorktree, ExitWorktree | 2 |
| 配置 | Config | 1 |
| MCP | MCPTool, ListMcpResources, ReadMcpResource | 3 |
| LSP | LSPTool | 1 |
| 团队 | SendMessage, TeamCreate, TeamDelete, TeamDispatch | 4 |
| Skill | SkillTool | 1 |
| Notebook | NotebookEdit | 1 |
| 杂项 | Brief, ToolSearch, SyntheticOutput, RemoteTrigger | 4 |
| **总计** | | **38** |

**注意**：`CronStore` 的执行回调尚未接入 REPL 查询循环，仅打印警告日志：
```go
cronStore.Start(func(job *tools.CronJob) {
    fmt.Fprintf(os.Stderr, "[cron] WARNING: Job %s fired but execution is not yet implemented. Prompt: %s\n", job.ID, job.Prompt)
})
```

### 3.4 Commands 模块（`gosrc/commands/`）

**已实现（共 8 个内置命令）**：

```go
type Command interface {
    Name() string
    Aliases() []string
    Description() string
    Execute(ctx *Context, args string) error
}
```

| 命令 | 别名 | 功能 | 实现状态 |
|------|------|------|---------|
| `/help` | — | 列出所有命令 | ✅ |
| `/exit` | `/quit` | 退出 REPL（返回 `ErrExit`）| ✅ |
| `/clear` | — | 清空对话历史 | ✅ |
| `/compact` | — | 手动触发上下文压缩 | ✅ 含 tool_result 完整性保护 |
| `/model` | — | 显示/切换模型 | ✅ |
| `/cost` | — | 显示累计 token 用量 | ✅ |
| `/version` | — | 打印应用版本 | ✅ |
| `/session` | — | 会话管理 (show/list/load) | ✅ |

**`/compact` 实现细节（Go 独有设计）**：

```
/compact 触发
   │
   ├── 若历史为空 → 提示 "Nothing to compact"
   ├── 若历史 ≤ 4 条 → 提示 "Already small enough"
   │
   ▼
keepFrom = len(msgs) - 4  // 至少保留最后 4 条
   │
   ▼  // 向前扫描：避免孤立的 tool_result
while keepFrom > 0:
    if msgs[keepFrom].Role == "user" && hasToolResultContent(msg):
        keepFrom--  // 回退到包含完整 tool_use/tool_result 配对的 assistant 消息
    else: break
   │
   ▼
SetMessages(msgs[keepFrom:])
// 输出: "Context compacted: kept N of M messages"
```

**Go Commands 注册架构**：

```go
// commands.Registry 与 registry.Registry 完全独立
type Registry struct {
    commands map[string]Command  // 按名/别名索引
    ordered  []Command           // 插入顺序（用于 /help 排序）
}
// 解析: "/cmd args" → Parse() → (Command, args string)
// 快速查找: Find("/cmd") 自动剥离前缀 "/"
// 注意: 重复注册会 panic（与 Tool Registry 静默覆盖不同）
```

**测试覆盖**：命令解析、别名、`/` 前缀剥离、内置命令行为，共约 20 个测试用例。

### 3.5 状态总览表格

| 模块 | 核心实现 | 测试覆盖 | 原版功能覆盖率 | 生产就绪度 |
|------|---------|---------|-------------|----------|
| MCP | ✅ 完整客户端 | ✅ 18 用例 | ~55% | 🟡 基础可用 |
| Skills | ✅ 基础加载 | ✅ 17 用例 | ~35% | 🟡 基础可用 |
| Registry | ✅ 完整 | ✅ 15 用例 | ~90% | 🟢 生产就绪 |
| Commands | ✅ 8/85 命令 | ✅ 20 用例 | ~9% | 🟠 严重不足 |

### 3.6 扩展系统数据流总图

```
启动阶段
──────────────────────────────────────────────────────────
SetupRegistry(provider, cwd, allowedDirs)
   │
   ├── 注册内置工具（38 个）
   │     ├── tools.MCPManager.LoadFromSettings(".claude/settings.json")
   │     │      └── 为每个 MCP 服务器:
   │     │            mcp.NewClient(name, config)
   │     │               └── initialize 握手
   │     └── tools.NewMCPTool(mcpManager) → registry.Register
   │
   └── skills.NewLoader(dirs).Load()
         └── 扫描 .md / SKILL.md → Skill 对象
               └── SkillTool → registry.Register

运行阶段（REPL 循环）
──────────────────────────────────────────────────────────
用户输入
   │
   ├─ IsCommand("/xxx") ?
   │     ▼
   │   commands.Registry.Parse("/cmd args")
   │     └── cmd.Execute(ctx, args)
   │           ├── /clear → QueryLoop.SetMessages(nil)
   │           ├── /model → QueryLoop.SetModel(name)
   │           ├── /compact → 裁剪 + tool_result 修复
   │           └── /session load → SessionStore.Load + SetMessages
   │
   └─ 普通输入 → LLM API 调用
         │
         LLM 返回 tool_use 块
         ▼
       registry.ExecuteTool(name, input)
         ├── Bash/FileRead/... → 直接执行
         ├── mcp__xxx__yyy     → dynamic MCP tool / compat MCPTool.Execute
         │     └── client.CallTool(ctx, originalName, input)
         │           └── tools/call JSON-RPC → MCP 子进程
         └── Skill             → SkillTool.Execute
               └── 返回提示模板内容（注入 LLM 上下文）
```

---

## 四、关键知识背景

### 4.1 MCP 协议规范（Model Context Protocol）

MCP 是 Anthropic 提出的标准化 LLM 工具服务协议，版本 `2024-11-05`。

**传输层**：
- **stdio**：客户端启动子进程，通过 stdin/stdout 通信（Go 已实现）
- **HTTP/SSE**：客户端连接 HTTP 服务器，通过 Server-Sent Events 接收（Go 未实现）

**RPC 层**：JSON-RPC 2.0（`id` 字段区分请求/通知）

**核心 RPC 方法**：

| 方法 | 方向 | 作用 | Go 实现 |
|------|------|------|---------|
| `initialize` | C→S | 协议握手 | ✅ |
| `notifications/initialized` | C→S | 握手完成通知 | ✅ |
| `tools/list` | C→S | 发现工具（含分页） | ✅ |
| `tools/call` | C→S | 调用工具 | ✅ |
| `resources/list` | C→S | 发现资源 | ✅ |
| `resources/read` | C→S | 读取资源 | ✅ |
| `prompts/list` | C→S | 发现提示模板 | ❌ |
| `prompts/get` | C→S | 获取提示内容 | ❌ |
| `ToolListChangedNotification` | S→C | 工具变更推送 | ❌ |
| `ResourceListChangedNotification` | S→C | 资源变更推送 | ❌ |
| `PromptListChangedNotification` | S→C | 提示变更推送 | ❌ |

**工具调用响应格式**：

```json
{
  "content": [
    {"type": "text", "text": "结果文本"},
    {"type": "image", "data": "base64...", "mimeType": "image/png"}
  ],
  "isError": false
}
```

Go 实现仅合并 `text` 类型内容，`image`/`resource` 等富媒体类型被丢弃。

**游标分页**：`tools/list` 可能返回 `nextCursor`，需循环请求直到 `nextCursor == ""`。Go 已正确实现。

### 4.2 JSON-RPC 2.0 要点

```json
// 请求（有 id）
{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}

// 响应
{"jsonrpc": "2.0", "id": 1, "result": {"tools": [...]}}

// 通知（无 id，fire-and-forget）
{"jsonrpc": "2.0", "method": "notifications/initialized"}
```

Go 使用 `github.com/creachadair/jrpc2`，通过 `channel.Line` 实现换行分帧（newline-delimited JSON）。

### 4.3 插件系统设计模式

**Go 采用策略模式**：

```
types.Tool interface
    Name() string
    Description() string
    Schema() types.JSONSchema
    Execute(ctx, input) (ToolResult, error)
         ▲
    ┌────┴────┬──────────┬──────────┬──────────┐
 BashTool  MCPTool  SkillTool  AgentTool  ...
```

所有工具实现同一接口，Registry 无需知道具体类型——这是典型的**依赖倒置**。

**错误分层**：

```
ExecuteToolWithError() ──► (ToolResultBlock, error)
    │
    ├── error != nil  → 基础设施错误（RPC 断连、nil dereference）
    │                    调用方应中断本轮对话
    └── result.IsError → 业务错误（工具执行失败、权限拒绝）
                          透传给 LLM，让其自动重试或调整策略
```

### 4.4 Skills 前置模板注入机制

SkillTool 执行后返回的是**提示内容字符串**，不是执行结果。调用侧需要将此内容注入 LLM 上下文（作为 system message 或 user message 前缀），触发 LLM 按 skill 指示执行。

这与 MCP tools 的"执行-返回结果"模式不同：

```
MCP Tool: LLM → tools/call → 子进程执行 → 返回结果 → LLM 继续
Skill:    LLM → Skill工具 → 返回提示模板 → 注入上下文 → LLM 按模板行动
```

### 4.5 Registry Clone 模式（子 Agent 深度控制）

```go
childReg := parentReg.Clone()
// 替换 AgentTool 为深度递增版本（防止无限递归）
childReg.Register(&tools.AgentTool{
    Provider: p,
    Registry: childReg,
    Depth:    parent.Depth + 1,
})
```

`Clone()` 是浅拷贝——所有工具共享，但 AgentTool 被替换。父注册表不受影响。

### 4.6 Commands Registry 与 Tool Registry 设计差异

| 特性 | Tool Registry | Commands Registry |
|------|--------------|-------------------|
| 重复注册策略 | 静默覆盖 | **PANIC**（确保唯一性）|
| 线程安全 | ✅ RWMutex | 仅初始化时写入 |
| 克隆支持 | ✅ Clone() | ❌ |
| 别名支持 | ❌ | ✅ Aliases() |
| 错误语义 | 双层（infra/business） | 单层（error） |

---

## 五、评估指标

### 5.1 MCP 协议覆盖率

| 指标 | 值 |
|------|-----|
| 已实现 RPC 方法 | 6 / 11 = **54.5%** |
| 传输层支持 | 1 / 2 = **50%**（仅 stdio）|
| 内容类型支持 | 1 / 3 = **33%**（仅 text）|
| 动态管理能力 | 0 / 5 = **0%**（重连/开关/通知/MCP命令/MCP Skills）|
| **MCP 加权覆盖率** | **~38%** |

### 5.2 命令覆盖率

| 指标 | Go | TS |
|------|----|----|
| 总命令数 | **8** | **~85** |
| 总覆盖率 | **9.4%** | — |
| 无条件命令覆盖率 | 8/35 = **23%** | — |
| 已覆盖的核心命令 | help, exit, clear, compact, model, cost, version, session | — |
| 缺失的高优先级命令 | login, mcp, config, memory, doctor, init, skills | — |

### 5.3 Skills 加载兼容性

| 指标 | Go | TS |
|------|----|----|
| Frontmatter 字段支持 | 2 / ~12 = **17%** | 100% |
| Skill 来源支持 | 2 / 3 = **67%**（disk+dir，缺 MCP）| 100% |
| 内置 Bundled Skills | 0 / 16 = **0%** | 100% |
| 模板变量支持 | `{{ARGS}}`, `{{PROMPT}}` | `$ARGUMENTS` + 完整命名参数 |
| **Skills 核心路径覆盖率** | **~35%** | — |

### 5.4 Registry 功能完整度

| 指标 | Go | TS |
|------|----|----|
| 注册工具数 | **38** | **~45**（含动态 MCP）|
| 核心接口完整性 | **~90%** | — |
| 线程安全 | ✅ | N/A（单线程 React）|
| Clone 隔离 | ✅ | 不适用 |
| Cron 执行连通 | ❌（存储有，执行未连通）| N/A |
| **Registry 综合完整度** | **~55%** | — |

### 5.5 扩展系统层整体覆盖率

```
模块        覆盖率    可视化
─────────────────────────────────────────
MCP         ~38%   ████████░░░░░░░░░░░░
Skills      ~35%   ███████░░░░░░░░░░░░░
Commands    ~9%    ██░░░░░░░░░░░░░░░░░░
Registry    ~55%   ███████████░░░░░░░░░

加权均值    ~35%
（MCP×30% + Skills×25% + Commands×20% + Registry×25%）
```

---

## 六、与原版的差距及后续规划

### 6.1 优先级矩阵

```
高影响 + 低难度（立即执行）
─────────────────────────────
[CMD-1]  /login + /logout 命令
[CMD-2]  /mcp 命令（查看已连接的 MCP 服务器）
[CMD-3]  /config 命令（管理配置）
[CMD-4]  /memory 命令（查看/编辑记忆文件）
[SKL-1]  完整 YAML frontmatter 解析（allowed-tools, model, aliases）

高影响 + 中等难度（下一迭代）
─────────────────────────────
[MCP-1]  HTTP/SSE 传输层支持
[MCP-2]  服务器端变更通知（ToolListChanged 等）
[MCP-3]  运行时动态重连 / 服务器开关
[SKL-2]  context: fork — 子 agent 隔离执行
[CMD-5]  /doctor、/init、/skills、/status 等核心命令（共 ~15 个）
[REG-1]  Cron 执行回调接入 REPL 查询循环

中等影响（后续版本）
─────────────────────────────
[MCP-4]  prompts/list + prompts/get RPC
[MCP-5]  MCP 命令贡献（fetchCommandsForClient）
[SKL-3]  MCP Skills 发现（feature-flagged）
[SKL-4]  内置 Bundled Skills 移植（loop, simplify, verify 等高价值 skill）
[SKL-5]  Skill 锁文件完整校验（hash 验证）
[CMD-6]  /compact 与 TS 原版完整压缩流水线对齐

低优先级
─────────────────────────────
[MCP-6]  企业级 MCP 策略过滤
[MCP-7]  权限中继（ChannelPermission）
[SKL-6]  Shell 插值（`!`cmd``）
[SKL-7]  Skill 搜索索引
[CMD-7]  实验性/团队协作命令（btw, agents, share 等）
```

### 6.2 架构层面的主要差距

**1. MCP 服务器生命周期层已迁入 services/mcp**

原版有完整的连接生命周期：connect → watch notifications → reconnect → toggle disable/enable → disconnect。  
Go 当前由 `services/mcp.Manager` 覆盖这些生产路径，并通过 registry setup 将健康状态、动态工具刷新和 list_changed 失效桥接到工具层。

**当前实现**：生命周期能力已经迁到 `services/mcp.Manager`，由 reconnect/session/health 组件处理断线恢复、缓存清理和状态快照；legacy `tools.MCPManager` 仅作为兼容回退路径保留。

**2. Skills 执行模式单一**

原版支持 `context: inline`（当前对话）和 `context: fork`（子 agent 隔离）。Go 当前所有 skill 都以 inline 方式返回提示内容。  
缺少 fork 模式意味着长 skill（如 `ultrawork`）会污染主对话上下文。

**建议**：在 `SkillTool.Execute` 中检查 `skill.Context` 字段，若为 `fork` 则通过 AgentTool 启动子 agent。

**3. 命令系统缺少核心交互命令**

`/login`/`/mcp`/`/config`/`/doctor` 是用户最常用的命令，缺失严重影响可用性。

**建议**：按优先级 CMD-1 至 CMD-5 逐批实现，每批约 5 个命令。

**4. TS 命令中 Skill 与 Command 界限模糊**

原版中大量 TS 命令（如 `/review`、`/commit`、`/branch`）实际上是有 JS 逻辑的"重型命令"，而 Go 版只有轻量 REPL 控制命令。  
这些重型命令在 Go 中更适合实现为 **Skill**（注入提示模板 + 允许工具执行），而非直接 Go 函数。

**建议**：`/review`、`/commit`、`/branch` 等工作流命令优先用 Skill 实现，而非 Go 代码。

### 6.3 里程碑规划

```
v0.3（当前）
└── MCP 客户端 stdio ✅ | Skills 基础加载 ✅ | Registry 完整 ✅ | 8 个命令 ✅

v0.4（下一版本）
├── HTTP/SSE MCP 传输
├── MCP 动态重连 + 变更通知
├── YAML frontmatter 完整支持（allowed-tools, model, aliases, context）
└── +15 核心命令（login, mcp, config, memory, doctor, init, skills, status...)

v0.5
├── Skill fork 执行模式
├── 内置 Bundled Skills（8 个高价值 skill）
├── MCP prompts/list + MCP Skills
├── Cron 执行连通
└── +15 工作流命令（review, commit, branch, compact-full...）

v1.0（功能对等目标）
├── MCP 全功能（企业策略、权限中继、Strict 模式）
├── Skills 全功能（MCP Skills、搜索索引、锁文件校验）
└── 命令系统 50%+ 覆盖率
```

---

*文档由 Claude Code 文档工程师生成 · 数据截止日期：2026-04-05*
