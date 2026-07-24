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

// reasoningEffortForRequest mirrors current Codex behavior: Ultra is a client
// orchestration preset, while the API request itself uses the Max effort.
func reasoningEffortForRequest(effort string) string {
	effort = strings.TrimSpace(effort)
	if strings.EqualFold(effort, "ultra") {
		return "max"
	}
	return effort
}
