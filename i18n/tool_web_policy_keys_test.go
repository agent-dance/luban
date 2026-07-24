package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolWebPolicyKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolWebPolicyKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
			if strings.Contains(got, "%!") {
				t.Errorf("Text(%s, %q) has invalid formatting: %q", lang.Code(), key, got)
			}
		}
	}
}

func TestToolWebPolicyEnglishCompatibility(t *testing.T) {
	if got := Text(LangEN, KeyToolWebPolicyRegionBlocked); got != "Web search is only available in the US" {
		t.Fatalf("region blocked = %q", got)
	}
	if got := Text(LangEN, KeyToolWebPolicyRateLimited); got != "Web search rate limit exceeded; try again in a minute" {
		t.Fatalf("rate limited = %q", got)
	}
	if got := Format(LangEN, KeyToolWebPolicyRateLimitedWithLimit, 60); got != "Web search rate limit exceeded; try again in a minute (limit=60/min)" {
		t.Fatalf("rate limited with limit = %q", got)
	}
}

func TestToolWebPolicyInternalWrapPreservesSentinelWithoutLeakingIt(t *testing.T) {
	sentinel := errors.New("internal-web-policy-sentinel")
	err := WrapInternalError(KeyToolWebPolicyRateLimitedWithLimit, sentinel, 60)
	if !errors.Is(err, sentinel) {
		t.Fatal("WrapInternalError did not preserve errors.Is")
	}
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolWebPolicyRateLimitedWithLimit, 60)
		if strings.Contains(got, sentinel.Error()) {
			t.Fatalf("Format(%s) leaked internal sentinel: %q", lang.Code(), got)
		}
		if !strings.Contains(got, "60") {
			t.Fatalf("Format(%s) omitted rate limit: %q", lang.Code(), got)
		}
	}
}
