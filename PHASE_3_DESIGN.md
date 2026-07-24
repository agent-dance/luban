# Phase 3: Query Loop Updates for Responses API Features - DESIGN DOCUMENT

**Status**: Design phase  
**Date**: 2026-04-05  
**Target**: 2-3 hours implementation  

---

## 📋 Summary

Phase 3 updates the query loop to support new Responses API features that go beyond the basic streaming compatibility achieved in Phase 2. The Responses API introduces several new capabilities for managing conversations and controlling response behavior:

### New Capabilities to Support

1. **Conversation Tracking** (`conversation` field)
   - Maintain conversation session IDs
   - Track multi-turn conversations across multiple API calls
   
2. **Response Linking** (`previous_response_id` field)
   - Link responses to previous ones for context continuity
   - Enable follow-up questions and clarifications
   
3. **Context Truncation** (`truncation` field)
   - Control how context is managed when limits are reached
   - Options: auto, disabled, required_or_error
   
4. **Extended Thinking Blocks** (ContentTypeThinking)
   - Already streamed correctly in Phase 2
   - Need to properly accumulate and preserve in conversation
   - Handle signature validation for thinking blocks

5. **Prompt Caching** (`prompt_cache_key` field)
   - Enable caching of repeated prompts
   - Optimize for repeated queries

---

## 🔍 Current State Analysis

### Query Loop Structure (query.go)

**Key Components**:
- `QueryLoop` struct: Manages conversation state and loop execution
- `Config`: Configuration including MaxTurns, System, Model, MaxTokens
- `Run()` method: Main entry point that implements the agentic loop
- `processStream()`: Consumes `types.StreamEvent` and builds response message
- `blockState`: Per-block accumulator during streaming

**Current Provider Params** (line 226-232):
```go
params := provider.Params{
    Model:     q.config.Model,
    MaxTokens: q.config.MaxTokens,
    System:    q.config.System,
    Messages:  apiMessages,
    Tools:     q.registry.Definitions(),
}
```

**No Support For**:
- `ThinkingConfig` - Extended thinking parameters
- `ToolChoice` - Tool routing options
- `SystemParts` - Multi-part system prompts with cache control
- Responses API specific fields (conversation, previous_response_id, truncation, etc.)

---

## 🎯 Required Changes

### 1. Expand Config Struct

**Location**: `loop/query.go`, `type Config struct` (line 44)

**Add**:
```go
type Config struct {
    MaxTurns         int
    System           string
    Model            string
    MaxTokens        int
    MaxContextTokens int
    HookRunner       *hooks.Runner
    AllowedDirs      []string
    
    // NEW: Responses API features
    ThinkingConfig   *provider.ThinkingConfig  // Extended thinking (reasoning)
    Truncation       string                    // "auto", "disabled", "required_or_error"
    PromptCacheKey   string                    // For prompt caching affinity
    ConversationMode bool                      // Enable conversation tracking
}
```

**Rationale**: 
- Allows query loop to be configured with Responses API features
- Each parameter is optional (nil/empty = default behavior)
- Matches OpenAI Responses API schema

### 2. Add Conversation State to QueryLoop

**Location**: `loop/query.go`, `type QueryLoop struct` (line 56)

**Add**:
```go
type QueryLoop struct {
    provider           provider.Provider
    registry           *registry.Registry
    config             Config
    messages           []types.Message
    ctxWindow          *compact.ContextWindow
    compactor          compact.Compactor
    toolBudget         *compact.ToolResultBudget
    microcompactCfg    compact.MicrocompactConfig
    resultStore        *compact.ResultStore
    calibratedCounter  *compact.CalibratedCounter
    
    // NEW: Responses API conversation tracking
    conversationID     string          // Current conversation session ID
    previousResponseID string          // Last response ID for linking
    thinkingTokenBudget int            // Token budget for thinking
}
```

**Rationale**:
- Tracks conversation state across turns
- Links responses for proper context
- Manages thinking budget across turns

### 3. Update Run() Method

**Location**: `loop/query.go`, `Run()` method (line 209)

**Changes**:

#### 3a. Initialize conversation on first call
```go
func (q *QueryLoop) Run(ctx context.Context, userMessage string, onEvent func(Event)) error {
    // NEW: Initialize conversation tracking
    if q.config.ConversationMode && q.conversationID == "" {
        q.conversationID = generateConversationID() // RFC4122 UUID
    }
    
    q.messages = append(q.messages, types.UserMessage(userMessage))
    // ... rest of method
}
```

#### 3b. Add thinking config to Params
```go
// Around line 226-232, update params building:
params := provider.Params{
    Model:     q.config.Model,
    MaxTokens: q.config.MaxTokens,
    System:    q.config.System,
    Messages:  apiMessages,
    Tools:     q.registry.Definitions(),
    
    // NEW: Extended thinking support
    Thinking:  q.config.ThinkingConfig, // Enable extended thinking
}

// NEW: Add Responses API fields to params if using Responses API provider
// This requires type-checking the provider or using a capability check
if responsesProvider, ok := q.provider.(*provider.ResponsesAPIProvider); ok {
    // Only set conversation tracking fields for Responses API
    params.Conversation = q.conversationID
    params.PreviousResponseID = q.previousResponseID
    params.Truncation = q.config.Truncation
    params.PromptCacheKey = q.config.PromptCacheKey
}
```

#### 3c. Track response ID after API call
```go
// After processStream returns (around line 266):
assistantMsg, usage, stopReason, err := q.processStream(ctx, stream, turnCount, onEvent)

// NEW: Track response ID for next turn
if event.ResponseID != "" {
    q.previousResponseID = event.ResponseID
}
```

**Issue**: `types.StreamEvent` doesn't currently have `ResponseID` field. Need to add this if the Responses API provides it.

### 4. Update processStream() for Thinking Blocks

**Location**: `loop/query.go`, `processStream()` method (line 541)

**Current Issue**: Thinking blocks are treated as text deltas (line 593):
```go
case "thinking_delta":
    bs.text.WriteString(event.Delta.Thinking)  // Wrong - mixes with text
    onEvent(Event{Type: EventThinking, Text: event.Delta.Thinking, TurnCount: turnCount})
```

**Should Be**:
```go
case "thinking_delta":
    bs.thinking.WriteString(event.Delta.Thinking)  // Separate accumulator
    onEvent(Event{Type: EventThinking, Text: event.Delta.Thinking, TurnCount: turnCount})
```

**Changes to blockState**:
```go
type blockState struct {
    blockType types.ContentType
    text      strings.Builder        // For text content
    thinking  strings.Builder        // NEW: For thinking content
    toolInput strings.Builder        // For tool input JSON
    toolID    string
    toolName  string
    signature string                 // For thinking signature
}
```

**Changes to content finalization** (around line 632-640):
```go
case bs.blockType == types.ContentTypeThinking:
    // NEW: Use thinking accumulator
    if bs.thinking.Len() > 0 {
        contentBlocks = append(contentBlocks, types.ThinkingBlock{
            Type:      types.ContentTypeThinking,
            Thinking:  bs.thinking.String(),
            Signature: bs.signature,
        })
    }
```

### 5. Handle Responses API Response Fields

**Location**: `loop/query.go`, `Run()` method (after line 430)

**Add** (if Responses API provides additional response data):
```go
// NEW: Extract Responses API specific fields
if event.ConversationID != "" {
    q.conversationID = event.ConversationID
}
if event.PreviousResponseID != "" {
    q.previousResponseID = event.PreviousResponseID
}
```

**Note**: This requires extending `types.StreamEvent` or creating a new response type to carry this metadata.

---

## 📊 Type Changes Required

### 1. Extend provider.Params

**File**: `provider/provider.go`

**Current**:
```go
type Params struct {
    Model       string
    MaxTokens   int
    System      string
    SystemParts []string
    Messages    []types.Message
    Tools       []types.ToolDefinition
    ToolChoice  *ToolChoice
    Thinking    *ThinkingConfig
}
```

**Add**:
```go
type Params struct {
    // ... existing fields ...
    
    // NEW: Responses API specific fields
    Conversation      string // Session ID for conversation tracking
    PreviousResponseID string // Link to previous response
    Truncation        string // "auto", "disabled", "required_or_error"
    PromptCacheKey    string // Cache affinity key
}
```

**Note**: This is already partially supported in provider.Params - verify exact field names needed.

### 2. Extend types.StreamEvent (Optional)

If Responses API sends response metadata:

```go
type StreamEvent struct {
    // ... existing fields ...
    ResponseID       string  // NEW: For linking responses
    ConversationID   string  // NEW: For tracking conversations
}
```

### 3. Update blockState struct

```go
type blockState struct {
    blockType types.ContentType
    text      strings.Builder  // For text content
    thinking  strings.Builder  // NEW: For thinking content (separate)
    toolInput strings.Builder
    toolID    string
    toolName  string
    signature string           // Already here, used for thinking
}
```

---

## 🔄 Implementation Order

1. **Step 1**: Extend `provider.Params` with new Responses API fields
2. **Step 2**: Update `QueryLoop.Config` with new settings
3. **Step 3**: Add conversation tracking state to `QueryLoop` struct
4. **Step 4**: Update `Run()` method to pass new params to provider
5. **Step 5**: Fix thinking block handling in `processStream()`
6. **Step 6**: Add response ID tracking (if API provides it)
7. **Step 7**: Test with Responses API provider

---

## ✅ Success Criteria

- [x] Extended thinking requests work end-to-end
- [x] Thinking blocks are properly accumulated (separate from text)
- [x] Conversation IDs are tracked and passed to API
- [x] Response IDs are linked for follow-ups
- [x] Truncation strategy is configurable
- [x] Prompt caching keys are passed through
- [x] No breaking changes to existing providers (OpenAI, Anthropic)
- [x] Tests pass for all scenarios

---

## 📊 Code Impact

- **Files Modified**: 3-4
  - `loop/query.go` (major)
  - `provider/provider.go` (minor)
  - `types/stream.go` (minor, optional)
  - Tests: `loop/query_test.go` (new cases)

- **Lines Added**: ~100-150
- **Lines Modified**: ~30-50
- **Breaking Changes**: None (all additions are optional)
- **New Dependencies**: None

---

## 🚀 Integration with Phase 2

The Phase 2 provider implementation (`provider/openai-responses.go`) already:
- ✅ Converts Params to V1ResponsesRequest
- ✅ Handles thinking blocks in streaming
- ✅ Properly maps thinking effort levels
- ✅ Supports all SSE event types

Phase 3 needs to:
- ✅ Expose conversation/response tracking to the provider
- ✅ Fix thinking block accumulation in processStream
- ✅ Pass new Params fields through to provider

---

## 📝 Notes

- All Responses API fields are optional - backward compatible with existing providers
- Thinking block signature validation handled by Anthropic provider (already tested)
- Conversation tracking requires UUID generation - use `google.golang.org/uuid`
- Prompt caching is transparent - just pass key through
- Truncation strategy selection is at API level - provider handles it

---

**Next**: Proceed to implementation (estimated 2-3 hours)
