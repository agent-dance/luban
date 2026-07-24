# 统一信息披露策略与命令展示合同

> Task: `task_03`
> 适用范围：本项目所有前台/后台工具、slash command、MCP、权限决策、任务/团队/子 Agent 事件
> 结论性质：产品无关的确定性策略；字段名结合本项目现有 Go 类型给出落地映射

## 0. 结论

展示策略不能由“输出超过 N 行就折叠”决定。正确模型包含三个相互独立的轴：

1. **事实保留（Retention）**：原始输入、输出、结构化 envelope、关联 ID 是否以及保留多久；由安全、隐私和会话保留策略决定。
2. **披露深度（Disclosure）**：当前界面显示隐藏、单行、结构化摘要还是完整证据；由下面的确定性优先级决定。
3. **注意力/承载面（Attention/Surface）**：信息留在状态栏、主 transcript、工作视图、decision overlay，还是系统通知；由是否需要用户行动和风险跃迁决定。

三轴不得互相替代。尤其是：

- 折叠只改变 presentation state，不能截断唯一的原始副本。
- “完整展示”指完整的**逻辑内容**，不等于把十万行文本一次性塞进 transcript；可以进入可分页、可搜索、可导出的 evidence viewer。
- 风险、审批、错误和用户明确请求会提高披露下限；终端宽度和输出体积只能改变布局/承载面，不能降低该下限。
- 可重复的低价值成功事件可以把每个成员隐藏到一个可展开的组内，但组必须保留成员数量、范围、结果以及逐成员证据索引。

## 1. 四级披露语义

本文以 `D0..D3` 表示同一 observation 的披露深度。它可以映射到本项目已有的 `DisclosureSummary / DisclosureDetail / DisclosureEvidence`，其中 `D0` 是聚合/投影层新增的“本条不生成独立 transcript 节点”，而不是删除 observation。

| 等级 | 名称 | 语义合同 | 允许的典型内容 | 禁止行为 |
| --- | --- | --- | --- | --- |
| `D0` | Hidden | 不生成独立可见节点，但仍更新组计数、状态和证据索引 | spinner tick、无变化 heartbeat、合法聚合组的单个低风险成功成员 | 隐藏错误、审批、破坏性动作、用户消息；丢失原始证据 |
| `D1` | Folded one-line | 一行领域摘要，维持因果主线 | `actor + action + object + outcome + key metric + has-more` | 对原始文本做盲目 `truncate(60)`；只显示“完成”而没有对象/范围 |
| `D2` | Structured summary | 足以让用户判断结果、影响和下一步的结构化详情；不要求包含全部 raw payload | 错误四要素、diffstat、失败用例、Agent 结果、warning、重试状态 | 用“详情可用”替代原因/影响/恢复动作；隐藏部分成功或副作用 |
| `D3` | Full logical content / Evidence | 决策所需字段全部可见，或完整原始证据可访问 | 权限/plan decision 全体字段；完整 stdout/stderr、diff、transcript、响应体 | 因体积或窄屏再次无提示截断；查看详情触发重放或新副作用 |

`D3` 的实现可为 inline、overlay、pager、`$EDITOR` 或 export；必须满足完整、可定位、无重放、可返回原焦点。若保留策略禁止保存某字段，D3 要明确显示 `not retained: <policy reason>`，不能伪装成空输出。

### 1.1 所有可见层的最小语义骨架

除纯内部事件外，任一可见 observation 至少具有：

```text
stable_id, session_id, turn_id, work_unit_id, actor,
family, action, primary_object, lifecycle_state, outcome,
started_at/sequence, has_more, evidence_refs
```

字段可不全部绘制，但领域 formatter 必须能访问它们。状态/结果必须来自 runtime 状态机，不得从模型摘要或 stdout 文案猜测。

## 2. 决策输入及离散化

决策器只接收结构化事实，禁止 renderer 临时解析自然语言来决定严重度。

| 输入 | 离散值 | 对披露的作用 |
| --- | --- | --- |
| 用户意图 `intent` | `quiet / normal / inspect / full / audit` | `inspect` 至少 D2；`full/audit` 为 D3；`quiet` 只能降低普通成功事件，不能越过硬下限 |
| 用户局部状态 | `pinned / explicitly_collapsed / untouched` | pinned D2/D3 不被后续结果或 resize 自动降级；显式折叠错误最低仍 D1 并保留错误标识 |
| 行动性 `actionability` | `none / review / input / approval / authentication / conflict` | input/approval/authentication/conflict 为 D3；review 至少 D2 |
| 风险 `risk` | `low / medium / high / destructive / external_side_effect` | 待执行 destructive/high-risk 为 D3；已执行的副作用回执至少 D2 |
| 结果 `outcome` | `queued/running/succeeded/warning/partial/failed/denied/cancelled/timed_out/disconnected/orphan/conflict` | warning 和所有非成功终态至少 D2；orphan/conflict 也是数据完整性错误 |
| 交互性 `interactivity` | `background / foreground-progress / requested-output / live-interactive` | 用户直接要求查看的输出为 D3；普通进度 D1，状态跃迁时升级 |
| 新颖度 `novelty` | `first / changed / repeated-identical` | 仅 repeated-identical 且低风险成功可参与 D0 聚合 |
| 持续时间 `duration` | `normal / slow / stalled / retrying` | elapsed 本身不刷屏；stalled/retrying 且影响判断时升至 D2 |
| 体积 `volume` | `small / large / huge / binary`，由语义单元、字节、行、宽字符和媒体类型共同判定 | 选择 inline/pager/export，不得单独决定严重度或是否保留 |
| 重复 `repetition` | `unique / groupable / ungroupable` | 只有通过第 4 节全部不变量的事件才可折入组 |
| 可访问性 | `fullscreen / classic / screen_reader / narrow / no_color` | 改变布局与更新方式，不改变语义等级 |

## 3. 确定性披露优先级

优先级从 P0 到 P10 依次判定。高优先级规则给出不可被低优先级覆盖的**下限**。同一事件命中多条时取最高披露等级；只有 P9/P10 可以选择 D0。

| 优先级 | 条件 | 最低/精确等级 | 说明 |
| --- | --- | --- | --- |
| P0 | 访问控制、secret/PII、二进制/控制字符安全 | 先净化，再继续判定 | 这是安全变换，不是折叠。禁止展示的字段在所有等级都脱敏；原文是否保留由独立 retention policy 决定 |
| P1 | 用户明确 `full/audit`、打开 evidence、或已 pin 在 D3 | `D3` | 只要证据存在就不得以体积/窄屏降级；改用 pager/virtualization |
| P2 | 正等待 approval、authentication、冲突选择，或破坏性动作执行前决策 | `D3` | 展示完整 decision contract；不得聚合、自动折叠或仅显示 Yes/No |
| P3 | 用户消息、assistant 最终答复、用户直接要求的命令原始输出 | `D3` | 与工具日志分离；可分页但不做有损摘要 |
| P4 | `orphan/conflict`、身份/关联不确定、证据完整性失败 | `D2` | 不能把关联失败伪装成普通工具失败；显示受影响 ID 和诊断入口 |
| P5 | `failed/partial/denied/cancelled/timed_out/disconnected/shutdown` | `D2` | 必须回答发生了什么、已完成什么、系统正在做什么、用户下一步是什么 |
| P6 | warning、自动重试、stalled、结果被截断/分页、scope 扩大或内容不完整 | `D2` | warning 不能藏在成功摘要内；重试原地更新，耗尽后转 P5 |
| P7 | `needs_review`、写入/删除/外部副作用已发生、plan gate、Agent 完成 | `D2` | 展示影响/产物/验证；原始细节仍在 D3 |
| P8 | queued/running、普通且唯一的成功事件 | `D1` | running 原地更新；成功由领域 formatter 给出对象、结果和关键指标 |
| P9 | 同组、低风险、无 warning 的重复成功事件，且组摘要已经可见 | 成员 `D0`，组 `D1` | 必须满足第 4 节全部聚合不变量；错误/审批到来立即拆组并升级 |
| P10 | 无语义变化的 tick、duplicate heartbeat、token/layout 内部事件 | `D0` | 只允许省略 presentation event；状态机和必要 telemetry 可继续记录 |

### 3.1 修饰规则

- `intent=inspect`：在上述下限上升一级，最高 D3。
- `intent=quiet`：只能把 P8 的普通成功 D1 放入一个可见 recap/group 后变为 D0；不能改变 P1-P7。
- `risk=high/destructive`：若尚未执行且需要决定，走 P2；若已经执行/自动允许，至少走 P7 并显示不可抵赖的回执。
- `volume=large/huge`：绝不直接降低等级，只把 D3 从 inline 转为 pager/editor/export，并在 D1/D2 显示数量、范围和 `has_more`。
- 用户显式把一个错误折叠回一行后，可以显示 D1，但必须保留 `failed`、对象和“details available”；不得进入 D0，也不得把显式折叠误记成“已解决”。
- pinned disclosure 是用户局部状态；全局 `show all` 不覆盖它，后续状态更新也不得自动降级。

## 4. 可执行算法伪代码

### 4.1 单 observation 决策

```text
enum Level { HIDDEN=0, ONE_LINE=1, SUMMARY=2, FULL=3 }

function decideDisclosure(o, user, env, policy): Decision
    # Retention and sanitization are independent of the visible level.
    retained = policy.retention.store(o.raw_payload, o.structured_envelope,
                                      o.evidence_metadata)
    safe = policy.redaction.project(o, user.access_scope)

    if safe.isPresentationOnlyTick and not safe.hasSemanticStateChange:
        return Decision(HIDDEN, surface="none", retained, reason="P10")

    level = ONE_LINE
    reasons = ["P8 baseline"]

    # Floors are applied from lower severity to higher severity; max() makes
    # the result independent of implementation branch order.
    if safe.lifecycle in {"warning", "retrying", "stalled"} or
       safe.isTruncated or safe.scopeExpanded:
        level = max(level, SUMMARY); reasons += ["P6"]

    if safe.outcome in {"failed", "partial", "denied", "cancelled",
                        "timed_out", "disconnected", "shutdown"}:
        level = max(level, SUMMARY); reasons += ["P5"]

    if safe.outcome in {"orphan", "conflict"} or not safe.identityIsReliable or
       safe.evidenceIntegrityFailed:
        level = max(level, SUMMARY); reasons += ["P4"]

    if safe.needsReview or safe.sideEffectOccurred or safe.planGate or
       safe.isTerminalAgentResult:
        level = max(level, SUMMARY); reasons += ["P7"]

    if safe.actionability in {"input", "approval", "authentication", "conflict"} or
       safe.pendingDestructiveAction:
        level = FULL; reasons += ["P2"]

    if safe.isUserAuthored or safe.isAssistantFinal or safe.isDirectlyRequestedOutput:
        level = FULL; reasons += ["P3"]

    if user.intent == "inspect":
        level = min(FULL, level + 1); reasons += ["intent=inspect"]

    if user.intent in {"full", "audit"} or user.requestedEvidence or
       safe.disclosure.userPinnedFull:
        level = FULL; reasons += ["P1"]
    else if safe.disclosure.userPinnedDetail:
        level = max(level, SUMMARY); reasons += ["pinned-detail"]

    # Quiet can only affect the safe-success baseline. It never lowers a floor.
    groupCandidate = false
    if user.intent == "quiet" and reasons == ["P8 baseline"] and
       safe.outcome == "succeeded":
        groupCandidate = true

    surface = chooseSurface(level, safe, env)
    return Decision(level, surface, retained, groupCandidate, reasons)
```

### 4.2 聚合判定

```text
function canAggregate(member, group): bool
    return member.decision.level <= ONE_LINE
       and member.outcome == "succeeded"
       and member.risk == "low"
       and member.actionability == "none"
       and not member.sideEffectOccurred
       and not member.warning
       and not member.userPinned
       and member.sessionId == group.sessionId
       and member.turnId == group.turnId
       and member.actorId == group.actorId
       and member.workUnitId == group.workUnitId
       and member.family == group.family
       and domainCompatible(member, group)       # same search intent, read batch, etc.
       and group.isLive                         # frozen turn history is immutable

function addToGroup(member, group):
    assert canAggregate(member, group)
    group.memberIds.append(member.id)
    group.evidenceRefs.append(member.evidenceRefs)
    group.count += 1
    group.range = domainUnion(group.range, member.primaryObject)
    group.metrics = domainReduce(group.metrics, member.metrics)
    member.visibleLevel = HIDDEN
    group.visibleLevel = ONE_LINE

function onNonGroupableEvent(event, group):
    freeze(group)                               # stable summary and exact member index
    render(decideDisclosure(event, ...))        # warning/error is a separate visible item
```

禁止使用仅按时间窗口或 tool name 的聚合键。至少需要 `session + turn + actor + work_unit + family + domain intent`；并行同名工具结果必须用稳定 ID 关联，不能依赖邻接或 LIFO。

### 4.3 承载面选择

```text
function chooseSurface(level, o, env): Surface
    if o.actionability in {approval, authentication, conflict, input}:
        return DECISION_OVERLAY
    if level == FULL and (o.volume in {huge, binary} or env.isNarrow):
        return EVIDENCE_VIEWER_OR_EDITOR
    if o.isBackgroundWork and level <= SUMMARY:
        return WORK_VIEW_WITH_TRANSCRIPT_TRANSITION
    if o.lifecycle == running and level == ONE_LINE:
        return IN_PLACE_ACTIVITY
    return TRANSCRIPT
```

### 4.4 状态跃迁不变量

1. `queued -> running` 原地更新一个稳定 ID，不追加 spinner tick。
2. `running -> warning/failed/needs_input` 只能保持或提高披露等级；警告历史必须进入终态摘要。
3. `running -> succeeded` 可以生成简洁终态摘要，但不能覆盖用户 pinned disclosure。
4. 同一个 operation 的 call 与 result 共享 identity 和 disclosure state。
5. turn 结束后 group 冻结；后到的错误作为显式 late/conflict observation，不重写旧成功事实。
6. 展开 D3 不重放工具、不重新请求网络、不改变工作目录；关闭后恢复焦点、scroll anchor 和输入草稿。

## 5. 命令族字段合同

下表中的“必选字段”是领域 presentation model 的必选字段，不代表每个字段都必须塞进 D1。D1 选取动作、主对象、结果和最多一两个判断指标；D2/D3 逐层补全。`evidence_ref` 在存在原始输入/输出时始终必选，但可以只以 `details available` 呈现。

| 命令族（本项目实例） | 必选字段 | 条件/可选字段 | 领域摘要要求 |
| --- | --- | --- | --- |
| 文件（`Read`, `Write`, `Edit`, `FileAppend`, `FileDelete`, `FileMove`, `FileLink`, `NotebookEdit`, `FileList`） | action；规范化 path/paths；读范围或变更类型；actor/work unit；outcome；bytes/lines 或 files changed；risk；evidence_ref | encoding、line range、old/new path、diffstat、mode/symlink、hash、失败文件、backup/rollback | 读成功为 `Read <path> · <lines>`；变更必须说明文件数和影响，删除/覆盖不得只显示 `Done` |
| 搜索（`Glob`, `Grep`, `FileGlob`, `FileSearch`, `ToolSearch`, `LSP`） | query/pattern；scope/root；search mode；outcome；match count；file count；truncated/has_more；evidence_ref | case/regex/context、include/exclude、duration、top locations、diagnostic severity | 空结果也显示已搜范围；海量结果显示总数与范围，不能让首 N 行冒充完整结果 |
| Shell/进程（`Bash`, `PowerShell`, process tools、后台 task output） | actor；原始/安全引用 command；cwd/host/sandbox；lifecycle；exit code/signal/interrupted；duration；stdout_ref；stderr_ref | pid/task_id、timeout、retry、resource metrics、environment delta、test pass/fail/skip、partial effects | D1 不省略 cwd/结果；测试用领域结果而非最后一行；失败 D2 分开呈现 exit 与 stderr/部分成功 |
| Web/网络（`WebSearch`, `WebFetch`, `HttpGet/Post`, DNS/Ping/port/Whois） | action/method；规范化 URL/domain/query；outcome/status；final source/redirect；duration；bytes/results；evidence_ref | content type、cache、rate limit、retry、citation titles、robots/policy、open-world risk | 不展示 credential/query secret；搜索要显示结果/来源数，fetch 要显示最终 URL 与 status，失败区分网络、策略和内容错误 |
| MCP（dynamic tool、`MCPTool`, resource/prompt/list/auth） | server；capability/tool/resource/prompt；action；actor；outcome；latency；correlation/tool_use_id；evidence_ref | server version、transport、readOnly/destructive/openWorld annotation、retry、auth state、schema validation | D1 始终点名 server；断连/认证按 server 聚合为健康状态，但具体失败 observation 不被吞掉 |
| Task/Team/Subagent（`Agent/Task`, `Task*`, `Team*`） | task/agent/team id；profile/actor；parent id；work unit；objective/description；state/phase；outcome；blocked reason；artifact/evidence refs | latest tool、tool/message count、elapsed、tokens/cost、model、worktree/cwd、owner、control actions | 主 transcript 只保留状态跃迁与结果；并行活动在 work view 按父子树和 actionability 分区；详见第 7 节 |
| Plan/Goal/Checklist（`Enter/ExitPlanMode`, `TodoWrite`, `Get/Create/UpdateGoal`） | artifact/goal/plan id；version；mode/gate；scope；step states；active step；outcome；decision state；evidence_ref | owner、dependencies、estimates、changed-since-review、test criteria、budget | plan 审批展示完整 plan 和变更；日常 checklist 更新可折为当前项+剩余数；不能用百分比替代具体阻塞项 |
| Worktree/Git/Session（`Enter/ExitWorktree`, git operations、session/resume） | action；repo/project；original/current cwd；worktree；branch/base；dirty/conflict state；outcome；resulting location；evidence_ref | commit SHA、remote、diffstat、ahead/behind、stash、session name/time | 创建/切换/退出后显示最终 cwd/branch；冲突和脏工作区为 D2/D3 决策，不能只说 checkout failed |
| Permission/Decision（permission prompt、plan gate、`AskUserQuestion`） | decision_id；actor；action；target/scope；impact；risk level/reason；rule source；approval scope/persistence；完整 choices 及语义；deadline；request body/review details | tool input 的安全投影、post mode、matching rule、session/work unit | pending 必须 D3、不可聚合；resolved 留 D2 不可抵赖回执（谁、何时、选择、作用域），Esc/timeout/cancel 彼此区分 |
| Messaging（`SendMessage`, `SendUserMessage`, follow-up/notification/shutdown） | sender；recipient/audience；message type；session/work unit；content 或正式摘要；delivery state；time/sequence；correlation id | attachments、reply target、urgency、delivery attempts、redaction note | 用户可见消息与最终答复完整显示；内部 routine ack 可 D1/聚合；shutdown/needs-input 等控制消息至少 D2，失败显示是否送达 |

### 5.1 六状态披露矩阵

符号：`D0` 隐藏独立成员；`D1` 单行；`D2` 结构化摘要；`D3` 完整逻辑内容/证据。斜线表示根据同格条件二选一，而不是随机启发式。

| 命令族 | Default/queued | Running | Success | Warning/partial | Error/denied | User-expanded |
| --- | --- | --- | --- | --- | --- | --- |
| 文件读 | D1 | D1，批量原地计数 | D1；合法批量成员 D0+组 D1 | D2（范围/编码/截断） | D2 | D3 原文/完整元数据 |
| 文件写/改/删 | D1；待审批走 D3 | D1；多文件为 change-set | D2（files+diffstat+失败数）；纯 no-op 可 D1 | D2 | D2（含已发生副作用/回滚） | D3 diff/完整内容/回执 |
| 搜索 | D1 | D1（scope+当前计数） | D1；用户直接要全结果则 D3 | D2（truncated/scope changed） | D2 | D3 全部命中，可分页 |
| Shell/进程 | D1；待审批走 D3 | 普通 D1；live-interactive/requested-output D3 | 只读 D1；写入/测试 review D2 | D2 | D2 | D3 stdout+stderr+envelope |
| Web/网络 | D1；授权/开放世界决策 D3 | D1（host+phase/retry） | D1；直接请求响应体 D3 | D2（redirect/rate/partial） | D2 | D3 完整响应/搜索证据 |
| MCP | D1；auth/危险 annotation 决策 D3 | D1（server+capability） | D1；server rollup 可组 | D2（schema/version/degraded） | D2 | D3 完整 request/result envelope |
| Task/Team/Subagent | D1 | D1 组/树；needs-input D3 | routine bookkeeping D1；Agent/工作单元终态 D2 | D2 | D2；需要选择时 D3 | D3 子 transcript/产物/证据 |
| Plan/Goal/Checklist | D2；plan approval D3 | D1（active step） | checklist D1；plan/goal gate D2 | D2 | D2 | D3 完整 artifact/history |
| Worktree/Git/Session | D2；冲突决策 D3 | D1 | 只读 D1；切换/提交/推送 D2 | D2 | D2；冲突决策 D3 | D3 diff/log/session preview |
| Permission/Decision | D3 | D3（等待倒计时不刷屏） | D2 receipt | D2/D3（规则/范围变化） | D2 receipt；需重选 D3 | D3 原请求、规则、完整回执 |
| Messaging | 用户/正式消息 D3；内部通知 D1/D2 | delivery D1 | routine ack D0/组 D1；正式消息保留 D3 | D2 | D2 | D3 完整 message/attachments |

## 6. 错误、warning 与完整展示合同

所有 D2 错误必须按固定顺序回答：

```text
1. What happened: operation + object + deterministic outcome/code
2. Preserved work: partial results, files/effects already produced
3. Automatic action: retry/rollback/waiting/none, attempt and deadline
4. User action: next safe command/choice, or "no action required"
5. Evidence: stable details link/ref
```

warning 不能写成绿色 success 的尾注；它至少是 D2，或在成功 D1 旁有独立、可朗读的 warning 标记和一键 D2。`cancelled_by_user`、`cancelled_by_parent`、`timed_out`、`denied`、`disconnected`、`partial` 不得归并成一个 `failed` 文案。

## 7. Subagent / Team 专项展示合同

### 7.1 主视图与子视图边界

- 主 transcript 只展示 spawn、needs-input/approval、重要 warning、terminal result 等**状态跃迁**；不交错复制每个子 Agent 的 token、spinner 和工具 raw output。
- work view 以 `parent_agent_id -> agent_id -> work_unit_id` 构树，先按 `Needs input / Failed / Ready for review / Running / Completed` 分区，再在分区内按父子和开始顺序稳定排序。
- 行摘要必须点名 `profile/name + objective + state`，不能只写 `Task completed`。同名 Agent 依靠稳定 ID 区分。
- 子 Agent 输出中的确定性状态、产物和验证来自事件/工具事实；模型生成 headline 只能作为补充，不能决定 completed/failed。
- 完整子 transcript 进入 D3，可以搜索/导出；主 Agent 只接收正式 result/structured message，不展示隐藏推理过程。

### 7.2 Subagent 状态矩阵

| 状态 | 主 transcript | Work view row/detail 的必选字段 | 注意力 |
| --- | --- | --- | --- |
| spawned/queued | D1 一次 | agent id/name/profile、parent、objective、cwd/worktree、foreground/background、queued time | A1 |
| running | 不追加 tick；D1 原地活动 | phase、elapsed、latest **semantic** activity、tool/message count、cancel/inspect actions；token/cost 可选 | A1 |
| needs input/approval/auth | D3 独立 decision，不藏在树计数里 | requester、完整 question/action/target、为何阻塞、choices/scope/deadline、父 work unit | A3 |
| warning/stalled/retrying | D2 状态跃迁 | cause、attempt、last progress、已保留产物、自动动作、何时需要用户介入 | A2 |
| completed/ready for review | D2 terminal result | deterministic outcome、正式 result、artifacts/files、verification、duration、tool count、usage、完整 transcript ref | A2；用户订阅时可系统通知 |
| failed/partial/timed out | D2，若需要选择则 D3 | 失败原因、partial output/effects、last tool、cleanup/retry state、next action、transcript/evidence ref | A2/A3 |
| cancelled | D2 receipt | initiator（user/parent/system）、reason、time、partial effects、children disposition | A2 |
| resumed/attached | D1 transition | original id、session/worktree、previous state、new owner/focus、unread transitions | A1 |

### 7.3 并发与聚合

并发 Agent 的组摘要示例：

```text
Research · 6 agents: 3 running, 1 needs input, 1 ready, 1 failed
```

其中 `needs input` 和 `failed` 必须拥有独立可达节点；不能把“6 agents”作为唯一入口。组内每个 Agent 保留独立状态、输出和证据。父 Agent 失败或取消时必须说明子项是 cancelled、detached 还是仍在后台运行，不能假设级联成功。

## 8. Narrow terminal 与 screen reader

披露等级在所有 renderer 中保持一致；只改变排版和更新协议。

### 8.1 Narrow terminal

字段保留顺序：

```text
actionability/risk > outcome > actor > action > primary object
> next action > has-more > critical metric > secondary metrics/decorations
```

- 先整段移走低优先级 metrics，再对 object 做中间省略；绝不删 outcome、risk、needs-input、错误码或 next action。
- 同名/同尾路径不能因截断变得不可区分；必要时分两行显示 scope 与 basename。
- D2/D3 允许自然换行；用户已展开内容在 resize 后 reflow 或进入 pager，不自动折回 D1。
- 使用 cell-width aware 的 CJK/emoji/combining-character 计算；不能按 byte/rune 数截断。
- 所有省略都显示 `… +N` 或 `details available`，并有键盘可达的展开动作。

### 8.2 Screen reader

- 使用追加式、线性、稳定顺序：`actor -> action -> object -> state -> result -> next action -> details available`。
- spinner tick、token tick、无变化 progress 不朗读；只朗读首次 running、重要里程碑和终态。原地视觉更新必须对应一条去重后的状态跃迁文本。
- 颜色、icon、缩进、并列列不能承载唯一语义；显式朗读 `Failed`、`High risk`、`Collapsed, details available`。
- group 摘要先朗读总数和异常数；异常成员紧随其后，不能要求用户靠二维位置发现。
- decision choices 带稳定序号、完整含义、授权时长/范围；焦点默认落在安全选项，但不能自动提交。
- D3 长内容按逻辑段分页并提供位置，例如 `lines 201-300 of 2,431`；搜索/导出与视觉模式等价。

## 9. 代表性用例演算

以下把第 3/4 节策略实际运行在 success、failure、approval、long-output、repetition 和 subagent 场景上。

| 用例输入 | 命中规则 | 决策结果 | 必须保留/显示的证据 |
| --- | --- | --- | --- |
| 同一 actor/turn/work unit 连续读取 42 个 Go 文件，全部成功、低风险 | 各成员 P8，且满足 P9 聚合 | 42 个成员 D0；一个 D1：`Read 42 files · 8,214 lines` | 42 个 stable ID/path/result/evidence ref；展开后逐成员可达 |
| `go test ./...` 输出 18,000 行，exit 1，前 127 包通过、2 个失败 | P5；volume 只选 surface | D2：command/cwd/exit/duration/pass/fail、失败包、部分成功、next action；D3 pager 有完整 stdout/stderr | 完整输出、结构化测试统计、exit/signal、关联 ID |
| Agent 请求执行 `rm -rf ./build-cache`，需一次性批准 | P2（pending destructive approval） | D3 decision overlay，不能折叠 | actor、原命令、cwd、target、影响、risk reason、rule source、allow-once/deny 等 choice 语义 |
| 用户明确“显示 WebFetch 原始响应”，响应 6 MB | P3/P1；volume=huge | D3 evidence viewer，不降成摘要 | final URL、status/headers、完整允许展示的 body、分页位置、导出；secret 仍按 P0 脱敏 |
| 30 个相同 MCP list 调用成功，第 31 个认证失败 | 前 30 个 P9，第 31 个 P5 | 成功组 D1；认证失败独立 D2/D3 auth decision，组不能吸收失败 | server/capability、成功成员索引、失败 error/auth scope、retry/next action |
| 6 个 subagent 并发；3 running、1 completed、1 needs input、1 failed | running P8，completed P7，needs input P2，failed P5 | work group D1；completed/failed D2；needs input D3 且主 transcript 单独出现 | 每个 agent id/parent/state；正式 result、failure evidence、完整 question 与 transcript refs |
| 自动允许的 `FileDelete` 成功，无审批界面 | 已发生 destructive side effect，P7 | D2 回执，不得因 success 进入 D0/P9 | actor、path/scope、删除数量、policy/rule、time、result/evidence |
| 40 列终端 + screen reader，后台任务由 running 转 timed_out | P5 + accessibility transform | 同为 D2；线性朗读一次终态，visual 模式换行 | task id/object、Timed out、deadline、partial output、next action、details link |

这些结果证明：错误、审批、破坏性动作和用户明确请求不会被静默折叠；重复成功可以分组但证据不丢失；窄屏/读屏只改变布局而不改变语义等级。

## 10. 反例与无效实现

1. **统一行数阈值**：一行 `rm -rf /` 比 500 行只读日志风险高；行数不能代表重要性。
2. **先截断再存储**：将 tool result 先裁成 20 行再创建详情，D3 永远无法恢复，属于数据丢失而非渐进披露。
3. **只取 head/tail**：关键 error、partial side effect、warning 可能在中间；必须用领域解析器+完整 evidence ref。
4. **把 exit code 当全部语义**：`grep` 的 exit 1 可能是正常“无匹配”，exit 0 也可能伴随 warning/partial result；由命令族解释结果。
5. **按同名/邻接关联并行结果**：并发的两个 `Bash`/`Agent` 会错配输入和输出；必须使用 `tool_use_id`/agent id。
6. **错误混入成功组**：`29 succeeded, 1 failed` 不能渲染为绿色 `30 operations completed`；失败必须拆组并显示下一步。
7. **丢失部分副作用**：写了 4/5 个文件后失败，若只显示 failed 会诱发危险重试；D2 必须列出已完成/未完成和幂等性。
8. **用最后一行总结测试**：progress bar、清理日志或 shell prompt 可能是最后一行；测试摘要要读取结构化结果或已知 runner contract。
9. **把聚合等同计数**：`17 operations completed` 丢失意图、范围和对象；正确摘要是 `Grep "SessionID" in 42 files · 17 matches`。
10. **高风险成功自动隐藏**：删除、push、发送外部消息即使成功也需要 D2 回执；success 不是 low value 的同义词。
11. **approval 仅显示 Yes/No**：没有 actor、target、影响、scope 和 rule source，用户无法做知情决策。
12. **resize 自动折叠**：用户正在审阅 D3 diff 时窗口变窄，不得把它降成 D1；应 reflow/pager 并恢复焦点。
13. **读屏依赖原地 spinner**：视觉上状态变了但没有线性状态跃迁文本，读屏用户会错过完成/失败。
14. **颜色作为唯一状态**：红/绿点无法被无色终端或 screen reader 识别；必须同时有文字。
15. **路径末尾截断**：`service-a/config.yaml` 与 `service-b/config.yaml` 都变成 `…/config.yaml`，会误导审批；保留可区分 scope。
16. **D3 触发重放**：点击“详情”重新执行 WebFetch/Bash 会改变证据、产生费用或副作用；详情必须读取 immutable store。
17. **隐藏内容没有数量/入口**：用户无法知道丢了多少结果；任何折叠/分页都要显示 count/range/has_more。
18. **raw ANSI/控制字符直通**：工具输出可移动光标、伪造 UI 或污染终端；存储原始字节与安全渲染投影必须分开。
19. **模型摘要决定终态**：headline 写“done”不代表 runtime completed；状态、错误和审批必须来自确定性事件。
20. **secret 因“full”重新出现**：P1 不覆盖 P0；用户请求完整输出仍受访问控制和脱敏策略约束。

## 11. 与本项目现状的映射

本项目已有实现足以承载该策略的大部分核心：

- `tui/observation_store.go` 已定义确定性 `ObservationOutcome`，并区分 succeeded、failed、partial、denied、cancelled、timed_out、orphan、conflict 等，不需要从字符串猜状态。
- 同文件已有 per-observation 的 `DisclosureState{Level, HasMore, UserPinned}` 以及 `Summary -> Detail -> Evidence`，并用 `(session, tool_use_id)` 关联 call/result。
- `defaultResultDisclosure` 已把错误和非正常终态提升到 Detail；它应被本文决策器替代/包裹，以加入 actionability、risk、intent、interactivity、repetition 和 command-family formatter。
- `tui/detail_store.go` 的 `DetailRef{Source, Key, Size, Digest}`、内存/私有文件 store 和 evidence journal 已实现“展示与证据保留分离”的关键不变量。
- `permissions/structured_prompt.go` 的 `PromptRequest` 已含 decision/session/tool/actor/work unit、action/target/impact、risk reason、rule source、approval scope、choices、body/review details，足以实现 D3 decision contract。
- `tui/activity_store.go` 可承载 running/group/work view；Agent/Task/Team 已有 stable id、parent、phase、latest tool、usage 与 transcript/output 引用，适合第 7 节合同。

还需补齐的策略层边界是：

1. 给 observation/presentation 增加领域 `Family`、`Actionability`、`Risk`、`SideEffectOccurred`、`Warning`、`Volume`、`Intent` 投影，或通过纯函数上下文提供。
2. 增加 `D0` 聚合投影而不是修改/删除 observation；group 保存成员 ID 和 evidence refs。
3. 为上表每个 command family 注册 deterministic formatter，禁止 renderer 对 raw text 通用截断。
4. decision policy 输出 `{level, surface, reason_codes}`；reason codes 用于测试、调试和可解释性。
5. fullscreen、classic、screen-reader 共用同一决策结果，只实现不同 renderer adapter。

## 12. 验收测试清单

实现阶段至少建立以下 table-driven/property tests：

- P0-P10 每条优先级和两两冲突：`quiet + failed`、`huge + approval`、`pinned + success update`、`full + secret`。
- 所有非成功 outcome 的下限不低于 D2；pending decision 必为 D3；显式折叠错误不低于 D1。
- 任何进入 D0 的 observation 都能在某个可见 group 中找到 stable ID，且 evidence digest/size 可校验。
- 聚合 key 对不同 session/turn/actor/work unit/intent 不串组；成员中出现 warning/error 时拆组。
- 每个命令族覆盖 queued/running/success/warning/error/expanded 六态，以及 large/huge/binary。
- call/result 乱序和同名并发不靠邻接关联；orphan/conflict 自动 D2。
- user expand 不重放工具；close/resize 后焦点、scroll anchor、draft 恢复。
- screen-reader 输出无 spinner/token 噪音，状态跃迁 append-only、去重且含文字 outcome。
- 40/80/120 列、CJK/emoji/长路径下关键字段不丢失、不重叠、同名对象可区分。
- 100,000 observations 时可见树与 viewport + pinned/expanded 数量成比例，而不是全历史线性渲染。

最终验收标准不是“界面看起来更短”，而是：用户能在最低必要信息量下判断当前状态、风险和下一步，并能无损进入证据；同时正常重复工作不会淹没主因果线。
