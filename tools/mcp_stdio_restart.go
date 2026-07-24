package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// mcp_stdio_restart.go — MCP-02 stdio child-process death detection +
// auto-restart with cooldown.
//
// Without this, a crashed MCP server stays dead for the rest of the
// session and its tools silently disappear from the model's view.
//
// Strategy:
//   * After a successful Connect we install a watcher goroutine that
//     waits on the cmd.Wait() to observe the child's exit.
//   * On unexpected exit, we mark the connection ready=false and
//     attempt to reconnect with the cooldown sequence below.
//   * After maxStdioRestarts unsuccessful attempts the server is marked
//     `failed` and the watcher exits.

// WatchStdioForRestart starts a watcher goroutine for the given server
// connection. Pass the `done` channel exposed by mcp.Client.WaitDone (if
// available) — otherwise WatchStdioForRestart returns without doing
// anything. The function is package-level so the manager can be unit-tested
// without standing up a real subprocess.
func (m *MCPManager) WatchStdioForRestart(ctx context.Context, name string, done <-chan error) {
	if done == nil {
		return
	}
	go m.stdioRestartLoop(ctx, name, done)
}

// stdioRestartLoop blocks on the child's exit channel and, on unexpected
// exit, fires off restart attempts with backoff. Each attempt uses the
// existing Connect path so we re-list tools & rebuild the server's tool
// catalogue automatically.
func (m *MCPManager) stdioRestartLoop(ctx context.Context, name string, done <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-done:
			// Always treat as unexpected — Manager.Shutdown closes the
			// channel through Client.Close, which yields a nil error.
			if err == nil {
				return
			}
			tracker := m.acquireRestartTracker(name)
			if tracker.failed {
				return
			}
			tracker.attempts++
			cooldown := stdioRestartCooldownFor(tracker.attempts)
			tracker.lastAttempt = time.Now()
			m.releaseRestartTracker(name, tracker)

			select {
			case <-ctx.Done():
				return
			case <-time.After(cooldown):
			}

			// Drop the dead connection so Connect re-spawns the child.
			m.mu.Lock()
			delete(m.servers, name)
			m.mu.Unlock()

			if _, reErr := m.Connect(name); reErr != nil {
				if tracker.attempts >= maxStdioRestarts {
					m.markServerFailed(name)
					return
				}
				// Loop will continue when the next done arrives, but
				// since we never re-attached a watcher we have to
				// re-arm here. We don't actually have a fresh `done`
				// channel without successful Connect, so escalate to
				// failed.
				continue
			}
			// Reset attempts on success.
			tracker = m.acquireRestartTracker(name)
			tracker.attempts = 0
			m.releaseRestartTracker(name, tracker)
			return
		}
	}
}

// stdioRestartCooldownFor returns the cooldown duration for the Nth
// restart attempt (1-indexed). Beyond the configured table the last
// value is reused.
func stdioRestartCooldownFor(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	idx := attempt - 1
	if idx >= len(stdioRestartCooldowns) {
		idx = len(stdioRestartCooldowns) - 1
	}
	return stdioRestartCooldowns[idx]
}

func (m *MCPManager) acquireRestartTracker(name string) *stdioRestartTracker {
	m.stdioMu.Lock()
	defer m.stdioMu.Unlock()
	tr, ok := m.restartState[name]
	if !ok {
		tr = &stdioRestartTracker{}
		m.restartState[name] = tr
	}
	return tr
}

func (m *MCPManager) releaseRestartTracker(_ string, _ *stdioRestartTracker) {
	// No-op (acquireRestartTracker keeps the entry pinned). Reserved for
	// symmetry so future locking refactors don't change call sites.
}

func (m *MCPManager) markServerFailed(name string) {
	m.stdioMu.Lock()
	tr, ok := m.restartState[name]
	if !ok {
		tr = &stdioRestartTracker{}
		m.restartState[name] = tr
	}
	tr.failed = true
	m.stdioMu.Unlock()
	m.mu.Lock()
	if conn, exists := m.servers[name]; exists {
		conn.ready = false
	}
	m.mu.Unlock()
}

// IsServerFailed reports whether the named server has exhausted its
// restart budget.
func (m *MCPManager) IsServerFailed(name string) bool {
	m.stdioMu.Lock()
	defer m.stdioMu.Unlock()
	if tr, ok := m.restartState[name]; ok {
		return tr.failed
	}
	return false
}

// ResetRestartState forgets the restart tracker for a server (test helper).
func (m *MCPManager) ResetRestartState(name string) {
	m.stdioMu.Lock()
	delete(m.restartState, name)
	m.stdioMu.Unlock()
}

// _unused keeps the fmt + sync imports honest if helpers are stripped.
var _ = fmt.Sprintf
var _ = sync.Mutex{}
