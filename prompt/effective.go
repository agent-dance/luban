package prompt

import "strings"

// EffectiveSystemPromptInput describes every prompt source that can contribute
// to the final system prompt. Precedence mirrors the original TypeScript path:
// override, coordinator, agent, custom, default, then append.
type EffectiveSystemPromptInput struct {
	Override    string
	Coordinator string
	Agent       string
	Custom      string
	Default     SystemPrompt
	Append      string
}

// BuildEffectiveSystemPrompt returns the ordered system prompt blocks after
// applying original precedence rules. Override replaces everything, including
// append. Custom replaces only the default prompt and still receives append.
func BuildEffectiveSystemPrompt(in EffectiveSystemPromptInput) SystemPrompt {
	if text := strings.TrimSpace(in.Override); text != "" {
		return SystemPrompt{{Text: text, Source: "override", Name: "override"}}
	}

	var base SystemPrompt
	switch {
	case strings.TrimSpace(in.Coordinator) != "":
		base = SystemPrompt{{Text: strings.TrimSpace(in.Coordinator), Source: "coordinator", Name: "coordinator"}}
	case strings.TrimSpace(in.Agent) != "":
		base = SystemPrompt{{Text: strings.TrimSpace(in.Agent), Source: "agent", Name: "agent"}}
	case strings.TrimSpace(in.Custom) != "":
		base = SystemPrompt{{Text: strings.TrimSpace(in.Custom), Source: "custom", Name: "custom"}}
	default:
		base = cloneSystemPrompt(in.Default)
	}

	if text := strings.TrimSpace(in.Append); text != "" {
		base = append(base, SystemPromptBlock{Text: text, Source: "append", Name: "append"})
	}
	return base
}

func cloneSystemPrompt(in SystemPrompt) SystemPrompt {
	if len(in) == 0 {
		return nil
	}
	out := make(SystemPrompt, len(in))
	copy(out, in)
	return out
}
