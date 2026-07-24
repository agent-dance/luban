# task_05 - Go 项目信息展示实现与缺口映射

> 调查时间：2026-07-15
> 调查方式：只读检查当前工作树、`HEAD` 对照、实现符号和既有测试；未修改任何现有实现文件。
> 证据口径：本文的行号均指调查时的**当前工作树**。`[HEAD]` 表示已提交基线，`[DIRTY]` 表示当前未提交增量，`[TARGET]` 表示目标展示策略而非现状。

## 1. 结论

这个项目已经完成了展示策略最难逆转的基础工程，不应重写 TUI：

- 工具 call/result 已按 `SessionID + ToolUseID` 关联，不依赖邻接或完成顺序。
- 原始文本和结构化 `ToolResultBlock` 已进入不可变 `DetailStore`，摘要与证据分离。
- 每个 observation 已有 `Summary -> Detail -> Evidence` 三档披露、局部 pin、全局 show-all、搜索、导出和 editor 下钻。
- `ActivityStore` 已用稳定 ID 原地更新 tool/agent/background/MCP/decision/hook 状态，并按 actionability 排序。
- Decision 已从普通工具输出中分离，fullscreen 与 screen-reader 都有专用交互面。
- Agent 已有 completed/error/aborted/partial 结构化 union、后台持久通知和基础领域摘要。

真正缺少的是一个位于 `ObservationStore` 与各 surface renderer 之间的**语义展示政策层**。当前默认披露只由 outcome/error/evidence 决定；成功结果统一退化为“状态 + 字节数”，没有命令族 formatter、D0 聚合成员、warning/side-effect/risk/intent 下限，也没有完整的 subagent work-view 行合同。

最小可行边界是：

```text
现有 Runtime Event / ToolResultBlock
  -> 现有 ObservationStore + DetailStore（不改所有权）
  -> 新增纯函数 PresentationPolicy + FormatterRegistry
  -> 现有 Message / Activity 投影
  -> 现有 fullscreen / screen-reader / JSON surfaces
```

首版不需要新依赖，不需要更换 `go-tui`，也不需要修改工具执行协议。应先覆盖 10-12 个高频命令族和 Agent terminal result，再增加安全聚合。

## 2. 当前架构与 event-to-render 数据流

### 2.1 主数据流

| 阶段 | 当前实现与精确入口 | 当前行为 | 目标映射 |
| --- | --- | --- | --- |
| 运行时事实 | `loop/events.go:34` `Event`；`loop/query.go:929` `runLoop`；tool call/result 分别在 `loop/query.go:1620`、`:1655`、`:1662` 发出 | Event 携带 `TurnID/ProjectRoot/ActorID/ActorType/WorkUnitID`；result 携带明确 `ToolUseID/Outcome/Data/Metadata/Usage` | 复用。这里是 presentation policy 的确定性事实来源，不从 prose 猜状态 |
| surface adapter | `repl_tui.go:2719` `makeTUIEventHandler`；`repl_screen_reader.go:727` `makeScreenReaderEventHandler`；print/REPL 在 `render.go:113`、`:150` | 把 session/epoch 基线上下文与 event 上的 turn/actor/work-unit 合并；分发 text/tool/progress/error/usage | 复用，但把共享 policy 放在 renderer 之前或共享 presenter 中，避免 fullscreen 与 screen-reader 分叉 |
| renderer 能力分派 | `ui/send_user_message.go:34` `ToolEventContext`，`:71` `StructuredToolRenderer`，`:123` `DispatchToolCallEvent`，`:148` `DispatchToolResultEvent` | identity-aware renderer 收完整 block；旧 renderer 降级为字符串；Brief/SendUserMessage 走专用可见消息通道 | 复用 optional-interface 兼容边界；新增 formatter 不应破坏 legacy renderer |
| TUI 收件 | `tui/renderer.go:253` `RenderToolCall`，`:267` `RenderToolResult`，`:280` `observationOutcomeForResult`，`:302` `toolObservationContext` | epoch gate 后把完整 call/result 送入 `AppState`；结构化 outcome 优先，缺省才由 `IsError` 回退 | 复用；policy 不应在这里丢弃 raw payload |
| observation reducer | `tui/state.go:856` `applyToolCall`，`:895` `applyToolResult`；底层 `tui/observation_store.go:185`、`:241` | call 创建一条稳定 observation；result 原地更新同一 transcript anchor，并同时更新 activity | 复用；这是接入 presentation decision 的最小位置 |
| 证据保留 | `tui/observation_store.go:247-267` 保存文本与结构化 envelope；`tui/detail_store.go:43` `DetailStore`；`:137` file-backed `Put`；`:206` evidence journal | 先保存完整 evidence，再做摘要；文件证据 SHA-256、0600、原子发布并校验 size/digest | 完全复用；任何 D0/D1 都只改变投影，不能改变这里的保留 |
| transcript projection | `tui/state.go:1283` `messageFromObservation`，`:1301` `observationSummary`；`tui/root.go:1156` `renderToolObservation` | Message 只是 observation anchor；render 时重新读取 observation 和 disclosure | 复用 anchor；重构 `observationSummary` 为 formatter registry 输出 |
| disclosure rendering | `tui/root.go:1161-1233`；`:3977-3990` Ctrl+O/Alt+O；`tui/state.go:1017-1079` reveal/cycle | Summary 显示 outcome/bytes/has-more；Detail 显示完整 input + result；Evidence 再显示 envelope + identity；局部展开可返回原焦点 | 复用交互和存储；扩展成 D0/D1/D2/D3 语义即可 |
| bounded history | `tui/root.go:878`、`:907` `boundedTranscriptMessages` | viewport window + overscan；pin/focus 可把窗口外 observation 带回，避免 100k history 构造全树 | 复用；聚合组也必须保留稳定 member index，不能绕过窗口约束 |

完整因果链如下：

```text
provider stream
  -> QueryLoop emits loop.Event
  -> makeTUIEventHandler merges session/epoch/actor/work-unit identity
  -> ui.DispatchTool*Event preserves complete ToolUseBlock/ToolResultBlock
  -> TuiRenderer epoch gate
  -> AppState.applyToolCall/applyToolResult
      -> ObservationStore (stable identity + disclosure)
      -> DetailStore (raw text + structured envelope)
      -> ActivityStore (in-place lifecycle/actionability)
      -> Message anchor (visible transcript)
  -> RootComponent reads Observation at render time
      -> Summary / Detail / Evidence
      -> bounded transcript / activity overlay / decision overlay
```

### 2.2 Observation、Detail 与 disclosure 的现状

`tui/observation_store.go:124-154` 已定义目标策略需要的大多数关联字段：session、turn、work-unit、actor、tool-use、tool name/input、deterministic outcome、result/envelope refs。稳定 ID 在 `:465` 由 session 和 tool-use ID 生成；缺 ID 和重复 ID 在 `:189-220`、`:271-323` 显式成为 orphan/conflict，而不是猜最近工具。

现有三档 disclosure 在 `tui/observation_store.go:107-122`：

| 当前档 | 当前 fullscreen 行为 | 与目标 D0-D3 的关系 |
| --- | --- | --- |
| `DisclosureSummary` | call 行 + `outcome · N bytes · detail available`（`tui/root.go:1187`） | 相当于 D1，但内容不是领域摘要 |
| `DisclosureDetail` | 完整 JSON input + 全部文本 result（`:1174-1210`） | 接近 D2，但经常过量：缺少“结构化判断摘要”中间层 |
| `DisclosureEvidence` | Detail + 完整 structured envelope + evidence identity（`:1211-1232`） | D3，可复用 |

唯一默认政策 `defaultResultDisclosure` 位于 `tui/observation_store.go:522-532`：所有 success/normal 默认 Summary；failed/partial/denied/cancelled/timed-out/escaped/shutdown/orphan/conflict 默认 Detail。它没有读取：

- tool family / command semantics；
- risk、side effect、needs review；
- warning、retry、stall、truncated；
- output volume/shape；
- user intent（quiet/inspect/full/audit）；
- aggregation eligibility；
- Agent terminal/result/artifact/verification semantics。

因此基础正确，但 policy 仍是 outcome-only。

### 2.3 Activity 与并发工作视图

`tui/activity_store.go:18-128` 已有 scope/kind/phase/state/actionability/action/progress/control/event/activity/counts。状态包括 running、needs-input、completed、failed、cancelled、ready-for-review；种类包括 tool、agent、background、MCP、decision、hook。

关键已有能力：

- `ActivityStore.Apply` 在 `tui/activity_store.go:140-183` 按 ID 原地合并，拒绝旧 sequence，terminal 不回退。
- outcome/state 一致性在 `:348-367`；稀疏更新保留旧字段在 `:192-250`。
- work-unit/actor 分组与 actionability 排序在 `:370-405`、`:442-454`，顺序是 needs input -> ready review -> failed/cancelled -> running -> completed。
- cancel/jump/details 动作在 `:419-439`。
- 状态条在 `tui/root.go:1638-1678` 显式报 needs input、failed、partial、denied、timed out、cancelled、ready review。
- activity overlay 在 `tui/root.go:1701-1778` 展示 work/actor/id/kind/state/outcome/progress/actions，并有窄屏分支。
- `/activity` 和 `/detail` 的命令入口在 `commands/builtins.go:39-95`；键盘焦点/详情跳转在 `tui/root.go:4096-4155`。

当前 gap：

- `ActivityEvent` 没有 started/updated/finished 时间、duration、cost、token、artifact、verification、parent-agent、objective、latest semantic event 等领域字段。
- tool kind/phase 仍由名称字符串启发式分类：`tui/state.go:938-964`；应由 formatter/presentation facts 补足，而不是继续扩充 substring 列表。
- overlay 是平面列表，按 work/actor 连续排列，但没有真正的 parent-agent tree、组 rollup 或 critical-path 分区。
- terminal activity 固定最多保留 64 条（`tui/activity_store.go:16`、`:264-277`）；完整历史仍在 observation/detail/session，不应把 ActivityStore 当审计源。
- 低风险重复成功没有 D0 member/aggregate group；每个 tool call 仍有 transcript anchor。

### 2.4 Decision

Decision 已是独立的强制完整 surface，而不是普通 tool result：

- `tui/decision.go:12-18` 保存完整 Prompt/Response/ResolvedAt，深拷贝输入与 choices。
- `tui/renderer.go:680-768` 在请求期间创建 `ActivityNeedsInput`，阻塞等待响应，完成后保存不可抵赖 receipt，并区分 approved/rejected/escaped/cancelled/timed-out/shutdown。
- `tui/root.go:2814` `renderDecisionDialog` 展示 actor/work/action/target/impact/risk/scope/rule/body/review details/post mode。
- screen-reader 有独立 input arbiter 和 Decision receipt，不与命令输入竞争（`ui/screen_reader_renderer.go:204`、`:550` 起）。

这条路径已经符合目标 D3 下限，PresentationPolicy 只需声明“Decision 不参与普通折叠/聚合”，不应复制另一套 dialog。

### 2.5 Screen reader、终端与 JSON 的不同现状

| surface | 当前实现 | 结论 |
| --- | --- | --- |
| fullscreen | `tui/root.go:1156` observation 三档；activity/decision 专面 | 基础最完整，作为共享 policy 的首个 consumer |
| screen reader | `ui/screen_reader_renderer.go:454-480` 每个 tool 开始时朗读完整 identity+input，结束朗读完整 evidence；Thinking 静默，spinner no-op（`:428-438`） | 因 append-only 保证顺序和可访问性，但当前比视觉默认更啰嗦，尚未共用 D1/D2 policy；应输出去重后的语义状态跃迁，并保留“details available”路径 |
| classic term | `ui/term_renderer.go:125-149` 名称专属 input preview + 原样完整结果 | legacy fallback，不具 observation/detail store；最小版本可保持兼容，完整策略需决定是否给 classic mode 建 retained transcript |
| NDJSON | `ui/json_renderer.go:124-160` 完整 identity、input、outcome、content blocks/data/metadata/usage | 机器接口应继续 lossless，不应用视觉 D0/D1；可额外发 presentation hints，但不得替代 raw event |

### 2.6 会话持久化、搜索与导出

- `repl_tui.go:1714` `persistTUISessionLifecycle` 保存 projection、interaction、usage、Decision history、disclosure return 和 evidence metadata。
- `repl_tui.go:1815` `restoreTUISessionEvidence`、`:1910` `sessionDetailRefs` 恢复并校验证据引用。
- session sidecar 的 evidence/disclosure meta 定义于 `session/session.go:89`、`:96`、`:103`、`:140`。
- `tui/state.go:536-657` 管理 transcript search view、focus/scroll return；`commands/builtins.go:98-152` 暴露 `/search`、`/export`、`/editor`。
- `tui/transcript_io.go` 以 observation/detail 为源做搜索、human export、raw audit export 和 editor 临时投影。

新增 D0/aggregate 时必须扩展 session projection 保存 group identity/member refs 或保证可从 observations 确定性重建；不能只把聚合后的可见字符串写入 session。

## 3. 命令展示实现映射

### 3.1 工具命令

工具注册集中于 `registry_setup.go:623-822`，覆盖 Bash/PowerShell、Read/Write/Edit、Glob/Grep、Agent、Task/Todo/Goal/Plan/AskUser/Brief、Web、Cron、Worktree、MCP/LSP、Team/Message、Skill、Notebook 和 ToolSearch。当前所有工具默认共享同一 observation policy；只有少数 formatter 是专用的：

| 命令族 | 当前展示实现 | 可复用 | 主要缺口 |
| --- | --- | --- | --- |
| Read/Write/Edit/Glob/Grep/Bash/Web/MCP/LSP/Task 等 | call preview 由 `tui/toolInputPreview`；success 统一由 `observationSummary`/bytes；失败由 `classifyToolErrorLines`（`tui/root.go:1435`） | stable input/result/evidence/identity | 没有领域 success formatter、diffstat、test counts、match/file counts、status/latency、truncation/partial-effects/next-action |
| Agent | call preview + `agentToolResultLines`（`tui/root.go:1465-1600`） | completed/backgrounded/teammate 基础语义，140-rune result preview，tools/tokens/duration | error/aborted 仍主要走通用 error；缺 artifacts/verification/cost/parent/objective/latest activity/transcript action；没有 group/tree rollup |
| SendUserMessage/Brief | `ui/send_user_message.go:113-186` 隐藏 generic tool chrome，TUI 用专用 assistant message | 符合“用户消息完整显示” | 保持独立，不纳入聚合 |
| Decision/AskUser/Plan gate | 专用 overlay/receipt | 已满足高优先级下限 | policy 只做强制 D3/禁止聚合 |
| Hook/Compaction | hook summary 和 compaction evidence/activity 有专门结构化路径（`tui/renderer.go:521-628`） | stable activity + lossless evidence | 可作为新 formatter/policy 的模式参考 |

建议首批 formatter：`Bash/PowerShell`、`Read`、`Write/Edit/NotebookEdit`、`Glob/Grep/LSP`、`WebSearch/WebFetch`、`MCP/dynamic mcp__*`、`Agent`、`Task*`、`Goal/Plan`、`Worktree/Git`。其余继续走 deterministic generic fallback。

### 3.2 Slash commands

slash command 接口只有 `Name/Description/Execute`（`commands/commands.go:43-49`）；输出经 `commands.Context.OnEvent func(string)`（`:90-146`）变成普通 `Info` 文本。`RegisterBuiltins` 在 `commands/builtins.go:14-37` 注册 help/clear/goal/search/export/editor/mouse/activity/detail/compact/model/session/config/status/context/init/resume/review/doctor/mcp。

因此 slash 命令目前没有统一 typed presentation model。最小版本不必把所有 slash command 转为 observation；但应该制定输出合同：

- inspector 类 `/status /context /cost /mcp /permissions /doctor /session /resume` 保留自己的多行结构，不接受通用 1-line 截断；窄屏通过其专用 view/reflow 处理。
- action 类 `/clear /compact /model /config /rename /mouse /detail /activity` 返回一条明确 receipt，失败走 Error/Decision。
- escape 类 `/search /export /editor` 必须返回稳定结果数、目标 path 或错误，且不重复 raw evidence。
- `/help` 由 registry 动态生成，属于完整命令目录，不进入 aggregation。

长期应把 `OnEvent string` 扩展为可选 `CommandPresentation{Kind, Outcome, Summary, Sections, EvidenceRefs}`，仍保留 string fallback。否则命令 policy 只能靠命令自己拼文本，无法共享 narrow/screen-reader 规则。

## 4. Subagent / background 的当前实现

### 4.1 前台 Agent 结果合同

- `tools/agent_output_union.go:37-137` 定义 common base 与 `AgentCompleted/AgentError/AgentAborted/AgentPartial`。
- `tools/agent_contract.go:14-90` 定义严格 output union schema；completed 包含 agent/type/prompt/content/tool count/usage/cwd/mode/isolation/model/worktree/latest tool，partial 包含 task/output/session URL。
- `tools/agent_contract.go:139-207` 把 typed union 映射为完整 `ToolResultBlock`；foreground completed 保留 content、agent ID、usage 和 worktree。
- `tools/agent.go:399-417` `agentRunSummary` 聚合 output/tool count/tokens/duration/usage/cwd/mode/isolation/worktree/transcript/latest tool；`formatCompletedAgentResult` 在 `tools/agent.go:5407` 生成 wire JSON。
- TUI `agentToolResultLines` 在 `tui/root.go:1491-1563` 只投影 completed/async/teammate/error-like 状态。

这是可靠 formatter 的数据基础。缺口主要在 presentation payload 没有显式 artifacts、verification summary、parent agent ID、objective headline 与 cost；TUI 自己再次定义了一个不完整的 `agentResultPayload`，存在 schema 漂移风险。应复用 `tools.AgentResult` 的公开 decoder 或在 `types/presentation` 放一份稳定、无执行依赖的 view model，避免在 root 中手写重复 JSON struct。

### 4.2 后台 Agent 与任务

```text
AgentTool / shell
  -> BackgroundTaskManager 持久 RuntimeTaskRecord
  -> completion RuntimeNotification
  -> observer: 当前可见 session/project 的 Info receipt
  -> follow-up: owning session 的模型回合
  -> polling adapter: ActivityAgent/ActivityBackground row
```

精确入口：

- `tools/background_tasks.go:149` `BackgroundTaskSnapshot`、`:298` snapshot。
- `tools/runtime_task_store.go:30-47` `RuntimeNotification` 是持久投递 envelope。
- `tools/background_tasks.go:1304-1321` Agent completion 填充 transcript/duration/tokens；`:1343-1378` 构造通知。
- `repl_tui.go:752-788` `installTUIBackgroundNotifications` 只对当前 session/project显示，并把 local-agent result/error 拼进 Info 文本。
- `repl_tui.go:790-863` `runTUIBackgroundFollowUp` 以 actor=`background`、work-unit=`taskID` 运行 owning-session follow-up。
- `repl_tui.go:910-1014` 每 250ms 比对 snapshot signature，映射为稳定 `background:<taskID>` activity；local agent completed -> `ReadyReview`。

当前主 transcript 不会流入每个子工具 tick，这是正确的；但后台 Info receipt 会直接拼完整 `snapshot.Result`，较大的 Agent result 仍可能淹没主线。Activity row 没有显示 objective/profile/duration/latest semantic activity/artifacts/verification；background snapshot 也没有把 Agent 的 structured result直接带给 row formatter。

建议：completion notification 仍是 delivery 事实；主 transcript 只产生 D2 terminal transition；完整 result/transcript 保存为 DetailRefs。work view row 使用 typed Agent view model，而不是从 `snapshot.Result` 拼接字符串。

## 5. 当前 dirty worktree 的影响

调查时 `git status --short` 有 30 个 tracked 修改、1 个删除和若干 untracked 文件。以下只列与展示策略直接相关的未提交增量；它们属于当前工作树证据，不应被当成 `HEAD` 基线或本任务产出。

| `[DIRTY]` 范围 | 未提交行为 | 对本方案的意义 |
| --- | --- | --- |
| `tools/agent.go`、`tools/agent_contract.go`、`tools/background_tasks.go`、`tools/runtime_task_store.go` | 给 Agent usage 增加 provider/model identity；后台通知携带完整 `Usage` | 可扩展为 Agent row 的 tokens/cost，但当前只用于费用归因，不等于已实现 cost 展示 |
| `loop/query.go`、`render.go`、`repl_tui.go`、新文件 `repl_usage_accounting.go` | 失败 provider attempt、fallback、nested tool、manual compaction 通过 `EventProviderUsage` 独立计费；避免把 child model usage按 parent model 重定价 | PresentationPolicy 应消费“已计算的 deterministic cost”，不要在 root 重算；当前增量可降低 Agent cost 字段实现成本 |
| `ui/cost_tracker.go`、`commands/builtins.go`、`commands/commands.go` | 记录 exact bucket breakdown、unknown cost、session breakdown，`/cost` 不再把 unknown 当免费 | `/cost` output contract 更可靠；与折叠政策正交 |
| `tui/root.go`、`tui/app.go`、`tui/renderer.go`、`tui/state.go`、`ui/term_renderer.go`、`ui/screen_reader_renderer.go`、`i18n/i18n.go` | 大量可见文案国际化与 language switching | 不改变 observation/activity/disclosure 架构；新增 formatter 必须接入同一 i18n，而不是硬编码英文 |
| `repl_tui_observation_test.go`、`repl_tui_test.go`、`tools/agent*_test.go`、`ui/cost_tracker_test.go` 等 | 锁定新 usage/accounting 行为 | 可复用作 cost/Agent 指标回归，但不是 disclosure policy 测试 |
| `tui/root_status_bar_test.go` | 正在追随 `formatSessionUsageSummary(..., costKnown, lang)` 新签名 | 主代理已验证当前 `go build ./...` 通过；本任务复跑时 `go test ./tui` 在 `root_status_bar_test.go:47/55/63` 的旧调用均少 `costKnown bool`，这是当前 dirty 工作树的测试基线问题 |

`HEAD` 已经包含 Agent formatter、background activity、ObservationStore/DetailStore/ActivityStore；不能把这些误记为本轮 dirty 改动。`git show HEAD:gosrc/tui/root.go` 已确认 `agentToolResultLines` 在基线存在，`git show HEAD:gosrc/repl_tui.go` 已确认 background bindings 在基线存在。

## 6. Requirement -> file/symbol/test 可追踪矩阵

| 目标要求 | 当前 file/symbol | 已有回归证据 | 判定 |
| --- | --- | --- | --- |
| 并行 call/result 稳定关联 | `loop.Event`；`ui.ToolEventContext`；`ObservationStore.ApplyToolCall/Result` | `tui/observation_state_test.go:10,48,88,115`；`repl_tui_observation_test.go:908` | 复用 |
| outcome 不从文字猜测 | `types.ToolResultBlock.Outcome` -> `observationOutcomeForResult` -> `ObservationOutcome` | `tui/detail_store_test.go:220,274`；`tui/observation_outcome_test.go:5` | 复用 |
| D1/D2/D3 可逆下钻 | `DisclosureState`；`RevealObservation/CycleObservationDisclosure`；root Ctrl+O/Alt+O | `tui/observation_state_test.go:141,224,250`；`repl_tui_observation_test.go:683` | 复用/扩展 D0 |
| 完整证据不因折叠丢失 | `DetailStore`、EnvelopeRefs、evidence journal | `tui/detail_store_test.go:29,50,82,121,152,195` | 复用 |
| 搜索/导出/editor | `tui/transcript_io.go`；slash command callbacks | `tui/transcript_search_export_test.go:19-376` | 复用 |
| 超长历史有界 | `boundedTranscriptMessages` | `tui/performance_acceptance_test.go:14-260` | 复用；为 aggregate 增测 |
| warning/error/denied/partial 不静默 | `defaultResultDisclosure`；`renderStructuredToolResultLines`；ActivityCounts | `tui/information_accessibility_test.go:109,130`；`tui/agent_observability_test.go:133` | 扩展 warning/retry/stall；当前 warning 不是 observation outcome |
| side effect / review 至少 D2 | 当前只有 local Agent completion -> ReadyReview；普通 write/edit success 仍 Summary | 无跨命令 policy 测试 | 缺失 |
| 命令族语义摘要 | Agent 与少数 compaction/hook 专用；其余 generic bytes | `tui/agent_observability_test.go:8-109` | 扩展 formatter registry |
| 合法重复成功 D0 + group D1 | 当前 Activity 原地更新单 operation；无 aggregate entity | 仅同名并行独立性 `tui/activity_store_test.go:65` | 缺失；先设计 aggregate identity |
| Actionability 优先并发视图 | `ActivityStore.activitySortRank`、root status/overlay | `tui/activity_store_test.go:123,164,185,299,314`；`tui/information_accessibility_test.go:153-233` | 复用/扩展 tree rollup |
| Agent terminal D2 + full transcript | Agent result union、DetailRefs、background notification、ready-review | `tools/agent_conformance_test.go:30-118,488`；`repl_tui_test.go:450`；`repl_tui_observation_test.go:38` | 扩展行合同和 transcript action |
| child permission 上浮 | structured `PromptRequest` actor/work-unit；Decision activity | `tui/information_accessibility_test.go:13,41`；`ui/screen_reader_renderer_test.go:87,119,479` | 基础可复用；需六 Agent mixed-state 集成测试 |
| screen-reader 语义等价 | append-only structured tool/decision/error | `ui/screen_reader_renderer_test.go:62-479`；`repl_screen_reader_lifecycle_test.go:26-749` | 重构为共享 policy，不降低信息下限 |
| machine output lossless | `JSONRenderer.RenderToolCall/Result` | `ui/json_renderer_test.go:112,166,195` | 保持 raw；可加 presentation hint |
| session resume 保留 disclosure/evidence | session projection/meta + restore functions | `tui/session_projection_test.go:23-146`；`repl_tui_observation_test.go:283-795` | 复用；aggregate meta 待定 |
| narrow terminal 不丢关键状态 | root activity narrow branch、status segment shedding、Decision layout | `tui/information_accessibility_test.go:13-41,172,312`；`tui/root_status_bar_test.go:122` | 扩展 formatter 的 field-priority 测试 |

## 7. 缺口分类

### 7.1 直接复用

1. `loop.Event`、`ToolEventContext` 与 tool-use ID 的完整因果身份。
2. `ObservationStore` 的并发安全、orphan/conflict 保留和 per-observation disclosure。
3. `DetailStore` 的无损 raw/envelope、私有文件存储、journal 与完整性校验。
4. `ActivityStore` 的 stable-ID reducer、actionability、sequence fence、terminal no-regression。
5. fullscreen 的局部 disclosure、全局 show-all、focus/scroll/draft return。
6. search/export/editor、bounded transcript、session evidence persistence。
7. Decision overlay/audit 和 screen-reader input arbiter。
8. Agent typed result union、transcript path、usage/duration/tool count 基础数据。
9. JSON renderer 的 lossless machine contract。

### 7.2 小范围扩展

1. `Observation` 增加 presentation facts 或引用：family、risk、side-effect、warning flags、metrics、artifact refs、policy decision/reasons。优先把这些放在独立 `Presentation` 字段，避免污染 execution facts。
2. `DisclosureLevel` 增加 D0/HiddenMember，或在 aggregate projection 层用 `Visible bool`；推荐后者，避免改变已有 enum/session 解码语义。
3. `ActivityEvent` 增加 timing/metrics/parent/objective/latest semantic activity/artifact/verification view fields。
4. Agent formatter补齐 error/aborted/partial、artifacts、verification、cost、transcript action。
5. slash command增加 optional typed presentation callback，保留 `OnEvent string` fallback。
6. screen-reader消费共享 decision，输出线性 D1/D2；D3 通过 detail/export command，而不是默认倾倒每个 raw result。

### 7.3 需要重构但可保持小边界

1. 把 `tui/state.go:1301` 通用 `observationSummary` 和 `tui/root.go:1491` Agent 私有 JSON parser 抽到纯函数 `presentation` 包或 `tui/presentation_*` 文件。
2. 把 outcome-only `defaultResultDisclosure` 改成 `PresentationPolicy.Decide(facts, userIntent, surface)`；ObservationStore 仍负责保留和关联，不负责命令语义。
3. 将 aggregation 建成只读投影：group 保存 member observation IDs/evidence refs，不能把 members 从 ObservationStore 删除。
4. background notification 不再把完整 Agent `snapshot.Result` 拼进 `Info`；改为 terminal summary + detail ref。
5. 把 tool kind/phase substring heuristics降为 fallback，让工具/formatter提供显式 family/phase。

### 7.4 未知，实施前必须验证

1. 各工具 `Data/Metadata` 是否稳定提供 formatter 所需计数；缺字段时是扩展 tool result schema，还是安全解析 stdout。原则上不要把 stdout parser 作为 deterministic truth。
2. redaction 的统一边界目前不在 presentation pipeline；D3 input/envelope 直接可见。引入 user full/audit 之前必须确认 secret/access policy。
3. Agent transcript 的访问控制、retention、跨 project/session resume 权限是否足以支持一键 D3。
4. classic TermRenderer 是否也必须具备 retained details，还是产品明确只保证 fullscreen/screen-reader/JSON。
5. aggregation group 是持久化还是恢复时确定性重建；涉及排序稳定性、late result 与 frozen turn 语义。
6. 当前 warning 没有统一 typed outcome；需要盘点哪些 warning 来自 `EventSystemWarning`、result metadata、hook 或工具 prose。
7. i18n dirty work尚未稳定，formatter 文案 key 的最终 API/语言 fallback 需要先收敛。

## 8. 最小实现边界与顺序

### 8.1 MVP 写范围

建议新增/修改的最小边界：

```text
新增：tui/presentation_policy.go
新增：tui/presentation_formatter.go
新增：tui/presentation_formatter_{shell,file,search,agent}.go
新增：tui/presentation_policy_test.go
新增：tui/presentation_formatter_test.go

小改：tui/observation_store.go   # 接受/保存 policy decision，不改变证据所有权
小改：tui/state.go               # 用 formatter 生成 Message/Activity presentation
小改：tui/root.go                # 渲染 typed summary/detail，不解析 Agent wire JSON
小改：ui/screen_reader_renderer.go / repl_screen_reader.go # 共享语义投影
小改：repl_tui.go                # background Agent terminal summary + detail ref
可选：commands/commands.go       # typed command presentation callback
```

不在 MVP 改动：`loop/query.go` 的执行调度、tools registry、DetailStore 文件格式、session repository 核心、go-tui layout engine、Agent scheduling/permission policy。

### 8.2 推荐顺序

1. 先用测试锁定当前 stable identity、evidence、disclosure return；这些已有充分覆盖。
2. 新增纯函数 policy decision table：failure/partial/denied/cancelled/timeout/orphan/conflict/warning/side-effect/review/decision/user-full 的不可降级下限。
3. 实现 generic formatter fallback 和 4 个首批领域 formatter：shell、file read/write/edit、search、Agent。
4. fullscreen 接入 typed summary；保持现有 Detail/Evidence 完全可达。
5. screen-reader 接入同一 semantic projection，验证 append-only 去重和字段顺序。
6. background Agent terminal notification 改为 summary + refs。
7. 最后加入 aggregate projection；先 Read/Glob/Grep 低风险成功，再 MCP rollup。warning/error 出现时拆出独立 D2。

### 8.3 估算边界（供 task_06 汇总）

| 工作项 | 规模判断 | 主要风险 |
| --- | --- | --- |
| policy + shared facts + tests | 中 | outcome/warning/risk 数据不齐，容易退回 prose inference |
| 10-12 formatter | 中到大 | 各 tool result schema 不一致；测试 fixture 数量大 |
| Agent row/terminal + background refs | 中 | tools/tui 包边界与 session transcript access |
| screen-reader parity | 中 | append-only 不能依赖视觉原地更新；必须防重复 |
| aggregation + persistence/rebuild | 大 | 并发、late output、freeze、member index、search/export |
| slash typed output | 小到中，可后置 | 命令众多但接口扩展可选兼容 |

最小可用版本可限定为 policy + 4 个 formatter + Agent terminal + fullscreen/screen-reader parity，不把 aggregation 和所有 slash command typed 化塞入第一批。

## 9. 测试矩阵

### 9.1 必须保留的已有套件

| 层 | 命令/文件 | 证明什么 |
| --- | --- | --- |
| observation/detail | `go test ./tui -run 'Test(Observation\|ToolCallAndResult\|MissingToolUse\|DuplicateToolUse\|Detail\|LongToolResult\|StructuredToolResult)'` | 并发关联、orphan/conflict、无损 evidence、默认 disclosure |
| activity | `go test ./tui -run 'TestActivityStore\|TestAppStateActivity\|TestApplyRuntimeError'` | 原地更新、排序、动作、terminal no-regression、错误归因 |
| fullscreen/accessibility | `go test ./tui -run 'Test(Decision\|Activity\|TranscriptLinear\|ExpandedActivity\|FullRootViewport)'` | narrow/decision/actionability/键盘路径 |
| Agent UI | `go test ./tui -run 'Test(Agent\|RenderAgent\|RenderToolErrors)'` | Agent call/terminal/background/teammate 摘要 |
| search/export/perf | `go test ./tui -run 'Test(Transcript\|HundredThousand\|FirstToken\|DetailDisclosure)'` | 证据可达、有界树和 p95 |
| screen reader | `go test ./ui -run TestScreenReader`；root package screen-reader lifecycle tests | append-only、完整 Decision、输入仲裁、background/compaction receipts |
| Agent contract | `go test ./tools -run 'Test(Conformance_OutputUnion\|AgentToolCompletedResult\|Task05BackgroundAgent)'` | typed result union、usage/transcript/notification |
| machine surface | `go test ./ui -run TestJSONRenderer` | raw identity/outcome/envelope 不被视觉 policy 破坏 |

### 9.2 新增 policy table tests

至少使用表驱动覆盖：

| facts | 预期 |
| --- | --- |
| low-risk read success | D1，可聚合候选 |
| write/edit success、Agent completed/ready review | 至少 D2，不可降到 D0 |
| warning/retry/stall/truncated | 至少 D2，保留 reason + automatic/user next action |
| failed/partial/denied/cancelled/timed out | D2；Decision needed 时 D3 |
| orphan/conflict/evidence integrity failure | D2 且不可聚合 |
| permission/auth/input/destructive pre-action | D3 overlay |
| user inspect | 提升一档但不超过 D3 |
| user full/audit/pinned | D3；redaction 仍优先 |
| quiet success | 只有满足全部 aggregation predicates 才成为 D0 member |

### 9.3 formatter contract tests

每个 formatter 对 queued/running/success/warning/error/user-expanded 做 golden 或结构化断言，重点不是整段像素字符串，而是必选字段和优先级：

- Bash：command/cwd/exit/duration/stdout+stderr refs；测试 pass/fail/skip。
- Read：path/range/lines/bytes；missing/permission/encoding。
- Write/Edit：files/+/-/no-op/partial effects/rollback。
- Glob/Grep：query/scope/matches/files/truncated/has-more。
- Web/MCP：query/server/tool/status/latency/auth/rate-limit。
- Agent：id/profile/objective/state/duration/tools/tokens/cost/result preview/artifacts/verification/transcript ref。

### 9.4 六 Agent 并发集成场景

固定 6 个稳定 ID：3 running、1 needs input、1 ready review、1 failed。断言：

1. group rollup 计数准确且稳定；needs-input/failed/ready-review 在 running 前。
2. 6 个 row 保持独立 actor/objective/work-unit，不因同名 profile 合并。
3. noisy running Agent 的 100 个 tool ticks 不新增 100 条主 transcript 行。
4. needs-input 产生完整 attributed Decision，并可从 group collapsed 状态访问。
5. failed Agent 显示 partial effects/last tool/next action/transcript ref。
6. ready-review 显示 artifacts/verification/result preview。
7. late completion 只更新对应 ID；已 terminal 的其他 row 不回退。
8. screen-reader 只朗读 6 个关键 transition，不朗读 spinner/token ticks。
9. export/raw audit 仍包含全部 member observations 与 evidence refs。

### 9.5 验证门槛与当前已知失败

最终集成顺序：

```text
go test ./tui ./ui ./tools ./commands ./loop
go test -race ./tui ./ui ./tools
go build ./...
go vet ./...
```

本任务取证时，`go test ./tui` 曾报告 `root_status_bar_test.go:47/55/63` 缺少新增的 `costKnown bool`。主代理最终集成复跑时，该并发工作树问题已被用户改动修复，`go test ./tui` 通过。这个时间线说明实施前必须冻结可重复基线；早期失败不再是当前 blocker，但仍是 dirty tree 集成风险的直接证据。

## 10. 最终判断

项目当前状态应定义为：**展示基础设施已成熟，语义政策和领域 formatter 未完成，subagent work view 只有生命周期骨架**。

最小风险路径不是再造 transcript 或替换 renderer，而是：

1. 保持 `ObservationStore`、`DetailStore`、`ActivityStore` 和 session evidence 所有权不变；
2. 在 projection 前新增纯函数 policy/formatter；
3. 先让 success 摘要有领域语义、失败有固定结构、Agent terminal 有完整判断信息；
4. 再用 member-indexed projection 加 D0 聚合；
5. fullscreen、screen-reader 和 JSON 分别采用同一语义决策、不同物理呈现，其中 JSON 始终 lossless。

这样既能复用现有高价值实现，也把共享改动限制在 `tui/state.go`、`tui/root.go` 和两个 adapter 周围；无须新增依赖，且可以按 formatter/Agent/aggregation 三条相对独立的实现线并行推进。
