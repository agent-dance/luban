package agent

import (
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/types"
)

func TestAgentProgressEmitterPropagatesCorrelationToEveryObserver(t *testing.T) {
	emitter := newAgentProgressEmitter("child", "explore")
	emitter.ConfigureCorrelation(" session ", " turn ", " work ", " agent-call ")
	emitter.ConfigureRun("run-1", 2, "batch-1")

	var primary, additional []agentcontract.ProgressEvent
	emitter.SetObserver(func(event agentcontract.ProgressEvent) {
		primary = append(primary, event)
	})
	unsubscribe := emitter.AddObserver(func(event agentcontract.ProgressEvent) {
		additional = append(additional, event)
	})

	if !emitter.Emit(agentcontract.ProgressEvent{Phase: agentcontract.ProgressAssistant, PartialText: "tail"}) {
		t.Fatal("progress event was not emitted")
	}
	if !emitter.Finish(agentcontract.ProgressCompleted, "") {
		t.Fatal("terminal progress event was not emitted")
	}
	unsubscribe()
	unsubscribe()

	for name, events := range map[string][]agentcontract.ProgressEvent{"primary": primary, "additional": additional} {
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
	emitter := newAgentProgressEmitter("child", "explore")
	var events []agentcontract.ProgressEvent
	emitter.AddObserver(func(event agentcontract.ProgressEvent) {
		events = append(events, event)
	})
	usage := &types.Usage{InputTokens: 1_200, OutputTokens: 80, CacheReadInputTokens: 900}
	lastRequestUsage := &types.Usage{InputTokens: 700, OutputTokens: 30, CacheReadInputTokens: 400}
	emitter.Emit(agentcontract.ProgressEvent{
		Phase: agentcontract.ProgressAssistant, MessageCount: 2, LatestTool: "Read", TokensUsed: 1_280,
		Provider: "anthropic", Model: "claude-sonnet-4-20250514", Usage: usage, LastRequestUsage: lastRequestUsage,
	})
	usage.InputTokens = 9_999
	lastRequestUsage.InputTokens = 8_888
	emitter.Emit(agentcontract.ProgressEvent{Phase: agentcontract.ProgressToolUse, MessageCount: 2, LatestTool: "Bash"})
	emitter.Finish(agentcontract.ProgressError, "tool failed")

	if len(events) != 3 {
		t.Fatalf("progress events = %d, want assistant, tool use, and terminal", len(events))
	}
	toolProgress := events[1]
	if toolProgress.Usage == nil || toolProgress.Usage.InputTokens != 1_200 || toolProgress.LastRequestUsage == nil || toolProgress.LastRequestUsage.InputTokens != 700 || toolProgress.Provider != "anthropic" || toolProgress.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("tool progress lost sticky usage snapshot: %+v", toolProgress)
	}
	terminal := events[2]
	if terminal.Phase != agentcontract.ProgressError || terminal.Detail != "tool failed" || terminal.MessageCount != 2 || terminal.LatestTool != "Bash" || terminal.TokensUsed != 1_280 {
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
	var events []agentcontract.ProgressEvent
	unsubscribe := tool.SubscribeProgress(func(event agentcontract.ProgressEvent) {
		events = append(events, event)
	})

	first := tool.progressForAgentRun("first", "explore")
	first.ConfigureCorrelation("session", "turn", "work", "call-1")
	first.Emit(agentcontract.ProgressEvent{Phase: agentcontract.ProgressStart})
	first.Finish(agentcontract.ProgressCompleted, "")

	second := tool.progressForAgentRun("second", "plan")
	second.ConfigureCorrelation("session", "turn", "work", "call-2")
	second.Emit(agentcontract.ProgressEvent{Phase: agentcontract.ProgressAssistant})
	if len(events) != 3 {
		t.Fatalf("subscription received %d events across two emitters, want 3", len(events))
	}
	if events[0].AgentID != "first" || events[0].ParentToolUseID != "call-1" || events[2].AgentID != "second" || events[2].ParentToolUseID != "call-2" {
		t.Fatalf("subscription lost per-run identity: %+v", events)
	}

	unsubscribe()
	second.Emit(agentcontract.ProgressEvent{Phase: agentcontract.ProgressToolUse})
	if len(events) != 3 {
		t.Fatalf("unsubscribed observer received another event: %+v", events)
	}
}
