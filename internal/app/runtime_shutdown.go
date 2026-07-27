package app

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/provider"
)

const applicationShutdownTimeout = 20 * time.Second

type applicationShutdownStep struct {
	key i18n.Key
	run func(context.Context) error
}

// shutdownApplicationRuntime closes process-owned runtime services in dependency
// order. Producers stop first; registry-backed services remain available until
// foreground and background consumers have drained.
func shutdownApplicationRuntime(ctx context.Context, deps *RegistryDeps, eng engine.Engine, providerClosers ...provider.CloseProvider) []error {
	if ctx == nil {
		ctx = context.Background()
	}
	steps := make([]applicationShutdownStep, 0, 5)
	if deps != nil && deps.Schedule != nil {
		steps = append(steps, applicationShutdownStep{key: i18n.KeyStartupShutdownSchedule, run: deps.StopSchedule})
	}
	if !isNilApplicationEngine(eng) {
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
	for _, closer := range providerClosers {
		if closer == nil {
			continue
		}
		providerCloser := closer
		steps = append(steps, applicationShutdownStep{key: i18n.KeyStartupShutdownProvider, run: func(context.Context) error {
			return providerCloser.Close()
		}})
	}

	issues := executeApplicationShutdownSteps(ctx, steps)
	if deps != nil {
		deps.StopWebFetchCache()
	}
	return issues
}

// An uninitialized *CoreEngine converted to engine.Engine is a non-nil
// interface with a nil dynamic pointer. Startup failures can reach shutdown
// before engine construction, so lifecycle ownership must reject every typed
// nil implementation before taking its Shutdown method value.
func isNilApplicationEngine(eng engine.Engine) bool {
	if eng == nil {
		return true
	}
	value := reflect.ValueOf(eng)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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
