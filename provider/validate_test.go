package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type capMockProvider struct {
	name string
	caps ProviderCapabilities
}

func (m *capMockProvider) Name() string    { return m.name }
func (m *capMockProvider) ModelID() string { return "mock-model" }
func (m *capMockProvider) CreateStream(_ context.Context, _ Params) (<-chan types.StreamEvent, error) {
	return nil, nil
}
func (m *capMockProvider) Capabilities() ProviderCapabilities { return m.caps }

func TestValidateParams_ThinkingOnUnsupportedProvider(t *testing.T) {
	p := &capMockProvider{
		name: "openai",
		caps: ProviderCapabilities{Thinking: false, ToolUse: true},
	}

	params := Params{
		Thinking: &ThinkingConfig{Enabled: true, BudgetTokens: 2048},
	}

	err := ValidateParams(p, params)
	if err == nil {
		t.Fatal("expected error when thinking requested on non-thinking provider")
	}
	if !strings.Contains(err.Error(), "does not support extended thinking") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateParams_ThinkingOnSupportedProvider(t *testing.T) {
	p := &capMockProvider{
		name: "anthropic",
		caps: ProviderCapabilities{Thinking: true, ToolUse: true},
	}

	params := Params{
		Thinking: &ThinkingConfig{Enabled: true, BudgetTokens: 2048},
	}

	err := ValidateParams(p, params)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParams_NoThinkingRequested(t *testing.T) {
	p := &capMockProvider{
		name: "openai",
		caps: ProviderCapabilities{Thinking: false},
	}

	err := ValidateParams(p, Params{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParams_NoCapabilityProvider(t *testing.T) {
	// A provider that doesn't implement CapabilityProvider should pass validation
	p := &mockProvider{
		name: "custom",
	}

	params := Params{
		Thinking: &ThinkingConfig{Enabled: true},
	}

	err := ValidateParams(p, params)
	if err != nil {
		t.Errorf("expected nil error for non-CapabilityProvider, got: %v", err)
	}
}

func TestValidateParams_CustomToolsRequireExplicitCapability(t *testing.T) {
	definition := responsesCustomToolFixture()
	tests := []struct {
		name string
		p    Provider
		want bool
	}{
		{name: "unknown provider", p: &mockProvider{name: "custom"}, want: true},
		{name: "unknown capability", p: &capMockProvider{name: "unknown", caps: ProviderCapabilities{ToolUse: true}}, want: true},
		{name: "explicitly unsupported", p: &capMockProvider{name: "chat", caps: ProviderCapabilities{ToolUse: true, CustomTools: CapabilityUnsupported}}, want: true},
		{name: "explicitly supported", p: &capMockProvider{name: "responses", caps: ProviderCapabilities{ToolUse: true, CustomTools: CapabilitySupported}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateParams(test.p, Params{Model: "gpt-5.6-sol", Tools: []types.ToolDefinition{definition}})
			if (err != nil) != test.want {
				t.Fatalf("error = %v, wantError=%v", err, test.want)
			}
		})
	}
}

func TestValidateParams_RejectsMalformedCustomDefinitionBeforeCapability(t *testing.T) {
	definition := responsesCustomToolFixture()
	definition.Format = &types.ToolInputFormat{Type: "grammar", Syntax: "regex", Definition: ".*"}
	p := &capMockProvider{name: "responses", caps: ProviderCapabilities{ToolUse: true, CustomTools: CapabilitySupported}}
	if err := ValidateParams(p, Params{Tools: []types.ToolDefinition{definition}}); err == nil || !strings.Contains(err.Error(), "invalid or unsupported") {
		t.Fatalf("malformed definition error = %v", err)
	}
}
