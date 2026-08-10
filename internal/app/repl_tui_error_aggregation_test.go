package app

import (
	"net/http"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestRuntimeErrorCauseAlreadyRenderedMatchesOnlyTerminalCause(t *testing.T) {
	rendered := &types.APIError{Type: "api_error", Status: 400, Message: "provider failure"}
	unrelated := &types.APIError{Type: "api_error", Status: 429, Message: "other failure"}
	terminal := i18n.WrapError(i18n.KeyLoopQueryAPICallFailed, rendered)

	if !runtimeErrorCauseAlreadyRendered(terminal, []*types.APIError{rendered}) {
		t.Fatal("wrapped terminal provider error was not deduplicated")
	}
	if runtimeErrorCauseAlreadyRendered(terminal, []*types.APIError{unrelated}) {
		t.Fatal("unrelated earlier runtime error suppressed terminal receipt")
	}
	if runtimeErrorCauseAlreadyRendered(nil, []*types.APIError{rendered}) {
		t.Fatal("nil terminal error was treated as rendered")
	}
}

func TestRuntimeErrorCauseAlreadyRenderedDeduplicatesCopiedWrappedHTTPFailures(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusNotFound,
		http.StatusTooManyRequests,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			rendered := &types.APIError{
				Type: "api_error", Code: "gateway_rejected", Message: "provider failure", Status: status,
				Provider: "openai", APIFormat: "responses", Endpoint: "https://gateway.example/…/responses",
				RequestID: "req-copied-17", SuggestedAPIFormat: "chat-completions",
			}
			copied := *rendered
			terminal := i18n.WrapError(i18n.KeyLoopQueryAPICallFailed, &copied)
			if !runtimeErrorCauseAlreadyRendered(terminal, []*types.APIError{rendered}) {
				t.Fatal("copied and wrapped provider failure was rendered twice")
			}

			differentRequest := copied
			differentRequest.RequestID = "req-distinct-18"
			if runtimeErrorCauseAlreadyRendered(
				i18n.WrapError(i18n.KeyLoopQueryAPICallFailed, &differentRequest),
				[]*types.APIError{rendered},
			) {
				t.Fatal("distinct provider request was incorrectly deduplicated")
			}
		})
	}
}
