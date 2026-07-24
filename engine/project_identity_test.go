package engine

import (
	"context"
	"testing"
)

func TestEngineQueryProjectRootOverridesNestedExecutionCWDForEvents(t *testing.T) {
	const projectRoot = "/workspace/project"
	eng, err := New(Config{
		Provider: &mockProvider{name: "project-identity", modelID: "project-identity-model"},
		Sessions: newMemorySessionManager(), ProjectRoot: "/workspace/default", CWD: "/workspace/default/nested",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Shutdown(context.Background())

	events, err := eng.Query(context.Background(), QueryRequest{
		SessionID: "session-project", Message: "hello", ProjectRoot: projectRoot, CWD: "/workspace/project/nested",
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for event := range events {
		if event.Final {
			if event.Error != nil {
				t.Fatal(event.Error)
			}
			continue
		}
		seen++
		if event.Inner.ProjectRoot != projectRoot {
			t.Fatalf("event %s project root = %q, want %q", event.Inner.Type, event.Inner.ProjectRoot, projectRoot)
		}
	}
	if seen == 0 {
		t.Fatal("engine query emitted no events")
	}
}
