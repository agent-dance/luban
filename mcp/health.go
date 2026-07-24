package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	defaultHealthInterval     = 30 * time.Second
	defaultUnhealthyThreshold = 3
)

// healthMonitor tracks ping state for one server.
type healthMonitor struct {
	cancel           context.CancelFunc
	consecutiveFails int
	mu               sync.Mutex
}

// pingServer sends a JSON-RPC "ping" to the named server's process via stdin.
// We implement a lightweight ad-hoc ping since the lifecycle layer doesn't hold
// a full jrpc2.Client — it only manages the OS process.
// Returns nil on success (process is alive and stdin is writable).
func (m *LifecycleManager) pingServer(name string) error {
	m.mu.Lock()
	ms, exists := m.servers[name]
	m.mu.Unlock()
	if !exists {
		return i18n.NewError(i18n.KeyLegacyMCPHealthServerNotFound, name)
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.state != stateRunning || ms.cmd == nil {
		return i18n.NewError(i18n.KeyLegacyMCPServerNotRunning, name)
	}

	// Probe liveness: signal(0) succeeds if the process exists.
	if err := ms.cmd.Process.Signal(sigZero()); err != nil {
		return i18n.WrapError(i18n.KeyLegacyMCPProcessNotAlive, err, name)
	}

	// If stdin is available, send an actual ping JSON-RPC notification.
	if ms.cmd.Stdin != nil {
		ping := map[string]any{
			"jsonrpc": "2.0",
			"method":  "ping",
			"id":      1,
		}
		data, _ := json.Marshal(ping)
		data = append(data, '\n')
		// Best-effort write; ignore errors (process may not support ping).
		if sw, ok := ms.cmd.Stdin.(interface{ Write([]byte) (int, error) }); ok {
			sw.Write(data) //nolint:errcheck
		}
	}

	return nil
}

// StartHealthCheck starts a background goroutine that periodically pings the
// named server.  After defaultUnhealthyThreshold consecutive failures it marks
// the server as unhealthy and triggers a reconnect if a policy is configured.
func (m *LifecycleManager) StartHealthCheck(name string, interval time.Duration) {
	m.mu.Lock()
	ms, exists := m.servers[name]
	m.mu.Unlock()
	if !exists {
		slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPServerNotFound), "name", name)
		return
	}

	if interval <= 0 {
		interval = defaultHealthInterval
	}

	ms.mu.Lock()
	// Cancel previous health check if any.
	if ms.cancelHealth != nil {
		ms.cancelHealth()
	}
	ctx, cancel := context.WithCancel(context.Background())
	ms.cancelHealth = cancel
	ms.mu.Unlock()

	go m.healthLoop(ctx, name, interval)
}

// StopHealthCheck stops the health-check goroutine for the named server.
func (m *LifecycleManager) StopHealthCheck(name string) {
	m.mu.Lock()
	ms, exists := m.servers[name]
	m.mu.Unlock()
	if !exists {
		return
	}

	ms.mu.Lock()
	if ms.cancelHealth != nil {
		ms.cancelHealth()
		ms.cancelHealth = nil
	}
	ms.mu.Unlock()
	slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPHealthStopped), "name", name)
}

func (m *LifecycleManager) healthLoop(ctx context.Context, name string, interval time.Duration) {
	slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPHealthLoopStarted), "name", name, "interval", interval)
	defer slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPHealthLoopStopped), "name", name)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	consecutiveFails := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := m.pingServer(name)
			if err == nil {
				if consecutiveFails > 0 {
					slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPHealthyAgain), "name", name)
					consecutiveFails = 0
				}
				continue
			}

			consecutiveFails++
			slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPPingFailed),
				"name", name, "consecutiveFails", consecutiveFails, "error", err)

			if consecutiveFails >= defaultUnhealthyThreshold {
				slog.Error(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPMarkedUnhealthy),
					"name", name, "consecutiveFails", consecutiveFails)

				// Mark server as unhealthy.
				m.mu.Lock()
				ms, exists := m.servers[name]
				m.mu.Unlock()
				if exists {
					ms.mu.Lock()
					ms.state = stateError
					ms.lastError = i18n.NewError(i18n.KeyLegacyMCPHealthCheckFailed, consecutiveFails)
					hasReconnect := ms.reconnectPolicy != nil
					var policy ReconnectPolicy
					if hasReconnect {
						policy = *ms.reconnectPolicy
					}
					ms.mu.Unlock()

					if hasReconnect {
						m.EnableReconnect(name, policy)
					}
				}

				consecutiveFails = 0 // reset so we don't spam reconnects
			}
		}
	}
}
