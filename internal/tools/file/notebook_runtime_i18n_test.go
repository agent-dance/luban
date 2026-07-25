package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

type notebookRuntimeProvider struct {
	snapshot types.ToolRuntimeContext
}

func (p *notebookRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return p.snapshot
}

func TestNotebookEditNormalizesRelativePathFromCurrentRuntime(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	runtime := &notebookRuntimeProvider{snapshot: types.ToolRuntimeContext{
		ProjectRoot: rootA,
		AllowedDirs: []string{rootA},
	}}
	tool := &NotebookEditTool{Runtime: runtime}
	input := map[string]any{"notebook_path": "notes/demo.ipynb", "new_source": "x", "extra": "preserved"}

	backfilled, err := tool.BackfillObservableInput(input)
	if err != nil {
		t.Fatal(err)
	}
	wantA := filepath.Join(rootA, "notes", "demo.ipynb")
	if got := backfilled["notebook_path"]; got != wantA {
		t.Fatalf("backfilled notebook_path = %v, want %q", got, wantA)
	}
	if backfilled["extra"] != "preserved" || input["notebook_path"] != "notes/demo.ipynb" {
		t.Fatalf("backfill did not preserve a defensive input copy: input=%#v backfilled=%#v", input, backfilled)
	}

	runtime.snapshot = types.ToolRuntimeContext{ProjectRoot: rootB, AllowedDirs: []string{rootB}}
	normalized, err := tool.NormalizeToolInput(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	wantB := filepath.Join(rootB, "notes", "demo.ipynb")
	if got := normalized["notebook_path"]; got != wantB {
		t.Fatalf("normalized notebook_path = %v, want latest runtime path %q", got, wantB)
	}
}

func TestNotebookEditExecutesRelativeToRuntimeProjectRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixture.ipynb")
	if err := os.WriteFile(path, []byte(fixtureNotebookJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	seedNotebookReadState(t, state, path)
	tool := &NotebookEditTool{
		Runtime: &notebookRuntimeProvider{snapshot: types.ToolRuntimeContext{
			ProjectRoot: root,
			AllowedDirs: []string{root},
		}},
		ReadState: state,
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"notebook_path": "fixture.ipynb",
		"cell_id":       "def67890",
		"new_source":    "print(42)",
	})
	if err != nil || result.IsError {
		t.Fatalf("relative NotebookEdit failed: result=%+v err=%v", result, err)
	}
	out, ok := result.Data.(NotebookEditResult)
	if !ok || out.NotebookPath != path {
		t.Fatalf("result path = %#v, want %q", result.Data, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "print(42)") {
		t.Fatalf("runtime-root notebook was not updated: %s", contents)
	}
}

func TestNotebookEditPromptCopyUsesActiveRuntimeLanguage(t *testing.T) {
	previous := i18n.DetectOrLoadLanguage()
	t.Cleanup(func() { _ = i18n.SaveLanguage(previous) })
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	tool := &NotebookEditTool{}
	if got := tool.Description(); got != i18n.Text(i18n.LangZH, i18n.KeyToolNotebookEditDescription) {
		t.Fatalf("NotebookEdit description = %q", got)
	}

	checks := map[string]i18n.Key{
		"notebook_path": i18n.KeyToolNotebookEditInputPathDescription,
		"cell_id":       i18n.KeyToolNotebookEditInputCellIDDescription,
		"new_source":    i18n.KeyToolNotebookEditInputNewSourceDescription,
		"cell_type":     i18n.KeyToolNotebookEditInputCellTypeDescription,
		"edit_mode":     i18n.KeyToolNotebookEditInputModeDescription,
	}
	for field, key := range checks {
		property, ok := tool.Schema().Properties[field].(map[string]any)
		if !ok || property["description"] != i18n.Text(i18n.LangZH, key) {
			t.Errorf("schema field %s = %#v", field, tool.Schema().Properties[field])
		}
	}

	if err := i18n.SaveLanguage(i18n.LangJA); err != nil {
		t.Fatal(err)
	}
	if got := tool.Description(); got != i18n.Text(i18n.LangJA, i18n.KeyToolNotebookEditDescription) {
		t.Fatalf("NotebookEdit description did not follow runtime language: %q", got)
	}
}
