package mcp

import (
	"context"
	"log/slog"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// ReconnectPolicy controls exponential-backoff reconnect behaviour.
type ReconnectPolicy struct {
	MaxAttempts  int           // 0 means use default (5)
	InitialDelay time.Duration // first backoff; default 1s
	MaxDelay     time.Duration // cap on backoff; default 30s
	// StableThreshold is how long a server must stay up before the backoff
	// counter is reset. Default 60s.
	StableThreshold time.Duration
}

func (p ReconnectPolicy) maxAttempts() int {
	if p.MaxAttempts <= 0 {
		return 5
	}
	return p.MaxAttempts
}

func (p ReconnectPolicy) initialDelay() time.Duration {
	if p.InitialDelay <= 0 {
		return time.Second
	}
	return p.InitialDelay
}

func (p ReconnectPolicy) maxDelay() time.Duration {
	if p.MaxDelay <= 0 {
		return 30 * time.Second
	}
	return p.MaxDelay
}

func (p ReconnectPolicy) stableThreshold() time.Duration {
	if p.StableThreshold <= 0 {
		return 60 * time.Second
	}
	return p.StableThreshold
}

// EnableReconnect starts a background goroutine that watches the named server
// process and automatically restarts it on unexpected exit, using exponential
// backoff as defined by policy.  Cancelling ctx stops the loop.
func (m *LifecycleManager) EnableReconnect(name string, policy ReconnectPolicy) {
	m.mu.Lock()
	ms, exists := m.servers[name]
	m.mu.Unlock()
	if !exists {
		slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPReconnectNotFound), "name", name)
		return
	}

	ms.mu.Lock()
	// Cancel any previous reconnect loop.
	if ms.cancelReconnect != nil {
		ms.cancelReconnect()
	}
	ctx, cancel := context.WithCancel(context.Background())
	ms.cancelReconnect = cancel
	ms.reconnectPolicy = &policy
	ms.mu.Unlock()

	go m.reconnectLoop(ctx, name, policy)
}

// reconnectLoop watches for unexpected process exit and attempts to restart.
func (m *LifecycleManager) reconnectLoop(ctx context.Context, name string, policy ReconnectPolicy) {
	slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPReconnectLoopStarted), "name", name)
	defer slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPReconnectLoopStopped), "name", name)

	// Maintain backoff state across outer iterations so a server that crashes
	// again quickly does not get a freshly-reset delay. The counters are only
	// reset when the server has been stable for at least stableThreshold.
	delay := policy.initialDelay()
	attempts := 0

	for {
		// Record the time we began waiting so we can calculate uptime later.
		waitStart := time.Now()

		// Wait for the process to exit (or context cancellation).
		if err := m.waitForExit(ctx, name); err != nil {
			// Context cancelled — intentional shutdown.
			return
		}

		// Check if the stop was intentional (state == stopped by user).
		m.mu.Lock()
		ms, exists := m.servers[name]
		m.mu.Unlock()
		if !exists {
			return
		}
		ms.mu.Lock()
		isIntentional := ms.state == stateStopped && ms.cancelReconnect == nil
		ms.mu.Unlock()
		if isIntentional {
			return
		}

		uptime := time.Since(waitStart)
		slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPUnexpectedExit),
			"name", name, "uptime", uptime)

		// Reset backoff only if the server was stable long enough.
		if uptime >= policy.stableThreshold() {
			slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPStableReset), "name", name)
			delay = policy.initialDelay()
			attempts = 0
		}

		// Exponential backoff loop.
		restarted := false
		for attempts < policy.maxAttempts() {
			attempts++

			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}

			slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPRestartAttempt),
				"name", name, "attempt", attempts, "maxAttempts", policy.maxAttempts())

			ms.mu.Lock()
			cfg := ms.cfg
			ms.restartCount++
			err := startProcess(ms, cfg)
			if err == nil {
				ms.mu.Unlock()
				slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPRestartSucceeded), "name", name, "attempt", attempts)
				restarted = true
				break
			}

			ms.lastError = i18n.WrapError(i18n.KeyLegacyMCPReconnectAttempt, err, attempts)
			ms.state = stateError
			ms.mu.Unlock()
			slog.Error(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPRestartFailed),
				"name", name, "attempt", attempts, "error", err)

			// Double the delay, capped at MaxDelay.
			delay *= 2
			if delay > policy.maxDelay() {
				delay = policy.maxDelay()
			}
		}

		if !restarted {
			// Exhausted attempts — give up reconnecting.
			slog.Error(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPReconnectExhausted), "name", name,
				"maxAttempts", policy.maxAttempts())
			return
		}
		// Server restarted — go back to waitForExit.
	}
}

// waitForExit blocks until the named server process exits or ctx is cancelled.
// Returns nil when the process exits, ctx.Err() on cancellation.
func (m *LifecycleManager) waitForExit(ctx context.Context, name string) error {
	// Poll every 500ms — lightweight and avoids goroutine leaks from exec.Cmd.Wait.
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			m.mu.Lock()
			ms, exists := m.servers[name]
			m.mu.Unlock()
			if !exists {
				return i18n.NewError(i18n.KeyLegacyMCPServerDisappeared, name)
			}

			ms.mu.Lock()
			proc := ms.cmd
			isRunning := ms.state == stateRunning
			ms.mu.Unlock()

			if !isRunning || proc == nil {
				return nil
			}

			// Check if the process is still alive by sending signal 0.
			if err := proc.Process.Signal(sigZero()); err != nil {
				// Process has exited.
				ms.mu.Lock()
				if ms.state == stateRunning {
					ms.state = stateError
					ms.lastError = i18n.NewError(i18n.KeyLegacyMCPProcessUnexpectedExit)
				}
				ms.mu.Unlock()
				return nil
			}
		}
	}
}
