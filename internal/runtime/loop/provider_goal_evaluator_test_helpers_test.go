package loop

import "github.com/agent-dance/luban/provider"

func NewProviderGoalEvaluator(value provider.Provider) *ProviderGoalEvaluator {
	return NewProviderGoalEvaluatorWithModel(value, "")
}
