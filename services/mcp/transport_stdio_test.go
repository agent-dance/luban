package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

func TestStdioTransportWorksWithProtocolClientEnvCwdAndStderr(t *testing.T) {
	t.Setenv("MCP_PARENT_ENV", "parent-value")
	cwd := t.TempDir()
	wantCWD, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", cwd, err)
	}
	cfg := stdioHelperConfig("protocol")
	cfg.Dir = cwd
	cfg.StderrLimit = 24
	cfg.Env["MCP_HELPER_ENV"] = "server-value"
	cfg.Env["MCP_STDIO_HELPER_STDERR"] = "stderr-prefix-abcdefghijklmnopqrstuvwxyz"

	transport, err := NewStdioTransport(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport: %v", err)
	}
	defer transport.Close() //nolint:errcheck

	client, err := NewProtocolClient(context.Background(), transport, ClientOptions{
		InitializeTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProtocolClient: %v; stderr=%q", err, transport.StderrString())
	}
	defer client.Close() //nolint:errcheck

	type echoResult struct {
		Value     string `json:"value"`
		CWD       string `json:"cwd"`
		HelperEnv string `json:"helperEnv"`
		ParentEnv string `json:"parentEnv"`
	}

	values := []string{"first", "second"}
	errCh := make(chan error, len(values))
	var wg sync.WaitGroup
	for _, value := range values {
		value := value
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var out echoResult
			if err := client.CallRaw(ctx, "echo", map[string]any{"value": value}, &out); err != nil {
				errCh <- err
				return
			}
			if out.Value != value {
				errCh <- fmt.Errorf("echo value = %q, want %q", out.Value, value)
				return
			}
			if out.CWD != wantCWD {
				errCh <- fmt.Errorf("cwd = %q, want %q", out.CWD, wantCWD)
				return
			}
			if out.HelperEnv != "server-value" {
				errCh <- fmt.Errorf("helper env = %q, want server-value", out.HelperEnv)
				return
			}
			if out.ParentEnv != "parent-value" {
				errCh <- fmt.Errorf("parent env = %q, want inherited parent-value", out.ParentEnv)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	stderr := transport.StderrString()
	if !strings.Contains(stderr, "stderr-prefix") {
		t.Fatalf("stderr buffer = %q, want captured helper stderr", stderr)
	}
	if !strings.Contains(stderr, "[truncated]") {
		t.Fatalf("stderr buffer = %q, want bounded truncation marker", stderr)
	}
}

func TestStdioTransportShellPrefixEscapeHatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-prefix command shape is platform-specific on Windows")
	}
	cfg := stdioHelperConfig("protocol")
	cfg.ShellPrefix = "/bin/sh -c"

	transport, err := NewStdioTransport(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport with shell prefix: %v", err)
	}
	defer transport.Close() //nolint:errcheck

	client, err := NewProtocolClient(context.Background(), transport, ClientOptions{
		InitializeTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProtocolClient with shell prefix: %v; stderr=%q", err, transport.StderrString())
	}
	defer client.Close() //nolint:errcheck
}

func TestStdioTransportCloseTerminatesProcessAndIsIdempotent(t *testing.T) {
	transport, err := NewStdioTransport(context.Background(), stdioHelperConfig("sleep"))
	if err != nil {
		t.Fatalf("NewStdioTransport: %v", err)
	}
	if transport.PID() == 0 {
		t.Fatalf("PID = 0, want child pid")
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	select {
	case exit := <-transport.Done():
		if exit.PID == 0 {
			t.Fatalf("exit PID = 0, want child pid")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for child process exit")
	}
}

func TestStdioTransportProcessExitMapsReceiveToTransportClosed(t *testing.T) {
	cfg := stdioHelperConfig("exit")
	cfg.Env["MCP_STDIO_HELPER_STDERR"] = "fatal helper stderr"
	transport, err := NewStdioTransport(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewStdioTransport: %v", err)
	}
	defer transport.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = transport.Receive(ctx)
	if !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("Receive error = %v, want ErrTransportClosed", err)
	}
	if !strings.Contains(err.Error(), "fatal helper stderr") {
		t.Fatalf("Receive error = %v, want stderr diagnostics", err)
	}
}

func TestStdioTransportInitializeCancellationClosesProcess(t *testing.T) {
	transport, err := NewStdioTransport(context.Background(), stdioHelperConfig("no-init"))
	if err != nil {
		t.Fatalf("NewStdioTransport: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = NewProtocolClient(ctx, transport, ClientOptions{
		InitializeTimeout: time.Second,
	})
	if err == nil {
		t.Fatalf("NewProtocolClient succeeded, want initialization timeout")
	}

	select {
	case <-transport.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for process cleanup after initialization cancellation")
	}
}

func TestNewStdioTransportStartFailureReturnsError(t *testing.T) {
	_, err := NewStdioTransport(context.Background(), StdioConfig{Command: "/definitely/not/a/real/mcp-server"})
	if err == nil {
		t.Fatalf("NewStdioTransport succeeded, want start error")
	}
	if !strings.Contains(err.Error(), mcpFormat(i18n.KeyMCPStdioStartFailed, "/definitely/not/a/real/mcp-server")) {
		t.Fatalf("error = %v, want start diagnostic", err)
	}
}

func TestNewStdioTransportHonorsPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewStdioTransport(ctx, stdioHelperConfig("sleep"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("NewStdioTransport error = %v, want context.Canceled", err)
	}
}

func stdioHelperConfig(mode string) StdioConfig {
	return StdioConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestStdioTransportHelperProcess", "--"},
		Env: map[string]string{
			"MCP_STDIO_HELPER":      "1",
			"MCP_STDIO_HELPER_MODE": mode,
		},
	}
}

func TestStdioTransportHelperProcess(t *testing.T) {
	if os.Getenv("MCP_STDIO_HELPER") != "1" {
		return
	}

	switch os.Getenv("MCP_STDIO_HELPER_MODE") {
	case "protocol":
		if stderr := os.Getenv("MCP_STDIO_HELPER_STDERR"); stderr != "" {
			_, _ = fmt.Fprint(os.Stderr, stderr)
		}
		runStdioProtocolHelper()
		os.Exit(0)
	case "exit":
		if stderr := os.Getenv("MCP_STDIO_HELPER_STDERR"); stderr != "" {
			_, _ = fmt.Fprint(os.Stderr, stderr)
		}
		os.Exit(7)
	case "sleep", "no-init":
		select {}
	default:
		os.Exit(2)
	}
}

func runStdioProtocolHelper() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var msg JSONRPCMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if len(msg.ID) == 0 {
			continue
		}
		switch msg.Method {
		case "initialize":
			_ = writeStdioHelperResult(encoder, msg.ID, map[string]any{
				"protocolVersion": MCPProtocolVersion,
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{"name": "stdio-helper", "version": "1.0.0"},
			})
		case "echo":
			var params struct {
				Value string `json:"value"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			cwd, _ := os.Getwd()
			_ = writeStdioHelperResult(encoder, msg.ID, map[string]any{
				"value":     params.Value,
				"cwd":       cwd,
				"helperEnv": os.Getenv("MCP_HELPER_ENV"),
				"parentEnv": os.Getenv("MCP_PARENT_ENV"),
			})
		default:
			_ = writeStdioHelperResult(encoder, msg.ID, map[string]any{"ok": true})
		}
	}
}

func writeStdioHelperResult(encoder *json.Encoder, id json.RawMessage, result any) error {
	msg, err := NewResultMessage(id, result)
	if err != nil {
		return err
	}
	return encoder.Encode(msg)
}
