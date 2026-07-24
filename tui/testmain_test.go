package tui

import (
	"os"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

// TUI presentation tests must not inherit or mutate the developer's persisted
// display language. A private HOME also makes English fallback assertions
// deterministic across local, CI, and screen-reader runs.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "claude-code-go-tui-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("LANG", "en_US.UTF-8")
	_ = os.Unsetenv("LC_ALL")
	_ = os.Unsetenv("LC_MESSAGES")
	if err := i18n.SaveLanguage(i18n.LangEN); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
