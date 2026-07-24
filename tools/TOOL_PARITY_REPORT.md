# Tool Parity Report

Chinese version: [TOOL_PARITY_REPORT.zh-CN.md](./TOOL_PARITY_REPORT.zh-CN.md)

Date: 2026-04-17  
Repo: `gosrc` vs `../src`

## Scope

This report compares the original TypeScript tool surface in `../src/tools.ts` with the Go clone in `registry_setup.go`.

It separates parity into three layers:

1. Surface parity
   Model-facing registry inclusion, tool name, and input schema.
2. Runtime parity
   Backing implementation, side effects, persistence model, async behavior, and external integrations.
3. Output parity
   Tool-result content shape and meaning.

Important: `32/32 tools at ★★★★★ surface parity` means only layer 1 is aligned in the current environment. It does not mean every tool already has full runtime/output parity.

## Current Environment Snapshot

The active baseline in this session was computed under:

- `USER_TYPE=` (external build)
- `NODE_ENV=`
- `ENABLE_LSP_TOOL=`
- `ENABLE_TOOL_SEARCH=`
- `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=`
- `EMBEDDED_SEARCH_TOOLS=`
- `CLAUDE_CODE_ENABLE_TASKS=`
- `AGENT_TRIGGERS=`
- `AGENT_TRIGGERS_REMOTE=`

Under that environment, the active base tool set is 32 tools.

## Validation Run

- `python3 tools/scripts/tool_parity_audit.py`
  Result: 32 TS base tools, 32 Go registered tools, and all 32 active tools at `★★★★★` on the surface layer.
- `go test ./tools ./registry ./permissions ./loop -count=1`
  Result: pass.
- `go test ./... -count=1`
  Result: all tool-related packages pass; `tui` still has unrelated pre-existing failures in `markdown_test.go` and `stream_renderer_test.go`.

## Executive Summary

- Surface parity for the active base tools is `★★★★★`.
- Gating parity for the current environment is also aligned.
- Full semantic parity is not yet `★★★★★`.
- The biggest remaining runtime/output gaps are in:
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

Bottom line:

- If the question is “does the model now see the same active tool names and input parameters?” the answer is yes.
- If the question is “is every tool already implemented identically end-to-end?” the answer is no.

## Parity Legend

- `★★★★★`: effectively aligned for this layer.
- `★★★★☆`: highly aligned, with only minor simplifications or omissions.
- `★★★☆☆`: same intent and usable shape, but important differences remain.
- `★★☆☆☆`: same surface or general intent, but major semantic differences remain.
- `★☆☆☆☆`: missing, placeholder-only, or effectively not equivalent.

Dimension meaning:

- `Surface`: rate against tool name, registration, and input schema.
- `Runtime`: rate against execution semantics, persistence model, side effects, async behavior, and integrations.
- `Output`: rate against tool-result structure and meaning.

## Active Tool Matrix

| Tool | Description | Parameters | Go implementation overview | Output format | Surface | Runtime | Output | Difference / why |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `Agent` | Launch a sub-agent | `description`, `prompt`, `subagent_type?`, `model?`, `run_in_background?`, `name?`, `team_name?`, `mode?`, `isolation?`, `cwd?` | Runs a nested Go loop synchronously with depth limits | Plain text or error text | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original supports richer async/background/remote/team flows and structured result statuses; Go currently returns text only and rejects background mode |
| `AskUserQuestion` | Ask the user structured questions | `questions: [{header, question, options, multiSelect?}]` | Interactive terminal prompt with validation | JSON string of answers | `★★★★★` | `★★★★☆` | `★★★☆☆` | Core flow matches; Go returns serialized JSON text instead of the richer SDK-side typed result pipeline |
| `Bash` | Execute shell commands | `command`, `timeout?`, `description?`, `run_in_background?`, `dangerouslyDisableSandbox?` | Executes `bash -c`, applies simplified sandbox selection and dangerous-command blocking | JSON string with `stdout`, `stderr?`, `exit_code`, `error?` | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | Original supports background tasks, sed-edit approval plumbing, richer permission/read-only validation, and more nuanced shell semantics |
| `SendUserMessage` | Send a brief user-facing message | `message`, `attachments?`, `status?` | CLI passthrough only; attachments and status are not interpreted | Plain text | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original tool is a richer user-facing messaging surface; Go currently just echoes the message |
| `CronCreate` | Create a cron trigger | `cron`, `prompt`, `recurring?`, `durable?` | Stores job and schedules it in Go cron store | Plain text summary | `★★★★★` | `★★★☆☆` | `★★★☆☆` | Jobs are tracked, but firing is not wired back into the main query loop; fire callback only logs |
| `CronDelete` | Delete a cron trigger | `id` | Removes job from cron store | Plain text summary | `★★★★★` | `★★★☆☆` | `★★★☆☆` | Same limitation as `CronCreate`: scheduler exists, end-to-end execution loop is incomplete |
| `CronList` | List cron triggers | none | Reads cron store and formats job list | Plain text list | `★★★★★` | `★★★☆☆` | `★★★☆☆` | Read/list behavior is fine; trigger execution path is still simplified |
| `EnterPlanMode` | Enter plan mode and create a plan file | none | Creates `.claude/plans/<timestamp>.md` and flips shared plan state | Plain text instructions | `★★★★★` | `★★★★☆` | `★★★☆☆` | Core state change matches; original has more UI/system-prompt integration |
| `EnterWorktree` | Create an isolated git worktree | `name?` | Uses `git worktree add`, tracks shared worktree state | Plain text summary | `★★★★★` | `★★★★☆` | `★★★★☆` | Core behavior is aligned enough for current baseline |
| `ExitPlanMode` | Exit plan mode | `allowedPrompts?: [{tool:"Bash", prompt:string}]` | Reads stored plan file and exits plan mode | Plain text plan summary | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original handles approval flow, teammate/leader plan handoff, request IDs, and permission orchestration; Go only exits and prints the plan |
| `ExitWorktree` | Keep or remove worktree | `action`, `discard_changes?` | Clears state or removes worktree/branch via git | Plain text summary | `★★★★★` | `★★★★☆` | `★★★★☆` | Core keep/remove flow is present and understandable |
| `Edit` | Replace text in a file | `file_path`, `old_string`, `new_string`, `replace_all?` | Reads file, applies string replacement, writes back atomically | JSON string summary | `★★★★★` | `★★★★☆` | `★★★☆☆` | Intent matches; original has more UX/result metadata and edit instrumentation |
| `Read` | Read file content | `file_path`, `offset?`, `limit?`, `pages?` | Reads text file line ranges with line numbers | Plain text with numbered lines or warning text | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original also handles notebooks, images, PDFs, token/byte limits, file caching, and richer result types; Go only implements plain-text line reading |
| `Write` | Write file content | `file_path`, `content` | Atomic overwrite with allowed-dir checks | JSON string summary | `★★★★★` | `★★★★☆` | `★★★☆☆` | Core write path is sound; original preserves more editor/file-history behavior |
| `Glob` | Find files by glob pattern | `pattern`, `path?` | Walks filesystem, applies glob matching, truncates large result sets | Plain text file list | `★★★★★` | `★★★★☆` | `★★★★☆` | Core use case aligns for the active baseline |
| `Grep` | Search file contents | `pattern`, `path?`, `glob?`, `output_mode?`, `-B`, `-A`, `-C`, `context`, `-n`, `-i`, `type?`, `head_limit?`, `offset?`, `multiline?` | Pure-Go recursive scan with regex, context, offsets, and file-type filtering | Plain text result list | `★★★★★` | `★★★☆☆` | `★★★☆☆` | API surface is aligned, but original uses ripgrep semantics and has more precise search behavior/performance guarantees |
| `ListMcpResourcesTool` | List MCP resources | `server?` | Lists configured servers or queries a connected server | Plain text list | `★★★★★` | `★★★★☆` | `★★★★☆` | Behavior is broadly aligned for current built-in MCP resource listing |
| `NotebookEdit` | Edit notebook cells | `notebook_path`, `cell_id?`, `new_source`, `cell_type?`, `edit_mode?` | Parses `.ipynb`, replaces/inserts/deletes cells, writes file atomically | Plain text status | `★★★★★` | `★★★☆☆` | `★★☆☆☆` | Original returns richer notebook result metadata and attribution info; Go only reports a simple text status |
| `ReadMcpResourceTool` | Read one MCP resource | `server`, `uri` | Connects to MCP server and reads resource content | Plain text content | `★★★★★` | `★★★★☆` | `★★★★☆` | Good parity for the active resource-read path |
| `RemoteTrigger` | Manage remote agent triggers | `action`, `trigger_id?`, `body?` | Local in-memory CRUD store plus legacy webhook fallback | JSON string with `status` and `json` | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original talks to real remote trigger infrastructure; Go is a local approximation only |
| `SendMessage` | Send a message to a teammate | `to`, `summary?`, `message` | Uses local coordinator message bus and simple team state | JSON string with `success`, `message`, `routing?`, `recipients?` | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | Original swarm/inbox/request flow is much richer; Go implements a local queue-style subset |
| `Skill` | Load a skill prompt | `skill`, `args?` | Resolves installed skill and returns prepared prompt content | Plain text | `★★★★★` | `★★★☆☆` | `★★★☆☆` | Useful and aligned in spirit, but still simpler than the full original skill/runtime integration |
| `TaskCreate` | Create a task | `subject`, `description`, `activeForm?`, `metadata?` | Creates task in in-memory store | Plain text summary | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original task system is file-backed and integrated with hooks/team behaviors; Go tasks are in-memory and return simpler text |
| `TaskGet` | Get a task | `taskId` | Reads task from in-memory store | JSON string task object | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | Shape is understandable, but persistence and surrounding task lifecycle are simplified |
| `TaskList` | List tasks | none | Lists in-memory tasks | Plain text list | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Same core reason as `TaskCreate`: storage and orchestration are much simpler |
| `TaskOutput` | Get task output | `task_id`, `block?`, `timeout?` | Returns stored output from in-memory task record | Plain text output | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original async task pipeline is richer; Go only exposes simplified stored output |
| `TaskStop` | Stop a task | `task_id?`, `shell_id?` | Marks task as stopped in in-memory store | JSON string summary | `★★★★★` | `★★☆☆☆` | `★★★☆☆` | Supports both ids, but the underlying task model is far simpler than original |
| `TaskUpdate` | Update a task | `taskId`, `subject?`, `description?`, `activeForm?`, `status?`, `addBlocks?`, `addBlockedBy?`, `owner?`, `metadata?` | Mutates in-memory task store, supports `deleted` pseudo-status | Plain text summary | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Input surface matches, but original task system has persistence, hooks, and team-aware behaviors that Go only partially mirrors |
| `TodoWrite` | Write a todo list | `todos: [{subject, description?, status}]` | Upserts tasks by subject into in-memory store | Plain text summary | `★★★★★` | `★★☆☆☆` | `★★☆☆☆` | Original behavior sits on the real task subsystem; Go uses a simplified in-memory adapter |
| `ToolSearch` | Search tools by keyword | `query`, `max_results?` | Simple registry name/description substring search | Plain text list | `★★★★★` | `★★★☆☆` | `★★★☆☆` | Good surface parity, but original defer-loading/discovery model is more sophisticated |
| `WebFetch` | Fetch one web page | `url`, `prompt` | HTTP fetch + basic HTML-to-text extraction + cache | Plain text with `Prompt:` header and page content | `★★★★★` | `★★★☆☆` | `★★★☆☆` | Core fetch works, but original has richer extraction, permissions, and result modeling |
| `WebSearch` | Search the web | `query`, `allowed_domains?`, `blocked_domains?` | Performs cached search and formats text results | Plain text search results | `★★★★★` | `★★★☆☆` | `★★★☆☆` | Same intent, but original web search stack is richer and more tightly integrated with the overall runtime |

## Conditional Tools Not Active In This Environment

These tools were not part of the active 32-tool baseline because their gates were off. The gates in Go were aligned so they stay hidden under the same conditions.

| Tool | Gate in original | Current state | Notes |
| --- | --- | --- | --- |
| `Config` | `USER_TYPE=ant` | Inactive | Go now gates it the same way; implementation is present but was not part of the active baseline |
| `LSP` | `ENABLE_LSP_TOOL=true` | Inactive | Go now gates it the same way; implementation exists but only some LSP operations are implemented |
| `TeamCreate` | `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` or `--agent-teams` | Inactive | Go gate now matches current external-build behavior; implementation is still a simplified local team runtime |
| `TeamDelete` | same as `TeamCreate` | Inactive | Same status as `TeamCreate` |
| `TestingPermission` | `NODE_ENV=test` | Inactive | Go now gates it correctly and routes it through an always-ask permission path |

## Out Of Scope / Not In Current Baseline

The following were not part of the current active base-tool audit and should not be confused with the current `32-tool / ★★★★★ surface` result:

- Ant-only tools such as `Tungsten` and `REPL`
- Windows-only `PowerShellTool`
- Feature-gated tools like `Sleep`, `Monitor`, `Workflow`, `WebBrowserTool`, `PushNotificationTool`, `SubscribePRTool`, `SendUserFileTool`, and others
- Dynamic per-server `mcp__*` tools generated from MCP servers

Those are either not active in this environment, not implemented in Go, or both.

## Why Surface Can Be `★★★★★` While Runtime Still Differs

The current audit script proves:

- the same tools are exposed
- under the same current gates
- with the same model-facing names
- with the same model-facing input parameters

It does not prove:

- the same backing services exist
- the same persistence model exists
- the same async/background execution exists
- the same output schema exists
- the same edge-case behavior exists

That is why tools like `Agent`, `Read`, `RemoteTrigger`, and the `Task*` family still show real runtime gaps even though the active surface is now `★★★★★`.

## Recommended Next Work If Full End-To-End Parity Is The Goal

Priority order:

1. `Read`
   Add notebook/PDF/image handling and byte/token-limited reading semantics.
2. `Bash`
   Add background task support and closer permission/sed-edit behavior.
3. `Agent`
   Add async/background output modes and closer swarm/remote/worktree execution semantics.
4. `Task*` and `TodoWrite`
   Move from in-memory storage to the original persistent task model and output shapes.
5. `RemoteTrigger` and `SendMessage` / `Team*`
   Replace local stand-ins with the original backing flows.
6. `ExitPlanMode`
   Rebuild approval/request-id/leader approval behavior.

## Final Verdict

Current status is:

- API-shape-compatible for the active base tool set
- not yet a full semantic clone of the original tool runtime

If someone asks “are there still differences?”, the correct answer is:

- No for active tool registry, names, and input schemas
- Yes for several high-value runtime and output paths listed above
