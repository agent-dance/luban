package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/ui"
)

// RunPrintMode executes a single query, streams output, and returns an exit code.
func RunPrintMode(eng engine.Engine, r ui.Renderer, query string, verbose bool) int {
	if query == "" {
		fmt.Fprint(os.Stderr, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyPrintQueryRequired))
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

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
