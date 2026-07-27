package harness

import "fmt"

// ValidatePromptCacheKeyIsolation proves that a cache namespace is never
// reused across an agent, task, or repetition. Per-attempt validation already
// requires one stable key; this experiment-level gate detects cross-run reuse.
func ValidatePromptCacheKeyIsolation(state ExperimentState) error {
	owners := make(map[string]string)
	for runKey, record := range state.Runs {
		if record.Execution == nil || record.Execution.EvidencePath == "" {
			continue
		}
		rounds, err := ReadJSONLines[ProviderRoundEvidence](record.Execution.EvidencePath)
		if err != nil {
			return fmt.Errorf("read provider evidence for cache isolation %s: %w", runKey, err)
		}
		firstKey := ""
		for _, round := range rounds {
			if round.ProviderAttemptKind == "inference" && round.PromptCacheKeyPresent {
				firstKey = round.PromptCacheKeyHash
				break
			}
		}
		if firstKey == "" {
			continue
		}
		if owner, duplicate := owners[firstKey]; duplicate && owner != runKey {
			return fmt.Errorf("prompt cache key is shared across formal runs %s and %s", owner, runKey)
		}
		owners[firstKey] = runKey
	}
	return nil
}
