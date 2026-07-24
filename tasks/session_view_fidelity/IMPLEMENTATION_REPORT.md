# Session view fidelity implementation report

Status: completed at behavioral exactness; production hardening partial  
Date: 2026-07-16  
Task index: [`INDEX.json`](INDEX.json)

## Contract

The converged invariant is:

`same DurableSessionView + same RenderContext + same renderer/version => identical terminal Cells and equivalent action map`

`DurableSessionView` owns settled session presentation. `RenderContext` owns language, viewport, theme/palette, model catalog enrichment, provider connectivity, and other explicitly dynamic environment inputs. Active permission prompts, pickers, query cancellation, spinner phase, and new post-resume events are transient and are not restored as historical state.

## Implemented architecture

- Checkpoint v3 is the canonical resume source for capable sessions. It is keyed by the complete transcript digest, wrapped in a SHA-256 envelope, capability-marked, size-bounded, privately stored, atomically published, and fail-closed.
- `DurableSessionView` is embedded once by both `SessionSnapshot` and the checkpoint schema.
- Settled boundaries freeze the view through `App.UpdateSync`; evidence and file I/O run after the event-loop transaction.
- A monotonic per-session view sequence plus writer identity prevents older same-digest captures from overwriting newer checkpoints in one process and detects equal-sequence writer conflicts.
- Draft text, rune cursor, deterministic long-draft viewport, transcript anchor, scroll, provider/model, usage, cost-known state, goal, mode, decisions, activities, tasks, pending images, disclosure returns, and tool-group expansion are restored in one publication.
- `/resume`, `/fork`, and `/clear` no longer append navigation receipts into the target session.
- Fork separates immutable presentation labels from target operational observation/work identities, restores exact aggregate/action surfaces, copies all evidence, rejects stale picker snapshots, and rolls back incomplete forks.
- Session publication clears stale modal overlays and invalidates the previous frame's group hit map before the target frame is rendered.

## Simplifications

- Exact checkpoints bypass legacy projection/meta/journal parsing instead of overlaying several partial authorities.
- The renderer still consumes the same Root and semantic projection; no second resume-only renderer exists.
- Aggregate and activity controls are captured once. Current policy re-derivation is limited to legacy data.
- Cursor position shares the TextArea's reactive cursor state, covering every key, paste, history, and programmatic mutation without enumerating handlers.
- Newest-wins uses the existing content-addressed checkpoint rather than another side database.

## Cost

Incremental forecast: 8/15/28 person-days (low/base/high). The high case includes multi-process writer leasing, cross-file crash transactions, and unavailable native platform/manual certification. Actual human time is unknown and is not inferred from tool telemetry.

## Remaining risks

- A fork currently preserves source display paths while the model transcript may contain target physical artifact paths. A formal logical-display/physical-path type is still required before path actions are added to historical rows.
- Same-process writers are serialized and sequence-checked. Two independent processes editing the same session still require a repository-level writer lease to establish one authority.
- Transcript, meta, checkpoint, and manifest are separate files. Crash gaps fail closed, preserving correctness but not availability; a true cross-file transaction is larger storage work.
- Multiline paste payload mapping, active prompt-history navigation, and dismissed/selected slash autocomplete remain Root-local next-action state. Current Cell parity restores the visible draft/cursor, but those future interactions need a dedicated durable composer substate for literal behavioral identity.
- Changing language intentionally changes RenderContext. Current language-switch migration reprojects model-derived rows; it is not a claim of cross-language Cell identity.
- All available full/race/platform-build/PTY/tmux/visual gates are green. Native/manual and multi-process/storage-transaction items remain explicitly Not tested in [`TERMINAL_MATRIX.md`](TERMINAL_MATRIX.md).

## Verification outcome

- `go test ./... -count=1`: pass.
- `go test -race ./... -count=1`: pass.
- `go vet ./...`, `go build ./...`, and `git diff --check`: pass.
- Nested `pkg/go-tui` full race: pass.
- Linux/amd64, Windows/amd64, and Darwin/amd64 cross-builds: pass.
- macOS PTY and detached 80x24 tmux execution: pass.
- Visual-verdict: 100/100 pass. Live, resume, and fork PNGs have the same SHA-256 (`7aabad...e47`) and zero differing pixels.

The visual evidence is stored under `.omx/state/session-view-fidelity/visual/`; the authoritative verdict is `.omx/state/session-view-fidelity/ralph-progress.json`.
