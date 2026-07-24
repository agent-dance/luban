// Package tools contains WebSearch contract regression tests retained from the
// alignment audit. These cases now describe required production behavior.
package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

// TestWebSearchAlignment_ResultBlockHasCitedText asserts the block schema
// surfaces a CitedText field (matching the Anthropic citation block contract).
// The provider-native output contract retains citation text.
func TestWebSearchAlignment_ResultBlockHasCitedText(t *testing.T) {
	blockType := reflect.TypeOf(WebSearchResultBlock{})
	if _, ok := blockType.FieldByName("CitedText"); !ok {
		fields := make([]string, 0, blockType.NumField())
		for i := 0; i < blockType.NumField(); i++ {
			fields = append(fields, blockType.Field(i).Name)
		}
		t.Fatalf("WebSearchResultBlock must expose CitedText field (citation block parity); got fields=%v", fields)
	}
}

// TestWebSearchAlignment_ResultBlockHasPageAge asserts that PageAge (the
// freshness label TS surfaces) is present on each result block.
func TestWebSearchAlignment_ResultBlockHasPageAge(t *testing.T) {
	blockType := reflect.TypeOf(WebSearchResultBlock{})
	if _, ok := blockType.FieldByName("PageAge"); !ok {
		fields := make([]string, 0, blockType.NumField())
		for i := 0; i < blockType.NumField(); i++ {
			fields = append(fields, blockType.Field(i).Name)
		}
		t.Fatalf("WebSearchResultBlock must expose PageAge field; got fields=%v", fields)
	}
}

// TestWebSearchAlignment_ToolResultBlockEmittedByExecute checks that
// WebSearchTool.Execute emits a typed web_search_tool_result block, not just
// loose `websearch_*=*` text blocks. The TS provider returns the canonical
// server-tool block.
func TestWebSearchAlignment_ToolResultBlockEmittedByExecute(t *testing.T) {
	tool := NewWebSearchTool(nil)
	// Inject a synthetic provider so the test doesn't need network access.
	tool.providerSearch = func(_ context.Context, in WebSearchInput) (webSearchStructuredPayload, error) {
		return webSearchStructuredPayload{
			Query:  in.Query,
			Method: string(webExecutionModeProviderNative),
			Results: []searchResult{
				{Title: "Go", URL: "https://go.dev", Snippet: "The Go Programming Language"},
			},
		}, nil
	}
	tool.mode = webExecutionModeProviderNative

	res, err := tool.Execute(context.Background(), map[string]any{
		"query": "golang",
	})
	if err != nil {
		t.Fatalf("infra error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}

	foundBlock := false
	for _, blk := range res.ContentBlocks {
		if reflect.TypeOf(blk) == reflect.TypeOf(WebSearchToolResultBlock{}) {
			foundBlock = true
			break
		}
		// Some implementations may wrap the block in a TextBlock with a
		// type marker — accept that too.
		if tb, ok := blk.(types.TextBlock); ok && strings.Contains(tb.Text, `"type":"web_search_tool_result"`) {
			foundBlock = true
			break
		}
	}
	if !foundBlock {
		blockTypes := make([]string, 0, len(res.ContentBlocks))
		for _, b := range res.ContentBlocks {
			blockTypes = append(blockTypes, reflect.TypeOf(b).String())
		}
		t.Fatalf("expected a web_search_tool_result block in ContentBlocks (TS server-tool parity); got block types=%v", blockTypes)
	}
}

// TestWebSearchAlignment_BlockedDomainsRejectsScheme verifies that values
// passed to blocked_domains containing a scheme (e.g. "https://example.com")
// are rejected before a provider request is constructed.
func TestWebSearchAlignment_BlockedDomainsRejectsScheme(t *testing.T) {
	tool := NewWebSearchTool(nil)
	tool.providerSearch = func(_ context.Context, in WebSearchInput) (webSearchStructuredPayload, error) {
		return webSearchStructuredPayload{Query: in.Query, Method: string(webExecutionModeProviderNative)}, nil
	}
	tool.mode = webExecutionModeProviderNative

	res, _ := tool.Execute(context.Background(), map[string]any{
		"query":           "anything",
		"blocked_domains": []any{"https://blocked.example"},
	})
	if !res.IsError {
		t.Fatalf("expected validation rejection of blocked_domains entry containing scheme; got success: %s", res.Content)
	}
}

// TestWebSearchAlignment_BlockedDomainsRejectsEmptyEntry asserts that an
// empty-string entry in blocked_domains is a validation error, mirroring the
// TS reference which treats "" as an invalid domain.
func TestWebSearchAlignment_BlockedDomainsRejectsEmptyEntry(t *testing.T) {
	tool := NewWebSearchTool(nil)
	tool.providerSearch = func(_ context.Context, in WebSearchInput) (webSearchStructuredPayload, error) {
		return webSearchStructuredPayload{Query: in.Query, Method: string(webExecutionModeProviderNative)}, nil
	}
	tool.mode = webExecutionModeProviderNative

	res, _ := tool.Execute(context.Background(), map[string]any{
		"query":           "x",
		"blocked_domains": []any{""},
	})
	if !res.IsError {
		t.Fatalf("expected validation rejection of empty-string entry in blocked_domains; got success: %s", res.Content)
	}
}

// TestWebSearchAlignment_MetadataResultsCount asserts that the result
// Metadata surfaces results_count so the renderer can show "N results" in a
// sidebar.
func TestWebSearchAlignment_MetadataResultsCount(t *testing.T) {
	tool := NewWebSearchTool(nil)
	tool.providerSearch = func(_ context.Context, in WebSearchInput) (webSearchStructuredPayload, error) {
		return webSearchStructuredPayload{
			Query:  in.Query,
			Method: string(webExecutionModeProviderNative),
			Results: []searchResult{
				{Title: "A", URL: "https://a.example", Snippet: "1"},
				{Title: "B", URL: "https://b.example", Snippet: "2"},
				{Title: "C", URL: "https://c.example", Snippet: "3"},
			},
		}, nil
	}
	tool.mode = webExecutionModeProviderNative

	res, _ := tool.Execute(context.Background(), map[string]any{"query": "xx"})
	if v := res.Metadata["results_count"]; v != "3" {
		t.Fatalf("metadata.results_count: want %q, got %q (map=%v)",
			"3", v, res.Metadata)
	}
}

// TestWebSearchAlignment_ServerToolMaxUsesIs8 pins the current server-tool
// max_uses default.
func TestWebSearchAlignment_ServerToolMaxUsesIs8(t *testing.T) {
	const want = 8
	if webSearchServerToolMaxUses != want {
		t.Fatalf("webSearchServerToolMaxUses: want %d, got %d", want, webSearchServerToolMaxUses)
	}
}
