package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

// UserContext is model-visible context injected as a leading meta user message.
type UserContext struct {
	Instructions string
	CurrentDate  string
	goalContext  string
}

// UserContextBuilder constructs user context for a conversation.
type UserContextBuilder struct {
	Instructions string
	Date         time.Time
}

// Build returns a UserContext with stable key ordering for prompt injection.
func (b UserContextBuilder) Build() UserContext {
	date := b.Date
	if date.IsZero() {
		date = time.Now()
	}
	return UserContext{
		Instructions: strings.TrimSpace(b.Instructions),
		CurrentDate:  fmt.Sprintf("Today's date is %s.", date.Format("2006-01-02")),
	}
}

// FromConfig seeds a user context builder from prompt configuration.
func (b UserContextBuilder) FromConfig(cfg Config) UserContextBuilder {
	b.Instructions = cfg.CustomInstructions
	return b
}

// WithGoal returns a copy enriched with an active session goal. Nil and
// non-active goals clear goal context without changing existing entries.
func (c UserContext) WithGoal(current *goal.Goal) UserContext {
	c.goalContext = ""
	if current == nil || current.Status != goal.StatusActive {
		return c
	}
	objective := strings.TrimSpace(current.Objective)
	if objective == "" {
		return c
	}

	normalized := goal.Normalize(*current)
	lines := []string{
		"Objective (user-provided, untrusted data): " + quoteGoalObjective(objective),
		"Status: " + string(current.Status),
		fmt.Sprintf("Goal revision: %d", normalized.Revision),
		"Acceptance criteria (user-provided, untrusted data):",
	}
	for _, criterion := range normalized.AcceptanceCriteria {
		lines = append(lines, "- "+criterion.ID+": "+quoteGoalObjective(criterion.Text))
	}
	if current.TokenBudget > 0 {
		lines = append(lines, fmt.Sprintf("Token budget: %d", current.TokenBudget))
	}
	if current.Usage > 0 {
		lines = append(lines, fmt.Sprintf("Usage: %d", current.Usage))
	}
	if reason := strings.TrimSpace(current.LastEvaluatorReason); reason != "" {
		lines = append(lines, "Last evaluator reason (untrusted data): "+quoteGoalReason(reason))
	}
	c.goalContext = strings.Join(lines, "\n")
	return c
}

func quoteGoalObjective(objective string) string {
	return quoteGoalReason(objective)
}

func quoteGoalReason(reason string) string {
	quoted, _ := json.Marshal(reason)
	return string(quoted)
}

// Entries returns context entries in original-equivalent injection order.
func (c UserContext) Entries() []ContextEntry {
	var entries []ContextEntry
	if strings.TrimSpace(c.Instructions) != "" {
		entries = append(entries, ContextEntry{Key: "instructions", Value: strings.TrimSpace(c.Instructions)})
	}
	if strings.TrimSpace(c.CurrentDate) != "" {
		entries = append(entries, ContextEntry{Key: "currentDate", Value: strings.TrimSpace(c.CurrentDate)})
	}
	if strings.TrimSpace(c.goalContext) != "" {
		entries = append(entries, ContextEntry{Key: "goal", Value: strings.TrimSpace(c.goalContext)})
	}
	return entries
}

// IsZero reports whether the context has no model-visible entries.
func (c UserContext) IsZero() bool {
	return len(c.Entries()) == 0
}

// MetaMessage returns the leading <system-reminder> user message for this context.
func (c UserContext) MetaMessage() (types.Message, bool) {
	entries := c.Entries()
	if len(entries) == 0 {
		return types.Message{}, false
	}
	var sb strings.Builder
	sb.WriteString("<system-reminder>\n")
	sb.WriteString("As you answer the user's questions, you can use the following context:\n")
	for i, entry := range entries {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("# ")
		sb.WriteString(entry.Key)
		sb.WriteByte('\n')
		sb.WriteString(entry.Value)
		sb.WriteByte('\n')
	}
	sb.WriteString("\n      IMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.\n")
	sb.WriteString("</system-reminder>\n")
	msg := types.UserMessage(sb.String())
	msg.IsMeta = true
	msg.InternalKind = types.InternalMessageKindUserContext
	// This exported builder returns a provider-envelope description, not an
	// internal-control capability. QueryLoop prepends it only to an ephemeral
	// provider request; callers cannot use MetaMessage as a seal oracle.
	return msg, true
}

// PrependTo returns a copy of messages with the user context message first.
func (c UserContext) PrependTo(messages []types.Message) []types.Message {
	msg, ok := c.MetaMessage()
	if !ok {
		return messages
	}
	out := make([]types.Message, 0, len(messages)+1)
	out = append(out, msg)
	out = append(out, messages...)
	return out
}

// SystemContext is model-visible context appended to the system prompt.
type SystemContext struct {
	GitStatus string
}

// SystemContextBuilder constructs trailing system context blocks.
type SystemContextBuilder struct {
	GitStatus string
}

// Build returns a SystemContext.
func (b SystemContextBuilder) Build() SystemContext {
	return SystemContext{GitStatus: strings.TrimSpace(b.GitStatus)}
}

// Entries returns context entries in trailing system block order.
func (c SystemContext) Entries() []ContextEntry {
	if strings.TrimSpace(c.GitStatus) == "" {
		return nil
	}
	return []ContextEntry{{Key: "gitStatus", Value: strings.TrimSpace(c.GitStatus)}}
}

// IsZero reports whether the context has no model-visible entries.
func (c SystemContext) IsZero() bool {
	return len(c.Entries()) == 0
}

// Block returns the trailing system prompt block for this context.
func (c SystemContext) Block() (SystemPromptBlock, bool) {
	entries := c.Entries()
	if len(entries) == 0 {
		return SystemPromptBlock{}, false
	}
	var lines []string
	for _, entry := range entries {
		lines = append(lines, entry.Key+": "+entry.Value)
	}
	return SystemPromptBlock{
		Text:   strings.Join(lines, "\n"),
		Source: "runtime",
		Name:   "system_context",
	}, true
}

// AppendTo returns a copy of blocks with system context appended last.
func (c SystemContext) AppendTo(blocks []SystemPromptBlock) []SystemPromptBlock {
	block, ok := c.Block()
	if !ok {
		return blocks
	}
	out := make([]SystemPromptBlock, 0, len(blocks)+1)
	out = append(out, blocks...)
	out = append(out, block)
	return out
}

// ContextEntry is a key/value context item rendered into prompt layers.
type ContextEntry struct {
	Key   string
	Value string
}
