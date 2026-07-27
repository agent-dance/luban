// Package cli provides minimal flag parsing for LUBAN Code using only
// the standard library flag package — no external dependencies.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/prompt"
)

const Version = "v0.1.0"

// maxPipedPromptBytes bounds implicit print-mode input before any provider or
// session runtime is initialized.
const maxPipedPromptBytes int64 = 4 << 20

// ErrHelp is returned by ParseArgs when --help / -h is passed.
var ErrHelp = errors.New("help requested")

// ErrVersion is returned by ParseArgs when --version / -v is passed.
var ErrVersion = errors.New("version requested")

// multiString is a flag.Value implementation that accumulates repeated flags.
// e.g. --allowed-dir /a --allowed-dir /b → []string{"/a", "/b"}
type multiString []string

func (m *multiString) String() string { return strings.Join(*m, ", ") }
func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// Options holds every parsed CLI option.
type Options struct {
	Model              string   // --model / -m
	Provider           string   // --provider
	API                string   // --api (e.g. "responses" for OpenAI Responses API)
	ReasoningEffort    string   // --reasoning-effort (e.g. "xhigh")
	ServiceTier        string   // --service-tier (currently "default")
	ResponsesWebSocket bool     // --responses-websocket (explicit public Responses transport opt-in)
	PinnedModel        bool     // --pinned-model / --no-model-fallback
	Print              bool     // -p  (print mode: single query, no REPL)
	Resume             bool     // --resume
	SessionID          string   // --session-id
	MaxTurns           int      // --max-turns  (default 100)
	SystemPrompt       string   // --system-prompt
	AllowedDirs        []string // --allowed-dir (repeatable)
	AllowAll           bool     // --allow-all (skip interactive permission prompts)
	Sandbox            bool     // --sandbox (enable OS-level sandboxing for shell commands)
	ForceSandboxTools  bool     // --force-sandbox-tools (fail closed unless every shell execution is sandboxed)
	AllowedTools       string   // --allowed-tools (comma-separated whitelist)
	DisallowedTools    string   // --disallowed-tools (comma-separated blacklist)
	SDK                bool     // --sdk (stream-JSON / SDK transport mode)
	Version            bool     // --version / -v
	Help               bool     // --help / -h
	Verbose            bool     // --verbose
	DebugFile          string   // --debug-file (write developer-facing LLM runtime diagnostics)
	NoColor            bool     // --no-color (disable ANSI color output)
	OutputFormat       string   // --output-format: "text" (default) | "json" | "stream-json"
	Quiet              bool     // --quiet / -q (only output final assistant text)
	ScreenReader       bool     // --screen-reader (append-only accessible interactive mode)
	Agents             string   // --agents JSON object defining additional agents
	PromptDump         bool     // --prompt-dump
	PromptDumpJSON     bool     // --prompt-dump-json
	Language           string   // --language
	OutputStyle        string   // --output-style

	// Task 7: domain restrictions for web tools
	AllowedDomains    string // --allowed-domains (comma-separated, supports *.example.com)
	DisallowedDomains string // --disallowed-domains (comma-separated)

	// Remaining positional args after flag parsing (used as the query in -p mode)
	Args []string
}

// newFlagSet builds a FlagSet wired to opts and dirs. The caller sets fs.Usage.
func newFlagSet(opts *Options, dirs *multiString, lang i18n.Language) *flag.FlagSet {
	fs := flag.NewFlagSet(brand.CommandName, flag.ContinueOnError)
	fs.StringVar(&opts.Model, "model", "", i18n.Text(lang, i18n.KeyCLIFlagModel))
	fs.StringVar(&opts.Model, "m", "", i18n.Text(lang, i18n.KeyCLIFlagModel))
	fs.StringVar(&opts.Provider, "provider", "", i18n.Text(lang, i18n.KeyCLIFlagProvider))
	fs.StringVar(&opts.API, "api", "", i18n.Text(lang, i18n.KeyCLIFlagAPI))
	fs.StringVar(&opts.ReasoningEffort, "reasoning-effort", "", i18n.Text(lang, i18n.KeyCLIFlagReasoningEffort))
	fs.StringVar(&opts.ServiceTier, "service-tier", "", i18n.Text(lang, i18n.KeyCLIFlagServiceTier))
	fs.BoolVar(&opts.ResponsesWebSocket, "responses-websocket", false, i18n.Text(lang, i18n.KeyCLIFlagResponsesWebSocket))
	fs.BoolVar(&opts.PinnedModel, "pinned-model", false, i18n.Text(lang, i18n.KeyCLIFlagPinnedModel))
	fs.BoolVar(&opts.PinnedModel, "no-model-fallback", false, i18n.Text(lang, i18n.KeyCLIFlagPinnedModel))
	fs.BoolVar(&opts.Print, "p", false, i18n.Text(lang, i18n.KeyCLIFlagPrint))
	fs.BoolVar(&opts.Print, "print", false, i18n.Text(lang, i18n.KeyCLIFlagPrint))
	fs.BoolVar(&opts.Resume, "resume", false, i18n.Text(lang, i18n.KeyCLIFlagResume))
	fs.StringVar(&opts.SessionID, "session-id", "", i18n.Text(lang, i18n.KeyCLIFlagSessionID))
	fs.IntVar(&opts.MaxTurns, "max-turns", 100, i18n.Text(lang, i18n.KeyCLIFlagMaxTurns))
	fs.StringVar(&opts.SystemPrompt, "system-prompt", "", i18n.Text(lang, i18n.KeyCLIFlagSystemPrompt))
	fs.Var(dirs, "allowed-dir", i18n.Text(lang, i18n.KeyCLIFlagAllowedDir))
	fs.BoolVar(&opts.AllowAll, "allow-all", false, i18n.Text(lang, i18n.KeyCLIFlagAllowAll))
	fs.StringVar(&opts.AllowedTools, "allowed-tools", "", i18n.Text(lang, i18n.KeyCLIFlagAllowedTools))
	fs.StringVar(&opts.DisallowedTools, "disallowed-tools", "", i18n.Text(lang, i18n.KeyCLIFlagDisallowedTools))
	fs.BoolVar(&opts.Sandbox, "sandbox", false, i18n.Text(lang, i18n.KeyCLIFlagSandbox))
	fs.BoolVar(&opts.ForceSandboxTools, "force-sandbox-tools", false, i18n.Text(lang, i18n.KeyCLIFlagForceSandboxTools))
	fs.BoolVar(&opts.SDK, "sdk", false, i18n.Text(lang, i18n.KeyCLIFlagSDK))
	fs.BoolVar(&opts.Version, "version", false, i18n.Text(lang, i18n.KeyCLIFlagVersion))
	fs.BoolVar(&opts.Version, "v", false, i18n.Text(lang, i18n.KeyCLIFlagVersion))
	fs.BoolVar(&opts.Verbose, "verbose", false, i18n.Text(lang, i18n.KeyCLIFlagVerbose))
	fs.StringVar(&opts.DebugFile, "debug-file", "", i18n.Text(lang, i18n.KeyCLIFlagDebugFile))
	fs.BoolVar(&opts.NoColor, "no-color", false, i18n.Text(lang, i18n.KeyCLIFlagNoColor))
	fs.StringVar(&opts.OutputFormat, "output-format", "text", i18n.Text(lang, i18n.KeyCLIFlagOutputFormat))
	fs.BoolVar(&opts.Quiet, "quiet", false, i18n.Text(lang, i18n.KeyCLIFlagQuiet))
	fs.BoolVar(&opts.Quiet, "q", false, i18n.Text(lang, i18n.KeyCLIFlagQuiet))
	fs.BoolVar(&opts.ScreenReader, "screen-reader", false, i18n.Text(lang, i18n.KeyCLIFlagScreenReader))
	fs.StringVar(&opts.Agents, "agents", "", i18n.Text(lang, i18n.KeyCLIFlagAgents))
	fs.BoolVar(&opts.PromptDump, "prompt-dump", false, i18n.Text(lang, i18n.KeyCLIFlagPromptDump))
	fs.BoolVar(&opts.PromptDumpJSON, "prompt-dump-json", false, i18n.Text(lang, i18n.KeyCLIFlagPromptDumpJSON))
	fs.StringVar(&opts.Language, "language", "", i18n.Text(lang, i18n.KeyCLIFlagLanguage))
	fs.StringVar(&opts.OutputStyle, "output-style", "", i18n.Text(lang, i18n.KeyCLIFlagOutputStyle))
	fs.StringVar(&opts.AllowedDomains, "allowed-domains", "", i18n.Text(lang, i18n.KeyCLIFlagAllowedDomains))
	fs.StringVar(&opts.DisallowedDomains, "disallowed-domains", "", i18n.Text(lang, i18n.KeyCLIFlagDisallowedDomains))
	return fs
}

// setUsage attaches the standard usage printer to fs.
func setUsage(fs *flag.FlagSet, lang i18n.Language, output io.Writer) {
	if output == nil {
		output = io.Discard
	}
	fs.Usage = func() {
		fmt.Fprint(output, i18n.Format(lang, i18n.KeyCLIUsage, brand.CommandName))
		fmt.Fprint(output, i18n.Text(lang, i18n.KeyCLIHelpCodingSurface))
		fmt.Fprint(output, i18n.Text(lang, i18n.KeyCLIOptions))
		printFlagDefaults(output, fs, lang)
		fmt.Fprint(output, i18n.Text(lang, i18n.KeyCLIExamples))
		fmt.Fprint(output, i18n.Format(lang, i18n.KeyCLIExampleInteractive, brand.CommandName))
		fmt.Fprint(output, i18n.Format(lang, i18n.KeyCLIExamplePrint, brand.CommandName))
		fmt.Fprint(output, i18n.Format(lang, i18n.KeyCLIExampleModel, brand.CommandName, brand.DeepSeekDefaultModel))
		fmt.Fprint(output, i18n.Format(lang, i18n.KeyCLIExampleAllowedDir, brand.CommandName))
	}
}

func printFlagDefaults(w io.Writer, fs *flag.FlagSet, lang i18n.Language) {
	fs.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		if name == "" {
			fmt.Fprintf(w, "  -%s\n\t%s", f.Name, usage)
		} else {
			fmt.Fprintf(w, "  -%s %s\n\t%s", f.Name, name, usage)
		}
		if f.DefValue != "" && f.DefValue != "0" && f.DefValue != "false" {
			fmt.Fprint(w, i18n.Format(lang, i18n.KeyCLIFlagDefault, f.DefValue))
		}
		fmt.Fprintln(w)
	})
}

func printHelp(output io.Writer) {
	lang := i18n.DetectOrLoadLanguage()
	var opts Options
	var dirs multiString
	fs := newFlagSet(&opts, &dirs, lang)
	setUsage(fs, lang, output)
	fs.Usage()
}

// ParseArgs parses the supplied args slice and returns the populated Options.
// It returns ErrHelp when --help / -h is present and ErrVersion for --version / -v.
// Real parse errors (unknown flags, bad values) are returned as plain errors.
// It does NOT call os.Exit and does NOT print help or version text.
func ParseArgs(args []string) (Options, error) {
	lang := i18n.DetectOrLoadLanguage()
	var opts Options
	var dirs multiString

	fs := newFlagSet(&opts, &dirs, lang)
	// Suppress the default error+usage output from flag so callers control output.
	fs.SetOutput(io.Discard)
	setUsage(fs, lang, io.Discard)

	// Intercept -h / --help before flag.Parse so we control exit.
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			opts.Help = true
			return opts, ErrHelp
		}
	}

	if err := fs.Parse(args); err != nil {
		return opts, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyCLIParseFailure, err))
	}

	if opts.Version {
		return opts, ErrVersion
	}
	if opts.ForceSandboxTools {
		opts.Sandbox = true
	}
	if opts.ServiceTier != "" && opts.ServiceTier != "default" {
		return opts, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyCLIServiceTierInvalid, opts.ServiceTier))
	}
	if opts.ResponsesWebSocket && strings.TrimSpace(opts.API) != "" && !strings.EqualFold(strings.TrimSpace(opts.API), "responses") {
		return opts, i18n.NewError(i18n.KeyCLIResponsesWebSocketRequiresResponses)
	}

	opts.AllowedDirs = []string(dirs)
	opts.Args = fs.Args()
	if err := ValidateInputMode(opts, true); err != nil {
		return opts, err
	}

	// C8: Validate session ID to prevent path traversal attacks.
	if opts.SessionID != "" {
		validSessionID := regexp.MustCompile(`^[a-zA-Z0-9_T:.\-]+$`)
		if !validSessionID.MatchString(opts.SessionID) {
			return opts, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyCLIInvalidSessionChars, opts.SessionID))
		}
		if strings.Contains(opts.SessionID, "..") {
			return opts, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyCLIInvalidSessionParent, opts.SessionID))
		}
	}

	return opts, nil
}

// ValidateInputMode rejects combinations that would assign stdin to more than
// one transport. Screen-reader mode is interactive and therefore also needs a
// terminal stdin instead of a pipe carrying a print or SDK payload.
func ValidateInputMode(opts Options, stdinTerminal bool) error {
	lang := i18n.DetectOrLoadLanguage()
	if opts.SDK && opts.Print {
		return i18n.NewError(i18n.KeyCLIInputModeSDKPrint)
	}
	if !opts.ScreenReader {
		return nil
	}
	if opts.SDK {
		return fmt.Errorf("%s", i18n.Text(lang, i18n.KeyCLIScreenReaderSDK))
	}
	if opts.Print {
		return fmt.Errorf("%s", i18n.Text(lang, i18n.KeyCLIScreenReaderPrint))
	}
	if format := strings.ToLower(strings.TrimSpace(opts.OutputFormat)); format != "" && format != "text" {
		return fmt.Errorf("%s", i18n.Format(lang, i18n.KeyCLIScreenReaderOutput, format))
	}
	if !stdinTerminal {
		return fmt.Errorf("%s", i18n.Text(lang, i18n.KeyCLIScreenReaderTerminal))
	}
	return nil
}

// EarlyAction identifies a CLI request that completes before application
// runtime initialization. The command package owns no process-exit behavior;
// callers execute the action and decide what to do with its return code.
type EarlyAction uint8

const (
	EarlyActionNone EarlyAction = iota
	EarlyActionHelp
	EarlyActionVersion
	EarlyActionMCP
	EarlyActionPromptDump
)

// Invocation is the canonical result of parsing one process invocation.
type Invocation struct {
	Options    Options
	Action     EarlyAction
	ActionArgs []string
}

// ParseInvocation parses a complete command invocation without printing or
// terminating the process. MCP remains a first-position subcommand, matching
// the established command-line contract.
func ParseInvocation(args []string) (Invocation, error) {
	if len(args) > 0 && args[0] == "mcp" {
		return Invocation{Action: EarlyActionMCP, ActionArgs: append([]string(nil), args[1:]...)}, nil
	}
	opts, err := ParseArgs(args)
	if err != nil {
		switch {
		case errors.Is(err, ErrHelp):
			return Invocation{Options: opts, Action: EarlyActionHelp}, nil
		case errors.Is(err, ErrVersion):
			return Invocation{Options: opts, Action: EarlyActionVersion}, nil
		default:
			return Invocation{}, err
		}
	}
	if opts.PromptDump || opts.PromptDumpJSON {
		return Invocation{Options: opts, Action: EarlyActionPromptDump}, nil
	}
	return Invocation{Options: opts}, nil
}

// RunEarlyAction writes an early action to the supplied process streams and
// returns its exit code. It is a no-op for a normal runtime invocation.
func RunEarlyAction(invocation Invocation, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	switch invocation.Action {
	case EarlyActionNone:
		return 0
	case EarlyActionHelp:
		printHelp(stdout)
		return 0
	case EarlyActionVersion:
		fmt.Fprintf(stdout, "%s %s\n", brand.CommandName, Version)
		return 0
	case EarlyActionMCP:
		return RunMCPCLI(invocation.ActionArgs, stdout, stderr)
	case EarlyActionPromptDump:
		if err := dumpPrompt(invocation.Options, stdout); err != nil {
			fmt.Fprint(stderr, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCLIError, err))
			return 1
		}
		return 0
	default:
		return 2
	}
}

// ReadPipedPrompt reads the complete prompt for implicit print mode while
// enforcing a fixed memory bound. Explicit positional queries and SDK input do
// not enter this path.
func ReadPipedPrompt(input io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(input, maxPipedPromptBytes+1))
	if err != nil {
		return "", i18n.WrapError(i18n.KeyCLIStdinReadFailure, err)
	}
	if int64(len(data)) > maxPipedPromptBytes {
		return "", i18n.NewError(i18n.KeyCLIStdinTooLarge, maxPipedPromptBytes)
	}
	return string(data), nil
}

// RunMCPCLI executes the non-interactive `luban-code mcp ...` management
// surface. It is intentionally small and delegates behavior to commands/mcp.go.
func RunMCPCLI(args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprint(stderr, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCLIError, err))
		return 1
	}
	cmd := commands.NewMCPCommand(nil)
	ctx := &commands.Context{
		Language: i18n.DetectOrLoadLanguage(),
		CWD:      cwd,
		OnEvent: func(s string) {
			fmt.Fprint(stdout, s)
		},
	}
	if len(args) == 0 {
		args = []string{"list"}
	}
	if err := cmd.Execute(ctx, strings.Join(args, " ")); err != nil {
		fmt.Fprint(stderr, i18n.Format(ctx.Language, i18n.KeyCLIError, err))
		return 1
	}
	return 0
}

// dumpPrompt writes the rendered prompt construction snapshot requested by
// --prompt-dump or --prompt-dump-json. It intentionally does not initialize a
// provider or make API calls.
func dumpPrompt(opts Options, output io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCLIWorkingDirectoryError, err))
	}
	systemCtx := prompt.SystemContextBuilder{GitStatus: prompt.LoadGitContext(cwd)}.Build()
	userCtx := prompt.UserContextBuilder{Date: time.Now()}.
		FromConfig(prompt.Config{CustomInstructions: prompt.DiscoverInstructions(cwd)}).
		Build()

	var blocks prompt.SystemPrompt
	if strings.TrimSpace(opts.SystemPrompt) != "" {
		blocks = prompt.SystemPrompt{{Text: opts.SystemPrompt, Source: "override", Name: "override"}}
	} else {
		blocks = prompt.BuildSystemPromptBlocks(nil, prompt.Config{
			CWD:            cwd,
			AdditionalDirs: opts.AllowedDirs,
			Language:       opts.Language,
			OutputStyle:    opts.OutputStyle,
		})
	}
	blocks = systemCtx.AppendTo(blocks)
	blocks = prompt.ApplyCacheScopes(blocks, prompt.CacheScopeOptions{GlobalSafe: true})
	dump := prompt.BuildPromptDump(blocks, userCtx, systemCtx)
	if opts.PromptDumpJSON {
		return prompt.WritePromptDumpJSON(output, dump)
	}
	return prompt.WritePromptDumpText(output, dump)
}
