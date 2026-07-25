package compact

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type fakePlanState struct {
	active bool
	file   string
}

func (s fakePlanState) IsActive() bool   { return s.active }
func (s fakePlanState) PlanFile() string { return s.file }

type fakeBackgroundTasks []BackgroundTaskSnapshot

func (t fakeBackgroundTasks) PostCompactBackgroundTasks() []BackgroundTaskSnapshot {
	return []BackgroundTaskSnapshot(t)
}

type fakeMCPState []MCPServerSnapshot

func (s fakeMCPState) PostCompactMCPServers() []MCPServerSnapshot {
	return []MCPServerSnapshot(s)
}

type fakeAgents []AgentDefinitionSnapshot

func (a fakeAgents) PostCompactAgentDefinitions(string) []AgentDefinitionSnapshot {
	return []AgentDefinitionSnapshot(a)
}

func TestRuntimeAttachmentProviderRestoresSupportedState(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(planPath, []byte("# Plan\n\nKeep planning."), 0644); err != nil {
		t.Fatal(err)
	}

	provider := &RuntimeAttachmentProvider{
		PlanState:       fakePlanState{active: true, file: planPath},
		BackgroundTasks: fakeBackgroundTasks{{ID: "7", Type: "local_agent", Status: "running", Description: "review branch"}},
		MCPState:        fakeMCPState{{Name: "docs", Tools: []string{"search", "read"}, Instructions: "Prefer docs resources."}},
		AgentDefinitions: fakeAgents{{
			Name:      "Explore",
			WhenToUse: "Map repository context before implementation.",
			Source:    "builtin",
		}},
		SessionID:         "session-1",
		CWD:               dir,
		DeferredToolNames: func() []string { return []string{"TaskCreate", "TaskOutput"} },
		LoadedToolNames:   func() []string { return []string{"TaskCreate"} },
	}

	attachments := provider.PostCompactAttachments(context.Background(), PostCompactAttachmentState{})
	joined := joinMessageText(attachments)
	for _, want := range []string{
		"Post-compaction plan state",
		"Keep planning.",
		"Post-compaction plan mode reminder",
		"Post-compaction background tasks",
		"review branch",
		"Post-compaction deferred tools",
		"TaskOutput",
		"Post-compaction agent listing",
		"Explore",
		"Post-compaction MCP state",
		"Prefer docs resources.",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("attachments missing %q:\n%s", want, joined)
		}
	}
}

func TestRuntimeAttachmentProviderMissingPlanFileEmitsOnlyPlanModeReminder(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "plans", "not-created.md")
	provider := &RuntimeAttachmentProvider{PlanState: fakePlanState{active: true, file: missing}}
	attachments := provider.PostCompactAttachments(context.Background(), PostCompactAttachmentState{})
	joined := joinMessageText(attachments)
	if strings.Contains(joined, "Post-compaction plan state") {
		t.Fatalf("missing plan file must not produce a plan-file attachment:\n%s", joined)
	}
	if strings.Count(joined, "Post-compaction plan mode reminder") != 1 {
		t.Fatalf("active plan mode must produce exactly one reminder:\n%s", joined)
	}
}

func joinMessageText(messages []types.Message) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, msg.GetText())
	}
	return strings.Join(parts, "\n")
}
