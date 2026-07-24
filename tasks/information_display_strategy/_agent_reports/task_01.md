# Claude Code 信息展示策略证据地图

> 任务：`task_01`<br>
> 调查日期：2026-07-15<br>
> 范围：Claude Code 交互终端中 thinking、工具调用、结果、错误、长命令、权限、后台工作、任务清单和 subagent 的展示密度与展开机制。<br>
> 结论口径：本文描述“当前官方文档 + 本地 TypeScript 基线所能证明的行为”，不把推断写成已观察事实。

## 1. 证据口径与基线

本文使用以下标签：

- **[官方事实]**：Anthropic 官方文档或官方 `anthropics/claude-code` changelog 明示。
- **[源码事实]**：本地 `../src` TypeScript 基线中的直接控制流、阈值或渲染字段。
- **[直接观察]**：在真实 Claude Code TUI 中复现得到。本任务没有启动交互式 Claude Code 做视觉走查，因此本类证据为空。
- **[推断]**：由多条官方/源码证据归纳出的产品策略，不代表 Anthropic 对设计原则的公开表述。
- **[未知]**：证据不足、版本面不一致或受 feature flag/终端能力影响。

本地基线通过 `git -C ../src rev-parse HEAD` 固定为 `0136d07ff40bd3ecf874f86d0679df878e6528e5`（提交时间 2026-07-15）。官方 changelog 调查时最新条目为 `2.1.209`；本地源码没有可用于证明其发行版本号的稳定常量，因此本文不会把这两者直接等同。[官方 changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)

主要官方来源：

- [Interactive mode](https://code.claude.com/docs/en/interactive-mode)
- [Fullscreen mode](https://code.claude.com/docs/en/fullscreen)
- [Create custom subagents](https://code.claude.com/docs/en/sub-agents)
- [Permissions](https://code.claude.com/docs/en/permissions)
- [Interactive commands](https://code.claude.com/docs/en/commands)
- [Claude Code changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)

## 2. 排名结论

| 排名 | 结论 | 置信度 | 依据 |
|---|---|---:|---|
| 1 | Claude Code 的首要规则不是“按字符数折叠”，而是先按**生命周期与决策风险**分层：等待授权、运行中、失败等状态优先暴露；成功后的细节才更积极地收拢。 | 高 | 工具头部有 queued/running/permission/error/success 分支，权限又按工具类型进入专门 UI：`../src/components/messages/AssistantToolUseMessage.tsx:102-156`、`../src/components/messages/AssistantToolUseMessage.tsx:239-265`、`../src/components/permissions/PermissionRequest.tsx:47-81`。 |
| 2 | 第二层规则是**命令语义**：每个工具自己决定默认字段与“最小完成证据”，不存在一个通用的 raw JSON 展示模板。 | 高 | Bash、Read、Grep、WebSearch、WebFetch、MCP、Agent 都有独立 renderer；详见第 6 节源码矩阵。 |
| 3 | 第三层规则是**信息量与重复度预算**：长输出截行，同类连续调用聚合，读/搜类跨工具折叠，subagent 只把摘要带回父上下文。 | 高 | `../src/utils/terminal.ts:7-60`、`../src/utils/groupToolUses.ts:48-64`、`../src/components/messages/CollapsedReadSearchContent.tsx:142-292`；[subagent 官方文档](https://code.claude.com/docs/en/sub-agents)。 |
| 4 | “折叠”与“丢弃”是两回事：默认 UI 可以只显示一行，但 transcript、局部展开、任务详情或输出文件仍可保留证据；真正的上下文隔离/压缩由 subagent、compaction 等另一层负责。 | 高 | fullscreen 点击同时展开 tool call/result，`[` 把全会话以展开态写入原生 scrollback；后台 Bash 输出存文件；subagent 只返回摘要。[Fullscreen](https://code.claude.com/docs/en/fullscreen)、[Interactive mode](https://code.claude.com/docs/en/interactive-mode)、[Subagents](https://code.claude.com/docs/en/sub-agents)。 |
| 5 | 展示密度是**视图级 + 行级**共同控制：`Ctrl+O` 进入详细 transcript；fullscreen 可点击单行；transcript 内 `Ctrl+E` 控制更老消息；`/focus` 则主动切到安静视图。 | 高 | `../src/components/Messages.tsx:559-624`、`../src/keybindings/defaultBindings.ts:32-49`、`../src/keybindings/defaultBindings.ts:161-169`；[Fullscreen](https://code.claude.com/docs/en/fullscreen)。 |

## 3. 展示密度决策模型

### 3.1 四档密度不是单一枚举，而是多层决策结果

| 密度 | 典型条件 | 默认内容 | 如何展开 | 证据是否仍保留 |
|---|---|---|---|---|
| **隐藏** | 空 thinking、标记 `hideInTranscript` 的 thinking、已解析的透明包装工具、非 transcript 的 Pre/PostToolUse hook、前几次可重试 API 错误、quiet/focus 视图中的历史细节 | 不占行，或只由相邻聚合摘要代表 | `Ctrl+O`/退出 `/focus`；部分内容只能在 transcript/任务详情看到 | 通常只是 UI 隐藏；源码仍在 message/task lookup 中。`../src/components/messages/AssistantThinkingMessage.tsx:33-40`、`../src/components/messages/AssistantToolUseMessage.tsx:123-156`、`../src/components/messages/HookProgressMessage.tsx:34-79`、`../src/components/messages/SystemAPIErrorMessage.tsx:10-39`。 |
| **单行** | thinking 默认态、工具调用头、成功统计、后台任务 pill、完成 subagent | 身份 + 关键输入 + 状态/计数；不铺开原始 payload | `Ctrl+O`；fullscreen 中仅“确有更多内容”的结果行可点 | 是；展开时复用同一 tool/result key。`../src/components/messages/AssistantThinkingMessage.tsx:40-58`、`../src/components/Messages.tsx:559-624`。 |
| **紧凑块** | 长输出、运行中命令、连续读/搜、subagent 进度、任务清单 | 少量尾部输出、计数、耗时、最近活动、`+N` 提示 | `Ctrl+O`、fullscreen 点击、进入 `/tasks` 或 agent detail | 是，但具体保留位置不同：主 transcript、subagent transcript、task state 或 output file。`../src/utils/terminal.ts:7-60`、`../src/tools/AgentTool/UI.tsx:445-625`。 |
| **完整** | transcript/verbose、fullscreen 单行展开、`Ctrl+E` show all、`[` dump、权限决策面 | 完整 thinking/tool input/result（仍受工具自身硬限制、上游 payload 和安全边界约束） | 视图本身即展开；`v` 可进外部编辑器 | 当前可渲染记录被完整呈现；不代表被 compaction/pruning 删除的历史可恢复。[Interactive mode](https://code.claude.com/docs/en/interactive-mode)、[Fullscreen](https://code.claude.com/docs/en/fullscreen)。 |

### 3.2 决策顺序

**[推断，高置信]** 从源码分派顺序可归纳为以下优先级：

1. **能否渲染**：空内容、透明 wrapper、brief/focus 模式先过滤。
2. **当前状态**：queued、classifier checking、waiting permission、running、rejected、error、success 决定状态符号和文案。
3. **工具语义**：调用独立工具 renderer，选取路径、pattern、URL、command、diffstat 等关键字段。
4. **上下文位置**：主线程、subagent condensed view、transcript、fullscreen、最新 `!` shell 结果各有不同密度。
5. **重复与体积**：同工具调用分组、read/search 聚合、行数/宽度截断。
6. **用户显式展开**：全局 `Ctrl+O`、transcript `Ctrl+E`、fullscreen 行点击、`[` dump 或任务详情覆盖默认密度。

直接支撑这一顺序的入口包括 `../src/components/Messages.tsx:421-529`、`../src/components/Messages.tsx:559-624`、`../src/utils/groupToolUses.ts:48-64` 和 `../src/components/messages/AssistantToolUseMessage.tsx:102-265`。

## 4. 生命周期行为矩阵

| 生命周期 | 默认展示密度 | 默认展示什么 | 展开/操作 | 留存证据 |
|---|---|---|---|---|
| **Queued** | 单行 | 工具状态点、工具名/关键输入和 `Waiting…`；透明 wrapper 只在 running 时显示 progress | `Ctrl+O` | tool use/message 仍在 lookup。`../src/components/messages/AssistantToolUseMessage.tsx:102-156`、`../src/components/messages/AssistantToolUseMessage.tsx:239-265`。 |
| **Classifier checking** | 单行动态状态 | Bash 等权限分类器检查文案 | 等待完成；权限界面可进入 debug/explanation | 状态字段 `classifierCheckInProgress` 保留在确认对象。`../src/components/messages/AssistantToolUseMessage.tsx:239-265`、`../src/components/permissions/PermissionRequest.tsx:103-125`。 |
| **Running** | 单行或紧凑块 | 状态点、工具专属 progress；Bash 显示尾部 5 行、耗时、累计行/字节；Agent 显示最近 3 条活动或窄屏统计 | `Ctrl+B` 可将 Bash/agent 后台化；`Ctrl+O` 展开 | 主 message、task progress 或后台输出文件。`../src/components/shell/ShellProgressMessage.tsx:19-83`、`../src/tools/AgentTool/UI.tsx:445-510`；[Interactive mode](https://code.claude.com/docs/en/interactive-mode)。 |
| **Permission required** | 完整决策面 | 请求者/worker badge、工具专属关键 payload、触发规则解释、允许/持久允许/拒绝选项；Bash 可显示 destructive/unsandboxed 警告 | `Ctrl+E` 按需生成 what/why/risk；Esc 拒绝 | 决策及反馈进入 permission response；后台 subagent 请求会在主会话显示且命名请求者。`../src/components/permissions/PermissionRequest.tsx:47-81`、`../src/components/permissions/BashPermissionRequest/BashPermissionRequest.tsx:435-480`；[Permissions](https://code.claude.com/docs/en/permissions)、[Subagents](https://code.claude.com/docs/en/sub-agents)。 |
| **Success** | 单行或语义紧凑块 | 绿色完成点 + 最小完成证据：例如行数、文件数、diff、HTTP 状态、搜索次数、token/tool count | `Ctrl+O`；fullscreen 对截断结果可点 | tool result 保留；工具可只渲染摘要而不把原始内容展示到 UI。`../src/components/ToolUseLoader.tsx:18-33`、`../src/components/messages/UserToolResultMessage/UserToolResultMessage.tsx:36-89`。 |
| **Error** | 显著但截断 | 红色状态；优先输出语义错误；通用验证错误默认 `Invalid tool parameters`；非 verbose 通用错误最多 10 行 | `Ctrl+O` 展开 exact error | error block 保留。`../src/components/FallbackToolUseErrorMessage.tsx:11-86`、`../src/components/ToolUseLoader.tsx:18-33`。 |
| **Retrying API error** | 早期隐藏，持续失败才显示 | 前 3 次 retry 不显示；之后展示错误与重试倒计时；非 verbose 文本最多 1000 字符 | transcript/verbose 可得到更多文本，但仍受上游错误内容限制 | system error message 仍参与重试状态。`../src/components/messages/SystemAPIErrorMessage.tsx:10-39`、`../src/components/messages/SystemAPIErrorMessage.tsx:58-106`。 |
| **Rejected / canceled / interrupted** | 独立结果态 | 与普通 error 分派分离，显示 rejected/canceled 语义；文件修改可显示被拒绝的 diff 摘要 | `Ctrl+O` 查看更完整内容 | 分派按 cancellation/reject/interruption/is_error 顺序处理。`../src/components/messages/UserToolResultMessage/UserToolResultMessage.tsx:36-89`。 |
| **Backgrounded** | footer pill + task 列表 | 任务类型、截断后的命令/描述、running/done/unread；主行给下箭头或 `/tasks` CTA | `/tasks`、下方向键、foreground/view/stop；`Ctrl+B` 进入后台 | Bash 有 task ID/output file；agent 有 task state/transcript。`../src/components/tasks/BackgroundTask.tsx:17-148`、`../src/components/tasks/BackgroundTaskStatus.tsx:192-233`；[Interactive mode](https://code.claude.com/docs/en/interactive-mode)。 |
| **Background completed** | 仍留在 `/tasks` | `done`，未读时附 `unread`；v2.1.208 起完成 background agent 不立即消失 | 打开 detail/attach；可清理 | 完成 agent 留到 cleanup；失败/停止项的保留策略与完成项不同。[Subagents](https://code.claude.com/docs/en/sub-agents)、[Changelog 2.1.208](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)。 |

## 5. 全局交互契约

### 5.1 视图与快捷键

| 操作 | 展示效果 | 源码/官方依据 |
|---|---|---|
| `Ctrl+O` | 切换 transcript/detail。thinking、tool input/result、MCP 调用和 subagent 内部进度进入 verbose 路径；重复的 MCP 调用默认可压成 `Called … N times` 单行。 | `../src/keybindings/defaultBindings.ts:32-49`；[Interactive mode](https://code.claude.com/docs/en/interactive-mode)。 |
| transcript 内 `Ctrl+E` | 切换是否显示更老的消息；这是 transcript 的 show-all，不是权限解释。 | `../src/keybindings/defaultBindings.ts:161-169`、`../src/components/Messages.tsx:681-689`。 |
| 权限框内 `Ctrl+E` | 懒加载并切换 what/why/risk 解释；风险标签 Low/Med/High。 | `../src/keybindings/defaultBindings.ts:131-148`、`../src/components/permissions/PermissionExplanation.tsx:41-58`、`../src/components/permissions/PermissionExplanation.tsx:89-141`；[Permissions](https://code.claude.com/docs/en/permissions)。 |
| fullscreen 点击结果行 | 只有有额外内容的行可点；tool call 和对应 result 使用相同 key 一起展开。 | `../src/components/Messages.tsx:559-624`；[Fullscreen](https://code.claude.com/docs/en/fullscreen)。 |
| `[` | 把完整会话以 tool output 展开态写入终端原生 scrollback。 | [Fullscreen](https://code.claude.com/docs/en/fullscreen)、[Interactive mode](https://code.claude.com/docs/en/interactive-mode)。 |
| `v` | transcript viewer 中送往外部编辑器，适合终端内不便阅读的完整证据。 | [Interactive mode](https://code.claude.com/docs/en/interactive-mode)。 |
| `/focus` | fullscreen 的 quiet view：保留最后一个 prompt、工具调用单行摘要（编辑带 diffstat）和 final response；该偏好持续生效。 | [Fullscreen](https://code.claude.com/docs/en/fullscreen)。 |
| `Ctrl+B` | Bash 或 agent 运行中转后台；之后通过 `/tasks`/task ID 管理。 | [Interactive mode](https://code.claude.com/docs/en/interactive-mode)、[Subagents](https://code.claude.com/docs/en/sub-agents)。 |
| `Ctrl+T` | 切换会话任务清单，不等价于 `/tasks` 后台任务管理器。 | `../src/keybindings/defaultBindings.ts:32-49`；[Interactive mode](https://code.claude.com/docs/en/interactive-mode)。 |

### 5.2 历史窗口与渲染上限

**[源码事实]** 经典非虚拟列表在 transcript 且未 show-all 时取最近 30 条；普通非虚拟渲染上限为 200 条并按 50 条增加。fullscreen 虚拟列表走另一条路径，只渲染可见行来控制内存，而不是把历史裁成固定 30/200 条：`../src/components/Messages.tsx:276-308`、`../src/components/Messages.tsx:459-529`、[Fullscreen](https://code.claude.com/docs/en/fullscreen)。

**[源码事实]** streaming thinking 在流式期间或完成后的约 30 秒窗口中保持可见，之后依历史 thinking 规则收拢；最新直接 `!` shell 输出会自动展开：`../src/components/Messages.tsx:381-440`。

**[推断，高置信]** 这里体现了“工作记忆优先”：当前正在发生、刚发生、需要行动的内容占据默认视野；旧的可审计细节退到 transcript。

## 6. 逐工具字段与密度矩阵

### 6.1 核心工具

| 工具/命令 | 调用默认展示 | Running 默认展示 | Success 默认展示 | Error 默认展示 | 完整态额外字段 |
|---|---|---|---|---|---|
| **Bash** | 命令最多 2 行/160 display chars；在 fullscreen 可用 description/comment 替代；识别出的 `sed` edit 显示文件路径 | 尾部最多 5 行、elapsed、累计/估算行数与 bytes | stdout/stderr 走通用截行；无输出显示 `Done`/`No output`；后台时提示管理入口 | stderr/timeout/semantic error，通用错误仍可截断 | 完整命令和完整输出。`../src/tools/BashTool/UI.tsx:25-172`、`../src/components/shell/ShellProgressMessage.tsx:19-83`、`../src/tools/BashTool/BashToolResultMessage.tsx:66-189`。 |
| **Read** | 默认相对/显示路径；verbose 为完整路径 + offset/limit 行范围；PDF page 参数始终可见 | 工具头状态 | 文本行数、图片尺寸、notebook cells、PDF pages/bytes 等语义摘要 | 默认 `File not found`/`Error reading file`；verbose exact error | 完整路径、行范围和工具 result。`../src/tools/FileReadTool/UI.tsx:30-172`。 |
| **Grep** | pattern + 默认相对 path；verbose 为绝对 path | 工具头状态 | `Found N lines/files/matches`；默认不铺开每一条 match | invalid pattern/路径等语义错误 | count 细分与实际 content。`../src/tools/GrepTool/UI.tsx:15-186`。 |
| **Glob** | pattern + path；label 为 Search | 工具头状态 | 复用 Grep 搜索结果摘要 | 语义化错误 | 完整匹配列表。`../src/tools/GlobTool/UI.tsx:11-53`。 |
| **Edit** | 文件路径 | 工具头/permission diff | 常规主视图展示 Added/Removed 统计并渲染 structured diff；condensed subagent 视图可只保留统计 | read-first、not found、edit error 等语义提示 | 完整 diff/exact error。`../src/tools/FileEditTool/UI.tsx:57-153`、`../src/components/FileEditToolUpdatedMessage.tsx:45-110`。 |
| **Write(create)** | 默认显示路径，verbose 完整路径 | 工具头/permission preview | `Wrote N lines to path` + 前 10 行 + `+N lines` | rejection/error 可保留首行与 diff 线索 | 完整新文件内容。`../src/tools/FileWriteTool/UI.tsx:26-126`、`../src/tools/FileWriteTool/UI.tsx:138-182`。 |
| **Write(update)** | 同上 | 同上 | 更新已有文件时渲染完整 diff，不受 create 的 10 行预览门槛控制 | 同上 | exact error/完整路径。`../src/tools/FileWriteTool/UI.tsx:138-155`。 |
| **WebSearch** | query；verbose 增加 allowed/blocked domains | `Searching…`/`Found…` | 搜索次数 + duration；UI renderer 不直接铺开搜索结果正文 | 通用 tool error | domains；最终 renderer 仍以 count/duration 为主。`../src/tools/WebSearchTool/UI.tsx:25-91`。 |
| **WebFetch** | 默认 URL；verbose 加 prompt | `Fetching…` | bytes/status 摘要 | 通用 tool error | full result + prompt。`../src/tools/WebFetchTool/UI.tsx:9-61`。 |
| **MCP** | server/tool 身份；输入值非 verbose 最多约 80 chars | Running/progress bar/message | 内容走通用截行；无内容、图片和部分常见工具有专门紧凑格式；估算超过 10k tokens 时警告 | MCP/tool error | 完整 input/result；重复调用还可由 transcript 聚合。`../src/tools/MCPTool/UI.tsx:20-150`、`../src/tools/MCPTool/classifyForCollapse.ts:595-603`；[Interactive mode](https://code.claude.com/docs/en/interactive-mode)。 |
| **TaskStop** | tool use 行本身为空 | 不适用 | 被停止命令默认最多 2 行/160 chars，并加 `stopped` | 通用 error | 完整原命令。`../src/tools/TaskStopTool/UI.tsx:7-39`。 |

### 6.2 通用输出与聚合器

| 机制 | 默认规则 | 完整态 | 依据 |
|---|---|---|---|
| 通用文本输出 | 最多 3 个视觉行；若恰好只多 1 行，会显示 4 行以避免不必要的 `+1`；考虑终端宽度 | verbose 或特定最新 shell 上下文显示完整 | `../src/utils/terminal.ts:7-60`、`../src/components/shell/OutputLine.tsx:59-74`。 |
| 预截断性能保护 | 在渲染前按最大字符量取“above fold”，估算剩余隐藏行，避免巨型 payload 全量布局 | 不保证突破工具/模型的硬 payload 限制 | `../src/utils/terminal.ts:71-113`。 |
| 同工具调用分组 | 同一 API message 中相同工具调用达到 2 次即分组；verbose 跳过分组 | `Ctrl+O` 逐条展示 | `../src/utils/groupToolUses.ts:48-64`、`../src/utils/groupToolUses.ts:83-181`。 |
| Read/Search 折叠 | 跨 Read/Grep/Glob/MCP-read-search/Bash-search 统计 reads/searches/files；慢 Bash 运行超过约 2 秒时仍露出耗时/行数 | transcript 逐项列出一行结果与 hooks/memory | `../src/components/messages/CollapsedReadSearchContent.tsx:142-292`。 |
| 高价值外部动作 | git commit/push/PR 等 outcome 优先作为一行证据，而不只计入“若干 Bash” | transcript 展开命令与输出 | `../src/components/messages/CollapsedReadSearchContent.tsx:294-360`。 |
| Hook | PreToolUse/PostToolUse 在普通视图隐藏；transcript 显示 `N hooks ran`。其他未完成 hook 显示 `Running <event> hook(s)…` | transcript 静态摘要，避免 transient running 文案卡死 | `../src/components/messages/HookProgressMessage.tsx:34-114`。 |

## 7. Thinking 展示策略

1. **[源码事实]** 空 thinking 或 `hideInTranscript` 直接不渲染：`../src/components/messages/AssistantThinkingMessage.tsx:33-40`。
2. **[源码事实]** 默认态只显示一行 `Thinking` 和 `Ctrl+O to expand` 提示；transcript/verbose 才渲染完整 markdown：`../src/components/messages/AssistantThinkingMessage.tsx:40-84`。
3. **[源码事实]** 在 subagent context 或虚拟列表中会抑制重复的 `Ctrl+O` 提示，避免每一行都复制帮助文本：`../src/components/CtrlOToExpand.tsx:10-49`。
4. **[源码事实]** streaming thinking 属于“当前工作证据”，会在流式期间和完成后短时保留；旧 thinking 再按历史策略收拢：`../src/components/Messages.tsx:381-419`。
5. **[未知]** 本地 renderer 只能证明“收到的 thinking block 如何显示”，不能证明服务端内部推理的生成、过滤或披露边界；本文不把 UI 的“完整 thinking”表述为模型所有内部推理均可见。

## 8. Subagent 专项策略

### 8.1 两个信息平面必须区分

**父模型上下文平面。** 官方文档明确把 subagent 定位为独立 context：高容量工具输出留在 subagent 内，只把最终摘要返回主会话。这是 context isolation，不只是视觉折叠。[Subagents](https://code.claude.com/docs/en/sub-agents)

**人类可审计 UI 平面。** 本地 `AgentTool` 仍维护 prompt、progress messages、toolUseCount、tokenCount、duration 和 response；默认主视图只给近况/统计，transcript 才展开内部活动：`../src/tools/AgentTool/UI.tsx:241-307`、`../src/tools/AgentTool/UI.tsx:315-409`。

因此：**[推断，高置信]** “只返回摘要”不等于“用户永远看不到细节”。父模型拿摘要以节省 context，人类可通过 transcript/任务详情审计已保留的 subagent 轨迹。

### 8.2 Foreground / background 规则

| 场景 | 调度与展示 | 权限 | 完成/失败 |
|---|---|---|---|
| Foreground subagent | 阻塞主 agent；用户看到当前 agent 进度 | permission prompt 直接传到主会话 | 结果返回后主 agent 继续。[Subagents](https://code.claude.com/docs/en/sub-agents) |
| Background subagent | v2.1.198 起为默认；当父 agent 必须先拿到结果时改用 foreground；可由用户明确指定或 `Ctrl+B` 转后台 | v2.1.186 起 prompt 在主会话弹出并命名 subagent；Esc 只拒绝该 tool call，不停止 agent | v2.1.208 起完成项继续留在 `/tasks`；API error 时标失败并向父级带回 error 与最后输出。[Subagents](https://code.claude.com/docs/en/sub-agents) |

### 8.3 默认主行应显示什么

| 阶段 | 默认信息 | transcript/展开信息 | 源码依据 |
|---|---|---|---|
| 初始化 | agent description/type；若模型不同则显示 model tag | prompt | `../src/tools/AgentTool/UI.tsx:411-443`。 |
| 运行 | 正常宽度显示最近最多 3 条处理后的活动；窄屏退化为 `In progress… N tools · N tokens` | 全部 progress/tool messages | `../src/tools/AgentTool/UI.tsx:33-33`、`../src/tools/AgentTool/UI.tsx:95-179`、`../src/tools/AgentTool/UI.tsx:445-510`。 |
| 重复读/搜 | 合成 `Searched…`/`Read…` 类摘要，并以 `+N more tools` 表示隐藏量 | 逐工具调用和一行结果 | `../src/tools/AgentTool/UI.tsx:512-569`。 |
| 后台化 | `Backgrounded agent` + manage/expand 提示 | prompt + 已有 transcript | `../src/tools/AgentTool/UI.tsx:315-409`。 |
| 完成 | `Done (N tool uses · N tokens · duration)`；默认不重复最终长 response | prompt、内部 progress、最终 response | `../src/tools/AgentTool/UI.tsx:315-409`。 |
| reject/error | 保留最近 progress 摘要，再显示错误，而不是用错误覆盖全部执行线索 | 完整 progress/error | `../src/tools/AgentTool/UI.tsx:571-625`。 |

### 8.4 推荐给上层报告的 subagent 展示原则

以下是**[推断]**，不是官方规范原文：

- 主行稳定展示 `agent identity/description + lifecycle + 最新活动 + tool/token/time 统计`。
- 主会话默认不流式复制 subagent 全部 stdout；高容量细节留在 agent-local transcript。
- 等待用户输入/权限必须越过折叠层，明确标注“哪个 subagent 在请求”。
- 完成后默认只显示 outcome + 成本统计；错误时额外保留最后活动，帮助判断失败发生在哪一步。
- 对并发 agent 使用 task list/pill 做状态总览，把每个 agent 的完整轨迹放入可进入的 detail，而不是在主 transcript 并行刷屏。

## 9. 权限提示专项策略

### 9.1 不是统一确认框

`PermissionRequest` 按 FileEdit、FileWrite、Bash、PowerShell、WebFetch、NotebookEdit、plan、Skill、AskUserQuestion、read/search filesystem 和 fallback 工具分派不同组件：`../src/components/permissions/PermissionRequest.tsx:47-81`。

| 权限类型 | 决策前默认展示 | 可选深入信息 | 依据 |
|---|---|---|---|
| Bash | **完整 command**、description、规则触发原因；若启用则显示 destructive warning；未在 sandbox 中时标题标 `unsandboxed`；提供一次允许、保存规则、拒绝/反馈等选项 | `Ctrl+E` 生成 what/why/risk；`Ctrl+D` 是 permission debug（源码能力，不应作为普通用户主路径） | `../src/components/permissions/BashPermissionRequest/BashPermissionRequest.tsx:304-319`、`../src/components/permissions/BashPermissionRequest/BashPermissionRequest.tsx:435-480`。 |
| FileEdit | `Edit file`、相对路径/文件名、将要应用的 structured diff | IDE diff 或完整 diff 路径 | `../src/components/permissions/FileEditPermissionRequest/FileEditPermissionRequest.tsx:57-79`、`../src/components/permissions/FileEditPermissionRequest/FileEditPermissionRequest.tsx:151-161`。 |
| FileWrite | 明确 `Create file` 或 `Overwrite file`、路径/文件名、内容 diff | IDE diff | `../src/components/permissions/FileWritePermissionRequest/FileWritePermissionRequest.tsx:85-141`。 |
| WebFetch | URL/tool render、description、hostname；“不再询问”按 domain 建规则 | verbose 加 prompt | `../src/components/permissions/WebFetchPermissionRequest/WebFetchPermissionRequest.tsx:12-28`、`../src/components/permissions/WebFetchPermissionRequest/WebFetchPermissionRequest.tsx:164-229`。 |
| Read/Grep/Glob | filesystem 专用请求，围绕目标路径/规则，而非通用 JSON | 规则解释 | 分派证据：`../src/components/permissions/PermissionRequest.tsx:75-80`。 |
| 通用/MCP | user-facing tool name、用 verbose renderer 展示的 input、description 最多 3 行、MCP 标识 | 规则解释和反馈 | `../src/components/permissions/FallbackPermissionRequest.tsx:239-323`。 |

**[官方事实]** 权限规则优先级为 deny > ask > allow；读取等只读行为与 Bash/文件修改的默认授权层级不同，`/permissions` 可查看规则及来源。工具的用户显示名也可能与 canonical tool name 不同。[Permissions](https://code.claude.com/docs/en/permissions)

**[推断，高置信]** 权限界面是“完整展示”的例外：在用户做不可逆/扩大信任范围的选择前，Claude Code 宁可展示完整 command/diff/domain 和风险上下文，而不沿用普通成功结果的紧凑预算。

## 10. 后台任务与会话任务清单

### 10.1 `/tasks` 后台工作面板

- 只纳入 background tasks；若恰好一个任务，面板可直接进入其 detail：`../src/components/tasks/BackgroundTasksDialog.tsx:122-163`。
- 排序优先 running，再区分 local Bash、remote/local agent、teammate、workflow 等类型；foreground agent 不混入后台列表：`../src/components/tasks/BackgroundTasksDialog.tsx:181-208`。
- 列表标签按类型选 command、title、agent description、`@agentName`、workflow summary 等：`../src/components/tasks/BackgroundTasksDialog.tsx:492-550`。
- detail 提供 view/foreground/stop/stop all/close 等操作；shell、agent、teammate 有独立计数：`../src/components/tasks/BackgroundTasksDialog.tsx:360-414`。
- local Bash/agent 默认 activity 文本按约 40 chars 截断，agent 完成显示 `done`，未通知显示 `unread`：`../src/components/tasks/BackgroundTask.tsx:17-148`。
- 主界面 footer 在无 running task 时隐藏；有任务时压成一个 summary pill，并在需要时显示 `↓ to view`：`../src/components/tasks/BackgroundTaskStatus.tsx:192-233`。

### 10.2 `Ctrl+T` 会话任务清单

- 官方文档把它定义为 Claude 的 task checklist，并明确区别于 `/tasks` 的后台 shells/subagents。[Interactive mode](https://code.claude.com/docs/en/interactive-mode)
- 本地 v2 面板根据终端高度显示 0 或 3-10 个可见任务；超过预算时优先 recent completed（30 秒）、in progress、pending、older completed，并把隐藏量汇总为 `+N in progress/pending/completed`：`../src/components/TaskListV2.tsx:21-48`、`../src/components/TaskListV2.tsx:135-189`。
- standalone 视图显示总数、done/in progress/open；每行显示状态、subject、owner、blocker，in-progress 且未阻塞时可再显示最近 activity：`../src/components/TaskListV2.tsx:191-210`、`../src/components/TaskListV2.tsx:242-370`。

**[推断，高置信]** 两个 task 面服务不同问题：`Ctrl+T` 回答“计划完成到哪一步”，`/tasks` 回答“哪些异步执行单元还在跑、需要输入或可管理”。把两者合并会混淆计划状态与进程状态。

## 11. 可访问性与终端表面

1. **[官方事实]** v2.1.208 新增 opt-in screen reader plain-text rendering，可由 `claude --ax-screen-reader`、`CLAUDE_AX_SCREEN_READER=1` 或设置 `axScreenReader: true` 开启。[Changelog 2.1.208](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)
2. **[官方事实]** v2.1.200 相关更新还包括隐藏装饰性 glyph、为 transcript 符号提供短文本标签、将嵌套表格改为更适合朗读的 `Header: value` 形式。[Claude Code changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)
3. **[官方事实]** fullscreen 只渲染可见消息、支持鼠标点击折叠行；滚动离开底部后暂停 auto-follow，并用 `Jump to bottom`/新消息计数提示恢复位置。[Fullscreen](https://code.claude.com/docs/en/fullscreen)
4. **[源码事实]** 本地核心快捷键通过 action 名解析而非把帮助文案散落在各 renderer 中，因此重绑定后的提示可以使用实际 chord；例如 transcript、todo、permission explanation 都注册在 keybinding 表中：`../src/keybindings/defaultBindings.ts:32-49`、`../src/keybindings/defaultBindings.ts:131-169`。
5. **[未知]** 本地源码搜索未找到 `axScreenReader`/screen-reader renderer，说明该能力可能位于未包含的 Ink/runtime 层，或本地基线与 2.1.208 public tree 不完全一致；可访问性细节只能引用官方 changelog，不能从此基线补充实现级结论。

## 12. 关键阈值清单

| 对象 | 默认阈值/预算 | 依据 |
|---|---:|---|
| 通用 tool output | 3 个视觉行；仅多 1 行时可显示 4 行 | `../src/utils/terminal.ts:7-60` |
| Bash 调用命令 | 2 行 / 160 display chars | `../src/tools/BashTool/UI.tsx:25-129` |
| Bash running output | 最近 5 行 | `../src/components/shell/ShellProgressMessage.tsx:19-83` |
| 通用 tool error | 默认最多 10 行 | `../src/components/FallbackToolUseErrorMessage.tsx:11-86` |
| API error text | 非 verbose 最多 1000 chars | `../src/components/messages/SystemAPIErrorMessage.tsx:58-106`、`../src/components/messages/AssistantTextMessage.tsx:199-200` |
| API retry error 可见性 | 第 4 次起显示 | `../src/components/messages/SystemAPIErrorMessage.tsx:10-39` |
| Write create preview | 10 行 | `../src/tools/FileWriteTool/UI.tsx:26-126` |
| Agent recent progress | 3 条 | `../src/tools/AgentTool/UI.tsx:33-33`、`../src/tools/AgentTool/UI.tsx:445-510` |
| MCP input value | 非 verbose 约 80 chars | `../src/tools/MCPTool/UI.tsx:20-55` |
| MCP 大结果警告 | 估算 10k tokens | `../src/tools/MCPTool/UI.tsx:20-55` |
| 经典 transcript 初始历史 | 30 messages | `../src/components/Messages.tsx:276-308` |
| 普通非虚拟历史 | 200 messages，步长 50 | `../src/components/Messages.tsx:276-308` |
| TaskList v2 visible rows | 矮终端 0；否则 3-10 | `../src/components/TaskListV2.tsx:21-48` |
| recent completed task 优先窗口 | 30 秒 | `../src/components/TaskListV2.tsx:21-21`、`../src/components/TaskListV2.tsx:135-164` |
| Markdown table（官方 2.1.208） | 超过 200 行只显示前 200 + `N more rows` | [Changelog 2.1.208](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) |
| Background Bash output files | 会话退出清理；单任务 5 GB cap | [Interactive mode](https://code.claude.com/docs/en/interactive-mode) |

## 13. 证据留存模型

| 展示对象 | 默认表面 | 深入表面 | 留存位置 | 何时可能真的不可恢复 |
|---|---|---|---|---|
| 主工具调用 | 单行/紧凑结果 | transcript、fullscreen 局部展开、`[` dump | 主 conversation messages/tool results | compaction、持久化裁剪、上游 result 本身已截断；这些不由 renderer 决定。 |
| Bash running/background | 尾部行或 pill | `/tasks` detail、Read output file | task state + output file | 会话退出清理、5 GB cap 或任务清理。[Interactive mode](https://code.claude.com/docs/en/interactive-mode) |
| Subagent | 最近活动/Done 统计 | Agent transcript、`/tasks`/attach | subagent 独立 context/task state | agent cleanup、API 无任何 partial output；官方错误语义见 [Subagents](https://code.claude.com/docs/en/sub-agents)。 |
| Task checklist | 高优先级 subset + hidden summary | standalone/full task list | task store | 用户删除/reset/会话生命周期；task persistence 穿过 compaction 的行为由官方 [Interactive mode](https://code.claude.com/docs/en/interactive-mode) 说明。 |
| Permission | 完整决策 payload | explanation/debug/IDE diff | confirm state + permission rule/update | 用户拒绝后工具未执行，因此没有 success result；拒绝反馈仍可传回模型。`../src/components/permissions/PermissionRequest.tsx:103-126`。 |

## 14. 版本与表面不确定性

1. **版本映射未知。** 本地 commit hash 与公开 `2.1.209` 无可验证映射；报告把官方当前行为与本地 renderer 证据并列，不声称二者是同一 binary。
2. **Fullscreen 是 research preview。** `/tui fullscreen` 的点击、虚拟滚动、`/focus`、sticky footer 与 classic 原生 scrollback 不完全相同；实现策略必须按 surface 分支。[Fullscreen](https://code.claude.com/docs/en/fullscreen)
3. **Feature flags。** 本地存在 `KAIROS`、`BASH_CLASSIFIER`、`REVIEW_ARTIFACT`、`WORKFLOW_SCRIPTS`、`MONITOR_TOOL` 等条件分支；external build 未必启用所有 UI：例如 `../src/keybindings/defaultBindings.ts:45-48`、`../src/components/permissions/PermissionRequest.tsx:35-41`。
4. **终端能力。** 宽度、行数、VT/kitty protocol、mouse 支持会改变快捷键、截断和可点击体验；TaskList 本身直接按 `rows/columns` 计算：`../src/components/TaskListV2.tsx:37-48`。
5. **局部展开范围。** fullscreen 点击只对 renderer 声明 `isResultTruncated` 或已折叠的 read/search/advisor/success result 生效，并非任意行都可点击：`../src/components/Messages.tsx:559-594`。
6. **完整不等于无限。** `Ctrl+O` 可以绕过多数 UI 软截断，但无法恢复 API 未返回、工具硬裁剪、已清理 output file 或 compaction 真正移除的数据。
7. **Direct observation 缺口。** 本任务未用真实 terminal、screen reader、窄屏、mouse 和多个并发 background agents 做交互复现；视觉符号、颜色对比、动画与 rebind 后实际提示仍需单独走查。

## 15. 对展示策略设计的可复用结论

以下均为**[推断]**，用于父任务后续整合 Claude Code 与 Codex，不冒充 Anthropic 官方规范：

1. **先状态、后体积。** permission/error/waiting/running 是必须穿透折叠层的状态；只有稳定 success 才以体积预算为主。
2. **每个命令定义最小完成证据。** Bash 是 exit/output，Read 是 path + lines/pages，search 是 pattern + match count，edit 是 path + diffstat/diff，network 是 URL/domain + status/bytes，agent 是 outcome + tool/token/time。
3. **默认只展示推进当前决策所需的信息。** 历史审计证据进入 transcript/detail，不在主视图重复刷屏。
4. **聚合必须保留可解释计数。** 不能只写“若干操作”；至少要有类别、数量、当前/最后活动和展开入口。
5. **错误不能只剩一个红点。** 默认语义摘要要回答“哪类失败”，展开态保留 exact error 和最后成功活动。
6. **subagent 要双平面。** 模型上下文只收 summary，人类 UI 仍保留可进入的 agent-local transcript；权限请求明确请求者。
7. **折叠不是删除。** UI、conversation context、task state、output file、persistent transcript 必须作为不同 retention 层单独设计和说明。
8. **可访问性需要替代表达，不只是去颜色。** 状态必须有文本标签；装饰符号、表格、动画与点击操作要有 plain-text/keyboard 等价路径。[Changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)

## 16. 最小后续验证清单

这些是减少未知项的只读/观察性 probe，不是实现建议：

1. 在 Claude Code 2.1.209 分别以 classic、`/tui fullscreen`、`--ax-screen-reader` 录制同一脚本：Bash success/error/background、Read/Grep group、Edit permission、MCP repeated call、foreground/background subagent。
2. 用 40/80/160 columns 与 10/24/60 rows 验证源码阈值和布局降级。
3. 对每个样例分别采集默认、`Ctrl+O`、transcript `Ctrl+E`、fullscreen click、`[` dump，确认“隐藏但保留”与“真正被裁剪”的边界。
4. 让 background subagent 触发 permission、API partial failure、completed/unread/cleanup，验证 v2.1.186/198/208 的版本化行为。

在完成这些 probe 前，本文对**决策架构、源码阈值和官方交互契约置信度高**；对**具体 2.1.209 binary 的视觉表现、screen-reader 文案和 feature-flag 组合置信度中等或未知**。
