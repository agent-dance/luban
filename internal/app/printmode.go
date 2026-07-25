package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/engine"
)

// RunPrintMode executes a single query, streams output, and returns an exit code.
func RunPrintMode(eng engine.Engine, r presentation.Renderer, query string, verbose bool) int {
	if query == "" {
		fmt.Fprint(os.Stderr, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyPrintQueryRequired))
		return 1
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	ch, err := eng.Query(ctx, engine.QueryRequest{
		Message: query,
	})
	if err != nil {
		r.Error(engine.UserFacingError(i18n.DetectOrLoadLanguage(), err))
		return 1
	}

	handler := makeEventHandler(r, verbose)
	var runErr error
	for evt := range ch {
		if evt.Final {
			runErr = evt.Error
		} else {
			handler(evt.Inner)
		}
	}

	r.Newline()
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		r.Error(engine.UserFacingError(i18n.DetectOrLoadLanguage(), runErr))
		return 1
	}
	return 0
}
