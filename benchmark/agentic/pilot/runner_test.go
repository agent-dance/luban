package pilot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestValidatePairedPlanRequiresExactAdjacentMatrix(t *testing.T) {
	entries := make([]harness.PlanEntry, 0, len(ExactTaskIDs)*2)
	for index, taskID := range ExactTaskIDs {
		pairID := fmt.Sprintf("pair-%d", index)
		entries = append(entries,
			harness.PlanEntry{Ordinal: index * 2, PairID: pairID, TaskID: taskID, AgentID: "codex", Repetition: 0},
			harness.PlanEntry{Ordinal: index*2 + 1, PairID: pairID, TaskID: taskID, AgentID: "luban", Repetition: 0},
		)
	}
	valid := harness.RunPlan{Entries: entries}
	if err := validatePairedPlan(valid); err != nil {
		t.Fatal(err)
	}

	attacks := map[string]func([]harness.PlanEntry){
		"non adjacent pair": func(values []harness.PlanEntry) { values[1].PairID = "other" },
		"same agent twice":  func(values []harness.PlanEntry) { values[1].AgentID = values[0].AgentID },
		"duplicate task": func(values []harness.PlanEntry) {
			values[2].TaskID = values[0].TaskID
			values[3].TaskID = values[0].TaskID
		},
		"wrong ordinal":    func(values []harness.PlanEntry) { values[4].Ordinal++ },
		"wrong repetition": func(values []harness.PlanEntry) { values[6].Repetition = 1 },
	}
	for name, mutate := range attacks {
		t.Run(name, func(t *testing.T) {
			candidate := append([]harness.PlanEntry(nil), entries...)
			mutate(candidate)
			if err := validatePairedPlan(harness.RunPlan{Entries: candidate}); err == nil {
				t.Fatal("mutated paired plan was accepted")
			}
		})
	}
}

func TestWriteBytesNoClobberPreservesFirstReceipt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attempt", AttemptReceiptName)
	if err := writeBytesNoClobber(path, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeBytesNoClobber(path, []byte("second\n"), 0o600); err == nil {
		t.Fatal("second receipt overwrote an immutable attempt")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "first\n" {
		t.Fatalf("receipt = %q", raw)
	}
}

func TestReadStrictJSONRejectsTrailingAndDuplicateValues(t *testing.T) {
	tests := map[string]string{
		"trailing value": `{"state":"sealed"}{"state":"unreserved"}`,
		"duplicate key":  `{"state":"sealed","state":"unreserved"}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger.json")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			var target map[string]any
			if err := readStrictJSON(path, &target); err == nil {
				t.Fatal("ambiguous JSON was accepted")
			}
		})
	}
}

func TestSafeJoinRejectsArtifactEscape(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"../escape", "a/../../escape", string(filepath.Separator) + "absolute"} {
		t.Run(strings.ReplaceAll(relative, string(filepath.Separator), "_"), func(t *testing.T) {
			if _, err := safeJoin(root, relative); err == nil {
				t.Fatal("artifact escape was accepted")
			}
		})
	}
	joined, err := safeJoin(root, "pilot/artifacts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		t.Fatalf("joined path = %q", joined)
	}
}
