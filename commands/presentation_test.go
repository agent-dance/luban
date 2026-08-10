package commands_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/provider"
)

func TestBuiltinCommandPresentationContractsCoverRegistry(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)

	wantNames := []string{
		"exit", "help", "clear", "goal", "search", "export", "editor", "mouse",
		"activity", "detail", "compact", "model", "session", "config", "status",
		"context", "init", "resume", "fork", "review", "doctor", "language", "skills", "mcp",
	}
	all := registry.All()
	if len(all) != len(wantNames) {
		t.Fatalf("registered builtins = %d, want %d", len(all), len(wantNames))
	}
	gotNames := make([]string, len(all))
	for index, command := range all {
		gotNames[index] = command.Name()
		contract, exact := registry.PresentationContract(command.Name())
		if !exact {
			t.Errorf("/%s uses fallback presentation contract", command.Name())
		}
		if contract.Command != command.Name() || contract.Family == "" || contract.Display == "" || contract.Risk == "" ||
			contract.DefaultAction == "" || contract.CompletedNextActionKey == "" || contract.FailedNextActionKey == "" {
			t.Errorf("/%s has incomplete presentation contract: %+v", command.Name(), contract)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("registered builtin order/names = %v, want %v", gotNames, wantNames)
	}
}

func TestCommandPresentationEmitsTypedLifecycleWithoutLeakingRawEvents(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)

	var rawOnly strings.Builder
	if err := registry.Find("help").Execute(&commands.Context{OnEvent: func(value string) { rawOnly.WriteString(value) }}, ""); err != nil {
		t.Fatal(err)
	}

	var withPresentation strings.Builder
	var events []commands.CommandPresentation
	ctx := &commands.Context{
		OnEvent: func(value string) { withPresentation.WriteString(value) },
		OnCommandPresentation: func(event commands.CommandPresentation) {
			events = append(events, event)
		},
	}
	if err := registry.Find("help").Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if withPresentation.String() != rawOnly.String() {
		t.Fatalf("typed callback changed raw event output\nwith callback: %q\nraw only: %q", withPresentation.String(), rawOnly.String())
	}
	if len(events) != 2 {
		t.Fatalf("presentation events = %d, want start + terminal: %+v", len(events), events)
	}
	if events[0].State != commands.CommandStateRunning || events[0].Action != "list" {
		t.Fatalf("start event = %+v", events[0])
	}
	terminal := events[1]
	if terminal.State != commands.CommandStateCompleted || terminal.Outcome != commands.CommandOutcomeSucceeded ||
		terminal.Result == "" || terminal.NextAction == "" || terminal.Display != commands.CommandDisplayInspector {
		t.Fatalf("terminal event = %+v", terminal)
	}
	if terminal.Version != 2 || len(terminal.Sections) == 0 {
		t.Fatalf("terminal mirror/schema contract = %+v", terminal)
	}
}

func TestCommandPresentationFailureIsExplicitAndActionable(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	var events []commands.CommandPresentation
	ctx := &commands.Context{OnCommandPresentation: func(event commands.CommandPresentation) { events = append(events, event) }}

	err := registry.Find("clear").Execute(ctx, "view")
	if err == nil {
		t.Fatal("clear view without callback unexpectedly succeeded")
	}
	terminal := events[len(events)-1]
	if terminal.State != commands.CommandStateCompleted || terminal.Outcome != commands.CommandOutcomeFailed ||
		terminal.Action != "view" || terminal.Result == "" || terminal.NextAction == "" || !terminal.OutcomeReliable {
		t.Fatalf("failure presentation = %+v", terminal)
	}
	if !strings.Contains(terminal.Result, "not configured") {
		t.Fatalf("failure result omitted cause: %+v", terminal)
	}
}

type progressThenErrorCommand struct{ err error }

type typedCommandOutcomeError struct{ outcome commands.CommandOutcome }

func (e typedCommandOutcomeError) Error() string                           { return string(e.outcome) }
func (e typedCommandOutcomeError) CommandOutcome() commands.CommandOutcome { return e.outcome }

func (*progressThenErrorCommand) Name() string        { return "progress-error" }
func (*progressThenErrorCommand) Aliases() []string   { return nil }
func (*progressThenErrorCommand) Description() string { return "Emit progress before failing" }
func (c *progressThenErrorCommand) Execute(ctx *commands.Context, _ string) error {
	ctx.OnEvent("progress body")
	return c.err
}

func TestCommandPresentationRendersErrorAfterCapturedProgress(t *testing.T) {
	registry := commands.NewRegistry()
	registry.Register(&progressThenErrorCommand{err: errors.New("terminal failure")})
	var raw string
	var events []commands.CommandPresentation
	err := registry.Find("progress-error").Execute(&commands.Context{
		OnEvent:               func(value string) { raw += value },
		OnCommandPresentation: func(event commands.CommandPresentation) { events = append(events, event) },
	}, "")
	if err == nil {
		t.Fatal("command unexpectedly succeeded")
	}
	terminal := events[len(events)-1]
	if terminal.Result != "terminal failure" {
		t.Fatalf("terminal error ownership = %+v", terminal)
	}
	if raw != "" {
		t.Fatalf("raw command output escaped the presentation boundary: %q", raw)
	}
}

func TestCommandPresentationClassifiesStandardTerminalErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want commands.CommandOutcome
	}{
		{name: "cancelled", err: context.Canceled, want: commands.CommandOutcomeCancelled},
		{name: "timed_out", err: context.DeadlineExceeded, want: commands.CommandOutcomeTimedOut},
		{name: "denied", err: os.ErrPermission, want: commands.CommandOutcomeDenied},
		{name: "partial", err: typedCommandOutcomeError{outcome: commands.CommandOutcomePartial}, want: commands.CommandOutcomePartial},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := commands.NewRegistry()
			registry.Register(&progressThenErrorCommand{err: test.err})
			var events []commands.CommandPresentation
			_ = registry.Find("progress-error").Execute(&commands.Context{
				OnEvent:               func(string) {},
				OnCommandPresentation: func(event commands.CommandPresentation) { events = append(events, event) },
			}, "")
			if terminal := events[len(events)-1]; terminal.Outcome != test.want {
				t.Fatalf("outcome = %q, want %q: %+v", terminal.Outcome, test.want, terminal)
			}
		})
	}
}

func TestModelPersistenceFailureUsesWarningOutcome(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	file, err := os.CreateTemp(t.TempDir(), "not-a-directory")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var raw string
	var events []commands.CommandPresentation
	err = registry.Find("model").Execute(&commands.Context{
		CWD:                   file.Name(),
		QueryLoop:             &stubQL{model: "old-model"},
		OnEvent:               func(value string) { raw += value },
		OnCommandPresentation: func(event commands.CommandPresentation) { events = append(events, event) },
	}, "new-model")
	if err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if terminal.Outcome != commands.CommandOutcomeWarning || !terminal.OutcomeReliable {
		t.Fatalf("model persistence terminal = %+v", terminal)
	}
	if !strings.Contains(terminal.Result, "Warning: failed to persist model:") || raw != "" {
		t.Fatalf("warning presentation = %+v, raw=%q", terminal, raw)
	}
}

func TestCommandPresentationUsesExplicitDomainFailureWithoutParsingText(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	var events []commands.CommandPresentation
	ctx := &commands.Context{
		OnEvent: func(string) {},
		OnCommandPresentation: func(event commands.CommandPresentation) {
			events = append(events, event)
		},
	}
	if err := registry.Find("config").Execute(ctx, "get missing-key"); err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if terminal.Outcome != commands.CommandOutcomeFailed || !terminal.OutcomeReliable {
		t.Fatalf("explicit domain terminal outcome = %q reliable=%t, want failed/true: %+v", terminal.Outcome, terminal.OutcomeReliable, terminal)
	}
	if terminal.Result == "" {
		t.Fatalf("domain failure omitted result: %+v", terminal)
	}
}

type typedDomainOutcomeCommand struct {
	outcome commands.CommandOutcome
}

func (c *typedDomainOutcomeCommand) Name() string        { return "typed-domain" }
func (c *typedDomainOutcomeCommand) Aliases() []string   { return nil }
func (c *typedDomainOutcomeCommand) Description() string { return "Emit a typed domain outcome" }
func (c *typedDomainOutcomeCommand) Execute(ctx *commands.Context, _ string) error {
	ctx.OnEvent("diagnostic body")
	ctx.OnCommandDomainResult(commands.CommandDomainResult{
		Outcome: c.outcome, Result: "typed domain result", NextAction: "take the typed next action",
		Sections:     []commands.CommandPresentationSection{{Label: "Checks", Text: "3 passed"}},
		EvidenceRefs: []string{"artifact://report-7"},
	})
	return nil
}

func TestCommandPresentationPreservesEveryTypedTerminalOutcomeAndSections(t *testing.T) {
	outcomes := []commands.CommandOutcome{
		commands.CommandOutcomeSucceeded,
		commands.CommandOutcomeWarning,
		commands.CommandOutcomePartial,
		commands.CommandOutcomeFailed,
		commands.CommandOutcomeDenied,
		commands.CommandOutcomeCancelled,
		commands.CommandOutcomeTimedOut,
		commands.CommandOutcomeInterrupted,
	}
	for _, outcome := range outcomes {
		t.Run(string(outcome), func(t *testing.T) {
			registry := commands.NewRegistry()
			registry.Register(&typedDomainOutcomeCommand{outcome: outcome})
			var events []commands.CommandPresentation
			var raw string
			err := registry.Find("typed-domain").Execute(&commands.Context{
				OnEvent: func(value string) { raw += value },
				OnCommandPresentation: func(event commands.CommandPresentation) {
					events = append(events, event)
				},
			}, "")
			if err != nil {
				t.Fatal(err)
			}
			terminal := events[len(events)-1]
			if terminal.Outcome != outcome || !terminal.OutcomeReliable || terminal.Result != "typed domain result" {
				t.Fatalf("terminal = %+v", terminal)
			}
			if raw != "" {
				t.Fatalf("raw domain output escaped presentation: %+v raw=%q", terminal, raw)
			}
			if len(terminal.Sections) != 1 || terminal.Sections[0].Label != "Checks" || len(terminal.EvidenceRefs) != 1 || !terminal.HasMore {
				t.Fatalf("sections/evidence lost: %+v", terminal)
			}
		})
	}
}

func TestTypedDomainOutcomesCoverTextReturningBuiltins(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)

	tests := []struct {
		name    string
		args    string
		context func(*testing.T) *commands.Context
		want    commands.CommandOutcome
	}{
		{name: "session", args: "list", context: func(*testing.T) *commands.Context { return &commands.Context{} }, want: commands.CommandOutcomeFailed},
		{name: "config", args: "get absent", context: func(t *testing.T) *commands.Context { return &commands.Context{CWD: t.TempDir()} }, want: commands.CommandOutcomeFailed},
		{name: "init", context: func(t *testing.T) *commands.Context { return &commands.Context{CWD: t.TempDir()} }, want: commands.CommandOutcomeSucceeded},
		{name: "resume", context: func(*testing.T) *commands.Context { return &commands.Context{} }, want: commands.CommandOutcomeFailed},
		{name: "review", context: func(t *testing.T) *commands.Context { return &commands.Context{CWD: t.TempDir()} }, want: commands.CommandOutcomeFailed},
		{name: "doctor", context: func(t *testing.T) *commands.Context {
			return &commands.Context{CWD: t.TempDir(), QueryLoop: &stubQL{}, ProviderRegistry: provider.DefaultRegistry()}
		}, want: commands.CommandOutcomeUnknown},
		{name: "mcp", args: "get absent", context: func(t *testing.T) *commands.Context { return &commands.Context{CWD: t.TempDir()} }, want: commands.CommandOutcomeFailed},
		{name: "language", args: "show", context: func(*testing.T) *commands.Context {
			return &commands.Context{SwitchLanguage: func(string) string { return "Language: English" }}
		}, want: commands.CommandOutcomeSucceeded},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := test.context(t)
			ctx.OnEvent = func(string) {}
			var events []commands.CommandPresentation
			ctx.OnCommandPresentation = func(event commands.CommandPresentation) { events = append(events, event) }
			if err := registry.Find(test.name).Execute(ctx, test.args); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(events) != 2 {
				t.Fatalf("events = %d, want running + terminal: %+v", len(events), events)
			}
			terminal := events[1]
			if !terminal.OutcomeReliable || terminal.Outcome == commands.CommandOutcomeUnknown {
				t.Fatalf("terminal outcome is not explicit: %+v", terminal)
			}
			if test.want != commands.CommandOutcomeUnknown && terminal.Outcome != test.want {
				t.Fatalf("outcome = %q, want %q: %+v", terminal.Outcome, test.want, terminal)
			}
		})
	}
}

func TestCommandPresentationRedactsSensitiveResult(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	var events []commands.CommandPresentation
	ctx := &commands.Context{
		CWD:     t.TempDir(),
		OnEvent: func(string) {},
		OnCommandPresentation: func(event commands.CommandPresentation) {
			events = append(events, event)
		},
	}
	if err := registry.Find("config").Execute(ctx, "set apiKey must-not-leak"); err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if strings.Contains(terminal.Result, "must-not-leak") || !strings.Contains(terminal.Result, "[REDACTED]") {
		t.Fatalf("typed result leaked config credential: %+v", terminal)
	}
	if terminal.Target != "apiKey" {
		t.Fatalf("config target = %q, want key only", terminal.Target)
	}
}

func TestCommandPresentationRedactorCoversAuthTokensAndDeviceCodes(t *testing.T) {
	redacted := commands.RedactCommandPresentationText("token: must-not-leak\nUser Code: ABCD-EFGH\ntotal tokens: 42\nCredentials: ANTHROPIC_API_KEY not set", 0)
	if strings.Contains(redacted, "must-not-leak") || strings.Contains(redacted, "ABCD-EFGH") {
		t.Fatalf("auth material leaked: %q", redacted)
	}
	if !strings.Contains(redacted, "total tokens: 42") {
		t.Fatalf("safe usage metric was redacted: %q", redacted)
	}
	if !strings.Contains(redacted, "ANTHROPIC_API_KEY not set") {
		t.Fatalf("credential identifier without a value was redacted: %q", redacted)
	}
}

func TestCommandPresentationResolvesDynamicRiskAndAliases(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	var events []commands.CommandPresentation
	ctx := &commands.Context{
		DeleteHistory: func(string) error { return nil },
		OnCommandPresentation: func(event commands.CommandPresentation) {
			events = append(events, event)
		},
	}
	if err := registry.Find("session").Execute(ctx, "delete session-7"); err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if terminal.Action != "delete" || terminal.Target != "session-7" || terminal.Risk != commands.CommandRiskDestructive ||
		terminal.Display != commands.CommandDisplayDecision {
		t.Fatalf("destructive session presentation = %+v", terminal)
	}

	events = nil
	err := registry.Find("quit").Execute(&commands.Context{OnCommandPresentation: func(event commands.CommandPresentation) {
		events = append(events, event)
	}}, "")
	if !errors.Is(err, commands.ErrExit) {
		t.Fatalf("quit error = %v", err)
	}
	terminal = events[len(events)-1]
	if terminal.Command != "exit" || terminal.Outcome != commands.CommandOutcomeExitRequested ||
		terminal.Risk != commands.CommandRiskLow || terminal.Display != commands.CommandDisplayReceipt || terminal.Result != "" {
		t.Fatalf("alias presentation = %+v", terminal)
	}
}

func TestModelPickerStillUsesTypedCommandLifecycle(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	opened := 0
	var events []commands.CommandPresentation
	err := registry.Find("model").Execute(&commands.Context{
		OpenModelPicker: func() error { opened++; return nil },
		OnCommandPresentation: func(event commands.CommandPresentation) {
			events = append(events, event)
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if opened != 1 || len(events) != 2 || events[1].Outcome != commands.CommandOutcomeSucceeded || events[1].Action != "list" {
		t.Fatalf("picker lifecycle: opened=%d events=%+v", opened, events)
	}
}

func TestUncataloguedCommandUsesConservativePresentationFallback(t *testing.T) {
	registry := commands.NewRegistry()
	registry.Register(&pingCmd{})
	contract, exact := registry.PresentationContract("ping")
	if exact || contract.Command != "ping" || contract.Display != commands.CommandDisplayReceipt || contract.Risk != commands.CommandRiskUnknown {
		t.Fatalf("fallback contract = %+v exact=%t", contract, exact)
	}

	var events []commands.CommandPresentation
	if err := registry.Find("pong").Execute(&commands.Context{OnCommandPresentation: func(event commands.CommandPresentation) {
		events = append(events, event)
	}}, "payload"); err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if terminal.Outcome != commands.CommandOutcomeUnknown || terminal.Command != "ping" || terminal.Action != "execute" {
		t.Fatalf("fallback terminal = %+v", terminal)
	}
}

type longPresentationCommand struct{}

func (*longPresentationCommand) Name() string        { return "long-output" }
func (*longPresentationCommand) Aliases() []string   { return nil }
func (*longPresentationCommand) Description() string { return "Emit long output" }
func (*longPresentationCommand) Execute(ctx *commands.Context, _ string) error {
	ctx.OnEvent("HEAD_SENTINEL\n" + strings.Repeat("middle output\n", 300) + "TAIL_SENTINEL")
	return nil
}

type ansiPresentationCommand struct{}

func (*ansiPresentationCommand) Name() string        { return "ansi-output" }
func (*ansiPresentationCommand) Aliases() []string   { return nil }
func (*ansiPresentationCommand) Description() string { return "Emit styled output" }
func (*ansiPresentationCommand) Execute(ctx *commands.Context, _ string) error {
	ctx.OnEvent("\x1b[31mfailed-looking text\x1b[0m\n")
	return nil
}

func TestCommandPresentationStripsANSIFromResult(t *testing.T) {
	registry := commands.NewRegistry()
	registry.Register(&ansiPresentationCommand{})
	var events []commands.CommandPresentation
	if err := registry.Find("ansi-output").Execute(&commands.Context{
		OnCommandPresentation: func(event commands.CommandPresentation) { events = append(events, event) },
	}, ""); err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if strings.Contains(terminal.Result, "\x1b[") || terminal.Result != "failed-looking text" {
		t.Fatalf("typed result retained unstable ANSI bytes: %+v", terminal)
	}
}

func TestCommandPresentationResultIsBoundedAndRetainsHeadAndTail(t *testing.T) {
	registry := commands.NewRegistry()
	registry.Register(&longPresentationCommand{})
	var events []commands.CommandPresentation
	var raw strings.Builder
	if err := registry.Find("long-output").Execute(&commands.Context{
		OnEvent:               func(value string) { raw.WriteString(value) },
		OnCommandPresentation: func(event commands.CommandPresentation) { events = append(events, event) },
	}, ""); err != nil {
		t.Fatal(err)
	}
	terminal := events[len(events)-1]
	if !terminal.HasMore || !strings.Contains(terminal.Result, "HEAD_SENTINEL") || !strings.Contains(terminal.Result, "TAIL_SENTINEL") {
		t.Fatalf("bounded result lost disclosure metadata or sentinels: %+v", terminal)
	}
	if utf8.RuneCountInString(terminal.Result) > 1200 {
		t.Fatalf("bounded result has %d runes", utf8.RuneCountInString(terminal.Result))
	}
	if raw.Len() != 0 {
		t.Fatalf("raw long output escaped presentation: %d bytes", raw.Len())
	}
}
