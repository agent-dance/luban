package mcp

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	defaultStdioStderrLimit = 64 * 1024 * 1024
	stdioSIGINTWait         = 100 * time.Millisecond
	stdioSIGTERMWait        = 400 * time.Millisecond
	stdioKillWait           = 100 * time.Millisecond
)

// StdioConfig captures the inputs for a stdio MCP server process.
type StdioConfig struct {
	Command     string
	Args        []string
	Env         map[string]string
	Dir         string
	Stderr      io.Writer
	StderrLimit int
	ShellPrefix string
}

// ProcessExit is emitted when the child MCP server exits.
type ProcessExit struct {
	PID   int
	State *os.ProcessState
	Err   error
}

// StdioTransport bridges a child-process stdio channel to the services-layer
// Transport interface using newline-framed JSON-RPC 2.0.
type StdioTransport struct {
	cmd    *exec.Cmd
	line   *LineTransport
	stdin  io.WriteCloser
	stdout io.ReadCloser

	stderr *boundedStderrBuffer

	closed   chan struct{}
	waitDone chan struct{}
	exitCh   chan ProcessExit

	closeOnce sync.Once
	closeMu   sync.Mutex
	closeErr  error

	exitMu sync.RWMutex
	exit   ProcessExit
	exited bool
}

// NewStdioTransport spawns a stdio MCP server and returns a Transport over its
// stdin/stdout pipes. The caller must close the returned transport.
func NewStdioTransport(ctx context.Context, cfg StdioConfig) (*StdioTransport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, i18n.NewError(i18n.KeyMCPStdioCommandRequired)
	}

	command, args := stdioCommandAndArgs(cfg)
	cmd := exec.Command(command, args...)
	cmd.Env = BuildSubprocessEnv(cfg.Env)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPStdioPipeFailed, err, "stdin")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, i18n.WrapError(i18n.KeyServicesMCPStdioPipeFailed, err, "stdout")
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, i18n.WrapError(i18n.KeyServicesMCPStdioPipeFailed, err, "stderr")
	}

	stderrLimit := cfg.StderrLimit
	if stderrLimit <= 0 {
		stderrLimit = defaultStdioStderrLimit
	}
	stderrBuf := newBoundedStderrBuffer(stderrLimit)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, i18n.WrapError(i18n.KeyServicesMCPStdioStartFailed, err, command)
	}

	t := &StdioTransport{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderrBuf,
		closed:   make(chan struct{}),
		waitDone: make(chan struct{}),
		exitCh:   make(chan ProcessExit, 1),
	}
	t.line = NewLineTransport(stdout, stdin, nil)

	go t.captureStderr(stderrPipe, cfg.Stderr)
	go t.observeExit()

	select {
	case <-ctx.Done():
		_ = t.Close()
		return nil, ctx.Err()
	default:
	}

	return t, nil
}

// Send writes one JSON-RPC message to the child process.
func (t *StdioTransport) Send(ctx context.Context, msg JSONRPCMessage) error {
	if t == nil || t.line == nil {
		return newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "stdio")
	}
	err := t.line.Send(ctx, msg)
	if err != nil {
		return t.wrapClosedError("send", err)
	}
	return nil
}

// Receive reads one JSON-RPC message from the child process.
func (t *StdioTransport) Receive(ctx context.Context) (JSONRPCMessage, error) {
	if t == nil || t.line == nil {
		return JSONRPCMessage{}, newTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "stdio")
	}
	msg, err := t.line.Receive(ctx)
	if err != nil {
		return JSONRPCMessage{}, t.wrapClosedError("receive", err)
	}
	return msg, nil
}

// Close terminates the child process and releases stdio pipes. It is
// idempotent and uses the same quick SIGINT -> SIGTERM -> SIGKILL escalation as
// the TypeScript client to keep CLI shutdown responsive.
func (t *StdioTransport) Close() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() {
		close(t.closed)
		closeErr := t.closeProcess()
		closeErr = errors.Join(closeErr, t.closePipes())
		t.closeMu.Lock()
		t.closeErr = closeErr
		t.closeMu.Unlock()
	})
	t.closeMu.Lock()
	defer t.closeMu.Unlock()
	return t.closeErr
}

// PID returns the child process id, or 0 if the process was not started.
func (t *StdioTransport) PID() int {
	if t == nil || t.cmd == nil || t.cmd.Process == nil {
		return 0
	}
	return t.cmd.Process.Pid
}

// Done is closed after the child process exits. The first receive yields its
// exit details.
func (t *StdioTransport) Done() <-chan ProcessExit {
	if t == nil {
		ch := make(chan ProcessExit)
		close(ch)
		return ch
	}
	return t.exitCh
}

// StderrString returns the bounded stderr captured from the child process.
func (t *StdioTransport) StderrString() string {
	if t == nil || t.stderr == nil {
		return ""
	}
	return t.stderr.String()
}

func (t *StdioTransport) captureStderr(stderr io.Reader, extra io.Writer) {
	if stderr == nil {
		return
	}
	var w io.Writer = t.stderr
	if extra != nil {
		w = io.MultiWriter(t.stderr, extra)
	}
	_, _ = io.Copy(w, stderr)
}

func (t *StdioTransport) observeExit() {
	exit := ProcessExit{PID: t.PID()}
	if t.cmd != nil {
		exit.Err = t.cmd.Wait()
		exit.State = t.cmd.ProcessState
	}

	t.exitMu.Lock()
	t.exit = exit
	t.exited = true
	t.exitMu.Unlock()

	select {
	case t.exitCh <- exit:
	default:
	}
	close(t.exitCh)
	close(t.waitDone)
}

func (t *StdioTransport) closeProcess() error {
	if t == nil || t.cmd == nil || t.cmd.Process == nil {
		return nil
	}
	if t.hasExited() {
		return nil
	}

	if runtime.GOOS == "windows" {
		_ = t.cmd.Process.Kill()
		t.waitForExit(stdioKillWait)
		return nil
	}

	if err := t.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = t.cmd.Process.Kill()
		t.waitForExit(stdioKillWait)
		return nil
	}
	if t.waitForExit(stdioSIGINTWait) {
		return nil
	}

	if err := t.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = t.cmd.Process.Kill()
		t.waitForExit(stdioKillWait)
		return nil
	}
	if t.waitForExit(stdioSIGTERMWait) {
		return nil
	}

	_ = t.cmd.Process.Kill()
	t.waitForExit(stdioKillWait)
	return nil
}

func (t *StdioTransport) closePipes() error {
	var errs []error
	if t.stdin != nil {
		if err := t.stdin.Close(); err != nil && !isBenignPipeClose(err) {
			errs = append(errs, err)
		}
	}
	if t.stdout != nil {
		if err := t.stdout.Close(); err != nil && !isBenignPipeClose(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (t *StdioTransport) waitForExit(timeout time.Duration) bool {
	if t == nil {
		return true
	}
	select {
	case <-t.waitDone:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (t *StdioTransport) hasExited() bool {
	if t == nil {
		return true
	}
	t.exitMu.RLock()
	defer t.exitMu.RUnlock()
	return t.exited
}

func (t *StdioTransport) wrapClosedError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, ErrTransportClosed) || isBenignPipeClose(err) || t.hasExited() {
		return newTransportClosedErrorFunc(func() string { return t.closedReason(operation) }, err)
	}
	return err
}

func (t *StdioTransport) closedReason(operation string) string {
	lang := i18n.DetectOrLoadLanguage()
	parts := []string{i18n.Format(lang, i18n.KeyServicesMCPStdioOperationFailedReason, operation)}
	t.exitMu.RLock()
	exit := t.exit
	exited := t.exited
	t.exitMu.RUnlock()
	if exited {
		if exit.Err != nil {
			parts = append(parts, i18n.Format(lang, i18n.KeyServicesMCPStdioProcessExitDetailReason, exit.Err))
		} else {
			parts = append(parts, i18n.Text(lang, i18n.KeyServicesMCPStdioProcessExitedReason))
		}
	}
	if stderr := strings.TrimSpace(t.StderrString()); stderr != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyServicesMCPStdioStderrReason, stderr))
	}
	return strings.Join(parts, ": ")
}

func isBenignPipeClose(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, os.ErrClosed)
}

func stdioCommandAndArgs(cfg StdioConfig) (string, []string) {
	prefix := strings.TrimSpace(cfg.ShellPrefix)
	if prefix == "" {
		prefix = strings.TrimSpace(os.Getenv("CLAUDE_CODE_SHELL_PREFIX"))
	}
	if prefix == "" {
		return cfg.Command, append([]string(nil), cfg.Args...)
	}

	parts := strings.Fields(prefix)
	if len(parts) == 0 {
		return cfg.Command, append([]string(nil), cfg.Args...)
	}
	commandLine := strings.Join(append([]string{cfg.Command}, cfg.Args...), " ")
	args := append([]string(nil), parts[1:]...)
	args = append(args, commandLine)
	return parts[0], args
}

type boundedStderrBuffer struct {
	mu        sync.Mutex
	limit     int
	truncated bool
	data      []byte
}

func newBoundedStderrBuffer(limit int) *boundedStderrBuffer {
	if limit <= 0 {
		limit = defaultStdioStderrLimit
	}
	return &boundedStderrBuffer{limit: limit}
}

func (b *boundedStderrBuffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.truncated = true
		return len(p), nil
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *boundedStderrBuffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := string(b.data)
	if b.truncated {
		out += "... [truncated]"
	}
	return out
}
