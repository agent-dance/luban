package i18n

import "testing"

func TestTUICommandRuntimeCatalogIsComplete(t *testing.T) {
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
	keys := []Key{
		KeyTUIToolErrorHeader, KeyTUIToolPermissionDenied,
		KeyRuntimeActivityKindCommand, KeyRuntimeActivityNeedsInput,
		KeyRuntimeProviderDisconnected, KeyRuntimeDecisionScopeRule,
		KeyRuntimePermissionReviewNormalizedPath, KeyRuntimePermissionReviewAllowedDir,
		KeyRuntimePermissionReviewAccess, KeyRuntimePermissionAccessReadOnly,
		KeyRuntimePermissionAccessWrite, KeyRuntimePermissionAccessExecute,
		KeyRuntimePermissionReviewMatchedRule, KeyRuntimePermissionReviewRequiredScope,
		KeyRuntimePresentationLevelEvidence, KeyRuntimeCodePlainText,
		KeyRuntimeSkillVisibilityManualOnly, KeyRuntimeSkillScopeManaged,
		KeyRuntimeProjectInstructionsTemplate,
		KeyRuntimeCommandActivityActionFailed, KeyRuntimeCommandDetailFailed,
		KeyRuntimeCommandSearchFailed, KeyRuntimeCommandExportFailed,
		KeyRuntimeCommandEditorFailed, KeyRuntimeCommandMouseFailed,
		KeyRuntimeCommandSessionDeleteFailed,
		KeyRuntimeEditorEnvMissing, KeyRuntimeClipboardCommandMissing,
		KeyRuntimeWeekdayWednesday, KeyRuntimeCacheBreakDebug,
		KeyRuntimeJSONMarshalLog,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing in %s: %q", key, lang.Code(), got)
			}
		}
	}
}

func TestTUICommandRuntimeLabelsUseRequestedLanguage(t *testing.T) {
	checks := map[string]string{
		RuntimeActivityKindLabel(LangZH, "command"):        "命令",
		RuntimeActivityStateLabel(LangJA, "needs_input"):   "入力が必要",
		RuntimeActivityActionLabel(LangKO, "acknowledge"):  "확인",
		RuntimeProviderStatusLabel(LangDE, "disconnected"): "nicht verbunden",
		RuntimeDecisionOutcomeLabel(LangRU, "approved"):    "одобрено",
		RuntimeDecisionChoiceLabel(LangZH, "always_allow"): "始终允许",
		RuntimePresentationLevelLabel(LangJA, "evidence"):  "保持された証拠",
		RuntimeSkillVisibilityLabel(LangZH, "manual-only"): "仅手动",
		RuntimeSkillScopeLabel(LangDE, "managed"):          "verwaltete Richtlinie",
		RuntimeSkillContextLabel(LangRU, "fork"):           "изолированный Agent",
	}
	for got, want := range checks {
		if got != want {
			t.Fatalf("label = %q, want %q", got, want)
		}
	}
	if got := RuntimeSkillSourceLabel(LangZH, "mcp"); got != "MCP" {
		t.Fatalf("MCP protocol label changed: %q", got)
	}
}

func TestPermissionReviewCopyUsesRequestedLanguageAndPreservesIdentifiers(t *testing.T) {
	path := "/workspace/projekt/bericht.txt"
	if got, want := Format(LangDE, KeyRuntimePermissionReviewNormalizedPath, path), "Normalisierter Pfad: "+path; got != want {
		t.Fatalf("normalized path detail = %q, want %q", got, want)
	}
	rule := "shell.policy.ask.dynamic_target"
	if got, want := Format(LangZH, KeyRuntimePermissionReviewMatchedRule, rule), "命中的规则："+rule; got != want {
		t.Fatalf("matched rule detail = %q, want %q", got, want)
	}
	if got := Format(LangJA, KeyRuntimePermissionReviewAccess, Text(LangJA, KeyRuntimePermissionAccessWrite)); got != "アクセス: 書き込み" {
		t.Fatalf("access detail = %q", got)
	}
}

func TestRuntimeActivityStatePreservesSucceededSemantic(t *testing.T) {
	if got := RuntimeActivityStateLabel(LangEN, "succeeded"); got != "succeeded" {
		t.Fatalf("succeeded state = %q", got)
	}
	if got := RuntimeActivityStateLabel(LangZH, "succeeded"); got != "成功" {
		t.Fatalf("Chinese succeeded state = %q", got)
	}
}
