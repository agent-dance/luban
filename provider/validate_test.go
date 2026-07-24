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
