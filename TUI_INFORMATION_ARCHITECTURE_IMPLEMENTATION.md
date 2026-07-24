# TUI Information Architecture Implementation And Verification Record

Status: P0/P1 implementation and final verification complete, 2026-07-14

Authority: `TUI_INFORMATION_DESIGN.md`, repository code/tests, and the active Goal.
This document records migration decisions and verification evidence. It does not
replace the product-level design contract.

## 1. Historical Pre-Migration Baseline

This table is the historical baseline captured before the migration; it is not
current verification evidence. The worktree was already dirty and continued
changing during collection, so the baseline records the observed commands and
defects rather than claiming a reproducible clean-tree snapshot. Existing changes
are user-owned and were not reverted or reformatted outside the migration.

| Check | Baseline | Evidence |
| --- | --- | --- |
| `go test ./...` | FAIL | Existing failures in root worktree-hook, commands branding/MCP persistence, input history path, and tools remote-trigger/OAuth tests. No TUI failure. |
| `go test -race ./...` | FAIL | Same existing functional failures; no race detector report. 96.15s. |
| `go build ./...` | PASS | 2.61s. |
| `go vet ./...` | PASS | 1.49s. |
| `staticcheck ./...` | TOOLCHAIN BLOCKED | Installed staticcheck was built with Go 1.25.0; module uses Go 1.26.1. |
| `golangci-lint run ./...` | TOOLCHAIN BLOCKED | Local launcher depends on missing `fef`. |
| `pkg/go-tui: go test ./...` | PASS | 3.15s. |
| `pkg/go-tui: go test -race ./...` | PASS | 10.05s, no races. |
| `pkg/go-tui: go build ./...` | PASS | 1.10s. |
| `pkg/go-tui: go vet ./...` | FAIL | Existing generated self-assignments in examples 10 and 22. |
| terminal/buffer/resize/suspend targeted tests | PASS | Existing mock/emulator suite, 2.70s. No real PTY matrix exists. |

Baseline defects relevant to this migration:

1. `ui.Renderer` drops `ToolUseID`; `tui/renderer.go` assigns results to the
   most recent tool call and truncates payloads to 20 lines.
2. `tui/root.go` groups calls/results only when adjacent and builds render nodes
   for every historical message.
3. `/resume` changes engine/runtime state without hydrating `AppState.Messages`.
4. `/clear` is unregistered; its dormant implementation overwrites the current
   session and never clears presentation state.
5. permissions retain tool/session identity until `permissions.CLIPermissionHandler`,
   then collapse to name/input and a bare `y/n/a` response.
6. activity sources retain stable IDs in runtime layers, while TUI exposes only
   a stack of tool names.

## 2. Target Ownership And Invariants

The migration uses existing runtime and session structures. It adds no dependency.

| State | Owner | Lifetime | Primary key |
| --- | --- | --- | --- |
| runtime event | loop/engine producer plus renderer adapter | transient execution stream; durable facts project into audit/evidence | `TurnID` + `WorkUnitID` + `ActorID` + domain ID; adapter supplies session/epoch |
| query event stream | engine `queryEventStream` | one query through atomic Final publication and channel close | `(ProjectRoot, SessionID)` + query lifetime |
| observation | TUI presentation reducer and session projection | retained with the session; durable ID excludes presentation epoch | observation ID |
| tool observation | TUI presentation reducer | call through terminal outcome and later resume | `(SessionID, ToolUseID)`; deterministic legacy fallback otherwise |
| activity | deterministic activity reducer | active presentation epoch; terminal state never regresses, while repeated terminal evidence may update count/timeline | `(SessionID, Epoch, ActivityID)`; grouped by work unit and actor |
| hook execution | hook runner, loop event, and activity/evidence projection | one concrete hook configuration invocation, including delayed Agent completion | `HookExecutionID` + `ConfigID` + turn/work/actor/tool/task |
| tool identity ledger | query loop plus session metadata | complete session lifetime across compaction, save, shutdown, restart, and resume | `(ProjectDir, SessionID)` + `seen_tool_use_ids` |
| decision | permission/plan bridge and TUI | request through durable terminal result | presentation session/epoch + `DecisionID`; execution session retained separately |
| disclosure | observation reducer plus session metadata/evidence journal | retained with the observation until explicitly changed or history is deleted | observation ID |
| detail/evidence | memory or private file detail store | process lifetime for memory; session retention for file artifacts | `DetailRef{Source, Digest, Size}` with an opaque audit key |
| visible transcript | TUI presentation state | current view epoch | session epoch |
| model context | engine/session conversation | current workspace-scoped conversation | `(ProjectRoot, SessionID)` |
| audit | existing session repository and detail artifacts | recoverable session history | `(ProjectDir, SessionID)` |
| deletion marker | project-scoped session store | permanent after delete-history commits; physical cleanup may be retried | `(ProjectDir, SessionID)` via `<session>.deleted` |

Required invariants:

- A result, permission, spinner, hook, or activity never derives ownership from
  slice position, adjacency, tool name, or "most recent" state.
- A summary never owns the only copy of raw evidence.
- Tool call and result are one observation with one disclosure state.
- State/outcome/actionability are deterministic enums; generated text is never
  used for completion or permission logic.
- Identity-aware asynchronous events are admitted only when their adapter-owned
  session ID and epoch match the visible projection; stale events may update
  their audit but not the active view. Legacy renderer methods remain
  non-correlating compatibility fallbacks and are not used by internal structured
  tool dispatch.
- A session transition prepares engine, transcript, usage, runtime context,
  focus, and mode before a serialized commit. Commit failure rolls back; if both
  commit and rollback fail, the permission surface fails closed instead of
  presenting a mode that was not restored.
- Project root is immutable query identity and is distinct from execution CWD.
  It crosses engine, loop events, tool execution, hook evidence, UI adapters,
  SDK events, and JSON machine output without being inferred from a nested CWD.
- Malformed session metadata is never treated as absent and is never overwritten
  by startup. Delete-history publishes a durable marker before cleanup; all
  later readers, writers, engines, and background follow-ups treat it as a
  terminal deletion across process restarts.
- ToolUseID uniqueness spans the full logical session, not only the currently
  visible or model-visible transcript. A project-scoped metadata ledger survives
  compaction and restart and is unioned with IDs derived from legacy transcripts.
- Query event emission and terminal Final/close share one mutex gate. Once
  closure commits, late execution observers deterministically drop their event;
  repeated finish calls are harmless and cannot send on a closed channel.
- Closing detail/decision/search restores the captured focus, scroll anchor, and
  input draft. Viewing evidence never executes a tool or model request.
- The render tree is bounded by the current transcript window plus explicitly
  pinned/expanded observations, not total session history.

## 3. Cleanup Plan

Behavior is locked with regression tests before each production pass.

### Pass 1: dead and invalid paths

- Delete the 20-line TUI truncation after lossless detail tests pass.
- Delete backward scanning for the most recent tool call after structured event
  dispatch is covered.
- Delete adjacency-only tool grouping after observation rendering is covered.
- Remove the unreachable `Collapsed`-only expanded branch after disclosure
  levels and keyboard actions replace it.
- Keep classic renderer truncation only if its full-output escape path is proven;
  otherwise route it through the same lossless evidence contract.

### Pass 2: duplicate and positional state

- Replace `SpinnerTools []string` and name-based removal with ID-keyed activities.
- Reuse `ToolUseBlock.ID`, `ToolResultBlock.ToolUseID`, engine session IDs,
  runtime Agent IDs, session repository artifacts, and existing terminal APIs.
- Keep legacy `ui.Renderer` methods as compatibility fallbacks; TUI uses an
  optional structured interface until all internal callers migrate.

### Pass 3: boundary and lifecycle repair

- Normalize runtime facts once into observations; renderers do not infer domain
  outcomes from neighboring strings.
- Route permission and Plan approval through structured Decisions.
- Move session projection into an explicit staged transition boundary.
- Gate background/old-query projection by owning session epoch.

### Pass 4: test reinforcement and simplification

- Add the terminal, accessibility, performance, and long-session matrices.
- Remove compatibility helpers only when repository-wide searches prove no
  remaining caller depends on them.
- Run targeted tests after every pass and full gates after every phase.

## 4. Staged Migration

### Phase 1: identity, observations, and evidence (P0 A/B)

1. Add structured event context and optional structured tool renderer methods.
2. Carry session, turn, actor, work-unit, and tool-use IDs from loop/engine to TUI.
3. Add an observation reducer keyed by stable ID. Legacy saved sessions use a
   deterministic per-message/block key; results with an explicit `ToolUseID`
   join only that call. Missing IDs remain standalone legacy observations and
   never use nearest-call inference.
4. Add `DetailRef` storage using the existing session artifact directory when
   available and an in-memory store for transient/test use.
5. Store full input/result evidence and deterministic outcome. `DetailRef` owns
   byte size and digest, presentation computes line counts when needed, and the
   Activity reducer owns actionability. Default success to Summary and errors to
   Detail; both can enter Evidence.
6. Use `DisclosureState{Level, HasMore, UserPinned}` for structured observations,
   bound to observation ID, with keyboard-equivalent selection and cycling.
   Retain `Message.Collapsed` only for thinking, legacy sessions, and the
   unstructured renderer compatibility surface.
7. Maintain a session-lifetime ToolUseID ledger from prior calls and results.
   Persist the stable, normalized set as `SessionMeta.SeenToolUseIDs` under the
   JSON key `seen_tool_use_ids`; carry it
   through transcript save, compaction, shutdown, restart, and resume. Union it
   with IDs derivable from model-visible messages so old metadata remains
   compatible. Keep ledgers project-scoped, retain the old ledger on clear
   conversation, and start the new clear-conversation session with an empty one.
8. Reject a missing, same-batch duplicate, or later-turn reused ID before any
   tool, permission, or Hook side effect in both buffered and streaming paths.
9. Separate same-session transcript mutation from session replacement.
   `SetMessagesPreservingToolUseLedger` retains compacted identities when an MCP
   prompt appends messages; `SetMessages` keeps intentional replacement semantics
   for a true session transition.
10. Serialize runtime event publication and terminal closure in
    `queryEventStream`: commit the closed state, publish exactly one Final event,
    and close under the same gate. Late observers return a deterministic drop
    result, while repeated finish calls are idempotent.
11. Sort rich Hook configuration event keys before flattening them so `ConfigID`
   is stable across loads and Go map iteration order. Keep `HookExecutionID`
   unique per actual invocation.
12. Execute PreQuery immediately before model I/O and PostQuery after the model
   response, with complete session/project/turn/work/actor evidence. Both are
   fail-closed policy boundaries when execution fails or blocks continuation.

Tests first: out-of-order concurrent tools, same-name tools, >20-line exact
evidence, distinct error/partial/denied/cancelled/timeout, non-adjacent grouping,
joint disclosure, no execution during evidence access, focus/anchor/draft restore,
same-batch and later-turn ToolUseID rejection, compact/save/shutdown/restart/resume
ledger retention, legacy/clear/cross-project isolation, stable ConfigID,
same-session MCP prompt retention, atomic Final/close with deterministic late
observer drop, and fail-closed PreQuery/PostQuery ordering.

### Phase 2: session epochs and clear semantics (P0 C)

1. Add an immutable session snapshot and monotonically increasing presentation
   epoch to `AppState`.
2. Add a persisted-message projector that preserves text, thinking, tool use,
   tool result, images/unknown blocks, and stable legacy fallbacks.
3. Hydrate startup and interactive resume from the same target snapshot.
4. Prepare target cwd, transcript, hooks, runtime config, mode, and detail store;
   apply only after `Engine.Resume` and validation succeed. Roll back all prepared
   mutations on failure.
5. Reject stale engine/background events at the active projection boundary.
6. Implement distinct commands:
   - `/clear view`: reset visible projection only.
   - `/clear` and `/clear conversation`: create a new empty session/model context,
     publish an empty presentation epoch, preserve the previous session/audit.
   - `/session delete <session>`: explicit high-risk confirmation and repository
     deletion; never an alias of clear.
7. Publish `<session>.deleted` atomically before removing transcript, metadata,
   and artifacts. Keep the marker after cleanup, aggregate cleanup failures for
   retry, and reject save/load/resume/follow-up paths after a committed deletion.
8. Treat metadata parse/read failures as startup and transition failures. Only a
   true not-found/empty-store condition may create a fresh session; `SaveMeta`
   must not replace an unreadable sidecar.
9. Reset/load usage, cwd, hooks, permission/plan mode, focus, and scroll with the
   target epoch. Clear permission cache at session boundary.

Tests first: clear-view isolation, recoverable clear-conversation, resume parity,
corrupt metadata at startup/save/transition, durable delete cleanup and restart
non-resurrection, failure injection/rollback, stale-event rejection, switch/query
race, permission cache isolation, focus/scroll/draft behavior.

### Phase 3: structured Decisions (P0 D)

1. Introduce `PromptRequest` and `PromptResponse` with decision/tool/turn/work
   IDs, actor, action, target, impact, risk reason, rule source, approval scope,
   choices, and explicit terminal result. The durable `DecisionRecord` adds the
   resolution timestamp.
2. Propagate runtime Agent ID through tool execution and permission adapters.
3. Preserve separate approved, rejected, escaped, cancelled, timed-out, and
   shutdown outcomes. Pending prompts observe context cancellation.
4. Render Plan approval as a Plan Decision: full plan evidence, allowed prompts,
   and explicit post-approval execution/permission mode. Do not use the generic
   one-line input preview.
5. Append decision lifecycle observations to audit/presentation without treating
   them as ordinary free-text messages.

Tests first: full multi-section Plan review, actor attribution, concurrent request
correlation, all terminal outcomes, narrow/CJK/accessibility decision rendering.

### Phase 4: activity and attention model (P1 E)

1. Add an ID-keyed deterministic reducer for tool, Agent, background task, and
   required hook activity facts. PostSampling, Stop, and StopFailure summaries
   carry hook execution, turn, work-unit, and actor IDs into evidence.
2. Default signal: `<phase> · N activities · M needs input`.
3. Expanded work view groups by work unit and actor; ticks update in place.
4. Preserve failed, partial, denied, needs-input, and ready-review states when
   successful siblings aggregate.
5. Expose keyboard inspect, locate, and cancel actions only where callbacks exist.
6. Keep immutable background ownership as `(ProjectRoot, SessionID, TaskID)`,
   separate from nested execution CWD. Both fullscreen and screen-reader
   consumers require project/session admission before projection.
7. Persist notifications independently of consumers. A sink-only delivery is
   replayed exactly once when a follow-up consumer is later installed; deletion
   is acknowledged as a terminal discard so a restart cannot resurrect history.
   Screen-reader mode installs the same notification/follow-up lifecycle and
   emits linear completion, discard, and model-follow-up receipts.
8. Pin the parent Hook execution observer and complete correlation input in a
   retained Agent's immutable origin. When the Agent completes after its request
   context has ended, rehydrate that context before Notification execution so
   evidence retains parent session/project, turn, work unit, actor/type,
   ToolUseID, Agent task ID, execution/config ID, and raw Hook output.
   Persist each actual execution in `RuntimeNotification.HookExecutions` with
   Hook type, execution/config/index identity, a credential-redacted Hook
   snapshot, immutable parent input, raw output, and `RecordedAt`. This durable
   receipt remains available after the parent query's event stream closes.
9. Treat a successful compaction boundary as structured, lossless evidence.
   Preserve trigger; pre/post/true-post token counts and retained/discarded
   ranges; summary and display message; previous-tail identity; discovered tools;
   and preserved-segment start/count/anchor/direction in `DetailStore`, with a
   linked disclosure `Observation` and completed `Activity` supporting jump,
   detail, transcript search, and export.
10. Use stable trigger-qualified Activity identities for automatic, reactive,
    and manual compaction. Preserve success, failure, and cancellation as
    distinct terminal evidence; screen-reader mode deduplicates append-only
    start/boundary/end receipts and announces failure and cancellation separately.
    Every real successful path orders `compact_start` → boundary → `compact_end`;
    failed and cancelled paths terminate without publishing a false boundary.

Tests first: same SessionID across different project roots, nested execution CWD,
sink-only persistence followed by late consumer registration, restart replay,
deleted-session terminal discard, retained-Agent parent Hook causality, complete
compaction boundary search/export evidence, auto/reactive/manual lifecycle, and
fullscreen/screen-reader success/failure/cancellation parity.

MCP heartbeat/health remains P2 unless measured failures require a persistent
surface. Existing command inspectors remain the default.

### Phase 5: fullscreen escape and bounded projection (P1 F/G)

1. Add transcript search over observations and evidence with stable result IDs.
2. Add exact human-readable/structured export and `$VISUAL`/`$EDITOR` open paths
   for either the complete transcript or one observation's complete evidence.
3. Add runtime mouse toggle plus startup configuration; retain complete keyboard
   scrolling, observation selection, disclosure, back, copy, and export paths.
4. Add `--screen-reader` / `CLAUDE_CODE_SCREEN_READER=1` as an append-only,
   no-cursor-control interactive renderer with explicit actor/action/outcome,
   complete Plan review, context help, and decision/query/session receipts.
5. Window transcript rendering around viewport/anchor with bounded overscan and
   pinned observations. Page navigation shifts the window without losing the
   logical anchor.
6. Verify close, suspend/resume, resize, normal error returns, and panic cleanup
   restore cursor, mouse modes (including drag), alternate screen, raw mode,
   bracketed paste, and keyboard protocol.

Tests first: search/export/editor/mouse/keyboard flows, three terminal sizes,
CJK/long paths, screen-reader lines, 100,000 observations, p95 budgets, and
terminal lifecycle/subprocess PTY cleanup.

## 5. Requirement Mapping

| Contract | Primary code surface | Regression evidence |
| --- | --- | --- |
| P0 A stable identity | `loop/events.go`, `loop/query.go`, `loop/tool_identity.go`, `engine/core.go`, `commands/mcp_prompts.go`, `session/session.go`, `hooks/config.go`, `ui/send_user_message.go`, `tui/observation_store.go` | out-of-order same-name results, persistent session-lifetime ToolUseID reuse rejection before side effects across compact/restart/resume and same-session MCP mutation, legacy/clear/project isolation, atomic Final/close with deterministic late-observer drop, orphan/conflict fallback, deterministic hook ConfigID, ProjectRoot machine/SDK identity |
| P0 B lossless disclosure | `tui/detail_store.go`, observation/state/root and transcript I/O | exact >20-line and structured envelope evidence, atomic evidence journal, complete compaction boundary evidence, joint disclosure, no replay, interaction restore |
| P0 C session consistency | `session/session.go`, `session/repository.go`, `session_setup.go`, `session_switcher.go`, `repl_tui.go`, session projector/commands | clear-view isolation, recoverable new conversation, corrupt-meta startup fail-closed, durable delete marker/restart non-resurrection, resume parity, commit-boundary cancellation, rollback/fail-closed and stale-event tests |
| P0 D Decisions | loop/engine permission request, `permissions/structured_prompt.go`, fullscreen/screen-reader views, Plan tool | complete Plan, real actor, choices/rule source, immutable audit, six distinct outcomes and shutdown receipts |
| P1 E activity | `tui/activity_store.go`, `tui/renderer.go`, `tools/background_tasks.go`, `tools/runtime_task_store.go`, event/background/Agent/hook producers | production Verifying phase, sparse in-place updates, work/actor grouping, inspect/cancel/locate, attention and occurrence counts, retained-Agent parent Notification causality with durable redacted execution receipts, complete auto/reactive/manual compaction evidence and distinct terminal states |
| P1 F disclosure | observation/disclosure state, session metadata, root keymap | summary/detail/evidence, durable return points, local pin independent from Alt+O global show-all, mouse/keyboard parity |
| P1 G fullscreen escape | search/export/editor/mouse commands; screen-reader renderer; go-tui lifecycle | keyboard-only flow, per-observation editor, searchable/exportable compaction evidence, buffer/accessibility tests, append-only linear output including background completion/follow-up and compaction success/failure/cancellation, real terminal matrix |
| performance | go-tui async event/frame path, presentation window, detail store | event-to-terminal-frame and Root-buffer p95, detail read/disclosure p95, true 100k observation tree bound |

## 6. Verification Gates

Each phase must pass its targeted unit and integration tests before the next phase.
Final verification runs:

1. `gofmt` on changed Go files only, followed by a changed-file format check.
2. Targeted `tui`, `loop`, `permissions`, `session`, engine, command, and root tests.
3. `go test ./...`, `go test -race ./...`, `go build ./...`, `go vet ./...`.
4. `pkg/go-tui` tests, race, build, vet, and terminal/buffer/resize/suspend suites.
5. Available lint/staticcheck with toolchain compatibility recorded.
6. 40x12, 80x24, 120x40 buffer matrix; CJK and long-path fixtures.
7. First-token, memory/disk evidence p95, and 100,000-observation bounded-tree tests.
8. Real macOS and Linux PTY runs, tmux, and one direct IDE integrated terminal;
   emulator/subprocess evidence supplements but does not replace this matrix.

P2 is implemented only if P0/P1 verification produces evidence that command-only
MCP/hook inspection cannot support safe diagnosis.

## 7. Implemented Result And Verification

Implemented ownership boundaries:

- `ObservationStore` owns exact call/result correlation and per-observation
  disclosure. It never scans neighboring transcript messages. Legacy sessions
  receive deterministic per-message/block IDs; missing or duplicate tool IDs
  become explicit orphan/conflict observations instead of nearest-call or LIFO
  guesses.
- `DetailStore` owns immutable evidence in memory or private session artifacts.
  File artifacts are content-addressed, private, atomically published, and paired
  with an observation evidence journal so a resume can recover references even
  if the broader session sidecar was not rewritten before a crash. Search,
  export, and detail editor access read this store without replay. Plan review
  instead reads the complete structured Decision body and review details.
  Compaction boundaries use the same path for complete trigger, count/range,
  summary, discovered-tool, tail, display-message, and preserved-segment evidence.
- `SessionSnapshot` commits projection, epoch, usage, interaction state,
  disclosure return points, Decision audit, and permission mode through the
  serialized transition boundary. Session metadata distinguishes unknown usage
  from measured zero. `RuntimeContextResumer` builds the target workspace-scoped
  conversation before `sessionSwitcher` publishes CWD, hooks, system prompt,
  tool runtime, and visible identity. Commit or rollback failure cannot publish a
  more permissive presentation than the surviving runtime state.
- Engine conversations, deletion tombstones, background notification targets,
  and follow-up queries use the composite project/session identity. Duplicate
  session IDs in different projects therefore remain independent; legacy task
  records without a project root are accepted only when their origin is
  unambiguous.
- Delete-history first atomically publishes `<session>.deleted`, then removes
  metadata, artifacts, and transcript. The marker remains permanently; cleanup
  errors are aggregated and a repeated delete retries cleanup without making
  the ID live again. Session repository, engine resume/query, and delayed
  background follow-ups consult the marker, so process restart cannot resurrect
  a deleted conversation.
- Startup, latest-session discovery, resume, and metadata save propagate corrupt
  sidecar errors. They create a new session only for a genuinely empty store and
  never replace malformed metadata as if it were missing.
- `PromptRequest`/`PromptResponse` carry attributed permission and Plan facts;
  the execution decision remains separate from the six presentation outcomes.
  Background bubble requests use the parent session for presentation admission
  while retaining the Agent execution session, actor, turn, and work unit.
  Fullscreen retains a structured receipt after modal close; screen-reader mode
  announces the receipt inline without cursor rewriting.
- `ActivityStore` owns ID-keyed state, outcome, attention counts, ordering, and
  available actions. Tool input/actor facts deterministically select Executing
  or Verifying, including background commands. Every user query has a unique
  query-scoped TurnID. Each configured Hook invocation has an execution/config
  ID; sorted configuration loading keeps ConfigID stable across process loads.
  PreQuery/PostQuery bracket model I/O as fail-closed, evidence-bearing policy
  boundaries. Background work has its own task/work-unit ID and an immutable
  origin snapshot. In-flight work therefore keeps its project, session, CWD,
  hook runner, notification sink, permission/Plan state, and Todo store when the
  foreground switches workspace.
- Retained Agents pin the parent Hook observer and correlation context at launch.
  Delayed Notification Hook execution therefore reaches the real parent observer
  with parent session/project, turn, work unit, actor/type and Agent ToolUseID,
  plus task, execution/config IDs and raw output evidence. The runtime task record
  durably stores one `HookExecutions` receipt per actual config: execution/config
  and config-index identity, credential-redacted Hook snapshot, immutable parent
  input, raw output, and `RecordedAt`. The real delayed-Agent/CoreEngine path
  completes after parent Final without panic or evidence loss.
- ToolUseID validation includes every call/result already present in session
  history and the project-scoped `SessionMeta.SeenToolUseIDs` ledger. The ledger
  survives compaction, transcript save, shutdown, restart, and resume; legacy
  metadata falls back to transcript-derived IDs, clear keeps the old session's
  ledger without copying it to the new session, and equal session IDs in different
  projects remain isolated. Missing, duplicate, and later-turn reused IDs fail
  before tool, permission, or Hook effects, rather than allowing two causal
  records to share an identity. Same-session message rewrites use
  `SetMessagesPreservingToolUseLedger`; MCP prompt insertion therefore cannot
  erase compacted IDs, while session transitions retain explicit replacement
  semantics through `SetMessages`.
- `ProjectRoot` is carried separately from nested execution `CWD` through
  `QueryRequest`, engine runtime, every loop event, `ToolExecutionContext`, Hook
  correlation, `ToolEventContext`, SDK events, and JSON machine output.
- Engine `queryEventStream` guards emit, terminal Final, and close with one
  mutex. Final publication and channel closure are atomic from observer writers'
  perspective; emissions arriving after the closed-state commit are dropped
  deterministically, and repeated finish calls are idempotent.
- Background tasks persist their immutable project/session origin rather than a
  nested execution directory. Fullscreen and screen-reader projection require
  both identities to match. Persisted sink-only notifications replay once when
  a follow-up consumer appears; screen-reader mode emits linear completion and
  follow-up receipts, and a deleted-session follow-up is terminally discarded.
- Compaction uses one stable Activity identity while progressing and maps
  `compact_end`, `compact_failed`, and `compact_cancelled` to distinct succeeded,
  failed, and cancelled terminal outcomes. Terminal compaction activities are
  no longer cancellable. Automatic, reactive, and manual boundaries retain
  pre/post/true-post token counts, retained/discarded ranges, complete summary,
  previous-tail identity, discovered tools, display message, and preserved
  segment. Successful evidence is linked through Activity to a disclosure
  Observation and `DetailStore`; failed/cancelled terminal evidence is also
  retained, and the result participates in transcript search and export. The
  real automatic, reactive, and manual paths all emit start → boundary → end on
  success; failure and cancellation emit their distinct terminal state with no
  boundary event.
- Hook evidence snapshots the exact input/output and execution/config identity,
  preserves raw stdout/stderr with observed byte counts and explicit truncation,
  and redacts credential-like HTTP headers from presentation evidence.
- `TranscriptShowAll` is a render-only global override. It never mutates local
  per-observation disclosure or pinned state.
- `ScreenReaderRenderer` owns a linear interactive path with a single cancellable
  input arbiter. Decisions preempt command input and require a scoped
  `decision <id> <choice>` response; pre-decision lines remain commands.
  Untrusted C0/C1/ANSI input is rendered visibly rather than executed, and an
  approval is denied if its session admission or durable audit write fails. It
  never enters alternate screen or emits mouse, colour, animation, cursor
  movement, or carriage-return overwrite sequences. Compaction emits de-duplicated
  append-only start/boundary/completion receipts and distinct failure/cancellation
  receipts with trigger and reason.
- go-tui owns all terminal escape state, including mouse button motion, external
  process suspension, and termination-signal cleanup. Unix wakeups are
  non-blocking/coalesced; Windows reads have a cancellable single owner; raw-mode
  restoration failure stops rendering and is returned by `Run`/`Close`. Windows
  behavior is covered by unit tests and compile/link validation only; no Windows
  runtime claim is made.

Removed or simplified paths:

- Removed TUI backward result lookup and adjacency-only tool grouping.
- Removed 20-line truncation from both fullscreen and classic terminal paths.
- Removed manual application-level `?1002` writes after the terminal library
  gained symmetric mouse ownership.
- Replaced the dormant destructive clear fallback with explicit view and new
  conversation callbacks; history deletion is a separate high-risk Decision.
- Bounded default transcript element construction to viewport-derived overscan
  plus explicitly pinned observations. The complete audit remains searchable
  and exportable.
- Restored `/help` with contextual fullscreen shortcuts and added direct
  `$EDITOR` access for one observation's full result and structured envelope.
- Retained the unstructured `ui.Renderer` methods and `Message.Collapsed` only as
  compatibility surfaces for classic output, thinking blocks, and old sessions.
  Internal tool dispatch uses `StructuredToolRenderer`; classic output prints the
  complete result but does not offer observation-level disclosure.

Final verification was run on 2026-07-14 on macOS Darwin arm64 with Go 1.26.1.
All timings are wall-clock results from the final worktree unless a range is
explicitly identified as repeated measurements.

| Check | Result |
| --- | --- |
| formatting / diff hygiene | PASS; `gofmt -d` over tracked and untracked changed Go files produced no output; `git diff --check` passed |
| post-verification process/artifact hygiene | PASS; 216 Goal-owned `/private/tmp` directories (about 76GB) were removed and the remaining count is zero; two revoked orphan processes were terminated; no terminal-fixture or tmux process remained |
| root `go test ./...` | PASS; 90.92s total, `tools` 79.768s |
| root `go test -race ./...` | PASS; 116.12s total, `tools` 103.024s, no race detector report |
| root `go build ./...` | PASS; 7.13s |
| root `go vet ./...` | PASS; 4.46s |
| root Windows amd64 compile/link | PASS; 11.04s with `GOOS=windows GOARCH=amd64 go test -exec=/usr/bin/true ./...`; compile/link only, not a runtime test |
| `pkg/go-tui` `go test ./...` | PASS; 3.79s |
| `pkg/go-tui` `go test -race ./...` | PASS; 5.80s, no race detector report |
| `pkg/go-tui` build / vet | PASS; 1.75s / 0.71s; generator receiver-shadowing and generated self-assignment defects have regression coverage |
| `pkg/go-tui` Python terminal tests | PASS; 9 tests in 1.22s |
| `pkg/go-tui` Windows amd64 compile/link | PASS; 1.80s; compile/link only, not a runtime test |
| staticcheck 2026.1 full repository | Delta gate PASS: final worktree 259 historical diagnostics versus clean HEAD 280; normalized additions 0, removals 21. The repository-wide result is not mislabeled as diagnostic-free. |
| `pkg/go-tui` staticcheck 2026.1 | Delta gate PASS: final worktree 28 diagnostics versus clean HEAD 28; normalized additions 0, removals 0. |
| staticcheck production safety gate | PASS: `staticcheck -tests=false -checks 'SA2*,SA4*,SA5*,SA6*,SA9*' ./...` |
| first-token performance | Async event to terminal frame, 100 samples: p95 18.708–58.666µs; Root token to buffer: p95 103.25–154.916µs; direct token state update: p95 417–666ns; all below 100ms |
| state-transition performance | p95 2.167–3.542µs and independent of token debounce |
| detail performance | Raw memory read p95 209–250ns; raw file read p95 24.125–48.041µs; memory disclosure-to-Root p95 15.299–15.429ms; file disclosure-to-Root p95 23.970–28.542ms; below 100ms / 250ms budgets |
| long transcript | PASS with 100,000 real observations and 5,000 user-pinned observations; render nodes remained within the asserted 5,000–16,000 viewport-plus-pins bound |
| buffer/accessibility | PASS in the full suites for 40x12, 80x24, 120x40, CJK, long paths, linear screen-reader semantics, keyboard actions, and focus/anchor/draft restoration |

### 7.1 Real Terminal Matrix

The terminal fixture checks the actual raw-mode release and symmetric
alternate-screen, cursor, and mouse ownership sequences. Hashes below are output
SHA-256 prefixes recorded by the matrix runner.

| Host / surface | Scenario evidence | Result |
| --- | --- | --- |
| macOS Darwin arm64, openpty | normal resize/suspend/resume: 19 checks, `6c64dda8…`; SIGTERM: 4 checks, `2acafa98…`; controlled nonzero exit: 5 checks, `df4ae83a…` | PASS |
| macOS Darwin arm64, tmux 3.6a | real tmux pane normal resize/suspend/resume: 19 checks, `da4618d5…` | PASS |
| Lima Linux arm64, openpty | normal: 19 checks, `87c72507…`; SIGTERM: 4 checks, `0867eb8a…`; controlled nonzero exit: 5 checks, `541642bc…` | PASS |
| Lima Linux amd64, openpty | normal: 19 checks, `a8c46471…`; SIGTERM: 4 checks, `d2a595b9…`; controlled nonzero exit: 5 checks, `84d922ec…` | PASS |
| Cursor direct integrated TTY | resized 159x25 to 159x38; Ctrl+Z released termios (`termios_same=yes`, stop receipt `rc=146`), `fg` redrew, and `q` returned 0; SIGTERM returned 0 with identical termios; controlled nonzero exit returned 23 with identical termios | PASS |

The Cursor run used the integrated pane's controlling TTY directly, not only an
IDE environment marker around a nested PTY. It was captured earlier in this Goal
after terminal lifecycle code had reached the stable implementation used by the
final tree; subsequent session, background, and identity work did not change
that terminal lifecycle. After verification, the user's panel layout and working
directory were restored. Real PTY/IDE evidence is supplemented by buffer/emulator
and subprocess lifecycle tests for panic, raw-mode restoration errors, bracketed
paste, keyboard protocol, and Windows reader cancellation. Windows remains
compile/link-only because no Windows runtime was available.

### 7.2 P2 Decision

No always-visible MCP/hooks health dashboard was added. `/mcp` and `/doctor` use
the live backend and remain available in screen-reader mode, which preserves the
command-oriented diagnostic path without adding ambient transcript noise.

Structured execution/config identity and raw evidence were added for Session,
UserPrompt, tool, task, Agent, and compact Hook lifecycles only because P0
causality and auditability require them. This event-level Activity/evidence is not
a persistent health panel. Final P0/P1 verification found no measured diagnosis
gap that justified the additional P2 surface.

## 8. Remaining Risks And Follow-up

No known P0/P1 correctness blocker remains after the final unit, race, build,
static-delta, performance, accessibility, and real-terminal gates. Residual
constraints are explicit:

- Windows terminal behavior has unit and compile/link coverage but no native
  runtime evidence in this environment.
- On Windows the durable deletion marker file is flushed with `fsync`, but the
  containing directory cannot be flushed after the atomic rename through the
  available implementation. Crash durability is therefore weaker than on Unix;
  the marker remains logically fail-closed during normal operation and restart.
- The root repository still has 259 historical staticcheck diagnostics and
  `pkg/go-tui` has 28. The final worktree adds none; future cleanup should keep
  using the normalized HEAD delta gate instead of claiming a clean baseline.
- Durable deletion markers are intentionally retained indefinitely. Any future
  marker-retention policy must preserve the non-resurrection guarantee for late
  or replayed background work.
- MCP/Hook health stays command-oriented through `/mcp` and `/doctor`. Add a
  persistent P2 surface only if future measured incidents show that these live
  inspectors are insufficient.
