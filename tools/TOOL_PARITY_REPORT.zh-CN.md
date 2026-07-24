# 工具一致性报告

英文版：[TOOL_PARITY_REPORT.md](./TOOL_PARITY_REPORT.md)

日期：2026-04-17  
仓库：`gosrc` 对比 `../src`

## 报告范围

本报告对比了原版 TypeScript 工具面 `../src/tools.ts` 与 Go 复刻版 `registry_setup.go`。

报告将一致性拆成三层：

1. 接口一致性
   模型可见的工具注册、工具名、输入 schema。
2. 运行时一致性
   背后实现、外部副作用、持久化模型、异步行为、外部集成。
3. 输出一致性
   工具结果的内容形态与语义是否一致。

重要说明：当前报告里的 `32/32 工具在 Surface 层达到 ★★★★★`，仅表示当前环境下的第 1 层已经对齐，不代表所有工具已经在运行时和输出层面完全同构。

## 当前环境快照

本次基线是在以下环境下计算出来的：

- `USER_TYPE=`（external build）
- `NODE_ENV=`
- `ENABLE_LSP_TOOL=`
- `ENABLE_TOOL_SEARCH=`
- `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=`
- `EMBEDDED_SEARCH_TOOLS=`
- `CLAUDE_CODE_ENABLE_TASKS=`
- `AGENT_TRIGGERS=`
- `AGENT_TRIGGERS_REMOTE=`

在这个环境下，激活中的基础工具集合为 32 个。

## 验证结果

- `python3 tools/scripts/tool_parity_audit.py`
  结果：TS 基础工具 32 个，Go 注册工具 32 个，当前激活的 32 个工具在 Surface 层全部达到 `★★★★★`。
- `go test ./tools ./registry ./permissions ./loop -count=1`
  结果：通过。
- `go test ./... -count=1`
  结果：所有工具相关包通过；`tui` 仍有与本次工具对齐无关的既有失败，位于 `markdown_test.go` 与 `stream_renderer_test.go`。

## 总结

- 当前激活工具集的接口一致性已经达到 `★★★★★`。
- 当前环境下的 gating 也已经对齐。
- 但完整的语义一致性还没有达到 `★★★★★`。
- 主要仍有运行时或输出差异的工具集中在：
  - `Agent`
  - `Bash`
  - `SendUserMessage`
  - `ExitPlanMode`
  - `Read`
  - `RemoteTrigger`
  - `SendMessage`
  - `TaskCreate` / `TaskGet` / `TaskList` / `TaskOutput` / `TaskStop` / `TaskUpdate`
  - `TodoWrite`
  - `ToolSearch`
  - `WebFetch`
  - `WebSearch`

一句话结论：

- 如果问题是“模型现在看到的工具名和入参是否一致”，答案是：一致。
- 如果问题是“每个工具是否已经端到端实现得和原版一模一样”，答案是：还没有。

## 一致性标记说明

- `★★★★★`：这一层可以视为基本等价。
- `★★★★☆`：高度接近，只剩轻微简化或少量缺项。
- `★★★☆☆`：意图一致、接口可用，但仍有重要差异。
- `★★☆☆☆`：接口面或总体方向相同，但语义差异仍然很大。
- `★☆☆☆☆`：缺失、仅占位、或基本不能视作等价。

维度含义：

- `Surface`：按工具名、注册情况、输入 schema 评星。
- `Runtime`：按执行语义、持久化模型、副作用、异步行为、外部集成评星。
- `Output`：按 tool-result 的结构和语义评星。

## 当前激活工具矩阵

| 工具 | 工具描述 | 参数 | Go 实现概要 | 输出格式 | Surface | Runtime | Output | 差异 / 原因 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `Agent` | 启动子 Agent | `description`, `prompt`, `subagent_type?`, `model?`, `run_in_background?`, `name?`, `team_name?`, `mode?`, `isolation?`, `cwd?` | 同步启动一个嵌套 Go loop，带深度限制 | 纯文本或错误文本 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版支持更完整的异步、后台、remote、team 流程和结构化结果状态；Go 当前只返回文本，并直接拒绝后台模式 |
| `AskUserQuestion` | 结构化向用户提问 | `questions: [{header, question, options, multiSelect?}]` | 终端交互式提问并校验选择 | JSON 字符串答案 | `★★★★★` | `★★★★☆` | `★★★☆☆` | 核心交互逻辑接近；Go 返回的是序列化 JSON 文本，不是原版 SDK 侧更丰富的 typed result 流程 |
| `Bash` | 执行 shell 命令 | `command`, `timeout?`, `description?`, `run_in_background?`, `dangerouslyDisableSandbox?` | 通过 `bash -c` 执行命令，附带简化版沙箱与危险命令拦截 | JSON 字符串，包含 `stdout`、`stderr?`、`exit_code`、`error?` | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | 原版支持后台任务、sed-edit 审批链路、更完整的权限/只读校验和更复杂的 shell 语义；Go 目前只覆盖基础执行面 |
| `SendUserMessage` | 给用户发送简短消息 | `message`, `attachments?`, `status?` | CLI 场景下直接透传消息；附件和状态目前不参与实际逻辑 | 纯文本 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版是更完整的用户消息能力；Go 目前只是把消息原样回显 |
| `CronCreate` | 创建 cron 触发器 | `cron`, `prompt`, `recurring?`, `durable?` | 将任务写入 Go cron store 并调度 | 纯文本摘要 | `★★★★★` | `★★★☆☆` | `★★★☆☆` | 任务会被跟踪和调度，但触发后的执行还没有真正回接主查询循环，当前 fire callback 只做日志 |
| `CronDelete` | 删除 cron 触发器 | `id` | 从 cron store 中移除任务 | 纯文本摘要 | `★★★★★` | `★★★☆☆` | `★★★☆☆` | 与 `CronCreate` 同类限制：调度结构存在，端到端执行链路不完整 |
| `CronList` | 列出 cron 触发器 | 无 | 读取 cron store 并格式化输出 | 纯文本列表 | `★★★★★` | `★★★☆☆` | `★★★☆☆` | 列表行为没问题，但触发执行路径仍是简化版 |
| `EnterPlanMode` | 进入 plan mode 并生成计划文件 | 无 | 创建 `.claude/plans/<timestamp>.md` 并切换共享 plan state | 纯文本说明 | `★★★★★` | `★★★★☆` | `★★★☆☆` | 核心状态切换一致，但原版还有更多 UI / system prompt 层面的联动 |
| `EnterWorktree` | 创建隔离 git worktree | `name?` | 调用 `git worktree add` 并跟踪共享 worktree 状态 | 纯文本摘要 | `★★★★★` | `★★★★☆` | `★★★★☆` | 对当前基线来说，核心行为已足够接近 |
| `ExitPlanMode` | 退出 plan mode | `allowedPrompts?: [{tool:"Bash", prompt:string}]` | 读取计划文件并退出 plan mode | 纯文本计划摘要 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版包含审批流程、teammate/leader 计划流转、request ID 和权限编排；Go 当前只是退出并打印计划 |
| `ExitWorktree` | 保留或删除 worktree | `action`, `discard_changes?` | 清理状态或通过 git 删除 worktree / branch | 纯文本摘要 | `★★★★★` | `★★★★☆` | `★★★★☆` | 核心 keep/remove 流程已具备 |
| `Edit` | 文件内容替换 | `file_path`, `old_string`, `new_string`, `replace_all?` | 读文件、替换文本、原子写回 | JSON 字符串摘要 | `★★★★★` | `★★★★☆` | `★★★☆☆` | 主要意图已对齐，但原版有更多 UX / 结果元信息与编辑分析 |
| `Read` | 读取文件内容 | `file_path`, `offset?`, `limit?`, `pages?` | 按文本行范围读取并带行号返回 | 带行号的纯文本，或 warning 文本 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版还支持 notebook、图片、PDF、token/byte 限流、读取缓存和更丰富的结果类型；Go 目前只实现纯文本分段读取 |
| `Write` | 写入文件内容 | `file_path`, `content` | 原子覆盖写入，并做 allowed-dir 校验 | JSON 字符串摘要 | `★★★★★` | `★★★★☆` | `★★★☆☆` | 核心写入路径没问题，但原版保留了更多 editor / file-history 语义 |
| `Glob` | 按 glob 找文件 | `pattern`, `path?` | 遍历文件系统并执行 glob 匹配，对大量结果做截断 | 纯文本文件列表 | `★★★★★` | `★★★★☆` | `★★★★☆` | 对当前基线来说，核心用途已经对齐 |
| `Grep` | 搜索文件内容 | `pattern`, `path?`, `glob?`, `output_mode?`, `-B`, `-A`, `-C`, `context`, `-n`, `-i`, `type?`, `head_limit?`, `offset?`, `multiline?` | 纯 Go 递归扫描，支持 regex、上下文、偏移和文件类型过滤 | 纯文本结果列表 | `★★★★★` | `★★★☆☆` | `★★★☆☆` | API surface 已对齐，但原版依赖 ripgrep 语义，行为与性能边界更精确 |
| `ListMcpResourcesTool` | 列出 MCP 资源 | `server?` | 列出配置中的 server，或查询已连接 server | 纯文本列表 | `★★★★★` | `★★★★☆` | `★★★★☆` | 对当前内建 MCP 资源列出路径来说，行为基本接近 |
| `NotebookEdit` | 编辑 notebook 单元格 | `notebook_path`, `cell_id?`, `new_source`, `cell_type?`, `edit_mode?` | 解析 `.ipynb`，替换 / 插入 / 删除 cell，原子写回 | 纯文本状态 | `★★★★★` | `★★★☆☆` | `★★☆☆☆` | 原版返回更丰富的 notebook 结果元信息和 attribution 信息；Go 目前只返回简单状态文本 |
| `ReadMcpResourceTool` | 读取单个 MCP 资源 | `server`, `uri` | 连接 MCP server 并读取资源内容 | 纯文本内容 | `★★★★★` | `★★★★☆` | `★★★★☆` | 对当前资源读取路径来说，对齐度较好 |
| `RemoteTrigger` | 管理远程触发器 | `action`, `trigger_id?`, `body?` | 本地内存 CRUD store，加 legacy webhook fallback | JSON 字符串，包含 `status` 和 `json` | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版调用真实远程触发器基础设施；Go 当前只是本地近似物 |
| `SendMessage` | 给 teammate 发送消息 | `to`, `summary?`, `message` | 基于本地 coordinator message bus 和简化 team state 实现 | JSON 字符串，包含 `success`、`message`、`routing?`、`recipients?` | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | 原版 swarm / inbox / request 流程复杂得多；Go 目前实现的是本地队列式子集 |
| `Skill` | 加载 skill 提示词 | `skill`, `args?` | 解析已安装 skill，并返回准备好的 skill 内容 | 纯文本 | `★★★★★` | `★★★☆☆` | `★★★☆☆` | 方向和用途一致，但仍比原版完整 skill/runtime 集成简单 |
| `TaskCreate` | 创建任务 | `subject`, `description`, `activeForm?`, `metadata?` | 在内存 task store 中创建任务 | 纯文本摘要 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版任务系统是文件持久化并与 hooks/team 行为深度集成；Go 任务当前完全在内存里，返回值也更简单 |
| `TaskGet` | 获取任务 | `taskId` | 从内存 task store 中读取任务 | JSON 字符串任务对象 | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | 外形可用，但持久化模型和任务生命周期都比原版简化很多 |
| `TaskList` | 列出任务 | 无 | 列出内存中的任务 | 纯文本列表 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 根本原因同 `TaskCreate`：存储和协同模型都更简单 |
| `TaskOutput` | 获取任务输出 | `task_id`, `block?`, `timeout?` | 从内存任务记录里读取输出 | 纯文本输出 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版异步任务链路更完整；Go 只是读一个简化版存储字段 |
| `TaskStop` | 停止任务 | `task_id?`, `shell_id?` | 将内存任务状态标记为 stopped | JSON 字符串摘要 | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | 虽然支持两个 id 字段，但底层任务模型仍远比原版简单 |
| `TaskUpdate` | 更新任务 | `taskId`, `subject?`, `description?`, `activeForm?`, `status?`, `addBlocks?`, `addBlockedBy?`, `owner?`, `metadata?` | 修改内存 task store，支持 `deleted` 伪状态 | 纯文本摘要 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 输入接口已对齐，但原版任务系统的持久化、hooks 和 team-aware 行为在 Go 中只部分模拟 |
| `TodoWrite` | 写入 todo 列表 | `todos: [{subject, description?, status}]` | 按 subject 对任务执行 upsert，落到内存 task store | 纯文本摘要 | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | 原版行为依赖真实任务子系统；Go 当前只是一个内存适配层 |
| `ToolSearch` | 按关键字搜索工具 | `query`, `max_results?` | 对 registry 的名字和描述做简单子串搜索 | 纯文本列表 | `★★★★★` | `★★★☆☆` | `★★★☆☆` | 接口面很好，但原版 defer-loading / discovery 模型更完整 |
| `WebFetch` | 抓取单个网页 | `url`, `prompt` | HTTP 抓取 + 基础 HTML 转文本 + cache | 纯文本，前缀带 `Prompt:` | `★★★★★` | `★★★☆☆` | `★★★☆☆` | 核心抓取可用，但原版的提取、权限和结果建模更丰富 |
| `WebSearch` | 网页搜索 | `query`, `allowed_domains?`, `blocked_domains?` | 执行缓存搜索并格式化文本结果 | 纯文本搜索结果 | `★★★★★` | `★★★☆☆` | `★★★☆☆` | 方向一致，但原版 web search 栈更成熟，也更深地融入整体运行时 |

## 当前环境未激活的条件工具

以下工具因为 gate 关闭，没有进入当前 32 工具基线。Go 侧 gating 已同步，因此它们在同样条件下也保持隐藏。

| 工具 | 原版 gate | 当前状态 | 说明 |
| --- | --- | --- | --- |
| `Config` | `USER_TYPE=ant` | 未激活 | Go 已按相同方式 gating；实现存在，但不属于当前激活基线 |
| `LSP` | `ENABLE_LSP_TOOL=true` | 未激活 | Go 已同步 gate；实现存在，但当前只覆盖部分 LSP 操作 |
| `TeamCreate` | `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` 或 `--agent-teams` | 未激活 | Go gate 已与当前 external build 行为对齐；实现仍是简化版本地 team runtime |
| `TeamDelete` | 与 `TeamCreate` 相同 | 未激活 | 状态同 `TeamCreate` |
| `TestingPermission` | `NODE_ENV=test` | 未激活 | Go 已正确同步 gate，并接入了 always-ask 权限路径 |

## 不在当前基线内的内容

以下内容不属于当前激活的 base-tool 审计范围，不能被拿来混淆当前 `32 工具 / Surface ★★★★★` 的结论：

- 仅 ant build 激活的工具，例如 `Tungsten`、`REPL`
- 仅 Windows 激活的 `PowerShellTool`
- feature-gated 工具，例如 `Sleep`、`Monitor`、`Workflow`、`WebBrowserTool`、`PushNotificationTool`、`SubscribePRTool`、`SendUserFileTool` 等
- 各个 MCP server 动态生成的 `mcp__*` 工具

这些工具要么当前环境未激活，要么 Go 尚未实现，要么两者兼有。

## 为什么 Surface 可以是 `★★★★★`，但 Runtime 仍然不一致

当前审计脚本能证明的是：

- 同样的工具被暴露了
- 在同样的当前 gate 条件下
- 用同样的模型可见工具名
- 带着同样的模型可见输入参数

它不能证明的是：

- 背后的服务是否同一个
- 持久化模型是否同一个
- 异步 / 后台执行模型是否同一个
- 输出 schema 是否同一个
- 边界行为是否同一个

所以像 `Agent`、`Read`、`RemoteTrigger`、`Task*` 这些工具，虽然 Surface 已经达到 `★★★★★`，但运行时和输出层面依然存在真实差异。

## 如果目标是完整端到端同构，下一步该做什么

建议优先级如下：

1. `Read`
   补 notebook / PDF / 图片处理，以及 byte/token 限流语义。
2. `Bash`
   补后台任务支持，以及更接近原版的 permission / sed-edit 逻辑。
3. `Agent`
   补异步 / 后台输出模式，以及更接近 swarm / remote / worktree 的执行语义。
4. `Task*` 与 `TodoWrite`
   从内存模型迁移到原版持久化任务模型，并补齐输出形状。
5. `RemoteTrigger` 与 `SendMessage` / `Team*`
   用真实后端流程替换当前本地近似实现。
6. `ExitPlanMode`
   重建 approval / request-id / leader approval 链路。

## 最终裁决

当前状态应被准确描述为：

- 对当前激活基础工具集，API 形状已经兼容
- 但还不是原版工具运行时的完整语义克隆

如果有人问“现在还有没有差异”，正确答案是：

- 对当前激活工具集的注册、名称、输入 schema：没有
- 对若干高价值工具的运行时和输出行为：仍然有，而且文档上面已经逐项写明
