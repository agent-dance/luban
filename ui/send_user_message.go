package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

// SendUserMessageRenderMode captures the three TS Brief renderer variants.
type SendUserMessageRenderMode string

const (
	SendUserMessageRenderDefault    SendUserMessageRenderMode = "default"
	SendUserMessageRenderTranscript SendUserMessageRenderMode = "transcript"
	SendUserMessageRenderBriefOnly  SendUserMessageRenderMode = "brief_only"
)

type SendUserMessageRenderOptions struct {
	Mode              SendUserMessageRenderMode
	Now               time.Time
	DropAssistantText bool
	TurnCount         int
}

// TurnAwareRenderer receives the loop turn before its event is rendered.
// This lets reconciled UIs remove only text from the assistant message that
// actually contained SendUserMessage, even when sibling tools finish first.
type TurnAwareRenderer interface {
	SetRenderTurn(int)
}

// RuntimeLanguageRenderer exposes the language of a stateful presentation
// surface so semantic RuntimeEvents are localized only at the final boundary.
type RuntimeLanguageRenderer interface {
	RuntimeLanguage() i18n.Language
}

// ToolEventContext preserves the stable execution identity that presentation
// layers need to correlate concurrent calls, results, activities, and actors.
// SessionEpoch is a presentation generation fence; it is not part of a durable
// observation ID.
type ToolEventContext struct {
	SessionID         string
	SessionEpoch      uint64
	ContextGeneration uint64
	// ContextGenerationPersisted explicitly distinguishes a manifest-backed
	// generation from a new/legacy conversation. Zero is not a wildcard.
	ContextGenerationPersisted bool
	ProjectRoot                string
	TurnID                     string
	ActorID                    string
	ActorType                  string
	WorkUnitID                 string
}

// RuntimeIdentity returns the stable, path-free correlation identity used by
// RuntimeEvent. ProjectRoot is intentionally excluded from the shared event.
func (c ToolEventContext) RuntimeIdentity(toolUseID string) types.RuntimeIdentity {
	return types.RuntimeIdentity{
		SessionID: c.SessionID, Epoch: c.SessionEpoch, ContextGeneration: c.ContextGeneration,
		TurnID: c.TurnID, ToolUseID: toolUseID, WorkUnitID: c.WorkUnitID,
		ActorID: c.ActorID, ActorType: c.ActorType,
	}
}

// NewRuntimeErrorEvent is the UI adapter from provider/runtime error payloads
// into the shared public/private RuntimeEvent contract.
func NewRuntimeErrorEvent(ctx ToolEventContext, toolUseID, message string, apiError *types.APIError, metadata map[string]any) types.RuntimeEvent {
	return runtimeevent.NewErrorEvent(ctx.RuntimeIdentity(toolUseID), message, apiError, metadata)
}

// RuntimeErrorPublicMessage converts private runtime/provider error material
// into the strict user projection. The raw message, API error, metadata, and
// correlation identity never participate in the returned display string.
//
// languageSet lets stateful surfaces project with their active runtime
// language. Stateless renderers should pass false and let the projector detect
// the current language at this final presentation boundary.
func RuntimeErrorPublicMessage(ctx ToolEventContext, toolUseID, message string, apiError *types.APIError, metadata map[string]any, language i18n.Language, languageSet bool) string {
	event := NewRuntimeErrorEvent(ctx, toolUseID, message, apiError, metadata)
	projection, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
		Language: language, LanguageSet: languageSet,
	})
	if err == nil {
		return projection.Message
	}
	if !languageSet {
		language = i18n.DetectOrLoadLanguage()
	}
	return i18n.Text(language, i18n.KeyRuntimeErrorPublicSummary)
}

// RuntimeWarningPublicMessage renders only the semantic warning projection.
// Private causes and metadata remain reachable through the source RuntimeEvent
// for explicit audit use, but are never interpolated into user-visible copy.
func RuntimeWarningPublicMessage(event types.RuntimeEvent, language i18n.Language, languageSet bool) string {
	projection, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
		Language: language, LanguageSet: languageSet,
	})
	if err == nil && event.Kind == types.RuntimeEventKindWarning {
		return projection.Message
	}
	if !languageSet {
		language = i18n.DetectOrLoadLanguage()
	}
	return i18n.Text(language, i18n.KeyRuntimeWarningPublicSummary)
}

// DispatchRuntimeWarningEvent is the only default path from a loop warning to
// a text renderer. The renderer receives a strict user projection, never the
// loop Event's legacy Text/Error/Metadata fields.
func DispatchRuntimeWarningEvent(renderer Renderer, event types.RuntimeEvent, language i18n.Language, languageSet bool) {
	if renderer == nil {
		return
	}
	renderer.Warning(RuntimeWarningPublicMessage(event, language, languageSet))
}

// HookSummary carries one completed hook execution without coupling renderers
// to the query loop package.
type HookSummary struct {
	ExecutionID string
	ToolUseID   string
	Name        string
	Status      string
	Summary     string
	Metadata    map[string]any
}

// StructuredHookRenderer receives causally identified hook results. Legacy
// renderers continue to use the informational-text fallback.
type StructuredHookRenderer interface {
	RenderHookSummary(ToolEventContext, HookSummary)
}

// StructuredRuntimeErrorRenderer receives a causally identified runtime error
// without forcing machine consumers to infer its tool or work unit.
type StructuredRuntimeErrorRenderer interface {
	RuntimeErrorEvent(ToolEventContext, string, string, *types.APIError, map[string]any)
}

// DispatchRuntimeErrorEvent is the default renderer dispatch entry point for
// loop runtime errors. Structured renderers receive the private inputs so
// they can retain an authorized diagnostic projection; legacy renderers receive
// only the strict user projection and therefore cannot accidentally print raw
// EventError.Text or provider details.
func DispatchRuntimeErrorEvent(renderer Renderer, ctx ToolEventContext, toolUseID, message string, apiError *types.APIError, metadata map[string]any) {
	if renderer == nil {
		return
	}
	if structured, ok := renderer.(StructuredRuntimeErrorRenderer); ok {
		structured.RuntimeErrorEvent(ctx, toolUseID, message, apiError, metadata)
		return
	}
	renderer.Error(RuntimeErrorPublicMessage(ctx, toolUseID, message, apiError, metadata, i18n.LangEN, false))
}

// StructuredToolRenderer is optional so legacy renderers keep compiling while
// identity-aware surfaces receive complete tool blocks without lossy string
// adaptation.
type StructuredToolRenderer interface {
	RenderToolCall(ToolEventContext, types.ToolUseBlock)
	RenderToolResult(ToolEventContext, types.ToolResultBlock)
}

// SessionEpochRenderer gates asynchronous events against the currently visible
// presentation generation.
type SessionEpochRenderer interface {
	AdmitSessionEpoch(uint64) bool
}

// ContextGenerationRenderer additionally fences events after a model-context
// replacement within the same visible session epoch.
type ContextGenerationRenderer interface {
	AdmitContextGeneration(ToolEventContext) bool
}

func AdmitSessionEpoch(renderer Renderer, epoch uint64) bool {
	if aware, ok := renderer.(SessionEpochRenderer); ok {
		return aware.AdmitSessionEpoch(epoch)
	}
	return true
}

func AdmitContextGeneration(renderer Renderer, context ToolEventContext) bool {
	if aware, ok := renderer.(ContextGenerationRenderer); ok {
		return aware.AdmitContextGeneration(context)
	}
	return AdmitSessionEpoch(renderer, context.SessionEpoch)
}

func SetRenderTurn(renderer Renderer, turnCount int) {
	if aware, ok := renderer.(TurnAwareRenderer); ok {
		aware.SetRenderTurn(turnCount)
	}
}

// SendUserMessageRenderer is optional so embedders implementing the legacy
// Renderer interface keep compiling. Event dispatch falls back to Text/Info.
type SendUserMessageRenderer interface {
	RenderSendUserMessage(types.SendUserMessageOutput, SendUserMessageRenderOptions)
}

type ContextualSendUserMessageRenderer interface {
	RenderSendUserMessageEvent(ToolEventContext, types.SendUserMessageOutput, SendUserMessageRenderOptions)
}

type LosslessSendUserMessageRenderer interface {
	RenderHiddenToolCall(ToolEventContext, types.ToolUseBlock)
	RenderSendUserMessageToolEvent(ToolEventContext, types.ToolResultBlock, types.SendUserMessageOutput, SendUserMessageRenderOptions)
}

// DispatchToolCall suppresses generic tool-use chrome for SendUserMessage.
func DispatchToolCall(renderer Renderer, name string, input map[string]any) {
	if renderer == nil || name == "SendUserMessage" || name == "Brief" {
		return
	}
	renderer.ToolCall(name, input)
}

// DispatchToolCallEvent preserves tool identity for structured renderers and
// falls back to the legacy renderer contract for other surfaces.
func DispatchToolCallEvent(renderer Renderer, ctx ToolEventContext, call types.ToolUseBlock) {
	if renderer == nil {
		return
	}
	if call.Name == "SendUserMessage" || call.Name == "Brief" {
		if lossless, ok := renderer.(LosslessSendUserMessageRenderer); ok {
			lossless.RenderHiddenToolCall(ctx, call)
		}
		return
	}
	if structured, ok := renderer.(StructuredToolRenderer); ok {
		structured.RenderToolCall(ctx, call)
		return
	}
	renderer.ToolCall(call.Name, call.Input)
}

// DispatchToolResult routes typed SendUserMessage data to its user-visible
// channel. It returns true when the result was handled specially.
func DispatchToolResult(renderer Renderer, result types.ToolResultBlock) bool {
	return DispatchToolResultEvent(renderer, ToolEventContext{}, result)
}

// DispatchToolResultEvent preserves the complete result block for structured
// renderers while retaining Brief's dedicated user-visible channel.
func DispatchToolResultEvent(renderer Renderer, ctx ToolEventContext, result types.ToolResultBlock) bool {
	if renderer == nil || result.IsError {
		if renderer != nil {
			if structured, ok := renderer.(StructuredToolRenderer); ok {
				structured.RenderToolResult(ctx, result)
			} else {
				renderer.ToolResult(result.TextContent(), result.IsError)
			}
		}
		return false
	}
	output, ok := sendUserMessageOutput(result.Data)
	if !ok {
		if structured, ok := renderer.(StructuredToolRenderer); ok {
			structured.RenderToolResult(ctx, result)
		} else {
			renderer.ToolResult(result.TextContent(), false)
		}
		return false
	}
	if contextual, ok := renderer.(ContextualSendUserMessageRenderer); ok {
		if lossless, losslessOK := renderer.(LosslessSendUserMessageRenderer); losslessOK {
			lossless.RenderSendUserMessageToolEvent(ctx, result, output, SendUserMessageRenderOptions{Mode: SendUserMessageRenderDefault})
			return true
		}
		contextual.RenderSendUserMessageEvent(ctx, output, SendUserMessageRenderOptions{Mode: SendUserMessageRenderDefault})
		return true
	}
	if briefRenderer, ok := renderer.(SendUserMessageRenderer); ok {
		briefRenderer.RenderSendUserMessage(output, SendUserMessageRenderOptions{Mode: SendUserMessageRenderDefault})
		return true
	}
	if output.Message != "" {
		renderer.Text(output.Message)
	}
	for _, attachment := range output.Attachments {
		renderer.Info(FormatSendUserMessageAttachment(attachment))
	}
	return true
}

// IsSendUserMessageResult lets event adapters decide whether streamed
// assistant text should be retained before they dispatch the typed result.
func IsSendUserMessageResult(result types.ToolResultBlock) bool {
	if result.IsError {
		return false
	}
	_, ok := sendUserMessageOutput(result.Data)
	return ok
}

func sendUserMessageOutput(data any) (types.SendUserMessageOutput, bool) {
	switch output := data.(type) {
	case types.SendUserMessageOutput:
		return output, true
	case *types.SendUserMessageOutput:
		if output != nil {
			return *output, true
		}
	}
	return types.SendUserMessageOutput{}, false
}

// FormatSendUserMessage renders the content shared by terminal and TUI modes.
func FormatSendUserMessage(output types.SendUserMessageOutput, options SendUserMessageRenderOptions) string {
	lang := i18n.DetectOrLoadLanguage()
	var lines []string
	switch options.Mode {
	case SendUserMessageRenderTranscript:
		if output.Message != "" {
			lines = append(lines, "* "+output.Message)
		}
	case SendUserMessageRenderBriefOnly:
		label := "Claude"
		if timestamp := FormatSendUserMessageTimestampInLanguage(lang, output.SentAt, options.Now); timestamp != "" {
			label += " " + timestamp
		}
		lines = append(lines, label)
		if output.Message != "" {
			lines = append(lines, output.Message)
		}
	default:
		if output.Message != "" {
			lines = append(lines, output.Message)
		}
	}
	for _, attachment := range output.Attachments {
		lines = append(lines, FormatSendUserMessageAttachmentInLanguage(lang, attachment))
	}
	return strings.Join(lines, "\n")
}

func FormatSendUserMessageAttachment(attachment types.SendUserMessageAttachment) string {
	return FormatSendUserMessageAttachmentInLanguage(i18n.DetectOrLoadLanguage(), attachment)
}

func FormatSendUserMessageAttachmentInLanguage(lang i18n.Language, attachment types.SendUserMessageAttachment) string {
	kind := i18n.Text(lang, i18n.KeyPresentationFile)
	if attachment.IsImage {
		kind = i18n.Text(lang, i18n.KeyPresentationImage)
	}
	return i18n.Format(lang, i18n.KeyPresentationAttachment, kind, attachment.Path, formatBriefFileSize(attachment.Size))
}

func formatBriefFileSize(size int64) string {
	const kib = int64(1024)
	const mib = 1024 * kib
	switch {
	case size >= mib:
		return fmt.Sprintf("%.1f MB", float64(size)/float64(mib))
	case size >= kib:
		return fmt.Sprintf("%.1f KB", float64(size)/float64(kib))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func FormatSendUserMessageTimestamp(sentAt string, now time.Time) string {
	return FormatSendUserMessageTimestampInLanguage(i18n.DetectOrLoadLanguage(), sentAt, now)
}

func FormatSendUserMessageTimestampInLanguage(lang i18n.Language, sentAt string, now time.Time) string {
	value, err := time.Parse(time.RFC3339Nano, sentAt)
	if err != nil {
		return ""
	}
	if now.IsZero() {
		now = time.Now()
	}
	value = value.Local()
	now = now.Local()
	if value.Year() == now.Year() && value.YearDay() == now.YearDay() {
		return value.Format("15:04")
	}
	days := int(now.Sub(value).Hours() / 24)
	if days >= 0 && days < 7 {
		return i18n.Format(lang, i18n.KeyRuntimeRecentTimestamp, localizedWeekday(lang, value.Weekday()), value.Format("15:04"))
	}
	return value.Format("2006-01-02 15:04")
}

func localizedWeekday(lang i18n.Language, weekday time.Weekday) string {
	keys := [...]i18n.Key{
		i18n.KeyRuntimeWeekdaySunday, i18n.KeyRuntimeWeekdayMonday, i18n.KeyRuntimeWeekdayTuesday,
		i18n.KeyRuntimeWeekdayWednesday, i18n.KeyRuntimeWeekdayThursday, i18n.KeyRuntimeWeekdayFriday,
		i18n.KeyRuntimeWeekdaySaturday,
	}
	if weekday < time.Sunday || weekday > time.Saturday {
		return ""
	}
	return i18n.Text(lang, keys[int(weekday)])
}
