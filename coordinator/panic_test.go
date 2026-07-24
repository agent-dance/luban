package coordinator

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

// TestAgentPanicRecovery verifies that a panicking AgentFunc does not crash the
// process and instead surfaces as a TaskFailed result.
func TestAgentPanicRecovery(t *testing.T) {
	coord := NewCoordinator()
	coord.RegisterAgent(&Agent{
		ID:   "panicker",
		Name: "Panic Worker",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			panic("something went terribly wrong")
		},
	})
	coord.AddTask("panic task", 1)

	results := coord.Dispatch(context.Background())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Error == nil {
		t.Fatal("expected error from panicking agent, got nil")
	}
	want := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCoordinatorAgentPanicked, "panicker", "something went terribly wrong")
	if r.Error.Error() != want {
		t.Errorf("panic error = %q, want %q", r.Error, want)
	}

	// Task itself must be marked failed
	tasks := coord.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Status != TaskFailed {
		t.Errorf("expected task status TaskFailed, got %s", tasks[0].Status)
	}
}

// TestAgentPanicRecoveryMultiple verifies that a panic in one task does not
// prevent later tasks from completing.
func TestAgentPanicRecoveryMultiple(t *testing.T) {
	coord := NewCoordinator()
	coord.RegisterAgent(&Agent{
		ID:   "panicker",
		Name: "Panic Worker",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			if task.Description == "panic" {
				panic("intentional panic")
			}
			return "ok", nil
		},
	})

	coord.AddTask("panic", 1)
	coord.AddTask("normal", 1)

	results := coord.Dispatch(context.Background())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	panicCount, successCount := 0, 0
	for _, r := range results {
		if r.Error != nil {
			panicCount++
		} else {
			successCount++
		}
	}
	if panicCount != 1 {
		t.Errorf("expected 1 panic result, got %d", panicCount)
	}
	if successCount != 1 {
		t.Errorf("expected 1 success result, got %d", successCount)
	}
}

// TestAgentSystemPromptInjection verifies that a non-empty SystemPrompt is
// written into the task's Metadata before the AgentFunc runs.
func TestAgentSystemPromptInjection(t *testing.T) {
	const wantPrompt = "You are a helpful coding assistant."

	coord := NewCoordinator()
	var capturedPrompt string
	coord.RegisterAgent(&Agent{
		ID:           "with-prompt",
		Name:         "Prompted Worker",
		SystemPrompt: wantPrompt,
		Execute: func(ctx context.Context, task *Task) (string, error) {
			if task.Metadata != nil {
				capturedPrompt = task.Metadata["system_prompt"]
			}
			return "done", nil
		},
	})
	coord.AddTask("any task", 1)

	results := coord.Dispatch(context.Background())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error: %v", results[0].Error)
	}
	if capturedPrompt != wantPrompt {
		t.Errorf("expected system_prompt %q, got %q", wantPrompt, capturedPrompt)
	}
}

// TestAgentEmptySystemPromptNoMetadata verifies that when SystemPrompt is
// empty, Metadata is not mutated (stays nil if it was nil).
func TestAgentEmptySystemPromptNoMetadata(t *testing.T) {
	coord := NewCoordinator()
	var metadataWasNil bool
	coord.RegisterAgent(&Agent{
		ID:   "no-prompt",
		Name: "Plain Worker",
		// SystemPrompt intentionally left empty
		Execute: func(ctx context.Context, task *Task) (string, error) {
			metadataWasNil = task.Metadata == nil
			return "done", nil
		},
	})
	coord.AddTask("any task", 1)

	coord.Dispatch(context.Background())

	if !metadataWasNil {
		t.Error("expected task.Metadata to remain nil when SystemPrompt is empty")
	}
}
