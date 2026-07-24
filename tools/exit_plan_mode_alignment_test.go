package tools

import (
	"context"
	"os"
	"strings"
	"testing"
)

// These fixtures pin the current ExitPlanModeV2 contract. The approval phase
// is represented by the Registry's one-time approved execution helper:
// CheckPermissions owns the preceding prompt and Registry owns the commit.
// TS ref: src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:110-141,419-491.
func runExitPlanModeAlignment(t *testing.T, allowed []map[string]any) (
	state *PlanState,
	planFile string,
	data exitPlanModeResult,
	content string,
) {
	t.Helper()
	tmp := t.TempDir()
	t.Chdir(tmp)
	state = NewPlanState(tmp)

	enter := NewEnterPlanModeTool(state)
	if result, err := enter.Execute(context.Background(), map[string]any{}); err != nil || result.IsError {
		t.Fatalf("EnterPlanMode setup failed: result=%#v err=%v", result, err)
	}
	planFile = state.PlanFile()
	if err := os.WriteFile(planFile, []byte("# Alignment Plan\n\nImplement it safely."), 0o600); err != nil {
		t.Fatalf("materialize plan: %v", err)
	}

	input := map[string]any{}
	if allowed != nil {
		items := make([]any, 0, len(allowed))
		for _, prompt := range allowed {
			items = append(items, prompt)
		}
		input["allowedPrompts"] = items
	}
	result, err := executeApprovedToolForTest(t, NewExitPlanModeTool(state), input)
	if err != nil || result.IsError {
		t.Fatalf("approved ExitPlanMode failed: result=%#v err=%v", result, err)
	}
	typed, ok := result.Data.(exitPlanModeResult)
	if !ok {
		t.Fatalf("ExitPlanMode data type = %T, want exitPlanModeResult", result.Data)
	}
	return state, planFile, typed, result.Content
}

func TestExitPlanModeAlignment_ResultUsesTypedV2DataNotJSONEnvelope(t *testing.T) {
	_, _, typed, content := runExitPlanModeAlignment(t, nil)
	if typed.Plan == nil || typed.IsAgent || typed.Status != ExitPlanModeApproved {
		t.Fatalf("typed result = %#v", typed)
	}
	if strings.HasPrefix(strings.TrimSpace(content), "{") {
		t.Fatalf("model-visible result leaked a Go JSON envelope: %q", content)
	}
}

func TestExitPlanModeAlignment_ResultHasPlanPath(t *testing.T) {
	_, planFile, typed, _ := runExitPlanModeAlignment(t, nil)
	if typed.FilePath != planFile {
		t.Fatalf("filePath = %q, want %q", typed.FilePath, planFile)
	}
}

func TestExitPlanModeAlignment_ResultHasApprovedPlanData(t *testing.T) {
	_, _, typed, _ := runExitPlanModeAlignment(t, nil)
	if typed.Plan == nil || !strings.Contains(*typed.Plan, "Alignment Plan") {
		t.Fatalf("approved plan missing from typed output: %#v", typed)
	}
	if typed.AwaitingLeaderApproval || typed.RequestID != "" {
		t.Fatalf("local approval must not masquerade as teammate request: %#v", typed)
	}
}

func TestExitPlanModeAlignment_AllowedPromptsPersistAfterExit(t *testing.T) {
	allowed := []map[string]any{
		{"tool": "Bash", "prompt": "go test ./..."},
		{"tool": "Bash", "prompt": "go build ./..."},
	}
	state, _, _, _ := runExitPlanModeAlignment(t, allowed)
	got := state.AllowedPrompts()
	if len(got) != len(allowed) {
		t.Fatalf("AllowedPrompts length = %d, want %d: %#v", len(got), len(allowed), got)
	}
	if !state.AllowedPromptMatches("Bash", "go test ./... -count=1") ||
		!state.AllowedPromptMatches("Bash", "go build ./... -v") {
		t.Fatalf("approved prompts do not grant post-exit Bash categories: %#v", got)
	}
}

func TestExitPlanModeAlignment_ResultHasModelVisibleApprovalMessage(t *testing.T) {
	_, planFile, _, content := runExitPlanModeAlignment(t, nil)
	for _, want := range []string{
		"User has approved your plan",
		"## Approved Plan:",
		planFile,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("approved result missing %q: %q", want, content)
		}
	}
}
