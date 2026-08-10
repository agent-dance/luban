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

// PrintModeConfig binds a one-shot query to the exact startup session and
// workspace identity already published to the tool runtime.
type PrintModeConfig struct {
	SessionID         string
	SessionProjectDir string
	ProjectRoot       string
	CWD               string
	Query             string
	Verbose           bool
	Resume            bool
}

// RunPrintMode executes a single query in the already-resolved startup session,
// streams output, and returns an exit code.
func RunPrintMode(eng engine.Engine, r presentation.Renderer, cfg PrintModeConfig) int {
	if cfg.Query == "" {
		fmt.Fprint(os.Stderr, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyPrintQueryRequired))
		return 1
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if cfg.Resume {
		if _, err := eng.Resume(ctx, cfg.SessionID); err != nil {
			r.Error(engine.UserFacingError(i18n.DetectOrLoadLanguage(), err))
			return 1
		}
	}

	ch, err := eng.Query(ctx, engine.QueryRequest{
		SessionID:         cfg.SessionID,
		SessionProjectDir: cfg.SessionProjectDir,
		ProjectRoot:       cfg.ProjectRoot,
		CWD:               cfg.CWD,
		Message:           cfg.Query,
	})
	if err != nil {
		r.Error(engine.UserFacingError(i18n.DetectOrLoadLanguage(), err))
		return 1
	}

	handler := makeEventHandler(r, cfg.Verbose)
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
