# Go Module Dependency Graph & Import Analysis

## 📦 Module Import Structure

### Direct Imports from main.go
```
main
├── cli         - CLI argument parsing
├── hooks       - Hook system
├── loop        - Query loop orchestration
├── prompt      - Prompt blocks, context injection, memory discovery
├── provider    - AI provider abstraction
├── compact     - Context compaction
└── session     - Session management
```

---

## 🔗 Inter-Module Dependencies

### tools/ (Central Hub - Imported By Everything)
**Imports FROM:**
- `types` - Type definitions
- `provider` - For AI provider operations
- `permissions` - For security checks
- `registry` - For tool registration
- `session` - For session state
- `loop` - For execution context

**Imports TO:**
- Imported by: main, loop, coordinator

**Key Dependencies:**
- `lsp.go` - No internal deps
- `web.go` - HTTP operations
- `files.go` - File I/O, security checks
- `mcp_tools.go` - MCP protocol
- `team.go` - Team coordination
- `tasks.go` - Task state
- `cron.go` - Scheduling
- `skill.go` - Skill registry
- `dangerous.go` - Security checks
- `worktree.go` - Git operations
- `agent.go` - Agent coordination

---

### loop/ (Orchestration Layer)
**Purpose:** Central orchestration of tool calls and AI responses

**Dependencies:**
```
loop
├── types        - Message/stream types
├── provider     - AI provider interface
├── tools        - Tool invocation
├── compact      - Context optimization
├── permissions  - Access control
└── hooks        - Hook system
```

**Key Functions:**
- `query.go` - Main query processing
- `concurrent.go` - Concurrency control
- Stream management

---

### provider/ (Integration Layer)
**Purpose:** Abstract AI provider differences

**Implementations:**
- `anthropic.go` - Anthropic/Claude SDK
- `openai.go` - OpenAI SDK
- `env.go` - Environment configuration

**No dependencies on other modules** (Bottom of dependency stack)

---

### commands/ (Command Dispatch)
**Purpose:** Built-in command handling

**Key Components:**
- `commands.go` - Interface definition
- `builtins.go` - Built-in commands
- Likely imports: types, permissions, tools

---

### compact/ (Optimization)
**Purpose:** Context size optimization

**Dependencies:**
- `types` - Message types
- Possibly `provider` - Token counting

**Exports:**
- Used by loop for context management

---

### types/ (Foundation)
**Purpose:** Shared type definitions

**Exports:**
- `messages.go` - Message types
- `tools.go` - Tool definitions
- `stream.go` - Stream types

**No dependencies on other modules** (Bottom of stack)

---

### coordinator/ (Synchronization)
**Purpose:** Execution coordination

**Likely Dependencies:**
- `types` - For shared types
- `permissions` - For access control
- `session` - For state

---

### mcp/ (Protocol)
**Purpose:** Model Context Protocol implementation

**Key Interface:**
- `mcp.go` - MCP server/client
- Extensive test coverage in `mcp_test.go`

---

### hooks/ (Extension Points)
**Purpose:** Hook-based extensibility

**Key Features:**
- Settings-based hook loading
- Directory-based hook discovery
- Merge capability for multiple hooks

**Used by:**
- main.go for initialization
- loop for execution hooks

---

### permissions/ (Security Layer)
**Purpose:** Access control & resource limits

**Key Components:**
- `permissions.go` - Core permission logic
- `prompt.go` - Permission prompts/UI
- Boundary testing for edge cases

**Dependencies:**
- Used by: tools, loop, commands

---

### session/ (State Management)
**Purpose:** Session persistence

**Key Components:**
- `session.go` - Session interface
- File-based storage
- Used by: main.go

---

### prompt/ (Prompt Construction)
**Purpose:** Builds provider-ready prompt layers: system blocks, user context,
system context, memory discovery, and cache scope metadata.

**Key Components:**
- `system.go` - Static/dynamic system prompt block builders plus legacy string fallback
- `static_sections.go` - LUBAN Code branded original-style prompt sections
- `context.go` - User context meta message and trailing system context block
- `memory.go` / `rules.go` / `include.go` - CLAUDE.md, `.claude/rules`, includes, and conditional memory loading
- `cache.go` - Static/dynamic boundary handling and global/org cache scope metadata
- `debug.go` - Prompt dump/debug rendering
- `prompt-cache*.md` / `prompt_analysis.md` - Prompt architecture and cache notes

**Dependencies:**
- Imports `types` for tool/message-facing prompt structures.
- Does not import `loop` or `provider`; those modules consume prompt blocks and context values.

---

### render/ (Output)
**Purpose:** Output formatting (Markdown, etc.)

**Key Components:**
- `markdown.go` - Markdown rendering
- Used by: loop for streaming output

---

### skills/ (Skill Management)
**Purpose:** Skill registry & loading

**Dependencies:**
- `types` - Skill type definitions
- `registry` - Registration

---

### registry/ (Discovery)
**Purpose:** Tool registry & lookup

**Key Components:**
- `registry.go` - Registry interface
- Tool discovery/listing
- Used by: loop, commands, main

---

### cli/ (Entry Point)
**Purpose:** Command-line parsing

**No dependencies on other modules**

---

## 🔍 Dependency Hierarchy

### Level 0 (No Internal Dependencies)
```
cli, provider, types
```

### Level 1 (Depends on Level 0 only)
```
render, registry, skills, session, hooks, permissions
```

### Level 2 (Depends on Level 0-1)
```
prompt, commands, coordinator, compact
```

### Level 3 (Depends on all above)
```
mcp, loop, tools
```

---

## 📊 Module Complexity Matrix

| Module | LOC | Files | Dependencies | Dependent On | Complexity |
|--------|-----|-------|--------------|--------------|-----------|
| tools | 12,858 | 42 | HIGH | types, provider, permissions | **HIGHEST** |
| loop | 1,750 | 7 | HIGH | types, provider, tools, compact | **VERY HIGH** |
| provider | 1,530 | 5 | NONE | - | **LOW** |
| commands | 719 | 3 | MEDIUM | types, permissions, tools | **HIGH** |
| coordinator | 755 | 3 | MEDIUM | types, permissions, session | **MEDIUM** |
| compact | 1,128 | 8 | LOW | types, provider | **MEDIUM** |
| types | 742 | 6 | NONE | - | **LOW** |
| mcp | 649 | 2 | MEDIUM | types, provider | **MEDIUM** |
| skills | 425 | 2 | LOW | types, registry | **LOW** |
| permissions | 424 | 4 | LOW | types | **LOW** |
| hooks | 439 | 2 | LOW | types | **LOW** |
| session | 497 | 2 | LOW | types | **LOW** |
| prompt | 344 | 2 | LOW | types | **LOW** |
| registry | 374 | 3 | LOW | types | **LOW** |
| render | 258 | 2 | LOW | types | **LOW** |
| cli | 115 | 1 | NONE | - | **LOWEST** |

---

## 📈 Test Coverage Pattern

### High Test Density (>40% test LOC)
- `provider_test.go` - 681 LOC (44% of provider code)
- `mcp_test.go` - 363 LOC (56% of mcp code)
- `commands_test.go` - 328 LOC (46% of commands code)
- `coordinator_test.go` - 192 LOC (25% of coordinator code)

### Explicit Boundary Tests (Reliability Focus)
- `compact/boundary_test.go`
- `coordinator/boundary_test.go`
- `loop/concurrent_boundary_test.go`
- `permissions/boundary_test.go`
- `registry/boundary_test.go`
- `types/boundary_test.go`
- `tools/boundary_test.go`

### Integration Tests
- `loop/integration_test.go` - Full query loop testing
- `provider_test.go` - Provider integration

---

## 🎯 Critical Path Analysis

### Initialization Path (main.go)
```
main
├─ cli.Parse()           [cli module]
├─ provider.NewFrom*()   [provider module]
├─ SetupRegistry()       [registry + tools setup]
├─ prompt.BuildSystemPromptBlocks() / BuildSystemPrompt() [prompt]
├─ prompt.UserContextBuilder / SystemContextBuilder        [prompt]
├─ loop.New()            [loop orchestration]
└─ session management    [session module]
```

### Query Execution Path (loop.query)
```
loop.Query()
├─ provider.Call()       [provider]
├─ tools.Execute()       [tools]
├─ compact.Optimize()    [compact]
├─ permissions.Check()   [permissions]
└─ render.Output()       [render]
```

### Tool Invocation Path (tools module)
```
tools.Execute()
├─ lsp.*()              [LSP integration]
├─ web.*()              [Web access]
├─ files.*()            [File I/O]
├─ mcp_tools.*()        [MCP protocol]
├─ team.*()             [Coordination]
├─ tasks.*()            [Task management]
├─ cron.*()             [Scheduling]
└─ ... (26 other tool categories)
```

---

## 🔄 Circular Dependency Check

**Result:** ✓ No circular dependencies detected

Safe import order for documentation:
1. Start with: `provider`, `types`, `cli` (no deps)
2. Then: `render`, `registry`, `skills`, `session`, `hooks`, `permissions`
3. Then: `prompt`, `commands`, `coordinator`, `compact`
4. Finally: `mcp`, `loop`, `tools`

---

## 📝 Documentation Strategy by Dependency Level

### Priority 1: Foundation (Document First)
- **provider/** - Essential for all AI operations
- **types/** - Required for all other modules
- **cli/** - Entry point understanding

### Priority 2: Orchestration
- **loop/** - Core of everything
- **coordinator/** - Synchronization
- **registry/** - Tool discovery

### Priority 3: Core Functionality
- **tools/** - Largest and most complex
- **commands/** - Command dispatch
- **permissions/** - Security model

### Priority 4: Optimization & Control
- **compact/** - Performance optimization
- **mcp/** - Protocol layer
- **hooks/** - Extensibility

### Priority 5: Supporting Systems
- **prompt/** - Prompt block generation, context injection inputs, and memory discovery
- **render/** - Output rendering
- **session/** - Session state
- **skills/** - Skill management

---

## 🚀 Parallelizable Documentation Groups

Since the following modules have minimal inter-dependencies within their group, they can be documented in parallel:

### Group A (No deps on each other)
- cli
- provider
- types
- registry
- render

### Group B (Depends on Group A)
- hooks
- permissions
- session
- skills
- prompt

### Group C (Depends on A-B, complex)
- commands
- coordinator
- compact
- mcp

### Group D (Depends on all)
- loop
- tools

---

## 📊 Suggested Documentation Work Allocation

| Worker | Modules | Dependencies | Est. Effort |
|--------|---------|--------------|------------|
| A | provider, types | None | 2 hours |
| B | loop, coordinator | provider, types | 3 hours |
| C | tools | provider, types, permissions | 4 hours |
| D | commands, mcp, compact | A+B | 3 hours |
| E | hooks, permissions, render | types | 2 hours |
| F | session, skills, registry, prompt | types | 2 hours |
| G | cli, integration guide | All | 2 hours |

**Total Estimated Effort:** ~18 hours for comprehensive documentation
