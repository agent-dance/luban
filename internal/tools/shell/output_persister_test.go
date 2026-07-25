package shell

import (
	"testing"
)

type recordingOutputPersister struct {
	root    string
	content []byte
}

func (p *recordingOutputPersister) PersistShellOutput(root string, content []byte, _ int64, _ string) (PersistedOutput, error) {
	p.root = root
	p.content = append([]byte(nil), content...)
	return PersistedOutput{
		Path:         root + "/persisted-output.txt",
		OriginalSize: int64(len(content)),
		ModelText:    "persisted output receipt",
	}, nil
}

func TestBashLargeOutputUsesInjectedPersister(t *testing.T) {
	requireBashAvailable(t)
	root := t.TempDir()
	persister := &recordingOutputPersister{}
	tool := &BashTool{CWD: root, OutputPersister: persister}
	result, err := executeApprovedBashForTest(t, tool, map[string]any{
		"command": "head -c 40000 /dev/zero | tr '\\0' x",
	})
	if err != nil || result.IsError {
		t.Fatalf("large output result=%#v err=%v", result, err)
	}
	if persister.root != root || len(persister.content) != 40000 {
		t.Fatalf("persister received root=%q bytes=%d", persister.root, len(persister.content))
	}
	if result.Content != "persisted output receipt" || result.Metadata["persistedOutputPath"] == "" || result.Metadata["persistedOutputSize"] != "40000" {
		t.Fatalf("persisted output receipt=%#v", result)
	}
	output, ok := result.Data.(*BashOutput)
	if !ok || output.PersistedOutputSize != 40000 || output.PersistedOutputPath == "" {
		t.Fatalf("typed persisted output=%T %#v", result.Data, result.Data)
	}
}
