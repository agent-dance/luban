package app

import (
	"context"
	"errors"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/engine"
)

const applicationShutdownTimeout = 20 * time.Second

type applicationShutdownStep struct {
	key i18n.Key
	run func(context.Context) error
}

// shutdownApplicationRuntime closes process-owned runtime services in dependency
// order. Producers stop first; registry-backed services remain available until
// foreground and background consumers have drained.
func shutdownApplicationRuntime(ctx context.Context, deps *RegistryDeps, eng engine.Engine) []error {
	if ctx == nil {
		ctx = context.Background()
	}
	steps := make([]applicationShutdownStep, 0, 5)
	if deps != nil && deps.Schedule != nil {
		steps = append(steps, applicationShutdownStep{key: i18n.KeyStartupShutdownSchedule, run: deps.StopSchedule})
	}
	if eng != nil {
		steps = append(steps, applicationShutdownStep{key: i18n.KeyStartupShutdownEngine, run: eng.Shutdown})
	}
	if deps != nil && deps.BackgroundTasks != nil {
		steps = append(steps, applicationShutdownStep{key: i18n.KeyStartupShutdownBackground, run: deps.BackgroundTasks.Shutdown})
	}
	if deps != nil && deps.ServiceMCP != nil {
		steps = append(steps, applicationShutdownStep{key: i18n.KeyStartupShutdownMCP, run: deps.StopMCPRuntimeBridge})
	}
	if deps != nil && deps.lspManager != nil {
		steps = append(steps, applicationShutdownStep{key: i18n.KeyStartupShutdownLSP, run: deps.ShutdownLSP})
	}

	issues := executeApplicationShutdownSteps(ctx, steps)
	if deps != nil {
		deps.StopWebFetchCache()
	}
	return issues
}

func executeApplicationShutdownSteps(ctx context.Context, steps []applicationShutdownStep) []error {
	issues := make([]error, 0, len(steps))
	for _, step := range steps {
		if step.run == nil {
			continue
		}
		if err := step.run(ctx); err != nil {
			issues = append(issues, i18n.WrapInternalError(step.key, err))
		}
	}
	return issues
}

func joinApplicationShutdownIssues(issues []error) error {
	return errors.Join(issues...)
}

func applicationExitCodeAfterShutdown(exitCode int, issues []error) int {
	if exitCode == 0 && len(issues) > 0 {
		return 1
	}
	return exitCode
}
