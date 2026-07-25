package transport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

func TestClientCatalogListsCollectEveryPage(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		firstPage  any
		secondPage any
		list       func(context.Context, *Client) ([]string, error)
	}{
		{
			name:   "tools",
			method: "tools/list",
			firstPage: map[string]any{
				"tools":      []map[string]any{{"name": "tool-a"}},
				"nextCursor": "tools-page-2",
			},
			secondPage: map[string]any{"tools": []map[string]any{{"name": "tool-b"}}},
			list: func(ctx context.Context, client *Client) ([]string, error) {
				result, err := client.ListTools(ctx)
				if err != nil {
					return nil, err
				}
				names := make([]string, 0, len(result.Tools))
				for _, tool := range result.Tools {
					names = append(names, tool.Name)
				}
				return names, nil
			},
		},
		{
			name:   "resources",
			method: "resources/list",
			firstPage: map[string]any{
				"resources":  []map[string]any{{"uri": "memo://a", "name": "resource-a"}},
				"nextCursor": "resources-page-2",
			},
			secondPage: map[string]any{"resources": []map[string]any{{"uri": "memo://b", "name": "resource-b"}}},
			list: func(ctx context.Context, client *Client) ([]string, error) {
				result, err := client.ListResourcesResult(ctx)
				if err != nil {
					return nil, err
				}
				uris := make([]string, 0, len(result.Resources))
				for _, resource := range result.Resources {
					uris = append(uris, resource.URI)
				}
				return uris, nil
			},
		},
		{
			name:   "prompts",
			method: "prompts/list",
			firstPage: map[string]any{
				"prompts":    []map[string]any{{"name": "prompt-a"}},
				"nextCursor": "prompts-page-2",
			},
			secondPage: map[string]any{"prompts": []map[string]any{{"name": "prompt-b"}}},
			list: func(ctx context.Context, client *Client) ([]string, error) {
				result, err := client.ListPrompts(ctx)
				if err != nil {
					return nil, err
				}
				names := make([]string, 0, len(result.Prompts))
				for _, prompt := range result.Prompts {
					names = append(names, prompt.Name)
				}
				return names, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, transport := startInitializedClient(t, clientOptions{})
			type listResult struct {
				items []string
				err   error
			}
			resultCh := make(chan listResult, 1)
			go func() {
				items, err := tt.list(context.Background(), client)
				resultCh <- listResult{items: items, err: err}
			}()

			firstRequest := transport.nextSent(t)
			assertCatalogRequestCursor(t, firstRequest, tt.method, "")
			transport.push(t, mustResultMessage(t, firstRequest.ID, tt.firstPage))

			secondRequest := transport.nextSent(t)
			assertCatalogRequestCursor(t, secondRequest, tt.method, tt.method[:strings.IndexByte(tt.method, '/')]+"-page-2")
			transport.push(t, mustResultMessage(t, secondRequest.ID, tt.secondPage))

			result := <-resultCh
			if result.err != nil {
				t.Fatalf("%s: %v", tt.method, result.err)
			}
			if len(result.items) != 2 || !strings.HasSuffix(result.items[0], "a") || !strings.HasSuffix(result.items[1], "b") {
				t.Fatalf("%s items = %v", tt.method, result.items)
			}
		})
	}
}

func TestClientCatalogListRejectsRepeatedCursor(t *testing.T) {
	client, transport := startInitializedClient(t, clientOptions{})
	type callResult struct {
		result *catalog.ListToolsResult
		err    error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		result, err := client.ListTools(context.Background())
		resultCh <- callResult{result: result, err: err}
	}()

	firstRequest := transport.nextSent(t)
	assertCatalogRequestCursor(t, firstRequest, "tools/list", "")
	transport.push(t, mustResultMessage(t, firstRequest.ID, map[string]any{
		"tools":      []map[string]any{{"name": "tool-a"}},
		"nextCursor": "repeated-cursor",
	}))
	secondRequest := transport.nextSent(t)
	assertCatalogRequestCursor(t, secondRequest, "tools/list", "repeated-cursor")
	transport.push(t, mustResultMessage(t, secondRequest.ID, map[string]any{
		"tools":      []map[string]any{{"name": "tool-b"}},
		"nextCursor": "repeated-cursor",
	}))

	result := <-resultCh
	if result.result != nil || result.err == nil {
		t.Fatalf("ListTools result=%#v err=%v, want repeated-cursor failure", result.result, result.err)
	}
	if !strings.Contains(result.err.Error(), "tools/list") || !strings.Contains(result.err.Error(), "repeated-cursor") {
		t.Fatalf("ListTools error did not preserve protocol values: %v", result.err)
	}
}

func assertCatalogRequestCursor(t *testing.T, request protocol.JSONRPCMessage, method, wantCursor string) {
	t.Helper()
	if request.Method != method {
		t.Fatalf("request method = %q, want %q", request.Method, method)
	}
	if wantCursor == "" {
		if len(request.Params) != 0 {
			t.Fatalf("first %s params = %s, want omitted", method, request.Params)
		}
		return
	}
	var params map[string]string
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatalf("decode %s params: %v", method, err)
	}
	if params["cursor"] != wantCursor || len(params) != 1 {
		t.Fatalf("%s params = %#v, want cursor %q", method, params, wantCursor)
	}
}
