# Phase 4.3: Search Operations - Implementation Guide

**Status**: READY FOR IMPLEMENTATION  
**Estimated Duration**: 45 minutes  
**Tools to Implement**: 4  
**Total Lines Expected**: ~400-500 lines (implementation + tests)

---

## Tools to Implement

### 1. GlobTool ⭐ Priority 1
**Purpose**: Fast file pattern matching using glob patterns

**Tool Definition**:
```go
type GlobTool struct{}
```

**Input Schema**:
```go
{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "Glob pattern (e.g., '**/*.go', 'src/**/*.ts')"
    }
  },
  "required": ["pattern"]
}
```

**Output Schema**:
```go
{
  "pattern": "string",
  "matches": [
    {
      "path": "string",
      "is_dir": "boolean",
      "size": "number",
      "modified": "number (unix timestamp)"
    }
  ],
  "count": "number"
}
```

**Implementation Notes**:
- Use `filepath.Glob()` for pattern matching
- Gather file metadata for each match
- Return sorted by modification time (most recent first)
- Handle errors gracefully (invalid patterns)

---

### 2. GrepTool ⭐ Priority 1
**Purpose**: Search for text patterns in files

**Tool Definition**:
```go
type GrepTool struct{}
```

**Input Schema**:
```go
{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "Text pattern to search for (supports regex)"
    },
    "files": {
      "type": "string",
      "description": "Glob pattern for files to search (e.g., '**/*.go')"
    },
    "case_insensitive": {
      "type": "boolean",
      "description": "Case-insensitive search"
    },
    "context_lines": {
      "type": "number",
      "description": "Lines of context before/after match (default: 0)"
    }
  },
  "required": ["pattern", "files"]
}
```

**Output Schema**:
```go
{
  "pattern": "string",
  "search_files": "string",
  "matches": [
    {
      "file": "string",
      "line_number": "number",
      "line_content": "string",
      "context": ["string"] // optional context lines
    }
  ],
  "total_matches": "number"
}
```

**Implementation Notes**:
- Use `strings.Contains()` for simple text search
- Support regex if pattern looks like regex (contains special chars)
- Include line numbers for each match
- Optional context lines before/after
- Use `filepath.Glob()` to find files to search

---

### 3. RipgrepTool ⭐ Priority 2
**Purpose**: Fast recursive search using ripgrep (if available)

**Tool Definition**:
```go
type RipgrepTool struct{}
```

**Input Schema**:
```go
{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "Regex pattern to search for"
    },
    "directory": {
      "type": "string",
      "description": "Directory to search (default: '.')"
    },
    "case_insensitive": {
      "type": "boolean",
      "description": "Case-insensitive search"
    },
    "file_type": {
      "type": "string",
      "description": "File type filter (e.g., 'go', 'ts', 'py')"
    }
  },
  "required": ["pattern"]
}
```

**Output Schema**:
```go
{
  "pattern": "string",
  "directory": "string",
  "matches": [
    {
      "file": "string",
      "line_number": "number",
      "column": "number",
      "line_content": "string"
    }
  ],
  "total_matches": "number"
}
```

**Implementation Notes**:
- Use `exec.CommandContext()` to call `rg` command
- Graceful fallback if ripgrep not installed (return error with suggestion to install)
- Parse ripgrep output format
- Much faster than GrepTool for large codebases

---

### 4. FindTool ⭐ Priority 3
**Purpose**: Find files using predicates (name, type, size, etc.)

**Tool Definition**:
```go
type FindTool struct{}
```

**Input Schema**:
```go
{
  "type": "object",
  "properties": {
    "directory": {
      "type": "string",
      "description": "Directory to search (default: '.')"
    },
    "name": {
      "type": "string",
      "description": "File name pattern (glob-style)"
    },
    "type": {
      "type": "string",
      "description": "File type: 'f' (file), 'd' (directory), 'l' (symlink)"
    },
    "min_size": {
      "type": "number",
      "description": "Minimum size in bytes"
    },
    "max_size": {
      "type": "number",
      "description": "Maximum size in bytes"
    },
    "recursive": {
      "type": "boolean",
      "description": "Search recursively (default: true)"
    }
  },
  "required": ["directory"]
}
```

**Output Schema**:
```go
{
  "directory": "string",
  "filters": {
    "name": "string|null",
    "type": "string|null",
    "min_size": "number|null",
    "max_size": "number|null"
  },
  "matches": [
    {
      "path": "string",
      "name": "string",
      "type": "string",
      "size": "number",
      "modified": "number (unix timestamp)"
    }
  ],
  "count": "number"
}
```

**Implementation Notes**:
- Use `filepath.Walk()` for recursive search
- Filter by name using glob patterns
- Filter by type (file/directory/symlink)
- Filter by size range
- Collect metadata for each match

---

## Test Strategy

### Test File: `tools/search_operations_test.go`

**Expected Test Coverage**:
```
TestGlobTool
  ├─ Valid patterns
  ├─ No matches
  ├─ Recursive patterns (**/...)
  ├─ Special characters handling
  └─ Performance with many files

TestGrepTool
  ├─ Basic text search
  ├─ Case sensitivity
  ├─ Context lines
  ├─ No matches
  ├─ Multiple files
  └─ Large files

TestRipgrepTool
  ├─ Basic regex search
  ├─ File type filtering
  ├─ Case insensitivity
  ├─ Command execution
  ├─ Tool not installed (graceful fallback)
  └─ Complex patterns

TestFindTool
  ├─ By name pattern
  ├─ By type (file/dir)
  ├─ By size range
  ├─ Combined filters
  ├─ Recursive search
  └─ Empty results

TestSearchToolSchemas
  ├─ Schema validation for all tools
  └─ Metadata validation
```

### Helper Functions
```go
func createTestFiles(t *testing.T, dir string) // Create test file structure
func searchForPattern(t *testing.T, dir string) // Utility for grep testing
```

---

## Implementation Checklist

- [ ] Create `tools/search_operations.go`
- [ ] Implement `GlobTool`
  - [ ] Name(), Description(), Schema()
  - [ ] Execute() with pattern matching
  - [ ] Metadata gathering
- [ ] Implement `GrepTool`
  - [ ] Name(), Description(), Schema()
  - [ ] Execute() with text search
  - [ ] Line number tracking
  - [ ] Context lines support
- [ ] Implement `RipgrepTool`
  - [ ] Name(), Description(), Schema()
  - [ ] Execute() with ripgrep command
  - [ ] Graceful fallback for missing tool
- [ ] Implement `FindTool`
  - [ ] Name(), Description(), Schema()
  - [ ] Execute() with multiple filters
  - [ ] File type detection
- [ ] Create `tools/search_operations_test.go`
  - [ ] Test helper functions
  - [ ] Individual tool tests
  - [ ] Schema validation tests
- [ ] Update `registry_setup.go`
  - [ ] Register all 4 tools
  - [ ] Verify registry integration
- [ ] Run full test suite
  - [ ] All new tests pass
  - [ ] No regression in existing tests
  - [ ] Build succeeds
- [ ] Create `PHASE_4_3_COMPLETION.md`
- [ ] Commit with comprehensive message

---

## Code Quality Guidelines

### Error Handling
```go
// Business error (tool-level)
return ErrorResponsef("pattern not found in any files"), nil

// Infrastructure error (execution-level)
return types.ToolResult{}, fmt.Errorf("context cancelled")
```

### Input Extraction
```go
pattern, err := MustGetStringField(input, "pattern")
if err != nil {
    return ErrorResponse(err), nil
}

directory := GetStringField(input, "directory", ".")
recursive := GetBoolField(input, "recursive", true)
```

### Output Marshalling
```go
return ResponseJSON(map[string]any{
    "pattern": pattern,
    "matches": results,
    "count":   len(results),
})
```

---

## Expected Output

**Files Created**:
- `tools/search_operations.go` (~250-300 lines)
- `tools/search_operations_test.go` (~200-250 lines)

**Registry Updates**:
- 4 new tool registrations

**Documentation**:
- `PHASE_4_3_COMPLETION.md`

**Metrics**:
- 4/4 tools implemented
- 10+ tests passing
- Zero build warnings/errors
- ~3.5-4s total test suite execution

---

## Success Criteria

✅ All 4 tools implemented  
✅ 10+ tests passing (100% pass rate)  
✅ Clean build (0 warnings, 0 errors)  
✅ Tools registered in registry  
✅ Comprehensive documentation  
✅ Code follows Phase 4.1/4.2 patterns  

---

## References

- Phase 4.1 (Shell Operations): tools/shell_operations.go
- Phase 4.2 (Git Operations): tools/git_operations.go
- Error handling patterns: tools/file_operations.go
- Registry integration: registry_setup.go

---

## Notes

- GlobTool and GrepTool should use stdlib (no external dependencies)
- RipgrepTool is optional performance optimization (graceful fallback)
- FindTool complements GlobTool with predicate-based filtering
- All tools should follow established input/output patterns from 4.1 & 4.2

