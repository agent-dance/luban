package i18n

import (
	"strings"
	"testing"
)

func TestAgenticLocal5SemanticCopyCoversEveryLanguage(t *testing.T) {
	const expectedKeys = 53
	if got := len(agenticLocal5Keys); got != expectedKeys {
		t.Fatalf("agentic local-five report key count = %d, want %d", got, expectedKeys)
	}

	seen := make(map[Key]struct{}, len(agenticLocal5Keys))
	for _, key := range agenticLocal5Keys {
		if !strings.HasPrefix(string(key), "agentic.local5.") {
			t.Errorf("agentic local-five report key %q has an unexpected namespace", key)
		}
		if _, duplicate := seen[key]; duplicate {
			t.Errorf("agentic local-five report key %q is duplicated", key)
		}
		seen[key] = struct{}{}

		translations, registered := semanticTranslations[key]
		if !registered {
			t.Errorf("agentic local-five report key %q is not registered", key)
			continue
		}
		for _, language := range AllLanguages() {
			got := strings.TrimSpace(translations[language])
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", language.Code(), key, got)
			}
		}
	}

	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("ValidateSemanticCatalog() error = %v", err)
	}
}

func TestAgenticLocal5CriticalDefinitionsRemainExplicit(t *testing.T) {
	checks := map[Key][]string{
		KeyAgenticLocal5Watermark:                              {"5-TASK", "NON-OFFICIAL", "NOT A PUBLIC BENCHMARK SCORE"},
		KeyAgenticLocal5CaveatCostFormulaEstimate:              {"deduplicated", "legacy runner", "provider invoice"},
		KeyAgenticLocal5CaveatExactLLMDefinition:               {"HTTP POST /responses", "provider-requests.jsonl", "not a start-time WAL"},
		KeyAgenticLocal5CaveatIncompleteRefusal:                {"N/A", "blocks", "never converted to zero"},
		KeyAgenticLocal5OptimizationThreeToolCatalog:           {"Inspect", "ApplyPatch", "Run", "exactly"},
		KeyAgenticLocal5OptimizationPreciseTelemetry:           {"formal telemetry schema", "completion-time HTTP meter", "request-start WAL"},
		KeyAgenticLocal5OptimizationPrintSessionQuartet:        {"session ID", "project root", "root-denied", "zero"},
		KeyAgenticLocal5OptimizationInspectCursorCompatibility: {"requests:[]", "max_*", "two-call", "zero"},
		KeyAgenticLocal5ConclusionNoComprehensiveSuperiority:   {"not demonstrated", "comprehensive superiority"},
		KeyAgenticLocal5ConclusionMeasured:                     {"strict raw", "POSTs", "wall time", "estimated cost"},
		KeyAgenticLocal5EfficiencyPrimaryConclusion:            {"Tool wrappers", "end-to-end efficiency"},
		KeyAgenticLocal5EfficiencyCompletionTail:               {"upper-bound", "cannot be assumed wholly wasted"},
		KeyAgenticLocal5EfficiencyFlightProof:                  {"write-effect", "proof obligation", "5/43", "37/43"},
		KeyAgenticLocal5LimitationEvaluatorSemantics:           {"partially applied", "nonzero", "Skim timeout"},
		KeyAgenticLocal5LimitationExperimentalDesign:           {"not random", "one run", "order"},
		KeyAgenticLocal5LimitationModelUnverified:              {"request-side", "do not attest"},
		KeyAgenticLocal5LimitationMutableRawNoManifest:         {"mutable", "SHA-256 manifest"},
		KeyAgenticLocal5Footer:                                 {"self-contained", "mutable", "not a content-addressed manifest"},
		KeyAgenticLocal5CaveatSharedPassScope:                  {"both agents passed", "strict evaluator", "one observed run", "unconditional", "statistical superiority"},
		KeyAgenticLocal5CaveatSharedPassToolTaxonomy:           {"different tool catalogs", "diagnostic only", "not like-for-like"},
		KeyAgenticLocal5SharedPassLongerSlower:                 {"input per POST is close", "provider time per POST", "output tokens per POST", "two-task sample", "does not establish causation"},
	}
	for key, fragments := range checks {
		copy := Text(LangEN, key)
		for _, fragment := range fragments {
			if !strings.Contains(strings.ToLower(copy), strings.ToLower(fragment)) {
				t.Errorf("Text(en, %q) = %q; want fragment %q", key, copy, fragment)
			}
		}
	}
}

func TestAgenticLocal5SharedPassScopeFormatsEveryLanguage(t *testing.T) {
	for _, language := range AllLanguages() {
		got := Format(language, KeyAgenticLocal5CaveatSharedPassScope, 2)
		if !strings.Contains(got, "2") || strings.Contains(got, "%!") {
			t.Errorf("Format(%s, shared-pass scope, 2) = %q", language.Code(), got)
		}
	}
}
