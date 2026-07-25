package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

func TestSubagentHeaderShowsSessionUsageAsDimSupplementaryText(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityRunning, false)
	fixture.root.termWidth = 180
	fixture.activity.Progress.ElapsedMs = 46_600
	fixture.activity.Progress.TokensUsed = 19_724
	fixture.activity.Progress.Model = "claude-sonnet-4-20250514"
	fixture.activity.Progress.Usage = &types.Usage{
		InputTokens:          19_303,
		OutputTokens:         421,
		CacheReadInputTokens: 16_000,
	}
	fixture.activity.Progress.LastRequestUsage = &types.Usage{
		InputTokens:          12_000,
		OutputTokens:         250,
		CacheReadInputTokens: 9_000,
	}

	card := fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, fixture.activity)
	buf := gtui.NewBuffer(180, 20)
	card.Render(buf, 180, 20)
	rendered := renderElementText(card, 180, 20)
	wantMetrics := "Session: in 19.3K · 83% cached · out 421 · $0.0210 · 46.6 s"
	if !strings.Contains(rendered, wantMetrics) {
		t.Fatalf("subagent title metrics =\n%s\nwant %q", rendered, wantMetrics)
	}
	lines := subagentLayoutNonEmptyTextLines(rendered)
	if len(lines) < 2 {
		t.Fatalf("subagent card omitted metadata row:\n%s", rendered)
	}
	for _, obsolete := range []string{
		i18n.Format(i18n.LangEN, i18n.KeyTUIAgentDuration, 46.6),
		i18n.Format(i18n.LangEN, i18n.KeyTUIAgentTokens, 19_724),
	} {
		if strings.Contains(lines[1], obsolete) {
			t.Fatalf("subagent metadata row retained obsolete metric %q:\n%s", obsolete, rendered)
		}
	}

	metricColumn := strings.Index(lines[0], "Session: in 19.3K")
	if metricColumn < 0 || buf.Cell(metricColumn, 0).Style.Attrs&gtui.AttrDim == 0 {
		t.Fatalf("subagent title metrics are not dimmed: column=%d style=%+v", metricColumn, buf.Cell(metricColumn, 0).Style)
	}
	if buf.Cell(2, 0).Style.Attrs&gtui.AttrBold == 0 || buf.Cell(2, 0).Style.Attrs&gtui.AttrDim != 0 {
		t.Fatalf("subagent primary title lost its strong visual hierarchy: %+v", buf.Cell(2, 0).Style)
	}
}

func TestFormatSubagentHeaderMetricsMarksUnknownPricing(t *testing.T) {
	got := formatSubagentHeaderMetrics(ActivityProgress{
		Model:            "unknown-model",
		Usage:            &types.Usage{InputTokens: 2_000, OutputTokens: 250, CacheReadInputTokens: 1_000},
		LastRequestUsage: &types.Usage{InputTokens: 1_200, OutputTokens: 100, CacheReadInputTokens: 300},
	}, i18n.LangEN)
	want := "Session: in 2.0K · 50% cached · out 250 · cost unknown"
	if got != want {
		t.Fatalf("formatSubagentHeaderMetrics() = %q, want %q", got, want)
	}
}

func TestFormatSubagentHeaderMetricsDoesNotRelabelCumulativeUsageAsRequest(t *testing.T) {
	got := formatSubagentHeaderMetrics(ActivityProgress{
		Model: "claude-sonnet-4-20250514",
		Usage: &types.Usage{InputTokens: 2_000, OutputTokens: 250, CacheReadInputTokens: 1_000},
	}, i18n.LangEN)
	want := "Session: in 2.0K · 50% cached · out 250 · $0.0070"
	if got != want {
		t.Fatalf("formatSubagentHeaderMetrics() = %q, want %q", got, want)
	}
}

func TestSubagentProgressSegmentStaysExpandedUntilVisibleSuccessor(t *testing.T) {
	const (
		sessionID = "session"
		toolUseID = "agent-call"
	)
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionID.Set(sessionID)
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: sessionID, Epoch: 1})
	ctx := ToolEventContext{SessionID: sessionID, TurnID: "turn", WorkUnitID: "work", ActorID: "assistant"}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{
		ID: toolUseID, Name: "Agent", Input: map[string]any{"description": "inspect rendering", "subagent_type": "explore"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyActivity(ActivityEvent{
		ID: "background:child", RunID: "run-1", Attempt: 1, SessionID: sessionID, Epoch: 1,
		TurnID: "turn", WorkUnitID: "child", Kind: ActivityBackground, Name: "Agent",
		Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
		Progress: ActivityProgress{
			Current: 4, AgentID: "child", AgentType: "explore", ParentToolUseID: toolUseID,
			Phase: "assistant", LatestTool: "Read", Output: "oldest\nsecond\nthird\n最新一行",
			ElapsedMs: 1500, TokensUsed: 42,
		},
	}); err != nil {
		t.Fatal(err)
	}

	root := NewRootComponent(state, nil, nil)
	root.termWidth = 80
	rendered := collectElementText(root.renderMessageArea(20))
	for _, want := range []string{"Agent", "inspect rendering", "oldest", "second", "third", "最新一行"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expanded segment does not contain %q: %q", want, rendered)
		}
	}
	if heading := "Live output"; strings.Contains(rendered, heading) {
		t.Fatalf("expanded segment retained fixed output heading %q: %q", heading, rendered)
	}
	message := state.Messages.Get()[0]
	observation, ok := state.GetObservation(message.ObservationID)
	if !ok {
		t.Fatal("Agent observation missing")
	}
	activity, ok := root.subagentActivityForObservation(observation)
	if !ok {
		t.Fatal("subagent activity missing")
	}
	root.termWidth = 40
	segment := root.renderSubagentProgressSegment(message, observation, activity)
	wantHeight := 2 + len(subagentProgressOutputLinesAtWidth(activity.Progress.Output, 37, subagentProgressOutputRows))
	if got := segment.HeightForWidth(40); got != wantHeight {
		t.Fatalf("expanded segment height = %d, want %d", got, wantHeight)
	}

	id := subagentProgressSegmentID(activity)
	if !transcriptObservationOngoing(state.Messages.Get(), "", activity.Progress.ParentToolUseID) {
		t.Fatalf("trailing Agent observation was not recognized as ongoing: message=%+v activity=%+v", message, activity.Progress)
	}
	state.SetToolSegmentExpanded(id, false)
	if !root.toggleToolSegmentByID(id) {
		t.Fatal("subagent segment did not toggle")
	}
	stillExpanded := root.renderSubagentProgressSegment(message, observation, activity)
	if got := stillExpanded.HeightForWidth(40); got != wantHeight {
		t.Fatalf("ongoing subagent segment collapsed to height %d, want %d", got, wantHeight)
	}
	if text := collectElementText(stillExpanded); !strings.Contains(text, "最新一行") || !strings.HasPrefix(text, "▾ ") {
		t.Fatalf("ongoing subagent segment lost its expanded output: %q", text)
	}

	state.AppendMessage(Message{Kind: MsgAssistant, Text: "Agent progress noted."})
	collapsed := root.renderSubagentProgressSegment(message, observation, activity)
	if got := collapsed.HeightForWidth(40); got != 1 {
		t.Fatalf("segment with visible successor height = %d, want 1", got)
	}
	if text := collectElementText(collapsed); strings.Contains(text, "最新一行") || !strings.HasPrefix(text, "▸ ") {
		t.Fatalf("collapsible segment leaked output or lost its marker: %q", text)
	}
}

func TestSubagentRunningCardDynamicContentHasNoReservedHeadingRow(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityRunning, false)
	fixture.root.termWidth = 40
	activity := fixture.activity
	activity.Progress.AgentType = ""

	render := func() (int, string) {
		t.Helper()
		card := fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, activity)
		return card.HeightForWidth(40), renderElementText(card, 40, 30)
	}
	assertNoReservedRows := func(stage, rendered string) {
		t.Helper()
		for _, forbidden := range []string{
			"Live output",
			i18n.Text(i18n.LangEN, i18n.KeySubagentSegmentWaiting),
		} {
			if strings.Contains(rendered, forbidden) {
				t.Fatalf("%s running card retained reserved row %q:\n%s", stage, forbidden, rendered)
			}
		}
	}

	activity.Progress.Phase = "start"
	activity.Progress.LatestTool = ""
	activity.Progress.Output = ""
	height, rendered := render()
	assertNoReservedRows("start", rendered)
	if height != 2 {
		t.Fatalf("start card height = %d, want header + metadata = 2:\n%s", height, rendered)
	}

	activity.Progress.Phase = "tool_use"
	activity.Progress.LatestTool = "Read"
	height, rendered = render()
	assertNoReservedRows("tool progress", rendered)
	if tool := i18n.Format(i18n.LangEN, i18n.KeyREPLTUIToolName, "Read"); !strings.Contains(rendered, tool) {
		t.Fatalf("tool progress card omitted structured tool label %q:\n%s", tool, rendered)
	}
	if height != 2 {
		t.Fatalf("tool progress card height = %d, want unchanged height 2:\n%s", height, rendered)
	}

	activity.Progress.Phase = "assistant"
	activity.Progress.Output = strings.Join([]string{
		"live-line-01", "live-line-02", "live-line-03", "live-line-04",
		"live-line-05", "live-line-06", "live-line-07", "live-line-08",
		"live-line-09", "live-line-10", "live-line-11", "live-line-12",
	}, "\n")
	height, rendered = render()
	assertNoReservedRows("text stream", rendered)
	var visibleOutput []string
	for _, line := range subagentLayoutNonEmptyTextLines(rendered) {
		if strings.Contains(line, "live-line-") {
			visibleOutput = append(visibleOutput, line)
		}
	}
	if len(visibleOutput) != subagentProgressOutputRows {
		t.Fatalf("text stream shows %d output rows, want %d latest visual rows:\n%s", len(visibleOutput), subagentProgressOutputRows, rendered)
	}
	if strings.Contains(rendered, "live-line-01") || strings.Contains(rendered, "live-line-02") {
		t.Fatalf("text stream retained rows older than its ten-row window:\n%s", rendered)
	}
	if !strings.Contains(rendered, "live-line-03") || !strings.Contains(rendered, "live-line-12") {
		t.Fatalf("text stream lost rows from its latest ten-row window:\n%s", rendered)
	}
	if want := 2 + subagentProgressOutputRows; height != want {
		t.Fatalf("text stream card height = %d, want header + metadata + output = %d:\n%s", height, want, rendered)
	}
}

func TestSubagentTerminalCardDisplaysFinalConclusionWithoutRunRecordAccess(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityReadyReview, true)
	fixture.setExpanded(true)
	rendered := renderElementText(fixture.root.renderToolObservation(fixture.message), 120, 12)
	for _, want := range []string{
		i18n.RuntimeActivityStateLabel(i18n.LangEN, string(ActivityCompleted)),
		i18n.Text(i18n.LangEN, i18n.KeySubagentSegmentResultPendingView),
		i18n.Text(i18n.LangEN, i18n.KeySubagentSegmentResultSummary),
		"Forecast summary for the parent.",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("terminal card omitted %q: %q", want, rendered)
		}
	}
	if misleading := i18n.RuntimeActivityStateLabel(i18n.LangEN, string(ActivityReadyReview)); strings.Contains(rendered, misleading) {
		t.Fatalf("attention replaced terminal lifecycle with %q: %q", misleading, rendered)
	}
	if strings.Contains(strings.ToLower(rendered), "run record") {
		t.Fatalf("terminal card exposed a run-record action: %q", rendered)
	}
	if strings.Contains(rendered, "latest meaningful progress") {
		t.Fatalf("terminal card retained stale live output: %q", rendered)
	}
	for id := range fixture.root.segmentRefs.All() {
		if strings.HasPrefix(id, "subagent-record:") {
			t.Fatalf("terminal card registered forbidden run-record click region %q", id)
		}
	}
}

func TestSubagentTerminalCardDefaultsCollapsedAtStructuredEnd(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityReadyReview, true)
	card := fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, fixture.activity)
	if got := card.HeightForWidth(120); got != 1 {
		t.Fatalf("terminal Agent card height = %d, want collapsed header", got)
	}
	rendered := collectElementText(card)
	if !strings.HasPrefix(rendered, "▸ ") {
		t.Fatalf("terminal Agent card did not use collapsed marker: %q", rendered)
	}
	if !strings.Contains(rendered, i18n.Text(i18n.LangEN, i18n.KeySubagentSegmentResultPendingView)) {
		t.Fatalf("collapsed terminal Agent card lost its pending-result summary: %q", rendered)
	}
	if strings.Contains(rendered, subagentLayoutConclusion) {
		t.Fatalf("collapsed terminal Agent card leaked its conclusion: %q", rendered)
	}
}

func TestSubagentTerminalCardTitleAcknowledgesWithoutRevealingEvidence(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityReadyReview, true)
	_ = fixture.root.renderToolObservation(fixture.message)
	before, ok := fixture.root.state.GetActivity(fixture.activity.ID)
	if !ok || before.Lifecycle != ActivityLifecycleCompleted || !before.Attention.Unread || before.Acknowledged {
		t.Fatalf("initial terminal attention = %+v ok=%t", before, ok)
	}
	observationBefore, ok := fixture.root.state.GetObservation(fixture.observation.ID)
	if !ok || observationBefore.Disclosure.Level == DisclosureEvidence || observationBefore.Disclosure.UserPinned {
		t.Fatalf("initial terminal disclosure = %+v ok=%t", observationBefore.Disclosure, ok)
	}

	if !fixture.root.toggleToolSegmentByID(subagentProgressSegmentID(fixture.activity)) {
		t.Fatal("terminal Agent title click was not handled")
	}
	after, ok := fixture.root.state.GetActivity(fixture.activity.ID)
	if !ok || after.Lifecycle != ActivityLifecycleCompleted || after.Attention.Unread || !after.Acknowledged {
		t.Fatalf("terminal title click did not acknowledge completed result: %+v ok=%t", after, ok)
	}
	observationAfter, ok := fixture.root.state.GetObservation(fixture.observation.ID)
	if !ok || observationAfter.Disclosure.Level == DisclosureEvidence || observationAfter.Disclosure.UserPinned {
		t.Fatalf("terminal title click exposed full evidence: %+v ok=%t", observationAfter.Disclosure, ok)
	}
	rendered := renderElementText(fixture.root.renderToolObservation(fixture.message), 120, 12)
	if pending := i18n.Text(i18n.LangEN, i18n.KeySubagentSegmentResultPendingView); strings.Contains(rendered, pending) {
		t.Fatalf("acknowledged terminal card retained pending-result badge %q: %q", pending, rendered)
	}
}

func TestSubagentTerminalCardCompleteConclusionCanCollapseAndReexpand(t *testing.T) {
	conclusion := strings.Join([]string{
		"terminal-result-01",
		"terminal-result-02",
		"terminal-result-03",
		"terminal-result-04",
		"terminal-result-05-ASCII-" + strings.Repeat("abcdefghij", 15) + "-中文自动换行不截断",
		"terminal-result-06",
		"terminal-result-07",
		"terminal-result-08",
		"terminal-result-09",
		"terminal-result-10",
		"terminal-result-11",
		"terminal-result-12",
		"FINAL_CONCLUSION",
	}, "\n")
	fixture := newSubagentLayoutContractFixtureWithConclusion(t, ActivityCompleted, true, conclusion)
	fixture.root.termWidth = 40
	fixture.root.state.AppendMessage(segmentTestAssistant("Agent result incorporated."))
	fixture.setExpanded(true)

	assertNoEvidenceOrRunRecord := func(stage, rendered string) {
		t.Helper()
		observation, ok := fixture.root.state.GetObservation(fixture.observation.ID)
		if !ok || observation.Disclosure.Level == DisclosureEvidence || observation.Disclosure.UserPinned {
			t.Fatalf("%s terminal disclosure exposed full evidence: %+v ok=%t", stage, observation.Disclosure, ok)
		}
		if strings.Contains(strings.ToLower(rendered), "run record") {
			t.Fatalf("%s terminal card exposed a run-record action:\n%s", stage, rendered)
		}
		for id := range fixture.root.segmentRefs.All() {
			if strings.HasPrefix(id, "subagent-record:") {
				t.Fatalf("%s terminal card registered forbidden run-record click region %q", stage, id)
			}
		}
	}
	assertExpanded := func(stage string) {
		t.Helper()
		rendered := renderElementText(fixture.root.renderToolObservation(fixture.message), 40, 200)
		if !strings.Contains(subagentLayoutRemoveWhitespace(rendered), subagentLayoutRemoveWhitespace(conclusion)) {
			t.Fatalf("%s terminal card did not show the complete Agent conclusion:\n%s", stage, rendered)
		}
		assertNoEvidenceOrRunRecord(stage, rendered)
	}

	assertExpanded("initial")
	segmentID := subagentProgressSegmentID(fixture.activity)
	if !fixture.root.toggleToolSegmentByID(segmentID) {
		t.Fatal("terminal Agent title click did not collapse the card")
	}
	collapsed := renderElementText(fixture.root.renderToolObservation(fixture.message), 40, 200)
	for _, hidden := range []string{"terminal-result-01", "FINAL_CONCLUSION"} {
		if strings.Contains(collapsed, hidden) {
			t.Fatalf("collapsed terminal card retained conclusion text %q:\n%s", hidden, collapsed)
		}
	}
	assertNoEvidenceOrRunRecord("collapsed", collapsed)

	if !fixture.root.toggleToolSegmentByID(segmentID) {
		t.Fatal("terminal Agent title click did not re-expand the card")
	}
	assertExpanded("re-expanded")
}

func TestSubagentShowAllDoesNotAcknowledgePendingResult(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityReadyReview, true)
	fixture.root.state.SetTranscriptShowAll(true)
	rendered := renderElementText(fixture.root.renderToolObservation(fixture.message), 120, 12)
	if summary := i18n.Text(i18n.LangEN, i18n.KeySubagentSegmentResultSummary); !strings.Contains(rendered, summary) {
		t.Fatalf("show-all did not expand terminal result summary %q: %q", summary, rendered)
	}

	activity, ok := fixture.root.state.GetActivity(fixture.activity.ID)
	if !ok || activity.Lifecycle != ActivityLifecycleCompleted || !activity.Attention.Unread || activity.Acknowledged {
		t.Fatalf("automatic show-all consumed pending-result attention: %+v ok=%t", activity, ok)
	}
}

func TestAttachToolObservationDetailKeepsOneTranscriptAnchorAndTypedPresentation(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", WorkUnitID: "work", ActorID: "assistant", Outcome: OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "agent-call", Name: "Agent", Input: map[string]any{"description": "inspect"}}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: "agent-call", Outcome: types.ToolOutcomeSucceeded,
		Data:    map[string]any{"kind": "completed", "content": []any{map[string]any{"type": "text", "text": "typed result"}}},
		Content: "typed result\n<usage>total_tokens: 1</usage>",
	}); err != nil {
		t.Fatal(err)
	}
	messagesBefore := state.Messages.Get()
	observationBefore, ok := state.GetObservation(messagesBefore[0].ObservationID)
	if !ok {
		t.Fatal("Agent observation missing")
	}
	ref, err := state.RetainDetailForEpoch("session", 1, "background:agent:transcript", []byte("transcript evidence"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AttachToolObservationDetailForEpoch("session", 1, "agent-call", ref); err != nil {
		t.Fatal(err)
	}
	if messagesAfter := state.Messages.Get(); len(messagesAfter) != len(messagesBefore) {
		t.Fatalf("attaching run evidence changed transcript anchors: before=%d after=%d", len(messagesBefore), len(messagesAfter))
	}
	observationAfter, ok := state.GetObservation(observationBefore.ID)
	if !ok || len(observationAfter.ResultRefs) != len(observationBefore.ResultRefs)+1 {
		t.Fatalf("attached run evidence missing: before=%+v after=%+v", observationBefore.ResultRefs, observationAfter.ResultRefs)
	}
	if observationAfter.Presentation.Summary != observationBefore.Presentation.Summary || !strings.Contains(strings.Join(observationAfter.Presentation.DetailLines, "\n"), "typed result") {
		t.Fatalf("attaching evidence replaced typed presentation: before=%+v after=%+v", observationBefore.Presentation, observationAfter.Presentation)
	}
	visibleEvidence, err := state.ObservationEvidence(observationBefore.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(visibleEvidence), "transcript evidence") || !strings.Contains(string(visibleEvidence), "typed result") {
		t.Fatalf("Agent evidence view exposed complete run record or lost conclusion: %q", visibleEvidence)
	}
}

func TestUpdateToolObservationAgentResultReplacesAsyncLaunchWithTypedPreview(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", WorkUnitID: "work", ActorID: "assistant", Outcome: OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "agent-call", Name: "Agent", Input: map[string]any{"description": "inspect", "run_in_background": true}}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: "agent-call", Outcome: types.ToolOutcomeSucceeded,
		Data:    map[string]any{"kind": "partial", "status": "async_launched", "agentId": "agent-secret"},
		Content: "agentId: agent-secret\n<usage>duration_ms: 1</usage>",
	}); err != nil {
		t.Fatal(err)
	}
	before := len(state.Messages.Get())
	if _, err := state.UpdateToolObservationAgentResultForEpoch("session", 1, "agent-call", "FINAL-BACKGROUND-RESULT"); err != nil {
		t.Fatal(err)
	}
	if got := len(state.Messages.Get()); got != before {
		t.Fatalf("typed result update changed transcript anchors: before=%d after=%d", before, got)
	}
	observation, ok := state.GetObservation(state.Messages.Get()[0].ObservationID)
	if !ok {
		t.Fatal("Agent observation missing")
	}
	details := strings.Join(observation.Presentation.DetailLines, "\n")
	if !strings.Contains(details, "FINAL-BACKGROUND-RESULT") {
		t.Fatalf("typed final result missing: %+v", observation.Presentation)
	}
	for _, forbidden := range []string{"agent-secret", "agentId", "<usage>", "duration_ms"} {
		if strings.Contains(details, forbidden) {
			t.Fatalf("typed final result leaked %q: %q", forbidden, details)
		}
	}
}
