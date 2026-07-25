package interaction

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestInteractionToolsDeclareCanonicalMetadata(t *testing.T) {
	tests := []struct {
		name string
		tool types.ToolMetadataProvider
		want types.ToolMetadata
	}{
		{
			name: "AskUserQuestion",
			tool: NewAskUserQuestionTool(nil),
			want: types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true},
		},
		{
			name: "EnterPlanMode",
			tool: &EnterPlanModeTool{},
			want: types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000},
		},
		{
			name: "ExitPlanMode",
			tool: &ExitPlanModeTool{},
			want: types.ToolMetadata{Write: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.tool.ToolMetadata(nil); got != test.want {
				t.Fatalf("ToolMetadata() = %#v, want %#v", got, test.want)
			}
		})
	}
}
