package gitutil

import (
	"strings"
	"testing"
)

func TestNoPromptEnv(t *testing.T) {
	env := noPromptEnv()
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "SSH_ASKPASS_REQUIRE=never"} {
		if !containsEnv(env, want) {
			t.Errorf("noPromptEnv missing %q", want)
		}
	}
	for _, value := range env {
		if strings.HasPrefix(value, "GIT_SSH_COMMAND=") && strings.Contains(value, "BatchMode=yes") {
			return
		}
	}
	t.Error("GIT_SSH_COMMAND must contain BatchMode=yes")
}

func containsEnv(env []string, want string) bool {
	for _, value := range env {
		if value == want {
			return true
		}
	}
	return false
}
