package loop

import (
	"os"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

// Runtime presentation tests must not inherit the developer's saved language.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "luban-loop-home-")
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
