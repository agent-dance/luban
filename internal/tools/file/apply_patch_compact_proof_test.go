package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
)

func TestApplyPatchCompactionProofCarriesCASRevisionAndTotalsOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "private.go")
	if err := os.WriteFile(path, []byte("package private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	receipt, err := ledger.Commit(root, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	result := ApplyPatchResult{
		Status: "success", ChangedPaths: []string{"private.go"},
		Files:   []ApplyPatchFileStat{{Path: "private.go", Operation: "update", Hunks: 2, Additions: 5, Deletions: 1}},
		Summary: ApplyPatchSummary{Files: 1, Hunks: 2, Additions: 5, Deletions: 1}, revision: receipt,
	}
	proof := result.CompactionProof()
	if proof.Patch == nil || proof.Revision == nil || proof.Patch.CAS != "committed" || proof.Revision.Epoch != 1 ||
		proof.Patch.Files != 1 || proof.Patch.Hunks != 2 || proof.Patch.Additions != 5 || proof.Patch.Deletions != 1 {
		t.Fatalf("proof = %#v", proof)
	}
	encoded, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private.go") || strings.Contains(string(encoded), "package private") || strings.Contains(string(encoded), "update") {
		t.Fatalf("ApplyPatch proof leaked patch details: %s", encoded)
	}
}
