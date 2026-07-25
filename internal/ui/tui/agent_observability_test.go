package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestAgentToolCallPreviewUsesDescriptionAndType(t *testing.T) {
	got := toolInputPreview("Agent", map[string]any{
		"description":   "Compare WebFetch",
		"subagent_type": "general-purpose",
		"prompt":        "This prompt is intentionally longer and less useful for the compact status line.",
	})
	want := "Compare WebFetch [general-purpose]"
	if got != want {
		t.Fatalf("toolInputPreview(Agent) = %q, want %q", got, want)
	}
}

func TestRenderAgentStartLine(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)

	start := collectElementText(root.renderToolCallLine(Message{
		Kind:     MsgToolCall,
		ToolName: "Agent",
		Text:     "Compare WebFetch [general-purpose]",
	}))
	if !strings.Contains(start, "Agent") || !strings.Contains(start, "Compare WebFetch") {
		t.Fatalf("agent start line should include tool name and task preview, got %q", start)
	}
}

func TestRenderAgentCompletedResultSummarizesJSON(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	result := `{
		"status":"completed",
		"agentId":"agent-123",
		"agentType":"general-purpose",
		"content":[{"type":"text","text":"Found the WebFetch implementation and summarized the differences."}],
		"totalDurationMs":2500,
		"totalTokens":321,
		"totalToolUseCount":4
	}`

	text := collectElementText(root.renderToolResultBlock(Message{
		Kind:      MsgToolResult,
		ToolName:  "Agent",
		Text:      result,
		Collapsed: true,
	}))

	for _, want := range []string{"Agent completed: agent-123", "general-purpose", "4 tools", "321 tokens", "Found the WebFetch implementation"} {
		if !strings.Contains(text, want) {
			t.Fatalf("completed agent result missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, `"status"`) {
		t.Fatalf("completed agent result should not expose raw JSON, got %q", text)
	}
}

func TestAgentTerminalCardUsesSafeFinalInformationWithoutRawRecordAccess(t *testing.T) {
	tests := []struct {
		name       string
		lifecycle  ActivityLifecycle
		outcome    ObservationOutcome
		toolResult types.ToolResultBlock
		want       string
	}{
		{
			name:      "success conclusion",
			lifecycle: ActivityLifecycleCompleted,
			outcome:   OutcomeSucceeded,
			toolResult: types.ToolResultBlock{
				Outcome: types.ToolOutcomeSucceeded,
				Content: "SAFE-SUCCESS-CONCLUSION\nagentId: agent-internal-success\n<usage>total_tokens: 987</usage>",
				Data: map[string]any{
					"kind":           "completed",
					"status":         "completed",
					"agentId":        "agent-internal-success",
					"content":        []any{map[string]any{"type": "text", "text": "SAFE-SUCCESS-CONCLUSION"}},
					"transcriptPath": "/private/tmp/agent-internal-success.jsonl",
				},
			},
			want: "SAFE-SUCCESS-CONCLUSION",
		},
		{
			name:      "failure reason",
			lifecycle: ActivityLifecycleFailed,
			outcome:   OutcomeFailed,
			toolResult: types.ToolResultBlock{
				Outcome: types.ToolOutcomeFailed,
				Content: "RAW-PROVIDER-FAILURE\nagentId: agent-internal-failure\n<usage>duration_ms: 321</usage>",
				Data: map[string]any{
					"kind":           "failed",
					"status":         "failed",
					"agentId":        "agent-internal-failure",
					"error":          "SAFE-FAILURE-REASON",
					"transcriptPath": "/private/tmp/agent-internal-failure.jsonl",
				},
			},
			want: "SAFE-FAILURE-REASON",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				sessionID = "agent-terminal-session"
				toolUseID = "agent-terminal-call"
			)
			state := NewAppState()
			state.Language.Set(i18n.LangEN)
			state.SessionID.Set(sessionID)
			state.SessionEpoch.Set(1)
			state.Activities = NewActivityStore(ActivityScope{SessionID: sessionID, Epoch: 1})
			ctx := ToolEventContext{
				SessionID: sessionID, TurnID: "turn", WorkUnitID: "work", ActorID: "assistant",
			}
			if err := state.ApplyToolCall(ctx, types.ToolUseBlock{
				ID: toolUseID, Name: "Agent",
				Input: map[string]any{"description": "verify terminal result", "subagent_type": "verifier"},
			}); err != nil {
				t.Fatal(err)
			}
			tc.toolResult.ToolUseID = toolUseID
			ctx.Outcome = tc.outcome
			if err := state.ApplyToolResult(ctx, tc.toolResult); err != nil {
				t.Fatal(err)
			}
			if err := state.ApplyActivity(ActivityEvent{
				ID: "background:agent-terminal", RunID: "agent-terminal-run", Attempt: 1,
				SessionID: sessionID, Epoch: 1, TurnID: ctx.TurnID, WorkUnitID: "agent-work",
				Kind: ActivityAgent, Name: "Agent", Lifecycle: tc.lifecycle, Outcome: tc.outcome,
				Progress: ActivityProgress{
					AgentID: "agent-terminal", AgentType: "verifier", ParentToolUseID: toolUseID,
					Output: "RAW-LIVE-OUTPUT\nagentId: agent-live-internal\n<usage>tool_uses: 44</usage>",
				},
			}); err != nil {
				t.Fatal(err)
			}

			message := state.Messages.Get()[0]
			if _, ok := state.GetObservation(message.ObservationID); !ok {
				t.Fatal("Agent observation missing")
			}
			ref, err := state.RetainDetailForEpoch(sessionID, 1, "agent-terminal-raw-record", []byte("RAW-RETAINED-EVIDENCE"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.AttachToolObservationDetailForEpoch(sessionID, 1, toolUseID, ref); err != nil {
				t.Fatal(err)
			}

			root := NewRootComponent(state, nil, nil)
			observation, ok := state.GetObservation(message.ObservationID)
			if !ok {
				t.Fatal("Agent observation missing before expansion")
			}
			activity, ok := root.subagentActivityForObservation(observation)
			if !ok {
				t.Fatal("terminal Agent activity missing before expansion")
			}
			state.SetToolSegmentExpanded(subagentProgressSegmentID(activity), true)
			rendered := renderElementText(root.renderToolObservation(message), 120, 30)
			if !strings.Contains(rendered, tc.want) {
				t.Fatalf("terminal Agent card omitted safe final information %q:\n%s", tc.want, rendered)
			}
			for _, forbidden := range []string{
				"Run record", "View run record", "Evidence", "RAW-PROVIDER-FAILURE",
				"RAW-LIVE-OUTPUT", "RAW-RETAINED-EVIDENCE", "agentId", "<usage>",
				"total_tokens", "tool_uses", "duration_ms", "/private/tmp/",
			} {
				if strings.Contains(rendered, forbidden) {
					t.Errorf("terminal Agent card exposed %q:\n%s", forbidden, rendered)
				}
			}
			for id := range root.segmentRefs.All() {
				if strings.HasPrefix(id, "subagent-record:") {
					t.Errorf("terminal Agent card registered forbidden run-record action %q", id)
				}
			}
		})
	}
}

func TestRenderNonAgentJSONStatusUsesRegularToolSummary(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	text := collectElementText(root.renderToolResultBlock(Message{
		Kind:      MsgToolResult,
		ToolName:  "TaskOutput",
		Text:      `{"status":"completed","result":"ok"}`,
		Collapsed: true,
	}))

	if strings.Contains(text, "Agent completed") {
		t.Fatalf("non-agent JSON status should not be rendered as agent output: %q", text)
	}
	if !strings.Contains(text, "TaskOutput") {
		t.Fatalf("non-agent collapsed summary should keep the tool name, got %q", text)
	}
}

func TestRenderAgentBackgroundedResultShowsOutputFile(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	result := `{
		"isAsync":true,
		"status":"async_launched",
		"description":"Compare WebSearch",
		"agentId":"agent-456",
		"outputFile":"C:/tmp/agent-456/output.txt",
		"message":"Use SendMessage with to: \"agent-456\" to continue this agent."
	}`

	text := collectElementText(root.renderToolResultBlock(Message{
		Kind:      MsgToolResult,
		ToolName:  "Agent",
		Text:      result,
		Collapsed: true,
	}))

	for _, want := range []string{"Agent moved to the background: agent-456", "Compare WebSearch", "output: C:/tmp/agent-456/output.txt", "Use SendMessage"} {
		if !strings.Contains(text, want) {
			t.Fatalf("backgrounded agent result missing %q in %q", want, text)
		}
	}
}

func TestRenderTeammateSpawnedResultIsDistinct(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	result := `{
		"status":"teammate_spawned",
		"name":"reviewer",
		"team_name":"team-alpha",
		"agent_id":"agent-789",
		"output_file":"C:/tmp/agent-789/output.txt"
	}`

	text := collectElementText(root.renderToolResultBlock(Message{
		Kind:      MsgToolResult,
		ToolName:  "Agent",
		Text:      result,
		Collapsed: true,
	}))

	for _, want := range []string{"Teammate started: reviewer", "team-alpha", "output: C:/tmp/agent-789/output.txt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("teammate result missing %q in %q", want, text)
		}
	}
}

func TestRenderToolErrorsUsesTypedOutcomeInsteadOfProse(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	cases := []struct {
		name     string
		toolName string
		content  string
		outcome  ObservationOutcome
		want     string
	}{
		{
			name:     "permission denied",
			toolName: "Read",
			content:  "Permission denied for tool: Read",
			outcome:  OutcomeDenied,
			want:     "Read denied",
		},
		{
			name:     "safety denied",
			toolName: "Bash",
			content:  "Safety: denied Bash - denied by ask-always mode (no prompt or user rejected)",
			outcome:  OutcomeDenied,
			want:     "Bash denied",
		},
		{
			name:     "user cancelled",
			toolName: "Agent",
			content:  "user canceled",
			outcome:  OutcomeCancelled,
			want:     "Agent cancelled",
		},
		{
			name:     "context cancelled",
			toolName: "Agent",
			content:  "context canceled",
			outcome:  OutcomeCancelled,
			want:     "Agent cancelled",
		},
		{
			name:     "execution cancelled",
			toolName: "Grep",
			content:  "Tool execution cancelled",
			outcome:  OutcomeCancelled,
			want:     "Grep cancelled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := collectElementText(root.renderToolResultBlock(Message{
				Kind:     MsgToolResult,
				ToolName: tc.toolName,
				Text:     tc.content,
				IsError:  true,
				Outcome:  tc.outcome,
			}))
			if !strings.Contains(text, tc.want) {
				t.Fatalf("error classification missing %q in %q", tc.want, text)
			}
			if !strings.Contains(text, tc.content) {
				t.Fatalf("error details should remain visible, got %q", text)
			}
		})
	}
}
