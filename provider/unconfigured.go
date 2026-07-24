package provider

import (
	"context"
	"fmt"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// UnconfiguredProvider lets the terminal UI start before credentials exist.
// Actual model calls fail with an actionable message instead of blocking setup
// workflows such as /model and /config.
type UnconfiguredProvider struct {
	name    string
	model   string
	envVar  string
	message string
}

func NewUnconfiguredProvider(name, model, envVar, message string) *UnconfiguredProvider {
	if model == "" {
		model = brand.DeepSeekDefaultModel
	}
	if message == "" {
		message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderUnconfigured, name)
	}
	return &UnconfiguredProvider{name: name, model: model, envVar: envVar, message: message}
}

func (p *UnconfiguredProvider) Name() string { return p.name }

func (p *UnconfiguredProvider) ModelID() string { return p.model }

func (p *UnconfiguredProvider) CreateStream(context.Context, Params) (<-chan types.StreamEvent, error) {
	return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderUnconfiguredAction, p.message, p.envVar, brand.CommandName))
}

func (p *UnconfiguredProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		ToolUse:    true,
		MaxContext: LookupMaxContext(p.model),
	}
}
