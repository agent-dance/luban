package shell_test

import (
	"os/exec"
	"testing"

	"github.com/agent-dance/luban/types"
)

func requireBashAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not on PATH: %v", err)
	}
	if err := exec.Command("bash", "-c", "true").Run(); err != nil {
		t.Skipf("bash is present but not runnable: %v", err)
	}
}

func deterministicShellPolicyContext() types.PolicyContext {
	return types.PolicyContext{
		CWD:         "/workspace/project",
		HomeDir:     "/Users/tester",
		AllowedDirs: []string{"/workspace/project"},
		KnownEnvironment: map[string]string{
			"HOME": "/Users/tester",
		},
		TrustedTempRoots: []string{"/tmp"},
	}
}
