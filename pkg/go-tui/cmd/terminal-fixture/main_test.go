package main

import (
	"testing"

	tui "github.com/grindlemire/go-tui"
)

func TestFixtureControllerStopsWithRequestedExitCode(t *testing.T) {
	tests := []struct {
		name     string
		key      rune
		wantCode int
	}{
		{name: "normal", key: 'q', wantCode: 0},
		{name: "controlled error", key: 'e', wantCode: fixtureErrorExitCode},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stops := 0
			controller := fixtureController{stop: func() { stops++ }}

			consumed := controller.handleKey(tui.KeyEvent{Key: tui.KeyRune, Rune: test.key})

			if !consumed {
				t.Fatal("fixture control key was not consumed")
			}
			if stops != 1 {
				t.Fatalf("stop calls = %d, want 1", stops)
			}
			if controller.exitCode != test.wantCode {
				t.Fatalf("exit code = %d, want %d", controller.exitCode, test.wantCode)
			}
		})
	}
}

func TestFixtureControllerLeavesCtrlZForAppSuspendFallback(t *testing.T) {
	stops := 0
	controller := fixtureController{stop: func() { stops++ }}

	consumed := controller.handleKey(tui.KeyEvent{Key: tui.KeyRune, Rune: 'z', Mod: tui.ModCtrl})

	if consumed {
		t.Fatal("Ctrl+Z must reach the app suspend fallback")
	}
	if stops != 0 {
		t.Fatalf("stop calls = %d, want 0", stops)
	}
}

func TestFixtureControllerPublishesControlKeysThroughComponentKeyMap(t *testing.T) {
	controller := fixtureController{}

	bindings := controller.KeyMap()

	if len(bindings) != 3 {
		t.Fatalf("key bindings = %d, want 3", len(bindings))
	}
	if root := controller.Render(nil); root == nil {
		t.Fatal("fixture component returned a nil render root")
	}
}

func TestFixtureControllerCanRequestSIGTERMThroughInjectedSignal(t *testing.T) {
	requests := 0
	controller := fixtureController{terminate: func() { requests++ }}

	consumed := controller.handleKey(tui.KeyEvent{Key: tui.KeyRune, Rune: 't'})

	if !consumed {
		t.Fatal("SIGTERM control key was not consumed")
	}
	if requests != 1 {
		t.Fatalf("termination requests = %d, want 1", requests)
	}
}
