package tui

import (
	"testing"

	gotui "github.com/grindlemire/go-tui"
)

func TestDefaultTUIAppOptionsEnableMouseCapture(t *testing.T) {
	app := &gotui.App{}
	if err := gotui.WithoutMouse()(app); err != nil {
		t.Fatal(err)
	}
	for _, option := range defaultTUIAppOptions(nil) {
		if err := option(app); err != nil {
			t.Fatal(err)
		}
	}

	if !app.MouseEnabled() {
		t.Fatal("default TUI options disabled terminal mouse reporting")
	}
}
