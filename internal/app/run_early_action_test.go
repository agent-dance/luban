package app

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

func TestRunPreservesEarlyActionExitCodesAndStreams(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "help", args: []string{"--help"}, wantCode: 0, wantStdout: "Usage: " + brand.CommandName},
		{name: "version", args: []string{"--version"}, wantCode: 0, wantStdout: brand.CommandName + " " + cli.Version + "\n"},
		{name: "prompt dump", args: []string{"--prompt-dump", "--system-prompt", "app prompt marker"}, wantCode: 0, wantStdout: "app prompt marker"},
		{name: "SDK print conflict", args: []string{"--sdk", "--print"}, wantCode: 2, wantStderr: i18n.Text(i18n.LangEN, i18n.KeyCLIInputModeSDKPrint)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := captureRun(t, test.args)
			if code != test.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout = %q stderr = %q", code, test.wantCode, stdout, stderr)
			}
			if test.wantStdout != "" && !strings.Contains(stdout, test.wantStdout) {
				t.Fatalf("stdout = %q, want substring %q", stdout, test.wantStdout)
			}
			if test.wantStderr != "" && !strings.Contains(stderr, test.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", stderr, test.wantStderr)
			}
		})
	}
}

func captureRun(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	oldArgs, oldStdout, oldStderr := os.Args, os.Stdout, os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		t.Fatal(err)
	}
	os.Args = append([]string{brand.CommandName}, args...)
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	defer func() {
		os.Args, os.Stdout, os.Stderr = oldArgs, oldStdout, oldStderr
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
	}()

	stdoutResult := make(chan string, 1)
	stderrResult := make(chan string, 1)
	go readCapturedStream(stdoutReader, stdoutResult)
	go readCapturedStream(stderrReader, stderrResult)
	code := Run()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	return code, <-stdoutResult, <-stderrResult
}

func readCapturedStream(reader io.Reader, result chan<- string) {
	data, _ := io.ReadAll(reader)
	result <- string(data)
}
