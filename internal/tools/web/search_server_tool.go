// Package tools — Anthropic web_search_20250305 server-tool integration.
//
// Mirrors src/tools/WebSearchTool/WebSearchTool.ts which uses the
// Anthropic-hosted web_search server tool to deliver normalised,
// citation-attached results.
package web

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/types"
)

// WebSearchServerToolName matches the SDK BetaWebSearchTool20250305 type.
const WebSearchServerToolName = "web_search_20250305"

// webSearchServerToolMaxUses mirrors the server-tool contract pinned by tests.
const webSearchServerToolMaxUses = 8

// WebSearchServerToolProvider executes a single web_search_20250305 call
// against the Anthropic API and returns normalised search results.
type WebSearchServerToolProvider interface {
	SearchViaServerTool(ctx context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error)
}

// WebSearchServerToolRequest captures the inputs needed to invoke the
// server tool. AllowedDomains/BlockedDomains are passed through unchanged.
type WebSearchServerToolRequest struct {
	Query          string
	AllowedDomains []string
	BlockedDomains []string
	MaxUses        int
	OnProgress     func(WebSearchProgressEvent)
}

// WebSearchServerToolResult is one web_search_tool_result block. Content is
// either Results or ErrorCode, preserving the TS mixed output array.
type WebSearchServerToolResult struct {
	ToolUseID string
	Results   []WebSearchResult
	ErrorCode string
}

// WebSearchServerToolEntry preserves the API block order used by the TS
// output builder: text commentary and result blocks may be interleaved.
type WebSearchServerToolEntry struct {
	Text   string
	Result *WebSearchServerToolResult
}

// WebSearchServerToolResponse is the parsed result set. Citations is
// raw block JSON propagated to downstream consumers without parsing so
// citation metadata round-trips cleanly.
type WebSearchServerToolResponse struct {
	Results      []WebSearchResult
	ResultBlocks []WebSearchServerToolResult
	Entries      []WebSearchServerToolEntry
	Citations    []string
	DurationMs   int64
	Usage        types.Usage

	// websearch-tool-result-error-rendering: when the upstream
	// web_search server returns an error envelope (rate-limited, upstream
	// failure, etc.) instead of an array, the provider sets ErrorCode so
	// the WebSearch executor can render "Web search error: <code>" rather
	// than "no results".
	ErrorCode string
}

// WebSearchServerToolFunc adapts a closure to the provider interface.
type WebSearchServerToolFunc func(ctx context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error)

// SearchViaServerTool implements WebSearchServerToolProvider.
func (f WebSearchServerToolFunc) SearchViaServerTool(ctx context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
	return f(ctx, req)
}

// ErrWebSearchServerToolUnavailable signals that the server tool cannot
// service the request.
var ErrWebSearchServerToolUnavailable = errors.New("web_search server tool is unavailable")

// runWebSearchServerTool dispatches to the provider and normalises the
// response. Returns ErrWebSearchServerToolUnavailable when no provider
// is registered.
func runWebSearchServerTool(
	ctx context.Context,
	provider WebSearchServerToolProvider,
	in WebSearchInput,
	onProgress ...func(WebSearchProgressEvent),
) (webSearchStructuredPayload, error) {
	if provider == nil {
		return webSearchStructuredPayload{}, ErrWebSearchServerToolUnavailable
	}
	progress := func(WebSearchProgressEvent) {}
	if len(onProgress) > 0 && onProgress[0] != nil {
		progress = onProgress[0]
	}
	resp, err := provider.SearchViaServerTool(ctx, WebSearchServerToolRequest{
		Query:          in.Query,
		AllowedDomains: in.AllowedDomains,
		BlockedDomains: in.BlockedDomains,
		MaxUses:        webSearchServerToolMaxUses,
		OnProgress:     progress,
	})
	if err != nil {
		return webSearchStructuredPayload{}, fmt.Errorf("web_search server tool failed: %w", err)
	}
	entries := append([]WebSearchServerToolEntry(nil), resp.Entries...)
	blocks := append([]WebSearchServerToolResult(nil), resp.ResultBlocks...)
	if len(entries) == 0 && len(blocks) == 0 && (resp.Results != nil || strings.TrimSpace(resp.ErrorCode) != "") {
		blocks = append(blocks, WebSearchServerToolResult{
			Results:   append([]WebSearchResult(nil), resp.Results...),
			ErrorCode: strings.TrimSpace(resp.ErrorCode),
		})
	}
	if len(entries) == 0 {
		for i := range blocks {
			block := blocks[i]
			entries = append(entries, WebSearchServerToolEntry{Result: &block})
		}
	}

	output := WebSearchOutput{
		Query:           in.Query,
		Results:         make([]any, 0, len(entries)),
		DurationSeconds: float64(resp.DurationMs) / 1000,
	}
	results := make([]searchResult, 0)
	for _, entry := range entries {
		if strings.TrimSpace(entry.Text) != "" {
			output.Results = append(output.Results, strings.TrimSpace(entry.Text))
			continue
		}
		if entry.Result == nil {
			continue
		}
		block := *entry.Result
		if code := strings.TrimSpace(block.ErrorCode); code != "" {
			output.Results = append(output.Results, FormatWebSearchToolResultError(WebSearchToolResultError{ErrorCode: code}))
			continue
		}
		searchOutput := WebSearchOutputSearchResult{ToolUseID: block.ToolUseID}
		for _, result := range block.Results {
			title := strings.TrimSpace(result.Title)
			url := strings.TrimSpace(result.URL)
			if url == "" {
				continue
			}
			if title == "" {
				title = url
			}
			snippet := capSnippet(strings.TrimSpace(result.Snippet))
			searchOutput.Content = append(searchOutput.Content, WebSearchOutputLink{Title: title, URL: url})
			results = append(results, searchResult{Title: title, URL: url, Snippet: snippet})
		}
		output.Results = append(output.Results, searchOutput)
	}
	usage := resp.Usage
	return webSearchStructuredPayload{
		Results: results,
		Output:  output,
		Usage:   &usage,
	}, nil
}

// SetWebSearchServerToolProvider wires the canonical provider-native executor.
func (w *WebSearchTool) SetWebSearchServerToolProvider(p WebSearchServerToolProvider) {
	w.serverTool = p
}

// HasWebSearchServerTool reports whether a provider has been configured.
func (w *WebSearchTool) HasWebSearchServerTool() bool {
	return w.serverTool != nil
}
