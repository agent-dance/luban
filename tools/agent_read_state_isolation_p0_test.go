package tools

import (
	"context"
	"testing"

	loopapi "github.com/agent-dance/luban/loop"
)

func TestP0AgentFileToolsShareClonedReadStateWithoutSharingParent(t *testing.T) {
	parent := NewReadFileState()
	parent.Set("/tmp/parent-read.txt", ReadFileEntry{TimestampMs: 1, Content: "parent"})
	cache := make(map[*ReadFileState]*ReadFileState)
	plans := make(map[*PlanState]*PlanState)

	read := cloneAgentRuntimeFileTool(&FileReadTool{ReadState: parent}, nil, nil, "", plans, cache).(*FileReadTool)
	edit := cloneAgentRuntimeFileTool(&FileEditTool{ReadState: parent}, nil, nil, "", plans, cache).(*FileEditTool)
	write := cloneAgentRuntimeFileTool(&FileWriteTool{ReadState: parent}, nil, nil, "", plans, cache).(*FileWriteTool)
	notebook := cloneAgentRuntimeFileTool(&NotebookEditTool{ReadState: parent}, nil, nil, "", plans, cache).(*NotebookEditTool)

	if read.ReadState == parent || edit.ReadState != read.ReadState || write.ReadState != read.ReadState || notebook.ReadState != read.ReadState {
		t.Fatalf("agent file tools did not share one actor-local clone: parent=%p read=%p edit=%p write=%p notebook=%p", parent, read.ReadState, edit.ReadState, write.ReadState, notebook.ReadState)
	}
	if inherited, ok := read.ReadState.Get("/tmp/parent-read.txt"); !ok || inherited.Content != "parent" {
		t.Fatalf("agent clone did not inherit visible parent evidence: %+v %v", inherited, ok)
	}
	read.ReadState.Set("/tmp/child-only.txt", ReadFileEntry{TimestampMs: 2})
	if _, ok := parent.Get("/tmp/child-only.txt"); ok {
		t.Fatal("child evidence leaked back into parent state")
	}
}

func TestP0ForgedLoopContextCannotCreateReadEvidence(t *testing.T) {
	state := NewReadFileState()
	ctx := loopapi.WithToolExecutionContext(context.Background(), loopapi.ToolExecutionContext{
		SessionID: "forged-session", ActorID: "forged-agent",
	})
	state.RecordReadForContext(ctx, "/tmp/forged.txt", ReadFileEntry{
		TimestampMs: 1, CoverageKnown: true, CoverageComplete: true, FullSnapshot: true,
	})
	if state.Len() != 0 {
		t.Fatal("forged loop context created read evidence")
	}
}
