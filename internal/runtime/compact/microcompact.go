package compact

import (
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// compactableTools lists tools whose results can be safely cleared.
// Matches TS COMPACTABLE_TOOLS: FileRead, Shell tools, Grep, Glob,
// WebSearch, WebFetch, FileEdit, FileWrite.
var compactableTools = map[string]bool{
	"Read": true, "Bash": true, "Grep": true, "Glob": true,
	"WebSearch": true, "WebFetch": true, "Write": true, "Edit": true,
}

// MicrocompactConfig controls microcompact behavior.
type MicrocompactConfig struct {
	// KeepRecent is the number of recent compactable tool results to preserve
	// when time-based microcompact fires. It is floored at 1 so the most recent
	// compactable result always remains available.
	KeepRecent int

	// TimeBasedEnabled controls the TS-equivalent time-based trigger.
	TimeBasedEnabled bool

	// QuerySource identifies the source of the request. Time-based microcompact
	// requires an explicit main-thread source; the zero value is undefined and
	// never triggers.
	QuerySource MicrocompactQuerySource

	// IdleThreshold is the duration after which old tool results can be cleared.
	// Zero disables time-based microcompact. Default: 60 minutes.
	IdleThreshold time.Duration

	// LastActivity records when the last assistant/API turn completed. Used as
	// the Go equivalent of TS's last assistant message timestamp.
	LastActivity time.Time

	// CachedEnabled enables Anthropic cache_edits-style microcompact. This path
	// preserves local message content and only adds provider-bound cache edit
	// directives when the prompt cache is expected to be warm.
	CachedEnabled bool

	// CachedTriggerThreshold is the active compactable tool-result count that
	// triggers cache_edits generation. Zero disables cached microcompact.
	CachedTriggerThreshold int

	// CachedKeepRecent is the number of most recent active tool results to
	// preserve after cached microcompact triggers. Zero falls back to KeepRecent.
	CachedKeepRecent int
}

type MicrocompactQuerySource string

const (
	MicrocompactSourceUndefined MicrocompactQuerySource = ""
	MicrocompactSourceMain      MicrocompactQuerySource = "repl_main_thread"
	MicrocompactSourceNonMain   MicrocompactQuerySource = "non_main"
)

type MicrocompactResult struct {
	Messages           []types.Message
	Changed            bool
	TimeBasedTriggered bool
	ToolsCleared       int
	ToolsKept          int
}

// DefaultMicrocompactConfig returns sensible defaults.
// QuerySource intentionally defaults to undefined; callers must opt in with an
// explicit main-thread source before time-based microcompact can fire.
func DefaultMicrocompactConfig() MicrocompactConfig {
	return MicrocompactConfig{
		KeepRecent:       5,
		TimeBasedEnabled: true,
		IdleThreshold:    60 * time.Minute,
		QuerySource:      MicrocompactSourceUndefined,
	}
}

func (cfg MicrocompactConfig) keepRecent() int {
	keep := cfg.KeepRecent
	if keep <= 0 {
		return 1
	}
	return keep
}

func (cfg MicrocompactConfig) shouldTimeBasedTrigger(messages []types.Message) bool {
	if !cfg.TimeBasedEnabled || cfg.QuerySource != MicrocompactSourceMain {
		return false
	}
	if cfg.IdleThreshold <= 0 || cfg.LastActivity.IsZero() {
		return false
	}
	if !hasAssistantMessage(messages) {
		return false
	}
	return time.Since(cfg.LastActivity) >= cfg.IdleThreshold
}

func MicrocompactWithResult(messages []types.Message, cfg MicrocompactConfig) MicrocompactResult {
	if !cfg.shouldTimeBasedTrigger(messages) {
		return MicrocompactResult{Messages: messages}
	}

	keepRecent := cfg.keepRecent()

	// First pass: find all compactable tool result positions (message index + block index)
	type resultPos struct {
		msgIdx   int
		blockIdx int
	}
	var positions []resultPos

	for i, msg := range messages {
		// Tool results are user-role protocol blocks. In particular, never
		// interpret developer catalog content as compactable conversation data.
		switch effectiveCompactionRole(msg) {
		case types.RoleDeveloper:
			continue
		case types.RoleUser:
		default:
			continue
		}
		for j, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				// Check if this is from a compactable tool by looking at the
				// preceding assistant message's tool_use name
				toolName := findToolName(messages, tr.ToolUseID)
				if compactableTools[toolName] {
					positions = append(positions, resultPos{i, j})
				}
			}
		}
	}

	// If we have fewer compactable results than keepRecent, nothing to clear
	clearCount := len(positions) - keepRecent
	if clearCount <= 0 {
		return MicrocompactResult{Messages: messages}
	}

	// Build a set of positions to clear (oldest N)
	toClear := make(map[[2]int]bool)
	for _, pos := range positions[:clearCount] {
		toClear[[2]int{pos.msgIdx, pos.blockIdx}] = true
	}

	// Second pass: create new messages with cleared content
	result := make([]types.Message, len(messages))
	for i, msg := range messages {
		// Start from the complete value so ID, IsMeta, and developer catalog
		// metadata survive microcompaction unchanged.
		result[i] = msg
		result[i].Content = make([]types.ContentBlock, 0, len(msg.Content))
		for j, block := range msg.Content {
			if toClear[[2]int{i, j}] {
				tr := block.(types.ToolResultBlock)
				tr.Content = microcompactClearedText()
				tr.ContentBlocks = nil
				result[i].Content = append(result[i].Content, tr)
			} else {
				result[i].Content = append(result[i].Content, block)
			}
		}
	}

	return MicrocompactResult{
		Messages:           result,
		Changed:            true,
		TimeBasedTriggered: true,
		ToolsCleared:       clearCount,
		ToolsKept:          len(positions) - clearCount,
	}
}

func microcompactClearedText() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCompactMicrocompactResultCleared)
}

// findToolName searches backward for a tool_use block matching the given ID.
func findToolName(messages []types.Message, toolUseID string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		switch effectiveCompactionRole(messages[i]) {
		case types.RoleDeveloper:
			continue
		case types.RoleAssistant:
		default:
			continue
		}
		for _, block := range messages[i].Content {
			if tu, ok := block.(types.ToolUseBlock); ok && tu.ID == toolUseID {
				return tu.Name
			}
		}
	}
	return ""
}

func hasAssistantMessage(messages []types.Message) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleAssistant {
			return true
		}
	}
	return false
}
