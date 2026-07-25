package file

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/types"
)

func seedCanonicalFileReadState(t testing.TB, state *ReadFileState, path string) {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	reader := &FileReadTool{AllowedDirs: []string{filepath.Dir(abs)}, ReadState: state}
	result, err := reader.Execute(context.Background(), map[string]any{"file_path": abs})
	if err != nil || result.IsError {
		t.Fatalf("seed canonical read state: result=%+v err=%v", result, err)
	}
}

func recordStrongReadEvidenceForTest(t testing.TB, state *ReadFileState, path string) {
	t.Helper()
	seedCanonicalFileReadState(t, state, path)
}

type testPlanMode struct {
	active bool
}

func (p testPlanMode) IsActive() bool { return p.active }

type testRuntimeProvider struct {
	snapshot types.ToolRuntimeContext
}

func (p testRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return p.snapshot
}
