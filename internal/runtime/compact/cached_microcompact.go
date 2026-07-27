package compact

import (
	"encoding/json"

	"github.com/agent-dance/luban/types"
)

const ContentTypeCacheEdits types.ContentType = "cache_edits"

type CacheEdit struct {
	Type           string `json:"type"`
	CacheReference string `json:"cache_reference"`
}

type CacheEditsBlock struct {
	Type  string      `json:"type"`
	Edits []CacheEdit `json:"edits"`
}

type PinnedCacheEdits struct {
	UserMessageIndex int
	Block            CacheEditsBlock
	ProofLedger      string
}

type CachedMicrocompactState struct {
	RegisteredTools map[string]struct{}
	DeletedRefs     map[string]struct{}
	ToolOrder       []string
	ToolGroups      [][]string
	PinnedEdits     []PinnedCacheEdits
}

type CachedMicrocompactResult struct {
	Messages       []types.Message
	Changed        bool
	DeletedToolIDs []string
	ReclaimedBytes int
	ProofBytes     int
	BytesSaved     int
}

func NewCachedMicrocompactState() *CachedMicrocompactState {
	return &CachedMicrocompactState{
		RegisteredTools: make(map[string]struct{}),
		DeletedRefs:     make(map[string]struct{}),
	}
}

func (s *CachedMicrocompactState) Reset() {
	if s == nil {
		return
	}
	s.RegisteredTools = make(map[string]struct{})
	s.DeletedRefs = make(map[string]struct{})
	s.ToolOrder = nil
	s.ToolGroups = nil
	s.PinnedEdits = nil
}

func (cfg MicrocompactConfig) ShouldUseCachedMicrocompact() bool {
	return cfg.CachedEnabled && cfg.QuerySource == MicrocompactSourceMain && cfg.CachedTriggerThreshold > 0
}

func (cfg MicrocompactConfig) cachedKeepRecent() int {
	if cfg.CachedKeepRecent <= 0 {
		return cfg.keepRecent()
	}
	return cfg.CachedKeepRecent
}

func CachedMicrocompact(messages []types.Message, cfg MicrocompactConfig, state *CachedMicrocompactState) CachedMicrocompactResult {
	if state == nil || !cfg.ShouldUseCachedMicrocompact() {
		return CachedMicrocompactResult{Messages: messages}
	}
	ensureCachedMicrocompactState(state)

	compactableIDs := collectCompactableToolIDs(messages, cfg)
	compactableSet := make(map[string]struct{}, len(compactableIDs))
	for _, id := range compactableIDs {
		compactableSet[id] = struct{}{}
	}

	for _, msg := range messages {
		if effectiveCompactionRole(msg) != types.RoleUser {
			continue
		}
		var group []string
		for _, block := range msg.Content {
			tr, ok := block.(types.ToolResultBlock)
			if !ok || tr.ToolUseID == "" {
				continue
			}
			if _, ok := compactableSet[tr.ToolUseID]; !ok {
				continue
			}
			if _, seen := state.RegisteredTools[tr.ToolUseID]; seen {
				continue
			}
			state.RegisteredTools[tr.ToolUseID] = struct{}{}
			state.ToolOrder = append(state.ToolOrder, tr.ToolUseID)
			group = append(group, tr.ToolUseID)
		}
		if len(group) > 0 {
			state.ToolGroups = append(state.ToolGroups, group)
		}
	}

	active := make([]string, 0, len(state.ToolOrder))
	for _, id := range state.ToolOrder {
		if _, deleted := state.DeletedRefs[id]; !deleted {
			active = append(active, id)
		}
	}
	overage := len(active) - cfg.CachedTriggerThreshold
	if overage <= 0 {
		return CachedMicrocompactResult{Messages: appendPinnedCacheEdits(messages, state.PinnedEdits)}
	}
	keepRecent := cfg.cachedKeepRecent()
	if keepRecent < 1 {
		keepRecent = 1
	}
	deleteCount := len(active) - keepRecent
	if deleteCount <= 0 {
		return CachedMicrocompactResult{Messages: appendPinnedCacheEdits(messages, state.PinnedEdits)}
	}

	deleted := append([]string(nil), active[:deleteCount]...)
	pinnedIndex := lastUserMessageIndex(messages)
	if pinnedIndex < 0 {
		// A developer catalog is an instruction boundary. Wait for the next
		// current user message rather than pinning new cache edits to an older
		// turn on the other side of that boundary.
		return CachedMicrocompactResult{Messages: appendPinnedCacheEdits(messages, state.PinnedEdits)}
	}
	for _, id := range deleted {
		state.DeletedRefs[id] = struct{}{}
	}
	block := CacheEditsBlockForDeletes(deleted)
	proofLedger, reclaimedBytes, proofBytes := cachedAgenticV2ProofLedger(messages, deleted)
	if len(block.Edits) > 0 {
		state.PinnedEdits = append(state.PinnedEdits, PinnedCacheEdits{
			UserMessageIndex: pinnedIndex,
			Block:            block,
			ProofLedger:      proofLedger,
		})
	}

	return CachedMicrocompactResult{
		Messages:       appendPinnedCacheEdits(messages, state.PinnedEdits),
		Changed:        len(deleted) > 0,
		DeletedToolIDs: deleted,
		ReclaimedBytes: reclaimedBytes,
		ProofBytes:     proofBytes,
		BytesSaved:     max(reclaimedBytes-proofBytes, 0),
	}
}

func CacheEditsBlockForDeletes(ids []string) CacheEditsBlock {
	edits := make([]CacheEdit, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		edits = append(edits, CacheEdit{Type: "delete", CacheReference: id})
	}
	return CacheEditsBlock{Type: string(ContentTypeCacheEdits), Edits: edits}
}

func appendPinnedCacheEdits(messages []types.Message, pinned []PinnedCacheEdits) []types.Message {
	if len(pinned) == 0 {
		return messages
	}
	out := make([]types.Message, len(messages))
	copy(out, messages)
	for _, edit := range pinned {
		if edit.UserMessageIndex < 0 || edit.UserMessageIndex >= len(out) {
			continue
		}
		if out[edit.UserMessageIndex].Role != types.RoleUser {
			continue
		}
		raw, err := json.Marshal(edit.Block)
		if err != nil {
			continue
		}
		msg := out[edit.UserMessageIndex]
		content := append([]types.ContentBlock(nil), msg.Content...)
		content = insertCacheCompactionBlocks(content, types.UnknownBlock{
			Type: ContentTypeCacheEdits,
			Raw:  raw,
		}, edit.ProofLedger)
		msg.Content = content
		out[edit.UserMessageIndex] = msg
	}
	return out
}

func insertCacheCompactionBlocks(content []types.ContentBlock, block types.ContentBlock, proofLedger string) []types.ContentBlock {
	lastToolResult := -1
	for i, item := range content {
		if _, ok := item.(types.ToolResultBlock); ok {
			lastToolResult = i
		}
	}
	insertions := []types.ContentBlock{block}
	if proofLedger != "" && !containsTextBlock(content, proofLedger) {
		insertions = append(insertions, types.TextBlock{Type: types.ContentTypeText, Text: proofLedger})
	}
	pos := lastToolResult + 1
	if lastToolResult < 0 {
		pos = len(content) - 1
		if pos < 0 {
			pos = 0
		}
	}
	needsContinuation := pos == len(content)
	out := make([]types.ContentBlock, 0, len(content)+len(insertions)+1)
	out = append(out, content[:pos]...)
	out = append(out, insertions...)
	out = append(out, content[pos:]...)
	if needsContinuation {
		out = append(out, types.TextBlock{Type: types.ContentTypeText, Text: "."})
	}
	return out
}

func containsTextBlock(content []types.ContentBlock, text string) bool {
	for _, block := range content {
		if value, ok := block.(types.TextBlock); ok && value.Text == text {
			return true
		}
	}
	return false
}

func collectCompactableToolIDs(messages []types.Message, cfg MicrocompactConfig) []string {
	var ids []string
	results := toolResultsByUseID(messages)
	for _, msg := range messages {
		if effectiveCompactionRole(msg) != types.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if tu, ok := block.(types.ToolUseBlock); ok {
				if legacyCompactableTools[tu.Name] {
					ids = append(ids, tu.ID)
					continue
				}
				if !cfg.AgenticV2ProofsEnabled || !isAgenticV2ProofTool(tu.Name) {
					continue
				}
				result, exists := results[tu.ID]
				if !exists {
					continue
				}
				proof, ok := agenticV2ProofContent(tu.Name, result)
				if ok && len(proof) < len(result.TextContent()) {
					ids = append(ids, tu.ID)
				}
			}
		}
	}
	return ids
}

func ensureCachedMicrocompactState(state *CachedMicrocompactState) {
	if state.RegisteredTools == nil {
		state.RegisteredTools = make(map[string]struct{})
	}
	if state.DeletedRefs == nil {
		state.DeletedRefs = make(map[string]struct{})
	}
}

func lastUserMessageIndex(messages []types.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		switch effectiveCompactionRole(messages[i]) {
		case types.RoleUser:
			return i
		case types.RoleDeveloper:
			return -1
		}
	}
	return -1
}
