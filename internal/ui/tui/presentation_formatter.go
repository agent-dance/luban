package tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
	gotui "github.com/grindlemire/go-tui"
	"mvdan.cc/sh/v3/syntax"
)

const (
	maxPresentationSummaryRunes    = 240
	maxPresentationDetailRunes     = 960
	maxPresentationDetailDiffRunes = 8000
	maxAgentPresentationCauseRunes = 240
)

type PresentationLifecycleState string

const (
	PresentationLifecycleSpawning  PresentationLifecycleState = "spawning"
	PresentationLifecycleQueued    PresentationLifecycleState = "queued"
	PresentationLifecycleRunning   PresentationLifecycleState = "running"
	PresentationLifecycleWaiting   PresentationLifecycleState = "waiting"
	PresentationLifecycleRetrying  PresentationLifecycleState = "retrying"
	PresentationLifecycleBlocked   PresentationLifecycleState = "blocked"
	PresentationLifecycleCompleted PresentationLifecycleState = "completed"
	PresentationLifecycleFailed    PresentationLifecycleState = "failed"
	PresentationLifecycleCancelled PresentationLifecycleState = "cancelled"
)

type FormattedPresentation struct {
	Lifecycle             PresentationLifecycleState   `json:"lifecycle"`
	Language              string                       `json:"language,omitempty"`
	Family                CommandFamily                `json:"family"`
	Outcome               ObservationOutcome           `json:"outcome"`
	Summary               string                       `json:"summary"`
	DetailLines           []string                     `json:"detail_lines,omitempty"`
	DetailDiff            string                       `json:"detail_diff,omitempty"`
	Object                string                       `json:"object,omitempty"`
	AggregationIntent     string                       `json:"aggregation_intent,omitempty"`
	Risk                  PresentationRisk             `json:"risk,omitempty"`
	SideEffect            bool                         `json:"side_effect,omitempty"`
	NeedsReview           bool                         `json:"needs_review,omitempty"`
	PlanGate              bool                         `json:"plan_gate,omitempty"`
	TerminalAgent         bool                         `json:"terminal_agent,omitempty"`
	RequiresDecision      bool                         `json:"requires_decision,omitempty"`
	Warning               bool                         `json:"warning,omitempty"`
	Retrying              bool                         `json:"retrying,omitempty"`
	Stalled               bool                         `json:"stalled,omitempty"`
	ScopeExpanded         bool                         `json:"scope_expanded,omitempty"`
	Truncated             bool                         `json:"truncated,omitempty"`
	Background            bool                         `json:"background,omitempty"`
	Sensitive             bool                         `json:"sensitive,omitempty"`
	HasEvidence           bool                         `json:"has_evidence,omitempty"`
	HasUsefulDetail       bool                         `json:"has_useful_detail,omitempty"`
	Completeness          types.ToolResultCompleteness `json:"completeness,omitempty"`
	FullEvidenceAvailable bool                         `json:"full_evidence_available,omitempty"`
	// HasMore reports that the row has additional detail or evidence.
	HasMore bool `json:"has_more,omitempty"`
}

func (f FormattedPresentation) Facts(outcome ObservationOutcome) PresentationFacts {
	if f.Outcome != OutcomeUnknown {
		outcome = f.Outcome
	}
	return PresentationFacts{
		Family: f.Family, Outcome: outcome, Risk: f.Risk, HasEvidence: f.HasEvidence || f.HasMore,
		SideEffect: f.SideEffect, NeedsReview: f.NeedsReview, PlanGate: f.PlanGate, TerminalAgentResult: f.TerminalAgent,
		RequiresDecision: f.RequiresDecision,
		Warning:          f.Warning, Retrying: f.Retrying, Stalled: f.Stalled, Truncated: f.Truncated,
		ScopeExpanded: f.ScopeExpanded,
		Background:    f.Background, Sensitive: f.Sensitive,
	}
}

var staticToolFamilies = map[string]CommandFamily{
	"Bash": FamilyShell, "PowerShell": FamilyShell,
	"Read":  FamilyFileRead,
	"Write": FamilyFileWrite, "Edit": FamilyFileWrite, "NotebookEdit": FamilyFileWrite,
	"Glob": FamilySearch, "Grep": FamilySearch, "LSP": FamilySearch, "ToolSearch": FamilySearch,
	"WebFetch": FamilyWeb, "WebSearch": FamilyWeb,
	"ListMcpResourcesTool": FamilyMCP, "ReadMcpResourceTool": FamilyMCP,
	"Agent":      FamilyAgent,
	"TaskCreate": FamilyTask, "TaskList": FamilyTask, "TaskUpdate": FamilyTask, "TaskGet": FamilyTask,
	"TaskStop": FamilyTask, "TaskOutput": FamilyTask,
	"GetGoal": FamilyGoal, "CreateGoal": FamilyGoal, "UpdateGoal": FamilyGoal,
	"EnterPlanMode": FamilyDecision, "ExitPlanMode": FamilyDecision, "AskUserQuestion": FamilyDecision,
	"SendUserMessage": FamilyMessage, "SendMessage": FamilyMessage,
	"TeamCreate": FamilyTeam, "TeamDelete": FamilyTeam,
	"CronCreate": FamilyCron, "CronDelete": FamilyCron, "CronList": FamilyCron,
	"EnterWorktree": FamilyWorktree, "ExitWorktree": FamilyWorktree,
	"Config":        FamilyConfig,
	"Skill":         FamilySkill,
	"RemoteTrigger": FamilyRemote,
}

var semanticToolActionKeys = map[string]i18n.Key{
	"Bash": i18n.KeyToolActionRunCommand, "PowerShell": i18n.KeyToolActionRunCommand,
	"Read":  i18n.KeyToolActionReadFile,
	"Write": i18n.KeyToolActionCreateFile,
	"Edit":  i18n.KeyToolActionUpdateFile, "NotebookEdit": i18n.KeyToolActionEditNotebook,
	"Glob": i18n.KeyToolActionFindFiles, "Grep": i18n.KeyToolActionSearchText, "LSP": i18n.KeyToolActionInspectCode, "ToolSearch": i18n.KeyToolActionFindTools,
	"WebFetch": i18n.KeyToolActionFetchWeb, "WebSearch": i18n.KeyToolActionSearchWeb,
	"ListMcpResourcesTool": i18n.KeyToolActionListMCPResources,
	"ReadMcpResourceTool":  i18n.KeyToolActionReadMCPResource,
	"Agent":                i18n.KeyToolActionRunAgent,
	"TaskCreate":           i18n.KeyToolActionCreateTask, "TaskList": i18n.KeyToolActionListTasks,
	"TaskUpdate": i18n.KeyToolActionUpdateTask, "TaskGet": i18n.KeyToolActionGetTask,
	"TaskStop":   i18n.KeyToolActionStopTask,
	"TaskOutput": i18n.KeyToolActionReadTaskOutput,
	"GetGoal":    i18n.KeyToolActionGetGoal, "CreateGoal": i18n.KeyToolActionCreateGoal, "UpdateGoal": i18n.KeyToolActionUpdateGoal,
	"EnterPlanMode": i18n.KeyToolActionEnterPlanMode, "ExitPlanMode": i18n.KeyToolActionExitPlanMode,
	"AskUserQuestion": i18n.KeyToolActionAskUser,
	"SendUserMessage": i18n.KeyToolActionSendUserMessage,
	"SendMessage":     i18n.KeyToolActionSendMessage,
	"TeamCreate":      i18n.KeyToolActionCreateTeam, "TeamDelete": i18n.KeyToolActionDeleteTeam,
	"CronCreate": i18n.KeyToolActionCreateSchedule, "CronDelete": i18n.KeyToolActionDeleteSchedule, "CronList": i18n.KeyToolActionListSchedules,
	"EnterWorktree": i18n.KeyToolActionEnterWorktree, "ExitWorktree": i18n.KeyToolActionExitWorktree,
	"Config": i18n.KeyToolActionConfigure,
	"Skill":  i18n.KeyToolActionLoadSkill, "RemoteTrigger": i18n.KeyToolActionRemoteRequest,
}

func semanticToolActionInLanguage(lang i18n.Language, toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if key, ok := semanticToolActionKeys[toolName]; ok {
		return i18n.Text(lang, key)
	}
	lower := strings.ToLower(toolName)
	if strings.HasPrefix(lower, "mcp__") || strings.HasPrefix(lower, "server_tool_") {
		return i18n.Text(lang, i18n.KeyToolActionUseMCPTool)
	}
	if toolName != "" {
		return toolName
	}
	return i18n.Text(lang, i18n.KeyPresentationFallbackTool)
}

// SemanticToolActionInLanguage exposes the same user-facing action vocabulary
// to append-only and assistive renderers.
func SemanticToolActionInLanguage(lang i18n.Language, toolName string) string {
	return semanticToolActionInLanguage(lang, toolName)
}

func CommandFamilyForTool(name string) CommandFamily {
	name = strings.TrimSpace(name)
	if family, ok := staticToolFamilies[name]; ok {
		return family
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "mcp__") || strings.HasPrefix(lower, "server_tool_") {
		return FamilyMCP
	}
	return FamilyUnknown
}

func FormatToolPresentation(toolName string, input map[string]any, outcome ObservationOutcome, result *types.ToolResultBlock) FormattedPresentation {
	return FormatToolPresentationInLanguage(i18n.DetectOrLoadLanguage(), toolName, input, outcome, result)
}

func FormatToolPresentationInLanguage(lang i18n.Language, toolName string, input map[string]any, outcome ObservationOutcome, result *types.ToolResultBlock) FormattedPresentation {
	family := CommandFamilyForTool(toolName)
	data := map[string]any{}
	metadata := map[string]string{}
	content := ""
	hasEvidence := false
	if result != nil {
		data = structuredPresentationData(toolName, result)
		metadata = cloneStringMap(result.Metadata)
		content = result.TextContent()
		hasEvidence = content != "" || result.Data != nil || len(result.ContentBlocks) > 0 || len(result.Metadata) > 0 || result.Usage != nil
	}
	data["_presentationResultBytes"] = len(content)
	data["_presentationResultLines"] = presentationLineCount(content)
	data["_presentationToolReferences"] = toolReferenceCount(result)
	effectiveOutcome := promotedPresentationOutcome(outcome, result, data, metadata)

	formatted := FormattedPresentation{
		Lifecycle: PresentationLifecycleRunning,
		Language:  lang.Code(), Family: family, Outcome: effectiveOutcome, Risk: RiskLow, HasEvidence: hasEvidence, HasMore: hasEvidence,
	}
	if result != nil {
		formatted.Completeness = result.Completeness.Clone()
	}
	formatted.Sensitive = presentationValueHasSensitiveKey(input) || presentationValueHasSensitiveKey(data) ||
		presentationMetadataHasSensitiveKey(metadata) || presentationValueHasSensitiveLocator(input) || presentationValueHasSensitiveLocator(data)
	formatted.Object = presentationObjectInLanguage(lang, toolName, input, data)
	formatted.AggregationIntent = presentationAggregationIntent(family, toolName, input)
	applyPresentationSemantics(&formatted, toolName, input, metadata, data, effectiveOutcome)
	if effectiveOutcome != outcome {
		formatted.Warning = true
	}
	formatted.Warning = formatted.Warning || presentationStructuredWarning(data, metadata)
	formatted.Retrying = metadataBool(metadata, "retrying") || metadataBool(metadata, "retry")
	formatted.Stalled = metadataBool(metadata, "stalled")
	formatted.ScopeExpanded = metadataBool(metadata, "scope_expanded") || metadataBool(metadata, "scopeExpanded") || presentationBool(data, "scope_expanded", "scopeExpanded")
	if presentationResultUsesDisplayPreview(lang, content, effectiveOutcome, family) {
		formatted.Completeness = formatted.Completeness.WithDisplayPreview()
	}
	formatted.Truncated = formatted.Completeness.RetainedResultIncomplete()
	if formatted.Truncated {
		formatted.Warning = true
	}
	formatted.Lifecycle = presentationLifecycleFor(effectiveOutcome, result != nil, formatted.Retrying)

	summaryProjection := formatted
	summaryProjection.Object = compactDefaultPresentationLocator(summaryProjection.Object)
	formatted.Summary = formatPresentationSummary(lang, toolName, input, effectiveOutcome, data, metadata, summaryProjection)
	formatted.Summary = normalizeRoutineSuccessSummary(lang, effectiveOutcome, formatted.Summary)
	formatted.DetailLines = formatPresentationDetails(lang, input, effectiveOutcome, data, metadata, content, formatted)
	formatted.DetailDiff = formatPresentationDetailDiff(lang, data, formatted.Family)
	refreshCompletenessDetails(lang, &formatted)
	formatted.HasUsefulDetail = presentationHasUsefulDetail(toolName, result, data, formatted)
	if formatted.Family == FamilyAgent {
		formatted.HasMore = false
		formatted.HasUsefulDetail = false
	}
	formatted.Summary = truncatePresentationRunes(strings.Join(strings.Fields(formatted.Summary), " "), maxPresentationSummaryRunes)
	if formatted.Summary == "" {
		formatted.Summary = strings.TrimSpace(semanticToolActionInLanguage(lang, toolName) + " " + observationOutcomeLabelInLanguage(lang, effectiveOutcome))
	}
	return formatted
}

func normalizeRoutineSuccessSummary(lang i18n.Language, outcome ObservationOutcome, summary string) string {
	if outcome != OutcomeSucceeded {
		return summary
	}
	parts := strings.Split(summary, " · ")
	if len(parts) <= 2 {
		return summary
	}
	completed := observationOutcomeLabelInLanguage(lang, OutcomeSucceeded)
	out := parts[:0]
	removed := false
	for _, part := range parts {
		if !removed && strings.TrimSpace(part) == completed {
			removed = true
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, " · ")
}

func presentationLifecycleFor(outcome ObservationOutcome, hasResult, retrying bool) PresentationLifecycleState {
	if retrying {
		return PresentationLifecycleRetrying
	}
	if !hasResult || outcome == OutcomeRunning || outcome == OutcomeUnknown {
		return PresentationLifecycleRunning
	}
	switch outcome {
	case OutcomeSucceeded:
		return PresentationLifecycleCompleted
	case OutcomeCancelled, OutcomeShutdown:
		return PresentationLifecycleCancelled
	default:
		return PresentationLifecycleFailed
	}
}

func presentationHasUsefulDetail(toolName string, result *types.ToolResultBlock, data map[string]any, formatted FormattedPresentation) bool {
	if result == nil || formatted.Family == FamilyAgent {
		return false
	}
	if formatted.Outcome != OutcomeSucceeded || formatted.Warning || formatted.Retrying || formatted.Stalled || formatted.Truncated || formatted.ScopeExpanded ||
		formatted.SideEffect || formatted.NeedsReview || formatted.RequiresDecision {
		return true
	}
	if toolName == "ListMcpResourcesTool" && presentationArrayLen(firstPresentationValue(data, "resources")) == 0 {
		return false
	}
	if toolName == "TaskList" && presentationArrayLen(firstPresentationValue(data, "tasks")) == 0 {
		return false
	}
	if toolName == "CronList" && presentationArrayLen(firstPresentationValue(data, "jobs")) == 0 {
		return false
	}
	if len(result.ContentBlocks) > 0 || strings.Count(strings.TrimSpace(result.TextContent()), "\n") > 0 || len([]rune(strings.TrimSpace(result.TextContent()))) > 160 {
		return true
	}
	switch value := result.Data.(type) {
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return strings.TrimSpace(result.TextContent()) != ""
	}
}

func applyPresentationSemantics(formatted *FormattedPresentation, toolName string, input map[string]any, metadata map[string]string, data map[string]any, outcome ObservationOutcome) {
	switch formatted.Family {
	case FamilyShell:
		applyShellPresentationSemantics(formatted, toolName, input, metadata, data)
	case FamilyFileWrite:
		formatted.Risk = RiskMedium
		formatted.SideEffect = true
	case FamilyAgent:
		formatted.Risk = RiskMedium
		formatted.NeedsReview = outcome == OutcomeSucceeded
		kind := strings.ToLower(firstPresentationString(data, "kind", "status"))
		formatted.Background = kind == "partial" || kind == "async_launched" || presentationBool(data, "isAsync")
		if formatted.Background {
			formatted.NeedsReview = false
		}
		formatted.TerminalAgent = outcome != OutcomeRunning && outcome != OutcomeUnknown && !formatted.Background
	case FamilyTask:
		if toolName != "TaskList" && toolName != "TaskGet" && toolName != "TaskOutput" {
			formatted.Risk = RiskMedium
			formatted.SideEffect = true
		}
	case FamilyGoal:
		if toolName != "GetGoal" {
			formatted.Risk = RiskMedium
			formatted.SideEffect = true
			formatted.NeedsReview = true
		}
	case FamilyDecision:
		formatted.Risk = RiskHigh
		formatted.RequiresDecision = toolName == "AskUserQuestion" || toolName == "ExitPlanMode"
		formatted.SideEffect = toolName == "EnterPlanMode" || toolName == "ExitPlanMode"
		formatted.PlanGate = toolName == "ExitPlanMode"
	case FamilyMessage, FamilyTeam, FamilyWorktree:
		formatted.Risk = RiskMedium
		formatted.SideEffect = true
	case FamilyCron:
		if toolName != "CronList" {
			formatted.Risk = RiskMedium
			formatted.SideEffect = true
		}
	case FamilyConfig:
		action := strings.ToLower(firstNonEmptyString(presentationString(input["action"]), mapPresenceAction(input, "value")))
		if action == "set" {
			formatted.Risk = RiskMedium
			formatted.SideEffect = true
		}
	case FamilyRemote:
		action := strings.ToLower(presentationString(input["action"]))
		if action != "list" && action != "get" {
			formatted.Risk = RiskMedium
			formatted.SideEffect = true
		}
	case FamilySkill:
		formatted.Risk = RiskMedium
	case FamilyMCP:
		formatted.Risk = RiskMedium
		formatted.SideEffect = true
		if toolName == "ListMcpResourcesTool" || toolName == "ReadMcpResourceTool" || metadataBool(metadata, "read_only") || metadataFalse(metadata, "side_effect") {
			formatted.Risk = RiskLow
			formatted.SideEffect = false
		}
	}
	if toolName == "TeamDelete" || (toolName == "ExitWorktree" && strings.EqualFold(presentationString(input["action"]), "remove")) {
		formatted.Risk = RiskDestructive
	}
	if metadataBool(metadata, "side_effect") || presentationBool(data, "side_effect", "sideEffect") {
		formatted.SideEffect = true
		if formatted.Risk == RiskLow {
			formatted.Risk = RiskMedium
		}
	}
	if metadataBool(metadata, "needs_review") || presentationBool(data, "needs_review", "needsReview") {
		formatted.NeedsReview = true
	}
	if metadataBool(metadata, "destructive") || presentationBool(data, "destructive") {
		formatted.Risk = RiskDestructive
	}
	_ = input
}

func applyShellPresentationSemantics(formatted *FormattedPresentation, toolName string, input map[string]any, metadata map[string]string, data map[string]any) {
	semantic := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		metadata["semanticCategory"], metadata["semantic_category"],
		firstPresentationString(data, "semanticCategory", "semantic_category"),
	)))
	switch semantic {
	case "read", "process":
		formatted.Risk = RiskLow
	case "write":
		formatted.Risk = RiskMedium
		formatted.SideEffect = true
	case "network":
		// Network access still carries execution/sandbox risk, but an entirely
		// literal HTTP GET/HEAD transcript does not itself prove a state change.
		// This is a display-only refinement: execution classification and
		// permission metadata remain untouched.
		formatted.Risk = RiskMedium
		formatted.SideEffect = toolName != "Bash" || !isReadOnlyHTTPPresentationCommand(presentationString(input["command"]))
	case "destructive":
		formatted.Risk = RiskDestructive
		formatted.SideEffect = true
	case "unknown":
		formatted.Risk = RiskUnknown
	}
	if warning := strings.TrimSpace(firstNonEmptyString(metadata["destructiveWarning"], metadata["destructive_warning"])); warning != "" {
		formatted.Risk = RiskDestructive
		formatted.SideEffect = true
		formatted.Warning = true
	}
	if strings.TrimSpace(firstNonEmptyString(metadata["securityWarn"], metadata["security_warning"])) != "" {
		formatted.Warning = true
	}
	formatted.Background = presentationBool(input, "run_in_background", "runInBackground") ||
		presentationBool(data, "background", "isBackground", "run_in_background", "runInBackground")
}

// isReadOnlyHTTPPresentationCommand recognizes the narrow HTTP read shape that
// is safe to describe as non-mutating in the transcript. It deliberately does
// not feed back into Bash permissions, sandboxing, or execution semantics.
func isReadOnlyHTTPPresentationCommand(command string) bool {
	if strings.TrimSpace(command) == "" {
		return false
	}
	program, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(command), "")
	if err != nil {
		return false
	}
	sawHTTPRead := false
	safe := true
	syntax.Walk(program, func(node syntax.Node) bool {
		if !safe {
			return false
		}
		switch typed := node.(type) {
		case *syntax.CallExpr:
			name, args, literal := presentationShellCall(typed)
			if !literal {
				safe = false
				return false
			}
			switch name {
			case "curl":
				if !presentationCurlIsHTTPRead(args) {
					safe = false
					return false
				}
				sawHTTPRead = true
			case "wget":
				if !presentationWgetIsHTTPRead(args) {
					safe = false
					return false
				}
				sawHTTPRead = true
			default:
				if !presentationShellReadConsumer(name, args) {
					safe = false
					return false
				}
			}
		case *syntax.Stmt:
			for _, redirect := range typed.Redirs {
				switch redirect.Op {
				case syntax.RdrOut, syntax.AppOut, syntax.RdrAll:
					target, literal := presentationLiteralShellWord(redirect.Word)
					if !literal || target != "/dev/null" && target != "/dev/stdout" && target != "/dev/stderr" {
						safe = false
						return false
					}
				}
			}
		}
		return true
	})
	return safe && sawHTTPRead
}

func presentationShellCall(call *syntax.CallExpr) (string, []string, bool) {
	if call == nil || len(call.Args) == 0 || len(call.Assigns) > 0 {
		return "", nil, false
	}
	words := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		literal, ok := presentationLiteralShellWord(word)
		if !ok || literal == "" {
			return "", nil, false
		}
		words = append(words, literal)
	}
	name := words[0]
	if slash := strings.LastIndexAny(name, `/\\`); slash >= 0 {
		name = name[slash+1:]
	}
	return strings.ToLower(name), words[1:], true
}

func presentationLiteralShellWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	var builder strings.Builder
	for _, part := range word.Parts {
		switch typed := part.(type) {
		case *syntax.Lit:
			builder.WriteString(typed.Value)
		case *syntax.SglQuoted:
			builder.WriteString(typed.Value)
		case *syntax.DblQuoted:
			for _, quoted := range typed.Parts {
				literal, ok := quoted.(*syntax.Lit)
				if !ok {
					return "", false
				}
				builder.WriteString(literal.Value)
			}
		default:
			return "", false
		}
	}
	return builder.String(), true
}

func presentationCurlIsHTTPRead(args []string) bool {
	method := "GET"
	hasData := false
	forceGet := false
	unsafeMethodSeen := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-X" || arg == "--request":
			if index+1 >= len(args) {
				return false
			}
			index++
			method = strings.ToUpper(strings.TrimSpace(args[index]))
			unsafeMethodSeen = unsafeMethodSeen || method != "GET" && method != "HEAD"
		case strings.HasPrefix(arg, "-X") && len(arg) > 2:
			method = strings.ToUpper(strings.TrimSpace(arg[2:]))
			unsafeMethodSeen = unsafeMethodSeen || method != "GET" && method != "HEAD"
		case strings.HasPrefix(arg, "--request="):
			method = strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(arg, "--request=")))
			unsafeMethodSeen = unsafeMethodSeen || method != "GET" && method != "HEAD"
		case arg == "-I" || arg == "--head" || presentationCombinedCurlFlag(arg, 'I'):
			method = "HEAD"
		case arg == "-G" || arg == "--get" || presentationCombinedCurlFlag(arg, 'G'):
			forceGet = true
		case presentationCurlDataFlag(arg):
			hasData = true
		}
	}
	if forceGet && method == "GET" {
		method = "GET"
	} else if hasData && method == "GET" {
		return false
	}
	if unsafeMethodSeen || method != "GET" && method != "HEAD" {
		return false
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if presentationCurlMutatesOrHidesIntent(arg) || presentationCurlMethodOverrideHeader(arg, args, &index) {
			return false
		}
	}
	return presentationHasLiteralHTTPURL(args)
}

func presentationCurlDataFlag(arg string) bool {
	if strings.HasPrefix(arg, "--data") || strings.HasPrefix(arg, "--json") || strings.HasPrefix(arg, "--form") {
		return true
	}
	return presentationCombinedCurlFlag(arg, 'd') || presentationCombinedCurlFlag(arg, 'F')
}

func presentationCurlMutatesOrHidesIntent(arg string) bool {
	for _, flag := range []string{
		"--upload-file", "--output", "--remote-name", "--remote-name-all", "--remote-header-name",
		"--output-dir", "--create-dirs", "--cookie-jar", "--dump-header", "--trace", "--trace-ascii",
		"--trace-config", "--etag-save", "--write-out", "--config", "--next", "--quote", "--proto-default",
	} {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	for _, flag := range []byte{'T', 'o', 'O', 'c', 'D', 'K', 'w', 'J', 'Q'} {
		if presentationCombinedCurlFlag(arg, flag) {
			return true
		}
	}
	return false
}

func presentationCombinedCurlFlag(arg string, wanted byte) bool {
	if len(arg) < 2 || arg[0] != '-' || strings.HasPrefix(arg, "--") {
		return false
	}
	for index := 1; index < len(arg); index++ {
		flag := arg[index]
		if flag == wanted {
			return true
		}
		if strings.ContainsRune("AbcdeEFHKoPQrTtuwxXYZz", rune(flag)) {
			// The remainder is the value of this option, not more flags.
			return false
		}
	}
	return false
}

func presentationCurlMethodOverrideHeader(arg string, args []string, index *int) bool {
	value := ""
	switch {
	case arg == "-H" || arg == "--header":
		if *index+1 >= len(args) {
			return true
		}
		*index++
		value = args[*index]
	case strings.HasPrefix(arg, "-H") && len(arg) > 2:
		value = arg[2:]
	case strings.HasPrefix(arg, "--header="):
		value = strings.TrimPrefix(arg, "--header=")
	}
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "x-http-method-override:") || strings.HasPrefix(lower, "x-method-override:")
}

func presentationWgetIsHTTPRead(args []string) bool {
	stdoutOnly := false
	spider := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--spider":
			spider = true
		case arg == "-O" || arg == "--output-document":
			if index+1 >= len(args) || args[index+1] != "-" {
				return false
			}
			index++
			stdoutOnly = true
		case arg == "--output-document=-" || strings.Contains(arg, "O-") && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--"):
			stdoutOnly = true
		case strings.HasPrefix(arg, "--post-") || strings.HasPrefix(arg, "--body-") || strings.HasPrefix(arg, "--method=") ||
			arg == "--method" || arg == "-i" || arg == "--input-file" || strings.HasPrefix(arg, "--input-file="):
			return false
		}
	}
	return (stdoutOnly || spider) && presentationHasLiteralHTTPURL(args)
}

func presentationHasLiteralHTTPURL(args []string) bool {
	for _, arg := range args {
		lower := strings.ToLower(strings.TrimSpace(arg))
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return true
		}
		if strings.Contains(lower, "://") || strings.HasPrefix(lower, "-") || strings.ContainsAny(lower, " \t") {
			continue
		}
		// curl defaults scheme-less hostnames to HTTP. Require a host-like
		// literal so option values and local paths do not qualify accidentally.
		host := lower
		if slash := strings.IndexByte(host, '/'); slash >= 0 {
			host = host[:slash]
		}
		if strings.Contains(host, ".") || host == "localhost" || strings.HasPrefix(host, "localhost:") {
			return true
		}
	}
	return false
}

func presentationShellReadConsumer(name string, args []string) bool {
	switch name {
	case "head", "tail", "wc", "cut", "tr", "uniq", "column", "fold", "grep", "egrep", "fgrep", "rg", "jq", "yq":
		return true
	case "sed":
		for _, arg := range args {
			if arg == "--in-place" || strings.HasPrefix(arg, "--in-place=") || strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg, 'i') {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func formatPresentationSummary(lang i18n.Language, toolName string, input map[string]any, outcome ObservationOutcome, data map[string]any, metadata map[string]string, formatted FormattedPresentation) string {
	state := observationOutcomeLabelInLanguage(lang, outcome)
	actionLabel := semanticToolActionInLanguage(lang, toolName)
	switch formatted.Family {
	case FamilyFileRead:
		file := presentationNestedMap(data, "file")
		parts := []string{i18n.Text(lang, i18n.KeyPresentationAggregateRead), firstNonEmptyString(formatted.Object, firstPresentationString(file, "filePath"))}
		if variant := firstPresentationString(data, "type"); variant != "" && variant != "text" {
			parts = append(parts, variant)
		}
		if count, ok := presentationIntFrom([]map[string]any{file, data}, "numLines", "line_count", "lineCount"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationLines, count))
			if start, startOK := presentationInt(file, "startLine"); startOK {
				if total, totalOK := presentationInt(file, "totalLines"); totalOK {
					end := start + count - 1
					if count == 0 {
						end = start
					}
					parts = append(parts, i18n.Format(lang, i18n.KeyPresentationWindow, start, end, total))
				}
			}
		}
		if bytes, ok := presentationIntFrom([]map[string]any{file, data}, "originalSize", "byte_count", "byteCount", "bytes"); ok {
			parts = append(parts, formatPresentationBytes(bytes))
		}
		if dimensions := presentationNestedMap(file, "dimensions"); len(dimensions) > 0 {
			width, widthOK := presentationInt(dimensions, "originalWidth")
			height, heightOK := presentationInt(dimensions, "originalHeight")
			if widthOK && heightOK {
				parts = append(parts, fmt.Sprintf("%dx%d", width, height))
			}
		}
		if count, ok := presentationInt(file, "count"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationParts, count))
		}
		if outcome != OutcomeSucceeded && outcome != OutcomeRunning {
			parts = append(parts, state)
		}
		return joinPresentationParts(parts)
	case FamilyFileWrite:
		verb := i18n.Text(lang, i18n.KeyPresentationUpdated)
		operation := strings.ToLower(firstPresentationString(data, "type", "edit_mode", "editMode"))
		if operation == "create" || presentationBool(data, "created") || toolName == "Write" && presentationBool(data, "new_file", "newFile") {
			verb = i18n.Text(lang, i18n.KeyPresentationCreated)
		} else if toolName == "NotebookEdit" && operation != "" {
			verb = notebookEditVerb(lang, operation)
		} else if outcome != OutcomeSucceeded {
			verb = actionLabel + " " + state
		}
		parts := []string{verb, formatted.Object}
		if cell := firstPresentationString(data, "cell_id", "cellId"); cell != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationCell, cell))
		}
		if bytes, ok := presentationInt(data, "bytes", "bytes_written", "bytesWritten", "byte_count", "byteCount"); ok {
			parts = append(parts, formatPresentationBytes(bytes))
		} else if content, ok := data["content"].(string); ok && content != "" {
			parts = append(parts, formatPresentationBytes(int64(len(content))))
		}
		if occurrences, ok := presentationInt(presentationNestedMap(data, "metadata"), "occurrences"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationReplacements, occurrences))
		}
		if toolName == "NotebookEdit" {
			parts = append(parts, firstPresentationString(data, "cell_type", "cellType"), firstPresentationString(data, "language"))
		}
		added, removed, haveDiff := presentationDiffStats(data)
		if haveDiff {
			parts = append(parts, fmt.Sprintf("+%d", added))
			parts = append(parts, fmt.Sprintf("-%d", removed))
		}
		return joinPresentationParts(parts)
	case FamilySearch:
		parts := []string{actionLabel}
		if toolName == "LSP" {
			parts = append(parts, presentationString(input["operation"]), formatted.Object)
			if line, ok := presentationInt(input, "line"); ok {
				character, _ := presentationInt(input, "character")
				parts = append(parts, fmt.Sprintf("%d:%d", line, character))
			}
		}
		if query := presentationQuery(input); query != "" {
			parts = append(parts, query)
		}
		countKeys := []string{"numMatches", "match_count", "matchCount", "result_count", "resultCount", "count"}
		if toolName == "ToolSearch" {
			countKeys = append(countKeys, "_presentationToolReferences")
		}
		if count, ok := presentationIntFrom([]map[string]any{data, stringMapAsAny(metadata)}, countKeys...); ok {
			label := i18n.Format(lang, i18n.KeyPresentationMatches, formatPresentationInt(count))
			if toolName == "ToolSearch" {
				if count == 0 && outcome == OutcomeSucceeded {
					label = i18n.Text(lang, i18n.KeyToolEmptyTools)
				} else {
					label = i18n.Format(lang, i18n.KeyPresentationTools, formatPresentationInt(count))
				}
			} else if count == 0 && outcome == OutcomeSucceeded {
				label = i18n.Text(lang, i18n.KeyToolEmptyMatches)
			}
			parts = append(parts, label)
		}
		if count, ok := presentationIntFrom([]map[string]any{data, stringMapAsAny(metadata)}, "numFiles", "file_count", "fileCount", "files"); ok {
			if count == 0 && outcome == OutcomeSucceeded {
				parts = append(parts, i18n.Text(lang, i18n.KeyToolEmptyFiles))
			} else {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationFiles, formatPresentationInt(count)))
			}
		}
		if formatted.Truncated {
			parts = append(parts, i18n.Text(lang, i18n.KeyPresentationTruncated))
		}
		if mode := firstPresentationString(data, "mode"); mode != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationMode, mode))
		}
		if limit, ok := presentationInt(data, "appliedLimit"); ok && limit > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationLimit, limit))
		}
		if offset, ok := presentationInt(data, "appliedOffset"); ok && offset > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationOffsetValue, offset))
		}
		if outcome != OutcomeSucceeded && outcome != OutcomeRunning {
			parts = append(parts, state)
		}
		return joinPresentationParts(parts)
	case FamilyShell:
		parts := []string{actionLabel}
		if command := presentationCommandInLanguage(lang, input); command != "" {
			parts = append(parts, command)
		}
		parts = append(parts, state)
		if code := firstNonEmptyString(firstPresentationString(data, "exitCode"), firstPresentationMetadata(metadata, "exit_code", "exit")); code != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationExit, code))
		}
		if taskID := firstPresentationString(data, "backgroundTaskId"); taskID != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationBackground, taskID))
		}
		if duration, ok := presentationDurationMs(data, metadata); ok {
			parts = append(parts, formatPresentationDuration(duration))
		}
		if latest := firstPresentationString(data, "latestToolUse", "latest_tool_use"); latest != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationLatest, latest))
		}
		if output := firstPresentationString(data, "outputFile", "output_file"); output != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationOutput, compactDefaultPresentationLocator(output)))
		}
		if sessionURL := firstPresentationString(data, "sessionUrl", "session_url"); sessionURL != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationSession, sanitizePresentationLocatorInLanguage(lang, sessionURL)))
		}
		return joinPresentationParts(parts)
	case FamilyAgent:
		agentID := firstPresentationString(data, "agentId", "agent_id", "taskId", "task_id")
		kind := firstPresentationString(data, "kind", "status")
		if kind == "" {
			kind = state
		}
		parts := []string{i18n.Text(lang, i18n.KeyPresentationAgent), agentID, kind}
		if role := firstPresentationString(data, "agentType", "agent_type"); role != "" {
			parts = append(parts, role)
		}
		if tools, ok := presentationInt(data, "totalToolUseCount", "toolUseCount", "tool_uses"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationTools, formatPresentationInt(tools)))
		}
		if tokens, ok := presentationInt(data, "totalTokens", "total_tokens"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationTokens, formatPresentationInt(tokens)))
		}
		if duration, ok := presentationDurationMs(data, metadata); ok {
			parts = append(parts, formatPresentationDuration(duration))
		}
		if presentationBool(data, "interrupted") {
			parts = append(parts, i18n.Text(lang, i18n.KeyPresentationInterrupted))
		}
		return joinPresentationParts(parts)
	case FamilyWeb:
		parts := []string{actionLabel}
		if object := formatted.Object; object != "" {
			parts = append(parts, object)
		}
		if status := firstPresentationString(data, "status", "status_code", "statusCode", "code"); status != "" {
			parts = append(parts, status)
			if statusText := firstPresentationString(data, "codeText", "statusText"); statusText != "" {
				parts = append(parts, statusText)
			}
		}
		if count, ok := presentationIntFrom([]map[string]any{data, stringMapAsAny(metadata)}, "results_count", "result_count", "resultCount", "source_count", "sourceCount"); ok {
			if count == 0 && outcome == OutcomeSucceeded {
				parts = append(parts, i18n.Text(lang, i18n.KeyToolEmptySources))
			} else {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationSources, formatPresentationInt(count)))
			}
		}
		if bytes, ok := presentationInt(data, "bytes"); ok {
			parts = append(parts, formatPresentationBytes(bytes))
		}
		if duration, ok := presentationDurationMs(data, metadata); ok {
			parts = append(parts, formatPresentationDuration(duration))
		}
		if method := firstPresentationMetadata(metadata, "method"); method != "" {
			parts = append(parts, method)
		}
		if outcome != OutcomeSucceeded && outcome != OutcomeRunning {
			parts = append(parts, state)
		}
		return joinPresentationParts(parts)
	case FamilyMCP:
		if toolName == "ListMcpResourcesTool" && outcome == OutcomeSucceeded {
			resources := presentationArrayLen(firstPresentationValue(data, "resources"))
			if count, ok := presentationIntFrom([]map[string]any{data, stringMapAsAny(metadata)}, "resource_count", "resourceCount", "resources_count", "resourcesCount"); ok {
				resources = int(count)
			}
			if resources == 0 {
				return joinPresentationParts([]string{i18n.Text(lang, i18n.KeyToolEmptyResources), formatted.Object})
			}
		}
		parts := []string{actionLabel, formatted.Object}
		if count, ok := presentationIntFrom([]map[string]any{data, stringMapAsAny(metadata)}, "resource_count", "resourceCount", "resources_count", "resourcesCount"); ok {
			if count == 0 && outcome == OutcomeSucceeded {
				parts = append(parts, i18n.Text(lang, i18n.KeyToolEmptyResources))
			} else {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationResources, formatPresentationInt(count)))
			}
		} else if resources := presentationArrayLen(firstPresentationValue(data, "resources")); resources > 0 || toolName == "ListMcpResourcesTool" {
			if resources == 0 && outcome == OutcomeSucceeded {
				parts = append(parts, i18n.Text(lang, i18n.KeyToolEmptyResources))
			} else if resources > 0 {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationResources, resources))
			}
		}
		if mediaType := firstNonEmptyString(firstPresentationString(data, "media_type", "mediaType", "mime_type", "mimeType"), firstPresentationMetadata(metadata, "media_type", "mime_type")); mediaType != "" {
			parts = append(parts, mediaType)
		}
		if contents := presentationArrayLen(firstPresentationValue(data, "contents")); contents > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationContents, contents))
			if first := firstPresentationArrayMap(firstPresentationValue(data, "contents")); len(first) > 0 {
				if mediaType := firstPresentationString(first, "mimeType", "mime_type"); mediaType != "" {
					parts = append(parts, mediaType)
				}
				if text := firstPresentationString(first, "text"); text != "" {
					parts = append(parts, formatPresentationBytes(int64(len(text))))
				}
				if blob := firstPresentationString(first, "blobSavedTo"); blob != "" {
					parts = append(parts, i18n.Format(lang, i18n.KeyPresentationBlob, blob))
				}
			}
		}
		if blocks := presentationArrayLen(firstPresentationValue(data, "content")); blocks > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationResultBlocks, blocks))
		}
		if bytes, ok := presentationIntFrom([]map[string]any{data, stringMapAsAny(metadata)}, "bytes", "byte_count", "byteCount", "size"); ok {
			parts = append(parts, formatPresentationBytes(bytes))
		}
		if status := firstPresentationString(data, "status", "connection_state", "connectionState"); status != "" && status != state {
			parts = append(parts, status)
		}
		if duration, ok := presentationDurationMs(data, metadata); ok {
			parts = append(parts, formatPresentationDuration(duration))
		}
		return joinPresentationParts(parts)
	case FamilyTask:
		return formatTaskPresentationSummary(lang, actionLabel, toolName, input, data, state, outcome)
	case FamilyGoal:
		return formatGoalPresentationSummary(lang, actionLabel, input, data, state, outcome)
	case FamilyDecision:
		parts := []string{actionLabel, state}
		if toolName == "AskUserQuestion" {
			questions := firstPresentationValue(data, "questions")
			if presentationArrayLen(questions) == 0 {
				questions = input["questions"]
			}
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationQuestions, presentationArrayLen(questions)))
			if choices := presentationQuestionChoiceCount(questions); choices > 0 {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationChoices, choices))
			}
			if answers := presentationMapLen(firstPresentationValue(data, "answers")); answers > 0 {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationAnswers, answers))
			}
		}
		if path := firstPresentationString(data, "filePath"); path != "" {
			parts = append(parts, compactDefaultPresentationLocator(path))
		}
		if presentationBool(data, "awaitingLeaderApproval") {
			parts = append(parts, i18n.Text(lang, i18n.KeyPresentationAwaitingApproval))
		}
		if edited, present := presentationBoolValue(data["planWasEdited"]); present {
			parts = append(parts, i18n.Format(lang, i18n.KeyRuntimePresentationFlagEdited, edited))
		}
		if isAgent, present := presentationBoolValue(data["isAgent"]); present {
			parts = append(parts, i18n.Format(lang, i18n.KeyRuntimePresentationFlagAgent, isAgent))
		}
		if gate := firstPresentationMetadata(metadata, "exitPlanModeStatus"); gate != "" {
			parts = append(parts, gate)
		}
		if requestID := firstPresentationString(data, "requestId", "request_id"); requestID != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationRequest, requestID))
		}
		return joinPresentationParts(parts)
	case FamilyMessage:
		parts := []string{actionLabel, state}
		if target := firstNonEmptyString(firstPresentationString(data, "target"), presentationString(input["to"])); target != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationTarget, sanitizePresentationLocatorInLanguage(lang, target)))
		}
		if recipients := presentationArrayLen(firstPresentationValue(data, "recipients")); recipients > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationRecipients, recipients))
		}
		if attachments := presentationArrayLen(firstPresentationValue(data, "attachments")); attachments > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationAttachments, attachments))
		}
		if requestID := firstPresentationString(data, "request_id", "requestId"); requestID != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationRequest, requestID))
		}
		if sentAt := firstPresentationString(data, "sentAt", "sent_at"); sentAt != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationSent, sentAt))
		}
		if messageStatus := firstPresentationMetadata(metadata, "messageStatus"); messageStatus != "" {
			parts = append(parts, messageStatus)
		}
		return joinPresentationParts(parts)
	case FamilyTeam:
		parts := []string{actionLabel, state}
		if team := firstNonEmptyString(firstPresentationString(data, "team_name", "teamName", "name"), presentationString(input["team_name"])); team != "" {
			parts = append(parts, team)
		}
		if teamID := firstPresentationString(data, "team_id", "teamId"); teamID != "" {
			parts = append(parts, teamID)
		}
		if lead := firstPresentationString(data, "lead_agent_id", "leadAgentId"); lead != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationLead, lead))
		}
		if file := firstPresentationString(data, "team_file_path", "teamFilePath"); file != "" {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationConfig, compactDefaultPresentationLocator(file)))
		}
		if members := presentationArrayLen(firstPresentationValue(data, "members", "agents")); members > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationMembers, members))
		}
		return joinPresentationParts(parts)
	case FamilyCron:
		parts := []string{actionLabel, state}
		if id := firstPresentationString(data, "id"); id != "" {
			parts = append(parts, id)
		}
		if schedule := presentationString(input["cron"]); schedule != "" {
			parts = append(parts, schedule)
		}
		if next := firstPresentationMetadata(metadata, "next_fire"); next != "" {
			if next == "unknown" || next == "(unknown)" {
				next = i18n.Text(lang, i18n.KeyPresentationCronNextUnknown)
			}
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationNext, next))
		}
		if timezone := firstPresentationMetadata(metadata, "tz"); timezone != "" {
			if timezone == "Local" {
				timezone = i18n.Text(lang, i18n.KeyPresentationCronTimezoneLocal)
			}
			parts = append(parts, timezone)
		}
		if jobs := presentationArrayLen(firstPresentationValue(data, "jobs")); toolName == "CronList" {
			if jobs == 0 && outcome == OutcomeSucceeded {
				return i18n.Text(lang, i18n.KeyToolEmptySchedules)
			} else {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationJobs, jobs))
			}
			recurring, durable := presentationScheduleJobCounts(firstPresentationValue(data, "jobs"))
			if recurring > 0 {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationRecurring, recurring))
			}
			if durable > 0 {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationDurable, durable))
			}
		}
		for _, flag := range []string{"recurring", "durable"} {
			if value, present := presentationBoolValue(data[flag]); present {
				key := i18n.KeyRuntimePresentationFlagRecurring
				if flag == "durable" {
					key = i18n.KeyRuntimePresentationFlagDurable
				}
				parts = append(parts, i18n.Format(lang, key, value))
			}
		}
		return joinPresentationParts(parts)
	case FamilyWorktree:
		parts := []string{actionLabel, state}
		if action := firstPresentationString(data, "action"); action != "" {
			parts = append(parts, action)
		}
		if path := firstPresentationString(data, "worktreePath", "originalCwd"); path != "" {
			parts = append(parts, compactDefaultPresentationLocator(path))
		}
		if branch := firstPresentationString(data, "worktreeBranch"); branch != "" {
			parts = append(parts, branch)
		}
		if files, ok := presentationInt(data, "discardedFiles"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationDiscardedFiles, files))
		}
		if commits, ok := presentationInt(data, "discardedCommits"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationDiscardedCommits, commits))
		}
		if tmux := firstPresentationString(data, "tmuxSessionName"); tmux != "" {
			parts = append(parts, "tmux "+tmux)
		}
		return joinPresentationParts(parts)
	case FamilyConfig:
		action := strings.ToLower(firstNonEmptyString(
			presentationString(input["action"]),
			firstPresentationString(data, "action"),
			mapPresenceAction(input, "value"),
		))
		displayAction := actionLabel
		if action == "get" {
			displayAction = i18n.Text(lang, i18n.KeyToolActionReadConfiguration)
		}
		return joinPresentationParts([]string{displayAction, state, action, firstNonEmptyString(presentationString(input["setting"]), presentationString(input["key"]), firstPresentationString(data, "setting", "key"))})
	case FamilySkill:
		return joinPresentationParts([]string{
			actionLabel,
			firstNonEmptyString(metadata["commandName"], presentationString(input["skill"])),
			firstNonEmptyString(metadata["status"], state),
			firstNonEmptyString(metadata["loadedFrom"], metadata["loaded_from"]),
			firstNonEmptyString(metadata["model"], metadata["reasoningEffort"]),
			firstNonEmptyString(metadata["permissionDecision"], metadata["permission_decision"]),
		})
	case FamilyRemote:
		parts := []string{actionLabel, presentationString(input["action"]), state}
		if status := firstPresentationString(data, "status"); status != "" {
			parts = append(parts, "HTTP "+status)
		}
		if id := presentationString(input["trigger_id"]); id != "" {
			parts = append(parts, id)
		}
		if payload := firstPresentationString(data, "json"); payload != "" {
			parts = append(parts, formatPresentationBytes(int64(len(payload))))
		}
		return joinPresentationParts(parts)
	case FamilyUnknown:
		return formatSafeFallbackSummary(lang, toolName, input, data, state)
	default:
		return formatSafeFallbackSummary(lang, toolName, input, data, state)
	}
}

func notebookEditVerb(lang i18n.Language, operation string) string {
	switch operation {
	case "insert":
		return i18n.Text(lang, i18n.KeyPresentationInsertedCell)
	case "delete":
		return i18n.Text(lang, i18n.KeyPresentationDeletedCell)
	case "replace":
		return i18n.Text(lang, i18n.KeyPresentationReplacedCell)
	default:
		return i18n.Text(lang, i18n.KeyPresentationUpdatedCell)
	}
}

func formatPresentationDetails(lang i18n.Language, input map[string]any, outcome ObservationOutcome, data map[string]any, metadata map[string]string, content string, formatted FormattedPresentation) []string {
	lines := make([]string, 0, 6)
	if formatted.Object != "" {
		lines = append(lines, i18n.Format(lang, i18n.KeyPresentationObjectValue, formatted.Object))
	}
	if isNonSuccessPresentationOutcome(outcome) {
		lines = append(lines, i18n.Format(lang, i18n.KeyPresentationOutcome, observationOutcomeLabelInLanguage(lang, outcome)))
	}
	if code := firstNonEmptyString(firstPresentationString(data, "exitCode"), firstPresentationMetadata(metadata, "exit_code", "exit")); code != "" {
		lines = append(lines, i18n.Format(lang, i18n.KeyPresentationProcessExit, code))
	}
	if duration, ok := presentationDurationMs(data, metadata); ok {
		lines = append(lines, i18n.Format(lang, i18n.KeyPresentationDuration, formatPresentationDuration(duration)))
	}
	if formatted.SideEffect {
		lines = append(lines, i18n.Text(lang, i18n.KeyPresentationImpact))
	}
	if formatted.NeedsReview {
		lines = append(lines, i18n.Text(lang, i18n.KeyPresentationReviewNext))
	}
	if transcript := firstPresentationString(data, "transcriptPath", "transcript_path"); transcript != "" {
		if formatted.Family != FamilyAgent {
			lines = append(lines, i18n.Format(lang, i18n.KeyPresentationTranscript, transcript))
		}
	}
	for _, path := range []string{
		firstPresentationString(data, "rawOutputPath", "raw_output_path"),
		firstPresentationString(data, "persistedOutputPath", "persisted_output_path"),
		firstPresentationString(data, "outputPath", "output_path", "outputFile", "output_file"),
		firstPresentationString(data, "team_file_path", "teamFilePath"),
	} {
		if path != "" && formatted.Family != FamilyAgent {
			lines = append(lines, i18n.Format(lang, i18n.KeyPresentationEvidenceReference, sanitizePresentationLocatorInLanguage(lang, path)))
		}
	}
	if isNonSuccessPresentationOutcome(outcome) && formatted.Family != FamilyUnknown {
		cause := firstPresentationString(data, "error", "message", "reason", "exitReason")
		if cause == "" && formatted.Family != FamilyAgent {
			cause = content
		}
		causeLimit := maxPresentationDetailRunes
		if formatted.Family == FamilyAgent {
			causeLimit = maxAgentPresentationCauseRunes
		}
		if preview := RedactPresentationTextInLanguage(lang, cause, causeLimit); preview != "" {
			lines = append(lines, i18n.Format(lang, i18n.KeyPresentationCause, preview))
		}
	}
	if outcome == OutcomeSucceeded && formatted.Family == FamilyAgent {
		if conclusion := agentPresentationResultText(data); conclusion != "" {
			lines = append(lines, i18n.Format(lang, i18n.KeyPresentationResultValue, conclusion))
		}
	} else if outcome == OutcomeSucceeded && content != "" && formatted.Family != FamilyUnknown {
		resultPrefixWidth := presentationDisplayWidth(i18n.Format(lang, i18n.KeyPresentationResultValue, ""))
		if formatted.Family == FamilyShell && presentationLooksLikeTerminalArt(content) {
			// Shell terminal art remains available through the retained evidence.
			// Projecting its cursor-oriented canvas into transcript prose produces
			// misleading borders and unstable wrapping, even after ANSI stripping.
		} else if preview := boundedPresentationResultPreviewInLanguage(lang, content, maxPresentationDetailRunes-resultPrefixWidth); preview != "" {
			lines = append(lines, i18n.Format(lang, i18n.KeyPresentationResultValue, preview))
		}
	}
	if formatted.Sensitive && formatted.Family != FamilyUnknown {
		redacted, _ := RedactPresentationValue(input).(map[string]any)
		if encoded, err := json.Marshal(redacted); err == nil {
			lines = append(lines, i18n.Format(lang, i18n.KeyPresentationInput, truncatePresentationRunes(string(encoded), maxPresentationDetailRunes)))
		}
	}
	resultPrefix := strings.TrimSpace(i18n.Format(lang, i18n.KeyPresentationResultValue, ""))
	for index := range lines {
		line := strings.TrimSpace(lines[index])
		if formatted.Family == FamilyAgent && resultPrefix != "" && strings.HasPrefix(line, resultPrefix) {
			// The Agent card is itself collapsible, so its typed conclusion is
			// kept intact and wrapped by the renderer instead of being previewed.
			lines[index] = line
			continue
		}
		lines[index] = truncatePresentationDisplayWidth(line, maxPresentationDetailRunes)
	}
	return lines
}

func presentationResultUsesDisplayPreview(lang i18n.Language, content string, outcome ObservationOutcome, family CommandFamily) bool {
	if outcome != OutcomeSucceeded || strings.TrimSpace(content) == "" || family == FamilyUnknown || family == FamilyAgent {
		return false
	}
	if family == FamilyShell && presentationLooksLikeTerminalArt(content) {
		return false
	}
	prefixWidth := presentationDisplayWidth(i18n.Format(lang, i18n.KeyPresentationResultValue, ""))
	return presentationDisplayWidth(RedactPresentationTextInLanguage(lang, content, 0)) > maxPresentationDetailRunes-prefixWidth
}

func appendCompletenessDetails(lang i18n.Language, lines []string, formatted FormattedPresentation) []string {
	switch formatted.Completeness.Source {
	case types.ToolResultCompletenessSourceTruncated:
		lines = append(lines, i18n.Text(lang, i18n.KeyPresentationSourceTruncatedWarning))
	case types.ToolResultCompletenessCaptureDropped:
		lines = append(lines, i18n.Text(lang, i18n.KeyPresentationCaptureDroppedWarning))
	}
	if formatted.Completeness.View == types.ToolResultCompletenessPagination {
		lines = append(lines, i18n.Text(lang, i18n.KeyPresentationPaginationWarning))
	} else if formatted.Completeness.View == types.ToolResultCompletenessDisplayPreview {
		key := i18n.KeyPresentationDisplayPreviewWarning
		if formatted.FullEvidenceAvailable {
			key = i18n.KeyPresentationDisplayPreviewEvidence
		}
		lines = append(lines, i18n.Text(lang, key))
	}
	return lines
}

func refreshCompletenessDetails(lang i18n.Language, formatted *FormattedPresentation) {
	if formatted == nil {
		return
	}
	known := map[string]struct{}{}
	for _, key := range []i18n.Key{
		i18n.KeyPresentationPaginationWarning,
		i18n.KeyPresentationSourceTruncatedWarning,
		i18n.KeyPresentationCaptureDroppedWarning,
		i18n.KeyPresentationDisplayPreviewWarning,
		i18n.KeyPresentationDisplayPreviewEvidence,
	} {
		known[i18n.Text(lang, key)] = struct{}{}
	}
	filtered := formatted.DetailLines[:0]
	for _, line := range formatted.DetailLines {
		if _, ok := known[line]; !ok {
			filtered = append(filtered, line)
		}
	}
	formatted.DetailLines = appendCompletenessDetails(lang, filtered, *formatted)
}

func agentPresentationResultText(data map[string]any) string {
	content := firstPresentationValue(data, "content")
	blocks, ok := content.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, value := range blocks {
		block := structuredPresentationMap(value)
		blockType := strings.ToLower(strings.TrimSpace(firstPresentationString(block, "type")))
		if blockType != "" && blockType != "text" {
			continue
		}
		if text := strings.TrimSpace(firstPresentationString(block, "text")); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return RedactPresentationText(strings.Join(parts, "\n"), 0)
}

func boundedPresentationResultPreviewInLanguage(lang i18n.Language, value string, limit int) string {
	redacted := RedactPresentationTextInLanguage(lang, value, 0)
	if limit < 1 || presentationDisplayWidth(redacted) <= limit {
		return redacted
	}
	separator := "\n…\n"
	separatorWidth := presentationDisplayWidth(separator)
	tailWidth := limit / 3
	headWidth := limit - separatorWidth - tailWidth
	if headWidth < 1 {
		return truncatePresentationDisplayWidth(redacted, limit)
	}
	return presentationDisplayPrefix(redacted, headWidth) + separator + presentationDisplaySuffix(redacted, tailWidth)
}

// sanitizePresentationTerminalText converts untrusted process output to plain
// transcript text. CSI/OSC/DCS-family escape strings and C0/C1 controls are
// removed before redaction and display-width bounding, so neither a cursor
// movement nor an invisible control byte can distort the TUI.
func sanitizePresentationTerminalText(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for index := 0; index < len(value); {
		current := value[index]
		switch current {
		case 0x1b:
			index = skipPresentationEscape(value, index)
			continue
		case 0x9b:
			index = skipPresentationCSI(value, index+1)
			continue
		case 0x90, 0x98, 0x9d, 0x9e, 0x9f:
			index = skipPresentationControlString(value, index+1)
			continue
		case '\n':
			out.WriteByte('\n')
			index++
			continue
		case '\t':
			out.WriteByte(' ')
			index++
			continue
		}
		if current < 0x20 || current == 0x7f {
			index++
			continue
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		if r == utf8.RuneError && size == 1 {
			index++
			continue
		}
		if r >= 0x80 && r <= 0x9f {
			switch r {
			case 0x9b:
				index = skipPresentationCSI(value, index+size)
			case 0x90, 0x98, 0x9d, 0x9e, 0x9f:
				index = skipPresentationControlString(value, index+size)
			default:
				index += size
			}
			continue
		}
		if unicode.IsControl(r) {
			index += size
			continue
		}
		out.WriteRune(r)
		index += size
	}
	return out.String()
}

func skipPresentationEscape(value string, index int) int {
	index++
	if index >= len(value) {
		return index
	}
	switch value[index] {
	case '[':
		return skipPresentationCSI(value, index+1)
	case ']', 'P', 'X', '^', '_':
		return skipPresentationControlString(value, index+1)
	default:
		return index + 1
	}
}

func skipPresentationCSI(value string, index int) int {
	for index < len(value) {
		current := value[index]
		index++
		if current >= 0x40 && current <= 0x7e {
			break
		}
	}
	return index
}

func skipPresentationControlString(value string, index int) int {
	for index < len(value) {
		switch value[index] {
		case 0x07, 0x9c:
			return index + 1
		case 0x1b:
			if index+1 < len(value) && value[index+1] == '\\' {
				return index + 2
			}
		}
		if index+1 < len(value) && value[index] == 0xc2 && value[index+1] == 0x9c {
			return index + 2
		}
		index++
	}
	return index
}

func presentationDisplayWidth(value string) int {
	width := 0
	for _, r := range value {
		if r == '\n' {
			width++
			continue
		}
		width += gotui.RuneWidth(r)
	}
	return width
}

func truncatePresentationDisplayWidth(value string, limit int) string {
	if limit < 1 || presentationDisplayWidth(value) <= limit {
		return value
	}
	return presentationDisplayPrefix(value, limit)
}

func presentationDisplayPrefix(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	width := 0
	var out strings.Builder
	for _, r := range value {
		runeWidth := gotui.RuneWidth(r)
		if r == '\n' {
			runeWidth = 1
		}
		if width+runeWidth > limit {
			break
		}
		out.WriteRune(r)
		width += runeWidth
	}
	return out.String()
}

func presentationDisplaySuffix(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	width := 0
	start := len(runes)
	for start > 0 {
		r := runes[start-1]
		runeWidth := gotui.RuneWidth(r)
		if r == '\n' {
			runeWidth = 1
		}
		if width+runeWidth > limit {
			break
		}
		start--
		width += runeWidth
	}
	return string(runes[start:])
}

func presentationLooksLikeTerminalArt(value string) bool {
	if presentationHasTerminalLayoutControl(value) {
		return true
	}
	plain := sanitizePresentationTerminalText(value)
	graphics, braille, printable, lines, lineWidth, maxLineWidth := 0, 0, 0, 1, 0, 0
	for _, r := range plain {
		if r == '\n' {
			lines++
			if lineWidth > maxLineWidth {
				maxLineWidth = lineWidth
			}
			lineWidth = 0
			continue
		}
		lineWidth += gotui.RuneWidth(r)
		if unicode.IsSpace(r) {
			continue
		}
		printable++
		if r >= 0x2500 && r <= 0x259f || r >= 0x2800 && r <= 0x28ff {
			graphics++
		}
		if r >= 0x2800 && r <= 0x28ff {
			braille++
		}
	}
	if lineWidth > maxLineWidth {
		maxLineWidth = lineWidth
	}
	if braille >= 8 && lines >= 3 {
		return true
	}
	return graphics >= 12 && (graphics*100 >= max(1, printable)*8 || maxLineWidth >= 120)
}

func presentationHasTerminalLayoutControl(value string) bool {
	for index := 0; index < len(value); {
		switch value[index] {
		case 0x1b:
			if index+1 >= len(value) {
				return false
			}
			if value[index+1] == '[' {
				end := skipPresentationCSI(value, index+2)
				if end > index+2 && value[end-1] != 'm' {
					return true
				}
				index = end
				continue
			}
			if strings.ContainsRune("PX^_", rune(value[index+1])) {
				return true
			}
			index = skipPresentationEscape(value, index)
			continue
		case 0x9b:
			end := skipPresentationCSI(value, index+1)
			if end > index+1 && value[end-1] != 'm' {
				return true
			}
			index = end
			continue
		}
		_, size := utf8.DecodeRuneInString(value[index:])
		if size < 1 {
			size = 1
		}
		index += size
	}
	return false
}

func formatTaskPresentationSummary(lang i18n.Language, actionLabel, toolName string, input, data map[string]any, state string, outcome ObservationOutcome) string {
	parts := []string{actionLabel}
	switch toolName {
	case "TaskCreate":
		task := presentationNestedMap(data, "task")
		parts = append(parts, firstPresentationString(task, "id"), firstNonEmptyString(firstPresentationString(task, "subject"), presentationString(input["subject"])))
	case "TaskList":
		counts := presentationStatusCounts(firstPresentationValue(data, "tasks"))
		for _, status := range []string{"in_progress", "pending", "completed"} {
			if count := counts[status]; count > 0 {
				switch status {
				case "in_progress":
					parts = append(parts, i18n.Format(lang, i18n.KeyPresentationActive, count))
				case "pending":
					parts = append(parts, i18n.Format(lang, i18n.KeyPresentationPending, count))
				case "completed":
					parts = append(parts, i18n.Format(lang, i18n.KeyPresentationCompleted, count))
				}
			}
		}
		if len(counts) == 0 {
			if outcome == OutcomeSucceeded {
				return i18n.Text(lang, i18n.KeyToolEmptyTasks)
			} else {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationTasks, 0))
			}
		}
		if blocked := presentationTaskBlockedCount(firstPresentationValue(data, "tasks")); blocked > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationBlocked, blocked))
		}
	case "TaskGet":
		task := presentationNestedMap(data, "task")
		parts = append(parts, firstPresentationString(task, "id"), firstPresentationString(task, "subject"), firstPresentationString(task, "status"))
		if blocked := presentationArrayLen(firstPresentationValue(task, "blockedBy")); blocked > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationBlockedBy, blocked))
		}
	case "TaskUpdate":
		parts = append(parts, firstNonEmptyString(firstPresentationString(data, "taskId"), presentationString(input["taskId"])))
		if transition := presentationNestedMap(data, "statusChange"); len(transition) > 0 {
			parts = append(parts, firstPresentationString(transition, "from")+" -> "+firstPresentationString(transition, "to"))
		} else if fields := presentationArrayLen(firstPresentationValue(data, "updatedFields")); fields > 0 {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationFields, fields))
		}
		if presentationBool(data, "verificationNudgeNeeded") {
			parts = append(parts, i18n.Text(lang, i18n.KeyPresentationVerificationNeeded))
		}
	case "TaskStop":
		parts = append(parts,
			firstNonEmptyString(firstPresentationString(data, "task_id"), presentationString(input["task_id"]), presentationString(input["shell_id"])),
			firstPresentationString(data, "task_type"),
		)
	case "TaskOutput":
		parts = append(parts, firstNonEmptyString(firstPresentationString(data, "task_id", "taskId"), presentationString(input["task_id"])))
		if retrieval := firstPresentationString(data, "retrieval_status", "retrievalStatus"); retrieval != "" {
			parts = append(parts, retrieval)
		}
		if taskStatus := firstPresentationString(data, "task_status", "taskStatus", "status"); taskStatus != "" {
			parts = append(parts, taskStatus)
		}
		if bytes, ok := presentationInt(data, "output_bytes", "outputBytes"); ok {
			parts = append(parts, formatPresentationBytes(bytes))
			if total, totalOK := presentationInt(data, "total_bytes", "totalBytes"); totalOK && total != bytes {
				parts = append(parts, i18n.Format(lang, i18n.KeyPresentationOf, formatPresentationBytes(total)))
			}
		}
		if start, ok := presentationInt(data, "start_offset", "startOffset"); ok {
			end, _ := presentationInt(data, "end_offset", "endOffset")
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationOffset, start, end))
		}
		if presentationBool(data, "block", "follow") {
			parts = append(parts, i18n.Text(lang, i18n.KeyPresentationFollow))
		}
		if code, ok := presentationInt(data, "exit_code", "exitCode"); ok {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationExit, strconv.FormatInt(code, 10)))
		}
		if presentationBool(data, "was_truncated", "wasTruncated") {
			parts = append(parts, i18n.Text(lang, i18n.KeyPresentationTruncated))
		}
	}
	if outcome != OutcomeSucceeded && outcome != OutcomeRunning {
		parts = append(parts, state)
	} else if len(parts) == 1 {
		parts = append(parts, state)
	}
	return joinPresentationParts(parts)
}

func formatGoalPresentationSummary(lang i18n.Language, actionLabel string, input, data map[string]any, state string, outcome ObservationOutcome) string {
	parts := []string{actionLabel}
	goal := presentationNestedMap(data, "goal")
	if objective := firstNonEmptyString(firstPresentationString(goal, "objective"), presentationString(input["objective"])); objective != "" {
		parts = append(parts, truncatePresentationRunes(objective, 100))
	}
	if status := firstPresentationString(goal, "status"); status != "" {
		parts = append(parts, status)
	}
	criteriaCount := presentationArrayLen(firstPresentationValue(goal, "acceptance_criteria", "acceptanceCriteria"))
	if criteriaCount > 0 {
		evaluation := presentationNestedMap(goal, "last_acceptance_evaluation")
		if len(evaluation) == 0 {
			evaluation = presentationNestedMap(goal, "lastAcceptanceEvaluation")
		}
		results := firstPresentationValue(evaluation, "criteria")
		if presentationArrayLen(results) == criteriaCount {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationGoalCriteriaProgress,
				presentationGoalCriteriaMet(results), criteriaCount))
		} else {
			parts = append(parts, i18n.Format(lang, i18n.KeyPresentationGoalCriteriaCount, criteriaCount))
		}
	}
	if used, ok := presentationInt(goal, "tokens_used", "tokensUsed"); ok {
		parts = append(parts, i18n.Format(lang, i18n.KeyPresentationTokens, formatPresentationInt(used)))
	}
	if budget, ok := presentationInt(goal, "token_budget", "tokenBudget"); ok && budget > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyPresentationBudget, formatPresentationInt(budget)))
	}
	if outcome != OutcomeSucceeded && outcome != OutcomeRunning {
		parts = append(parts, state)
	}
	return joinPresentationParts(parts)
}

func presentationGoalCriteriaMet(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	var results []map[string]any
	if json.Unmarshal(encoded, &results) != nil {
		return 0
	}
	met := 0
	for _, result := range results {
		if presentationBool(result, "met") {
			met++
		}
	}
	return met
}

func formatSafeFallbackSummary(lang i18n.Language, toolName string, input, data map[string]any, state string) string {
	parts := []string{firstNonEmptyString(strings.TrimSpace(toolName), i18n.Text(lang, i18n.KeyPresentationFallbackTool)), state}
	if keys := sortedPresentationKeys(input, false); len(keys) > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyPresentationInputKeys, strings.Join(keys, ",")))
	}
	if bytes, ok := presentationInt(data, "_presentationResultBytes"); ok && bytes > 0 {
		parts = append(parts, formatPresentationBytes(bytes))
	}
	if lines, ok := presentationInt(data, "_presentationResultLines"); ok && lines > 1 {
		parts = append(parts, i18n.Format(lang, i18n.KeyPresentationLines, lines))
	}
	if bytes, ok := presentationInt(data, "_presentationResultBytes"); ok && bytes > 0 {
		details := i18n.Text(lang, i18n.KeyPresentationDetailsAvailable)
		if lang == i18n.LangEN {
			details = strings.ToLower(details)
		}
		parts = append(parts, details)
	}
	return joinPresentationParts(parts)
}

func presentationStatusCounts(value any) map[string]int {
	items, ok := value.([]any)
	if !ok {
		return map[string]int{}
	}
	counts := make(map[string]int)
	for _, item := range items {
		entry := structuredPresentationMap(item)
		if status := firstPresentationString(entry, "status"); status != "" {
			counts[status]++
		}
	}
	return counts
}

func presentationTaskBlockedCount(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	var tasks []map[string]any
	if json.Unmarshal(encoded, &tasks) != nil {
		return 0
	}
	count := 0
	for _, task := range tasks {
		if presentationArrayLen(task["blockedBy"]) > 0 {
			count++
		}
	}
	return count
}

func presentationScheduleJobCounts(value any) (recurring, durable int) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, 0
	}
	var jobs []map[string]any
	if json.Unmarshal(encoded, &jobs) != nil {
		return 0, 0
	}
	for _, job := range jobs {
		if value, present := presentationBoolValue(job["recurring"]); present && value {
			recurring++
		}
		if value, present := presentationBoolValue(job["durable"]); present && value {
			durable++
		}
	}
	return recurring, durable
}

func presentationArrayLen(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []string:
		return len(typed)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return 0
		}
		var items []any
		if json.Unmarshal(encoded, &items) != nil {
			return 0
		}
		return len(items)
	}
}

func presentationMapLen(value any) int {
	if value == nil {
		return 0
	}
	if direct, ok := value.(map[string]any); ok {
		return len(direct)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil {
		return 0
	}
	return len(object)
}

func firstPresentationArrayMap(value any) map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var items []map[string]any
	if json.Unmarshal(encoded, &items) != nil || len(items) == 0 {
		return map[string]any{}
	}
	return items[0]
}

func presentationQuestionChoiceCount(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	var questions []map[string]any
	if json.Unmarshal(encoded, &questions) != nil {
		return 0
	}
	total := 0
	for _, question := range questions {
		total += presentationArrayLen(question["options"])
	}
	return total
}

func firstPresentationValue(data map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}

func presentationNestedMap(data map[string]any, key string) map[string]any {
	return structuredPresentationMap(data[key])
}

func presentationIntFrom(sources []map[string]any, keys ...string) (int64, bool) {
	for _, source := range sources {
		if value, ok := presentationInt(source, keys...); ok {
			return value, true
		}
	}
	return 0, false
}

func stringMapAsAny(input map[string]string) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func presentationDiffStats(data map[string]any) (int64, int64, bool) {
	gitDiff := presentationNestedMap(data, "gitDiff")
	if added, ok := presentationInt(gitDiff, "additions"); ok {
		removed, _ := presentationInt(gitDiff, "deletions")
		return added, removed, true
	}
	added, addedOK := presentationInt(data, "added", "lines_added", "linesAdded")
	removed, removedOK := presentationInt(data, "removed", "lines_removed", "linesRemoved")
	if addedOK || removedOK {
		return added, removed, true
	}
	hunks, ok := data["structuredPatch"].([]any)
	if !ok || len(hunks) == 0 {
		return 0, 0, false
	}
	for _, raw := range hunks {
		hunk := structuredPresentationMap(raw)
		newLines, _ := presentationInt(hunk, "newLines")
		oldLines, _ := presentationInt(hunk, "oldLines")
		added += newLines
		removed += oldLines
	}
	return added, removed, true
}

func formatPresentationDetailDiff(lang i18n.Language, data map[string]any, family CommandFamily) string {
	if family != FamilyFileWrite {
		return ""
	}
	diff := presentationStructuredPatch(data)
	if diff == "" {
		diff = presentationString(presentationNestedMap(data, "gitDiff")["patch"])
	}
	return RedactPresentationTextInLanguage(lang, diff, maxPresentationDetailDiffRunes)
}

func presentationStructuredPatch(data map[string]any) string {
	encoded, err := json.Marshal(data["structuredPatch"])
	if err != nil {
		return ""
	}
	var hunks []map[string]any
	if json.Unmarshal(encoded, &hunks) != nil || len(hunks) == 0 {
		return ""
	}

	var diff strings.Builder
	for _, hunk := range hunks {
		oldStart, oldStartOK := presentationInt(hunk, "oldStart")
		oldLines, oldLinesOK := presentationInt(hunk, "oldLines")
		newStart, newStartOK := presentationInt(hunk, "newStart")
		newLines, newLinesOK := presentationInt(hunk, "newLines")
		if !oldStartOK || !oldLinesOK || !newStartOK || !newLinesOK {
			continue
		}
		// Unified-diff hunk syntax is a protocol value, not translatable copy.
		fmt.Fprintf(&diff, "@@ -%d,%d +%d,%d @@\n", oldStart, oldLines, newStart, newLines)
		lineBytes, lineErr := json.Marshal(hunk["lines"])
		if lineErr != nil {
			continue
		}
		var lines []string
		if json.Unmarshal(lineBytes, &lines) != nil {
			continue
		}
		for _, line := range lines {
			diff.WriteString(line)
			diff.WriteByte('\n')
		}
	}
	return strings.TrimSpace(diff.String())
}

func presentationCommandInLanguage(lang i18n.Language, input map[string]any) string {
	if description := presentationString(input["description"]); description != "" {
		return truncatePresentationRunes(description, 100)
	}
	command := presentationString(input["command"])
	if command == "" {
		return ""
	}
	if presentationTextLineContainsSecret(command) {
		return i18n.Text(lang, i18n.KeyPresentationRedactedCommand)
	}
	return truncatePresentationRunes(strings.Join(strings.Fields(command), " "), 100)
}

func mapPresenceAction(input map[string]any, key string) string {
	if _, ok := input[key]; ok {
		return "set"
	}
	return "get"
}

func sortedPresentationKeys(input map[string]any, includeInternal bool) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		if !includeInternal && strings.HasPrefix(key, "_") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func RedactPresentationValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitivePresentationKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			if presentationObjectKeyIsLocator(key) {
				if text, ok := item.(string); ok {
					out[key] = sanitizePresentationLocator(text)
					continue
				}
			}
			out[key] = RedactPresentationValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, item := range typed {
			if isSensitivePresentationKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			if presentationObjectKeyIsLocator(key) {
				out[key] = sanitizePresentationLocator(item)
				continue
			}
			out[key] = redactPresentationString(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = RedactPresentationValue(typed[i])
		}
		return out
	case string:
		return redactPresentationString(typed)
	default:
		return value
	}
}

func redactPresentationString(value string) string {
	if presentationLocatorHasSensitiveMaterial(value) {
		return sanitizePresentationLocator(value)
	}
	return value
}

// RedactPresentationText provides a bounded display projection for diagnostic
// prose. It never influences outcome classification. JSON objects are redacted
// structurally; non-JSON lines containing credential-shaped keys are replaced
// wholesale rather than attempting to preserve a potentially secret value.
func RedactPresentationText(value string, limit int) string {
	return RedactPresentationTextInLanguage(i18n.DetectOrLoadLanguage(), value, limit)
}

func RedactPresentationTextInLanguage(lang i18n.Language, value string, limit int) string {
	value = strings.TrimSpace(sanitizePresentationTerminalText(value))
	if value == "" {
		return ""
	}
	var decoded any
	if json.Unmarshal([]byte(value), &decoded) == nil {
		if encoded, err := json.Marshal(RedactPresentationValue(decoded)); err == nil {
			return truncatePresentationDisplayWidth(string(encoded), limit)
		}
	}
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if presentationTextLineContainsSecret(line) {
			lines[index] = i18n.Text(lang, i18n.KeyPresentationRedactedDetail)
		}
	}
	return truncatePresentationDisplayWidth(strings.Join(lines, "\n"), limit)
}

func presentationValueHasSensitiveKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if isSensitivePresentationKey(key) || presentationValueHasSensitiveKey(item) {
				return true
			}
		}
	case map[string]string:
		for key := range typed {
			if isSensitivePresentationKey(key) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if presentationValueHasSensitiveKey(item) {
				return true
			}
		}
	}
	return false
}

func isSensitivePresentationKey(key string) bool {
	normalized := normalizedPresentationKey(key)
	for _, safeMetric := range []string{
		"tokens", "token_budget", "token_count", "total_tokens", "input_tokens", "output_tokens", "max_tokens",
		"cache_read_tokens", "cache_creation_tokens", "tokenbudget", "tokencount", "totaltokens", "inputtokens",
		"outputtokens", "maxtokens", "cachereadtokens", "cachecreationtokens",
	} {
		if normalized == safeMetric {
			return false
		}
	}
	for _, marker := range []string{"password", "passwd", "secret", "api_key", "apikey", "authorization", "cookie", "private_key", "credential"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	if normalized == "token" || strings.HasSuffix(normalized, "_token") || strings.HasSuffix(normalized, "token") || strings.HasPrefix(normalized, "token_") {
		return true
	}
	return false
}

func normalizedPresentationKey(key string) string {
	return strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(strings.TrimSpace(key)))
}

func presentationMetadataHasSensitiveKey(metadata map[string]string) bool {
	for key := range metadata {
		if isSensitivePresentationKey(key) {
			return true
		}
	}
	return false
}

func presentationTextLineContainsSecret(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{
		"password=", "password:", "passwd=", "passwd:", "api_key=", "api_key:", "apikey=", "apikey:",
		"api-key=", "api-key:", "token=", "token:", "token\"=", "token\":",
		"access_token=", "access_token:", "refresh_token=", "refresh_token:", "authorization:", "authorization=",
		"accesstoken=", "accesstoken:", "refreshtoken=", "refreshtoken:", "cookie:", "cookie=", "private_key",
		"client_secret=", "client_secret:", "clientsecret=", "clientsecret:", "bearer ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func structuredPresentationMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if typed, ok := value.(map[string]any); ok {
		return cloneStringAnyMap(typed)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(encoded, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func structuredPresentationData(toolName string, result *types.ToolResultBlock) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	if toolName == "ListMcpResourcesTool" {
		if encoded, err := json.Marshal(result.Data); err == nil {
			var resources []any
			if json.Unmarshal(encoded, &resources) == nil {
				return map[string]any{"resources": resources, "resourceCount": len(resources)}
			}
		}
	}
	data := structuredPresentationMap(result.Data)
	if len(data) > 0 || !toolAllowsStructuredContentFallback(toolName) {
		return data
	}
	content := strings.TrimSpace(result.TextContent())
	if !strings.HasPrefix(content, "{") {
		return data
	}
	var decoded map[string]any
	if json.Unmarshal([]byte(content), &decoded) == nil && decoded != nil {
		return decoded
	}
	return data
}

func toolAllowsStructuredContentFallback(toolName string) bool {
	switch toolName {
	case "Agent", "TaskStop", "TeamCreate", "TeamDelete":
		return true
	default:
		return false
	}
}

func promotedPresentationOutcome(outcome ObservationOutcome, result *types.ToolResultBlock, _ map[string]any, _ map[string]string) ObservationOutcome {
	if result == nil {
		return outcome
	}
	if mapped := observationOutcomeFromToolOutcome(result.Outcome); mapped != OutcomeUnknown {
		return mapped
	}
	return outcome
}

func observationOutcomeFromToolOutcome(outcome types.ToolOutcome) ObservationOutcome {
	switch outcome {
	case types.ToolOutcomeSucceeded:
		return OutcomeSucceeded
	case types.ToolOutcomeFailed:
		return OutcomeFailed
	case types.ToolOutcomePartial:
		return OutcomePartial
	case types.ToolOutcomeDenied:
		return OutcomeDenied
	case types.ToolOutcomeCancelled:
		return OutcomeCancelled
	case types.ToolOutcomeTimedOut:
		return OutcomeTimedOut
	default:
		return OutcomeUnknown
	}
}

func presentationStructuredWarning(data map[string]any, metadata map[string]string) bool {
	if metadataBool(metadata, "warning") || metadataBool(metadata, "partial") || strings.TrimSpace(metadata["regressed"]) != "" {
		return true
	}
	return firstPresentationString(data, "warning") != ""
}

func presentationBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}

func toolReferenceCount(result *types.ToolResultBlock) int {
	if result == nil {
		return 0
	}
	count := 0
	for _, block := range result.ContentBlocks {
		switch block.(type) {
		case types.ToolReferenceBlock, *types.ToolReferenceBlock:
			count++
		}
	}
	return count
}

func presentationLineCount(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func presentationObjectInLanguage(lang i18n.Language, toolName string, input, data map[string]any) string {
	family := CommandFamilyForTool(toolName)
	if family == FamilyUnknown {
		return ""
	}
	if family == FamilyMCP {
		server := sanitizePresentationLocatorInLanguage(lang, firstNonEmptyString(
			presentationString(input["server_name"]), presentationString(input["serverName"]), presentationString(input["server"]),
			firstPresentationString(data, "server", "server_name", "serverName", "mcp.serverName"),
		))
		capability := firstNonEmptyString(
			presentationString(input["tool_name"]), presentationString(input["toolName"]), presentationString(input["capability"]),
			firstPresentationString(data, "tool_name", "toolName", "capability", "method", "prompt_name", "promptName"),
			sanitizePresentationLocatorInLanguage(lang, firstNonEmptyString(presentationString(input["uri"]), presentationString(input["resource_uri"]), presentationString(input["resourceUri"]), firstPresentationString(data, "uri", "resource_uri", "resourceUri"))),
		)
		if server == "" && strings.HasPrefix(strings.ToLower(toolName), "mcp__") {
			parts := strings.SplitN(strings.TrimPrefix(toolName, "mcp__"), "__", 2)
			server = parts[0]
			if len(parts) == 2 {
				capability = parts[1]
			}
		}
		return truncatePresentationRunes(strings.Trim(strings.Join([]string{server, capability}, "/"), "/"), 120)
	}
	keys := []string{"file_path", "filePath", "path", "name", "base_ref", "notebook_path", "notebookPath", "uri", "url", "query", "server", "task_id", "taskId", "goal_id", "goalId", "team_name", "teamName", "job_id", "jobId", "id", "target", "setting", "key", "worktreePath"}
	if toolName == "Agent" {
		keys = append([]string{"agentId", "agent_id", "description"}, keys...)
	}
	sources := []map[string]any{input, data, presentationNestedMap(data, "file"), presentationNestedMap(data, "task"), presentationNestedMap(data, "goal")}
	for _, source := range sources {
		for _, key := range keys {
			if value, ok := source[key]; ok {
				if isSensitivePresentationKey(key) {
					return "[REDACTED]"
				}
				text := presentationString(value)
				if presentationObjectKeyIsLocator(key) {
					text = sanitizePresentationLocatorInLanguage(lang, text)
				}
				if text != "" {
					return truncatePresentationRunes(text, 120)
				}
			}
		}
	}
	return ""
}

// compactDefaultPresentationLocator keeps default cards useful without
// projecting an absolute local filesystem locator. Exact paths remain in the
// retained evidence and explicit evidence view.
func compactDefaultPresentationLocator(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if parsed, err := url.Parse(value); err == nil && strings.EqualFold(parsed.Scheme, "file") {
		value = parsed.Path
	}
	if !filepath.IsAbs(value) {
		return value
	}
	clean := filepath.Clean(value)
	volume := filepath.VolumeName(clean)
	relative := strings.TrimLeft(strings.TrimPrefix(clean, volume), string(filepath.Separator))
	if relative == "" {
		return filepath.Base(clean)
	}
	return relative
}

// sanitizePresentationLocator returns a display-only URL/URI projection. It
// keeps the host and path useful for attribution while removing userinfo and
// credential-shaped query or fragment values. Execution and retained evidence
// continue to use the original value.
func sanitizePresentationLocator(value string) string {
	return sanitizePresentationLocatorInLanguage(i18n.DetectOrLoadLanguage(), value)
}

func sanitizePresentationLocatorInLanguage(lang i18n.Language, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		if presentationLocatorComponentSensitive(value) {
			return i18n.Text(lang, i18n.KeyPresentationRedactedLocator)
		}
		return value
	}
	if parsed.User != nil {
		parsed.User = nil
	}
	if parsed.RawQuery != "" {
		parsed.RawQuery = sanitizePresentationLocatorQuery(parsed.RawQuery)
	}
	if parsed.Fragment != "" {
		fragment := sanitizePresentationLocatorFragment(parsed.Fragment)
		if decoded, decodeErr := url.QueryUnescape(fragment); decodeErr == nil {
			fragment = decoded
		}
		parsed.Fragment = fragment
		parsed.RawFragment = ""
	}
	return parsed.String()
}

func sanitizePresentationLocatorQuery(raw string) string {
	query, err := url.ParseQuery(raw)
	if err != nil {
		// Invalid query syntax is uncommon in a display locator and cannot be
		// safely inspected field by field. Drop it rather than echoing a secret.
		return "redacted"
	}
	for key := range query {
		if isSensitivePresentationLocatorKey(key) {
			query[key] = []string{"[REDACTED]"}
		}
	}
	return query.Encode()
}

func sanitizePresentationLocatorFragment(fragment string) string {
	prefix, rawQuery := "", fragment
	if index := strings.IndexByte(fragment, '?'); index >= 0 {
		prefix, rawQuery = fragment[:index+1], fragment[index+1:]
	} else if !strings.Contains(fragment, "=") {
		return fragment
	}
	if rawQuery == "" {
		return prefix
	}
	return prefix + sanitizePresentationLocatorQuery(rawQuery)
}

func presentationValueHasSensitiveLocator(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, item := range typed {
			if presentationValueHasSensitiveLocator(item) {
				return true
			}
		}
	case map[string]string:
		for _, item := range typed {
			if presentationValueHasSensitiveLocator(item) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if presentationValueHasSensitiveLocator(item) {
				return true
			}
		}
	case string:
		return presentationLocatorHasSensitiveMaterial(typed)
	}
	return false
}

func presentationObjectKeyIsLocator(key string) bool {
	switch normalizedPresentationKey(key) {
	case "url", "uri", "target", "to":
		return true
	default:
		return false
	}
}

func presentationLocatorHasSensitiveMaterial(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return presentationLocatorComponentSensitive(value)
	}
	if parsed.User != nil {
		return true
	}
	if parsed.RawQuery != "" {
		query, queryErr := url.ParseQuery(parsed.RawQuery)
		if queryErr != nil {
			return true
		}
		for key := range query {
			if isSensitivePresentationLocatorKey(key) {
				return true
			}
		}
	}
	fragment := parsed.Fragment
	if index := strings.IndexByte(fragment, '?'); index >= 0 {
		fragment = fragment[index+1:]
	} else if !strings.Contains(fragment, "=") {
		fragment = ""
	}
	if fragment != "" {
		query, queryErr := url.ParseQuery(fragment)
		if queryErr != nil {
			return true
		}
		for key := range query {
			if isSensitivePresentationLocatorKey(key) {
				return true
			}
		}
	}
	return false
}

func presentationLocatorComponentSensitive(value string) bool {
	decoded, err := url.QueryUnescape(value)
	if err == nil {
		value = decoded
	}
	for _, component := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '&' || r == ';' || r == '?' || r == '#'
	}) {
		key, _, found := strings.Cut(component, "=")
		if found && isSensitivePresentationLocatorKey(strings.TrimSpace(key)) {
			return true
		}
	}
	return false
}

func isSensitivePresentationLocatorKey(key string) bool {
	normalized := normalizedPresentationKey(key)
	compact := strings.ReplaceAll(normalized, "_", "")
	switch compact {
	case "key", "apikey", "token", "accesstoken", "refreshtoken", "idtoken", "secret", "clientsecret",
		"signature", "sig", "code", "authorization", "credential", "password", "passwd", "jwt", "sessionid":
		return true
	}
	for _, suffix := range []string{"_key", "_token", "_secret", "_signature", "_sig", "_code", "_credential", "_password"} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

func presentationAggregationIntent(family CommandFamily, toolName string, input map[string]any) string {
	scope := ""
	for _, key := range []string{"root", "cwd", "path"} {
		if value, ok := input[key]; ok {
			scope = presentationString(value)
			break
		}
	}
	return string(family) + ":" + toolName + ":" + scope
}

func presentationQuery(input map[string]any) string {
	for _, key := range []string{"pattern", "query", "symbol"} {
		if value, ok := input[key]; ok {
			return truncatePresentationRunes(presentationString(value), 100)
		}
	}
	return ""
}

func presentationInt(data map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case int:
			return int64(typed), true
		case int64:
			return typed, true
		case float64:
			return int64(typed), true
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return parsed, true
			}
		case string:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func presentationDurationMs(data map[string]any, metadata map[string]string) (int64, bool) {
	if value, ok := presentationInt(data, "durationMs", "duration_ms", "totalDurationMs", "total_duration_ms"); ok {
		return value, true
	}
	if value, ok := presentationInt(presentationNestedMap(data, "metadata"), "durationMs", "duration_ms"); ok {
		return value, true
	}
	for _, key := range []string{"duration_ms", "durationMs"} {
		if raw := metadata[key]; raw != "" {
			if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
				return value, true
			}
		}
	}
	if seconds, ok := presentationFloat(data, "durationSeconds", "duration_seconds"); ok {
		return int64(seconds * 1000), true
	}
	for _, key := range []string{"durationSeconds", "duration_seconds"} {
		if raw := metadata[key]; raw != "" {
			if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
				return int64(seconds * 1000), true
			}
		}
	}
	return 0, false
}

func presentationFloat(data map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		switch value := data[key].(type) {
		case float64:
			return value, true
		case float32:
			return float64(value), true
		case int:
			return float64(value), true
		case int64:
			return float64(value), true
		case json.Number:
			parsed, err := value.Float64()
			return parsed, err == nil
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			return parsed, err == nil
		}
	}
	return 0, false
}

func presentationBool(data map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
			return parsed
		}
	}
	return false
}

func metadataBool(metadata map[string]string, key string) bool {
	parsed, _ := strconv.ParseBool(strings.TrimSpace(metadata[key]))
	return parsed
}

func metadataFalse(metadata map[string]string, key string) bool {
	raw, ok := metadata[key]
	if !ok {
		return false
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil && !parsed
}

func firstPresentationString(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			if text := presentationString(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func firstPresentationMetadata(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if text := strings.TrimSpace(metadata[key]); text != "" {
			return text
		}
	}
	return ""
}

func presentationString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func formatPresentationBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	value := float64(bytes)
	unit := "B"
	for _, candidate := range units {
		value /= 1024
		unit = candidate
		if value < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

func formatPresentationDuration(milliseconds int64) string {
	if milliseconds < 1000 {
		return fmt.Sprintf("%dms", milliseconds)
	}
	return fmt.Sprintf("%.1fs", float64(milliseconds)/1000)
}

func formatPresentationInt(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	raw := strconv.FormatInt(value, 10)
	for index := len(raw) - 3; index > 0; index -= 3 {
		raw = raw[:index] + "," + raw[index:]
	}
	if negative {
		return "-" + raw
	}
	return raw
}

func joinPresentationParts(parts []string) string {
	filtered := parts[:0]
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return strings.Join(filtered, " · ")
}

func truncatePresentationRunes(value string, limit int) string {
	if limit < 1 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit-1]) + "…"
}
