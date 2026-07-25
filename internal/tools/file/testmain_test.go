package file

import (
	"os"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestMain(m *testing.M) {
	testHome, err := os.MkdirTemp("", "luban-file-tools-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", testHome)
	_ = os.Setenv("LANG", "en_US.UTF-8")
	_ = os.Unsetenv("LC_ALL")
	_ = os.Unsetenv("LC_MESSAGES")
	if err := i18n.SaveLanguage(i18n.LangEN); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(testHome)
	os.Exit(code)
}
