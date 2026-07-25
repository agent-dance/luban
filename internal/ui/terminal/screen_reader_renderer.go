package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type screenReaderLine struct {
	text     string
	err      error
	sequence uint64
}

type screenReaderInputKind uint8

const (
	screenReaderCommandInput screenReaderInputKind = iota
	screenReaderDecisionInput
)

type screenReaderInputLease struct {
	kind               screenReaderInputKind
	ctx                context.Context
	prompt             string
	decisionID         string
	granted            chan struct{}
	lines              chan screenReaderLine
	nextLine           chan struct{}
	released           chan struct{}
	once               sync.Once
	grantOnce          sync.Once
	activationSequence uint64
}

func newScreenReaderInputLease(ctx context.Context, kind screenReaderInputKind, prompt, decisionID string) *screenReaderInputLease {
	return &screenReaderInputLease{
		kind: kind, ctx: ctx, prompt: prompt, decisionID: decisionID,
		granted: make(chan struct{}), lines: make(chan screenReaderLine, 1), nextLine: make(chan struct{}, 1), released: make(chan struct{}),
	}
}

func (l *screenReaderInputLease) release() {
	if l != nil {
		l.once.Do(func() { close(l.released) })
	}
}

// ScreenReaderRenderer is an append-only terminal surface. It never emits
// cursor movement, alternate-screen, mouse, animation, colour, or overwrite
// sequences; every critical state transition is expressed as linear text.
type ScreenReaderRenderer struct {
	w             io.Writer
	lines         <-chan screenReaderLine
	inputRequests chan *screenReaderInputLease
	inputDone     chan struct{}
	stopCh        chan struct{}
	scannerDone   chan struct{}
	closeOnce     sync.Once
	writeMu       sync.Mutex
	decisionMu    sync.Mutex
	identityMu    sync.RWMutex
	sessionID     func() string
	recorder      func(ScreenReaderDecisionRecord) error
	inputSequence atomic.Uint64
}

type ScreenReaderDecisionRecord struct {
	Prompt     permissions.PromptRequest
	Response   permissions.PromptResponse
	ResolvedAt time.Time
}

func NewScreenReaderRenderer(w io.Writer, input io.Reader) *ScreenReaderRenderer {
	if w == nil {
		w = io.Discard
	}
	lineCh := make(chan screenReaderLine, 128)
	renderer := &ScreenReaderRenderer{
		w: w, lines: lineCh, inputRequests: make(chan *screenReaderInputLease, 4),
		inputDone: make(chan struct{}), stopCh: make(chan struct{}), scannerDone: make(chan struct{}),
	}
	if input == nil {
		close(lineCh)
		close(renderer.scannerDone)
	} else {
		go scanScreenReaderInput(input, lineCh, renderer.stopCh, renderer.scannerDone, &renderer.inputSequence)
	}
	go renderer.runInputArbiter()
	return renderer
}

// Close stops input arbitration and waits for the input owner to release its
// goroutines. Real terminal input uses a cancellable platform poller.
func (r *ScreenReaderRenderer) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() { close(r.stopCh) })
	<-r.inputDone
	select {
	case <-r.scannerDone:
		return nil
	case <-time.After(time.Second):
		return errors.New(i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderStopped))
	}
}

func scanScreenReaderInputGeneric(input io.Reader, lines chan<- screenReaderLine, stop <-chan struct{}, done chan<- struct{}, sequence *atomic.Uint64) {
	defer close(done)
	defer close(lines)
	reader := bufio.NewReader(input)
	emit := func(line screenReaderLine) bool {
		line.sequence = sequence.Add(1)
		select {
		case lines <- line:
			return true
		case <-stop:
			return false
		}
	}
	for {
		text, err := reader.ReadString('\n')
		if len(text) > 0 {
			text = strings.TrimSuffix(text, "\n")
			text = strings.TrimSuffix(text, "\r")
			if !emit(screenReaderLine{text: text}) {
				return
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return
		}
		emit(screenReaderLine{err: err})
		return
	}
}

// cancelScreenReaderReadUntilDone closes the cancel-before-read gap on
// platforms where cancellation can race with submission of a blocking read.
// Once shutdown starts, cancellation is retried until the read loop confirms
// that it has exited.
func cancelScreenReaderReadUntilDone(stop, done <-chan struct{}, cancel func()) {
	select {
	case <-done:
		return
	case <-stop:
	}
	for {
		select {
		case <-done:
			return
		default:
		}
		if cancel != nil {
			cancel()
		}
		timer := time.NewTimer(100 * time.Microsecond)
		select {
		case <-done:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (r *ScreenReaderRenderer) write(format string, args ...any) {
	r.writeMu.Lock()
	_, _ = io.WriteString(r.w, screenReaderSafeText(fmt.Sprintf(format, args...)))
	r.writeMu.Unlock()
}

func (r *ScreenReaderRenderer) ReadCommand(ctx context.Context) (string, error) {
	lease, err := r.acquireInput(ctx, screenReaderCommandInput, r.Prompt(), "")
	if err != nil {
		return "", err
	}
	defer lease.release()
	return lease.readLine(ctx)
}

func (r *ScreenReaderRenderer) SetDecisionRecorder(recorder func(ScreenReaderDecisionRecord) error) {
	r.decisionMu.Lock()
	r.recorder = recorder
	r.decisionMu.Unlock()
}

func (r *ScreenReaderRenderer) SetSessionIdentityResolver(resolver func() string) {
	r.identityMu.Lock()
	r.sessionID = resolver
	r.identityMu.Unlock()
}

func (r *ScreenReaderRenderer) visibleSessionID() string {
	r.identityMu.RLock()
	resolver := r.sessionID
	r.identityMu.RUnlock()
	if resolver == nil {
		return ""
	}
	return strings.TrimSpace(resolver())
}

func (r *ScreenReaderRenderer) acquireInput(ctx context.Context, kind screenReaderInputKind, prompt, decisionID string) (*screenReaderInputLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lease := newScreenReaderInputLease(ctx, kind, prompt, decisionID)
	select {
	case r.inputRequests <- lease:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.inputDone:
		return nil, io.EOF
	}
	select {
	case <-lease.granted:
		if err := ctx.Err(); err != nil {
			lease.release()
			return nil, err
		}
		return lease, nil
	case <-ctx.Done():
		lease.release()
		return nil, ctx.Err()
	case <-r.inputDone:
		lease.release()
		return nil, io.EOF
	}
}

func (l *screenReaderInputLease) readLine(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case line := <-l.lines:
		return line.text, line.err
	}
}

func (l *screenReaderInputLease) requestNextLine() {
	select {
	case l.nextLine <- struct{}{}:
	default:
	}
}

func (r *ScreenReaderRenderer) runInputArbiter() {
	defer close(r.inputDone)
	var active *screenReaderInputLease
	var suspendedCommand *screenReaderInputLease
	var pending []*screenReaderInputLease
	var bufferedCommandLines []screenReaderLine
	lineDelivered := false
	inputLines := r.lines
	inputClosed := false

	isClosed := func(ch <-chan struct{}) bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	valid := func(lease *screenReaderInputLease) bool {
		return lease != nil && lease.ctx.Err() == nil && !isClosed(lease.released)
	}
	activate := func(lease *screenReaderInputLease, resumed bool) bool {
		if !valid(lease) {
			return false
		}
		active = lease
		lease.activationSequence = r.inputSequence.Load()
		if resumed {
			r.write("%s", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderCommandResume))
		}
		r.write("%s", lease.prompt)
		lease.grantOnce.Do(func() { close(lease.granted) })
		return true
	}
	activateNext := func() {
		if active != nil {
			return
		}
		if suspendedCommand != nil {
			candidate := suspendedCommand
			suspendedCommand = nil
			if activate(candidate, true) {
				return
			}
		}
		for len(pending) > 0 {
			candidate := pending[0]
			pending = pending[1:]
			if activate(candidate, false) {
				return
			}
		}
	}
	handleRequest := func(lease *screenReaderInputLease) {
		if !valid(lease) {
			return
		}
		if active == nil {
			activate(lease, false)
			return
		}
		if lease.kind == screenReaderDecisionInput && active.kind == screenReaderCommandInput && suspendedCommand == nil {
			if !lineDelivered && valid(active) {
				suspendedCommand = active
			}
			active = nil
			lineDelivered = false
			r.write("%s", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderCommandPause))
			activate(lease, false)
			return
		}
		pending = append(pending, lease)
	}
	reserveCommandLine := func(line screenReaderLine) {
		const maxBufferedCommandLines = 128
		if len(bufferedCommandLines) >= maxBufferedCommandLines {
			r.write("%s", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderQueueFull))
			return
		}
		bufferedCommandLines = append(bufferedCommandLines, line)
	}
	deliverBufferedLine := func() {
		if active == nil || lineDelivered {
			return
		}
		index := -1
		switch active.kind {
		case screenReaderCommandInput:
			if len(bufferedCommandLines) > 0 {
				index = 0
			}
		}
		if index >= 0 {
			line := bufferedCommandLines[index]
			bufferedCommandLines = append(bufferedCommandLines[:index], bufferedCommandLines[index+1:]...)
			active.lines <- line
			lineDelivered = true
			return
		}
		if inputClosed {
			active.lines <- screenReaderLine{err: io.EOF}
			lineDelivered = true
		}
	}

	for {
		activateNext()
		deliverBufferedLine()
		if inputClosed && len(bufferedCommandLines) == 0 && active == nil && suspendedCommand == nil && len(pending) == 0 {
			return
		}
		var lineSource <-chan screenReaderLine
		var released <-chan struct{}
		var cancelled <-chan struct{}
		var nextLine <-chan struct{}
		if active != nil {
			released = active.released
			cancelled = active.ctx.Done()
			nextLine = active.nextLine
			if !inputClosed && !lineDelivered && (active.kind == screenReaderDecisionInput || len(bufferedCommandLines) == 0) {
				lineSource = inputLines
			}
		} else if !inputClosed {
			lineSource = inputLines
		}
		select {
		case <-r.stopCh:
			leases := append([]*screenReaderInputLease{active, suspendedCommand}, pending...)
			for _, lease := range leases {
				if lease == nil {
					continue
				}
				lease.grantOnce.Do(func() { close(lease.granted) })
				select {
				case lease.lines <- screenReaderLine{err: io.EOF}:
				default:
				}
			}
			return
		case lease := <-r.inputRequests:
			handleRequest(lease)
		case line, ok := <-lineSource:
			if !ok {
				inputClosed = true
				inputLines = nil
				line = screenReaderLine{err: io.EOF}
			}
			if active != nil {
				if active.kind == screenReaderDecisionInput && line.err == nil {
					if line.sequence != 0 && line.sequence <= active.activationSequence {
						reserveCommandLine(line)
						r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderReservedEarly, active.decisionID))
						continue
					}
					_, scoped := screenReaderDecisionPayload(line.text, active.decisionID)
					if !scoped {
						reserveCommandLine(line)
						r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderReservedScope, active.decisionID, active.decisionID))
						continue
					}
				}
				active.lines <- line
				lineDelivered = true
			} else if line.err == nil {
				reserveCommandLine(line)
			}
		case <-released:
			active = nil
			lineDelivered = false
		case <-cancelled:
			active = nil
			lineDelivered = false
		case <-nextLine:
			if active != nil && active.kind == screenReaderDecisionInput {
				lineDelivered = false
			}
		}
	}
}

func (r *ScreenReaderRenderer) Text(s string)   { r.write("%s", s) }
func (r *ScreenReaderRenderer) Thinking(string) {}
func screenReaderLanguage() i18n.Language       { return i18n.DetectOrLoadLanguage() }

func (r *ScreenReaderRenderer) Error(s string) {
	r.write("%s%s\n", i18n.Text(screenReaderLanguage(), i18n.KeyTerminalErrorPrefix), s)
}
func (r *ScreenReaderRenderer) Info(s string) {
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderInfo, s))
}
func (r *ScreenReaderRenderer) Success(s string) {
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderSuccess, s))
}
func (r *ScreenReaderRenderer) Warning(s string) {
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderWarning, s))
}
func (r *ScreenReaderRenderer) Bold(s string) { r.write("%s\n", s) }
func (r *ScreenReaderRenderer) Newline()      { r.write("\n") }
func (r *ScreenReaderRenderer) Goodbye() {
	r.write("%s\n", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderClosed))
}
func (r *ScreenReaderRenderer) Prompt() string {
	return i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderInput)
}
func (r *ScreenReaderRenderer) SpinnerStart(string) func() { return func() {} }

func (r *ScreenReaderRenderer) Banner(provider, model string) {
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderBanner, provider, model))
}

func (r *ScreenReaderRenderer) SessionInfo(id string, tools []string) {
	lang := screenReaderLanguage()
	r.write("%s.\n", i18n.Format(lang, i18n.KeyTerminalSession, id))
	r.write("%s", i18n.Format(lang, i18n.KeyScreenReaderTools, strings.Join(tools, ", ")))
	r.write("%s", i18n.Text(lang, i18n.KeyScreenReaderHelp))
}

func (r *ScreenReaderRenderer) RenderToolCall(ctx presentation.ToolEventContext, call types.ToolUseBlock) {
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderToolStarted, call.Name, call.ID, ctx.SessionID, ctx.ProjectRoot, ctx.TurnID, ctx.WorkUnitID, ctx.ActorID, ctx.ActorType))
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderToolInput, screenReaderJSON(call.Input)))
}

func (r *ScreenReaderRenderer) RenderToolResult(ctx presentation.ToolEventContext, result types.ToolResultBlock) {
	lang := screenReaderLanguage()
	event := types.NewToolResultRuntimeEvent(ctx.RuntimeIdentity(result.ToolUseID), result, i18n.KeyRuntimeToolResultPublicSummary, nil)
	projection, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
		Language: lang, LanguageSet: true,
	})
	if err != nil {
		r.RuntimeErrorEvent(ctx, result.ToolUseID, "", nil, nil)
		return
	}
	r.write("%s", i18n.Format(lang, i18n.KeyScreenReaderToolFinished, result.ToolUseID,
		i18n.RuntimeActivityStateLabel(lang, string(projection.Outcome)), ctx.SessionID, ctx.ProjectRoot, ctx.TurnID, ctx.WorkUnitID, ctx.ActorID, ctx.ActorType))
	if content := result.TextContent(); content != "" {
		r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderEvidence, content))
	}
}

func (r *ScreenReaderRenderer) RenderHookSummary(ctx presentation.ToolEventContext, summary presentation.HookSummary) {
	lang := screenReaderLanguage()
	r.write("%s", i18n.Format(lang, i18n.KeyScreenReaderHookFinished, summary.Name, summary.ExecutionID, summary.ToolUseID,
		i18n.RuntimeActivityStateLabel(lang, summary.Status), ctx.SessionID, ctx.ProjectRoot, ctx.TurnID, ctx.WorkUnitID, ctx.ActorID, ctx.ActorType))
	if summary.Summary != "" {
		r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderHookSummary, summary.Summary))
	}
	if len(summary.Metadata) > 0 {
		r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderHookEvidence, screenReaderJSON(summary.Metadata)))
	}
}

// RuntimeErrorEvent uses the same public projection as the visual TUI. Raw API
// errors, metadata, paths, and correlation identifiers remain private
// diagnostics and are never announced by default.
func (r *ScreenReaderRenderer) RuntimeErrorEvent(ctx presentation.ToolEventContext, toolUseID, message string, apiError *types.APIError, metadata map[string]any) {
	lang := screenReaderLanguage()
	event := presentation.NewRuntimeErrorEvent(ctx, toolUseID, message, apiError, metadata)
	projection, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
		Language: lang, LanguageSet: true,
	})
	publicMessage := i18n.Text(lang, i18n.KeyRuntimeErrorPublicSummary)
	if err == nil {
		publicMessage = projection.Message
	}
	r.write("%s", i18n.Format(lang, i18n.KeyScreenReaderRuntimeError, publicMessage))
}

func (r *ScreenReaderRenderer) RenderSendUserMessageEvent(_ presentation.ToolEventContext, output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions) {
	if text := presentation.FormatSendUserMessage(output, options); text != "" {
		r.write("%s\n", text)
	}
}

func (r *ScreenReaderRenderer) Usage(u *types.Usage) {
	if u == nil {
		return
	}
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderUsage, u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens))
}

// UsageSemantics announces the same session and context accounting scopes as
// the visual surface. Last-request usage remains structured data only and is
// never substituted for the session segment.
func (r *ScreenReaderRenderer) UsageSemantics(snapshot presentation.UsageSemanticsSnapshot) {
	lang := screenReaderLanguage()
	if snapshot.CumulativeSession.Known {
		r.write("%s\n", FormatSessionUsage(lang, snapshot.CumulativeSession))
	} else {
		r.write("%s\n", i18n.Text(lang, i18n.KeyUsageSessionUnavailable))
	}
	if snapshot.ModelContext.Known {
		r.ModelContext(snapshot.ModelContext)
	} else {
		r.write("%s\n", i18n.Text(lang, i18n.KeyUsageContextUnknown))
	}
}

func (r *ScreenReaderRenderer) ModelContext(context presentation.ModelContextProjection) {
	lang := screenReaderLanguage()
	if !context.Known || context.CapacityTokens <= 0 || context.Measurement == presentation.ContextMeasurementUnknown || context.Measurement == "" {
		r.write("%s\n", i18n.Text(lang, i18n.KeyUsageContextUnknown))
		return
	}
	key := i18n.KeyUsageContextPlain
	switch context.Measurement {
	case presentation.ContextMeasurementLocalEstimate:
		key = i18n.KeyUsageContextEstimatedPlain
	case presentation.ContextMeasurementLocalLowerBound:
		key = i18n.KeyUsageContextLowerBoundPlain
	}
	r.write("%s\n", i18n.Format(lang, key, context.PercentUsed, fmtK(context.UsedTokens), fmtK(context.CapacityTokens)))
}

func (r *ScreenReaderRenderer) CostSummary(turnCost, cumulativeCost float64, inputTokens, outputTokens int) {
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderCost, turnCost, cumulativeCost, inputTokens, outputTokens))
}

func (r *ScreenReaderRenderer) ContextBar(usedTokens, maxTokens int) {
	percent := 0
	if maxTokens > 0 {
		percent = usedTokens * 100 / maxTokens
	}
	r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderContext, usedTokens, maxTokens, percent))
}

func (r *ScreenReaderRenderer) DecisionRequest(ctx context.Context, request permissions.PromptRequest) permissions.PromptResponse {
	r.decisionMu.Lock()
	defer r.decisionMu.Unlock()
	request = cloneScreenReaderPrompt(request)
	if request.Kind == permissions.PromptKindAskUser {
		return r.askUserDecisionRequest(ctx, request)
	}
	var review strings.Builder
	lang := screenReaderLanguage()
	review.WriteString(i18n.Format(lang, i18n.KeyScreenReaderDecision, request.DecisionID,
		i18n.RuntimePromptKindLabel(lang, string(request.Kind)), request.ActorID, request.ActorType, request.WorkUnitID))
	if request.ExecutionSessionID != "" {
		review.WriteString(i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderExecution, request.ExecutionSessionID))
	}
	review.WriteString(i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderAction, request.Action, request.Target, request.Impact, request.RiskLevel, request.RiskReason, request.RuleSource, request.ApprovalScope))
	if request.Body != "" {
		review.WriteString(i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderReviewBody, request.Body))
	}
	for index, detail := range request.ReviewDetails {
		review.WriteString(i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderReviewDetail, index+1, detail))
	}
	if request.PostMode != "" {
		review.WriteString(i18n.Format(lang, i18n.KeyScreenReaderPostMode, i18n.RuntimeModeLabel(lang, request.PostMode)))
	}
	choices := append([]string(nil), request.Choices...)
	if len(choices) == 0 {
		choices = []string{"allow_once", "reject", "always_allow"}
	}
	for index, choice := range choices {
		label := i18n.RuntimeDecisionChoiceLabel(lang, choice)
		if label != choice {
			label += " (" + choice + ")"
		}
		review.WriteString(i18n.Format(lang, i18n.KeyScreenReaderChoice, index+1, label))
	}
	decisionPrompt := i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderPrompt, request.DecisionID)
	lease, err := r.acquireInput(ctx, screenReaderDecisionInput, review.String()+decisionPrompt, request.DecisionID)
	if err != nil {
		return r.resolveInterruptedDecision(request, err)
	}
	defer lease.release()
	for {
		line, err := lease.readLine(ctx)
		if err != nil {
			return r.resolveInterruptedDecision(request, err)
		}
		payload, scoped := screenReaderDecisionPayload(line, request.DecisionID)
		choice, outcome, ok := screenReaderChoice(payload, choices)
		if !scoped || !ok {
			r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderInvalidChoice, request.DecisionID, decisionPrompt))
			lease.requestNextLine()
			continue
		}
		response := screenReaderDecisionResponse(request.DecisionID, outcome, choice)
		return r.commitDecision(request, response)
	}
}

func (r *ScreenReaderRenderer) AskUserQuestions(ctx context.Context, request interaction.AskUserInteractionRequest) (interaction.AskUserInteractionResponse, error) {
	result := interaction.AskUserInteractionResponse{RequestID: request.RequestID, Outcome: interaction.AskUserInteractionCancelled}
	if visible := r.visibleSessionID(); visible != "" && visible != strings.TrimSpace(request.SessionID) {
		result.Outcome = interaction.AskUserInteractionStale
		return result, nil
	}
	questionnaire := &permissions.AskUserQuestionnaire{Questions: make([]permissions.AskUserQuestion, len(request.Questions))}
	for index, question := range request.Questions {
		converted := permissions.AskUserQuestion{Question: question.Question, Header: question.Header, MultiSelect: question.MultiSelect, Options: make([]permissions.AskUserOption, len(question.Options))}
		for optionIndex, option := range question.Options {
			converted.Options[optionIndex] = permissions.AskUserOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
		}
		questionnaire.Questions[index] = converted
	}
	response := r.DecisionRequest(ctx, permissions.PromptRequest{
		DecisionID: request.RequestID, SessionID: request.SessionID, TurnID: request.TurnID, ToolUseID: request.ToolUseID,
		ToolName: "AskUserQuestion", ActorID: request.ActorID, ActorType: request.ActorType, WorkUnitID: request.WorkUnitID,
		Kind: permissions.PromptKindAskUser, Action: i18n.Text(screenReaderLanguage(), i18n.KeyToolActionAskUser), Questionnaire: questionnaire,
	})
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if visible := r.visibleSessionID(); visible != "" && visible != strings.TrimSpace(request.SessionID) {
		result.Outcome = interaction.AskUserInteractionStale
		return result, nil
	}
	if response.Outcome == permissions.PromptOutcomeShutdown {
		result.Outcome = interaction.AskUserInteractionShutdown
		return result, nil
	}
	if response.Outcome != permissions.PromptOutcomeApproved || response.Questionnaire == nil {
		return result, nil
	}
	result.Answers = make(map[string]interaction.AnswerSelection, len(response.Questionnaire.Answers))
	result.Annotations = make(map[string]interaction.AnnotationEntry)
	for _, question := range request.Questions {
		answer, ok := response.Questionnaire.Answers[question.Question]
		if !ok {
			continue
		}
		result.Answers[question.Question] = interaction.AnswerSelection{Selection: append([]string(nil), answer.Selection...), OtherText: answer.OtherText}
		annotation := interaction.AnnotationEntry{Notes: answer.Notes}
		if !question.MultiSelect && len(answer.Selection) == 1 {
			for _, option := range question.Options {
				if option.Label == answer.Selection[0] {
					annotation.Preview = option.Preview
					break
				}
			}
		}
		if annotation.Notes != "" || annotation.Preview != "" {
			result.Annotations[question.Question] = annotation
		}
	}
	result.Outcome = interaction.AskUserInteractionCompleted
	return result, nil
}

func (r *ScreenReaderRenderer) askUserDecisionRequest(ctx context.Context, request permissions.PromptRequest) permissions.PromptResponse {
	if request.Questionnaire == nil || len(request.Questionnaire.Questions) == 0 {
		return r.commitDecision(request, screenReaderDecisionResponse(request.DecisionID, permissions.PromptOutcomeRejected, ""))
	}
	answers := make(map[string]permissions.AskUserAnswer, len(request.Questionnaire.Questions))
	lang := screenReaderLanguage()
	for index, question := range request.Questionnaire.Questions {
		promptID := request.DecisionID + ":" + strconv.Itoa(index+1)
		var review strings.Builder
		review.WriteString(i18n.Format(lang, i18n.KeyAskUserProgress, index+1, len(request.Questionnaire.Questions)) + "\n")
		review.WriteString("[" + question.Header + "] " + question.Question + "\n")
		for optionIndex, option := range question.Options {
			review.WriteString(fmt.Sprintf("%d. %s — %s\n", optionIndex+1, option.Label, option.Description))
			if option.Preview != "" {
				review.WriteString(option.Preview + "\n")
			}
		}
		review.WriteString(i18n.Text(lang, i18n.KeyAskUserOtherOption) + "\n")
		if question.MultiSelect {
			review.WriteString(i18n.Text(lang, i18n.KeyAskUserMultiPrompt))
		} else {
			review.WriteString(i18n.Format(lang, i18n.KeyAskUserSinglePrompt, len(question.Options)))
		}
		decisionPrompt := i18n.Format(lang, i18n.KeyScreenReaderAskUserPrompt, promptID)
		lease, err := r.acquireInput(ctx, screenReaderDecisionInput, review.String()+decisionPrompt, promptID)
		if err != nil {
			return r.resolveInterruptedDecision(request, err)
		}
		for {
			line, readErr := lease.readLine(ctx)
			if readErr != nil {
				lease.release()
				return r.resolveInterruptedDecision(request, readErr)
			}
			payload, scoped := screenReaderDecisionPayload(line, promptID)
			if !scoped {
				r.write("%s", i18n.Format(lang, i18n.KeyScreenReaderAskUserInvalid, promptID, decisionPrompt))
				lease.requestNextLine()
				continue
			}
			if strings.EqualFold(payload, "escape") || strings.EqualFold(payload, "esc") || strings.EqualFold(payload, "cancel") {
				lease.release()
				return r.commitDecision(request, screenReaderDecisionResponse(request.DecisionID, permissions.PromptOutcomeEscaped, ""))
			}
			answer, ok := parseScreenReaderAskUserAnswer(payload, question)
			if !ok {
				r.write("%s", i18n.Format(lang, i18n.KeyScreenReaderAskUserInvalid, promptID, decisionPrompt))
				lease.requestNextLine()
				continue
			}
			answers[question.Question] = answer
			lease.release()
			break
		}
	}
	response := screenReaderDecisionResponse(request.DecisionID, permissions.PromptOutcomeApproved, "submit")
	response.Questionnaire = &permissions.AskUserQuestionnaireResponse{Answers: answers}
	return r.commitDecision(request, response)
}

func parseScreenReaderAskUserAnswer(payload string, question permissions.AskUserQuestion) (permissions.AskUserAnswer, bool) {
	payload, notes := screenReaderAskUserNotes(payload)
	parts := []string{payload}
	if question.MultiSelect {
		parts = strings.Split(payload, ",")
	}
	answer := permissions.AskUserAnswer{Notes: notes}
	seen := make(map[string]struct{})
	for _, raw := range parts {
		piece := strings.TrimSpace(raw)
		if piece == "" {
			continue
		}
		lower := strings.ToLower(piece)
		if strings.HasPrefix(lower, "other:") || strings.HasPrefix(lower, "o:") {
			colon := strings.Index(piece, ":")
			answer.OtherText = strings.TrimSpace(piece[colon+1:])
			if answer.OtherText == "" {
				return permissions.AskUserAnswer{}, false
			}
			continue
		}
		matched := ""
		if number, err := strconv.Atoi(piece); err == nil && number >= 1 && number <= len(question.Options) {
			matched = question.Options[number-1].Label
		} else {
			for _, option := range question.Options {
				if strings.EqualFold(strings.TrimSpace(option.Label), piece) {
					matched = option.Label
					break
				}
			}
		}
		if matched == "" {
			return permissions.AskUserAnswer{}, false
		}
		if _, duplicate := seen[matched]; !duplicate {
			seen[matched] = struct{}{}
			answer.Selection = append(answer.Selection, matched)
		}
	}
	count := len(answer.Selection)
	if answer.OtherText != "" {
		count++
	}
	return answer, count > 0 && (question.MultiSelect || count == 1)
}

func screenReaderAskUserNotes(payload string) (string, string) {
	marker := " n:"
	index := strings.LastIndex(strings.ToLower(payload), marker)
	if index < 0 {
		return strings.TrimSpace(payload), ""
	}
	return strings.TrimSpace(payload[:index]), strings.TrimSpace(payload[index+len(marker):])
}

func screenReaderDecisionPayload(input, decisionID string) (string, bool) {
	prefix := "decision " + decisionID + " "
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, prefix) {
		return "", false
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	return payload, payload != ""
}

func (r *ScreenReaderRenderer) resolveInterruptedDecision(request permissions.PromptRequest, err error) permissions.PromptResponse {
	outcome := permissions.PromptOutcomeShutdown
	if errors.Is(err, context.DeadlineExceeded) {
		outcome = permissions.PromptOutcomeTimedOut
	} else if errors.Is(err, context.Canceled) {
		outcome = permissions.PromptOutcomeCancelled
	}
	return r.commitDecision(request, screenReaderDecisionResponse(request.DecisionID, outcome, ""))
}

func (r *ScreenReaderRenderer) commitDecision(request permissions.PromptRequest, response permissions.PromptResponse) permissions.PromptResponse {
	if r.recorder != nil {
		record := ScreenReaderDecisionRecord{Prompt: request, Response: response, ResolvedAt: time.Now()}
		if err := r.recorder(record); err != nil {
			r.write("%s", i18n.Format(screenReaderLanguage(), i18n.KeyScreenReaderAuditFailed, err))
			if response.Outcome == permissions.PromptOutcomeApproved {
				response = screenReaderDecisionResponse(request.DecisionID, permissions.PromptOutcomeRejected, "")
				r.write("%s", i18n.Text(screenReaderLanguage(), i18n.KeyScreenReaderAuditBlocked))
			}
		}
	}
	r.decisionReceipt(response)
	return response
}

func (r *ScreenReaderRenderer) decisionReceipt(response permissions.PromptResponse) {
	lang := screenReaderLanguage()
	choice := response.Choice
	if choice == "" {
		choice = "none"
	}
	r.write("%s", i18n.Format(lang, i18n.KeyScreenReaderReceipt, response.DecisionID,
		i18n.RuntimeDecisionOutcomeLabel(lang, string(response.Outcome)), i18n.RuntimeDecisionChoiceLabel(lang, choice)))
}

func cloneScreenReaderPrompt(request permissions.PromptRequest) permissions.PromptRequest {
	if request.Input != nil {
		if data, err := json.Marshal(request.Input); err == nil {
			var cloned map[string]any
			if json.Unmarshal(data, &cloned) == nil {
				request.Input = cloned
			}
		}
	}
	request.Choices = append([]string(nil), request.Choices...)
	request.ReviewDetails = append([]string(nil), request.ReviewDetails...)
	request.Questionnaire = cloneScreenReaderAskUserQuestionnaire(request.Questionnaire)
	return request
}

func cloneScreenReaderAskUserQuestionnaire(questionnaire *permissions.AskUserQuestionnaire) *permissions.AskUserQuestionnaire {
	if questionnaire == nil {
		return nil
	}
	cloned := &permissions.AskUserQuestionnaire{Questions: make([]permissions.AskUserQuestion, len(questionnaire.Questions))}
	for index, question := range questionnaire.Questions {
		cloned.Questions[index] = question
		cloned.Questions[index].Options = append([]permissions.AskUserOption(nil), question.Options...)
	}
	return cloned
}

func screenReaderChoice(input string, choices []string) (string, permissions.PromptOutcome, bool) {
	lower := strings.ToLower(input)
	if lower == "escape" || lower == "esc" {
		return "", permissions.PromptOutcomeEscaped, true
	}
	if index, err := strconv.Atoi(lower); err == nil && index >= 1 && index <= len(choices) {
		return screenReaderChoiceOutcome(choices[index-1])
	}
	for _, choice := range choices {
		if lower == strings.ToLower(choice) {
			return screenReaderChoiceOutcome(choice)
		}
	}
	switch lower {
	case "y", "yes":
		for _, choice := range choices {
			if choice == "allow_once" || choice == "execute" {
				return screenReaderChoiceOutcome(choice)
			}
		}
	case "a", "always":
		for _, choice := range choices {
			if choice == "always_allow" {
				return screenReaderChoiceOutcome(choice)
			}
		}
	case "n", "no":
		for _, choice := range choices {
			if choice == "reject" || choice == "stay_in_plan" {
				return screenReaderChoiceOutcome(choice)
			}
		}
	}
	return "", "", false
}

func screenReaderChoiceOutcome(choice string) (string, permissions.PromptOutcome, bool) {
	if choice == "reject" || choice == "stay_in_plan" {
		return choice, permissions.PromptOutcomeRejected, true
	}
	return choice, permissions.PromptOutcomeApproved, true
}

func screenReaderDecisionResponse(id string, outcome permissions.PromptOutcome, choice string) permissions.PromptResponse {
	response := permissions.PromptResponse{DecisionID: id, Outcome: outcome, Choice: choice, Decision: permissions.DecisionDeny}
	if outcome == permissions.PromptOutcomeApproved {
		switch choice {
		case "always_allow":
			response.Decision = permissions.DecisionAllow
		case "allow_once", "execute":
			response.Decision = permissions.DecisionAllowOnce
		}
	}
	return response
}

func screenReaderJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func screenReaderSafeText(value string) string {
	var safe strings.Builder
	for _, char := range value {
		switch {
		case char == '\n':
			safe.WriteByte('\n')
		case char == '\r':
			safe.WriteString(`\r`)
		case char == '\t':
			safe.WriteString(`\t`)
		case char == 0x1b:
			safe.WriteString(`\x1b`)
		case char < 0x20 || char == 0x7f:
			fmt.Fprintf(&safe, `\x%02x`, char)
		case char >= 0x80 && char <= 0x9f:
			fmt.Fprintf(&safe, `\u%04x`, char)
		default:
			safe.WriteRune(char)
		}
	}
	return safe.String()
}
