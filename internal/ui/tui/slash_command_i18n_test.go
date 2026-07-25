package tui

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestSlashSuggestionsResolveSemanticDescriptionKeys(t *testing.T) {
	state := computeSlashSuggestions("/go", []SlashCommandEntry{{
		Name:        "goal",
		Description: "Set or manage the session goal",
	}}, i18n.LangZH)
	if state == nil || len(state.Items) != 1 {
		t.Fatalf("suggestions = %#v", state)
	}
	item := state.Items[0]
	if item.DescriptionKey != i18n.KeyCommandGoalDescription {
		t.Fatalf("description key = %q, want %q", item.DescriptionKey, i18n.KeyCommandGoalDescription)
	}
	if got, want := localizedSlashCommandDescription(i18n.LangZH, item), "设置或管理会话目标"; got != want {
		t.Fatalf("localized description = %q, want %q", got, want)
	}
}

func TestSlashSuggestionUnknownExtensionKeepsRawDescription(t *testing.T) {
	item := SlashCommandEntry{Name: "extension", Description: "Extension-provided copy"}
	if got := localizedSlashCommandDescription(i18n.LangZH, item); got != item.Description {
		t.Fatalf("localized extension description = %q, want %q", got, item.Description)
	}
}
