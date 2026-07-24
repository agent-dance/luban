package provider

import (
	"context"
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// DisconnectedProvider is a placeholder provider that lets the UI boot before
// credentials are configured. Query attempts fail with a guided message until
// the user connects a real provider.
type DisconnectedProvider struct {
	name    string
	model   string
	message string
}

func NewDisconnectedProvider(name, model, message string) *DisconnectedProvider {
	if name == "" {
		name = "anthropic"
	}
	if model == "" {
		model = CatalogDefaultModel(name, "claude-sonnet-5")
	}
	if message == "" {
		message = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderDisconnected)
	}
	return &DisconnectedProvider{name: name, model: model, message: message}
}

func (p *DisconnectedProvider) Name() string {
	return p.name
}

func (p *DisconnectedProvider) ModelID() string {
	return p.model
}

func (p *DisconnectedProvider) CreateStream(_ context.Context, _ Params) (<-chan types.StreamEvent, error) {
	return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderDisconnectedAction, p.message, p.name, p.name, p.name))
}
