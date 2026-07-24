package tools

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAgentToolCoreInputUsesSemanticWhitelist(t *testing.T) {
	tests := []struct {
		name    string
		toolUse types.ToolUseBlock
		want    string
		absent  []string
	}{
		{
			name: "read keeps only location and range",
			toolUse: types.ToolUseBlock{
				Name: "Read",
				Input: map[string]any{
					"file_path": "/tmp/main.go",
					"offset":    2,
					"limit":     10,
					"content":   "must-not-be-rendered",
					"api_key":   "sk-12345678901234567890",
				},
			},
			want:   "file_path=/tmp/main.go · offset=2 · limit=10",
			absent: []string{"content", "must-not-be-rendered", "api_key", "sk-"},
		},
		{
			name: "web fetch removes URL credentials and sensitive query values",
			toolUse: types.ToolUseBlock{
				Name: "WebFetch",
				Input: map[string]any{
					"url":     "https://user:pass@example.com/report?q=kept&token=do-not-render#private",
					"prompt":  "summarize the report",
					"headers": map[string]any{"Authorization": "Bearer do-not-render"},
				},
			},
			want:   "url=https://example.com/report?q=kept · prompt=summarize the report",
			absent: []string{"user", "pass", "token", "do-not-render", "private", "headers", "Authorization"},
		},
		{
			name: "unknown tools expose no arbitrary input",
			toolUse: types.ToolUseBlock{
				Name:  "CustomTool",
				Input: map[string]any{"payload": "opaque-value"},
			},
			want:   "",
			absent: []string{"payload", "opaque-value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentToolCoreInput(tt.toolUse)
			if got != tt.want {
				t.Fatalf("agentToolCoreInput() = %q, want %q", got, tt.want)
			}
			for _, value := range tt.absent {
				if strings.Contains(got, value) {
					t.Fatalf("agentToolCoreInput() leaked %q in %q", value, got)
				}
			}
		})
	}
}

func TestAgentToolResponsePreviewRemovesANSIAndSecretLines(t *testing.T) {
	result := types.ToolResultBlock{
		Content: "\x1b[31mfirst safe line\x1b[0m\n" +
			"<system-reminder>internal protocol must not render</system-reminder>\n" +
			"-----BEGIN RSA PRIVATE KEY----- do-not-render\n" +
			"second safe line\x00",
	}

	got := agentToolResponsePreview(result)
	for _, want := range []string{"first safe line", "second safe line"} {
		if !strings.Contains(got, want) {
			t.Fatalf("agentToolResponsePreview() = %q, missing %q", got, want)
		}
	}
	for _, forbidden := range []string{"\x1b", "\x00", "PRIVATE KEY", "do-not-render", "system-reminder", "internal protocol"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("agentToolResponsePreview() retained unsafe value %q in %q", forbidden, got)
		}
	}
}

func TestAgentLiveOutputAccumulatesMultilineToolResponses(t *testing.T) {
	var output agentLiveOutputBuffer
	output.appendAssistant("checking")
	output.appendToolCall(types.ToolUseBlock{
		ID:    "fetch-1",
		Name:  "WebFetch",
		Input: map[string]any{"url": "https://example.com/report"},
	})
	output.appendToolResult("WebFetch", types.ToolResultBlock{
		ToolUseID: "fetch-1",
		Content:   "first line\nsecond line",
	})
	output.appendToolCall(types.ToolUseBlock{
		ID:    "read-1",
		Name:  "Read",
		Input: map[string]any{"file_path": "/tmp/main.go"},
	})
	output.appendToolResult("Read", types.ToolResultBlock{
		ToolUseID: "read-1",
		Content:   "third line\nfourth line",
	})

	want := strings.Join([]string{
		"checking",
		"→ WebFetch url=https://example.com/report",
		"← WebFetch",
		"first line",
		"second line",
		"→ Read file_path=/tmp/main.go",
		"← Read",
		"third line",
		"fourth line",
	}, "\n")
	if got := output.snapshot(); got != want {
		t.Fatalf("snapshot() =\n%s\nwant:\n%s", got, want)
	}
}

type agentLiveOutputTool struct {
	name    string
	content string
	delay   time.Duration
}

func (t agentLiveOutputTool) Name() string        { return t.name }
func (t agentLiveOutputTool) Description() string { return "agent live output test tool" }
func (t agentLiveOutputTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t agentLiveOutputTool) IsConcurrentSafe() bool { return true }
func (t agentLiveOutputTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	if t.delay > 0 {
		select {
		case <-ctx.Done():
			return types.ToolResult{}, ctx.Err()
		case <-time.After(t.delay):
		}
	}
	return types.ToolResult{Content: t.content}, nil
}

func TestRunAgentQueryLoopCorrelatesToolResultsByToolUseID(t *testing.T) {
	reg := registry.New()
	reg.Register(agentLiveOutputTool{
		name:    "Read",
		content: "READ_RESULT line one\nREAD_RESULT line two",
		delay:   30 * time.Millisecond,
	})
	reg.Register(agentLiveOutputTool{
		name:    "WebFetch",
		content: "FETCH_RESULT line one\nFETCH_RESULT line two",
	})
	provider := &sequencedAgentProvider{responses: [][]types.StreamEvent{
		parallelAgentToolUseEvents(
			types.ToolUseBlock{
				Type:  types.ContentTypeToolUse,
				ID:    "read-1",
				Name:  "Read",
				Input: map[string]any{"file_path": "/tmp/main.go"},
			},
			types.ToolUseBlock{
				Type:  types.ContentTypeToolUse,
				ID:    "fetch-1",
				Name:  "WebFetch",
				Input: map[string]any{"url": "https://example.com/report"},
			},
		),
		agentTextEvents("final-only"),
	}}
	query := loop.New(provider, reg, loop.Config{
		MaxTurns:  4,
		MaxTokens: 1024,
		Model:     provider.ModelID(),
		AgentID:   "agent-live-output",
		AgentType: "explore",
	})

	emitter := NewAgentProgressEmitter("agent-live-output", "explore", 32)
	var mu sync.Mutex
	var events []AgentProgressEvent
	emitter.AddObserver(func(event AgentProgressEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	ctx := withAgentProgressEmitter(context.Background(), emitter)
	summary, err := runAgentQueryLoop(ctx, query, agentSessionMetadata{AgentType: "explore"}, "agent-live-output", "inspect", nil)
	if err != nil {
		t.Fatalf("runAgentQueryLoop() error = %v", err)
	}
	if summary.Output != "final-only" {
		t.Fatalf("summary.Output = %q, want final assistant text only", summary.Output)
	}
	for _, forbidden := range []string{"→", "←", "READ_RESULT", "FETCH_RESULT"} {
		if strings.Contains(summary.Output, forbidden) {
			t.Fatalf("final output was polluted by live tool output %q: %q", forbidden, summary.Output)
		}
	}

	mu.Lock()
	captured := append([]AgentProgressEvent(nil), events...)
	mu.Unlock()
	var latest string
	for _, event := range captured {
		if len(event.PartialText) >= len(latest) {
			latest = event.PartialText
		}
	}
	for _, want := range []string{
		"→ Read file_path=/tmp/main.go",
		"→ WebFetch url=https://example.com/report",
		"← WebFetch\nFETCH_RESULT line one\nFETCH_RESULT line two",
		"← Read\nREAD_RESULT line one\nREAD_RESULT line two",
		"final-only",
	} {
		if !strings.Contains(latest, want) {
			t.Fatalf("progress PartialText =\n%s\nmissing %q", latest, want)
		}
	}
	for _, swapped := range []string{
		"← Read\nFETCH_RESULT",
		"← WebFetch\nREAD_RESULT",
	} {
		if strings.Contains(latest, swapped) {
			t.Fatalf("tool result was associated with the wrong tool in progress output: %q", latest)
		}
	}
}
