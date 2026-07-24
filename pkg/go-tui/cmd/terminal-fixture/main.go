package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	tui "github.com/grindlemire/go-tui"
)

const (
	fixtureReadyMarker   = "GO_TUI_TERMINAL_FIXTURE_READY"
	fixtureErrorExitCode = 23
)

type fixtureController struct {
	stop      func()
	terminate func()
	exitCode  int
}

func (c *fixtureController) handleKey(event tui.KeyEvent) bool {
	if event.Key != tui.KeyRune || event.Mod != tui.ModNone {
		return false
	}

	switch event.Rune {
	case 'q':
		c.exitCode = 0
	case 'e':
		c.exitCode = fixtureErrorExitCode
	case 't':
		if c.terminate != nil {
			c.terminate()
		}
		return true
	default:
		return false
	}

	if c.stop != nil {
		c.stop()
	}
	return true
}

func (c *fixtureController) KeyMap() tui.KeyMap {
	return tui.KeyMap{
		tui.OnStop(tui.Rune('q'), func(event tui.KeyEvent) { c.handleKey(event) }),
		tui.OnStop(tui.Rune('e'), func(event tui.KeyEvent) { c.handleKey(event) }),
		tui.OnStop(tui.Rune('t'), func(event tui.KeyEvent) { c.handleKey(event) }),
	}
}

func (c *fixtureController) Render(*tui.App) *tui.Element {
	return tui.New(tui.WithText(fixtureReadyMarker))
}

func main() {
	os.Exit(runFixture())
}

func runFixture() int {
	controller := &fixtureController{
		terminate: func() {
			process, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = process.Signal(syscall.Signal(15))
			}
		},
	}
	app, err := tui.NewApp(
		tui.WithRootComponent(controller),
		tui.WithLegacyKeyboard(),
	)
	if err != nil {
		writeFixtureEvent("startup_error", 1, err)
		return 1
	}
	controller.stop = app.Stop

	if err := app.Run(); err != nil {
		writeFixtureEvent("lifecycle_error", 1, err)
		return 1
	}
	if controller.exitCode != 0 {
		writeFixtureEvent("controlled_error", controller.exitCode, nil)
	}
	return controller.exitCode
}

func writeFixtureEvent(event string, code int, err error) {
	payload := struct {
		Event string `json:"event"`
		Code  int    `json:"code"`
		Error string `json:"error,omitempty"`
	}{
		Event: event,
		Code:  code,
	}
	if err != nil {
		payload.Error = err.Error()
	}
	encoded, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		fmt.Fprintf(os.Stderr, "terminal fixture: %s\n", marshalErr)
		return
	}
	_, _ = fmt.Fprintln(os.Stderr, string(encoded))
}
