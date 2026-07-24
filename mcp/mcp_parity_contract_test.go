package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/creachadair/jrpc2/handler"
)

func TestMCPParityContractTSDynamicToolNamesUseDoubleUnderscore(t *testing.T) {
	stdio := startMCPTestClient(t, "my server.name", handler.Map{
		"tools/list": handler.New(func(context.Context, json.RawMessage) (any, error) {
			return map[string]any{"tools": []map[string]any{{
				"name":        "Search Issues!",
				"description": "search",
				"inputSchema": map[string]any{"type": "object"},
			}}}, nil
		}),
	})
	stdioTools, err := stdio.ListTools()
	if err != nil {
		t.Fatalf("stdio ListTools: %v", err)
	}
	if got, want := stdioTools[0].Name(), "mcp__my_server_name__Search_Issues_"; got != want {
		t.Fatalf("stdio model tool name = %q, want %q", got, want)
	}
	if got := stdioTools[0].OriginalName; got != "Search Issues!" {
		t.Fatalf("stdio OriginalName = %q", got)
	}

	sse := startTestSSEClient(t, "remote server", map[string]func(json.RawMessage) (any, error){
		"tools/list": func(json.RawMessage) (any, error) {
			return map[string]any{"tools": []map[string]any{{
				"name":        "lookup value",
				"description": "lookup",
				"inputSchema": map[string]any{"type": "object"},
			}}}, nil
		},
	})
	sseTools, err := sse.ListTools()
	if err != nil {
		t.Fatalf("sse ListTools: %v", err)
	}
	if got, want := sseTools[0].Name(), "mcp__remote_server__lookup_value"; got != want {
		t.Fatalf("sse model tool name = %q, want %q", got, want)
	}
	if got := sseTools[0].OriginalName; got != "lookup value" {
		t.Fatalf("sse OriginalName = %q", got)
	}
}

func TestMCPParityContractPendingTSClientPreservesRawToolAndResourceResults(t *testing.T) {
	t.Skip("TS parity target not implemented in task_01. TS source: ../src/services/mcp/client.ts CallToolResultSchema/ListResourcesResultSchema/ReadResourceResultSchema. Unskip in task_03/task_10/task_11 when the low-level mcp.Client exposes structured tool/resource envelopes without the legacy flattened string helpers.")
}
