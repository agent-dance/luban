# Session Goals

Session goals let a user give one conversation a durable completion condition.
An active goal is added to model context, evaluated after eligible turns, and
can automatically continue the query loop until the transcript demonstrates
completion or a runtime limit stops it.

The feature does not change tool permissions, sandbox policy, or approval
behavior.

## Slash Command

The `/goal` command manages the goal for the current session.

| Command | Behavior |
| --- | --- |
| `/goal` | Show the persisted goal and its status. |
| `/goal status` | Show the persisted goal and its status. |
| `/goal view` | Alias for `status`. |
| `/goal <objective>` | Set or replace the current goal. |
| `/goal set <objective>` | Set or replace the current goal. |
| `/goal edit <objective>` | Edit an active or paused goal. |
| `/goal pause` | Pause an active goal. |
| `/goal resume` | Resume a paused or blocked goal. |
| `/goal clear` | Persist the goal as cleared. |
| `/goal stop`, `off`, `reset`, `none`, `cancel` | Aliases for `clear`. |

Objectives are trimmed, must not be empty, and are limited to 4,000 Unicode
characters. The limit counts characters rather than UTF-8 bytes.

Successfully setting a goal or resuming a paused or blocked goal immediately
starts a model turn using the objective as its initial user input. Editing also
starts a turn when the edited goal remains active; editing a paused goal keeps
it paused. Status, view, pause, and clear only inspect or change state and do
not start a query.

User-issued `set` replaces the current goal, including an unfinished one. The
model-facing `CreateGoal` tool is deliberately narrower and refuses to replace
an active, paused, or blocked goal.

## State And Persistence

A session stores at most one goal in `SessionMeta.Goal`. The persisted record
contains:

- objective and status;
- optional goal token budget;
- accumulated main-assistant-turn output-token usage and assistant-turn count;
- the last evaluator reason;
- creation, update, achieved, and blocked timestamps.

Statuses are `active`, `paused`, `achieved`, `blocked`, and `cleared`. Clearing
does not delete metadata; it writes the `cleared` status so a resumed process
does not reactivate the old goal accidentally.

Goal metadata is preserved by partial `SessionMeta` updates and transcript
saves. Session resume loads the same metadata sidecar, so the goal state,
usage, last reason, and terminal status survive process restart. Legacy session
metadata without a `goal` field remains valid and is treated as having no goal.

The main runtime resolves the current session and project dynamically. This
keeps model tools attached to the active session after an in-process session
switch.

## Prompt Context

Only an `active` goal is added to the provider request. Paused, achieved,
blocked, cleared, and absent goals add no goal context.

The active context includes the objective and status, plus token budget, usage,
and last evaluator reason when present. The user-provided objective and the
model-generated evaluator reason are explicitly labeled as untrusted data and
JSON-quoted before prompt injection. The context is emitted as a leading meta
user message alongside the existing runtime context. The persisted transcript
is not rewritten merely to inject the goal context.

## Evaluation And Continuation

Goal evaluation runs only when a turn ends without tool calls. The ordering is:

```text
assistant turn ends without tool calls
  -> run Stop hooks
     -> Stop hook prevents continuation: stop without goal evaluation
     -> Stop hook returns blocking feedback: continue with that feedback
  -> load the current goal
     -> absent or non-active: use the normal query-loop stop behavior
  -> run the goal evaluator
     -> met: persist achieved and stop
     -> unmet: persist progress, check budgets, append a meta continuation, continue
     -> evaluator or persistence error: warn and stop automatically
```

The evaluator is transcript-only. It receives the objective and a structured
projection of messages, tool uses, and tool results. Its provider request has
no tools, disables thinking, limits output to 256 tokens, and requires exactly
one strict JSON object containing `met` and `reason`.

The query loop validates every successful evaluator result at its own boundary,
including results from a custom evaluator. The reason must be non-empty after
trimming and is limited to 512 Unicode characters. Invalid results fail closed.
Persisted evaluator-failure reasons are also bounded to 512 Unicode characters.

Evaluator API usage is emitted through a dedicated `goal_evaluation` event. For
the built-in evaluator, its metadata names the model that actually performed
the evaluation. Cost trackers attribute that auxiliary usage to the named model
and add it to session totals without changing the conversation model, main
turn's `LastTurn`, or turn count. A custom evaluator that does not participate
in the optional model-binding contract is attributed to the query snapshot's
conversation model.

The built-in evaluator uses the engine's current provider reference and binds to
the conversation model captured by the query snapshot. `/model` and
`QueryLoop.SetModel` therefore affect later queries, while an in-flight query
keeps its immutable model binding. The implementation does not promise Haiku or
any other dedicated small model. Custom evaluators may optionally implement the
same model-binding interface but are not required to do so.

Evaluator failure is fail-closed for automatic continuation: the loop records
the failure when possible, emits a warning, leaves the goal active, and stops.
It does not assume completion and does not fall through to another automatic
continuation policy.

## Runtime Limits

Goal continuation remains bounded by all existing loop controls:

- `MaxTurns` is the outer hard turn bound. The CLI runtime defaults it to 100
  when no positive value is configured.
- A goal token budget, when present, stops continuation once accumulated output
  tokens from every main-model assistant turn reach the budget. Tool-use turns
  and truncated `max_tokens` recovery attempts are included, and reaching the
  budget on one prevents the next model turn.
  Evaluator API usage is measured through its dedicated event and is not added
  to `Goal.Usage`.
- The existing query-loop token-budget policy still applies, including its
  normal early-stop behavior.
- Context cancellation, provider errors, Stop-hook decisions, and goal
  persistence errors also stop continuation.

Assistant-turn usage is persisted as each active-goal turn completes. On an
evaluable no-tool turn, the evaluator decision and reason are persisted before
the budget checks. This leaves the assistant-turn count, output-token usage,
and last reason available for status display even when no further turn is
allowed.

## Model Tools

Three strict, typed tools expose the narrow model-side lifecycle:

| Tool | Input | Classification |
| --- | --- | --- |
| `GetGoal` | Empty object | Read-only and concurrency-safe. |
| `CreateGoal` | Required `objective`; optional positive `token_budget` | Mutating. Refuses to replace an unfinished goal. |
| `UpdateGoal` | `status` equal to `complete` or `blocked` | Mutating. Only transitions an active goal. |

`complete` maps to the persisted `achieved` status. Pause, resume, edit, clear,
and arbitrary state replacement are not exposed through `UpdateGoal`.

The tools are always loaded for the root agent and hidden from child-agent
registries. This prevents a child working on one slice of a task from marking
the root session goal complete or blocked.

## Permissions

Setting a goal does not enable tools, change permission mode, expand allowed
directories, bypass explicit denies, or weaken sandbox checks. Automatic goal
turns use the same registry and permission handler as ordinary turns.

`GetGoal` is read-only. `CreateGoal` and `UpdateGoal` are classified as write
tools, so they remain subject to the active permission policy. In Plan mode the
registry blocks them like other write tools until Plan mode is exited. The
evaluator itself cannot call tools.

## TUI And Screen Reader

The TUI refreshes goal state after `/goal` commands, when applying a session
snapshot, and from the final `goal_status` projection emitted after each query.
This final refresh reflects evaluator-driven `achieved` and model-tool-driven
`blocked` transitions without requiring another command or repository read in
the event handler. Active goals appear in the status bar as
`Goal: <objective>`; paused goals appear as `Goal paused: <objective>`. The
segment is limited to 40 terminal cells and is omitted as a whole when the
terminal is too narrow. Control characters and line breaks in an objective are
collapsed so the segment remains one line. Terminal goal states do not occupy
the status bar.

Screen-reader mode routes `/goal` through the same command registry and emits
line-oriented status and command receipts. Goal activation starts the same
immediate model turn as the TUI and does not depend on the visual status-bar
projection.

## Security Boundary

The user-provided objective, evaluator transcript, and evaluator output are
untrusted data. In particular, the objective and `LastEvaluatorReason` are data,
not instructions, commands, permission grants, or trusted system messages. Code
that renders or reuses them must preserve that distinction.

The evaluator system prompt tells the evaluator to treat the transcript as
untrusted evidence. Before any evaluator reason is persisted, the query-loop
boundary requires non-empty text and enforces the 512-character limit; failure
reasons are bounded as well. When the objective or reason is included in
active-goal prompt context, and when the reason is included in a continuation,
it is JSON-quoted and explicitly labeled as untrusted data. Embedded reminder
delimiters, newlines, or instruction-like text therefore remain quoted data
instead of becoming new prompt structure.

These controls preserve the data boundary; they do not make the reason trusted.
Contributors must not interpret evaluator-reason content as markup or
authority-bearing instructions.

## Known Limitations

- Goal completion is transcript evidence, not an independent inspection of the
  filesystem, tests, external services, or command results outside the
  transcript.
- The default evaluator uses the current conversation provider and model; there
  is no separate evaluator-model selection policy yet.
- Goal usage counts output tokens from every main-model assistant turn,
  including tool-use turns. It does not count input tokens or evaluator usage.
  Evaluator usage still contributes to session and actual-model cost totals but
  does not replace the main turn's `LastTurn` record.
- In-process metadata updates are synchronized, but the metadata
  read-modify-write path has no cross-process file lock. Two processes writing
  the same session metadata concurrently can overwrite each other's partial
  updates.

## Verification

Run focused goal tests:

```bash
go test ./goal ./session ./commands -run Goal -count=1
go test ./prompt ./loop -run Goal -count=1
go test ./tools -run Goal -count=1
go test ./tui -run Goal -count=1
go test ./registry -run 'Goal|Visibility|Tool' -count=1
go test . -run 'Goal|SetupRegistry' -count=1
go test -race ./tools -run Goal -count=1
```

Run the complete Go test suite before release:

```bash
go test ./...
```
