# Codex 信息展示策略证据地图

> 调查日期：2026-07-15（Asia/Shanghai）
>
> 范围：Codex CLI、ChatGPT desktop app 中的 Codex、Codex IDE extension、`codex exec`。
>
> 结论性质：产品事实、CLI 当前开源实现和本项目建议严格分层；App/IDE 的未公开渲染细节不作反推。

## 1. 执行摘要

Codex 的展示策略不是一个统一的“超过 N 行就折叠”规则，而是由四个维度共同决定：

1. **语义角色**：最终答复、用户决策请求、错误必须留在主路径；探索、工具输出和子代理中间过程优先摘要化。
2. **风险与可行动性**：审批会展示动作、原因、权限边界和决策范围；普通进度只保留状态、耗时和中断入口。
3. **信息体积**：CLI 对普通命令输出和 MCP 结果使用固定预算；完整命令记录通过 transcript 或专用视图检查。
4. **交互表面**：App/IDE 使用面板、任务、子线程和通知分层；CLI 使用主 transcript、`Ctrl+T`、斜杠命令和覆盖层；`exec` 使用 `stderr`/`stdout`/JSONL 通道分离。

Codex 官方对子代理策略给出了最明确的设计理由：主线程保留需求、决策和最终产物，将探索笔记、测试日志、堆栈和命令输出移到子线程，并只把摘要返回主线程，以降低 context pollution/rot。[M-MA]

CLI 当前开源实现把这套原则落实为**类型驱动的静态策略**：

- 连续读文件、列目录、搜索命令合并为一个 `Explored` 单元，只显示 `Read`/`List`/`Search` 与目标；`Ctrl+T` transcript 才显示每条完整命令、格式化输出、退出状态和耗时。[S-EXEC]
- 普通代理命令在主视图最多展示 5 个屏幕行，用户自己发起的 shell 命令最多 50 个屏幕行；保留头尾，中间显示省略行数和 transcript 入口。[S-OUTPUT]
- 推理默认显示 summary 事件、默认不显示 raw reasoning；某些没有展示标题的 reasoning block 只进入 transcript。[M-REASON][S-REASON]
- 文件修改事件在 transcript 中只显示文件级摘要，完整 Git 差异由 `/diff` 的无输出上限专用覆盖层展示；App 则用 review pane，并允许逐文件展开/折叠。[S-PATCH][S-DIFF][M-REVIEW]
- 子代理主线程行只显示 spawn/send/wait/close、代理标签、有限 prompt/result preview；完整工作通过 `/agent` 切换到代理线程查看。[S-AGENT][M-MA]

**关键边界**：App 和 IDE 的公开文档没有披露普通工具卡片的默认展开状态、输出行阈值、推理卡片折叠阈值、错误堆栈截断阈值或自动展开算法。因此本报告不能把 CLI 的 5/50 行规则外推到 App/IDE；API/app-server 事件模型也不能作为某个 UI 如何渲染的证据。[GAPS]

## 2. 证据等级与术语

| 标记 | 含义 | 可以证明什么 | 不能证明什么 |
| --- | --- | --- | --- |
| `M` | 2026-07-15 获取的当前 Codex manual | 已发布产品行为、跨表面能力、用户控制 | 私有 rollout、未写入文档的具体像素/折叠阈值 |
| `S` | OpenAI `openai/codex` 提交 `f90e7de` | 该提交中 CLI/`exec` 的实际实现 | App/IDE 私有客户端行为；未来版本稳定合同 |
| `P` | 官方单页文档 | 该页面当前公开的命令或控制 | 页面未描述的渲染启发式 |
| `R` | 本报告建议 | 可供本 Go 项目实现参考 | 不是 Codex 产品事实 |
| `U` | Undocumented，公开证据缺口 | 当前无法可靠断言 | 不代表功能不存在 |

本文采用以下展示状态：

- **完整展示**：内容直接留在主对话/主输出，不需要额外操作才能阅读。
- **摘要展示**：显示类型、对象、状态、关键结果或头尾，隐藏大体积正文。
- **独立视图**：主路径显示入口，完整内容在 transcript、review pane、agent thread、terminal 或 pager 中。
- **通道分流**：内容仍完整输出，但被送到 `stderr`、`stdout` 或 JSONL，不在同一人类视图混排。
- **不可公开确认**：文档只证明事件或能力存在，没有证明具体 UI 展开状态。

## 3. Codex 的展示决策模型

### 3.1 主路径只保留“决策带宽”

官方明确把 exploration notes、test logs、stack traces、command output 定义为会污染主上下文的中间噪声；子代理的作用之一就是把这些内容移出主线程，返回 distilled summaries。主代理应聚焦 requirements、decisions 和 final outputs。[M-MA]

因此 Codex 的一级信息优先级可以从公开行为归纳为：

| 优先级 | 信息 | 默认处置 | 官方证据 |
| --- | --- | --- | --- |
| P0 | 需要用户选择的审批、权限、问题 | 阻塞式显示；包含决策所需上下文 | [M-APPROVAL][S-APPROVAL] |
| P0 | 最终答复、失败、不可恢复错误 | 留在主 transcript/最终输出 | [M-EXEC][S-ERROR] |
| P1 | 当前工作状态、耗时、中断入口 | 单行或少量行，动态更新 | [S-STATUS] |
| P1 | 文件改动与 review findings | 文件摘要 + 专用 diff/review 视图 | [M-REVIEW][S-DIFF] |
| P2 | 推理 | reasoning summary；raw reasoning 默认关闭 | [M-REASON][S-REASON] |
| P2 | shell/MCP/web/tool 活动 | 动作、目标、状态、短结果；大输出收拢 | [S-EXEC][S-MCP][S-SEARCH] |
| P2 | 子代理活动 | 生命周期与短摘要；完整线程按需打开 | [M-MA][S-AGENT] |

这张优先级表是对官方文档和源码的一致性归纳，不是 OpenAI 公布的正式优先级枚举。

### 3.2 生命周期决定“动态替换”还是“永久留痕”

CLI 的运行中状态是 composer 上方的一行 status row，默认标题为 `Working`，含耗时和 `Esc` 中断提示；详情默认最多 3 行，超出用省略号。这样能持续反馈进展而不让每次心跳都写入 transcript。[S-STATUS]

完成后，命令、计划、错误、文件摘要等转换成历史单元；进度 spinner 和临时详情不需要全部永久保存。计划更新则作为 `Updated Plan` 历史单元完整列出步骤状态，完成项、进行中项和待办项使用不同样式。[S-PLAN]

### 3.3 风险越高，展示越接近原始动作

CLI 审批覆盖层会按请求类型显示：来源线程、环境、理由、权限规则、完整命令或 MCP server/message，并提供一次批准、会话/前缀范围批准、拒绝等选项；大 payload 有全屏入口，来自非当前代理线程的请求可按 `o` 打开来源线程。[M-MA][S-APPROVAL]

这和普通命令输出的策略相反：普通输出可以省略中段，但审批不得只显示“某命令请求权限”而省略实际命令与边界。

### 3.4 内容类型优先于纯长度

CLI 并非所有内容都使用同一个行数上限：

- 探索型命令先按语义解析并合并，而不是先按字符数截断。[S-EXEC]
- 普通代理命令主视图为 5 屏幕行，用户 shell 为 50 屏幕行。[S-OUTPUT]
- MCP 文本结果按 5 行乘终端宽度估算 grapheme 预算；合法 JSON 先紧凑格式化再截断。[S-MCP]
- 子代理 prompt/error/result preview 分别限制为 160/160/240 graphemes；`/agent` 状态预览最多读取 6 个近期 item，最终只显示 3 个屏幕行。[S-AGENT]
- status details 默认 3 行；并发 auto-review 详情最多列 3 条，再显示 `+N more`。[S-STATUS]
- `/diff` 明确绕开通用后台命令输出上限，使用独立 pager 显示完整 payload。[S-DIFF]

## 4. 跨表面展示状态矩阵

### 4.1 核心信息类型

| 信息类型 | CLI（交互 TUI） | Desktop app | IDE extension | `codex exec` 人类输出 | `codex exec --json` |
| --- | --- | --- | --- | --- | --- |
| 最终答复 | 主 transcript 完整展示；`/copy`/`Ctrl+O` 复制最近完成输出。[M-CLI] | 任务 transcript；可在任务内查找文本，公开文档未说明自动折叠最终答复。[M-APP] | 当前 chat transcript；review 可内联或 detached。[M-IDE][M-REVIEW] | 管道场景最终消息写 `stdout`，进度写 `stderr`；TTY 场景当前实现避免重复打印。[M-EXEC][S-EXEC-HUMAN] | `item.completed/agent_message`，最终 turn 有 `turn.completed` 和 usage。[M-EXEC][S-EXEC-JSON] |
| 推理 | 默认显示 summary、raw 默认关闭；headerless summary 可只进 `Ctrl+T` transcript。[M-REASON][S-REASON] | `U`：文档没有公开 reasoning card 展开/折叠规则。只证明可选择 reasoning effort。[M-APP-SET] | `U`：文档证明可选 reasoning effort，未证明推理文本如何收拢。[M-IDE] | summary 默认可写 `stderr`；`hide_agent_reasoning` 可隐藏，raw 只在显式配置后使用。[S-EXEC-HUMAN] | 独立 `reasoning` item；这是输出协议本身，不是 UI 证明。[M-EXEC][S-EXEC-JSON] |
| 读/列/搜探索 | 连续探索命令合并为 `Exploring/Explored`，只列动作与目标；transcript 展开每条命令、输出、exit/duration。[S-EXEC] | `U`：可看到任务活动，但无公开分组阈值。[GAPS] | `U`：可看到任务活动，但无公开分组阈值。[GAPS] | 每条 command start/completion 写 `stderr`，完成时可打印完整 aggregated output。[S-EXEC-HUMAN] | 每条 command 有 started/updated/completed item 和完整字段。[S-EXEC-JSON] |
| 普通 shell 命令 | 标题含 Running/Ran、命令、成功/失败；主视图输出保留头尾，普通代理命令 5 屏幕行。[S-OUTPUT] | `U`：文档未公开 agent command card 的行数或默认展开状态；集成 terminal 是独立完整输出面。[M-TERMINAL][GAPS] | `U`：文档未公开行数或默认展开状态。[GAPS] | `stderr` 显示命令、cwd、状态、duration、exit code 和非空 aggregated output。[S-EXEC-HUMAN] | `command_execution` 含 command、aggregated_output、exit_code、status。[S-EXEC-JSON] |
| 用户 shell 命令 | 标记 `You ran`，当前实现主视图最多 50 屏幕行，仍保留 transcript 入口。[S-OUTPUT] | 集成 terminal 独立于任务 transcript，可清屏；ChatGPT 可读取当前 terminal output。[M-TERMINAL] | 编辑器 terminal 行为不属于 Codex 展示合同；无公开 Codex 截断规则。[GAPS] | 不适用为交互式用户 shell 表面。 | 不适用。 |
| MCP/动态工具 | MCP 显示 server/tool、arguments、状态和短结果；文本结果约 5 行预算，错误显式显示；图像结果用独立提示。[S-MCP] | `U`：文档仅证明 MCP status/connected servers 和审批能力，不说明每种 result card 的展开策略。[P-APP-SLASH][M-APPROVAL] | 同 App：`/mcp` 可看 connected servers，普通结果卡策略未公开。[M-IDE][GAPS] | MCP start/completion 只显示 server/tool/status，失败时显示 error；成功 result body 当前人类 renderer 不打印。[S-EXEC-HUMAN] | `mcp_tool_call` item 含 arguments、result、error、status。[S-EXEC-JSON] |
| Web search | 单行 `Searching/Searched the web` + query/open/find detail；多 query 只预览首项加 `...`。[S-SEARCH] | `U`：未公开 search card 的折叠阈值。[GAPS] | `U`：未公开 search card 的折叠阈值。[GAPS] | `stderr` 显示 `web search:` 与 query。[S-EXEC-HUMAN] | `web_search` item 含 query/action。[S-EXEC-JSON] |
| 文件修改 | 每次 patch 先显示文件级 summary；失败显示短 stderr；不把完整 patch 混进主 transcript。[S-PATCH] | review pane 显示整个 repo 当前状态，可切 Unstaged/Staged/Commit/Branch/Last turn；文件 diff 可折叠。[M-REVIEW] | review 在当前 task 或 detached task；文档未说明逐文件折叠规则。[M-IDE][M-REVIEW] | patch completion 显示 status 和路径；turn diff update 可打印 diff 到 `stderr`。[S-EXEC-HUMAN] | `file_change` 列 path/kind/status；注意该 item 不是完整 diff。[S-EXEC-JSON] |
| 完整 diff/review | `/diff` 独立全屏 pager，包含 untracked files，显式取消输出 cap；`/review` 产生 transcript turn。[M-CLI][S-DIFF][M-REVIEW] | review pane 是完整审阅面；逐文件背景点击展开/折叠，并支持行评论、stage/revert 到 file/hunk。[M-REVIEW] | `/review` 可选 base/uncommitted；结果在当前 task 或 detached task，具体 pane 折叠细节未公开。[M-IDE][M-REVIEW] | `codex review` 为非交互 review，结果遵循最终输出通道。[M-CLI][M-EXEC] | review 产生的 agent/tool/file events 逐条输出；不要把 schema 当作 App/IDE UI 证据。[S-EXEC-JSON] |
| 计划 | Proposed Plan 和 Updated Plan 作为完整历史单元；步骤逐项显示状态。[S-PLAN] | `/plan` 进入规划模式；未公开大型 plan 的自动折叠阈值。[P-APP-SLASH][GAPS] | `/plan` 进入规划模式；未公开大型 plan 的自动折叠阈值。[M-IDE][GAPS] | plan explanation 和全部步骤以状态符号写 `stderr`。[S-EXEC-HUMAN] | todo list/plan update 为 item 生命周期事件。[M-EXEC][S-EXEC-JSON] |
| 运行进度 | composer 上方单行状态含状态、耗时、中断；详情默认最多 3 行；footer/title 可配置 model/context/limits/git/tokens/session/task progress。[M-CLI][S-STATUS] | Goal progress row 位于 composer 上方，可 pause/resume/edit/clear；通知和 pet 仅提示 attention state。[M-GOAL][M-NOTIFY] | Goal 在当前 workspace chat 中持续；background-agent panel 可显示 agent status。[M-GOAL][M-MA] | 所有进度写 `stderr`，最终答复与管道数据分离。[M-EXEC] | started/updated/completed event 保留逐项生命周期。[M-EXEC][S-EXEC-JSON] |
| 审批 | modal 显示来源、reason、command/permission，支持一次/范围批准与拒绝；大 payload 可全屏，非当前 agent 可开来源线程。[M-APPROVAL][S-APPROVAL] | composer 下方权限模式；auto-review item 显示 Reviewing/Approved/Denied/Aborted/Timed out，可含风险和授权评估。[M-APPROVAL] | composer 下方权限控制；公开文档未规定 approval card 细节。[M-APPROVAL][GAPS] | 不能新弹交互审批；需要新批准的动作失败并回传 parent workflow。[M-MA] | 无交互输入通道；error/failed item 表达失败。[S-EXEC-JSON] |
| 错误/警告 | 错误作为红色历史单元保留；warning 使用醒目标记；retry error 可更新临时状态而非重复永久刷屏。[S-ERROR] | `U`：文档没有统一错误卡截断合同；auto-review timeout/denial 有明确状态。[M-APPROVAL][GAPS] | `U`：文档没有统一错误卡截断合同。[GAPS] | `ERROR:`/`warning:` 写 `stderr`；turn failed/interrupted 不冒充最终答复。[S-EXEC-HUMAN] | 可恢复 item error 与不可恢复 top-level `error`/`turn.failed` 分开。[S-EXEC-JSON] |
| 子代理 | 主线程记录 spawn/send/wait/close 和短 preview；`/agent`/`/subagents` 或快捷键切完整线程；来源审批可跨线程浮出。[M-MA][S-AGENT] | 每个 subagent thread 可检查，主任务收到 summary；具体 activity card 折叠阈值未公开。[M-MA][GAPS] | active subagents 在 composer 上方 panel；展开看 status、stop all、打开单线程。[M-MA] | 人类 renderer 至少显示 collab tool start；新审批无法交互时失败。[S-EXEC-HUMAN][M-MA] | `collab_tool_call` 含 sender/receiver/prompt/agents_states/status。[S-EXEC-JSON] |
| 通知 | 可配置 TUI 通知条件和 turn completion 外部程序；不是 transcript 替代品。[M-NOTIFY] | completion、permission、question 通知可独立配置；pet 只显示 Running/Needs input/Ready/Blocked。[M-NOTIFY] | 官方通知章节未列 IDE 独立通知通道。[M-NOTIFY] | 由调用者/CI 接管 stderr、exit code 和 artifacts。 | 由消费者按 event 类型决定通知。 |

### 4.2 “可检查”不等于“完整保留”

需要特别区分三类入口：

1. **真正的完整记录**：CLI `ExecCell::transcript_lines` 对命令展示每次 call、格式化输出、退出状态和耗时；`/diff` 明确以 uncapped payload 进入 pager。[S-EXEC][S-DIFF]
2. **更适合复制的表现**：`/raw` 只切换 copy-friendly raw scrollback；它不是“解除所有业务层截断”。例如 MCP `raw_lines` 仍复用 5 行结果格式化逻辑，因此不能承诺 `/raw` 会恢复 MCP 的完整原始结果。[M-CLI][S-MCP]
3. **机器完整性**：`codex exec --json` 输出每个事件和 item payload，适合程序消费；它是 `exec` 自己的公开输出，不证明 App/IDE 对同样事件如何折叠。[M-EXEC][S-EXEC-JSON]

## 5. 子代理展示策略

### 5.1 官方跨表面合同

- App：每个子代理 thread 均可打开检查，主任务接收其 summary。[M-MA]
- CLI：`/agent` 在 active agent threads 之间切换，主线程收集结果后给 consolidated final response。[M-MA]
- IDE：background-agent UI 可用时，active subagents 位于 composer 上方；panel 可展开查看 status、stop all 或打开单个 thread。[M-MA]
- 所有本地表面：子代理活动会出现于 App、CLI、IDE；可用 nickname 仅改变展示标签，不改变底层 agent type。[M-MA]
- 审批：CLI 即使当前查看主线程，也可显示 inactive agent 的 approval；overlay 标注来源线程并允许先打开线程。非交互流程不能弹新审批，动作失败并回传父工作流。[M-MA]

### 5.2 CLI 当前实现的三层信息密度

| 层级 | 展示内容 | 限制/规则 | 完整内容入口 | 证据 |
| --- | --- | --- | --- | --- |
| 主线程事件行 | Started/Interacted/Interrupted；Spawned/Sent input/Waiting/Finished waiting/Closed；nickname/role/model/reasoning | spawn prompt 160 graphemes；error 160；completed response 240；空值不展示 | `/agent` 打开 thread | [S-AGENT] |
| `/agent` 状态摘要 | 每个运行子代理 path + 最近活动 | 从最近 6 个 unique item 取摘要，最终只保留 3 个屏幕行；命令/消息各自最多 240 graphemes | 在 picker 选择代理 | [S-AGENT] |
| 子代理 thread | 该代理自己的完整 transcript、命令和工具单元 | 使用与普通线程相同的展示/收拢规则 | `Ctrl+T`、`/diff` 等线程内入口 | [M-MA][S-EXEC] |

### 5.3 子代理主线程应该显示什么（Codex 事实的抽象）

主线程至少需要：

- **身份**：nickname/path/role，避免并发同类代理无法区分。[M-MA][S-AGENT]
- **动作**：spawn、send/steer、wait、interrupt/close。[S-AGENT]
- **状态**：pending/running/completed/interrupted/error/shutdown/not found。[S-AGENT]
- **短结果**：completed/error 的 preview，不复制整段子线程日志。[S-AGENT]
- **检查入口**：打开单独 thread；审批还要显示来源并能跳转。[M-MA][S-APPROVAL]

主线程不应默认复制：子代理的逐条探索、完整测试日志、完整命令输出、重复 reasoning summary。这个结论是官方 subagent 文档直接给出的设计目标，不只是实现偏好。[M-MA]

### 5.4 公开缺口

App/IDE 没有公开以下合同：

- 一个 activity card 默认何时折叠/展开；
- completed subagent 的卡片保留多久；
- summary preview 的字符或行阈值；
- 多个 subagent status 是否按活跃度、创建顺序或完成顺序排列；
- thread 内大型 tool result 是否存在与 CLI 相同的 5/50 行规则。

因此不得从 CLI 源码或 app-server schema外推这些行为。[GAPS]

## 6. 用户检查、展开与导航控制目录

### 6.1 CLI：直接影响证据可见性的命令

| 控制 | 向用户展示什么 | 展示层级/用途 | 证据 |
| --- | --- | --- | --- |
| `Ctrl+T` | transcript overlay；命令单元可显示每次完整 command、formatted output、exit/duration | 从摘要主视图进入详细记录 | [M-KEYMAP][S-EXEC] |
| `/raw` | copy-friendly raw scrollback | 去除富格式便于选择/复制；不保证解除业务层截断 | [M-CLI][S-MCP] |
| `/copy` / `Ctrl+O` | 最近完成的 Codex response 或 plan | 只复制最终内容，不复制整条工具轨迹 | [M-CLI] |
| `/diff` | 当前 Git diff，含 untracked files；空时显示 No changes detected | 独立全屏 pager；payload 不设 cap | [M-CLI][S-DIFF] |
| `/review` | 工作树/base/commit/custom scope 的 prioritized findings | review 作为 transcript turn；不改工作树 | [M-REVIEW] |
| `/agent`, `/subagents` | agent picker/状态，选择后切换完整 agent thread | 子代理证据导航 | [M-MA][M-CLI][S-AGENT] |
| `/ps` | background terminals 与 recent output | 不离开主 transcript 检查长命令 | [M-CLI] |
| `/status` | model、approval policy、writable roots、context capacity、session 配置/usage | 当前会话摘要 | [M-CLI] |
| `/usage` | daily/weekly/cumulative token activity 或 reset | 账户消耗视图 | [M-CLI] |
| `/mcp [verbose]` | configured MCP tools；verbose 增加 server details | 外部工具能力与连接状态 | [M-CLI] |
| `/debug-config` | config layers、requirements、policy/network diagnostics | 配置证据视图 | [M-CLI] |
| `/statusline` | 可选 footer 字段及排序：model/context/limits/git/tokens/session 等 | 持续低密度状态 | [M-CLI] |
| `/title` | project/status/thread/branch/model/task progress 等 terminal title 字段 | 窗口级状态 | [M-CLI] |
| `/compact` | 将可见 conversation 摘要化以释放 context | 改变后续模型上下文，不是展开控件 | [M-CLI] |
| `/archive` | 从活跃列表收起 session，但保留 transcript | 信息生命周期管理 | [M-CLI] |
| `/delete` | 永久删除 transcript 和 descendant sessions | 不可逆，不是普通收拢 | [M-CLI] |
| `/feedback` | feedback dialog/发送日志 | 诊断提交，不是证据展开 | [M-CLI] |

CLI 还允许用 `tui.keymap.global.open_transcript` 重映射 transcript 快捷键；默认示例为 `Ctrl+T`。`tui.alternate_screen` 可改为保留终端 scrollback，`status_line=[]` 可隐藏 footer。[M-KEYMAP]

### 6.2 App：直接影响证据可见性的控制

| 控制 | 展示内容 | 证据 |
| --- | --- | --- |
| `Cmd/Ctrl+F` | 当前 task 内查找文本 | [M-APP] |
| `Cmd/Ctrl+G` | 搜索并重开历史 task；扩展匹配可包含 task content 和 Git branch | [M-APP] |
| Review panel | repo 状态、Unstaged/Staged/Commit/Branch/Last turn；按文件展开/折叠；行评论；stage/revert | [M-REVIEW] |
| Integrated terminal | 当前 project/worktree 的 terminal output；ChatGPT 可读取 | [M-TERMINAL] |
| Goal progress row | pause/resume/edit/clear，位于 composer 上方 | [M-GOAL] |
| Subagent thread | 打开单个 agent 的工作和结果 | [M-MA] |
| Completion/permission/question notifications | 只提示需要注意的状态；不替代 task transcript | [M-NOTIFY] |
| `Cmd/Ctrl+B`, `Cmd/Ctrl+Alt+B`, `Cmd/Ctrl+J` | sidebar、review panel、bottom panel 的显示/隐藏 | [M-APP] |

App 当前官方 slash commands 包括：`/approve`、`/cloud`、`/cloud-environment`、`/compact`、`/fast`、`/feedback`、`/fork`、`/goal`、`/ide-context`、`/init`、`/local`、`/mcp`、`/memories`、`/model`、`/pet`、`/personality`、`/plan`、`/project`、`/reasoning`、`/review`、`/side`、`/status`、`/task`、`/worktree`。这些命令可因环境和账户权限而变化；skill 和 custom prompt 也可进入 slash list。[P-APP-SLASH]

其中和展示最相关的输出合同是：

- `/status`：task ID、context usage、rate limits；
- `/mcp`：connected servers 状态；
- `/review`：进入专用 review flow；
- `/compact`：压缩 context，而不是简单隐藏 UI；
- `/side`：临时侧对话，不中断 main task；
- `/goal`：以 progress row 显示长期目标状态。[P-APP-SLASH][M-GOAL]

### 6.3 IDE extension：直接影响证据可见性的控制

IDE slash commands 当前包括 `/approve`、`/cloud`、`/cloud-environment`、`/compact`、`/fast`、`/feedback`、`/fork`、`/goal`、`/ide-context`、`/init`、`/local`、`/mcp`、`/memories`、`/model`、`/personality`、`/plan`、`/project`、`/reasoning`、`/review`、`/side`、`/status`、`/worktree`。[M-IDE]

IDE 特有的展示控制：

- `chatgpt.reviewDelivery=inline|detached` 决定 review 留在当前 task 还是新 task；[M-IDE]
- `chat.fontSize` 控制 conversation/composer，`chat.editor.fontSize` 控制 code snippets 和 diffs；[M-IDE]
- Command Palette 可打开 sidebar/new panel，selected range 或 entire file 可作为显式 context 加入当前 task；[M-IDE]
- background-agent panel 可展开检查 status、stop all 或打开单个 agent thread。[M-MA]

IDE 文档没有公开 `Ctrl+T` 等同的 transcript overlay，也没有公开 CLI `/raw` 的等价控制；不可假设存在。[GAPS]

### 6.4 `codex exec`：输出选择即展示策略

| 方式 | `stdout` | `stderr` | 适用场景 | 证据 |
| --- | --- | --- | --- | --- |
| 默认 human mode | 管道场景只放最终 agent message | config、prompt、reasoning summary、tool/command progress、diff/plan、warning/error、usage | shell pipeline 和人类日志 | [M-EXEC][S-EXEC-HUMAN] |
| `--json` | JSONL 全事件流 | 诊断仍由进程日志设置控制 | 机器消费、审计、UI adapter | [M-EXEC][S-EXEC-JSON] |
| `-o/--output-last-message` | 最终消息仍写 `stdout` | 正常进度 | 同时持久化最终结果 | [M-EXEC] |
| `--output-schema` | schema-conformant 最终 JSON | 正常进度 | 稳定下游字段 | [M-EXEC] |
| `RUST_LOG` | 不改变最终业务输出合同 | 控制 CLI/app-server 日志过滤；`exec` 默认 error | 调试运行时 | [M-DIAG] |

`--json` 的事件类别包括 `thread.started`、`turn.started`、`turn.completed`、`turn.failed`、`item.started/updated/completed` 和 top-level `error`；item 类型覆盖 agent message、reasoning、command execution、file change、MCP、collab、web search、todo/error。事件 schema 适合程序化完整性，但不能拿来声称 App/IDE 必须显示全部字段。[M-EXEC][S-EXEC-JSON]

## 7. 每类命令/工具应该展示的最小字段

本节前半是 Codex 当前事实的归纳，后半的“本项目建议”单独标记。

| 命令/工具类 | Codex 当前最小可见信息 | 收拢策略 | 详细入口 | 证据 |
| --- | --- | --- | --- | --- |
| Read | `Read` + 去重后的文件名 | 连续 read 合并，不展示主视图 stdout | transcript | [S-EXEC] |
| List files | `List` + path/command | 与其他探索命令合并 | transcript | [S-EXEC] |
| Search | `Search` + query + optional path | 与其他探索命令合并 | transcript | [S-EXEC] |
| 未识别 shell | Running/Ran + 完整 command + success/fail + output preview | 2 行命令 continuation；5 屏幕行 output，保留头尾 | transcript | [S-OUTPUT] |
| User shell | `You ran` + command + output | 最多 50 屏幕行 | transcript/terminal scrollback | [S-OUTPUT] |
| Background wait/input | waited/interacted + command/input preview | input preview 80 chars | transcript、`/ps` | [S-OUTPUT][M-CLI] |
| MCP | Calling/Called + server/tool(args) + status/result/error preview | result 5 行预算；JSON 先 compact | MCP status、日志或机器事件 | [S-MCP] |
| Web search | Searching/Searched + search/open/find detail | 多 query 预览第一项 | 无公开 CLI 结果展开合同 | [S-SEARCH][GAPS] |
| Patch | Edited/文件级 add/update/delete summary | 主 transcript 不展开 patch body | `/diff` | [S-PATCH][S-DIFF] |
| Plan update | explanation + 每一步状态 | 完整历史单元，不按 5 行命令规则截断 | transcript | [S-PLAN] |
| Error | 明确错误样式 + message；工具错误附着于工具单元 | retry 状态可临时更新以减少重复 | transcript/log | [S-ERROR] |
| Approval | 来源 thread/env、reason、exact action、permission rule、scope options | modal 受窗口限制时可全屏 | fullscreen、open source thread | [S-APPROVAL] |
| Subagent spawn | identity/role + model/effort + prompt preview | prompt 160 graphemes | agent thread | [S-AGENT] |
| Subagent wait/result | 每个 identity + status + result/error preview | result 240/error 160 graphemes | agent thread | [S-AGENT] |

### 本 Go 项目建议（`R`，不是 Codex 产品事实）

建议将上述最小字段设计为稳定的数据契约，而不是直接复制 CLI 文案：

1. 每个展示 item 至少有 `kind`、`label`、`status`、`summary`、`detail_ref`、`source_agent`、`started_at`、`duration`、`severity`。
2. `summary` 是主路径预算内的可读摘要；`detail_ref` 指向不可丢失的完整原始记录。
3. 折叠逻辑先按 `kind` 决策，再按行数/屏幕高度决策；read/list/search 可聚合，approval/error/final 不应按普通 tool output 规则处理。
4. 任何截断必须显示 omitted count 或明确的“查看完整记录”入口；不能静默丢弃。
5. App/CLI/IDE/JSON adapter 共享状态模型，但各自决定视图密度；不要把 JSON event schema直接当 UI schema。

## 8. 明确的公开文档缺口

以下问题在本轮 current manual、官方单页和官方 CLI 源码中没有跨表面产品合同：

1. App/IDE 普通 shell output 的默认行数、字符数或像素高度阈值。
2. App/IDE reasoning summary 何时完整展示、何时折叠、是否有“show more”。
3. App/IDE MCP、browser、computer-use、image tool result 的统一 card 展开规则。
4. App/IDE error stack trace、test log、compiler output 的截断策略。
5. App/IDE subagent activity preview 的排序、数量、字符上限。
6. CLI MCP result 被截断后，在同一 interactive session 中恢复完整原始 result 的通用入口。
7. CLI `/raw` 的名称容易被理解为“完整原始数据”，但源码只证明它切换 copy-friendly raw render；个别 cell 仍可能业务层截断。[S-MCP]
8. UI 如何利用 app-server event 字段做折叠。事件存在只能证明数据可用，不能证明某客户端展示它。

在这些缺口上，报告只给出 bounded uncertainty，不声称“Codex 一定如此”。

## 9. 可验证的设计原则与反例

| 原则 | Codex 证据 | 反例/警告 |
| --- | --- | --- |
| 主线程显示结论，不复制全部中间噪声 | subagent summaries 而非 raw output | 不能因此隐藏 error、approval 或最终验证结果。[M-MA] |
| 截断必须可发现 | `… +N lines (ctrl + t to view transcript)` | MCP raw view 仍可能受业务层 truncation，不能承诺所有入口都无损。[S-OUTPUT][S-MCP] |
| 高风险动作展示精确 payload | approval reason/command/permission/scope | 只显示工具名或“需要权限”不足以安全决策。[S-APPROVAL] |
| diff 用专用视图 | App review pane、CLI `/diff` pager | 只在 transcript 中列 changed files 不能替代 code review。[M-REVIEW][S-DIFF] |
| 状态高频更新不刷屏 | 一行 status + 3 行 detail，elapsed/interrupt 固定 | 将每个 heartbeat 写成永久 message 会破坏 transcript 信噪比。[S-STATUS] |
| 子代理必须可识别、可进入 | nickname/role/path + agent thread | 只有“3 agents running”无法定位阻塞或错误来源。[M-MA][S-AGENT] |
| 人类输出和机器输出分开 | `stderr` progress、`stdout` final、JSONL events | 把彩色 TUI 文本作为稳定 API 会造成脆弱解析。[M-EXEC] |

## 10. 证据索引

### Current manual / official product docs

- **[M-MA] Multi-agent operations**：manual lines 228-276（跨表面 availability、主线程/子线程职责）、367-443（consolidation、管理、审批）、509-533（nickname）。官方页：[Subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents.md)。
- **[M-APPROVAL] Approvals and sandboxing**：manual lines 1903-1935（sandbox/approval 与 side-effecting tools）、2138-2141（App auto-review item states）、2461-2495（权限范围及 App/CLI/IDE 控制）。官方页：[Agent approvals & security](https://learn.chatgpt.com/docs/agent-approvals-security.md)、[Auto-review](https://learn.chatgpt.com/docs/sandboxing/auto-review.md)。
- **[M-REASON] Reasoning display configuration**：manual lines 2932-2941、3916-3926（summary、verbosity、`hide_agent_reasoning=false`、`show_raw_agent_reasoning=false`）。官方页：[Advanced configuration](https://learn.chatgpt.com/docs/config-file/config-advanced.md)。
- **[M-CLI] CLI slash controls**：manual lines 6760-6843（完整命令目录与用途）、6849-6911（部分命令确认行为）。官方页：[Slash commands in Codex CLI](https://learn.chatgpt.com/docs/developer-commands.md?surface=cli)。
- **[M-KEYMAP] CLI transcript/status presentation**：manual lines 3410-3423、4202-4248（`open_transcript`、alternate screen、status line、terminal title）。官方页：[Config basics](https://learn.chatgpt.com/docs/config-file/config-basic.md)、[Configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference.md)。
- **[M-APP] Desktop app commands**：manual lines 5362-5405（panel/task/find/search 快捷键）。官方页：[ChatGPT desktop app commands](https://learn.chatgpt.com/docs/reference/commands.md)。
- **[M-APP-SET] Desktop app settings**：manual lines 5591-5633（follow-up、notifications、appearance）。官方页：[ChatGPT desktop app settings](https://learn.chatgpt.com/docs/reference/settings.md)。
- **[P-APP-SLASH] App slash commands**：2026-07-15 直接获取的官方页：[Slash commands](https://learn.chatgpt.com/docs/reference/slash-commands.md)。
- **[M-IDE] IDE commands/settings/slash commands**：manual lines 6151-6269。官方页：[IDE commands](https://learn.chatgpt.com/docs/developer-commands.md?surface=ide)、[IDE settings](https://learn.chatgpt.com/docs/developer-settings.md?surface=ide)。
- **[M-REVIEW] Code review**：manual lines 5987-6008、6016-6057、6065-6128。官方页：[Code review](https://learn.chatgpt.com/docs/code-review.md)。
- **[M-TERMINAL] Integrated terminal**：manual lines 6481-6510。官方页：[Integrated terminal](https://learn.chatgpt.com/docs/integrated-terminal.md)。
- **[M-GOAL] Long-running work**：manual lines 13465-13548。官方页：[Long-running work](https://learn.chatgpt.com/docs/long-running-work.md)。
- **[M-NOTIFY] Notifications**：manual lines 13562-13605。官方页：[Notifications](https://learn.chatgpt.com/docs/notifications.md)。
- **[M-EXEC] Non-interactive mode**：manual lines 10685-10723（`stderr` progress / `stdout` final）、10735-10777（permissions、JSONL、last message、schema）。官方页：[Non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode.md)。
- **[M-DIAG] Exec diagnostics**：manual lines 2640-2649（`RUST_LOG` 与 `exec` 默认 error level）。官方页：[Advanced configuration](https://learn.chatgpt.com/docs/config-file/config-advanced.md)。

Manual snapshot metadata：`codex-manual.md` mtime `2026-07-15T02:45:14+0800`，SHA-256 `1c1bc51b9b962b873fc1b1f8b003067a8ad5411f506f971be742f5a521cbc58a`；outline mtime `2026-07-15T02:58:04+0800`。

### Official `openai/codex` source at `f90e7deea6a715bbd153044af6f475eefa749177`

- **[S-EXEC] Exploration grouping and transcript detail**：[model grouping](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/exec_cell/model.rs#L119-L165)、[main vs transcript rendering](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/exec_cell/render.rs#L195-L363)。
- **[S-OUTPUT] Command/output budgets**：[constants and head-tail policy](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/exec_cell/render.rs#L33-L35)、[screen-row truncation and transcript hint](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/exec_cell/render.rs#L442-L658)、[layout 2/5](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/exec_cell/render.rs#L682-L711)。
- **[S-REASON] CLI reasoning summary**：[summary cell and transcript-only behavior](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/messages.rs#L217-L288)、[header-driven transcript-only selection](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/messages.rs#L495-L512)。
- **[S-MCP] MCP result rendering**：[five-line result rendering](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/mcp.rs#L89-L115)、[display/raw lines](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/mcp.rs#L119-L239)、[JSON compact + grapheme truncation](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/text_formatting.rs#L17-L43)。
- **[S-SEARCH] Web search summary**：[web search cell](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/search.rs#L5-L123)。
- **[S-PATCH] Patch summary**：[file summary and short failure output](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/patches.rs#L6-L62)。
- **[S-DIFF] Full diff pager**：[uncapped workspace command contract](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/workspace_command.rs#L1-L12)、[`/diff` dispatch](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/chatwidget/slash_dispatch.rs#L392-L416)、[full pager](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/app/event_dispatch.rs#L450-L465)。
- **[S-PLAN] Plan display**：[proposed plan](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/plans.rs#L78-L136)、[plan status list](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/plans.rs#L160-L237)。
- **[S-STATUS] Live status**：[one-row contract and three-line default](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/status_indicator_widget.rs#L1-L61)、[detail truncation](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/status_indicator_widget.rs#L202-L232)、[elapsed/interrupt/inline rendering](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/status_indicator_widget.rs#L235-L299)、[parallel review aggregation](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/chatwidget/status_state.rs#L70-L104)。
- **[S-APPROVAL] Approval payload and controls**：[header fields](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/bottom_pane/approval_overlay.rs#L676-L810)、[fullscreen/open-thread actions](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/bottom_pane/approval_overlay.rs#L529-L567)、[default key bindings](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/keymap.rs#L1136-L1150)。
- **[S-AGENT] Multi-agent presentation**：[preview constants and labels](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/multi_agents.rs#L29-L103)、[lifecycle rows](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/multi_agents.rs#L203-L461)、[result/error previews](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/multi_agents.rs#L621-L660)、[`/agent` bounded status feed](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/app/agent_status_feed.rs#L1-L203)。
- **[S-ERROR] Error/warning history**：[notice styles](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/notices.rs#L83-L86)、[error cell](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/tui/src/history_cell/notices.rs#L203-L219)。
- **[S-EXEC-HUMAN] Human `exec` renderer**：[item start/completion](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/exec/src/event_processor_with_human_output.rs#L67-L207)、[warning/error/diff/plan](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/exec/src/event_processor_with_human_output.rs#L226-L374)、[final channel behavior](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/exec/src/event_processor_with_human_output.rs#L376-L415)。
- **[S-EXEC-JSON] JSONL schema**：[event lifecycle](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/exec/src/exec_events.rs#L8-L99)、[item types](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/exec/src/exec_events.rs#L101-L163)、[collab state](https://github.com/openai/codex/blob/f90e7deea6a715bbd153044af6f475eefa749177/codex-rs/exec/src/exec_events.rs#L207-L256)。

### [GAPS] 缺口判定方法

本轮对 current manual outline 及正文检索了 `reasoning`、`tool call`、`command output`、`transcript`、`expand/collapse/fold`、`approval`、`diff`、`progress`、`error`、`subagent`、`notification`、`exec`、`IDE`、`app`。对 App slash commands 的 manual 缺口又读取了官方单页；对 CLI 具体阈值读取了官方开源实现。上述来源仍未给出 App/IDE 的通用折叠启发式，因此以 `U` 标记，而非依赖经验或当前会话截图进行猜测。

## 11. 结论

Codex 可验证的总体策略是：**主路径保持可决策、可行动和可审计；高频或高体积中间过程摘要化；完整证据转移到与内容类型匹配的独立视图；机器接口保留事件级结构。**

最值得复用的不是 CLI 的具体“5 行”数字，而是四条不变量：

1. 截断必须显式且有检查入口；
2. approval/error/final 的展示优先级高于普通 tool output；
3. subagent 的主线程信息是 identity + lifecycle + short result，完整工作留在 thread；
4. diff、terminal、transcript、review、JSONL 各自承担不同证据，不应挤进一个无限增长的主对话。

其中 CLI 的阈值和渲染分支是当前实现证据；App/IDE 未公开的折叠规则仍应留作产品实验和可观测性验证，不能伪装成官方事实。
