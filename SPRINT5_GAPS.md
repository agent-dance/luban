# Claude Code Go — Sprint 5 High-Value Feature Gaps

## Executive Summary
The Go rewrite is ~**785k LOC** with solid infrastructure (provider abstraction, tool registry, streaming loop, sessions, hooks, compaction) but **lacks end-to-end CLI usability** without key UX/rendering features. Most gaps are in **interactive input handling, permission UI, and rendering display quality**.

---

## 1. MISSING: Conversation Rendering & UI Display (⚠️ HIGH IMPACT)

### What TypeScript Has (146 components, Ink-based UI)
- **AgentProgressLine.tsx** — animated agent task execution progress
- **ContextVisualization.tsx** — full conversation context browser with cost/token stats
- **CompactSummary.tsx** — renders context compaction summaries with visual diffs
- **FileEditToolDiff.tsx** — structured diff viewer with syntax highlighting
- **FullscreenLayout.tsx** — full-screen TUI with interactive panes
- **Auto-collapsing tool results** — long output truncated with "expand" prompts
- **Cost visualization** — per-turn and cumulative cost tracking display
- **Thinking block rendering** — styled, dimmed, collapsible

### What Go Has (52 LOC ui/)
```go
TermRenderer {
  Text(s string)           // raw text dump
  Thinking(s string)       // dimmed text
  ToolCall(name, input)    // simple one-liner with icon
  ToolResult(content, isError) // indented, 20-line truncation
  Usage(u *types.Usage)    // token cache stats only
}
```

### Gaps
- ❌ **No interactive diff viewer** — file edits shown as raw text
- ❌ **No cost/token visualization** — Usage struct shows cache stats only, no per-turn breakdown
- ❌ **No context browser** — can't inspect full message history interactively
- ❌ **No progress indicators** — Agent/Team task execution shows no status
- ❌ **No auto-collapse for large results** — all 20 lines shown even if huge
- ❌ **Minimal thinking block rendering** — just dimmed text, no section headers
- ❌ **No structured output** — JSON/table rendering for tool results unimplemented

### Sprint 5 Action
Implement **progressive rendering** in `ui/term_renderer.go`:
1. Cost tracker integration with per-turn display
2. Diff viewer for FileEdit results (use `go-diff` library)
3. Progress bar for Agent/Team task execution
4. Expandable/collapsible sections for long results

---

## 2. MISSING: Interactive Input Handling (⚠️ MEDIUM-HIGH IMPACT)

### What TypeScript Has
- **Multiline input detection** — paste buffer detection, auto-expand textarea
- **Paste detection** — `pasteStore.ts` serializes large pastes to disk, inline references in history
- **Readline history** — session-aware command history with multiline support
- **Keybindings** — Ctrl+D to send, Ctrl+V to paste, Ctrl+X for cancel
- **Input validation** — reject commands that exceed token limits

### What Go Has
```go
readline.NewEx(&readline.Config{
  Prompt: r.Prompt(),
  HistoryLimit: 1000,
})
```
Just vanilla readline, no customization.

### Gaps
- ❌ **No multiline input** — single-line only (readline limitation)
- ❌ **No paste detection** — large clipboard pastes hang/crash
- ❌ **No keybinding customization** — can't override Ctrl+D behavior
- ❌ **No session-aware history** — history not persisted per-session
- ❌ **No input truncation warnings** — no token limit checks before sending

### Sprint 5 Action
Replace `readline` with **bubble tea** (`github.com/charmbracelet/bubbletea`):
1. Multiline text input with word wrapping
2. Paste detection + ResultStore integration for large inputs
3. Custom keybindings (Ctrl+Enter to send, Ctrl+X to cancel)
4. Session-aware history persistence

---

## 3. MISSING: Permission Prompts UI (⚠️ MEDIUM IMPACT)

### What TypeScript Has
- **Interactive permission dialogs** — React modals with Accept/Deny/Always buttons
- **Detailed risk indicators** — ⚠️ **Dangerous** badge for `rm -rf`, `sudo`, network calls
- **Command preview** — full command displayed before confirmation
- **Bypass modes** — --allow-all flag + UX opt-out modal

### What Go Has
```go
permissions.NewInteractivePrompt(os.Stderr, os.Stdin)
  // Reads "y/n" from stdin, writes prompt to stderr
```

### Gaps
- ❌ **No structured permission UI** — raw text prompts to stderr
- ❌ **No risk badges** — can't distinguish safe vs. dangerous operations
- ❌ **No "Always allow" memory** — one-off decisions not cached across operations
- ❌ **No bypass confirmation** — --allow-all doesn't show any warning dialog

### Sprint 5 Action
Implement **interactive permission handler** in `permissions/prompt.go`:
1. Use bubble tea for modal-style prompts
2. Add risk level classification (green/yellow/red)
3. Implement session-scoped caching (already in Checker struct)
4. Show confirmation when --allow-all is active

---

## 4. MISSING: Streaming Display & Token Counting (⚠️ MEDIUM IMPACT)

### What TypeScript Has
- **Real-time token counter** — shows `[...tokens]` as LLM streams
- **Cost-per-token display** — inline cost updates during streaming
- **Buffered output** — batches text updates to avoid flicker
- **Streaming state machine** — tracks "thinking", "text", "tool", etc. visually

### What Go Has
```go
loop.Event {
  Type: "text" | "thinking" | "tool_use" | "tool_result" | "turn_end" | "error"
  Text, ToolUse, ToolResult, Usage
}
// Each event handler writes directly to terminal
```

### Gaps
- ❌ **No token counter display** — Usage struct not shown until turn_end
- ❌ **No real-time cost updates** — can't show incremental cost as tokens arrive
- ❌ **No thinking block progress** — no indication of how long thinking will take
- ❌ **No stream buffering** — every event flushed immediately (causes terminal noise)

### Sprint 5 Action
Enhance `loop/query.go` event emission:
1. Add `EventTokenUpdate(count int)` for real-time token counting
2. Add `EventCostUpdate(usd float64)` for streaming cost display
3. Implement event batching in event handler (250ms debounce)
4. Update TermRenderer to show live token/cost meters

---

## 5. MISSING: Git Integration & Project Detection (⚠️ LOW-MEDIUM IMPACT)

### What TypeScript Has
- **`prompt.LoadGitContext()`** — parses `git status`, branch, remote URL
- **`projectDetection`** — detects project type (monorepo, typescript, python, etc.)
- **Auto-includes** — .git/config, package.json, setup.py in system prompt context
- **Branch awareness** — adds current branch to context

### What Go Has
```go
func LoadGitContext(cwd string) string {
  // Returns git status output raw
  // Doesn't parse branch, remote, or detect project type
}
```

### Gaps
- ❌ **No project type detection** — framework/language detection not implemented
- ❌ **No .gitignore parsing** — file suggestions not filtered by VCS ignore rules
- ❌ **No branch info** — branch name not included in system prompt
- ❌ **No remote tracking** — can't detect upstream/fork relationship
- ❌ **No auto-file inclusion** — doesn't scan for key files like package.json

### Sprint 5 Action (Lower priority — system works without it)
Expand `prompt/` package:
1. Implement `DetectProjectType()` — scan for package.json, go.mod, setup.py, etc.
2. Parse git branch and remote info
3. Load .gitignore rules for file filtering
4. Auto-include framework-specific context files

---

## 6. BROKEN/STUB IMPLEMENTATIONS

### Team/Coordinator (Partial)
```go
// swarm/team.go:45
// TODO: pass a real context from callers once the public API is updated.
```
- **Team.ExecuteTask()** accepts `context.Background()` instead of real context
- **dispatch_test.go** has boundary tests but production dispatch not fully wired

### Agent Depth Control (Stubbed)
```go
// tools/agent.go — depth param exists but not used
type AgentTool struct {
  Depth int  // set from registry but never enforced
}
```
- Agents can infinitely recurse without depth limiting
- No tests for max-depth enforcement

### Plan Mode (Partial)
```go
// tools/planmode.go — 109 LOC, basic structure
// Missing:
// - /plan command registration
// - Sub-task decomposition display
// - Task completion tracking
```

### Skill Tool (Partial)
```go
// tools/skill.go — 229 LOC
// Missing:
// - Skill registry discovery
// - Skill parameter schema inference
// - Skill execution result capture
```

---

## 7. WHAT MAKES THE GO BINARY NOT USABLE END-TO-END

| Blocker | Severity | Why |
|---------|----------|-----|
| **No multiline input** | HIGH | Users can't paste code; can't write long prompts |
| **Terminal rendering too minimal** | HIGH | No progress indicators, diffs, cost tracking; feels "incomplete" |
| **Raw permission prompts** | MEDIUM | UX feedback on what's dangerous is missing |
| **No project detection** | LOW | Still works, but less context-aware than TS version |
| **Streaming display noise** | MEDIUM | Real-time token counts missing; hard to track progress |

### Critical Path to "Usable CLI"
1. **Multiline input** + paste detection (2-3 days)
2. **Terminal rendering enhancements** (diff viewer, progress, cost) (2-3 days)
3. **Permission UI** + risk badges (1-2 days)
4. **Streaming display** + token counter (1 day)

---

## 8. WHAT'S SOLID (Reuse in Sprint 5)

✅ **Provider abstraction** — all backends working (Anthropic, OpenAI, Responses, Retry)  
✅ **Loop/query execution** — streaming, concurrent tools, compaction all working  
✅ **Tool registry** — 50+ tools implemented  
✅ **Session persistence** — save/restore working  
✅ **Hooks system** — pre/post tool instrumentation  
✅ **Permissions framework** — rule-based, interactive, feature gates all there  

### Build On These For Sprint 5:
- Wrap Renderer in bubble tea for interactive modals
- Add hooks for rendering lifecycle (PreRender, PostRender)
- Extend Usage struct for real-time token/cost tracking
- Use existing permissions.Checker for modal-based approval flow

---

## 9. SPRINT 5 ROADMAP (Prioritized by Impact)

### Must-Have (Blocking usability)
1. **Multiline input + paste detection** → replace readline with bubble tea
2. **Permission UI modal** → interactive dialog for tool approvals
3. **Rendering enhancements** → diff viewer, progress bar, cost display

### Should-Have (Polish)
4. **Streaming token counter** → real-time token/cost updates
5. **Plan mode command** → /plan slash command support
6. **Agent depth limiting** → enforce max recursion depth

### Nice-to-Have (Context quality)
7. **Project detection** → auto-detect framework/language
8. **Git integration** → branch awareness, remote tracking
9. **Skill registry** → full skill discovery and invocation

---

## Code Pointers for Sprint 5 Implementation

**Rendering pipeline:**
- `/Users/buthim/Develop/claude-code/gosrc/ui/term_renderer.go` (52 LOC, expand to 300+)
- `/Users/buthim/Develop/claude-code/gosrc/loop/query.go` (event types to extend)
- `/Users/buthim/Develop/claude-code/gosrc/render.go` (event handler)

**Input handling:**
- Replace `readline.NewEx()` in `/Users/buthim/Develop/claude-code/gosrc/repl.go` (line 186)
- Add bubble tea text input model

**Permissions:**
- Extend `/Users/buthim/Develop/claude-code/gosrc/permissions/prompt.go` (26 LOC)
- Add modal rendering + risk classification

**Streaming:**
- Extend `types/stream.go` Event types
- Add EventTokenUpdate, EventCostUpdate types
- Modify loop event emission in `/Users/buthim/Develop/claude-code/gosrc/loop/query.go`

