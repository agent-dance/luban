package mcp

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// LifecycleConfig holds process launch parameters for the LifecycleManager.
type LifecycleConfig struct {
	Command string
	Args    []string
	Env     []string // raw "KEY=VALUE" pairs appended to os.Environ()
	WorkDir string
}

// serverState enumerates possible lifecycle states.
type serverState string

const (
	stateStopped serverState = "stopped"
	stateRunning serverState = "running"
	stateError   serverState = "error"
)

// ServerStatus is a snapshot of a managed server's runtime state.
type ServerStatus struct {
	State        string
	PID          int
	StartedAt    time.Time
	RestartCount int
	LastError    error
}

// managedServer is the internal record for one server process.
type managedServer struct {
	mu           sync.Mutex
	cfg          LifecycleConfig
	cmd          *exec.Cmd
	state        serverState
	pid          int
	startedAt    time.Time
	restartCount int
	lastError    error

	// used by health-check and reconnect goroutines
	cancelHealth    context.CancelFunc
	cancelReconnect context.CancelFunc
	reconnectPolicy *ReconnectPolicy
}

// LifecycleManager manages multiple MCP server processes.
type LifecycleManager struct {
	mu      sync.Mutex
	servers map[string]*managedServer
}

// NewLifecycleManager returns an empty LifecycleManager.
func NewLifecycleManager() *LifecycleManager {
	return &LifecycleManager{
		servers: make(map[string]*managedServer),
	}
}

// Start launches a new server process for name using cfg.
// Returns an error if the server is already running.
func (m *LifecycleManager) Start(ctx context.Context, name string, cfg LifecycleConfig) error {
	m.mu.Lock()
	ms, exists := m.servers[name]
	if !exists {
		ms = &managedServer{cfg: cfg}
		m.servers[name] = ms
	}
	m.mu.Unlock()

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.state == stateRunning {
		return i18n.NewError(i18n.KeyLegacyMCPServerAlreadyRunning, name, ms.pid)
	}

	if err := startProcess(ms, cfg); err != nil {
		startErr := i18n.WrapError(i18n.KeyLegacyMCPStartNamedServer, err, name)
		ms.state = stateError
		ms.lastError = startErr
		slog.Error(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPStartFailed), "name", name, "error", err)
		return startErr
	}

	slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPStarted), "name", name, "pid", ms.pid)
	return nil
}

// startProcess launches the process and updates ms fields. Caller must hold ms.mu.
func startProcess(ms *managedServer, cfg LifecycleConfig) error {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = append(os.Environ(), cfg.Env...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	ms.cmd = cmd
	ms.pid = cmd.Process.Pid
	ms.state = stateRunning
	ms.startedAt = time.Now()
	ms.lastError = nil
	return nil
}

// Stop gracefully stops the named server.
// It sends SIGTERM and waits up to 5 seconds before sending SIGKILL.
func (m *LifecycleManager) Stop(ctx context.Context, name string) error {
	m.mu.Lock()
	ms, exists := m.servers[name]
	m.mu.Unlock()
	if !exists {
		return i18n.NewError(i18n.KeyLegacyMCPServerNotFound, name)
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()
	return stopProcess(ctx, ms, name)
}

// stopProcess sends SIGTERM then SIGKILL. Caller must hold ms.mu.
func stopProcess(ctx context.Context, ms *managedServer, name string) error {
	if ms.state != stateRunning || ms.cmd == nil {
		ms.state = stateStopped
		return nil
	}

	// Cancel any running health / reconnect loops.
	if ms.cancelHealth != nil {
		ms.cancelHealth()
		ms.cancelHealth = nil
	}
	if ms.cancelReconnect != nil {
		ms.cancelReconnect()
		ms.cancelReconnect = nil
	}

	proc := ms.cmd.Process
	if err := proc.Signal(sigTerm()); err != nil {
		slog.Debug(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPSigtermFailed), "name", name, "error", err)
	}

	done := make(chan error, 1)
	go func() { done <- ms.cmd.Wait() }()

	gracePeriod := 5 * time.Second
	timer := time.NewTimer(gracePeriod)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
		slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPShutdownTimeout), "name", name)
		proc.Kill() //nolint:errcheck
		<-done
	case <-ctx.Done():
		proc.Kill() //nolint:errcheck
		<-done
		ms.state = stateStopped
		ms.pid = 0
		ms.cmd = nil
		return ctx.Err()
	}

	ms.state = stateStopped
	ms.pid = 0
	ms.cmd = nil
	slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPStopped), "name", name)
	return nil
}

// Restart stops then starts the named server.
func (m *LifecycleManager) Restart(ctx context.Context, name string) error {
	m.mu.Lock()
	ms, exists := m.servers[name]
	m.mu.Unlock()
	if !exists {
		return i18n.NewError(i18n.KeyLegacyMCPServerNotFound, name)
	}

	ms.mu.Lock()
	cfg := ms.cfg
	if err := stopProcess(ctx, ms, name); err != nil && !errors.Is(err, context.Canceled) {
		ms.mu.Unlock()
		return i18n.WrapError(i18n.KeyLegacyMCPStopDuringRestart, err, name)
	}
	ms.restartCount++
	if err := startProcess(ms, cfg); err != nil {
		startErr := i18n.WrapError(i18n.KeyLegacyMCPStartDuringRestart, err, name)
		ms.state = stateError
		ms.lastError = startErr
		ms.mu.Unlock()
		return startErr
	}
	ms.mu.Unlock()

	slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPRestarted), "name", name, "restartCount", ms.restartCount)
	return nil
}

// Status returns a snapshot of the named server's status.
// If the server is not known, a stopped-state status is returned.
func (m *LifecycleManager) Status(name string) ServerStatus {
	m.mu.Lock()
	ms, exists := m.servers[name]
	m.mu.Unlock()
	if !exists {
		return ServerStatus{State: string(stateStopped)}
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ServerStatus{
		State:        string(ms.state),
		PID:          ms.pid,
		StartedAt:    ms.startedAt,
		RestartCount: ms.restartCount,
		LastError:    ms.lastError,
	}
}

// StopAll shuts down every managed server. It collects and joins all errors.
func (m *LifecycleManager) StopAll(ctx context.Context) error {
	m.mu.Lock()
	names := make([]string, 0, len(m.servers))
	for n := range m.servers {
		names = append(names, n)
	}
	m.mu.Unlock()

	var errs []error
	for _, name := range names {
		if err := m.Stop(ctx, name); err != nil {
			errs = append(errs, i18n.WrapError(i18n.KeyLegacyMCPNamedServerError, err, name))
		}
	}
	return errors.Join(errs...)
}
