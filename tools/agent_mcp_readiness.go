package tools

// agent_mcp_readiness.go implements the WaitForMCPReadiness gate described by
// tasks/agent.json subtask agent-07. The TS reference (mcpReadiness.ts) blocks
// sub-agent start until every MCP server referenced via the tool allow-list
// (mcp__<server>__*) reports ready, with a configurable timeout.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
)

const defaultAgentMCPReadinessTimeout = 30 * time.Second

func requiredMCPServersForProfile(profile agentProfile, probe MCPReadinessProbe) []string {
	seen := map[string]struct{}{}
	add := func(name string) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" && name != "*" {
			seen[name] = struct{}{}
		}
	}
	for _, name := range profile.RequiredMCPServers {
		add(name)
	}
	for _, name := range RequiredMCPServersFromAllowList(profile.AllowedToolSpecs) {
		add(name)
	}
	allConfigured := false
	for _, spec := range profile.AllowedToolSpecs {
		normalized := strings.ToLower(strings.TrimSpace(spec))
		if normalized == "mcp__*" || normalized == "mcp__*__*" {
			allConfigured = true
			break
		}
	}
	if allConfigured && probe != nil {
		for _, name := range probe.ServerNames() {
			add(name)
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (t *AgentTool) effectiveMCPReadinessProbe(source *registry.Registry) MCPReadinessProbe {
	if t == nil {
		return nil
	}
	if probe := t.MCPReadinessProbeHook(); probe != nil {
		return probe
	}
	if source == nil {
		return nil
	}
	if mcpTool, ok := source.Get("MCPTool").(*MCPTool); ok && mcpTool != nil && mcpTool.manager != nil {
		return mcpTool.manager
	}
	return nil
}

func (t *AgentTool) waitForMCPReadiness(ctx context.Context, profile agentProfile) (MCPReadinessReport, error) {
	return t.waitForMCPReadinessUsingRegistry(ctx, profile, t.Registry)
}

func (t *AgentTool) waitForMCPReadinessUsingRegistry(ctx context.Context, profile agentProfile, source *registry.Registry) (MCPReadinessReport, error) {
	probe := t.effectiveMCPReadinessProbe(source)
	required := requiredMCPServersForProfile(profile, probe)
	if len(required) == 0 {
		return MCPReadinessReport{}, nil
	}
	t.runtimeMu.Lock()
	timeout := t.mcpTimeout
	t.runtimeMu.Unlock()
	if timeout <= 0 {
		timeout = defaultAgentMCPReadinessTimeout
	}
	return WaitForMCPReadiness(ctx, probe, required, timeout)
}

// MCPReadinessProbe is the minimal contract WaitForMCPReadiness needs from a
// manager. Production code uses *MCPManager; tests can supply a fake.
type MCPReadinessProbe interface {
	// ServerNames returns the configured server names.
	ServerNames() []string
	// Connect attempts to bring a configured server online and returns nil on success.
	Connect(name string) (*MCPServerConn, error)
}

// MCPReadinessReport summarises the outcome of a readiness gate.
type MCPReadinessReport struct {
	Required  []string
	Ready     []string
	Failed    map[string]string
	ElapsedMs int64
}

// IsReady reports whether every required server reported ready.
func (r MCPReadinessReport) IsReady() bool {
	if len(r.Required) == 0 {
		return true
	}
	if len(r.Failed) > 0 {
		return false
	}
	if len(r.Ready) < len(r.Required) {
		return false
	}
	return true
}

// RequiredMCPServersFromAllowList extracts server names from a tool allow-list,
// matching mcp__<server>__<tool> entries (TS uses the same convention).
// Globs in the form `mcp__<server>__*` are supported. Wildcard `mcp__*`
// matches no specific server (caller may interpret it as "all configured").
func RequiredMCPServersFromAllowList(allowList []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, entry := range allowList {
		trimmed := strings.TrimSpace(entry)
		if !strings.HasPrefix(trimmed, "mcp__") {
			continue
		}
		parts := strings.Split(trimmed, "__")
		if len(parts) < 2 {
			continue
		}
		server := strings.ToLower(strings.TrimSpace(parts[1]))
		if server == "" || server == "*" {
			continue
		}
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		out = append(out, server)
	}
	return out
}

// WaitForMCPReadiness blocks until every server in requiredServers reports
// ready, or until the context is cancelled / timeout elapses. The timeout is
// applied as a deadline on a derived context. Returning nil means all required
// servers are ready; the report carries timings either way.
func WaitForMCPReadiness(
	ctx context.Context,
	probe MCPReadinessProbe,
	requiredServers []string,
	timeout time.Duration,
) (MCPReadinessReport, error) {
	report := MCPReadinessReport{
		Required: append([]string(nil), requiredServers...),
		Failed:   map[string]string{},
	}
	if len(requiredServers) == 0 {
		return report, nil
	}
	if probe == nil {
		report.Failed["__manager__"] = toolRuntimeText(i18n.KeyToolAgentMCPManagerMissingDetail)
		return report, i18n.NewError(i18n.KeyToolAgentMCPManagerNotConfigured)
	}
	start := time.Now()
	deadline := time.Time{}
	if timeout > 0 {
		deadline = start.Add(timeout)
	}

	configured := map[string]struct{}{}
	for _, name := range probe.ServerNames() {
		configured[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	for _, server := range requiredServers {
		key := strings.ToLower(strings.TrimSpace(server))
		if key == "" {
			continue
		}
		if _, ok := configured[key]; !ok {
			report.Failed[key] = toolRuntimeText(i18n.KeyToolAgentMCPServerNotConfiguredDetail)
			continue
		}
	}

	if len(report.Failed) > 0 {
		report.ElapsedMs = time.Since(start).Milliseconds()
		var missing []string
		for k := range report.Failed {
			missing = append(missing, k)
		}
		return report, i18n.NewError(i18n.KeyToolAgentMCPRequiredServersNotConfigured, strings.Join(missing, ", "))
	}

	for _, server := range requiredServers {
		key := strings.ToLower(strings.TrimSpace(server))
		if key == "" {
			continue
		}
		// Honour cancellation between attempts.
		select {
		case <-ctx.Done():
			report.ElapsedMs = time.Since(start).Milliseconds()
			return report, ctx.Err()
		default:
		}
		serverCtx := ctx
		var cancel context.CancelFunc
		if !deadline.IsZero() {
			serverCtx, cancel = context.WithDeadline(ctx, deadline)
		}
		err := connectMCPWithRetry(serverCtx, probe, key, deadline)
		if cancel != nil {
			cancel()
		}
		if err != nil {
			report.Failed[key] = err.Error()
			continue
		}
		report.Ready = append(report.Ready, key)
	}
	report.ElapsedMs = time.Since(start).Milliseconds()
	if len(report.Failed) > 0 {
		var failed []string
		for k, v := range report.Failed {
			failed = append(failed, fmt.Sprintf("%s: %s", k, v))
		}
		return report, i18n.NewError(i18n.KeyToolAgentMCPReadinessFailed, strings.Join(failed, "; "))
	}
	return report, nil
}

// connectMCPWithRetry retries Connect on transient errors until the deadline
// (or ctx) elapses. Backoff starts at 200ms and caps at 1s.
func connectMCPWithRetry(ctx context.Context, probe MCPReadinessProbe, name string, deadline time.Time) error {
	backoff := 200 * time.Millisecond
	for {
		_, err := probe.Connect(name)
		if err == nil {
			return nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return i18n.WrapError(i18n.KeyToolAgentMCPReadinessTimedOutWithCause, err)
		}
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return i18n.WrapError(i18n.KeyToolAgentMCPReadinessTimedOutWithCause, ctx.Err())
			}
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < time.Second {
			backoff *= 2
		}
		if !deadline.IsZero() && time.Now().Add(backoff).After(deadline) {
			// One last attempt right at the deadline.
			if _, err := probe.Connect(name); err == nil {
				return nil
			}
			return i18n.NewError(i18n.KeyToolAgentMCPReadinessTimedOut)
		}
	}
}
