package loop

import (
	"context"
	"errors"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
)

func isUserInterrupt(err error) bool {
	return errors.Is(err, context.Canceled)
}

func emitUserInterruption(onEvent func(stream.Event), turnCount int, reason string) {
	if onEvent == nil {
		return
	}
	if reason == "" {
		reason = "interrupt"
	}
	onEvent(stream.Event{
		Type:           stream.EventUserInterruption,
		Text:           i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeUserInterrupted),
		TerminalReason: "user_interruption",
		TurnCount:      turnCount,
		Metadata: map[string]any{
			"reason": reason,
		},
	})
}
