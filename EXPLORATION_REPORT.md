# Go Claude Code Rewrite: Exploration Report
**Date:** April 6, 2026 | **Repository:** /Users/buthim/Develop/claude-code/gosrc

---

## AREA 1: SDK Tool Approval Callback

### Current State: How PermissionHandler Works

**Files Analyzed:**
- `sdk/transport.go` (lines 1-504)
- `sdk/permission.go` (lines 1-107)
- `engine/permission.go` (lines 1-84)
- `permissions/engine_adapter.go` (lines 1-37)
- `permissions/permissions.go` (lines 1-210)

#### Internal PermissionHandler Interface (engine/permission.go:29-31)
```go
type PermissionHandler interface {
    Check(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
}
```

**Key Facts:**
- `PermissionRequest` (lines 21-27): Contains `SessionID`, `ToolName`, `Input` (map[string]any), `Description`
- `PermissionDecision` enum (lines 9-19): `PermissionAllow`, `PermissionDeny`, `PermissionAllowOnce`
- Three built-in implementations exist:
  1. **AllowAllHandler** (line 34-39): Unconditional allow
  2. **CLIPermissionHandler** (permissions/engine_adapter.go): Interactive terminal prompts
  3. **SDKPermissionHandler** (sdk/permission.go:64-106): SDK-aware callback system

#### SDK Permission Bridge Architecture (sdk/permission.go)

The SDK exposes a **permissionBridge** that:
1. **Registers** pending permission challenges with unique request IDs
2. **Sends** can_use_tool JSON over stdout to the SDK client
3. **Waits** on a channel for the client's response
4. **Delivers** the decision back to the waiting query goroutine

**Critical Flow (sdk/transport.go:269-289):**
```go
func (s *SDKServer) handleSetPermissionMode(req SDKControlRequest) error {
    // SDK client can switch modes: "default", "plan", "auto-edit", "full-auto"
    if pmReq.Mode == "full-auto" {
        s.eng.SetPermission(engine.AllowAllHandler{})
    } else {
        s.eng.SetPermission(s.permissionHandler())  // SDK bridge handler
    }
    return s.sendControlSuccess(req.RequestID, nil)
}
```

#### SDK Consumer Interface Exposed

**Problem: Limited Control**
- SDK consumers get **no way to provide custom approval callbacks**
- Only choices: "full-auto" (AllowAllHandler) OR SDK protocol (can_use_tool round-trip)
- If an SDK consumer wants custom logic (e.g., "auto-approve read-only ops"), they must:
  1. Receive can_use_tool over stdout
  2. Implement approval logic on their side
  3. Send can_use_tool response back over stdin

**Missing:** Direct callback registration like:
```go
// This doesn't exist:
sdk.SetApprovalCallback(func(req engine.PermissionRequest) (engine.PermissionDecision, error) {
    // custom logic
})
```

#### What Needs to Change

**sdk/transport.go + SDK public API:**
1. **Add new control request subtype** `set_approval_callback_mode` with options:
   - `auto_callback`: Use SDK's internal callback instead of protocol round-trip
   - `protocol_callback`: Current behavior (can_use_tool messages)

2. **Add new handler type** in SDK that wraps a user-provided callback:
   ```go
   // Pseudocode
   type CallbackPermissionHandler struct {
       fn func(req engine.PermissionRequest) (engine.PermissionDecision, error)
   }
   ```

3. **Export the callback registration interface** so SDK clients can inject approval logic without JSON protocol overhead

4. **Thread the callback through** `SetPermission()` path so engine respects custom decisions

**Impact:** SDK consumers could register a Go function once instead of round-tripping every tool decision through stdin/stdout JSON serialization.

---

## AREA 2: Sprint 5 Wiring into main.go/repl.go

### Status Summary: MOSTLY UNCONNECTED

#### 2a. input/ package (multiline reader)

**Files:** `input/multiline.go` (lines 1-111), `input/reader.go` (lines 1-147)

**Current State:**
- ✅ **Implemented & Working**: MultilineReader detects pastes, handles backslash continuation
- ❌ **NOT WIRED**: Used only in `input/reader.go` (line 64) if `MultilineEnabled` is set
- ❌ **NOT WIRED**: `MultilineReader` never instantiated in main.go or repl.go

**Finding:**
- `repl.go` line 191-201: Uses hardcoded `readline.NewEx()` from chzyer package
- **No reference to** `input.NewReader()` or `MultilineEnabled` option anywhere in main flow
- `input/reader.go:NewReader()` is **dead code in the REPL context**

**What's Missing:**
- Line in main.go after creating readline instance: Should wrap with input.Reader
- Or: Replace readline usage entirely with input.Reader (which wraps readline)
- Example fix location: main.go line 191 should be:
  ```go
  // OLD:
  rl, err := readline.NewEx(&readline.Config{...})
  
  // NEW:
  reader, err := input.NewReader(input.ReaderOpts{
      Prompt: r.Prompt(),
      MultilineEnabled: true,
  })
  ```

#### 2b. permissions/risk.go → ClassifyRisk()

**File:** `permissions/risk.go` (lines 1-269)

**Current State:**
- ✅ **Implemented**: Comprehensive risk classification (RiskLow/Medium/High)
- ✅ **Referenced once**: `permissions/rich_prompt.go:55` calls `ClassifyRisk(toolName, input)`
- ❌ **NOT USED in permission flow**: RichPrompt.ask() calls it, but RichPrompt is only used when:
  - `permissions/permissions.go:106-107` sets up an interactive prompt (if --allow-all is NOT set)
  - Even then, it's only used in CLI mode, not in REPL queries

**Finding:**
- ClassifyRisk exists but the decision isn't **surfaced to the user during tool execution**
- No risk badge appears in terminal when a tool is about to run
- Risk levels aren't logged or displayed anywhere except in permission prompt

**What's Missing:**
- Hook into query execution to display risk level BEFORE tool runs
- Show something like: `[🔴 High] Running Bash... rm -rf /`
- Or integrate with permissions/store.go to auto-approve low-risk operations

#### 2c. ui/cost_tracker.go → CostTracker

**File:** `ui/cost_tracker.go` (lines 1-96)

**Current State:**
- ✅ **Implemented**: Tracks input/output tokens, cache usage, USD cost per turn
- ❌ **NOT INSTANTIATED**: No CostTracker created anywhere in codebase
- ❌ **NOT USED**: RecordTurn() never called from anywhere

**Finding:**
- `cost_tracker.go` is a complete, working implementation
- But it's orphaned: no main.go or repl.go integration
- `engine/` or `loop/` never calls RecordTurn() after each query

**What's Missing:**
- `main.go` line 137: After creating renderer, should also:
  ```go
  costTracker := ui.NewCostTracker(p.ModelID())
  ```
- Hook into event stream in REPL loop (repl.go line 211-217) to capture:
  - `loop.EventTurnEnd` → extract usage, call `costTracker.RecordTurn()`
  - Display total cost at end of session

#### 2d. ui/spinner.go → SpinnerStart during tool execution

**Files:** `ui/spinner.go` (lines 1-76), `ui/term_renderer.go:140-147`

**Current State:**
- ✅ **Partially Implemented**: 
  - Spinner class exists and works
  - `TermRenderer.SpinnerStart()` method exists (wraps Spinner.Start())
  - NoOp implementations for JSON/Quiet modes
- ❌ **NOT CALLED**: Never invoked during tool execution

**Finding:**
- `ui/term_renderer.go:140-147` defines `SpinnerStart()` returning a stop func
- But it's **never called** from the query execution path
- Query loop (loop/query.go) doesn't trigger spinners

**What's Missing:**
- Hook into `loop/query.go` tool execution (around line 700-750 for EventToolUse)
- Before tool runs, call: `spinner := r.SpinnerStart(toolName); defer spinner.Stop()`
- Currently only thing shown is: `⚡ Running {toolName}...` from ToolCall() method

#### 2e. ui/context_bar.go → ContextBar after turns

**File:** `ui/context_bar.go` (lines 1-70), `ui/term_renderer.go` has `ContextBar()` method

**Current State:**
- ✅ **Implemented**: FormatContextBar() renders a 20-char bar with token usage percentages
- ✅ **Interface defined**: Renderer interface includes `ContextBar(usedTokens, maxTokens int)`
- ❌ **NEVER CALLED**: Not invoked after turns complete

**Finding:**
- Function exists and is correct
- Renderer has the method (all 3 implementations: Term, JSON, Quiet)
- But `repl.go` (line 211-217) event loop never calls it

**What's Missing:**
- After `loop.EventTurnEnd`, extract context usage from engine or track it
- Call: `r.ContextBar(usedTokens, maxContextTokens)` before printing newline
- Location: repl.go around line 224

#### 2f. ui/buffered_writer.go → Wrapping stdout

**File:** `ui/buffered_writer.go` (lines 1-65)

**Current State:**
- ✅ **Implemented**: Batches writes, flushes at ~60fps
- ❌ **NOT USED**: stdout never wrapped with BufferedWriter
- ❌ **NOT WIRED**: Renderer always writes directly to os.Stdout

**Finding:**
- `ui/term_renderer.go:32` creates TermRenderer with `w io.Writer` parameter
- `main.go:137` passes `os.Stdout` directly: `r := ui.NewTermRenderer(os.Stdout)`
- Should wrap: `wrapped := ui.NewBufferedWriter(os.Stdout)`

**What's Missing:**
- main.go line 137-138:
  ```go
  // OLD:
  r := ui.NewTermRenderer(os.Stdout)
  
  // NEW:
  buffered := ui.NewBufferedWriter(os.Stdout)
  defer buffered.Close()
  r := ui.NewTermRenderer(buffered)
  ```

#### 2g. cli/pipe.go → IsInteractive() check

**File:** `cli/pipe.go` (lines 1-34)

**Current State:**
- ✅ **Implemented**: `IsInteractive()` correctly detects TTY on stdin/stdout
- ❌ **NOT USED**: Never checked anywhere

**Finding:**
- Function is correct and available
- Should be used to decide between: interactive readline vs. piped input handling
- But `main.go` never calls it

**What's Missing:**
- `main.go` around line 162 (before "Interactive REPL mode" comment):
  ```go
  // ── Interactive REPL mode ───────────────────────────────────────────────
  if !cli.IsInteractive() {
      fmt.Fprintf(os.Stderr, "Not running in interactive mode; reading from stdin\n")
      // Could fall back to non-interactive input handling or error
  }
  ```
- Or use it to switch input modes (currently always tries readline, which fails on piped input)

### Summary: Sprint 5 Gaps

| Component | Status | Gap |
|-----------|--------|-----|
| MultilineReader | ✅ Built | ❌ Not wired into repl.go |
| ClassifyRisk | ✅ Built | ❌ Result not displayed/acted upon |
| CostTracker | ✅ Built | ❌ Not instantiated or called |
| SpinnerStart | ✅ Built | ❌ Not called during tool exec |
| ContextBar | ✅ Built | ❌ Not called after turns |
| BufferedWriter | ✅ Built | ❌ Not wrapping stdout |
| IsInteractive | ✅ Built | ❌ Not checked before REPL |

---

## AREA 3: Multimodal/Image Input Support

### Current State: INFRASTRUCTURE ONLY

**Files Analyzed:**
- `engine/types.go` (lines 1-49): QueryRequest structure
- `loop/query.go` (lines 237-298): Message handling
- `sdk/types.go`: SDK message types
- `sdk/transport.go`: Message extraction

#### Type System Support

**Finding in loop/query.go (lines 488, 752, 765, 770, 852):**
- ContentType enum includes: `ContentTypeText`, `ContentTypeToolUse`, `ContentTypeThinking`
- **NO `ContentTypeImage`** referenced in query.go
- **NO image content blocks created** anywhere in query execution

**SDK Message Extraction (sdk/transport.go:464-503):**
```go
// extractText pulls plain text out of an API-format user message.
// Handles both {"type":"text","text":"..."} content blocks and bare strings.
func extractText(raw json.RawMessage) (string, error) {
    // Only extracts text content
    // Ignores images, if present
}
```

**Problem:**
- extractText() has NO image handling
- If client sends image in SDKUserMessage, it's silently dropped
- Only text is extracted and sent to the query loop

#### REPL Input Flow

**Finding in repl.go (lines 140-151):**
```go
line, err := reader.Readline()  // Returns string only
// ...
input := strings.TrimSpace(line)  // Plain string
```

**Problem:**
- Readline returns `string`, not multimodal content
- No way to pass image file paths or base64 data
- Engine's QueryRequest accepts only `Message: string` (engine/types.go:14)

#### API Support (Provider Layer)

**Provider Support:**
- Anthropic provider (provider/anthropic.go) handles images in Anthropic API
- OpenAI provider (provider/openai.go) handles images in OpenAI API
- But Claude Code layers never construct image blocks

#### Gaps to Close

**Missing for Image Support:**

1. **QueryRequest needs multimodal content:**
   ```go
   // Current:
   type QueryRequest struct {
       Message string
   }
   
   // Needed:
   type QueryRequest struct {
       Message string
       Content []types.ContentBlock  // Add this
   }
   ```

2. **REPL input handling needs image support:**
   - Detect `@image_path.png` syntax in user input
   - Load file, detect media type, encode as base64
   - Build ImageBlock and send to loop

3. **SDK message extraction needs image support:**
   ```go
   // Current: only text extracted
   // Needed: parse and forward image blocks
   // Handle: {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "..."}}
   ```

4. **Loop query execution needs image forwarding:**
   ```go
   // Current: q.messages = append(q.messages, types.UserMessage(userMessage))
   // Needed: Support image blocks in user messages
   ```

**Current Workaround:**
- Users can already pass images via SDK by sending raw JSON with image content blocks
- But REPL has NO way to input images

---

## AREA 4: Provider Auto-Detection

### Current State: EXPLICIT ENV VAR ONLY

**File:** `provider/env.go` (lines 1-212)

#### Current Provider Selection Logic (lines 20-34)

```go
func NewFromEnvWithOverrides(providerOverride, modelOverride string) (Provider, error) {
    providerType := providerOverride
    if providerType == "" {
        providerType = os.Getenv("PROVIDER")
    }
    
    // Special cases for enterprise providers
    if providerType == "" && os.Getenv("CLAUDE_CODE_USE_BEDROCK") == "1" {
        providerType = "bedrock"
    }
    if providerType == "" && os.Getenv("CLAUDE_CODE_USE_VERTEX") == "1" {
        providerType = "vertex"
    }
    
    // Default fallback
    if providerType == "" {
        providerType = "anthropic"
    }
}
```

#### Current Supported Providers

Lines 36-203 handle:
- `anthropic` (default)
- `bedrock`
- `vertex`
- `openai` / `openai-responses`
- `ollama` (local)
- `deepseek`
- `gemini`
- `groq`
- `mistral`
- `oauth`

#### What Auto-Detection Would Need

**Missing Logic:**

1. **API Key Format Recognition:**
   - Anthropic keys start with `sk-ant-` (40+ chars)
   - OpenAI keys start with `sk-` (48 chars)
   - Gemini keys are UUIDs or long alphanumeric
   - DeepSeek keys start with `sk-` (different length than OpenAI)

2. **Environment Variable Sniffing:**
   ```go
   // Pseudocode for auto-detection:
   if os.Getenv("ANTHROPIC_API_KEY") != "" && apiKeyFormat(key) == "anthropic" {
       return "anthropic"
   }
   if os.Getenv("OPENAI_API_KEY") != "" {
       // Check key format to distinguish from others
       return "openai"
   }
   // ... etc for each provider
   ```

3. **API Endpoint Testing (Optional):**
   - Make small test request to infer provider
   - E.g., POST to `/v1/messages` for Anthropic
   - POST to `/chat/completions` for OpenAI
   - But this adds latency — not recommended

#### Current Behavior Without Auto-Detection

**Lines 189-202 (Anthropic fallback):**
```go
// If PROVIDER not set and no feature flags, defaults to anthropic
apiKey := os.Getenv("ANTHROPIC_API_KEY")
if apiKey == "" {
    return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is required (set PROVIDER=ollama for local models)")
}
```

**Problem:**
- If user has `OPENAI_API_KEY` set but forgets `PROVIDER=openai`, it fails with confusing message
- No intelligent detection of which provider is actually configured

#### What Would Need to Change

**In provider/env.go (lines 20-35):**

1. Add detection function:
   ```go
   func detectProviderFromEnv() string {
       // Check for API key presence + format
       // Return provider name or empty string
   }
   ```

2. Call it before fallback:
   ```go
   if providerType == "" {
       providerType = detectProviderFromEnv()
   }
   if providerType == "" {
       providerType = "anthropic"  // final fallback
   }
   ```

3. Add API key format validators:
   ```go
   func isAnthropicKey(key string) bool
   func isOpenAIKey(key string) bool
   func isGeminiKey(key string) bool
   // ... etc
   ```

#### Current Usable Features

- ✅ Explicit `PROVIDER=` env var (preferred method)
- ✅ Feature flags: `CLAUDE_CODE_USE_BEDROCK=1`
- ✅ Model overrides via CLI: `--provider openai --model gpt-4o`
- ❌ No automatic detection

#### Recommendation

Auto-detection should be **opt-in** (via flag like `--auto-detect-provider`) because:
1. It adds startup latency (env var parsing)
2. Could be confusing if multiple keys are set
3. Format detection alone can have false positives
4. Users should be explicit about provider choice for production

---

## Summary Table: What Needs Fixing

| Area | Component | Current | Gap | Priority |
|------|-----------|---------|-----|----------|
| 1 | SDK Approval Callback | ProtocolBased | NoCustomCallback | **HIGH** |
| 2 | MultilineReader | Built | NotWired | MEDIUM |
| 2 | ClassifyRisk | Built | NotUsed | MEDIUM |
| 2 | CostTracker | Built | NotCalled | LOW |
| 2 | Spinner | Built | NotCalled | LOW |
| 2 | ContextBar | Built | NotCalled | LOW |
| 2 | BufferedWriter | Built | NotWired | LOW |
| 2 | IsInteractive | Built | NotChecked | MEDIUM |
| 3 | ImageSupport | Missing | NoREPLInput | MEDIUM |
| 4 | AutoDetection | Missing | OptionalFeature | LOW |

---

## Critical Code Locations for Fixes

### main.go Changes Needed

**Line 137:** Wrap stdout with BufferedWriter
**Line 162:** Check IsInteractive() before REPL
**Line 191:** Use input.Reader instead of readline directly

### repl.go Changes Needed

**Line 211-217:** Call onEvent() to handle EventTurnEnd, display context bar, record cost
**Line 224:** Add context bar display after turn completes

### sdk/transport.go Changes Needed

**Line 49-286:** Add permission callback mode registration
**Line 464-503:** Add image block extraction to extractText()

