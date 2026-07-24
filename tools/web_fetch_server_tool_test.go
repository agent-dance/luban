package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunWebFetchServerTool_NilProviderUnavailable(t *testing.T) {
	_, err := runWebFetchServerTool(context.Background(), nil, WebFetchInput{URL: "https://x"}, nil, nil)
	if !errors.Is(err, ErrWebFetchServerToolUnavailable) {
		t.Fatalf("expected ErrWebFetchServerToolUnavailable, got %v", err)
	}
}

func TestRunWebFetchServerTool_PropagatesError(t *testing.T) {
	want := errors.New("upstream failure")
	provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
		return WebFetchServerToolResponse{}, want
	})
	_, err := runWebFetchServerTool(context.Background(), provider, WebFetchInput{URL: "https://x"}, nil, nil)
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestRunWebFetchServerTool_RedirectMarker(t *testing.T) {
	provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
		return WebFetchServerToolResponse{
			IsRedirect:   true,
			RedirectURL:  "https://example.org/new",
			RedirectCode: 302,
		}, nil
	})
	payload, err := runWebFetchServerTool(context.Background(), provider, WebFetchInput{URL: "https://example.com/old"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(payload.Summary, "REDIRECT DETECTED") || !strings.Contains(payload.Summary, "Status: 302 Found") {
		t.Fatalf("expected REDIRECT marker, got %q", payload.Summary)
	}
	if !strings.Contains(payload.Summary, "https://example.com/old") || !strings.Contains(payload.Summary, "https://example.org/new") {
		t.Fatalf("redirect marker missing original/new URL: %q", payload.Summary)
	}
}

func TestRunWebFetchServerTool_PassesPromptAndDomains(t *testing.T) {
	var captured WebFetchServerToolRequest
	provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
		captured = req
		return WebFetchServerToolResponse{Summary: "ok"}, nil
	})
	if _, err := runWebFetchServerTool(
		context.Background(),
		provider,
		WebFetchInput{URL: "https://example.com", Prompt: "find rate limits"},
		[]string{"example.com"},
		[]string{"spam.com"},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.URL != "https://example.com" || captured.Prompt != "find rate limits" {
		t.Fatalf("URL/prompt not propagated: %+v", captured)
	}
	if len(captured.AllowedDomains) != 1 || captured.AllowedDomains[0] != "example.com" {
		t.Fatalf("allowed domains lost: %+v", captured.AllowedDomains)
	}
	if len(captured.BlockedDomains) != 1 || captured.BlockedDomains[0] != "spam.com" {
		t.Fatalf("blocked domains lost: %+v", captured.BlockedDomains)
	}
	if captured.MaxTokens != WebFetchSummariserMaxTokens {
		t.Fatalf("max tokens not propagated: %d", captured.MaxTokens)
	}
}

func TestRunWebFetchServerTool_ResolvedURLFallback(t *testing.T) {
	provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
		return WebFetchServerToolResponse{Summary: "body"}, nil
	})
	payload, err := runWebFetchServerTool(context.Background(), provider, WebFetchInput{URL: "https://example.com/foo"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.URL != "https://example.com/foo" {
		t.Fatalf("expected URL fallback to input, got %q", payload.URL)
	}
	if payload.Method != string(webExecutionModeProviderNative) {
		t.Fatalf("method should be provider_native, got %q", payload.Method)
	}
}

func TestRunWebFetchServerTool_PrefersResolvedURL(t *testing.T) {
	provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
		return WebFetchServerToolResponse{Summary: "body", ResolvedURL: "https://example.org/canonical"}, nil
	})
	payload, err := runWebFetchServerTool(context.Background(), provider, WebFetchInput{URL: "https://example.com/old"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if payload.URL != "https://example.org/canonical" {
		t.Fatalf("expected resolved URL, got %q", payload.URL)
	}
}

func TestFormatRedirectMarker(t *testing.T) {
	cases := []struct {
		from, to string
		code     int
		status   string
	}{
		{"https://a.com", "https://b.com", 301, "301 Moved Permanently"},
		{"https://a.com", "", 307, "307 Temporary Redirect"},
		{" https://a.com ", " https://b.com ", 302, "302 Found"},
	}
	for _, tc := range cases {
		got := formatRedirectMarker(tc.from, tc.to, tc.code, "extract")
		for _, want := range []string{"REDIRECT DETECTED", "Original URL: " + strings.TrimSpace(tc.from), "Redirect URL: " + strings.TrimSpace(tc.to), "Status: " + tc.status, `- prompt: "extract"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("formatRedirectMarker(%q, %q, %d) missing %q: %q", tc.from, tc.to, tc.code, want, got)
			}
		}
	}
}

func TestSetWebFetchServerToolProvider(t *testing.T) {
	tool := NewWebFetchTool(NewSearchCache())
	if tool.HasWebFetchServerTool() {
		t.Fatal("expected no provider initially")
	}
	provider := WebFetchServerToolFunc(func(ctx context.Context, req WebFetchServerToolRequest) (WebFetchServerToolResponse, error) {
		return WebFetchServerToolResponse{}, nil
	})
	tool.SetWebFetchServerToolProvider(provider, true)
	if !tool.HasWebFetchServerTool() {
		t.Fatal("HasWebFetchServerTool should be true after Set")
	}
	if tool.mode != webExecutionModeProviderNative {
		t.Fatalf("expected provider_native mode, got %q", tool.mode)
	}
}

func TestWebFetchServerToolNameAndBetaHeader(t *testing.T) {
	if WebFetchServerToolName != "web_fetch_20250910" {
		t.Fatalf("server tool name drift: %q", WebFetchServerToolName)
	}
	if webFetchServerToolBetaHeader != "web-fetch-2025-09-10" {
		t.Fatalf("beta header drift: %q", webFetchServerToolBetaHeader)
	}
}
