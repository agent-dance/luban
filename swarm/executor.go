package swarm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	tmuxpkg "github.com/agent-dance/luban/tmux"
)

// TeammateExecutor spawns and manages LUBAN Code teammate processes in tmux panes.
type TeammateExecutor struct {
	backend  tmuxpkg.TmuxBackend
	mailbox  *Mailbox
	teamName string
	cwd      string
	binary   string // path to luban-code binary

	cleanupMu       sync.Mutex
	pendingCleanup  map[string]TeamMember
	terminatedPanes map[string]struct{}
	// sendMessage is an internal seam for deterministic failure injection. It
	// defaults to mailbox.Send and is intentionally not part of the public API.
	sendMessage func(context.Context, string, Message) error
}

// NewTeammateExecutor creates a TeammateExecutor.
// The binary path defaults to "./luban-code" relative to cwd if empty.
func NewTeammateExecutor(backend tmuxpkg.TmuxBackend, teamName, cwd string) (*TeammateExecutor, error) {
	mb, err := NewMailbox(teamName)
	if err != nil {
		return nil, fmt.Errorf("new teammate executor: %w", err)
	}
	executor := &TeammateExecutor{
		backend:         backend,
		mailbox:         mb,
		teamName:        teamName,
		cwd:             cwd,
		binary:          filepath.Join(cwd, brand.CommandName),
		pendingCleanup:  make(map[string]TeamMember),
		terminatedPanes: make(map[string]struct{}),
	}
	executor.sendMessage = mb.Send
	return executor, nil
}

// SpawnOpts configures a new teammate.
type SpawnOpts struct {
	Name        string
	Task        string
	Color       string
	Model       string   // optional model override
	LeaderPane  string   // leader's pane ID for split targeting
	Index       int      // teammate index (for layout decisions)
	AllowAll    bool     // if true, pass --allow-all; default is least-privilege
	Permissions []string // explicit permission flags (e.g., "--allow-tool Bash")
}

// Spawn creates a new teammate in a tmux pane and sends it an initial task.
// It returns the TeamMember descriptor that can be persisted in TeamConfig.
func (e *TeammateExecutor) Spawn(ctx context.Context, opts SpawnOpts) (*TeamMember, error) {
	if e == nil || e.backend == nil {
		return nil, fmt.Errorf("spawn %s: tmux backend is not configured", opts.Name)
	}
	if err := validateName(opts.Name, "agent name"); err != nil {
		return nil, fmt.Errorf("spawn %s: %w", opts.Name, err)
	}
	if !e.backend.Available() {
		return nil, fmt.Errorf("spawn %s: tmux backend is unavailable", opts.Name)
	}
	// 1. Assign color from palette if not provided.
	colorName, tmuxColor := tmuxpkg.AssignColor(opts.Index)
	if opts.Color != "" {
		if err := tmuxpkg.ValidateColor(opts.Color); err != nil {
			return nil, fmt.Errorf("spawn %s: invalid color: %w", opts.Name, err)
		}
		colorName = opts.Color
		tmuxColor = opts.Color
	}
	// Finish every validation and command-construction step before acquiring a
	// pane. A preflight failure therefore has no resource to compensate.
	cmd, err := buildCommand(e.binary, e.cwd, opts.Name, e.teamName, opts.Model, opts.AllowAll, opts.Permissions)
	if err != nil {
		return nil, fmt.Errorf("spawn %s: %w", opts.Name, err)
	}

	// 2. Create or split a pane.
	var paneID string

	if e.backend.InsideTmux() && opts.LeaderPane != "" {
		paneID, err = e.backend.SplitPane(ctx, opts.LeaderPane, true, 50)
		if err != nil {
			return nil, fmt.Errorf("spawn %s: split pane: %w", opts.Name, err)
		}
	} else {
		sessionName := fmt.Sprintf("%s-%s", e.teamName, opts.Name)
		paneID, err = e.backend.CreateSession(ctx, sessionName)
		if err != nil {
			return nil, fmt.Errorf("spawn %s: create session: %w", opts.Name, err)
		}
	}
	member := TeamMember{
		AgentID: opts.Name, Name: opts.Name, Color: colorName,
		TmuxPaneID: paneID, BackendType: "tmux", CWD: e.cwd, IsActive: true,
	}
	rollback := func(primary error) (*TeamMember, error) {
		cleanupCtx := context.WithoutCancel(ctx)
		if killErr := e.backend.KillPane(cleanupCtx, paneID); killErr != nil {
			e.rememberPendingCleanup(member)
			return nil, errors.Join(primary, fmt.Errorf("rollback pane %s: %w", paneID, killErr))
		}
		e.markPaneTerminated(paneID)
		return nil, primary
	}

	// 3. Set pane title and border color (non-fatal cosmetic ops).
	title := fmt.Sprintf(" %s [%s] ", opts.Name, e.teamName)
	if setErr := e.backend.SetPaneTitle(ctx, paneID, title, tmuxColor); setErr != nil {
		slog.Debug(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxSwarmPaneTitle), "pane", paneID, "err", setErr)
	}
	if setErr := e.backend.SetPaneBorderColor(ctx, paneID, tmuxColor); setErr != nil {
		slog.Debug(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxSwarmPaneBorder), "pane", paneID, "err", setErr)
	}

	// 4. Send keys to the pane to start the process.
	if err := e.backend.SendKeys(ctx, paneID, cmd); err != nil {
		return rollback(fmt.Errorf("spawn %s: send keys: %w", opts.Name, err))
	}

	// 5. Send the initial task via mailbox. Its UUID and timestamp are prepared
	// once so an uncertain retry cannot append a second logical task.
	if opts.Task != "" {
		msg := Message{
			ID:        NewMessageID(),
			From:      "leader",
			Text:      opts.Task,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Color:     colorName,
		}
		send := e.sendMessage
		if send == nil {
			send = e.mailbox.Send
		}
		if err := send(ctx, opts.Name, msg); err != nil {
			return rollback(fmt.Errorf("spawn %s: send task: %w", opts.Name, err))
		}
	}

	// 6. Rebalance the layout (tiled gives equal space to all panes).
	if e.backend.InsideTmux() && opts.LeaderPane != "" {
		if layoutErr := e.backend.SelectLayout(ctx, paneID, "tiled"); layoutErr != nil {
			slog.Debug(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxSwarmLayout), "pane", paneID, "err", layoutErr)
		}
	}

	return &member, nil
}

// Cleanup kills all teammate panes and removes the team config.
// Collects all errors rather than silently dropping earlier ones.
func (e *TeammateExecutor) Cleanup(ctx context.Context, members []TeamMember) error {
	if e == nil || e.backend == nil {
		return fmt.Errorf("cleanup: tmux backend is not configured")
	}
	e.cleanupMu.Lock()
	all := make([]TeamMember, 0, len(members)+len(e.pendingCleanup))
	all = append(all, members...)
	for _, member := range e.pendingCleanup {
		all = append(all, member)
	}
	e.cleanupMu.Unlock()

	var errs []error
	seen := make(map[string]struct{}, len(all))
	for _, m := range all {
		if m.TmuxPaneID == "" {
			continue
		}
		if _, duplicate := seen[m.TmuxPaneID]; duplicate {
			continue
		}
		seen[m.TmuxPaneID] = struct{}{}
		e.cleanupMu.Lock()
		_, alreadyTerminated := e.terminatedPanes[m.TmuxPaneID]
		e.cleanupMu.Unlock()
		if alreadyTerminated {
			continue
		}
		if err := e.backend.KillPane(ctx, m.TmuxPaneID); err != nil {
			errs = append(errs, fmt.Errorf("kill pane %s (%s): %w", m.TmuxPaneID, m.Name, err))
			e.rememberPendingCleanup(m)
			continue
		}
		e.markPaneTerminated(m.TmuxPaneID)
	}
	// Never erase the only durable cleanup inventory while a pane is still
	// alive. A later Cleanup call retries exactly those failed resources.
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if err := DeleteTeamConfig(e.teamName); err != nil {
		errs = append(errs, fmt.Errorf("delete team config: %w", err))
	} else {
		e.cleanupMu.Lock()
		clear(e.pendingCleanup)
		clear(e.terminatedPanes)
		e.cleanupMu.Unlock()
	}
	return errors.Join(errs...)
}

func (e *TeammateExecutor) rememberPendingCleanup(member TeamMember) {
	if e == nil || strings.TrimSpace(member.TmuxPaneID) == "" {
		return
	}
	e.cleanupMu.Lock()
	if e.pendingCleanup == nil {
		e.pendingCleanup = make(map[string]TeamMember)
	}
	e.pendingCleanup[member.TmuxPaneID] = member
	e.cleanupMu.Unlock()
}

func (e *TeammateExecutor) markPaneTerminated(paneID string) {
	if e == nil || strings.TrimSpace(paneID) == "" {
		return
	}
	e.cleanupMu.Lock()
	if e.terminatedPanes == nil {
		e.terminatedPanes = make(map[string]struct{})
	}
	e.terminatedPanes[paneID] = struct{}{}
	delete(e.pendingCleanup, paneID)
	e.cleanupMu.Unlock()
}

// allowedFlagNames is the whitelist of recognized permission flag names.
// --allow-all is intentionally excluded — it must be set via SpawnOpts.AllowAll
// to prevent privilege escalation via the permissions array.
var allowedFlagNames = map[string]bool{
	"--allow-tool":    true,
	"--disallow-tool": true,
	"--allow-mcp":     true,
	"--disallow-mcp":  true,
}

// Quoting model (two-layer shell parsing):
// Layer 1: tmux pane shell parses: env -i K=V sh -c '<inner>'
// Layer 2: sh -c parses <inner> which contains shell-quoted args
// Therefore: values inside inner must be quoted for Layer 2 (sh -c).
//
//	The entire inner is quoted for Layer 1 (pane shell).
func buildCommand(binary, cwd, agentName, teamName, model string, allowAll bool, permissions []string) (string, error) {
	if binary == "" {
		return "", fmt.Errorf("buildCommand: binary path must not be empty")
	}
	if cwd == "" {
		return "", fmt.Errorf("buildCommand: cwd must not be empty")
	}
	permFlags, err := buildPermFlags(allowAll, permissions)
	if err != nil {
		return "", fmt.Errorf("buildCommand: %w", err)
	}
	modelFlag, err := buildModelFlag(model)
	if err != nil {
		return "", fmt.Errorf("buildCommand: %w", err)
	}

	// Quote all values, propagating errors for newline/CR.
	qCwd, err := shellQuote(cwd)
	if err != nil {
		return "", fmt.Errorf("buildCommand: cwd: %w", err)
	}
	qBinary, err := shellQuote(binary)
	if err != nil {
		return "", fmt.Errorf("buildCommand: binary: %w", err)
	}
	qAgent, err := shellQuote(agentName)
	if err != nil {
		return "", fmt.Errorf("buildCommand: agentName: %w", err)
	}
	qTeam, err := shellQuote(teamName)
	if err != nil {
		return "", fmt.Errorf("buildCommand: teamName: %w", err)
	}

	// Sanitized environment — only pass non-empty safe vars.
	var envParts []string
	for _, kv := range []struct{ key, val string }{
		{"HOME", os.Getenv("HOME")},
		{"PATH", os.Getenv("PATH")},
		{"TERM", os.Getenv("TERM")},
		{"LANG", os.Getenv("LANG")},
	} {
		if kv.val != "" {
			q, err := shellQuote(kv.val)
			if err != nil {
				return "", fmt.Errorf("buildCommand: env %s: %w", kv.key, err)
			}
			envParts = append(envParts, fmt.Sprintf("%s=%s", kv.key, q))
		}
	}

	inner := fmt.Sprintf("cd %s && %s --agent-id %s --agent-name %s --team-name %s%s%s",
		qCwd, qBinary, qAgent, qAgent, qTeam, permFlags, modelFlag,
	)
	qInner, err := shellQuote(inner)
	if err != nil {
		return "", fmt.Errorf("buildCommand: inner: %w", err)
	}

	cmd := "env -i " + strings.Join(envParts, " ") + " sh -c " + qInner
	return cmd, nil
}

// buildPermFlags returns the permission flags string.
// Validates flag names against a whitelist; flag values are shell-quoted.
// Returns error if any flag name is unrecognized.
func buildPermFlags(allowAll bool, permissions []string) (string, error) {
	if allowAll {
		return " --allow-all", nil
	}
	var b strings.Builder
	for _, p := range permissions {
		parts := strings.SplitN(p, " ", 2)
		flagName := parts[0]
		if !allowedFlagNames[flagName] {
			return "", fmt.Errorf("unknown permission flag: %q", flagName)
		}
		b.WriteString(" " + flagName)
		if len(parts) == 2 {
			q, err := shellQuote(parts[1])
			if err != nil {
				return "", fmt.Errorf("permission value: %w", err)
			}
			b.WriteString(" " + q)
		}
	}
	return b.String(), nil
}

// buildModelFlag returns the model flag string or empty.
func buildModelFlag(model string) (string, error) {
	if model != "" {
		q, err := shellQuote(model)
		if err != nil {
			return "", fmt.Errorf("model: %w", err)
		}
		return " --model " + q, nil
	}
	return "", nil
}

// shellQuote wraps a string in single quotes, escaping any embedded single
// quotes so the result is safe to embed in a shell command.
// Returns error if the string contains newlines/carriage returns which would
// break tmux send-keys (interpreted as Enter key).
func shellQuote(s string) (string, error) {
	if strings.ContainsAny(s, "\n\r") {
		return "", fmt.Errorf("shellQuote: value contains newline/CR, unsafe for tmux send-keys: %q", s)
	}
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		if r == '\'' {
			b.WriteString("'\\''")
		} else {
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String(), nil
}
