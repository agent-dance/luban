package tasktools

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/types"
)

func (t *TaskListTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		ShouldDefer: true,
		SearchHint:  runtimeText(i18n.KeyToolTaskListDiscoveryHint),
	}
}

func (t *TaskListTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}
