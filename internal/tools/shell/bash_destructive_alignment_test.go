package shell

// Regression tests for Bash destructive-command warnings.
//
// These tests pin warnings across git, SQL, Kubernetes, Terraform, Helm,
// filesystem, and raw-device operations.
//
// Each test calls destructiveCommandWarning and expects fire == true with a
// non-empty warning.

import (
	"testing"
)

//  1. git push --force overwrites remote history.
//     TS ref: src/tools/BashTool/destructiveCommandWarning.ts (git block)
func TestBashAlignment_Destructive_GitPushForce(t *testing.T) {
	cmd := "git push --force origin main"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 2. git push -f short flag.
func TestBashAlignment_Destructive_GitPushShortF(t *testing.T) {
	cmd := "git push -f origin master"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 3. git reset --hard discards local history.
func TestBashAlignment_Destructive_GitResetHard(t *testing.T) {
	cmd := "git reset --hard origin/main"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

//  4. SQL: DROP TABLE drops a table irrecoverably.
//     TS ref: destructiveCommandWarning.ts SQL block.
func TestBashAlignment_Destructive_SQLDropTable(t *testing.T) {
	cmd := `psql -c "DROP TABLE users;"`
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

//  5. kubectl delete namespace cascades pod deletion across the cluster.
//     TS ref: destructiveCommandWarning.ts k8s block.
func TestBashAlignment_Destructive_KubectlDeleteNamespace(t *testing.T) {
	cmd := "kubectl delete namespace prod"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 6. kubectl delete --all wipes every resource of a kind.
func TestBashAlignment_Destructive_KubectlDeleteAll(t *testing.T) {
	cmd := "kubectl delete pods --all"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

//  7. terraform destroy nukes all managed infrastructure.
//     TS ref: destructiveCommandWarning.ts terraform block.
func TestBashAlignment_Destructive_TerraformDestroy(t *testing.T) {
	cmd := "terraform destroy -auto-approve"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 8. helm uninstall removes the entire release including persistent volumes.
func TestBashAlignment_Destructive_HelmUninstall(t *testing.T) {
	cmd := "helm uninstall my-release --namespace prod"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// 9. rm with long flags is equivalent to short -rf/-fr.
func TestBashAlignment_Destructive_RmLongFlags(t *testing.T) {
	cmd := "rm --recursive --force /tmp/foo"
	w, fire := destructiveCommandWarning(cmd)
	if !fire || w == "" {
		t.Errorf("expected destructive warning for %q, got fire=%v warning=%q", cmd, fire, w)
	}
}

// Sanity: a benign command must not trigger the destructive warning.
func TestBashAlignment_Destructive_NegativeLs(t *testing.T) {
	cmd := "ls -la"
	w, fire := destructiveCommandWarning(cmd)
	if fire || w != "" {
		t.Errorf("benign command %q should NOT trigger destructive warning, got %q (fire=%v)", cmd, w, fire)
	}
}
