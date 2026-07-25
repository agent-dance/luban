package app

import (
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
)

func TestTaskGetTask27SetupRegistryAlwaysRegistersCanonicalTaskTools(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
	}{
		{name: "interactive", interactive: true},
		{name: "non-interactive", interactive: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, tc.interactive)
			if deps.Schedule != nil {
				t.Cleanup(func() { stopScheduleForTest(t, deps) })
			}
			for _, name := range []string{"TaskCreate", "TaskGet", "TaskUpdate", "TaskList"} {
				if deps.Registry.Get(name) == nil {
					t.Errorf("Registry.Get(%q) is nil", name)
				}
			}
			if deps.Registry.Get("TaskStop") == nil || deps.Registry.Get("TaskOutput") == nil {
				t.Fatal("TaskStop and TaskOutput must remain registration-gate independent")
			}
		})
	}
}
