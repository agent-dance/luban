package mcp

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

const (
	defaultRemoteReconnectAttempts       = 5
	defaultRemoteReconnectInitialDelay   = time.Second
	defaultRemoteReconnectMaxDelay       = 30 * time.Second
	defaultRemoteReconnectJitterFraction = 0.20
	defaultConnectionLostThreshold       = 5
)

var defaultStdioReconnectCooldowns = []time.Duration{
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
}

// ReconnectPolicy controls automatic MCP connection recovery.
type ReconnectPolicy struct {
	RemoteMaxAttempts       int
	RemoteInitialDelay      time.Duration
	RemoteMaxDelay          time.Duration
	RemoteJitterFraction    float64
	ConnectionLostThreshold int
	StdioCooldowns          []time.Duration
}

func DefaultReconnectPolicy() ReconnectPolicy {
	return ReconnectPolicy{
		RemoteMaxAttempts:       defaultRemoteReconnectAttempts,
		RemoteInitialDelay:      defaultRemoteReconnectInitialDelay,
		RemoteMaxDelay:          defaultRemoteReconnectMaxDelay,
		RemoteJitterFraction:    defaultRemoteReconnectJitterFraction,
		ConnectionLostThreshold: defaultConnectionLostThreshold,
		StdioCooldowns:          append([]time.Duration(nil), defaultStdioReconnectCooldowns...),
	}
}

// ConnectionLostEvent is emitted when automatic reconnect gives up.
type ConnectionLostEvent struct {
	ServerName string
	Transport  TransportType
	Attempts   int
	Err        error
}

type ConnectionLostListener func(ConnectionLostEvent)

func WithReconnectPolicy(policy ReconnectPolicy) ManagerOption {
	return func(m *Manager) {
		m.reconnectPolicy = policy.withDefaults()
	}
}

func WithConnectionLostListener(listener ConnectionLostListener) ManagerOption {
	return func(m *Manager) {
		m.connectionLostHandler = listener
	}
}

func (p ReconnectPolicy) withDefaults() ReconnectPolicy {
	if p.RemoteMaxAttempts <= 0 {
		p.RemoteMaxAttempts = defaultRemoteReconnectAttempts
	}
	if p.RemoteInitialDelay <= 0 {
		p.RemoteInitialDelay = defaultRemoteReconnectInitialDelay
	}
	if p.RemoteMaxDelay <= 0 {
		p.RemoteMaxDelay = defaultRemoteReconnectMaxDelay
	}
	if p.RemoteJitterFraction < 0 {
		p.RemoteJitterFraction = 0
	}
	if p.RemoteJitterFraction == 0 && p.RemoteInitialDelay == defaultRemoteReconnectInitialDelay && p.RemoteMaxDelay == defaultRemoteReconnectMaxDelay {
		p.RemoteJitterFraction = defaultRemoteReconnectJitterFraction
	}
	if p.ConnectionLostThreshold <= 0 {
		p.ConnectionLostThreshold = defaultConnectionLostThreshold
	}
	if len(p.StdioCooldowns) == 0 {
		p.StdioCooldowns = append([]time.Duration(nil), defaultStdioReconnectCooldowns...)
	} else {
		p.StdioCooldowns = append([]time.Duration(nil), p.StdioCooldowns...)
	}
	return p
}

func (p ReconnectPolicy) maxAttemptsFor(config MCPServerConfig) int {
	if isLocalManagerTransport(config) {
		return len(p.withDefaults().StdioCooldowns)
	}
	p = p.withDefaults()
	if p.ConnectionLostThreshold > 0 && p.ConnectionLostThreshold < p.RemoteMaxAttempts {
		return p.ConnectionLostThreshold
	}
	return p.RemoteMaxAttempts
}

func (p ReconnectPolicy) delayBeforeAttempt(config MCPServerConfig, attempt int) time.Duration {
	p = p.withDefaults()
	if isLocalManagerTransport(config) {
		if attempt <= 0 || attempt > len(p.StdioCooldowns) {
			return 0
		}
		return p.StdioCooldowns[attempt-1]
	}
	if attempt <= 1 {
		return 0
	}
	delay := p.RemoteInitialDelay
	for i := 2; i < attempt; i++ {
		delay *= 2
		if delay >= p.RemoteMaxDelay {
			delay = p.RemoteMaxDelay
			break
		}
	}
	if delay > p.RemoteMaxDelay {
		delay = p.RemoteMaxDelay
	}
	return jitterDuration(delay, p.RemoteJitterFraction)
}

func jitterDuration(base time.Duration, fraction float64) time.Duration {
	if base <= 0 || fraction <= 0 {
		return base
	}
	jitter := (rand.Float64()*2 - 1) * fraction
	return base + time.Duration(float64(base)*jitter)
}

func (m *Manager) watchClientClosed(name string, config MCPServerConfig, client *Client) {
	if m == nil || client == nil {
		return
	}
	go func() {
		<-client.done
		m.handleClientClosed(name, config, client, client.closedError())
	}()
}

func (m *Manager) handleClientClosed(name string, config MCPServerConfig, client *Client, cause error) {
	if m == nil || client == nil {
		return
	}
	hash := HashMCPConfig(config)
	m.mu.Lock()
	state, ok := m.states[name]
	if !ok || state.Client != client || state.Type != MCPStateConnected || state.ConfigHash != hash || m.disabled[name] {
		m.mu.Unlock()
		return
	}
	state.Client = nil
	state.Type = MCPStatePending
	state.Tools = nil
	state.Resources = nil
	state.Prompts = nil
	state.Error = reconnectErrorString(cause)
	state.ReconnectAttempt = 0
	state.MaxReconnectAttempts = m.reconnectPolicy.withDefaults().maxAttemptsFor(config)
	m.cache.ClearServer(name)
	m.setStateLocked(state)
	if IsSessionExpiredError(cause) {
		m.cancelReconnectLocked(name)
		m.mu.Unlock()
		return
	}
	m.scheduleReconnectLocked(name, config, cause)
	m.mu.Unlock()
}

func (m *Manager) scheduleReconnectLocked(name string, config MCPServerConfig, cause error) {
	if m == nil || name == "" {
		return
	}
	m.cancelReconnectLocked(name)
	ctx, cancel := context.WithCancel(context.Background())
	if m.reconnectTimers == nil {
		m.reconnectTimers = make(map[string]context.CancelFunc)
	}
	m.reconnectTimers[name] = cancel
	policy := m.reconnectPolicy.withDefaults()
	go m.reconnectLoop(ctx, name, config, policy, cause)
}

func (m *Manager) reconnectLoop(ctx context.Context, name string, config MCPServerConfig, policy ReconnectPolicy, cause error) {
	maxAttempts := policy.maxAttemptsFor(config)
	var lastErr error = cause
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		delay := policy.delayBeforeAttempt(config, attempt)
		if !sleepContext(ctx, delay) {
			return
		}
		if !m.beginReconnectAttempt(ctx, name, config, attempt, maxAttempts, lastErr) {
			return
		}
		result, catalog := m.connect(ctx, name, config)
		if result.Type == MCPStateFailed && result.Error != "" {
			lastErr = errors.New(result.Error)
		} else {
			lastErr = nil
		}
		if m.finishReconnectAttempt(ctx, name, config, result, catalog, attempt, maxAttempts, lastErr) {
			return
		}
	}
}

func (m *Manager) beginReconnectAttempt(ctx context.Context, name string, config MCPServerConfig, attempt, maxAttempts int, cause error) bool {
	if ctx.Err() != nil || m == nil {
		return false
	}
	hash := HashMCPConfig(config)
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.configs[name]
	if !ok || HashMCPConfig(current) != hash || current.Scope != config.Scope || m.disabled[name] {
		m.cancelReconnectLocked(name)
		return false
	}
	state := m.states[name]
	if state.Client != nil && state.Type == MCPStateConnected {
		m.cancelReconnectLocked(name)
		return false
	}
	state.Name = name
	state.Type = MCPStatePending
	state.Config = config
	state.ConfigHash = hash
	state.Client = nil
	state.Tools = nil
	state.Resources = nil
	state.Prompts = nil
	state.Error = reconnectErrorString(cause)
	state.ReconnectAttempt = attempt
	state.MaxReconnectAttempts = maxAttempts
	m.setStateLocked(state)
	return true
}

// finishReconnectAttempt returns true when the loop is complete.
func (m *Manager) finishReconnectAttempt(ctx context.Context, name string, config MCPServerConfig, result MCPServerConnection, catalog *connectionCatalogPublication, attempt, maxAttempts int, cause error) bool {
	if m == nil {
		if result.Client != nil {
			_ = result.Client.Close()
		}
		return true
	}
	hash := HashMCPConfig(config)
	var listener ConnectionLostListener
	var event ConnectionLostEvent
	m.mu.Lock()
	current, ok := m.configs[name]
	if ctx.Err() != nil || !ok || HashMCPConfig(current) != hash || current.Scope != config.Scope || m.disabled[name] {
		m.cancelReconnectLocked(name)
		m.mu.Unlock()
		if result.Client != nil {
			_ = result.Client.Close()
		}
		return true
	}
	result.ReconnectAttempt = attempt
	result.MaxReconnectAttempts = maxAttempts
	switch result.Type {
	case MCPStateConnected:
		m.cancelReconnectLocked(name)
		m.publishConnectionCatalogLocked(catalog)
		m.setStateLocked(result)
		m.mu.Unlock()
		if result.Client != nil {
			m.watchClientClosed(name, config, result.Client)
		}
		return true
	case MCPStateNeedsAuth, MCPStateDisabled:
		m.cancelReconnectLocked(name)
		m.setStateLocked(result)
		m.mu.Unlock()
		return true
	case MCPStateFailed:
		if attempt >= maxAttempts {
			m.cancelReconnectLocked(name)
			m.setStateLocked(result)
			listener = m.connectionLostHandler
			event = ConnectionLostEvent{
				ServerName: name,
				Transport:  config.Type,
				Attempts:   attempt,
				Err:        cause,
			}
			m.mu.Unlock()
			if listener != nil {
				listener(event)
			}
			return true
		}
		result.Type = MCPStatePending
		m.setStateLocked(result)
	}
	m.mu.Unlock()
	return false
}

func (m *Manager) cancelReconnectLocked(name string) {
	if m == nil || name == "" || m.reconnectTimers == nil {
		return
	}
	if cancel := m.reconnectTimers[name]; cancel != nil {
		cancel()
	}
	delete(m.reconnectTimers, name)
}

func (m *Manager) cancelAllReconnectLocked() {
	if m == nil || m.reconnectTimers == nil {
		return
	}
	for name, cancel := range m.reconnectTimers {
		if cancel != nil {
			cancel()
		}
		delete(m.reconnectTimers, name)
	}
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func reconnectErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
