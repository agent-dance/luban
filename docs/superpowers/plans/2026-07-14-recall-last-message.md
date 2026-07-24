# Claude-Compatible Prompt History Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reproduce Claude Code 2.1.207's core Up/Down prompt-history navigation, including draft restoration, edited recalls, project/session ordering, and multiline persistence, with the requested Codex-style directional boundary snap before crossing history entries.

**Architecture:** A pure TUI navigator owns reversible history state, a JSONL input store owns durable project/session records, and the focused `slashAwareTextArea` routes boundary arrows into root callbacks before falling back to native cursor movement. `RootComponent` joins these units without changing message processing.

**Tech Stack:** Go 1.26, local `github.com/grindlemire/go-tui` replacement, standard library JSON/filesystem packages, standard `testing` package.

## Global Constraints

- Add no dependencies.
- Match official Claude Code 2.1.207 draft/history semantics while applying the requested directional boundary-snap refinement.
- Preserve modal, autocomplete, pending-image, and modified-arrow priorities.
- Keep the legacy readline history file unchanged; TUI multiline history uses JSONL.
- Cap recalled records at 100 and ignore malformed JSONL records.

---

### Task 1: Reversible In-Memory Navigation

**Files:**
- Create: `tui/input_history.go`
- Create: `tui/input_history_test.go`
- Modify: `tui/root.go`
- Modify: `tui/slash_aware_textarea.go`
- Modify: `pkg/go-tui/textarea.go`

**Interfaces:**
- `newPromptHistoryNavigator(entries []string) *promptHistoryNavigator`
- `(*promptHistoryNavigator).Previous(current string) (string, bool)`
- `(*promptHistoryNavigator).Next(current string) (string, bool)`
- `(*promptHistoryNavigator).Add(value string)`
- `(*promptHistoryNavigator).Replace(entries []string)`
- `(*promptHistoryNavigator).ResetNavigation()`
- `(*TextArea).CursorLine() int`, `(*TextArea).LineCount() int`, `(*TextArea).SetCursorPosition(int)`

- [x] **Step 1: Add failing behavior tests**

Create real `RootComponent` tests that submit `first` and `second`, then assert `Up -> second -> first` and `Down -> second -> original draft`. Add an edited-entry case and a width-4 wrapped-input case where the first Up only moves the cursor and the second Up recalls history.

- [x] **Step 2: Verify RED**

Run: `go test ./tui -run 'TestPromptHistory' -count=1`

Expected: FAIL because focused Up/Down only delegates to the base text area and no history state exists.

- [x] **Step 3: Implement the navigator and focused routing**

Use newest-first entries and `index == -1` for the live draft:

```go
type promptHistoryNavigator struct {
	entries []string
	index   int
	draft   string
	edits   map[int]string
}
```

On first `Previous`, save a non-whitespace draft; before every index change, save edits for the active history item. `Next` at index zero restores the saved draft. `Add` prepends a non-whitespace, non-consecutive value and resets navigation.

In `slashAwareTextArea`, invoke history Up only when `CursorLine() == 0`, and history Down only when `CursorLine() == LineCount()-1`. Keep slash suggestions and pending-image callbacks ahead of history; delegate to the base key otherwise.

- [x] **Step 4: Format and verify GREEN**

Run: `gofmt -w tui/input_history.go tui/input_history_test.go tui/root.go tui/slash_aware_textarea.go pkg/go-tui/textarea.go`

Run: `go test ./tui -run 'TestPromptHistory|TestSlashAwareTextAreaArrowKeysMoveSuggestions|TestRootPendingImagesCanBeSelectedAndDeleted' -count=1`

Expected: PASS.

### Task 2: Project/Session JSONL Persistence

**Files:**
- Create: `input/prompt_history.go`
- Create: `input/prompt_history_test.go`
- Modify: `tui/root.go`

**Interfaces:**
- `input.DefaultPromptHistoryPath() string`
- `input.PromptHistoryEntry{Display string, Timestamp int64, Project string, SessionID string}`
- `input.LoadPromptHistory(path, project, sessionID string, limit int) []PromptHistoryEntry`
- `input.AppendPromptHistory(path string, entry PromptHistoryEntry) error`

- [x] **Step 1: Add failing persistence tests**

Use `t.TempDir()` to assert that embedded newlines round-trip through JSONL, current-session records precede other records in the same project, other projects are filtered, malformed lines are ignored, the limit is enforced, and identical consecutive project/session records are written once.

- [x] **Step 2: Verify RED**

Run: `go test ./input -run 'TestPromptHistory' -count=1`

Expected: FAIL to compile because the prompt-history store API does not exist.

- [x] **Step 3: Implement the JSONL store and root integration**

Append one JSON object per line with file mode `0600`. Loading reads chronological records, walks backward, partitions current-session and other same-project entries, then truncates the combined newest-first list to 100. `RootComponent` lazily loads by current session, prepends submissions immediately, and appends the durable record before invoking `onSubmit`.

- [x] **Step 4: Verify persistence and integration**

Run: `gofmt -w input/prompt_history.go input/prompt_history_test.go tui/root.go`

Run: `go test ./input ./tui -count=1`

Expected: PASS.

### Task 3: Repository Verification

**Files:**
- Verify all files changed by Tasks 1-2.

- [x] **Step 1: Run static and behavioral checks**

Task-scoped tests, race detection, and vet pass. Repository-wide tests and vet
are currently blocked by unrelated dirty `tools/` changes with missing symbols;
all buildable packages, including `input` and `tui`, pass.

Run: `go test ./... -count=1`

Run: `go vet ./...`

Run the repository lint command if declared by `Makefile`, `Taskfile`, or CI configuration.

- [x] **Step 2: Review the final diff**

Confirm no generated files, unrelated dirty-worktree files, dependencies, Ctrl+R behavior, or queue-editing behavior entered the diff. Confirm all new state resets on submit and session change.

- [x] **Step 3: Commit with Lore trailers**

Stage only the two design documents and the prompt-history implementation/test files. The intent line explains recoverable prompt reuse; trailers record official 2.1.207 parity as the constraint, test commands, narrow scope risk, and excluded queue/search behavior.

### Task 4: Directional Boundary-Snap Refinement

**Files:**
- Modify: `pkg/go-tui/textarea.go`
- Modify: `pkg/go-tui/textarea_test.go`
- Modify: `tui/slash_aware_textarea.go`
- Modify: `tui/input_history_test.go`

- [x] **Step 1: Add failing tests for first/last-row snapping**

Assert that Up on the first row snaps to input start before recalling older
history, Down on the last row snaps to input end before recalling newer
history, and intermediate rows retain native vertical movement.

- [x] **Step 2: Verify RED**

The previous implementation immediately crossed history at the first/last
visual row, so the boundary-snap assertions fail.

- [x] **Step 3: Implement and verify the minimal boundary check**

Expose the clamped rune cursor offset from `TextArea`. In
`slashAwareTextArea`, snap only when already on the directional boundary row;
otherwise delegate to the native text-area movement.
