# Claude Code Go - Architecture Documentation Index

## 📋 Documents Created

This comprehensive architecture overview has been split into **two documents** for easy reference:

### 1. **QUICK_ARCHITECTURE_REFERENCE.md** (452 lines, 14KB)
**Start here for quick lookups and overviews.**

Contains:
- **One-page startup sequence** - How main() initializes the system
- **Request-response flow** - Single turn of the agentic loop
- **Core interfaces** - Provider, Tool, Compactor
- **File organization by responsibility** - Where each concern lives
- **Message flow diagram** - How data moves through the system
- **Function signatures** - Quick reference for key APIs
- **Provider selection logic** - Environment variable precedence
- **Message types table** - All content block types
- **Tool categories** - 25 tools organized by function
- **Hooks system** - Instrumentation hooks
- **Config objects** - All configuration structures with defaults
- **Error semantics** - Tool vs infrastructure errors
- **Environment & authentication** - Setup examples
- **Key dependencies** - go.mod packages
- **Performance hints** - Optimization notes
- **Common extension points** - How to add tools/providers

**Best for:** Quick answers, API reference, mental model, getting started

### 2. **ARCHITECTURE.md** (764 lines, 27KB)
**Deep dive reference with complete details.**

Contains:
- **Full directory tree** - All 42 directories explained
- **Core startup flow** - Detailed initialization sequence with all steps
- **REPL event loop** - Complete message processing pipeline
- **Key data structures** - Full struct definitions with all fields
- **Provider implementations** - Details for each provider (Anthropic, OpenAI, Responses)
- **Loop mechanism** - Complete QueryLoop with all methods
- **Provider directory** - File-by-file breakdown (env.go, anthropic.go, openai.go, etc.)
- **Tools directory** - All 25 tools categorized and described
- **Configuration & initialization** - Detailed config structures
- **Coordinator** - Multi-agent task coordination system
- **Context compaction** - How context windows are managed
- **CLI execution modes** - Print mode, REPL, resume
- **Environment variables** - Complete reference for all providers
- **Key exported types** - Constructors and factory functions
- **Hot paths** - Performance-critical code paths
- **Error handling philosophy** - Design principles

**Best for:** Deep understanding, implementation details, architecture decisions

---

## 🎯 Quick Navigation by Task

### "I need to..."

**...understand how the system works**
→ Start with QUICK_ARCHITECTURE_REFERENCE.md → "One-Page Startup Sequence" & "Request-Response Flow"

**...add a new LLM provider**
→ QUICK_ARCHITECTURE_REFERENCE.md → "Common Extension Points"  
→ ARCHITECTURE.md → "Provider Implementations" & "Provider/ Directory Details"

**...add a new tool**
→ QUICK_ARCHITECTURE_REFERENCE.md → "Common Extension Points"  
→ ARCHITECTURE.md → "Tools/ Directory"

**...debug the agentic loop**
→ QUICK_ARCHITECTURE_REFERENCE.md → "Request-Response Flow"  
→ ARCHITECTURE.md → "Loop (Agentic Tool-Use Loop) - Hot Path"  
→ See: loop/query.go (765 LOC)

**...understand how hooks work**
→ QUICK_ARCHITECTURE_REFERENCE.md → "Hooks System"  
→ ARCHITECTURE.md → "Hook System"

**...set up the project locally**
→ QUICK_ARCHITECTURE_REFERENCE.md → "Environment & Authentication"  
→ go.mod (dependencies)

**...optimize performance**
→ QUICK_ARCHITECTURE_REFERENCE.md → "Performance Hints"  
→ ARCHITECTURE.md → "Hot Paths & Performance Considerations"

**...understand provider selection**
→ QUICK_ARCHITECTURE_REFERENCE.md → "Provider Selection Logic"  
→ provider/env.go (169 LOC)

**...trace a tool execution**
→ ARCHITECTURE.md → "Message Flow Through System"  
→ registry/registry.go → tools/

**...understand context compaction**
→ ARCHITECTURE.md → "Context Compaction"  
→ compact/compact.go

---

## 🏗️ Architecture Patterns

### Pattern 1: Provider Abstraction
```
LLM Backend Abstraction
├─ Interface: Provider
├─ Implementations:
│  ├─ AnthropicProvider (373 LOC)
│  ├─ OpenAIProvider (551 LOC) — supports 8 OpenAI-compatible services
│  └─ ResponsesProvider
└─ Wrapped: RetryProvider (exponential backoff)
```

### Pattern 2: Tool-Use Loop
```
Agentic Loop (765 LOC in loop/query.go)
├─ Stream from LLM
├─ Parse content blocks (text, thinking, tool_use)
├─ On tool_use:
│  ├─ Execute tool(s) concurrently
│  ├─ Collect results
│  ├─ Add to message history
│  └─ Loop back
└─ On end_turn: exit loop
```

### Pattern 3: Registry Pattern
```
Tool System
├─ Interface: Tool
├─ Registry: map[name]Tool + ordered slice
├─ 25+ built-in tools
├─ O(1) lookup by name
└─ Concurrent execution via loop/concurrent.go
```

### Pattern 4: Hook Instrumentation
```
Event Hooks
├─ PreToolUse, PostToolUse, SessionStart, SessionEnd, UserPromptSubmit
├─ Loaded from: .claude/settings.json + .claude/hooks/
├─ Executed as subprocess
├─ Can block execution or modify input
└─ Errors silently ignored (non-fatal)
```

### Pattern 5: Message Serialization
```
Polymorphic Content Blocks
├─ TextBlock
├─ ThinkingBlock
├─ ToolUseBlock
├─ ToolResultBlock
├─ ImageBlock
└─ UnknownBlock (preserves unknown types)
```

---

## 📊 System Statistics

| Metric | Value |
|--------|-------|
| **Go Version** | 1.26.1 |
| **Total Directories** | 42 |
| **Core Packages** | 13 (cli, provider, loop, types, registry, tools, coordinator, compact, hooks, session, prompt, commands, mcp) |
| **Tool Implementations** | 25 files |
| **Provider Implementations** | 3 (Anthropic, OpenAI, Responses) |
| **Supporting Providers** | 8 (via OpenAI SDK: Ollama, DeepSeek, Gemini, Groq, Mistral, etc.) |
| **Main Entry Points** | 1 (main.go) |
| **CLI Options** | 13 flags |
| **Message Content Types** | 6 (Text, Thinking, ToolUse, ToolResult, Image, Unknown) |
| **Hook Types** | 5 (PreToolUse, PostToolUse, SessionStart, SessionEnd, UserPromptSubmit) |
| **Query Loop Max Turns** | 100 (configurable) |
| **Max Context Tokens** | 200,000 (Anthropic) |
| **Tool Execution** | Concurrent by default |
| **Context Compaction** | Optional (if MaxContextTokens > 0) |

---

## 🔄 Data Flow Map

```
┌─────────────────────────────────────────────────────────────┐
│                        main.go                              │
├─────────────────────────────────────────────────────────────┤
│ 1. Parse CLI options (cli/cli.go)                          │
│ 2. Create Provider (provider/env.go)                       │
│ 3. Setup Registry (registry_setup.go) — 25 tools           │
│ 4. Build System Prompt (prompt/)                           │
│ 5. Create QueryLoop (loop/query.go)                        │
│ 6. Enter REPL (repl.go) or PrintMode (printmode.go)        │
└────────────────┬────────────────────────────────────────────┘
                 │
        ┌────────▼────────┐
        │  User Input     │
        │  (readline)     │
        └────────┬────────┘
                 │
        ┌────────▼──────────────────────┐
        │ ql.Run(userMessage, onEvent)  │  loop/query.go (765 LOC)
        ├───────────────────────────────┤
        │ • Add to messages[]            │
        │ • provider.CreateStream()      │  provider/* (streaming)
        │ • Process StreamEvent          │
        │ • On tool_use:                 │
        │   - registry.Get()             │  registry/registry.go
        │   - tool.Execute()             │  tools/* (25 implementations)
        │   - Add ToolResultBlock        │
        │   - Loop back                  │
        │ • On end_turn: exit            │
        └────────┬──────────────────────┘
                 │
        ┌────────▼────────────────────┐
        │ onEvent(loop.Event)         │
        ├─────────────────────────────┤
        │ • Emit to REPL callback     │
        │ • Render text/thinking      │
        │ • Update terminal UI        │
        └────────┬────────────────────┘
                 │
        ┌────────▼────────────┐
        │ Next turn or exit   │
        └─────────────────────┘
```

---

## 🔑 Key Takeaways

1. **Multi-Provider Architecture**: Abstract Provider interface lets you swap LLM backends (Anthropic, OpenAI, Ollama, DeepSeek, Gemini, Groq, Mistral)

2. **Streaming-First**: All providers stream responses in real-time via `<-chan types.StreamEvent`

3. **Tool-Use Loop**: 765-line QueryLoop implements the core agentic pattern:
   - Stream from LLM
   - Execute tools (concurrent)
   - Loop until `stop_reason != "tool_use"`

4. **Registry Pattern**: 25+ tools registered at startup, O(1) lookup, concurrent execution

5. **Extensibility**: Add tools/providers by implementing interfaces and registering

6. **Instrumentation**: Hook system allows pre/post-tool monitoring and modification

7. **Persistence**: Sessions saved to filesystem, oversized tool results persisted separately

8. **Coordination**: Coordinator + MessageBus for multi-agent task dispatch

9. **Context Management**: Optional LLM-based summarization for long conversations

10. **Error Handling**: Clear distinction between tool-level errors (LLM sees) and infrastructure errors (abort loop)

---

## 📂 File Reference Quick Lookup

| Need | File | LOC |
|------|------|-----|
| Entry point | main.go | 153 |
| CLI parsing | cli/cli.go | 116 |
| REPL loop | repl.go | (check) |
| Print mode | printmode.go | (check) |
| Agentic loop | loop/query.go | 765 |
| Provider abstraction | provider/provider.go | ~80 |
| Provider factory | provider/env.go | 169 |
| Anthropic backend | provider/anthropic.go | 373 |
| OpenAI backend | provider/openai.go | 551 |
| Retry wrapper | provider/retry.go | ~150 |
| Tool interface | types/tools.go | (check) |
| Messages | types/messages.go | (check) |
| Registry | registry/registry.go | (check) |
| Tool setup | registry_setup.go | 124 |
| File tools | tools/file_operations.go | (check) |
| Agent tool | tools/agent.go | ~100 |
| Team tools | tools/team.go | ~200 |
| Coordinator | coordinator/coordinator.go | (check) |
| Compaction | compact/compact.go | (check) |
| Hooks | hooks/hooks.go | (check) |
| Sessions | session/session.go | (check) |

---

## 🚀 Getting Started Checklist

- [ ] Read: QUICK_ARCHITECTURE_REFERENCE.md "One-Page Startup Sequence"
- [ ] Read: ARCHITECTURE.md "Core Startup Flow"
- [ ] Review: main.go (153 LOC) — entry point
- [ ] Review: provider/env.go (169 LOC) — provider selection
- [ ] Review: loop/query.go (765 LOC) — agentic loop
- [ ] Review: registry_setup.go (124 LOC) — tool registration
- [ ] Browse: tools/ directory — understand tool pattern
- [ ] Trace: A single request through the system mentally
- [ ] Try: Run the tool with different providers
- [ ] Extend: Add a new tool following the pattern

---

## 📞 Reference Documents

This index is part of a 3-document set:

1. **QUICK_ARCHITECTURE_REFERENCE.md** ← Use for quick answers
2. **ARCHITECTURE.md** ← Use for deep dives
3. **ARCHITECTURE_INDEX.md** ← You are here

All documents are in the root of the project and are up-to-date as of April 5, 2026.

