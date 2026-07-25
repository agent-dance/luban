package app

import (
	"github.com/agent-dance/luban/types"
)

func task23SkillRuntimeTextEvents(text string) []types.StreamEvent {
	stop := types.StopReasonEndTurn
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}
