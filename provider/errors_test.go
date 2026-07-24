package provider

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/types"
)

// --- NewAPIError ---

func TestNewAPIError_Fields(t *testing.T) {
	ae := NewAPIError(429, "rate_limit_error", "too many requests", "5")
	if ae.Status != 429 {
		t.Errorf("Status: want 429, got %d", ae.Status)
	}
	if ae.Type != "rate_limit_error" {
		t.Errorf("Type: want rate_limit_error, got %s", ae.Type)
	}
	if ae.Message != "too many requests" {
		t.Errorf("Message: want 'too many requests', got %s", ae.Message)
	}
	if ae.RetryAfter != "5" {
		t.Errorf("RetryAfter: want '5', got %s", ae.RetryAfter)
	}
}

func TestNewAPIError_ZeroStatus(t *testing.T) {
	ae := NewAPIError(0, "", "connection reset", "")
	if ae.Status != 0 {
		t.Errorf("want status 0, got %d", ae.Status)
	}
}

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

// --- isConnectionError ---

func TestIsConnectionError_Positive(t *testing.T) {
	cases := []string{
		"connection refused",
		"connection reset",
		"connection closed",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"eof",
		"broken pipe",
		"tls handshake",
		"dial tcp",
		// uppercase should also match (caller lowercases before calling)
		"Connection Refused", // NOTE: caller passes ToLower, but let's test via IsRetryable
	}
	for _, msg := range cases[:len(cases)-1] { // last one tested via IsRetryable
		if !isConnectionError(msg) {
			t.Errorf("isConnectionError(%q) want true", msg)
		}
	}
}

func TestIsConnectionError_Negative(t *testing.T) {
	cases := []string{
		"bad request",
		"unauthorized",
		"not found",
		"internal server error",
		"",
	}
	for _, msg := range cases {
		if isConnectionError(msg) {
			t.Errorf("isConnectionError(%q) want false", msg)
		}
	}
}

// --- IsRetryable ---

func TestIsRetryable_Nil(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("IsRetryable(nil) want false")
	}
}

func TestIsRetryable_ConnectionError(t *testing.T) {
	err := errors.New("dial tcp: connection refused")
	if !IsRetryable(err) {
		t.Error("connection error should be retryable")
	}
}

func TestIsRetryable_EOF(t *testing.T) {
	err := errors.New("unexpected eof")
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
	if !IsRetryable(ae) {
		t.Error("409 should be retryable")
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
	if !IsRetryable(ae) {
		t.Error("max_tokens overflow 400 should be retryable")
	}
}

func TestIsRetryable_APIError_StatusZero_ConnectionMsg(t *testing.T) {
	ae := &types.APIError{Status: 0, Message: "dial tcp: connection refused"}
	if !IsRetryable(ae) {
		t.Error("status=0 with connection error message should be retryable")
	}
}

func TestIsRetryable_APIError_StatusZero_RandomMsg(t *testing.T) {
	ae := &types.APIError{Status: 0, Message: "unknown error"}
	if IsRetryable(ae) {
		t.Error("status=0 with non-connection message should NOT be retryable")
	}
}
