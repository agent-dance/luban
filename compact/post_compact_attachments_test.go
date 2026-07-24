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

type fakeSkillInvocations []InvokedSkillSnapshot

func (s fakeSkillInvocations) PostCompactInvokedSkills(string) []InvokedSkillSnapshot {
	return []InvokedSkillSnapshot(s)
}

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
		InvokedSkills:   fakeSkillInvocations{{Name: "review", ToolUseID: "toolu_1", Source: "project"}},
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
		"Post-compaction invoked skills",
		"review",
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

func TestUnsupportedTSPostCompactAttachmentTypesAreDocumented(t *testing.T) {
	joined := strings.Join(UnsupportedTSPostCompactAttachmentTypes, "\n")
	for _, want := range []string{
		"full_skill_listing_reinjection",
		"loaded_nested_memory_paths",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("unsupported TS attachment list missing %q: %#v", want, UnsupportedTSPostCompactAttachmentTypes)
		}
	}
	for _, nowSupported := range []string{
		"session_start_hook_results",
		"transcript_path_attachment",
		"tools_mcp_initialize_instruction_delta",
	} {
		if strings.Contains(joined, nowSupported) {
			t.Fatalf("unsupported TS attachment list still contains supported surface %q: %#v", nowSupported, UnsupportedTSPostCompactAttachmentTypes)
		}
	}
}

func TestSummaryCompactorSkipsRecentFileAlreadyVisibleInPreservedTail(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "visible.go")
	if err := os.WriteFile(filePath, []byte("package visible\n"), 0644); err != nil {
		t.Fatal(err)
	}

	msgs := []types.Message{
		types.UserMessage("old"),
		assistantWithToolUse("Read", filePath),
		types.AssistantMessage("middle"),
		types.UserMessage("keep-1"),
		assistantWithToolUse("Read", filePath),
		types.UserMessage("keep-2"),
	}
	sc := &SummaryCompactor{
		Summarize: func(context.Context, string, string) (string, error) {
			return "summary", nil
		},
		KeepRecent:  3,
		AllowedDirs: []string{dir},
	}

	result, err := sc.Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attachments) != 0 {
		t.Fatalf("expected no recovered file attachments when file is visible in preserved tail, got %s", joinMessageText(result.Attachments))
	}
}

func joinMessageText(messages []types.Message) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		parts = append(parts, msg.GetText())
	}
	return strings.Join(parts, "\n")
}
