// Package compact implements full, partial, automatic, and tool-result context
// compaction together with the trusted controls used to install their output.
package compact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tokenizer"
	"github.com/agent-dance/luban/types"
)

// TokenCounter estimates token count for text
type TokenCounter interface {
	Count(text string) int
}

// CalibratedCounter dynamically adjusts its chars-per-token ratio based on
// actual API Usage data. It starts with a default ratio and converges toward
// the real ratio as more data points arrive. Safe for concurrent use.
type CalibratedCounter struct {
	mu           sync.RWMutex
	defaultRatio float64 // initial chars-per-token (default 4.0 for English)
	ratio        float64 // current calibrated ratio
	totalChars   int64   // accumulated character count
	totalTokens  int64   // accumulated token count from API
}

// NewCalibratedCounter creates a counter with the given default chars-per-token
// ratio. Use 4.0 for mostly-English, 2.5 for mixed CJK/English content.
func NewCalibratedCounter(defaultRatio float64) *CalibratedCounter {
	if defaultRatio <= 0 {
		defaultRatio = 4.0
	}
	return &CalibratedCounter{
		defaultRatio: defaultRatio,
		ratio:        defaultRatio,
	}
}

// Count estimates the token count for the given text using the current
// calibrated ratio.
func (c *CalibratedCounter) Count(text string) int {
	c.mu.RLock()
	ratio := c.ratio
	c.mu.RUnlock()
	if ratio <= 0 {
		return len(text) / 4
	}
	return int(float64(len(text)) / ratio)
}

// Calibrate updates the ratio based on observed API usage. Call this after
// each API response with the actual text that was sent and the token count
// reported by the API.
func (c *CalibratedCounter) Calibrate(chars int, tokens int) {
	if tokens <= 0 || chars <= 0 {
		return
	}
	c.mu.Lock()
	c.totalChars += int64(chars)
	c.totalTokens += int64(tokens)
	c.ratio = float64(c.totalChars) / float64(c.totalTokens)
	c.mu.Unlock()
}

// Ratio returns the current chars-per-token ratio.
func (c *CalibratedCounter) Ratio() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ratio
}

const (
	// AutocompactBufferTokens is the safety margin subtracted from the
	// effective context window to trigger compaction.
	AutocompactBufferTokens = 13000

	// WarningThresholdBufferTokens is the output-token reservation subtracted
	// from MaxTokens to compute the effective (input-only) context window.
	WarningThresholdBufferTokens = 20000

	// MaxConsecutiveAutocompactFailures is the circuit-breaker limit. After
	// this many consecutive auto-compact failures the engine stops retrying
	// and leaves the current model view intact.
	MaxConsecutiveAutocompactFailures = 3
)

// ContextWindow tracks token usage and thresholds
type ContextWindow struct {
	MaxTokens       int
	MaxOutputTokens int // max tokens per response; 0 = use WarningThresholdBufferTokens
	UsedInput       int
	UsedOutput      int
	CacheRead       int // tokens read from cache
	CacheCreated    int // tokens written to cache
	Counter         TokenCounter

	usageMu             sync.RWMutex
	providerUsageKnown  bool
	requestEstimate     ModelContextTokenEstimate
	requestEstimateLive bool

	// consecutiveFailures counts auto-compact failures for the circuit breaker.
	consecutiveFailures int

	// suppressWarning hides compact warnings immediately after full compact or
	// microcompact success until the next provider usage update.
	suppressWarning bool
}

// CompactionTrackerSnapshot is an opaque checkpoint of ContextWindow state.
// Installation and persistence transactions use it to ensure a failed
// compaction cannot change either warnings or the visible context estimate.
type CompactionTrackerSnapshot struct {
	consecutiveFailures int
	suppressWarning     bool
	usedInput           int
	usedOutput          int
	cacheRead           int
	cacheCreated        int
	providerUsageKnown  bool
	requestEstimate     ModelContextTokenEstimate
	requestEstimateLive bool
}

// CaptureCompactionTracker snapshots warning state and the current context
// measurement so a failed installation can restore the exact prior view.
func (cw *ContextWindow) CaptureCompactionTracker() CompactionTrackerSnapshot {
	if cw == nil {
		return CompactionTrackerSnapshot{}
	}
	cw.usageMu.RLock()
	defer cw.usageMu.RUnlock()
	estimate := cw.requestEstimate
	estimate.Overheads = append([]TokenOverheadEstimate(nil), cw.requestEstimate.Overheads...)
	estimate.UnknownOverheads = append([]TokenOverheadKind(nil), cw.requestEstimate.UnknownOverheads...)
	return CompactionTrackerSnapshot{
		consecutiveFailures: cw.consecutiveFailures,
		suppressWarning:     cw.suppressWarning,
		usedInput:           cw.UsedInput,
		usedOutput:          cw.UsedOutput,
		cacheRead:           cw.CacheRead,
		cacheCreated:        cw.CacheCreated,
		providerUsageKnown:  cw.providerUsageKnown,
		requestEstimate:     estimate,
		requestEstimateLive: cw.requestEstimateLive,
	}
}

// RestoreCompactionTracker restores a checkpoint captured before an
// installation transaction. The snapshot is intentionally opaque outside the
// compact package so callers cannot synthesize circuit-breaker state.
func (cw *ContextWindow) RestoreCompactionTracker(snapshot CompactionTrackerSnapshot) {
	if cw == nil {
		return
	}
	cw.usageMu.Lock()
	cw.consecutiveFailures = snapshot.consecutiveFailures
	cw.suppressWarning = snapshot.suppressWarning
	cw.UsedInput = snapshot.usedInput
	cw.UsedOutput = snapshot.usedOutput
	cw.CacheRead = snapshot.cacheRead
	cw.CacheCreated = snapshot.cacheCreated
	cw.providerUsageKnown = snapshot.providerUsageKnown
	cw.requestEstimate = snapshot.requestEstimate
	cw.requestEstimate.Overheads = append([]TokenOverheadEstimate(nil), snapshot.requestEstimate.Overheads...)
	cw.requestEstimate.UnknownOverheads = append([]TokenOverheadKind(nil), snapshot.requestEstimate.UnknownOverheads...)
	cw.requestEstimateLive = snapshot.requestEstimateLive
	cw.usageMu.Unlock()
}

// NewContextWindow creates a context window tracker with a tokenizer counter.
func NewContextWindow(maxTokens int) *ContextWindow {
	return &ContextWindow{
		MaxTokens: maxTokens,
		Counter:   tokenizer.NewTiktokenCounter(),
	}
}

// UpdateUsage updates token counts from API response
func (cw *ContextWindow) UpdateUsage(usage *types.Usage) {
	if usage != nil {
		cw.usageMu.Lock()
		defer cw.usageMu.Unlock()
		cw.UsedInput = usage.TotalInputTokens()
		cw.UsedOutput = usage.OutputTokens
		cw.CacheRead = usage.CacheReadInputTokens
		cw.CacheCreated = usage.CacheCreationInputTokens
		cw.providerUsageKnown = true
		cw.requestEstimateLive = false
		cw.suppressWarning = false
	}
}

// UpdateLocalEstimate publishes the most recent complete estimate or known
// lower bound until a provider usage report supersedes it.
func (cw *ContextWindow) UpdateLocalEstimate(estimate ModelContextTokenEstimate) {
	if cw == nil {
		return
	}
	cw.usageMu.Lock()
	cw.requestEstimate = estimate
	cw.requestEstimate.Overheads = append([]TokenOverheadEstimate(nil), estimate.Overheads...)
	cw.requestEstimate.UnknownOverheads = append([]TokenOverheadKind(nil), estimate.UnknownOverheads...)
	cw.requestEstimate.KnownTotalTokens = max(cw.requestEstimate.KnownTotalTokens, 0)
	cw.requestEstimateLive = true
	cw.providerUsageKnown = false
	cw.usageMu.Unlock()
}

// UpdatePostCompactUsage immediately replaces the pre-compact provider value
// with the successful boundary's complete local estimate.
func (cw *ContextWindow) UpdatePostCompactUsage(inputTokens int) {
	if cw == nil {
		return
	}
	if inputTokens <= 0 {
		// A successful boundary invalidates the pre-compact provider value. When
		// neither post-compact count is available, expose unknown rather than a
		// fabricated zero or the stale pre-compact measurement.
		cw.usageMu.Lock()
		cw.UsedInput = 0
		cw.UsedOutput = 0
		cw.CacheRead = 0
		cw.CacheCreated = 0
		cw.providerUsageKnown = false
		cw.requestEstimate = ModelContextTokenEstimate{}
		cw.requestEstimateLive = false
		cw.usageMu.Unlock()
		return
	}
	cw.UpdateLocalEstimate(ModelContextTokenEstimate{
		KnownTotalTokens: inputTokens,
		Complete:         true,
	})
}

// effectiveContextWindowSize returns the input-only budget after reserving
// space for the model's output.  Matches getEffectiveContextWindowSize() in
// the original TS:
//
//	effectiveContextWindow = contextWindow - min(maxOutputTokens, 20_000)
func (cw *ContextWindow) effectiveContextWindowSize() int {
	return EffectiveInputWindowSize(cw.MaxTokens, cw.MaxOutputTokens)
}

// autoCompactThreshold returns the input-token count at which compaction
// should trigger.  Matches getAutoCompactThreshold() in the original TS:
//
//	threshold = effectiveContextWindow - AUTOCOMPACT_BUFFER_TOKENS
func (cw *ContextWindow) autoCompactThreshold() int {
	t := AutoCompactThresholdForWindow(cw.effectiveContextWindowSize())
	if override := autoCompactPercentOverride(cw.effectiveContextWindowSize()); override > 0 && override < t {
		return override
	}
	return t
}

// AutoCompactThreshold exposes the active input-side threshold to projection
// admission. It includes the same model-window and experimental overrides as
// semantic auto-compaction, so the two decisions cannot drift.
func (cw *ContextWindow) AutoCompactThreshold() int {
	if cw == nil {
		return 0
	}
	return cw.autoCompactThreshold()
}

// AutoCompactThresholdWithMinPercent returns the ordinary fixed-buffer
// threshold or a reviewed percentage floor, whichever compacts later. This
// keeps the established safety buffer for large windows while avoiding a
// disproportionate buffer in deliberately small stress-test windows.
func (cw *ContextWindow) AutoCompactThresholdWithMinPercent(percent int) int {
	if cw == nil {
		return 0
	}
	threshold := cw.autoCompactThreshold()
	if percent <= 0 {
		return threshold
	}
	if percent > 100 {
		percent = 100
	}
	percentageFloor := cw.effectiveContextWindowSize() * percent / 100
	if percentageFloor > threshold {
		return percentageFloor
	}
	return threshold
}

// PreviousCacheReadTokens returns the last provider-reported cache hit. Local
// estimates never fabricate cache reuse; before the first usage report it is
// therefore zero.
func (cw *ContextWindow) PreviousCacheReadTokens() int {
	if cw == nil {
		return 0
	}
	cw.usageMu.RLock()
	defer cw.usageMu.RUnlock()
	if !cw.providerUsageKnown {
		return 0
	}
	return max(cw.CacheRead, 0)
}

// ShouldCompact checks if context compression should trigger.
// The circuit breaker prevents infinite compaction loops: after
// MaxConsecutiveAutocompactFailures consecutive failures, ShouldCompact
// returns false and the caller must either compact explicitly or fail closed;
// ordinary conversation messages must not be silently discarded.
func (cw *ContextWindow) ShouldCompact() bool {
	if cw.ConsecutiveFailures() >= MaxConsecutiveAutocompactFailures {
		return false // circuit breaker tripped
	}
	return cw.currentInputTokens() > cw.autoCompactThreshold()
}

// RecordCompactSuccess resets the circuit-breaker failure counter.
func (cw *ContextWindow) RecordCompactSuccess() {
	cw.usageMu.Lock()
	cw.consecutiveFailures = 0
	cw.suppressWarning = true
	cw.usageMu.Unlock()
}

// RecordMicrocompactSuccess suppresses compact warnings after a successful
// microcompact without changing the full auto-compact failure circuit breaker.
func (cw *ContextWindow) RecordMicrocompactSuccess() {
	if cw != nil {
		cw.usageMu.Lock()
		cw.suppressWarning = true
		cw.usageMu.Unlock()
	}
}

// RecordCompactFailure increments the circuit-breaker failure counter.
func (cw *ContextWindow) RecordCompactFailure() {
	cw.usageMu.Lock()
	defer cw.usageMu.Unlock()
	cw.consecutiveFailures++
}

// ConsecutiveFailures returns the current auto-compact circuit-breaker count.
func (cw *ContextWindow) ConsecutiveFailures() int {
	if cw == nil {
		return 0
	}
	cw.usageMu.RLock()
	defer cw.usageMu.RUnlock()
	return cw.consecutiveFailures
}

// Remaining returns estimated remaining input tokens
func (cw *ContextWindow) Remaining() int {
	return cw.effectiveContextWindowSize() - cw.currentInputTokens()
}

// TokenWarningState returns the current TS-equivalent warning calculation.
func (cw *ContextWindow) TokenWarningState(tokenUsage int, autoCompactEnabled bool) TokenWarningState {
	if cw == nil {
		return TokenWarningState{}
	}
	if tokenUsage < 0 {
		tokenUsage = cw.currentInputTokens()
	}
	cw.usageMu.RLock()
	suppressWarning := cw.suppressWarning
	cw.usageMu.RUnlock()
	return CalculateTokenWarningState(TokenWarningOptions{
		MaxTokens:          cw.MaxTokens,
		MaxOutputTokens:    cw.MaxOutputTokens,
		TokenUsage:         tokenUsage,
		AutoCompactEnabled: autoCompactEnabled,
		SuppressWarning:    suppressWarning,
	})
}

// EstimateMessages returns the known lower bound for a message slice. Callers
// that need proof of component coverage must use EstimateMessagesDetailed and
// inspect Complete/UnknownOverheads.
func (cw *ContextWindow) EstimateMessages(msgs []types.Message) int {
	return cw.EstimateMessagesDetailed(msgs, ModelContextOverhead{}).KnownTotalTokens
}

// Compactor compresses conversation history to fit within token budgets.
// Implementations must respect ctx cancellation — particularly those that
// make network calls (e.g. LLM summarization).
type Compactor interface {
	Compact(ctx context.Context, messages []types.Message, keepRecent int) (*CompactionResult, error)
}

// ToolResultBudget truncates oversized tool results
type ToolResultBudget struct {
	MaxCharsPerResult int // default 15000
}

func NewToolResultBudget() *ToolResultBudget {
	return &ToolResultBudget{MaxCharsPerResult: 15000}
}

// Apply truncates tool results that exceed the budget
func (t *ToolResultBudget) Apply(messages []types.Message) []types.Message {
	lang := i18n.DetectOrLoadLanguage()
	result := make([]types.Message, len(messages))
	for i, msg := range messages {
		result[i] = msg
		result[i].Content = make([]types.ContentBlock, 0, len(msg.Content))
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok {
				if tr.HasStructuredContent() && tr.HasMediaContent() {
					result[i].Content = append(result[i].Content, tr)
					continue
				}
				text := tr.TextContent()
				if len(text) > t.MaxCharsPerResult {
					text = text[:t.MaxCharsPerResult] + i18n.Format(lang, i18n.KeyAuxCompactTruncated, len(tr.TextContent()))
					tr.Content = text
					tr.ContentBlocks = nil
				}
				result[i].Content = append(result[i].Content, tr)
			} else {
				result[i].Content = append(result[i].Content, block)
			}
		}
	}
	return result
}

// isPromptTooLongError reports whether err indicates the prompt exceeded the
// context window (PTL = Prompt Too Long).
func isPromptTooLongError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *types.APIError
	if errors.As(err, &apiErr) && apiErr != nil {
		apiType := strings.ToLower(strings.TrimSpace(apiErr.Type))
		msg := strings.ToLower(apiErr.Message)
		if apiType == "prompt_too_long" ||
			apiType == "context_window_full" ||
			strings.Contains(msg, "prompt is too long") ||
			strings.Contains(msg, "prompt_too_long") ||
			strings.Contains(msg, "context_window_full") {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "prompt_too_long") ||
		strings.Contains(msg, "context_window_full")
}

// CompactPromptTooLongError reports that the complete compact input could not
// fit the summarizer context. Compaction fails closed instead of retrying with
// a lossy subset of the conversation.
type CompactPromptTooLongError struct {
	Cause error
}

func (e *CompactPromptTooLongError) Error() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCompactSummaryPTLHistoryPreserved)
}

func (e *CompactPromptTooLongError) Unwrap() error { return e.Cause }

func (e *CompactPromptTooLongError) Is(target error) bool {
	return target == ErrCompactPromptTooLong
}

// IsCompactPromptTooLongError reports whether the complete summary input was
// rejected as too long and compaction preserved the current history.
func IsCompactPromptTooLongError(err error) bool {
	var target *CompactPromptTooLongError
	return errors.As(err, &target) || errors.Is(err, ErrCompactPromptTooLong)
}

// SummaryCompactor calls the LLM to summarize old messages.
// CustomInstructions is passed through to the structured summarizer on each
// call.
type SummaryCompactor struct {
	SummarizeMessages  MessageSummarizeFunc // structured path; see summarize.go
	CustomInstructions string               // user-specified compact instructions; "" = default
	KeepRecent         int
	TranscriptPath     string // readable persisted transcript path for compact summaries
	// TranscriptPathResolver refreshes content-addressed audit references at
	// compaction time. When set, its result is authoritative and the static path
	// is never used as a stale fallback.
	TranscriptPathResolver func() string
	AttachmentProvider     PostCompactAttachmentProvider
	SessionID              string
	CWD                    string
	HookRunner             *hooks.Runner
	OnProgress             func(CompactProgressEvent)
	OnTelemetry            func(CompactionTelemetryEvent)
}

// CompactProgressEvent reports coarse compaction lifecycle progress to callers.
type CompactProgressEvent struct {
	Type     string
	HookType string
	Trigger  string
}

func (s *SummaryCompactor) summarizeMessages(ctx context.Context, messages []types.Message) (string, error) {
	ctx, flushUsage := withCompactAttemptUsage(ctx)
	defer flushUsage()
	if s.SummarizeMessages == nil {
		return "", i18n.NewError(i18n.KeyCompactSummaryNoSummarizer)
	}
	return s.SummarizeMessages(ctx, messages, s.CustomInstructions)
}

func (s *SummaryCompactor) summarizePartialMessages(ctx context.Context, messages []types.Message, userFeedback string) (string, error) {
	customInstructions := mergeCompactInstructions(s.CustomInstructions, userFeedback)
	if s.SummarizeMessages == nil {
		return "", i18n.NewError(i18n.KeyCompactSummaryNoSummarizer)
	}
	return s.SummarizeMessages(ctx, messages, customInstructions)
}

func mergeCompactInstructions(customInstructions, userFeedback string) string {
	custom := strings.TrimSpace(customInstructions)
	feedback := strings.TrimSpace(userFeedback)
	switch {
	case custom != "" && feedback != "":
		return custom + "\n\nUser context: " + feedback
	case custom != "":
		return custom
	case feedback != "":
		return "User context: " + feedback
	default:
		return ""
	}
}

func mergeCompactHookInstructions(userInstructions, hookInstructions string) string {
	userInstructions = strings.TrimSpace(userInstructions)
	hookInstructions = strings.TrimSpace(hookInstructions)
	switch {
	case userInstructions == "":
		return hookInstructions
	case hookInstructions == "":
		return userInstructions
	default:
		return userInstructions + "\n\n" + hookInstructions
	}
}

func systemReminderMessagesFromHookOutputs(outputs []hooks.HookOutput) []types.Message {
	var messages []types.Message
	for _, output := range outputs {
		if text := strings.TrimSpace(output.SystemReminder); text != "" {
			messages = append(messages, types.UserMessage(text))
		}
		if text := strings.TrimSpace(output.AdditionalContext); text != "" {
			messages = append(messages, types.UserMessage(text))
		}
		for _, text := range output.AdditionalContexts {
			if text = strings.TrimSpace(text); text != "" {
				messages = append(messages, types.UserMessage(text))
			}
		}
	}
	return messages
}

func compactHookError(ctx context.Context, hookType hooks.HookType, outputs []hooks.HookOutput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, output := range outputs {
		if output.Block {
			reason := strings.TrimSpace(output.Stderr)
			if reason == "" {
				reason = strings.TrimSpace(output.ExecutionError)
			}
			if reason == "" {
				reason = strings.TrimSpace(output.SystemReminder)
			}
			if reason == "" {
				return i18n.NewError(i18n.KeyCompactHookBlockedWithoutReason, hookType)
			}
			return i18n.NewError(i18n.KeyCompactHookBlocked, hookType, reason)
		}
	}
	return nil
}

func compactHookOutputs(executions []hooks.HookExecution) []hooks.HookOutput {
	outputs := make([]hooks.HookOutput, 0, len(executions))
	for _, execution := range executions {
		outputs = append(outputs, execution.Output)
	}
	return outputs
}

func (s *SummaryCompactor) runPreCompactHooks(ctx context.Context, trigger string) (string, string, error) {
	if s.HookRunner == nil || !s.HookRunner.HasHooks(hooks.HookPreCompact) {
		return s.CustomInstructions, "", nil
	}
	s.emitProgress(CompactProgressEvent{Type: "hooks_start", HookType: "pre_compact", Trigger: trigger})
	var custom *string
	if s.CustomInstructions != "" {
		custom = &s.CustomInstructions
	}
	executions := s.HookRunner.RunDetailedObserved(ctx, hooks.HookPreCompact, hooks.HookInput{
		Trigger:            trigger,
		CustomInstructions: custom,
	})
	outputs := compactHookOutputs(executions)
	if err := compactHookError(ctx, hooks.HookPreCompact, outputs); err != nil {
		return s.CustomInstructions, "", err
	}
	instructions := s.CustomInstructions
	var display []string
	for _, output := range outputs {
		instructions = mergeCompactHookInstructions(instructions, output.NewCustomInstructions)
		if text := strings.TrimSpace(output.UserDisplayMessage); text != "" {
			display = append(display, text)
		}
	}
	return instructions, strings.Join(display, "\n"), nil
}

func (s *SummaryCompactor) runSessionStartHooks(ctx context.Context) ([]types.Message, error) {
	if s.HookRunner == nil || !s.HookRunner.HasHooks(hooks.HookSessionStart) {
		return nil, nil
	}
	s.emitProgress(CompactProgressEvent{Type: "hooks_start", HookType: "session_start"})
	executions := s.HookRunner.RunDetailedObserved(ctx, hooks.HookSessionStart, hooks.HookInput{})
	outputs := compactHookOutputs(executions)
	if err := compactHookError(ctx, hooks.HookSessionStart, outputs); err != nil {
		return nil, err
	}
	return systemReminderMessagesFromHookOutputs(outputs), nil
}

func (s *SummaryCompactor) runPostCompactHooks(ctx context.Context, trigger, summary string) (string, error) {
	if s.HookRunner == nil || !s.HookRunner.HasHooks(hooks.HookPostCompact) {
		return "", nil
	}
	s.emitProgress(CompactProgressEvent{Type: "hooks_start", HookType: "post_compact", Trigger: trigger})
	executions := s.HookRunner.RunDetailedObserved(ctx, hooks.HookPostCompact, hooks.HookInput{
		Trigger:        trigger,
		CompactSummary: summary,
	})
	outputs := compactHookOutputs(executions)
	if err := compactHookError(ctx, hooks.HookPostCompact, outputs); err != nil {
		return "", err
	}
	var display []string
	for _, output := range outputs {
		if text := strings.TrimSpace(output.UserDisplayMessage); text != "" {
			display = append(display, text)
		}
	}
	return strings.Join(display, "\n"), nil
}

func (s *SummaryCompactor) emitProgress(event CompactProgressEvent) {
	if s.OnProgress != nil {
		s.OnProgress(event)
	}
}

func (s *SummaryCompactor) emitTelemetry(event CompactionTelemetryEvent) {
	if s.OnTelemetry != nil {
		s.OnTelemetry(event)
	}
}

func (s *SummaryCompactor) Compact(ctx context.Context, messages []types.Message, keepRecent int) (*CompactionResult, error) {
	return s.CompactWithTrigger(ctx, messages, keepRecent, "manual")
}

// PartialCompactConversation summarizes one side of a selected pivot while
// preserving the other side verbatim. Direction "from" keeps messages before
// the pivot and summarizes messages from the pivot onward. Direction "up_to"
// summarizes messages before the pivot and keeps messages from the pivot onward.
func (s *SummaryCompactor) PartialCompactConversation(ctx context.Context, allMessages []types.Message, pivotIndex int, direction PartialCompactDirection, userFeedback string) (*CompactionResult, error) {
	lang := i18n.DetectOrLoadLanguage()
	if direction == "" {
		direction = PartialCompactDirectionFrom
	}
	if direction != PartialCompactDirectionFrom && direction != PartialCompactDirectionUpTo {
		return nil, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyAuxCompactInvalidDirection, direction))
	}
	if len(allMessages) == 0 {
		return nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyAuxCompactEmptyHistory))
	}
	if pivotIndex < 0 || pivotIndex > len(allMessages) {
		return nil, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyAuxCompactInvalidPivot, pivotIndex, len(allMessages)))
	}
	if ctx != nil && ctx.Err() != nil {
		return &CompactionResult{MessagesToKeep: allMessages}, ctx.Err()
	}

	pivotIndex = adjustPartialCompactPivot(allMessages, pivotIndex, direction)

	var messagesToSummarize []types.Message
	var messagesToKeep []types.Message
	switch direction {
	case PartialCompactDirectionUpTo:
		messagesToSummarize = allMessages[:pivotIndex]
		messagesToKeep = filterPartialUpToPreservedMessages(allMessages[pivotIndex:])
	default:
		messagesToSummarize = allMessages[pivotIndex:]
		messagesToKeep = allMessages[:pivotIndex]
	}
	if len(messagesToSummarize) == 0 {
		if direction == PartialCompactDirectionUpTo {
			return nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyAuxCompactNothingBefore))
		}
		return nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyAuxCompactNothingAfter))
	}
	if len(messagesToKeep) == 0 {
		return nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyAuxCompactPreserveNone))
	}

	processed := StripReinjectedAttachments(messagesToSummarize)
	processed = StripImagesFromMessages(processed)
	processed = EnforcePerMessageBudget(processed)
	summary, err := s.summarizePartialMessages(ctx, processed, userFeedback)
	if err != nil {
		return nil, err
	}

	counter := NewContextWindow(0)
	preCompactTokenCount := counter.EstimateMessages(allMessages)
	summaryMessage := NewCompactSummaryMessage(GetPartialCompactUserSummaryMessage(summary, direction, s.usableTranscriptPath()))
	preservedStart := pivotIndex
	if direction == PartialCompactDirectionFrom {
		preservedStart = 0
	}
	boundary := NewCompactBoundaryMessage(CompactBoundaryMetadata{
		Trigger:                   "partial_" + string(direction),
		PreCompactTokenCount:      preCompactTokenCount,
		PreviousTailIdentifier:    previousTailIdentifier(allMessages),
		PreCompactDiscoveredTools: discoveredToolNames(allMessages),
		PreservedSegment: &PreservedSegmentMetadata{
			StartIndex: preservedStart,
			Count:      len(messagesToKeep),
			Anchor:     partialPreservedAnchor(allMessages, pivotIndex, direction),
			Direction:  string(direction),
		},
	})

	result := &CompactionResult{
		BoundaryMarker:       &boundary,
		SummaryMessages:      []types.Message{summaryMessage},
		MessagesToKeep:       messagesToKeep,
		PreCompactTokenCount: preCompactTokenCount,
	}
	if direction == PartialCompactDirectionFrom {
		// The preserved messages happened before the summarized suffix. Keep
		// that causal order in the provider view: boundary, earlier verbatim
		// context, then the summary of the newer portion.
		result.PreparedMessages = make([]types.Message, 0, len(messagesToKeep)+2)
		result.PreparedMessages = append(result.PreparedMessages, boundary)
		result.PreparedMessages = append(result.PreparedMessages, messagesToKeep...)
		result.PreparedMessages = append(result.PreparedMessages, summaryMessage)
	}
	result.PostCompactTokenCount = counter.EstimateMessages(BuildPostCompactMessages(result))
	result.TruePostCompactTokenCount = result.PostCompactTokenCount
	return result, nil
}

func adjustPartialCompactPivot(messages []types.Message, pivotIndex int, direction PartialCompactDirection) int {
	if direction == PartialCompactDirectionUpTo {
		return AdjustIndexToPreserveAPIInvariants(messages, pivotIndex)
	}
	return AdjustHeadEndToPreservePartialInvariants(messages, pivotIndex)
}

func AdjustHeadEndToPreservePartialInvariants(messages []types.Message, headEnd int) int {
	if headEnd <= 0 || headEnd >= len(messages) {
		return headEnd
	}
	adjusted := AdjustHeadEndToPreserveAPIInvariants(messages, headEnd)
	for {
		assistantIDs := assistantIDSet(messages[:adjusted])
		next := adjusted
		for i := adjusted; i < len(messages); i++ {
			if messages[i].Role != types.RoleAssistant || messages[i].ID == "" {
				continue
			}
			if _, ok := assistantIDs[messages[i].ID]; ok {
				next = i + 1
			}
		}
		if next == adjusted {
			return adjusted
		}
		adjusted = AdjustHeadEndToPreserveAPIInvariants(messages, next)
	}
}

func filterPartialUpToPreservedMessages(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if IsCompactBoundaryMessage(msg) {
			continue
		}
		if IsCompactSummaryMessage(msg) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func partialPreservedAnchor(messages []types.Message, pivotIndex int, direction PartialCompactDirection) string {
	if direction == PartialCompactDirectionFrom {
		return previousTailIdentifier(messages[:pivotIndex])
	}
	if pivotIndex <= 0 {
		return ""
	}
	return previousTailIdentifier(messages[:pivotIndex])
}

// CompactWithTrigger is the same compaction contract as Compact but lets loop
// callers preserve whether the boundary came from manual or automatic compact.
func (s *SummaryCompactor) CompactWithTrigger(ctx context.Context, messages []types.Message, keepRecent int, trigger string) (*CompactionResult, error) {
	keep := s.KeepRecent
	if keepRecent > 0 {
		keep = keepRecent
	}
	if keep == 0 {
		keep = 10
	}
	if trigger == "" {
		trigger = "manual"
	}

	preCompactMessages := messages
	counter := NewContextWindow(0)
	preCompactTokenCount := counter.EstimateMessages(preCompactMessages)
	messages = GetMessagesAfterCompactBoundary(messages)
	// Keep the trusted summary that immediately follows the latest boundary.
	// It is the only semantic representation of the compacted prefix and must
	// be folded into the next summary together with the newly compacted delta.
	// Dropping it here made repeated compaction forget progressively older
	// decisions even though the raw pre-boundary transcript was unavailable.

	if len(messages) == 0 {
		err := compactError(ErrCompactNotEnoughMessages, MessageNotEnoughMessages, nil)
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), nil, err))
		return nil, err
	}
	if len(messages) <= keep {
		return &CompactionResult{MessagesToKeep: messages}, nil
	}
	tailStart := AdjustIndexToPreserveAPIInvariants(messages, len(messages)-keep)
	if tailStart <= 0 || tailStart >= len(messages) {
		return &CompactionResult{MessagesToKeep: messages}, nil
	}

	// Bail early if context is already cancelled
	if ctx.Err() != nil {
		err := compactUserAbortError(ctx.Err())
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), nil, err))
		return nil, err
	}

	originalInstructions := s.CustomInstructions
	effectiveInstructions, preHookDisplay, err := s.runPreCompactHooks(ctx, trigger)
	if err != nil {
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), nil, err))
		return nil, err
	}
	s.CustomInstructions = effectiveInstructions
	defer func() {
		s.CustomInstructions = originalInstructions
	}()
	s.emitProgress(CompactProgressEvent{Type: "compact_start", Trigger: trigger})
	s.emitTelemetry(CompactionTelemetryEvent{
		Kind:                 CompactionTelemetryStart,
		Trigger:              trigger,
		PreCompactTokenCount: preCompactTokenCount,
		OriginalMessageCount: len(preCompactMessages),
	})

	// Collect old messages to summarize
	old := messages[:tailStart]
	old = StripReinjectedAttachments(old)
	// Strip images and enforce per-message budget before summarization to
	// avoid wasting tokens on base64 data and oversized tool results.
	old = StripImagesFromMessages(old)
	old = EnforcePerMessageBudget(old)

	var compactUsage *types.Usage
	ctx = withCompactUsageRecorder(ctx, func(usage *types.Usage) {
		addCompactUsage(&compactUsage, usage)
	})

	summary, err := s.summarizeMessages(ctx, old)
	if err != nil {
		if isCompactUserAbortCause(err) {
			err = compactUserAbortError(err)
			s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), compactUsage, err))
			return nil, err
		}
		// A PTL response must fail closed. Retrying with a prefix removed would
		// let a summary publish after silently losing the earliest rounds.
		if isPromptTooLongError(err) {
			err = &CompactPromptTooLongError{Cause: err}
		}
		if IsCompactPromptTooLongError(err) ||
			IsCompactIncompleteResponseError(err) ||
			IsCompactNoSummaryError(err) ||
			IsCompactAPIError(err) {
			s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), compactUsage, err))
			return nil, err
		}
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), compactUsage, err))
		return nil, i18n.WrapError(i18n.KeyCompactSummaryFailed, err)
	}
	if strings.TrimSpace(summary) == "" {
		err := compactError(ErrCompactNoSummary, MessageNoSummary, nil)
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), compactUsage, err))
		return nil, err
	}
	if startsWithAPIErrorPrefix(summary) {
		err := compactSummaryAPIError(summary)
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), compactUsage, err))
		return nil, err
	}

	preservedTail := messages[tailStart:]
	summaryMessage := NewCompactSummaryMessage(GetCompactUserSummaryMessage(summary, true, s.usableTranscriptPath(), false))
	boundary := NewCompactBoundaryMessage(CompactBoundaryMetadata{
		Trigger:                   trigger,
		PreCompactTokenCount:      preCompactTokenCount,
		PreviousTailIdentifier:    previousTailIdentifier(preCompactMessages),
		PreCompactDiscoveredTools: discoveredToolNames(preCompactMessages),
		PreservedSegment: &PreservedSegmentMetadata{
			StartIndex: tailStart,
			Count:      len(preservedTail),
			Anchor:     previousTailIdentifier(messages[:tailStart]),
		},
	})

	// A historical successful Read proves what the model observed, not that a
	// later filesystem reopen is still authorized or contains the same bytes.
	// Until compaction carries an immutable Read evidence reference, fail closed
	// and require a fresh Read instead of reopening live paths here.
	var attachments []types.Message
	if s.AttachmentProvider != nil {
		attachments = append(attachments, s.AttachmentProvider.PostCompactAttachments(ctx, PostCompactAttachmentState{
			OriginalMessages:      preCompactMessages,
			MessagesAfterBoundary: messages,
			PreservedTail:         preservedTail,
			SessionID:             s.SessionID,
			CWD:                   s.CWD,
		})...)
	}

	hookResults, err := s.runSessionStartHooks(ctx)
	if err != nil {
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), compactUsage, err))
		return nil, err
	}
	postHookDisplay, err := s.runPostCompactHooks(ctx, trigger, summary)
	if err != nil {
		s.emitTelemetry(compactFailureTelemetry(trigger, preCompactTokenCount, len(preCompactMessages), compactUsage, err))
		return nil, err
	}
	displayMessage := strings.Join(nonEmptyStrings(preHookDisplay, postHookDisplay), "\n")

	result := &CompactionResult{
		BoundaryMarker:       &boundary,
		SummaryMessages:      []types.Message{summaryMessage},
		MessagesToKeep:       preservedTail,
		Attachments:          attachments,
		HookResults:          hookResults,
		UserDisplayMessage:   displayMessage,
		PreCompactTokenCount: preCompactTokenCount,
		CompactionUsage:      compactUsage,
		PostCompactTokenCount: counter.EstimateMessages(BuildPostCompactMessages(&CompactionResult{
			BoundaryMarker:  &boundary,
			SummaryMessages: []types.Message{summaryMessage},
			MessagesToKeep:  preservedTail,
			Attachments:     attachments,
			HookResults:     hookResults,
		})),
	}
	result.TruePostCompactTokenCount = result.PostCompactTokenCount
	s.emitTelemetry(CompactionTelemetryEvent{
		Kind:                      CompactionTelemetrySuccess,
		Trigger:                   trigger,
		PreCompactTokenCount:      result.PreCompactTokenCount,
		PostCompactTokenCount:     result.PostCompactTokenCount,
		TruePostCompactTokenCount: result.TruePostCompactTokenCount,
		OriginalMessageCount:      len(preCompactMessages),
		CompactedMessageCount:     len(BuildPostCompactMessages(result)),
		CompactionUsage:           UsageMetricsFromUsage(compactUsage),
	})
	return result, nil
}

func compactFailureTelemetry(trigger string, preCompactTokenCount, originalMessageCount int, usage *types.Usage, err error) CompactionTelemetryEvent {
	event := CompactionTelemetryEvent{
		Kind:                 CompactionTelemetryFailure,
		Trigger:              trigger,
		PreCompactTokenCount: preCompactTokenCount,
		OriginalMessageCount: originalMessageCount,
		CompactionUsage:      UsageMetricsFromUsage(usage),
	}
	if err != nil {
		event.ErrorType = fmt.Sprintf("%T", err)
	}
	return event
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
