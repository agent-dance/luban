package transport

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
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

const (
	defaultStdioStderrLimit = 64 * 1024 * 1024
	stdioSIGINTWait         = 100 * time.Millisecond
	stdioSIGTERMWait        = 400 * time.Millisecond
	stdioKillWait           = 100 * time.Millisecond
)

// StdioConfig captures the inputs for a stdio MCP server process.
type StdioConfig struct {
	Command string
	Args    []string
	Env     map[string]string
	Dir     string
}

type processExit struct {
	PID   int
	State *os.ProcessState
	Err   error
}

// stdioTransport bridges a child-process stdio channel to the services-layer
// Transport interface using newline-framed JSON-RPC 2.0.
type stdioTransport struct {
	cmd    *exec.Cmd
	line   *lineTransport
	stdin  io.WriteCloser
	stdout io.ReadCloser

	stderr *boundedStderrBuffer

	waitDone chan struct{}

	closeOnce sync.Once
	closeMu   sync.Mutex
	closeErr  error

	exitMu sync.RWMutex
	exit   processExit
	exited bool
}

// NewStdioTransport spawns a stdio MCP server and returns a Transport over its
// stdin/stdout pipes. The caller must close the returned transport.
func NewStdioTransport(ctx context.Context, cfg StdioConfig) (*stdioTransport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, i18n.NewError(i18n.KeyMCPStdioCommandRequired)
	}

	command := cfg.Command
	cmd := exec.Command(command, cfg.Args...)
	cmd.Env = buildSubprocessEnv(cfg.Env)
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

	stderrBuf := newBoundedStderrBuffer(defaultStdioStderrLimit)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, i18n.WrapError(i18n.KeyServicesMCPStdioStartFailed, err, command)
	}

	t := &stdioTransport{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderrBuf,
		waitDone: make(chan struct{}),
	}
	t.line = newLineTransport(stdout, stdin, nil)

	go t.captureStderr(stderrPipe)
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
func (t *stdioTransport) Send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	if t == nil || t.line == nil {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "stdio")
	}
	err := t.line.Send(ctx, msg)
	if err != nil {
		return t.wrapClosedError("send", err)
	}
	return nil
}

// Receive reads one JSON-RPC message from the child process.
func (t *stdioTransport) Receive(ctx context.Context) (protocol.JSONRPCMessage, error) {
	if t == nil || t.line == nil {
		return protocol.JSONRPCMessage{}, NewTransportClosedError(i18n.KeyServicesMCPTransportNotInitializedReason, ErrTransportClosed, "stdio")
	}
	msg, err := t.line.Receive(ctx)
	if err != nil {
		return protocol.JSONRPCMessage{}, t.wrapClosedError("receive", err)
	}
	return msg, nil
}

// Close terminates the child process and releases stdio pipes. It is
// idempotent and uses the same quick SIGINT -> SIGTERM -> SIGKILL escalation as
// the TypeScript client to keep CLI shutdown responsive.
func (t *stdioTransport) Close() error {
	if t == nil {
		return nil
	}
	t.closeOnce.Do(func() {
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

func (t *stdioTransport) pid() int {
	if t == nil || t.cmd == nil || t.cmd.Process == nil {
		return 0
	}
	return t.cmd.Process.Pid
}

func (t *stdioTransport) stderrString() string {
	if t == nil || t.stderr == nil {
		return ""
	}
	return t.stderr.String()
}

func (t *stdioTransport) captureStderr(stderr io.Reader) {
	if stderr == nil {
		return
	}
	_, _ = io.Copy(t.stderr, stderr)
}

func (t *stdioTransport) observeExit() {
	exit := processExit{PID: t.pid()}
	if t.cmd != nil {
		exit.Err = t.cmd.Wait()
		exit.State = t.cmd.ProcessState
	}

	t.exitMu.Lock()
	t.exit = exit
	t.exited = true
	t.exitMu.Unlock()

	close(t.waitDone)
}

func (t *stdioTransport) closeProcess() error {
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

func (t *stdioTransport) closePipes() error {
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

func (t *stdioTransport) waitForExit(timeout time.Duration) bool {
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

func (t *stdioTransport) hasExited() bool {
	if t == nil {
		return true
	}
	t.exitMu.RLock()
	defer t.exitMu.RUnlock()
	return t.exited
}

func (t *stdioTransport) wrapClosedError(operation string, err error) error {
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

func (t *stdioTransport) closedReason(operation string) string {
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
	if stderr := strings.TrimSpace(t.stderrString()); stderr != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyServicesMCPStdioStderrReason, stderr))
	}
	return strings.Join(parts, ": ")
}

func isBenignPipeClose(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, os.ErrClosed)
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
