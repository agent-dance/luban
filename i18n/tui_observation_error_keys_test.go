package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestTUIObservationErrorCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range tuiObservationErrorKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestTUIObservationErrorEnglishContracts(t *testing.T) {
	cause := errors.New("raw-cause")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyTUIObservationStoreNotFound, nil, "observation not found"},
		{KeyTUIObservationMissingToolUseID, nil, "missing tool use ID"},
		{KeyTUIObservationToolUseIDConflict, nil, "tool use ID conflict"},
		{KeyTUIObservationToolCallMissingID, []any{"Read", cause}, `tool call "Read": raw-cause`},
		{KeyTUIObservationToolCallIDConflict, []any{"Read", "toolu-7", cause}, `tool call "Read" (toolu-7): raw-cause`},
		{KeyTUIObservationRetainResultEvidence, []any{cause}, "retain tool result evidence: raw-cause"},
		{KeyTUIObservationEncodeStructuredResult, []any{cause}, "encode structured tool result evidence: raw-cause"},
		{KeyTUIObservationRetainStructuredResult, []any{cause}, "retain structured tool result evidence: raw-cause"},
		{KeyTUIObservationToolResultMissingID, []any{cause}, "tool result: raw-cause"},
		{KeyTUIObservationToolResultMatchCount, []any{"toolu-7", 2, cause}, "tool result toolu-7 has 2 matching calls: raw-cause"},
	}
	for _, test := range tests {
		if got := Format(LangEN, test.key, test.args...); got != test.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", test.key, got, test.want)
		}
	}
}

type tuiObservationTypedCause struct{ marker string }

func (e *tuiObservationTypedCause) Error() string { return "raw-typed-cause-42" }

func TestTUIObservationErrorUsesRuntimeLanguageAndPreservesCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := &tuiObservationTypedCause{marker: "42"}
	err := WrapError(KeyTUIObservationRetainResultEvidence, cause)
	if !errors.Is(err, cause) {
		t.Fatal("WrapError did not preserve errors.Is")
	}
	var typed *tuiObservationTypedCause
	if !errors.As(err, &typed) || typed.marker != "42" {
		t.Fatalf("WrapError did not preserve errors.As: %#v", typed)
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english != "retain tool result evidence: raw-typed-cause-42" {
		t.Fatalf("English error = %q", english)
	}
	if chinese == english || !strings.Contains(chinese, "raw-typed-cause-42") || strings.Contains(chinese, "%!") {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}
