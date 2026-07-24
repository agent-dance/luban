package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type fakeGoalToolRuntime struct {
	current   *goal.Goal
	loadCalls int
	saveCalls int
}

func (f *fakeGoalToolRuntime) LoadGoal() (*goal.Goal, error) {
	f.loadCalls++
	if f.current == nil {
		return nil, nil
	}
	current := *f.current
	return &current, nil
}

func (f *fakeGoalToolRuntime) SaveGoal(next goal.Goal) error {
	f.saveCalls++
	saved := next
	f.current = &saved
	return nil
}

var _ GoalRuntime = (*fakeGoalToolRuntime)(nil)

type goalToolRuntimeContext struct {
	value types.ToolRuntimeContext
}

func (p goalToolRuntimeContext) ToolRuntimeContext() types.ToolRuntimeContext {
	return p.value
}

func TestGoalToolsDeclareStrictSchemasAndMetadata(t *testing.T) {
	runtime := &fakeGoalToolRuntime{}
	tests := []struct {
		tool           types.Tool
		name           string
		wantReadOnly   bool
		wantWrite      bool
		wantConcurrent bool
	}{
		{tool: NewGetGoalTool(runtime), name: "GetGoal", wantReadOnly: true, wantConcurrent: true},
		{tool: NewCreateGoalTool(runtime), name: "CreateGoal", wantWrite: true},
		{tool: NewUpdateGoalTool(runtime), name: "UpdateGoal", wantWrite: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.tool.Name(); got != test.name {
				t.Fatalf("Name() = %q, want %q", got, test.name)
			}
			definition := types.ToDefinition(test.tool)
			if !definition.Strict || !definition.InputSchema.RejectsUnknownFields() {
				t.Fatalf("%s input contract is not strict: %+v", test.name, definition)
			}
			if definition.OutputSchema == nil || definition.OutputSchema.Type != "object" {
				t.Fatalf("%s output schema = %+v, want object", test.name, definition.OutputSchema)
			}
			if definition.Metadata.ReadOnly != test.wantReadOnly ||
				definition.Metadata.Write != test.wantWrite ||
				definition.Metadata.ConcurrencySafe != test.wantConcurrent {
				t.Fatalf("%s metadata = %+v", test.name, definition.Metadata)
			}
			if definition.Metadata.MaxResultSizeChars <= 0 {
				t.Fatalf("%s has no result size budget: %+v", test.name, definition.Metadata)
			}

			reg := registry.New()
			reg.Register(test.tool)
			metadata := reg.ToolMetadata(test.name, nil)
			if metadata.ReadOnly != test.wantReadOnly || metadata.Write != test.wantWrite || metadata.ConcurrencySafe != test.wantConcurrent {
				t.Fatalf("registry metadata for %s = %+v", test.name, metadata)
			}
		})
	}

	createSchema := NewCreateGoalTool(runtime).Schema()
	objective := goalToolSchemaProperty(t, createSchema, "objective")
	if objective["type"] != "string" || objective["maxLength"] != goal.MaxObjectiveCharacters {
		t.Fatalf("CreateGoal objective schema = %+v", objective)
	}
	budget := goalToolSchemaProperty(t, createSchema, "token_budget")
	if budget["type"] != "integer" || budget["minimum"] != 1 {
		t.Fatalf("CreateGoal token_budget schema = %+v", budget)
	}
	criteria := goalToolSchemaProperty(t, createSchema, "acceptance_criteria")
	if criteria["type"] != "array" || criteria["minItems"] != 1 || criteria["maxItems"] != goal.MaxAcceptanceCriteria {
		t.Fatalf("CreateGoal acceptance_criteria schema = %+v", criteria)
	}
	if !reflect.DeepEqual(createSchema.Required, []string{"objective", "acceptance_criteria"}) {
		t.Fatalf("CreateGoal required fields = %v", createSchema.Required)
	}

	updateSchema := NewUpdateGoalTool(runtime).Schema()
	status := goalToolSchemaProperty(t, updateSchema, "status")
	if status["type"] != "string" || !reflect.DeepEqual(goalToolSchemaStrings(status["enum"]), []string{"complete", "blocked", "revise"}) {
		t.Fatalf("UpdateGoal status schema = %+v", status)
	}
	if len(updateSchema.Properties) != 3 || !reflect.DeepEqual(updateSchema.Required, []string{"status"}) {
		t.Fatalf("UpdateGoal revision schema = %+v", updateSchema)
	}
	if revision := goalToolSchemaProperty(t, updateSchema, "expected_revision"); revision["type"] != "integer" || revision["minimum"] != 1 {
		t.Fatalf("UpdateGoal expected_revision schema = %+v", revision)
	}
}

func TestGoalToolsAreAlwaysLoadedForRootAndHiddenFromAgents(t *testing.T) {
	runtime := &fakeGoalToolRuntime{}
	for _, tool := range []types.Tool{
		NewGetGoalTool(runtime),
		NewCreateGoalTool(runtime),
		NewUpdateGoalTool(runtime),
	} {
		t.Run(tool.Name(), func(t *testing.T) {
			if metadata := registry.DiscoveryMetadata(tool); !metadata.AlwaysLoad {
				t.Fatalf("%s discovery metadata = %+v, want always loaded", tool.Name(), metadata)
			}

			reg := registry.New()
			reg.Register(tool)
			reg.SetRuntimeContextProvider(goalToolRuntimeContext{value: types.ToolRuntimeContext{}})
			if !reg.IsToolEnabled(tool) {
				t.Fatalf("%s is disabled for the root agent", tool.Name())
			}

			reg.SetRuntimeContextProvider(goalToolRuntimeContext{value: types.ToolRuntimeContext{AgentID: "child-agent"}})
			if reg.IsToolEnabled(tool) {
				t.Fatalf("%s is enabled for a child agent", tool.Name())
			}
		})
	}
}

func TestGetGoalReturnsTypedStateOrExplicitlyUnset(t *testing.T) {
	active := goalToolTestGoal(t, goal.StatusActive)
	tool := NewGetGoalTool(&fakeGoalToolRuntime{current: &active})
	result := executeGoalTool(t, tool, map[string]any{})
	assertGoalToolData(t, result, &active)
	if strings.HasPrefix(strings.TrimSpace(result.Content), "{") || !strings.Contains(result.Content, active.Objective) {
		t.Fatalf("GetGoal model text = %q", result.Content)
	}
	mapped := types.MapToolResult(tool, result, "toolu_get_goal")
	if mapped.ToolUseID != "toolu_get_goal" || mapped.Data == nil || mapped.Content == "" {
		t.Fatalf("GetGoal mapped result = %+v", mapped)
	}

	unset := executeGoalTool(t, NewGetGoalTool(&fakeGoalToolRuntime{}), map[string]any{})
	assertGoalToolData(t, unset, nil)
	if !strings.Contains(strings.ToLower(unset.Content), "no goal") {
		t.Fatalf("unset GetGoal model text = %q", unset.Content)
	}
}

func TestGoalToolsRejectUnknownInputBeforeRuntimeAccess(t *testing.T) {
	tests := []struct {
		name  string
		tool  func(GoalRuntime) types.Tool
		input map[string]any
	}{
		{name: "GetGoal", tool: func(runtime GoalRuntime) types.Tool { return NewGetGoalTool(runtime) }, input: map[string]any{"extra": true}},
		{name: "CreateGoal", tool: func(runtime GoalRuntime) types.Tool { return NewCreateGoalTool(runtime) }, input: map[string]any{"objective": "ship", "acceptance_criteria": []string{"tests pass"}, "extra": true}},
		{name: "UpdateGoal", tool: func(runtime GoalRuntime) types.Tool { return NewUpdateGoalTool(runtime) }, input: map[string]any{"status": "complete", "extra": true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeGoalToolRuntime{}
			result, err := test.tool(runtime).Execute(context.Background(), test.input)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !result.IsError {
				t.Fatalf("unknown input accepted: %+v", result)
			}
			if runtime.loadCalls != 0 || runtime.saveCalls != 0 {
				t.Fatalf("runtime used before strict validation: loads=%d saves=%d", runtime.loadCalls, runtime.saveCalls)
			}
		})
	}
}

func TestCreateGoalValidatesObjectiveAndOptionalPositiveBudget(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		wantError  bool
		wantBudget int
	}{
		{name: "omitted budget", input: map[string]any{"objective": "ship release", "acceptance_criteria": []string{"tests pass"}}, wantBudget: 0},
		{name: "positive budget", input: map[string]any{"objective": "ship release", "acceptance_criteria": []string{"tests pass"}, "token_budget": 1}, wantBudget: 1},
		{name: "unicode boundary", input: map[string]any{"objective": strings.Repeat("\u754c", goal.MaxObjectiveCharacters), "acceptance_criteria": []string{"verified"}}, wantBudget: 0},
		{name: "missing objective", input: map[string]any{"acceptance_criteria": []string{"tests pass"}}, wantError: true},
		{name: "blank objective", input: map[string]any{"objective": " \t\n ", "acceptance_criteria": []string{"tests pass"}}, wantError: true},
		{name: "missing criteria", input: map[string]any{"objective": "ship release"}, wantError: true},
		{name: "blank criterion", input: map[string]any{"objective": "ship release", "acceptance_criteria": []string{" \t "}}, wantError: true},
		{name: "objective too long", input: map[string]any{"objective": strings.Repeat("\u754c", goal.MaxObjectiveCharacters+1), "acceptance_criteria": []string{"verified"}}, wantError: true},
		{name: "explicit zero budget", input: map[string]any{"objective": "ship release", "acceptance_criteria": []string{"tests pass"}, "token_budget": 0}, wantError: true},
		{name: "negative budget", input: map[string]any{"objective": "ship release", "acceptance_criteria": []string{"tests pass"}, "token_budget": -1}, wantError: true},
		{name: "fractional budget", input: map[string]any{"objective": "ship release", "acceptance_criteria": []string{"tests pass"}, "token_budget": 1.5}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeGoalToolRuntime{}
			result, err := NewCreateGoalTool(runtime).Execute(context.Background(), test.input)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.IsError != test.wantError {
				t.Fatalf("CreateGoal result = %+v, wantError=%v", result, test.wantError)
			}
			if test.wantError {
				if runtime.saveCalls != 0 {
					t.Fatalf("invalid CreateGoal saved %d times", runtime.saveCalls)
				}
				return
			}
			if runtime.current == nil || runtime.current.Status != goal.StatusActive || runtime.current.TokenBudget != test.wantBudget {
				t.Fatalf("created goal = %+v", runtime.current)
			}
			assertGoalToolData(t, result, runtime.current)
		})
	}
}

func TestCreateGoalRejectsUnfinishedGoalAndReplacesTerminalGoal(t *testing.T) {
	for _, status := range []goal.Status{goal.StatusActive, goal.StatusPaused, goal.StatusBlocked} {
		t.Run("reject "+string(status), func(t *testing.T) {
			current := goalToolTestGoal(t, status)
			runtime := &fakeGoalToolRuntime{current: &current}
			result := executeGoalToolResult(t, NewCreateGoalTool(runtime), map[string]any{"objective": "replacement", "acceptance_criteria": []string{"replacement verified"}})
			if !result.IsError || runtime.saveCalls != 0 || runtime.current.Objective != current.Objective {
				t.Fatalf("CreateGoal replaced unfinished %s goal: result=%+v goal=%+v", status, result, runtime.current)
			}
		})
	}

	for _, status := range []goal.Status{goal.StatusAchieved, goal.StatusCleared} {
		t.Run("replace "+string(status), func(t *testing.T) {
			current := goalToolTestGoal(t, status)
			runtime := &fakeGoalToolRuntime{current: &current}
			result := executeGoalTool(t, NewCreateGoalTool(runtime), map[string]any{"objective": "replacement", "acceptance_criteria": []string{"replacement verified"}})
			if runtime.saveCalls != 1 || runtime.current == nil || runtime.current.Status != goal.StatusActive || runtime.current.Objective != "replacement" {
				t.Fatalf("CreateGoal replacement = result:%+v goal:%+v", result, runtime.current)
			}
		})
	}
}

func TestUpdateGoalOnlyMarksActiveGoalCompleteOrBlocked(t *testing.T) {
	tests := []struct {
		status       string
		wantStatus   goal.Status
		wantTerminal func(goal.Goal) bool
	}{
		{status: "complete", wantStatus: goal.StatusAchieved, wantTerminal: func(value goal.Goal) bool { return value.AchievedAt != nil }},
		{status: "blocked", wantStatus: goal.StatusBlocked, wantTerminal: func(value goal.Goal) bool { return value.BlockedAt != nil }},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			current := goalToolTestGoal(t, goal.StatusActive)
			runtime := &fakeGoalToolRuntime{current: &current}
			result := executeGoalTool(t, NewUpdateGoalTool(runtime), map[string]any{"status": test.status})
			if runtime.saveCalls != 1 || runtime.current == nil || runtime.current.Status != test.wantStatus || !test.wantTerminal(*runtime.current) {
				t.Fatalf("UpdateGoal(%s) = result:%+v goal:%+v", test.status, result, runtime.current)
			}
			if runtime.current.Objective != current.Objective || runtime.current.TokenBudget != current.TokenBudget {
				t.Fatalf("UpdateGoal changed goal identity: before=%+v after=%+v", current, runtime.current)
			}
			wantKind := goal.EvaluatorReasonModelDone
			if test.status == "blocked" {
				wantKind = goal.EvaluatorReasonModelBlocked
			}
			if runtime.current.LastEvaluatorReasonKind != wantKind {
				t.Fatalf("UpdateGoal(%s) reason kind = %q, want %q", test.status, runtime.current.LastEvaluatorReasonKind, wantKind)
			}
			assertGoalToolData(t, result, runtime.current)
		})
	}

	for _, status := range []string{"", "active", "achieved", "paused", "cleared"} {
		t.Run("reject status "+status, func(t *testing.T) {
			current := goalToolTestGoal(t, goal.StatusActive)
			runtime := &fakeGoalToolRuntime{current: &current}
			result := executeGoalToolResult(t, NewUpdateGoalTool(runtime), map[string]any{"status": status})
			if !result.IsError || runtime.saveCalls != 0 {
				t.Fatalf("UpdateGoal accepted status %q: %+v", status, result)
			}
		})
	}
}

func TestUpdateGoalRejectsMissingOrInactiveGoal(t *testing.T) {
	runtime := &fakeGoalToolRuntime{}
	result := executeGoalToolResult(t, NewUpdateGoalTool(runtime), map[string]any{"status": "complete"})
	if !result.IsError || runtime.saveCalls != 0 {
		t.Fatalf("UpdateGoal accepted missing goal: %+v", result)
	}

	for _, status := range []goal.Status{goal.StatusPaused, goal.StatusBlocked, goal.StatusAchieved, goal.StatusCleared} {
		t.Run(string(status), func(t *testing.T) {
			current := goalToolTestGoal(t, status)
			runtime := &fakeGoalToolRuntime{current: &current}
			result := executeGoalToolResult(t, NewUpdateGoalTool(runtime), map[string]any{"status": "complete"})
			if !result.IsError || runtime.saveCalls != 0 || runtime.current.Status != status {
				t.Fatalf("UpdateGoal changed %s goal: result=%+v goal=%+v", status, result, runtime.current)
			}
		})
	}
}

func TestUpdateGoalCannotSelfReportCompletionWithoutAcceptedCriteria(t *testing.T) {
	current, err := goal.CreateWithCriteria("ship", []string{"tests pass", "docs updated"}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := &fakeGoalToolRuntime{current: &current}
	result := executeGoalToolResult(t, NewUpdateGoalTool(runtime), map[string]any{"status": "complete"})
	if !result.IsError || runtime.saveCalls != 0 || runtime.current.Status != goal.StatusActive {
		t.Fatalf("unevaluated completion accepted: result=%+v goal=%+v", result, runtime.current)
	}
	if !strings.Contains(strings.ToLower(result.Content), "acceptance") {
		t.Fatalf("completion error does not explain acceptance gate: %q", result.Content)
	}
}

func TestUpdateGoalLetsAgentReviseAcceptanceCriteriaWithVersionGuard(t *testing.T) {
	current := goalToolTestGoal(t, goal.StatusActive)
	runtime := &fakeGoalToolRuntime{current: &current}
	result := executeGoalTool(t, NewUpdateGoalTool(runtime), map[string]any{
		"status": "revise", "expected_revision": 1,
		"acceptance_criteria": []string{"focused tests pass", "TUI shows every acceptance status"},
	})
	if runtime.saveCalls != 1 || runtime.current == nil || runtime.current.Status != goal.StatusActive || runtime.current.Revision != 2 {
		t.Fatalf("revised goal = result:%+v goal:%+v", result, runtime.current)
	}
	if runtime.current.LastAcceptanceEvaluation != nil || len(runtime.current.AcceptanceCriteria) != 2 {
		t.Fatalf("revision retained stale evaluation or lost criteria: %+v", runtime.current)
	}
	if runtime.current.AcceptanceCriteria[0].ID != "AC-1" || runtime.current.AcceptanceCriteria[1].ID != "AC-2" ||
		!strings.Contains(result.Content, "TUI shows every acceptance status") {
		t.Fatalf("revised acceptance contract = %+v content=%q", runtime.current.AcceptanceCriteria, result.Content)
	}

	stale := executeGoalToolResult(t, NewUpdateGoalTool(runtime), map[string]any{
		"status": "revise", "expected_revision": 1, "acceptance_criteria": []string{"weakened"},
	})
	if !stale.IsError || runtime.saveCalls != 1 || runtime.current.Revision != 2 {
		t.Fatalf("stale revision changed goal: result=%+v goal=%+v", stale, runtime.current)
	}

	missingRevision := executeGoalToolResult(t, NewUpdateGoalTool(runtime), map[string]any{
		"status": "revise", "acceptance_criteria": []string{"new"},
	})
	if !missingRevision.IsError || runtime.saveCalls != 1 {
		t.Fatalf("revision without guard was accepted: %+v", missingRevision)
	}
}

func TestGoalToolsHandleNilRuntimeWithoutPanicking(t *testing.T) {
	tests := []struct {
		name  string
		tool  types.Tool
		input map[string]any
	}{
		{name: "GetGoal", tool: NewGetGoalTool(nil), input: map[string]any{}},
		{name: "CreateGoal", tool: NewCreateGoalTool(nil), input: map[string]any{"objective": "ship", "acceptance_criteria": []string{"tests pass"}}},
		{name: "UpdateGoal", tool: NewUpdateGoalTool(nil), input: map[string]any{"status": "complete"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.tool.Execute(context.Background(), test.input)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !result.IsError || !strings.Contains(strings.ToLower(result.Content), "goal runtime") {
				t.Fatalf("nil runtime result = %+v", result)
			}
		})
	}
}

func goalToolSchemaProperty(t *testing.T, schema types.JSONSchema, name string) map[string]any {
	t.Helper()
	property, ok := schema.Properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q = %#v", name, schema.Properties[name])
	}
	return property
}

func goalToolSchemaStrings(value any) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, _ := value.(string)
			out = append(out, text)
		}
		return out
	default:
		return nil
	}
}

func executeGoalTool(t *testing.T, tool types.Tool, input map[string]any) types.ToolResult {
	t.Helper()
	result := executeGoalToolResult(t, tool, input)
	if result.IsError {
		t.Fatalf("%s returned tool error: %+v", tool.Name(), result)
	}
	return result
}

func executeGoalToolResult(t *testing.T, tool types.Tool, input map[string]any) types.ToolResult {
	t.Helper()
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("%s Execute() error = %v", tool.Name(), err)
	}
	return result
}

func assertGoalToolData(t *testing.T, result types.ToolResult, want *goal.Goal) {
	t.Helper()
	if result.Data == nil {
		t.Fatal("goal tool result has no typed data")
	}
	typeOfData := reflect.TypeOf(result.Data)
	if typeOfData.Kind() == reflect.Map {
		t.Fatalf("goal tool Data is an untyped map: %#v", result.Data)
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal goal tool Data: %v", err)
	}
	var envelope struct {
		Goal *goal.Goal `json:"goal"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode goal tool Data %s: %v", encoded, err)
	}
	if !reflect.DeepEqual(envelope.Goal, want) {
		t.Fatalf("goal tool Data = %+v, want %+v (JSON %s)", envelope.Goal, want, encoded)
	}
}

func goalToolTestGoal(t *testing.T, status goal.Status) goal.Goal {
	t.Helper()
	createdAt := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	current, err := goal.Create("finish the goal tools", 10_000, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	current, err = goal.RecordAcceptanceEvaluation(current, current.Revision, []goal.AcceptanceCriterionEvaluation{{
		CriterionID: "AC-1", Met: true, Reason: "verified",
	}}, "verified", createdAt.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	changedAt := createdAt.Add(time.Minute)
	switch status {
	case goal.StatusActive:
		return current
	case goal.StatusPaused:
		current, err = goal.Pause(current, changedAt)
	case goal.StatusBlocked:
		current, err = goal.Block(current, "waiting for dependency", changedAt)
	case goal.StatusAchieved:
		current, err = goal.Achieve(current, "done", changedAt)
	case goal.StatusCleared:
		current, err = goal.Clear(current, changedAt)
	default:
		t.Fatalf("unsupported goal status %q", status)
	}
	if err != nil {
		t.Fatalf("build %s goal: %v", status, err)
	}
	return current
}
