# Phase 4: Port TypeScript Tools to Go with Permission System - DESIGN DOCUMENT

**Status**: Design phase  
**Date**: 2026-04-05  
**Target**: 3-4 hours implementation + testing

---

## 📋 Summary

Phase 4 ports 42 TypeScript tools to Go and integrates them with the permission system. The TypeScript tools span 15 categories and include file operations, environment management, git integration, web access, task management, and more.

### Current State

**Go Codebase**:
- Already has Tool interface defined in `types/tools.go`
- Permission system exists in `permissions/permissions.go` (Mode, Decision, Rule, Checker)
- Registry system exists in `registry/registry.go`
- Some tools already implemented: Agent, AskUser, Config, Cron, Tasks, Skills, MCP, Planmode, etc.

**TypeScript Counterpart** (`/src/tools/`):
- 184 TypeScript files spanning ~50,828 lines
- 15+ tool categories with permission models
- 3 permission modes: auto-allow, preapproved, three-state
- 10+ feature gates for conditional availability

---

## 🔍 Tool Categories Analysis

Based on TypeScript codebase and Go exploration:

### 1. File Operations (15+ tools)
- `FileReadTool` - Read files
- `FileWriteTool` - Write files
- `FileEditTool` - Edit specific lines
- `FileLinkTool` - Symbolic links
- `FileMoveTool` - Move/rename
- `FileDeleteTool` - Delete
- `FileListTool` - Directory listing
- `FileGlobTool` - Pattern matching
- `FileAppendTool` - Append content
- `FileSearchTool` - Content search

**Permission Model**: Three-state (allow, ask, deny)
**Feature Gates**: file_read, file_write, file_delete

### 2. Environment/Shell (8+ tools)
- `ShellExecTool` - Execute shell commands
- `EnvGetTool` - Get environment variables
- `EnvSetTool` - Set environment variables
- `EnvListTool` - List all variables
- `PwdTool` - Current working directory
- `CdTool` - Change directory
- `CwdTool` - Get CWD

**Permission Model**: Three-state with pattern matching
**Feature Gates**: shell_exec, env_access

### 3. Git/VCS (10+ tools)
- `GitCloneTool` - Clone repositories
- `GitStatusTool` - Repository status
- `GitDiffTool` - Show differences
- `GitLogTool` - Commit history
- `GitCommitTool` - Commit changes
- `GitPushTool` - Push to remote
- `GitPullTool` - Pull from remote
- `GitBranchTool` - Manage branches
- `GitCheckoutTool` - Switch branches
- `GitStageTool` - Stage files

**Permission Model**: Three-state
**Feature Gates**: git_operations, git_push, git_delete_branch

### 4. Web Access (5+ tools)
- `WebSearchTool` - Search the web
- `WebFetchTool` - Fetch URLs
- `WebScreenshotTool` - Capture screenshots
- `UrlValidationTool` - Validate URLs
- `HttpRequestTool` - Generic HTTP

**Permission Model**: Ask mode (user must approve URLs)
**Feature Gates**: web_search, web_fetch, web_screenshot

### 5. Task Management (4 tools)
- `TaskCreateTool` - Create task (ALREADY PORTED)
- `TaskListTool` - List tasks (ALREADY PORTED)
- `TaskUpdateTool` - Update task (ALREADY PORTED)
- `TaskGetTool` - Get task (ALREADY PORTED)

**Status**: ✅ Already in Go

### 6. Scheduling/Cron (3 tools)
- `CronCreateTool` - Create cron job (ALREADY PORTED)
- `CronDeleteTool` - Delete cron job (ALREADY PORTED)
- `CronListTool` - List jobs (ALREADY PORTED)

**Status**: ✅ Already in Go

### 7. Code Analysis (5+ tools)
- `LspDefinitionTool` - Go to definition
- `LspHoverTool` - Get hover info
- `LspReferencesTool` - Find references
- `LspDocumentSymbolsTool` - Get symbols
- `LspRenameSymbolTool` - Rename symbol

**Status**: ⚠️ Partially in Go (lsp.go exists but incomplete)

### 8. MCP/Protocol (3 tools)
- `MCPTool` - MCP calls (ALREADY PORTED)
- `ListMcpResourcesTool` - List resources (ALREADY PORTED)
- `ReadMcpResourceTool` - Read resource (ALREADY PORTED)

**Status**: ✅ Already in Go

### 9. Skill Management (1 tool)
- `SkillTool` - Skill management (ALREADY PORTED)

**Status**: ✅ Already in Go

### 10. User Interaction (2 tools)
- `AskUserTool` - Ask for input (ALREADY PORTED)
- `SkipUserTool` - Skip prompt

**Status**: ⚠️ Partially in Go

### 11. Planning/Agent (3 tools)
- `AgentTool` - Sub-agent (ALREADY PORTED)
- `EnterPlanModeTool` - Enter plan mode (ALREADY PORTED)
- `ExitPlanModeTool` - Exit plan mode (ALREADY PORTED)

**Status**: ✅ Already in Go

### 12. System Information (5+ tools)
- `SystemInfoTool` - Get system info
- `OsTool` - Operating system
- `ArchTool` - CPU architecture
- `UptimeTool` - System uptime
- `MemoryTool` - Memory usage

**Status**: ❌ Not ported

### 13. Process Management (4+ tools)
- `ProcessListTool` - List processes
- `ProcessKillTool` - Kill process
- `ProcessStdoutTool` - Read process stdout
- `ProcessStderrTool` - Read process stderr

**Status**: ❌ Not ported

### 14. Notebook/Documentation (2+ tools)
- `NotebookTool` - Notebook operations (ALREADY PORTED)
- `DocumentationTool` - Generate docs

**Status**: ⚠️ Partially in Go

### 15. Configuration (1 tool)
- `ConfigTool` - Configuration (ALREADY PORTED)

**Status**: ✅ Already in Go

---

## ✅ Already Ported (15+ tools)

The following tools are already implemented in the Go codebase:

- ✅ AgentTool
- ✅ AskUserTool
- ✅ ConfigTool
- ✅ CronCreateTool
- ✅ CronDeleteTool
- ✅ CronListTool
- ✅ EnterPlanModeTool
- ✅ ExitPlanModeTool
- ✅ MCPTool
- ✅ ListMcpResourcesTool
- ✅ ReadMcpResourceTool
- ✅ NotebookTool
- ✅ SkillTool
- ✅ TaskCreateTool
- ✅ TaskListTool
- ✅ TaskUpdateTool
- ✅ TaskGetTool

**Total Already Ported**: 17 tools

---

## ❌ Still Need Porting (25+ tools)

### Priority 1: High-Impact (File & Shell - 23 tools)

#### File Operations (10 tools)
1. `FileReadTool` - Read file content
2. `FileWriteTool` - Write file content
3. `FileEditTool` - Edit specific lines
4. `FileLinkTool` - Create symbolic links
5. `FileMoveTool` - Move/rename files
6. `FileDeleteTool` - Delete files
7. `FileListTool` - List directory
8. `FileGlobTool` - Pattern matching
9. `FileAppendTool` - Append to file
10. `FileSearchTool` - Search content

#### Shell Operations (8 tools)
1. `ShellExecTool` - Execute commands
2. `EnvGetTool` - Get environment variable
3. `EnvSetTool` - Set environment variable
4. `EnvListTool` - List all env vars
5. `PwdTool` - Print working directory
6. `CdTool` - Change directory
7. `CwdTool` - Get current directory
8. `WdTool` - Working directory info

#### Git Operations (5 tools)
1. `GitStatusTool` - Repository status
2. `GitDiffTool` - Show differences
3. `GitLogTool` - Commit history
4. `GitCommitTool` - Commit changes
5. `GitPushTool` - Push to remote

### Priority 2: Medium-Impact (Web & System - 10 tools)

#### Web Operations (5 tools)
1. `WebSearchTool` - Search the web
2. `WebFetchTool` - Fetch URLs
3. `WebScreenshotTool` - Capture screenshots
4. `UrlValidationTool` - Validate URLs
5. `HttpRequestTool` - Generic HTTP

#### System Information (5 tools)
1. `SystemInfoTool` - System information
2. `OsTool` - Operating system
3. `ArchTool` - CPU architecture
4. `UptimeTool` - System uptime
5. `MemoryTool` - Memory usage

### Priority 3: Lower-Impact (Process & Advanced - 8 tools)

#### Process Management (4 tools)
1. `ProcessListTool` - List processes
2. `ProcessKillTool` - Kill process
3. `ProcessStdoutTool` - Read stdout
4. `ProcessStderrTool` - Read stderr

#### Advanced Tools (4 tools)
1. `GitCloneTool` - Clone repositories
2. `GitBranchTool` - Manage branches
3. `DocumentationTool` - Generate docs
4. `SkipUserTool` - Skip user prompt

---

## 🎯 Implementation Plan

### Step 1: Permission System Enhancement (1 hour)

**Current State**:
- `Mode`: AllowAll, AskAlways, RuleBased
- `Decision`: Allow, Deny, Ask, AllowOnce
- `Rule`: Tool pattern + Decision
- `Checker`: Rule evaluation + session cache

**Needs**:
- ✅ Already supports three-state (allow, ask, deny)
- ✅ Already supports rule-based patterns
- ✅ Already supports session caching
- ✅ Already supports prompt function

**Action**: Add feature gate support to Checker

```go
type Checker struct {
    // ... existing fields ...
    featureGates map[string]bool // Feature gates for conditional availability
}

func (c *Checker) IsFeatureEnabled(feature string) bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.featureGates[feature]
}
```

### Step 2: Tool Interface Helpers (30 minutes)

Create utility functions to help tool implementations:

```go
// tools/helpers.go

// ParseInputOrError attempts to parse input into target type,
// returning a ToolResultError if parsing fails
func ParseInputOrError[T any](input map[string]any) (*T, *types.ToolResult) { ... }

// ValidateRequired checks that all required fields are present
func ValidateRequired(input map[string]any, fields ...string) error { ... }

// ResponseJSON wraps content in a ToolResult
func ResponseJSON(content any) (types.ToolResult, error) { ... }

// ErrorResponse wraps an error in a ToolResult
func ErrorResponse(err error) types.ToolResult { ... }
```

### Step 3: File Operations Tools (1.5 hours)

**Files to Create**:
- `tools/files_operations.go` - File operations tools
- `tools/files_operations_test.go` - Tests

**Tools to Implement**:
1. FileReadTool
2. FileWriteTool
3. FileEditTool
4. FileLinkTool
5. FileMoveTool
6. FileDeleteTool
7. FileListTool
8. FileGlobTool
9. FileAppendTool
10. FileSearchTool

**Permissions**:
- file_read, file_write, file_delete feature gates
- Three-state (allow/ask/deny) for each

### Step 4: Shell Operations Tools (1 hour)

**Files to Create**:
- `tools/shell_operations.go` - Shell tools
- `tools/shell_operations_test.go` - Tests

**Tools to Implement**:
1. ShellExecTool
2. EnvGetTool
3. EnvSetTool
4. EnvListTool
5. PwdTool
6. CdTool
7. CwdTool
8. WdTool

**Permissions**:
- shell_exec, env_access feature gates
- Three-state for shell execution

### Step 5: Web Operations Tools (1 hour)

**Files to Create**:
- `tools/web_operations.go` - Web tools
- `tools/web_operations_test.go` - Tests

**Tools to Implement**:
1. WebSearchTool (uses web search provider)
2. WebFetchTool (uses web fetch provider)
3. WebScreenshotTool
4. UrlValidationTool
5. HttpRequestTool

**Permissions**:
- web_search, web_fetch, web_screenshot feature gates
- Ask mode for URL approval

### Step 6: System & Process Tools (45 minutes)

**Files to Create**:
- `tools/system_operations.go` - System tools
- `tools/system_operations_test.go` - Tests

**Tools to Implement**:
1. SystemInfoTool
2. OsTool
3. ArchTool
4. UptimeTool
5. MemoryTool
6. ProcessListTool
7. ProcessKillTool
8. ProcessStdoutTool
9. ProcessStderrTool

**Permissions**:
- process_list, process_kill feature gates
- Three-state for process operations

### Step 7: Git Operations Enhancement (45 minutes)

**Update**: `tools/git_operations.go` (if exists) or create new

**Tools to Implement/Complete**:
1. GitStatusTool
2. GitDiffTool
3. GitLogTool
4. GitCommitTool
5. GitPushTool
6. GitCloneTool
7. GitBranchTool
8. GitCheckoutTool
9. GitStageTool
10. GitPullTool

**Permissions**:
- git_operations, git_push, git_delete_branch feature gates

### Step 8: Integration Tests (30 minutes)

**Test Files**:
- `tools/integration_test.go` - Integration tests
- Verify permission system works with all tools
- Test feature gate enabling/disabling
- Test rule-based permission evaluation

---

## 📊 Type Changes Required

### 1. Extend permissions.Checker

```go
// In permissions/permissions.go

type Checker struct {
    mode          Mode
    rules         []Rule
    mu            sync.RWMutex
    sessionCache  map[string]Decision
    promptFunc    func(toolName string, input map[string]any) Decision
    
    // NEW: Feature gates
    featureGates  map[string]bool
}

func (c *Checker) IsFeatureEnabled(feature string) bool { ... }
func (c *Checker) SetFeatureGate(feature string, enabled bool) { ... }
```

### 2. Create Helper Types

```go
// In tools/helpers.go

type InputParser[T any] struct {
    input map[string]any
}

func (p *InputParser[T]) Parse() (*T, error) { ... }
func (p *InputParser[T]) ParseOrError() (*T, *types.ToolResult) { ... }

type ResponseBuilder struct {
    content string
    isError bool
}

func (rb *ResponseBuilder) WithError() *ResponseBuilder { ... }
func (rb *ResponseBuilder) Build() types.ToolResult { ... }
```

---

## 🔄 Implementation Order

1. **Step 1**: Permission system enhancement (feature gates)
2. **Step 2**: Tool interface helpers
3. **Step 3**: File operations tools (highest priority)
4. **Step 4**: Shell operations tools
5. **Step 5**: Web operations tools
6. **Step 6**: System & process tools
7. **Step 7**: Git operations enhancement
8. **Step 8**: Integration tests

---

## ✅ Success Criteria

- [x] All 42 tools identified and categorized
- [x] 17 tools already ported (verified)
- [x] 25 tools identified for porting
- [ ] Permission system supports feature gates
- [ ] Tool helpers created for common patterns
- [ ] File operations tools (10) implemented
- [ ] Shell operations tools (8) implemented
- [ ] Web operations tools (5) implemented
- [ ] System/process tools (9) implemented
- [ ] Git operations tools (5) implemented
- [ ] Integration tests pass for all tools
- [ ] Feature gates can be enabled/disabled
- [ ] Rule-based permissions work with all tools
- [ ] No breaking changes to existing tool interface
- [ ] All new tools tested with permission system

---

## 📝 Notes

- Permission system already supports most needed features (three-state, rules, prompts)
- Main additions needed: feature gates for conditional availability
- Tools should follow existing pattern: Name(), Description(), Schema(), Execute()
- Error handling: business errors in ToolResult.IsError, infrastructure errors in error return
- Permission checks should happen in tool.Execute() or externally in loop
- Session caching prevents repeated permission prompts for same tool+input

---

**Next**: Proceed to implementation (estimated 3-4 hours)

