package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/benchmark/agentic/pierbackend"
	"github.com/agent-dance/luban/benchmark/agentic/pilot"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

type options struct {
	manifestPath   string
	backendPath    string
	workDir        string
	hostReceipt    string
	guestPreflight string
	pairLimit      int
	execute        bool
}

type commandResult struct {
	SchemaVersion     string          `json:"schema_version"`
	FormalCompatible  bool            `json:"formal_compatible"`
	Command           string          `json:"command"`
	ExternalExecution bool            `json:"external_execution"`
	Preflight         pilot.Preflight `json:"preflight"`
	Ledger            *pilot.Ledger   `json:"ledger,omitempty"`
}

func main() {
	commandIO := cli.ProcessCommandIO()
	os.Exit(runMain(context.Background(), os.Args[1:], commandIO.Stdout, commandIO.Stderr))
}

func runMain(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	language := i18n.DetectOrLoadLanguage()
	if len(arguments) == 1 && slices.Contains([]string{"-h", "--help"}, arguments[0]) {
		_, _ = fmt.Fprintln(stdout, i18n.Text(language, i18n.KeyAgenticPilotUsage))
		return 0
	}
	if len(arguments) == 0 || !slices.Contains([]string{"preflight", "run"}, arguments[0]) {
		_, _ = fmt.Fprintln(stderr, i18n.Text(language, i18n.KeyAgenticPilotUsage))
		return 2
	}
	command := arguments[0]
	parsed, help, err := parseOptions(language, command, arguments[1:])
	if help {
		_, _ = fmt.Fprintln(stdout, i18n.Text(language, i18n.KeyAgenticPilotUsage))
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyBenchmarkCLIFailed, err))
		return 2
	}
	if command == "run" && !parsed.execute {
		_, _ = fmt.Fprintln(stderr, i18n.Text(language, i18n.KeyBenchmarkCLIExecuteRequired))
		return 2
	}
	result, err := execute(ctx, command, parsed)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyBenchmarkCLIFailed, err))
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyBenchmarkCLIFailed, err))
		return 1
	}
	return 0
}

func parseOptions(language i18n.Language, command string, arguments []string) (options, bool, error) {
	result := options{
		manifestPath: os.Getenv("AGENTIC_BENCH_MANIFEST"), backendPath: os.Getenv("AGENTIC_BENCH_BACKEND_CONFIG"),
		workDir: os.Getenv("AGENTIC_BENCH_WORK_DIR"), hostReceipt: os.Getenv("AGENTIC_PILOT_HOST_RECEIPT"),
		guestPreflight: os.Getenv("AGENTIC_PILOT_GUEST_PREFLIGHT_RECEIPT"),
	}
	set := flag.NewFlagSet(command, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&result.manifestPath, "manifest", result.manifestPath, i18n.Text(language, i18n.KeyBenchmarkCLIManifestFlag))
	set.StringVar(&result.backendPath, "backend-config", result.backendPath, i18n.Text(language, i18n.KeyBenchmarkCLIBackendFlag))
	set.StringVar(&result.workDir, "work-dir", result.workDir, i18n.Text(language, i18n.KeyBenchmarkCLIWorkDirFlag))
	set.StringVar(&result.hostReceipt, "host-receipt", result.hostReceipt, i18n.Text(language, i18n.KeyAgenticPilotHostReceiptFlag))
	set.StringVar(&result.guestPreflight, "guest-preflight-receipt", result.guestPreflight, i18n.Text(language, i18n.KeyAgenticPilotGuestPreflightFlag))
	set.IntVar(&result.pairLimit, "pair-limit", 0, i18n.Text(language, i18n.KeyAgenticPilotPairLimitFlag))
	set.BoolVar(&result.execute, "execute", false, i18n.Text(language, i18n.KeyBenchmarkCLIExecuteFlag))
	if err := set.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if set.NArg() != 0 || result.manifestPath == "" || result.backendPath == "" || result.workDir == "" || result.hostReceipt == "" || result.guestPreflight == "" {
		return options{}, false, errors.New("pilot_required_configuration_missing")
	}
	for _, target := range []*string{&result.manifestPath, &result.backendPath, &result.workDir, &result.hostReceipt, &result.guestPreflight} {
		absolute, err := filepath.Abs(*target)
		if err != nil {
			return options{}, false, err
		}
		*target = absolute
	}
	return result, false, nil
}

func execute(ctx context.Context, command string, options options) (commandResult, error) {
	loaded, err := harness.LoadManifest(options.manifestPath)
	if err != nil {
		return commandResult{}, err
	}
	config, err := pierbackend.LoadConfigFile(options.backendPath)
	if err != nil {
		return commandResult{}, err
	}
	backend, err := pierbackend.NewDevelopment(config)
	if err != nil {
		return commandResult{}, err
	}
	runner := pilot.Runner{
		Loaded: loaded, Backend: backend, WorkDir: options.workDir, HostEnvironment: os.Environ(),
		HostStorageReceiptPath: options.hostReceipt, GuestStoragePreflightPath: options.guestPreflight,
		PairLimit: options.pairLimit,
	}
	result := commandResult{SchemaVersion: "agentic-bench/development-pilot-command-v1", FormalCompatible: false, Command: command}
	if command == "preflight" {
		result.Preflight, err = runner.Preflight(ctx)
		return result, err
	}
	ledger, preflight, err := runner.Run(ctx)
	result.ExternalExecution, result.Preflight, result.Ledger = true, preflight, &ledger
	return result, err
}
