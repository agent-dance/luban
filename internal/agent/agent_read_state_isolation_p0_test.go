package agent

import (
	"context"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
)

func TestP0AgentFileToolsShareClonedReadStateWithoutSharingParent(t *testing.T) {
	parent := toolfile.NewReadFileState()
	parent.SetForContext(context.Background(), "/tmp/parent-read.txt", toolfile.ReadFileEntry{TimestampMs: 1, Content: "parent"})
	cache := make(map[*toolfile.ReadFileState]*toolfile.ReadFileState)
	plans := make(map[*toolinteraction.PlanState]*toolinteraction.PlanState)

	read := cloneAgentRuntimeFileTool(&toolfile.FileReadTool{ReadState: parent}, nil, nil, "", plans, cache).(*toolfile.FileReadTool)
	edit := cloneAgentRuntimeFileTool(&toolfile.FileEditTool{ReadState: parent}, nil, nil, "", plans, cache).(*toolfile.FileEditTool)
	write := cloneAgentRuntimeFileTool(&toolfile.FileWriteTool{ReadState: parent}, nil, nil, "", plans, cache).(*toolfile.FileWriteTool)
	notebook := cloneAgentRuntimeFileTool(&toolfile.NotebookEditTool{ReadState: parent}, nil, nil, "", plans, cache).(*toolfile.NotebookEditTool)

	if read.ReadState == parent || edit.ReadState != read.ReadState || write.ReadState != read.ReadState || notebook.ReadState != read.ReadState {
		t.Fatalf("agent file tools did not share one actor-local clone: parent=%p read=%p edit=%p write=%p notebook=%p", parent, read.ReadState, edit.ReadState, write.ReadState, notebook.ReadState)
	}
	if inherited, ok := read.ReadState.GetForContext(context.Background(), "/tmp/parent-read.txt"); !ok || inherited.Content != "parent" {
		t.Fatalf("agent clone did not inherit visible parent evidence: %+v %v", inherited, ok)
	}
	read.ReadState.SetForContext(context.Background(), "/tmp/child-only.txt", toolfile.ReadFileEntry{TimestampMs: 2})
	if _, ok := parent.GetForContext(context.Background(), "/tmp/child-only.txt"); ok {
		t.Fatal("child evidence leaked back into parent state")
	}
}

func TestP0ForgedLoopContextCannotCreateReadEvidence(t *testing.T) {
	state := toolfile.NewReadFileState()
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		SessionID: "forged-session", ActorID: "forged-agent",
	})
	state.RecordReadForContext(ctx, "/tmp/forged.txt", toolfile.ReadFileEntry{
		TimestampMs: 1, CoverageComplete: true, FullSnapshot: true,
	})
	if _, ok := state.GetForContext(ctx, "/tmp/forged.txt"); ok {
		t.Fatal("forged loop context created read evidence")
	}
}
