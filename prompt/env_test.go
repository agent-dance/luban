package prompt

import (
	"strings"
	"testing"
)

func TestEnvironmentContextBuilderIncludesSessionDetails(t *testing.T) {
	got := EnvironmentContextBuilder{
		PrimaryCWD:       "/repo",
		AdditionalDirs:   []string{"/repo", "/extra"},
		Platform:         "test-os",
		Shell:            "/bin/testsh",
		OSVersion:        "test-version",
		ModelID:          "model-id",
		ModelDescription: "Model Name",
		KnowledgeCutoff:  "2026-01",
	}.Build()

	assertInOrder(t, got, []string{
		"You have been invoked in the following environment:",
		" - Primary working directory: /repo",
		" - Additional working directories:",
		"  - /extra",
		" - Platform: test-os",
		" - Shell: /bin/testsh",
		" - OS version: test-version",
		" - You are powered by the model named Model Name. The exact model ID is model-id.",
		" - Assistant knowledge cutoff is 2026-01.",
	})
	if strings.Contains(got, "  - /repo") {
		t.Fatalf("primary cwd should not be duplicated as an additional directory:\n%s", got)
	}
}

func TestBuildSystemPromptUsesEnvironmentContextWithoutGitStatus(t *testing.T) {
	cfg := Config{
		CWD:              "/repo",
		AdditionalDirs:   []string{"/extra"},
		ModelID:          "model-id",
		ModelDescription: "Model Name",
		KnowledgeCutoff:  "2026-01",
		GitContext:       "This is the git status at the start of the conversation.",
	}
	got := BuildSystemPrompt(nil, cfg)

	for _, want := range []string{
		"Primary working directory: /repo",
		"Additional working directories:",
		"/extra",
		"Platform:",
		"Model Name",
		"Assistant knowledge cutoff is 2026-01.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected system prompt to contain %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "git status at the start of the conversation") {
		t.Fatal("git context should be injected through SystemContext, not base system prompt")
	}

	block, ok := (SystemContextBuilder{}).FromConfig(cfg).Build().Block()
	if !ok {
		t.Fatal("expected system context block")
	}
	if !strings.Contains(block.Text, "gitStatus: This is the git status at the start of the conversation.") {
		t.Fatalf("expected git status in SystemContext block, got %q", block.Text)
	}
}
