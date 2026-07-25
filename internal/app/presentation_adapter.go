package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/types"
)

type semanticToolAggregationGroup struct {
	key       string
	baseKey   string
	family    tui.CommandFamily
	intent    string
	first     presentation.ToolPresentation
	members   []presentation.ToolPresentation
	memberIDs []string
	frozen    bool
}

type semanticToolAggregationBinding struct {
	baseKey  string
	groupKey string
}

type semanticToolAggregationBuffer struct {
	toolKeys   map[string]semanticToolAggregationBinding
	groups     map[string]*semanticToolAggregationGroup
	active     map[string]string
	generation map[string]int
	order      []string
}

func newSemanticToolAggregationBuffer() *semanticToolAggregationBuffer {
	return &semanticToolAggregationBuffer{
		toolKeys: make(map[string]semanticToolAggregationBinding), groups: make(map[string]*semanticToolAggregationGroup),
		active: make(map[string]string), generation: make(map[string]int),
	}
}

// Start returns true only for the first running transition in a safe group or
// for a non-groupable tool. Subsequent low-value running members stay silent.
func (b *semanticToolAggregationBuffer) Start(ctx presentation.ToolEventContext, call types.ToolUseBlock) bool {
	if b == nil || strings.TrimSpace(call.ID) == "" {
		return true
	}
	formatted := tui.FormatToolPresentation(call.Name, call.Input, tui.OutcomeSucceeded, nil)
	if formatted.AggregationIntent == "" || formatted.SideEffect || formatted.Risk != tui.RiskLow || !semanticFamilyCanAggregate(formatted.Family) {
		return true
	}
	baseKey := semanticAggregateKey(ctx, formatted.Family, formatted.AggregationIntent,
		tui.CanonicalAggregationDomainIntent(formatted.Family, call.Name, call.Input))
	group, created := b.ensureSemanticAggregateGroup(baseKey, formatted.Family, formatted.AggregationIntent)
	b.toolKeys[call.ID] = semanticToolAggregationBinding{baseKey: baseKey, groupKey: group.key}
	return created
}

func (b *semanticToolAggregationBuffer) Complete(ctx presentation.ToolEventContext, call types.ToolUseBlock, result types.ToolResultBlock, presentation presentation.ToolPresentation) bool {
	if b == nil || strings.TrimSpace(call.ID) == "" {
		return true
	}
	binding, tracked := b.toolKeys[call.ID]
	delete(b.toolKeys, call.ID)
	formatted := tui.FormatToolPresentation(call.Name, call.Input, presentationOutcomeForToolResult(result), &result)
	decision := tui.DecidePresentation(formatted.Facts(formatted.Outcome))
	baseKey := binding.baseKey
	if baseKey == "" && formatted.AggregationIntent != "" && semanticFamilyCanAggregate(formatted.Family) {
		baseKey = semanticAggregateKey(ctx, formatted.Family, formatted.AggregationIntent,
			tui.CanonicalAggregationDomainIntent(formatted.Family, call.Name, call.Input))
	}
	if semanticAggregationBoundary(formatted) {
		b.rotateSemanticAggregate(baseKey)
		return true
	}
	if !tracked || !decision.AggregationEligible {
		return true
	}
	group := b.groups[binding.groupKey]
	if group == nil || group.frozen {
		return true
	}
	if len(group.members) == 0 {
		group.first = presentation
	}
	group.members = append(group.members, presentation)
	group.memberIDs = append(group.memberIDs, call.ID)
	_ = ctx
	return false
}

func (b *semanticToolAggregationBuffer) ensureSemanticAggregateGroup(baseKey string, family tui.CommandFamily, intent string) (*semanticToolAggregationGroup, bool) {
	if activeKey := b.active[baseKey]; activeKey != "" {
		if group := b.groups[activeKey]; group != nil && !group.frozen {
			return group, false
		}
	}
	generation := b.generation[baseKey]
	key := baseKey
	if generation > 0 {
		key = fmt.Sprintf("%s\x00segment:%d", baseKey, generation)
	}
	group := &semanticToolAggregationGroup{key: key, baseKey: baseKey, family: family, intent: intent}
	b.groups[key] = group
	b.active[baseKey] = key
	b.order = append(b.order, key)
	return group, true
}

func (b *semanticToolAggregationBuffer) rotateSemanticAggregate(baseKey string) {
	if b == nil || strings.TrimSpace(baseKey) == "" {
		return
	}
	activeKey := b.active[baseKey]
	group := b.groups[activeKey]
	if group == nil || group.frozen {
		return
	}
	group.frozen = true
	delete(b.active, baseKey)
	b.generation[baseKey]++
}

func semanticAggregationBoundary(formatted tui.FormattedPresentation) bool {
	if formatted.AggregationIntent == "" || formatted.Outcome == tui.OutcomeUnknown || formatted.Outcome == tui.OutcomeRunning {
		return false
	}
	return formatted.Outcome != tui.OutcomeSucceeded || formatted.Warning || formatted.RequiresDecision || formatted.SideEffect
}

func (b *semanticToolAggregationBuffer) Flush() []presentation.ToolPresentation {
	if b == nil {
		return nil
	}
	out := make([]presentation.ToolPresentation, 0, len(b.groups))
	for _, key := range b.order {
		group := b.groups[key]
		if group == nil || len(group.members) == 0 {
			continue
		}
		if len(group.members) == 1 {
			out = append(out, group.first)
			continue
		}
		aggregate := group.first
		aggregate.ToolName = "Aggregate"
		aggregate.ToolUseID = semanticAggregateID(group.key)
		aggregate.Action = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAdapterAggregateAction)
		aggregate.Object = group.intent
		aggregate.State = presentation.ToolPresentationStateSucceeded
		aggregate.Result = semanticAggregateSummary(group.family, len(group.members))
		aggregate.NextAction = ""
		aggregate.DetailLines = []string{i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAdapterAggregateMembers, strings.Join(group.memberIDs, ", "))}
		aggregate.PresentationLevel = presentation.ToolPresentationLevelFolded
		aggregate.ReasonCodes = []string{string(tui.ReasonAggregationCandidate)}
		aggregate.HasMore = true
		out = append(out, aggregate)
	}
	b.toolKeys = make(map[string]semanticToolAggregationBinding)
	b.groups = make(map[string]*semanticToolAggregationGroup)
	b.active = make(map[string]string)
	b.generation = make(map[string]int)
	b.order = nil
	return out
}

func semanticFamilyCanAggregate(family tui.CommandFamily) bool {
	switch family {
	case tui.FamilyFileRead, tui.FamilySearch, tui.FamilyWeb, tui.FamilyMCP:
		return true
	default:
		return false
	}
}

func semanticAggregateKey(ctx presentation.ToolEventContext, family tui.CommandFamily, intent, domainIntent string) string {
	values := []string{ctx.SessionID, ctx.TurnID, ctx.ActorID, ctx.WorkUnitID, string(family), intent, domainIntent}
	var builder strings.Builder
	for _, value := range values {
		fmt.Fprintf(&builder, "%d:%s|", len(value), value)
	}
	return builder.String()
}

func semanticAggregateID(key string) string {
	digest := sha256.Sum256([]byte("semantic-presentation-aggregate-v1\x00" + key))
	return "aggregate:" + hex.EncodeToString(digest[:12])
}

func semanticAggregateSummary(family tui.CommandFamily, count int) string {
	lang := i18n.DetectOrLoadLanguage()
	label := string(family)
	switch family {
	case tui.FamilyFileRead:
		label = i18n.Text(lang, i18n.KeyPresentationAggregateRead)
	case tui.FamilySearch:
		label = i18n.Text(lang, i18n.KeyPresentationAggregateSearch)
	case tui.FamilyWeb:
		label = i18n.Text(lang, i18n.KeyPresentationAggregateWeb)
	case tui.FamilyMCP:
		label = "MCP"
	}
	return i18n.Format(lang, i18n.KeyAdapterAggregateSummary, label, count)
}

func newTUICommandPresentationSink(app tuiActivityApp, sessionID string, epoch uint64, runID string, terminalOutput ...func(string)) func(commands.CommandPresentation) {
	var sourceSequence uint64
	return func(presentation commands.CommandPresentation) {
		if app == nil {
			return
		}
		sourceSequence++
		lifecycle, outcome := commandPresentationActivityLifecycle(presentation)
		event := tui.ActivityEvent{
			ID: "command:" + presentation.Command, RunID: runID, Attempt: 1,
			SessionID: sessionID, Epoch: epoch, WorkUnitID: runID,
			Actor: tui.ActivityActor{ID: "assistant", Type: "assistant"}, Kind: tui.ActivityCommand,
			Name:      "/" + presentation.Command + " " + presentation.Action,
			Phase:     tui.ActivityPhaseForTool(presentation.Command+" "+presentation.Action, nil, "assistant"),
			Lifecycle: lifecycle, Outcome: outcome, SourceSequence: sourceSequence,
			Attention: tui.ActivityAttention{Kind: tui.ActivityAttentionNone},
			Progress:  tui.ActivityProgress{Message: commandPresentationProgressMessage(presentation)},
			Control:   tui.ActivityControl{JumpTarget: "transcript"},
		}
		if presentation.Outcome == commands.CommandOutcomeWarning {
			event.Attention = tui.ActivityAttention{Kind: tui.ActivityAttentionWarning, Severity: tui.ActivityAttentionSeverityWarning, Unread: true}
		}
		app.UpdateSync(func() {
			event.Control.DetailRefs = retainCommandPresentationDetails(app.State(), sessionID, epoch, runID, presentation)
			_ = app.State().ApplyActivity(event)
		})
		if presentation.State == commands.CommandStateCompleted && len(terminalOutput) > 0 && terminalOutput[0] != nil {
			terminalOutput[0](formatCommandPresentationTerminal(presentation))
		}
	}
}

func retainCommandPresentationDetails(state *tui.AppState, sessionID string, epoch uint64, runID string, presentation commands.CommandPresentation) []tui.DetailRef {
	if state == nil || presentation.State != commands.CommandStateCompleted {
		return nil
	}
	sections := commandPresentationBoundedSections(presentation.Sections)
	evidenceRefs := boundedCommandEvidenceRefs(presentation.EvidenceRefs)
	refs := make([]tui.DetailRef, 0, len(sections)+len(evidenceRefs))
	retain := func(kind string, index, limit int, value string) {
		value = commands.RedactCommandPresentationText(value, limit)
		if strings.TrimSpace(value) == "" {
			return
		}
		ref, err := state.RetainDetailForEpoch(sessionID, epoch,
			fmt.Sprintf("command:%s:%s:%d", runID, kind, index), []byte(value))
		if err == nil {
			refs = append(refs, ref)
		}
	}
	for index, section := range sections {
		retain("section", index, 560, strings.TrimSpace(section.Label)+"\n"+strings.TrimSpace(section.Text))
	}
	for index, evidenceRef := range evidenceRefs {
		retain("evidence", index, 240, evidenceRef)
	}
	return refs
}

func commandPresentationActivityLifecycle(presentation commands.CommandPresentation) (tui.ActivityLifecycle, tui.ObservationOutcome) {
	if presentation.State == commands.CommandStateRunning {
		return tui.ActivityLifecycleRunning, tui.OutcomeRunning
	}
	switch presentation.Outcome {
	case commands.CommandOutcomeSucceeded, commands.CommandOutcomeWarning, commands.CommandOutcomeExitRequested:
		return tui.ActivityLifecycleCompleted, tui.OutcomeSucceeded
	case commands.CommandOutcomePartial:
		return tui.ActivityLifecycleFailed, tui.OutcomePartial
	case commands.CommandOutcomeFailed:
		return tui.ActivityLifecycleFailed, tui.OutcomeFailed
	case commands.CommandOutcomeDenied:
		return tui.ActivityLifecycleFailed, tui.OutcomeDenied
	case commands.CommandOutcomeCancelled, commands.CommandOutcomeInterrupted:
		return tui.ActivityLifecycleCancelled, tui.OutcomeCancelled
	case commands.CommandOutcomeTimedOut:
		return tui.ActivityLifecycleCancelled, tui.OutcomeTimedOut
	default:
		return tui.ActivityLifecycleCompleted, tui.OutcomeUnknown
	}
}

func commandPresentationProgressMessage(presentation commands.CommandPresentation) string {
	parts := []string{strings.TrimSpace(presentation.Summary)}
	if result := strings.TrimSpace(presentation.Result); result != "" {
		parts = append(parts, result)
	}
	for _, section := range commandPresentationNonMirroredSections(presentation) {
		parts = append(parts, commands.RedactCommandPresentationText(section.Label, 80)+"="+commands.RedactCommandPresentationText(section.Text, 480))
	}
	lang := i18n.DetectOrLoadLanguage()
	display := i18n.CommandDisplayLabel(lang, string(presentation.Display))
	risk := i18n.CommandRiskLabel(lang, string(presentation.Risk))
	parts = append(parts, i18n.Format(lang, i18n.KeyAdapterCommandDisplayRisk, display, risk))
	if next := strings.TrimSpace(presentation.NextAction); next != "" {
		parts = append(parts, i18n.Format(lang, i18n.KeyAdapterCommandNext, next))
	}
	for _, ref := range boundedCommandEvidenceRefs(presentation.EvidenceRefs) {
		parts = append(parts, i18n.Format(lang, i18n.KeyAdapterCommandEvidenceRefs, ref))
	}
	if presentation.HasMore {
		parts = append(parts, i18n.Text(lang, i18n.KeyAdapterCommandMoreRetained))
	}
	if presentation.Sensitive {
		parts = append(parts, i18n.Text(lang, i18n.KeyAdapterCommandSensitiveHidden))
	}
	return strings.Join(nonEmptySemanticStrings(parts), " | ")
}

func renderScreenReaderCommandPresentation(renderer *ui.ScreenReaderRenderer, presentation commands.CommandPresentation) {
	if renderer == nil {
		return
	}
	if presentation.State == commands.CommandStateRunning {
		target := ""
		if strings.TrimSpace(presentation.Target) != "" {
			target = " " + strings.TrimSpace(presentation.Target)
		}
		renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAdapterCommandRunning, presentation.Command, presentation.Action, target))
		return
	}
	renderer.Info(formatCommandPresentationTerminal(presentation))
}

func formatCommandPresentationTerminal(presentation commands.CommandPresentation) string {
	lang := i18n.DetectOrLoadLanguage()
	outcome := string(presentation.Outcome)
	if presentation.Outcome == commands.CommandOutcomeUnknown {
		outcome = i18n.Text(lang, i18n.KeyAdapterCommandUnstructured)
	} else {
		outcome = i18n.CommandOutcomeLabel(lang, outcome)
	}
	line := i18n.Format(lang, i18n.KeyAdapterCommandTerminal, presentation.Command, outcome)
	if result := strings.TrimSpace(presentation.Result); result != "" {
		line += " " + result
	}
	for _, section := range commandPresentationNonMirroredSections(presentation) {
		line += " " + commands.RedactCommandPresentationText(section.Label, 80) + ": " + commands.RedactCommandPresentationText(section.Text, 480)
	}
	display := i18n.CommandDisplayLabel(lang, string(presentation.Display))
	risk := i18n.CommandRiskLabel(lang, string(presentation.Risk))
	line += i18n.Format(lang, i18n.KeyAdapterCommandDisplayRisk, display, risk)
	if next := strings.TrimSpace(presentation.NextAction); next != "" {
		line += i18n.Format(lang, i18n.KeyAdapterCommandNext, next)
	}
	if refs := boundedCommandEvidenceRefs(presentation.EvidenceRefs); len(refs) > 0 {
		line += i18n.Format(lang, i18n.KeyAdapterCommandEvidenceRefs, strings.Join(refs, ", "))
	}
	if presentation.HasMore {
		line += i18n.Text(lang, i18n.KeyAdapterCommandMoreRetained)
	}
	if presentation.Sensitive {
		line += i18n.Text(lang, i18n.KeyAdapterCommandSensitiveHidden)
	}
	return line
}

func commandPresentationNonMirroredSections(presentation commands.CommandPresentation) []commands.CommandPresentationSection {
	if len(presentation.Sections) == 1 {
		lang := i18n.DetectOrLoadLanguage()
		label := strings.TrimSpace(presentation.Sections[0].Label)
		isResult := strings.EqualFold(label, i18n.Text(lang, i18n.KeyCommandPresentationResult))
		if !isResult {
			for _, candidate := range i18n.AllLanguages() {
				if strings.EqualFold(label, i18n.Text(candidate, i18n.KeyCommandPresentationResult)) {
					isResult = true
					break
				}
			}
		}
		if isResult && strings.TrimSpace(presentation.Sections[0].Text) == strings.TrimSpace(presentation.Result) {
			return nil
		}
	}
	return commandPresentationBoundedSections(presentation.Sections)
}

func commandPresentationBoundedSections(sections []commands.CommandPresentationSection) []commands.CommandPresentationSection {
	if len(sections) > 8 {
		sections = sections[:8]
	}
	out := make([]commands.CommandPresentationSection, 0, len(sections))
	for _, section := range sections {
		label := commands.RedactCommandPresentationText(section.Label, 80)
		text := commands.RedactCommandPresentationText(section.Text, 480)
		if strings.TrimSpace(label) != "" && strings.TrimSpace(text) != "" {
			out = append(out, commands.CommandPresentationSection{Label: label, Text: text})
		}
	}
	return out
}

func boundedCommandEvidenceRefs(refs []string) []string {
	if len(refs) > 8 {
		refs = refs[:8]
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref = commands.RedactCommandPresentationText(ref, 240); strings.TrimSpace(ref) != "" {
			out = append(out, ref)
		}
	}
	return out
}

func nonEmptySemanticStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func semanticToolCallPresentation(ctx presentation.ToolEventContext, call types.ToolUseBlock) presentation.ToolPresentation {
	return semanticToolPresentation(ctx, call, tui.OutcomeRunning, nil)
}

func semanticToolResultPresentation(ctx presentation.ToolEventContext, call types.ToolUseBlock, result types.ToolResultBlock) presentation.ToolPresentation {
	outcome := presentationOutcomeForToolResult(result)
	return semanticToolPresentation(ctx, call, outcome, &result)
}

func semanticToolPresentation(ctx presentation.ToolEventContext, call types.ToolUseBlock, outcome tui.ObservationOutcome, result *types.ToolResultBlock) presentation.ToolPresentation {
	lang := i18n.DetectOrLoadLanguage()
	formatted := tui.FormatToolPresentationInLanguage(lang, call.Name, call.Input, outcome, result)
	if formatted.Outcome != tui.OutcomeUnknown {
		outcome = formatted.Outcome
	}
	decision := tui.DecidePresentation(formatted.Facts(outcome))
	details := append([]string(nil), formatted.DetailLines...)
	nextAction := ""
	localizedReviewNext := i18n.Text(lang, i18n.KeyPresentationReviewNext)
	for index := 0; index < len(details); index++ {
		if details[index] == localizedReviewNext {
			nextAction = i18n.Text(lang, i18n.KeyAdapterReviewNext)
			details = append(details[:index], details[index+1:]...)
			break
		}
	}
	reasons := make([]string, len(decision.Reasons))
	for index, reason := range decision.Reasons {
		reasons[index] = string(reason)
	}
	action := tui.SemanticToolActionInLanguage(lang, call.Name)
	return presentation.ToolPresentation{
		ToolName: call.Name, ToolUseID: firstSemanticString(resultToolUseID(result), call.ID),
		WorkUnitID: ctx.WorkUnitID, Actor: ctx.ActorID, Action: action, Object: formatted.Object,
		State: presentationStateForOutcome(outcome), Result: formatted.Summary, NextAction: nextAction,
		DetailLines: details, PresentationLevel: decision.EffectiveLevel.String(), ReasonCodes: reasons,
		HasMore: formatted.HasUsefulDetail, Redacted: decision.Redacted,
	}
}

func presentationOutcomeForToolResult(result types.ToolResultBlock) tui.ObservationOutcome {
	switch result.Outcome {
	case types.ToolOutcomeSucceeded:
		return tui.OutcomeSucceeded
	case types.ToolOutcomeFailed:
		return tui.OutcomeFailed
	case types.ToolOutcomePartial:
		return tui.OutcomePartial
	case types.ToolOutcomeDenied:
		return tui.OutcomeDenied
	case types.ToolOutcomeCancelled:
		return tui.OutcomeCancelled
	case types.ToolOutcomeTimedOut:
		return tui.OutcomeTimedOut
	default:
		if result.IsError {
			return tui.OutcomeFailed
		}
		return tui.OutcomeSucceeded
	}
}

func presentationStateForOutcome(outcome tui.ObservationOutcome) string {
	switch outcome {
	case tui.OutcomeRunning:
		return presentation.ToolPresentationStateRunning
	case tui.OutcomeSucceeded:
		return presentation.ToolPresentationStateSucceeded
	case tui.OutcomeFailed, tui.OutcomeOrphan, tui.OutcomeConflict, tui.OutcomeShutdown:
		return presentation.ToolPresentationStateFailed
	case tui.OutcomePartial:
		return presentation.ToolPresentationStatePartial
	case tui.OutcomeDenied:
		return presentation.ToolPresentationStateDenied
	case tui.OutcomeCancelled, tui.OutcomeEscaped:
		return presentation.ToolPresentationStateCancelled
	case tui.OutcomeTimedOut:
		return presentation.ToolPresentationStateTimedOut
	default:
		return presentation.ToolPresentationStateUnknown
	}
}

func resultToolUseID(result *types.ToolResultBlock) string {
	if result == nil {
		return ""
	}
	return result.ToolUseID
}

func firstSemanticString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
