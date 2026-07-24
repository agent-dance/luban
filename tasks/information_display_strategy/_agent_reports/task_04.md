# Task 04 - Subagent 与并行活动展示合同

> 调研快照：2026-07-15（Asia/Shanghai）
> 范围：parent agent、普通 subagent、team teammate、background task、parallel tool activity 的分组、摘要、展开、导航、归因、权限与失败升级
> 结论类型：Claude Code / Codex 一手事实 + 本仓库现状审计 + 本报告的规范性建议。凡属推导而非产品明文行为，均以“建议”或“推断”标出。

## 1. 执行摘要

推荐采用四层渐进披露，而不是在“全隐藏”和“全量 transcript”之间二选一：

1. **L0 状态带**：只回答“现在是否仍在工作、是否有人等我、是否失败”。
2. **L1 活动组与 agent 行**：回答“谁在做什么、关键路径在哪、最近发生了什么”。
3. **L2 agent 详情**：回答“用了哪些工具、耗时/用量、改了什么、为什么停住”。
4. **L3 transcript / evidence**：保留完整子会话和原始工具结果，按需打开，不默认流入主 transcript。

三条不可妥协的规则：

- **错误和权限请求可以穿透折叠状态**。父组折叠、用户正在看另一个 agent、甚至主线程正在输出时，都不能吞掉需要用户处理的事件。
- **工作单元状态与单次工具调用状态必须分离**。一个测试命令失败后 agent 可能正在修复；不能因为最新工具失败就把整个 agent 判死，也不能因为 agent 仍在运行就隐藏失败证据。
- **展示摘要可丢帧，事实和证据不能丢**。高频进度允许合并；终态、权限、失败、取消、结果和完整 transcript 必须可追溯。

本仓库已有稳定身份、Observation/Detail 证据、Activity reducer、结构化权限对话框和 Agent 结果用量等扎实基础，但还没有完整兑现上述合同。最关键的 P0 缺口是：

- `AgentProgressEvent` 已有 `latestTool / elapsedMs / tokensUsed / partialText`，但 TUI 后台活动只轮询 `status/result/error`，agent 行拿不到实时事实；
- `BackgroundTaskSnapshot` 丢掉了持久层已有的起止时间、用量、transcript 和通知元数据；
- Activity 状态缺少 `spawning / queued / waiting / blocked`，也没有通用 `parent_id / batch_id / run_id / depth / dependency`；
- `ActivityStore` 把终态锁死，同一 agent ID 恢复执行后发出的 `running` 更新会被忽略，无法正确展示 resume；
- 默认 Observation 摘要可能把结构化 Agent 完成结果压成“succeeded + N bytes”，用户无法直接看见结果预览、工具数、token 和耗时；
- 当前只有扁平 activity 列表，没有 delegation batch 折叠、关键路径、attention shelf 或嵌套树。

达到本文完整验收标准，预计 **20-31 个工程人日**，单名熟悉仓库的高级工程师约 **4-6 周**；只做可演示 MVP 约 **8-12 人日**，但会缺少可靠 resume、关键路径、完整证据/可访问性和竞态验证，不应冒充完成态。

## 2. 先把几个概念拆开

如果把 task、agent、tool call、transcript 都叫“活动”，最后只能得到一个会撒谎的 spinner。建议使用以下边界：

| 概念 | 唯一身份 | 生命周期 | 默认展示位置 | 说明 |
|---|---|---|---|---|
| Delegation batch | `batch_id` | open -> settled | L0/L1 组头 | 一次并行委派或一个 team 阶段；是折叠单位 |
| Work unit | `work_unit_id` | pending -> active -> terminal | L1/L2 | 可分配任务，可能被不同 agent 接手 |
| Agent identity | `agent_id` + `agent_path` | created -> closed | L1/L2 | 可跨多次 run/resume；昵称不是身份 |
| Agent run | `run_id` + `attempt` | spawning -> ... -> terminal | L1/L2 | 每次初始执行或 resume 都是新 run |
| Tool observation | `tool_use_id` | started -> terminal | 子 transcript / L2 | agent run 内部的一次工具调用 |
| Decision | `decision_id` | requested -> resolved | attention shelf / overlay | 权限或计划审批，关联 agent run 和 tool |
| Evidence | `DetailRef` / transcript ref | immutable | L3 | 原始输出、结构化 envelope、错误、决策审计 |

关键点：`ready_for_review` 和 `needs_input` 更适合作为 **attention**，不是与 `running/completed` 同一层的生命周期。一个 agent 可以已经 `completed`，同时仍有 `ready_for_review`；也可以生命周期为 `blocked`，attention 为 `needs_input`。

## 3. Claude Code 与 Codex 的一手事实

### 3.1 Claude Code

Claude Code 当前文档体现了以下展示取向：

- Subagent 的首要价值就是隔离高噪声搜索、日志和文件内容，只把摘要返回主会话；完整工作保留在独立 context/transcript 中。[Create custom subagents](https://code.claude.com/docs/en/sub-agents)
- 前台 subagent 阻塞主会话；后台 subagent 与主会话并发。当前文档说明后台 agent 的权限提示会在主会话弹出并点名请求者，`Esc` 只拒绝这一次工具调用而不终止 agent。[Run subagents in foreground or background](https://code.claude.com/docs/en/sub-agents#run-subagents-in-foreground-or-background)
- 完成的后台 agent 会留在 `/tasks` 中并排在运行任务之后；失败或用户停止的 agent 离开列表。说明“完成后仍可检查”与“持续占据主 transcript”是两回事。
- API 错误必须作为失败返回，而不能把错误文本伪装成研究结论；若已有部分输出，前台和后台路径都要保留部分结果。
- 嵌套 subagent 面板显示完整树，行上有 `(+N)` 后代数；打开一行能看 sibling、direct children 和回到 `main` 的路径。当前文档声明深度上限为 5。
- Resume 使用同一 agent ID 开新 run；当前文档特别说明已完成/失败的 agent 恢复时应重新显示为 running，而不是保留旧终态。
- 子 transcript 独立持久化，不受主会话 compaction 影响；默认清理期为 30 天。这给“主界面收拢、证据仍完整”提供了直接先例。
- Fork/subagent panel 支持上下选择、`Enter` 打开 transcript/发送消息、`x` 停止或移除、`Esc` 回到输入框。
- `Ctrl+O` 打开 transcript viewer；详细工具使用与执行放在这里。MCP 重复调用默认可折成类似 “Called slack 3 times” 的一行。[Interactive mode](https://code.claude.com/docs/en/interactive-mode)
- Task checklist 与后台 shell/subagent 列表明确分离：`Ctrl+T` 是计划清单，`/tasks` 才是正在运行的 shell 与 agent。产品没有把“任务状态”与“执行实例状态”混为一谈。
- Agent team 的 in-process 面板在所有 agent 都 idle 后隐藏 idle 行；超过 3 个 idle 时，多余行折成 `N idle agents`。Working、failed、当前正在查看的 teammate 永远保留独立行。[Agent teams](https://code.claude.com/docs/en/agent-teams)
- Hooks 暴露 `SubagentStart(agent_id, agent_type)` 和 `SubagentStop(agent_transcript_path, last_assistant_message)`；`TaskCreated/TaskCompleted/TeammateIdle` 则针对工作单元/teammate 协调。这再次证明 agent 与 task 是不同对象。[Hooks reference](https://code.claude.com/docs/en/hooks)

环境 caveat：本机存在两套 Claude Code CLI。当前 `PATH` 的 `claude` 解析到 `~/.xvm/data/node/npm-packages/bin/claude`，版本是 **2.1.159**；`/opt/homebrew/bin/claude` 是 **2.1.209**。因此本报告以当前官方文档和较新的 Homebrew 安装作为当前产品行为依据，不把默认 `PATH` 中旧二进制的缺失误判为 Claude Code 当前设计；复现实验必须同时记录解析路径与版本。

### 3.2 Codex

Codex 当前 Manual 与开源实现体现了相同的大方向，但更强调 thread 导航和有界预览：

- 官方 Manual 明确说主 agent 应保留需求、决策和最终输出，subagent 承担探索、测试、日志等噪声，只向主线程返回 summary；App、CLI、IDE 都允许打开 subagent thread 检查过程。[Codex Manual - Multi-agent operations](https://developers.openai.com/codex/codex-manual.md#multi-agent-operations)
- CLI 用 `/agent` 切换 agent thread；IDE 背景面板显示 active subagents，可展开、停止全部或打开单独 thread；Web 以只读 Active/Done 列表检查详情。
- 失焦 agent thread 的审批请求仍能浮到当前线程；overlay 标明来源 thread，并允许按 `o` 先打开来源再决定。非交互路径无法新批准时，动作失败并回传父工作流。
- `agents.max_threads` 默认 6，`agents.max_depth` 默认 1。展示层应显示“达到并发/深度限制”的事实，而不是让用户猜为什么工作一直 queued。
- Codex 协议的 `AgentStatus` 是 `pending_init / running / interrupted / completed(message) / errored(message) / shutdown / not_found`，状态由结构化 turn/error/shutdown 事件导出，不靠解析人类文本。见官方开源提交 `f90e7dee` 的 [`protocol.rs`](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/protocol/src/protocol.rs#L1703-L1723) 与 [`agent/status.rs`](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/core/src/agent/status.rs#L4-L27)。
- TUI 的 agent 展示对 prompt、error、response 分别做 160/160/240 grapheme 有界预览；spawn、send input、wait、resume、close 都生成简短语义行，wait 多 agent 时按 agent 汇总状态而非复制整段 thread。见 [`multi_agents.rs`](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/multi_agents.rs#L29-L31)。
- `/agent` running status feed 每个 agent 最多读取 6 个最近 item，最终只显示最近 3 行；command、file change、MCP、dynamic tool、web search 等先转成语义摘要。见 [`agent_status_feed.rs`](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/app/agent_status_feed.rs#L16-L19)。
- Agent picker 使用稳定 first-seen/spawn order，显示 nickname + role，并在打开 picker 时用 active turn 刷新 liveness；已关闭 thread 变暗但仍可检查。见 [`session_lifecycle.rs`](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/app/session_lifecycle.rs#L10-L62)。
- 失焦 thread 的 pending approvals 在 composer 上方最多列 3 个，更多用省略号，并给 `/agent to switch threads`；审批 overlay 显示 `Thread:` 和“open thread”动作。见 [`pending_thread_approvals.rs`](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/bottom_pane/pending_thread_approvals.rs#L40-L69) 与 [`approval_overlay.rs`](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/bottom_pane/approval_overlay.rs#L622-L644)。

### 3.3 共同原则与差异

| 维度 | Claude Code | Codex | 对本项目的启示 |
|---|---|---|---|
| 主 transcript | 子过程隔离，只回摘要 | 子 thread 隔离，只回摘要 | 禁止默认流式复制完整 child transcript |
| 详细检查 | subagent/fork transcript panel、`/tasks` | agent thread、`/agent`、Active/Done | 摘要必须带可达的 inspect/jump target |
| 高并发收拢 | idle >3 聚合，working/failed/viewed 保留 | 每 agent 预览最多 3 行、最多 6 个 item | 以状态和注意力决定折叠，不按原始字节数硬切 |
| 权限 | prompt 浮到主会话并点名 agent | inactive thread prompt 浮出，可打开来源 | 权限是全局 attention，不受父组折叠影响 |
| 嵌套 | 树、路径、后代数、固定深度限制 | thread/path、默认浅层 | 行合同必须有 parent/path/depth，不能后补猜树 |
| 失败 | API error 与 findings 分离，保留 partial | `Errored(message)`，有界 error preview | `failure` 与 `result_preview` 分字段 |
| 用量 | 强调并行 agent 会显著增 token | 明示每个 subagent 独立消耗 token | group 与行都要支持 known/unknown usage |

没有任何官方资料给出一套跨所有命令的“万能折叠公式”。本文后续策略是从上述事实归纳出的规范性设计，不冒充 Claude/Codex 的内部算法。

## 4. 展示决策模型

### 4.1 先看注意力，再看体积

展示级别由以下优先级决定：

```text
必须用户动作 / 安全风险 / 失败
  > 关键路径上的运行或等待
  > 当前聚焦对象
  > 新完成且未审阅
  > 普通运行
  > 已完成且已确认
  > 重复/高频进度
```

“输出很长”只能决定 L2/L3 如何截断，不能让 failure 退回折叠；“输出很短”也不能让每个心跳都进入主 transcript。

### 4.2 四层披露合同

| 层 | 默认容量 | 内容 | 禁止内容 |
|---|---:|---|---|
| L0 状态带 | 1 行 | group 总数、running、waiting、needs input、failed、关键路径 | agent 原始输出、完整 ID、路径、prompt |
| L1 活动组/agent 行 | 每组 1 头 + 有界行 | actor、短目标、状态、最新语义事件、耗时、attention、结果预览 | 连续 token、完整命令输出、完整 child transcript |
| L2 详情 | 一个聚焦 agent | 完整目标、阶段、工具统计、usage、变更、依赖、错误、控制动作 | 未脱敏 secret、无限输出 |
| L3 证据 | 用户主动打开 | child transcript、原始 tool result、structured envelope、decision audit | 无来源的推断性摘要 |

### 4.3 默认折叠规则

- `spawning/queued/running/waiting`：L1 保留独立行；同 agent 的高频事件原位更新，不追加 transcript 行。
- `blocked/needs_input/failed`：强制出现在 attention shelf，父组折叠也可见。
- `cancelled`：在当前 turn/batch 中保留一行；用户确认或下一稳定边界后收进组计数，证据不删。
- `completed + ready_for_review`：保留结果预览；确认后收拢为组计数。
- `completed + acknowledged`：默认只计数；当前聚焦、关键结果或用户 pinned 时仍独立显示。
- 重复工具事件：同 `tool_use_id` 原位更新；同一 MCP/server 的连续同类成功可聚合为 `Called X N times`，失败单独拆出。

## 5. 规范性字段合同

### 5.1 推荐记录形状

以下是展示域合同，不要求照抄为一个 Go struct，但字段语义必须唯一：

```json
{
  "schema_version": 1,
  "activity_id": "activity:...",
  "run_id": "run:...",
  "attempt": 2,
  "batch_id": "delegation:turn-7:1",
  "parent_activity_id": "activity:parent",
  "session_id": "...",
  "epoch": 4,
  "turn_id": "...",
  "work_unit_id": "task-04",
  "actor": {
    "agent_id": "agent-...",
    "agent_path": "main/reviewer/verifier",
    "nickname": "Delta",
    "role": "verifier",
    "kind": "subagent",
    "depth": 2,
    "descendant_count": 0
  },
  "objective": {
    "label": "Verify auth fixes",
    "full_ref": "detail:objective"
  },
  "lifecycle": {
    "state": "waiting",
    "phase": "verification",
    "outcome": "unknown",
    "reason_code": "dependency",
    "reason_text": "Waiting for tester"
  },
  "attention": {
    "kind": "none",
    "severity": "info",
    "unread": false,
    "decision_id": null
  },
  "progress": {
    "latest_event": "Tests 37/120",
    "latest_tool": "Bash",
    "current": 37,
    "total": 120,
    "tool_use_count": 8,
    "started_at": "...",
    "updated_at": "...",
    "finished_at": null,
    "elapsed_ms": 18100
  },
  "usage": {
    "known": true,
    "input_tokens": 12000,
    "output_tokens": 3400,
    "cache_read_tokens": 9000,
    "total_tokens": 15400,
    "cost": {"known": false, "currency": "USD", "value": null}
  },
  "dependency": {
    "blocked_by": ["run:tester"],
    "unblocks": [],
    "on_critical_path": true
  },
  "result": {
    "preview": null,
    "change_summary": null,
    "partial_available": false,
    "transcript_ref": "transcript:...",
    "evidence_refs": []
  },
  "control": {
    "can_open": true,
    "can_steer": true,
    "can_cancel": true,
    "can_resume": false,
    "can_retry": false
  },
  "ordering": {
    "created_sequence": 17,
    "event_sequence": 42,
    "source_sequence": 109,
    "observed_at": "...",
    "dropped_progress_events": 0
  }
}
```

### 5.2 身份与范围字段

| 字段 | 必需 | L1 展示 | 语义与约束 |
|---|---|---|---|
| `activity_id` | 所有记录 | 不默认显示 | 展示实体 ID；不可用名称或数组位置替代 |
| `run_id` | agent/background run | 仅详情 | 每次初始执行/resume 唯一；终态单调性以此为边界 |
| `attempt` | agent run | 仅 resume/详情 | 从 1 递增；同 agent ID 恢复不能覆盖旧 run 证据 |
| `batch_id` | 并行委派 | 组头 | 折叠和统计单位；同 turn 可有多个 batch |
| `parent_activity_id` | 嵌套 agent | 树视图 | 直接父级；缺失时标 `unknown`，不能靠时间邻接猜 |
| `session_id/epoch` | 所有记录 | 不显示 | 防止旧 session/旧 epoch 污染当前 UI |
| `turn_id` | turn 内活动 | 详情 | 归属主 turn，不等于 run |
| `work_unit_id` | 可分配工作 | 组/详情 | 与 agent ID 分开；任务可换人，agent 可做多任务 |
| `agent_id` | agent | fallback | 权威身份；昵称重名或复用时仍不串线 |
| `agent_path` | nested agent | L1 首选 | 例如 `main/reviewer/verifier`；支持返回 main 和树导航 |
| `nickname` | 可选 | L1 首选 | 只用于展示，不用于路由 |
| `role` | agent | L1 | 如 explorer/tester，不等于当前 objective |
| `kind` | 所有 actor | 详情/必要时 L1 | `parent/subagent/teammate/background/tool/runtime` |
| `depth/descendant_count` | nested agent | `(+N)` | 接近深度限制时显示 `depth 2/3`；无后代不占空间 |

### 5.3 生命周期、注意力与原因字段

| 字段 | 必需 | 规则 |
|---|---|---|
| `state` | 是 | 只接受结构化枚举：`spawning/queued/running/waiting/blocked/failed/cancelled/completed` |
| `phase` | running/waiting | `initializing/researching/executing/verifying/finalizing`；未知即 `working`，不解析任意文本 |
| `outcome` | terminal | `succeeded/failed/partial/denied/cancelled/timed_out/shutdown/...`；与 state 分开 |
| `reason_code` | queued/waiting/blocked/failed/cancelled | 稳定机器码，如 `capacity/dependency/retry_backoff/permission/api_error/user_cancel/depth_limit` |
| `reason_text` | 有原因时 | 一行安全摘要；原始错误走 evidence ref |
| `attention.kind` | 是 | `none/needs_input/ready_for_review/warning/critical` |
| `attention.severity` | 非 none | `info/warning/error/critical`，不能只靠颜色表达 |
| `attention.unread` | 是 | 决定是否计入 L0；打开详情不等于确认，需显式 acknowledge 语义 |
| `decision_id` | needs_input | 连接 permission/plan decision，便于从 shelf 跳到 overlay 或来源 thread |

### 5.4 进度、时间、工具和用量字段

| 字段 | 默认行 | 详情 | 约束 |
|---|---|---|---|
| `latest_event` | 是，最多 1 行 | 历史有界列表 | 必须是语义事件，如 `Running go test`，不能塞 raw partial stream |
| `latest_tool` | 有值时 | 是 | 工具名/类别；敏感输入不放 L1 |
| `current/total/unit` | total 已知时 | 是 | 例如 `37/120 tests`；未知时不伪造百分比 |
| `tool_use_count` | completed 或长任务 | 是 | 计已开始还是已完成必须固定，建议计 started 并另记 failed count |
| `started_at/updated_at/finished_at` | 计算 elapsed | 是 | 源时间与 observed time 分开；跨进程用 UTC |
| `elapsed_ms` | running >2s、terminal | 是 | 由可信时间计算；不要每 250ms 重绘所有行 |
| `usage.known` | 是 | 是 | 未知必须显示 `usage unavailable`，不能把 0 当未知 |
| token 明细 | 可选 total | 是 | input/output/cache/total 分开；组头只聚合 known 值并显示覆盖率 |
| `cost.known/currency/value` | 通常隐藏 | 是 | provider/model/pricing 不完整时禁止估成 0 |
| `dropped_progress_events` | 非 0 时 warning | 是 | 高频 channel 丢帧可接受，但必须可诊断且不影响终态 |

### 5.5 结果、证据与控制字段

| 字段 | 状态 | 默认行为 |
|---|---|---|
| `result.preview` | completed/partial/failed with partial | 1 行、建议 <=240 graphemes；不得把 API error 当结论 |
| `change_summary` | 有写入时 | `3 files changed (+42/-8)` 或领域摘要；不是 raw diff |
| `partial_available` | failed/cancelled/partial | 明示 `partial result saved`，并给 Details |
| `transcript_ref` | agent | L2 action；不在 L1 泄露本地绝对路径 |
| `evidence_refs` | terminal/error/decision | L3；不可变、带 digest/size/source |
| `can_open` | 可检查 | `Enter` 默认动作 |
| `can_steer` | running/waiting/blocked | 只在运行时支持；打开 child 后普通文本路由到 child |
| `can_cancel` | 非 terminal | 取消必须指向 run，不误杀同 agent 的新 attempt |
| `can_resume/retry` | terminal/blocked | resume 保留 agent context；retry 是新 agent/run，UI 文案必须区分 |

## 6. 各生命周期应该展示什么

| 状态 | L1 必显 | L2 补充 | 折叠/升级 |
|---|---|---|---|
| `spawning` | 请求角色、短目标、`Spawning`、elapsed | launch request、model/reasoning、isolation | <300ms 可不闪现；失败必须留下 durable row |
| `queued` | actor/role、目标、`Queued`、原因 | capacity limit、队列位置（仅已知时）、queued since | 关键路径 queued 保留；普通 queued 可在组内聚合 |
| `running` | 谁、目标、phase、latest event、elapsed | latest tool、工具统计、usage、最近 3 个语义事件 | 独立行；高频事件原位合并 |
| `waiting` | 等什么、等待对象/条件、elapsed | `blocked_by`、retry time、是否可 steer/cancel | 预期等待不升错误；关键路径保留 |
| `blocked` | 原因、需要谁做什么、等待时长 | 原始错误/decision、恢复动作 | attention shelf；父组折叠也可见 |
| `failed` | 错误分类、短错误、partial 是否可用、elapsed | 原始错误、最后成功事件、transcript、resume/retry | 立即升级；直到 acknowledge 不收拢 |
| `cancelled` | 取消者、原因、partial、elapsed | 取消时最后工具、清理结果 | 当前 batch 保留；确认后计数 |
| `completed` | 结果预览、change summary、elapsed | tools、tokens/cost、verification、transcript | ready review 时保留；确认后折叠 |

### 6.1 Waiting 与 Blocked 的判定

- **Waiting**：系统知道自动恢复条件，例如依赖 run 完成、backoff 到期、等待并发槽位。
- **Blocked**：需要外部动作、权限决定、缺失输入，或系统不知道如何自动恢复。
- 不能把“30 秒没输出”直接判 blocked；它只能触发 `stalled` warning。Agent 可能正在长时间编译。

### 6.2 Resume 的正确语义

Agent identity 不变，但 run identity 改变：

```text
agent A
  run A/1  completed  result retained
  run A/2  running    current row
```

L1 可以仍只显示一行 `A · running · attempt 2`；L2 必须能看旧 run。任何“终态永不回退”规则都应作用于 `run_id`，不能锁死 `agent_id`。

## 7. 并行分组与关键路径

### 7.1 分组键

推荐层级：

```text
session
  delegation batch / team phase
    work unit
      agent run
        tool observations
```

禁止按以下方式分组：

- 按 tool name：六个 agent 都调用 `Read` 不代表同一工作；
- 按到达邻接：并发事件天然交错，late output 会把错误串到别人；
- 按 nickname：昵称可重复、复用或为空；
- 只按 `turn_id`：一个 turn 可先研究 batch，再执行 batch。

### 7.2 组头合同

L0/L1 组头至少包含：

```text
<batch label> · 6 agents · 2 running · 1 waiting · 1 needs input · 1 failed · 1 done
critical: Boreal -> Curie · usage: 48.2k tokens (5/6 known)
```

字段：`batch_id, label, total, counts_by_state, unread_attention_counts, critical_path_summary, known_usage_sum, usage_coverage, started_at, elapsed, collapsed, acknowledged`。

组计数必须由唯一 run/agent 状态派生，不能数事件；同一失败重复上报只增加 occurrence，不增加 failed agent 数。

### 7.3 关键路径规则

- 只从显式 `blocked_by/unblocks` 依赖边和 parent wait 行为计算，不从耗时或事件顺序猜。
- 没有依赖图时显示 `critical path unknown`，而不是把最慢 agent 冒充关键路径。
- 关键路径上的 `running/waiting/queued` 行在父组折叠时仍显示一行。
- 关键路径失败使 group attention 升为 critical；非关键失败仍为 error，但不能隐藏。
- 父 agent 正在等待 N 个 child 时，wait 本身是 parent 的 phase，不要新增一个伪 agent。

### 7.4 稳定排序

建议稳定 key：

```text
attention rank
  -> on_critical_path (true first)
  -> state rank
  -> batch created_sequence
  -> work unit created_sequence
  -> agent path / stable id
```

排序变化不能移动键盘 focus；focus 绑定 `activity_id/run_id`。新错误可播报并加入 attention shelf，但不应偷走用户正在编辑的输入或自动打开详情。

### 7.5 单一噪声 agent 的限流

- Source reducer 可接收全量事件；L1 非聚焦行最多每 500ms 更新一次，纯 elapsed 更新最多每 5s；聚焦行可每 1s。
- 终态、权限、失败、取消立即刷新，不受 debounce。
- 同一 `tool_use_id` 的 progress 原位覆盖；最近语义历史最多 6 项，默认渲染最后 3 行。
- `partialText` 只进入 child transcript/详情预览生成器，不逐 token 写主 transcript。
- 输出超过上限时保留 bytes/lines、截断原因和 evidence ref；不能只显示 `...`。

## 8. Agent/Activity 相关命令的展示合同

| 操作 | 主 transcript | 活动面板 | 必须保留的证据 |
|---|---|---|---|
| Spawn accepted | 一条 `Spawned <name> [role]`，prompt 仅安全短预览 | 新 row `spawning/queued/running` | request、agent/run IDs、完整 objective ref、model/isolation |
| Spawn failed | 一条红色失败语义行 | failed row 越级 | validation/limit/error、未创建 agent 的 request ID |
| Send/steer | 一条收拢 receipt；重复消息可计数 | 更新 latest event，不制造新 agent | sender/receiver、message ref、delivery outcome |
| Wait one/many | 进行中只改 parent phase；结束一条汇总 | 突出 critical/waiting rows | targets、deadline、返回 statuses |
| Stop/cancel | 一条 durable receipt | run -> cancelled，显示 initiator | request、cleanup outcome、partial refs |
| Resume | 一条 `Resumed A · attempt 2` | 同 agent 新 run -> running | old/new run IDs、保留 context、resume message ref |
| Retry as new agent | 明示 `Retried as B` | 新 agent row，关联 `retry_of` | 原 failure 与新 spawn 的双向关系 |
| Open details/transcript | 不增加 transcript 消息 | 切换 focus / child thread | 纯 UI 动作无需模型上下文 |
| Background | 一条 `Backgrounded A` | row 持续更新 | task ID、output/transcript ref、owner session |
| Child tool call | 默认不进主 transcript | agent row 只显示 latest meaningful event | child transcript 内完整 call/result |
| Permission request | 立即 overlay/shelf，点名来源 | child -> blocked + needs_input | decision request、response、来源 run/tool |
| Permission resolved | 一条短 receipt | child 按实际 run 状态恢复，不假定完成 | outcome、choice、resolver、resolved_at |

## 9. Parent、Subagent、Team 和 Parallel Tool 的差异

### 9.1 Parent agent

- 主 transcript 本身就是 parent 的详情，默认不再套一张 parent 卡。
- 当存在 child 时，agent picker 第一项显示 `Main [default]`；L0 可显示 parent 当前 phase。
- Parent 的 wait/aggregate 是协调状态，不计作第七个 agent。

### 9.2 普通 subagent

- 一行对应当前 run；完成后展示一行 distilled result。
- 默认不流式转播工具日志；`Enter` 打开独立 transcript。
- 没有 agent 间通信时，不展示伪 mailbox 或 team task 语义。

### 9.3 Team teammate

- Teammate 是长期 actor，可依次认领多个 work unit；因此 actor 行下应列 current task，详情里有已完成任务历史。
- Agent 间消息只显示语义 receipt 和未读/失败；不要把 mailbox 全量塞进主 transcript。
- Idle teammate 可在全组 idle 后延迟隐藏；working、failed、needs input、当前查看者永不被 idle 聚合吞掉。
- 超过 3 个 idle 时用 `N idle agents` 聚合是合理默认；展开后仍按稳定路径显示。

### 9.4 Parallel tool activity

- 多个直接 tool call 属于同一 parent/agent run 的 children，不冒充 subagent。
- L0 可显示 `3 tools running`；L1 在 agent 行下只展示 critical/failed/long-running tool，其余聚合。
- Tool terminal 不自动终结 agent。Agent 是否完成只由 agent run 终态决定。

## 10. 权限与失败升级

### 10.1 权限请求

父组折叠时仍显示：

```text
! Delta [writer] needs approval
  Write docs/auth.md · high risk · requested 12s ago   [Open] [Reject]
```

要求：

- 必显 agent nickname/role，且内部携带 `agent_id/run_id/execution_session_id`；
- 必显 action、target、impact、risk、rule source、approval scope；
- `Open` 进入来源 agent/thread，但决策仍绑定原 `decision_id`；
- overlay 关闭后恢复之前 focus、scroll 和 input draft；
- 多个请求串行决策，但 shelf 列出所有来源及数量；不得只显示第一个而让其他 agent 看似“运行中”；
- 无法交互的 background/noninteractive 路径 fail closed，并在父组显示 denied/failed 原因。

### 10.2 失败

失败行必须把三件事拆开：

1. **failure class**：API、permission、tool、validation、timeout、internal；
2. **last meaningful event**：失败前做到哪里；
3. **partial result**：是否有可检查输出。

示例：

```text
x Echo [security] failed · API rate limit
  last: Found 2 high-risk paths · partial report saved   [Details] [Resume]
```

只写 `Agent failed` 不够；直接展开 400 行 stack trace 也不叫透明。短摘要必须可行动，原始错误必须可检查。

### 10.3 Late output 与竞态

- 每个 event 带 `run_id + event_sequence`；旧 sequence 不回退当前投影。
- 终态后到达的旧 progress 只进 evidence/history，不改变终态。
- 终态后到达的错误如果属于同一 run，升级 outcome/attention 但保留原终态时间和冲突审计；不能悄悄丢弃。
- Resume 是新 run，因此允许 agent-level 当前状态从 completed 变 running。
- 同名/同类型并行工具只按 `tool_use_id` 关联；缺失或冲突变成 orphan/conflict 证据，禁止“找最近同名工具”猜配对。

## 11. 可访问性、确定性和证据保留

### 11.1 可访问性

- 状态必须有文本，不只靠颜色、spinner 或图标：`failed`、`needs approval`、`cancelled` 均要读得出来。
- Screen reader announcement 队列只播报语义变化：spawn、needs input、failed、completed、critical path changed；不播每次 token/elapsed。
- 窄屏（至少 40 columns）顺序固定为 `state -> progress -> objective`；字段换行，不把 actor 和错误截没。
- Unicode 图标须有 ASCII fallback；动画遵守 reduced-motion，并提供静态 `running` 文本。
- 键盘焦点按 ID 保持；`Enter` details、`g` jump、`c` cancel 等动作只在面板聚焦时劫持按键。
- 展开/收拢不丢 input draft、scroll anchor 或返回位置。

### 11.2 确定性

- 所有状态来自枚举/事件，不解析结果 prose。
- Agent nickname 仅展示，路由与关联始终使用稳定 ID。
- 组计数按唯一 run/agent 派生，事件重放不重复计数。
- 排序有完整 tie-breaker；map 遍历顺序不得泄漏到 UI。
- 已展示的 terminal receipt 不因晚到 progress 消失。
- 当前 epoch 之外的事件不能污染 UI，但应进入正确 session 的持久日志/通知。

### 11.3 证据保留与隐私

- 保存完整 child transcript、tool result text、structured envelope、permission decision、terminal summary 和 digest。
- L1 不显示本地绝对 transcript path；用 `Open transcript` 动作解析内部 ref。
- Prompt/command/tool input 的 L1 预览必须去控制字符、截断并做 secret-aware redaction；原始内容的访问遵循原 session 权限。
- 截断要记录原 bytes/lines、截断原因、digest 和 ref。
- Export 必须包含当前隐藏的 evidence 与 decision audit；“折叠”绝不等于删除。
- Retention policy 显式化；过期后 UI 显示 `evidence expired`，不能假装从未存在。

## 12. 六 agent 并发混合结果演练

### 12.1 场景定义

同一个 batch `auth-review` 在 t=0 并发启动 6 个 agent：

| Agent | Role | Objective | 显式依赖 |
|---|---|---|---|
| Atlas | explorer | 映射认证调用链 | 无 |
| Boreal | tester | 运行 120 个 auth 测试 | 无 |
| Curie | verifier | 基于测试结果验证修复 | `Boreal` |
| Delta | writer | 更新 auth 运维文档 | 无 |
| Echo | security | 审计 token 泄漏 | 无 |
| Flux | benchmark | 跑性能基线 | 无 |

### 12.2 t=18s：混合状态

- Atlas：completed，结果 `Mapped 12 auth call sites`；
- Boreal：running，`go test` 37/120；
- Curie：waiting for Boreal；
- Delta：blocked + needs_input，请求写 `docs/auth.md`；
- Echo：failed，API rate limit，但有 2 条 partial findings；
- Flux：cancelled by user，尚无结果。

父组折叠时仍应呈现：

```text
auth-review · 6 agents · 1 running · 1 waiting · 1 needs input · 1 failed · 1 cancelled · 1 done
critical: Boreal -> Curie · elapsed 18s · usage 31.4k tokens (4/6 known)

! Delta [writer] needs approval · Write docs/auth.md · high risk           [Open]
x Echo [security] failed · API rate limit · 2 partial findings saved      [Details] [Resume]
> Boreal [tester] running · go test · 37/120 tests · 18s                  [Open] [Cancel]
```

说明：

- Atlas 的普通成功被组计数收拢，没有占第四行，但结果仍可展开；
- Curie 是关键路径上的 waiting，宽屏可作为第四行显示，窄屏则由组头 `Boreal -> Curie` 保证不丢；
- Delta 与 Echo 穿透父组折叠；
- Flux 的取消留在 expanded view 和组计数，不挤走权限/错误；
- 任何 agent 的 raw transcript 都没有流入主 transcript。

### 12.3 展开组

```text
auth-review
  v Atlas [explorer] completed · 12 call sites mapped · 8.2s · 6 tools · 7.1k tok
  > Boreal [tester] running · go test · 37/120 · 18s · 8 tools · 9.8k tok
  = Curie [verifier] waiting · blocked by Boreal · 17s · critical path
  ! Delta [writer] blocked · permission required · Write docs/auth.md
  x Echo [security] failed/partial · API rate limit · 2 findings saved · 11.4s
  - Flux [benchmark] cancelled by user · no result · 4.1s
```

选择 Echo 后的 L2：

```text
Echo [security] · main/Echo · run 1 · failed (partial)
Objective: Audit auth token leakage
Last event: Found unsafe token logging in two handlers
Failure: provider rate_limit · retryable
Usage: 5.3k tokens · 5 tools · 11.4s
Result: 2 findings saved; review before retry
Actions: [Open transcript] [Open evidence] [Resume] [Acknowledge]
```

### 12.4 t=42s：依赖释放和终态

- Boreal completed，120/120 passed；
- Curie 自动从 waiting -> running -> completed，验证 3 个风险；
- Delta 的请求被拒绝，agent 无替代写入路径，保持 blocked/denied；
- Echo 仍 failed/partial；Flux 仍 cancelled。

最终折叠组：

```text
auth-review · 6 agents · 3 done · 1 blocked · 1 failed · 1 cancelled · 42s
! unresolved: Delta permission denied; Echo partial failure                  [Review 2]
```

用户执行 `Review 2` 并分别 acknowledge 后，才可收成一行普通历史：

```text
v auth-review · settled with issues · 3 done, 1 blocked, 1 failed, 1 cancelled · 61.8k tokens
```

这个演练满足四个关键问题：谁在做什么、什么发生了变化、哪里失败、如何检查；同时任何一个 noisy agent 都无法垄断主 transcript。

## 13. 本仓库现状审计

### 13.1 可直接复用的基础

| 能力 | 当前证据 | 评价 |
|---|---|---|
| 稳定 Activity 基础身份 | `tui/activity_store.go:18-128` 有 session/epoch/turn/work unit/actor/kind/state/outcome/sequence | 可复用；需补 run/batch/parent/dependency/time/usage |
| 并发安全与确定排序 | `tui/activity_store.go:140-183, 280-325, 370-405` | 已防旧 sequence、按 work/actor 排；排序策略需升级 |
| 不同同名并行活动不串线 | `tui/activity_store_test.go:65-88` | 正确基础，不要回退到 name-based correlation |
| Outcome 与 State 一致性 | `tui/activity_store.go:348-367` | 可保留，但 attention 应拆出 |
| Observation 三层披露 | `tui/observation_store.go:107-154` 的 summary/detail/evidence | 与本文 L1-L3 高度一致 |
| 原始/结构化证据 | `tui/observation_store.go:239-337, 449-497` | 已保留 tool result 与 envelope，可作为 L3 |
| 错误默认展开 | `tui/observation_store.go:522-532` | 符合失败升级原则 |
| 权限身份与审计 | `permissions/structured_prompt.go:23-47`，`tui/renderer.go:680-768` | 已有 decision/session/execution session/actor/work unit，且写 DecisionHistory |
| 权限 UI 点名来源 | `tui/root.go:2835-2865` | 已显示 actor、work、execution session、action/target/risk/scope |
| 结构化 Agent terminal result | `tools/agent_output_union.go:37-128` | 已有 transcript/duration/tokens/tools/model/result variants |
| Agent 实时 progress 事实 | `tools/agent_progress.go:32-60` | 已有 phase/latest tool/partial/elapsed/tokens，但未接进 TUI |
| Durable background records/notifications | `tools/runtime_task_store.go:30-84` | 持久层已有 timestamps、usage、transcript、owner 等多数事实 |
| Activity 摘要与详情动作 | `tui/root.go:1638-1797, 4085-4155` | 已有 status strip、activity view、cancel/jump/details 和窄屏路径 |
| 后台证据回接 | `repl_tui.go:910-1014` | 轮询 task 并写 Observation/Activity，方向正确 |

### 13.2 P0 断层

#### A. Progress 有生产者，没有展示消费者

`AgentProgressEvent` 包含 `MessageCount, LatestTool, PartialText, ElapsedMs, TokensUsed`，emitter 也保证 publication order 并对满 buffer 做 drop-oldest（`tools/agent_progress.go:48-65, 98-139`）。但仓库内除 Agent 代码/测试外没有消费 `ProgressChannel` 的生产展示路径；`bindTUIBackgroundActivities` 每 250ms 只看 snapshot 的 `Status/Result/Error`（`repl_tui.go:928-983`）。

**后果**：running agent 的“latest meaningful event、工具、用量、耗时”合同无法实现；只会长期显示静态 description。

**修复**：把 emitter 注册到按 `run_id` 索引的 runtime event hub；TUI adapter 消费结构化 progress 并 coalesce 成 ActivityEvent。Snapshot 只负责冷启动/恢复，不应承担实时流。

#### B. Snapshot 把展示需要的事实裁掉了

`RuntimeTaskRecord` 有 `StartedAt/FinishedAt/UpdatedAt/Notification`，notification 有 transcript、duration、tokens、provider/model/usage（`tools/runtime_task_store.go:30-84`）；但 `BackgroundTaskSnapshot` 只有 ID/type/status/description/command/prompt/output/error/result/owner/alias（`tools/background_tasks.go:149-164`）。

**后果**：刷新/恢复路径无法显示可信 elapsed、usage、transcript ref；UI 若自己从首次观察时间计时会在重启后撒谎。

**修复**：扩展 snapshot 或新增 presentation snapshot，保留 nullable timestamps/usage/transcript/run ID；未知值用指针/known flag，不用零值冒充。

#### C. Resume 会被终态锁死

`ActivityStore.Apply` 遇到 terminal existing row 时，若新 event state 与旧 state 不同就直接忽略（`tui/activity_store.go:154-170`）。后台 agent resume 复用同一 task/agent ID，并把 task status 重新置为 running（`tools/agent_sessions.go:462-489, 570-581`）；TUI activity ID 又固定为 `background:<taskID>`。

**后果**：已 completed/failed/cancelled 的 agent resume 后，Activity 面板仍显示旧终态。这个问题不是颜色不好看，而是用户会误判工作没有运行。

**修复**：引入 `run_id/attempt`。每个 run 终态单调，agent projection 选择最新 run；禁止简单放开 terminal -> running，否则晚到旧事件会造成真正回退。

#### D. 状态词汇不足

当前 `ActivityState` 只有 `running/needs_input/completed/failed/cancelled/ready_for_review`（`tui/activity_store.go:41-50`）。没有 spawning、queued、waiting、blocked；`needs_input/ready_for_review` 又与生命周期混在一起。

**后果**：队列、依赖等待、retry backoff、权限阻塞最终都只能伪装成 running 或 failed，L0 计数不可信。

**修复**：增加 lifecycle + attention 两轴，提供旧状态迁移 adapter 和 table-driven state/outcome compatibility tests。

#### E. Agent 默认结果摘要过度收拢

Legacy/直接 tool-result renderer 能把 Agent completed 显示为 ID、type、tools、tokens、duration 和 140-rune result preview（`tui/root.go:1491-1526`）。但 identity-aware Observation 在 summary level 只显示 `outcome + bytes + detail available`（`tui/root.go:1184-1193`），只有展开 evidence 后才会走结构化 Agent 结果渲染。

**后果**：越新的结构化路径反而比 legacy 路径信息更少，用户看不到“谁完成了什么”。

**修复**：Observation summary projector 按 tool result typed data 生成安全语义摘要；仍保留 evidence ref，不要在 renderer 重新解析 JSON 字符串。

### 13.3 P1/P2 缺口

| 优先级 | 缺口 | 影响 | 建议 |
|---|---|---|---|
| P1 | 无 `batch_id/parent_id/agent_path/depth` | 不能可靠分组或展示嵌套 | 在 spawn/runtime 层生成，不在 UI 猜 |
| P1 | 无 dependency edges / critical path | 父组折叠时不知道该保留谁 | Wait/Task dependency 写显式边 |
| P1 | Activity view 是扁平逐行列表 | 六并发时噪声迅速增长 | group header + collapsed rows + attention shelf |
| P1 | cancellation controller 只认 `background:` | 前台/远程/teammate 控制不统一 | control capability 由 runtime 提供 typed handle |
| P1 | progress drop 无 gap 计数 | 诊断不了展示为何跳跃 | 增加 dropped count/last source seq |
| P1 | parent/child permission 只能看到 actor 文本，无一键切来源 thread | 决策上下文仍需手工查 | 将 execution session 映射为 open target |
| P2 | terminal activity 仅保留 64 条 | 大 batch 的旧证据可从面板消失 | UI projection 可裁，durable index 不裁；显示 retained/archived |
| P2 | 后台轮询按 ID 排 snapshot | 冷启动顺序不等于用户心智顺序 | created sequence + attention sort |
| P2 | 组 usage 无 known coverage | 部分 agent 未上报时总计误导 | `sum + known/total` |

## 14. 实施方案与成本

### 14.1 分阶段计划

| 阶段 | 工作 | 估算 |
|---|---|---:|
| 0. 合同与回归测试 | 先锁定并行同名、late event、terminal、resume、权限跨 agent、6-agent golden scenario | 2-3 人日 |
| 1. 展示域模型 | lifecycle/attention、run/attempt、batch/parent/path/dependency、nullable time/usage、迁移 adapter | 3-5 人日 |
| 2. 实时事件接线 | Progress hub、notification/snapshot 冷启动、coalescing、gap 计数、terminal durable delivery | 3-4 人日 |
| 3. Reducer 与聚合 | run 单调状态机、resume projection、group counts、critical path、late/conflict 处理 | 3-5 人日 |
| 4. TUI 交互 | L0 状态带、group row、agent row、attention shelf、详情、nested path、40-column layout | 4-6 人日 |
| 5. 权限/失败提升 | source-thread jump、focus return、partial failure、resume/retry/acknowledge actions | 2-3 人日 |
| 6. 持久化/导出/验证 | restart restore、raw export、screen reader、race/perf、full suite/manual walkthrough | 3-5 人日 |
| **合计** | 完整验收范围 | **20-31 人日** |

估算假设：

- 复用现有 go-tui、Activity/Observation/Detail/Decision/RuntimeTask 基础；
- 不新增依赖；
- 只覆盖本仓库 CLI/TUI，不同时重做 Web/IDE/App Server 客户端；
- 不实现 tmux split-pane team UI；
- 现有脏工作树中的权限继承改动先稳定并合并，避免同文件高冲突开发。

不确定性约 ±30%，最大变量不是画行，而是统一 foreground/background/retained/team/remote 的 run 身份和事件来源。

### 14.2 MVP 与生产版边界

**8-12 人日 MVP** 可以做：现有 background snapshot 扩字段、running/failed/completed 行、权限 attention、结果预览、简单 group counts、六 agent demo。

但 MVP 不包含：可靠 resume run model、nested tree、dependency critical path、gap accounting、restart 后完整 focus/ack、screen reader 全验证、race/perf 广覆盖。把 MVP 称为“完整 subagent 展示策略实现”是在给技术债换名字。

### 14.3 推荐文件边界

尽量扩展现有边界，不另造平行系统：

- `tui/activity_store.go`：展示域 reducer、run/group projection；
- `tui/observation_store.go` / `tui/detail_store.go`：不可变 evidence；
- `tools/agent_progress.go`：事件事实与 gap，不负责渲染；
- `tools/runtime_task_store.go` / `tools/background_tasks.go`：durable snapshot 字段；
- `repl_tui.go`：runtime event -> presentation adapter，替换纯轮询实时路径；
- `tui/root.go`：L0/L1/L2 组件与 focus；
- `tui/renderer.go` / `permissions/structured_prompt.go`：decision/attention 连接；
- `ui/screen_reader_renderer.go`：语义播报；
- `tui/transcript_io.go`：隐藏 evidence 与 decision export。

### 14.4 验证门槛

至少新增以下证明：

1. 六 agent 同时启动且同名工具交错，身份/计数不串；
2. 一个 agent 1000 个 progress event 不能让 transcript 增长 1000 行；
3. progress buffer 丢帧后 terminal、error、permission 仍必达；
4. completed agent resume 后显示 running attempt 2，旧 terminal 证据仍可查；
5. terminal 后 late running event 不回退当前 run；
6. collapsed group 中 permission 与 failed 行仍可见；
7. inactive child permission 显示 actor/source 并可打开来源，返回后 draft/focus 不丢；
8. 40x12、80x24、120x40 viewport 不遮挡 action、错误和 input；
9. screen reader 只播语义变化，不播 progress 风暴；
10. restart/session switch 不混 epoch，不把旧 agent 通知投到新项目；
11. export 包含折叠内容的原 evidence、decision 和 transcript refs；
12. `go test -race` 覆盖 event hub/reducer/snapshot 并发路径。

## 15. 验收标准追踪

| Task 04 验收要求 | 本报告的落点 |
|---|---|
| 能回答 who is doing what | L1 actor/objective/latest event；字段合同 5.2-5.4 |
| 能回答 what changed | completed result/change summary；命令合同与 6-agent 演练 |
| 能回答 what failed | failure class/partial/evidence；第 10 节 |
| 能回答 how to inspect | open/details/transcript/evidence/jump actions；5.5、8、11 |
| 单一 noisy agent 不垄断 transcript | L0-L3、coalescing、有界 3 行/6 item、raw transcript 隔离 |
| 折叠时错误/权限仍提升 | attention shelf、10.1/10.2、12.2 演练 |
| 生命周期全覆盖 | spawning/queued/running/waiting/blocked/failed/cancelled/completed 矩阵 |
| 嵌套/聚合/顺序/late output | 第 7、9、10.3、11.2 节 |
| 可访问性/证据保留 | 第 11 节 |
| 六并发混合结果 walkthrough | 第 12 节 |

## 16. 压力测试裁决

### 必须修，否则展示会说谎

1. **Resume 没有 run identity** -> 同 ID 终态锁死。建议：agent 与 run 分层，单调性按 run 管。
2. **实时 progress 没接入展示** -> 行上无法回答“现在在做什么”。建议：事件 hub + coalescing，snapshot 只做恢复。
3. **权限/失败没有独立 attention 层** -> 父组一折叠就可能埋雷。建议：全局 shelf + 来源跳转 + durable decision/error。
4. **状态词汇把等待、阻塞、需要输入混在一起** -> 计数和 spinner 都不可信。建议：lifecycle/outcome/attention 三轴。

### 应该修，否则高并发很快失控

1. batch/parent/path/dependency 缺失 -> 只能靠 UI 猜树和关键路径；应从 runtime 源头生成。
2. snapshot 丢 timestamps/usage/transcript -> 重启后无法诚实展示；应保留 nullable facts。
3. typed Agent result 在默认 Observation 摘要里退化成字节数 -> 应做 typed semantic projector。

### 已经做对的基础

- 稳定 tool ID correlation、session/epoch 隔离；
- Observation summary/detail/evidence 三层；
- 错误默认展开和不可变 DetailRef；
- 权限请求包含 actor/work/execution session 且有决策审计；
- Activity 排序和 action 已有测试骨架。

一句话结论：**别把“把所有 child 输出都打印出来”叫透明，那只是把信息洪水改名为可观测性；真正的透明是主界面给出可行动摘要，任何失败和权限都能越级出现，而每一条摘要背后都有可打开、可归因、不会被折叠删除的证据。**
