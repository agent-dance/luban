package tools

// Alignment red tests for bash_destructive_warning.go.
//
// Audit ref: alignment_audit.md P1-4 — bash_destructive_warning.go:16-26
// only carries 8 patterns vs the TS reference which covers git/SQL/k8s/
// terraform/helm in addition to rm/dd/mkfs/shred. These tests pin the
// missing categories so the gap is observable.
//
// Each test calls DestructiveCommandWarning and expects fire == true with a
// non-empty warning. The current implementation returns ("", false) for
// every command below.

import (
	"testing"
)

//  1. git push --force overwrites remote history.
//     TS ref: src/tools/BashTool/destructiveCommandWarning.ts (git block)
func TestBashAlignment_Destructive_GitPushForce(t *testing.T) {
	cmd := "git push --force origin main"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 2. git push -f short flag.
func TestBashAlignment_Destructive_GitPushShortF(t *testing.T) {
	cmd := "git push -f origin master"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 3. git reset --hard discards local history.
func TestBashAlignment_Destructive_GitResetHard(t *testing.T) {
	cmd := "git reset --hard origin/main"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

//  4. SQL: DROP TABLE drops a table irrecoverably.
//     TS ref: destructiveCommandWarning.ts SQL block.
func TestBashAlignment_Destructive_SQLDropTable(t *testing.T) {
	cmd := `psql -c "DROP TABLE users;"`
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

//  5. kubectl delete namespace cascades pod deletion across the cluster.
//     TS ref: destructiveCommandWarning.ts k8s block.
func TestBashAlignment_Destructive_KubectlDeleteNamespace(t *testing.T) {
	cmd := "kubectl delete namespace prod"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 6. kubectl delete --all wipes every resource of a kind.
func TestBashAlignment_Destructive_KubectlDeleteAll(t *testing.T) {
	cmd := "kubectl delete pods --all"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

//  7. terraform destroy nukes all managed infrastructure.
//     TS ref: destructiveCommandWarning.ts terraform block.
func TestBashAlignment_Destructive_TerraformDestroy(t *testing.T) {
	cmd := "terraform destroy -auto-approve"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 8. helm uninstall removes the entire release including persistent volumes.
func TestBashAlignment_Destructive_HelmUninstall(t *testing.T) {
	cmd := "helm uninstall my-release --namespace prod"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

//  9. rm with long flags. Current regex needs short -rf/-fr; AST hasFlag
//     explicitly skips `--`-prefixed args (dangerous.go:118-127).
func TestBashAlignment_Destructive_RmLongFlags(t *testing.T) {
	cmd := "rm --recursive --force /tmp/foo"
	w, fire := DestructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// Sanity: benign command must NOT trigger. Locks the false-positive
// contract from tasks/bash.json acceptance criteria.
func TestBashAlignment_Destructive_NegativeLs(t *testing.T) {
	cmd := "ls -la"
	w, fire := DestructiveCommandWarning(cmd)
	if fire || w != "" {
		t.Errorf("benign command %q should NOT trigger destructive warning, got %q (fire=%v)", cmd, w, fire)
	}
}
