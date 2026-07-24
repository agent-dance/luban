package swarm

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestUserFacingErrorUsesSemanticCopy(t *testing.T) {
	tests := []struct {
		err  error
		key  i18n.Key
		name string
	}{
		{errors.New(`team "alpha" not found`), i18n.KeyAuxSwarmTeamNotFound, "not found"},
		{errors.New("agent name must not be empty"), i18n.KeyAuxSwarmInvalidName, "invalid"},
		{errors.New("mailbox send: lock: busy"), i18n.KeyAuxSwarmMailboxFailed, "mailbox"},
		{errors.New("save team config: disk full"), i18n.KeyAuxSwarmFailed, "fallback"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, lang := range i18n.AllLanguages() {
				if got, want := UserFacingError(lang, tc.err), i18n.Text(lang, tc.key); got != want {
					t.Fatalf("UserFacingError(%s) = %q, want %q", lang.Code(), got, want)
				}
			}
		})
	}
}
