package provider

import (
	"context"
	"strings"

	"github.com/agent-dance/luban/types"
)

type localProviderRequestIDContextKey struct{}
type providerFailureDiagnosticContextKey struct{}

// WithLocalProviderRequestID correlates the runtime attempt with provider and
// presentation diagnostics. The identifier is generated locally and carries
// no provider-controlled content.
func WithLocalProviderRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, localProviderRequestIDContextKey{}, strings.TrimSpace(requestID))
}

func localProviderRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(localProviderRequestIDContextKey{}).(string)
	return strings.TrimSpace(value)
}

func withProviderFailureDiagnostic(ctx context.Context, diagnostic *types.ProviderFailureDiagnostic) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, providerFailureDiagnosticContextKey{}, diagnostic.Clone())
}

func providerFailureDiagnosticFromContext(ctx context.Context) *types.ProviderFailureDiagnostic {
	if ctx == nil {
		return nil
	}
	diagnostic, _ := ctx.Value(providerFailureDiagnosticContextKey{}).(*types.ProviderFailureDiagnostic)
	return diagnostic.Clone()
}

func baseResponsesFailureDiagnostic(
	ctx context.Context,
	providerName, model, transport, endpoint string,
	upstreamRequestID string,
) *types.ProviderFailureDiagnostic {
	return &types.ProviderFailureDiagnostic{
		SchemaVersion:     types.ProviderFailureDiagnosticSchema,
		LocalRequestID:    safeProviderProtocolIdentifier(localProviderRequestID(ctx)),
		UpstreamRequestID: safeProviderProtocolIdentifier(upstreamRequestID),
		Provider:          CanonicalProviderName(providerName),
		Model:             strings.TrimSpace(model),
		APIFormat:         "responses",
		Transport:         transport,
		Endpoint:          redactProviderEndpoint(endpoint, "responses"),
		FailurePoint:      types.ProviderFailureUnknown,
	}
}

func safeProviderProtocolIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("._:-/", char) {
			continue
		}
		return ""
	}
	return value
}

func safeResponsesItemType(value string) string {
	switch value {
	case "message", "function_call", "custom_tool_call", "reasoning":
		return value
	default:
		return ""
	}
}

func safeResponsesItemStatus(value string) string {
	switch value {
	case "in_progress", "completed", "incomplete", "failed":
		return value
	default:
		return ""
	}
}

func safeResponsesIncompleteReason(value string) string {
	switch value {
	case "max_output_tokens", "max_tokens", "content_filter", "safety", "policy":
		return value
	case "":
		return "missing"
	default:
		return "unknown"
	}
}

func safeResponsesWireEvent(value string) string {
	switch value {
	case "response.created", "response.in_progress", "response.output_item.added",
		"response.content_part.added", "response.output_text.delta", "response.content_part.delta",
		"response.function_call_arguments.delta", "response.custom_tool_call_input.delta",
		"response.reasoning.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta",
		"response.function_call_arguments.done", "response.custom_tool_call_input.done",
		"response.output_item.done", "response.content_part.done", "response.completed",
		"response.incomplete", "response.failed", "error":
		return value
	default:
		return "unknown"
	}
}

func applyFailureDiagnosticToAPIError(apiErr *types.APIError, diagnostic *types.ProviderFailureDiagnostic) *types.APIError {
	if apiErr == nil {
		return nil
	}
	if diagnostic == nil {
		return apiErr
	}
	apiErr.FailureDiagnostic = diagnostic.Clone()
	apiErr.Provider = diagnostic.Provider
	apiErr.APIFormat = diagnostic.APIFormat
	apiErr.Endpoint = diagnostic.Endpoint
	apiErr.RequestID = diagnostic.UpstreamRequestID
	contract := ClassifyAttemptError(apiErr)
	diagnostic.Stage = contract.Stage
	diagnostic.Class = contract.Class
	diagnostic.ReplaySafety = contract.ReplaySafety
	apiErr.FailureDiagnostic.Stage = contract.Stage
	apiErr.FailureDiagnostic.Class = contract.Class
	apiErr.FailureDiagnostic.ReplaySafety = contract.ReplaySafety
	return apiErr
}
