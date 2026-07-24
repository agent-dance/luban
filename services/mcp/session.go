package mcp

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

const sessionNotFoundJSONRPCCode = -32001

// SessionExpiredError marks the MCP Streamable HTTP "session not found"
// condition. The next operation should obtain a fresh connection/session.
type SessionExpiredError struct {
	ServerName string
	Err        error
}

func (e *SessionExpiredError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPSessionExpired)
	}
	if e.ServerName == "" {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPSessionExpired)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPServerSessionExpired, e.ServerName)
}

func (e *SessionExpiredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsSessionExpiredError detects the MCP spec's stale-session shape:
// HTTP 404 paired with JSON-RPC error code -32001.
func IsSessionExpiredError(err error) bool {
	if err == nil {
		return false
	}
	var expired *SessionExpiredError
	if errors.As(err, &expired) {
		return true
	}
	var remote *RemoteHTTPError
	if !errors.As(err, &remote) {
		return false
	}
	if remote.StatusCode != 404 {
		return false
	}
	if remote.RPCError != nil && remote.RPCError.Code == sessionNotFoundJSONRPCCode {
		return true
	}
	body := strings.ReplaceAll(remote.Body, " ", "")
	return strings.Contains(body, `"code":-32001`)
}

func wrapSessionExpired(name string, err error) error {
	if err == nil {
		err = i18n.NewError(i18n.KeyServicesMCPSessionExpired)
	}
	if IsSessionExpiredError(err) {
		var expired *SessionExpiredError
		if errors.As(err, &expired) {
			return err
		}
		return &SessionExpiredError{ServerName: name, Err: err}
	}
	return err
}

// MarkSessionExpired clears live connection and catalogue caches, closes the
// stale client, and leaves the server pending so the next GetOrConnect creates
// a fresh MCP session.
func (m *Manager) MarkSessionExpired(name string, cause error) bool {
	if m == nil || name == "" || !IsSessionExpiredError(cause) {
		return false
	}
	var oldClient *Client
	m.mu.Lock()
	m.cancelReconnectLocked(name)
	state, ok := m.states[name]
	if !ok {
		m.mu.Unlock()
		return true
	}
	if state.Client != nil {
		oldClient = state.Client
	}
	state.Client = nil
	state.Type = MCPStatePending
	state.Tools = nil
	state.Resources = nil
	state.Prompts = nil
	state.Error = (&SessionExpiredError{ServerName: name, Err: cause}).Error()
	state.ReconnectAttempt = 0
	state.MaxReconnectAttempts = 0
	m.cache.ClearServer(name)
	m.setStateLocked(state)
	m.mu.Unlock()
	if oldClient != nil {
		_ = oldClient.Close()
	}
	return true
}

// MarkNeedsAuth records an auth failure from a live tool call, closes the
// stale client, and moves the server into needs-auth so dynamic registration
// can expose the authenticate pseudo-tool.
func (m *Manager) MarkNeedsAuth(name string, cause error) bool {
	if m == nil || name == "" || !IsAuthRequiredError(cause) {
		return false
	}
	var oldClient *Client
	m.mu.Lock()
	config, ok := m.configs[name]
	if !ok {
		m.mu.Unlock()
		return false
	}
	needsAuth, ok := RecordNeedsAuthFromError(m.needsAuthCache, name, config, cause)
	if !ok {
		m.mu.Unlock()
		return false
	}
	state := m.states[name]
	if state.Client != nil {
		oldClient = state.Client
	}
	m.cancelReconnectLocked(name)
	m.cache.ClearServer(name)
	state = MCPServerConnection{
		Name:       name,
		Type:       MCPStateNeedsAuth,
		Config:     config,
		ConfigHash: HashMCPConfig(config),
		NeedsAuth:  &needsAuth,
		Error:      needsAuth.Reason,
	}
	m.setStateLocked(state)
	m.mu.Unlock()
	if oldClient != nil {
		_ = oldClient.Close()
	}
	return true
}

// RecoverExpiredSession applies MarkSessionExpired and immediately reconnects.
// It is intentionally connection-level; callers decide whether any higher-level
// tool/resource operation is safe to retry.
func (m *Manager) RecoverExpiredSession(ctx context.Context, name string, cause error) (MCPServerConnection, bool, error) {
	if m == nil {
		return MCPServerConnection{}, false, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if !m.MarkSessionExpired(name, cause) {
		return MCPServerConnection{}, false, nil
	}
	state, err := m.GetOrConnect(ctx, name)
	return state, true, err
}

type sessionAwareTransport struct {
	name string
	base Transport

	mu         sync.RWMutex
	expiredErr error
}

func wrapSessionAwareTransport(name string, base Transport) Transport {
	if base == nil {
		return nil
	}
	return &sessionAwareTransport{name: name, base: base}
}

func (t *sessionAwareTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	err := t.base.Send(ctx, msg)
	if IsSessionExpiredError(err) {
		err = wrapSessionExpired(t.name, err)
		t.recordExpired(err)
		_ = t.base.Close()
	}
	return err
}

func (t *sessionAwareTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	msg, err := t.base.Receive(ctx)
	if IsSessionExpiredError(err) {
		err = wrapSessionExpired(t.name, err)
		t.recordExpired(err)
		_ = t.base.Close()
		return msg, err
	}
	if err != nil && errors.Is(err, ErrTransportClosed) {
		if expired := t.sessionExpiredError(); expired != nil {
			return msg, expired
		}
	}
	return msg, err
}

func (t *sessionAwareTransport) Close() error {
	if t == nil || t.base == nil {
		return nil
	}
	return t.base.Close()
}

func (t *sessionAwareTransport) recordExpired(err error) {
	if t == nil || err == nil {
		return
	}
	t.mu.Lock()
	if t.expiredErr == nil {
		t.expiredErr = err
	}
	t.mu.Unlock()
}

func (t *sessionAwareTransport) sessionExpiredError() error {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.expiredErr
}
