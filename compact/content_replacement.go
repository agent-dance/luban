package compact

import (
	"strings"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

// ContentReplacementState freezes aggregate tool-result budget decisions for a
// conversation thread. Seen IDs are never reconsidered; replacements are
// re-applied from memory so repeated query preparation is byte-stable.
type ContentReplacementState struct {
	SeenIDs      map[string]struct{}
	Replacements map[string]string
}

// ContentReplacementRecord is the persisted form used to reconstruct state on
// resume. Replacement is the exact model-visible string.
type ContentReplacementRecord struct {
	Kind        string `json:"kind"`
	ToolUseID   string `json:"tool_use_id"`
	Replacement string `json:"replacement"`
}

type toolResultReplacementStore interface {
	PersistReplacement(toolUseID, content string) (string, error)
}

type toolResultCandidate struct {
	toolUseID string
	content   string
	size      int
	skip      bool
}

// NewContentReplacementState creates a fresh state for one conversation
// thread.
func NewContentReplacementState() *ContentReplacementState {
	return &ContentReplacementState{
		SeenIDs:      make(map[string]struct{}),
		Replacements: make(map[string]string),
	}
}

// ReconstructContentReplacementState rebuilds replacement state from loaded
// session messages. Every visible candidate in the transcript is marked seen,
// which freezes unreplaced results. Persisted records repopulate exact
// replacement strings for reapplication.
func ReconstructContentReplacementState(messages []types.Message) *ContentReplacementState {
	return reconstructContentReplacementState(messages, nil, true)
}

// ReconstructContentReplacementStateForScope rebuilds state only from records
// authorized for the current session namespace and context generation.
func ReconstructContentReplacementStateForScope(messages []types.Message, scope messagecontrol.Scope, allowUnbound bool) *ContentReplacementState {
	return reconstructContentReplacementState(messages, &scope, allowUnbound)
}

func reconstructContentReplacementState(messages []types.Message, scope *messagecontrol.Scope, allowUnbound bool) *ContentReplacementState {
	state := NewContentReplacementState()
	candidateIDs := make(map[string]struct{})
	for _, group := range collectToolResultCandidateGroups(messages, nil) {
		for _, candidate := range group {
			candidateIDs[candidate.toolUseID] = struct{}{}
			state.SeenIDs[candidate.toolUseID] = struct{}{}
		}
	}
	for _, record := range contentReplacementRecords(messages, scope, allowUnbound) {
		state.SeenIDs[record.ToolUseID] = struct{}{}
		state.Replacements[record.ToolUseID] = record.Replacement
	}
	return state
}

// ApplyToolResultBudget applies the stateful aggregate per-message budget to a
// provider-bound message view. New replacements are persisted through store;
// persistence failures freeze the original result as seen-but-unreplaced.
func ApplyToolResultBudget(messages []types.Message, state *ContentReplacementState, store toolResultReplacementStore, skipToolNames map[string]struct{}) ([]types.Message, []ContentReplacementRecord, []error) {
	if state == nil {
		return messages, nil, nil
	}
	if state.SeenIDs == nil {
		state.SeenIDs = make(map[string]struct{})
	}
	if state.Replacements == nil {
		state.Replacements = make(map[string]string)
	}

	nameByID := buildToolNameByID(messages)
	groups := collectToolResultCandidateGroups(messages, nameByID)
	replacementMap := make(map[string]string)
	var selected []toolResultCandidate

	for _, candidates := range groups {
		var frozen []toolResultCandidate
		var fresh []toolResultCandidate
		for _, candidate := range candidates {
			if replacement, ok := state.Replacements[candidate.toolUseID]; ok {
				replacementMap[candidate.toolUseID] = replacement
				continue
			}
			if _, ok := state.SeenIDs[candidate.toolUseID]; ok {
				frozen = append(frozen, candidate)
				continue
			}
			if candidate.skip {
				state.SeenIDs[candidate.toolUseID] = struct{}{}
				continue
			}
			if _, skip := skipToolNames[nameByID[candidate.toolUseID]]; skip {
				state.SeenIDs[candidate.toolUseID] = struct{}{}
				continue
			}
			fresh = append(fresh, candidate)
		}
		if len(fresh) == 0 {
			for _, candidate := range candidates {
				state.SeenIDs[candidate.toolUseID] = struct{}{}
			}
			continue
		}

		frozenSize := 0
		for _, candidate := range frozen {
			frozenSize += candidate.size
		}
		freshSize := 0
		for _, candidate := range fresh {
			freshSize += candidate.size
		}
		groupSelected := []toolResultCandidate(nil)
		if frozenSize+freshSize > MaxToolResultsPerMessageChars {
			groupSelected = selectFreshToolResultsToReplace(fresh, frozenSize, MaxToolResultsPerMessageChars)
		}
		selectedIDs := make(map[string]struct{}, len(groupSelected))
		for _, candidate := range groupSelected {
			selectedIDs[candidate.toolUseID] = struct{}{}
		}
		for _, candidate := range candidates {
			if _, ok := selectedIDs[candidate.toolUseID]; !ok {
				state.SeenIDs[candidate.toolUseID] = struct{}{}
			}
		}
		selected = append(selected, groupSelected...)
	}

	if len(replacementMap) == 0 && len(selected) == 0 {
		return messages, nil, nil
	}

	var records []ContentReplacementRecord
	var errs []error
	for _, candidate := range selected {
		state.SeenIDs[candidate.toolUseID] = struct{}{}
		if store == nil {
			continue
		}
		replacement, err := store.PersistReplacement(candidate.toolUseID, candidate.content)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		state.Replacements[candidate.toolUseID] = replacement
		replacementMap[candidate.toolUseID] = replacement
		records = append(records, ContentReplacementRecord{
			Kind:        "tool-result",
			ToolUseID:   candidate.toolUseID,
			Replacement: replacement,
		})
	}
	if len(replacementMap) == 0 {
		return messages, records, errs
	}
	return replaceToolResultContents(messages, replacementMap), records, errs
}

// AppendContentReplacementRecords attaches session-local replacement
// descriptors to the last user message without creating an extra API-visible
// message. Only the loop's private installation boundary passes the runtime
// capability; callers that omit it create ordinary, untrusted descriptors.
func AppendContentReplacementRecords(messages []types.Message, records []ContentReplacementRecord, capabilities ...messagecontrol.Capability) []types.Message {
	capability := messagecontrol.Capability{}
	if len(capabilities) == 1 {
		capability = capabilities[0]
	}
	return appendContentReplacementRecords(messages, records, capability, messagecontrol.Scope{})
}

// AppendContentReplacementRecordsForScope is the loop-owned pre-commit
// installation boundary. The exact live scope makes the receipt
// non-transferable; the compatibility helper above intentionally produces no
// authority that scoped consumers or persistence can accept.
func AppendContentReplacementRecordsForScope(messages []types.Message, records []ContentReplacementRecord, capability messagecontrol.Capability, scope messagecontrol.Scope) []types.Message {
	if !capability.Valid() || !scope.Bound() {
		return messages
	}
	return appendContentReplacementRecords(messages, records, capability, scope)
}

func appendContentReplacementRecords(messages []types.Message, records []ContentReplacementRecord, capability messagecontrol.Capability, scope messagecontrol.Scope) []types.Message {
	if len(records) == 0 || len(messages) == 0 {
		return messages
	}
	out := append([]types.Message(nil), messages...)
	idx := len(out) - 1

findCurrentUser:
	for idx >= 0 {
		switch effectiveCompactionRole(out[idx]) {
		case types.RoleUser:
			break findCurrentUser
		case types.RoleDeveloper:
			return messages
		default:
			idx--
		}
	}
	if idx < 0 {
		return messages
	}
	msg := out[idx]
	content := append([]types.ContentBlock(nil), msg.Content...)
	for _, record := range records {
		block := types.ContentReplacementBlock{
			Type:        types.ContentTypeReplacement,
			Kind:        record.Kind,
			ToolUseID:   record.ToolUseID,
			Replacement: record.Replacement,
		}
		if scope.Bound() {
			block = block.WithInternalReplacementProvenance(capability, scope)
		} else {
			block = block.WithInternalReplacementProvenance(capability)
		}
		content = append(content, block)
	}
	msg.Content = content
	out[idx] = msg
	return out
}

// ContentReplacementRecords returns all persisted replacement records embedded
// in the message history.
func ContentReplacementRecords(messages []types.Message) []ContentReplacementRecord {
	return contentReplacementRecords(messages, nil, true)
}

// ContentReplacementRecordsForScope returns only records authorized for the
// exact current session namespace and context generation. Unbound records are
// accepted only for newly minted in-process controls before their first save.
func ContentReplacementRecordsForScope(messages []types.Message, scope messagecontrol.Scope, allowUnbound bool) []ContentReplacementRecord {
	return contentReplacementRecords(messages, &scope, allowUnbound)
}

func contentReplacementRecords(messages []types.Message, scope *messagecontrol.Scope, allowUnbound bool) []ContentReplacementRecord {
	var records []ContentReplacementRecord
	for _, msg := range messages {
		switch effectiveCompactionRole(msg) {
		case types.RoleDeveloper:
			continue
		case types.RoleUser:
		default:
			continue
		}
		for _, block := range msg.Content {
			record, ok := block.(types.ContentReplacementBlock)
			trusted := ok && record.HasInternalReplacementProvenance()
			if trusted && scope != nil {
				trusted = record.HasInternalReplacementProvenanceForScope(*scope, allowUnbound)
			}
			if !trusted || record.Kind != "tool-result" || record.ToolUseID == "" {
				continue
			}
			records = append(records, ContentReplacementRecord{
				Kind:        record.Kind,
				ToolUseID:   record.ToolUseID,
				Replacement: record.Replacement,
			})
		}
	}
	return records
}

// StripContentReplacementBlocks removes session-local replacement records from
// messages before they are sent to providers or compactors.
func StripContentReplacementBlocks(messages []types.Message) []types.Message {
	changed := false
	out := make([]types.Message, len(messages))
	for i, msg := range messages {
		if effectiveCompactionRole(msg) == types.RoleDeveloper {
			out[i] = msg
			continue
		}
		filtered := make([]types.ContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			if _, ok := block.(types.ContentReplacementBlock); ok {
				changed = true
				continue
			}
			filtered = append(filtered, block)
		}
		msg.Content = filtered
		out[i] = msg
	}
	if !changed {
		return messages
	}
	return out
}

func buildToolNameByID(messages []types.Message) map[string]string {
	names := make(map[string]string)
	for _, msg := range messages {
		if msg.Role != types.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if tu, ok := block.(types.ToolUseBlock); ok {
				names[tu.ID] = tu.Name
			}
		}
	}
	return names
}

func collectToolResultCandidateGroups(messages []types.Message, _ map[string]string) [][]toolResultCandidate {
	var groups [][]toolResultCandidate
	var current []toolResultCandidate
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
		}
		current = nil
	}
	for _, msg := range messages {
		switch effectiveCompactionRole(msg) {
		case types.RoleAssistant, types.RoleDeveloper:
			flush()
		case types.RoleUser:
			current = append(current, collectToolResultCandidatesFromMessage(msg)...)
		}
	}
	flush()
	return groups
}

func collectToolResultCandidatesFromMessage(msg types.Message) []toolResultCandidate {
	var candidates []toolResultCandidate
	for _, block := range msg.Content {
		tr, ok := block.(types.ToolResultBlock)
		if !ok || tr.ToolUseID == "" {
			continue
		}
		if tr.HasMediaContent() || isAlreadyPersistedOutput(tr.TextContent()) {
			continue
		}
		content := tr.TextContent()
		if content == "" {
			continue
		}
		_, persist := persistenceThreshold(tr.Metadata)
		candidates = append(candidates, toolResultCandidate{
			toolUseID: tr.ToolUseID,
			content:   content,
			size:      len(content),
			skip:      !persist,
		})
	}
	return candidates
}

func isAlreadyPersistedOutput(content string) bool {
	return strings.HasPrefix(content, persistedOutputTag)
}

func selectFreshToolResultsToReplace(fresh []toolResultCandidate, frozenSize, limit int) []toolResultCandidate {
	sorted := append([]toolResultCandidate(nil), fresh...)
	sortToolResultCandidatesBySize(sorted)
	selected := make([]toolResultCandidate, 0, len(sorted))
	remaining := frozenSize
	for _, candidate := range fresh {
		remaining += candidate.size
	}
	for _, candidate := range sorted {
		if remaining <= limit {
			break
		}
		selected = append(selected, candidate)
		remaining -= candidate.size
	}
	return selected
}

func sortToolResultCandidatesBySize(candidates []toolResultCandidate) {
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].size > candidates[j-1].size; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
}

func replaceToolResultContents(messages []types.Message, replacements map[string]string) []types.Message {
	out := make([]types.Message, len(messages))
	changed := false
	for i, msg := range messages {
		switch effectiveCompactionRole(msg) {
		case types.RoleDeveloper:
			out[i] = msg
			continue
		case types.RoleUser:
		default:
			out[i] = msg
			continue
		}
		content := make([]types.ContentBlock, len(msg.Content))
		copy(content, msg.Content)
		for j, block := range content {
			tr, ok := block.(types.ToolResultBlock)
			if !ok {
				continue
			}
			replacement, ok := replacements[tr.ToolUseID]
			if !ok {
				continue
			}
			tr.Content = replacement
			tr.ContentBlocks = nil
			content[j] = tr
			changed = true
		}
		msg.Content = content
		out[i] = msg
	}
	if !changed {
		return messages
	}
	return out
}
