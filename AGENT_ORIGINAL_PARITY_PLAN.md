# Agent / Team Original-Parity Roadmap

> 权限契约已于 2026-07-14 被替代：`Agent.mode` 与自定义 Agent 的
> `permissionMode` 均已删除，所有 subagent 只继承启动时捕获的父权限快照。
> 下文涉及这些字段的内容仅保留为历史记录；规范以
> `docs/superpowers/specs/2026-07-14-subagent-permission-inheritance-design.md`
> 为准。

本文档定义 Go 版 `Agent` / `Team` / 子 Agent 子系统与原版 `src` 的完整对齐路线。目标不是只修复当前截图暴露的问题，也不是逐行照搬 TypeScript 代码，而是把原版可观察行为抽象为可验收契约，再用 Go 版现有架构实现。

## 背景

当前截图暴露了一个复合问题：

- 普通 `Agent` 被带上 `mode: "plan"` 后，子 Agent 进入“先发计划、等待批准”的 teammate 语义，导致只返回“计划已发送等待批准”。
- 子 Agent 的 `Grep` / `Glob` 在 Windows 环境下依赖外部 `rg`，一旦 PATH 不含可执行 `rg` 就失效。
- 子 Agent 只有 Bash 型 shell 能力时，Windows 下 Bash / WSL 不可用会导致 PowerShell、cmd、Python 等 fallback 全部失败。
- `isolation: "worktree"` 被用于只读对比任务时改变路径上下文，使 `gosrc` 与原版 `src` 的相对路径不可靠。
- 普通 Agent、named teammate、TeamCreate 管理的团队成员三套生命周期边界混在一起，导致 prompt、schema 和运行时行为不稳定。

原版 `src` 中存在相似字段，但边界更清楚：`mode` 只在 teammate 分支转成 `plan_mode_required`，普通 subagent 不会因为传了 `mode: "plan"` 就进入 plan 审批；原版还有独立 `PowerShellTool` 与 bundled / embedded ripgrep 解析链，Windows 下不会只剩不可用的 Bash 和 PATH `rg`。

## 范围修正

当前问题只是 P0 入口，不是本路线图的全部。完整复刻方向必须覆盖原版 Agent/Team 子系统的全生命周期：

- 工具 schema 与 prompt contract。
- built-in / custom / plugin agent profile 的发现、过滤、合并与 frontmatter 语义。
- 普通 Agent、foreground / background Agent、named Agent、fork Agent、worktree Agent、Team teammate 的分流。
- 子 Agent tool pool、权限模式、MCP server requirement、技能和 memory 注入。
- task 注册、进度、输出文件、notification、resume、cancel、background transition。
- shell/search/file 工具在子 Agent 环境中的可用性。
- worktree 创建、路径提示、变更检测、清理和 resume。
- TeamCreate / SendMessage / TaskOutput / teammate mailbox 的协同语义。
- UI 可观察行为：Agent 进度、权限请求、错误展示、后台任务提示。
- Go 版明确的有意差异：子 Agent 模型继承当前 model，不回退到原版 Anthropic legacy model alias。

因此，本文档将任务拆成两层：

- **完整复刻轨道**：覆盖原版 Agent/Team 子系统的所有行为域。
- **P0 修复切片**：优先处理这次截图暴露的 plan mode、Team 误判、搜索、Windows shell、Auto Mode 等阻断问题。

## 当前实施状态

状态：**本轮 Agent / Team original-parity 修复已完成并通过执行验收**。

此前将 Slice A-J 标记为“完整收口”的结论不成立。真实执行 `WebFetch` / `WebSearch` 对比任务时，`Explore` 子 Agent 暴露 `max turns (50) exceeded`，说明当时只验证了局部单元/包级测试，没有完成 Definition of Done 要求的真实模型端到端验收。

本轮重新定义完成口径：只有通过下面“执行验收计划”的所有 gate，才允许再次声明 Agent / Team original parity 完成。当前 Gate 0-6 已通过；真实模型端到端验收已复跑，并确认父会话能等待两个 foreground `Explore` one-shot Agent 返回后再对称汇总。

已落地并验收的行为：

- Slice A / E：普通 `Agent` 与 Team teammate 分流已显式化。只有存在有效 team 且传入 `name` 时才进入 teammate；显式 `team_name` 缺失或不存在会返回清晰错误。普通 `Agent(mode: "plan")` 不再注入 teammate 的“等待 lead approval”系统提示；teammate `mode: "plan"` 仍保留原版 plan teammate 语义。
- Slice B / G：`Grep` / `Glob` 增加 Go-native fallback，`rg` 缺失、不可执行或被 Windows 权限拒绝时仍可递归搜索、匹配 glob、过滤 VCS、处理内容/计数/上下文输出。`PowerShell` 已加入 async agent 工具池，Windows 子 Agent 不再只依赖 Bash / WSL。
- Slice C / F：Windows 可注册独立 `PowerShell` 工具，支持前台/后台执行、plan mode 阻断、session CWD 注入、Agent 子注册表继承；权限系统新增 PowerShell 风险分类、只读命令自动放行、提示预览和缓存键。
- Slice D：Agent schema / prompt 收紧为原版等价契约。普通并行 Agent 要在同一轮发多个 foreground tool call；只有能不等结果继续时才使用 background；Go 版隐藏 legacy `model` schema，明确子 Agent 继承当前 model。
- Slice F：built-in / project / user / managed/plugin / inline profile 的加载、覆盖、过滤、MCP requirement、tools / disallowedTools、memory、skills、background、permissionMode、isolation frontmatter 均有 runtime 测试。
- Slice G：fork Agent 与普通 `general-purpose` 分离；fork 继承父上下文、父工具定义和父模型；fork output 不暴露为可轮询文件；worktree notice 只在 fork + worktree 组合下出现。
- Slice H：foreground、background、auto-background、cancel、error、resume、TaskOutput 生命周期已经补齐。后台任务重启后不会复用 ID，cancel 会立即持久化 killed 状态，output file 与 persisted result 保持一致。
- Slice I：TUI 已覆盖 Agent start/progress/backgrounded/completed/error、permission denied、safety denied、user canceled、context canceled、tool execution canceled；状态栏、权限弹框和 Agent JSON 结果展示保持可读。
- Slice J：Go 版内置 agent profile 的模型默认统一为 `inherit`，显式 legacy 模型覆盖在 Go/Codex build 中不进入 schema，实际运行继承当前会话模型；该差异有测试保护。

本轮新发现并已纳入 P0/P1 的缺口：

- 原版 `Explore` / `Plan` 没有默认 `maxTurns`，Go 版曾把未配置上限强制设为 50，导致长搜索硬失败。
- 原版 `max_turns_reached` 是 attachment 信号，`runAgent` 记录后结束并保留已有 assistant 输出；Go 版曾把它作为 error 返回给父会话。
- 原版 sync Agent 在已有 assistant 输出时倾向返回部分结果；Go 版曾把部分结果包进 error，父会话只能看到 `Agent failed`。
- Go 版曾在存在 `BackgroundTaskManager` 时把 foreground one-shot `Explore` / `Plan` 也注册成 retained session，导致完成后出现可继续后台任务语义；已改为 one-shot 直接同步执行并返回纯文本。
- Go 版曾允许普通 Agent 的 `mode=plan` 进入子 Agent 权限模式；已改为只有 teammate 分支保留 plan approval，普通 Agent 仍可传递 `bypassPermissions` / `dontAsk` 等权限模式。
- Go 版 `mode` schema 曾是任意 string，模型会填出 `read-only` 等原版 enum 不允许的值；已改为 permission mode enum。
- 模型会把 `isolation=worktree` 误用于 read-only one-shot `Explore` / `Plan`，改变相对路径上下文并导致 JSON metadata 包装；已增加 prompt/schema guidance 和运行时保护：任务文本未明确要求 worktree 时忽略 one-shot worktree isolation。
- 子 Agent / TeamCreate 曾安装固定 10 分钟 deadline；已移除，生命周期跟随父 context。
- inline agent MCP server 曾可能泄漏到父 registry；已改为 scoped snapshot/restore cleanup。

本轮完成证据：

- `go test ./tools -run "TestAgentToolSchemaAndLegacyAlias|TestAgentToolOneShotBuiltInSuppressesAccidentalWorktreeIsolation|TestOneShotWorktreeIsolationSuppressionRequiresExplicitWorktreeRequest|TestAgentToolOrdinaryPlanModeDoesNotInjectTeammateApproval|TestAgentToolOneShotBuiltInBypassesRetainedSession" -count=1`
- `go test ./tools -run "TestAgentToolBypassModePropagatesPermissionOverride|TestAgentToolPropagatesPermissionModeRequestOptions|TestAgentToolOrdinaryPlanModeDoesNotInjectTeammateApproval|TestAgentToolTeammatePlanModeKeepsLeadApprovalPrompt|TestAgentToolSchemaAndLegacyAlias|TestAgentToolOneShotBuiltInSuppressesAccidentalWorktreeIsolation" -count=1`
- `go test ./... -count=1`
- `git diff --check` exits 0; Windows CRLF warnings are emitted but no whitespace errors are reported.
- Real CLI Gate 6 command: `go run . -p -q -allow-all -max-turns 30 "<two foreground Explore agents comparing WebFetch/WebSearch against ../src>"`
  - Passed on Windows.
  - Observed two foreground one-shot `Explore` results returned as plain text, then parent summarized both reports.
  - Did not observe `Agent failed: max turns (50) exceeded`, `Team "... " does not exist`, legacy `sonnet/haiku/opus` model errors, denied Bash blocking read-only search, illegal `mode=read-only`, or worktree JSON wrapping.

Residual scope note:

- The Gate 6 reports intentionally compare `WebFetch` / `WebSearch` implementations and show those tools are not direct ports of original `src`. That is separate from this document's Agent / Team orchestration parity goal; it should be tracked as Web tool parity work if product requires it.

## 执行验收计划

### Gate 0：原版证据锁定

验收项：

- 对照 `src/tools/AgentTool/AgentTool.tsx`、`src/tools/AgentTool/runAgent.ts`、`src/tools/AgentTool/built-in/exploreAgent.ts`、`src/tools/AgentTool/built-in/planAgent.ts`、`src/query.ts`，列出 Go 版每个差异。
- 每个差异必须归类为 `implemented`、`intentionally different`、`not equivalent`。
- `intentionally different` 必须有产品理由和回归测试，例如子 Agent 继承当前 model。

完成证据：

- 文档中保留原版文件和行为证据。
- 测试名或验收脚本能直接映射到每条差异。

### Gate 1：P0 真实阻断修复

验收项：

- 未声明 `maxTurns` 的 built-in `Explore` / `Plan` 不再被 Go 版注入默认 50 turn 上限。
- 显式声明 `maxTurns` 时，query loop 返回 typed max-turns signal；Agent 层保留已有输出，不把整次调用标记为 `Agent failed`。
- 普通 Agent 的 `mode: "plan"` 不进入 teammate approval。
- 普通 named Agent 没有 team 上下文时不报 `Team does not exist`。
- 需要结果的并行 research/comparison Agent 默认 foreground，父会话必须等待结果再汇总。

完成证据：

- 单元测试覆盖 built-in Agent 超过 50 个工具回合后自然完成。
- 单元测试覆盖显式 `maxTurns` 后保留部分 assistant 输出。
- 真实执行 `WebFetch` / `WebSearch` 双 Agent 对比，不出现 `max turns (50) exceeded`、`Agent launched` 被当最终答案、teammate plan approval 误触发。

### Gate 2：平台与工具可用性

验收项：

- Windows 下无 PATH `rg` 或 packaged `rg.exe` 被拒绝执行时，`Glob` / `Grep` fallback 可用。
- Windows 下 Bash / WSL 不可用时，PowerShellTool 可用，且不会把 PowerShell fallback 走到 Bash。
- `Explore` / `Plan` prompt 在 Windows 下不会暗示只能使用 Bash；必须优先使用可用的 `Glob` / `Grep` / `Read`，PowerShell 只作为平台 read-only shell。
- search fallback 的排序、limit、offset、hidden/VCS 过滤差异有测试边界。

完成证据：

- 无 `rg` 环境的 `Glob` / `Grep` 测试通过。
- PowerShell read-only 命令风险分类和 Auto Mode 自动通过测试通过。
- Windows real-run 不再因为 shell/search 选择错误导致循环。

### Gate 3：上下文、profile 与输出契约

验收项：

- `Explore` / `Plan` 对齐原版的上下文裁剪：不携带无关 CLAUDE/AGENTS 规则和 stale gitStatus，除非 Go 版明确登记差异。
- built-in one-shot Agent (`Explore` / `Plan`) 完成后不附加 SendMessage 续聊提示和 usage trailer。
- profile 加载覆盖 built-in / project / user / managed/plugin / inline 的合并、过滤、frontmatter runtime 字段。
- `tools` / `disallowedTools`、`permissionMode`、`background`、`memory`、`skills`、`mcpServers` 不只是解析字段，必须影响运行时。

完成证据：

- profile runtime 测试覆盖上述字段。
- completed result 格式测试覆盖 one-shot built-ins 与普通 Agent 的差异。
- 真实 `Explore` prompt 不被项目实施准则污染到开始写代码或进入计划审批。

### Gate 4：权限继承与 UI 行为

验收项：

- Auto Mode 下普通子 Agent 的低风险 `Read` / `Glob` / `Grep` / read-only PowerShell 不弹权限框。
- `bypassPermissions`、`acceptEdits`、`auto` 的父级优先级不被 profile `permissionMode` 覆盖。
- teammate `mode: "plan"` 仍保留 lead approval，不被 Auto Mode 绕过。
- Permission modal 支持左右箭头、快捷键、active state 对比度，输入框不被压扁，`y` 不触发 `context canceled`。

完成证据：

- permission engine/table tests 覆盖 mode precedence。
- TUI permission dialog tests 覆盖键盘导航、确认、取消、布局。
- 手工或自动 TUI 验收记录包含 Auto Mode read/search 无弹窗场景。

### Gate 5：生命周期、hook、MCP、resume

验收项：

- `SubagentStart` / `SubagentStop` hooks 与原版生命周期一致，Stop hooks 转换为 SubagentStop，并在 agent 结束后清理 scoped hooks。
- agent-specific MCP servers 支持初始化、工具合并、结束清理；plugin/admin trusted 与 user-controlled policy 边界清晰。
- foreground、background、auto-background、cancel、error、resume、TaskOutput 的 task/output/metadata 状态一致。
- fork Agent 继承父上下文、父工具、父模型，且 worktree notice 只在 fork + worktree 时注入。
- 没有固定 10 分钟子 Agent 硬超时导致长任务与原版偏离；若保留上限，必须登记为有意差异并提供产品理由。

完成证据：

- lifecycle tests 覆盖 foreground/background/resume/cancel/error。
- hook/MCP tests 覆盖成功、失败、清理路径。
- fork/worktree tests 覆盖继承、递归拒绝、路径提示、resume。

### Gate 6：端到端验收

验收项：

- 在 Windows 当前项目中执行两个 foreground `Explore` Agent，分别对比 `WebFetch` 和 `WebSearch` 的 Go 版与原版 `src` 实现。
- 两个 Agent 都必须返回有效差异报告，父会话汇总为对称结论。
- 过程中不得出现：
  - `Agent failed: max turns (50) exceeded`
  - `Team "current/default/team" does not exist`
  - `sonnet/haiku/opus model is not supported`
  - `denied Bash` 阻断只读搜索
  - Auto Mode 下低风险 read/search 权限弹框
  - 只返回“计划已发送等待批准”

完成证据：

- 保存 e2e 命令、运行环境、关键输出摘要。
- `go test ./... -count=1` 通过。
- `git diff --check` 通过。

有意差异：

- 原版 `model` schema 暴露 `sonnet` / `opus` / `haiku`，Go 版隐藏该字段并继承当前 model。这是产品要求，用于避免 ChatGPT/Codex account 下 legacy Anthropic alias 不可用。
- 原版通过 bundled / embedded ripgrep 解析链兜底；Go 版当前通过 Go-native fallback 达到相同可用性目标。该 fallback 不宣称覆盖 ripgrep 的所有边缘语义，核心文件发现、glob、内容搜索、limit/offset、hidden/VCS 过滤已有测试。
- Go TUI 不逐像素复刻 Ink UI，但保留原版可观察语义、键盘交互和错误表达。

## 原版行为证据

以下文件是本计划的原版对齐依据：

- `src/tools/AgentTool/AgentTool.tsx`
  - `mode` schema 描述为 spawned teammate 的 permission mode。
  - 只有 `teamName && name` 分支调用 `spawnTeammate`，并把 `spawnMode === "plan"` 映射为 `plan_mode_required`。
  - 普通 subagent 分支使用 selected agent，并通过 `selectedAgent.permissionMode ?? "acceptEdits"` 组装工具池。
  - `isolation: "worktree"` 是显式 opt-in，不是普通研究任务默认路径。
- `src/tools/AgentTool/prompt.ts`
  - foreground 是默认路径；需要结果才能继续的 research / comparison agent 不应后台化。
  - 用户要求并行时，主模型应在同一轮里发多个 Agent tool use，等待结果后再汇总。
  - prompt 明确要求说明 Agent 是写代码还是只研究。
- `src/utils/ripgrep.ts`
  - ripgrep 解析不是单纯 `PATH rg`：优先 system，然后 bundled / embedded，再 vendor ripgrep。
  - Windows 启动子进程时使用 `windowsHide`，并处理 embedded `argv0: "rg"`。
- `src/utils/glob.ts` 与 `src/tools/GrepTool/GrepTool.ts`
  - `Glob` / `Grep` 依赖统一的 `ripGrep` 适配层，而不是各自直接 `exec.LookPath("rg")`。
- `src/tools/PowerShellTool/PowerShellTool.tsx`
  - Windows 有独立 PowerShell 工具和权限判断，不把所有 shell fallback 都强制经过 Bash。
- `src/tools/shared/spawnMultiAgent.ts`
  - teammate 会继承 auto permission mode。
  - teammate 会传播显式 CLI model，但 Go 版有一个有意差异：普通子 Agent 与 teammate 必须继承当前选用模型，不使用原版的 sonnet / opus / haiku 默认。

## 完整复刻行为域

### A. Agent Tool Schema / Prompt

原版行为：

- `Agent` 工具 schema 暴露 `description`、`prompt`、`subagent_type`、`model`、`run_in_background`。
- 在 agent swarms / Kairos / fork gate 条件下额外暴露 `name`、`team_name`、`mode`、`isolation`、`cwd`。
- prompt 负责告诉主模型什么时候该用 Agent、什么时候不该用 Agent、foreground/background 如何选择、如何写完整 prompt。
- prompt 明确：需要结果才能继续时使用 foreground；background 任务完成后通过 notification 回来，不主动轮询。

Go 版验收：

- Schema 字段、可见条件、字段说明与原版等价，Go 版不支持的字段必须显式隐藏或返回清晰错误。
- Prompt 覆盖 foreground/background、parallel agents、普通 subagent vs teammate、worktree opt-in、result relay。
- 模型字段必须按 Go 版产品要求改写：默认继承当前 model，不展示 `sonnet` / `opus` / `haiku` 作为可自动选择模型。

### B. Agent Profile Discovery

原版行为：

- 内建 agents、项目 `.claude/agents`、用户 agents、managed/plugin agents、inline agents 会合并。
- MCP requirement 不满足的 agent 不出现在可用列表。
- permission deny rules 可以过滤 agent types。
- frontmatter 支持 model、tools、disallowedTools、permissionMode、mcpServers、memory、skills、background、isolation 等行为字段。

Go 版验收：

- Agent profile 加载顺序、覆盖规则、去重规则、过滤规则可通过测试证明。
- `tools` 与 `disallowedTools` 同时存在时，运行时工具池与展示一致。
- MCP requirements 没满足时，agent 不应被模型看到或调用成功。
- profile 的 `permissionMode`、`background`、`isolation`、`memory`、`skills` 均影响 runtime，而不仅是展示。

### C. Ordinary Agent Lifecycle

原版行为：

- 普通 Agent 默认 foreground。
- foreground Agent 完成后直接返回一个 final result 给调用方。
- 长时间运行时可显示 background hint，用户可背景化。
- background Agent 通过 task registry、output file、notification 回到主会话。
- 普通 Agent 的结果对用户不可直接可见，主模型负责汇总。

Go 版验收：

- sync foreground、explicit background、auto-background transition 三条路径都有测试。
- final result、error、abort、background notification 的状态转换与输出文件一致。
- 主会话不能把 “Agent launched” 当作需要结果任务的最终答复。
- `TaskOutput(block=true)` 能等待后台 Agent 完成并返回输出。

### D. Fork Agent / Context Inheritance

原版行为：

- fork path 通过省略 `subagent_type` 触发，继承父上下文与父工具定义，用于降低主上下文污染。
- fork prompt 是 directive，不重新解释背景。
- fork 运行中主模型不能读取 output file 轮询，除非用户明确要求。
- fork + worktree 时会注入原 cwd 与 worktree cwd 的路径映射提示。

Go 版验收：

- omit `subagent_type` 的 fork 行为与普通 `general-purpose` 行为可区分。
- fork child 继承父上下文、工具定义和模型，且不能递归 fork。
- fork output notification 不被主模型伪造。
- worktree notice 只在实际 fork/worktree 组合时注入。

### E. Team / Teammate Lifecycle

原版行为：

- teammate spawn 只在 `teamName && name` 成立时进入。
- teammate 的 `mode: "plan"` 映射为 `plan_mode_required`，普通 Agent 不受影响。
- teammate 不能再 spawn nested teammate；应 omit name/team_name 启动普通 subagent。
- teammate 可通过 SendMessage / mailbox 与 lead 协作。
- teammate 继承必要 CLI flags：permission mode、model、settings、plugin dirs 等。

Go 版验收：

- TeamCreate 是创建 team 的唯一入口。
- `team_name` 未存在时必须报清晰错误，不得猜测默认 team。
- teammate roster、mailbox、SendMessage routing、resume 与 task metadata 一致。
- Auto Mode、model、settings 等上下文传播有测试。

### F. Tool Pool / Permissions

原版行为：

- 子 Agent 通过 agent profile 与 permission context 组装自己的工具池。
- `selectedAgent.permissionMode ?? "acceptEdits"` 用于普通 Agent 工具权限。
- Auto Mode 下低风险 read/search 自动通过；plan approval 与 bypass/auto 有明确优先级。
- permission UI 要能处理 keyboard navigation、confirm/cancel、active state contrast。

Go 版验收：

- 普通 Agent、Plan agent、teammate、fork agent 的工具池分别测试。
- Auto Mode 对普通 child 和 teammate 继承有效。
- Plan approval 不被 Auto Mode 绕过。
- Permission modal 的键盘交互和布局有 TUI 测试。

### G. Search / Shell / File Tool Availability

原版行为：

- Glob/Grep 通过统一 ripgrep adapter 工作，不硬依赖 PATH `rg`。
- Windows 有 PowerShellTool；Bash 不可用不影响 PowerShell。
- FileRead、Glob、Grep、Bash/PowerShell 的 search/read 标记影响 UI 折叠与 permission 分类。

Go 版验收：

- 无 PATH `rg` 时仍能搜索。
- Windows Bash/WSL 不可用时 PowerShell 可用。
- 子 Agent 工具池包含平台正确的 shell 工具。
- search/read collapse 与 permission 分类覆盖测试。

### H. Worktree / CWD / Resume

原版行为：

- `isolation: "worktree"` 显式 opt-in。
- `cwd` 与 worktree 互斥。
- worktree 没有变更时清理，有变更时保留路径与 branch。
- resume Agent 时能恢复 worktree metadata，必要时校验路径存在。

Go 版验收：

- worktree create / cleanup / changed keep / resume path 均有测试。
- 工作区路径提示不会污染非 worktree Agent。
- 跨 `gosrc` / `src` 对比任务支持明确 `cwd` 或绝对路径。

### I. UI / Task Observability

原版行为：

- Agent 运行过程有 progress line。
- 后台 Agent 有 task list / output file / completion notification。
- 工具调用错误、权限拒绝、用户取消、abort 都有可读展示。

Go 版验收：

- TUI 展示 Agent start / progress / complete / error / backgrounded。
- task output 文件与 UI 状态一致。
- 权限错误、工具不可用错误不能被吞掉或变成空结果。

### J. Intentional Divergence Register

这些点不应盲目复刻原版：

- 模型：Go 版子 Agent 默认继承当前选用 model，不使用 `sonnet` / `opus` / `haiku` 作为默认。
- 运行时：Go 版可用 Go-native fallback 代替原版 bundled ripgrep，只要行为契约可验收。
- UI：Go TUI 可以不逐像素复刻 Ink，但必须保留可观察语义、键盘交互和错误表达。

任何新增差异必须写入本节，并给出理由与测试边界。

## Go 版目标契约

### 1. 普通 Agent 与 teammate 必须分离

普通 Agent 是“启动一个子任务并返回结果”。只有以下条件同时满足时才进入 teammate / team 分支：

- `team_name` 指向一个已存在的 TeamCreate team，或当前上下文已经处在 team 中。
- `name` 被设置，用于团队成员寻址。
- runtime 能确认这是 teammate spawn，而不是普通 subagent 命名。

验收标准：

- 普通 `Agent({ subagent_type, description, prompt, mode: "plan" })` 不得触发“等待 lead approval”。
- 普通 `Agent({ name, description, prompt })` 在没有 team 上下文时不得报 “Team does not exist”；`name` 只能作为普通 Agent 的显示名 / 后台寻址名。
- `Agent({ team_name, name, mode: "plan" })` 仍然必须进入 teammate 分支，并设置 plan approval。
- teammate 内部再次带 `name` / `team_name` spawn teammate 时，应保持原版约束：拒绝嵌套 teammate，引导其 omit 这些字段启动普通 subagent。

实施项：

- 在 `gosrc/tools/agent.go` 中把 `mode` 的运行时效果限定到 teammate 分支。
- 普通 Agent 分支如果收到 `mode`，只用于权限检查或直接忽略，不得注入 “Permission mode is plan...” 到普通 Agent system prompt。
- schema 文案保留 `mode` 但明确“only when spawning a teammate with team_name + name”；普通 Agent prompt 中不得鼓励填写。
- 添加 runtime guard：没有有效 team 时，`team_name` 不得被自动推断为 `"team"`、`"current"` 或 `"default"`。

### 2. 子 Agent 默认 foreground，结果必须回到主会话

默认普通 Agent 应同步完成并返回结果，除非显式 `run_in_background: true` 或 agent profile 强制 background。

验收标准：

- 用户要求“启动两个子 Agent 分别对比 X / Y”时，主会话必须等待两个结果，并汇总差异。
- 如果子 Agent 被后台化，主会话不得猜测结果；只能说明已后台运行，并等待 notification / TaskOutput。
- foreground Agent 过程中可以显示进度，但不能把“Agent launched”当作最终答案。

实施项：

- 检查 `gosrc/tools/agent.go` 中 async / background 判定，确保普通 Agent 默认 foreground。
- prompt 增加原版等价约束：需要结果时不要设置 background；并行对比应在同一 assistant turn 中发多个 Agent tool calls。
- `TaskOutput` / notification 只用于后台任务；foreground path 必须返回子 Agent final message。

### 3. `mode: "plan"` 只代表 teammate plan approval

原版 `mode` 字段对普通 subagent 不改变其 system prompt。Go 版当前 `buildAgentSystemPrompt` 会在 `mode == "plan"` 时追加“先产生计划、等待 lead approval”，这是导致 WebFetch Agent 停在计划审批的直接原因。

验收标准：

- 普通 Agent 即使误传 `mode: "plan"`，也必须继续执行只读研究任务。
- `Plan` agent type 自身仍是只读规划 agent，但它不等同于 teammate plan approval。
- teammate `mode: "plan"` 仍需要 `SendMessage` 给 lead 等待批准。

实施项：

- 拆分两个概念：
  - `agentPermissionMode`: ordinary subagent tool permission mode。
  - `teammatePlanModeRequired`: team / teammate approval gate。
- `buildAgentSystemPrompt` 不应根据 ordinary Agent 的 permission mode 注入 teammate approval 文案。
- teammate system prompt 单独注入 “lead approval” 指令。

### 4. Worktree isolation 必须显式且路径语义可解释

`isolation: "worktree"` 是写代码隔离或明确 sandbox 的工具，不应成为只读对比默认行为。

验收标准：

- 普通只读对比任务默认在当前工作目录运行。
- 当显式设置 `isolation: "worktree"` 时，子 Agent 的系统提示必须说明原 cwd 与 worktree cwd 的映射。
- 对比 `gosrc` 和 `src` 时，必须支持 `cwd` 明确指定，并在文档中推荐跨根目录任务使用绝对路径。
- `cwd` 与 `isolation: "worktree"` 同时设置时必须报错，保持原版互斥规则。

实施项：

- 保持 `isolation` opt-in。
- 普通 Agent prompt 中说明只在需要隔离改代码时使用 worktree。
- `gosrc/tools/agent.go` 中保留 worktree notice；仅在 fork / inherited context 下追加路径转换提醒。
- 添加测试覆盖 `cwd` / `worktree` 互斥、worktree explicit-only。

### 5. Grep / Glob 不能硬依赖 PATH `rg`

Go 版必须实现原版等价的 ripgrep 解析层。若无法做到 embedded ripgrep，至少必须有 Go-native fallback，保证子 Agent 在 Windows 无 PATH `rg` 时仍可搜索。

验收标准：

- 清空或移除 `PATH` 中的 `rg` 后，`Glob` 仍能列出匹配文件。
- 清空或移除 `PATH` 中的 `rg` 后，`Grep` 仍能按 pattern 返回匹配文件 / 内容。
- fallback 必须尊重 hidden、ignore、glob filter、head limit、offset、read permission ignore patterns。
- fallback 的错误不能伪装成 “No files found”；超时和权限错误必须显式返回。

实施项：

- 新增统一 `ripgrep` adapter：
  - 优先使用显式配置的 `rg`。
  - 再使用 bundled / vendor `rg.exe`。
  - 最后使用 Go-native fallback。
- `Glob` 和 `Grep` 都调用同一个 adapter。
- Go-native fallback 使用 `filepath.WalkDir`、`path/filepath.Match` / doublestar 语义、`regexp`、大小写选项和分页。
- 将 `resolveRipgrepPath` 的错误从 hard fail 改为 fallback trigger。

### 6. Windows shell 能力必须平台化

Go 版不能在 Windows 上把所有 shell fallback 都通过 Bash 执行。原版有 PowerShellTool，Go 版也需要可用的 Windows shell tool，至少在子 Agent 工具池中暴露。

验收标准：

- Windows 下子 Agent 可以执行只读 PowerShell 命令，如 `Get-ChildItem`、`Select-String`、`Get-Content`。
- Bash / WSL 不可用时，不影响 PowerShellTool。
- Bash 失败应报告 Bash 不可用，而不是让 PowerShell/cmd/Python fallback 全部以 Bash 错误失败。
- permission UI 显示工具名应准确：PowerShell tool 显示 PowerShell，Bash tool 显示 Bash。

实施项：

- 实现并注册 `PowerShell` 工具，与 `Bash` 独立存在。
- 在 Windows 平台默认为子 Agent 提供 PowerShellTool。
- `Bash` 工具继续保留，但不作为 Windows shell fallback 的唯一入口。
- 权限系统为 PowerShell 定义独立 read-only 判定和 allow rules。

### 7. Auto Mode 权限必须继承到子 Agent / teammate

已经进入 Auto Mode 时，低风险 read/search 操作不应弹出 Permission Required，除非风险分类明确需要。

验收标准：

- 主会话 Auto Mode 下，普通子 Agent 的 `Read` / `Grep` / `Glob` / read-only shell 不弹权限框。
- 主会话 Auto Mode 下，teammate 也继承 auto permission mode。
- `mode: "plan"` teammate 不继承 bypass / auto 直接执行改动，仍受 plan approval。
- 手动 Ask Mode 下仍按权限规则弹框。

实施项：

- 检查 `permissions` adapter 中 parent mode 到 child mode 的传播。
- teammate launch args 传播 `--permission-mode auto` 等价语义。
- 权限判定测试覆盖普通 Agent 与 teammate 两条路径。

### 8. 子 Agent 模型选择是有意差异：继承当前 model

这里明确不完全对齐原版。原版 schema 里 `model` 是 `sonnet` / `opus` / `haiku`，且 built-in profile 可能有默认模型。Go 版产品要求是：现版使用当前选用的 model 启动子 Agent，避免 ChatGPT account 下 `sonnet` / `haiku` / `opus` 不可用。

验收标准：

- 主会话使用 `openai/gpt-5.4` 时，普通子 Agent 默认也使用 `openai/gpt-5.4`。
- teammate 默认也继承 leader model，除非用户显式配置 Go 版支持的模型。
- 任何路径不得自动写入 `sonnet` / `haiku` / `opus`。
- 如果用户显式传入不支持模型，错误要说明该模型不可用并建议继承当前模型。

实施项：

- 保留 `inheritedSubagentModel` 逻辑。
- 移除 Go 版内建 profile 中不可用的 Anthropic legacy model 默认。
- 测试覆盖普通 Agent、Team teammate、resume Agent 的模型继承。

## 交付切片

以下切片是完整复刻路线的实施顺序。Slice A-E 是 P0，直接对应当前截图阻断；Slice F-J 是完整复刻补齐项，防止只修遇到的问题后再次偏离原版。

### Slice A: Agent / Team 语义边界

目标：修复普通 Agent 被误导进 teammate plan approval 的根因。

改动范围：

- `gosrc/tools/agent.go`
- `gosrc/tools/agent_test.go`
- `gosrc/tools/team.go`
- `gosrc/tools/team_test.go`

验收：

- 普通 Agent 传 `mode: "plan"` 不等待 lead approval。
- teammate 传 `mode: "plan"` 仍等待 lead approval。
- 没有 team 时 `team_name` 不被猜测为 `"team"` / `"current"` / `"default"`。
- 普通 named Agent 与 teammate named spawn 行为可区分。

### Slice B: 子 Agent 搜索能力

目标：`Grep` / `Glob` 在无 PATH `rg` 的 Windows 环境仍可用。

改动范围：

- `gosrc/tools/search.go`
- `gosrc/tools/search_test.go`
- 新增 `gosrc/tools/ripgrep_adapter.go`
- 新增 `gosrc/tools/search_fallback.go`

验收：

- 模拟 `exec.LookPath("rg")` 失败时，Glob fallback 通过。
- 模拟 `exec.LookPath("rg")` 失败时，Grep fallback 通过。
- fallback 与 ripgrep path 在结果排序、limit、offset、hidden 文件处理上行为一致或差异明确记录。

### Slice C: Windows PowerShell tool

目标：Windows 子 Agent 不依赖 Bash / WSL 执行只读 shell。

改动范围：

- `gosrc/tools/powershell.go`
- `gosrc/tools/powershell_test.go`
- `gosrc/registry_setup.go`
- `gosrc/permissions/*`
- `gosrc/tui/*` 权限展示如需调整

验收：

- Windows 下 PowerShellTool 可执行 `Get-ChildItem` / `Select-String`。
- Bash 不可用不会影响 PowerShellTool。
- Auto Mode 下 read-only PowerShell 命令按风险分类自动允许。

### Slice D: Prompt 与 schema 对齐

目标：让模型少填错参数，减少 runtime 兜底压力。

改动范围：

- `gosrc/tools/agent.go`
- Agent prompt / schema 相关测试

验收：

- Agent tool description 明确 foreground / background 使用边界。
- `mode` 文案只描述 teammate。
- `isolation` 文案明确是显式 worktree opt-in。
- prompt 中包含“需要结果时不要后台化”的等价原版约束。

### Slice E: 端到端回归

目标：复现截图场景并证明已修复。

验收场景：

1. 在 Windows、无可用 PATH `rg`、Bash / WSL 不可用的环境中：
   - 启动两个普通 Agent 对比 `WebFetch` 和 `WebSearch`。
   - 两个 Agent 都能通过 Glob / Grep fallback 或 PowerShell 完成文件发现。
   - 主会话汇总两个结果，而不是停在 “Agent launched”。
2. 在 Auto Mode 中：
   - 子 Agent 的低风险 `Read` / `Grep` / `Glob` 不弹权限框。
   - 中高风险 shell / write 操作仍按权限策略处理。
3. 在 teammate plan mode 中：
   - `Agent({ team_name, name, mode: "plan" })` 仍要求 lead approval。
   - approval 后 teammate 继续执行，不影响普通 Agent 行为。

### Slice F: Agent profile 完整加载

目标：补齐 built-in / custom / plugin / inline agent 的发现、过滤、合并与 frontmatter 语义。

改动范围：

- `gosrc/tools/agent.go`
- `gosrc/tools/agent_plugins.go`
- `gosrc/registry_setup.go`
- agent profile 相关测试

验收：

- 内建、项目、用户、managed/plugin、inline profile 合并顺序可测。
- MCP requirement 未满足时 agent 不可见或调用失败信息清晰。
- `tools` / `disallowedTools` 展示和运行时工具池一致。
- `memory` / `skills` / `background` / `permissionMode` / `isolation` frontmatter 影响 runtime。

### Slice G: Fork Agent 与上下文继承

目标：对齐原版 fork subagent 语义，避免与 general-purpose 普通 Agent 混淆。

改动范围：

- `gosrc/tools/agent.go`
- fork message / context / worktree notice 测试

验收：

- omit `subagent_type` 触发 fork，而不是普通 fresh general-purpose。
- fork child 继承父上下文和工具定义。
- fork child 不允许递归 fork。
- fork notification 由 runtime 产生，主模型不得伪造结果。

### Slice H: Task / Resume / Notification 完整生命周期

目标：补齐 Agent task 生命周期，支持可观测、可恢复、可取消。

改动范围：

- `gosrc/tools/agent_sessions.go`
- `gosrc/tools/task*.go`
- `gosrc/session/*`
- `gosrc/tui/*`

验收：

- foreground、background、auto-background、cancel、error、resume 状态转换有测试。
- output file、metadata、UI task list 状态一致。
- `TaskOutput(block=true)` 能等待并返回最终输出。
- resume worktree Agent 时校验路径与 metadata。

### Slice I: UI 观察性与错误表达

目标：确保 Go TUI 不逐像素复刻也能保留原版可观察语义。

改动范围：

- `gosrc/tui/*`
- `gosrc/render/*`
- Agent progress / permission / task output 相关测试

验收：

- Agent start、progress、backgrounded、complete、error 均有稳定展示。
- 工具失败、权限拒绝、用户取消、context canceled 能区分显示。
- Permission modal 支持方向键、快捷键、active state 对比度、输入框不被压扁。

### Slice J: 差异登记与防回归

目标：把所有有意差异文档化，并用测试防止被误改成原版旧行为。

改动范围：

- `gosrc/AGENT_ORIGINAL_PARITY_PLAN.md`
- model inheritance / profile defaults / shell fallback 相关测试

验收：

- 子 Agent 模型继承当前 model 的差异有测试保护。
- 不会自动选择 `sonnet` / `opus` / `haiku`。
- Go-native search fallback 的已知差异有文档和测试边界。
- 每个未实现的原版行为都有明确状态：implemented、intentionally different、deferred with reason。

## 测试矩阵

| Area | Test | Expected |
| --- | --- | --- |
| Ordinary Agent | `mode: "plan"` without team | Executes normally; no lead approval prompt |
| Team Agent | `team_name + name + mode: "plan"` | Spawns teammate with plan approval |
| Named ordinary Agent | `name` without team | Runs as ordinary Agent; no Team missing error |
| Worktree | `isolation: "worktree"` explicit | Creates worktree and injects path notice |
| Worktree | no isolation | Uses current cwd |
| Search | no `rg` in PATH | Grep / Glob fallback works |
| Search | ripgrep available | Existing ripgrep path still works |
| Windows shell | Bash unavailable | PowerShellTool still works |
| Auto mode | child low-risk read/search | No permission modal |
| Ask mode | child read/search requiring approval | Permission modal appears and resolves |
| Model | current model `openai/gpt-5.4` | Child uses same model |
| Model | legacy `sonnet` not configured | Not auto-selected |

## Definition of Done

本计划完成必须同时满足：

- 普通 Agent、teammate、TeamCreate 三者的参数、生命周期和权限边界有测试保护。
- Windows 环境下搜索和只读 shell 不依赖 WSL / Bash。
- 无 PATH `rg` 时子 Agent 仍有可用搜索能力。
- Auto Mode 不再对低风险 read/search 子 Agent 弹权限框。
- `mode: "plan"` 的 teammate approval 行为保留，但不会污染普通 Agent。
- 子 Agent 模型继承当前模型，不回退到 `sonnet` / `haiku` / `opus`。
- 截图中的 WebFetch / WebSearch 对比场景可以端到端完成并返回对称报告。
- Agent profile discovery、tool pool、MCP requirement、memory、skills、frontmatter 行为均有测试。
- fork、background、resume、cancel、notification、TaskOutput 生命周期均有端到端或集成测试。
- 每个与原版不同的行为都记录在 Intentional Divergence Register，并有测试防回归。

## 风险与约束

- Go-native Grep fallback 很容易和 ripgrep 在 ignore、glob、multiline 上产生边缘差异。第一版必须覆盖核心文件发现与内容搜索，不宣称完全替代 ripgrep。
- PowerShellTool 的权限规则不能简单复用 Bash AST，需要单独实现保守的 read-only 分类。
- Team 和普通 Agent 的 schema 同时暴露 `name` / `team_name` / `mode` 会继续诱发误填，所以 runtime guard 必须比 prompt 更权威。
- 模型继承是 Go 版产品要求，和原版 Anthropic model alias 行为不同；后续对齐原版时不能把这个差异误改回去。

## 推荐实施顺序

1. Slice A：先修 Agent / Team 语义边界，因为这是截图中“计划等待批准”的直接根因。
2. Slice D：同步收紧 prompt / schema，减少后续复现概率。
3. Slice B：修搜索能力，让子 Agent 具备基本代码发现能力。
4. Slice C：补 Windows PowerShell tool，解决 Bash / WSL 不可用问题。
5. Slice E：做端到端复现与验收，确认 WebFetch / WebSearch 对比能完成。
6. Slice F：补齐 Agent profile 加载、过滤、frontmatter runtime 行为。
7. Slice G：对齐 fork Agent 与上下文继承语义。
8. Slice H：补齐 Task / Resume / Notification 生命周期。
9. Slice I：完善 UI 观察性与错误表达。
10. Slice J：建立差异登记和防回归测试，作为完整复刻收口 gate。
