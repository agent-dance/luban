package i18n

import (
	"strings"
	"testing"
)

func TestToolNotebookPromptCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range toolNotebookPromptKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || strings.HasPrefix(got, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestToolNotebookPromptEnglishContract(t *testing.T) {
	tests := []struct {
		key  Key
		want string
	}{
		{KeyToolNotebookEditDescription, "Edit cells in a Jupyter notebook (.ipynb file). Supports replace, insert, and delete operations."},
		{KeyToolNotebookEditInputPathDescription, "Path to the .ipynb file, relative to the project root or absolute"},
		{KeyToolNotebookEditInputCellIDDescription, "Cell ID to edit. For insert, the new cell is added after this ID."},
		{KeyToolNotebookEditInputNewSourceDescription, "New source content for the cell"},
		{KeyToolNotebookEditInputCellTypeDescription, "Cell type: code or markdown"},
		{KeyToolNotebookEditInputModeDescription, "Edit mode: replace (default), insert, or delete"},
	}
	for _, tt := range tests {
		if got := Text(LangEN, tt.key); got != tt.want {
			t.Errorf("Text(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
