package main

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/tui"
)

func TestTerminalTUIProviderStatusDoesNotMisclassifyPersistenceFailure(t *testing.T) {
	saveErr := i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, errors.New("injected save failure"))
	status, update := terminalTUIProviderStatus(saveErr, true)
	if !update || status != tui.StatusConnected {
		t.Fatalf("completed provider request with save failure = %v, %t; want connected", status, update)
	}

	status, update = terminalTUIProviderStatus(saveErr, false)
	if update || status != tui.StatusUnknown {
		t.Fatalf("pre-provider local failure = %v, %t; want no status update", status, update)
	}

	apiErr := provider.NewAPIError(503, "overloaded_error", "provider unavailable", "")
	status, update = terminalTUIProviderStatus(apiErr, false)
	if !update || status != tui.StatusError {
		t.Fatalf("provider API failure = %v, %t; want error", status, update)
	}
}
