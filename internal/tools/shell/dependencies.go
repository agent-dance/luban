package shell

import (
	"context"
	"os/exec"
	"time"
)

// PlanGate exposes only the plan-mode state required by shell tools.
type PlanGate interface {
	IsActive() bool
	AllowedPromptMatches(tool, prompt string) bool
}

// BackgroundRunner owns process lifetime and durable output for background
// shell commands.
type BackgroundRunner interface {
	StartShellCommand(
		context.Context,
		string,
		string,
		*exec.Cmd,
		time.Duration,
		func(error, int),
	) (taskID, outputPath string, err error)
}

// PersistedOutput is the storage receipt for one oversized process result.
type PersistedOutput struct {
	Path         string
	OriginalSize int64
	ModelText    string
}

// OutputPersister keeps shell execution independent of the runtime compactor.
type OutputPersister interface {
	PersistShellOutput(root string, content []byte, maxBytes int64, preview string) (PersistedOutput, error)
}
