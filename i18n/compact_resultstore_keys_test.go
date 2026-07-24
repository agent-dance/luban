package i18n

import (
	"errors"
	"strings"
	"testing"
)

type compactResultStoreTestCause struct{}

func (*compactResultStoreTestCause) Error() string { return "raw-os-error-42" }

func TestCompactResultStoreKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range compactResultStoreKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestCompactResultStoreEnglishCompatibility(t *testing.T) {
	cause := errors.New("raw-os-error")
	cases := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyCompactResultStoreUnavailable, nil, "persist raw tool output: result store is nil"},
		{KeyCompactResultStoreCreateRawDirectory, []any{cause}, "persist raw tool output: raw-os-error"},
		{KeyCompactResultStoreCreateRawFile, []any{cause}, "persist raw tool output: raw-os-error"},
		{KeyCompactResultStoreWriteRawFile, []any{"/raw/output", cause}, "persist raw tool output to /raw/output: raw-os-error"},
		{KeyCompactResultStoreCloseRawFile, []any{"/raw/output", cause}, "persist raw tool output to /raw/output: raw-os-error"},
		{KeyCompactResultStoreCreateResultFile, []any{"/raw/result", cause}, "persist tool result to /raw/result: raw-os-error"},
		{KeyCompactResultStoreWriteResultFile, []any{"/raw/result", cause}, "persist tool result to /raw/result: raw-os-error"},
		{KeyCompactResultStoreCloseResultFile, []any{"/raw/result", cause}, "persist tool result to /raw/result: raw-os-error"},
		{KeyCompactResultStoreSerializeStructured, []any{"toolu_raw", cause}, "serialize structured tool result toolu_raw: raw-os-error"},
	}

	for _, tc := range cases {
		if got := Format(LangEN, tc.key, tc.args...); got != tc.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestCompactResultStoreKeysPreserveRawValuesInEveryLanguage(t *testing.T) {
	cause := errors.New("raw-os-error-42")
	cases := []struct {
		key  Key
		args []any
		raw  []string
	}{
		{KeyCompactResultStoreCreateRawDirectory, []any{cause}, []string{"raw-os-error-42"}},
		{KeyCompactResultStoreCreateRawFile, []any{cause}, []string{"raw-os-error-42"}},
		{KeyCompactResultStoreWriteRawFile, []any{"/raw/output", cause}, []string{"/raw/output", "raw-os-error-42"}},
		{KeyCompactResultStoreCloseRawFile, []any{"/raw/output", cause}, []string{"/raw/output", "raw-os-error-42"}},
		{KeyCompactResultStoreCreateResultFile, []any{"/raw/result", cause}, []string{"/raw/result", "raw-os-error-42"}},
		{KeyCompactResultStoreWriteResultFile, []any{"/raw/result", cause}, []string{"/raw/result", "raw-os-error-42"}},
		{KeyCompactResultStoreCloseResultFile, []any{"/raw/result", cause}, []string{"/raw/result", "raw-os-error-42"}},
		{KeyCompactResultStoreSerializeStructured, []any{"toolu_raw", cause}, []string{"toolu_raw", "raw-os-error-42"}},
	}

	for _, tc := range cases {
		for _, lang := range AllLanguages() {
			got := Format(lang, tc.key, tc.args...)
			if strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %s) has an invalid format expansion: %q", lang.Code(), tc.key, got)
			}
			for _, raw := range tc.raw {
				if !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %s) lost raw value %q: %q", lang.Code(), tc.key, raw, got)
				}
			}
		}
	}
}

func TestCompactResultStoreErrorsUseRuntimeLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := &compactResultStoreTestCause{}
	err := WrapError(KeyCompactResultStoreWriteResultFile, cause, "/raw/result")
	if !errors.Is(err, cause) {
		t.Fatal("result-store error did not preserve its underlying cause")
	}
	var typedCause *compactResultStoreTestCause
	if !errors.As(err, &typedCause) || typedCause != cause {
		t.Fatal("result-store error did not preserve its typed underlying cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	if english != "persist tool result to /raw/result: raw-os-error-42" {
		t.Fatalf("English compatibility changed: %q", english)
	}
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, "/raw/result") || !strings.Contains(chinese, "raw-os-error-42") {
		t.Fatalf("runtime localization lost raw diagnostics: en=%q zh=%q", english, chinese)
	}
}
