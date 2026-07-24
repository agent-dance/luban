# Claude Code Go - Complete Architecture Overview

## Project Structure

```
gosrc/
├── main.go                    # Entry point, TUI bootstrap, signal handling
├── go.mod                     # Go 1.26.1, minimal deps: anthropic-sdk-go, go-openai, readline, jrpc2
├── registry_setup.go          # Tool registry initialization
├── repl_common.go             # Shared interactive helpers (session adapter, image parsing)
├── repl_tui.go                # Full-screen TUI event loop
├── render.go                  # Terminal rendering helpers
├── printmode.go               # Print mode (-p flag) execution
├── signals.go                 # Signal handler for Ctrl+C
├── session_setup.go           # Session management
│
├── cli/                       # CLI argument parsing
│   └── cli.go                # Options struct, Parse() using flag package
│
├── provider/                  # ABSTRACTED LLM backend (hot path)
│   ├── provider.go           # Provider interface, Params, ToolChoice, ThinkingConfig
│   ├── env.go                # NewFromEnvWithOverrides - provider factory
│   ├── anthropic.go (373 LOC)# AnthropicProvider - wraps anthropic-sdk-go
│   ├── openai.go (551 LOC)   # OpenAIProvider - OpenAI-compatible APIs
│   ├── responses.go          # ResponsesProvider - OpenAI Responses API (/v1/responses)
│   ├── retry.go              # RetryProvider - exponential backoff wrapper
│   ├── errors.go             # Provider error types
│   └── sse.go                # Server-sent events parsing
│
├── loop/                      # AGENTIC TOOL-USE LOOP (hot path, 765 LOC)
│   ├── query.go              # QueryLoop struct, Run(), processStream()
│   ├── query-loop.md         # Architecture documentation
│   ├── concurrent.go         # Concurrent tool execution
│   ├── errors.go             # Tool execution error handling
│   └── stream_test.go, concurrent_test.go, etc.
│
├── types/                     # Data type definitions
│   ├── tools.go              # Tool interface, ToolDefinition, JSONSchema
│   ├── messages.go           # Message, TextBlock, ToolUseBlock, ToolResultBlock, ThinkingBlock
│   ├── stream.go             # StreamEvent, ContentDelta, APIMessage, Usage
│   └── infrastructure.md
│
├── registry/                  # Tool registry & execution
│   ├── registry.go           # Registry struct - map[string]Tool + ordered slice
│   └── registry_test.go
│
├── tools/ (25 non-test files) # Tool implementations
│   ├── file_operations.go    # FileReadTool, FileWriteTool, FileEditTool
│   ├── shell_operations.go   # BashTool
│   ├── search.go             # GlobTool, GrepTool
│   ├── agent.go              # AgentTool - spawns sub-agents
│   ├── team.go               # TeamManager, SendMessageTool, TeamCreateTool
│   ├── lsp.go                # LSPTool - Language Server Protocol integration
│   ├── web.go                # WebFetchTool, WebSearchTool
│   ├── cron.go               # CronCreateTool, CronDeleteTool, CronListTool
│   ├── tasks.go              # Task management tools
│   ├── todowrite.go          # TodoWrite tool
│   ├── worktree.go           # EnterWorktreeTool, ExitWorktreeTool
│   ├── config.go             # ConfigTool
│   ├── mcp_tools.go          # MCP manager & tools
│   ├── notebook.go           # NotebookEditTool
│   ├── skill.go              # SkillTool
│   ├── askuser.go            # AskUserQuestionTool
│   ├── dangerous.go          # DangerousRunTool (password input)
│   ├── misc.go               # BriefTool, ToolSearchTool, SyntheticOutputTool, RemoteTriggerTool
│   └── helpers.go            # Utility functions
│
├── coordinator/              # Multi-agent task coordination
│   ├── coordinator.go        # Coordinator - task queue + agent dispatch
│   ├── session-orchestration.md
│   └── tests
│
├── compact/                  # Context window management & compaction
│   ├── compact.go            # SummaryCompactor - LLM-based summarization
│   ├── microcompact.go       # MicrocompactConfig - token budgeting
│   ├── post_compact.go       # Post-compaction file recovery
│   ├── resultstore.go        # ResultStore - oversized tool results
│   ├── context-compaction.md
│   └── tests
│
├── hooks/                    # Hook system for instrumentation
│   ├── hooks.go              # Hook types (PreToolUse, PostToolUse, SessionStart, etc.)
│   │                         # Runner, LoadFromSettings, LoadFromDir
│   └── hooks_test.go
│
├── session/                  # Session persistence
│   ├── session.go            # FileStore - session save/restore
│   └── session_test.go
│
├── prompt/                   # Prompt blocks, context injection, memory discovery
│   └── (system blocks, user/system context, CLAUDE.md/rules memory, cache scopes)
│
├── commands/                 # Built-in command handling
│   ├── builtins.go
│   └── commands.go
│
├── mcp/                      # Model Context Protocol
│   ├── mcp.go
│   └── extension-system.md
│
├── render/                   # Rendering engine
│   └── (terminal output)
│
└── permissions/              # File access permissions
    └── (interactive approval system)
```

---

## Core Startup Flow

### 1. **main()** → Initialization → TUI

```
main()
  ├─ cli.Parse()                          // Parse CLI flags
  ├─ provider.NewFromEnvWithOverrides()   // Create LLM backend
  │  └─ NewAnthropic() | NewOpenAI() | NewResponses() | NewRetryProvider()
  │
  ├─ SetupRegistry()                      // Register all tools (25+ built-in)
  │  ├─ File tools: Read, Write, Edit, Glob, Grep
  │  ├─ Shell: Bash
  │  ├─ Agent: AgentTool (sub-agents), TeamManager
  │  ├─ Cron: Create, Delete, List
  │  ├─ Web: Fetch, Search
  │  ├─ LSP: Language Server Protocol
  │  ├─ Config, Tasks, TodoWrite, Worktree, MCP, Notebook, Skill
  │  └─ Misc: Brief, ToolSearch, SyntheticOutput, RemoteTrigger
  │
  ├─ prompt.DiscoverMemoryFiles()         // CLAUDE.md / rules memory discovery
  ├─ prompt.UserContextBuilder            // claudeMd + currentDate meta user context
  ├─ prompt.SystemContextBuilder          // gitStatus system context
  ├─ prompt.BuildSystemPromptBlocks()     // static/dynamic system blocks
  ├─ prompt.ApplyCacheScopes()            // global/org/uncached block metadata
  │
  ├─ loop.New()                           // Create QueryLoop
  │  └─ Optionally enable context compaction if MaxContextTokens > 0
  │
  ├─ RunPrintMode() [if -p flag]          // Single query mode
  │  └─ exit(0)
  │
  └─ RunTUIREPL()                         // Interactive full-screen TUI
     ├─ tui.NewTUIApp()                   // go-tui app setup
     ├─ signal.Notify() → Ctrl+C handler
     └─ Per-user-input: handleTUIInput() → ql.Run()

```

### 2. **TUI Event Loop** → Query Execution

```
RunTUIREPL()
  └─ app.Run()
      └─ handleTUIInput(userMessage)
         ├─ HookRunner.Fire(PreToolUse)
         ├─ ql.Run(userMessage, onEvent)
         └─ HookRunner.Fire(SessionEnd)  [optional]

ql.Run(ctx, userMessage, onEvent callback)
  └─ Main agentic loop (up to MaxTurns, default 100)
     ├─ Add user message to messages[]
     ├─ Optionally compact context if needed
     ├─ Send to provider.CreateStream()
     ├─ Process stream events:
     │  ├─ EventText → emit to callback
     │  ├─ EventThinking → emit to callback
     │  ├─ EventToolUse → collect ToolUseBlock
     │  └─ On stop_reason="tool_use":
     │     ├─ For each tool call (concurrent or sequential):
     │     │  ├─ Execute tool via registry.ExecuteTool()
     │     │  ├─ Collect result
     │     │  └─ Add ToolResultBlock to messages
     │     ├─ If all results collected, loop back (next turn)
     │     └─ Emit EventToolResult
     │
     ├─ On stop_reason="end_turn" → exit loop, emit EventTurnEnd
     └─ Return accumulated Message + Usage stats
```

---

## Key Data Structures

### **Provider Interface** (`provider/provider.go`)
```go
type Provider interface {
    Name() string
    ModelID() string
    CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error)
}

type Params struct {
    Model              string
    MaxTokens          int
    System             string      // single system block (backward compat)
    SystemParts        []string    // legacy multi-part system with cache control
    SystemBlocks       []prompt.SystemPromptBlock // ordered blocks with metadata
    Messages           []types.Message
    Tools              []types.ToolDefinition
    ToolChoice         *ToolChoice // "auto", "any", or "tool"
    Thinking           *ThinkingConfig
    Conversation       string      // Responses API session ID
    PreviousResponseID string      // Responses API chaining
    PromptCacheKey     string      // Responses API cache affinity
    ReasoningEffort    string      // "low", "medium", "high"
}

type ToolChoice struct {
    Type string // "auto", "any", "tool"
    Name string // for Type == "tool"
}
```

### **Message Types** (`types/messages.go`)
```go
type Message struct {
    Role    Role           // "user" | "assistant"
    Content []ContentBlock // TextBlock, ToolUseBlock, ToolResultBlock, ThinkingBlock, ImageBlock, UnknownBlock
}

type TextBlock struct {
    Type string `json:"type"` // "text"
    Text string `json:"text"`
}

type ThinkingBlock struct {
    Type    string `json:"type"` // "thinking"
    Thinking string `json:"thinking"`
}

type ToolUseBlock struct {
    Type  string         `json:"type"` // "tool_use"
    ID    string         `json:"id"`
    Name  string         `json:"name"`
    Input map[string]any `json:"input"`
}

type ToolResultBlock struct {
    Type    string `json:"type"` // "tool_result"
    ToolUseID string `json:"tool_use_id"`
    Content string `json:"content"`
    IsError bool   `json:"is_error,omitempty"`
}

type ImageBlock struct {
    Type   string       `json:"type"` // "image"
    Source ImageSource  `json:"source"`
}
```

### **Tool Interface** (`types/tools.go`)
```go
type Tool interface {
    Name() string
    Description() string
    Schema() JSONSchema
    Execute(ctx context.Context, input map[string]any) (ToolResult, error)
}

type ToolResult struct {
    Content string `json:"content"`
    IsError bool   `json:"is_error,omitempty"` // business error (LLM sees it)
}
// Distinguish: (ToolResult{IsError: true}, nil) = business error
//              (ToolResult{}, err) = infrastructure error (aborts loop)
```

### **Tool Definition** (`types/tools.go`)
```go
type ToolDefinition struct {
    Name        string
    Description string
    InputSchema JSONSchema
}

type JSONSchema struct {
    Type        string         // "object", "string", etc.
    Properties  map[string]any // field name → {type, description}
    Required    []string       // required field names
    Description string
}
```

### **QueryLoop** (`loop/query.go`)
```go
type QueryLoop struct {
    provider            provider.Provider
    registry            *registry.Registry
    config              Config
    messages            []types.Message
    ctxWindow           *compact.ContextWindow        // nil if no compaction
    compactor           compact.Compactor
    toolBudget          *compact.ToolResultBudget
    microcompactCfg     compact.MicrocompactConfig
    resultStore         *compact.ResultStore          // oversized tool results
    calibratedCounter   *compact.CalibratedCounter    // nil if no compaction
}

func (q *QueryLoop) Run(ctx context.Context, userMessage string, 
                        onEvent func(Event)) error
func (q *QueryLoop) Messages() []types.Message
func (q *QueryLoop) SetMessages(msgs []types.Message)
func (q *QueryLoop) SetResultStore(rs *compact.ResultStore)
func (q *QueryLoop) SetSessionID(id string)
```

### **Registry** (`registry/registry.go`)
```go
type Registry struct {
    mu    sync.RWMutex
    tools map[string]types.Tool
    order []string // preserve registration order
}

func (r *Registry) Register(tool types.Tool)
func (r *Registry) Get(name string) types.Tool
func (r *Registry) All() []types.Tool
func (r *Registry) Definitions() []types.ToolDefinition
func (r *Registry) ExecuteTool(ctx context.Context, name string, 
                               input map[string]any) types.ToolResultBlock
```

---

## Provider Implementations

### **AnthropicProvider** (`provider/anthropic.go` - 373 LOC)
- Wraps `anthropics/anthropic-sdk-go`
- **Capabilities**: thinking ✓, toolUse ✓, cacheControl ✓, systemParts ✓, vision ✓, maxContext=200k
- **CreateStream**: Uses `anthropic.MessageNewParams`, converts Messages
- **Features**: 
  - Multi-part system prompts with cache control
  - Thinking blocks (extended thinking)
  - Tool use with structured input

### **OpenAIProvider** (`provider/openai.go` - 551 LOC)
- Wraps `sashabaranov/go-openai` (OpenAI SDK)
- **Supports**: OpenAI, Ollama, DeepSeek, Gemini (Google), Mistral, Groq (via OpenAI-compatible APIs)
- **Dialects**: Standard, Gemini, Mistral, Groq, DeepSeek, Ollama
- **Features**:
  - Custom base URLs (for local inference, alt providers)
  - Header injection & auth stripping for local servers
  - Tool use with function_call format conversion
  - Streaming via `net/http` with custom SSE parsing

### **ResponsesProvider** (`provider/responses.go`)
- OpenAI's Responses API (`/v1/responses`) with session affinity
- Maintains `lastResponseID` for chaining requests
- **Features**:
  - Conversation-aware caching
  - `previous_response_id` for context continuity
  - Prompt cache key for result reuse

### **RetryProvider** (`provider/retry.go`)
- Wraps any Provider with exponential backoff
- **Default retries**: up to 5, backoff 100ms → 8s max
- **529 retries**: up to 1 retry (overloaded)
- **Local inference** (Ollama): 2 retries, 100ms → 1s max

---

## Loop (Agentic Tool-Use Loop) - Hot Path

### **Query Loop Execution** (`loop/query.go` - 765 LOC)

**Config:**
```go
type Config struct {
    MaxTurns          int                  // max agentic turns (default 100)
    System            string               // system prompt
    SystemBlocks      []prompt.SystemPromptBlock // ordered system prompt blocks
    UserContext       prompt.UserContext   // claudeMd/currentDate meta user message
    SystemContext     prompt.SystemContext // gitStatus trailing system block
    Model             string               // optional model override
    MaxTokens         int                  // max output tokens per turn
    MaxContextTokens  int                  // triggers compaction if > 0
    HookRunner        *hooks.Runner        // optional instrumentation
    AllowedDirs       []string             // file access restrictions
    SessionID         string               // Responses API cache affinity
    ReasoningEffort   string               // "low" | "medium" | "high"
}
```

**Main Methods:**
```go
func (q *QueryLoop) Run(ctx context.Context, userMessage string, 
                        onEvent func(Event)) error
    // Main agentic loop: add user message, stream from provider,
    // process tool calls, collect results, loop until stop_reason != "tool_use"

func (q *QueryLoop) processStream(ctx context.Context, 
                                  stream <-chan types.StreamEvent,
                                  turnCount int,
                                  onEvent func(Event)) (*types.Message, *types.Usage, *types.StopReason, error)
    // Process provider stream events:
    // - Accumulate text/thinking blocks
    // - On tool_use: collect and execute tools
    // - Return complete message + usage stats

func (q *QueryLoop) forceTruncate(msgs []types.Message) []types.Message
    // Truncate message history to fit within MaxContextTokens
```

**Stream Events:**
```go
type Event struct {
    Type       EventType                // "text", "thinking", "tool_use", "tool_result", "turn_end", "error"
    Text       string                   // for text/thinking/error
    ToolUse    *types.ToolUseBlock      // for tool_use
    ToolResult *types.ToolResultBlock   // for tool_result
    Usage      *types.Usage             // for turn_end
    TurnCount  int                      // current turn number
}

type EventType string
const (
    EventText       EventType = "text"
    EventThinking   EventType = "thinking"
    EventToolUse    EventType = "tool_use"
    EventToolResult EventType = "tool_result"
    EventTurnEnd    EventType = "turn_end"
    EventError      EventType = "error"
)
```

### **Concurrent Tool Execution** (`loop/concurrent.go`)
- Executes tool calls concurrently (not sequentially)
- **Boundary testing** ensures safety in concurrent execution

---

## Provider/ Directory Details

| File | Purpose | LOC |
|------|---------|-----|
| `provider.go` | Provider interface, Params, ToolChoice, ThinkingConfig | ~80 |
| `env.go` | Provider factory (NewFromEnvWithOverrides) | 169 |
| `anthropic.go` | AnthropicProvider implementation | 373 |
| `openai.go` | OpenAIProvider for OpenAI-compatible APIs | 551 |
| `responses.go` | OpenAI Responses API provider | ~150 |
| `retry.go` | RetryProvider wrapper with exponential backoff | ~150 |
| `errors.go` | Error types & helpers | ~100 |
| `sse.go` | Server-sent events parsing | ~50 |

**Key Factory Logic** (`env.go`):
```go
func NewFromEnvWithOverrides(providerOverride, modelOverride string) (Provider, error) {
    // Determine provider: PROVIDER env, override, or default "anthropic"
    // Switch on provider type:
    //   "anthropic"      → NewAnthropic() + NewRetryProvider()
    //   "openai"         → NewOpenAI() + NewRetryProvider()
    //   "openai-responses" → NewResponses() + NewRetryProvider()
    //   "ollama"         → NewOpenAI(local) + NewRetryProvider(shortRetry)
    //   "deepseek"       → NewOpenAI(deepseek.com) + NewRetryProvider()
    //   "gemini"         → NewOpenAI(gemini compat) + NewRetryProvider()
    //   "groq"           → NewOpenAI(groq) + NewRetryProvider()
    //   "mistral"        → NewOpenAI(mistral) + NewRetryProvider()
    //   default          → Anthropic (requires ANTHROPIC_API_KEY)
}
```

---

## Tools/ Directory - Tool Implementations (25 files)

| Category | Tools | Files |
|----------|-------|-------|
| **File I/O** | Read, Write, Edit, Glob, Grep | file_operations.go, search.go |
| **Shell** | Bash | shell_operations.go |
| **Web** | WebFetch, WebSearch | web.go |
| **Search** | Glob, Grep | search.go |
| **Agent/Team** | Agent (sub-agents), Team tools, SendMessage | agent.go, team.go |
| **LSP** | LSP integration | lsp.go |
| **Cron** | Create, Delete, List jobs | cron.go |
| **Tasks** | Task CRUD, TodoWrite | tasks.go, todowrite.go |
| **Worktree** | Enter, Exit git worktree | worktree.go |
| **Config** | Get/set config | config.go |
| **MCP** | Model Context Protocol integration | mcp_tools.go |
| **Notebook** | Edit Jupyter notebooks | notebook.go |
| **Skill** | Manage OMC skills | skill.go |
| **User Interaction** | Ask user questions | askuser.go |
| **Dangerous** | Password input, dangerous operations | dangerous.go |
| **Misc** | Brief, ToolSearch, SyntheticOutput, RemoteTrigger | misc.go |

---

## Configuration & Initialization

### **CLI Options** (`cli/cli.go`)
```go
type Options struct {
    Model           string   // --model / -m
    Provider        string   // --provider
    Print           bool     // -p (print mode)
    Resume          bool     // --resume
    SessionID       string   // --session-id
    MaxTurns        int      // --max-turns (default 100)
    SystemPrompt    string   // --system-prompt
    AllowedDirs     []string // --allowed-dir (repeatable)
    AllowAll        bool     // --allow-all (skip permission prompts)
    Version         bool     // --version / -v
    Help            bool     // --help / -h
    Verbose         bool     // --verbose
    Args            []string // positional args (for -p mode)
}

func Parse() Options  // Parses os.Args[1:] with flag package
```

### **Registry Setup** (`registry_setup.go`)
```go
func SetupRegistry(p provider.Provider, cwd string, allowedDirs []string) *RegistryDeps {
    // Returns RegistryDeps:
    //   - Registry: map of all 25+ tools
    //   - AgentTool: reference for depth control
    //   - TeamManager: reference for system prompt wiring
    //   - CronStore: defer .Stop() to clean up
}
```

### **Hook System** (`hooks/hooks.go`)
```go
type HookType string
const (
    HookPreToolUse     HookType = "PreToolUse"
    HookPostToolUse    HookType = "PostToolUse"
    HookSessionStart   HookType = "SessionStart"
    HookSessionEnd     HookType = "SessionEnd"
    HookUserPromptSubmit HookType = "UserPromptSubmit"
)

type Hook struct {
    Type    HookType `json:"type"`
    Command string   `json:"command"`    // shell command to run
    Timeout int      `json:"timeout"`    // seconds, default 10
}

type Runner struct {
    hooks []Hook
}

func LoadFromSettings(settingsPath string) (*Runner, error)  // from .claude/settings.json
func LoadFromDir(dirPath string) (*Runner, error)            // from .claude/hooks/
func (r *Runner) Merge(other *Runner) *Runner
func (r *Runner) Fire(ctx context.Context, hookType HookType, input HookInput) (*HookOutput, error)
    // Executes hooks, captures stdout/stderr, parses JSON response
```

### **Session Management** (`session/session.go`)
```go
type FileStore struct {
    basePath string  // usually ~/.claude/sessions/
}

func NewFileStore(basePath string) *FileStore
func (fs *FileStore) Save(sessionID string, data SessionData) error
func (fs *FileStore) Load(sessionID string) (SessionData, error)
```

---

## Coordinator (Multi-Agent)

### **Coordinator** (`coordinator/coordinator.go`)
```go
type Coordinator struct {
    mu         sync.Mutex
    tasks      []*Task
    agents     map[string]*Agent
    messagebus *MessageBus
}

type Task struct {
    ID          string
    Description string
    Status      TaskStatus  // "pending" | "running" | "done" | "failed"
    AssignedTo  string      // agent ID
    Result      string
    Error       error
    Priority    int
    CreatedAt   time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    BlockedBy   []string
    Metadata    map[string]string
}

type Agent struct {
    ID           string
    Name         string
    Capabilities []string
    Execute      AgentFunc  // (ctx, task) → (result, error)
    SystemPrompt string
    busy         bool
}

type MessageBus struct {
    channels map[string]chan Message
}
```

---

## Context Compaction (`compact/`)

**Compaction triggers when:**
- Context window exceeds `MaxContextTokens`

**Compaction strategy:**
- LLM summarizes old messages
- Keeps last 20 messages uncompacted
- Uses `SummaryCompactor` with token budgeting

**ResultStore:**
- Persists oversized tool results to disk
- Restores on post-compaction

---

## CLI Execution Modes

### **Print Mode** (`-p` flag)
```bash
prc-code -p "write hello.txt"
# Single query, outputs result, exits
```

### **Interactive TUI Mode** (default)
```bash
prc-code
# Full-screen terminal UI, session saved to ~/.claude/sessions/
```

### **Resume Session**
```bash
prc-code --resume              # Last session
prc-code --session-id ABC123   # Specific session
```

---

## Environment Variables

| Variable | Purpose | Providers |
|----------|---------|-----------|
| `ANTHROPIC_API_KEY` | Anthropic API key | anthropic |
| `CLAUDE_MODEL` | Default Claude model | anthropic |
| `OPENAI_API_KEY` | OpenAI API key | openai, responses |
| `OPENAI_MODEL` | Default OpenAI model | openai |
| `OPENAI_BASE_URL` | Custom OpenAI base URL | openai, responses |
| `OPENAI_USE_RESPONSES` | Enable Responses API | openai |
| `OLLAMA_MODEL` | Ollama model | ollama |
| `OLLAMA_BASE_URL` | Ollama server URL | ollama |
| `DEEPSEEK_API_KEY` | DeepSeek API key | deepseek |
| `DEEPSEEK_MODEL` | DeepSeek model | deepseek |
| `GEMINI_API_KEY` | Google Gemini API key | gemini |
| `GEMINI_MODEL` | Gemini model | gemini |
| `GEMINI_BASE_URL` | Gemini base URL | gemini |
| `GROQ_API_KEY` | Groq API key | groq |
| `GROQ_MODEL` | Groq model | groq |
| `GROQ_BASE_URL` | Groq base URL | groq |
| `MISTRAL_API_KEY` | Mistral API key | mistral |
| `MISTRAL_MODEL` | Mistral model | mistral |
| `MISTRAL_BASE_URL` | Mistral base URL | mistral |
| `PROVIDER` | Provider override | all |
| `OPENAI_REASONING_EFFORT` | Reasoning effort | openai (reasoning models) |

---

## Key Exported Types & Constructors

### **Provider Factory**
```go
NewFromEnv() (Provider, error)
NewFromEnvWithOverrides(providerOverride, modelOverride string) (Provider, error)
NewAnthropic(cfg Config) *AnthropicProvider
NewOpenAI(cfg Config) *OpenAIProvider
NewResponses(cfg Config) *ResponsesProvider
NewRetryProvider(inner Provider, cfg RetryConfig) *RetryProvider
```

### **Loop**
```go
New(p Provider, reg *registry.Registry, cfg Config) *QueryLoop
(q *QueryLoop) Run(ctx context.Context, userMessage string, onEvent func(Event)) error
(q *QueryLoop) SetResultStore(rs *ResultStore)
(q *QueryLoop) SetSessionID(id string)
(q *QueryLoop) Messages() []types.Message
(q *QueryLoop) SetMessages(msgs []types.Message)
```

### **Registry**
```go
New() *Registry
(r *Registry) Register(tool types.Tool)
(r *Registry) Get(name string) types.Tool
(r *Registry) All() []types.Tool
(r *Registry) ExecuteTool(ctx context.Context, name string, input map[string]any) ToolResultBlock
```

### **Tools (Sample)**
```go
// File tools
type FileReadTool struct{ AllowedDirs []string }
type FileWriteTool struct{ AllowedDirs []string }
type FileEditTool struct{ AllowedDirs []string }
type GlobTool struct{}
type GrepTool struct{}

// Execution
type BashTool struct{}

// Agent/Team
type AgentTool struct{ Provider Provider; Registry *Registry; System string; Depth int }
type TeamManager struct{ coordinator *Coordinator; teams map[string]*TeamInfo }

// Scheduling
type CronStore struct{ /* ... */ }
```

---

## Hot Paths & Performance Considerations

1. **Provider Streaming** (`provider/*.go`)
   - HTTP streaming with buffered channels
   - Exponential backoff for transient failures
   - Response caching (Responses API)

2. **Loop Streaming** (`loop/query.go`)
   - Real-time event emission via callback
   - Concurrent tool execution (`loop/concurrent.go`)
   - Optional context compaction for long conversations

3. **Tool Registry** (`registry/registry.go`)
   - O(1) tool lookup (map-based)
   - Tool execution concurrent or sequential (configurable)

4. **Message Serialization** (`types/messages.go`)
   - Custom JSON marshalling for ContentBlock interface
   - Type-aware deserialization

---

## Error Handling Philosophy

1. **Tool-Level Errors**: `(ToolResult{IsError: true}, nil)`
   - LLM sees and reasons about them
   - Normal tool outcomes (e.g., file not found)

2. **Infrastructure Errors**: `(ToolResult{}, err)`
   - Propagates up, may abort loop
   - Context cancelled, unrecoverable system errors

3. **Provider Errors**: Wrapped, retried with exponential backoff

4. **Hook Errors**: Silently ignored (hooks are best-effort instrumentation)

---

## Summary

**Architecture Pattern**: Multi-provider, tool-use loop with streaming
- **Abstraction**: Provider interface abstracts LLM backends
- **Extensibility**: Registry-based tool system (25+ built-in)
- **Coordination**: Coordinator + MessageBus for multi-agent tasks
- **Persistence**: FileStore for sessions, ResultStore for large results
- **Instrumentation**: Hook system for pre/post-tool monitoring
- **Performance**: Streaming, concurrent execution, optional compaction
