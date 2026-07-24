package i18n

import (
	"strings"
	"testing"
)

func TestToolNotebookHelperCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range toolNotebookHelperKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestToolNotebookHelperEnglishContract(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolNotebookHelperNilNotebook, nil, "nil notebook"},
		{KeyToolNotebookHelperInvalidJSON, []any{"raw-cause"}, "invalid notebook JSON: raw-cause"},
		{KeyToolNotebookHelperUnknownEditMode, []any{"explode"}, "unknown edit_mode: explode"},
		{KeyToolNotebookHelperCellIDRequired, []any{"replace"}, "cell_id is required for replace"},
		{KeyToolNotebookHelperCellIDRequired, []any{"delete"}, "cell_id is required for delete"},
		{KeyToolNotebookHelperCellNotFound, []any{"cell-7"}, "Cell not found: cell-7"},
		{KeyToolNotebookHelperCellIDNotFound, []any{"raw-cell-id"}, `Cell with ID "raw-cell-id" not found in notebook.`},
		{KeyToolNotebookHelperCellIndexNotFound, []any{17}, "Cell with index 17 does not exist in notebook."},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestToolNotebookHelperErrorsUseRuntimeLanguage(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	err := NewError(KeyToolNotebookHelperCellIDNotFound, "raw-cell-id")
	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()

	if english == chinese {
		t.Fatalf("runtime language did not change the error: %q", english)
	}
	if !strings.Contains(chinese, "raw-cell-id") {
		t.Fatalf("localized error omitted the raw cell ID: %q", chinese)
	}
}

func TestToolNotebookHelperTranslationsPreserveRawValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		checks := []struct {
			key  Key
			args []any
			raw  string
		}{
			{KeyToolNotebookHelperUnknownEditMode, []any{"raw-operation"}, "raw-operation"},
			{KeyToolNotebookHelperCellNotFound, []any{"raw-cell-id"}, "raw-cell-id"},
		}
		for _, check := range checks {
			got := Format(lang, check.key, check.args...)
			if !strings.Contains(got, check.raw) {
				t.Errorf("%s omitted raw value %q: %q", lang.Code(), check.raw, got)
			}
			if strings.Contains(got, "%!") {
				t.Errorf("%s has a formatting error: %q", lang.Code(), got)
			}
		}
	}
}
