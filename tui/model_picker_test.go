package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func TestModelPickerEnterConnectStoresSetupHint(t *testing.T) {
	state := &ModelPickerState{Phase: PickerPhaseProvider}

	state.EnterConnect("bedrock", []string{"api_key", "aws_credentials"}, "", "https://proxy.example.com", "", "Set AWS_PROFILE, then reopen /model.")

	if state.Phase != PickerPhaseConnect {
		t.Fatalf("Phase = %v, want PickerPhaseConnect", state.Phase)
	}
	if state.ConnectProvider != "bedrock" {
		t.Fatalf("ConnectProvider = %q, want bedrock", state.ConnectProvider)
	}
	if state.ConnectHint != "Set AWS_PROFILE, then reopen /model." {
		t.Fatalf("ConnectHint = %q", state.ConnectHint)
	}
	if len(state.ConnectAuthMethods) != 2 || state.ConnectAuthMethods[0] != "api_key" {
		t.Fatalf("ConnectAuthMethods = %v", state.ConnectAuthMethods)
	}
	if state.ConnectBaseURLInput != "https://proxy.example.com" {
		t.Fatalf("ConnectBaseURLInput = %q", state.ConnectBaseURLInput)
	}
}

func TestModelPickerGoBackClearsSetupHint(t *testing.T) {
	state := &ModelPickerState{Phase: PickerPhaseConnect, ConnectHint: "configure externally", IsReconnect: true}

	state.GoBack()

	if state.Phase != PickerPhaseProvider {
		t.Fatalf("Phase = %v, want PickerPhaseProvider", state.Phase)
	}
	if state.ConnectHint != "" {
		t.Fatalf("ConnectHint = %q, want empty", state.ConnectHint)
	}
	if state.IsReconnect {
		t.Fatal("IsReconnect = true, want false")
	}
}

func TestModelPickerEnterReconnectUsesProviderConnectionOptions(t *testing.T) {
	state := &ModelPickerState{Phase: PickerPhaseProvider}
	provider := ProviderPickerEntry{
		Name:        "openai",
		AuthMethods: []string{"api_key", "oauth_pkce"},
		EnvKey:      "OPENAI_API_KEY",
		BaseURL:     "https://gateway.example.com/v1",
		SetupHint:   "Configure OpenAI credentials.",
	}

	state.EnterReconnect(provider)

	if state.Phase != PickerPhaseConnect || !state.IsReconnect {
		t.Fatalf("Phase/IsReconnect = %v/%v, want connect/true", state.Phase, state.IsReconnect)
	}
	if state.ConnectProvider != provider.Name || state.ConnectHint != provider.SetupHint {
		t.Fatalf("reconnect provider/hint = %q/%q", state.ConnectProvider, state.ConnectHint)
	}
	if len(state.ConnectAuthMethods) != 2 || state.ConnectAuthMethods[1] != "oauth_pkce" {
		t.Fatalf("ConnectAuthMethods = %v", state.ConnectAuthMethods)
	}
	if state.ConnectBaseURLInput != provider.BaseURL {
		t.Fatalf("ConnectBaseURLInput = %q, want %q", state.ConnectBaseURLInput, provider.BaseURL)
	}
}

func TestModelPickerCustomEndpointInputsTrackFocus(t *testing.T) {
	state := &ModelPickerState{}
	state.EnterConnect("openai", []string{"api_key"}, "OPENAI_API_KEY", "", "", "")

	if state.ConnectInputField != ConnectInputAPIKey {
		t.Fatalf("initial ConnectInputField = %v, want API key", state.ConnectInputField)
	}
	state.appendConnectInput('s')
	state.appendConnectInput('k')
	if state.ConnectAPIKeyInput != "sk" || state.ConnectBaseURLInput != "" {
		t.Fatalf("inputs after API key typing = key %q, URL %q", state.ConnectAPIKeyInput, state.ConnectBaseURLInput)
	}

	state.toggleConnectInputField()
	state.appendConnectInput('h')
	state.appendConnectInput('t')
	state.backspaceConnectInput()
	if state.ConnectBaseURLInput != "h" || state.ConnectAPIKeyInput != "sk" {
		t.Fatalf("inputs after URL edit = key %q, URL %q", state.ConnectAPIKeyInput, state.ConnectBaseURLInput)
	}
}

func TestRenderConnectPhaseShowsBaseURLAndAPIKey(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	state := &ModelPickerState{
		ConnectProvider:     "vertex",
		ConnectAuthMethods:  []string{"api_key", "gcp_adc"},
		ConnectSelectedAuth: 0,
		ConnectInputField:   ConnectInputBaseURL,
	}

	text := collectElementText(root.renderConnectPhase(gtui.New(), state))
	for _, want := range []string{"Base URL", "API key", "Tab switches fields"} {
		if !strings.Contains(text, want) {
			t.Fatalf("connect phase = %q, want %q", text, want)
		}
	}
}

func TestRenderProviderPhaseOffersReconnectForConnectedProvider(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	state := &ModelPickerState{
		ProviderSelected: 0,
		Providers: []ProviderPickerEntry{{
			Name:        "openai",
			DisplayName: "OpenAI",
			IsConnected: true,
		}},
	}

	text := collectElementText(root.renderProviderPhase(gtui.New(), state))
	if !strings.Contains(text, "R reconnect") {
		t.Fatalf("connected provider actions = %q, want reconnect option", text)
	}

	state.Providers[0].IsConnected = false
	text = collectElementText(root.renderProviderPhase(gtui.New(), state))
	if strings.Contains(text, "R reconnect") {
		t.Fatalf("disconnected provider actions = %q, want no reconnect option", text)
	}
}

func TestRenderProviderPhaseOffersConfigurationForSelectableLocalProvider(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	state := &ModelPickerState{
		ProviderSelected: 0,
		Providers: []ProviderPickerEntry{{
			Name:            "ollama",
			DisplayName:     "Ollama",
			CanSelectModels: true,
			AuthMethods:     []string{"api_key"},
		}},
	}

	text := collectElementText(root.renderProviderPhase(gtui.New(), state))
	if !strings.Contains(text, "R configure") {
		t.Fatalf("local provider actions = %q, want configure option", text)
	}
}

func TestRenderConnectPhaseLabelsReconnect(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	state := &ModelPickerState{
		ConnectProvider:    "openai",
		ConnectAuthMethods: []string{"oauth_pkce"},
		IsReconnect:        true,
		Providers: []ProviderPickerEntry{{
			Name:        "openai",
			DisplayName: "OpenAI",
		}},
	}

	text := collectElementText(root.renderConnectPhase(gtui.New(), state))
	if !strings.Contains(text, "Reconnect OpenAI") {
		t.Fatalf("reconnect phase = %q", text)
	}
}

func TestModelPickerEnterReasoningSelectsExistingEffort(t *testing.T) {
	state := &ModelPickerState{Phase: PickerPhaseModel}
	entry := ModelPickerEntry{
		ModelID:          "gpt-5.4",
		ReasoningEfforts: []string{"low", "medium", "high", "xhigh"},
		ReasoningEffort:  "high",
	}

	state.EnterReasoning(entry)

	if state.Phase != PickerPhaseReasoning {
		t.Fatalf("Phase = %v, want PickerPhaseReasoning", state.Phase)
	}
	if got := state.selectedReasoningEffort(); got != "high" {
		t.Fatalf("selected effort = %q, want high", got)
	}
}

func TestModelPickerEnterReasoningDefaultsToMedium(t *testing.T) {
	state := &ModelPickerState{Phase: PickerPhaseModel}
	entry := ModelPickerEntry{
		ModelID:          "gpt-5.4",
		ReasoningEfforts: []string{"low", "medium", "high", "xhigh"},
	}

	state.EnterReasoning(entry)

	if got := state.selectedReasoningEffort(); got != "medium" {
		t.Fatalf("selected effort = %q, want medium", got)
	}
}

func TestReasoningEffortInfoMatchesCodexLabels(t *testing.T) {
	tests := []struct {
		effort string
		label  string
		desc   string
	}{
		{"low", "Low", "Fast responses with lighter reasoning"},
		{"medium", "Medium", "Balances speed and reasoning depth for everyday tasks"},
		{"high", "High", "Greater reasoning depth for complex problems"},
		{"xhigh", "Extra High", "Very high reasoning depth for complex problems"},
		{"max", "Max", "Maximum reasoning depth for the hardest problems"},
	}
	for _, tt := range tests {
		got := ReasoningEffortInfo(tt.effort)
		if got.Label != tt.label || got.Description != tt.desc {
			t.Fatalf("ReasoningEffortInfo(%q) = %#v, want %q/%q", tt.effort, got, tt.label, tt.desc)
		}
	}
}

func TestModelPickerDescriptionMatchesCodexModelCopy(t *testing.T) {
	got := ModelPickerDescription(ModelPickerEntry{ModelID: "gpt-5.4"})
	want := "Strong model for everyday coding."
	if got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestModelPickerCopyUsesRuntimeLanguageAndPreservesExternalDescription(t *testing.T) {
	reasoning := ReasoningEffortInfoInLanguage(i18n.LangZH, "medium")
	if reasoning.Label != "中" || !strings.Contains(reasoning.Description, "日常任务") {
		t.Fatalf("reasoning copy was not localized: %#v", reasoning)
	}

	localized := ModelPickerDescriptionInLanguage(i18n.LangZH, ModelPickerEntry{ModelID: "gpt-5.4"})
	if localized == "Strong model for everyday coding." || !strings.Contains(localized, "日常编程") {
		t.Fatalf("model description was not localized: %q", localized)
	}

	external := "Provider supplied description"
	if got := ModelPickerDescriptionInLanguage(i18n.LangZH, ModelPickerEntry{ModelID: "external", Description: external}); got != external {
		t.Fatalf("external description changed: %q", got)
	}
}
