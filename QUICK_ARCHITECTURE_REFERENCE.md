# Quick Architecture Reference - Claude Code Go

## One-Page Startup Sequence

```
main() [main.go]
  ├─ cli.Parse() → Options
  ├─ provider.NewFromEnvWithOverrides(opts.Provider, opts.Model)
  │  └─ Returns: Provider interface (Anthropic, OpenAI, Responses, wrapped in RetryProvider)
  ├─ SetupRegistry(provider, cwd, allowedDirs) [registry_setup.go]
  │  └─ Returns: RegistryDeps{Registry, AgentTool, TeamManager, CronStore}
  ├─ prompt.BuildSystemPrompt() → string
  ├─ loop.New(provider, registry, config) → *QueryLoop
  │
  └─ if opts.Print:
      RunPrintMode(ql, query)
     else:
      RunTUIREPL(engine, store, session)  [repl_tui.go]
       └─ app.Run() → handleTUIInput() → ql.Run(userMsg, onEvent)
```

## Request-Response Flow (Single Turn)

```
User Message
   ↓
ql.Run(ctx, userMessage, onEvent)  [loop/query.go]
   ├─ Add user message to messages[]
   ├─ provider.CreateStream(Params{Messages, Tools, System, ...})
   │  └─ Returns: <-chan types.StreamEvent
   │
   ├─ Process stream:
   │  ├─ EventText → emit
   │  ├─ EventThinking → emit
   │  ├─ EventToolUse → collect
   │  │  └─ On stop_reason="tool_use":
   │  │     ├─ For each ToolUseBlock:
   │  │     │  ├─ tool := registry.Get(toolName)
   │  │     │  ├─ result := tool.Execute(ctx, input)
   │  │     │  └─ Add ToolResultBlock to messages[]
   │  │     └─ Loop back to provider.CreateStream()
   │  │
   │  └─ On stop_reason="end_turn":
   │     └─ Break loop, emit EventTurnEnd, return
   │
   └─ Return: Message, Usage, StopReason
      ↓
   Emit to TUI callback
      ↓
   Render to terminal
```

## Core Interfaces

### Provider
```go
interface Provider {
    Name() string
    ModelID() string
    CreateStream(ctx, Params) (<-chan StreamEvent, error)
}
```
**Implementations**: AnthropicProvider, OpenAIProvider, ResponsesProvider (all wrapped in RetryProvider)

### Tool
```go
interface Tool {
    Name() string
    Description() string
    Schema() JSONSchema
    Execute(ctx, input map[string]any) (ToolResult, error)
}
```
**25+ implementations**: File tools, Bash, Web, LSP, Agent, Team, Cron, Tasks, etc.

### Compactor
```go
interface Compactor {
    Compact(ctx, messages, budget) (compactedMessages, error)
}
```
**Implementation**: SummaryCompactor (uses LLM to summarize)

---

## File Organization by Responsibility

| Responsibility | Files | Key Types |
|---|---|---|
| **CLI & TUI** | main.go, cli/cli.go, repl_common.go, repl_tui.go, printmode.go, signals.go | Options, Event |
| **LLM Abstraction** | provider/*, types/* | Provider, Params, Message, ToolDefinition |
| **Agent Loop** | loop/query.go, loop/concurrent.go | QueryLoop, Config, Event |
| **Tool System** | registry/*, types/tools.go, tools/* | Registry, Tool, ToolResult |
| **Coordination** | coordinator/*, tools/team.go | Coordinator, Task, Agent, MessageBus |
| **Persistence** | session/*, compact/resultstore.go | FileStore, ResultStore |
| **Instrumentation** | hooks/ | Hook, Runner, HookOutput |
| **Context Mgmt** | compact/compact.go, compact/microcompact.go | ContextWindow, SummaryCompactor |

---

## Message Flow Through System

```
User Input
   ↓ (CLI)
cli.Parse() → Options
   ↓
provider.NewFromEnvWithOverrides()
   ↓
loop.New(provider, registry, config)
   ↓
ql.Run(userMessage)
   ├─ types.Message{Role: "user", Content: [TextBlock]}
   ├─ provider.CreateStream(types.Params{Messages, Tools, System})
   ├─ Receive types.StreamEvent via channel
   ├─ Emit loop.Event via callback
   ├─ On tool_use: registry.ExecuteTool() → types.ToolResult
   ├─ Add types.Message{Role: "assistant", Content: [ToolResultBlock]}
   └─ Loop until stop_reason != "tool_use"
   ↓
onEvent callback(loop.Event)
   ├─ render.Print(event)
   ├─ Optionally hooks.Fire(PostToolUse)
   └─ Update UI
```

---

## Key Function Signatures

### Provider Interface
```go
// Create streaming message (main entry point)
func (p Provider) CreateStream(ctx context.Context, 
                               params Params) (<-chan types.StreamEvent, error)

// Factory for provider from environment
func NewFromEnvWithOverrides(providerOverride, modelOverride string) (Provider, error)
```

### Loop Interface
```go
// Run one turn of the agentic loop
func (q *QueryLoop) Run(ctx context.Context, userMessage string, 
                        onEvent func(Event)) error

// Get/set message history
func (q *QueryLoop) Messages() []types.Message
func (q *QueryLoop) SetMessages(msgs []types.Message)

// Set result store for large tool outputs
func (q *QueryLoop) SetResultStore(rs *compact.ResultStore)

// Set session ID for Responses API
func (q *QueryLoop) SetSessionID(id string)
```

### Registry Interface
```go
// Register a tool
func (r *Registry) Register(tool types.Tool)

// Get tool by name
func (r *Registry) Get(name string) types.Tool

// Execute tool
func (r *Registry) ExecuteTool(ctx context.Context, name string, 
                               input map[string]any) types.ToolResultBlock
```

### Tool Interface
```go
func (t Tool) Name() string
func (t Tool) Description() string
func (t Tool) Schema() types.JSONSchema
func (t Tool) Execute(ctx context.Context, 
                      input map[string]any) (types.ToolResult, error)
```

---

## Provider Selection Logic (env.go)

| Env Variable | Override | Default | Provider Type | Retry Config |
|---|---|---|---|---|
| `PROVIDER=anthropic` | `--provider anthropic` | ✓ default | AnthropicProvider + RetryProvider | Standard |
| `PROVIDER=openai` | `--provider openai` | - | OpenAIProvider + RetryProvider | Standard |
| `OPENAI_USE_RESPONSES=1` | - | - | ResponsesProvider + RetryProvider | Standard |
| `PROVIDER=ollama` | `--provider ollama` | - | OpenAIProvider (local) + RetryProvider | Short (2 retries) |
| `PROVIDER=deepseek` | `--provider deepseek` | - | OpenAIProvider (deepseek.com) + RetryProvider | Standard |
| `PROVIDER=gemini` | `--provider gemini` | - | OpenAIProvider (Google compat) + RetryProvider | Standard |
| `PROVIDER=groq` | `--provider groq` | - | OpenAIProvider (Groq) + RetryProvider | Standard |
| `PROVIDER=mistral` | `--provider mistral` | - | OpenAIProvider (Mistral) + RetryProvider | Standard |

**Retry defaults**: 5 attempts, 100ms → 8s backoff, except local (2 attempts, 100ms → 1s)

---

## Message Types (types/messages.go)

| Type | Purpose | Key Fields |
|---|---|---|
| **TextBlock** | Plain text output | `Text: string` |
| **ThinkingBlock** | Extended thinking | `Thinking: string` |
| **ToolUseBlock** | Tool call | `ID, Name, Input map[string]any` |
| **ToolResultBlock** | Tool output | `ToolUseID, Content, IsError bool` |
| **ImageBlock** | Image input | `Source: {Type, URL/Base64/...}` |
| **UnknownBlock** | Unknown type | `Type, Raw json.RawMessage` |

---

## Tool Categories (tools/) - 25 Files

| Category | Count | Examples | File |
|---|---|---|---|
| **File I/O** | 5 | Read, Write, Edit, Glob, Grep | file_operations.go, search.go |
| **Shell** | 1 | Bash | shell_operations.go |
| **Web** | 2 | Fetch, Search | web.go |
| **Search** | 2 | Glob, Grep | search.go |
| **Agent/Team** | 2 | Agent, Team tools | agent.go, team.go |
| **LSP** | 1 | Language Server | lsp.go |
| **Cron** | 3 | Create, Delete, List | cron.go |
| **Tasks** | 2 | CRUD + TodoWrite | tasks.go, todowrite.go |
| **Worktree** | 2 | Enter, Exit | worktree.go |
| **Config** | 1 | Get/Set | config.go |
| **MCP** | 1 | MCP integration | mcp_tools.go |
| **Notebook** | 1 | Edit Jupyter | notebook.go |
| **Skill** | 1 | Skill mgmt | skill.go |
| **User Interaction** | 1 | Ask question | askuser.go |
| **Dangerous** | 1 | Password input | dangerous.go |
| **Misc** | 1 | Brief, ToolSearch, SyntheticOutput | misc.go |

---

## Hooks System (hooks/hooks.go)

**Hook Types:**
```
PreToolUse       → Before tool execution
PostToolUse      → After tool execution
SessionStart     → Session begins
SessionEnd       → Session ends
UserPromptSubmit → User input received
```

**Hook Flow:**
```
Hook (command string)
   ↓
Spawn subprocess
   ↓ stdin
HookInput{Type, ToolName, ToolInput, Result, UserInput}
   ↓ stdout
HookOutput{SystemReminder, Block, ModifiedInput, ExitCode, Stderr}
```

---

## Config Objects & Defaults

### cli.Options
```go
Model: ""                          // --model / -m
Provider: ""                       // --provider
Print: false                       // -p
Resume: false                      // --resume
SessionID: ""                      // --session-id
MaxTurns: 100                      // --max-turns
SystemPrompt: ""                   // --system-prompt
AllowedDirs: []string{}            // --allowed-dir (repeatable)
AllowAll: false                    // --allow-all
Verbose: false                     // --verbose
```

### loop.Config
```go
MaxTurns: 100                      // agentic turns limit
System: ""                         // system prompt
Model: ""                          // optional override
MaxTokens: 16384                   // output tokens per turn
MaxContextTokens: 200000           // triggers compaction
HookRunner: nil                    // optional hooks
AllowedDirs: []string{}            // file access restriction
SessionID: ""                      // Responses API affinity
ReasoningEffort: ""                // "low" | "medium" | "high"
```

### provider.Params
```go
Model: ""                          // model to use
MaxTokens: 0                       // output token limit
System: ""                         // single system block
SystemParts: []string{}            // multi-part system
Messages: []types.Message{}        // message history
Tools: []types.ToolDefinition{}   // available tools
ToolChoice: nil                    // "auto" | "any" | "tool"
Thinking: nil                      // extended thinking config
Conversation: ""                   // Responses API session
PreviousResponseID: ""             // Responses API chaining
PromptCacheKey: ""                 // cache affinity key
ReasoningEffort: ""                // reasoning model effort
```

---

## Error Semantics

**Tool-Level Error (LLM sees it):**
```go
return types.ToolResult{
    Content: "File not found: /tmp/missing.txt",
    IsError: true,  // ← tells LLM this was a business error
}, nil
```

**Infrastructure Error (aborts loop):**
```go
return types.ToolResult{}, fmt.Errorf("context cancelled")
// ← Go error means something unexpected happened
```

---

## Environment & Authentication

**Minimal Setup** (Anthropic):
```bash
export ANTHROPIC_API_KEY=sk-ant-...
./prc-code
```

**OpenAI Setup:**
```bash
export PROVIDER=openai
export OPENAI_API_KEY=sk-...
./prc-code
```

**Local Ollama:**
```bash
export PROVIDER=ollama
export OLLAMA_MODEL=llama2
export OLLAMA_BASE_URL=http://localhost:11434/v1
./prc-code
```

**Multiple Providers (CLI Override):**
```bash
./prc-code --provider openai --model gpt-4o
./prc-code --provider gemini --model gemini-2.5-pro
```

---

## Directory Purpose Summary

| Dir | Purpose | Hot Path? |
|---|---|---|
| `cli/` | Argument parsing | No |
| `provider/` | LLM backend abstraction | **Yes** |
| `loop/` | Agentic tool-use loop | **Yes** |
| `types/` | Data structures | **Yes** (marshalling) |
| `registry/` | Tool registry & dispatch | **Yes** |
| `tools/` | 25+ tool implementations | Yes (execution) |
| `coordinator/` | Multi-agent task queue | No |
| `compact/` | Context compaction | Maybe (if MaxContextTokens) |
| `hooks/` | Instrumentation system | No |
| `session/` | Session persistence | No |
| `prompt/` | System prompt building | No |
| `commands/` | Built-in commands | No |
| `mcp/` | Model Context Protocol | No |
| `render/` | Terminal rendering | No |

---

## Key Dependencies

**go.mod:**
```
github.com/anthropics/anthropic-sdk-go v1.30.0  # Anthropic API
github.com/sashabaranov/go-openai v1.41.2       # OpenAI SDK
github.com/chzyer/readline v1.5.1               # Interactive readline
github.com/creachadair/jrpc2 v1.3.5             # JSON-RPC 2.0
github.com/fatih/color v1.19.0                  # Terminal colors
github.com/bmatcuk/doublestar/v4 v4.10.0        # Glob patterns
github.com/pkoukk/tiktoken-go v0.1.8            # Token counting
```

---

## Shutdown Sequence

```
os.Interrupt or SIGTERM
   ↓
signals.Listen()
   ↓
globalCancel()  [context cancellation]
   ↓
REPL loop exits
   ↓
defer deps.CronStore.Stop()
   ↓
Session saved to FileStore
   ↓
Process exits
```

---

## Performance Hints

1. **Streaming**: Provider streams events in real-time, no buffering entire response
2. **Concurrency**: Tool execution is concurrent (loop/concurrent.go)
3. **Registry**: O(1) tool lookup (map-based)
4. **Compaction**: Optional, triggered only if MaxContextTokens set
5. **Caching**: Responses API maintains session affinity for cache reuse

---

## Common Extension Points

To add a new tool:
```go
// In tools/mytool.go
type MyTool struct{ /* config */ }
func (t *MyTool) Name() string { return "MyTool" }
func (t *MyTool) Description() string { return "..." }
func (t *MyTool) Schema() types.JSONSchema { return ... }
func (t *MyTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
    // Implement
}

// In registry_setup.go
reg.Register(&MyTool{})
```

To add a new provider:
```go
// In provider/myprovider.go
type MyProvider struct{ /* config */ }
func (p *MyProvider) Name() string { return "myprovider" }
func (p *MyProvider) ModelID() string { return p.model }
func (p *MyProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
    // Implement streaming
}

// In provider/env.go, add case in NewFromEnvWithOverrides()
case "myprovider":
    // Create and return MyProvider
```
