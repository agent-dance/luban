package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestCompactLoopErrorKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range compactLoopErrorKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing for %s", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
}

func TestCompactLoopErrorCopyPreservesRuntimeValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		unavailable := Text(lang, KeyCompactReactiveCompactorUnavailable)
		if unavailable == "" || unavailable == "["+string(KeyCompactReactiveCompactorUnavailable)+"]" {
			t.Fatalf("%s reactive-compactor copy is missing: %q", lang.Code(), unavailable)
		}
		api := Format(lang, KeyCompactSummaryAPICallFailed, "raw-provider-cause")
		if !strings.Contains(api, "raw-provider-cause") {
			t.Fatalf("%s API copy lost the provider cause: %q", lang.Code(), api)
		}
		hook := Format(lang, KeyCompactHookBlocked, "PreCompact", "raw-hook-reason")
		if !strings.Contains(hook, "PreCompact") || !strings.Contains(hook, "raw-hook-reason") {
			t.Fatalf("%s hook copy lost the hook identifier or raw reason: %q", lang.Code(), hook)
		}
	}
}

func TestCompactLoopRuntimeErrorsLocalizeAndPreserveCauses(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-provider-cause")
	err := WrapError(KeyCompactSummaryAPICallFailed, cause)
	if !errors.Is(err, cause) {
		t.Fatal("compaction error did not preserve its cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, cause.Error()) {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}

func TestCompactLoopErrorEnglishCompatibility(t *testing.T) {
	checks := map[string]string{
		Format(LangEN, KeyCompactSummaryAPICallFailed, "cause"):        "compaction API call failed: cause",
		Format(LangEN, KeyCompactSummaryStreamFailed, "cause"):         "compaction stream error: cause",
		Format(LangEN, KeyCompactSummaryFailed, "cause"):               "compact summary failed: cause",
		Format(LangEN, KeyCompactHookBlocked, "PreCompact", "reason"):  "PreCompact hook blocked compaction: reason",
		Text(LangEN, KeyLoopCompactionResultRejected):                  "Compaction returned a result that cannot be installed safely; history was left unchanged.",
		Format(LangEN, KeyLoopPostCompactResetTrackingFailed, "cause"): "reset session-memory compaction tracking: cause",
		Text(LangEN, KeyLoopPostCompactSkillCatalogEpochChanged):       "The Skill catalog changed while restoring post-compaction state.",
		Text(LangEN, KeyLoopPostCompactSkillCatalogMissing):            "The current Skill catalog snapshot is missing from the post-compaction history.",
		Text(LangEN, KeyLoopPostCompactSkillBodyEpochMissing):          "The post-compaction Skill body message is missing a valid context epoch.",
		Text(LangEN, KeyLoopPostCompactSkillEnvelopeTrailing):          "skill invocation envelope contains trailing JSON",
		Text(LangEN, KeyLoopPostCompactSkillEnvelopeNoBody):            "skill invocation envelope does not carry a body",
	}
	for got, want := range checks {
		if got != want {
			t.Fatalf("English compatibility changed: got %q want %q", got, want)
		}
	}
}
