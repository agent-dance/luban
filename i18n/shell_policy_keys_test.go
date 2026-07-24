package i18n

import "testing"

func TestShellPolicySemanticCatalogComplete(t *testing.T) {
	keys := []Key{
		KeyShellPolicyBlockRoot, KeyShellPolicyBlockHome, KeyShellPolicyBlockSystem,
		KeyShellPolicyBlockRawDevice, KeyShellPolicyBlockProtected, KeyShellPolicyBlockKnownPattern,
		KeyShellPolicyAskDynamicTarget, KeyShellPolicyAskDynamicFlags, KeyShellPolicyAskCommandSubst,
		KeyShellPolicyAskParseFailure, KeyShellPolicyAskUnprovenTarget, KeyShellPolicyAskDestructive,
		KeyShellPolicyAskStructural, KeyShellPolicyAskUnrestrictedCode, KeyShellPolicyRemediationApprove,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("missing %s translation for %s", lang, key)
			}
		}
	}
}
