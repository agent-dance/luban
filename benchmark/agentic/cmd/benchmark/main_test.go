package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestParseOptionsAcceptsTaskSizeEqualsForm(t *testing.T) {
	parsed, help, err := parseOptions(i18n.LangEN, []string{"--task-size=3", "--with-codex"})
	if err != nil || help {
		t.Fatalf("parseOptions() help=%t err=%v", help, err)
	}
	if parsed.taskSize != 3 || !parsed.withCodex || parsed.resultsRoot != "benchmark-results" || parsed.agentTimeout != 1800 || parsed.evaluatorTimeout != 2700 {
		t.Fatalf("parseOptions() = %#v", parsed)
	}
}

func TestRunMainHelpIsLocalized(t *testing.T) {
	t.Setenv("LANG", "zh_CN.UTF-8")
	var stdout, stderr bytes.Buffer
	if code := runMain(context.Background(), []string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("runMain() = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "用法") || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunMainRejectsCatalogOverflowBeforeExecution(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMain(context.Background(), []string{"--task-size=6"}, &stdout, &stderr); code != 2 {
		t.Fatalf("runMain() = %d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "1") || !strings.Contains(stderr.String(), "5") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunMainExplainsHowToCreateMissingCodexBaseline(t *testing.T) {
	var stdout, stderr bytes.Buffer
	arguments := []string{"--task-size=1", "--results-root", t.TempDir()}
	if code := runMain(context.Background(), arguments, &stdout, &stderr); code != 1 {
		t.Fatalf("runMain() = %d, stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "--with-codex") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
