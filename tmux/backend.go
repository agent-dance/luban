package tmux

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// safeColor matches valid tmux color values: named colors (e.g. "red",
// "blue"), hex colors (e.g. "#ff0000"), and tmux colour codes (e.g.
// "colour208"). Only alphanumeric characters and '#' are permitted —
// this rejects shell metacharacters that could be injected into tmux
// format strings.
var safeColor = regexp.MustCompile(`^[a-zA-Z0-9#]+$`)

// ValidateColor returns an error if color contains characters that are
// unsafe for embedding in tmux format strings / option values.
// Exported so callers (e.g. swarm/executor.go) can validate before passing
// a color value to SetPaneTitle or SetPaneBorderColor.
func ValidateColor(color string) error {
	if color == "" {
		return fmt.Errorf("tmux: color must not be empty")
	}
	if !safeColor.MatchString(color) {
		return fmt.Errorf("tmux: color %q contains invalid characters (only alphanumeric and '#' allowed)", color)
	}
	return nil
}

// TmuxBackend is the interface covering all public methods used by
// swarm/executor.go.  Swap in a mock in tests.
type TmuxBackend interface {
	Available() bool
	InsideTmux() bool
	CreateSession(ctx context.Context, name string) (string, error)
	SplitPane(ctx context.Context, target string, horizontal bool, sizePercent int) (string, error)
	SetPaneTitle(ctx context.Context, paneID, title, color string) error
	SetPaneBorderColor(ctx context.Context, paneID, color string) error
	SendKeys(ctx context.Context, paneID, keys string) error
	KillPane(ctx context.Context, paneID string) error
	SelectLayout(ctx context.Context, paneID, layout string) error
}

// Backend manages tmux panes for swarm teammates.
type Backend struct {
	socket     string // custom socket name for outside-tmux mode, empty = default
	insideTmux bool
}

// New detects the tmux environment and returns a Backend.
func New() *Backend {
	inside := os.Getenv("TMUX") != ""
	var socket string
	if !inside {
		socket = fmt.Sprintf("claude-swarm-%d", os.Getpid())
	}
	return &Backend{socket: socket, insideTmux: inside}
}

// Available checks if tmux is installed.
func (b *Backend) Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// InsideTmux reports whether the process is running inside a tmux session.
func (b *Backend) InsideTmux() bool {
	return b.insideTmux
}

// Socket returns the custom socket name (empty if using default).
func (b *Backend) Socket() string {
	return b.socket
}

// args prepends socket flags when a custom socket is configured.
func (b *Backend) args(subcmd ...string) []string {
	if b.socket != "" {
		return append([]string{"-L", b.socket}, subcmd...)
	}
	return subcmd
}

// run executes a tmux command and returns trimmed stdout.
func (b *Backend) run(ctx context.Context, subcmd ...string) (string, error) {
	if len(subcmd) == 0 {
		return "", fmt.Errorf("tmux: no subcommand specified")
	}
	args := b.args(subcmd...)
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tmux %s: %w: %s", subcmd[0], err, msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CreateSession creates a new detached tmux session (outside-tmux mode).
// Returns the pane ID of the initial window's pane.
func (b *Backend) CreateSession(ctx context.Context, name string) (paneID string, err error) {
	out, err := b.run(ctx,
		"new-session",
		"-d",       // detached
		"-s", name, // session name
		"-P",               // print info
		"-F", "#{pane_id}", // format: pane ID only
	)
	if err != nil {
		return "", err
	}
	return out, nil
}

// SplitPane splits an existing pane. Returns the new pane ID.
// horizontal=true splits left-right (vertical divider), false splits top-bottom.
func (b *Backend) SplitPane(ctx context.Context, targetPane string, horizontal bool, sizePercent int) (paneID string, err error) {
	splitFlag := "-v" // top-bottom
	if horizontal {
		splitFlag = "-h" // left-right
	}
	out, err := b.run(ctx,
		"split-window",
		splitFlag,
		"-t", targetPane,
		"-p", fmt.Sprintf("%d", sizePercent),
		"-P",
		"-F", "#{pane_id}",
	)
	if err != nil {
		return "", err
	}
	return out, nil
}

// SetPaneTitle sets the pane title and border format.
// Returns an error if color contains unsafe characters.
func (b *Backend) SetPaneTitle(ctx context.Context, paneID, title, color string) error {
	if err := ValidateColor(color); err != nil {
		return err
	}

	// Set the pane title via an option on the pane
	_, err := b.run(ctx,
		"select-pane",
		"-t", paneID,
		"-T", title,
	)
	if err != nil {
		return err
	}
	// Enable pane border status so title is visible
	_, err = b.run(ctx,
		"set-option",
		"-p",
		"-t", paneID,
		"pane-border-status", "top",
	)
	if err != nil {
		// Non-fatal: older tmux versions may not support per-pane options
		slog.Debug(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogTmuxBorderStatusFailed), "pane", paneID, "err", err)
	}
	// Set border format to show title with color
	format := fmt.Sprintf("#[fg=%s,bold] #{pane_title} #[default]", color)
	_, err = b.run(ctx,
		"set-option",
		"-p",
		"-t", paneID,
		"pane-border-format", format,
	)
	if err != nil {
		slog.Debug(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogTmuxBorderFormatFailed), "pane", paneID, "err", err)
	}
	return nil
}

// SetPaneBorderColor sets border color for a pane.
// Returns an error if color contains unsafe characters.
func (b *Backend) SetPaneBorderColor(ctx context.Context, paneID, color string) error {
	if err := ValidateColor(color); err != nil {
		return err
	}
	_, err := b.run(ctx,
		"select-pane",
		"-t", paneID,
		"-P", fmt.Sprintf("fg=%s", color),
	)
	return err
}

// SendKeys sends keystrokes to a pane (used to run commands in the pane's shell).
func (b *Backend) SendKeys(ctx context.Context, paneID, keys string) error {
	_, err := b.run(ctx,
		"send-keys",
		"-t", paneID,
		keys,
		"Enter",
	)
	return err
}

// KillPane kills a pane.
func (b *Backend) KillPane(ctx context.Context, paneID string) error {
	_, err := b.run(ctx,
		"kill-pane",
		"-t", paneID,
	)
	return err
}

// SelectLayout applies a layout to the window containing paneID.
func (b *Backend) SelectLayout(ctx context.Context, paneID, layout string) error {
	_, err := b.run(ctx,
		"select-layout",
		"-t", paneID,
		layout,
	)
	return err
}

// ResizePane resizes a pane by percentage.
func (b *Backend) ResizePane(ctx context.Context, paneID string, widthPercent int) error {
	_, err := b.run(ctx,
		"resize-pane",
		"-t", paneID,
		"-x", fmt.Sprintf("%d%%", widthPercent),
	)
	return err
}

// GetLeaderPaneID returns the current pane's ID (when inside tmux).
func (b *Backend) GetLeaderPaneID(ctx context.Context) (string, error) {
	if !b.insideTmux {
		return "", fmt.Errorf("not inside a tmux session")
	}
	out, err := b.run(ctx,
		"display-message",
		"-p",
		"#{pane_id}",
	)
	if err != nil {
		return "", err
	}
	return out, nil
}
