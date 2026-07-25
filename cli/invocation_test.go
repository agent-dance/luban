package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

func TestParseInvocationClassifiesEarlyActions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cli.EarlyAction
	}{
		{name: "runtime", args: []string{"hello"}, want: cli.EarlyActionNone},
		{name: "help", args: []string{"--help"}, want: cli.EarlyActionHelp},
		{name: "version", args: []string{"--version"}, want: cli.EarlyActionVersion},
		{name: "mcp", args: []string{"mcp", "list"}, want: cli.EarlyActionMCP},
		{name: "prompt dump", args: []string{"--prompt-dump"}, want: cli.EarlyActionPromptDump},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation, err := cli.ParseInvocation(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if invocation.Action != test.want {
				t.Fatalf("action = %v, want %v", invocation.Action, test.want)
			}
		})
	}
}

func TestRunEarlyActionPreservesHelpAndVersionOutput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "Usage: " + brand.CommandName},
		{name: "version", args: []string{"--version"}, want: fmt.Sprintf("%s %s\n", brand.CommandName, cli.Version)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation, err := cli.ParseInvocation(test.args)
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if code := cli.RunEarlyAction(invocation, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
			}
			if test.name == "version" {
				if got := stdout.String(); got != test.want {
					t.Fatalf("stdout = %q, want %q", got, test.want)
				}
			} else if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout = %q, want substring %q", stdout.String(), test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunEarlyActionPreservesPromptDumpOutput(t *testing.T) {
	invocation, err := cli.ParseInvocation([]string{"--prompt-dump", "--system-prompt", "focused prompt marker"})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := cli.RunEarlyAction(invocation, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "focused prompt marker") {
		t.Fatalf("prompt dump omitted override: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunEarlyActionPreservesMCPExitAndStreams(t *testing.T) {
	actionArgs := []string{"definitely-not-a-command"}
	var wantStdout, wantStderr bytes.Buffer
	wantCode := cli.RunMCPCLI(actionArgs, &wantStdout, &wantStderr)

	invocation, err := cli.ParseInvocation(append([]string{"mcp"}, actionArgs...))
	if err != nil {
		t.Fatal(err)
	}
	var gotStdout, gotStderr bytes.Buffer
	if gotCode := cli.RunEarlyAction(invocation, &gotStdout, &gotStderr); gotCode != wantCode {
		t.Fatalf("exit code = %d, want %d", gotCode, wantCode)
	}
	if gotStdout.String() != wantStdout.String() || gotStderr.String() != wantStderr.String() {
		t.Fatalf("streams = (%q, %q), want (%q, %q)", gotStdout.String(), gotStderr.String(), wantStdout.String(), wantStderr.String())
	}
}

func TestPrintAndSDKAreMutuallyExclusive(t *testing.T) {
	_, err := cli.ParseInvocation([]string{"--sdk", "--print"})
	want := i18n.Text(i18n.LangEN, i18n.KeyCLIInputModeSDKPrint)
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestReadPipedPromptPreservesCompleteInputAndEnforcesLimit(t *testing.T) {
	const pipedPromptLimit = 4 << 20
	const prompt = "first line\nsecond line\n"
	got, err := cli.ReadPipedPrompt(strings.NewReader(prompt))
	if err != nil {
		t.Fatal(err)
	}
	if got != prompt {
		t.Fatalf("prompt = %q, want %q", got, prompt)
	}

	tooLarge := strings.NewReader(strings.Repeat("x", pipedPromptLimit+1))
	if _, err := cli.ReadPipedPrompt(tooLarge); err == nil || !strings.Contains(err.Error(), fmt.Sprint(pipedPromptLimit)) {
		t.Fatalf("oversized prompt error = %v", err)
	}

	wantErr := errors.New("pipe read failed")
	if _, err := cli.ReadPipedPrompt(errorReader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("read error = %v, want wrapped %v", err, wantErr)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
