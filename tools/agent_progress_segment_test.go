package tools

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/types"
)

func TestAgentProgressEmitterPropagatesCorrelationToEveryObserver(t *testing.T) {
	emitter := NewAgentProgressEmitter("child", "explore", 4)
	emitter.ConfigureCorrelation(" session ", " turn ", " work ", " agent-call ")
	emitter.ConfigureRun("run-1", 2, "batch-1")

	var primary, additional []AgentProgressEvent
	emitter.SetObserver(func(event AgentProgressEvent) {
		primary = append(primary, event)
	})
	unsubscribe := emitter.AddObserver(func(event AgentProgressEvent) {
		additional = append(additional, event)
	})

	if !emitter.Emit(AgentProgressEvent{Phase: AgentPhaseAssistant, PartialText: "tail"}) {
		t.Fatal("progress event was not emitted")
	}
	if !emitter.Finish(AgentPhaseCompleted, "") {
		t.Fatal("terminal progress event was not emitted")
	}
	unsubscribe()
	unsubscribe()

	for name, events := range map[string][]AgentProgressEvent{"primary": primary, "additional": additional} {
		if len(events) != 2 {
			t.Fatalf("%s observer received %d events, want 2", name, len(events))
		}
		for _, event := range events {
			if event.SessionID != "session" || event.TurnID != "turn" || event.WorkUnitID != "work" || event.ParentToolUseID != "agent-call" {
				t.Fatalf("%s observer lost correlation: %+v", name, event)
			}
			if event.RunID != "run-1" || event.Attempt != 2 || event.BatchID != "batch-1" {
				t.Fatalf("%s observer lost run identity: %+v", name, event)
			}
		}
	}
}

func TestAgentProgressEmitterCarriesSessionUsageIntoTerminalEvent(t *testing.T) {
	emitter := NewAgentProgressEmitter("child", "explore", 4)
	var events []AgentProgressEvent
	emitter.AddObserver(func(event AgentProgressEvent) {
		events = append(events, event)
	})
	usage := &types.Usage{InputTokens: 1_200, OutputTokens: 80, CacheReadInputTokens: 900}
	lastRequestUsage := &types.Usage{InputTokens: 700, OutputTokens: 30, CacheReadInputTokens: 400}
	emitter.Emit(AgentProgressEvent{
		Phase: AgentPhaseAssistant, MessageCount: 2, LatestTool: "Read", TokensUsed: 1_280,
		Provider: "anthropic", Model: "claude-sonnet-4-20250514", Usage: usage, LastRequestUsage: lastRequestUsage,
	})
	usage.InputTokens = 9_999
	lastRequestUsage.InputTokens = 8_888
	emitter.Emit(AgentProgressEvent{Phase: AgentPhaseToolUse, MessageCount: 2, LatestTool: "Bash"})
	emitter.Finish(AgentPhaseError, "tool failed")

	if len(events) != 3 {
		t.Fatalf("progress events = %d, want assistant, tool use, and terminal", len(events))
	}
	toolProgress := events[1]
	if toolProgress.Usage == nil || toolProgress.Usage.InputTokens != 1_200 || toolProgress.LastRequestUsage == nil || toolProgress.LastRequestUsage.InputTokens != 700 || toolProgress.Provider != "anthropic" || toolProgress.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("tool progress lost sticky usage snapshot: %+v", toolProgress)
	}
	terminal := events[2]
	if terminal.Phase != AgentPhaseError || terminal.Detail != "tool failed" || terminal.MessageCount != 2 || terminal.LatestTool != "Bash" || terminal.TokensUsed != 1_280 {
		t.Fatalf("terminal progress lost the last session snapshot: %+v", terminal)
	}
	if terminal.Provider != "anthropic" || terminal.Model != "claude-sonnet-4-20250514" || terminal.Usage == nil || terminal.Usage.InputTokens != 1_200 || terminal.Usage.OutputTokens != 80 || terminal.Usage.CacheReadInputTokens != 900 {
		t.Fatalf("terminal progress usage = %+v", terminal)
	}
	if terminal.LastRequestUsage == nil || terminal.LastRequestUsage.InputTokens != 700 || terminal.LastRequestUsage.OutputTokens != 30 || terminal.LastRequestUsage.CacheReadInputTokens != 400 {
		t.Fatalf("terminal progress last request usage = %+v", terminal.LastRequestUsage)
	}
}

func TestBoundedAgentProgressTailKeepsUnicodeSuffix(t *testing.T) {
	input := strings.Repeat("前", maxAgentProgressTailRunes+1) + "尾"
	got := boundedAgentProgressTail(input)
	if count := utf8.RuneCountInString(got); count != maxAgentProgressTailRunes {
		t.Fatalf("tail rune count = %d, want %d", count, maxAgentProgressTailRunes)
	}
	if !strings.HasSuffix(got, "尾") {
		t.Fatalf("tail lost newest Unicode content: %q", got[len(got)-12:])
	}
}

func TestAgentToolProgressSubscriptionFollowsDistinctRunEmitters(t *testing.T) {
	tool := &AgentTool{}
	var events []AgentProgressEvent
	unsubscribe := tool.SubscribeProgress(func(event AgentProgressEvent) {
		events = append(events, event)
	})

	first := tool.progressForAgentRun("first", "explore")
	first.ConfigureCorrelation("session", "turn", "work", "call-1")
	first.Emit(AgentProgressEvent{Phase: AgentPhaseStart})
	first.Finish(AgentPhaseCompleted, "")

	second := tool.progressForAgentRun("second", "plan")
	second.ConfigureCorrelation("session", "turn", "work", "call-2")
	second.Emit(AgentProgressEvent{Phase: AgentPhaseAssistant})
	if len(events) != 3 {
		t.Fatalf("subscription received %d events across two emitters, want 3", len(events))
	}
	if events[0].AgentID != "first" || events[0].ParentToolUseID != "call-1" || events[2].AgentID != "second" || events[2].ParentToolUseID != "call-2" {
		t.Fatalf("subscription lost per-run identity: %+v", events)
	}

	unsubscribe()
	second.Emit(AgentProgressEvent{Phase: AgentPhaseToolUse})
	if len(events) != 3 {
		t.Fatalf("unsubscribed observer received another event: %+v", events)
	}
}
