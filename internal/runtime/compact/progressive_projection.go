package compact

import (
	"strings"

	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/types"
)

const (
	// Source reads are the working set for a code change. Keep the most recent
	// reads lossless even after a mutation has been consumed so a failed
	// verification can be diagnosed without rediscovering exact signatures.
	progressiveProtectedInspectResults = 2
	progressiveProtectedRunResults     = 2
	progressiveProtectedPatchResults   = 1
)

const (
	ProgressiveDecisionAdmittedAvoidCompact = "admit_avoid_compact"
	ProgressiveDecisionAdmittedNetSavings   = "admit_net_savings"
	ProgressiveDecisionKeepDisabled         = "keep_disabled"
	ProgressiveDecisionKeepIncomplete       = "keep_incomplete_estimate"
	ProgressiveDecisionKeepUnknownPricing   = "keep_unknown_pricing"
	ProgressiveDecisionKeepTokenSavings     = "keep_token_savings"
	ProgressiveDecisionKeepCompactThreshold = "keep_compact_threshold"
	ProgressiveDecisionKeepCost             = "keep_cost"
	ProgressiveDecisionKeepSessionBudget    = "keep_session_budget"
	ProgressiveDecisionKeepAnomaly          = "keep_anomaly"
	ProgressiveDecisionShadow               = "shadow"
)

// ProgressiveTokenPricing is the input-side portion of one model's published
// pricing. Values are USD per million tokens. Output pricing is deliberately
// excluded because admission credits only direct replacement savings; merely
// delaying a semantic-compaction response is not treated as avoided spend.
type ProgressiveTokenPricing struct {
	InputPerMtok     float64
	CacheReadPerMtok float64
	Known            bool
}

// ProgressiveProjectionAdmission freezes every value used by the token/cost
// gate for one preparation attempt. The caller supplies the complete provider
// request estimate so tool-result bytes are never used as a proxy for context
// or price.
type ProgressiveProjectionAdmission struct {
	Enabled                    bool
	Shadow                     bool
	Pressure                   bool
	Counter                    TokenCounter
	RawRequestTokens           int
	RawRequestEstimateKnown    bool
	AutoCompactThreshold       int
	PreviousCacheReadTokens    int
	PreviousUsageKnown         bool
	Pricing                    ProgressiveTokenPricing
	MinTokenSavings            int
	ReuseHorizon               int
	CacheRecoveryRequests      int
	ImminentCompactResetsCache bool
	RequireConsumedMutation    bool
	MinNetSavingsUSD           float64
	RemainingTools             int
	RemainingProjectedTokens   int
	AllowedTools               map[string]struct{}
}

// ProgressiveProjectionResult describes one frozen provider-view update. Raw
// history is not modified: Messages contains only the provider projection and
// Records is the durable private ledger the loop must append to its history.
type ProgressiveProjectionResult struct {
	Messages                   []types.Message
	Records                    []ContentReplacementRecord
	Changed                    bool
	Trigger                    string
	ProjectedTools             int
	RewrittenTools             int
	IndexedTools               int
	OriginalBytes              int
	ProjectedBytes             int
	BytesSaved                 int
	OriginalTokens             int
	ProjectedTokens            int
	TokensSaved                int
	ProjectedRequestTokens     int
	RawRequestTokens           int
	CacheBreakCostUSD          float64
	GrossCacheBreakCostUSD     float64
	AvoidedCompactInputCostUSD float64
	EstimatedNetSavingsUSD     float64
	AvoidsImmediateCompaction  bool
	Decision                   string
	Shadow                     bool
}

type progressiveInspectCandidate struct {
	toolUseID       string
	toolName        string
	original        string
	projected       string
	indexed         string
	originalTokens  int
	projectedTokens int
	indexedTokens   int
}

// ApplyProgressiveToolResultProjection performs a conservative phase-boundary
// projection. Investigation results remain lossless while the model is still
// deciding what to change. Once a successful ApplyPatch result has itself been
// consumed by a later assistant decision, older Inspect results may be frozen
// as deterministic proofs, while the most recent source reads remain intact.
//
// The production default admits only Inspect. Reviewed Run and ApplyPatch
// rewrites remain available behind the per-tool allowlist for measurement;
// failures stay active diagnostic evidence and the token/cost gate rejects
// batches that cannot repay a continuation reset.
func ApplyProgressiveToolResultProjection(messages []types.Message, state *ContentReplacementState, admission ProgressiveProjectionAdmission) ProgressiveProjectionResult {
	result := ProgressiveProjectionResult{Messages: messages, Decision: ProgressiveDecisionKeepDisabled}
	if state == nil || len(messages) == 0 || !admission.Enabled {
		return result
	}
	if state.SeenIDs == nil {
		state.SeenIDs = make(map[string]struct{})
	}
	if state.Replacements == nil {
		state.Replacements = make(map[string]string)
	}

	latestAssistant := latestAssistantMessageIndex(messages)
	if latestAssistant <= 0 {
		return result
	}
	toolNames := buildToolNameByID(messages)
	projectionBoundary, mutationSucceeded, mutationFound := latestConsumedMutation(messages, toolNames, latestAssistant)
	trigger := "consumed_mutation"
	if mutationFound && !mutationSucceeded {
		return result
	}
	if !mutationFound {
		if admission.RequireConsumedMutation {
			return result
		}
		if !admission.Pressure {
			return result
		}
		// A long investigation can hit semantic compaction before its first
		// mutation. Permit a pressure projection whenever enough source reads
		// have aged out of the protected set. The token/cost and session gates,
		// rather than an arbitrary result-count cap, bound each cache break.
		frozen := countFrozenProgressiveInspects(messages, state, toolNames, latestAssistant)
		projectionBoundary = latestAssistant
		if frozen == 0 {
			trigger = "working_set_pressure"
		} else {
			trigger = "working_set_pressure_top_up"
		}
	}

	var candidates []progressiveInspectCandidate
	for messageIndex := 0; messageIndex < projectionBoundary; messageIndex++ {
		if effectiveCompactionRole(messages[messageIndex]) != types.RoleUser {
			continue
		}
		for _, block := range messages[messageIndex].Content {
			toolResult, ok := block.(types.ToolResultBlock)
			if !ok || toolResult.ToolUseID == "" || toolResult.HasMediaContent() {
				continue
			}
			toolName := toolNames[toolResult.ToolUseID]
			if !progressiveAdmissionAllowsTool(admission, toolName) || strings.HasPrefix(trigger, "working_set_pressure") && toolName != "Inspect" {
				continue
			}
			if _, frozen := state.Replacements[toolResult.ToolUseID]; frozen {
				continue
			}
			original := toolResult.TextContent()
			if original == "" || isAlreadyPersistedOutput(original) {
				continue
			}
			projection, index, valid := progressiveToolProjection(toolName, toolResult, original)
			if !valid || len(projection) >= len(original) {
				continue
			}
			candidate := progressiveInspectCandidate{
				toolUseID: toolResult.ToolUseID,
				toolName:  toolName,
				original:  original,
				projected: projection,
				indexed:   index,
			}
			if admission.Counter != nil {
				candidate.originalTokens = admission.Counter.Count(original)
				candidate.projectedTokens = admission.Counter.Count(projection)
				if index != "" {
					candidate.indexedTokens = admission.Counter.Count(index)
				}
			}
			candidates = append(candidates, candidate)
		}
	}
	protectedInspectResults := progressiveProtectedInspectResults
	if strings.HasPrefix(trigger, "working_set_pressure") {
		// Under actual context pressure, retain the newest source result in full
		// and allow older results to fall back to recoverable indexes. Keeping two
		// full reads made the only batch capable of avoiding immediate semantic
		// compaction remain cache-negative in the measured 60k trace.
		protectedInspectResults = 1
	}
	candidates = filterProtectedProgressiveResults(candidates, protectedInspectResults)
	if len(candidates) == 0 {
		return result
	}
	if admission.RemainingTools > 0 && len(candidates) > admission.RemainingTools {
		candidates = candidates[:admission.RemainingTools]
	}
	if admission.Pressure {
		candidates = selectProgressivePressureBatch(candidates, admission)
	}
	if len(candidates) == 0 {
		result.Decision = ProgressiveDecisionKeepSessionBudget
		return result
	}
	result.Trigger = trigger
	result.ProjectedTools = len(candidates)

	for _, candidate := range candidates {
		if candidate.indexed != "" && candidate.projected == candidate.indexed {
			result.IndexedTools++
		} else {
			result.RewrittenTools++
		}
		result.OriginalBytes += len(candidate.original)
		result.ProjectedBytes += len(candidate.projected)
		result.OriginalTokens += candidate.originalTokens
		result.ProjectedTokens += candidate.projectedTokens
	}
	result.BytesSaved = result.OriginalBytes - result.ProjectedBytes
	result.TokensSaved = result.OriginalTokens - result.ProjectedTokens
	if admission.RemainingProjectedTokens > 0 && result.ProjectedTokens > admission.RemainingProjectedTokens {
		result.Decision = ProgressiveDecisionKeepSessionBudget
		return result
	}
	decision := evaluateProgressiveProjectionAdmission(&result, admission)
	result.Decision = decision
	if decision != ProgressiveDecisionAdmittedAvoidCompact && decision != ProgressiveDecisionAdmittedNetSavings {
		return result
	}
	if admission.Shadow {
		result.Decision = ProgressiveDecisionShadow
		result.Shadow = true
		return result
	}

	replacements := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		state.SeenIDs[candidate.toolUseID] = struct{}{}
		state.Replacements[candidate.toolUseID] = candidate.projected
		replacements[candidate.toolUseID] = candidate.projected
		result.Records = append(result.Records, ContentReplacementRecord{
			Kind:        "tool-result",
			ToolUseID:   candidate.toolUseID,
			Replacement: candidate.projected,
		})
	}
	result.Messages = replaceToolResultContents(messages, replacements)
	result.Changed = true
	return result
}

// selectProgressivePressureBatch chooses the smallest deterministic prefix
// that the complete token/cost gate would admit. Crossing the semantic-
// compaction threshold is necessary but not sufficient: when a smaller prefix
// would still lose money after invalidating the cache, selection continues
// until the cache-break cost is repaid. If no prefix is admissible, returning
// the largest budgeted prefix preserves useful rejection telemetry.
func selectProgressivePressureBatch(candidates []progressiveInspectCandidate, admission ProgressiveProjectionAdmission) []progressiveInspectCandidate {
	limit := len(candidates)
	if admission.RemainingTools > 0 {
		limit = min(limit, admission.RemainingTools)
	}
	if limit == 0 {
		return nil
	}
	probe := ProgressiveProjectionResult{}
	for index := 0; index < limit; index++ {
		probe.OriginalTokens += candidates[index].originalTokens
		probe.ProjectedTokens += candidates[index].projectedTokens
		probe.TokensSaved = probe.OriginalTokens - probe.ProjectedTokens
		if admission.RemainingProjectedTokens > 0 && probe.ProjectedTokens > admission.RemainingProjectedTokens {
			continue
		}
		decision := evaluateProgressiveProjectionAdmission(&probe, admission)
		if decision == ProgressiveDecisionAdmittedAvoidCompact || decision == ProgressiveDecisionAdmittedNetSavings {
			return candidates[:index+1]
		}
	}
	// Rich rewrites did not repay the cache break. Keep the full eligible
	// prefix, then index the oldest results one at a time until the same gate
	// admits it. Recent results never enter candidates because working-set
	// protection is applied before this function.
	for index := 0; index < limit; index++ {
		if candidates[index].indexed == "" || candidates[index].indexedTokens >= candidates[index].projectedTokens {
			continue
		}
		candidates[index].projected = candidates[index].indexed
		candidates[index].projectedTokens = candidates[index].indexedTokens
		probe = ProgressiveProjectionResult{}
		for candidateIndex := 0; candidateIndex < limit; candidateIndex++ {
			probe.OriginalTokens += candidates[candidateIndex].originalTokens
			probe.ProjectedTokens += candidates[candidateIndex].projectedTokens
		}
		probe.TokensSaved = probe.OriginalTokens - probe.ProjectedTokens
		if admission.RemainingProjectedTokens > 0 && probe.ProjectedTokens > admission.RemainingProjectedTokens {
			continue
		}
		decision := evaluateProgressiveProjectionAdmission(&probe, admission)
		if decision == ProgressiveDecisionAdmittedAvoidCompact || decision == ProgressiveDecisionAdmittedNetSavings {
			return candidates[:limit]
		}
	}
	return candidates[:limit]
}

func progressiveAdmissionAllowsTool(admission ProgressiveProjectionAdmission, toolName string) bool {
	if len(admission.AllowedTools) == 0 {
		return toolName == "Inspect"
	}
	_, ok := admission.AllowedTools[strings.ToLower(strings.TrimSpace(toolName))]
	return ok
}

func progressiveToolProjection(toolName string, result types.ToolResultBlock, original string) (string, string, bool) {
	switch toolName {
	case "Inspect":
		if !safeProgressiveInspectResult(result) {
			return "", "", false
		}
		proof, valid := agenticV2ProofContent(toolName, result)
		if !valid {
			return "", "", false
		}
		rewrite, valid := progressiveInspectRewriteContent(original, proof)
		if !valid {
			return "", "", false
		}
		index, _ := progressiveInspectIndexContent(original, proof)
		return rewrite, index, true
	case "Run":
		rewrite, valid := progressiveRunRewriteContent(result, original)
		return rewrite, "", valid
	case "ApplyPatch":
		if !safeProgressivePatchResult(result) {
			return "", "", false
		}
		rewrite, valid := agenticV2ProofContent(toolName, result)
		return rewrite, "", valid
	default:
		return "", "", false
	}
}

// ProgressiveToolResultSupportsAction reports whether the runtime can satisfy
// a shadow ContextUpdate proposal using its own deterministic policy. The
// model's rewrite text is never trusted as the replacement. KEEP is always a
// safe decision for a found target; DROP remains disabled until a separately
// reviewed receipt can prove that no recoverable information is lost.
func ProgressiveToolResultSupportsAction(toolName string, result types.ToolResultBlock, action string) bool {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "KEEP":
		return true
	case "REWRITE":
		original := result.TextContent()
		if original == "" || result.HasMediaContent() || isAlreadyPersistedOutput(original) {
			return false
		}
		rewrite, _, valid := progressiveToolProjection(toolName, result, original)
		return valid && len(rewrite) < len(original)
	case "INDEX":
		if toolName != "Inspect" || !safeProgressiveInspectResult(result) {
			return false
		}
		original := result.TextContent()
		proof, valid := agenticV2ProofContent(toolName, result)
		if !valid {
			return false
		}
		index, valid := progressiveInspectIndexContent(original, proof)
		return valid && len(index) < len(original)
	default:
		return false
	}
}

func filterProtectedProgressiveResults(candidates []progressiveInspectCandidate, protectedInspectResults int) []progressiveInspectCandidate {
	counts := make(map[string]int)
	for _, candidate := range candidates {
		counts[candidate.toolName]++
	}
	protect := map[string]int{
		"Inspect": max(protectedInspectResults, 0), "Run": progressiveProtectedRunResults, "ApplyPatch": progressiveProtectedPatchResults,
	}
	seen := make(map[string]int)
	out := make([]progressiveInspectCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		seen[candidate.toolName]++
		if seen[candidate.toolName] > counts[candidate.toolName]-protect[candidate.toolName] {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func evaluateProgressiveProjectionAdmission(result *ProgressiveProjectionResult, admission ProgressiveProjectionAdmission) string {
	if result != nil {
		result.RawRequestTokens = admission.RawRequestTokens
	}
	if result == nil || admission.Counter == nil || !admission.RawRequestEstimateKnown || !admission.PreviousUsageKnown || admission.RawRequestTokens <= 0 {
		return ProgressiveDecisionKeepIncomplete
	}
	if !admission.Pricing.Known || admission.Pricing.InputPerMtok <= 0 || admission.Pricing.CacheReadPerMtok < 0 {
		return ProgressiveDecisionKeepUnknownPricing
	}
	minTokenSavings := admission.MinTokenSavings
	if minTokenSavings <= 0 {
		minTokenSavings = 1
	}
	if result.TokensSaved < minTokenSavings {
		return ProgressiveDecisionKeepTokenSavings
	}

	result.ProjectedRequestTokens = max(admission.RawRequestTokens-result.TokensSaved, 0)
	cacheRead := min(max(admission.PreviousCacheReadTokens, 0), admission.RawRequestTokens)
	uncachedRaw := admission.RawRequestTokens - cacheRead
	baselineCost := tokenCostUSD(cacheRead, admission.Pricing.CacheReadPerMtok) +
		tokenCostUSD(uncachedRaw, admission.Pricing.InputPerMtok)
	// Rewriting an old result invalidates continuation and every cache block
	// after the first changed byte. Treat the whole projected request as cold;
	// this intentionally overstates the cache-break penalty.
	projectedColdCost := tokenCostUSD(result.ProjectedRequestTokens, admission.Pricing.InputPerMtok)
	cacheRecoveryRequests := max(admission.CacheRecoveryRequests, 1)
	result.GrossCacheBreakCostUSD = max(projectedColdCost-baselineCost, 0) * float64(cacheRecoveryRequests)
	result.CacheBreakCostUSD = result.GrossCacheBreakCostUSD
	currentSavings := max(baselineCost-projectedColdCost, 0)
	futureSavings := tokenCostUSD(result.TokensSaved*max(admission.ReuseHorizon, 0), admission.Pricing.CacheReadPerMtok)
	directNetSavings := currentSavings + futureSavings - result.CacheBreakCostUSD

	result.AvoidsImmediateCompaction = admission.AutoCompactThreshold > 0 &&
		admission.RawRequestTokens > admission.AutoCompactThreshold &&
		result.ProjectedRequestTokens <= admission.AutoCompactThreshold
	if result.AvoidsImmediateCompaction {
		if admission.ImminentCompactResetsCache {
			// Semantic compaction is the actual counterfactual and invalidates the
			// continuation too. Do not charge projection for a cache reset that
			// happens in both branches. No semantic-compaction or output-token
			// credit is added, so the comparison remains conservative.
			result.CacheBreakCostUSD = 0
			directNetSavings = currentSavings + futureSavings
		}
		// Delaying semantic compaction is not the same as permanently avoiding
		// it: the task may cross the threshold again. Assign no speculative
		// compaction credit and require the replacement itself to repay both
		// observed cache-recovery requests within the configured reuse horizon.
		result.AvoidedCompactInputCostUSD = 0
		result.EstimatedNetSavingsUSD = directNetSavings
		if result.EstimatedNetSavingsUSD >= max(admission.MinNetSavingsUSD, 0) {
			return ProgressiveDecisionAdmittedAvoidCompact
		}
	}
	// Never pay for a cache break immediately before semantic compaction. A
	// pressure batch above the hard threshold must avoid that compaction; an
	// ordinary multi-turn reuse estimate cannot make the double rewrite safe.
	if admission.AutoCompactThreshold > 0 && admission.RawRequestTokens > admission.AutoCompactThreshold {
		return ProgressiveDecisionKeepCompactThreshold
	}

	result.EstimatedNetSavingsUSD = directNetSavings
	if result.EstimatedNetSavingsUSD >= max(admission.MinNetSavingsUSD, 0) {
		return ProgressiveDecisionAdmittedNetSavings
	}
	return ProgressiveDecisionKeepCost
}

func tokenCostUSD(tokens int, perMillion float64) float64 {
	return float64(max(tokens, 0)) * perMillion / 1_000_000
}

func latestAssistantMessageIndex(messages []types.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if effectiveCompactionRole(messages[index]) == types.RoleAssistant {
			return index
		}
	}
	return -1
}

func latestConsumedMutation(messages []types.Message, toolNames map[string]string, latestAssistant int) (int, bool, bool) {
	for index := latestAssistant - 1; index >= 0; index-- {
		if effectiveCompactionRole(messages[index]) != types.RoleUser {
			continue
		}
		for _, block := range messages[index].Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok || toolNames[result.ToolUseID] != "ApplyPatch" {
				continue
			}
			return index, !result.IsError && result.Outcome == types.ToolOutcomeSucceeded, true
		}
	}
	return -1, false, false
}

func countFrozenProgressiveInspects(messages []types.Message, state *ContentReplacementState, toolNames map[string]string, before int) int {
	count := 0
	for messageIndex := 0; messageIndex < before; messageIndex++ {
		if effectiveCompactionRole(messages[messageIndex]) != types.RoleUser {
			continue
		}
		for _, block := range messages[messageIndex].Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok || toolNames[result.ToolUseID] != "Inspect" {
				continue
			}
			replacement, frozen := state.Replacements[result.ToolUseID]
			if frozen && strings.Contains(replacement, compactproof.SchemaVersion) {
				count++
			}
		}
	}
	return count
}

// ProgressiveProjectionBudgetUsage reconstructs the session budget from the
// exact frozen replacements. This survives resume because replacement records
// are persisted in private history and replayed into ContentReplacementState.
func ProgressiveProjectionBudgetUsage(state *ContentReplacementState, counter TokenCounter) (tools, projectedTokens int) {
	if state == nil {
		return 0, 0
	}
	for _, replacement := range state.Replacements {
		if !strings.Contains(replacement, compactproof.SchemaVersion) ||
			!strings.Contains(replacement, progressiveInspectRewriteSchema) && !strings.Contains(replacement, progressiveRunRewriteSchema) &&
				!strings.Contains(replacement, `"tool":"ApplyPatch"`) {
			continue
		}
		tools++
		if counter != nil {
			projectedTokens += max(counter.Count(replacement), 0)
		}
	}
	return tools, projectedTokens
}

func safeProgressivePatchResult(result types.ToolResultBlock) bool {
	if result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
		return false
	}
	provider, ok := result.Data.(compactproof.Provider)
	if !ok {
		return false
	}
	proof := provider.CompactionProof()
	return proof.Patch != nil && proof.Patch.CAS == "committed" && proof.Patch.FailureReason == ""
}

func safeProgressiveInspectResult(result types.ToolResultBlock) bool {
	if result.IsError || (result.Outcome != types.ToolOutcomeSucceeded && result.Outcome != types.ToolOutcomePartial) {
		return false
	}
	provider, ok := result.Data.(compactproof.Provider)
	if !ok {
		return false
	}
	proof := provider.CompactionProof()
	if proof.Inspect == nil || len(proof.Inspect.ErrorCodes) > 0 {
		return false
	}
	// Metadata provides a provider-independent fail-closed gate in addition to
	// the typed Inspect proof.
	for _, key := range []string{"error.code", "inspect.error_code", "schedule.reason"} {
		if result.Metadata[key] != "" {
			return false
		}
	}
	return true
}
