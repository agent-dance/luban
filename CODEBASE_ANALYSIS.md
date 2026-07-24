# Claude Code Go Codebase Structure & Documentation Audit

**Date:** 2026-04-05  
**Total Go Code:** 21,908 lines across 16 top-level modules  
**Analysis Tool:** Python cache metrics simulator + bash introspection

---

## 📊 EXECUTIVE SUMMARY

### Go Modules Overview (gosrc/)

| Module | Lines | Files | Subdirs | Status |
|--------|-------|-------|---------|--------|
| **tools** | 12,858 | 42 | 1 | ⚠️ LARGEST, partial docs |
| **loop** | 1,750 | 7 | 0 | ✓ Well-tested |
| **provider** | 1,530 | 5 | 0 | ✓ Core infrastructure |
| **compact** | 1,128 | 8 | 0 | ✓ Has context-compaction.md |
| **commands** | 719 | 3 | 0 | ⚠️ No docs |
| **coordinator** | 755 | 3 | 0 | ⚠️ No docs |
| **session** | 497 | 2 | 0 | ⚠️ No docs |
| **skills** | 425 | 2 | 0 | ⚠️ No docs |
| **permissions** | 424 | 4 | 0 | ⚠️ No docs |
| **hooks** | 439 | 2 | 0 | ⚠️ No docs |
| **mcp** | 649 | 2 | 0 | ⚠️ No docs |
| **prompt** | 344 | 2 | 1 | ✓ Has prompt-cache.md & analysis scripts |
| **registry** | 374 | 3 | 0 | ⚠️ No docs |
| **render** | 258 | 2 | 0 | ⚠️ No docs |
| **types** | 742 | 6 | 0 | ⚠️ No docs |
| **cli** | 115 | 1 | 0 | ⚠️ No docs |

---

## 📁 DETAILED MODULE BREAKDOWN

### 🔴 HIGH PRIORITY - LARGEST MODULES (Need Documentation)

#### **tools/** (12,858 LOC - 42 Go files)
**Purpose:** Tool implementations and integrations  
**Key Files:**
- `lsp.go` (977 LOC) - Language Server Protocol integration
- `web.go` (576 LOC) - Web/HTTP tools
- `files.go` (637 LOC) - File operations
- `mcp_tools.go` (602 LOC) - MCP (Model Context Protocol) tools
- `team.go` (409 LOC) - Team collaboration features
- `tasks.go` (389 LOC) - Task management
- `cron.go` (481 LOC) - Cron/scheduling
- `search.go` (430 LOC) - Search functionality
- `skill.go` (294 LOC) - Skill registry
- `dangerous.go` (295 LOC) - Security-sensitive operations
- `parse.go` (268 LOC) - Parsing utilities
- `config.go` (212 LOC) - Configuration
- `misc.go` (250 LOC) - Miscellaneous utilities
- `worktree.go` (259 LOC) - Git worktree operations
- `agent.go` (122 LOC) - Agent integration
- `askuser.go` (207 LOC) - User prompts
- `notebook.go` (244 LOC) - Notebook support
- `urlvalidation.go` (121 LOC) - URL validation
- Plus 24 test files and platform-specific files (fd_path_*.go)

**Subdirectories:** `.claude/` (internal state)  
**Documentation:** ⚠️ NONE - Extensive module needs breakdown

---

#### **loop/** (1,750 LOC - 7 Go files)
**Purpose:** Query loop & concurrent execution engine  
**Key Files:**
- `query.go` (531 LOC) - Main query processing
- `query_test.go` (353 LOC) - Query tests
- `concurrent.go` (276 LOC) - Concurrency primitives
- `integration_test.go` (227 LOC) - Integration tests
- `stream_test.go` (180 LOC)
- `concurrent_test.go` (149 LOC)
- `concurrent_boundary_test.go` (34 LOC)

**Responsibilities:**
- Orchestrates tool calls and AI responses
- Manages concurrent execution
- Stream processing for real-time results
- Boundary testing for reliability

**Documentation:** ⚠️ NONE

---

#### **provider/** (1,530 LOC - 5 Go files)
**Purpose:** AI provider abstraction layer  
**Key Files:**
- `openai.go` (421 LOC) - OpenAI integration
- `anthropic.go` (303 LOC) - Anthropic/Claude integration
- `provider_test.go` (681 LOC) - Comprehensive tests
- `env.go` (87 LOC) - Environment variable handling
- `provider.go` (38 LOC) - Core interface

**Architecture:**
- Pluggable provider interface
- Support for Anthropic & OpenAI
- Environment-based configuration
- Extensive test coverage

**Documentation:** ⚠️ NONE

---

### 🟡 MEDIUM PRIORITY - WELL-TESTED MODULES

#### **compact/** (1,128 LOC - 8 Go files) ✓ HAS DOCS
**Purpose:** Context compaction & token optimization  
**Documentation:** ✓ `context-compaction.md` exists

**Key Files:**
- `compact.go` (307 LOC) - Core compaction algorithm
- `compact_test.go` (206 LOC)
- `microcompact.go` (93 LOC) - Micro-compression
- `microcompact_test.go` (210 LOC)
- `boundary_test.go` (181 LOC)
- `resultstore.go` (66 LOC) - Result caching
- `summarize.go` (48 LOC) - Summarization
- `prompt.go` (17 LOC)

**Key Insight:** This module has existing documentation and a supporting Python analysis script at `prompt/scripts/cache_metrics.py` for simulating cache hit rates.

---

#### **coordinator/** (755 LOC - 3 Go files)
**Purpose:** Coordination & synchronization  
**Key Files:**
- `coordinator.go` (401 LOC)
- `coordinator_test.go` (192 LOC)
- `boundary_test.go` (162 LOC)

**Documentation:** ⚠️ NONE

---

#### **commands/** (719 LOC - 3 Go files)
**Purpose:** Built-in commands & dispatching  
**Key Files:**
- `builtins.go` (264 LOC) - Built-in command implementations
- `commands_test.go` (328 LOC) - Comprehensive tests
- `commands.go` (127 LOC) - Core command interface

**Documentation:** ⚠️ NONE

---

#### **mcp/** (649 LOC - 2 Go files)
**Purpose:** Model Context Protocol implementation  
**Key Files:**
- `mcp.go` (286 LOC)
- `mcp_test.go` (363 LOC) - Extensive testing

**Documentation:** ⚠️ NONE

---

### 🟢 LOWER PRIORITY - SMALLER MODULES

#### **types/** (742 LOC - 6 Go files)
**Purpose:** Shared type definitions  
**Key Files:**
- `messages.go` (175 LOC)
- `tools.go` (165 LOC)
- `messages_test.go` (127 LOC)
- `stream.go` (108 LOC)
- `boundary_test.go` (118 LOC)
- `tools_test.go` (49 LOC)

**Documentation:** ⚠️ NONE

---

#### **hooks/** (439 LOC - 2 Go files)
**Purpose:** Hook system for extensibility  
**Key Files:**
- `hooks.go` (265 LOC)
- `hooks_test.go` (174 LOC)

**Documentation:** ⚠️ NONE

---

#### **permissions/** (424 LOC - 4 Go files)
**Purpose:** Access control & security  
**Key Files:**
- `permissions.go` (179 LOC)
- `permissions_test.go` (82 LOC)
- `prompt.go` (72 LOC)
- `boundary_test.go` (91 LOC)

**Documentation:** ⚠️ NONE

---

#### **prompt/** (344 LOC - 2 Go files + scripts/) ✓ HAS DOCS
**Purpose:** System prompt building & caching  
**Documentation:** ✓ `prompt-cache.md` and `prompt-cache-analysis.md`

**Key Files:**
- `system.go` (196 LOC)
- `prompt_test.go` (148 LOC)

**Scripts:**
- `scripts/cache_metrics.py` (257 LOC) - **Python cache metrics simulator**
  - Compares 3 caching strategies: no-cache, Go 3-breakpoint, TS full version
  - Mathematical model for cache hit rate prediction
  - Output formats: table, JSON, CSV
  - Parameters: system tokens, tool tokens, delta per turn, total turns
  - Pricing calculation based on Sonnet 4 ($3/MTok)

**Documentation:** ⚠️ NONE for Go code (but has Python analysis tools)

---

#### **skills/** (425 LOC - 2 Go files)
**Purpose:** Skill registry & management  
**Key Files:**
- `skills.go` (210 LOC)
- `skills_test.go` (215 LOC)

**Documentation:** ⚠️ NONE

---

#### **session/** (497 LOC - 2 Go files)
**Purpose:** Session persistence & management  
**Key Files:**
- `session.go` (277 LOC)
- `session_test.go` (220 LOC)

**Documentation:** ⚠️ NONE

---

#### **registry/** (374 LOC - 3 Go files)
**Purpose:** Tool registry & discovery  
**Key Files:**
- `registry.go` (141 LOC)
- `registry_test.go` (104 LOC)
- `boundary_test.go` (129 LOC)

**Documentation:** ⚠️ NONE

---

#### **render/** (258 LOC - 2 Go files)
**Purpose:** Output rendering (Markdown, etc.)  
**Key Files:**
- `markdown.go` (159 LOC)
- `markdown_test.go` (99 LOC)

**Documentation:** ⚠️ NONE

---

#### **cli/** (115 LOC - 1 Go file)
**Purpose:** Command-line argument parsing  
**Key File:**
- `cli.go` (115 LOC)

**Documentation:** ⚠️ NONE

---

## 📊 TypeScript Counterparts in /src

The following TypeScript modules correspond to Go modules (showing TS has more extensive implementations):

| Go Module | TS Equivalent | TS Lines | TS Files | Notes |
|-----------|---------------|----------|----------|-------|
| tools | tools | 50,828 | 184 | TS has 3-4x more code |
| loop | query | 652 | 4 | Query-specific in TS |
| provider | (distributed) | - | - | Part of services |
| commands | commands | 26,428 | 189 | Extensive command set |
| hooks | hooks | 19,204 | 104 | Parallel implementation |
| coordinator | coordinator | 369 | 1 | Similar scope |
| prompt | (in utils/services) | - | - | Different architecture |
| skills | skills | 4,066 | 20 | Parallel implementation |
| session | state | 1,190 | 6 | State management |
| render | components | 81,546 | 389 | Rendering layer |
| types | types | 3,446 | 11 | Type definitions |
| mcp | (in services) | - | - | Integrated differently |
| cli | cli | 12,353 | 19 | CLI interface |

**Major TS-Only Modules:**
- `services/` (53,680 LOC, 130 files) - Business logic
- `utils/` (180,472 LOC, 564 files) - Utilities
- `components/` (81,546 LOC, 389 files) - UI components
- `bridge/` (12,613 LOC, 31 files) - TS-Go bridge layer

---

## 🔍 EXISTING DOCUMENTATION INVENTORY

### Go Side
- ✓ `gosrc/compact/context-compaction.md` - Context optimization strategy
- ✓ `gosrc/prompt/prompt-cache.md` - Prompt caching explanation
- ✓ `gosrc/prompt/prompt-cache-analysis.md` - Detailed analysis
- ✓ `gosrc/prompt/scripts/cache_metrics.py` - Cache simulation tool (257 LOC)

### Root Level
- `go.mod` - Module definition (Go 1.26.1)
- Dependencies: Anthropic SDK, OpenAI SDK, readline, jrpc2, tiktoken-go

---

## 📈 KEY METRICS

### Code Distribution
- **Average module size:** 1,369 LOC
- **Median module size:** 542 LOC
- **Largest module (tools):** 12,858 LOC (58.6% of all Go code)
- **Test coverage pattern:** Most modules have corresponding `*_test.go` files
- **Boundary tests:** 8 modules have explicit `boundary_test.go` files

### Documentation Gap
- **Documented modules:** 2/16 (12.5%)
- **Undocumented modules:** 14/16 (87.5%)
- **Documented LOC:** ~650 LOC (3% of total)
- **Undocumented LOC:** ~21,258 LOC (97% of total)

---

## 🎯 PARALLEL DOCUMENTATION WORK PRIORITIES

### **Tier 1 - Critical (Blocking others)**
1. **tools/** - Foundation for all tool operations
2. **loop/** - Core orchestration engine
3. **provider/** - AI provider abstraction

### **Tier 2 - High Value (Key systems)**
4. **coordinator/** - Synchronization & execution
5. **commands/** - Command dispatch system
6. **mcp/** - Protocol implementation

### **Tier 3 - Supporting (Type systems)**
7. **types/** - Shared definitions
8. **hooks/** - Extensibility framework
9. **permissions/** - Security model

### **Tier 4 - Utility (Smaller scope)**
10. **skills/** - Skill management
11. **session/** - Session handling
12. **registry/** - Tool discovery
13. **render/** - Output formatting
14. **cli/** - CLI interface

---

## 📝 DOCUMENTATION TEMPLATE RECOMMENDATIONS

### For Large Modules (tools, loop, provider)
- Architecture overview diagram (ASCII or reference external)
- Component breakdown table
- Key algorithms/patterns used
- Integration points with other modules
- API documentation for exported types/functions
- Example usage
- Testing strategy

### For Medium Modules (coordinator, commands, etc.)
- Purpose & responsibilities
- Key types & interfaces
- Public API documentation
- Integration points
- Example code snippets

### For Small Modules (cli, render, etc.)
- Brief purpose statement
- Key exported functions/types
- Usage examples
- Links to related modules

---

## 🔧 ANALYSIS TOOLS AVAILABLE

### Python Cache Metrics Simulator
**Location:** `gosrc/prompt/scripts/cache_metrics.py`  
**Purpose:** Mathematical simulation of cache hit rates across 3 strategies

**Usage:**
```bash
# Default (8K system, 15K tools, 2.5K delta, 20 turns)
python3 scripts/cache_metrics.py

# Custom parameters
python3 scripts/cache_metrics.py --turns 50 --system 12000

# JSON/CSV output
python3 scripts/cache_metrics.py --json > cache_analysis.json
python3 scripts/cache_metrics.py --csv > cache_analysis.csv
```

**Output:** Compares no-cache vs. Go 3-breakpoint vs. TS full implementation

---

## 🚀 RECOMMENDATIONS FOR PARALLEL WORK

### Phase 1: Foundational (Can work in parallel)
- [ ] Document `tools/` module (large, impacts others)
- [ ] Document `loop/` module (orchestration)
- [ ] Document `provider/` module (integration point)
- [ ] Document `types/` module (supports all)

### Phase 2: Systems (After Phase 1)
- [ ] Document `coordinator/`
- [ ] Document `commands/`
- [ ] Document `mcp/`
- [ ] Document `hooks/`
- [ ] Document `permissions/`

### Phase 3: Utilities (After Phase 2)
- [ ] Document `skills/`, `session/`, `registry/`
- [ ] Document `render/`, `cli/`
- [ ] Cross-reference with TypeScript equivalents

### Phase 4: Integration
- [ ] Create architecture diagram showing all modules
- [ ] Document TypeScript bridge layer (`src/bridge/`)
- [ ] Create developer onboarding guide

---

## 📌 NEXT STEPS

1. **Verify TS/Go correspondence** - Check `src/bridge/` for actual integration
2. **Identify key APIs** - Use LSP to find exported types/functions
3. **Test file analysis** - Count test coverage by module
4. **Dependency graph** - Map imports between modules
5. **Performance notes** - Check for optimization comments in code

