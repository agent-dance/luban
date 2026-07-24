package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestNilMemoryTokenStoreSaveUsesRuntimeLanguage(t *testing.T) {
	previous := i18n.DetectOrLoadLanguage()
	t.Cleanup(func() {
		if err := i18n.SaveLanguage(previous); err != nil {
			t.Errorf("restore language: %v", err)
		}
	})

	var store *MemoryTokenStore
	err := store.Save(context.Background(), "server-key", StoredOAuthCredentials{})
	if err == nil {
		t.Fatal("nil MemoryTokenStore.Save succeeded")
	}

	if saveErr := i18n.SaveLanguage(i18n.LangEN); saveErr != nil {
		t.Fatalf("set English: %v", saveErr)
	}
	english := err.Error()
	if english != "services/mcp: nil memory token store" {
		t.Fatalf("English error = %q", english)
	}

	if saveErr := i18n.SaveLanguage(i18n.LangZH); saveErr != nil {
		t.Fatalf("set Chinese: %v", saveErr)
	}
	chinese := err.Error()
	if chinese == english || !strings.Contains(chinese, "memory token store") {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}
