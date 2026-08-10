package provider

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/agent-dance/luban/types"
)

// openAIProtocolProvider prefers the cataloged Responses protocol for a known
// OpenAI model on a custom gateway. Chat-only gateways are detected from an
// endpoint-level failure and remembered for the lifetime of the provider.
type openAIProtocolProvider struct {
	mu                sync.RWMutex
	responses         *ResponsesProvider
	chat              *OpenAIProvider
	useChat           bool
	confirmed         bool
	responsesRejected bool
	probeDone         chan struct{}
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
		caps := p.chat.Capabilities()
		// The wrapper, unlike the naked Chat provider, owns a lossless local
		// projection from custom definitions to JSON functions. Advertising the
		// capability here lets ValidateParams admit that wrapper contract; the
		// projected request is validated again by the Chat provider.
		caps.CustomTools = CapabilitySupported
		return caps
	}
	caps := p.responses.Capabilities()
	if caps.CustomTools != CapabilitySupported {
		// A protocol probe may still fall back before sending any model output.
		// The wrapper can represent custom definitions on that Chat path even
		// when this particular Responses profile cannot.
		caps.CustomTools = CapabilitySupported
	}
	return caps
}

func (p *openAIProtocolProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	if err := ValidateParams(p, params); err != nil {
		return nil, err
	}
	forceChatProjection := definitionsHaveCustomTools(params.Tools) && !p.responsesSupportsCustomTools(params)
	for {
		p.mu.Lock()
		if forceChatProjection {
			p.useChat = true
			p.confirmed = false
		}
		if p.useChat {
			responsesRejected := p.responsesRejected
			p.mu.Unlock()
			chatStream, chatErr := p.chat.CreateStream(ctx, projectCustomToolsForChat(params))
			if chatErr != nil && responsesRejected {
				markAttemptedAPIFormats(chatErr, "responses", "chat-completions")
			}
			return chatStream, chatErr
		}
		if p.confirmed {
			p.mu.Unlock()
			stream, err := p.responses.CreateStream(ctx, params)
			if !responsesEndpointUnavailable(err) {
				return stream, err
			}

			// A successful request without tools only confirms that the endpoint
			// accepts the base Responses envelope. A later request can still expose
			// an unsupported tool catalog (for example, custom ApplyPatch on a
			// Chat-compatible gateway). Because CreateStream failed before any model
			// output was delivered, switching protocols here cannot duplicate output.
			p.mu.Lock()
			p.useChat = true
			p.confirmed = false
			p.responsesRejected = true
			p.mu.Unlock()
			if attemptErr := beginNestedTransportAttempt(ctx, err); attemptErr != nil {
				return nil, attemptErr
			}
			chatStream, chatErr := p.chat.CreateStream(ctx, projectCustomToolsForChat(params))
			if chatErr != nil {
				markAttemptedAPIFormats(chatErr, "responses", "chat-completions")
			}
			return chatStream, chatErr
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
			p.responsesRejected = true
		}
		probeDone := p.probeDone
		p.probeDone = nil
		close(probeDone)
		p.mu.Unlock()

		if unavailable {
			if attemptErr := beginNestedTransportAttempt(ctx, err); attemptErr != nil {
				return nil, attemptErr
			}
			chatStream, chatErr := p.chat.CreateStream(ctx, projectCustomToolsForChat(params))
			if chatErr != nil {
				markAttemptedAPIFormats(chatErr, "responses", "chat-completions")
			}
			return chatStream, chatErr
		}
		return stream, err
	}
}

func (p *openAIProtocolProvider) responsesSupportsCustomTools(params Params) bool {
	if !definitionsHaveCustomTools(params.Tools) {
		return true
	}
	profile := p.responses.snapshotRequestProfile()
	model := profile.modelFor(params)
	responsesLite := profile.chatGPTCodexBackend && isOpenAIResponsesLiteModel(model)
	return !responsesLite && responsesCustomToolDefinitionsSupported(profile.semantics, model, params.Tools)
}

// projectCustomToolsForChat converts only the provider-facing definition.
// Tool name and InputSchema remain identical, so a function call is resolved
// by the same local registry entry and passes the same schema/permission/hook
// path. The caller-owned Params and definitions are never mutated.
func projectCustomToolsForChat(params Params) Params {
	if !definitionsHaveCustomTools(params.Tools) {
		return params
	}
	projected := params
	projected.Tools = append([]types.ToolDefinition(nil), params.Tools...)
	for index := range projected.Tools {
		definition := &projected.Tools[index]
		if !definition.IsCustom() {
			continue
		}
		definition.Type = types.ToolDefinitionTypeFunction
		definition.Format = nil
		definition.Strict = definition.InputSchema.RejectsUnknownFields()
	}
	return projected
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
		// This provider exists only for an implicit protocol probe against an
		// OpenAI-compatible custom endpoint. Gateways commonly collapse an
		// unknown /responses route to a generic 400 (for example, "Upstream
		// request failed"), so diagnostic prose cannot be used as capability
		// evidence. The explicit --api responses path never constructs this
		// negotiating provider and therefore remains authoritative.
		return true
	default:
		return false
	}
}
