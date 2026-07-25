package i18n

import (
	"strings"
	"testing"
)

func TestServicesMCPCatalogPaginationKeysCoverEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		cursor := Format(lang, KeyServicesMCPCatalogCursorLoop, "tools/list", "cursor-7")
		if !strings.Contains(cursor, "tools/list") || !strings.Contains(cursor, "cursor-7") || strings.Contains(cursor, "%!") {
			t.Errorf("cursor copy for %s = %q", lang.Code(), cursor)
		}
		for _, key := range []Key{KeyServicesMCPCatalogPageLimit, KeyServicesMCPCatalogItemLimit} {
			limit := Format(lang, key, "resources/list", 1000)
			if !strings.Contains(limit, "resources/list") || !strings.Contains(limit, "1000") || strings.Contains(limit, "%!") {
				t.Errorf("limit copy for %s key %q = %q", lang.Code(), key, limit)
			}
		}
	}
}
