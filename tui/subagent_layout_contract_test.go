package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

const (
	subagentLayoutTask       = "weather-check"
	subagentLayoutLongOutput = "oldest progress line\nsecond progress line\nthird progress line\nlatest meaningful progress"
	subagentLayoutConclusion = "Forecast summary for the parent."
)

type subagentLayoutContractFixture struct {
	root        *RootComponent
	message     Message
	observation Observation
	activity    Activity
}

func (fixture subagentLayoutContractFixture) setExpanded(expanded bool) {
	fixture.root.state.SetToolSegmentExpanded(subagentProgressSegmentID(fixture.activity), expanded)
}

func newSubagentLayoutContractFixture(t *testing.T, state ActivityState, withResult bool) subagentLayoutContractFixture {
	t.Helper()
	return newSubagentLayoutContractFixtureWithConclusion(t, state, withResult, subagentLayoutConclusion)
}

func newSubagentLayoutContractFixtureWithConclusion(t *testing.T, state ActivityState, withResult bool, conclusion string) subagentLayoutContractFixture {
	t.Helper()

	const (
		sessionID = "layout-session"
		toolUseID = "layout-agent-call"
	)
	appState := NewAppState()
	appState.Language.Set(i18n.LangEN)
	appState.SessionID.Set(sessionID)
	appState.SessionEpoch.Set(1)
	appState.Activities = NewActivityStore(ActivityScope{SessionID: sessionID, Epoch: 1})
	ctx := ToolEventContext{
		SessionID:  sessionID,
		TurnID:     "layout-turn",
		WorkUnitID: "layout-work",
		ActorID:    "assistant",
	}
	if err := appState.ApplyToolCall(ctx, types.ToolUseBlock{
		ID:   toolUseID,
		Name: "Agent",
		Input: map[string]any{
			"description":   subagentLayoutTask,
			"subagent_type": "search-specialist-with-a-long-profile-name",
		},
	}); err != nil {
		t.Fatal(err)
	}

	outcome := OutcomeRunning
	phase := "assistant"
	if state == ActivityCompleted || state == ActivityReadyReview {
		outcome = OutcomeSucceeded
		phase = "completed"
	}
	if err := appState.ApplyActivity(ActivityEvent{
		ID: "background:layout-child", RunID: "layout-run-1",
		SessionID: sessionID, Epoch: 1, TurnID: ctx.TurnID, WorkUnitID: "layout-child",
		Kind: ActivityAgent, Name: "Agent", State: state, Outcome: outcome,
		Progress: ActivityProgress{
			Current: 11, AgentID: "agent-layout-child",
			AgentType: "search-specialist-with-a-long-profile-name", ParentToolUseID: toolUseID,
			Phase: phase, LatestTool: "Bash", Output: subagentLayoutLongOutput,
			ElapsedMs: 54_300, TokensUsed: 145_555,
		},
	}); err != nil {
		t.Fatal(err)
	}

	if withResult {
		ctx.Outcome = OutcomeSucceeded
		if err := appState.ApplyToolResult(ctx, types.ToolResultBlock{
			ToolUseID: toolUseID,
			Content: conclusion + "\n" +
				"agentId: agent-layout-child\n" +
				"<usage>total_tokens: 145555\ntool_uses: 17\nduration_ms: 54300</usage>",
			Data: map[string]any{
				"kind":              "completed",
				"status":            "completed",
				"agentId":           "agent-layout-child",
				"agentType":         "search-specialist-with-a-long-profile-name",
				"content":           []any{map[string]any{"type": "text", "text": conclusion}},
				"durationMs":        54_300,
				"totalTokens":       145_555,
				"totalToolUseCount": 17,
				"transcriptPath":    "/tmp/agent-layout-child.jsonl",
			},
			Outcome: types.ToolOutcomeSucceeded,
		}); err != nil {
			t.Fatal(err)
		}
	}

	messages := appState.Messages.Get()
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want one Agent observation", len(messages))
	}
	root := NewRootComponent(appState, nil, nil)
	observation, ok := appState.GetObservation(messages[0].ObservationID)
	if !ok {
		t.Fatal("Agent observation missing")
	}
	activity, ok := root.subagentActivityForObservation(observation)
	if !ok {
		t.Fatal("correlated subagent activity missing")
	}
	return subagentLayoutContractFixture{root: root, message: messages[0], observation: observation, activity: activity}
}

func TestSubagentLayoutContractRunningCardShowsAtMostTenPhysicalOutputRows(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityRunning, false)
	liveLines := make([]string, 12)
	for index := range liveLines {
		liveLines[index] = fmt.Sprintf("live-line-%02d", index+1)
	}
	fixture.activity.Progress.Output = strings.Join(liveLines, "\n")
	card := fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, fixture.activity)

	const width = 80
	rendered := renderElementText(card, width, 30)
	var outputRows []string
	for _, line := range subagentLayoutNonEmptyTextLines(rendered) {
		if strings.Contains(line, "live-line-") {
			outputRows = append(outputRows, line)
		}
	}
	if len(outputRows) != 10 {
		t.Fatalf("80-column running card uses %d physical output rows, want 10:\n%s", len(outputRows), rendered)
	}
	if strings.Contains(rendered, "live-line-01") || strings.Contains(rendered, "live-line-02") {
		t.Fatalf("running card did not retain the latest ten output rows:\n%s", rendered)
	}
	if !strings.Contains(rendered, "live-line-03") || !strings.Contains(rendered, "live-line-12") {
		t.Fatalf("running card lost rows from the latest ten-row window:\n%s", rendered)
	}
	if heading := "Live output"; strings.Contains(subagentLayoutRemoveWhitespace(rendered), subagentLayoutRemoveWhitespace(heading)) {
		t.Fatalf("running card still shows the fixed output heading %q:\n%s", heading, rendered)
	}
}

func TestSubagentLayoutContractRunningCardOmitsFixedOutputHeadingInRuntimeLanguage(t *testing.T) {
	tests := []struct {
		name            string
		lang            i18n.Language
		obsoleteHeading string
	}{
		{name: "English", lang: i18n.LangEN, obsoleteHeading: "Live output"},
		{name: "Chinese", lang: i18n.LangZH, obsoleteHeading: "实时输出"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSubagentLayoutContractFixture(t, ActivityRunning, false)
			fixture.root.state.Language.Set(test.lang)
			fixture.activity.Progress.Output = "latest-agent-progress"
			rendered := renderElementText(fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, fixture.activity), 80, 20)
			if !strings.Contains(rendered, "latest-agent-progress") {
				t.Fatalf("running card lost progress output:\n%s", rendered)
			}
			if heading := test.obsoleteHeading; strings.Contains(subagentLayoutRemoveWhitespace(rendered), subagentLayoutRemoveWhitespace(heading)) {
				t.Fatalf("running card still shows the fixed output heading %q:\n%s", heading, rendered)
			}
		})
	}
}

func TestSubagentLayoutContractRunningCardWithProgressDoesNotFallBackToWaiting(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityRunning, false)
	fixture.activity.Progress.Output = ""
	fixture.root.termWidth = 120
	rendered := renderElementText(fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, fixture.activity), 120, 20)

	wantPhase := localizedSubagentProgressPhase(i18n.LangEN, fixture.activity.Progress.Phase)
	wantTool := i18n.Format(i18n.LangEN, i18n.KeyREPLTUIToolName, fixture.activity.Progress.LatestTool)
	for _, want := range []string{wantPhase, wantTool} {
		if want == "" || !strings.Contains(rendered, want) {
			t.Fatalf("running card lost structured progress %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{
		"Live output",
		i18n.Text(i18n.LangEN, i18n.KeySubagentSegmentWaiting),
	} {
		if strings.Contains(subagentLayoutRemoveWhitespace(rendered), subagentLayoutRemoveWhitespace(forbidden)) {
			t.Errorf("running card with tool/phase progress still shows placeholder %q:\n%s", forbidden, rendered)
		}
	}
}

func TestSubagentLayoutContractRunningToolStreamKeepsLatestTenVisualRows(t *testing.T) {
	const toolCall = `TOOL_CALL Bash command="go test ./tui -run TestSubagentLayoutContract" timeout_ms=30000`
	responseLines := []string{
		"TOOL_RESPONSE line-01 package=tui",
		"TOOL_RESPONSE line-02 tests=42 passed=42",
		"TOOL_RESPONSE line-03 status=success",
		"FINAL_RESPONSE_LINE",
	}
	streamLines := make([]string, 0, 8+1+len(responseLines))
	for index := 1; index <= 8; index++ {
		streamLines = append(streamLines, fmt.Sprintf("STALE_PROGRESS_%02d", index))
	}
	streamLines = append(streamLines, toolCall)
	streamLines = append(streamLines, responseLines...)
	stream := strings.Join(streamLines, "\n")
	wantFirstStaleByWidth := map[int]int{40: 7, 80: 5, 120: 4}

	languages := []struct {
		name string
		lang i18n.Language
	}{
		{name: "English", lang: i18n.LangEN},
		{name: "Chinese", lang: i18n.LangZH},
	}
	for _, language := range languages {
		t.Run(language.name, func(t *testing.T) {
			fixture := newSubagentLayoutContractFixture(t, ActivityRunning, false)
			fixture.root.state.Language.Set(language.lang)
			fixture.activity.Progress.Output = stream
			fixture.activity.Progress.DroppedCount = 7
			for _, width := range []int{40, 80, 120} {
				t.Run(subagentLayoutWidthName(width), func(t *testing.T) {
					fixture.root.termWidth = width
					card := fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, fixture.activity)
					rendered := renderElementText(card, width, 40)
					if rows := subagentLayoutPhysicalRowsForOutput(rendered, stream); rows != 10 {
						t.Fatalf("%d-column tool stream uses %d visual output rows, want the latest 10:\n%s", width, rows, rendered)
					}
					for index := 1; index <= 8; index++ {
						staleLine := fmt.Sprintf("STALE_PROGRESS_%02d", index)
						visible := strings.Contains(rendered, staleLine)
						wantVisible := index >= wantFirstStaleByWidth[width]
						if visible != wantVisible {
							t.Fatalf("%d-column latest visual window visibility for %q = %v, want %v:\n%s", width, staleLine, visible, wantVisible, rendered)
						}
					}
					compact := subagentLayoutRemoveWhitespace(rendered)
					if !strings.Contains(compact, subagentLayoutRemoveWhitespace(toolCall)) {
						t.Fatalf("%d-column tool stream collapsed the call to its tool name and lost core arguments:\n%s", width, rendered)
					}
					for _, responseLine := range responseLines {
						if !strings.Contains(compact, subagentLayoutRemoveWhitespace(responseLine)) {
							t.Fatalf("%d-column tool stream lost response line %q:\n%s", width, responseLine, rendered)
						}
					}
					for _, coalesced := range []string{"7 updates coalesced", "已合并 7 次更新"} {
						if strings.Contains(subagentLayoutRemoveWhitespace(collectElementText(card)), subagentLayoutRemoveWhitespace(coalesced)) {
							t.Fatalf("%d-column tool stream still shows coalesced-update metadata %q:\n%s", width, coalesced, rendered)
						}
					}
				})
			}
		})
	}
}

func TestSubagentLayoutContractRunningOutputWrapsWithoutTruncation(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "ASCII", output: "ASCIIWRAPBEGIN" + strings.Repeat("abcdefghij", 15) + "ASCIIWRAPEND"},
		{name: "CJK", output: "中文换行开始" + strings.Repeat("天气预报结果", 10) + "中文换行结束"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSubagentLayoutContractFixture(t, ActivityRunning, false)
			fixture.activity.Progress.Output = test.output
			for _, width := range []int{40, 80, 120} {
				t.Run(subagentLayoutWidthName(width), func(t *testing.T) {
					fixture.root.termWidth = width
					card := fixture.root.renderSubagentProgressSegment(fixture.message, fixture.observation, fixture.activity)
					rendered := renderElementText(card, width, 30)
					compact := subagentLayoutRemoveWhitespace(rendered)
					if !strings.Contains(compact, test.output) {
						t.Fatalf("%d-column running output was truncated instead of wrapped:\n%s", width, rendered)
					}
					physicalRows := subagentLayoutPhysicalRowsForOutput(rendered, test.output)
					if physicalRows <= 1 {
						t.Fatalf("%d-column long %s output did not wrap: rows=%d\n%s", width, test.name, physicalRows, rendered)
					}
					if physicalRows > 10 {
						t.Fatalf("%d-column long %s output uses %d physical rows, want at most 10:\n%s", width, test.name, physicalRows, rendered)
					}
				})
			}
		})
	}
}

func TestSubagentLayoutContractCompletedCardShowsConclusionWithoutRunRecord(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityCompleted, true)
	fixture.setExpanded(true)

	for _, width := range []int{40, 80, 120} {
		t.Run(subagentLayoutWidthName(width), func(t *testing.T) {
			fixture.root.termWidth = width
			card := fixture.root.renderToolObservation(fixture.message)
			rendered := renderElementText(card, width, 30)
			if !strings.Contains(subagentLayoutRemoveWhitespace(rendered), subagentLayoutRemoveWhitespace(subagentLayoutConclusion)) {
				t.Fatalf("%d-column completed card does not show the Agent conclusion:\n%s", width, rendered)
			}
			for _, forbidden := range []string{"Run record", "View run record"} {
				if strings.Contains(rendered, forbidden) {
					t.Fatalf("%d-column completed card still exposes %q:\n%s", width, forbidden, rendered)
				}
			}
		})
	}
}

func TestSubagentLayoutContractCompletedCardShowsCompleteWrappedResult(t *testing.T) {
	resultLines := make([]string, 0, 13)
	for index := 1; index <= 12; index++ {
		line := fmt.Sprintf("terminal-result-%02d", index)
		if index == 6 {
			line += "-ASCII-" + strings.Repeat("abcdefghij", 15) + "-中文自动换行不截断"
		}
		resultLines = append(resultLines, line)
	}
	resultLines = append(resultLines, "FINAL_CONCLUSION")
	longConclusion := strings.Join(resultLines, "\n")
	fixture := newSubagentLayoutContractFixtureWithConclusion(t, ActivityCompleted, true, longConclusion)
	fixture.setExpanded(true)

	for _, width := range []int{40, 80, 120} {
		t.Run(subagentLayoutWidthName(width), func(t *testing.T) {
			fixture.root.termWidth = width
			rendered := renderElementText(fixture.root.renderToolObservation(fixture.message), width, 80)
			if !strings.Contains(subagentLayoutRemoveWhitespace(rendered), subagentLayoutRemoveWhitespace(longConclusion)) {
				t.Fatalf("%d-column completed card truncated the Agent result instead of showing it in full:\n%s", width, rendered)
			}
			for index := 1; index <= 12; index++ {
				want := fmt.Sprintf("terminal-result-%02d", index)
				if !strings.Contains(rendered, want) {
					t.Fatalf("%d-column completed card lost result row %q:\n%s", width, want, rendered)
				}
			}
			if !strings.Contains(rendered, "FINAL_CONCLUSION") {
				t.Fatalf("%d-column completed card lost the final conclusion:\n%s", width, rendered)
			}
			physicalRows := subagentLayoutPhysicalRowsForOutput(rendered, longConclusion)
			if physicalRows <= len(resultLines) {
				t.Fatalf("%d-column completed card did not wrap its long result line: rows=%d logical=%d\n%s", width, physicalRows, len(resultLines), rendered)
			}
			for _, forbidden := range []string{"Run record", "View run record"} {
				if strings.Contains(rendered, forbidden) {
					t.Fatalf("%d-column completed card still exposes %q:\n%s", width, forbidden, rendered)
				}
			}
		})
	}
}

func TestSubagentLayoutContractTerminalCardOmitsLiveAndRawProtocol(t *testing.T) {
	fixture := newSubagentLayoutContractFixture(t, ActivityCompleted, true)
	rendered := renderElementText(fixture.root.renderToolObservation(fixture.message), 120, 30)

	for _, forbidden := range []string{
		"Live output",
		"oldest progress line",
		"latest meaningful progress",
		"agentId:",
		"<usage>",
		"total_tokens",
		"tool_uses:",
		"duration_ms:",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("terminal card leaked %q:\n%s", forbidden, rendered)
		}
	}
}

func TestSubagentLayoutContractKeepsStatusAndTaskVisibleAtSupportedWidths(t *testing.T) {
	tests := []struct {
		name  string
		state ActivityState
	}{
		{name: "running", state: ActivityRunning},
		{name: "completed", state: ActivityCompleted},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newSubagentLayoutContractFixture(t, test.state, test.state == ActivityCompleted)
			status := i18n.RuntimeActivityStateLabel(i18n.LangEN, string(test.state))
			for _, width := range []int{40, 80, 120} {
				t.Run(subagentLayoutWidthName(width), func(t *testing.T) {
					card := fixture.root.renderToolObservation(fixture.message)
					rendered := renderElementText(card, width, 10)
					if !strings.Contains(rendered, status) {
						t.Errorf("%d-column card lost status %q:\n%s", width, status, rendered)
					}
					if !strings.Contains(rendered, subagentLayoutTask) {
						t.Errorf("%d-column card lost task %q:\n%s", width, subagentLayoutTask, rendered)
					}
				})
			}
		})
	}
}

func TestSubagentLayoutContractEarlyBackgroundLaunchUsesSafeProvisionalCard(t *testing.T) {
	const (
		sessionID = "early-background-session"
		toolUseID = "early-background-agent-call"
		agentID   = "agent-early-hidden-id"
	)
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionID.Set(sessionID)
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: sessionID, Epoch: 1})
	ctx := ToolEventContext{
		SessionID:  sessionID,
		TurnID:     "early-background-turn",
		WorkUnitID: "early-background-work",
		ActorID:    "assistant",
	}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{
		ID:   toolUseID,
		Name: "Agent",
		Input: map[string]any{
			"description":       subagentLayoutTask,
			"subagent_type":     "search-specialist",
			"run_in_background": true,
		},
	}); err != nil {
		t.Fatal(err)
	}

	ctx.Outcome = OutcomeSucceeded
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: toolUseID,
		Content: "Async agent launched successfully.\n" +
			"agentId: " + agentID + " (internal ID - do not mention to user. Use SendMessage to continue.)\n" +
			"output_file: /tmp/" + agentID + "/output.txt\n" +
			"The agent is working in the background.",
		Data: map[string]any{
			"kind":              "partial",
			"status":            "async_launched",
			"isAsync":           true,
			"description":       subagentLayoutTask,
			"prompt":            "Find tomorrow's weather.",
			"agentId":           agentID,
			"agentType":         "search-specialist",
			"outputFile":        "/tmp/" + agentID + "/output.txt",
			"canReadOutputFile": true,
			"message":           "Use SendMessage with to: '" + agentID + "' to continue this agent.",
		},
		Outcome: types.ToolOutcomeSucceeded,
	}); err != nil {
		t.Fatal(err)
	}

	messages := state.Messages.Get()
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want one provisional Agent observation", len(messages))
	}
	root := NewRootComponent(state, nil, nil)
	observation, ok := state.GetObservation(messages[0].ObservationID)
	if !ok {
		t.Fatal("provisional Agent observation missing")
	}
	if activity, found := root.subagentActivityForObservation(observation); found {
		t.Fatalf("early background fixture unexpectedly has correlated activity: %+v", activity)
	}

	wantStatus := i18n.RuntimeActivityStateLabel(i18n.LangEN, string(ActivityRunning))
	for _, width := range []int{40, 80, 120} {
		t.Run(subagentLayoutWidthName(width), func(t *testing.T) {
			rendered := renderElementText(root.renderToolObservation(messages[0]), width, 10)
			if !strings.Contains(rendered, wantStatus) {
				t.Errorf("%d-column provisional card lost structured background status %q:\n%s", width, wantStatus, rendered)
			}
			if !strings.Contains(rendered, subagentLayoutTask) {
				t.Errorf("%d-column provisional card lost task %q:\n%s", width, subagentLayoutTask, rendered)
			}
			if count := strings.Count(rendered, subagentLayoutTask); count != 1 {
				t.Errorf("%d-column provisional card rendered task %d times, want exactly once:\n%s", width, count, rendered)
			}
			if lines := subagentLayoutNonEmptyTextLines(rendered); len(lines) > 4 {
				t.Errorf("%d-column provisional card uses %d visible rows, want at most 4:\n%s", width, len(lines), rendered)
			}
			for _, forbidden := range []string{
				"⚡",
				agentID,
				"Async agent launched successfully",
				"agentId",
				"async_launched",
				"partial",
				"internal ID",
				"SendMessage",
				"output_file",
				"/tmp/",
			} {
				if strings.Contains(rendered, forbidden) {
					t.Errorf("%d-column provisional card leaked %q:\n%s", width, forbidden, rendered)
				}
			}
		})
	}
}

func subagentLayoutVisibleLines(element *gtui.Element, width, height int) []string {
	rendered := renderElementText(element, width, height)
	return subagentLayoutNonEmptyTextLines(rendered)
}

func subagentLayoutNonEmptyTextLines(rendered string) []string {
	lines := make([]string, 0, strings.Count(rendered, "\n")+1)
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.TrimRight(line, " ")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func subagentLayoutRemoveWhitespace(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}

func subagentLayoutPhysicalRowsForOutput(rendered, output string) int {
	rows := 0
	compactOutput := subagentLayoutRemoveWhitespace(output)
	for _, line := range strings.Split(rendered, "\n") {
		compact := subagentLayoutRemoveWhitespace(line)
		if compact != "" && strings.Contains(compactOutput, compact) {
			rows++
		}
	}
	return rows
}

func subagentLayoutWidthName(width int) string {
	switch width {
	case 40:
		return "40_columns"
	case 80:
		return "80_columns"
	case 120:
		return "120_columns"
	default:
		return "width"
	}
}
