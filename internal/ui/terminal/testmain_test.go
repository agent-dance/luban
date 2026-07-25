package ui

import (
	"os"
	"testing"
)

// Renderers read the persisted display-language preference. Isolate it from a
// developer's real home directory so output assertions remain deterministic.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "luban-ui-test-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if err := os.Setenv("LANG", "en_US.UTF-8"); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
