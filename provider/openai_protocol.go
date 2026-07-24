package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/agent-dance/luban/types"
)

// openAIProtocolProvider prefers the cataloged Responses protocol for a known
// OpenAI model on a custom gateway. Chat-only gateways are detected from an
// endpoint-level failure and remembered for the lifetime of the provider.
type openAIProtocolProvider struct {
	mu        sync.RWMutex
	responses *ResponsesProvider
	chat      *OpenAIProvider
	useChat   bool
}

func newOpenAIProtocolProvider(responses *ResponsesProvider, chat *OpenAIProvider) *openAIProtocolProvider {
	return &openAIProtocolProvider{responses: responses, chat: chat}
}

func (p *openAIProtocolProvider) Name() string { return p.responses.Name() }

func (p *openAIProtocolProvider) ModelID() string { return p.responses.ModelID() }

func (p *openAIProtocolProvider) Capabilities() ProviderCapabilities {
	p.mu.RLock()
	useChat := p.useChat
	p.mu.RUnlock()
	if useChat {
		return p.chat.Capabilities()
	}
	return p.responses.Capabilities()
}

func (p *openAIProtocolProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	p.mu.RLock()
	useChat := p.useChat
	p.mu.RUnlock()
	if useChat {
		return p.chat.CreateStream(ctx, params)
	}

	stream, err := p.responses.CreateStream(ctx, params)
	if err == nil || !responsesEndpointUnavailable(err) {
		return stream, err
	}

	p.mu.Lock()
	p.useChat = true
	p.mu.Unlock()
	return p.chat.CreateStream(ctx, params)
}

func responsesEndpointUnavailable(err error) bool {
	var apiErr *types.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		message := strings.ToLower(apiErr.Message)
		return strings.Contains(message, "responses api") &&
			(strings.Contains(message, "not supported") || strings.Contains(message, "unsupported"))
	default:
		return false
	}
}
