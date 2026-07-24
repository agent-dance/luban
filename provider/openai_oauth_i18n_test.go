package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func requireOpenAIOAuthSemanticKey(t *testing.T, err error, want i18n.Key) i18n.SemanticErrorInfo {
	t.Helper()
	if err == nil {
		t.Fatalf("expected semantic error %q", want)
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != want {
		t.Fatalf("DescribeSemanticError(%v) = %#v, %v; want key %q", err, info, ok, want)
	}
	return info
}

func TestParseOpenAIChatGPTClaimsUsesSemanticErrors(t *testing.T) {
	tests := []struct {
		name string
		jwt  string
		key  i18n.Key
	}{
		{"empty", "  ", i18n.KeyProviderOpenAIOAuthIDTokenEmpty},
		{"format", "header..signature", i18n.KeyProviderOpenAIOAuthIDTokenFormatInvalid},
		{"decode", "header.!.signature", i18n.KeyProviderOpenAIOAuthIDTokenPayloadDecodeFailed},
		{"parse", "header." + base64.RawURLEncoding.EncodeToString([]byte("{")) + ".signature", i18n.KeyProviderOpenAIOAuthIDTokenPayloadParseFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseOpenAIChatGPTClaims(tt.jwt)
			info := requireOpenAIOAuthSemanticKey(t, err, tt.key)
			if tt.name == "decode" {
				var corrupt base64.CorruptInputError
				if !errors.As(err, &corrupt) || !info.IncludeCause {
					t.Fatalf("decode error did not preserve its typed cause: %#v", info)
				}
			}
			if tt.name == "parse" {
				var syntax *json.SyntaxError
				if !errors.As(err, &syntax) || !info.IncludeCause {
					t.Fatalf("parse error did not preserve its typed cause: %#v", info)
				}
			}
		})
	}
}

type openAIOAuthRoundTripFunc func(*http.Request) (*http.Response, error)

func (f openAIOAuthRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func configureOpenAIOAuthExchangeTest(t *testing.T, tokenURL string, client *http.Client) {
	t.Helper()
	originalURL := openAIOAuthTokenURL
	originalClient := openAIOAuthHTTPClient
	openAIOAuthTokenURL = tokenURL
	openAIOAuthHTTPClient = client
	t.Cleanup(func() {
		openAIOAuthTokenURL = originalURL
		openAIOAuthHTTPClient = originalClient
	})
}

func TestExchangeOpenAIIDTokenForAPIKeyUsesSemanticErrors(t *testing.T) {
	t.Run("build request", func(t *testing.T) {
		configureOpenAIOAuthExchangeTest(t, "://invalid-oauth-url", http.DefaultClient)
		_, err := exchangeOpenAIIDTokenForAPIKey(context.Background(), "raw-id-token")
		info := requireOpenAIOAuthSemanticKey(t, err, i18n.KeyProviderOpenAIOAuthAPIKeyExchangeRequestBuildFailed)
		var urlErr *url.Error
		if !errors.As(err, &urlErr) || !info.IncludeCause {
			t.Fatalf("request-build error did not preserve its typed cause: %#v", info)
		}
	})

	t.Run("request", func(t *testing.T) {
		cause := errors.New("raw-transport-cause")
		client := &http.Client{Transport: openAIOAuthRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, cause
		})}
		configureOpenAIOAuthExchangeTest(t, "https://oauth.invalid/token", client)
		_, err := exchangeOpenAIIDTokenForAPIKey(context.Background(), "raw-id-token")
		info := requireOpenAIOAuthSemanticKey(t, err, i18n.KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed)
		if !errors.Is(err, cause) || !info.IncludeCause {
			t.Fatalf("request error did not preserve errors.Is: %#v", info)
		}
	})

	t.Run("status and raw body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":"raw-remote-body"}`)
		}))
		t.Cleanup(server.Close)
		configureOpenAIOAuthExchangeTest(t, server.URL, server.Client())
		_, err := exchangeOpenAIIDTokenForAPIKey(context.Background(), "raw-id-token")
		info := requireOpenAIOAuthSemanticKey(t, err, i18n.KeyProviderOpenAIOAuthAPIKeyExchangeRejected)
		if len(info.Args) != 2 || info.Args[0] != http.StatusTooManyRequests || !strings.Contains(info.Args[1].(string), "raw-remote-body") {
			t.Fatalf("status error lost raw status/body: %#v", info.Args)
		}
	})

	t.Run("decode response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "{")
		}))
		t.Cleanup(server.Close)
		configureOpenAIOAuthExchangeTest(t, server.URL, server.Client())
		_, err := exchangeOpenAIIDTokenForAPIKey(context.Background(), "raw-id-token")
		info := requireOpenAIOAuthSemanticKey(t, err, i18n.KeyProviderOpenAIOAuthAPIKeyExchangeResponseDecodeFailed)
		if !errors.Is(err, io.ErrUnexpectedEOF) || !info.IncludeCause {
			t.Fatalf("response-decode error did not preserve errors.Is: %#v", info)
		}
	})

	t.Run("missing access_token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"token_type":"bearer"}`)
		}))
		t.Cleanup(server.Close)
		configureOpenAIOAuthExchangeTest(t, server.URL, server.Client())
		_, err := exchangeOpenAIIDTokenForAPIKey(context.Background(), "raw-id-token")
		requireOpenAIOAuthSemanticKey(t, err, i18n.KeyProviderOpenAIOAuthAPIKeyExchangeMissingAccessToken)
	})
}
