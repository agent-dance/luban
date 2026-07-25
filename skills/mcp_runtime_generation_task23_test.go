package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
)

func TestTask23ProjectAndUnifiedMCPProjectionPublishAtomically(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	task23WriteProjectSkill(t, rootA, "project-a")
	task23WriteProjectSkill(t, rootB, "project-b")
	store, err := NewFileOverrideStore(rootA, nil, NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := newManagerWithOverrideStore(store)
	if err := manager.ReplaceProjectSources(rootA); err != nil {
		t.Fatal(err)
	}
	oldInputs := task23UnifiedMCPInputs(t, "a")
	if err := manager.ReplaceMCPCatalogInputsAtGeneration(manager.ProjectGeneration(), oldInputs); err != nil {
		t.Fatal(err)
	}
	oldNames := []string{"project-a", "srv:prompt-a", "srv:resource-a"}
	newNames := []string{"project-b", "srv:prompt-b", "srv:resource-b"}
	if got := task23SnapshotNames(t, manager); !reflect.DeepEqual(got, oldNames) {
		t.Fatalf("initial names = %v, want %v", got, oldNames)
	}

	planB, err := manager.PrepareProjectSources(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.StageMCPCatalogInputs(planB, task23UnifiedMCPInputs(t, "b")); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	stop := make(chan struct{})
	errCh := make(chan error, 8)
	var readers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for {
				select {
				case <-stop:
					return
				default:
				}
				names, snapshotErr := task23SnapshotNamesE(manager)
				if snapshotErr != nil {
					select {
					case errCh <- snapshotErr:
					default:
					}
					return
				}
				if !reflect.DeepEqual(names, oldNames) && !reflect.DeepEqual(names, newNames) {
					select {
					case errCh <- fmt.Errorf("mixed project/MCP snapshot: %v", names):
					default:
					}
					return
				}
			}
		}()
	}
	close(start)
	if err := manager.ApplyProjectSources(planB); err != nil {
		t.Fatal(err)
	}
	close(stop)
	readers.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if got := task23SnapshotNames(t, manager); !reflect.DeepEqual(got, newNames) {
		t.Fatalf("target names = %v, want %v", got, newNames)
	}
}

func task23UnifiedMCPInputs(t *testing.T, suffix string) []MCPCatalogInput {
	t.Helper()
	prompt, err := NewMCPPromptCatalogInput("srv", "prompt-"+suffix, "prompt "+suffix, nil, "prompt "+suffix)
	if err != nil {
		t.Fatal(err)
	}
	resource := skillFromMCPResource("srv", catalog.Resource{
		URI: "skill://task23/resource-" + suffix + "/SKILL.md", Name: "resource-" + suffix,
	}, "---\ndescription: resource "+suffix+"\n---\nresource "+suffix)
	resourceInput, err := newMCPResourceCatalogInput("srv", resource)
	if err != nil {
		t.Fatal(err)
	}
	return []MCPCatalogInput{prompt, resourceInput}
}

func task23WriteProjectSkill(t *testing.T, root, name string) {
	t.Helper()
	dirs, err := ProjectDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(dirs[0].Dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ndescription: " + name + "\n---\n" + name
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task23SnapshotNames(t *testing.T, manager *Manager) []string {
	t.Helper()
	names, err := task23SnapshotNamesE(manager)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func task23SnapshotNamesE(manager *Manager) ([]string, error) {
	snapshot, err := manager.Snapshot("task23-atomic")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(snapshot.Skills))
	for _, row := range snapshot.Skills {
		names = append(names, row.Name)
	}
	sort.Strings(names)
	return names, nil
}
