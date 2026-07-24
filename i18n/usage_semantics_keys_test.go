package i18n

import "testing"

func TestUsageSemanticsKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyUsageSession, KeyUsageSessionUnknownCost, KeyUsageSessionNoCache, KeyUsageSessionNoCacheUnknownCost,
		KeyUsageSessionCompacted, KeyUsageSessionCompactedUnknownCost, KeyUsageSessionCompactedNoCache,
		KeyUsageSessionCompactedNoCacheUnknownCost, KeyUsageSessionUnavailable,
		KeyUsageContext, KeyUsageContextCompact, KeyUsageContextPlain, KeyUsageContextEstimate,
		KeyUsageContextEstimateCompact, KeyUsageContextEstimatePlain, KeyUsageContextLowerBound,
		KeyUsageContextLowerBoundCompact, KeyUsageContextLowerBoundPlain, KeyUsageContextUnknown,
		KeyCommandContextUsageExact, KeyCommandContextUsageEstimate, KeyCommandContextUsageLowerBound,
		KeyUsageLastRequest, KeyUsageLastRequestUnknown,
		KeyUsageCumulativeSession, KeyUsageCumulativeUnknown,
		KeyUsageCumulativeUnavailable, KeyUsageScopedCompact, KeyUsageScopedCompactUnknownCost,
		KeyUsageEffectiveContext, KeyUsageEffectiveContextCompact,
		KeyUsageEffectiveContextPlain, KeyUsageEffectiveEstimate, KeyUsageEffectiveEstimateCompact,
		KeyUsageEffectiveEstimatePlain, KeyUsageEffectiveLowerBound, KeyUsageEffectiveLowerBoundCompact,
		KeyUsageEffectiveLowerBoundPlain, KeyUsageEffectiveUnknown,
	}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("missing %s translation for %s", key, lang)
			}
		}
	}
}
