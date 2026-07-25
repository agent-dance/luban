package skills

const (
	// skillBudgetContextPercent is the fraction of the context window allocated
	// to skill listing (1%).
	skillBudgetContextPercent = 0.01

	// charsPerToken is the average characters per token for budget calculation.
	charsPerToken = 4

	// defaultCharBudget is the fallback budget when no context window size
	// is provided: 1% of 200k tokens × 4 chars/token = 8000 chars.
	defaultCharBudget = 8_000

	// maxListingDescChars is the per-entry hard cap for skill descriptions.
	// The listing is for discovery only — the Skill tool loads full content on
	// invoke, so verbose whenToUse strings waste cache_creation tokens.
	maxListingDescChars = 250
)

// GetCharBudget returns the character budget for skill listing.
// If contextWindowTokens > 0, budget = tokens × charsPerToken ×
// skillBudgetContextPercent. Otherwise it returns defaultCharBudget.
func GetCharBudget(contextWindowTokens int) int {
	if contextWindowTokens > 0 {
		return int(float64(contextWindowTokens) * charsPerToken * skillBudgetContextPercent)
	}
	return defaultCharBudget
}

// IsUserInvocable returns whether the skill can be invoked by users via /command.
// If UserInvocable is nil, skills default to user-invocable.
func (s *Skill) IsUserInvocable() bool {
	if s.UserInvocable != nil {
		return *s.UserInvocable
	}
	// Skills are user-invocable unless their manifest explicitly disables it.
	return true
}
