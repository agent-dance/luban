package i18n

import (
	"strings"
	"testing"
)

func TestToolFilePromptKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolFilePromptKeys {
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

func TestToolFileRichPromptsPreserveProtocolIdentifiers(t *testing.T) {
	for _, lang := range AllLanguages() {
		read := Text(lang, KeyToolFileReadDescription)
		for _, token := range []string{"file_path", "offset", "limit", "Edit", "pages"} {
			if !strings.Contains(read, token) {
				t.Errorf("Read prompt for %s omitted %q", lang.Code(), token)
			}
		}
		edit := Text(lang, KeyToolFileEditDescription)
		for _, token := range []string{"Read", "old_string", "new_string", "replace_all", "Write"} {
			if !strings.Contains(edit, token) {
				t.Errorf("Edit prompt for %s omitted %q", lang.Code(), token)
			}
		}
	}
}
