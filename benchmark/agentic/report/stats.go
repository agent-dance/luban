package report

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func finalizeExperiment(experiment *ExperimentData, meta ReportMeta, statistics StatisticsSpec) {
	byAgent := map[string][]RunData{}
	for _, run := range experiment.Runs {
		byAgent[run.AgentID] = append(byAgent[run.AgentID], run)
	}
	for _, agentID := range sortedMapKeys(byAgent) {
		runs := byAgent[agentID]
		slices.SortFunc(runs, compareRuns)
		agent := aggregateAgent(experiment.ID, agentID, runs, statistics)
		experiment.Agents = append(experiment.Agents, agent)
		experiment.ToolStats = append(experiment.ToolStats, agent.Tools...)
	}
	experiment.OrderStrata = aggregateOrderStrata(experiment.ID, experiment.Runs)
	if comparison := compareExperimentAgents(*experiment, meta.BaselineAgentID, meta.ContenderAgentID, statistics); comparison != nil {
		experiment.Comparisons = append(experiment.Comparisons, *comparison)
	}
	slices.SortFunc(experiment.Runs, compareRuns)
	slices.SortFunc(experiment.ProviderRounds, func(left, right RoundData) int {
		if comparison := strings.Compare(left.TaskID, right.TaskID); comparison != 0 {
			return comparison
		}
		if left.Repetition != right.Repetition {
			return left.Repetition - right.Repetition
		}
		if comparison := strings.Compare(left.AgentID, right.AgentID); comparison != 0 {
			return comparison
		}
		return left.Round - right.Round
	})
}

func aggregateOrderStrata(experimentID string, runs []RunData) []OrderStratumData {
	grouped := map[string][]RunData{}
	for _, run := range runs {
		if run.ExecutionPosition != "first" && run.ExecutionPosition != "second" {
			continue
		}
		grouped[run.AgentID+"\x00"+run.ExecutionPosition] = append(grouped[run.AgentID+"\x00"+run.ExecutionPosition], run)
	}
	result := make([]OrderStratumData, 0, len(grouped))
	for _, key := range sortedMapKeys(grouped) {
		stratumRuns := grouped[key]
		stratum := OrderStratumData{
			ExperimentID: experimentID, AgentID: stratumRuns[0].AgentID,
			Position: stratumRuns[0].ExecutionPosition, Raw: len(stratumRuns),
		}
		trialTotal, comparableCostTotal := 0.0, 0.0
		for _, run := range stratumRuns {
			switch run.Disposition {
			case string(harness.DeepSWEAttemptScored):
				stratum.Scored++
				if run.Passed != nil && *run.Passed {
					stratum.Passed++
				}
			case string(harness.DeepSWEAttemptExcluded):
				stratum.Excluded++
			}
			if run.Metrics.TrialDurationSeconds != nil {
				trialTotal += *run.Metrics.TrialDurationSeconds
				stratum.TrialObserved++
			}
			if run.Metrics.ComparableCostBasis == comparableCostBasisFrozen && run.Metrics.ComparableCost != nil {
				comparableCostTotal += *run.Metrics.ComparableCost
				stratum.ComparableCostObserved++
			}
		}
		if stratum.Scored > 0 {
			rate := float64(stratum.Passed) / float64(stratum.Scored)
			stratum.PassRate = &rate
		}
		if stratum.TrialObserved > 0 {
			mean := trialTotal / float64(stratum.TrialObserved)
			stratum.MeanTrialSeconds = &mean
		}
		if stratum.ComparableCostObserved > 0 {
			mean := comparableCostTotal / float64(stratum.ComparableCostObserved)
			stratum.MeanComparableCost = &mean
		}
		result = append(result, stratum)
	}
	return result
}

func aggregateAgent(experimentID, agentID string, runs []RunData, statistics StatisticsSpec) AgentData {
	agent := AgentData{ExperimentID: experimentID, AgentID: agentID, Runs: len(runs)}
	variants := map[string]struct{}{}
	tasks := map[string]struct{}{}
	passClusters := map[string][]float64{}
	allPassKnown := true
	passed := 0
	for _, run := range runs {
		variants[run.Variant] = struct{}{}
		tasks[run.TaskID] = struct{}{}
		if run.Passed == nil {
			allPassKnown = false
		} else if *run.Passed {
			passed++
			passClusters[run.TaskID] = append(passClusters[run.TaskID], 1)
		} else {
			passClusters[run.TaskID] = append(passClusters[run.TaskID], 0)
		}
	}
	agent.Tasks = len(tasks)
	variantNames := sortedMapKeys(variants)
	if len(variantNames) == 1 {
		agent.Variant = variantNames[0]
	} else {
		agent.Variant = "mixed"
	}
	if allPassKnown && len(runs) > 0 {
		agent.Passed = pointerInt(passed)
		rate := float64(passed) / float64(len(runs))
		agent.PassRate = &rate
		if len(passClusters) >= 2 {
			agent.PassCI = clusterBootstrap(passClusters, statistics, experimentID+"/"+agentID+"/pass")
		}
	}
	agent.Metrics = aggregateMetricData(runs)
	agent.Tools = aggregateTools(experimentID, agentID, runs)
	return agent
}

func aggregateMetricData(runs []RunData) MetricData {
	result := MetricData{
		WallTimeSeconds:                  sumFloatField(runs, func(metric MetricData) *float64 { return metric.WallTimeSeconds }),
		TrialDurationSeconds:             sumFloatField(runs, func(metric MetricData) *float64 { return metric.TrialDurationSeconds }),
		TransportAttempts:                sumIntField(runs, func(metric MetricData) *int { return metric.TransportAttempts }),
		PrewarmAttempts:                  sumIntField(runs, func(metric MetricData) *int { return metric.PrewarmAttempts }),
		PrewarmErrors:                    sumIntField(runs, func(metric MetricData) *int { return metric.PrewarmErrors }),
		LLMCallsStarted:                  sumIntField(runs, func(metric MetricData) *int { return metric.LLMCallsStarted }),
		CompletedLLMResponses:            sumIntField(runs, func(metric MetricData) *int { return metric.CompletedLLMResponses }),
		HTTPInferenceRequests:            sumIntField(runs, func(metric MetricData) *int { return metric.HTTPInferenceRequests }),
		WebSocketInferenceRequests:       sumIntField(runs, func(metric MetricData) *int { return metric.WebSocketInferenceRequests }),
		WebSocketConnections:             sumIntField(runs, func(metric MetricData) *int { return metric.WebSocketConnections }),
		PrewarmUsageObservations:         sumIntField(runs, func(metric MetricData) *int { return metric.PrewarmUsageObservations }),
		PrewarmInputTokens:               sumInt64Field(runs, func(metric MetricData) *int64 { return metric.PrewarmInputTokens }),
		PrewarmCachedInputTokens:         sumInt64Field(runs, func(metric MetricData) *int64 { return metric.PrewarmCachedInputTokens }),
		PrewarmOutputTokens:              sumInt64Field(runs, func(metric MetricData) *int64 { return metric.PrewarmOutputTokens }),
		PrewarmUnknownCostAttempts:       sumIntField(runs, func(metric MetricData) *int { return metric.PrewarmUnknownCostAttempts }),
		AllExecutedInputTokens:           sumInt64Field(runs, func(metric MetricData) *int64 { return metric.AllExecutedInputTokens }),
		AllExecutedCachedTokens:          sumInt64Field(runs, func(metric MetricData) *int64 { return metric.AllExecutedCachedTokens }),
		AllExecutedUncachedTokens:        sumInt64Field(runs, func(metric MetricData) *int64 { return metric.AllExecutedUncachedTokens }),
		AllExecutedNonCachedBaseTokens:   sumInt64Field(runs, func(metric MetricData) *int64 { return metric.AllExecutedNonCachedBaseTokens }),
		AllExecutedOutputTokens:          sumInt64Field(runs, func(metric MetricData) *int64 { return metric.AllExecutedOutputTokens }),
		AllExecutedCacheWriteInputTokens: sumInt64Field(runs, func(metric MetricData) *int64 { return metric.AllExecutedCacheWriteInputTokens }),
		CachePolicyObservedRequests:      sumIntField(runs, func(metric MetricData) *int { return metric.CachePolicyObservedRequests }),
		CacheKeyPresentRequests:          sumIntField(runs, func(metric MetricData) *int { return metric.CacheKeyPresentRequests }),
		CacheUniqueKeyCount:              sumIntField(runs, func(metric MetricData) *int { return metric.CacheUniqueKeyCount }),
		CacheKeyTransitions:              sumIntField(runs, func(metric MetricData) *int { return metric.CacheKeyTransitions }),
		ProviderRequests:                 sumIntField(runs, func(metric MetricData) *int { return metric.ProviderRequests }),
		ProviderRounds:                   sumIntField(runs, func(metric MetricData) *int { return metric.ProviderRounds }),
		ProviderErrors:                   sumIntField(runs, func(metric MetricData) *int { return metric.ProviderErrors }),
		ToolBearingRounds:                sumIntField(runs, func(metric MetricData) *int { return metric.ToolBearingRounds }),
		ToolInvocations:                  sumIntField(runs, func(metric MetricData) *int { return metric.ToolInvocations }),
		ToolTraceMatched:                 sumIntField(runs, func(metric MetricData) *int { return metric.ToolTraceMatched }),
		ToolTraceUnmatched:               sumIntField(runs, func(metric MetricData) *int { return metric.ToolTraceUnmatched }),
		PhysicalToolOperations:           sumIntField(runs, func(metric MetricData) *int { return metric.PhysicalToolOperations }),
		NativeEvents:                     sumIntField(runs, func(metric MetricData) *int { return metric.NativeEvents }),
		ToolErrors:                       sumIntField(runs, func(metric MetricData) *int { return metric.ToolErrors }),
		ToolCriticalPathMS:               sumInt64Field(runs, func(metric MetricData) *int64 { return metric.ToolCriticalPathMS }),
		ToolTotalLatencyMS:               sumInt64Field(runs, func(metric MetricData) *int64 { return metric.ToolTotalLatencyMS }),
		ToolQueueMS:                      sumInt64Field(runs, func(metric MetricData) *int64 { return metric.ToolQueueMS }),
		InputTokens:                      sumInt64Field(runs, func(metric MetricData) *int64 { return metric.InputTokens }),
		CachedInputTokens:                sumInt64Field(runs, func(metric MetricData) *int64 { return metric.CachedInputTokens }),
		CacheWriteInputTokens:            sumInt64Field(runs, func(metric MetricData) *int64 { return metric.CacheWriteInputTokens }),
		CacheMissTokens:                  sumInt64Field(runs, func(metric MetricData) *int64 { return metric.CacheMissTokens }),
		OutputTokens:                     sumInt64Field(runs, func(metric MetricData) *int64 { return metric.OutputTokens }),
		ReasoningOutputTokens:            sumInt64Field(runs, func(metric MetricData) *int64 { return metric.ReasoningOutputTokens }),
		CatalogCost:                      sumFloatField(runs, func(metric MetricData) *float64 { return metric.CatalogCost }),
		ComparableCost:                   sumFloatField(runs, func(metric MetricData) *float64 { return metric.ComparableCost }),
	}
	if len(runs) > 0 {
		result.ComparableCostBasis = runs[0].Metrics.ComparableCostBasis
		for _, run := range runs[1:] {
			if run.Metrics.ComparableCostBasis != result.ComparableCostBasis {
				result.ComparableCostBasis = "mixed_invalid"
				result.ComparableCost = nil
				break
			}
		}
	}
	if result.InputTokens != nil && result.CachedInputTokens != nil && *result.InputTokens > 0 {
		rate := float64(*result.CachedInputTokens) / float64(*result.InputTokens)
		result.TokenWeightedCacheHit = &rate
	}
	requestHits, requestObserved, requestComplete := 0, 0, true
	cacheLineageKnown, cacheLineageStable := true, true
	providerCost, providerPartial := 0.0, false
	catalogObservedCost, knownCatalogLowerBound := 0.0, 0.0
	knownCatalogLowerBoundObserved := false
	unknownCostAttempts, unknownCostKnown := 0, true
	costIdentityUnknownAttempts, costIdentityUnknownKnown := 0, true
	allExecutedInput, allExecutedCached := int64(0), int64(0)
	allExecutedCacheObserved := false
	for _, run := range runs {
		if run.Metrics.RequestCacheHits == nil || run.Metrics.RequestCacheObserved == nil {
			requestComplete = false
		} else {
			requestHits += *run.Metrics.RequestCacheHits
			requestObserved += *run.Metrics.RequestCacheObserved
		}
		if run.Metrics.CacheLineageStable == nil {
			cacheLineageKnown = false
		} else {
			cacheLineageStable = cacheLineageStable && *run.Metrics.CacheLineageStable
		}
		result.ProviderCostObserved += run.Metrics.ProviderCostObserved
		result.ProviderCostTotal += run.Metrics.ProviderCostTotal
		result.PhysicalToolObserved += run.Metrics.PhysicalToolObserved
		result.PhysicalToolTotal += run.Metrics.PhysicalToolTotal
		result.ToolCriticalObserved += run.Metrics.ToolCriticalObserved
		result.ToolCriticalTotal += run.Metrics.ToolCriticalTotal
		result.ToolTotalObserved += run.Metrics.ToolTotalObserved
		result.ToolTotalTotal += run.Metrics.ToolTotalTotal
		result.ToolQueueObserved += run.Metrics.ToolQueueObserved
		result.ToolQueueTotal += run.Metrics.ToolQueueTotal
		if run.Metrics.ProviderReportedCost != nil {
			providerCost += *run.Metrics.ProviderReportedCost
		} else if run.Metrics.ProviderCostPartial != nil {
			providerCost += *run.Metrics.ProviderCostPartial
			providerPartial = true
		} else if run.Metrics.ProviderCostObserved < run.Metrics.ProviderCostTotal {
			providerPartial = true
		}
		result.ToolTimingObserved += run.Metrics.ToolTimingObserved
		result.ToolTimingTotal += run.Metrics.ToolTimingTotal
		result.ToolErrorObserved += run.Metrics.ToolErrorObserved
		result.ToolErrorTotal += run.Metrics.ToolErrorTotal
		result.TokenUsageObserved += run.Metrics.TokenUsageObserved
		result.TokenUsageTotal += run.Metrics.TokenUsageTotal
		result.ReasoningTokenObserved += run.Metrics.ReasoningTokenObserved
		result.ReasoningTokenTotal += run.Metrics.ReasoningTokenTotal
		result.CacheWriteTokenObserved += run.Metrics.CacheWriteTokenObserved
		result.CacheWriteTokenTotal += run.Metrics.CacheWriteTokenTotal
		result.UnreportedCacheWriteRounds += run.Metrics.UnreportedCacheWriteRounds
		result.CostReceiptObserved += run.Metrics.CostReceiptObserved
		result.CostReceiptTotal += run.Metrics.CostReceiptTotal
		result.AllExecutedUsageObserved += run.Metrics.AllExecutedUsageObserved
		result.AllExecutedUsageTotal += run.Metrics.AllExecutedUsageTotal
		result.AllExecutedCacheWriteObserved += run.Metrics.AllExecutedCacheWriteObserved
		result.AllExecutedCacheWriteTotal += run.Metrics.AllExecutedCacheWriteTotal
		result.AllExecutedUnreportedCacheWrite += run.Metrics.AllExecutedUnreportedCacheWrite
		if run.Metrics.UnknownCostAttempts == nil {
			unknownCostKnown = false
		} else {
			unknownCostAttempts += *run.Metrics.UnknownCostAttempts
		}
		if run.Metrics.CostIdentityUnknownAttempts == nil {
			costIdentityUnknownKnown = false
		} else {
			costIdentityUnknownAttempts += *run.Metrics.CostIdentityUnknownAttempts
		}
		if run.Metrics.KnownCatalogCostLowerBound != nil {
			knownCatalogLowerBound += *run.Metrics.KnownCatalogCostLowerBound
			knownCatalogLowerBoundObserved = true
		}
		if run.Metrics.AllExecutedInputTokens != nil && run.Metrics.AllExecutedCachedTokens != nil {
			allExecutedInput += *run.Metrics.AllExecutedInputTokens
			allExecutedCached += *run.Metrics.AllExecutedCachedTokens
			allExecutedCacheObserved = true
		}
		if run.Metrics.KnownCacheWriteSurcharge != nil {
			if result.KnownCacheWriteSurcharge == nil {
				result.KnownCacheWriteSurcharge = pointerFloat(0)
			}
			*result.KnownCacheWriteSurcharge += *run.Metrics.KnownCacheWriteSurcharge
		}
		if run.Metrics.CatalogCost != nil {
			catalogObservedCost += *run.Metrics.CatalogCost
		} else if run.Metrics.CatalogCostPartial != nil {
			catalogObservedCost += *run.Metrics.CatalogCostPartial
		}
	}
	if unknownCostKnown {
		result.UnknownCostAttempts = pointerInt(unknownCostAttempts)
	}
	if costIdentityUnknownKnown {
		result.CostIdentityUnknownAttempts = pointerInt(costIdentityUnknownAttempts)
	}
	if knownCatalogLowerBoundObserved {
		result.KnownCatalogCostLowerBound = pointerFloat(knownCatalogLowerBound)
	}
	if allExecutedCacheObserved && allExecutedInput > 0 && result.AllExecutedUsageObserved == result.AllExecutedUsageTotal {
		rate := float64(allExecutedCached) / float64(allExecutedInput)
		result.AllExecutedCacheHit = &rate
	}
	if requestComplete && requestObserved > 0 {
		result.RequestCacheHits, result.RequestCacheObserved = pointerInt(requestHits), pointerInt(requestObserved)
		rate := float64(requestHits) / float64(requestObserved)
		result.RequestCacheHit = &rate
	}
	if cacheLineageKnown {
		result.CacheLineageStable = cloneBool(&cacheLineageStable)
	}
	if result.ProviderCostTotal > 0 && result.ProviderCostObserved == result.ProviderCostTotal && !providerPartial {
		result.ProviderReportedCost = pointerFloat(providerCost)
	} else if result.ProviderCostObserved > 0 {
		result.ProviderCostPartial = pointerFloat(providerCost)
	}
	if result.CatalogCost == nil && result.TokenUsageObserved > 0 {
		result.CatalogCostPartial = pointerFloat(catalogObservedCost)
	}
	return result
}

func aggregateTools(experimentID, agentID string, runs []RunData) []ToolData {
	byName := map[string][]ToolData{}
	for _, run := range runs {
		for _, tool := range run.Tools {
			byName[tool.Name] = append(byName[tool.Name], tool)
		}
	}
	result := make([]ToolData, 0, len(byName))
	for _, name := range sortedMapKeys(byName) {
		values := byName[name]
		tool := ToolData{ExperimentID: experimentID, AgentID: agentID, Name: name}
		callsKnown, errorsKnown, durationKnown := true, true, true
		calls, errorsCount, duration := 0, 0, int64(0)
		for _, value := range values {
			if value.Calls == nil {
				callsKnown = false
			} else {
				calls += *value.Calls
			}
			if value.Errors == nil {
				errorsKnown = false
			} else {
				errorsCount += *value.Errors
			}
			if value.DurationMS == nil {
				durationKnown = false
			} else {
				duration += *value.DurationMS
			}
			tool.TimingKnown += value.TimingKnown
			tool.TimingTotal += value.TimingTotal
			tool.ErrorKnown += value.ErrorKnown
			tool.ErrorTotal += value.ErrorTotal
		}
		if callsKnown {
			tool.Calls = pointerInt(calls)
		}
		if errorsKnown && tool.ErrorKnown == tool.ErrorTotal {
			tool.Errors = pointerInt(errorsCount)
		}
		if durationKnown && tool.TimingKnown == tool.TimingTotal {
			tool.DurationMS = pointerInt64(duration)
		}
		result = append(result, tool)
	}
	return result
}

func compareExperimentAgents(experiment ExperimentData, baselineID, contenderID string, statistics StatisticsSpec) *PairedComparison {
	baseline, contender := runsForAgent(experiment.Runs, baselineID), runsForAgent(experiment.Runs, contenderID)
	if len(baseline) == 0 || len(contender) == 0 {
		return nil
	}
	comparison := compareRunSets(experiment.ID, baselineID, contenderID, baseline, contender, allComparisonMetrics(), statistics)
	return &comparison
}

func compareRunSets(experimentID, baselineID, contenderID string, baseline, contender []RunData, metrics []ComparisonMetric, statistics StatisticsSpec) PairedComparison {
	baselineByKey := map[string]RunData{}
	for _, run := range baseline {
		baselineByKey[taskRepetitionKey(run)] = run
	}
	var paired [][2]RunData
	for _, run := range contender {
		if match, exists := baselineByKey[taskRepetitionKey(run)]; exists {
			paired = append(paired, [2]RunData{match, run})
		}
	}
	slices.SortFunc(paired, func(left, right [2]RunData) int { return compareRuns(left[0], right[0]) })
	tasks := map[string]struct{}{}
	comparison := PairedComparison{ExperimentID: experimentID, Baseline: baselineID, Contender: contenderID, Pairs: len(paired)}
	for _, pair := range paired {
		tasks[pair[0].TaskID] = struct{}{}
		if pair[0].Passed == nil || pair[1].Passed == nil {
			continue
		}
		switch {
		case *pair[1].Passed && !*pair[0].Passed:
			comparison.QualityWins++
		case !*pair[1].Passed && *pair[0].Passed:
			comparison.QualityLosses++
		default:
			comparison.QualityTies++
		}
	}
	comparison.Tasks = len(tasks)
	for _, metric := range metrics {
		comparison.Metrics = append(comparison.Metrics, compareMetric(paired, metric, statistics, experimentID+"/"+string(metric)))
	}
	return comparison
}

func compareMetric(pairs [][2]RunData, metric ComparisonMetric, statistics StatisticsSpec, salt string) MetricComparison {
	if metric == MetricTokenCacheHit || metric == MetricRequestCacheHit {
		return compareRatioMetric(pairs, metric, statistics, salt)
	}
	result := MetricComparison{Metric: metric}
	differences := map[string][]float64{}
	baselineSum, contenderSum := 0.0, 0.0
	for _, pair := range pairs {
		baseline, baselineOK := runMetric(pair[0], metric)
		contender, contenderOK := runMetric(pair[1], metric)
		if !baselineOK || !contenderOK {
			continue
		}
		baselineSum += baseline
		contenderSum += contender
		differences[pair[0].TaskID] = append(differences[pair[0].TaskID], contender-baseline)
		result.Pairs++
	}
	result.Tasks = len(differences)
	if result.Pairs == 0 {
		result.Note = "not_reported"
		return result
	}
	baselineMean := baselineSum / float64(result.Pairs)
	contenderMean := contenderSum / float64(result.Pairs)
	difference := contenderMean - baselineMean
	result.Baseline, result.Contender, result.Difference = &baselineMean, &contenderMean, &difference
	if baselineSum != 0 {
		relative := (contenderSum - baselineSum) / baselineSum
		result.RelativeChange = &relative
	}
	if result.Tasks >= 2 {
		result.CI = clusterBootstrap(differences, statistics, salt)
	}
	if result.Pairs < len(pairs) {
		result.Note = fmt.Sprintf("coverage=%d/%d", result.Pairs, len(pairs))
	}
	return result
}

type pairedRatioObservation struct {
	baselineNumerator    float64
	baselineDenominator  float64
	contenderNumerator   float64
	contenderDenominator float64
}

func compareRatioMetric(pairs [][2]RunData, metric ComparisonMetric, statistics StatisticsSpec, salt string) MetricComparison {
	result := MetricComparison{Metric: metric}
	clusters := map[string][]pairedRatioObservation{}
	baselineNumerator, baselineDenominator := 0.0, 0.0
	contenderNumerator, contenderDenominator := 0.0, 0.0
	for _, pair := range pairs {
		baselineNum, baselineDen, baselineOK := runRatioComponents(pair[0], metric)
		contenderNum, contenderDen, contenderOK := runRatioComponents(pair[1], metric)
		if !baselineOK || !contenderOK {
			continue
		}
		observation := pairedRatioObservation{
			baselineNumerator: baselineNum, baselineDenominator: baselineDen,
			contenderNumerator: contenderNum, contenderDenominator: contenderDen,
		}
		clusters[pair[0].TaskID] = append(clusters[pair[0].TaskID], observation)
		baselineNumerator += baselineNum
		baselineDenominator += baselineDen
		contenderNumerator += contenderNum
		contenderDenominator += contenderDen
		result.Pairs++
	}
	result.Tasks = len(clusters)
	if result.Pairs == 0 || baselineDenominator <= 0 || contenderDenominator <= 0 {
		result.Note = "not_reported"
		return result
	}
	baseline := baselineNumerator / baselineDenominator
	contender := contenderNumerator / contenderDenominator
	difference := contender - baseline
	result.Baseline, result.Contender, result.Difference = &baseline, &contender, &difference
	if baseline != 0 {
		relative := difference / baseline
		result.RelativeChange = &relative
	}
	if result.Tasks >= 2 {
		result.CI = clusterBootstrapRatio(clusters, statistics, salt)
	}
	if result.Pairs < len(pairs) {
		result.Note = fmt.Sprintf("coverage=%d/%d", result.Pairs, len(pairs))
	}
	return result
}

func runRatioComponents(run RunData, metric ComparisonMetric) (float64, float64, bool) {
	switch metric {
	case MetricTokenCacheHit:
		if run.Metrics.TokenWeightedCacheHit == nil || run.Metrics.CachedInputTokens == nil || run.Metrics.InputTokens == nil || *run.Metrics.InputTokens <= 0 {
			return 0, 0, false
		}
		return float64(*run.Metrics.CachedInputTokens), float64(*run.Metrics.InputTokens), true
	case MetricRequestCacheHit:
		if run.Metrics.RequestCacheHit == nil || run.Metrics.RequestCacheHits == nil || run.Metrics.RequestCacheObserved == nil || *run.Metrics.RequestCacheObserved <= 0 {
			return 0, 0, false
		}
		return float64(*run.Metrics.RequestCacheHits), float64(*run.Metrics.RequestCacheObserved), true
	default:
		return 0, 0, false
	}
}

func clusterBootstrapRatio(clusters map[string][]pairedRatioObservation, statistics StatisticsSpec, salt string) *ConfidenceInterval {
	keys := sortedMapKeys(clusters)
	if len(keys) < 2 {
		return nil
	}
	estimate := ratioDifference(keys, clusters)
	rng := newDeterministicRNG(statistics.Seed, salt)
	values := make([]float64, statistics.Resamples)
	sampled := make([]string, len(keys))
	for iteration := 0; iteration < statistics.Resamples; iteration++ {
		for index := range sampled {
			sampled[index] = keys[rng.intn(len(keys))]
		}
		values[iteration] = ratioDifference(sampled, clusters)
	}
	slices.Sort(values)
	alpha := (1 - statistics.ConfidenceLevel) / 2
	pairs := 0
	for _, key := range keys {
		pairs += len(clusters[key])
	}
	return &ConfidenceInterval{
		Estimate: estimate, Lower: percentile(values, alpha), Upper: percentile(values, 1-alpha),
		ConfidenceLevel: statistics.ConfidenceLevel, Method: statistics.Method,
		Tasks: len(keys), Pairs: pairs, Resamples: statistics.Resamples, Seed: statistics.Seed,
	}
}

func ratioDifference(keys []string, clusters map[string][]pairedRatioObservation) float64 {
	baselineNumerator, baselineDenominator := 0.0, 0.0
	contenderNumerator, contenderDenominator := 0.0, 0.0
	for _, key := range keys {
		for _, observation := range clusters[key] {
			baselineNumerator += observation.baselineNumerator
			baselineDenominator += observation.baselineDenominator
			contenderNumerator += observation.contenderNumerator
			contenderDenominator += observation.contenderDenominator
		}
	}
	if baselineDenominator <= 0 || contenderDenominator <= 0 {
		return math.NaN()
	}
	return contenderNumerator/contenderDenominator - baselineNumerator/baselineDenominator
}

func buildOptimizations(experiments []ExperimentData, ledger OptimizationLedger, statistics StatisticsSpec) ([]OptimizationData, error) {
	byID := map[string]*ExperimentData{}
	for index := range experiments {
		byID[experiments[index].ID] = &experiments[index]
	}
	var result []OptimizationData
	for _, entry := range ledger.Entries {
		beforeExperiment, beforeAgent, err := resolveEndpoint(byID, entry.Before)
		if err != nil {
			return nil, fmt.Errorf("optimization %s before endpoint: %w", entry.ID, err)
		}
		afterExperiment, afterAgent, err := resolveEndpoint(byID, entry.After)
		if err != nil {
			return nil, fmt.Errorf("optimization %s after endpoint: %w", entry.ID, err)
		}
		weakest := beforeExperiment.Class
		if classRank(afterExperiment.Class) < classRank(weakest) {
			weakest = afterExperiment.Class
		}
		optimization := OptimizationData{Entry: entry, Before: beforeAgent, After: afterAgent}
		beforeRuns := runsForAgent(beforeExperiment.Runs, entry.Before.AgentID)
		afterRuns := runsForAgent(afterExperiment.Runs, entry.After.AgentID)
		if err := validateOptimizationEndpoints(beforeExperiment, afterExperiment, beforeRuns, afterRuns); err != nil {
			return nil, fmt.Errorf("optimization %s endpoints are incompatible: %w", entry.ID, err)
		}
		if err := validateOptimizationImplementationBinding(entry, afterExperiment); err != nil {
			return nil, fmt.Errorf("optimization %s implementation binding: %w", entry.ID, err)
		}
		if entry.Ablation.Status == AblationMeasured {
			ablationExperiment, ablationAgent, resolveErr := resolveEndpoint(byID, *entry.Ablation.Endpoint)
			if resolveErr != nil {
				return nil, fmt.Errorf("optimization %s ablation endpoint: %w", entry.ID, resolveErr)
			}
			optimization.Ablation = ablationAgent
			if classRank(ablationExperiment.Class) < classRank(weakest) {
				weakest = ablationExperiment.Class
			}
			ablationRuns := runsForAgent(ablationExperiment.Runs, entry.Ablation.Endpoint.AgentID)
			if compatibilityErr := validateOptimizationEndpoints(ablationExperiment, afterExperiment, ablationRuns, afterRuns); compatibilityErr != nil {
				return nil, fmt.Errorf("optimization %s ablation endpoint is incompatible: %w", entry.ID, compatibilityErr)
			}
			if bindingErr := validateMeasuredAblationBuildIdentity(afterExperiment, entry.After.AgentID, ablationExperiment, entry.Ablation.Endpoint.AgentID); bindingErr != nil {
				return nil, fmt.Errorf("optimization %s measured ablation build identity: %w", entry.ID, bindingErr)
			}
			ablationComparison := compareRunSets(entry.ID+"/ablation", entry.Ablation.Endpoint.AgentID, entry.After.AgentID,
				ablationRuns, afterRuns, entry.Metrics, statistics)
			if err := requireCompleteMetricCoverage(ablationComparison, len(afterRuns)); err != nil {
				return nil, fmt.Errorf("optimization %s ablation coverage: %w", entry.ID, err)
			}
			optimization.AblationMetrics = ablationComparison.Metrics
		}
		if entry.EvidenceClass != weakest {
			return nil, fmt.Errorf("optimization %s evidence_class %s overstates endpoint evidence %s", entry.ID, entry.EvidenceClass, weakest)
		}
		optimization.ClassValid = true
		comparison := compareRunSets(entry.ID, entry.Before.AgentID, entry.After.AgentID, beforeRuns, afterRuns, entry.Metrics, statistics)
		if err := requireCompleteMetricCoverage(comparison, len(afterRuns)); err != nil {
			return nil, fmt.Errorf("optimization %s metric coverage: %w", entry.ID, err)
		}
		optimization.Metrics = comparison.Metrics
		result = append(result, optimization)
	}
	return result, nil
}

func validateOptimizationImplementationBinding(entry OptimizationEntry, afterExperiment *ExperimentData) error {
	if afterExperiment.Manifest == nil {
		if entry.EvidenceGrade != EvidenceDiagnosticBundle || !slices.Contains(entry.Confounders, "source_archive_unavailable") {
			return errors.New("unarchived diagnostic implementation requires diagnostic_bundle and source_archive_unavailable confounder")
		}
		return nil
	}
	afterAgent, exists := manifestAgentData(afterExperiment.Manifest.Agents, entry.After.AgentID)
	if !exists || afterAgent.SourceArchiveSHA == "" || len(afterAgent.ArchivedSourceFiles) == 0 {
		return errors.New("after endpoint does not expose a source-built archived implementation")
	}
	for _, implementation := range entry.Implementation {
		path := filepath.ToSlash(filepath.Clean(filepath.FromSlash(implementation.Path)))
		archivedSHA, exists := afterAgent.ArchivedSourceFiles[path]
		if !exists {
			return fmt.Errorf("path %s is absent from after endpoint source.tar", implementation.Path)
		}
		if archivedSHA != implementation.SHA256 {
			return fmt.Errorf("path %s SHA-256 differs from after endpoint source.tar", implementation.Path)
		}
	}
	return nil
}

func validateMeasuredAblationBuildIdentity(afterExperiment *ExperimentData, afterAgentID string, ablationExperiment *ExperimentData, ablationAgentID string) error {
	if afterExperiment.Manifest == nil || ablationExperiment.Manifest == nil {
		return errors.New("measured ablation requires archived harness manifests")
	}
	afterAgent, afterOK := manifestAgentData(afterExperiment.Manifest.Agents, afterAgentID)
	ablationAgent, ablationOK := manifestAgentData(ablationExperiment.Manifest.Agents, ablationAgentID)
	if !afterOK || !ablationOK || afterAgent.SourceArchiveSHA == "" || ablationAgent.SourceArchiveSHA == "" {
		return errors.New("measured ablation requires source-built endpoint identities")
	}
	if afterAgent.BinarySHA256 != ablationAgent.BinarySHA256 || afterAgent.SourceArchiveSHA != ablationAgent.SourceArchiveSHA || afterAgent.BuildReceiptSHA != ablationAgent.BuildReceiptSHA {
		return errors.New("feature-off and feature-on endpoints are not the same frozen build")
	}
	return nil
}

func manifestAgentData(agents []ManifestAgentData, id string) (ManifestAgentData, bool) {
	for _, agent := range agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return ManifestAgentData{}, false
}

func validateOptimizationEndpoints(beforeExperiment, afterExperiment *ExperimentData, beforeRuns, afterRuns []RunData) error {
	if len(beforeRuns) == 0 || len(beforeRuns) != len(afterRuns) {
		return errors.New("run counts differ")
	}
	beforeByKey := make(map[string]RunData, len(beforeRuns))
	for _, run := range beforeRuns {
		key := taskRepetitionKey(run)
		if _, duplicate := beforeByKey[key]; duplicate {
			return errors.New("before endpoint contains duplicate task/repetition keys")
		}
		beforeByKey[key] = run
	}
	for _, run := range afterRuns {
		before, exists := beforeByKey[taskRepetitionKey(run)]
		if !exists {
			return errors.New("task/repetition coverage differs")
		}
		if before.Provider != run.Provider || before.Model != run.Model || before.ReasoningEffort != run.ReasoningEffort {
			return errors.New("provider/model/reasoning contracts differ")
		}
	}
	if (beforeExperiment.Manifest == nil) != (afterExperiment.Manifest == nil) {
		return errors.New("evidence contracts use different provenance classes")
	}
	if beforeExperiment.Manifest != nil && !reflect.DeepEqual(optimizationManifestContract(*beforeExperiment.Manifest), optimizationManifestContract(*afterExperiment.Manifest)) {
		return errors.New("dataset, evaluator, environment, schedule, resources, or pricing differs")
	}
	return nil
}

func optimizationManifestContract(manifest ManifestData) any {
	return struct {
		DatasetName, DatasetRepository, DatasetCommit, DatasetTreeSHA string
		EvaluatorName, EvaluatorCommit, EvaluatorVersion              string
		EvaluatorProtocol                                             string
		SelectionMode                                                 string
		ExpectedTasks, Repetitions                                    int
		PairingSeed                                                   uint64
		MaxParallelPairs                                              int
		TaskNetwork, VerifierNetwork                                  string
		HostEnvAllowlist, AgentEgressHosts                            []string
		CPUs, MemoryMB, StorageMB, AgentTimeout, VerifierTimeout      int
		PricingCurrency                                               string
		PricingUnit                                                   int64
		PricingAt                                                     string
		PricingSource                                                 string
		PricingRates                                                  any
	}{
		manifest.DatasetName, manifest.DatasetRepository, manifest.DatasetCommit, manifest.DatasetTreeSHA,
		manifest.EvaluatorName, manifest.EvaluatorCommit, manifest.EvaluatorVersion, manifest.EvaluatorProtocol,
		manifest.SelectionMode, manifest.ExpectedTasks, manifest.Repetitions, manifest.PairingSeed, manifest.MaxParallelPairs,
		manifest.TaskNetwork, manifest.VerifierNetwork, slices.Clone(manifest.HostEnvAllowlist), slices.Clone(manifest.AgentEgressHosts),
		manifest.CPUs, manifest.MemoryMB, manifest.StorageMB, manifest.AgentTimeout, manifest.VerifierTimeout,
		manifest.PricingCurrency, manifest.PricingUnit, manifest.PricingAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), manifest.PricingSource,
		slices.Clone(manifest.PricingRates),
	}
}

func requireCompleteMetricCoverage(comparison PairedComparison, expectedPairs int) error {
	if comparison.Pairs != expectedPairs {
		return fmt.Errorf("paired coverage=%d/%d", comparison.Pairs, expectedPairs)
	}
	for _, metric := range comparison.Metrics {
		if metric.Pairs != expectedPairs || metric.Tasks != comparison.Tasks {
			return fmt.Errorf("metric %s coverage=%d/%d", metric.Metric, metric.Pairs, expectedPairs)
		}
	}
	return nil
}

func resolveEndpoint(experiments map[string]*ExperimentData, endpoint ExperimentEndpoint) (*ExperimentData, *AgentData, error) {
	experiment := experiments[endpoint.ExperimentID]
	if experiment == nil {
		return nil, nil, errors.New("experiment is absent")
	}
	for index := range experiment.Agents {
		if experiment.Agents[index].AgentID == endpoint.AgentID {
			return experiment, &experiment.Agents[index], nil
		}
	}
	return nil, nil, errors.New("agent is absent")
}

func attachFailureAnnotations(data *Data, annotations []FailureAnnotation) error {
	annotationByKey := map[string]FailureData{}
	for _, annotation := range annotations {
		key := failureKey(annotation.ExperimentID, annotation.TaskID, annotation.AgentID, annotation.Repetition)
		if _, exists := annotationByKey[key]; exists {
			return fmt.Errorf("duplicate failure annotation for %s", key)
		}
		annotationByKey[key] = FailureData{
			ExperimentID: annotation.ExperimentID, TaskID: annotation.TaskID, AgentID: annotation.AgentID,
			Repetition: annotation.Repetition, Category: annotation.Category,
			Summary: annotation.Summary, Evidence: slices.Clone(annotation.Evidence),
		}
	}
	used := map[string]struct{}{}
	for experimentIndex := range data.Experiments {
		experiment := &data.Experiments[experimentIndex]
		for runIndex := range experiment.Runs {
			run := &experiment.Runs[runIndex]
			if run.Passed == nil || *run.Passed {
				continue
			}
			key := failureKey(experiment.ID, run.TaskID, run.AgentID, run.Repetition)
			annotation, exists := annotationByKey[key]
			if !exists {
				return fmt.Errorf("failed run %s lacks a failure annotation", key)
			}
			run.Failure = &annotation
			data.FailureSummary = append(data.FailureSummary, annotation)
			used[key] = struct{}{}
		}
	}
	for key := range annotationByKey {
		if _, exists := used[key]; !exists {
			return fmt.Errorf("failure annotation %s does not identify a failed run", key)
		}
	}
	slices.SortFunc(data.FailureSummary, func(left, right FailureData) int {
		return strings.Compare(failureKey(left.ExperimentID, left.TaskID, left.AgentID, left.Repetition), failureKey(right.ExperimentID, right.TaskID, right.AgentID, right.Repetition))
	})
	return nil
}

func buildVerdict(experiments []ExperimentData) VerdictData {
	verdict := VerdictData{Status: "insufficient"}
	var comparison *PairedComparison
	formalGatesPassed := false
	for index := range experiments {
		if experiments[index].Class == ClassFormal && len(experiments[index].Comparisons) > 0 {
			comparison = &experiments[index].Comparisons[0]
			formalGatesPassed = allGatesPass(experiments[index].Gates)
			break
		}
	}
	criteria := []struct {
		metric ComparisonMetric
		higher bool
	}{
		{MetricPassRate, true}, {MetricWallTime, false}, {MetricTrialDuration, false},
		{MetricLLMCallsStarted, false}, {MetricTokenCacheHit, true}, {MetricComparableCost, false},
	}
	allKnown, allPassed := comparison != nil && formalGatesPassed, comparison != nil && formalGatesPassed
	for _, criterion := range criteria {
		row := VerdictCriterion{Metric: criterion.metric}
		var metric *MetricComparison
		if comparison != nil {
			for index := range comparison.Metrics {
				if comparison.Metrics[index].Metric == criterion.metric {
					metric = &comparison.Metrics[index]
					break
				}
			}
		}
		if !formalGatesPassed {
			allKnown = false
			row.Detail = "formal_gate_failed"
		} else if metric == nil || metric.Baseline == nil || metric.Contender == nil || metric.Difference == nil || metric.CI == nil ||
			metric.Pairs != comparison.Pairs || metric.Tasks != comparison.Tasks {
			allKnown = false
			row.Detail = "incomplete_paired_coverage"
		} else {
			passed := *metric.Contender > *metric.Baseline && metric.CI.Lower > 0
			if !criterion.higher {
				passed = *metric.Contender < *metric.Baseline && metric.CI.Upper < 0
			}
			row.Passed = &passed
			row.Detail = fmt.Sprintf("baseline=%g;contender=%g;difference_ci=[%g,%g];pairs=%d", *metric.Baseline, *metric.Contender, metric.CI.Lower, metric.CI.Upper, metric.Pairs)
			allPassed = allPassed && passed
		}
		verdict.Criteria = append(verdict.Criteria, row)
	}
	if allKnown && allPassed {
		verdict.Status = "verified_exceeds"
	} else if allKnown {
		verdict.Status = "not_exceeds"
	}
	return verdict
}

func allGatesPass(gates []GateData) bool {
	if len(gates) == 0 {
		return false
	}
	for _, gate := range gates {
		// These are explicitly diagnostic and never headline superiority gates.
		// Tool volume is not an efficiency objective, and controller duration is
		// not inferred from Pier trial timing when its own span is unavailable.
		if gate.Name == "tool_execution_coverage" || gate.Name == "controller_duration" {
			continue
		}
		if gate.Status != GatePass {
			return false
		}
	}
	return true
}

func clusterBootstrap(clusters map[string][]float64, statistics StatisticsSpec, salt string) *ConfidenceInterval {
	keys := sortedMapKeys(clusters)
	if len(keys) < 2 {
		return nil
	}
	estimate := clusterMean(keys, clusters)
	rng := newDeterministicRNG(statistics.Seed, salt)
	values := make([]float64, statistics.Resamples)
	sampled := make([]string, len(keys))
	for iteration := 0; iteration < statistics.Resamples; iteration++ {
		for index := range sampled {
			sampled[index] = keys[rng.intn(len(keys))]
		}
		values[iteration] = clusterMean(sampled, clusters)
	}
	slices.Sort(values)
	alpha := (1 - statistics.ConfidenceLevel) / 2
	lower := percentile(values, alpha)
	upper := percentile(values, 1-alpha)
	pairs := 0
	for _, key := range keys {
		pairs += len(clusters[key])
	}
	return &ConfidenceInterval{
		Estimate: estimate, Lower: lower, Upper: upper, ConfidenceLevel: statistics.ConfidenceLevel,
		Method: statistics.Method, Tasks: len(keys), Pairs: pairs, Resamples: statistics.Resamples, Seed: statistics.Seed,
	}
}

func clusterMean(keys []string, clusters map[string][]float64) float64 {
	total, count := 0.0, 0
	for _, key := range keys {
		for _, value := range clusters[key] {
			total += value
			count++
		}
	}
	if count == 0 {
		return math.NaN()
	}
	return total / float64(count)
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	position := quantile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

type deterministicRNG struct{ state uint64 }

func newDeterministicRNG(seed int64, salt string) *deterministicRNG {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(salt))
	state := uint64(seed) ^ hasher.Sum64()
	if state == 0 {
		state = 0x9e3779b97f4a7c15
	}
	return &deterministicRNG{state: state}
}

func (rng *deterministicRNG) intn(limit int) int {
	state := rng.state
	state ^= state << 13
	state ^= state >> 7
	state ^= state << 17
	rng.state = state
	return int(state % uint64(limit))
}

func runMetric(run RunData, metric ComparisonMetric) (float64, bool) {
	switch metric {
	case MetricPassRate:
		if run.Passed == nil {
			return 0, false
		}
		if *run.Passed {
			return 1, true
		}
		return 0, true
	case MetricWallTime:
		return floatMetric(run.Metrics.WallTimeSeconds)
	case MetricTrialDuration:
		return floatMetric(run.Metrics.TrialDurationSeconds)
	case MetricLLMCallsStarted:
		return intMetric(run.Metrics.LLMCallsStarted)
	case MetricProviderRounds:
		return intMetric(run.Metrics.ProviderRounds)
	case MetricProviderErrors:
		return intMetric(run.Metrics.ProviderErrors)
	case MetricToolBearingRounds:
		return intMetric(run.Metrics.ToolBearingRounds)
	case MetricToolInvocations:
		return intMetric(run.Metrics.ToolInvocations)
	case MetricPhysicalToolOperations:
		return intMetric(run.Metrics.PhysicalToolOperations)
	case MetricNativeEvents:
		return intMetric(run.Metrics.NativeEvents)
	case MetricToolErrors:
		return intMetric(run.Metrics.ToolErrors)
	case MetricInputTokens:
		return int64Metric(run.Metrics.AllExecutedInputTokens)
	case MetricCachedInputTokens:
		return int64Metric(run.Metrics.AllExecutedCachedTokens)
	case MetricCacheWriteInputTokens:
		return int64Metric(run.Metrics.AllExecutedCacheWriteInputTokens)
	case MetricOutputTokens:
		return int64Metric(run.Metrics.AllExecutedOutputTokens)
	case MetricReasoningTokens:
		return int64Metric(run.Metrics.ReasoningOutputTokens)
	case MetricTokenCacheHit:
		return floatMetric(run.Metrics.AllExecutedCacheHit)
	case MetricUncachedInputTokens:
		return int64Metric(run.Metrics.AllExecutedUncachedTokens)
	case MetricRequestCacheHit:
		return floatMetric(run.Metrics.RequestCacheHit)
	case MetricComparableCost:
		return floatMetric(run.Metrics.ComparableCost)
	case MetricProviderCost:
		return floatMetric(run.Metrics.ProviderReportedCost)
	default:
		return 0, false
	}
}

func allComparisonMetrics() []ComparisonMetric {
	return []ComparisonMetric{
		MetricPassRate, MetricWallTime, MetricTrialDuration, MetricLLMCallsStarted, MetricProviderRounds, MetricProviderErrors, MetricToolBearingRounds,
		MetricToolInvocations, MetricPhysicalToolOperations, MetricNativeEvents, MetricToolErrors,
		MetricInputTokens, MetricCachedInputTokens, MetricCacheWriteInputTokens, MetricUncachedInputTokens, MetricOutputTokens,
		MetricTokenCacheHit, MetricRequestCacheHit, MetricComparableCost, MetricProviderCost,
	}
}

func runsForAgent(runs []RunData, agentID string) []RunData {
	var result []RunData
	for _, run := range runs {
		if run.AgentID == agentID {
			result = append(result, run)
		}
	}
	return result
}

func compareRuns(left, right RunData) int {
	if comparison := strings.Compare(left.TaskID, right.TaskID); comparison != 0 {
		return comparison
	}
	if left.Repetition != right.Repetition {
		return left.Repetition - right.Repetition
	}
	return strings.Compare(left.AgentID, right.AgentID)
}

func taskRepetitionKey(run RunData) string { return fmt.Sprintf("%s/%d", run.TaskID, run.Repetition) }

func failureKey(experimentID, taskID, agentID string, repetition int) string {
	return fmt.Sprintf("%s/%s/%s/%d", experimentID, taskID, agentID, repetition)
}

func floatMetric(value *float64) (float64, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}

func intMetric(value *int) (float64, bool) {
	if value == nil {
		return 0, false
	}
	return float64(*value), true
}

func int64Metric(value *int64) (float64, bool) {
	if value == nil {
		return 0, false
	}
	return float64(*value), true
}

func sumFloatField(runs []RunData, field func(MetricData) *float64) *float64 {
	total := 0.0
	for _, run := range runs {
		value := field(run.Metrics)
		if value == nil {
			return nil
		}
		total += *value
	}
	return pointerFloat(total)
}

func sumIntField(runs []RunData, field func(MetricData) *int) *int {
	total := 0
	for _, run := range runs {
		value := field(run.Metrics)
		if value == nil {
			return nil
		}
		total += *value
	}
	return pointerInt(total)
}

func sumInt64Field(runs []RunData, field func(MetricData) *int64) *int64 {
	total := int64(0)
	for _, run := range runs {
		value := field(run.Metrics)
		if value == nil {
			return nil
		}
		total += *value
	}
	return pointerInt64(total)
}

func shellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for index, arg := range argv {
		if arg != "" && strings.IndexFunc(arg, func(character rune) bool {
			return !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && !strings.ContainsRune("_@%+=:,./-", character)
		}) == -1 {
			quoted[index] = arg
		} else {
			quoted[index] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		}
	}
	return strings.Join(quoted, " ")
}
