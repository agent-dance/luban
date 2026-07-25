package loop

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

func stripThinkingSignatures(messages []types.Message) []types.Message {
	out := make([]types.Message, len(messages))
	for i, msg := range messages {
		out[i] = msg
		if len(msg.Content) == 0 {
			continue
		}
		out[i].Content = make([]types.ContentBlock, len(msg.Content))
		for j, block := range msg.Content {
			if thinking, ok := block.(types.ThinkingBlock); ok {
				thinking.Signature = ""
				out[i].Content[j] = thinking
				continue
			}
			out[i].Content[j] = block
		}
	}
	return out
}

func emitFallbackTombstone(onEvent func(stream.Event), msg *types.Message, turnCount int, originalModel, fallbackModel string) {
	if onEvent == nil || msg == nil || len(msg.Content) == 0 {
		return
	}
	onEvent(stream.Event{
		Type:           stream.EventTombstone,
		TerminalReason: "model_fallback",
		TurnCount:      turnCount,
		Tombstone: &stream.TombstoneEvent{
			Reason:  "model_fallback",
			Summary: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopFallbackTombstoneSummary),
			Metadata: map[string]any{
				"original_model": originalModel,
				"fallback_model": fallbackModel,
			},
		},
		Metadata: map[string]any{
			"original_model": originalModel,
			"fallback_model": fallbackModel,
		},
	})
}
