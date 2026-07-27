package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/agent-dance/luban/benchmark/agentic/report"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

type options struct {
	inputPath  string
	outputPath string
}

func main() {
	commandIO := cli.ProcessCommandIO()
	os.Exit(runMain(os.Args[1:], commandIO.Stdout, commandIO.Stderr))
}

func runMain(arguments []string, stdout, stderr io.Writer) int {
	language := i18n.DetectOrLoadLanguage()
	parsed, help, err := parseOptions(language, arguments)
	if help {
		writeUsage(stdout, language)
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyAgenticReportCLIError, err))
		return 2
	}
	if err := report.GenerateFile(parsed.inputPath, parsed.outputPath); err != nil {
		_, _ = fmt.Fprintln(stderr, i18n.Format(language, i18n.KeyAgenticReportCLIError, err))
		return 1
	}
	_, _ = fmt.Fprintln(stdout, i18n.Format(language, i18n.KeyAgenticReportCLISuccess, parsed.outputPath))
	return 0
}

func parseOptions(language i18n.Language, arguments []string) (options, bool, error) {
	var result options
	set := flag.NewFlagSet("reportgen", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.StringVar(&result.inputPath, "input", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput))
	set.StringVar(&result.outputPath, "output", "", i18n.Text(language, i18n.KeyAgenticReportCLIFlagOutput))
	if err := set.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, true, nil
		}
		return options{}, false, err
	}
	if set.NArg() != 0 || result.inputPath == "" || result.outputPath == "" {
		return options{}, false, i18n.NewError(i18n.KeyAgenticReportCLIRequired)
	}
	var err error
	result.inputPath, err = filepath.Abs(result.inputPath)
	if err != nil {
		return options{}, false, err
	}
	result.outputPath, err = filepath.Abs(result.outputPath)
	return result, false, err
}

func writeUsage(writer io.Writer, language i18n.Language) {
	_, _ = fmt.Fprintf(writer, "--input PATH\t%s\n--output PATH\t%s\n",
		i18n.Text(language, i18n.KeyAgenticReportCLIFlagInput),
		i18n.Text(language, i18n.KeyAgenticReportCLIFlagOutput))
}
