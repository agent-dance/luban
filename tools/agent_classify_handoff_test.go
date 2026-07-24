package tools

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestClassifyAgentHandoff_NonAutoModePauses(t *testing.T) {
	transcript := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "all clear"},
		}},
	}
	if got := ClassifyAgentHandoff(transcript, "default"); got != HandoffPause {
		t.Fatalf("non-auto mode must pause, got %v", got)
	}
	if got := ClassifyAgentHandoff(transcript, ""); got != HandoffPause {
		t.Fatalf("empty mode must pause, got %v", got)
	}
}

func TestClassifyAgentHandoff_AutoModeContinuesOnCleanRun(t *testing.T) {
	transcript := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "Found the file at src/foo.go. Done."},
		}},
	}
	if got := ClassifyAgentHandoff(transcript, "auto"); got != HandoffContinue {
		t.Fatalf("auto mode + clean transcript should continue, got %v", got)
	}
}

func TestClassifyAgentHandoff_AutoRuntimeAliasesContinueOnCleanRun(t *testing.T) {
	transcript := []types.Message{
		types.AssistantMessage("Completed successfully."),
	}
	for _, mode := range []string{"acceptEdits", "bypassPermissions"} {
		t.Run(mode, func(t *testing.T) {
			if got := ClassifyAgentHandoff(transcript, mode); got != HandoffContinue {
				t.Fatalf("expected %s to use Auto handoff classification, got %v", mode, got)
			}
		})
	}
}

func TestClassifyAgentHandoff_AutoModePausesOnFailureMarker(t *testing.T) {
	transcript := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "Verification result: FAIL — build broken"},
		}},
	}
	if got := ClassifyAgentHandoff(transcript, "auto"); got != HandoffPause {
		t.Fatalf("FAIL marker should pause, got %v", got)
	}
}

func TestClassifyAgentHandoff_AutoModePausesOnUnresolvedToolUse(t *testing.T) {
	transcript := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_1", Name: "Bash", Input: map[string]any{}},
		}},
	}
	if got := ClassifyAgentHandoff(transcript, "auto"); got != HandoffPause {
		t.Fatalf("unresolved tool_use should pause, got %v", got)
	}
}

func TestClassifyAgentHandoff_PluggableClassifier(t *testing.T) {
	t.Cleanup(func() { SetAgentHandoffClassifier(nil) })
	SetAgentHandoffClassifier(AgentHandoffClassifierFunc(func(_ []types.Message, _ string) AgentHandoffVerdict {
		return HandoffContinue
	}))
	transcript := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "FAIL FAIL FAIL"},
		}},
	}
	if got := ClassifyAgentHandoff(transcript, "auto"); got != HandoffContinue {
		t.Fatalf("plug-in classifier should override default, got %v", got)
	}
}
