package tools

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/agent-dance/luban/sandbox"
)

type failingCommandSandbox struct{}

func (failingCommandSandbox) Name() string    { return "failing-test-sandbox" }
func (failingCommandSandbox) Available() bool { return true }
func (failingCommandSandbox) SandboxCapability() (sandbox.Capability, bool) {
	return sandbox.Capability{
		Backend: "failing-test-sandbox", ExecutablePath: "/usr/bin/failing-test-sandbox", ExecutableIdentity: "v1",
	}, true
}
func (failingCommandSandbox) Command(context.Context, sandbox.Config, string, ...string) (*exec.Cmd, error) {
	return nil, errors.New("private sandbox diagnostic")
}

func TestBashSandboxBuildFailureDoesNotWriteStderr(t *testing.T) {
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writeEnd
	t.Cleanup(func() { os.Stderr = original })

	tool := &BashTool{CWD: t.TempDir(), Sandbox: failingCommandSandbox{}}
	_, buildErr := tool.buildCommand(context.Background(), BashInput{}, "mkdir build")
	if closeErr := writeEnd.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stderr = original
	output, readErr := io.ReadAll(readEnd)
	if readErr != nil {
		t.Fatal(readErr)
	}
	_ = readEnd.Close()
	if buildErr == nil {
		t.Fatal("sandbox construction failure silently fell back to an unsandboxed command")
	}
	if strings.Contains(buildErr.Error(), "private sandbox diagnostic") {
		t.Fatalf("private sandbox cause leaked through user-visible error: %q", buildErr)
	}
	if len(output) != 0 {
		t.Fatalf("sandbox construction wrote outside the tool-result pipeline: %q", output)
	}
}
