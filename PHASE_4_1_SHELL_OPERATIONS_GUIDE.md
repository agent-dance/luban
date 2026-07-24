# Phase 4.1: Shell Operations Implementation Guide

**Estimated Time**: 1-1.5 hours  
**Priority**: HIGH - File + Shell operations are the most commonly used tools  
**Dependencies**: File operations ✅ complete

---

## Overview

Implement 8 shell-related tools for environment variable management and working directory control:

1. **EnvGetTool** - Get a single environment variable
2. **EnvSetTool** - Set an environment variable  
3. **EnvListTool** - List all environment variables
4. **PwdTool** - Print working directory
5. **CdTool** - Change working directory
6. **CwdTool** - Get current working directory (alias for pwd)
7. **WdTool** - Working directory info (enhanced pwd)
8. **ShellExecTool** - Already exists (Bash) but may need wrapper

---

## Implementation Pattern

All tools follow this pattern:

```go
type ToolNameTool struct {
	// State (if needed)
	WorkingDir string
}

func (t *ToolNameTool) Name() string {
	return "ToolName"
}

func (t *ToolNameTool) Description() string {
	return "Short description"
}

func (t *ToolNameTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"field_name": map[string]any{
				"type":        "string",
				"description": "Field description",
			},
		},
		Required: []string{"field_name"},
	}
}

func (t *ToolNameTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	// Extract input
	value, err := MustGetStringField(input, "field_name")
	if err != nil {
		return ErrorResponse(err), nil
	}
	
	// Perform operation
	result := performOperation(value)
	
	// Return response
	return ResponseJSON(result)
}
```

---

## Detailed Tool Specifications

### 1. EnvGetTool

**Purpose**: Get the value of a single environment variable

**Input Schema**:
```json
{
  "variable": "string (required) - Environment variable name (e.g., 'HOME', 'PATH')"
}
```

**Output**:
```json
{
  "variable": "VAR_NAME",
  "value": "value or null",
  "exists": true
}
```

**Implementation Notes**:
- Use `os.LookupEnv()` to safely get env var
- Return `exists: false` if not set
- Never fail; return empty/null for missing vars

```go
type EnvGetTool struct{}

func (t *EnvGetTool) Name() string {
	return "EnvGet"
}

func (t *EnvGetTool) Description() string {
	return "Get the value of an environment variable"
}

func (t *EnvGetTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"variable": map[string]any{
				"type":        "string",
				"description": "Environment variable name (e.g., HOME, PATH)",
			},
		},
		Required: []string{"variable"},
	}
}

func (t *EnvGetTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	varName, err := MustGetStringField(input, "variable")
	if err != nil {
		return ErrorResponse(err), nil
	}
	
	value, exists := os.LookupEnv(varName)
	
	return ResponseJSON(map[string]any{
		"variable": varName,
		"value":    value,
		"exists":   exists,
	})
}
```

---

### 2. EnvSetTool

**Purpose**: Set an environment variable for the current session

**Input Schema**:
```json
{
  "variable": "string (required) - Variable name",
  "value": "string (required) - Variable value"
}
```

**Output**:
```json
{
  "status": "success",
  "variable": "VAR_NAME",
  "value": "new_value"
}
```

**Permission Model**: `env_access` feature gate (ask/allow/deny)

**Implementation Notes**:
- Use `os.Setenv()` to set the variable
- Variable is only set for current process (not shell-wide)
- Confirm with user before setting sensitive vars (PASSWORD, TOKEN, KEY)

```go
type EnvSetTool struct{}

func (t *EnvSetTool) Name() string {
	return "EnvSet"
}

func (t *EnvSetTool) Description() string {
	return "Set an environment variable (current session only)"
}

func (t *EnvSetTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"variable": map[string]any{
				"type":        "string",
				"description": "Environment variable name",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Value to set",
			},
		},
		Required: []string{"variable", "value"},
	}
}

func (t *EnvSetTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	varName, err := MustGetStringField(input, "variable")
	if err != nil {
		return ErrorResponse(err), nil
	}
	
	value, err := MustGetStringField(input, "value")
	if err != nil {
		return ErrorResponse(err), nil
	}
	
	if err := os.Setenv(varName, value); err != nil {
		return ErrorResponsef("failed to set environment variable: %v", err), nil
	}
	
	return ResponseJSON(map[string]any{
		"status":   "success",
		"variable": varName,
		"value":    value,
	})
}
```

---

### 3. EnvListTool

**Purpose**: List all environment variables

**Input Schema**: (none)

**Output**:
```json
{
  "variables": {
    "HOME": "/Users/user",
    "PATH": "/usr/bin:...",
    ...
  },
  "count": 42
}
```

**Implementation Notes**:
- Use `os.Environ()` to get all vars
- Return as map for easy access
- Sort keys for consistent output (optional but nice)

```go
type EnvListTool struct{}

func (t *EnvListTool) Name() string {
	return "EnvList"
}

func (t *EnvListTool) Description() string {
	return "List all environment variables"
}

func (t *EnvListTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{},
		Required: []string{},
	}
}

func (t *EnvListTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	envVars := make(map[string]string)
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}
	
	return ResponseJSON(map[string]any{
		"variables": envVars,
		"count":     len(envVars),
	})
}
```

---

### 4. PwdTool

**Purpose**: Print working directory

**Input Schema**: (none)

**Output**:
```json
{
  "directory": "/Users/user/projects/claude-code"
}
```

```go
type PwdTool struct{}

func (t *PwdTool) Name() string {
	return "Pwd"
}

func (t *PwdTool) Description() string {
	return "Print the current working directory"
}

func (t *PwdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{},
		Required: []string{},
	}
}

func (t *PwdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ErrorResponsef("failed to get working directory: %v", err), nil
	}
	
	return ResponseJSON(map[string]string{
		"directory": cwd,
	})
}
```

---

### 5. CwdTool

**Purpose**: Get current working directory (alias for Pwd)

**Input Schema**: (none)

**Output**: Same as Pwd

```go
type CwdTool struct{}

func (t *CwdTool) Name() string {
	return "Cwd"
}

func (t *CwdTool) Description() string {
	return "Get the current working directory"
}

func (t *CwdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{},
		Required: []string{},
	}
}

func (t *CwdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ErrorResponsef("failed to get working directory: %v", err), nil
	}
	
	return ResponseJSON(map[string]string{
		"directory": cwd,
	})
}
```

---

### 6. WdTool

**Purpose**: Enhanced working directory info (pwd + stat info)

**Input Schema**: (none)

**Output**:
```json
{
  "directory": "/Users/user/projects",
  "exists": true,
  "readable": true,
  "writable": true,
  "size": 4096,
  "mode": "0755",
  "modified": 1775380000
}
```

```go
type WdTool struct{}

func (t *WdTool) Name() string {
	return "Wd"
}

func (t *WdTool) Description() string {
	return "Get enhanced working directory information"
}

func (t *WdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{},
		Required: []string{},
	}
}

func (t *WdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ErrorResponsef("failed to get working directory: %v", err), nil
	}
	
	info, err := os.Stat(cwd)
	if err != nil {
		return ErrorResponsef("failed to stat working directory: %v", err), nil
	}
	
	return ResponseJSON(map[string]any{
		"directory": cwd,
		"exists":    true,
		"readable":  hasPermission(cwd, 4), // read
		"writable":  hasPermission(cwd, 2), // write
		"size":      info.Size(),
		"mode":      fmt.Sprintf("0%o", info.Mode().Perm()),
		"modified":  info.ModTime().Unix(),
	})
}

func hasPermission(path string, mode int) bool {
	// Check if we have permission (simplified)
	return os.Access(path, os.FileMode(mode)) == nil
}
```

---

### 7. CdTool

**Purpose**: Change working directory (for subagent context)

**Input Schema**:
```json
{
  "directory": "string (required) - Path to change to"
}
```

**Output**:
```json
{
  "status": "success",
  "directory": "/new/path",
  "previous": "/old/path"
}
```

**Important Notes**:
- Changing CWD in Go process affects only the current process
- For REPL mode, store the intended CWD and apply on next command
- For SubAgent, the CWD change only affects that agent's file operations

**Implementation** (simplified - actual implementation may need process-wide coordination):

```go
type CdTool struct {
	CurrentDir string // Store intended CWD
}

func (t *CdTool) Name() string {
	return "Cd"
}

func (t *CdTool) Description() string {
	return "Change working directory"
}

func (t *CdTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to change to",
			},
		},
		Required: []string{"directory"},
	}
}

func (t *CdTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir, err := MustGetStringField(input, "directory")
	if err != nil {
		return ErrorResponse(err), nil
	}
	
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ErrorResponsef("directory does not exist: %s", dir), nil
	}
	
	// Get current dir before change
	oldDir, _ := os.Getwd()
	
	// Change directory (affects only this process)
	if err := os.Chdir(dir); err != nil {
		return ErrorResponsef("failed to change directory: %v", err), nil
	}
	
	// Store for reference
	t.CurrentDir = dir
	
	return ResponseJSON(map[string]any{
		"status":    "success",
		"directory": dir,
		"previous":  oldDir,
	})
}
```

---

### 8. ShellExecTool

**Status**: Already implemented as `BashTool`  
**Action**: No changes needed

Verify it's registered in `registry_setup.go`:
```go
reg.Register(&tools.BashTool{})
```

---

## File: tools/shell_operations.go

Create a new file with all 7 shell tools:

```go
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/agent-adaptor/luban/types"
)

// EnvGetTool gets an environment variable
type EnvGetTool struct{}

// ... (implement as above)

// EnvSetTool sets an environment variable
type EnvSetTool struct{}

// ... (implement as above)

// EnvListTool lists all environment variables
type EnvListTool struct{}

// ... (implement as above)

// PwdTool prints working directory
type PwdTool struct{}

// ... (implement as above)

// CwdTool gets current working directory
type CwdTool struct{}

// ... (implement as above)

// WdTool gets enhanced working directory info
type WdTool struct{}

// ... (implement as above)

// CdTool changes working directory
type CdTool struct {
	CurrentDir string
}

// ... (implement as above)
```

---

## File: tools/shell_operations_test.go

Create comprehensive tests:

```go
package tools

import (
	"context"
	"os"
	"testing"
)

func TestEnvGet(t *testing.T) {
	tool := &EnvGetTool{}
	
	// Set a test env var
	os.Setenv("TEST_VAR", "test_value")
	
	result, err := tool.Execute(context.Background(), map[string]any{
		"variable": "TEST_VAR",
	})
	
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	
	// Verify result structure
	if result.IsError {
		t.Errorf("Expected success, got error: %s", result.Content)
	}
}

// ... more tests for each tool
```

---

## Registry Update

Update `registry_setup.go` to register shell tools:

```go
// Shell/environment tools
reg.Register(&tools.EnvGetTool{})
reg.Register(&tools.EnvSetTool{})
reg.Register(&tools.EnvListTool{})
reg.Register(&tools.PwdTool{})
reg.Register(&tools.CwdTool{})
reg.Register(&tools.WdTool{})
reg.Register(&tools.CdTool{CurrentDir: cwd})
```

---

## Permission Integration

When permission system is active:

```go
// Feature gates for shell operations
checker.SetFeatureGates(map[string]bool{
	"env_access":  true,
	"shell_exec":  true,
})

// Rules (examples)
rules := []permissions.Rule{
	{
		Tool:     "EnvSet",
		Pattern:  "*PASSWORD*",
		Decision: permissions.DecisionAsk,
	},
	{
		Tool:     "Cd",
		Pattern:  "/etc",
		Decision: permissions.DecisionDeny,
	},
}
```

---

## Testing Checklist

- [ ] EnvGet returns correct values
- [ ] EnvGet returns null for missing vars
- [ ] EnvSet updates environment
- [ ] EnvList returns all vars
- [ ] Pwd returns current directory
- [ ] Cwd returns current directory
- [ ] Wd returns enhanced info
- [ ] Cd changes directory
- [ ] Cd validates directory exists
- [ ] All tools integrate with permission system
- [ ] Feature gates work correctly
- [ ] Error handling is consistent

---

## Estimated Completion

- Implementation: 45 min
- Testing: 20 min
- Integration: 15 min
- **Total**: ~80 minutes

---

## Next: Phase 4.2 (Git Operations)

Once shell tools are complete, proceed to git operations.

