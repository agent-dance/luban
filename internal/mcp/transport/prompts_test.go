package transport

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
)

type promptRawCaller struct {
	t      *testing.T
	calls  []promptRawCall
	result map[string]any
}

type promptRawCall struct {
	method string
	params any
}

func (f *promptRawCaller) CallRaw(_ context.Context, method string, params any, out any) error {
	f.calls = append(f.calls, promptRawCall{method: method, params: params})
	data, err := json.Marshal(f.result[method])
	if err != nil {
		f.t.Fatal(err)
	}
	return json.Unmarshal(data, out)
}

func TestMCPPromptGetSendsNameAndArguments(t *testing.T) {
	raw := &promptRawCaller{
		t: t,
		result: map[string]any{
			"prompts/get": catalog.GetPromptResult{
				Messages: []catalog.PromptMessage{{Role: "user", Content: catalog.PromptContent{Type: "text", Text: "ok"}}},
			},
		},
	}
	client := newProtocolTestClient(t, raw)
	args := map[string]string{"owner": "acme", "repo": "widget", "issue": ""}
	result, err := client.GetPrompt(context.Background(), "review", args)
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages = %#v", result.Messages)
	}
	if len(raw.calls) != 1 || raw.calls[0].method != "prompts/get" {
		t.Fatalf("calls = %#v", raw.calls)
	}
	params := raw.calls[0].params.(map[string]any)
	if params["name"] != "review" {
		t.Fatalf("name param = %#v", params["name"])
	}
	gotArgs := params["arguments"].(map[string]any)
	wantArgs := map[string]any{"owner": "acme", "repo": "widget", "issue": ""}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("arguments = %#v, want %#v", gotArgs, wantArgs)
	}
}
