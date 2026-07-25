package i18n

import "testing"

func TestCommandDescriptionKeysCoverEveryLanguage(t *testing.T) {
	names := []string{
		"activity", "clear", "compact", "config", "connect", "context",
		"detail", "diff", "doctor", "editor", "exit", "export",
		"fork", "goal", "help", "init", "language", "mcp",
		"model", "mouse", "permissions", "rename", "resume",
		"review", "search", "session", "skills", "status", "version",
	}
	if len(names) != len(commandDescriptionKeys) {
		t.Fatalf("tested command names = %d, registered keys = %d", len(names), len(commandDescriptionKeys))
	}
	for _, name := range names {
		key, ok := CommandDescriptionKey(name)
		if !ok {
			t.Errorf("CommandDescriptionKey(%q) is not registered", name)
			continue
		}
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got[0] == '[' {
				t.Errorf("Text(%s, %q) = %q, want a registered translation", lang.Code(), key, got)
			}
		}
	}

	if _, ok := CommandDescriptionKey("extension-command"); ok {
		t.Fatal("an extension command unexpectedly resolved to built-in copy")
	}
}
