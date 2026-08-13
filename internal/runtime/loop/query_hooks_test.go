package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type queryHookOrderProvider struct {
	path  string
	calls int
}

func (p *queryHookOrderProvider) Name() string    { return "query-hook-order" }
func (p *queryHookOrderProvider) ModelID() string { return "query-hook-order-model" }
func (p *queryHookOrderProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.calls++
	f, err := os.OpenFile(p.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString("provider\n"); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return makeStreamChan(parityTextEvents("answer")...), nil
}

func TestQueryHooksBracketProviderWithCorrelatedEvidence(t *testing.T) {
	dir := t.TempDir()
	orderPath := filepath.Join(dir, "order")
	preInputPath := filepath.Join(dir, "pre.json")
	postInputPath := filepath.Join(dir, "post.json")
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookPreQuery, Command: testHookCaptureAndAppendCommand(preInputPath, orderPath, "pre"), Timeout: 5},
		{Type: hooks.HookPostQuery, Command: testHookCaptureAndAppendCommand(postInputPath, orderPath, "post"), Timeout: 5},
	})
	prov := &queryHookOrderProvider{path: orderPath}
	query := New(prov, registry.New(), Config{
		MaxTurns: 2, MaxTokens: 1024, HookRunner: runner,
		SessionID: "session-query-hooks", ProjectRoot: dir, AgentID: "agent-reviewer", AgentType: "reviewer", CWD: filepath.Join(dir, "nested"),
	})

	var summaries []stream.Event
	if err := query.Run(context.Background(), "hello", func(event stream.Event) {
		if event.Type == stream.EventHookSummary {
			summaries = append(summaries, event)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	order, err := os.ReadFile(orderPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(order), "pre\nprovider\npost\n"; got != want {
		t.Fatalf("execution order = %q, want %q", got, want)
	}
	if len(summaries) != 2 {
		t.Fatalf("query hook summaries = %d, want 2: %#v", len(summaries), summaries)
	}
	for index, wantType := range []hooks.HookType{hooks.HookPreQuery, hooks.HookPostQuery} {
		event := summaries[index]
		if event.HookSummary == nil || event.HookSummary.HookName != string(wantType) || event.HookSummary.HookExecutionID == "" {
			t.Fatalf("summary %d = %#v", index, event)
		}
		if !strings.HasPrefix(event.TurnID, "session-query-hooks:query-") || !strings.HasSuffix(event.TurnID, ":turn-1") || event.WorkUnitID != "agent-reviewer" || event.ActorID != "agent-reviewer" || event.ActorType != "reviewer" {
			t.Fatalf("summary %d lost causal identity: %#v", index, event)
		}
		input, ok := event.HookSummary.Metadata["hook_input"].(hooks.HookInput)
		if !ok || input.SessionID != "session-query-hooks" || input.TurnID != event.TurnID || input.WorkUnitID != event.WorkUnitID || input.AgentID != event.ActorID {
			t.Fatalf("summary %d input causality = %#v", index, event.HookSummary.Metadata["hook_input"])
		}
	}
	for _, path := range []string{preInputPath, postInputPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var input map[string]any
		if err := json.Unmarshal(data, &input); err != nil {
			t.Fatal(err)
		}
		if input["project_root"] != dir {
			t.Fatalf("%s project_root = %#v, want %q", filepath.Base(path), input["project_root"], dir)
		}
	}
}

func TestQueryHooksFailClosedBeforeDownstreamSideEffects(t *testing.T) {
	tests := []struct {
		name          string
		hookType      hooks.HookType
		command       string
		wantCalls     int
		wantHookRuns  int32
		wantAssistant bool
	}{
		{name: "pre query execution error", hookType: hooks.HookPreQuery, command: testFailingHookCommand("pre-error"), wantCalls: 0},
		{name: "pre query explicit block", hookType: hooks.HookPreQuery, command: testHookOutputCommand(`{"block":true,"system_reminder":"pre-policy"}`), wantCalls: 0},
		{name: "post query execution error", hookType: hooks.HookPostQuery, command: testFailingHookCommand("post-error"), wantCalls: 1, wantAssistant: true},
		{name: "post query explicit block", hookType: hooks.HookPostQuery, command: testHookOutputCommand(`{"block":true,"system_reminder":"post-policy"}`), wantCalls: 1, wantAssistant: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			postSampling := &identityPostSamplingProbe{}
			prov := &queryHookOrderProvider{path: filepath.Join(t.TempDir(), "provider-order")}
			query := New(prov, registry.New(), Config{
				MaxTurns: 2, HookRunner: hooks.NewRunner([]hooks.Hook{{Type: test.hookType, Command: test.command, Timeout: 5}}),
				PostSamplingRunner: postSampling, SessionID: "session-fail-closed",
			})
			var summaries []stream.Event
			err := query.Run(context.Background(), "hello", func(event stream.Event) {
				if event.Type == stream.EventHookSummary {
					summaries = append(summaries, event)
				}
			})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "query hook") {
				t.Fatalf("Run error = %v, want query hook fail-closed error", err)
			}
			if prov.calls != test.wantCalls {
				t.Fatalf("provider calls = %d, want %d", prov.calls, test.wantCalls)
			}
			if got := postSampling.runs.Load(); got != test.wantHookRuns {
				t.Fatalf("post-sampling runs = %d, want %d", got, test.wantHookRuns)
			}
			if len(summaries) != 1 || summaries[0].HookSummary == nil || (summaries[0].HookSummary.Status != "failed" && summaries[0].HookSummary.Status != "blocked") {
				t.Fatalf("hook evidence = %#v", summaries)
			}
			messages := query.Messages()
			hasAssistant := false
			for _, message := range messages {
				if message.Role == types.RoleAssistant {
					hasAssistant = true
				}
			}
			if hasAssistant != test.wantAssistant {
				t.Fatalf("assistant response retained = %v, want %v; messages=%#v", hasAssistant, test.wantAssistant, messages)
			}
		})
	}
}
