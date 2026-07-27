package provider

import (
	"errors"
	"io"
	"net"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestAsAPIError_UnwrapsFallbackTriggeredError(t *testing.T) {
	cause := &types.APIError{Status: 429, Type: "rate_limit_error", Message: "rate limited"}
	err := &FallbackTriggeredError{
		OriginalModel: "gpt-5.5",
		FallbackModel: "gpt-5.5-mini",
		Cause:         cause,
	}

	got, ok := AsAPIError(err)
	if !ok {
		t.Fatal("AsAPIError should find wrapped APIError")
	}
	if got != cause {
		t.Fatalf("AsAPIError = %#v, want original cause %#v", got, cause)
	}
	var stdlibGot *types.APIError
	if !errors.As(err, &stdlibGot) {
		t.Fatal("errors.As should find wrapped APIError")
	}
}

// --- IsRetryable ---

func TestIsRetryable_Nil(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("IsRetryable(nil) want false")
	}
}

func TestIsRetryable_ConnectionError(t *testing.T) {
	err := &net.DNSError{Err: "temporary resolver failure", IsTemporary: true}
	if !IsRetryable(err) {
		t.Error("connection error should be retryable")
	}
}

func TestIsRetryable_EOF(t *testing.T) {
	err := io.ErrUnexpectedEOF
	if !IsRetryable(err) {
		t.Error("eof should be retryable")
	}
}

func TestIsRetryable_GenericNonConnection(t *testing.T) {
	err := errors.New("some random error")
	if IsRetryable(err) {
		t.Error("generic error should not be retryable")
	}
}

func TestIsRetryable_APIError_429(t *testing.T) {
	ae := &types.APIError{Status: 429}
	if !IsRetryable(ae) {
		t.Error("429 should be retryable")
	}
}

func TestIsRetryable_APIError_408(t *testing.T) {
	ae := &types.APIError{Status: 408}
	if !IsRetryable(ae) {
		t.Error("408 should be retryable")
	}
}

func TestIsRetryable_APIError_409(t *testing.T) {
	ae := &types.APIError{Status: 409}
	if IsRetryable(ae) {
		t.Error("untyped 409 should fail fast")
	}
}

func TestIsRetryable_APIError_529(t *testing.T) {
	ae := &types.APIError{Status: 529}
	if !IsRetryable(ae) {
		t.Error("529 should be retryable")
	}
}

func TestIsRetryable_APIError_401(t *testing.T) {
	ae := &types.APIError{Status: 401}
	if IsRetryable(ae) {
		t.Error("401 should NOT be retryable (no auth refresh mechanism; fail fast)")
	}
}

func TestIsRetryable_APIError_500(t *testing.T) {
	ae := &types.APIError{Status: 500}
	if !IsRetryable(ae) {
		t.Error("500 should be retryable")
	}
}

func TestIsRetryable_APIError_503(t *testing.T) {
	ae := &types.APIError{Status: 503}
	if !IsRetryable(ae) {
		t.Error("503 should be retryable")
	}
}

func TestIsRetryable_APIError_403(t *testing.T) {
	ae := &types.APIError{Status: 403}
	if IsRetryable(ae) {
		t.Error("403 should NOT be retryable")
	}
}

func TestIsRetryable_APIError_400_Plain(t *testing.T) {
	ae := &types.APIError{Status: 400, Message: "bad request"}
	if IsRetryable(ae) {
		t.Error("plain 400 should NOT be retryable")
	}
}

func TestIsRetryable_APIError_400_MaxTokens(t *testing.T) {
	ae := &types.APIError{Status: 400, Message: "max_tokens exceed context length"}
	if IsRetryable(ae) {
		t.Error("max_tokens overflow requires request recovery, not blind retry")
	}
}

func TestIsRetryable_PermanentProblemTypesOverrideServerStatus(t *testing.T) {
	cases := []*types.APIError{
		{Status: 500, Type: "context_length_exceeded", Message: "context window exceeded"},
		{Status: 500, Type: "insufficient_quota", Message: "billing quota exhausted"},
		{Status: 500, Type: "model_not_found", Message: "unknown model"},
		{Status: 500, Type: "authentication_error", Message: "invalid credential"},
	}
	for _, apiErr := range cases {
		if IsRetryable(apiErr) {
			t.Errorf("permanent problem retried: %+v", apiErr)
		}
	}
}

func TestIsRetryable_APIError_StatusZero_ConnectionMessageIsNotEvidence(t *testing.T) {
	ae := &types.APIError{Status: 0, Message: "dial tcp: connection refused"}
	if IsRetryable(ae) {
		t.Error("provider-controlled message must not authorize retry")
	}
}

func TestIsRetryable_APIError_StatusZero_RandomMsg(t *testing.T) {
	ae := &types.APIError{Status: 0, Message: "unknown error"}
	if IsRetryable(ae) {
		t.Error("status=0 with non-connection message should NOT be retryable")
	}
}

func TestClassifyAttemptErrorStructuredContract(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStage  types.ProviderErrorStage
		wantClass  types.ProviderErrorClass
		wantReplay types.ProviderReplaySafety
		wantRetry  bool
	}{
		{
			name:      "400 context is terminal",
			err:       &types.APIError{Status: 400, Code: "context_length_exceeded", Type: "invalid_request_error"},
			wantStage: types.ProviderErrorStageHeaders, wantClass: types.ProviderErrorClassContext,
			wantReplay: types.ProviderReplayUnsafe,
		},
		{
			name:      "429 throttle",
			err:       &types.APIError{Status: 429, Code: "rate_limit_exceeded"},
			wantStage: types.ProviderErrorStageHeaders, wantClass: types.ProviderErrorClassThrottle,
			wantReplay: types.ProviderReplaySafe, wantRetry: true,
		},
		{
			name:      "503 overload",
			err:       &types.APIError{Status: 503, Type: "server_error"},
			wantStage: types.ProviderErrorStageHeaders, wantClass: types.ProviderErrorClassOverload,
			wantReplay: types.ProviderReplaySafe, wantRetry: true,
		},
		{
			name:      "generic api error is permanent",
			err:       &types.APIError{Type: "api_error", Message: "upstream disconnected"},
			wantStage: types.ProviderErrorStageConnect, wantClass: types.ProviderErrorClassPermanent,
			wantReplay: types.ProviderReplayUnsafe,
		},
		{
			name:      "committed transport is unsafe unless explicitly proven",
			err:       &types.APIError{Type: "stream_interrupted", Stage: types.ProviderErrorStageCommitted},
			wantStage: types.ProviderErrorStageCommitted, wantClass: types.ProviderErrorClassTransport,
			wantReplay: types.ProviderReplayUnsafe,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyAttemptError(test.err)
			if got.Stage != test.wantStage || got.Class != test.wantClass || got.ReplaySafety != test.wantReplay {
				t.Fatalf("contract = %+v, want stage=%q class=%q replay=%q", got, test.wantStage, test.wantClass, test.wantReplay)
			}
			if got.Retryable() != test.wantRetry {
				t.Fatalf("Retryable() = %t, want %t", got.Retryable(), test.wantRetry)
			}
		})
	}
}
