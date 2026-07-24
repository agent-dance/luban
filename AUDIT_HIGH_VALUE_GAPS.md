# GO CLAUDE CODE AUDIT: HIGH-VALUE FEATURE GAPS
**Date:** April 6, 2026 | **Audit Type:** Quick comparison (TypeScript original vs Go rewrite)  
**Focus:** Daily-use features, shippability, integration completeness  
**Scope:** Identify gaps that would deliver maximum user value

---

## EXECUTIVE SUMMARY

The Go rewrite has solid infrastructure (92 commands mapped, 50+ tools, auth, hooks, MCP) but is **NOT shippable as daily replacement** because critical end-to-end flows exist in code but are **disconnected from the runtime loop**. Most gaps are integration/wiring issues, not missing implementations.

**Key Finding:** ~60% of implemented components are orphaned (never called from main execution path).

---

## TOP 5 HIGH-VALUE GAPS

### 1. **Interactive Dialog System** ⚠️ CRITICAL
**User Impact:** Cannot resume sessions, validate settings, or switch models  
**Implementation Status:** ~40% (components exist, not wired)  
**Blocking Shippability:** YES

**What's Missing:**
- `/resume` command returns error instead of launching session chooser dialog
- `/session` list/switch has no TUI (prints raw JSON)
- `/model` command doesn't exist (TypeScript has full model browser + switcher)
- No confirmation dialogs for risky operations
- No settings validation UI or onboarding flows

**Root Cause:**
- `ui/renderer.go` has `ShowDialog()` interface but never invoked
- `commands/resume.go` lacks dialog launcher
- REPL loop (`repl.go`) doesn't call interactive components
- No TUI dialog primitives implemented (select/confirm/input)

**TypeScript Reference:**
- `dialogLaunchers.tsx`: 7 specialized dialogs via Ink React
- `interactiveHelpers.tsx`: Full infrastructure for showDialog, exitWithError, etc.
- `components/`: 140+ interactive UI components

**Effort:** 8-12 hours
- Implement TUI primitives (termui/lipgloss: select menu, confirm, text input)
- Create session chooser: list/filter/preview sessions
- Wire into `commands/resume.go`, `commands/config.go`, model selection
- Add confirmation dialogs for destructive operations

---

### 2. **Plugin & Skill Management System** 🔌 HIGH
**User Impact:** No extensibility—cannot install plugins, reload skills, or manage extensions  
**Implementation Status:** ~20% (skill tool internal, no user commands)  
**Blocking Shippability:** YES

**What's Missing:**
- `/plugin` command (TypeScript has 9 files, ~7600 LOC dedicated to plugins)
  - No plugin discovery/marketplace browser
  - No install/remove/update workflow
  - No trust verification UI
- `/skills` command (list/reload/manage user skills)
- No plugin marketplace integration

**What Exists (Unused):**
- `tools/skill.go`: InvokeTool for running skills (internal only)
- `skills/skills.go`: Load & parse skills from disk
- `registry_setup.go`: Plugin registration infrastructure

**TypeScript Has:**
- `commands/plugin/`: Browse marketplace, discover, manage, settings, trust warnings
- `commands/skills/`: Full skills management command
- `commands/reload-plugins/`: Plugin reload on-demand
- Trust verification workflow with user approval

**Effort:** 12-16 hours
- Create `/plugin` command with subcommands: list, install, remove, marketplace, search
- Build TUI plugin browser (marketplace + installed plugins)
- Implement skill list/reload command
- Integrate plugin trust verification hook
- Add marketplace API client

---

### 3. **Model Selection & Switching** 🤖 HIGH
**User Impact:** Users stuck on single model—cannot explore or switch mid-session  
**Implementation Status:** 0% (hardcoded via CLI, no `/model` command)  
**Blocking Shippability:** YES

**What's Missing:**
- `/model` command (list available models, filter, switch per-session)
- Model browser UI with descriptions, cost, capabilities
- Per-session model selection persistence
- Model cost/capability display

**Current State:**
- `provider/models.go`: Full model catalog exists
- `cli/cli.go`: `--model` flag parsed
- `loop.QueryLoop.SetModel()`: Method exists but never called
- No mechanism to switch models after session start

**TypeScript Has:**
- `commands/model/`: Model selection browser, switcher, cost display
- Model filtering, preview, per-session override
- Integration with cost tracker

**Effort:** 6-8 hours (BEST ROI)
- Add `/model` command with subcommands: list, filter, switch
- Build model browser TUI (paginated list + preview)
- Wire `loop.SetModel()` call into command handler
- Add model metadata display (cost/capability specs)
- Persist selection to session metadata

---

### 4. **Telemetry & Analytics (GrowthBook Integration)** 📊 MEDIUM
**User Impact:** Product team cannot measure adoption, feature flags don't work, no A/B testing  
**Implementation Status:** 0% (TypeScript has full GrowthBook integration)  
**Blocking Shippability:** NO (but critical for production)

**What's Missing:**
- Event tracking: tool usage, session lifecycle, errors, user actions
- GrowthBook feature flag integration (gates for advisor, fast mode, agent swarms, etc.)
- A/B testing framework
- Analytics dashboard preparation

**TypeScript Has:**
- `services/analytics/growthbook.ts`: Full GrowthBook client, feature gates
- `services/analytics/index.ts`: Event logging with PII filtering
- Gates on: advisor mode, fast mode, agent swarms, effort levels
- Session analytics, cost tracking metrics

**Impact:**
- Cannot measure user adoption or feature engagement
- No feature flags (all users get same behavior)
- Cannot do A/B tests for UX improvements

**Effort:** 10-14 hours
- Add GrowthBook SDK integration
- Create event logging layer with PII filtering
- Wire events into tool/session/permission/error lifecycle hooks
- Add feature gate checks before major features

---

### 5. **Session Sync & Compaction Wiring** 💾 MEDIUM
**User Impact:** Sessions unbounded growth, fragile on crash, no cross-device sync  
**Implementation Status:** ~60% (compaction logic exists, not triggered; sync absent)  
**Blocking Shippability:** NO (degrades gracefully)

**What's Missing:**
- Auto-compaction trigger on session end/size threshold
- Remote session sync to Claude.ai
- Incremental session save (currently full rewrite each turn)
- Session metadata persistence

**Current State:**
- `compact/compact.go`: Full compaction logic implemented
- `session/session.go`: Writes entire message array after each turn (inefficient)
- No lifecycle hook for auto-compact
- No remote sync service

**TypeScript Has:**
- Auto-compact on session end if context > 70%
- Remote sync to Claude.ai via API
- Incremental message append, not full rewrite
- Session metadata backup

**Impact:**
- Sessions grow unbounded → memory/disk issues
- No incremental saves → fragile on crash
- Cannot resume from Claude.ai web
- Performance degrades as session ages

**Effort:** 8-10 hours
- Add lifecycle hook in `repl.go` after turn completes
- Trigger auto-compact if context usage > 70% of max
- Implement incremental message append (append-only log + background compaction)
- Add remote sync API client (Claude.ai sessions endpoint)

---

## SUMMARY TABLE

| Gap | Effort | User Impact | Blocks Shipping |
|-----|--------|------------|-----------------|
| **1. Interactive Dialogs** | 8-12h | Critical: can't resume/switch | ✅ YES |
| **2. Plugin/Skills System** | 12-16h | High: no extensibility | ✅ YES |
| **3. Model Switching** | 6-8h | High: single model only | ✅ YES |
| **4. Telemetry/Analytics** | 10-14h | Medium: visibility lost | ❌ NO |
| **5. Session Sync/Compact** | 8-10h | Medium: reliability | ❌ NO |
| **TOTAL** | **44-60h** | — | **2-3 weeks** |

---

## WHAT'S ALREADY WORKING ✅

**Core Infrastructure (No gaps):**
- ✅ Provider integration (Anthropic, OpenAI, Bedrock, Vertex) + retry logic
- ✅ 50+ tools (file, web, bash, git, lsp, etc.)
- ✅ Auth: OAuth PKCE, token store, middleware
- ✅ 18+ slash commands (paste, config, status, resume, review, permissions, etc.)
- ✅ Hooks: pre/post tool, HTTP, notification, session lifecycle, security hardening
- ✅ UI: Cost tracker, spinner, context bar, buffered writer, JSON/quiet renderers
- ✅ Input: Multiline, paste detection, clipboard images
- ✅ Permissions: Risk classification, sandbox awareness
- ✅ Swarm/Team: Tmux backend, mailbox, executor, team config
- ✅ Compaction: Message compaction, post-compaction filters
- ✅ MCP: Lifecycle, reconnect, health checks, SSE

**Wiring Issues (not missing implementations):**
- ❌ Cost tracker created but `RecordTurn()` never called
- ❌ Spinner methods exist but never invoked during tool execution
- ❌ Context bar renderer exists but never called after turns
- ❌ Risk classification exists but not displayed to user

---

## RECOMMENDED IMPLEMENTATION ORDER

**Priority 1: Model Switching (#3)** — 6-8h
- Highest ROI: unblocks immediate daily usage (today's blocker)
- Lowest complexity: reuses existing SetModel() method
- Direct user value: "Can't use a different model" → "Can switch anytime"

**Priority 2: Interactive Dialogs (#1)** — 8-12h  
- Enables session management (resume, context viewing)
- Makes `/session` command useful
- Prerequisite for plugin management UI

**Priority 3: Plugin/Skills System (#2)** — 12-16h
- Extensibility parity with TypeScript
- Multiple pieces: can parallelize with dialogs

**Priority 4: Analytics (#4) + Session Robustness (#5)** — 18-24h (parallel)
- Polish phase (non-blocking but important)
- Can run in parallel: different code paths
- Session robustness: add 2-3 lifecycle hooks
- Analytics: add GrowthBook client + event logging

---

## SHIPPABILITY VERDICT

**Current Status:** NOT READY FOR DAILY USE

**Blockers to fix before shipping:**
1. Model switching (immediate blocker: users need this daily)
2. Session resume dialog (usability blocker: can't easily switch sessions)
3. Plugin system (extensibility blocker: users need custom tools)

**After implementing #1-3:** Ready for daily replacement (with warnings)

**After implementing all 5:** Feature parity with TypeScript (production-ready)

---

## EFFORT SUMMARY

- **To Beta (Blockers #1-3):** 26-36 hours → **~1 week**
- **To Prod (All 5 gaps):** 44-60 hours → **~2-3 weeks**
- **Parallelizable:** #4 & #5 can run simultaneously with #1-3

**Recommended sprint structure:**
- Days 1-2: Model switching (#3) — quick win
- Days 3-4: Dialogs foundation + resume chooser (#1) — infrastructure
- Days 5-7: Plugin/skills commands (#2) — completion
- Days 8-10: Analytics (#4) + session robustness (#5) — parallel
