package i18n

import (
	"strings"
	"testing"
)

func TestToolCollaborationPromptKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolCollaborationPromptKeys {
		for _, language := range AllLanguages() {
			translation := Text(language, key)
			if strings.TrimSpace(translation) == "" || translation == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", language.Code(), key, translation)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolCollaborationPromptEnglishRendering(t *testing.T) {
	tests := []struct {
		key  Key
		want string
	}{
		{KeyToolTeamCreateDescription, "Create a team of agents for parallel work"},
		{KeyToolTeamDeleteDescription, "Delete a team and its agents"},
		{KeyToolSendMessageDescription, "Send a message to another agent"},
		{KeyToolTeamCreateTeamNameDescription, "Name for the new team to create."},
		{KeyToolTeamCreatePurposeDescription, "Team description/purpose."},
		{KeyToolTeamCreateAgentTypeDescription, `Type/role of the team lead (e.g., "researcher", "test-runner"). Used for team file and inter-agent coordination.`},
		{KeyToolSendMessageToDescription, `Recipient: teammate name, "*" for broadcast, or "uds:<socket-path>" for a local peer`},
		{KeyToolSendMessageSummaryDescription, "A 5-10 word summary shown as a preview in the UI (required when message is a string)"},
		{KeyToolSendMessageMessageDescription, "Plain text message content or a structured swarm control message"},
		{KeyToolSendMessagePlainTextDescription, "Plain text message content"},
		{KeyToolCollaborationRuntimeIdentityIncomplete, "active runtime identity is incomplete"},
		{KeyToolCollaborationManagerRequired, "team manager is required"},
		{KeyToolCollaborationSpawnReservationMissing, `Teammate spawn reservation %q is missing.`},
	}

	for _, tt := range tests {
		if got := Text(LangEN, tt.key); got != tt.want {
			t.Errorf("Text(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestToolCollaborationPromptProtocolValuesAreStable(t *testing.T) {
	protocolValues := []string{"researcher", "test-runner", `"*"`, "uds:<socket-path>"}
	for _, language := range AllLanguages() {
		text := Text(language, KeyToolTeamCreateAgentTypeDescription) + " " +
			Text(language, KeyToolSendMessageToDescription)
		for _, value := range protocolValues {
			if !strings.Contains(text, value) {
				t.Errorf("localized collaboration prompts for %s omitted protocol value %q: %q", language.Code(), value, text)
			}
		}
	}
}
