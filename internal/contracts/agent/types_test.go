package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestRuntimeNotificationRoundTripPreservesHookEvidence(t *testing.T) {
	recordedAt := time.Date(2026, time.July, 25, 8, 30, 0, 0, time.UTC)
	original := RuntimeNotification{
		ID: "notification-1", Kind: "completed", TaskID: "agent-1", RunID: "run-1",
		Usage: &types.Usage{InputTokens: 10, OutputTokens: 4}, CreatedAt: recordedAt,
		HookExecutions: []HookExecutionReceipt{{
			HookType: "Notification", ExecutionID: "hook-1", ConfigID: "config-1",
			Hook:   Hook{Type: "Notification", Kind: "command", Command: "notify"},
			Input:  HookInput{TaskID: "agent-1", Message: "done"},
			Output: HookOutput{ExitCode: 0, Stdout: "ok"}, RecordedAt: recordedAt,
		}},
	}
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RuntimeNotification
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TaskID != original.TaskID || decoded.RunID != original.RunID || len(decoded.HookExecutions) != 1 {
		t.Fatalf("notification identity changed after round trip: %#v", decoded)
	}
	receipt := decoded.HookExecutions[0]
	if receipt.Hook.Command != "notify" || receipt.Input.Message != "done" || receipt.Output.Stdout != "ok" {
		t.Fatalf("hook evidence changed after round trip: %#v", receipt)
	}
}

func TestTaskSnapshotUsesCanonicalProgressAndOutcomeVocabulary(t *testing.T) {
	snapshot := TaskSnapshot{
		ID: "agent-1", Type: TaskTypeLocalAgent, Outcome: RunOutcomeSucceeded,
		LatestProgress: &ProgressEvent{Phase: ProgressCompleted},
	}
	if snapshot.Type != "local_agent" || snapshot.Outcome != "succeeded" || snapshot.LatestProgress.Phase != "completed" {
		t.Fatalf("unexpected canonical snapshot vocabulary: %#v", snapshot)
	}
}
