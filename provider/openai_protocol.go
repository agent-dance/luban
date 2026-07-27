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
	confirmed bool
	probeDone chan struct{}
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
	if err := ValidateParams(p, params); err != nil {
		return nil, err
	}
	for {
		p.mu.Lock()
		if p.useChat {
			p.mu.Unlock()
			return p.chat.CreateStream(ctx, params)
		}
		if p.confirmed {
			p.mu.Unlock()
			return p.responses.CreateStream(ctx, params)
		}
		if probeDone := p.probeDone; probeDone != nil {
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-probeDone:
				continue
			}
		}
		p.probeDone = make(chan struct{})
		p.mu.Unlock()

		stream, err := p.responses.CreateStream(ctx, params)
		unavailable := responsesEndpointUnavailable(err)

		p.mu.Lock()
		if err == nil {
			p.confirmed = true
		} else if unavailable {
			p.useChat = true
		}
		probeDone := p.probeDone
		p.probeDone = nil
		close(probeDone)
		p.mu.Unlock()

		if unavailable {
			if definitionsHaveCustomTools(params.Tools) {
				return nil, err
			}
			if attemptErr := beginNestedTransportAttempt(ctx, err); attemptErr != nil {
				return nil, attemptErr
			}
			return p.chat.CreateStream(ctx, params)
		}
		return stream, err
	}
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
