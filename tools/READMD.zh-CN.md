# 工具一致性索引

本索引覆盖开启 `AGENT_TRIGGERS=1`、`AGENT_TRIGGERS_REMOTE=1`、`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` 后的完整 34 工具模型可见面。

## 范围

- 纳入范围: `../src/tools.ts` 在全功能基线下暴露的每一个工具。
- 排除范围: `SendMessage` 中被有意删除的 `bridge:` / Remote Control 子路径；以及不属于当前 `../src/tools.ts` 基线的 Go 内部工具。
- 每份详细报告都使用相同结构: 名称与描述、参数与类型、实现概要、流程图与决策地图、输出与格式、关键差异。

## 报告目录

| 工具 | 概要 | English | 中文 |
| --- | --- | --- | --- |
| `Agent` | 接口完全对齐；Go 现已支持同步运行、后台启动、本地续跑和 cwd 重映射，但还没有原版完整的 remote/swarm 生命周期。 | [English](reports/Agent.md) | [中文](reports/Agent.zh-CN.md) |
| `AskUserQuestion` | 接口完全对齐；CLI 问答流程已较接近，但 Go 仍返回序列化 JSON 文本，而不是原版更丰富的 typed result 流水线。 | [English](reports/AskUserQuestion.md) | [中文](reports/AskUserQuestion.zh-CN.md) |
| `Bash` | 接口完全对齐；权限检查、后台处理和输出措辞已明显改善，但原版完整的 shell/运行时栈仍然更深。 | [English](reports/Bash.md) | [中文](reports/Bash.zh-CN.md) |
| `SendUserMessage` | 接口完全对齐；Go 保留了相同契约，但实现上仍主要是轻量 CLI 透传。 | [English](reports/SendUserMessage.md) | [中文](reports/SendUserMessage.zh-CN.md) |
| `CronCreate` | 接口完全对齐；Go 已能存储并调度 cron 任务，但触发后回接主运行时的端到端链路仍比原版轻。 | [English](reports/CronCreate.md) | [中文](reports/CronCreate.zh-CN.md) |
| `CronDelete` | 接口完全对齐；删除定时任务的动作已对齐，但周围的 cron 运行时在 Go 中仍更轻。 | [English](reports/CronDelete.md) | [中文](reports/CronDelete.zh-CN.md) |
| `CronList` | 接口完全对齐；列出 cron 任务的能力已对齐，而更宽的 cron 执行模型在 Go 中仍更轻。 | [English](reports/CronList.md) | [中文](reports/CronList.zh-CN.md) |
| `EnterPlanMode` | 接口完全对齐；Go 现在已经持久化 plan-mode 状态并阻止重复进入，但原版运行时仍有更丰富的 UI、agent 上下文和权限集成。 | [English](reports/EnterPlanMode.md) | [中文](reports/EnterPlanMode.zh-CN.md) |
| `EnterWorktree` | 接口完全对齐；Go 现在已通过 canonical-root 解析、slug 校验和本地持久化状态，镜像了更多原版的 worktree-entry 安全模型。 | [English](reports/EnterWorktree.md) | [中文](reports/EnterWorktree.zh-CN.md) |
| `ExitPlanMode` | 接口完全对齐；Go 现在已经持久化并恢复本地 plan-mode 状态，也会展示 allowed prompt 分类，但原版带审批语义的退出流程仍然更丰富。 | [English](reports/ExitPlanMode.md) | [中文](reports/ExitPlanMode.zh-CN.md) |
| `ExitWorktree` | 接口完全对齐；Go 现在已通过 canonical repo 清理和持久化状态恢复，镜像了更多原版的保留或删除 worktree 流程。 | [English](reports/ExitWorktree.md) | [中文](reports/ExitWorktree.zh-CN.md) |
| `Edit` | 接口完全对齐；Go 已较好覆盖核心替换文本流程，但原版仍有更丰富的 editor 感知型埋点。 | [English](reports/Edit.md) | [中文](reports/Edit.zh-CN.md) |
| `Read` | 接口完全对齐；Go 现在已经在文本区间读取以及 notebook/图片 typed tool result 路径上更接近原版，但重复读取状态和更深的 PDF/会话语义仍然落后。 | [English](reports/Read.md) | [中文](reports/Read.zh-CN.md) |
| `Write` | 接口完全对齐；Go 已具备较强的原子写入路径，但原版仍保留更多 editor 与 file-history 语义。 | [English](reports/Write.md) | [中文](reports/Write.zh-CN.md) |
| `Glob` | 接口完全对齐；Go 现在已使用共享的 ripgrep 驱动发现路径，因此核心 glob 行为更接近原版。 | [English](reports/Glob.md) | [中文](reports/Glob.zh-CN.md) |
| `Grep` | 接口完全对齐；Go 现在已使用共享 ripgrep 驱动引擎，补上了旧纯 Go 扫描器最大的语义缺口。 | [English](reports/Grep.md) | [中文](reports/Grep.zh-CN.md) |
| `ListMcpResourcesTool` | 接口完全对齐；对当前启用的 MCP 资源列出路径，两边已比较接近。 | [English](reports/ListMcpResourcesTool.md) | [中文](reports/ListMcpResourcesTool.zh-CN.md) |
| `NotebookEdit` | 接口完全对齐；Go 已能在核心路径上正确编辑 notebook 单元，但 notebook 专属元信息和输出仍比原版轻。 | [English](reports/NotebookEdit.md) | [中文](reports/NotebookEdit.zh-CN.md) |
| `ReadMcpResourceTool` | 接口完全对齐；对当前启用的 MCP 单资源读取路径，两边已比较接近。 | [English](reports/ReadMcpResourceTool.md) | [中文](reports/ReadMcpResourceTool.zh-CN.md) |
| `RemoteTrigger` | 接口完全对齐；Go 现已打到真实 OAuth 支撑的 trigger API，但与原版在 feature、policy 和 lifecycle 上的完整对齐仍未完成。 | [English](reports/RemoteTrigger.md) | [中文](reports/RemoteTrigger.zh-CN.md) |
| `SendMessage` | 在受支持子集上接口完全对齐；teammate、本地 agent、mailbox 和 `uds:` 路径都已可用，但原版的 `bridge:` / Remote Control 路径在 Go 中被有意排除。 | [English](reports/SendMessage.md) | [中文](reports/SendMessage.zh-CN.md) |
| `Skill` | 接口完全对齐；skill 加载契约已对齐，但原版的 skill/运行时集成仍比 Go 更宽。 | [English](reports/Skill.md) | [中文](reports/Skill.zh-CN.md) |
| `TaskCreate` | 接口完全对齐；创建契约已对齐，Go 现在也已经是在持久化、scope-aware、带锁的后端之上创建任务，但原版运行时仍然更丰富。 | [English](reports/TaskCreate.md) | [中文](reports/TaskCreate.zh-CN.md) |
| `TaskGet` | 接口完全对齐；查询契约已对齐，任务对象现在也来自持久化、scope-aware 的 Go 后端，但原版运行时仍有更丰富的 hook 和类型系统。 | [English](reports/TaskGet.md) | [中文](reports/TaskGet.zh-CN.md) |
| `TaskList` | 接口完全对齐；列出契约已对齐，返回的任务现在也来自持久化、scope-aware、带锁的 Go 后端。 | [English](reports/TaskList.md) | [中文](reports/TaskList.zh-CN.md) |
| `TaskOutput` | 接口完全对齐；Go 现在已经从持久化 runtime-task store 中读取结果，并具备更好的阻塞行为，但原版异步 task-output 运行时仍更丰富。 | [English](reports/TaskOutput.md) | [中文](reports/TaskOutput.zh-CN.md) |
| `TaskStop` | 接口完全对齐；停止契约已对齐，而且现在也会作用于持久化 runtime-task 底座，而不只是进程内任务。 | [English](reports/TaskStop.md) | [中文](reports/TaskStop.zh-CN.md) |
| `TaskUpdate` | 接口完全对齐；更新契约已对齐，Go 现在也已经是在持久化、scope-aware、带锁的后端里更新任务。 | [English](reports/TaskUpdate.md) | [中文](reports/TaskUpdate.zh-CN.md) |
| `TeamCreate` | 接口完全对齐；Go 现已持久化更丰富的 team 元信息和护栏，但原版 swarm 运行时仍然更宽。 | [English](reports/TeamCreate.md) | [中文](reports/TeamCreate.zh-CN.md) |
| `TeamDelete` | 接口完全对齐；Go 现在已镜像更多原版的 active-member 清理护栏，但完整 swarm 拆除在原版运行时里仍然更宽。 | [English](reports/TeamDelete.md) | [中文](reports/TeamDelete.zh-CN.md) |
| `TodoWrite` | 接口完全对齐；todo 写入契约已对齐，Go 现在也已经把它路由到 scope-aware 的持久化 todo store，并具备原版的空列表清空语义。 | [English](reports/TodoWrite.md) | [中文](reports/TodoWrite.zh-CN.md) |
| `ToolSearch` | 接口完全对齐；Go 现在已经通过隐藏 deferred 工具、支持 `select:`、返回结构化 tool reference、并在下一轮加载工具，把关键的延迟发现闭环对齐得更接近原版。 | [English](reports/ToolSearch.md) | [中文](reports/ToolSearch.zh-CN.md) |
| `WebFetch` | 接口完全对齐；基础抓取行为已对齐，但内容抽取和结果建模在 Go 中仍然更轻。 | [English](reports/WebFetch.md) | [中文](reports/WebFetch.zh-CN.md) |
| `WebSearch` | 接口完全对齐；基础搜索行为已对齐，但底层搜索栈在 Go 中仍然更轻。 | [English](reports/WebSearch.md) | [中文](reports/WebSearch.zh-CN.md) |
