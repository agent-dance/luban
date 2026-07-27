package shell

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
)

func TestRunCompactionProofExcludesCommandsPathsAndOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "private-source.go")
	if err := os.WriteFile(path, []byte("package private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger := workspacerevision.NewLedger()
	receipt, err := ledger.Commit(root, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	output := &RunOutput{
		LogicalExecutionCommitted: true, RevisionSealDisposition: "revision_bound", receipt: receipt,
		modelText: "private process output",
		Steps: []RunStepOutput{{
			ID: "private-command-id", Status: runStatusFailed, ExitCode: 9,
			Invoked: true, ProcessDurationMS: 17, Resources: []string{path}, StdoutBytes: 21, StderrBytes: 8,
		}},
	}
	proof := output.CompactionProof()
	if proof.Run == nil || proof.Revision == nil || proof.Revision.Epoch != 1 || len(proof.Run.Steps) != 1 {
		t.Fatalf("proof = %#v", proof)
	}
	step := proof.Run.Steps[0]
	if step.Ordinal != 0 || step.Status != runStatusFailed || step.ExitCode != 9 || step.DurationMS != 17 || !step.Invoked {
		t.Fatalf("step proof = %#v", step)
	}
	encoded, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-command-id") || strings.Contains(string(encoded), "private-source.go") || strings.Contains(string(encoded), "private process output") {
		t.Fatalf("Run proof leaked private execution data: %s", encoded)
	}
}
