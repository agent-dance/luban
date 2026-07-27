package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/localbench"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

type options struct {
	taskSize         int
	withCodex        bool
	resultsRoot      string
	agentTimeout     int
	evaluatorTimeout int
}

func main() {
	commandIO := cli.ProcessCommandIO()
	os.Exit(runMain(context.Background(), os.Args[1:], commandIO.Stdout, commandIO.Stderr))
}

func runMain(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	language := i18n.DetectOrLoadLanguage()
	parsed, help, err := parseOptions(language, arguments)
	if help {
		_, _ = fmt.Fprintln(stdout, i18n.Text(language, i18n.KeyLocalBenchmarkUsage))
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Text(language, i18n.KeyLocalBenchmarkInvalidOptions))
		return 2
	}
	if parsed.taskSize < 1 || parsed.taskSize > localbench.CatalogSize() {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyLocalBenchmarkTaskSizeRange, localbench.CatalogSize()))
		return 2
	}
	repositoryRoot, err := findRepositoryRoot(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyLocalBenchmarkFailed, "execution.log"))
		return 1
	}
	executor := localbench.NewLocalExecutor()
	outcome, runErr := localbench.Run(ctx, localbench.Options{
		RepositoryRoot: repositoryRoot, ResultsRoot: parsed.resultsRoot,
		TaskSize: parsed.taskSize, AgentTimeout: parsed.agentTimeout,
		WithCodex:        parsed.withCodex,
		EvaluatorTimeout: parsed.evaluatorTimeout,
		Progress: func(key i18n.Key, arguments ...any) {
			_, _ = fmt.Fprintln(stderr, i18n.Format(language, key, arguments...))
		},
	}, executor, language)
	reportPath := displayPath(outcome.ReportPath)
	if runErr != nil {
		if reportPath != "" {
			_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyLocalBenchmarkPartial, reportPath))
		} else if semantic, ok := i18n.DescribeSemanticError(runErr); ok {
			_, _ = fmt.Fprintln(stderr, i18n.Format(language, semantic.Key, semantic.Args...))
		} else {
			_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyLocalBenchmarkFailed, displayPath(outcome.LogPath)))
		}
		return 1
	}
	if !outcome.Complete {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyLocalBenchmarkPartial, reportPath))
		return 1
	}
	if parsed.withCodex {
		_, _ = fmt.Fprintln(stdout, i18n.Format(language, i18n.KeyLocalBenchmarkCompletedWithCodex, reportPath, displayPath(outcome.CodexReportPath)))
	} else {
		_, _ = fmt.Fprintln(stdout, i18n.Format(language, i18n.KeyLocalBenchmarkCompleted, reportPath))
	}
	return 0
}

func parseOptions(language i18n.Language, arguments []string) (options, bool, error) {
	result := options{resultsRoot: "benchmark-results", agentTimeout: 1800, evaluatorTimeout: 2700}
	set := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.IntVar(&result.taskSize, "task-size", 0, i18n.Text(language, i18n.KeyLocalBenchmarkFlagTaskSize))
	set.BoolVar(&result.withCodex, "with-codex", false, i18n.Text(language, i18n.KeyLocalBenchmarkFlagWithCodex))
	set.StringVar(&result.resultsRoot, "results-root", result.resultsRoot, i18n.Text(language, i18n.KeyLocalBenchmarkFlagResultsRoot))
	set.IntVar(&result.agentTimeout, "agent-timeout", result.agentTimeout, i18n.Text(language, i18n.KeyLocalBenchmarkFlagAgentTimeout))
	set.IntVar(&result.evaluatorTimeout, "evaluator-timeout", result.evaluatorTimeout, i18n.Text(language, i18n.KeyLocalBenchmarkFlagEvaluatorTimeout))
	if err := set.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if set.NArg() != 0 || result.taskSize == 0 || strings.TrimSpace(result.resultsRoot) == "" || result.agentTimeout <= 0 || result.evaluatorTimeout <= 0 {
		return options{}, false, errors.New("invalid benchmark options")
	}
	return result, false, nil
}

func findRepositoryRoot(ctx context.Context) (string, error) {
	command := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	raw, err := command.Output()
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(string(raw))), nil
}

func displayPath(path string) string {
	if path == "" {
		return ""
	}
	current, err := os.Getwd()
	if err != nil {
		return filepath.Base(path)
	}
	relative, err := filepath.Rel(current, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(relative)
}
