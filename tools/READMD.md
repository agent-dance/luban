# Tool Parity Index

This index covers the full 34-tool model-facing surface with `AGENT_TRIGGERS=1`, `AGENT_TRIGGERS_REMOTE=1`, and `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`.

## Scope

- Included: every tool exposed by `../src/tools.ts` under the full-feature baseline.
- Excluded: the intentionally removed `bridge:` / Remote Control sub-path of `SendMessage`; Go-only internal tools that are not part of the current `../src/tools.ts` baseline.
- Every detailed report uses the same structure: name and description, parameters and types, implementation overview, a visual flow and decision map, output and format, and key gaps.

## Reports

| Tool | Summary | English | 中文 |
| --- | --- | --- | --- |
| `Agent` | Exact surface; Go now supports sync runs, background launch, local continuation, and cwd rebasing, but not the original full remote/swarm lifecycle. | [Agent](reports/Agent.md) | [中文](reports/Agent.zh-CN.md) |
| `AskUserQuestion` | Exact surface; the CLI questionnaire flow is close, but Go still returns serialized JSON text instead of the richer original typed result pipeline. | [AskUserQuestion](reports/AskUserQuestion.md) | [中文](reports/AskUserQuestion.zh-CN.md) |
| `Bash` | Exact surface; permission checks, background handling, and output phrasing have improved, but the original full shell/runtime stack is still deeper. | [Bash](reports/Bash.md) | [中文](reports/Bash.zh-CN.md) |
| `SendUserMessage` | Exact surface; Go keeps the same contract but still implements it mostly as a thin CLI passthrough. | [SendUserMessage](reports/SendUserMessage.md) | [中文](reports/SendUserMessage.zh-CN.md) |
| `CronCreate` | Exact surface; Go stores and schedules cron jobs, but end-to-end firing into the main runtime is still lighter than the original. | [CronCreate](reports/CronCreate.md) | [中文](reports/CronCreate.zh-CN.md) |
| `CronDelete` | Exact surface; deleting scheduled jobs is aligned, but the surrounding cron runtime remains lighter in Go. | [CronDelete](reports/CronDelete.md) | [中文](reports/CronDelete.zh-CN.md) |
| `CronList` | Exact surface; listing cron jobs is aligned, while the broader cron execution model remains lighter in Go. | [CronList](reports/CronList.md) | [中文](reports/CronList.zh-CN.md) |
| `EnterPlanMode` | Exact surface; Go now persists plan-mode state and prevents duplicate entry, but the original runtime still has richer UI, agent-context, and permission integration. | [EnterPlanMode](reports/EnterPlanMode.md) | [中文](reports/EnterPlanMode.zh-CN.md) |
| `EnterWorktree` | Exact surface; Go now mirrors more of the original worktree-entry safety model with canonical-root resolution, slug validation, and persisted local state. | [EnterWorktree](reports/EnterWorktree.md) | [中文](reports/EnterWorktree.zh-CN.md) |
| `ExitPlanMode` | Exact surface; Go now persists and restores local plan-mode state and surfaces allowed prompt categories, but the original approval-aware exit workflow is still richer. | [ExitPlanMode](reports/ExitPlanMode.md) | [中文](reports/ExitPlanMode.zh-CN.md) |
| `ExitWorktree` | Exact surface; Go now mirrors more of the original keep-or-remove worktree flow with canonical repo cleanup and persisted state recovery. | [ExitWorktree](reports/ExitWorktree.md) | [中文](reports/ExitWorktree.zh-CN.md) |
| `Edit` | Exact surface; Go captures the main replace-text workflow well, but the original still has richer editor-aware instrumentation. | [Edit](reports/Edit.md) | [中文](reports/Edit.zh-CN.md) |
| `Read` | Exact surface; Go now matches the original much more closely on text-range reads plus typed notebook/image tool results, while repeated-read state and deeper PDF/session semantics are still behind. | [Read](reports/Read.md) | [中文](reports/Read.zh-CN.md) |
| `Write` | Exact surface; Go has a strong atomic-write path, but the original still preserves more editor and file-history semantics. | [Write](reports/Write.md) | [中文](reports/Write.zh-CN.md) |
| `Glob` | Exact surface; Go now uses a shared ripgrep-backed discovery path, so core glob behavior is much closer to the original. | [Glob](reports/Glob.md) | [中文](reports/Glob.zh-CN.md) |
| `Grep` | Exact surface; Go now uses a shared ripgrep-backed engine, closing the biggest semantic gaps from the old pure-Go scanner. | [Grep](reports/Grep.md) | [中文](reports/Grep.zh-CN.md) |
| `ListMcpResourcesTool` | Exact surface; the active MCP resource-listing path is close between the two implementations. | [ListMcpResourcesTool](reports/ListMcpResourcesTool.md) | [中文](reports/ListMcpResourcesTool.zh-CN.md) |
| `NotebookEdit` | Exact surface; Go edits notebook cells correctly for the core path, but notebook-specific metadata and output are still lighter than in the original. | [NotebookEdit](reports/NotebookEdit.md) | [中文](reports/NotebookEdit.zh-CN.md) |
| `ReadMcpResourceTool` | Exact surface; the active MCP single-resource read path is close between the two implementations. | [ReadMcpResourceTool](reports/ReadMcpResourceTool.md) | [中文](reports/ReadMcpResourceTool.zh-CN.md) |
| `RemoteTrigger` | Exact surface; Go now hits the real OAuth-backed trigger API, but full feature, policy, and lifecycle parity with the original is still incomplete. | [RemoteTrigger](reports/RemoteTrigger.md) | [中文](reports/RemoteTrigger.zh-CN.md) |
| `SendMessage` | Exact surface for the supported subset; teammate, local-agent, mailbox, and `uds:` paths are useful, but the original removed `bridge:` / Remote Control path is intentionally out of scope in Go. | [SendMessage](reports/SendMessage.md) | [中文](reports/SendMessage.zh-CN.md) |
| `Skill` | Exact surface; the skill-loading contract is aligned, but the original skill/runtime integration is still broader than Go. | [Skill](reports/Skill.md) | [中文](reports/Skill.zh-CN.md) |
| `TaskCreate` | Exact surface; the create contract is aligned, and Go now creates tasks on top of a persistent, scope-aware, locked backend, though the original runtime is still richer. | [TaskCreate](reports/TaskCreate.md) | [中文](reports/TaskCreate.zh-CN.md) |
| `TaskGet` | Exact surface; the lookup contract is aligned, and the task object now comes from a persistent, scope-aware Go backend, though the original runtime still carries richer hooks and typing. | [TaskGet](reports/TaskGet.md) | [中文](reports/TaskGet.zh-CN.md) |
| `TaskList` | Exact surface; the listing contract is aligned, and listed tasks now come from a persistent, scope-aware, locked Go backend. | [TaskList](reports/TaskList.md) | [中文](reports/TaskList.zh-CN.md) |
| `TaskOutput` | Exact surface; Go now reads from a persisted runtime-task store with better blocking behavior, but the original async task-output runtime is still richer. | [TaskOutput](reports/TaskOutput.md) | [中文](reports/TaskOutput.zh-CN.md) |
| `TaskStop` | Exact surface; the stop contract is aligned, and it now operates against the persisted runtime-task substrate as well as in-process tasks. | [TaskStop](reports/TaskStop.md) | [中文](reports/TaskStop.zh-CN.md) |
| `TaskUpdate` | Exact surface; the update contract is aligned, and Go now updates tasks inside a persistent, scope-aware, locked backend. | [TaskUpdate](reports/TaskUpdate.md) | [中文](reports/TaskUpdate.zh-CN.md) |
| `TeamCreate` | Exact surface; Go now persists richer team metadata and guardrails, but the original swarm runtime is still broader. | [TeamCreate](reports/TeamCreate.md) | [中文](reports/TeamCreate.zh-CN.md) |
| `TeamDelete` | Exact surface; Go mirrors more of the original active-member cleanup guard now, but full swarm teardown is still broader in the original runtime. | [TeamDelete](reports/TeamDelete.md) | [中文](reports/TeamDelete.zh-CN.md) |
| `TodoWrite` | Exact surface; the todo-writing contract is aligned, and Go now routes it through a scope-aware persisted todo store with the original empty-list clearing semantics. | [TodoWrite](reports/TodoWrite.md) | [中文](reports/TodoWrite.zh-CN.md) |
| `ToolSearch` | Exact surface; Go now mirrors the key deferred-discovery loop much more closely with hidden deferred tools, `select:` support, structured tool references, and next-turn tool loading. | [ToolSearch](reports/ToolSearch.md) | [中文](reports/ToolSearch.zh-CN.md) |
| `WebFetch` | Exact surface; basic fetch behavior is aligned, but extraction and result modeling are still lighter in Go. | [WebFetch](reports/WebFetch.md) | [中文](reports/WebFetch.zh-CN.md) |
| `WebSearch` | Exact surface; basic search behavior is aligned, but the underlying search stack is still lighter in Go. | [WebSearch](reports/WebSearch.md) | [中文](reports/WebSearch.zh-CN.md) |
