# Prompt History Navigation with Boundary Snapping

## Goal

Match the core prompt-history behavior of official Claude Code 2.1.207 in the Go TUI, with a Codex-style boundary-snapping refinement requested after the initial implementation. Empty-composer Up Arrow recall remains the first state transition in a reversible Up/Down history navigator, not a one-off copy action.

## Source Evidence

- Official Claude Code 2.1.207 maps Chat `up`/`down` to `history:previous`/`history:next`.
- Its input engine enters history on Up only from the first visual row and on Down only from the last visual row; wrapped rows count as visual rows.
- Its history state saves a non-empty draft on the first Up, preserves edits made to recalled entries, and restores the draft when Down returns to the live position.
- Up places the cursor at the recalled entry's end. Down places it at the newer entry's start.
- `~/.claude/history.jsonl` stores multiline display text with project and session metadata. Recall filters to the current project, prioritizes the current session, limits navigation to 100 records, and suppresses consecutive duplicate submissions.
- Official documentation: `https://code.claude.com/docs/en/interactive-mode.md`, especially keyboard-shortcut lines 38-40 and command-history lines 212-239 as retrieved on 2026-07-14.
- Local official artifact: `/opt/homebrew/lib/node_modules/@anthropic-ai/claude-code/bin/claude.exe`, version 2.1.207, build 2026-07-10, signed by Anthropic PBC.

## Interaction Contract

- With an empty composer, unmodified Up recalls the newest eligible prompt.
- Repeated Up walks toward older prompts; Down walks toward newer prompts.
- Down from the newest recalled prompt restores the draft that existed before navigation, or an empty composer if there was no draft.
- Edits to a recalled prompt survive Up/Down navigation until submission or navigation reset.
- In multiline or wrapped input, Up/Down keeps native vertical movement on intermediate visual rows.
- On the first visual row, Up first snaps a non-zero cursor to the absolute input start; another Up enters older history.
- On the last visual row, Down first snaps a cursor before the text end to the absolute input end; another Down enters newer history.
- Shift/Ctrl/Alt-modified arrows keep their existing bindings and never trigger history.
- Modal overlays, expanded activity view, slash suggestions, and pending-image selection retain priority over history.
- Recalled slash commands do not immediately open a suggestion menu that blocks further history navigation.
- Submitting a non-whitespace prompt adds it to in-memory history immediately and persists it without blocking on model processing.
- Consecutive identical submissions for the same project/session create one persistent entry.

## Architecture

### Navigation state

Add a focused `promptHistoryNavigator` in `tui/input_history.go`. It stores recall-ordered prompt strings, a live position (`-1`), the pre-navigation draft, and temporary edits keyed by history index. It has `Previous`, `Next`, `Add`, `Replace`, and `ResetNavigation` operations independent of rendering.

### Persistence

Add a JSONL store in `input/prompt_history.go`, separate from the legacy plain-line readline file. Each record contains `display`, `timestamp`, `project`, and `sessionId`. Loading scans valid records, filters the current project, orders current-session records before other project records, returns newest first, and caps results at 100. Appending uses mode `0600` and suppresses a consecutive duplicate for the same project/session.

### Composer routing

`slashAwareTextArea` owns focused Up/Down dispatch because go-tui gives focused stop handlers priority over root preempt handlers. It keeps slash suggestions first, preserves native movement on intermediate rows, snaps to the directional boundary on the first/last row, then asks the root callback to navigate history on the following keypress.

Expose read-only cursor-line/line-count accessors and a bounded cursor setter from the locally replaced `pkg/go-tui/TextArea`. `RootComponent` uses them only through `slashAwareTextArea`.

### Root integration

`RootComponent` lazily loads history for the current project/session, records submissions before invoking the asynchronous submit callback, reloads when the session changes, and updates the composer plus cursor on navigation. It suppresses slash suggestions for the exact recalled value.

## Verification

Regression tests cover empty recall, multi-entry Up/Down, directional boundary snapping, native intermediate-row movement, draft restoration, edited-entry restoration, wrapped-row cursor precedence, slash/pending-image priority, modifier isolation, persistent multiline records, project filtering, current-session priority, corrupt-line tolerance, and consecutive duplicate suppression.

## Non-goals

- Ctrl+R reverse history search.
- Claude's queued-message editing layer while the assistant is working.
- Bash-mode-specific history filtering.
- History count labels and lazy 10-record paging.
- Paste-cache indirection for very large pasted payloads.
