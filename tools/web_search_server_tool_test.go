package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunWebSearchServerTool_NilProviderUnavailable(t *testing.T) {
	_, err := runWebSearchServerTool(context.Background(), nil, WebSearchInput{Query: "go"})
	if !errors.Is(err, ErrWebSearchServerToolUnavailable) {
		t.Fatalf("expected ErrWebSearchServerToolUnavailable, got %v", err)
	}
}

func TestRunWebSearchServerTool_PropagatesError(t *testing.T) {
	want := errors.New("network down")
	provider := WebSearchServerToolFunc(func(ctx context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		return WebSearchServerToolResponse{}, want
	})
	_, err := runWebSearchServerTool(context.Background(), provider, WebSearchInput{Query: "go"})
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestRunWebSearchServerTool_NormalisesResults(t *testing.T) {
	var captured WebSearchServerToolRequest
	provider := WebSearchServerToolFunc(func(ctx context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		captured = req
		return WebSearchServerToolResponse{
			Results: []WebSearchResult{
				{Title: "  Go  ", URL: "  https://go.dev  ", Snippet: "  language  "},
				{Title: "Empty URL drops me", URL: ""},
				{Title: "", URL: "https://example.com"},
				{Title: "Long", URL: "https://example.org", Snippet: strings.Repeat("a", WebSearchSnippetCap+200)},
			},
		}, nil
	})
	payload, err := runWebSearchServerTool(context.Background(), provider, WebSearchInput{
		Query:          "go",
		AllowedDomains: []string{"go.dev"},
		BlockedDomains: []string{"spam.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Query != "go" {
		t.Fatalf("query not propagated: %+v", captured)
	}
	if captured.MaxUses != webSearchServerToolMaxUses {
		t.Fatalf("max uses not propagated: %d", captured.MaxUses)
	}
	if len(captured.AllowedDomains) != 1 || captured.AllowedDomains[0] != "go.dev" {
		t.Fatalf("allowed domains not propagated: %+v", captured.AllowedDomains)
	}
	if len(payload.Results) != 3 {
		t.Fatalf("expected 3 results (one URL-empty drop), got %d: %+v", len(payload.Results), payload.Results)
	}
	first := payload.Results[0]
	if first.Title != "Go" || first.URL != "https://go.dev" || first.Snippet != "language" {
		t.Fatalf("trim not applied: %+v", first)
	}
	// Title fallback when missing.
	if payload.Results[1].Title != payload.Results[1].URL {
		t.Fatalf("expected title fallback to URL: %+v", payload.Results[1])
	}
	// Snippet capped.
	if r := []rune(payload.Results[2].Snippet); len(r) > WebSearchSnippetCap+1 {
		t.Fatalf("snippet not capped: rune len=%d", len(r))
	}
	if payload.Method != string(webExecutionModeProviderNative) {
		t.Fatalf("method should be provider_native, got %q", payload.Method)
	}
}

func TestRunWebSearchServerTool_ProgressComesOnlyFromProviderStream(t *testing.T) {
	var progress []WebSearchProgressEvent
	provider := WebSearchServerToolFunc(func(ctx context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		req.OnProgress(WebSearchProgressEvent{Type: "query_update", ToolUseID: "srv_1", Query: "actual query"})
		req.OnProgress(WebSearchProgressEvent{Type: "search_results_received", ToolUseID: "srv_1", Query: "actual query", ResultCount: 1})
		return WebSearchServerToolResponse{
			ResultBlocks: []WebSearchServerToolResult{{ToolUseID: "srv_1", Results: []WebSearchResult{{URL: "https://x"}}}},
		}, nil
	})
	payload, err := runWebSearchServerTool(context.Background(), provider, WebSearchInput{Query: "query"}, func(event WebSearchProgressEvent) {
		progress = append(progress, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Progress) != 0 {
		t.Fatalf("run helper manufactured progress markers: %+v", payload.Progress)
	}
	if len(progress) != 2 || progress[0].Type != "query_update" || progress[1].Type != "search_results_received" || progress[1].ResultCount != 1 || progress[1].Query != "actual query" {
		t.Fatalf("provider progress = %+v", progress)
	}
}

func TestSetWebSearchServerToolProvider(t *testing.T) {
	tool := NewWebSearchTool(NewSearchCache())
	if tool.HasWebSearchServerTool() {
		t.Fatal("expected no provider initially")
	}
	provider := WebSearchServerToolFunc(func(ctx context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		return WebSearchServerToolResponse{}, nil
	})
	tool.SetWebSearchServerToolProvider(provider, true)
	if !tool.HasWebSearchServerTool() {
		t.Fatal("HasWebSearchServerTool should be true after Set")
	}
	if tool.mode != webExecutionModeProviderNative {
		t.Fatalf("expected provider_native mode, got %q", tool.mode)
	}
}

func TestWebSearchServerToolNameAndBetaHeader(t *testing.T) {
	if WebSearchServerToolName != "web_search_20250305" {
		t.Fatalf("server tool name drift: %q", WebSearchServerToolName)
	}
	if webSearchServerToolBetaHeader != "web-search-2025-03-05" {
		t.Fatalf("beta header drift: %q", webSearchServerToolBetaHeader)
	}
	if webSearchServerToolMaxUses != 8 {
		t.Fatalf("max uses drift: %d", webSearchServerToolMaxUses)
	}
}
