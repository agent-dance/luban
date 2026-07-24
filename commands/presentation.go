package commands

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
)

const (
	commandPresentationVersion  = 2
	maxCommandPresentationRunes = 1200
)

// CommandFamily groups slash commands by the user workflow they affect.
type CommandFamily string

const (
	CommandFamilyLifecycle     CommandFamily = "lifecycle"
	CommandFamilyDiscovery     CommandFamily = "discovery"
	CommandFamilySession       CommandFamily = "session"
	CommandFamilyGoal          CommandFamily = "goal"
	CommandFamilyTranscript    CommandFamily = "transcript"
	CommandFamilyInterface     CommandFamily = "interface"
	CommandFamilyActivity      CommandFamily = "activity"
	CommandFamilyRuntime       CommandFamily = "runtime"
	CommandFamilyConfiguration CommandFamily = "configuration"
	CommandFamilyDiagnostics   CommandFamily = "diagnostics"
	CommandFamilyIntegration   CommandFamily = "integration"
	CommandFamilyExtension     CommandFamily = "extension"
)

// CommandDisplayCategory identifies the minimum semantic surface a command
// needs. Renderers may expand it, but must not silently lower decisions or
// evidence inspectors to a generic success line.
type CommandDisplayCategory string

const (
	CommandDisplayReceipt   CommandDisplayCategory = "receipt"
	CommandDisplayInspector CommandDisplayCategory = "inspector"
	CommandDisplayEvidence  CommandDisplayCategory = "evidence"
	CommandDisplayDecision  CommandDisplayCategory = "decision"
)

// CommandRiskCategory describes the consequence of the requested command, not
// merely whether its implementation returned an error.
type CommandRiskCategory string

const (
	CommandRiskUnknown     CommandRiskCategory = "unknown"
	CommandRiskLow         CommandRiskCategory = "low"
	CommandRiskMedium      CommandRiskCategory = "medium"
	CommandRiskHigh        CommandRiskCategory = "high"
	CommandRiskDestructive CommandRiskCategory = "destructive"
)

// CommandState is lifecycle-only. Outcome is kept separate so a legacy command
// can complete execution without being mislabeled as a domain success.
type CommandState string

const (
	CommandStateRunning   CommandState = "running"
	CommandStateCompleted CommandState = "completed"
)

// CommandOutcome is deliberately conservative for commands whose domain result
// is still encoded only in OnEvent prose.
type CommandOutcome string

const (
	CommandOutcomeUnknown       CommandOutcome = "unknown"
	CommandOutcomeSucceeded     CommandOutcome = "succeeded"
	CommandOutcomeWarning       CommandOutcome = "warning"
	CommandOutcomePartial       CommandOutcome = "partial"
	CommandOutcomeFailed        CommandOutcome = "failed"
	CommandOutcomeDenied        CommandOutcome = "denied"
	CommandOutcomeCancelled     CommandOutcome = "cancelled"
	CommandOutcomeTimedOut      CommandOutcome = "timed_out"
	CommandOutcomeInterrupted   CommandOutcome = "interrupted"
	CommandOutcomeExitRequested CommandOutcome = "exit_requested"
)

// CommandPresentationSection preserves the semantic shape of multi-part
// inspectors without requiring renderers to parse headings out of Result.
type CommandPresentationSection struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

// CommandPresentation is a stable, renderer-neutral command lifecycle record.
// Result is a bounded, redacted display projection; OnEvent retains the full
// legacy text and remains the compatibility/machine path.
type CommandPresentation struct {
	Version         int                          `json:"version"`
	Command         string                       `json:"command"`
	Family          CommandFamily                `json:"family"`
	Action          string                       `json:"action"`
	Target          string                       `json:"target,omitempty"`
	State           CommandState                 `json:"state"`
	Outcome         CommandOutcome               `json:"outcome"`
	Summary         string                       `json:"summary"`
	Result          string                       `json:"result,omitempty"`
	NextAction      string                       `json:"next_action"`
	Display         CommandDisplayCategory       `json:"display"`
	Risk            CommandRiskCategory          `json:"risk"`
	OutcomeReliable bool                         `json:"outcome_reliable"`
	Sensitive       bool                         `json:"sensitive,omitempty"`
	HasMore         bool                         `json:"has_more,omitempty"`
	Sections        []CommandPresentationSection `json:"sections,omitempty"`
	EvidenceRefs    []string                     `json:"evidence_refs,omitempty"`
	// ResultMirrorsEvents is true when Result is the bounded projection of
	// OnEvent output. LegacyOutputForwarded distinguishes a displayed legacy
	// stream from a typed-only consumer, so renderers can avoid duplicate text
	// without dropping the only available result.
	ResultMirrorsEvents   bool `json:"result_mirrors_events,omitempty"`
	LegacyOutputForwarded bool `json:"legacy_output_forwarded,omitempty"`
}

// CommandDomainResult is emitted by commands whose legacy Execute contract
// returns nil even when the requested domain action cannot be completed.
// Result and NextAction are optional semantic overrides; leaving Result empty
// preserves the bounded/redacted projection of the legacy OnEvent output.
type CommandDomainResult struct {
	Outcome      CommandOutcome               `json:"outcome"`
	Result       string                       `json:"result,omitempty"`
	NextAction   string                       `json:"next_action,omitempty"`
	Sections     []CommandPresentationSection `json:"sections,omitempty"`
	EvidenceRefs []string                     `json:"evidence_refs,omitempty"`
}

// CommandPresentationContract defines the stable semantics for one canonical
// slash command. TerminalOutcomeReliable is false when legacy implementations
// can report a domain failure only through OnEvent text while returning nil.
type CommandPresentationContract struct {
	Command                 string                 `json:"command"`
	Family                  CommandFamily          `json:"family"`
	Display                 CommandDisplayCategory `json:"display"`
	Risk                    CommandRiskCategory    `json:"risk"`
	DefaultAction           string                 `json:"default_action"`
	DefaultTarget           string                 `json:"default_target,omitempty"`
	CompletedNextAction     string                 `json:"completed_next_action"`
	FailedNextAction        string                 `json:"failed_next_action"`
	CompletedNextActionKey  i18n.Key               `json:"-"`
	FailedNextActionKey     i18n.Key               `json:"-"`
	TerminalOutcomeReliable bool                   `json:"terminal_outcome_reliable"`
}

// CommandPresentationProvider lets extension commands declare a typed contract
// without changing the long-standing Command interface.
type CommandPresentationProvider interface {
	PresentationContract() CommandPresentationContract
}

var builtinCommandPresentationContracts = map[string]CommandPresentationContract{
	"exit":     commandContract("exit", CommandFamilyLifecycle, CommandDisplayDecision, CommandRiskHigh, "exit", true),
	"help":     commandContract("help", CommandFamilyDiscovery, CommandDisplayInspector, CommandRiskLow, "list", true),
	"clear":    commandContract("clear", CommandFamilySession, CommandDisplayReceipt, CommandRiskHigh, "conversation", true),
	"goal":     commandContract("goal", CommandFamilyGoal, CommandDisplayInspector, CommandRiskMedium, "status", true),
	"search":   commandContract("search", CommandFamilyTranscript, CommandDisplayEvidence, CommandRiskLow, "search", true),
	"export":   commandContract("export", CommandFamilyTranscript, CommandDisplayReceipt, CommandRiskMedium, "export", true),
	"editor":   commandContract("editor", CommandFamilyTranscript, CommandDisplayEvidence, CommandRiskMedium, "open", true),
	"mouse":    commandContract("mouse", CommandFamilyInterface, CommandDisplayReceipt, CommandRiskMedium, "toggle", true),
	"activity": commandContract("activity", CommandFamilyActivity, CommandDisplayInspector, CommandRiskMedium, "list", true),
	"detail":   commandContract("detail", CommandFamilyTranscript, CommandDisplayEvidence, CommandRiskLow, "next", true),
	"compact":  commandContract("compact", CommandFamilyRuntime, CommandDisplayReceipt, CommandRiskMedium, "compact", true),
	"model":    commandContract("model", CommandFamilyRuntime, CommandDisplayInspector, CommandRiskMedium, "list", true),
	"session":  commandContract("session", CommandFamilySession, CommandDisplayInspector, CommandRiskMedium, "current", false),
	"config":   commandContract("config", CommandFamilyConfiguration, CommandDisplayInspector, CommandRiskMedium, "list", false),
	"status":   commandContract("status", CommandFamilyRuntime, CommandDisplayInspector, CommandRiskLow, "inspect", true),
	"context":  commandContract("context", CommandFamilyRuntime, CommandDisplayInspector, CommandRiskLow, "inspect", true),
	"init":     commandContract("init", CommandFamilyConfiguration, CommandDisplayReceipt, CommandRiskMedium, "initialize", false),
	"resume":   commandContract("resume", CommandFamilySession, CommandDisplayInspector, CommandRiskMedium, "list", false),
	"fork":     commandContract("fork", CommandFamilySession, CommandDisplayDecision, CommandRiskHigh, "select", true),
	"review":   commandContract("review", CommandFamilyTranscript, CommandDisplayEvidence, CommandRiskLow, "review", false),
	"doctor":   commandContract("doctor", CommandFamilyDiagnostics, CommandDisplayInspector, CommandRiskLow, "diagnose", false),
	"skills":   commandContract("skills", CommandFamilyExtension, CommandDisplayInspector, CommandRiskMedium, "list", false),
	"mcp":      commandContract("mcp", CommandFamilyIntegration, CommandDisplayInspector, CommandRiskHigh, "list", false),
	"language": commandContract("language", CommandFamilyInterface, CommandDisplayReceipt, CommandRiskMedium, "show", false),

	// Dormant implementations are intentionally not registered by
	// RegisterBuiltins. Keeping their contracts here prevents a future
	// re-registration from silently falling back to an untyped success line.
	"connect":     commandContract("connect", CommandFamilyIntegration, CommandDisplayDecision, CommandRiskHigh, "connect", false),
	"paste":       commandContract("paste", CommandFamilyInterface, CommandDisplayDecision, CommandRiskHigh, "paste", false),
	"permissions": commandContract("permissions", CommandFamilyConfiguration, CommandDisplayInspector, CommandRiskHigh, "inspect", false),
	"cost":        commandContract("cost", CommandFamilyRuntime, CommandDisplayInspector, CommandRiskLow, "inspect", true),
	"version":     commandContract("version", CommandFamilyDiscovery, CommandDisplayReceipt, CommandRiskLow, "inspect", true),
	"rename":      commandContract("rename", CommandFamilySession, CommandDisplayReceipt, CommandRiskMedium, "rename", false),
	"memory":      commandContract("memory", CommandFamilyConfiguration, CommandDisplayDecision, CommandRiskMedium, "edit", false),
	"diff":        commandContract("diff", CommandFamilyTranscript, CommandDisplayEvidence, CommandRiskLow, "inspect", false),
}

func commandContract(command string, family CommandFamily, display CommandDisplayCategory, risk CommandRiskCategory, action string, reliable bool) CommandPresentationContract {
	completedKey, _ := i18n.CommandPresentationNextKey(command, false)
	failedKey, _ := i18n.CommandPresentationNextKey(command, true)
	return commandContractWithKeys(command, family, display, risk, action, completedKey, failedKey, reliable)

}

func commandContractWithKeys(command string, family CommandFamily, display CommandDisplayCategory, risk CommandRiskCategory, action string, completedKey, failedKey i18n.Key, reliable bool) CommandPresentationContract {
	return CommandPresentationContract{
		Command: command, Family: family, Display: display, Risk: risk, DefaultAction: action,
		CompletedNextAction:    i18n.Text(i18n.DetectOrLoadLanguage(), completedKey),
		FailedNextAction:       i18n.Text(i18n.DetectOrLoadLanguage(), failedKey),
		CompletedNextActionKey: completedKey, FailedNextActionKey: failedKey,
		TerminalOutcomeReliable: reliable,
	}
}

type presentedCommand struct {
	command  Command
	contract CommandPresentationContract
	exact    bool
}

func wrapCommandPresentation(command Command) Command {
	if command == nil {
		return nil
	}
	if _, ok := command.(interface{ commandPresentationWrapped() }); ok {
		return command
	}
	contract, exact := commandPresentationContract(command)
	presented := &presentedCommand{command: command, contract: contract, exact: exact}
	if prompt, ok := command.(*MCPPromptCommand); ok {
		return &presentedMCPPromptCommand{presentedCommand: presented, prompt: prompt}
	}
	return presented
}

func (c *presentedCommand) Name() string        { return c.command.Name() }
func (c *presentedCommand) Aliases() []string   { return c.command.Aliases() }
func (c *presentedCommand) Description() string { return c.command.Description() }
func (c *presentedCommand) DescriptionKey() i18n.Key {
	key, _ := CommandDescriptionKey(c.command)
	return key
}
func (c *presentedCommand) commandPresentationWrapped() {}
func (c *presentedCommand) PresentationContract() CommandPresentationContract {
	return c.contract
}

// presentedMCPPromptCommand retains the optional discovery/typeahead surface
// of MCPPromptCommand while adding the common presentation lifecycle.
type presentedMCPPromptCommand struct {
	*presentedCommand
	prompt *MCPPromptCommand
}

func (c *presentedMCPPromptCommand) ArgumentHint() string { return c.prompt.ArgumentHint() }
func (c *presentedMCPPromptCommand) ArgumentNames() []string {
	return c.prompt.ArgumentNames()
}
func (c *presentedMCPPromptCommand) RequiredArgumentNames() []string {
	return c.prompt.RequiredArgumentNames()
}
func (c *presentedMCPPromptCommand) IsMCP() bool            { return c.prompt.IsMCP() }
func (c *presentedMCPPromptCommand) UserFacingName() string { return c.prompt.UserFacingName() }

func (c *presentedCommand) Execute(ctx *Context, args string) error {
	resolved := resolveCommandPresentation(c.contract, args)
	if ctx == nil {
		return c.command.Execute(nil, args)
	}

	emitCommandPresentation(ctx, CommandPresentation{
		Version: commandPresentationVersion, Command: c.Name(), Family: c.contract.Family,
		Action: resolved.action, Target: resolved.target, State: CommandStateRunning, Outcome: CommandOutcomeUnknown,
		Summary: commandPresentationSummary(c.Name(), resolved.action), NextAction: i18n.Text(ctx.Language, i18n.KeyCommandPresentationWait),
		Display: resolved.display, Risk: resolved.risk, OutcomeReliable: c.contract.TerminalOutcomeReliable,
	})

	legacyOnEvent := ctx.OnEvent
	legacyDomainResult := ctx.OnCommandDomainResult
	var output strings.Builder
	var domainResult CommandDomainResult
	domainResultReported := false
	executionContext := *ctx
	executionContext.OnEvent = func(value string) {
		output.WriteString(value)
		if legacyOnEvent != nil {
			legacyOnEvent(value)
		}
	}
	executionContext.OnCommandDomainResult = func(value CommandDomainResult) {
		domainResult = value
		domainResultReported = true
		if legacyDomainResult != nil {
			legacyDomainResult(value)
		}
	}
	err := c.command.Execute(&executionContext, args)

	outcome := CommandOutcomeUnknown
	outcomeReliable := c.contract.TerminalOutcomeReliable
	nextAction := i18n.Text(ctx.Language, i18n.KeyCommandPresentationInspectResult)
	result := strings.TrimSpace(output.String())
	resultMirrorsEvents := result != ""
	legacyOutputForwarded := resultMirrorsEvents && legacyOnEvent != nil
	sections := []CommandPresentationSection(nil)
	evidenceRefs := []string(nil)
	if errors.Is(err, ErrExit) {
		outcome = CommandOutcomeExitRequested
		outcomeReliable = true
		nextAction = localizedCommandNextAction(ctx.Language, c.contract, false)
		if result == "" {
			result = i18n.Text(ctx.Language, i18n.KeyCommandPresentationExitRequested)
		}
	} else if err != nil {
		outcome = CommandOutcomeForError(err)
		outcomeReliable = true
		nextAction = localizedCommandNextAction(ctx.Language, c.contract, true)
		result = err.Error()
		// The error replaces the bounded projection of any legacy progress
		// already forwarded through OnEvent. Keeping the mirror bit here would
		// suppress the only rendering of the actual terminal failure.
		resultMirrorsEvents = false
	} else if domainResultReported && commandOutcomeIsTerminal(domainResult.Outcome) {
		outcome = domainResult.Outcome
		outcomeReliable = true
		if domainResult.Result != "" {
			result = domainResult.Result
			resultMirrorsEvents = false
		}
		sections = append(sections, domainResult.Sections...)
		evidenceRefs = append(evidenceRefs, domainResult.EvidenceRefs...)
		if domainResult.NextAction != "" {
			nextAction = domainResult.NextAction
		} else if !commandOutcomeUsesFailedNextAction(outcome) {
			nextAction = localizedCommandNextAction(ctx.Language, c.contract, false)
		} else {
			nextAction = localizedCommandNextAction(ctx.Language, c.contract, true)
		}
	} else if c.contract.TerminalOutcomeReliable {
		outcome = CommandOutcomeSucceeded
		nextAction = localizedCommandNextAction(ctx.Language, c.contract, false)
	}
	if result == "" {
		result = i18n.Format(ctx.Language, i18n.KeyCommandPresentationCompleted, c.Name(), resolved.action)
		resultMirrorsEvents = false
		legacyOutputForwarded = false
	}
	result, sensitive, hasMore := boundedCommandPresentationText(result, maxCommandPresentationRunes)
	sections, sectionSensitive, sectionHasMore := normalizeCommandPresentationSections(ctx.Language, sections, result)
	evidenceRefs, evidenceSensitive, evidenceHasMore := normalizeCommandEvidenceRefs(evidenceRefs)
	emitCommandPresentation(ctx, CommandPresentation{
		Version: commandPresentationVersion, Command: c.Name(), Family: c.contract.Family,
		Action: resolved.action, Target: resolved.target, State: CommandStateCompleted, Outcome: outcome,
		Summary: commandPresentationSummary(c.Name(), resolved.action), Result: result, NextAction: nextAction,
		Display: resolved.display, Risk: resolved.risk, OutcomeReliable: outcomeReliable,
		Sensitive: sensitive || sectionSensitive || evidenceSensitive,
		HasMore:   hasMore || sectionHasMore || evidenceHasMore || len(evidenceRefs) > 0,
		Sections:  sections, EvidenceRefs: evidenceRefs,
		ResultMirrorsEvents: resultMirrorsEvents, LegacyOutputForwarded: legacyOutputForwarded,
	})
	return err
}

// CommandOutcomeForError maps standard cancellation, deadline, and permission
// failures onto the renderer-neutral command outcome vocabulary. Error types
// with richer domain knowledge can implement CommandOutcome() CommandOutcome;
// invalid or non-terminal values safely fall back to failed.
func CommandOutcomeForError(err error) CommandOutcome {
	if err == nil {
		return CommandOutcomeSucceeded
	}
	if errors.Is(err, ErrExit) {
		return CommandOutcomeExitRequested
	}
	var reported interface{ CommandOutcome() CommandOutcome }
	if errors.As(err, &reported) {
		if outcome := reported.CommandOutcome(); commandOutcomeIsTerminal(outcome) {
			return outcome
		}
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return CommandOutcomeTimedOut
	case errors.Is(err, context.Canceled):
		return CommandOutcomeCancelled
	case errors.Is(err, fs.ErrPermission):
		return CommandOutcomeDenied
	default:
		return CommandOutcomeFailed
	}
}

func commandOutcomeIsTerminal(outcome CommandOutcome) bool {
	switch outcome {
	case CommandOutcomeSucceeded, CommandOutcomeWarning, CommandOutcomePartial,
		CommandOutcomeFailed, CommandOutcomeDenied, CommandOutcomeCancelled,
		CommandOutcomeTimedOut, CommandOutcomeInterrupted, CommandOutcomeExitRequested:
		return true
	default:
		return false
	}
}

func commandOutcomeUsesFailedNextAction(outcome CommandOutcome) bool {
	switch outcome {
	case CommandOutcomeFailed, CommandOutcomeDenied, CommandOutcomeCancelled,
		CommandOutcomeTimedOut, CommandOutcomeInterrupted:
		return true
	default:
		return false
	}
}

func normalizeCommandPresentationSections(lang i18n.Language, sections []CommandPresentationSection, fallback string) ([]CommandPresentationSection, bool, bool) {
	if len(sections) == 0 && strings.TrimSpace(fallback) != "" {
		sections = []CommandPresentationSection{{Label: i18n.Text(lang, i18n.KeyCommandPresentationResult), Text: fallback}}
	}
	hasMore := len(sections) > 8
	if len(sections) > 8 {
		sections = sections[:8]
	}
	out := make([]CommandPresentationSection, 0, len(sections))
	sensitive := false
	for _, section := range sections {
		label, labelSensitive, labelMore := boundedCommandPresentationText(section.Label, 80)
		body, bodySensitive, bodyMore := boundedCommandPresentationText(section.Text, 480)
		label = strings.TrimSpace(label)
		body = strings.TrimSpace(body)
		if label == "" || body == "" {
			continue
		}
		out = append(out, CommandPresentationSection{Label: label, Text: body})
		sensitive = sensitive || labelSensitive || bodySensitive
		hasMore = hasMore || labelMore || bodyMore
	}
	return out, sensitive, hasMore
}

func normalizeCommandEvidenceRefs(refs []string) ([]string, bool, bool) {
	hasMore := len(refs) > 8
	if len(refs) > 8 {
		refs = refs[:8]
	}
	out := make([]string, 0, len(refs))
	sensitive := false
	for _, ref := range refs {
		value, valueSensitive, valueMore := boundedCommandPresentationText(ref, 240)
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
		sensitive = sensitive || valueSensitive
		hasMore = hasMore || valueMore
	}
	return out, sensitive, hasMore
}

func reportCommandDomainResult(ctx *Context, outcome CommandOutcome, result, nextAction string) {
	if ctx == nil || ctx.OnCommandDomainResult == nil {
		return
	}
	ctx.OnCommandDomainResult(CommandDomainResult{Outcome: outcome, Result: result, NextAction: nextAction})
}

func reportCommandSucceeded(ctx *Context) {
	reportCommandDomainResult(ctx, CommandOutcomeSucceeded, "", "")
}

func reportCommandFailed(ctx *Context) {
	reportCommandDomainResult(ctx, CommandOutcomeFailed, "", "")
}

func commandPresentationContract(command Command) (CommandPresentationContract, bool) {
	if command == nil {
		return fallbackCommandPresentationContract("unknown"), false
	}
	if wrapped, ok := command.(*presentedCommand); ok {
		return wrapped.contract, wrapped.exact
	}
	if provider, ok := command.(CommandPresentationProvider); ok {
		contract := provider.PresentationContract()
		if contract.Command == "" {
			contract.Command = command.Name()
		}
		return normalizeCommandPresentationContract(contract), true
	}
	if contract, ok := builtinCommandPresentationContracts[command.Name()]; ok {
		return contract, true
	}
	return fallbackCommandPresentationContract(command.Name()), false
}

// LookupCommandPresentationContract reads the canonical catalog without
// implying that the named command is currently registered.
func LookupCommandPresentationContract(name string) (CommandPresentationContract, bool) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	contract, ok := builtinCommandPresentationContracts[name]
	if ok {
		return contract, true
	}
	return fallbackCommandPresentationContract(name), false
}

func normalizeCommandPresentationContract(contract CommandPresentationContract) CommandPresentationContract {
	if contract.Family == "" {
		contract.Family = CommandFamilyExtension
	}
	if contract.Display == "" {
		contract.Display = CommandDisplayReceipt
	}
	if contract.Risk == "" {
		contract.Risk = CommandRiskUnknown
	}
	if contract.DefaultAction == "" {
		contract.DefaultAction = "execute"
	}
	if contract.CompletedNextActionKey == "" && contract.CompletedNextAction == "" {
		contract.CompletedNextActionKey = i18n.KeyCommandPresentationExtensionSuccess
		contract.CompletedNextAction = i18n.Text(i18n.DetectOrLoadLanguage(), contract.CompletedNextActionKey)
	}
	if contract.FailedNextActionKey == "" && contract.FailedNextAction == "" {
		contract.FailedNextActionKey = i18n.KeyCommandPresentationExtensionFailure
		contract.FailedNextAction = i18n.Text(i18n.DetectOrLoadLanguage(), contract.FailedNextActionKey)
	}
	return contract
}

func fallbackCommandPresentationContract(name string) CommandPresentationContract {
	return commandContractWithKeys(name, CommandFamilyExtension, CommandDisplayReceipt, CommandRiskUnknown, "execute",
		i18n.KeyCommandPresentationExtensionSuccess, i18n.KeyCommandPresentationExtensionFailure, false)
}

func localizedCommandNextAction(lang i18n.Language, contract CommandPresentationContract, failed bool) string {
	key := contract.CompletedNextActionKey
	value := contract.CompletedNextAction
	if failed {
		key = contract.FailedNextActionKey
		value = contract.FailedNextAction
	}
	if key != "" {
		return i18n.Text(lang, key)
	}
	if value != "" {
		// Extension contracts predating semantic keys remain compatible. First-party
		// contracts always take the key path above.
		return value
	}
	if failed {
		return i18n.Text(lang, i18n.KeyCommandPresentationInspectError)
	}
	return i18n.Text(lang, i18n.KeyCommandPresentationInspectResult)
}

// LocalizedCommandNextAction resolves a presentation contract using the
// active runtime language while preserving pre-semantic extension contracts.
func LocalizedCommandNextAction(lang i18n.Language, contract CommandPresentationContract, failed bool) string {
	return localizedCommandNextAction(lang, contract, failed)
}

func emitCommandPresentation(ctx *Context, event CommandPresentation) {
	if ctx != nil && ctx.OnCommandPresentation != nil {
		ctx.OnCommandPresentation(event)
	}
}

type resolvedCommandPresentation struct {
	action  string
	target  string
	display CommandDisplayCategory
	risk    CommandRiskCategory
}

func resolveCommandPresentation(contract CommandPresentationContract, args string) resolvedCommandPresentation {
	fields := strings.Fields(args)
	resolved := resolvedCommandPresentation{action: contract.DefaultAction, target: contract.DefaultTarget, display: contract.Display, risk: contract.Risk}
	first := commandField(fields, 0)
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args), first))

	switch contract.Command {
	case "clear":
		if first == "view" {
			resolved.action, resolved.risk = "view", CommandRiskLow
		} else {
			resolved.action = "conversation"
		}
	case "goal":
		switch first {
		case "", "status", "view":
			resolved.action, resolved.risk = "status", CommandRiskLow
		case "edit", "pause", "resume", "clear", "stop", "off", "reset", "none", "cancel":
			resolved.action = first
			resolved.target = boundedCommandTarget(rest)
		case "set":
			resolved.action, resolved.target = "set", boundedCommandTarget(rest)
		default:
			resolved.action, resolved.target = "set", boundedCommandTarget(args)
		}
	case "search":
		switch first {
		case "--next", "--previous", "--close":
			resolved.action = strings.TrimPrefix(first, "--")
		default:
			resolved.action, resolved.target = "search", boundedCommandTarget(args)
		}
	case "export":
		resolved.target = boundedCommandTarget(args)
	case "editor":
		if first == "detail" {
			resolved.action, resolved.target = "detail", boundedCommandTarget(rest)
		} else {
			resolved.action, resolved.target = "transcript", boundedCommandTarget(args)
		}
	case "mouse":
		if first != "" {
			resolved.action = strings.ToLower(first)
		}
	case "activity":
		if first == "" || first == "list" {
			resolved.action, resolved.risk = "list", CommandRiskLow
		} else if first == "close" {
			resolved.action, resolved.risk = "close", CommandRiskLow
		} else {
			resolved.target = boundedCommandTarget(first)
			resolved.action = commandField(fields, 1)
			if resolved.action == "" {
				resolved.action = "inspect"
			}
		}
	case "detail":
		resolved.target = boundedCommandTarget(first)
		if level := commandField(fields, 1); level != "" {
			resolved.action = level
		}
	case "model":
		if first == "" {
			resolved.action, resolved.risk = "list", CommandRiskLow
		} else {
			resolved.action, resolved.target, resolved.display = "switch", boundedCommandTarget(args), CommandDisplayReceipt
		}
	case "session":
		if first != "" {
			resolved.action = first
		}
		resolved.target = boundedCommandTarget(rest)
		switch resolved.action {
		case "current", "list":
			resolved.risk = CommandRiskLow
		case "delete":
			resolved.risk, resolved.display = CommandRiskDestructive, CommandDisplayDecision
		default:
			resolved.display = CommandDisplayReceipt
		}
	case "config":
		if first != "" {
			resolved.action = first
		}
		resolved.target = boundedCommandIdentifier(commandField(fields, 1), 160)
		if resolved.action == "list" || resolved.action == "get" {
			resolved.risk = CommandRiskLow
		} else {
			resolved.display = CommandDisplayReceipt
		}
	case "init":
		resolved.target = boundedCommandTarget(args)
	case "resume":
		if strings.TrimSpace(args) == "" {
			resolved.action, resolved.risk = "list", CommandRiskLow
		} else {
			resolved.action, resolved.target, resolved.display = "load", boundedCommandTarget(args), CommandDisplayReceipt
		}
	case "review":
		if strings.Contains(args, "--staged") || strings.Contains(args, "--cached") {
			resolved.action = "review-staged"
		}
	case "skills":
		if first != "" {
			resolved.action = first
		}
		resolved.target = boundedCommandTarget(commandField(fields, 1))
		switch resolved.action {
		case "list", "status", "show", "get", "info", "help", "-h", "--help":
			resolved.risk, resolved.display = CommandRiskLow, CommandDisplayInspector
		default:
			resolved.risk, resolved.display = CommandRiskMedium, CommandDisplayReceipt
		}
	case "mcp":
		if first != "" && first != "--json" {
			resolved.action = first
		}
		if first == "--json" {
			resolved.action = firstNonEmptyCommandField(commandField(fields, 1), "list")
		}
		resolved.target = mcpCommandTarget(fields)
		switch resolved.action {
		case "list", "status", "get", "show", "diagnostics", "doctor", "help", "-h", "--help":
			resolved.risk = CommandRiskLow
		case "remove":
			resolved.risk, resolved.display = CommandRiskDestructive, CommandDisplayDecision
		case "authenticate", "auth", "add-json":
			resolved.risk, resolved.display = CommandRiskHigh, CommandDisplayDecision
		default:
			resolved.risk, resolved.display = CommandRiskMedium, CommandDisplayReceipt
		}
	case "connect":
		resolved.target = boundedCommandTarget(args)
	case "permissions":
		if first != "" {
			resolved.action = first
		}
		resolved.target = boundedCommandTarget(rest)
	case "rename":
		resolved.target = boundedCommandTarget(args)
	}
	return resolved
}

func commandPresentationSummary(command, action string) string {
	return strings.TrimSpace("/" + command + " " + action)
}

func commandField(fields []string, index int) string {
	if index < 0 || index >= len(fields) {
		return ""
	}
	return fields[index]
}

func firstNonEmptyCommandField(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func mcpCommandTarget(fields []string) string {
	start := 1
	if commandField(fields, 0) == "--json" {
		start = 2
	}
	for index := start; index < len(fields); index++ {
		if fields[index] == "--json" {
			continue
		}
		return boundedCommandTarget(fields[index])
	}
	return ""
}

func boundedCommandTarget(value string) string {
	redacted, _, _ := boundedCommandPresentationText(value, 160)
	return redacted
}

func boundedCommandIdentifier(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit < 1 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}

// RedactCommandPresentationText returns a bounded display projection suitable
// for typed command events. It does not change legacy OnEvent output.
func RedactCommandPresentationText(value string, limit int) string {
	redacted, _, _ := boundedCommandPresentationText(value, limit)
	return redacted
}

func boundedCommandPresentationText(value string, limit int) (text string, sensitive, hasMore bool) {
	value = strings.TrimSpace(stripCommandPresentationANSI(value))
	if value == "" {
		return "", false, false
	}
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if commandPresentationLineSensitive(line) {
			lines[index] = "[REDACTED]"
			sensitive = true
		}
	}
	redacted := strings.Join(lines, "\n")
	if limit < 1 || utf8.RuneCountInString(redacted) <= limit {
		return redacted, sensitive, false
	}
	separator := "\n... output omitted ...\n"
	separatorRunes := utf8.RuneCountInString(separator)
	tailRunes := limit / 3
	headRunes := limit - separatorRunes - tailRunes
	if headRunes < 1 {
		runes := []rune(redacted)
		return string(runes[:limit]), sensitive, true
	}
	runes := []rune(redacted)
	return string(runes[:headRunes]) + separator + string(runes[len(runes)-tailRunes:]), sensitive, true
}

func stripCommandPresentationANSI(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for index := 0; index < len(value); {
		if value[index] != 0x1b || index+1 >= len(value) || value[index+1] != '[' {
			out.WriteByte(value[index])
			index++
			continue
		}
		index += 2
		for index < len(value) {
			current := value[index]
			index++
			if current >= 0x40 && current <= 0x7e {
				break
			}
		}
	}
	return out.String()
}

func commandPresentationLineSensitive(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{"token=", "token:", "user code:", "verification code:"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	normalized := strings.ToLower(strings.NewReplacer(" ", "", "-", "_", "\t", "").Replace(line))
	for _, marker := range []string{
		"password", "passwd", "api_key", "apikey", "access_token", "accesstoken", "refresh_token", "refreshtoken",
		"authorization", "client_secret", "clientsecret", "private_key", "cookie", "bearer",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
