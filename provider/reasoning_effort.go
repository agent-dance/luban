package provider

import "strings"

// DefaultReasoningEffort returns the catalog default for a model's selectable
// reasoning tiers. Medium is preferred when available; provider-defined lists
// that do not include it retain their first advertised tier as the default.
func DefaultReasoningEffort(efforts []string) string {
	if len(efforts) == 0 {
		return ""
	}
	for _, effort := range efforts {
		if effort == "medium" {
			return effort
		}
	}
	return efforts[0]
}

// DefaultReasoningEffortForModel returns the model-specific default when it is
// one of the advertised tiers, then falls back to the shared selection rule.
func DefaultReasoningEffortForModel(model ModelInfo) string {
	configured := strings.TrimSpace(model.DefaultReasoningEffort)
	for _, effort := range model.ReasoningEfforts {
		if effort == configured {
			return configured
		}
	}
	return DefaultReasoningEffort(model.ReasoningEfforts)
}

// reasoningEffortForRequest mirrors current Codex behavior: Ultra is a client
// orchestration preset, while the API request itself uses the Max effort.
func reasoningEffortForRequest(effort string) string {
	effort = strings.TrimSpace(effort)
	if strings.EqualFold(effort, "ultra") {
		return "max"
	}
	return effort
}
