package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/tools"
	tuiapp "github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/types"
)

func TestBackgroundActivityProjectsBoundedSubagentLiveOutput(t *testing.T) {
	snapshot := tools.BackgroundTaskSnapshot{
		ID: "child", Type: "local_agent", Status: "running", CurrentRunID: "run-1",
		LatestProgress: &tools.AgentProgressEvent{
			AgentID: "child", AgentType: "explore", ParentToolUseID: "agent-call",
			RunID: "run-1", SourceSequence: 7, Phase: tools.AgentPhaseAssistant,
			MessageCount: 3, LatestTool: "Read", PartialText: strings.Repeat("前", maxAgentLiveOutputRunes+1) + "尾",
			ElapsedMs: 1200, TokensUsed: 99, Provider: "anthropic", Model: "claude-sonnet-4-20250514",
			Usage:            &types.Usage{InputTokens: 80, OutputTokens: 19, CacheReadInputTokens: 60},
			LastRequestUsage: &types.Usage{InputTokens: 50, OutputTokens: 9, CacheReadInputTokens: 30},
		},
	}
	event := backgroundActivityEventInLanguage(i18n.LangEN, snapshot, "session", 2, "inspect", "agent", "child", nil, "", false)
	progress := event.Progress
	if progress.AgentID != "child" || progress.AgentType != "explore" || progress.ParentToolUseID != "agent-call" {
		t.Fatalf("subagent correlation was not projected: %+v", progress)
	}
	if progress.Phase != string(tools.AgentPhaseAssistant) || progress.LatestTool != "Read" || progress.Current != 3 || progress.ElapsedMs != 1200 || progress.TokensUsed != 99 {
		t.Fatalf("subagent metrics were not projected: %+v", progress)
	}
	if progress.Provider != "anthropic" || progress.Model != "claude-sonnet-4-20250514" || progress.Usage == nil || progress.Usage.InputTokens != 80 || progress.Usage.OutputTokens != 19 || progress.Usage.CacheReadInputTokens != 60 {
		t.Fatalf("subagent session usage was not projected: %+v", progress)
	}
	if progress.LastRequestUsage == nil || progress.LastRequestUsage.InputTokens != 50 || progress.LastRequestUsage.OutputTokens != 9 || progress.LastRequestUsage.CacheReadInputTokens != 30 {
		t.Fatalf("subagent last request usage was not projected: %+v", progress)
	}
	if count := utf8.RuneCountInString(progress.Output); count > maxAgentLiveOutputRunes || !strings.HasSuffix(progress.Output, "尾") {
		t.Fatalf("live output was not bounded to its newest tail: runes=%d suffix=%q", count, progress.Output[len(progress.Output)-12:])
	}
}

func TestDirectAgentProgressProjectsOneShotSegmentActivity(t *testing.T) {
	event := agentProgressActivityEventInLanguage(i18n.LangZH, tools.AgentProgressEvent{
		AgentID: "child", AgentType: "plan", SessionID: "session", TurnID: "turn", WorkUnitID: "work",
		ParentToolUseID: "agent-call", Phase: tools.AgentPhaseAssistant, SourceSequence: 2,
		MessageCount: 1, PartialText: "实时尾部",
	}, 3)
	if event.ID != "tool:agent-call" || event.Kind != tuiapp.ActivityAgent || event.State != tuiapp.ActivityRunning {
		t.Fatalf("direct progress activity identity = %+v", event)
	}
	if event.Progress.ParentToolUseID != "agent-call" || event.Progress.Output != "实时尾部" {
		t.Fatalf("direct progress activity payload = %+v", event.Progress)
	}
}
