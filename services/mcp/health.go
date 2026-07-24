package mcp

import (
	"sort"
	"time"
)

// HealthSnapshot is a point-in-time, UI/tool-safe view of MCP connectivity.
type HealthSnapshot struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	Counts      HealthCounts   `json:"counts"`
	Servers     []ServerHealth `json:"servers"`
}

type HealthCounts struct {
	Pending   int `json:"pending"`
	Connected int `json:"connected"`
	Failed    int `json:"failed"`
	NeedsAuth int `json:"needsAuth"`
	Disabled  int `json:"disabled"`
}

type ServerHealth struct {
	Name                 string             `json:"name"`
	State                MCPConnectionState `json:"state"`
	Transport            TransportType      `json:"transport,omitempty"`
	ConfigHash           string             `json:"configHash,omitempty"`
	Connected            bool               `json:"connected"`
	Pending              bool               `json:"pending"`
	Failed               bool               `json:"failed"`
	NeedsAuth            bool               `json:"needsAuth"`
	Disabled             bool               `json:"disabled"`
	ReconnectAttempt     int                `json:"reconnectAttempt,omitempty"`
	MaxReconnectAttempts int                `json:"maxReconnectAttempts,omitempty"`
	Error                string             `json:"error,omitempty"`
	ServerInfo           *ServerInfo        `json:"serverInfo,omitempty"`
}

// HealthSnapshot returns a stable, sorted snapshot of all known MCP servers.
func (m *Manager) HealthSnapshot() HealthSnapshot {
	snapshot := HealthSnapshot{
		GeneratedAt: time.Now(),
	}
	if m == nil {
		return snapshot
	}
	m.mu.Lock()
	states := m.snapshotLocked()
	m.mu.Unlock()

	snapshot.Servers = make([]ServerHealth, 0, len(states))
	for _, state := range states {
		health := serverHealthFromState(state)
		snapshot.Servers = append(snapshot.Servers, health)
		switch state.Type {
		case MCPStatePending:
			snapshot.Counts.Pending++
		case MCPStateConnected:
			snapshot.Counts.Connected++
		case MCPStateFailed:
			snapshot.Counts.Failed++
		case MCPStateNeedsAuth:
			snapshot.Counts.NeedsAuth++
		case MCPStateDisabled:
			snapshot.Counts.Disabled++
		}
	}
	return snapshot
}

func serverHealthFromState(state MCPServerConnection) ServerHealth {
	return ServerHealth{
		Name:                 state.Name,
		State:                state.Type,
		Transport:            state.Config.Type,
		ConfigHash:           state.ConfigHash,
		Connected:            state.Type == MCPStateConnected,
		Pending:              state.Type == MCPStatePending,
		Failed:               state.Type == MCPStateFailed,
		NeedsAuth:            state.Type == MCPStateNeedsAuth,
		Disabled:             state.Type == MCPStateDisabled,
		ReconnectAttempt:     state.ReconnectAttempt,
		MaxReconnectAttempts: state.MaxReconnectAttempts,
		Error:                state.Error,
		ServerInfo:           cloneServerInfo(state.ServerInfo),
	}
}

// ServerNamesByState returns sorted server names in a given connection state.
func (m *Manager) ServerNamesByState(state MCPConnectionState) []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0)
	for name, conn := range m.states {
		if conn.Type == state {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (m *Manager) PendingServerNames() []string {
	return m.ServerNamesByState(MCPStatePending)
}

func (m *Manager) FailedServerNames() []string {
	return m.ServerNamesByState(MCPStateFailed)
}

func (m *Manager) NeedsAuthServerNames() []string {
	return m.ServerNamesByState(MCPStateNeedsAuth)
}
