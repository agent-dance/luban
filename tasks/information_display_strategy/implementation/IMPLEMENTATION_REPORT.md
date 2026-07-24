# Information-display strategy implementation report

Implementation date: 2026-07-15  
Source design: [`INFORMATION_DISPLAY_STRATEGY_REPORT.md`](../../../INFORMATION_DISPLAY_STRATEGY_REPORT.md)  
Task index: [`INDEX.json`](INDEX.json)

## Outcome

The repository now implements the report's behavioral strategy rather than only documenting it:

- A pure, deterministic policy selects D0-D3, surface, reason codes, aggregation eligibility, and redaction requirements.
- Forty-one logical production-tool names (the union of conditionally registered tools) plus aliases, dynamic MCP tools, and unknown fallback have semantic formatters covering queued/running/success/warning/failure/inspect/full.
- Twenty-four registered slash commands have typed action/target/risk/display/lifecycle contracts. The v2 command schema distinguishes success, warning, partial, failure, denial, cancellation, timeout, interruption, and exit; it also retains sections and evidence references. Commands that historically encoded business failure in prose now report an explicit domain outcome; presentation never guesses from words such as `Error` or `Failed`.
- Typed command sections and evidence references are projected on every terminal surface, terminal errors remain visible even after legacy output, and screen-reader-only commands use the same typed lifecycle instead of bypassing it.
- TUI, classic terminal, screen reader, and JSON share semantic facts. JSON remains event-lossless; screen-reader output remains linear; classic/TUI can aggregate routine activity.
- Adapter-owned aggregate labels, tool actions, and command receipts resolve semantic i18n keys from the active runtime language; command names, paths, model/provider IDs, and protocol outcome/risk values remain untranslated.
- D0 aggregation is reversible, freezes at turn end, rejects late mutation, retains member/object/evidence indices, promotes warnings/failures, and scales linearly through 100,000 members.
- Transcript tool segments are a separate render-time layer: heterogeneous calls between visible LLM/user text share one container, preserve invocation order across out-of-order results, cross invisible internal tool rounds, and never lose older D0-hidden members when opened.
- Every settled consecutive non-Agent multi-tool segment collapses by default regardless of outcome or disclosure depth; running segments remain open. Alert state stays visible in the collapsed header, and expansion restores each member's structured details, including retained edit diffs. Group headers support mouse and keyboard toggles and resolve their labels from the active runtime language.
- Literal HTTP GET/HEAD shell reads no longer claim that they change state merely because they use the network. ANSI/control sequences are stripped before width bounding, terminal canvases are withheld from default detail while exact evidence remains retained, and the TUI root is opaque.
- Subagent presentation is run-aware: `agent_id` is separate from `run_id`, attempt, batch, parent, and breadcrumb path. Queue reasons, progress, duration, tokens, transcript evidence, attention, descendants, and prior runs are retained.
- Foreground Agent results preserve partial, timed-out, cancelled, and interrupted terminal states; resumed prompts carry the active run lineage; successful verification commands contribute redacted transcript anchors; crash reconciliation runs both at startup and while the TUI is alive.
- Decision requests use a keyed concurrent broker and persist exact prompt evidence. One visible overlay does not discard other waiters.
- Activity history uses stable insertion order rather than sorting the full history on every read, and write-through persistence backs off adaptively at large run counts while still flushing the latest revision on shutdown.
- Session presentation metadata v2 restores activities, old runs, acknowledgments, focus, viewport, and deterministic observation projection.

## Architecture after implementation

```text
tool/command/agent event
  -> structured facts + exact retained evidence
  -> PresentationPolicy (level, surfaces, reasons, aggregation eligibility)
  -> Formatter (semantic summary, object, metrics, bounded details)
  -> Same-intent Aggregator (D0 projection only; exact members remain retained)
  -> Transcript Segmenter (adjacent heterogeneous calls between visible text)
  -> TUI / classic / screen-reader projection
  -> JSON bypasses visual aggregation and retains every raw event
```

This preserves the report's central separation: policy decides how much must be visible; a formatter decides what is meaningful; aggregation changes only the projection; evidence retention is independent.

## Changed implementation areas

The implementation is concentrated in these areas:

- Policy and formatting: `tui/presentation_policy.go`, `tui/presentation_formatter.go`, and their table tests.
- Observation projection and aggregation: `tui/observation_store.go`, `tui/presentation_aggregate.go`, `tui/session_projection.go`, `tui/state.go`, `tui/root.go`. Nested MCP arguments are canonically hashed and LSP operation/location fields participate in the aggregation key.
- Transcript grouping and terminal containment: `tui/tool_segment.go`, `tui/root.go`, `tui/presentation_formatter.go`, semantic segment keys under `i18n/`, and background inheritance in the bundled `pkg/go-tui` renderer.
- Cross-surface semantics: `presentation_adapter.go`, `render.go`, `repl_screen_reader.go`, `ui/tool_presentation.go`, `ui/json_renderer.go`, and semantic adapter keys under `i18n/`. `SendUserMessage` JSON envelopes retain the complete structured content instead of only summary fields.
- Slash commands: `commands/presentation.go`, `commands/commands.go`, and domain-result reporting in session/config/init/resume/review/doctor/MCP/skills paths. `ResultMirrorsEvents` and `LegacyOutputForwarded` make the full legacy body and the typed receipt explicit owners instead of rendering the same payload twice.
- Subagents and lineage: `loop/tool_context.go`, `loop/query.go`, `tools/agent.go`, `tools/agent_contract.go`, `tools/agent_output_union.go`, `tools/agent_progress.go`, `tools/agent_sessions.go`, `tools/background_tasks.go`, `tools/runtime_task_store.go`.
- Activity, decisions, and persistence: `tui/activity_store.go`, `tui/decision.go`, `tui/renderer.go`, `repl_tui.go`, `session/session.go`.
- Verification: formatter/policy/aggregation/renderer/agent/queue/decision/session/accessibility tests and this implementation task directory.

The worktree also contains unrelated user-owned changes in fork, skills, language, cost, local file history, and backup-file cleanup. They were preserved and are not represented as display-strategy accomplishments merely because they share files or test runs.

## Simplifications made

- Reused ObservationStore, DetailStore, ActivityStore, Decision, transcript search/export, and session projection instead of building a second presentation database.
- Added pure policy/formatter boundaries instead of extending renderer-specific conditionals.
- Used typed metadata and explicit domain callbacks rather than natural-language parsing.
- Kept complete command `OnEvent` evidence while preventing typed receipts from replaying its bounded copy; long results remain available instead of being sacrificed for deduplication.
- Kept D0 as a reversible projection state; no raw observation or evidence deletion path was introduced.
- Kept transcript segments independent from D0 rather than overloading family/intent aggregates. Segment membership is derived from stable message anchors on every render, so no second persistence database or result-order list exists.
- Forms the complete segment before viewport and hidden-member projection; an opened group reuses the original observation/evidence rendering path for each member.
- Replaced an O(N²) aggregate duplicate scan with incremental representative updates and stable indices.
- Replaced per-read activity-history sorting with a stable append-order index and added adaptive persistence intervals for large histories.
- Reused the retained transcript/evidence model for verification references and command sections rather than introducing another artifact store.
- Kept screen reader and JSON as distinct output protocols while sharing semantic facts.
- Routed every registered screen-reader command and alias through either its surface-specific implementation or the typed registry; unavailable capabilities now fail explicitly rather than appearing as unknown commands.
- Added no dependency and created no compatibility-breaking replacement of the existing command or tool interfaces.

## Command and subagent behavior

Routine, low-risk reads/searches/web/MCP successes can become hidden same-intent aggregate members only after a visible representative exists. Independently, every consecutive non-Agent multi-tool run shares one transcript container even when member families, outcomes, and disclosure depths differ. Warnings, failures, integrity problems, decisions, and side effects retain their member-level policy inside the expanded container; settled abnormal segments collapse to an alert-bearing header, while Agent/Task lifecycle cards remain independent segments.

Subagent rows are ordered by actionability: needs input, ready for review, failed/timed out, running, completed, then ordinary cancelled. The latest run drives the current row; older attempts remain addressable through retained run history and transcript evidence. Queue state names `capacity:agent_session_worker` or `dependency:active_run` instead of presenting an unexplained spinner.

## Verification summary

The original task 01-06 automated gates passed at the stable snapshot recorded below:

- Full tests and full race tests.
- Vet, native build, and diff/format checks.
- Linux/amd64, Windows/amd64, and Darwin/amd64 cross-builds.
- macOS PTY and tmux pane execution.
- 40/80/120 columns, CJK, long paths, emoji/ZWJ/combining/ambiguous-width strings, and dynamic resize.
- 100,000 aggregation members, lossless JSON, append-only screen reader, run history, queue reasons, concurrent decisions, and session v2 restore.
- Forty-one logical production-tool wire-shape fixtures, including top-level MCP resource arrays, nested MCP contents and arguments, WebSearch seconds/count metadata, LSP location fields, TaskOutput offsets, and PowerShell process receipts.
- Exact foreground Agent incomplete outcomes, resume lineage, post-start crash reconciliation, verification transcript anchors, reverse-order same-agent decisions, and 100,000 activity updates with adaptive write-through.

Task 07 additionally passed its complete scoped gates: real invocation/result sequencing, reverse result completion, 40/80/120-column buffers, group mouse and keyboard interaction, active-language headers, read-only/mutating HTTP classification, ANSI/control removal, terminal-canvas containment, opaque background cells, full `tui` + `i18n` tests and race tests, bundled `go-tui` full + race tests, cross-platform `tui` builds, macOS PTY, tmux, vet, formatting, and diff checks.

A fresh full-root run during task 07 was not green because unrelated concurrent shared-worktree migrations changed many exact user-visible strings and runtime/session internals while their tests were still being updated. Task-owned packages were green in the same runs. The failing packages and expectations are recorded as a shared-worktree verification gap rather than misattributed to transcript grouping.

Exact scope and Not-tested rows are in [`TERMINAL_MATRIX.md`](TERMINAL_MATRIX.md).

## Cost

The original estimate remains the correct planning baseline:

| Scope | Low | Base | High |
| --- | ---: | ---: | ---: |
| Full behavioral alignment | 24 | 42 | 67 |
| Production hardening, cumulative | 38 | 63 | 100 |

These are forecast person-days. Actual person-days are `null`: the session has token/tool telemetry but no authoritative human-time ledger, so reporting zero or back-solving hours would be false precision. The six implementation task files allocate the full-alignment estimate as 4/8/14, 6/10/16, 2/4/6, 2/4/7, 5/8/13, and 5/8/11, which sums exactly to 24/42/67.

The screenshot-driven transcript-segment and terminal-containment remediation is tracked separately as `task_07`, with an incremental forecast of 2/4/6 person-days (low/base/high). Combined planning totals are therefore 26/46/73 person-days; actual person-days remain unknown rather than inferred from agent telemetry.

## Remaining risks

- Native Linux and Windows interaction was not executed on those operating systems; their compile/link gates passed.
- IDE terminal, VoiceOver, and NVDA require manual certification on hosts that provide them.
- The live six-agent behavior is covered by deterministic mixed-state tests, not a provider-backed multi-hour soak.
- Slow-terminal backpressure, long live-tail/pin soak, and deployed rollback remain production-operations work. Activity persistence still writes a full snapshot when it flushes; adaptive timing bounds frequency but does not turn it into an incremental log.
- New tools, slash commands, outcome enums, and persistence versions must extend the catalog/migration tests or consciously accept the fail-safe fallback.
- The shared dirty worktree can continue changing after this report. The implementation began from `7c6eea019e6f31449149770e986face7edd4d9ec`; the final verification snapshot is recorded in `TERMINAL_MATRIX.md` and includes the current uncommitted audit patch on top of the shared branch.
- The current shared-worktree full-root suite contains unrelated failures from concurrent i18n/runtime/session edits. Do not treat task 07's scoped green gates as a claim that those other changes are release-ready.

## Completion statement

All seven implementation tasks are complete at the **behavioral alignment** level. Tasks 01-06 retain their recorded stable-snapshot full verification; task 07 has full scoped automated verification and an explicit current-shared-worktree full-suite gap. Production hardening is intentionally marked partial because unavailable native/manual environments and unrelated concurrent failures were not fabricated as passing evidence.
