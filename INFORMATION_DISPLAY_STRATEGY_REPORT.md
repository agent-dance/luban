# Claude Code、Codex 与本项目的信息展示策略报告

> 研究日期：2026-07-15
> 研究对象：Claude Code 2.1.209、Codex 官方手册、`openai/codex` commit `f90e7deea6a715bbd153044af6f475eefa749177`、本仓库当前工作树
> 报告目的：说明何时隐藏、折叠、收拢或完整展示信息；定义每个命令和 subagent 的展示合同；评估本项目的实现路径与成本。

> 实施状态（2026-07-15）：本报告定义的行为对齐方案已经实现并通过全仓 test/race/vet/build、跨平台构建、macOS PTY/tmux 与终端/无障碍矩阵。实现证据、六个实施任务和未测试边界见 [`tasks/information_display_strategy/implementation/IMPLEMENTATION_REPORT.md`](tasks/information_display_strategy/implementation/IMPLEMENTATION_REPORT.md)。下文“当前实现/缺口”保留为实施前的研究快照，不应误读为最新代码状态。

## 1. 执行摘要

Claude Code 和 Codex 都没有一个“超过 N 行就折叠”的全局算法。两者真正采用的是同一种分层思想，但工程落点不同：

- Claude Code 主要把决策分散在每种工具的 renderer、`verbose/transcript` 模式和专用 Agent/Task 视图中。文件、Shell、MCP、Agent 等工具分别决定默认摘要、运行态和详细结果；`Ctrl+O`/`Ctrl+E` 提供 transcript 与 show-all 路径。
- Codex 把同一对象显式拆成 `display_lines`、`transcript_lines` 和 `raw_lines`。主视图保留有限的语义摘要；`Ctrl+T` 打开完整 transcript；命令输出采用头尾保留并明确显示省略行数；subagent 使用独立 thread、状态行和 `/agent` 切换。
- 两者都把失败、审批、需要输入和人工可操作状态提到正常成功之上；都让高频成功活动安静下来；都保留下钻到完整 transcript 或子会话的路径。
- 两者都没有公开一份完整、稳定的“展示决策规范”。本报告中关于产品意图的归纳属于推断；官方文档和源码事实分别标注。

本项目已经具备信息展示的关键基础设施：稳定 observation 身份、三档 disclosure、无损 DetailStore、结构化 Decision、ActivityStore、screen-reader renderer、搜索/导出和会话持久化。缺口集中在一个尚未显式存在的“语义展示政策层”：

1. 当前默认 disclosure 主要由 outcome 决定，没有综合命令族、风险、动作影响、重复度、持续时间、输出体积和用户意图。
2. 成功工具结果仍多为通用摘要，缺少 `Read 42 files`、`Tests failed 2/129`、`Edited 3 files · +42 -17` 这类领域 formatter。
3. Activity 已能按 work unit/actor 分组，但还缺 duration、cost、最新语义进度、artifact/verification 摘要和完整 subagent 生命周期合同。
4. 当前工作树最终复跑 `go build ./...`、`go vet ./...` 和 `go test ./...` 均通过。调查早期曾因并发中的未提交改动缺少 `costKnown bool` 参数导致 `tui` 无法编译，说明 dirty tree 的测试基线会变化，仍需 Phase 0 冻结。

推荐不重写现有 TUI，而是在 observation 与 renderer 之间增加纯函数 `PresentationPolicy + FormatterRegistry + Aggregator`。以当前基础估算：

| 目标 | 范围 | 人日（低/基准/高） |
| --- | --- | ---: |
| 最小可用 | outcome/risk/intent policy，6 个核心命令族/约 12 个高频工具，Agent 终态摘要，关键测试 | 10 / 18 / 28 |
| 完整对齐 | 全部注册命令族、聚合、subagent work view、screen reader、持久化与迁移 | 24 / 42 / 67 |
| 生产加固 | 完整对齐 + PTY/窄屏/跨平台/性能/故障注入/真实长会话验证 | 38 / 63 / 100 |

这些是累计工程人日，不是日历天。2 名熟悉代码的工程师并行时，完整对齐的基准日历时间约 5-6 周；继续增加人手会被共享的 `tui/state.go`、`tui/root.go` 和事件合同边界限制。

## 2. 证据边界

报告使用四种证据标记：

- **[E]** 官方文档、官方源码或本仓库可定位代码。
- **[O]** 当前本机/工作树的直接观察和命令结果。
- **[I]** 从多个事实归纳的设计意图，不代表产品官方承诺。
- **[D]** 对本项目的建议和决策。
- **[U]** 当前公开资料无法可靠确认。

主要证据：

- [E] Claude Code [Interactive mode](https://code.claude.com/docs/en/interactive-mode)、[Tools reference](https://code.claude.com/docs/en/tools-reference)、[Subagents](https://code.claude.com/docs/en/sub-agents)、[Agent view](https://code.claude.com/docs/en/agent-view)、[Agent teams](https://code.claude.com/docs/en/agent-teams)、[Accessibility](https://code.claude.com/docs/en/accessibility)、[Fullscreen](https://code.claude.com/docs/en/fullscreen)。
- [E] Claude Code 相邻 TypeScript 基线：`../src/tools/*/UI.tsx`、`../src/components/messages/*`、`../src/components/VirtualMessageList.tsx`。
- [E] Codex [Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)、[CLI slash commands](https://learn.chatgpt.com/docs/cli/slash-commands)、[App Server](https://learn.chatgpt.com/docs/app-server) 和官方 [`openai/codex`](https://github.com/openai/codex) 源码。
- [E] 本项目既有设计与实现记录：`TUI_INFORMATION_DESIGN.md`、`TUI_INFORMATION_ARCHITECTURE_IMPLEMENTATION.md`。
- [O] 最终复跑 `go build ./...`、`go vet ./...` 和 `go test ./...` 均通过。较早的 agent 快照记录过 `root_status_bar_test.go:47,55,63` 缺少 `costKnown bool`；该问题随后被工作树中的并发用户改动修复，不能再当作当前失败。

限制：

- Claude Code 的完整生产 UI 内部实现不是公开稳定 API。相邻 TypeScript 代码可证明当前基线行为，不能证明未来版本承诺。
- 本机存在两个 Claude CLI：默认 PATH 的 2.1.159 与 Homebrew 路径的 2.1.209；报告标题以明确调用的 2.1.209 和当前官方文档为研究快照，不把任一二进制行为外推为长期承诺。
- Codex desktop app 的所有布局规则并未全部开源。CLI 源码可以证明 TUI 行为，app/IDE 只能使用官方文档描述。
- 视觉“相似”不是验收目标；稳定身份、无损证据、注意力路由和可逆下钻才是。

## 3. 两个产品怎样决定展示深度

### 3.1 Claude Code

Claude Code 的展示决策主要由五层共同完成：

1. **工具专属 renderer**：每个工具提供 tool-use、progress、result、error renderer。`BashTool/UI.tsx:85-183` 在非 verbose 模式把命令限制到 2 行/160 字符，运行态显示实时进度，结果交给 Shell 专用结果组件；`FileReadTool/UI.tsx`、`GrepTool/UI.tsx`、`MCPTool/UI.tsx` 分别使用领域字段。
2. **全局 verbose/transcript 状态**：官方文档规定 `Ctrl+O` 打开 transcript viewer，`Ctrl+E` 切换 show-all。MCP 连续调用默认可折成 `Called slack 3 times`，transcript 中展开。[E]
3. **结果状态**：错误走独立 error renderer；正常成功优先使用摘要和有限输出；权限和交互问题进入专用 dialog。[E]
4. **工作单元视图**：后台 shell 与 subagent 不把完整流塞回主会话；`/tasks` 管理当前会话后台项，Agent View 使用 row -> peek -> attach 完整会话的三级下钻。[E]
5. **renderer 能力**：classic 模式依赖原生 scrollback；fullscreen 模式提供内部 transcript、搜索/导出路径；screen-reader 模式改为 append-only 标签化线性文本。[E]

Claude 的关键语义优化包括：

- 连续 search/read/REPL 活动在 Agent 进度里聚合计数，而不是逐条复制 raw result（`../src/tools/AgentTool/UI.tsx:130-179`）。
- Agent 默认结果只显示状态、byline 和管理/展开入口，transcript 模式才显示 prompt 和完整子进程消息（`../src/tools/AgentTool/UI.tsx:315-360` 起）。
- MCP 输入在非 verbose 下限制单字段长度；大响应给 context 风险告警；已知动作可以转成领域句子，例如 Slack send（`../src/tools/MCPTool/UI.tsx:20-149`）。
- Agent teams 的 idle 行在 30 秒后隐藏；超过 3 个 idle agent 时聚合为 `N idle agents`，但新活动会重新出现；权限请求上浮到 lead。[E]

**[I] Claude 的实际规则不是“是否 verbose”这么简单，而是 `tool family + lifecycle + outcome + repetition + surface + user override`。**

### 3.2 Codex

Codex CLI 的源码边界更明确：每个 `HistoryCell` 可以分别提供主视图、transcript 和 raw 输出。

1. **主视图预算**：命令 cell 默认显示命令头、最多 2 行续行和 5 行输出；中间省略时保留头尾并显示 `… +N lines (ctrl + t to view transcript)`（`codex-rs/tui/src/exec_cell/render.rs:24-255, 362-672`）。[E]
2. **语义命令分类**：读取、列目录和搜索被归为 `Exploring/Explored`，连续 read 合并对象；普通未知命令显示 `Running/Ran`（同文件 `264-470`）。[E]
3. **完整 transcript**：`transcript_lines` 包含完整命令、完整格式化输出、exit code 和 duration；`Ctrl+T` 打开包含 committed history 和 live tail 的 overlay（`app_backtrack.rs`、`pager_overlay.rs`）。[E]
4. **工具专属 cell**：WebSearch 是一行语义活动；Patch 是文件级变更摘要；MCP 显示 server/tool/arguments，并限制主视图结果行数；reasoning 只展示产品提供的 summary，不暴露隐藏原始推理（`history_cell/search.rs`、`patches.rs`、`mcp.rs`、`messages.rs:218-283`）。[E]
5. **subagent thread**：spawn/send/wait/resume/close 都形成结构化 history cell；prompt 预览限制 160 grapheme，完成结果 240，错误 160；`/agent` 切换 thread，非当前 agent 的审批仍可上浮并标注来源（`multi_agents.rs:203-660` 与官方手册）。[E]
6. **App/IDE**：官方手册说明 app 展示每个 subagent thread，可打开检查；IDE 的 background-agent panel 可展开、停止全部或进入单个 thread。[E]

**[I] Codex 的核心是“同一事实的多个投影”，而不是对已经截断的文本再做展开。**

### 3.3 直接比较

| 维度 | Claude Code | Codex | 本项目应采用 |
| --- | --- | --- | --- |
| 决策位置 | 分散在工具 renderer 与专用视图 | `HistoryCell` 的 display/transcript/raw + 专用 cell | 集中 policy 下限，工具 formatter 决定内容 |
| 普通成功 | 一行/有限结果，工具语义强 | 一行/5 行头尾，语义分类中等 | 一行领域摘要；不显示无意义 raw |
| 长输出 | OutputLine/verbose/transcript | 主视图头尾 + 明确省略数 + Ctrl+T | D2 显示统计和关键失败；D3 pager/export |
| 重复读取/搜索 | 可聚合计数 | `Explored` 合并连续 read | 在稳定 work-unit 内聚合，保留成员索引 |
| 错误 | error renderer，默认高可见 | 红色/失败 cell，保留结果 | 至少结构化 D2，回答部分结果和下一步 |
| 审批 | 专用 dialog，teammate 请求上浮 | approval overlay，可标来源 thread | D3 Decision，不进入普通工具折叠 |
| subagent | 主会话摘要；tasks/agent view/team panel | 独立 thread；主线程状态 cell；`/agent` | 组摘要 + row + peek + full transcript |
| 推理 | transcript 可展示产品允许的 thinking | 只展示 reasoning summary | 只展示用户可见 reasoning summary |
| 可访问性 | 独立 append-only screen-reader 模式 | rich/raw/transcript 多投影；公开资料较少 | 共用语义 policy，独立线性 renderer |
| 完整证据 | transcript、attach、editor/scrollback | transcript/raw/history | immutable DetailStore + search/export |

## 4. 本项目的统一展示决策

### 4.1 四档深度

| 等级 | 名称 | 行为 |
| --- | --- | --- |
| `D0 Hidden member` | 不生成独立 transcript 行 | 只允许无语义变化 tick 或已进入可见聚合组的低风险成功成员；observation/evidence 仍保留 |
| `D1 Folded` | 一行领域摘要 | actor + action + object + outcome + 1-2 个关键指标 + has-more |
| `D2 Structured` | 结构化判断摘要 | 影响、部分结果、warning/error、自动动作、下一步和详情入口 |
| `D3 Evidence` | 完整逻辑内容 | 完整 Decision、stdout/stderr、diff、response、子 transcript；大内容进入 pager/editor/export |

本项目已有 `Summary/Detail/Evidence`，只需要在 projection/aggregation 层增加 D0；D0 不能删除 observation。

### 4.2 不可覆盖的优先级

从高到低：

1. **脱敏与访问控制**：先净化，再决定深度；用户要求 full 也不能泄露 secret。
2. **用户明确要求完整、audit 或已经 pin**：D3。
3. **审批、认证、冲突、破坏性动作执行前**：D3 Decision，禁止聚合。
4. **用户消息、assistant 最终答复、用户直接要求的 raw 输出**：D3，可分页不可有损。
5. **orphan/conflict/证据完整性失败**：至少 D2。
6. **failed/partial/denied/cancelled/timed_out/disconnected/shutdown**：至少 D2。
7. **warning/retrying/stalled/truncated/scope expanded**：至少 D2。
8. **已发生写入、删除、push、外发消息、Agent 完成、ready for review**：至少 D2。
9. **普通 queued/running/成功**：D1，原地更新。
10. **可安全聚合的重复低风险成功**：成员 D0，组 D1。
11. **无变化 tick/heartbeat/layout event**：D0。

输出体积、终端宽度和动画偏好只能改变 surface 和排版，不能降低上述下限。

### 4.3 决策伪代码

```text
retain(raw, envelope, identity)            // 与展示深度独立
safe = redact(observation, access_scope)

if presentation_tick_without_state_change: D0
level = D1
if warning/retry/stall/truncated:          level = max(level, D2)
if failure/partial/denied/timeout:         level = max(level, D2)
if orphan/conflict/integrity_error:        level = max(level, D2)
if side_effect/needs_review/agent_done:    level = max(level, D2)
if decision/auth/conflict/destructive_pre: level = D3
if user_or_final_or_requested_raw:         level = D3
if user.inspect:                           level = min(D3, level + 1)
if user.full/audit/pinned_evidence:        level = D3

surface = decision ? overlay
        : level == D3 && (huge || narrow) ? pager/editor/export
        : background ? work_view + transition_line
        : transcript
```

### 4.4 允许聚合的必要条件

只有同时满足以下条件才可把成员降为 D0：

- 同 session、turn、actor、work unit、command family 和领域意图；
- outcome=succeeded、risk=low、无副作用、无 warning、无需 review；
- 用户没有 pin/展开；
- 组仍处于 live 状态；
- 组保存所有 member ID、对象范围、聚合指标和 evidence refs；
- 任一成员出现 warning/error/approval 时立即拆出独立 D2/D3 行。

禁止仅凭“同名工具 + 5 秒窗口”聚合，也禁止按 transcript 邻接关联并行结果。

## 5. 每个注册命令应该展示什么

### 5.1 工具命令

通用字段不在每行重复：所有命令都需要稳定 `tool_use_id`、session/turn/work-unit/actor、lifecycle、outcome、started/finished、has-more 和 evidence refs。表中列出领域上必须增加的字段。

| 命令 | 默认/运行 | 成功终态 | 失败/警告 | D3 完整内容 |
| --- | --- | --- | --- | --- |
| `Bash` | command label/安全命令、cwd、sandbox、elapsed；后台时 task ID | exit、duration；测试命令显示 pass/fail/skip；有副作用至少 D2 | exit/signal、stderr 摘要、partial effects、retry/next action | 原命令、stdout、stderr、env/cwd、进程/timeout envelope |
| `PowerShell` | 同 Bash，额外标 PowerShell host | exit、duration、结构化测试摘要 | 同 Bash，区分 policy/Windows process error | 完整 command/stdout/stderr/host metadata |
| `Read` | path、line range；批量 read 可聚合 | lines/bytes/encoding，D1 | 缺失/权限/编码/范围错误 D2 | 完整安全内容与读取元数据 |
| `Write` | path、create/overwrite、bytes/lines、risk | `Created/Overwrote <path>` + diffstat，D2 | 是否已写部分内容、rollback/backup | 完整新内容、原内容或 diff、写入回执 |
| `Edit` | path、replace scope/count | files、+/-、no-op，D2 | 匹配失败、冲突、已完成替换 | 完整 structured patch/diff/原新片段 |
| `NotebookEdit` | notebook、cell ID/type、mode | cell 变更、diffstat | cell 不存在、格式错误、部分保存 | 完整 cell source 与 notebook patch |
| `Glob` | pattern、root/scope、include/exclude | file count、top paths；批量可 D0+组 D1 | scope/truncation warning 或 I/O 错误 | 全部路径，可搜索/分页 |
| `Grep` | query/regex、scope、mode/context | match count、file count、top locations | regex/permission/truncated/scope expanded | 全部命中与上下文 |
| `LSP` | action、symbol/position、server/language | result/diagnostic count、top location | server unavailable/stale/partial | 完整 diagnostics/locations/protocol result |
| `WebSearch` | query、search mode/provider | result/source count、top citations | policy、rate limit、partial/open-world warning | 完整结果集与引用元数据 |
| `WebFetch` | safe URL、method、host、redirect phase | final URL、status、content type、bytes | network/policy/content/redirect 分型 | headers、允许展示的完整 body、redirect chain |
| `MCP` / dynamic `mcp__*` | server + capability/tool + 安全参数摘要 | server、outcome、latency、领域结果 | auth/schema/disconnect/timeout，不能只说 tool failed | 完整 request/result envelope 与 server metadata |
| `ListMcpResourcesTool` | server/filter | resource count、type/URI 摘要 | server/auth/degraded | 完整 resource descriptors |
| `ReadMcpResourceTool` | server、URI、media type | size/type/title | auth/not-found/unsupported/truncated | 完整 resource 或媒体引用 |
| `Agent` | agent id/nickname/role、objective、parent、queued/running phase | result preview、artifacts、verification、duration/tools/tokens/cost，D2 | failed/partial/timeout/cancel + partial effects + next action | 完整子 thread/transcript、工具证据与结果 envelope |
| `TaskCreate` | task id、subject、owner/dependencies | created receipt，通常 D1 | validation/hook/persistence error D2 | 完整 task record 与 hook receipt |
| `TaskList` | scope/filter | pending/running/blocked/completed counts + active top items | stale/incomplete state warning | 完整列表与依赖 |
| `TaskGet` | task id/subject | state、owner、blocks/blockedBy、latest update | not found/corrupt | 完整 task record/history |
| `TaskUpdate` | task id、from -> to、owner/dependencies | transition receipt；completed 显示 result/verification | invalid transition/blocked/partial | 完整前后 record 与 hook evidence |
| `TaskStop` | task id、command/agent | stopped + initiator + partial output | already done/not found/kill failure | 完整 task/process lifecycle |
| `TaskOutput` | task id、follow/block mode | status + new output size/offset | timeout/still-running/evicted | 完整 retained output，支持 offset/pager |
| `TodoWrite` | active item + pending count | checklist delta，routine D1 | invalid/stale update D2 | 完整 checklist/history |
| `GetGoal` | goal status/objective 摘要 | usage/budget/status | missing/corrupt goal | 完整 goal metadata |
| `CreateGoal` | objective、budget（若有） | goal id/status receipt，D2 | existing-goal conflict/invalid budget | 完整 goal metadata |
| `UpdateGoal` | goal id、requested terminal state | complete/blocked receipt + usage | premature completion/invalid blocked state | 完整 lifecycle/audit |
| `EnterPlanMode` | mode transition、scope | `Plan mode active`，D1/D2 | transition/permission failure | mode contract与上下文 |
| `ExitPlanMode` | plan artifact、review state | approved/rejected/edited + post mode，D2 | plan/hook/permission failure | 完整 plan、review details、Decision receipt |
| `AskUserQuestion` | 完整问题、choices、impact | selected answer receipt，D2 | timeout/escape/cancel 分开 | 原问题、全部 choices、answer/audit |
| `SendUserMessage` | 正式消息保持 D3 | delivery timestamp/attachments | delivery/attachment failure D2 | 完整消息与附件元数据 |
| `SendMessage` | sender -> recipient、消息类型、摘要 | delivered/queued ack；routine 可聚合 | unknown recipient/mailbox failure | 完整消息、routing 与 delivery receipt |
| `TeamCreate` | team name、lead、member plan | created + member count，D2 | conflict/persistence/permission | 完整 config、members、task scope |
| `TeamDelete` | team name、active member/task impact，待确认 D3 | deletion/shutdown receipt，D2 | partial shutdown/orphan member | 完整 cleanup/audit |
| `CronCreate` | schedule/timezone、task summary、next run | job id + next run，D2 | invalid schedule/persistence | 完整 schedule/payload |
| `CronList` | scope/timezone | active/paused count + next runs | store/decode error | 完整 jobs |
| `CronDelete` | job id + schedule/task impact | removed receipt，D2 | not found/active run handling | 完整 prior job/cleanup receipt |
| `EnterWorktree` | repo/base/target/path、dirty state | final cwd/branch/worktree，D2 | conflict/dirty/creation failure | 完整 git output、diff/status |
| `ExitWorktree` | current worktree、changes/branch、cleanup choice | resulting cwd + kept/removed state，D2 | dirty/conflict/cleanup partial | 完整 git/worktree evidence |
| `Config` | key、layer/source、old -> proposed | applied layer/effective value，D2 | validation/policy/restart required | 完整 config diff与来源链 |
| `Skill` | skill name/source、action | loaded/executed + output artifact | invalid/missing/unsafe script | 完整 skill result与引用资源 |
| `ToolSearch` | query、deferred pool scope | matches + selected tools | no match/incomplete catalog | 完整 match score/metadata |
| `RemoteTrigger` | target/session/remote action、risk | trigger id + remote status，D2 | auth/network/remote rejection | 完整 remote receipt |

补充规则：

- `TestingPermission` 等测试/内部工具默认不进入生产可见命令表；若意外出现在生产，按 orphan/configuration error D2 呈现。
- Server-side `web_search_*`/`web_fetch_*` 归入 Web family，不因协议名称不同创建另一套视觉语义。
- 任何新增工具必须注册 formatter；缺少 formatter 时使用安全 fallback：D1 显示 tool + object keys + outcome，非成功强制 D2，并保留 D3 envelope。

### 5.2 Slash 命令

Slash 命令当前通过 `commands.Context.OnEvent(string)` 输出普通文本，还没有 typed presentation model。首版保留兼容，但应按以下合同展示；长期增加可选 `CommandPresentation{Kind, Outcome, Summary, Sections, EvidenceRefs}`，避免各命令自行截断。

| 命令 | 默认应展示 | 完整/失败合同 |
| --- | --- | --- |
| `/exit` | 是否存在未完成工作、保存/关闭结果 | 需要确认时 D3；关闭失败说明未完成清理 |
| `/help` | 按 registry 生成的完整命令目录与简短描述 | 不聚合；窄屏 reflow，不静默截掉命令 |
| `/clear` | 清理范围、是否保留 session/evidence 的一行回执 | 失败说明未清理部分；不能让“清屏”暗示删除审计证据 |
| `/goal` | active objective、status、budget/usage；创建/更新显示前后状态 | 冲突、premature complete、blocked 规则错误走 D2 |
| `/search` | query、命中数、当前位置与 next/previous/close 状态 | 完整结果仍在 transcript；无结果和索引错误分开 |
| `/export` | 导出范围、格式、绝对路径、记录/字节数 | 写入失败、部分文件和脱敏策略走 D2 |
| `/editor` | 打开的 transcript/detail 对象和 editor | editor 未配置、临时文件或启动失败走 D2 |
| `/mouse` | `on/off` 最终状态 | 不支持/切换失败显示原因与键盘替代路径 |
| `/activity` | list/close/cancel/jump/details 的明确回执；list 进入 work view | 操作失败带 activity ID、当前状态和合法动作 |
| `/detail` | observation ID、原 -> 新 disclosure、has-more | ID/level 无效走 D2；不得改变全局 show-all 或丢 evidence |
| `/compact` | compaction 范围、前后 context 指标、保留的 evidence/goal | partial/failure 显示自动动作和可恢复状态 |
| `/model` | provider/model、context window、价格是否已知、持久化位置 | switch 失败保留旧模型；unknown cost 明示 unknown |
| `/session` | current/list/load/rename/delete 的结构化 session 信息 | delete 必须 D3 Decision；load/rename 失败不改变当前 session |
| `/config` | key、layer/source、effective value；修改显示 old -> new | validation/policy/restart required；secret 必须脱敏 |
| `/status` | session、provider/model、mode、work/cost/goal 的可扫描快照 | inspector 保留多行结构，未知值不伪装为零 |
| `/context` | used/available、compaction 阈值、主要占用来源 | 估算与精确值分开；溢出/不可用走 warning |
| `/init` | 创建/更新的配置文件和作用域 | 已存在/no-op、冲突、部分写入和 rollback |
| `/resume` | 候选 session 或已恢复 session、时间/目录/branch/message count | 恢复失败不污染当前 session；旧 schema 警告可下钻 |
| `/review` | review scope、发现计数、严重度与详情入口 | 无发现也说明测试范围；执行失败与“0 findings”分开 |
| `/doctor` | checks passed/warned/failed、修复建议 | inspector 不做一行截断；每个失败保留证据和命令建议 |
| `/mcp` | server 状态、capability/resource/tool count、auth 状态 | auth/schema/connectivity 分型；动态 MCP prompt 命令沿用 server/capability 合同 |

当前源码还保留 `/cost`、`/version` 等未注册实现；它们不能被当作生产命令覆盖率。若重新注册，`/cost` 必须区分 known/unknown price 并展示 per-model breakdown，`/version` 显示 runtime/version/build source 的一行确定性回执。

## 6. Subagent 与并行活动展示

### 6.1 三层界面

1. **Signal**：`Research · 6 agents: 3 running, 1 needs input, 1 ready, 1 failed`。只回答是否推进和是否要介入。
2. **Work view row/peek**：每个 agent 一行；选择后展示完整状态句、阻塞问题、产物和控制动作。
3. **Full thread**：进入独立子 thread/transcript，可搜索、导出、继续或停止。

主 transcript 只保留 spawn、needs-input/approval、重要 warning 和 terminal result。绝不默认交错复制每个子 agent 的 token、spinner、工具调用和 raw stdout。

### 6.2 行信息合同

| 状态 | 必须显示 | 默认深度 |
| --- | --- | --- |
| queued/spawned | nickname/role、stable ID、objective、parent、cwd/worktree、queued time | D1 |
| running | phase、elapsed、latest semantic activity、progress、last update、inspect/cancel | D1 原地更新 |
| needs input/approval | requester、完整问题/动作/目标、阻塞原因、choices、scope、deadline | D3 overlay + 独立主线 |
| stalled/retrying | cause、attempt、last progress、自动动作、何时升级 | D2 |
| ready/completed |正式 result、artifacts/files、verification、duration、tools、usage/cost、thread ref | D2 |
| failed/partial/timeout | root cause、partial effects、last tool、cleanup/retry、next action、evidence | D2；需选择时 D3 |
| cancelled | initiator、reason、partial effects、children disposition | D2 |
| resumed/attached | previous state、new focus/owner、unread transitions | D1 |

排序固定为：`Needs input -> Ready for review -> Failed/Partial/Timed out -> Running -> Completed -> Cancelled/Closed`。颜色仅作冗余编码，状态文字不可省略。

### 6.3 父子和并发规则

- 结构键是 `parent_agent_id -> agent_id -> work_unit_id`，不是 tool name 或消息邻接。
- 父行显示后代计数与最严重后代状态；子错误不能被父级“5/6 completed”吞掉。
- 父 agent 结束时必须说明子项是 completed、cancelled、detached 还是仍在 background。
- nested agent 使用 breadcrumb/path，不做卡片套卡片；折叠树仍保留异常后代计数。
- `agent_id` 表示身份，`run_id + attempt` 表示一次初始执行或 resume；旧 run 的终态不可阻止新 run 进入 running，也不可覆盖旧证据。
- work unit 与 agent 身份分开：任务可换 agent，agent 也可跨多个 work unit；聚合和归因不能只用 nickname/profile。
- lifecycle 与 attention 分开：`completed + ready_for_review`、`blocked + needs_input` 都是合法组合；attention shelf 可以穿透父组折叠。
- queued/waiting 必须给出 `capacity/dependency/retry_backoff/depth_limit` 等 reason code；达到并发或嵌套深度上限不能表现成无解释的 spinner。
- token/cost 不常驻抢占状态，放在 D2/D3 和组总计中；异常成本跃迁可升为 warning。
- 模型 headline 只能补充自然语言；状态、完成、失败、文件变更和验证必须来自确定性事件。
- inactive agent 的 permission 必须上浮，显示来源 thread，并允许先打开该 thread 再决定。

### 6.4 六并发演练

```text
Research · 6 agents: 2 running, 1 needs input, 1 ready, 1 failed, 1 completed

! Atlas [repo-map]      Needs input   Which generated files are in scope?
✓ Delta [codex-docs]    Ready review  14 sourced findings · 3 unknowns
✗ Echo [bench]          Failed        go test timeout · partial logs retained
• Nova [claude-docs]    Running       Comparing Agent View and /tasks · 2m14s
• Orion [policy]        Running       Validating 34 command contracts · 18/34
✓ Vega [a11y]           Completed     Screen-reader matrix · 9 tests · 46s
```

- Atlas 的完整问题作为 D3 Decision 独立出现，不能只显示组计数。
- Echo 保留失败原因、partial logs 和 retry/next action；组行保持 `1 failed` 直到用户确认。
- Delta/Vega 只在 terminal transition 追加一次主 transcript 行。
- Nova/Orion 的内部工具进度只原地更新 work view。
- 选择任一行打开 peek；再次操作进入完整 thread。关闭后恢复原焦点、scroll 和 input draft。

### 6.5 Screen reader

线性顺序固定为 `group summary -> exception members -> running members -> completed members`。只朗读首次 running、关键 milestone 和 terminal transition；不朗读 spinner/tick。示例：

```text
Agent group: 6 total. 1 needs input. 1 failed. 2 running. 2 ready or completed.
Agent Atlas, needs input: Which generated files are in scope? Decision available.
Agent Echo, failed: go test timeout. Partial output retained. Details available.
```

## 7. 本项目当前实现

调查快照约有 1,395 个 Go 文件、37.6 万行；`registry_setup.go:623-822` 有 42 个静态工具注册点，运行时还可增加动态 MCP 工具。这解释了为什么完整方案必须使用 command family + 安全 fallback，不能为每个工具继续堆一个 `root.go` 分支。

### 7.1 数据流

```text
loop.Event / ToolUseBlock / ToolResultBlock / Permission
  -> ui.ToolEventContext (session, turn, work unit, actor, outcome)
    -> tui.TuiRenderer
      -> ObservationStore --exact bytes/envelope--> DetailStore + journal
      -> ActivityStore
      -> Decision audit
        -> AppState session projection
          -> RootComponent / ScreenReaderRenderer / transcript search/export
```

### 7.2 可直接复用

| 能力 | 当前证据 | 判断 |
| --- | --- | --- |
| 稳定 call/result 关联 | `tui/observation_store.go:183-337`，用 session + ToolUseID；orphan/conflict 显式化 | 已具备 |
| 三档 disclosure | `tui/observation_store.go:107-122`、`tui/state.go:1017-1079` | 已具备 |
| outcome 下限 | `tui/observation_store.go:522-532`，非成功默认 Detail | 已具备但输入维度不足 |
| 无损 evidence | `tui/detail_store.go:32-55, 106-230`，内存/文件、SHA-256、0600、journal | 已具备 |
| 完整结构 envelope | `tui/observation_store.go:485-503` | 已具备 |
| 会话持久化 | `session/session.go:96-143`、`repl_tui.go:1627-1774` | 已具备 |
| 全局 show-all 不污染局部状态 | `tui/root.go:1161-1164` 与测试 | 已具备 |
| Activity reducer | `tui/activity_store.go:23-128, 140-183, 280-324` | 已具备 |
| actionability 排序 | `tui/activity_store.go:419-454` | 已具备 |
| Work view | `tui/root.go:1638-1778`，按 work/actor 显示 | 已具备基础 |
| Agent 领域摘要 | `tui/root.go:1465-1600`，completed/backgrounded/teammate | 已具备部分 |
| 结构化 Decision | `permissions/structured_prompt.go:5-54` | 已具备 |
| Screen reader | `ui/screen_reader_renderer.go`，append-only、决策输入仲裁 | 已具备 |
| 搜索/导出/性能 | `tui/transcript_io.go`、`tui/performance_acceptance_test.go` | 已具备 |

若按基础组件清单计，现有架构底座约有 75-85% 已就位；这不是剩余工程量或用户体验完成度。领域摘要、策略解释、全命令覆盖、并发竞态和真实终端验证仍是主要成本。

### 7.3 关键缺口

| 缺口 | 当前表现 | 推荐边界 |
| --- | --- | --- |
| 统一 policy | `defaultResultDisclosure` 只看 outcome/isError | 新增纯函数 `presentation/policy.go`，输出 level/surface/reason codes |
| 领域 formatter | `toolInputPreview` + 通用 bytes/line summary | `presentation/formatters/*.go`，按 family 注册 |
| D0 聚合 | Activity 有分组，transcript observation 仍逐项 | `presentation/aggregate.go` 保存 group/member/evidence refs |
| 成功语义 | Summary 常见为 outcome + bytes | 解析结构化 result/data，不从任意 prose 推断终态 |
| Agent 生命周期 | Agent result JSON 有 completed/backgrounded 等，但 Activity 字段不足，terminal lock 也不能表达同 agent 的新 run/resume | 用 `agent_id + run_id + attempt` 区分身份与执行，补 parent/objective/phase/duration/usage/artifacts |
| cost/duration | Agent payload有部分 usage；Activity 没有统一时间/成本字段 | 在 event/metrics 中补 started/updated/finished/usage，不塞 renderer 临时状态 |
| 错误合同 | 非成功会展开，但不总能回答 partial/automatic/next action | error formatter 固定五段合同 |
| formatter fallback | 未知工具走通用输出 | fail-safe fallback + formatter coverage test |
| 动态 test gate | 调查期间同一 dirty tree 先因签名漂移失败、随后通过 | 实施前冻结 SHA/patch 和 owner，再锁定基线；不能只引用一次偶然绿灯 |

## 8. 推荐实施方案

本节是依赖顺序，不按标题数字直接相加；统一预算以 9.2 的增量/累计表为准。

### Phase 0：恢复可信基线

- 冻结实施起点的 SHA/patch 与共享文件 owner，重跑全套测试；把调查期间出现后又被并发改动修复的签名漂移记录为基线风险。
- 跑 `go test ./...`、`go test -race ./...`、`go vet ./...`，记录既有非相关失败。
- 为现有 Summary/Detail/Evidence、Agent result、screen reader 和 Activity snapshot 加 golden/table tests。

### Phase 1：纯策略层

- 新增 `PresentationFacts`、`PresentationDecision{Level, Surface, ReasonCodes}`。
- 编码本报告优先级；先覆盖 failure/decision/side-effect/user-intent。
- 保持 renderer 只消费决定，不在 `root.go` 再推断风险或 outcome。

### Phase 2：核心 formatter

- 先做 Bash/PowerShell、Read/Write/Edit、Glob/Grep、Web、MCP、Agent、Task、Decision。
- 每个 formatter 覆盖 queued/running/success/warning/error/expanded。
- 增加 fail-safe generic formatter 和全 registry coverage test。

### Phase 3：聚合与 subagent

- D0 只发生在 presentation projection；observation/evidence 不删。
- 实现 read/search/MCP routine success 聚合。
- 补 Agent lifecycle facts、组计数、异常提升、row/peek/full-thread 路径。
- 事件乱序、late result、parent cancellation、inactive approval 必须有测试。

### Phase 4：全命令、迁移与多 renderer 对齐

- 补齐表中所有注册命令。
- fullscreen/classic/screen-reader 共用 policy，分别测试布局协议。
- 持久化用户 pin、group frozen summary 和 reason codes；旧 session 做向后兼容默认值。

### Phase 5：生产验证与稳定化

- 40x12、80x24、120x40、CJK/emoji/长路径。
- 100,000 observations、长 stdout、二进制/ANSI/control char、detail integrity。
- macOS/Linux PTY、tmux、IDE terminal、screen reader；Windows 至少 compile/link + 单测。
- 真实 6-agent 混合状态、权限上浮、故障注入、取消/恢复、会话切换。

## 9. 成本估算

### 9.1 假设

- 1 人日按 6 小时有效工程时间计算，不包含组织审批和产品排队时间。
- 团队已熟悉 Go、go-tui、当前 session/runtime 设计；新人增加 25-40%。
- 不引入新依赖，不重写 renderer，不同时修改工具业务语义。
- Claude/Codex 像素级复刻不在范围；目标是行为和信息合同。
- 当前 dirty tree 同时触及 TUI、REPL、Agent、cost 和 i18n 共享边界；这些改动属于用户工作，不能被本报告当作免费完成项，集成冲突和基线恢复计入风险区间。

### 9.2 按工作流估算

| 工作流 | MVP 增量 | 完整对齐增量 | 生产加固增量 | 主要不确定性 |
| --- | ---: | ---: | ---: | --- |
| 可信基线与回归锁定 | 1 / 2 / 3 | 0 / 1 / 2 | 1 / 1 / 2 | 当前 dirty tree 持续变化，调查期间出现过短暂 TUI 编译失败 |
| Policy、facts、schema/兼容 | 2 / 3 / 5 | 1 / 2 / 4 | 1 / 1 / 2 | 是否需要跨 session 持久化新 facts |
| 命令族 formatter | 3 / 5 / 8 | 3 / 5 / 8 | 1 / 2 / 3 | 工具 result 的结构化程度不一致 |
| 聚合与 subagent/work view | 1 / 2 / 3 | 4 / 6 / 10 | 2 / 3 / 5 | live/frozen/late event、resume 和 parent/child |
| TUI/terminal/screen reader | 1 / 2 / 3 | 2 / 3 / 5 | 2 / 4 / 6 | 真实终端矩阵、焦点恢复和 append-only 语义 |
| 测试、review 与稳定化 | 2 / 4 / 6 | 3 / 6 / 8 | 5 / 7 / 10 | race/PTY/100k observation/long output |
| 文档、迁移与回滚演练 | 0 / 0 / 0 | 1 / 1 / 2 | 2 / 3 / 5 | 旧 session、灰度和用户反馈迭代 |
| **阶段增量** | **10 / 18 / 28** | **14 / 24 / 39** | **14 / 21 / 33** | 逐行求和，不重复计费 |
| **累计** | **10 / 18 / 28** | **24 / 42 / 67** | **38 / 63 / 100** | 统一预算口径 |

- **最小可用：10-28 人日，Base 18**。只覆盖 policy、6 个核心命令族/约 12 个高频工具、Agent terminal summary 和关键回归。
- **完整对齐：累计 24-67 人日，Base 42**。覆盖全部注册命令族、聚合、subagent、screen reader 和迁移，但不承诺完整真实终端/平台故障注入。
- **生产加固：累计 38-100 人日，Base 63**。包含完整终端、性能、race、故障注入、灰度和稳定化。

### 9.3 人员与日历

| 配置 | 建议拆分 | 完整对齐基准日历 |
| --- | --- | --- |
| 1 名 senior | policy -> formatter -> subagent -> QA 串行 | 9-11 周 |
| 2 名 senior | A: policy/observation；B: formatter/work view；共同 QA | 5-6 周 |
| 3 名工程师 | policy owner + formatter owner + test/a11y owner | 4-5 周，但共享文件冲突上升 |
| 4-6 条 lane | 只有先拆包、冻结接口和指定 integration owner 后才有效 | 约 3-4 周；不能并发改 `root.go/state.go` |

### 9.4 最小替代与不做

| 方案 | 成本 | 得到 | 放弃/风险 |
| --- | ---: | --- | --- |
| 不做 | 0 | 保持现有三档 evidence 能力 | 成功摘要不语义化；并发和 subagent 判断成本继续偏高 |
| 只做 formatter | 5-10 人日 | 高频命令明显更可读 | 没有统一下限，未来工具继续漂移 |
| Policy + 6 个核心 family | 8-20 人日 | 高频价值，风险规则可测 | Agent 多 renderer 最低合同可能仍不完整 |
| 本文 MVP | 10-28 人日 | 核心策略、Agent 终态、多 renderer 最低合同 | 完整命令/聚合/work view/生产验证延后 |
| 完整方案 | 累计 24-67 人日 | 可持续扩展的一致合同 | 需要迁移、真实终端与跨模块协调 |

## 10. 红队与 pre-mortem

### 10.1 必须挑战的假设

1. **“基础设施 80% 完成，所以只剩 20% 工作”是错的。** 底座完成度和用户体验完成度不是线性关系；最后的 per-tool 语义、异常演练和真实终端验证通常最耗时。
2. **“有 DetailStore 就等于用户能找到详情”是错的。** 如果 summary 没有对象、范围和明确入口，证据存在也只是埋得更深。
3. **“一个 policy 能替代 formatter”是错的。** Policy 决定最低深度；formatter 决定什么信息值得放进去。没有 formatter，D2 只是更长的垃圾文本。
4. **“最高并发能线性缩短工期”是错的。** `tui/root.go`、`state.go` 和 event schema 是共享瓶颈；6 人同时改同一边界会把节省的时间还给冲突和回归。
5. **“竞品行为就是规范”是错的。** Claude/Codex 都在快速变化；应继承稳定设计原则，不绑定快捷键和像素。

### 10.2 Pre-mortem

| 六个月后的失败原因 | 早期信号 | 现在的预防措施 |
| --- | --- | --- |
| 界面很干净但关键失败被折叠 | 用户频繁打开 show-all、重复问“到底失败在哪” | 所有非成功 outcome 下限 D2；error 五段合同；failure golden tests |
| formatter 变成大量不可维护 switch | 每加工具都改 `root.go` | family registry + tool-owned facts + generic fail-safe；coverage test |
| 聚合把并发结果错配 | 同名 Bash/Agent 的结果偶发串行 | stable ID + group member index；禁止邻接/时间窗作为唯一键 |
| subagent 面板变成另一个日志瀑布 | 每个 tool tick 都新增行 | row 原地更新，只记录 semantic transition；full logs 留在 thread |
| screen reader 与视觉行为分叉 | 一边修了状态，另一边没有 | 共用 `PresentationDecision`，renderer 只改变布局/输出协议 |
| 旧 session 无法恢复 | 新字段上线后 disclosure/group 丢失 | 版本化 presentation meta + 缺省迁移 + resume fixtures |
| 并行开发拖垮 dirty tree | shared file 冲突和测试基线不断漂移 | 先修基线，按 package/owned file 分工，policy 接口先冻结 |

不可逆性约 4/10：这是内部 presentation 变更，可以 feature flag/配置回退；但一旦新 presentation metadata 持久化并被用户依赖，迁移和兼容成本上升。先保持 observation/audit schema 向后兼容，新的 group/policy 字段使用可选、版本化投影。

## 11. 并发任务拆分

任务位于 `tasks/information_display_strategy/`，总索引为 `INDEX.json`：

| 任务 | 内容 | owned output |
| --- | --- | --- |
| `task_01.json` | Claude Code 展示证据图 | `_agent_reports/task_01.md` |
| `task_02.json` | Codex 展示证据图 | `_agent_reports/task_02.md` |
| `task_03.json` | 统一 disclosure/命令合同 | `_agent_reports/task_03.md` |
| `task_04.json` | subagent/并发活动策略 | `_agent_reports/task_04.md` |
| `task_05.json` | 本项目实现与 gap map | `_agent_reports/task_05.md` |
| `task_06.json` | 成本、排期与红队 | `_agent_reports/task_06.md` |

工作区契约和 Codex 官方默认 `agents.max_threads=6` 支持 6 个逻辑并行子任务，因此拆成 6 份独立任务；本次实际运行时观察到同时只能启动 3 个 child agent，第 4 个返回 thread-limit，因此执行调度采用 `3 + 3` 两波。这个运行时限制不改变任务边界，但必须在估算中诚实记录，不能声称本次真的同时跑了 6 个。

六个 `task_N.json` 均已标记 completed，并记录各自报告路径；`_agent_reports/` 保留产品证据、统一政策、subagent、仓库映射和成本红队的完整独立结果，主报告只做交叉校准后的最终综合。

## 12. 验收标准

### 策略

- 每个非成功 outcome 的默认深度不低于 D2；pending decision 必为 D3。
- 进入 D0 的成员必须可由一个可见 group 找回 stable ID 和 evidence。
- 用户 full/audit 不受体积和窄屏降级；secret 仍受脱敏策略。
- 全局 show-all 不改变局部 pinned disclosure。

### 命令

- registry 中每个 production tool 都有 family formatter 或安全 fallback。
- 每个 formatter 覆盖 queued/running/success/warning/error/expanded。
- 测试、diff、search、web、MCP、Agent 采用结构化字段，不从最后一行猜终态。
- 写入/删除/push/外发消息即使成功也有 D2 回执。

### Subagent

- 用户能在三步内回答：谁在做什么、谁需要输入、谁失败、结果/产物在哪。
- inactive agent 的审批上浮且标明来源。
- 失败/partial/needs-input 不被父组或成功 sibling 吞掉。
- 完整子 thread 可搜索/导出；主 transcript 不复制工具洪流。

### 终端与可靠性

- 40/80/120 列、CJK、长路径、emoji 下关键状态不丢失、不重叠。
- screen reader append-only、无 spinner 噪音，状态有文字冗余。
- 100,000 observations 时渲染节点与 viewport + pinned/expanded 成比例。
- evidence 展开不重放工具；关闭恢复 focus/scroll/input draft。
- `go test ./...`、race、vet、PTY/terminal matrix 的结果明确记录，失败不能用窄测试掩盖。

## 13. 最终建议

1. 不重写现有 ObservationStore、DetailStore、Decision 和 ActivityStore。
2. 先冻结并锁定可重复测试基线，再建立 `PresentationPolicy` 的纯函数合同。
3. Policy 只决定深度、surface 和 reason codes；每个 command family 用 formatter 决定展示字段。
4. 优先交付 Bash/Read/Edit/Grep/Web/MCP/Agent/Task/Decision 这 12 个高频或高风险 formatter。
5. Subagent 采用 group signal -> row/peek -> full thread，不在主 transcript 流式复制子工具日志。
6. 所有 renderer 共用一个语义决定；fullscreen、classic、screen reader 只改变投影方式。
7. 第一阶段采用 feature flag 或配置回退，旧 session presentation metadata 保持向后兼容。

一句话：**主界面应该展示能改变用户下一步行动的信息；其余信息可以收拢，但必须可定位、可展开、可审计。**
