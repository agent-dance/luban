package web

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestWebFetchMetadataAndSchema(t *testing.T) {
	tool := NewWebFetchTool(nil)
	t.Cleanup(tool.FetchCache().Stop)
	metadata := tool.ToolMetadata(nil)
	if tool.Name() != "WebFetch" || !metadata.ConcurrencySafe || !metadata.ReadOnly {
		t.Fatalf("unexpected WebFetch metadata: name=%q metadata=%+v", tool.Name(), metadata)
	}
	schema := tool.Schema()
	if len(schema.Required) != 2 || schema.Required[0] != "url" || schema.Required[1] != "prompt" {
		t.Fatalf("WebFetch required fields = %v", schema.Required)
	}
}

func TestWebFetchRejectsMissingRequiredInput(t *testing.T) {
	tool := NewWebFetchTool(nil)
	t.Cleanup(tool.FetchCache().Stop)
	for _, input := range []map[string]any{
		{"prompt": "summarize"},
		{"url": "https://example.com"},
	} {
		result, err := tool.Execute(context.Background(), input)
		if err != nil || !result.IsError {
			t.Fatalf("input %v: result=%#v err=%v", input, result, err)
		}
	}
}

func TestWebFetchParityDomainPermissionSuggestion(t *testing.T) {
	tool := NewWebFetchTool(nil)
	t.Cleanup(tool.FetchCache().Stop)
	decision, err := tool.CheckPermissions(context.Background(), map[string]any{
		"url": "https://example.invalid/reference", "prompt": "Find the parity contract",
	}, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Behavior != types.PermissionBehaviorAsk || !strings.Contains(decision.Message, "WebFetch requires permission") {
		t.Fatalf("permission decision = %+v", decision)
	}
	if len(decision.Suggestions) != 1 {
		t.Fatalf("permission suggestions = %+v", decision.Suggestions)
	}
	suggestion := decision.Suggestions[0]
	if suggestion.Type != types.PermissionUpdateAddRules || suggestion.Destination != types.PermissionDestinationLocalSettings ||
		suggestion.Behavior != types.PermissionBehaviorAllow || len(suggestion.Rules) != 1 ||
		suggestion.Rules[0].ToolName != "WebFetch" || suggestion.Rules[0].RuleContent != "domain:example.invalid" {
		t.Fatalf("permission suggestion = %+v", suggestion)
	}
}

func TestIsBinaryContentType(t *testing.T) {
	for _, contentType := range []string{
		"application/pdf",
		"application/pdf; charset=utf-8",
		"application/zip",
		"image/png",
		"video/mp4",
	} {
		if !isBinaryContentType(contentType) {
			t.Errorf("isBinaryContentType(%q) = false, want true", contentType)
		}
	}
	for _, contentType := range []string{"text/html", "text/markdown", "application/json"} {
		if isBinaryContentType(contentType) {
			t.Errorf("isBinaryContentType(%q) = true, want false", contentType)
		}
	}
}

func TestWebSearchMetadataAndSchema(t *testing.T) {
	tool := NewWebSearchTool()
	metadata := tool.ToolMetadata(nil)
	if tool.Name() != "WebSearch" || !metadata.ConcurrencySafe || !metadata.ReadOnly {
		t.Fatalf("unexpected WebSearch metadata: name=%q metadata=%+v", tool.Name(), metadata)
	}
	if got := tool.Schema().Required; len(got) != 1 || got[0] != "query" {
		t.Fatalf("WebSearch required fields = %v", got)
	}
}

func TestWebSearchRejectsMissingQueryBeforeProviderDispatch(t *testing.T) {
	result, err := NewWebSearchTool().Execute(context.Background(), map[string]any{})
	if err != nil || !result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMatchDomain(t *testing.T) {
	for _, test := range []struct {
		host, pattern string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"EXAMPLE.COM", "example.com", true},
		{"sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true},
		{"notexample.com", "*.example.com", false},
	} {
		if got := matchDomain(test.host, test.pattern); got != test.want {
			t.Errorf("matchDomain(%q, %q) = %v, want %v", test.host, test.pattern, got, test.want)
		}
	}
}

func TestCheckDomainAllowed(t *testing.T) {
	for _, test := range []struct {
		name             string
		rawURL           string
		allowed, blocked []string
		wantErr          bool
	}{
		{name: "unrestricted", rawURL: "https://example.com", wantErr: false},
		{name: "allowed", rawURL: "https://docs.example.com", allowed: []string{"*.example.com"}, wantErr: false},
		{name: "not allowed", rawURL: "https://other.test", allowed: []string{"example.com"}, wantErr: true},
		{name: "blocked wins", rawURL: "https://example.com", allowed: []string{"example.com"}, blocked: []string{"example.com"}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := checkDomainAllowed(test.rawURL, test.allowed, test.blocked)
			if (err != nil) != test.wantErr {
				t.Fatalf("checkDomainAllowed() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestWebFetchDomainPolicyRunsBeforeNetwork(t *testing.T) {
	tool := NewWebFetchTool(nil)
	t.Cleanup(tool.FetchCache().Stop)
	tool.DisallowedDomains = []string{"blocked.example"}
	result, err := tool.Execute(context.Background(), map[string]any{
		"url": "https://blocked.example/page", "prompt": "title",
	})
	if err != nil || !result.IsError || !strings.Contains(strings.ToLower(result.Content), "blocked") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
