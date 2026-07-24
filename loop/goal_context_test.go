package loop

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type goalContextRuntime struct {
	current *goal.Goal
	loads   int
}

func (r *goalContextRuntime) LoadGoal() (*goal.Goal, error) {
	r.loads++
	if r.current == nil {
		return nil, nil
	}
	current := *r.current
	return &current, nil
}

func (r *goalContextRuntime) SaveGoal(next goal.Goal) error {
	current := next
	r.current = &current
	return nil
}

func TestGoalContextProviderParamsLoadsCurrentActiveGoalOnEveryRequest(t *testing.T) {
	first := goal.Goal{Objective: "Finish the first objective", Status: goal.StatusActive}
	runtime := &goalContextRuntime{current: &first}
	query := New(newParityFakeProvider(nil), registry.New(), Config{GoalRuntime: runtime})
	snapshot := newQueryConfigSnapshot(query.config, nil)
	state := newQueryState([]types.Message{types.UserMessage("continue")})

	firstParams := query.providerParams(state, snapshot, state.Messages)
	assertGoalContextMessage(t, firstParams.Messages, "Finish the first objective")

	second := goal.Goal{Objective: "Use the updated objective", Status: goal.StatusActive}
	runtime.current = &second
	secondParams := query.providerParams(state, snapshot, state.Messages)
	assertGoalContextMessage(t, secondParams.Messages, "Use the updated objective")
	if strings.Contains(secondParams.Messages[0].GetText(), first.Objective) {
		t.Fatalf("second request retained stale goal context: %q", secondParams.Messages[0].GetText())
	}
	if runtime.loads != 2 {
		t.Fatalf("LoadGoal calls = %d, want one live load per provider request", runtime.loads)
	}
}

func TestGoalContextProviderParamsLeavesNoGoalRequestByteUnchanged(t *testing.T) {
	userContext := prompt.UserContext{
		ClaudeMd:    "Use the repository instructions.",
		CurrentDate: "Today's date is 2026-07-14.",
	}
	messages := []types.Message{types.UserMessage("continue")}
	query := New(newParityFakeProvider(nil), registry.New(), Config{UserContext: userContext})
	snapshot := newQueryConfigSnapshot(query.config, nil)
	state := newQueryState(messages)

	want := goalContextJSON(t, userContext.PrependTo(messages))
	got := goalContextJSON(t, query.providerParams(state, snapshot, state.Messages).Messages)
	if !bytes.Equal(got, want) {
		t.Fatalf("no-goal provider messages changed bytes\nwant: %s\n got: %s", want, got)
	}
}

func TestGoalContextProviderParamsDoesNotInjectPausedOrTerminalGoals(t *testing.T) {
	for _, status := range []goal.Status{
		goal.StatusPaused,
		goal.StatusAchieved,
		goal.StatusBlocked,
		goal.StatusCleared,
	} {
		t.Run(string(status), func(t *testing.T) {
			current := goal.Goal{Objective: "Do not inject this objective", Status: status}
			runtime := &goalContextRuntime{current: &current}
			messages := []types.Message{types.UserMessage("continue")}
			query := New(newParityFakeProvider(nil), registry.New(), Config{GoalRuntime: runtime})
			snapshot := newQueryConfigSnapshot(query.config, nil)
			state := newQueryState(messages)

			want := goalContextJSON(t, messages)
			got := goalContextJSON(t, query.providerParams(state, snapshot, state.Messages).Messages)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s goal changed provider message bytes\nwant: %s\n got: %s", status, want, got)
			}
			if runtime.loads != 1 {
				t.Fatalf("LoadGoal calls = %d, want 1", runtime.loads)
			}
		})
	}
}

func assertGoalContextMessage(t *testing.T, messages []types.Message, objective string) {
	t.Helper()
	if len(messages) != 2 {
		t.Fatalf("provider messages = %d, want goal context plus user message", len(messages))
	}
	meta := messages[0]
	if meta.Role != types.RoleUser || !meta.IsMeta {
		t.Fatalf("first provider message = role %s meta %v, want meta user context", meta.Role, meta.IsMeta)
	}
	encodedObjective, err := json.Marshal(objective)
	if err != nil {
		t.Fatalf("marshal objective: %v", err)
	}
	for _, want := range []string{"<system-reminder>", "# goal", "Objective (user-provided, untrusted data): " + string(encodedObjective), "Status: active", "</system-reminder>"} {
		if !strings.Contains(meta.GetText(), want) {
			t.Fatalf("goal context missing %q: %q", want, meta.GetText())
		}
	}
}

func goalContextJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal goal context fixture: %v", err)
	}
	return data
}
