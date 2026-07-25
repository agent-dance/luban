package tui

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/cost"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/presentation"
	"github.com/agent-dance/luban/internal/ui/input"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/theme"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/skills"
	"github.com/grindlemire/go-tui"
)

// truncateRunes safely truncates a string to at most maxRunes Unicode code
// points, appending suffix if truncation occurred. This avoids splitting
// multi-byte UTF-8 sequences (which causes mojibake or panics).
func truncateRunes(s string, maxRunes int, suffix string) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + suffix
}

// setTextAreaField dynamically updates a private int field of a go-tui TextArea.
// go-tui treats TextArea fields as immutable after construction (no public setters),
// but we need them to track terminal width and limit input height dynamically.
// This uses reflect+unsafe as a surgical workaround until go-tui adds setters.
//
// Safety checks:
//   - field must exist (degrade gracefully if removed in future versions)
//   - field must be of kind reflect.Int (prevent memory corruption if type changes)
func setTextAreaField(ta *tui.TextArea, field string, value int) {
	v := reflect.ValueOf(ta).Elem()
	f := v.FieldByName(field)
	if !f.IsValid() {
		return // field removed in future go-tui version; degrade gracefully
	}
	if f.Kind() != reflect.Int {
		return // field type changed; refuse to write wrong-sized value
	}
	ptr := unsafe.Pointer(f.UnsafeAddr())
	*(*int)(ptr) = value
}

// textAreaText returns the current text content of a go-tui TextArea.
// go-tui v0.11.0 exposes TextArea.Text() as a public accessor.
func textAreaText(ta *tui.TextArea) string {
	return ta.Text()
}

// textAreaVisibleLines computes the number of screen lines the TextArea text
// will occupy when rendered at the given width. Uses terminal cell widths
// (CJK characters count as 2 cells), matching TextArea's internal wrapText().
func textAreaVisibleLines(text string, width int) int {
	if width <= 0 {
		return 1
	}
	if text == "" {
		return 1
	}

	lines := 0
	for _, para := range strings.Split(text, "\n") {
		if para == "" {
			lines++
			continue
		}

		lineWidth := 0
		for _, r := range para {
			rw := tui.RuneWidth(r)
			if lineWidth > 0 && lineWidth+rw > width {
				lines++
				lineWidth = 0
			}
			lineWidth += rw
		}
		if lineWidth > 0 || para == "" {
			lines++
		}
	}

	if lines < 1 {
		return 1
	}
	return lines
}

// imagePasteBinding returns a KeyBinding for image paste:
// Ctrl+V on macOS/Linux (where Ctrl+V is not used for text paste in terminals),
// Alt+V on Windows (where Ctrl+V is the standard text paste key).
// This matches the original TypeScript frontend's keybinding strategy in
// src/keybindings/defaultBindings.ts.
func imagePasteBinding(handler func(tui.KeyEvent)) tui.KeyBinding {
	if runtime.GOOS == "windows" {
		return tui.On(tui.Rune('v').Alt(), handler)
	}
	return tui.On(tui.KeyCtrlV, handler)
}

// RootComponent is the top-level TUI component that manages the entire
// application layout: banner, message history (scrollable), spinner,
// permission dialog, and input prompt.
type RootComponent struct {
	state *AppState
	app   *tui.App // set in BindApp; used for Stop() on Ctrl+C/Ctrl+D
	build buildinfo.Diagnostic

	// Internal sub-components
	input                  *slashAwareTextArea
	onSubmit               func(string)
	trySubmit              func(string) bool
	inputText              *tui.State[string]
	inputCursor            *tui.State[int]
	inputHistory           *promptHistoryNavigator
	inputHistoryPath       string
	inputHistoryProject    string
	inputHistorySession    string
	inputSelectionSession  string
	inputHistoryLoaded     bool
	inputHistoryPersistent bool
	pastes                 []pastedText
	nextPaste              int
	slash                  *tui.State[*slashSuggestionsState]
	scrollY                *tui.State[int]
	decisionScroll         *tui.State[int]
	decisionScrollTarget   *tui.State[decisionScrollTarget]
	historyStart           *tui.State[int]
	llmWorkingFrame        *tui.State[uint64]

	slashCommands          []SlashCommandEntry
	slashDismissedForInput string

	// termWidth caches the last known terminal width (in cells) for use by
	// renderMessage → renderMarkdown table rendering.
	termWidth  int
	termHeight int

	// Scroll behaviour
	stickToBottom *tui.State[bool] // true = auto-follow new content
	contentRef    *tui.Ref         // ref to the scrollable message area element
	decisionRef   *tui.Ref         // ref to the scrollable permission details element
	taskViewRef   *tui.Ref         // ref to the centered task summary control
	segmentRefs   *tui.RefMap[string]

	// Copy feedback: non-empty string = show brief "Copied!" toast in status bar.
	// Cleared after ~2 seconds by a resettable timer (not per-event goroutines).
	copyFeedback      *tui.State[string]
	copyFeedbackTimer *time.Timer // single resettable timer; nil when idle
	clipboardWriter   func(i18n.Language, string) error

	// An unmodified transcript drag cannot reach native terminal selection
	// while mouse reporting is active. Show a brief status hint once per drag.
	transcriptSelectionHintVisible *tui.State[bool]
	transcriptSelectionHintTimer   *time.Timer
	selectionHintShownForDrag      bool
	selectionHintGeneration        uint64

	// openModelPicker is called when the user presses Alt+P.
	// Set by the REPL layer via SetOpenModelPicker() to populate
	// and show the model picker overlay from the ModelCatalog.
	openModelPicker func()

	// onModeSwitch is called when the user presses Shift+Tab.
	// Set by the REPL layer via SetOnModeSwitch() to wire mode changes
	// to permissions.Checker and interaction.PlanState.
	onModeSwitch func(InteractionMode)

	// Language changes are committed as one serialized persistence/state
	// transaction. The observer runs only after both durable and active
	// language state have moved to the target.
	languageMu       sync.Mutex
	languageSaver    func(i18n.Language) error
	onLanguageSwitch func(i18n.Language)

	// onActivityAction executes an explicit action for the focused activity.
	onActivityAction func(string, ActivityAction)

	// onSteerQueued promotes the oldest queued composer submission into
	// guidance for the active work when Escape is pressed.
	onSteerQueued func()

	// Idle Ctrl+C uses a short confirmation window so an accidental interrupt
	// never exits the application on the first press.
	now               func() time.Time
	exitConfirmWindow time.Duration
	lastInterruptAt   time.Time
	onExit            func()
}

type pastedText struct {
	placeholder string
	text        string
}

var (
	_ tui.Component       = (*RootComponent)(nil)
	_ tui.KeyListener     = (*RootComponent)(nil)
	_ tui.MouseListener   = (*RootComponent)(nil)
	_ tui.WatcherProvider = (*RootComponent)(nil)
	_ tui.AppBinder       = (*RootComponent)(nil)
)

func decisionDialogRows(request *DecisionRequest) int {
	if request == nil {
		return 0
	}
	rows := 10
	if request.Body != "" {
		rows += strings.Count(request.Body, "\n") + 2
	}
	for _, detail := range request.ReviewDetails {
		rows += strings.Count(detail, "\n") + 1
	}
	if request.PostMode != "" {
		rows++
	}
	if request.ExecutionSessionID != "" {
		rows++
	}
	return rows
}

const decisionDialogMinRows = 8

// decisionScrollTarget identifies which readable viewport owns navigation
// while a permission prompt is open. Selection remains on Left/Right, so
// vertical navigation can scroll without silently changing the decision.
type decisionScrollTarget uint8

const (
	decisionScrollDetails decisionScrollTarget = iota
	decisionScrollTranscript
)

func (c *RootComponent) decisionDialogHeight(request *DecisionRequest) int {
	height := decisionDialogRows(request)
	if c.termHeight <= 0 {
		return height
	}
	maximum := c.termHeight - 6
	if maximum < decisionDialogMinRows {
		maximum = decisionDialogMinRows
	}
	if height > maximum {
		return maximum
	}
	return height
}

// NewRootComponentWithAdmission keeps the composer intact unless the
// synchronous admission callback has reserved execution capacity.
func NewRootComponentWithAdmission(state *AppState, trySubmit func(string) bool, slashCommands []SlashCommandEntry) *RootComponent {
	return newRootComponent(state, nil, trySubmit, slashCommands)
}

func newRootComponent(state *AppState, onSubmit func(string), trySubmit func(string) bool, slashCommands []SlashCommandEntry) *RootComponent {
	project, projectErr := os.Getwd()
	historyPath := input.DefaultPromptHistoryPath()
	if projectErr != nil {
		project = ""
		historyPath = ""
	}
	c := &RootComponent{
		state:                          state,
		onSubmit:                       onSubmit,
		trySubmit:                      trySubmit,
		inputText:                      tui.NewState(""),
		inputCursor:                    tui.NewState(0),
		inputHistory:                   newPromptHistoryNavigator(nil),
		inputSelectionSession:          state.SessionID.Get(),
		inputHistoryPath:               historyPath,
		inputHistoryProject:            project,
		decisionScroll:                 tui.NewState(0),
		decisionScrollTarget:           tui.NewState(decisionScrollDetails),
		historyStart:                   tui.NewState(-1),
		llmWorkingFrame:                tui.NewState(uint64(0)),
		slash:                          tui.NewState[*slashSuggestionsState](nil),
		scrollY:                        tui.NewState(0),
		stickToBottom:                  tui.NewState(true),
		contentRef:                     tui.NewRef(),
		decisionRef:                    tui.NewRef(),
		taskViewRef:                    tui.NewRef(),
		segmentRefs:                    tui.NewRefMap[string](),
		copyFeedback:                   tui.NewState(""),
		transcriptSelectionHintVisible: tui.NewState(false),
		clipboardWriter:                writeToClipboardInLanguage,
		languageSaver:                  i18n.SaveLanguage,
		slashCommands:                  cloneSlashCommands(slashCommands),
		now:                            time.Now,
		exitConfirmWindow:              2 * time.Second,
	}
	// Build the TextArea once with all options, including onSubmit if provided.
	// TextArea supports multi-line editing with word wrapping. Border is NOT
	// set on TextArea — the external inputRow container provides the visual
	// border. This avoids a go-tui bug where TextArea.wrapText() uses the
	// full width (including border) for line wrapping, causing text to
	// overflow the border by 2 characters.
	//
	// Width and maxHeight are dynamically updated on every Render() frame
	// to match the terminal dimensions. Enter submits, Ctrl+J inserts newline.
	opts := []tui.TextAreaOption{
		tui.WithTextAreaWidth(80),
		tui.WithTextAreaValue(c.inputText),
		tui.WithTextAreaCursorPosition(c.inputCursor),
		tui.WithTextAreaAtomicRanges(c.inputAtomicRanges),
		tui.WithTextAreaOnTextChange(c.handleInputTextChanged),
		tui.WithTextAreaCursor('|'),
		// No border — drawn by external container
		tui.WithTextAreaPlaceholder(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyInputPlaceholder)),
		tui.WithTextAreaAutoFocus(true),
		// Explicitly set text style to white. go-tui's TextArea defaults
		// to Style{} (ColorDefault fg), and WithTextStyle sets textStyleSet=true
		// which blocks style inheritance from parent elements. Without an
		// explicit foreground color, the text may be invisible on terminals
		// where the default fg color blends into the background.
		tui.WithTextAreaTextStyle(tui.NewStyle().Foreground(tui.White)),
	}
	if onSubmit != nil || trySubmit != nil {
		opts = append(opts, tui.WithTextAreaOnSubmit(func(text string) {
			c.submitInput(text)
		}))
	}
	c.input = newSlashAwareTextArea(
		c.hasSlashSuggestions,
		c.moveSlashSelection,
		c.executeSlashSuggestion,
		c.dismissSlashSuggestions,
		c.handleInputHistoryUp,
		c.handleInputHistoryDown,
		c.hasPickerOverlay,
		c.handleTextPaste,
		opts...,
	)
	c.input.handleOverlayPaste = c.handleOverlayPaste
	return c
}

func (c *RootComponent) handleOverlayPaste(text string) bool {
	p := c.state.ModelPicker.Get()
	if p == nil || !p.Visible || p.Phase != PickerPhaseConnect {
		return false
	}
	authMethod := "api_key"
	if len(p.ConnectAuthMethods) > 0 {
		if p.ConnectSelectedAuth < 0 || p.ConnectSelectedAuth >= len(p.ConnectAuthMethods) {
			return false
		}
		authMethod = p.ConnectAuthMethods[p.ConnectSelectedAuth]
	}
	if authMethod != "api_key" {
		return false
	}
	p.appendConnectPaste(text)
	p.ConnectError = ""
	c.state.ModelPicker.Set(p)
	return true
}

func (c *RootComponent) handleTextPaste(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text == "" {
		return
	}
	if !strings.Contains(text, "\n") {
		c.input.InsertText(text)
		return
	}

	c.nextPaste++
	placeholder := i18n.Format(c.state.Language.Get(), i18n.KeyPastedText, c.nextPaste, strings.Count(text, "\n")+1)
	c.pastes = append(c.pastes, pastedText{placeholder: placeholder, text: text})
	c.input.InsertText(placeholder)
}

func atomicPlaceholderRange(text, placeholder string) (tui.TextAreaAtomicRange, bool) {
	if placeholder == "" {
		return tui.TextAreaAtomicRange{}, false
	}
	byteStart := strings.Index(text, placeholder)
	if byteStart < 0 {
		return tui.TextAreaAtomicRange{}, false
	}
	start := utf8.RuneCountInString(text[:byteStart])
	return tui.TextAreaAtomicRange{
		Start: start,
		End:   start + utf8.RuneCountInString(placeholder),
	}, true
}

func (c *RootComponent) inputAtomicRanges(text string) []tui.TextAreaAtomicRange {
	ranges := make([]tui.TextAreaAtomicRange, 0, len(c.pastes)+len(c.state.PendingImages.Get()))
	for _, paste := range c.pastes {
		if span, ok := atomicPlaceholderRange(text, paste.placeholder); ok {
			ranges = append(ranges, span)
		}
	}
	for _, image := range c.state.PendingImages.Get() {
		if span, ok := atomicPlaceholderRange(text, imageComposerPlaceholder(image.Placeholder)); ok {
			ranges = append(ranges, span)
		}
	}
	return ranges
}

func (c *RootComponent) handleInputTextChanged(text string) {
	retainedPastes := c.pastes[:0]
	for _, paste := range c.pastes {
		if strings.Contains(text, paste.placeholder) {
			retainedPastes = append(retainedPastes, paste)
		}
	}
	c.pastes = retainedPastes

	for _, image := range c.state.PendingImages.Get() {
		if image.Placeholder != "" && !strings.Contains(text, image.Placeholder) {
			c.state.RemovePendingImage(image.ID)
		}
	}
}

func (c *RootComponent) submitInput(displayText string) {
	c.handleInputTextChanged(displayText)
	text := displayText
	for _, paste := range c.pastes {
		text = strings.Replace(text, paste.placeholder, paste.text, 1)
	}
	for _, image := range c.state.PendingImages.Get() {
		text = removeImageComposerPlaceholder(text, image.Placeholder)
	}
	admitted := false
	if c.trySubmit != nil {
		admitted = c.trySubmit(text)
	} else if c.onSubmit != nil {
		c.onSubmit(text)
		admitted = true
	}
	if !admitted {
		return
	}
	if strings.TrimSpace(text) != "" {
		c.ensureInputHistoryLoaded()
		c.inputHistory.Add(text)
		if c.inputHistoryPersistent {
			if err := input.AppendPromptHistory(c.inputHistoryPath, input.PromptHistoryEntry{
				Display:   text,
				Project:   c.inputHistoryProject,
				SessionID: c.state.SessionID.Get(),
			}); err != nil {
				c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyPromptHistoryNotSaved))
				c.scheduleCopyFeedbackClear()
			}
		}
	}
	c.input.Clear()
	c.pastes = nil
	c.nextPaste = 0
}

func removeImageComposerPlaceholder(text, placeholder string) string {
	token := imageComposerPlaceholder(placeholder)
	start := strings.Index(text, token)
	if start < 0 {
		return strings.Replace(text, placeholder, "", 1)
	}
	end := start + len(token)
	before, after := text[:start], text[end:]
	separator := ""
	if before != "" && after != "" {
		beforeRune, _ := utf8.DecodeLastRuneInString(before)
		afterRune, _ := utf8.DecodeRuneInString(after)
		if !unicode.IsSpace(beforeRune) && !unicode.IsSpace(afterRune) {
			separator = " "
		}
	}
	return before + separator + after
}

func (c *RootComponent) handleInputHistoryUp() bool {
	c.ensureInputHistoryLoaded()
	value, ok := c.inputHistory.Previous(c.inputText.Get())
	if !ok {
		return false
	}
	c.setInputFromHistory(value, true)
	return true
}

func (c *RootComponent) handleInputHistoryDown() bool {
	c.ensureInputHistoryLoaded()
	value, ok := c.inputHistory.Next(c.inputText.Get())
	if !ok {
		return false
	}
	c.setInputFromHistory(value, false)
	return true
}

func (c *RootComponent) setInputFromHistory(value string, cursorAtEnd bool) {
	// A recalled slash command is history, not a request to open autocomplete.
	// Keep the menu dismissed until the user changes the recalled value.
	c.slashDismissedForInput = value
	c.slash.Set(nil)
	c.persistSlashInteraction()
	c.input.SetText(value)
	if !cursorAtEnd {
		c.input.SetCursorPosition(0)
	}
}

func (c *RootComponent) ensureInputHistoryLoaded() {
	if !c.inputHistoryPersistent {
		return
	}
	sessionID := c.state.SessionID.Get()
	if c.inputHistoryLoaded && c.inputHistorySession == sessionID {
		return
	}
	entries := input.LoadPromptHistory(
		c.inputHistoryPath,
		c.inputHistoryProject,
		sessionID,
		maxPromptHistoryEntries,
	)
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Display)
	}
	c.inputHistory.Replace(values)
	c.inputHistorySession = sessionID
	c.inputHistoryLoaded = true
}

// BindApp binds all reactive state to the app.
func (c *RootComponent) BindApp(app *tui.App) {
	c.app = app
	c.inputHistoryPersistent = true
	c.state.bindBatch(app.Batch)
	c.state.Messages.BindApp(app)
	c.state.ActivityRevision.BindApp(app)
	c.state.ActivityFocus.BindApp(app)
	c.state.ActivityViewOffset.BindApp(app)
	c.state.LLMCall.BindApp(app)
	c.state.DecisionReq.BindApp(app)
	c.state.DecisionSelected.BindApp(app)
	c.state.DecisionHistory.BindApp(app)
	c.state.DecisionReceipt.BindApp(app)
	c.state.AskUserDraft.BindApp(app)
	c.state.TranscriptShowAll.BindApp(app)
	c.state.ToolSegmentExpansion.BindApp(app)
	c.state.Provider.BindApp(app)
	c.state.Model.BindApp(app)
	c.state.SessionID.BindApp(app)
	c.state.SessionUsageKnown.BindApp(app)
	c.state.SessionRoundUsageKnown.BindApp(app)
	c.state.InteractionRevision.BindApp(app)
	c.state.ViewRevision.BindApp(app)
	c.state.Tools.BindApp(app)
	c.state.Goal.BindApp(app)
	c.state.ContextWindowK.BindApp(app)
	c.state.ModelCostIn.BindApp(app)
	c.state.ModelCostOut.BindApp(app)
	c.state.ModelCostCurrency.BindApp(app)
	c.state.ModelCanSeeImages.BindApp(app)
	c.state.ReasoningEffort.BindApp(app)
	c.state.ProvStatus.BindApp(app)
	c.state.CumulativeCost.BindApp(app)
	c.state.SessionCostKnown.BindApp(app)
	c.state.SessionInputTokens.BindApp(app)
	c.state.SessionOutputTokens.BindApp(app)
	c.state.SessionCacheReadTokens.BindApp(app)
	c.state.SessionCacheCreateTokens.BindApp(app)
	c.state.SessionWebSearchRequests.BindApp(app)
	c.state.SessionTotalInputTokens.BindApp(app)
	c.state.SessionTotalOutputTokens.BindApp(app)
	c.state.SessionTotalCacheReadTokens.BindApp(app)
	c.state.SessionTotalCacheCreateTokens.BindApp(app)
	c.state.SessionHasCompacted.BindApp(app)
	c.state.SessionCompactionBaselineKnown.BindApp(app)
	c.state.SessionCompactionCount.BindApp(app)
	c.state.SessionCompletedRoundInputTokens.BindApp(app)
	c.state.SessionCompletedRoundOutputTokens.BindApp(app)
	c.state.SessionInputTokensAtCompact.BindApp(app)
	c.state.SessionCacheReadAtCompact.BindApp(app)
	c.state.UsedTokens.BindApp(app)
	c.state.MaxTokens.BindApp(app)
	c.state.ContextMeasurement.BindApp(app)
	c.state.PendingImages.BindApp(app)
	c.state.PendingImageSelected.BindApp(app)
	c.state.QueuedInputCount.BindApp(app)
	c.state.SessionPicker.BindApp(app)
	c.state.ForkPicker.BindApp(app)
	c.state.ModelPicker.BindApp(app)
	c.state.SkillsMenu.BindApp(app)
	c.state.Mode.BindApp(app)
	c.state.ExpandedView.BindApp(app)
	c.state.TaskListRevision.BindApp(app)
	c.state.TaskViewItems.BindApp(app)
	c.scrollY.BindApp(app)
	c.stickToBottom.BindApp(app)
	c.copyFeedback.BindApp(app)
	c.transcriptSelectionHintVisible.BindApp(app)
	c.inputText.BindApp(app)
	c.inputCursor.BindApp(app)
	c.decisionScroll.BindApp(app)
	c.decisionScrollTarget.BindApp(app)
	c.historyStart.BindApp(app)
	c.llmWorkingFrame.BindApp(app)
	c.slash.BindApp(app)
	c.input.BindApp(app)
}

// Render returns the element tree for the entire application.
func (c *RootComponent) Render(app *tui.App) *tui.Element {
	termWidth, termHeight := app.Size()
	return c.renderAtSize(app, termWidth, termHeight)
}

func (c *RootComponent) renderAtSize(app *tui.App, termWidth, termHeight int) *tui.Element {
	c.termWidth = termWidth
	c.termHeight = termHeight
	c.state.TermWidth = termWidth

	// Dynamically adjust TextArea width to match terminal width minus prompt
	// (">" = 3 cols) minus border (2 cols). The TextArea has no border of its
	// own — the external inputRow container provides the visual border. This
	// avoids a go-tui bug where TextArea.wrapText() uses the full width
	// (including border) for line wrapping, causing text to overflow by 2 chars.
	promptCols := len(c.state.Mode.Get().PromptPrefixInLanguage(c.state.Language.Get())) + 1
	const borderCols = 2 // left + right border of the external container
	contentWidth := termWidth - promptCols - borderCols
	if contentWidth < 10 {
		contentWidth = 10
	}
	setTextAreaField(c.input.TextArea, "width", contentWidth)

	// Compute dynamic input height: TextArea content height (no border) clamped
	// to [minInputLines, maxInputLines], plus 2 for the external border.
	const minInputLines = 1
	maxInputLines := termHeight / 3
	if maxInputLines < 3 {
		maxInputLines = 3
	}
	// Update TextArea's maxHeight so it only renders visible lines (viewport).
	// Without this, TextArea renders ALL lines but the container clips the bottom,
	// making lines beyond the visible area unreachable by the user.
	setTextAreaField(c.input.TextArea, "maxHeight", maxInputLines)

	// Don't trust TextArea.Height() here: go-tui v0.11.0 wraps by rune count,
	// while the renderer uses terminal cell widths. That underestimates height
	// for Chinese/CJK text and lets rendered lines extend below the border.
	taHeight := textAreaVisibleLines(textAreaText(c.input.TextArea), contentWidth)
	if taHeight < minInputLines {
		taHeight = minInputLines
	}
	if taHeight > maxInputLines {
		taHeight = maxInputLines
	}
	inputRowHeight := taHeight + borderCols // +2 for top/bottom border

	status := c.renderStatusBar(termWidth)
	statusRows := status.HeightForWidth(termWidth)

	// Reserve rows for chrome: banner (3) + spacer (1) + status bar (dynamic) +
	// optional centered LLM status band (top spacer + status) + optional slash
	// suggestions + input (dynamic). The existing status-bar spacer is the
	// matching bottom half of the LLM status band.
	llmStatusRows := 0
	llmStatus := c.state.LLMCall.Get()
	if llmStatus != nil {
		llmStatusRows = 2
	}
	slashRows := 0
	if suggestions := c.slash.Get(); suggestions != nil && len(suggestions.Items) > 0 {
		slashRows = len(visibleSlashSuggestions(suggestions)) + 2
	}
	permissionRows := 0
	activeDecision := c.activeDecisionRequest()
	if activeDecision != nil && activeDecision.Kind != permissions.PromptKindAskUser {
		permissionRows = c.decisionDialogHeight(activeDecision)
	}
	activeAskUser := c.activeAskUserRequest()
	askUserRows := c.askUserDialogHeight(activeAskUser)
	compactDecision := (activeDecision != nil || activeAskUser != nil) && termHeight < 20
	taskItems := c.state.TaskViewItems.Get()
	taskViewRows := 0
	if len(taskItems) > 0 {
		// The collapsed task status is always one row. Expanded details remain
		// bounded so the composer and status chrome cannot be pushed off-screen.
		taskViewRows = 1
		if c.state.ExpandedView.Get() == "tasks" {
			taskViewRows = len(taskItems) + 1
		}
		if taskViewRows > 6 {
			taskViewRows = 6
		}
	}
	activityViewRows := 0
	if c.state.ExpandedView.Get() == "activities" {
		activityViewRows = len(c.state.ActivitySnapshot().Activities) + 3
		if activityViewRows > 10 {
			activityViewRows = 10
		}
	}
	goalViewRows := 0
	goalView := c.state.Goal.Get()
	if goalView != nil && len(goalView.Criteria) > 0 && termHeight >= 12 {
		goalViewRows = len(goalView.Criteria) + 1
		if goalViewRows > 6 {
			goalViewRows = 6
		}
	}
	decisionReceiptRows := 0
	if !compactDecision && c.state.DecisionReceipt.Get() != "" {
		decisionReceiptRows = 1
	}
	taskTopSpacingRows := 0
	if taskViewRows > 0 {
		taskTopSpacingRows = 1
	}
	chromeRows := 3 + 1 + statusRows + llmStatusRows + slashRows + permissionRows + askUserRows + goalViewRows + taskTopSpacingRows + taskViewRows + activityViewRows + decisionReceiptRows + inputRowHeight
	if compactDecision {
		llmStatusRows, slashRows, goalViewRows, taskTopSpacingRows, taskViewRows, activityViewRows = 0, 0, 0, 0, 0, 0
		chromeRows = permissionRows + askUserRows + inputRowHeight
	}

	// The skills checklist is a normal child of this root column, not a true
	// go-tui modal. Reserve its complete, precomputed bordered height before the
	// message area consumes the remainder. At extreme heights where the fixed
	// application chrome leaves no usable content row, render the checklist as
	// an exclusive compact surface so its lower border cannot be pushed outside
	// the terminal.
	var skillsMenu *SkillsMenuState
	var skillsLayout skillsPanelLayout
	if menu := c.state.SkillsMenu.Get(); menu != nil && menu.Visible {
		skillsMenu = menu
		available := termHeight - chromeRows
		if available < skillsPanelBorderRows+1 {
			return c.renderSkillsMenuAtHeight(menu, max(termHeight, 0))
		}
		maximum := available
		if share := termHeight * 2 / 3; share >= skillsPanelBorderRows && maximum > share {
			maximum = share
		}
		skillsLayout = c.skillsMenuLayout(menu, maximum)
		chromeRows += skillsLayout.PanelHeight
	}

	// Root: vertical flex column filling the screen
	root := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithHeightPercent(100),
		tui.WithWidthPercent(100),
		// A nil background is transparent in go-tui. Fill the entire root so
		// stale tmux panes and previous terminal frames cannot bleed through.
		tui.WithBackground(tui.NewStyle().Background(tui.Black)),
	)

	// --- Banner ---
	if !compactDecision {
		banner := c.renderBanner()
		root.AddChild(banner)
	}

	// --- Message area (scrollable, fills remaining space) ---
	if !compactDecision {
		msgArea := c.renderMessageArea(termHeight - chromeRows)
		root.AddChild(msgArea)
	}
	if !compactDecision && goalViewRows > 0 {
		root.AddChild(c.renderGoalView(goalView, goalViewRows))
	}
	if !compactDecision && taskViewRows > 0 {
		root.AddChild(tui.New(tui.WithHeight(taskTopSpacingRows), tui.WithWidthPercent(100)))
		root.AddChild(c.renderTaskView(taskItems, taskViewRows))
	} else {
		c.taskViewRef.Set(nil)
	}
	if !compactDecision && activityViewRows > 0 {
		root.AddChild(c.renderActivityView(c.state.ActivitySnapshot(), activityViewRows))
	}
	if !compactDecision && decisionReceiptRows > 0 {
		root.AddChild(c.renderDecisionReceipt(c.state.DecisionReceipt.Get()))
	}

	// --- Vertically centered LLM status band (if active) ---
	if !compactDecision && llmStatus != nil {
		root.AddChild(tui.New(tui.WithHeight(1), tui.WithWidthPercent(100)))
		root.AddChild(c.renderLLMStatus(llmStatus))
	}

	// --- Spacer above status bar ---
	if !compactDecision {
		root.AddChild(tui.New(tui.WithHeight(1), tui.WithWidthPercent(100)))
	}

	// --- Status bar (cost + context) ---
	if !compactDecision {
		root.AddChild(status)
	}

	// --- Session picker overlay ---
	if picker := c.state.SessionPicker.Get(); picker != nil && picker.Visible {
		root.AddChild(c.renderSessionPicker(picker))
	}
	if picker := c.state.ForkPicker.Get(); picker != nil && picker.Visible {
		root.AddChild(c.renderForkPicker(picker))
	}

	// --- Model picker overlay ---
	if mp := c.state.ModelPicker.Get(); mp != nil && mp.Visible {
		root.AddChild(c.renderModelPicker(mp))
	}
	if skillsMenu != nil && skillsLayout.PanelHeight > 0 {
		root.AddChild(c.renderSkillsMenuWithLayout(skillsMenu, skillsLayout))
	}

	// --- Permission dialog overlay ---
	if request := c.activeDecisionRequest(); request != nil && request.Kind != permissions.PromptKindAskUser {
		root.AddChild(c.renderDecisionDialog(request))
	}
	if request := c.activeAskUserRequest(); request != nil {
		root.AddChild(c.renderAskUserDialog(request))
	}

	// --- Input prompt ---
	if suggestions := c.slash.Get(); !compactDecision && suggestions != nil && len(suggestions.Items) > 0 {
		root.AddChild(c.renderSlashSuggestions(suggestions))
	}

	// The external container provides the visual border (rounded, cyan when
	// focused). TextArea itself has no border, so its wrapText() uses the
	// correct content width.
	currentMode := c.state.Mode.Get()
	promptText := currentMode.PromptPrefixInLanguage(c.state.Language.Get())
	promptWidth := len(promptText) + 1 // +1 for spacing
	inputBorder := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Row),
		tui.WithHeight(inputRowHeight),
		tui.WithWidthPercent(100),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
	)
	promptColor := tui.Green
	if currentMode == ModePlanEdit {
		promptColor = tui.Yellow
	}
	prompt := tui.New(
		tui.WithText(promptText),
		tui.WithTextStyle(tui.NewStyle().Foreground(promptColor).Bold()),
		tui.WithWidth(promptWidth),
		tui.WithHeight(taHeight),
	)
	inputBorder.AddChild(prompt)

	// Use MountPersistent so the framework's walkComponents/buildDispatchTable
	// discovers TextArea as a proper child component. This ensures:
	// 1. TextArea's KeyMap() (OnFocused bindings) appears in the dispatch table
	// 2. focusCheck is correctly wired to TextArea.IsFocused() (not RootComponent)
	// 3. Focus-gated bindings (AnyRune→insertChar, Enter→submit, etc.) only
	//    fire when TextArea has focus, preventing phantom submits/inserts.
	//
	// The factory returns the pre-created c.input instance so we keep a stable
	// reference for Clear(), setTextAreaField(), etc.
	var inputEl *tui.Element
	if app != nil {
		inputEl = app.MountPersistent(c, 0, func() tui.Component { return c.input })
	} else {
		inputEl = c.input.Render(nil)
	}
	inputBorder.AddChild(inputEl)
	root.AddChild(inputBorder)

	return root
}

func (c *RootComponent) renderTaskView(items []TaskViewItem, rows int) *tui.Element {
	expanded := c.state.ExpandedView.Get() == "tasks"
	container := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithHeight(rows),
		tui.WithWidthPercent(100),
	)
	disclosure := "▸"
	if expanded {
		disclosure = "▾"
	}
	summaryRow := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Row),
		tui.WithJustify(tui.JustifyCenter),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
	)
	summary := tui.New(
		tui.WithText(strings.TrimSpace(i18n.Format(c.state.Language.Get(), i18n.KeyTasksTitle, len(items)))+" "+disclosure),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
		tui.WithHeight(1),
		tui.WithTruncate(true),
	)
	c.taskViewRef.Set(summary)
	summaryRow.AddChild(summary)
	container.AddChild(summaryRow)
	if !expanded {
		return container
	}

	itemRows := rows - 1
	show := len(items)
	overflow := false
	if show > itemRows {
		overflow = true
		show = itemRows - 1
	}
	for i := 0; i < show; i++ {
		item := items[i]
		marker := "[ ]"
		style := tui.NewStyle()
		switch item.Status {
		case "in_progress":
			marker = "[~]"
			style = style.Foreground(tui.Yellow)
		case "completed":
			marker = "[x]"
			style = style.Foreground(tui.Green)
		}
		line := fmt.Sprintf(" %s #%s %s", marker, item.ID, item.Subject)
		if item.Owner != "" {
			line += " @" + item.Owner
		}
		if len(item.BlockedBy) > 0 {
			blocked := make([]string, 0, len(item.BlockedBy))
			for _, id := range item.BlockedBy {
				blocked = append(blocked, "#"+id)
			}
			line += i18n.Format(c.state.Language.Get(), i18n.KeyTasksBlockedBy, strings.Join(blocked, ", "))
		}
		container.AddChild(tui.New(
			tui.WithText(line),
			tui.WithTextStyle(style),
			tui.WithHeight(1),
			tui.WithWidthPercent(100),
			tui.WithTruncate(true),
		))
	}
	if overflow {
		container.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyTasksMore, len(items)-show)),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithHeight(1),
			tui.WithWidthPercent(100),
			tui.WithTruncate(true),
		))
	}
	return container
}

func (c *RootComponent) renderGoalView(current *GoalViewState, rows int) *tui.Element {
	container := tui.New(tui.WithDirection(tui.Column), tui.WithHeight(rows), tui.WithWidthPercent(100))
	met, total := goalAcceptanceProgress(current)
	title := i18n.Format(c.state.Language.Get(), i18n.KeyTUIGoalPanelTitle,
		i18n.RootGoalStatusLabel(c.state.Language.Get(), current.Status), current.Revision, met, total,
		sanitizeGoalDisplayText(current.Objective))
	container.AddChild(tui.New(
		tui.WithText(title), tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
		tui.WithHeight(1), tui.WithWidthPercent(100), tui.WithTruncate(true),
	))
	available := rows - 1
	show := len(current.Criteria)
	overflow := false
	if show > available {
		overflow = true
		show = max(available-1, 0)
	}
	for index := 0; index < show; index++ {
		criterion := current.Criteria[index]
		key := i18n.KeyTUIGoalCriterionPending
		style := tui.NewStyle().Dim()
		switch criterion.Status {
		case "met":
			key, style = i18n.KeyTUIGoalCriterionMet, tui.NewStyle().Foreground(tui.Green)
		case "unmet":
			key, style = i18n.KeyTUIGoalCriterionUnmet, tui.NewStyle().Foreground(tui.Yellow)
		}
		container.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), key, criterion.ID, sanitizeGoalDisplayText(criterion.Text))),
			tui.WithTextStyle(style), tui.WithHeight(1), tui.WithWidthPercent(100), tui.WithTruncate(true),
		))
	}
	if overflow {
		container.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyTUIGoalCriteriaMore, len(current.Criteria)-show)),
			tui.WithTextStyle(tui.NewStyle().Dim()), tui.WithHeight(1), tui.WithWidthPercent(100), tui.WithTruncate(true),
		))
	}
	return container
}

// renderBanner creates the persistent terminal header.
func (c *RootComponent) renderBanner() *tui.Element {
	lang := c.state.Language.Get()
	title := fmt.Sprintf("%s — %s/%s", brand.RuntimeName, bannerProviderName(c.state.Provider.Get()), c.state.Model.Get())

	if effort := c.state.ReasoningEffort.Get(); effort != "" {
		if info := ReasoningEffortInfoInLanguage(lang, effort); info.Label != "" {
			title += " [🧠 " + info.Label + "]"
		}
	}

	// Append context window if known
	if ctxK := c.state.ContextWindowK.Get(); ctxK != "" {
		title += fmt.Sprintf(" [%s]", ctxK)
	}

	// Append pricing if known (cost per 1M tokens)
	costIn := c.state.ModelCostIn.Get()
	costOut := c.state.ModelCostOut.Get()
	if costIn > 0 || costOut > 0 {
		title += fmt.Sprintf(" [%s]", fmtCostPair(costIn, costOut, c.state.ModelCostCurrency.Get()))
	}

	banner := tui.New(
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithHeight(3),
		tui.WithWidthPercent(100),
	)
	banner.AddChild(tui.New(
		tui.WithText(title),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))
	return banner
}

func bannerProviderName(providerName string) string {
	return strings.TrimPrefix(providerName, "custom-")
}

// renderMessageArea creates the scrollable message list.
func (c *RootComponent) renderMessageArea(maxHeight int) *tui.Element {
	if maxHeight < 3 {
		maxHeight = 3
	}

	opts := []tui.Option{
		tui.WithDirection(tui.Column),
		tui.WithFlexGrow(1),
		tui.WithWidthPercent(100),
		tui.WithScrollable(tui.ScrollVertical),
		tui.WithScrollOffset(0, c.scrollY.Get()),
		tui.WithGap(1),
	}
	// A permission prompt deliberately leaves the transcript readable. When
	// keyboard focus is moved there, outline the transcript viewport so it is
	// clear that vertical navigation will not change or confirm the decision.
	if request := c.activeDecisionRequest(); request != nil && request.Kind != permissions.PromptKindAskUser && c.decisionScrollTarget.Get() == decisionScrollTranscript {
		opts = append(opts,
			tui.WithBorder(tui.BorderSingle),
			tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		)
	}

	// When stickToBottom is true, add the deferred scroll-to-bottom option.
	// This ensures the element scrolls to the true bottom AFTER layout
	// computes the real content height, solving the stale-maxY problem
	// where the OnChange watcher's scrollToBottom() could only read the
	// previous frame's content height.
	if c.stickToBottom.Get() {
		opts = append(opts, tui.WithScrollToBottomOnLayout())
	}

	container := tui.New(opts...)
	// Bind ref so scrollBy/HandleMouse can query MaxScroll at runtime.
	c.contentRef.Set(container)
	// Header refs belong to the current element tree. Rebuild the map on each
	// render so mouse hit-testing never targets stale, scrolled-away elements.
	c.segmentRefs = tui.NewRefMap[string]()

	items := c.boundedTranscriptRenderItems(maxHeight)
	if len(items) == 0 {
		container.AddChild(c.renderHomeLogo(maxHeight))
		return container
	}
	focused := c.state.ActiveSessionInteraction().FocusedObservationID
	showAll := c.state.TranscriptShowAll.Get()
	for _, item := range items {
		if item.Segment != nil {
			container.AddChild(c.renderToolSegment(*item.Segment, focused, showAll))
			continue
		}
		if item.Message == nil || !transcriptMessageVisible(*item.Message, focused, showAll) {
			continue
		}
		container.AddChild(c.renderMessage(*item.Message))
	}

	return container
}

// boundedTranscriptRenderItems expands a bounded raw window to complete tool
// runs before grouping it. This keeps segment membership intact without
// rebuilding and copying the entire transcript on every streaming frame.
// Focused and pinned observations outside the window are included with their
// complete segment rather than as context-free fragments.
func (c *RootComponent) boundedTranscriptRenderItems(viewportRows int) []TranscriptRenderItem {
	messages := c.state.Messages.Get()
	if len(messages) == 0 {
		return nil
	}
	if viewportRows < 1 {
		viewportRows = 1
	}
	budget := viewportRows*4 + 32
	start := c.historyStart.Get()
	if start < 0 || start+budget >= len(messages) {
		start = len(messages) - budget
		if start < 0 {
			start = 0
		}
	}
	end := start + budget
	if end > len(messages) {
		end = len(messages)
	}

	ranges := []transcriptMessageRange{expandTranscriptMessageRange(messages, start, end)}
	focused := c.state.ActiveSessionInteraction().FocusedObservationID
	selectedIDs := make(map[string]struct{})
	focusedMessageIndex := -1
	if strings.HasPrefix(focused, "message:") {
		if index, err := strconv.Atoi(strings.TrimPrefix(focused, "message:")); err == nil && index >= 0 && index < len(messages) {
			focusedMessageIndex = index
			ranges = append(ranges, expandTranscriptMessageRange(messages, index, index+1))
		}
	} else if focused != "" {
		selectedIDs[focused] = struct{}{}
	}
	pinned := c.state.PinnedObservationSnapshot()
	for _, observation := range pinned {
		selectedIDs[observation.ID] = struct{}{}
	}

	foundIDs := make(map[string]struct{}, len(selectedIDs))
	if len(selectedIDs) > 0 {
		for index, message := range messages {
			if _, selected := selectedIDs[message.ObservationID]; !selected {
				continue
			}
			foundIDs[message.ObservationID] = struct{}{}
			ranges = append(ranges, expandTranscriptMessageRange(messages, index, index+1))
		}
	}
	ranges = mergeTranscriptMessageRanges(ranges)
	projected := make([]TranscriptRenderItem, 0, budget)
	for _, messageRange := range ranges {
		for _, item := range BuildTranscriptToolSegments(messages[messageRange.Start:messageRange.End]) {
			item.Start += messageRange.Start
			item.End += messageRange.Start
			projected = append(projected, item)
		}
	}

	result := make([]TranscriptRenderItem, 0, len(projected)+len(selectedIDs))
	for _, observation := range pinned {
		if _, found := foundIDs[observation.ID]; found {
			continue
		}
		message := messageFromObservation(observation, MsgToolCall)
		result = append(result, TranscriptRenderItem{Message: &message, Start: -1, End: 0})
		foundIDs[observation.ID] = struct{}{}
	}
	if focused != "" && focusedMessageIndex < 0 {
		if _, found := foundIDs[focused]; !found {
			if observation, ok := c.state.GetObservation(focused); ok {
				message := messageFromObservation(observation, MsgToolCall)
				result = append(result, TranscriptRenderItem{Message: &message, Start: -1, End: 0})
			}
		}
	}
	return append(result, projected...)
}

type transcriptMessageRange struct {
	Start int
	End   int
}

func expandTranscriptMessageRange(messages []Message, start, end int) transcriptMessageRange {
	start = max(0, min(start, len(messages)))
	end = max(start, min(end, len(messages)))
	startTool := start
	for startTool < end && isTransparentTranscriptInternalRow(messages[startTool]) {
		startTool++
	}
	if startTool < len(messages) && isTranscriptToolObservation(messages[startTool]) {
		current := startTool
		for {
			previous := current - 1
			for previous >= 0 && isTransparentTranscriptInternalRow(messages[previous]) {
				previous--
			}
			if previous < 0 || !isTranscriptToolObservation(messages[previous]) || !sameTranscriptToolScope(messages[previous], messages[current]) {
				break
			}
			start = previous
			current = previous
		}
	}
	endTool := end - 1
	for endTool >= start && isTransparentTranscriptInternalRow(messages[endTool]) {
		endTool--
	}
	if endTool >= start && isTranscriptToolObservation(messages[endTool]) {
		current := endTool
		for {
			next := current + 1
			for next < len(messages) && isTransparentTranscriptInternalRow(messages[next]) {
				next++
			}
			if next >= len(messages) || !isTranscriptToolObservation(messages[next]) || !sameTranscriptToolScope(messages[current], messages[next]) {
				break
			}
			end = next + 1
			current = next
		}
	}
	return transcriptMessageRange{Start: start, End: end}
}

func mergeTranscriptMessageRanges(ranges []transcriptMessageRange) []transcriptMessageRange {
	if len(ranges) < 2 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})
	merged := ranges[:1]
	for _, current := range ranges[1:] {
		last := &merged[len(merged)-1]
		if current.Start <= last.End {
			if current.End > last.End {
				last.End = current.End
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

func transcriptMessageVisible(message Message, focused string, showAll bool) bool {
	return showAll || !message.PresentationHidden || message.ObservationID == focused || message.Disclosure.UserPinned
}

func (c *RootComponent) toolSegmentExpanded(segment TranscriptToolSegment, focused string, showAll bool) bool {
	if showAll {
		return true
	}
	if transcriptToolSegmentOngoing(c.state.Messages.Get(), segment) {
		return true
	}
	if expanded, ok := c.state.toolSegmentExpansionOverride(segment.ID); ok {
		return expanded
	}
	for _, message := range segment.Messages {
		if message.ObservationID == focused || message.Disclosure.UserPinned {
			return true
		}
	}
	return segment.DefaultExpanded
}

const (
	subagentProgressOutputRows      = 10
	subagentProgressOutputTailRunes = 2400
)

func subagentProgressSegmentID(activity Activity) string {
	parentToolUseID := strings.TrimSpace(activity.Progress.ParentToolUseID)
	if parentToolUseID == "" {
		return ""
	}
	identity := strings.TrimSpace(activity.RunID)
	if identity == "" {
		identity = strings.TrimSpace(activity.Progress.AgentID)
	}
	id := "subagent-segment:" + parentToolUseID
	if identity != "" {
		id += ":" + identity
	}
	return id
}

func (c *RootComponent) subagentActivityForObservation(observation Observation) (Activity, bool) {
	parentToolUseID := strings.TrimSpace(observation.ToolUseID)
	if !strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent") || parentToolUseID == "" {
		return Activity{}, false
	}
	var selected Activity
	found := false
	for _, activity := range c.state.ActivitySnapshot().Activities {
		if strings.TrimSpace(activity.Progress.ParentToolUseID) != parentToolUseID {
			continue
		}
		if !found || subagentActivityPreferred(activity, selected) {
			selected, found = activity, true
		}
	}
	return selected, found
}

func provisionalSubagentActivityForObservation(observation Observation) (Activity, bool) {
	parentToolUseID := strings.TrimSpace(observation.ToolUseID)
	if !strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent") || parentToolUseID == "" {
		return Activity{}, false
	}
	outcome := observation.Outcome
	// An async launch result settles the wrapper, not the detached work. Until
	// the retained activity arrives, keep the single card in a safe running
	// state and never render the launch envelope or internal Agent identity.
	if observation.Presentation.Background && (outcome == OutcomeSucceeded || outcome == OutcomePartial) {
		outcome = OutcomeRunning
	}
	lifecycle := activityLifecycleForOutcome(outcome)
	if lifecycle == "" {
		lifecycle = ActivityLifecycleRunning
		outcome = OutcomeRunning
	}
	attention := ActivityAttention{Kind: ActivityAttentionNone}
	agentType, _ := observation.ToolInput["subagent_type"].(string)
	return Activity{
		ActivityEvent: ActivityEvent{
			ID:        "tool:" + parentToolUseID,
			SessionID: observation.SessionID,
			Kind:      ActivityAgent,
			Lifecycle: lifecycle,
			Attention: attention,
			Outcome:   outcome,
			Progress: ActivityProgress{
				AgentType:       strings.TrimSpace(agentType),
				ParentToolUseID: parentToolUseID,
				Phase:           "start",
			},
		},
	}, true
}

func subagentActivityPreferred(candidate, current Activity) bool {
	candidateRetained := strings.HasPrefix(candidate.ID, "background:")
	currentRetained := strings.HasPrefix(current.ID, "background:")
	if candidateRetained != currentRetained {
		return candidateRetained
	}
	if candidate.LastSequence != current.LastSequence {
		return candidate.LastSequence > current.LastSequence
	}
	return candidate.SourceSequence > current.SourceSequence
}

func (c *RootComponent) subagentActivityBySegmentID(id string) (Activity, bool) {
	if id == "" {
		return Activity{}, false
	}
	for _, activity := range c.state.ActivitySnapshot().Activities {
		if subagentProgressSegmentID(activity) == id {
			return activity, true
		}
	}
	return Activity{}, false
}

func (c *RootComponent) subagentProgressSegmentExpanded(activity Activity, showAll bool) bool {
	if showAll {
		return true
	}
	if c.subagentProgressSegmentOngoing(activity) {
		return true
	}
	id := subagentProgressSegmentID(activity)
	if expanded, ok := c.state.toolSegmentExpansionOverride(id); ok {
		return expanded
	}
	if isTerminalActivityLifecycle(activity.Lifecycle) {
		return false
	}
	return true
}

func (c *RootComponent) subagentProgressSegmentOngoing(activity Activity) bool {
	return !isTerminalActivityLifecycle(activity.Lifecycle) && transcriptObservationOngoing(c.state.Messages.Get(), "", activity.Progress.ParentToolUseID)
}

func (c *RootComponent) renderSubagentProgressSegment(callMessage Message, observation Observation, activity Activity) *tui.Element {
	expanded := c.subagentProgressSegmentExpanded(activity, c.state.TranscriptShowAll.Get())
	marker := "▸"
	if expanded {
		marker = "▾"
	}

	lang := c.state.Language.Get()
	titleParts := make([]string, 0, 4)
	if name := strings.TrimSpace(callMessage.ToolName); name != "" {
		titleParts = append(titleParts, name)
	}
	primaryState := activityPresentationState(activity.Lifecycle, ActivityAttention{Kind: ActivityAttentionNone})
	titleParts = append(titleParts, i18n.RuntimeActivityStateLabel(lang, string(primaryState)))
	preview := strings.TrimSpace(callMessage.Text)
	if preview != "" {
		titleParts = append(titleParts, preview)
	}
	if activity.Attention.Kind == ActivityAttentionReadyForReview && activity.Attention.Unread {
		titleParts = append(titleParts, i18n.Text(lang, i18n.KeySubagentSegmentResultPendingView))
	}
	style := tui.NewStyle().Foreground(tui.Cyan).Bold()
	if activity.Lifecycle == ActivityLifecycleFailed || activity.Lifecycle == ActivityLifecycleCancelled {
		style = tui.NewStyle().Foreground(tui.Red).Bold()
	}
	title := marker + " " + strings.Join(titleParts, " · ")
	header := renderSubagentSegmentHeader(title, formatSubagentHeaderMetrics(activity.Progress, lang), style)
	id := subagentProgressSegmentID(activity)
	c.segmentRefs.Put(id, header)

	if !expanded {
		segment := tui.New(
			tui.WithDirection(tui.Column),
			tui.WithHeight(1),
			tui.WithWidthPercent(100),
			tui.WithOverflow(tui.OverflowHidden),
		)
		segment.AddChild(header)
		return segment
	}

	meta := make([]string, 0, 4)
	if agentType := strings.TrimSpace(activity.Progress.AgentType); agentType != "" && !strings.Contains(preview, "["+agentType+"]") {
		meta = append(meta, agentType)
	}
	terminal := isTerminalActivityLifecycle(activity.Lifecycle)
	if phase := localizedSubagentProgressPhase(lang, activity.Progress.Phase); !terminal && phase != "" {
		meta = append(meta, phase)
	}
	if activity.Progress.Current > 0 && !strings.EqualFold(strings.TrimSpace(activity.Progress.Phase), "mcp_ready") {
		meta = append(meta, i18n.Format(lang, i18n.KeySubagentSegmentTurns, activity.Progress.Current))
	}
	if toolName := strings.TrimSpace(activity.Progress.LatestTool); toolName != "" {
		meta = append(meta, i18n.Format(lang, i18n.KeyREPLTUIToolName, toolName))
	}
	metaRow := tui.New(
		tui.WithText("  "+strings.Join(meta, " · ")),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
		tui.WithWrap(false),
		tui.WithTruncate(true),
	)

	outputRows := make([]string, 0, subagentProgressOutputRows)
	// The transcript viewport reserves its final column for the scrollbar.
	// Keep two cells for the card indent and one for that viewport column so a
	// wrapped line is never clipped by the parent scroll container.
	termWidth := c.termWidth
	if termWidth <= 0 {
		termWidth = 80
	}
	contentWidth := termWidth - 3
	if contentWidth <= 0 {
		contentWidth = 1
	}
	if terminal {
		outputRows = append(outputRows, i18n.Text(lang, i18n.KeySubagentSegmentResultSummary))
		conclusion := strings.Join(subagentTerminalResultLines(lang, observation), "\n")
		conclusionRows := wrapTerminalCellLines(conclusion, contentWidth)
		outputRows = append(outputRows, conclusionRows...)
	} else {
		liveOutput := subagentProgressOutputLinesAtWidth(boundedSubagentPresentationOutput(activity.Progress.Output), contentWidth, subagentProgressOutputRows)
		if len(liveOutput) > 0 {
			outputRows = append(outputRows, liveOutput...)
		} else if !subagentProgressHasStructuredSignal(activity.Progress) {
			outputRows = append(outputRows, i18n.Text(lang, i18n.KeySubagentSegmentWaiting))
		}
	}
	segment := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithHeight(2+len(outputRows)),
		tui.WithWidthPercent(100),
		tui.WithOverflow(tui.OverflowHidden),
	)
	segment.AddChild(header)
	segment.AddChild(metaRow)
	for _, line := range outputRows {
		row := tui.New(
			tui.WithText("  "+line),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithHeight(1),
			tui.WithWidthPercent(100),
			tui.WithWrap(false),
		)
		segment.AddChild(row)
	}
	return segment
}

func renderSubagentSegmentHeader(title, metrics string, style tui.Style) *tui.Element {
	if metrics == "" {
		return tui.New(
			tui.WithText(title),
			tui.WithTextStyle(style),
			tui.WithHeight(1),
			tui.WithWidthPercent(100),
			tui.WithWrap(false),
			tui.WithTruncate(true),
		)
	}

	header := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Row),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
		tui.WithOverflow(tui.OverflowHidden),
	)
	header.AddChild(tui.New(
		tui.WithText(title),
		tui.WithTextStyle(style),
		tui.WithWidth(terminalCellWidth(title)),
		tui.WithHeight(1),
		tui.WithFlexShrink(0),
		tui.WithWrap(false),
		tui.WithTruncate(true),
	))
	header.AddChild(tui.New(
		tui.WithText(" · "+metrics),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithWidth(terminalCellWidth(metrics)+3),
		tui.WithHeight(1),
		tui.WithFlexShrink(1),
		tui.WithWrap(false),
		tui.WithTruncate(true),
	))
	return header
}

func formatSubagentHeaderMetrics(progress ActivityProgress, lang i18n.Language) string {
	parts := make([]string, 0, 2)
	if usage := progress.Usage; usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CacheReadInputTokens > 0 || usage.CacheCreationInputTokens > 0 || usage.ServerToolUse.WebSearchRequests > 0) {
		breakdown, costKnown := cost.CalculateCost(progress.Model, cost.TokenUsage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			WebSearchRequests:        usage.ServerToolUse.WebSearchRequests,
		})
		sessionUsage := SessionUsage{
			InputTokens:       usage.TotalInputTokens(),
			OutputTokens:      max(usage.OutputTokens, 0),
			CacheReadTokens:   max(usage.CacheReadInputTokens, 0),
			CacheCreateTokens: max(usage.CacheCreationInputTokens, 0),
			CumulativeCost:    breakdown.TotalUSD,
		}
		if requestUsage := progress.LastRequestUsage; requestUsage != nil {
			sessionUsage.RoundUsageKnown = true
			sessionUsage.LastInputTokens = requestUsage.TotalInputTokens()
			sessionUsage.LastOutputTokens = max(requestUsage.OutputTokens, 0)
			sessionUsage.LastCacheReadTokens = max(requestUsage.CacheReadInputTokens, 0)
			sessionUsage.LastCacheCreateTokens = max(requestUsage.CacheCreationInputTokens, 0)
		}
		parts = append(parts, formatSessionUsageSummary(sessionUsage, costKnown, lang))
	}
	if progress.ElapsedMs > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyTUIAgentDuration, float64(progress.ElapsedMs)/1000))
	}
	return strings.Join(parts, " · ")
}

func subagentProgressHasStructuredSignal(progress ActivityProgress) bool {
	return strings.TrimSpace(progress.Phase) != "" ||
		strings.TrimSpace(progress.LatestTool) != "" ||
		progress.Current > 0 ||
		progress.ElapsedMs > 0 ||
		progress.TokensUsed > 0
}

func subagentTerminalResultLines(lang i18n.Language, observation Observation) []string {
	resultPrefix := strings.TrimSpace(i18n.Format(lang, i18n.KeyPresentationResultValue, ""))
	causePrefix := strings.TrimSpace(i18n.Format(lang, i18n.KeyPresentationCause, ""))
	for _, prefix := range []string{resultPrefix, causePrefix} {
		for _, detail := range observation.Presentation.DetailLines {
			detail = strings.TrimSpace(detail)
			if detail == "" || prefix == "" || !strings.HasPrefix(detail, prefix) {
				continue
			}
			conclusion := strings.TrimSpace(strings.TrimPrefix(detail, prefix))
			if conclusion == "" {
				continue
			}
			return strings.Split(RedactPresentationText(conclusion, 0), "\n")
		}
	}
	return nil
}

func localizedSubagentProgressPhase(lang i18n.Language, phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "running", "completed", "failed", "cancelled":
		return i18n.RuntimeActivityStateLabel(lang, strings.ToLower(strings.TrimSpace(phase)))
	default:
		return i18n.RootAgentPhaseLabel(lang, phase)
	}
}

func subagentProgressOutputLinesAtWidth(output string, width, limit int) []string {
	if limit <= 0 {
		return nil
	}
	lines := wrapTerminalCellLines(output, width)
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

func wrapTerminalCellLines(text string, width int) []string {
	if width <= 0 {
		width = 1
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	paragraphs := strings.Split(text, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		var current strings.Builder
		cells := 0
		for _, char := range paragraph {
			charWidth := tui.RuneWidth(char)
			if charWidth <= 0 {
				charWidth = 1
			}
			if cells > 0 && cells+charWidth > width {
				lines = append(lines, current.String())
				current.Reset()
				cells = 0
			}
			current.WriteRune(char)
			cells += charWidth
		}
		if current.Len() > 0 {
			lines = append(lines, current.String())
		}
	}
	return lines
}

func boundedSubagentPresentationOutput(output string) string {
	output = RedactPresentationText(output, 0)
	runes := []rune(output)
	if len(runes) > subagentProgressOutputTailRunes {
		return string(runes[len(runes)-subagentProgressOutputTailRunes:])
	}
	return output
}

func (c *RootComponent) renderToolSegment(segment TranscriptToolSegment, focused string, showAll bool) *tui.Element {
	expanded := c.toolSegmentExpanded(segment, focused, showAll)
	marker := "▸"
	if expanded {
		marker = "▾"
	}
	title := segment.Summary(c.state.Language.Get())
	style := tui.NewStyle().Foreground(tui.Cyan).Bold()
	if segment.Alert {
		style = tui.NewStyle().Foreground(tui.Red).Bold()
	}
	header := tui.New(
		tui.WithText(marker+" "+title),
		tui.WithTextStyle(style),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
	)
	c.segmentRefs.Put(segment.ID, header)

	group := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
	)
	group.AddChild(header)
	if !expanded {
		return group
	}
	for _, message := range segment.Messages {
		// PresentationHidden belongs to the older same-intent aggregate. An
		// explicitly opened transcript segment must still reveal every call.
		message.PresentationHidden = false
		member := tui.New(
			tui.WithDirection(tui.Column),
			tui.WithWidthPercent(100),
			tui.WithPaddingTRBL(0, 0, 0, 2),
		)
		member.AddChild(c.renderToolSegmentMember(message, focused, showAll))
		group.AddChild(member)
	}
	return group
}

func (c *RootComponent) renderToolSegmentMember(message Message, focused string, showAll bool) *tui.Element {
	if strings.EqualFold(strings.TrimSpace(message.ToolName), "Agent") {
		return c.renderMessage(message)
	}
	if showAll || message.IsError || message.Outcome != OutcomeSucceeded || message.ObservationID == focused || message.Disclosure.UserPinned {
		return c.renderMessage(message)
	}
	if observation, ok := c.state.GetObservation(message.ObservationID); ok {
		if observation.Disclosure.Level != DisclosureSummary {
			return c.renderMessage(message)
		}
		// The outer group owns disclosure here. Keep members as stable one-line
		// receipts and avoid rendering a second nested chevron.
		observation.Presentation.HasUsefulDetail = false
		observation.Aggregation = ObservationAggregation{}
		return c.renderToolPresentationLine(observation, DisclosureState{Level: DisclosureSummary})
	}
	// Opening the outer group lists quiet successes as one row each. Exact
	// details remain available through member focus/disclosure and show-all.
	return c.renderToolCallLine(message)
}

func (c *RootComponent) restoreInteractionViewport(interaction SessionInteraction) {
	start := -1
	if anchor := interaction.ScrollAnchorID; anchor != "" {
		messages := c.state.Messages.Get()
		if strings.HasPrefix(anchor, "message:") {
			if index, err := strconv.Atoi(strings.TrimPrefix(anchor, "message:")); err == nil && index >= 0 && index < len(messages) {
				start = index
			}
		} else {
			for index := range messages {
				if messages[index].ObservationID == anchor {
					start = index
					break
				}
			}
		}
	}
	c.historyStart.Set(start)
}

func (c *RootComponent) syncSessionViewFromState() {
	if c == nil || c.state == nil {
		return
	}
	interaction := c.state.ActiveSessionInteraction()
	interaction = interactionWithPendingImagePlaceholders(interaction, c.state.PendingImages.Get())
	if active := c.state.ActiveSessionInteraction(); interaction.InputDraft != active.InputDraft || interaction.InputCursor != active.InputCursor || interaction.InputCursorSet != active.InputCursorSet {
		c.state.SetInteractionEditor(interaction.InputDraft, interaction.InputCursor)
	}
	if sessionID := c.state.SessionID.Get(); sessionID != c.inputSelectionSession {
		c.input.ClearSelection()
		c.inputSelectionSession = sessionID
	}
	c.restoreInteractionViewport(interaction)
	if c.input.Text() != interaction.InputDraft {
		c.input.SetText(interaction.InputDraft)
	}
	cursor := utf8.RuneCountInString(interaction.InputDraft)
	if interaction.InputCursorSet {
		cursor = interaction.InputCursor
	}
	if c.input.CursorPosition() != cursor {
		c.input.SetCursorPosition(cursor)
	}
	c.slashDismissedForInput = interaction.SlashDismissedInput
	if interaction.SlashDismissedInput != "" && interaction.SlashDismissedInput == interaction.InputDraft {
		c.slash.Set(nil)
	} else {
		suggestions := computeSlashSuggestions(interaction.InputDraft, c.slashCommands, c.state.Language.Get())
		if suggestions != nil && interaction.SlashSelectedSet && len(suggestions.Items) > 0 {
			suggestions.Selected = interaction.SlashSelected
			if suggestions.Selected < 0 {
				suggestions.Selected = 0
			}
			if suggestions.Selected >= len(suggestions.Items) {
				suggestions.Selected = len(suggestions.Items) - 1
			}
		}
		c.slash.Set(suggestions)
	}
	c.scrollY.Set(interaction.ScrollOffset)
	c.stickToBottom.Set(interaction.ScrollOffset == 0 && interaction.ScrollAnchorID == "")
}

func (c *RootComponent) invalidateSessionActionMap() {
	if c == nil {
		return
	}
	c.segmentRefs = tui.NewRefMap[string]()
	if c.contentRef != nil {
		c.contentRef.Set(nil)
	}
}

func (c *RootComponent) setHistoryStart(start int) {
	c.historyStart.Set(start)
	if start < 0 {
		c.state.SetInteractionAnchor("")
		return
	}
	messages := c.state.Messages.Get()
	if start >= len(messages) {
		c.state.SetInteractionAnchor("")
		return
	}
	anchor := messages[start].ObservationID
	if anchor == "" {
		anchor = fmt.Sprintf("message:%d", start)
	}
	c.state.SetInteractionAnchor(anchor)
}

func (c *RootComponent) renderHomeLogo(maxHeight int) *tui.Element {
	if maxHeight < 4 {
		maxHeight = 4
	}
	termWidth := c.termWidth
	if termWidth <= 0 {
		termWidth = 80
	}

	home := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
		tui.WithHeight(maxHeight),
		tui.WithJustify(tui.JustifyCenter),
		tui.WithAlign(tui.AlignCenter),
		tui.WithGap(0),
	)

	logoLines := brand.TerminalLogoLines(termWidth)
	logoStyle := tui.NewStyle().Foreground(tui.Cyan).Bold()
	for _, line := range logoLines {
		// go-tui collapses Unicode whitespace while laying out text. The braille
		// blank is a one-cell empty glyph, so it preserves the fixed-cell wordmark.
		line = strings.ReplaceAll(line, " ", "\u2800")
		home.AddChild(tui.New(
			tui.WithText(line),
			tui.WithTextStyle(logoStyle),
			tui.WithHeight(1),
		))
	}

	if maxHeight >= len(logoLines)+2 {
		home.AddChild(tui.New(
			tui.WithText(brand.RuntimeName),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
			tui.WithHeight(1),
		))
	}
	if maxHeight >= len(logoLines)+4 {
		home.AddChild(tui.New(
			tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyBrandTagline)),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithHeight(1),
		))
	}

	return home
}

// renderMessage creates an element for a single message.
func (c *RootComponent) renderMessage(msg Message) *tui.Element {
	switch msg.Kind {
	case MsgUser:
		container := tui.New(tui.WithDisplay(tui.DisplayFlex), tui.WithDirection(tui.Column), tui.WithWidthPercent(100))
		// Single text element with a styled prefix: "You: " is green/bold,
		// the rest uses the default foreground. This avoids the Flex Row
		// HeightForWidth miscalculation that caused wrapped lines to be
		// invisible (Row gave the text child the full width for height
		// estimation, underestimating the wrapped line count).
		prefix := i18n.Text(c.state.Language.Get(), i18n.KeyUserPrefix)
		container.AddChild(tui.New(
			tui.WithText(prefix+msg.Text),
			tui.WithTextPrefix(prefix, tui.NewStyle().Foreground(tui.Green).Bold()),
			tui.WithWidthPercent(100),
		))

		// Show image attachments as [Image #N] tags
		for _, img := range msg.Images {
			tag := fmt.Sprintf("  %s", i18n.Format(c.state.Language.Get(), i18n.KeyImageAttachment, img.ID, img.MediaType))
			container.AddChild(tui.New(
				tui.WithText(tag),
				tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Dim()),
				tui.WithWidthPercent(100),
			))
		}
		return container

	case MsgAssistant:
		// Render assistant text with incremental Markdown rendering.
		// If the message has a StreamRenderer (streaming or finalized), use
		// its cached block-level output — this avoids re-running glamour on
		// the entire text every render cycle.
		// Fallback to full renderMarkdown() for messages without a
		// StreamRenderer (e.g. restored from history).
		container := tui.New(
			tui.WithDirection(tui.Column),
			tui.WithWidthPercent(100),
		)
		if msg.Stream != nil {
			for _, el := range msg.Stream.Elements() {
				container.AddChild(el)
			}
		} else {
			mdElements := renderMarkdown(msg.Text)
			for _, el := range mdElements {
				container.AddChild(el)
			}
		}
		return container

	case MsgAssistantThinking:
		return c.renderThinking(msg)

	case MsgToolCall:
		if msg.ObservationID != "" {
			return c.renderToolObservation(msg)
		}
		return c.renderToolCallLine(msg)

	case MsgToolResult:
		if msg.ObservationID != "" {
			return c.renderToolObservation(msg)
		}
		return c.renderToolResultBlock(msg)

	case MsgSendUserMessage:
		return c.renderSendUserMessage(msg)

	case MsgError:
		container := tui.New(
			tui.WithDirection(tui.Column),
			tui.WithWidthPercent(100),
		)
		container.AddChild(tui.New(
			tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyErrorPrefix)+msg.Text),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red).Bold()),
			tui.WithWidthPercent(100),
		))
		return container

	case MsgInfo:
		return tui.New(
			tui.WithText(msg.Text),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithWidthPercent(100),
		)

	case MsgSuccess:
		return tui.New(
			tui.WithText(msg.Text),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Green)),
			tui.WithWidthPercent(100),
		)

	case MsgWarning:
		return tui.New(
			tui.WithText(msg.Text),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Yellow)),
			tui.WithWidthPercent(100),
		)

	default:
		return tui.New(
			tui.WithText(msg.Text),
			tui.WithWidthPercent(100),
		)
	}
}

func (c *RootComponent) renderToolObservation(msg Message) *tui.Element {
	observation, ok := c.state.GetObservation(msg.ObservationID)
	if !ok {
		return c.renderToolResultBlock(msg)
	}
	disclosure := observation.Disclosure
	if c.state.TranscriptShowAll.Get() {
		disclosure.Level = DisclosureEvidence
	}

	container := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
	)
	callMessage := msg
	redactedInput, _ := RedactPresentationValue(observation.ToolInput).(map[string]any)
	callMessage.Text = toolInputPreview(observation.ToolName, redactedInput)
	if activity, found := c.subagentActivityForObservation(observation); found {
		container.AddChild(c.renderSubagentProgressSegment(callMessage, observation, activity))
		return container
	} else if activity, found := provisionalSubagentActivityForObservation(observation); found {
		container.AddChild(c.renderSubagentProgressSegment(callMessage, observation, activity))
		return container
	} else {
		container.AddChild(c.renderToolPresentationLine(observation, disclosure))
	}

	if disclosure.Level == DisclosureEvidence && len(redactedInput) > 0 {
		if encoded, err := json.MarshalIndent(redactedInput, "", "  "); err == nil {
			container.AddChild(tui.New(
				tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyObservationInput, indentObservationText(string(encoded), "    "))),
				tui.WithTextStyle(tui.NewStyle().Dim()),
				tui.WithWidthPercent(100),
			))
		}
	}

	if len(observation.ResultRefs) == 0 {
		return container
	}
	if disclosure.Level == DisclosureSummary {
		return container
	}
	if disclosure.Level == DisclosureDetail {
		lines := append([]string(nil), observation.Presentation.DetailLines...)
		if observation.Aggregation.Representative {
			lines = append(aggregatePresentationMemberLines(c.state, observation.Aggregation.GroupID), lines...)
		}
		if len(lines) == 0 {
			lines = []string{observationSummaryInLanguage(c.state.Language.Get(), observation)}
		}
		for _, line := range lines {
			container.AddChild(tui.New(
				tui.WithText("  "+line),
				tui.WithTextStyle(tui.NewStyle().Dim()),
				tui.WithWidthPercent(100),
			))
		}
		if observation.Presentation.DetailDiff != "" {
			for _, element := range renderDiffLines(observation.Presentation.DetailDiff) {
				container.AddChild(element)
			}
		}
		return container
	}

	for _, ref := range observation.ResultRefs {
		evidence, err := c.state.ReadDetail(ref)
		if err != nil {
			container.AddChild(tui.New(
				tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyObservationEvidenceUnavailable, err.Error())),
				tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red)),
				tui.WithWidthPercent(100),
			))
			continue
		}
		result := messageFromObservation(observation, MsgToolResult)
		result.Text = RedactPresentationText(string(evidence), 0)
		result.Collapsed = false
		container.AddChild(c.renderToolResultBlock(result))
	}
	if disclosure.Level == DisclosureEvidence {
		for _, ref := range observation.EnvelopeRefs {
			evidence, err := c.state.ReadDetail(ref)
			if err != nil {
				container.AddChild(tui.New(tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyObservationStructuredUnavailable, err.Error())), tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red))))
				continue
			}
			container.AddChild(tui.New(
				tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyObservationStructuredEvidence, indentObservationText(redactStructuredPresentationEvidence(c.state.Language.Get(), evidence), "    "))),
				tui.WithTextStyle(tui.NewStyle().Dim()),
				tui.WithWidthPercent(100),
			))
		}
		presentationID, actorID, workUnitID := observationPresentationIdentity(observation)
		identity := i18n.Format(c.state.Language.Get(), i18n.KeyObservationEvidenceID, presentationID)
		if actorID != "" || workUnitID != "" {
			identity += i18n.Format(c.state.Language.Get(), i18n.KeyObservationEvidenceIdentity, actorID, workUnitID)
		}
		container.AddChild(tui.New(
			tui.WithText(identity),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithWidthPercent(100),
		))
		if observation.Aggregation.Representative {
			for _, line := range aggregatePresentationMemberLines(c.state, observation.Aggregation.GroupID) {
				container.AddChild(tui.New(
					tui.WithText("  "+line),
					tui.WithTextStyle(tui.NewStyle().Dim()),
					tui.WithWidthPercent(100),
				))
			}
		}
	}
	return container
}

func (c *RootComponent) renderToolPresentationLine(observation Observation, disclosure DisclosureState) *tui.Element {
	lang := c.state.Language.Get()
	summary := observationSummaryInLanguage(lang, observation)
	if strings.TrimSpace(summary) == "" {
		summary = semanticToolActionInLanguage(lang, observation.ToolName)
	}
	icon, color := toolPresentationIconAndColor(observation.Presentation, observation.Outcome)
	marker := ""
	if observation.Presentation.HasUsefulDetail {
		marker = "  ▸"
		if disclosure.Level != DisclosureSummary {
			marker = "  ▾"
		}
	}
	prefix := icon + " "
	return tui.New(
		tui.WithText(prefix+summary+marker),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithTextPrefix(prefix, tui.NewStyle().Foreground(color)),
		tui.WithWidthPercent(100),
	)
}

func toolPresentationIconAndColor(presentation FormattedPresentation, outcome ObservationOutcome) (string, tui.Color) {
	if presentation.Retrying || presentation.Lifecycle == PresentationLifecycleRetrying {
		return "↻", tui.Yellow
	}
	switch presentation.Lifecycle {
	case PresentationLifecycleSpawning, PresentationLifecycleQueued:
		return "◌", tui.Cyan
	case PresentationLifecycleWaiting, PresentationLifecycleBlocked:
		return "?", tui.Yellow
	case PresentationLifecycleFailed:
		return "✗", tui.Red
	case PresentationLifecycleCancelled:
		return "–", tui.White
	}
	switch outcome {
	case OutcomeSucceeded:
		if presentation.Warning {
			return "!", tui.Yellow
		}
		return "✓", tui.Green
	case OutcomePartial, OutcomeTimedOut:
		return "!", tui.Yellow
	case OutcomeFailed, OutcomeDenied, OutcomeEscaped, OutcomeOrphan, OutcomeConflict:
		return "✗", tui.Red
	case OutcomeCancelled, OutcomeShutdown:
		return "–", tui.White
	default:
		return "⚡", tui.Yellow
	}
}

func aggregatePresentationMemberLines(state *AppState, groupID string) []string {
	if state == nil || groupID == "" {
		return nil
	}
	group, ok := state.GetObservationAggregate(groupID)
	if !ok {
		return nil
	}
	lang := state.Language.Get()
	lines := []string{i18n.Format(lang, i18n.KeyAggregateMembers, len(group.MemberIDs))}
	stateLabel := i18n.Text(lang, i18n.KeyAggregateLive)
	if group.Frozen {
		stateLabel = i18n.Text(lang, i18n.KeyAggregateFrozen)
	}
	lines = append(lines, i18n.Format(lang, i18n.KeyAggregateState, stateLabel, group.ObjectCount, group.EvidenceCount))
	if len(group.ObjectSamples) > 0 {
		lines = append(lines, i18n.Format(lang, i18n.KeyAggregateObjects, strings.Join(group.ObjectSamples, ", ")))
	}
	const visibleMemberLimit = 20
	for index, id := range group.MemberIDs {
		if index >= visibleMemberLimit {
			lines = append(lines, i18n.Format(lang, i18n.KeyAggregateMoreMembers, len(group.MemberIDs)-visibleMemberLimit))
			break
		}
		summary := i18n.Text(lang, i18n.KeyAggregateEvidenceAvailable)
		presentationID := id
		if observation, exists := state.GetObservation(id); exists {
			summary = observation.Presentation.Summary
			presentationID, _, _ = observationPresentationIdentity(observation)
		}
		lines = append(lines, fmt.Sprintf("%d. %s - %s", index+1, presentationID, summary))
	}
	return lines
}

func observationPresentationIdentity(observation Observation) (id, actorID, workUnitID string) {
	id, actorID, workUnitID = observation.ID, observation.ActorID, observation.WorkUnitID
	if observation.PresentationID != "" {
		id = observation.PresentationID
	}
	if observation.PresentationActorID != "" {
		actorID = observation.PresentationActorID
	}
	if observation.PresentationWorkUnitID != "" {
		workUnitID = observation.PresentationWorkUnitID
	}
	return id, actorID, workUnitID
}

func redactStructuredPresentationEvidence(lang i18n.Language, evidence []byte) string {
	var value any
	if err := json.Unmarshal(evidence, &value); err != nil {
		return i18n.Text(lang, i18n.KeyTUIStructuredEvidenceDecode)
	}
	redacted := redactPresentationEvidenceValue(value)
	encoded, err := json.MarshalIndent(redacted, "", "  ")
	if err != nil {
		return i18n.Text(lang, i18n.KeyTUIStructuredEvidenceEncode)
	}
	return string(encoded)
}

func redactPresentationEvidenceValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitivePresentationKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactPresentationEvidenceValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = redactPresentationEvidenceValue(item)
		}
		return out
	case string:
		return RedactPresentationText(typed, 0)
	default:
		return typed
	}
}

func indentObservationText(text, prefix string) string {
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}

func (c *RootComponent) renderSendUserMessage(msg Message) *tui.Element {
	container := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
	)
	if msg.Brief == nil {
		return container
	}
	output := *msg.Brief
	if msg.BriefMode == presentation.SendUserMessageRenderTranscript {
		container.AddChild(tui.New(
			tui.WithText("*"),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan)),
		))
	}
	if msg.BriefMode == presentation.SendUserMessageRenderBriefOnly {
		label := "Claude"
		// Use the event's own timestamp as the display reference. Relative-to-
		// render-time labels cross day/week thresholds after /resume and make the
		// same durable view render different cells later.
		reference, _ := time.Parse(time.RFC3339Nano, output.SentAt)
		if timestamp := presentation.FormatSendUserMessageTimestampInLanguage(c.state.Language.Get(), output.SentAt, reference); timestamp != "" {
			label += " " + timestamp
		}
		container.AddChild(tui.New(
			tui.WithText(label),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
			tui.WithWidthPercent(100),
		))
	}
	if output.Message != "" {
		for _, element := range renderMarkdown(output.Message) {
			container.AddChild(element)
		}
	}
	for _, attachment := range output.Attachments {
		container.AddChild(tui.New(
			tui.WithText(presentation.FormatSendUserMessageAttachmentInLanguage(c.state.Language.Get(), attachment)),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithWidthPercent(100),
		))
	}
	return container
}

// renderThinking renders the complete provider reasoning text. Thinking is
// never collapsed: truncating a streamed block makes an intact reasoning
// message look as though transport or persistence lost content.
func (c *RootComponent) renderThinking(msg Message) *tui.Element {
	container := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
	)
	container.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyThinkingTitle)),
		tui.WithTextStyle(tui.NewStyle().Dim().Italic()),
	))
	for _, line := range strings.Split(msg.Text, "\n") {
		container.AddChild(tui.New(
			tui.WithText("  "+line),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithWidthPercent(100),
		))
	}
	return container
}

// renderToolCallLine renders a standalone ToolCall (not grouped with a result).
func (c *RootComponent) renderToolCallLine(msg Message) *tui.Element {
	if msg.ObservationID != "" {
		if observation, ok := c.state.GetObservation(msg.ObservationID); ok {
			return c.renderToolPresentationLine(observation, observation.Disclosure)
		}
	}
	lang := c.state.Language.Get()
	summary := semanticToolActionInLanguage(lang, msg.ToolName)
	if msg.Text != "" {
		summary += " · " + msg.Text
	}
	presentation := FormattedPresentation{Lifecycle: PresentationLifecycleRunning, Outcome: msg.Outcome}
	icon, color := toolPresentationIconAndColor(presentation, msg.Outcome)
	prefix := icon + " "
	return tui.New(
		tui.WithText(prefix+summary),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithTextPrefix(prefix, tui.NewStyle().Foreground(color)),
		tui.WithWidthPercent(100),
	)
}

// renderToolResultBlock renders a ToolResult with optional diff coloring.
func (c *RootComponent) renderToolResultBlock(msg Message) *tui.Element {
	if lines, ok := renderStructuredToolResultLinesInLanguage(c.state.Language.Get(), msg); ok {
		return renderToolResultLines(lines, msg.IsError)
	}

	if msg.IsError {
		// Error results: always shown expanded in red
		container := tui.New(
			tui.WithDirection(tui.Column),
			tui.WithWidthPercent(100),
		)
		container.AddChild(tui.New(
			tui.WithText("  ✗ "+i18n.Text(c.state.Language.Get(), i18n.KeyTUIToolErrorHeader)),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red).Bold()),
		))
		for _, line := range strings.Split(msg.Text, "\n") {
			container.AddChild(tui.New(
				tui.WithText("    "+line),
				tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red).Dim()),
				tui.WithWidthPercent(100),
			))
		}
		return container
	}

	if msg.Collapsed {
		// Collapsed: show one-line summary
		lineCount := strings.Count(msg.Text, "\n") + 1
		summary := i18n.Format(c.state.Language.Get(), i18n.KeyTUIAdditionalLines, lineCount)
		if msg.ToolName != "" {
			summary = i18n.Format(c.state.Language.Get(), i18n.KeyTUIToolAdditionalLines, msg.ToolName, lineCount)
		}
		return tui.New(
			tui.WithText(summary),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithWidthPercent(100),
		)
	}

	// Expanded: check for diff content and render accordingly
	if isDiffContent(msg.Text) {
		container := tui.New(
			tui.WithDirection(tui.Column),
			tui.WithWidthPercent(100),
		)
		for _, el := range renderDiffLines(msg.Text) {
			container.AddChild(el)
		}
		return container
	}

	// Regular expanded output
	return tui.New(
		tui.WithText("  ↳ "+msg.Text),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithWidthPercent(100),
	)
}

func renderStructuredToolResultLinesInLanguage(lang i18n.Language, msg Message) ([]string, bool) {
	if msg.Outcome != OutcomeUnknown && msg.Outcome != OutcomeSucceeded && msg.Outcome != OutcomeRunning {
		lines := []string{toolLabelInLanguage(lang, msg.ToolName) + " " + observationOutcomeLabelInLanguage(lang, msg.Outcome)}
		if trimmed := strings.TrimSpace(msg.Text); trimmed != "" {
			lines = append(lines, trimmed)
		}
		return lines, true
	}
	if msg.IsError {
		lines := []string{toolLabelInLanguage(lang, msg.ToolName) + " " + observationOutcomeLabelInLanguage(lang, OutcomeUnknown)}
		if trimmed := strings.TrimSpace(msg.Text); trimmed != "" {
			lines = append(lines, trimmed)
		}
		return lines, true
	}
	if lines, ok := agentToolResultLinesInLanguage(lang, msg.ToolName, msg.Text, false); ok {
		return lines, true
	}
	return nil, false
}

func renderToolResultLines(lines []string, isError bool) *tui.Element {
	container := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
	)
	style := tui.NewStyle().Dim()
	if isError {
		style = tui.NewStyle().Foreground(tui.Red).Dim()
	}
	for i, line := range lines {
		prefix := "  ↳ "
		if isError {
			prefix = "  ✗ "
		}
		if i > 0 {
			prefix = "    "
		}
		container.AddChild(tui.New(
			tui.WithText(prefix+line),
			tui.WithTextStyle(style),
			tui.WithWidthPercent(100),
		))
	}
	return container
}

func toolLabelInLanguage(lang i18n.Language, toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return i18n.Text(lang, i18n.KeyTUIToolFallback)
	}
	return toolName
}

type agentResultContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type agentResultPayload struct {
	IsAsync           bool                      `json:"isAsync"`
	Status            string                    `json:"status"`
	Prompt            string                    `json:"prompt"`
	Description       string                    `json:"description"`
	AgentID           string                    `json:"agentId"`
	AgentIDSnake      string                    `json:"agent_id"`
	AgentType         string                    `json:"agentType"`
	AgentTypeSnake    string                    `json:"agent_type"`
	Name              string                    `json:"name"`
	TeamName          string                    `json:"team_name"`
	OutputFile        string                    `json:"outputFile"`
	OutputFileSnake   string                    `json:"output_file"`
	CanReadOutputFile bool                      `json:"canReadOutputFile"`
	Message           string                    `json:"message"`
	Content           []agentResultContentBlock `json:"content"`
	TotalDurationMs   int64                     `json:"totalDurationMs"`
	TotalTokens       int                       `json:"totalTokens"`
	TotalToolUseCount int                       `json:"totalToolUseCount"`
}

func agentToolResultLinesInLanguage(lang i18n.Language, toolName, content string, isError bool) ([]string, bool) {
	if isError {
		return nil, false
	}
	var payload agentResultPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &payload); err != nil {
		return nil, false
	}
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		return nil, false
	}
	if !isAgentResultPayload(toolName, payload) {
		return nil, false
	}

	agentID := firstNonEmptyString(payload.AgentID, payload.AgentIDSnake)
	agentType := firstNonEmptyString(payload.AgentType, payload.AgentTypeSnake)
	outputFile := firstNonEmptyString(payload.OutputFile, payload.OutputFileSnake)
	description := firstNonEmptyString(payload.Description, payload.Name)
	switch status {
	case "completed":
		summary := i18n.Text(lang, i18n.KeyTUIAgentCompleted)
		if agentID != "" {
			summary += ": " + agentID
		}
		details := compactAgentDetails(lang, agentType, payload.TotalToolUseCount, payload.TotalTokens, payload.TotalDurationMs)
		if details != "" {
			summary += " (" + details + ")"
		}
		lines := []string{summary}
		if preview := agentContentPreview(payload.Content); preview != "" {
			lines = append(lines, preview)
		}
		return lines, true
	case "async_launched":
		summary := i18n.Text(lang, i18n.KeyTUIAgentBackgrounded)
		if agentID != "" {
			summary += ": " + agentID
		}
		if description != "" {
			summary += " - " + description
		}
		lines := []string{summary}
		if outputFile != "" {
			lines = append(lines, i18n.Format(lang, i18n.KeyTUIAgentOutput, outputFile))
		}
		if payload.Message != "" {
			lines = append(lines, payload.Message)
		}
		return lines, true
	case "teammate_spawned":
		summary := i18n.Text(lang, i18n.KeyTUITeammateSpawned)
		if payload.Name != "" {
			summary += ": " + payload.Name
		} else if agentID != "" {
			summary += ": " + agentID
		}
		if payload.TeamName != "" {
			summary += " (" + payload.TeamName + ")"
		}
		lines := []string{summary}
		if outputFile != "" {
			lines = append(lines, i18n.Format(lang, i18n.KeyTUIAgentOutput, outputFile))
		}
		return lines, true
	case "cancelled", "canceled":
		return []string{i18n.Format(lang, i18n.KeyTUIAgentStatus, i18n.TUIOutcomeLabel(lang, "cancelled"))}, true
	case "failed", "error":
		return []string{i18n.Format(lang, i18n.KeyTUIAgentStatus, i18n.TUIOutcomeLabel(lang, "failed"))}, true
	default:
		return nil, false
	}
}

func isAgentResultPayload(toolName string, payload agentResultPayload) bool {
	if strings.EqualFold(strings.TrimSpace(toolName), "Agent") {
		return true
	}
	if payload.AgentID != "" || payload.AgentIDSnake != "" || payload.AgentType != "" || payload.AgentTypeSnake != "" {
		return true
	}
	return payload.IsAsync || payload.Status == "teammate_spawned"
}

func compactAgentDetails(lang i18n.Language, agentType string, tools, tokens int, durationMs int64) string {
	var parts []string
	if agentType != "" {
		parts = append(parts, agentType)
	}
	if tools > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyTUIAgentTools, tools))
	}
	if tokens > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyTUIAgentTokens, tokens))
	}
	if durationMs > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyTUIAgentDuration, float64(durationMs)/1000))
	}
	return strings.Join(parts, ", ")
}

func agentContentPreview(blocks []agentResultContentBlock) string {
	for _, block := range blocks {
		if strings.EqualFold(block.Type, "text") || block.Type == "" {
			text := strings.Join(strings.Fields(block.Text), " ")
			return truncateRunes(text, 140, "...")
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

const (
	llmWorkingShimmerFrameInterval = 32 * time.Millisecond
	llmWorkingShimmerSweepDuration = 2 * time.Second
	llmWorkingShimmerPadding       = 10
	llmWorkingShimmerBandHalfWidth = 5.0
)

var llmWorkingShimmerStart = time.Now()

type llmWorkingShimmerPalette struct {
	base      [3]uint8
	highlight [3]uint8
}

func currentLLMWorkingShimmerPalette() llmWorkingShimmerPalette {
	configured := theme.Current()
	foreground, foregroundErr := tui.HexColor(configured.Foreground)
	background, backgroundErr := tui.HexColor(configured.Background)
	if foregroundErr == nil && backgroundErr == nil &&
		foreground.Type() == tui.ColorRGB && background.Type() == tui.ColorRGB {
		foregroundR, foregroundG, foregroundB := foreground.RGB()
		backgroundR, backgroundG, backgroundB := background.RGB()
		return llmWorkingShimmerPalette{
			base:      [3]uint8{foregroundR, foregroundG, foregroundB},
			highlight: [3]uint8{backgroundR, backgroundG, backgroundB},
		}
	}
	if detectTerminalBackgroundPreference() == terminalBackgroundLight {
		return llmWorkingShimmerPalette{
			base:      [3]uint8{36, 41, 47},
			highlight: [3]uint8{255, 255, 255},
		}
	}
	return llmWorkingShimmerPalette{
		base:      [3]uint8{230, 237, 243},
		highlight: [3]uint8{0, 0, 0},
	}
}

func formatLLMStatusDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	rounded := duration.Round(time.Second)
	if rounded >= time.Minute {
		return rounded.String()
	}
	return formatPresentationDuration(duration.Milliseconds())
}

func llmWorkingShimmerSpans(text string, elapsed time.Duration, trueColor bool) []tui.StyledSpan {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if elapsed < 0 {
		elapsed = 0
	}
	period := len(runes) + 2*llmWorkingShimmerPadding
	position := int(float64(elapsed%llmWorkingShimmerSweepDuration) /
		float64(llmWorkingShimmerSweepDuration) * float64(period))
	return llmWorkingShimmerSpansAtPositionWithPalette(
		runes, position, trueColor, currentLLMWorkingShimmerPalette())
}

func llmWorkingShimmerSpansAtPosition(runes []rune, position int) []tui.StyledSpan {
	return llmWorkingShimmerSpansAtPositionWithPalette(
		runes, position, false, llmWorkingShimmerPalette{})
}

func llmWorkingShimmerSpansAtPositionWithPalette(
	runes []rune,
	position int,
	trueColor bool,
	palette llmWorkingShimmerPalette,
) []tui.StyledSpan {
	spans := make([]tui.StyledSpan, 0, len(runes))
	for index, character := range runes {
		distance := math.Abs(float64(index + llmWorkingShimmerPadding - position))
		intensity := 0.0
		if distance <= llmWorkingShimmerBandHalfWidth {
			x := math.Pi * distance / llmWorkingShimmerBandHalfWidth
			intensity = 0.5 * (1 + math.Cos(x))
		}

		style := tui.NewStyle()
		if trueColor {
			alpha := min(max(intensity*0.9, 0), 1)
			blend := func(highlight, base uint8) uint8 {
				return uint8(float64(highlight)*alpha + float64(base)*(1-alpha))
			}
			style = style.Foreground(tui.RGBColor(
				blend(palette.highlight[0], palette.base[0]),
				blend(palette.highlight[1], palette.base[1]),
				blend(palette.highlight[2], palette.base[2]),
			)).Bold()
		} else {
			switch {
			case intensity < 0.2:
				style = style.Dim()
			case intensity >= 0.6:
				style = style.Bold()
			}
		}
		spans = append(spans, tui.StyledSpan{Text: string(character), Style: style})
	}
	return spans
}

func (c *RootComponent) renderLLMStatus(status *LLMCallStatus) *tui.Element {
	row := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Row),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
		tui.WithTruncate(true),
	)
	lang := c.state.Language.Get()
	if status.Phase == LLMCallWorking {
		requestDuration := "—"
		if status.HasRequestDuration {
			requestDuration = formatLLMStatusDuration(status.RequestDuration)
		}
		firstToken := "—"
		if status.HasFirstToken {
			firstToken = formatLLMStatusDuration(status.FirstTokenDuration)
		}

		total := status.TotalDuration
		now := time.Now()
		if c.now != nil {
			now = c.now()
		}
		if !status.WorkStartedAt.IsZero() {
			total = now.Sub(status.WorkStartedAt)
		} else if !status.UpdatedAt.IsZero() {
			total += now.Sub(status.UpdatedAt)
		}
		if total < 0 {
			total = 0
		}

		elapsed := time.Since(llmWorkingShimmerStart)
		trueColor := tui.DetectCapabilities().TrueColor
		if c.app != nil && c.app.Terminal() != nil {
			trueColor = c.app.Terminal().Caps().TrueColor
		}
		spans := []tui.StyledSpan{{Text: "  ", Style: tui.NewStyle()}}
		spans = append(spans, llmWorkingShimmerSpans("•", elapsed, trueColor)...)
		spans = append(spans, tui.StyledSpan{Text: " ", Style: tui.NewStyle()})
		spans = append(spans, llmWorkingShimmerSpans(i18n.Text(lang, i18n.KeyActivityWorking), elapsed, trueColor)...)
		dimStyle := tui.NewStyle().Dim()
		spans = append(spans,
			tui.StyledSpan{Text: " " + i18n.Format(lang, i18n.KeyLLMRequestInterruptStatus,
				formatLLMStatusDuration(total)), Style: dimStyle},
			tui.StyledSpan{Text: "  " + i18n.Format(lang, i18n.KeyLLMRequestMetrics,
				requestDuration, firstToken), Style: dimStyle},
		)
		row.AddChild(tui.New(
			tui.WithStyledSpans(spans),
			tui.WithWrap(false),
			tui.WithTruncate(true),
		))
		return row
	}

	primary := i18n.Text(lang, i18n.KeyLLMRequestProblem)
	row.AddChild(tui.New(
		tui.WithText("  "+primary),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Yellow)),
	))

	var detail string
	if status.Phase == LLMCallRetrying {
		detail = i18n.Format(lang, i18n.KeyLLMRequestRetrying, status.Attempt, status.MaxRetries, formatPresentationDuration(status.RetryDelay.Milliseconds()), status.Error)
	} else {
		detail = i18n.Format(lang, i18n.KeyLLMRequestError, status.Error)
	}
	row.AddChild(tui.New(
		tui.WithText("  "+detail),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithTruncate(true),
	))
	return row
}

func (c *RootComponent) tickLLMWorkingShimmer() {
	status := c.state.LLMCall.Get()
	if status == nil || status.Phase != LLMCallWorking {
		if c.llmWorkingFrame.Get() != 0 {
			c.llmWorkingFrame.Set(0)
		}
		return
	}
	c.llmWorkingFrame.Set(c.llmWorkingFrame.Get() + 1)
}

type llmWorkingShimmerWatcher struct {
	root *RootComponent
}

func (w llmWorkingShimmerWatcher) Start(eventQueue chan<- func(), stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(llmWorkingShimmerFrameInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				status := w.root.state.LLMCall.Get()
				if status == nil || status.Phase != LLMCallWorking {
					continue
				}
				select {
				case eventQueue <- w.root.tickLLMWorkingShimmer:
				case <-stopCh:
					return
				}
			}
		}
	}()
}

func (c *RootComponent) renderDecisionReceipt(text string) *tui.Element {
	return tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyDecisionReceipt, text)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
		tui.WithTruncate(true),
	)
}

func (c *RootComponent) renderActivityView(snapshot ActivitySnapshot, height int) *tui.Element {
	container := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithWidthPercent(100),
		tui.WithHeight(height),
		tui.WithBorder(tui.BorderSingle),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
	)
	container.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyActivitiesTitle)), tui.WithTextStyle(tui.NewStyle().Bold())))
	limit := height - 3
	if limit < 0 {
		limit = 0
	}
	offset := c.state.ActivityViewOffset.Get()
	if offset < 0 {
		offset = 0
	}
	previousWork, previousActor := "", ""
	havePreviousGroup := false
	for index, activity := range snapshot.Activities {
		if index < offset {
			continue
		}
		if index >= offset+limit {
			break
		}
		actions := make([]string, len(activity.Actions))
		for i := range activity.Actions {
			actions[i] = i18n.RuntimeActivityActionLabel(c.state.Language.Get(), string(activity.Actions[i]))
		}
		workGroup, actorGroup := activityWorkGroupKey(activity), activityActorGroupKey(activity)
		workLabel, actorLabel := workGroup, actorGroup
		if activity.PresentationWorkUnitID != "" {
			workLabel = activity.PresentationWorkUnitID
		}
		if workLabel == "" {
			workLabel = i18n.Text(c.state.Language.Get(), i18n.KeyActivityUnassigned)
		}
		if actorLabel == "" {
			actorLabel = i18n.Text(c.state.Language.Get(), i18n.KeyActivityUnassigned)
		}
		groupPrefix := "    "
		if !havePreviousGroup || workGroup != previousWork {
			groupPrefix = i18n.Format(c.state.Language.Get(), i18n.KeyActivityWorkActor, workLabel, actorLabel)
		} else if actorGroup != previousActor {
			groupPrefix = i18n.Format(c.state.Language.Get(), i18n.KeyActivityActor, actorLabel)
		}
		previousWork, previousActor, havePreviousGroup = workGroup, actorGroup, true
		marker := " "
		if c.state.ActivityFocus.Get() == activity.ID {
			marker = ">"
		}
		progress := activityProgressTextInLanguage(c.state.Language.Get(), activity.Progress)
		var line string
		if c.termWidth > 0 && c.termWidth < 60 {
			status := i18n.RuntimeActivityStateLabel(c.state.Language.Get(), string(activity.State))
			if outcome := observationOutcomeLabelInLanguage(c.state.Language.Get(), activity.Outcome); outcome != i18n.Text(c.state.Language.Get(), i18n.KeyTUIOutcomeUnknown) && outcome != status {
				status += "/" + outcome
			}
			line = marker + " " + status
			if progress != "" {
				line += " " + progress
			}
			if activity.Name != "" {
				line += " · " + activity.Name
			}
		} else {
			line = i18n.Format(c.state.Language.Get(), i18n.KeyActivityDetail, marker, groupPrefix, activity.ID,
				i18n.RuntimeActivityKindLabel(c.state.Language.Get(), string(activity.Kind)),
				i18n.RuntimeActivityStateLabel(c.state.Language.Get(), string(activity.State)),
				observationOutcomeLabelInLanguage(c.state.Language.Get(), activity.Outcome), activity.Name)
			if activity.RunID != "" {
				line += i18n.Format(c.state.Language.Get(), i18n.KeyTUIActivityAttempt, activity.Attempt, activity.RunID)
			}
			if activity.ParentRunID != "" {
				line += i18n.Format(c.state.Language.Get(), i18n.KeyTUIActivityParent, activity.ParentRunID)
			}
			if activity.Kind == ActivityAgent {
				line += activityAgentDescendantSummary(c.state.Language.Get(), activity, snapshot.Activities)
			}
			if progress != "" {
				line += i18n.Format(c.state.Language.Get(), i18n.KeyTUIActivityProgress, progress)
			}
			if activity.OccurrenceCount > 1 {
				line += i18n.Format(c.state.Language.Get(), i18n.KeyTUIActivityOccurrences, activity.OccurrenceCount, activity.FirstSequence, activity.LastSequence)
			}
			if len(actions) > 0 {
				line += "  [" + strings.Join(actions, ",") + "]"
			}
		}
		container.AddChild(tui.New(tui.WithText(line), tui.WithWidthPercent(100), tui.WithTruncate(true)))
	}
	return container
}

func activityAgentDescendantSummary(lang i18n.Language, parent Activity, activities []Activity) string {
	parentPath := strings.Trim(strings.TrimSpace(parent.AgentPath), "/")
	if parentPath == "" {
		parentPath = strings.Trim(strings.TrimSpace(parent.Actor.ID), "/")
	}
	if parentPath == "" {
		return ""
	}
	prefix := parentPath + "/"
	latest := make(map[string]Activity)
	for _, activity := range activities {
		if activity.Kind != ActivityAgent || activity.ID == parent.ID || !strings.HasPrefix(strings.TrimSpace(activity.AgentPath), prefix) {
			continue
		}
		current, exists := latest[activity.ID]
		if !exists || activityRunIsLater(activity, current) {
			latest[activity.ID] = activity
		}
	}
	if len(latest) == 0 {
		return ""
	}
	worstRank := 99
	worstState := i18n.RuntimeActivityStateLabel(lang, "unknown")
	for _, activity := range latest {
		rank := activitySortRank(activity)
		if rank < worstRank {
			worstRank = rank
			worstState = i18n.RuntimeActivityStateLabel(lang, string(activity.State))
			if activity.Attention.Kind == ActivityAttentionNeedsInput {
				worstState = i18n.RuntimeActivityStateLabel(lang, "needs_input")
			}
		}
	}
	return i18n.Format(lang, i18n.KeyActivityDescendants, len(latest), worstState)
}

func activityProgressTextInLanguage(lang i18n.Language, progress ActivityProgress) string {
	parts := make([]string, 0, 2)
	if progress.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", progress.Current, progress.Total))
	} else if progress.Current > 0 {
		parts = append(parts, i18n.Format(lang, i18n.KeyRuntimeActivityCurrent, progress.Current))
	}
	if message := strings.TrimSpace(progress.Message); message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

func (c *RootComponent) renderSkillsMenuAtHeight(menu *SkillsMenuState, availableHeight int) *tui.Element {
	return c.renderSkillsMenuAtSize(menu, c.termWidth, availableHeight)
}

func (c *RootComponent) renderSkillsMenuAtSize(menu *SkillsMenuState, width, availableHeight int) *tui.Element {
	layout := c.skillsMenuLayoutAtWidth(menu, width, availableHeight)
	return c.renderSkillsMenuWithLayout(menu, layout)
}

func (c *RootComponent) renderSkillsMenuWithLayout(menu *SkillsMenuState, layout skillsPanelLayout) *tui.Element {
	outer := tui.New(
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		tui.WithBackground(tui.NewStyle().Background(tui.Black)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithWidthPercent(100),
		tui.WithHeight(layout.PanelHeight),
	)
	return c.renderSkillsToggleView(outer, menu, layout)
}

func (c *RootComponent) skillsMenuLayout(menu *SkillsMenuState, availableHeight int) skillsPanelLayout {
	return c.skillsMenuLayoutAtWidth(menu, c.termWidth, availableHeight)
}

func (c *RootComponent) skillsMenuLayoutAtWidth(menu *SkillsMenuState, width, availableHeight int) skillsPanelLayout {
	request := skillsPanelLayoutRequest{
		TerminalWidth:   width,
		AvailableHeight: availableHeight,
	}
	if menu == nil {
		return calculateSkillsPanelLayout(request)
	}
	view := menu.Toggle
	request.HasSnapshot = view.HasSnapshot
	request.TotalRows = len(view.Filtered)
	request.Selected = view.Selected
	request.HasFilter = view.Query != ""
	notice := formatSkillsToggleNotice(c.state.Language.Get(), view.Notice)
	request.NoticeRows = len(wrapSkillsPanelNoticeLines(notice, request.TerminalWidth-skillsPanelHorizontalChrome))
	request.Refreshing = view.Refreshing
	if row := view.selectedRow(); row != nil {
		request.DetailRows = len(c.skillsToggleDetailLines(*row, view.Snapshot.Revision))
	}
	return calculateSkillsPanelLayout(request)
}

func (c *RootComponent) renderSkillsToggleView(outer *tui.Element, menu *SkillsMenuState, layout skillsPanelLayout) *tui.Element {
	if menu == nil {
		return outer
	}
	lang := c.state.Language.Get()
	view := menu.Toggle
	notice := formatSkillsToggleNotice(lang, view.Notice)
	if layout.ShowTitle {
		addSkillsPanelLine(outer, i18n.Text(lang, i18n.KeySkillsMenuTitle), tui.NewStyle().Foreground(tui.Cyan).Bold(), layout.InnerWidth)
	}
	if layout.ShowHelp {
		addSkillsPanelLine(outer, i18n.Text(lang, i18n.KeySkillsMenuHelp), tui.NewStyle().Dim(), layout.InnerWidth)
	}
	if layout.ShowFilter {
		addSkillsPanelLine(outer, i18n.Format(lang, i18n.KeySkillsMenuFilter, view.Query), tui.NewStyle().Dim(), layout.InnerWidth)
	}

	if layout.ShowPlaceholder {
		placeholder := i18n.Text(lang, i18n.KeySkillsMenuEmpty)
		if len(view.Snapshot.Skills) > 0 && len(view.Filtered) == 0 {
			placeholder = i18n.Format(lang, i18n.KeySkillsMenuNoMatches, view.Query)
			addSkillsPanelLine(outer, placeholder, tui.NewStyle(), layout.InnerWidth)
		} else {
			// This catalog-owned sentence contains no external metadata. Keep
			// its complete semantic text available to linear/accessibility
			// consumers while the visible row uses the same safe truncator.
			addSkillsPanelSemanticLine(outer, placeholder, tui.NewStyle(), layout.InnerWidth)
		}
	}

	if view.HasSnapshot {
		for visibleIndex := layout.Start; visibleIndex < layout.End; visibleIndex++ {
			rowIndex := view.Filtered[visibleIndex]
			if rowIndex < 0 || rowIndex >= len(view.Snapshot.Skills) {
				continue
			}
			row := view.Snapshot.Skills[rowIndex]
			prefix := "  "
			style := tui.NewStyle()
			if visibleIndex == view.Selected {
				prefix = "→ "
				style = style.Foreground(tui.Cyan).Bold()
			}
			checkbox := "[x]"
			if row.Visibility == skills.VisibilityOff {
				checkbox = "[ ]"
			}
			line := prefix + checkbox + " " + normalizeSkillsPanelLine(row.Name)
			if summary := normalizeSkillsPanelLine(skills.PresentedSummary(lang, row)); summary != "" {
				line += " — " + summary
			}
			addSkillsPanelLine(outer, line, style, layout.InnerWidth)
			if visibleIndex == view.Selected && layout.DetailRows > 0 {
				c.renderSkillsToggleDetailLines(outer, c.skillsToggleDetailLines(row, view.Snapshot.Revision), layout.DetailRows, layout.InnerWidth)
			}
		}
	}
	if layout.ShowPager {
		addSkillsPanelLine(outer, i18n.Format(lang, i18n.KeySkillsMenuShowing, layout.Start+1, layout.End, len(view.Filtered)), tui.NewStyle().Dim(), layout.InnerWidth)
	}
	for _, line := range visibleSkillsPanelNoticeLines(notice, layout.InnerWidth, layout.NoticeRows) {
		addSkillsPanelLine(outer, line, skillsMenuNoticeStyle(view.Notice), layout.InnerWidth)
	}
	if layout.ShowRefreshing {
		addSkillsPanelLine(outer, i18n.Text(lang, i18n.KeySkillsMenuRefreshing), tui.NewStyle().Foreground(tui.Cyan), layout.InnerWidth)
	}
	return outer
}

func (c *RootComponent) skillsToggleDetailLines(row skills.EffectiveSkill, revision skills.CatalogRevision) []string {
	lang := c.state.Language.Get()
	lines := []string{
		i18n.Format(lang, i18n.KeySkillsMenuDetailSummary, skills.PresentedSummary(lang, row)),
		i18n.Format(lang, i18n.KeySkillsMenuDetailSource, row.Source),
		i18n.Format(lang, i18n.KeySkillsMenuDetailPath, row.Locator),
		i18n.Format(lang, i18n.KeySkillsMenuDetailVisibilityScope, row.Visibility, row.VisibilitySource),
		i18n.Format(lang, i18n.KeySkillsMenuDetailIdentity, row.ID, revision),
	}
	if row.ShadowedBy != "" && row.Visibility != skills.VisibilityOff {
		lines = append(lines, i18n.Format(lang, i18n.KeySkillsMenuDetailShadowed, row.ShadowedBy))
	}
	if row.Mutable {
		lines = append(lines, i18n.Text(lang, i18n.KeySkillsMenuDetailMutable))
	} else {
		reason := skillsReadOnlyReasonInLanguage(lang, row.ReadOnlyReason)
		if reason == "" {
			reason = i18n.Text(lang, i18n.KeySkillsMenuReadOnlyUnspecified)
		}
		lines = append(lines, i18n.Format(lang, i18n.KeySkillsMenuDetailReadOnly, reason))
	}
	return lines
}

func (c *RootComponent) renderSkillsToggleDetailLines(outer *tui.Element, lines []string, limit, innerWidth int) {
	if limit > len(lines) {
		limit = len(lines)
	}
	for _, line := range lines[:max(limit, 0)] {
		addSkillsPanelSemanticLine(outer, "    "+line, tui.NewStyle().Dim(), innerWidth)
	}
}

func addSkillsPanelLine(outer *tui.Element, text string, style tui.Style, innerWidth int) {
	outer.AddChild(tui.New(
		tui.WithText(truncateSkillsPanelLine(text, innerWidth)),
		tui.WithTextStyle(style),
		tui.WithHeight(1),
		tui.WithWidthPercent(100),
		tui.WithWrap(false),
	))
}

func addSkillsPanelSemanticLine(outer *tui.Element, text string, style tui.Style, innerWidth int) {
	full := normalizeSkillsPanelLine(text)
	visible := truncateSkillsPanelLine(full, innerWidth)
	addSkillsPanelHiddenSemantics(outer, full, visible)
	addSkillsPanelLine(outer, visible, style, innerWidth)
}

func addSkillsPanelHiddenSemantics(outer *tui.Element, full, visible string) {
	if visible == full {
		return
	}
	// The visual line is bounded. Retain the complete localized semantics in
	// a non-layout node for linear/accessibility consumers and recovery audits.
	outer.AddChild(tui.New(
		tui.WithText(full),
		tui.WithWidth(0),
		tui.WithHeight(0),
		tui.WithWrap(false),
		tui.WithHidden(true),
	))
}

func skillsMenuNoticeStyle(notice SkillsToggleNotice) tui.Style {
	switch notice.Kind {
	case skillsToggleNoticeRefreshed:
		return tui.NewStyle().Foreground(tui.Green)
	case skillsToggleNoticeLoading, skillsToggleNoticeUpdating:
		return tui.NewStyle().Foreground(tui.Cyan)
	case skillsToggleNoticeReadOnly, skillsToggleNoticeStale, skillsToggleNoticeSessionOverride,
		skillsToggleNoticePersistenceFailed, skillsToggleNoticeRolledBack:
		return tui.NewStyle().Foreground(tui.Yellow)
	case skillsToggleNoticeNone:
		return tui.NewStyle()
	default:
		return tui.NewStyle().Foreground(tui.Red)
	}
}

func (c *RootComponent) openSkillsMenu(menu *SkillsMenuState) {
	if menu == nil {
		return
	}
	sessionID := menu.currentSessionID()
	if menu.Request.Backend == nil {
		menu.Toggle.cancelPending()
		menu.Toggle.Notice = SkillsToggleNotice{Kind: skillsToggleNoticeBackendUnavailable}
		c.state.SkillsMenu.Set(menu)
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		menu.Toggle.cancelPending()
		menu.Toggle.Notice = SkillsToggleNotice{Kind: skillsToggleNoticeSessionUnavailable}
		c.state.SkillsMenu.Set(menu)
		return
	}
	operation := menu.Toggle.beginLoad(sessionID)
	token := menu.token
	backend := menu.Request.Backend
	c.state.SkillsMenu.Set(menu)
	app := c.app
	go func() {
		snapshot, err := backend.Snapshot(sessionID)
		publish := func() {
			current := c.state.SkillsMenu.Get()
			if current == nil || current.token != token {
				return
			}
			if current.currentSessionID() != sessionID {
				c.openSkillsMenu(current.clone())
				return
			}
			next := current.clone()
			if next.Toggle.acceptSnapshot(operation, sessionID, snapshot, err) {
				c.state.SkillsMenu.Set(next)
			}
		}
		if app == nil {
			publish()
			return
		}
		app.QueueUpdateLossless(publish)
	}()
}

func (c *RootComponent) selectHighlightedSkill() {
	menu := c.state.SkillsMenu.Get()
	if menu == nil || !menu.Visible || menu.Toggle.Loading || (c.onSubmit == nil && c.trySubmit == nil) {
		return
	}
	currentSession := menu.currentSessionID()
	if strings.TrimSpace(currentSession) == "" || currentSession != menu.Toggle.SessionID {
		c.openSkillsMenu(menu.clone())
		return
	}
	row := menu.Toggle.selectedRow()
	if row == nil {
		return
	}

	// Submit the stable ID through the same input path as an explicitly typed
	// skill invocation. The REPL re-resolves the current catalog and enforces
	// visibility, shadowing, and invocation policy before any model request.
	c.state.SkillsMenu.Set(nil)
	c.submitInput("/" + string(row.ID) + " ")
}

func (c *RootComponent) toggleSelectedSkill() {
	menu := c.state.SkillsMenu.Get()
	if menu == nil || !menu.Visible || menu.Toggle.Loading {
		return
	}
	currentSession := menu.currentSessionID()
	if strings.TrimSpace(currentSession) == "" {
		next := menu.clone()
		next.Toggle.Notice = SkillsToggleNotice{Kind: skillsToggleNoticeSessionUnavailable}
		c.state.SkillsMenu.Set(next)
		return
	}
	if currentSession != menu.Toggle.SessionID {
		c.openSkillsMenu(menu.clone())
		return
	}
	if menu.Toggle.RefreshRequired {
		c.refreshSkillsToggle(menu.clone())
		return
	}
	row := menu.Toggle.selectedRow()
	if row == nil {
		return
	}
	if !row.Mutable {
		next := menu.clone()
		next.Toggle.rejectReadOnly(*row)
		c.state.SkillsMenu.Set(next)
		return
	}
	if menu.Request.Backend == nil {
		next := menu.clone()
		next.Toggle.Notice = SkillsToggleNotice{Kind: skillsToggleNoticeBackendUnavailable}
		c.state.SkillsMenu.Set(next)
		return
	}

	next := menu.clone()
	operation := next.Toggle.beginToggle(*row)
	token := next.token
	id := row.ID
	revision := next.Toggle.Snapshot.Revision
	backend := next.Request.Backend
	sessionID := currentSession
	c.state.SkillsMenu.Set(next)
	app := c.app
	go func() {
		result, err := backend.ToggleProjectVisibility(sessionID, id, revision)
		publish := func() {
			current := c.state.SkillsMenu.Get()
			if current == nil || current.token != token {
				return
			}
			if current.currentSessionID() != sessionID {
				c.openSkillsMenu(current.clone())
				return
			}
			updated := current.clone()
			if updated.Toggle.acceptToggle(operation, result, err) {
				c.state.SkillsMenu.Set(updated)
				if updated.Toggle.RefreshRequired {
					c.refreshSkillsToggle(updated.clone())
				}
			}
		}
		if app == nil {
			publish()
			return
		}
		app.QueueUpdateLossless(publish)
	}()
}

// refreshSkillsToggle is the read-only gate after a degraded transaction.
// It performs one authoritative Snapshot read. Space may retry this read after
// failure, but cannot reach ToggleProjectVisibility until it succeeds.
func (c *RootComponent) refreshSkillsToggle(menu *SkillsMenuState) {
	if menu == nil {
		return
	}
	sessionID := menu.currentSessionID()
	if menu.Request.Backend == nil {
		menu.Toggle.cancelPending()
		menu.Toggle.RefreshRequired = true
		menu.Toggle.Notice.Kind = skillsToggleNoticeBackendUnavailable
		c.state.SkillsMenu.Set(menu)
		return
	}
	if strings.TrimSpace(sessionID) == "" {
		menu.Toggle.cancelPending()
		menu.Toggle.RefreshRequired = true
		menu.Toggle.Notice.Kind = skillsToggleNoticeSessionUnavailable
		c.state.SkillsMenu.Set(menu)
		return
	}
	operation := menu.Toggle.beginRefresh()
	token := menu.token
	backend := menu.Request.Backend
	c.state.SkillsMenu.Set(menu)
	app := c.app
	go func() {
		snapshot, err := backend.Snapshot(sessionID)
		publish := func() {
			current := c.state.SkillsMenu.Get()
			if current == nil || current.token != token {
				return
			}
			if current.currentSessionID() != sessionID {
				c.openSkillsMenu(current.clone())
				return
			}
			next := current.clone()
			if next.Toggle.acceptRefresh(operation, sessionID, snapshot, err) {
				c.state.SkillsMenu.Set(next)
			}
		}
		if app == nil {
			publish()
			return
		}
		app.QueueUpdateLossless(publish)
	}()
}

func (c *RootComponent) renderSessionPicker(picker *SessionPickerState) *tui.Element {
	outer := tui.New(
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithWidthPercent(100),
	)
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeySessionPickerTitle)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))
	if picker.Query != "" {
		outer.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeySessionPickerQuery, picker.Query)),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}
	if len(picker.Entries) == 0 {
		outer.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeySessionPickerEmpty))))
		return outer
	}
	for i, entry := range picker.Entries {
		prefix := "  "
		style := tui.NewStyle()
		if i == picker.Selected {
			prefix = "→ "
			style = style.Foreground(tui.Cyan).Bold()
		}
		title := entry.Title
		if title == "" {
			title = entry.ID
		}
		line := prefix + title
		if entry.MessageCount > 0 {
			line += i18n.Format(c.state.Language.Get(), i18n.KeySessionPickerMessages, entry.MessageCount)
		}
		if entry.GitBranch != "" {
			line += "  [" + entry.GitBranch + "]"
		}
		outer.AddChild(tui.New(tui.WithText(line), tui.WithTextStyle(style)))
		if i == picker.Selected {
			if entry.PreviewText != "" {
				outer.AddChild(tui.New(
					tui.WithText("    "+entry.PreviewText),
					tui.WithTextStyle(tui.NewStyle().Dim()),
				))
			}
			for _, msg := range previewMessages(entry.Messages, 4) {
				text := strings.TrimSpace(msg.Text)
				if text == "" {
					continue
				}
				text = truncateRunes(strings.Join(strings.Fields(text), " "), 100, "…")
				outer.AddChild(tui.New(
					tui.WithText("    "+i18n.Format(c.state.Language.Get(), i18n.KeyRuntimeAssistantPreview, text)),
					tui.WithTextStyle(tui.NewStyle().Dim()),
				))
			}
		}
	}
	return outer
}

func previewMessages(msgs []Message, limit int) []Message {
	if len(msgs) <= limit {
		return msgs
	}
	return msgs[len(msgs)-limit:]
}

func (c *RootComponent) renderForkPicker(picker *ForkPickerState) *tui.Element {
	outer := tui.New(
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithWidthPercent(100),
	)
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyForkPickerTitle)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyForkPickerContextOnly)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	if len(picker.Entries) == 0 {
		outer.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyForkPickerEmpty))))
		return outer
	}
	const maxVisibleForkEntries = 7
	start, end := forkPickerVisibleRange(len(picker.Entries), picker.Selected, maxVisibleForkEntries)
	for i := start; i < end; i++ {
		entry := picker.Entries[i]
		prefix := "  "
		style := tui.NewStyle()
		if i == picker.Selected {
			prefix = "→ "
			style = style.Foreground(tui.Cyan).Bold()
		}
		userText := truncateRunes(strings.Join(strings.Fields(entry.UserText), " "), 120, "…")
		outer.AddChild(tui.New(
			tui.WithText(fmt.Sprintf("%s#%d  %s", prefix, i+1, userText)),
			tui.WithTextStyle(style),
		))
		if i == picker.Selected && strings.TrimSpace(entry.AssistantText) != "" {
			answer := truncateRunes(strings.Join(strings.Fields(entry.AssistantText), " "), 120, "…")
			outer.AddChild(tui.New(
				tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyForkPickerReply, answer)),
				tui.WithTextStyle(tui.NewStyle().Dim()),
			))
		}
	}
	if len(picker.Entries) > maxVisibleForkEntries {
		outer.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyForkPickerShowing, start+1, end, len(picker.Entries))),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}
	return outer
}

// renderModelPicker creates the model selection overlay.
// It supports three phases: provider selection (Phase 1), model selection (Phase 2),
// and connection flow (Phase 3) for unconnected providers.
func (c *RootComponent) renderModelPicker(mp *ModelPickerState) *tui.Element {
	outer := tui.New(
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithWidthPercent(100),
	)

	switch mp.Phase {
	case PickerPhaseProvider:
		return c.renderProviderPhase(outer, mp)
	case PickerPhaseModel:
		return c.renderModelPhase(outer, mp)
	case PickerPhaseReasoning:
		return c.renderReasoningPhase(outer, mp)
	case PickerPhaseConnect:
		return c.renderConnectPhase(outer, mp)
	case PickerPhaseEditLimits:
		return c.renderEditLimitsPhase(outer, mp)
	case PickerPhaseDeleteConfirm:
		return c.renderDeleteProviderPhase(outer, mp)
	default:
		return c.renderProviderPhase(outer, mp)
	}
}

// renderProviderPhase renders the first level: provider selection.
// Shows all providers with connection status indicators and setup hints.
func (c *RootComponent) renderProviderPhase(outer *tui.Element, mp *ModelPickerState) *tui.Element {
	lang := c.state.Language.Get()
	actions := i18n.Text(lang, i18n.KeyProviderPickerActionsDefault)
	if mp.ProviderSelected >= 0 && mp.ProviderSelected < len(mp.Providers) {
		selected := mp.Providers[mp.ProviderSelected]
		if selected.IsCreate {
			actions = i18n.Text(lang, i18n.KeyProviderPickerActionsCreate)
		} else if selected.UserDefined && selected.IsConnected {
			actions = i18n.Text(lang, i18n.KeyProviderPickerActionsConnectedCustom)
		} else if selected.UserDefined {
			actions = i18n.Text(lang, i18n.KeyProviderPickerActionsConfigureCustom)
		} else if selected.IsConnected {
			actions = i18n.Text(lang, i18n.KeyProviderPickerActionsConnected)
		} else if providerHasInteractiveConfig(selected) {
			actions = i18n.Text(lang, i18n.KeyProviderPickerActionsConfigure)
		}
	}
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(lang, i18n.KeyProviderPickerTitle, actions)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))

	if len(mp.Providers) == 0 {
		outer.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyProviderPickerEmpty))))
		return outer
	}

	const maxVisible = 12
	startIdx := 0
	if mp.ProviderSelected >= maxVisible {
		startIdx = mp.ProviderSelected - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(mp.Providers) {
		endIdx = len(mp.Providers)
	}

	for vi := startIdx; vi < endIdx; vi++ {
		prov := mp.Providers[vi]

		prefix := "  "
		style := tui.NewStyle()
		if vi == mp.ProviderSelected {
			prefix = "→ "
			style = style.Foreground(tui.Cyan).Bold()
		}

		statusIcon := "❌"
		switch prov.ConnectionState {
		case "connected":
			statusIcon = "✅"
		case "local_unverified":
			statusIcon = "◌"
		case "unknown":
			statusIcon = "?"
		}
		if prov.IsCreate {
			statusIcon = "+"
		}

		line := prefix + statusIcon + " " + prov.DisplayName
		if prov.IsActive {
			line += " ★"
		}

		meta := ""
		if prov.ModelCount > 0 && !prov.IsCreate {
			meta = i18n.Format(lang, i18n.KeyProviderPickerModelCount, prov.ModelCount)
		}
		if prov.ConnectionLabel != "" && !prov.IsCreate {
			meta += " — " + prov.ConnectionLabel
		} else if !prov.CanSelectModels && !prov.IsCreate {
			meta += i18n.Text(c.state.Language.Get(), i18n.KeyProviderPickerNotConnected)
		}

		row := tui.New(tui.WithDisplay(tui.DisplayFlex), tui.WithDirection(tui.Row), tui.WithWidthPercent(100))
		row.AddChild(tui.New(
			tui.WithText(line),
			tui.WithTextStyle(style),
		))
		if meta != "" {
			metaStyle := tui.NewStyle().Dim()
			if !prov.CanSelectModels || prov.ConnectionState == "local_unverified" {
				metaStyle = tui.NewStyle().Foreground(tui.Yellow).Dim()
			}
			row.AddChild(tui.New(
				tui.WithText(meta),
				tui.WithTextStyle(metaStyle),
			))
		}
		outer.AddChild(row)
	}

	if len(mp.Providers) > maxVisible {
		outer.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyProviderPickerCount, mp.ProviderSelected+1, len(mp.Providers))),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}

	return outer
}

func providerHasInteractiveConfig(entry ProviderPickerEntry) bool {
	for _, method := range entry.AuthMethods {
		switch method {
		case "api_key", "oauth_pkce", "device_code":
			return true
		}
	}
	return false
}

// renderModelPhase renders the second level: model selection for the chosen provider.
func (c *RootComponent) renderModelPhase(outer *tui.Element, mp *ModelPickerState) *tui.Element {
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyModelPickerTitle)),
		tui.WithTextStyle(tui.NewStyle().Bold()),
	))
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyModelPickerProviderHint, mp.SelectedProvider)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	if mp.Query != "" {
		outer.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyModelPickerFilter, mp.Query)),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}
	if len(mp.Filtered) == 0 {
		outer.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyModelPickerEmpty))))
		return outer
	}

	const maxVisible = 12
	startIdx := 0
	if mp.Selected >= maxVisible {
		startIdx = mp.Selected - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(mp.Filtered) {
		endIdx = len(mp.Filtered)
	}

	for vi := startIdx; vi < endIdx; vi++ {
		idx := mp.Filtered[vi]
		entry := mp.Entries[idx]

		prefix := "  "
		style := tui.NewStyle()
		if vi == mp.Selected {
			prefix = "→ "
			style = style.Foreground(tui.Cyan).Bold()
		}

		line := fmt.Sprintf("%s%d. %s", prefix, vi+1, entry.ModelID)
		if entry.IsDefault {
			line += i18n.Text(c.state.Language.Get(), i18n.KeyModelPickerDefault)
		}

		meta := ""
		if entry.ContextK != "" {
			meta += " [" + entry.ContextK + "]"
		}
		if entry.CanSeeImages {
			meta += " [" + i18n.Text(c.state.Language.Get(), i18n.KeyModelTagVision) + "]"
		} else {
			meta += " [" + i18n.Text(c.state.Language.Get(), i18n.KeyModelTagText) + "]"
		}
		if len(entry.ReasoningEfforts) > 0 {
			meta += " [" + i18n.Text(c.state.Language.Get(), i18n.KeyModelTagEffort) + "]"
		} else if entry.CanReason {
			meta += " [" + i18n.Text(c.state.Language.Get(), i18n.KeyModelTagThinking) + "]"
		}
		if entry.CostIn > 0 || entry.CostOut > 0 {
			meta += fmt.Sprintf(" [%s]", fmtCostPair(entry.CostIn, entry.CostOut, entry.CostCurrency))
		}

		row := tui.New(tui.WithDisplay(tui.DisplayFlex), tui.WithDirection(tui.Row), tui.WithWidthPercent(100))
		row.AddChild(tui.New(
			tui.WithText(line),
			tui.WithTextStyle(style),
			tui.WithWidth(34),
		))
		descStyle := tui.NewStyle().Dim()
		if vi == mp.Selected {
			descStyle = tui.NewStyle().Foreground(tui.Cyan).Bold()
		}
		row.AddChild(tui.New(
			tui.WithText(ModelPickerDescriptionInLanguage(c.state.Language.Get(), entry)),
			tui.WithTextStyle(descStyle),
		))
		outer.AddChild(row)

		if vi == mp.Selected && (entry.DisplayName != "" || meta != "") {
			detail := strings.TrimSpace(entry.DisplayName + " " + meta)
			outer.AddChild(tui.New(
				tui.WithText("    "+detail),
				tui.WithTextStyle(tui.NewStyle().Dim()),
			))
		}
	}

	if len(mp.Filtered) > maxVisible {
		outer.AddChild(tui.New(
			tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyModelPickerCount, mp.Selected+1, len(mp.Filtered))),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyModelPickerEffortHint)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))

	return outer
}

func (c *RootComponent) renderReasoningPhase(outer *tui.Element, mp *ModelPickerState) *tui.Element {
	model := mp.ReasoningModel
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyReasoningPickerTitle, model.ModelID)),
		tui.WithTextStyle(tui.NewStyle().Bold()),
	))
	if model.DisplayName != "" {
		outer.AddChild(tui.New(
			tui.WithText("  "+model.DisplayName),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}
	if len(model.ReasoningEfforts) == 0 {
		outer.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyReasoningPickerEmpty))))
		return outer
	}
	defaultEffort := DefaultReasoningEffort(model.ReasoningEfforts)
	for i, effort := range model.ReasoningEfforts {
		prefix := "  "
		style := tui.NewStyle()
		if i == mp.ReasoningSelected {
			prefix = "→ "
			style = style.Foreground(tui.Cyan).Bold()
		}
		info := ReasoningEffortInfoInLanguage(c.state.Language.Get(), effort)
		label := fmt.Sprintf("%s%d. %s", prefix, i+1, info.Label)
		if effort == defaultEffort {
			label += i18n.Text(c.state.Language.Get(), i18n.KeyModelPickerDefault)
		}
		row := tui.New(tui.WithDisplay(tui.DisplayFlex), tui.WithDirection(tui.Row), tui.WithWidthPercent(100))
		row.AddChild(tui.New(
			tui.WithText(label),
			tui.WithTextStyle(style),
			tui.WithWidth(28),
		))
		descStyle := tui.NewStyle().Dim()
		if i == mp.ReasoningSelected {
			descStyle = tui.NewStyle().Foreground(tui.Cyan).Bold()
		}
		row.AddChild(tui.New(
			tui.WithText(info.Description),
			tui.WithTextStyle(descStyle),
		))
		outer.AddChild(row)
	}
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyReasoningPickerConfirm)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	return outer
}

// renderConnectPhase renders the connection or reconnection flow for a provider.
// Shows available auth methods and custom endpoint inputs.
func (c *RootComponent) renderConnectPhase(outer *tui.Element, mp *ModelPickerState) *tui.Element {
	providerDisplay := mp.ConnectProvider
	for _, p := range mp.Providers {
		if p.Name == mp.ConnectProvider {
			providerDisplay = p.DisplayName
			break
		}
	}

	titleKey := i18n.KeyProviderConnectTitle
	if mp.IsReconnect {
		titleKey = i18n.KeyProviderReconnectTitle
	}
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), titleKey, providerDisplay)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))

	// Separator
	outer.AddChild(tui.New(
		tui.WithText("────────────────────────────────────────"),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))

	type authOption struct {
		label  string
		method string
		desc   string
	}
	methods := mp.ConnectAuthMethods
	if len(methods) == 0 {
		for _, p := range mp.Providers {
			if p.Name == mp.ConnectProvider && p.EnvKey != "" {
				methods = []string{"api_key"}
				break
			}
		}
	}
	options := make([]authOption, 0, len(methods))
	for _, method := range methods {
		switch method {
		case "api_key":
			envHint := ""
			for _, p := range mp.Providers {
				if p.Name == mp.ConnectProvider && p.EnvKey != "" {
					envHint = i18n.Format(c.state.Language.Get(), i18n.KeyProviderAuthEnvHint, p.EnvKey)
					break
				}
			}
			options = append(options, authOption{label: i18n.Format(c.state.Language.Get(), i18n.KeyProviderAuthAPIKeyLabel, envHint), method: method, desc: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthAPIKeyDescription)})
		case "oauth_pkce":
			options = append(options, authOption{label: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthOAuthLabel), method: method, desc: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthOAuthDescription)})
		case "device_code":
			options = append(options, authOption{label: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthDeviceLabel), method: method, desc: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthDeviceDescription)})
		case "aws_credentials":
			options = append(options, authOption{label: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthAWSLabel), method: method, desc: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthAWSDescription)})
		case "gcp_adc":
			options = append(options, authOption{label: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthGCPLabel), method: method, desc: i18n.Text(c.state.Language.Get(), i18n.KeyProviderAuthGCPDescription)})
		}
	}

	if len(options) == 0 {
		message := i18n.Text(c.state.Language.Get(), i18n.KeyProviderConnectExternalHint)
		if mp.ConnectHint != "" {
			message = mp.ConnectHint
		}
		outer.AddChild(tui.New(
			tui.WithText("  "+message),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Yellow)),
		))
		return outer
	}

	if len(options) > 1 {
		// Multiple auth methods — show selection list
		outer.AddChild(tui.New(
			tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyProviderConnectSelectMethod)),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
		outer.AddChild(tui.New(tui.WithHeight(1))) // spacer

		for i, opt := range options {
			prefix := "  "
			style := tui.NewStyle()
			if i == mp.ConnectSelectedAuth {
				prefix = "→ "
				style = style.Foreground(tui.Cyan).Bold()
			}

			row := tui.New(tui.WithDisplay(tui.DisplayFlex), tui.WithDirection(tui.Row), tui.WithWidthPercent(100))
			row.AddChild(tui.New(
				tui.WithText(prefix+opt.label),
				tui.WithTextStyle(style),
			))
			row.AddChild(tui.New(
				tui.WithText("  "+opt.desc),
				tui.WithTextStyle(tui.NewStyle().Dim()),
			))
			outer.AddChild(row)
		}

	}

	if mp.ConnectSelectedAuth < len(options) && options[mp.ConnectSelectedAuth].method == "api_key" {
		outer.AddChild(tui.New(tui.WithHeight(1)))
		outer.AddChild(tui.New(
			tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyProviderConnectInputHint)),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Yellow)),
		))

		if mp.ConnectDynamicModels {
			stylePrefix := "  "
			styleStyle := tui.NewStyle().Dim()
			if mp.ConnectInputField == ConnectInputAPIStyle {
				stylePrefix = "→ "
				styleStyle = tui.NewStyle().Foreground(tui.Cyan).Bold()
			}
			outer.AddChild(tui.New(
				tui.WithText(stylePrefix+i18n.Format(c.state.Language.Get(), i18n.KeyProviderConnectAPIStyle, mp.selectedAPIStyle().DisplayName())),
				tui.WithTextStyle(styleStyle),
			))
		}

		if mp.ConnectUserDefined {
			name := mp.ConnectProviderNameInput
			if name == "" {
				name = i18n.Text(c.state.Language.Get(), i18n.KeyProviderConnectNameDefault)
			}
			namePrefix := "  "
			nameStyle := tui.NewStyle().Dim()
			if mp.ConnectInputField == ConnectInputProviderName {
				namePrefix = "→ "
				nameStyle = tui.NewStyle().Foreground(tui.Cyan).Bold()
			}
			outer.AddChild(tui.New(
				tui.WithText(namePrefix+i18n.Format(c.state.Language.Get(), i18n.KeyProviderConnectName, name)),
				tui.WithTextStyle(nameStyle),
				tui.WithWidthPercent(100),
				tui.WithTruncate(true),
			))
		}

		baseURL := mp.ConnectBaseURLInput
		if baseURL == "" {
			if defaultBaseURL := mp.selectedDefaultBaseURL(); defaultBaseURL != "" {
				baseURL = defaultBaseURL
			} else if mp.ConnectUserDefined {
				baseURL = i18n.Text(c.state.Language.Get(), i18n.KeyProviderConnectRequired)
			} else {
				baseURL = i18n.Text(c.state.Language.Get(), i18n.KeyProviderConnectDefaultEndpoint)
			}
		}
		basePrefix := "  "
		baseStyle := tui.NewStyle().Dim()
		if mp.ConnectInputField == ConnectInputBaseURL {
			basePrefix = "→ "
			baseStyle = tui.NewStyle().Foreground(tui.Cyan).Bold()
		}
		outer.AddChild(tui.New(
			tui.WithText(basePrefix+i18n.Format(c.state.Language.Get(), i18n.KeyProviderConnectBaseURL, baseURL)),
			tui.WithTextStyle(baseStyle),
			tui.WithWidthPercent(100),
			tui.WithTruncate(true),
		))

		masked := mp.maskedAPIKey()
		if masked == "" {
			masked = i18n.Text(c.state.Language.Get(), i18n.KeyProviderConnectRequired)
		}
		keyPrefix := "  "
		keyStyle := tui.NewStyle().Dim()
		if mp.ConnectInputField == ConnectInputAPIKey {
			keyPrefix = "→ "
			keyStyle = tui.NewStyle().Foreground(tui.Cyan).Bold()
		}
		outer.AddChild(tui.New(
			tui.WithText(keyPrefix+i18n.Format(c.state.Language.Get(), i18n.KeyProviderConnectAPIKey, masked)),
			tui.WithTextStyle(keyStyle),
			tui.WithWidthPercent(100),
			tui.WithTruncate(true),
		))
	}

	// Show status/error messages
	if mp.ConnectStatus != "" {
		outer.AddChild(tui.New(tui.WithHeight(1))) // spacer
		outer.AddChild(tui.New(
			tui.WithText("  "+mp.ConnectStatus),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Yellow)),
		))
	}
	if mp.ConnectError != "" {
		outer.AddChild(tui.New(tui.WithHeight(1))) // spacer
		outer.AddChild(tui.New(
			tui.WithText("  ✗ "+mp.ConnectError),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red)),
		))
	}

	return outer
}

func (c *RootComponent) renderDeleteProviderPhase(outer *tui.Element, mp *ModelPickerState) *tui.Element {
	displayName := mp.DeleteProvider
	for _, entry := range mp.Providers {
		if entry.Name == mp.DeleteProvider {
			displayName = entry.DisplayName
			break
		}
	}
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyProviderDeleteTitle, displayName)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red).Bold()),
	))
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyProviderDeleteWarning)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Yellow)),
	))
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyProviderDeleteConfirm)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	if mp.DeleteError != "" {
		outer.AddChild(tui.New(
			tui.WithText("  ✗ "+mp.DeleteError),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red)),
		))
	}
	return outer
}

func (c *RootComponent) confirmProviderDeletion(p *ModelPickerState) {
	if p == nil || p.DeleteProvider == "" || p.OnDelete == nil {
		return
	}
	if err := p.OnDelete(p.DeleteProvider); err != nil {
		p.DeleteError = err.Error()
		c.state.ModelPicker.Set(p)
		return
	}
	filtered := make([]ProviderPickerEntry, 0, len(p.Providers)-1)
	for _, entry := range p.Providers {
		if entry.Name != p.DeleteProvider {
			filtered = append(filtered, entry)
		}
	}
	p.Providers = filtered
	p.Phase = PickerPhaseProvider
	p.DeleteProvider = ""
	p.DeleteError = ""
	p.clampProvider()
	c.state.ModelPicker.Set(p)
	if c.app != nil {
		c.app.RequestFullRedraw()
	}
}

func (c *RootComponent) renderEditLimitsPhase(outer *tui.Element, mp *ModelPickerState) *tui.Element {
	model := mp.LimitEditModel
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyModelLimitEditTitle, model.Provider, model.ModelID)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))
	outer.AddChild(tui.New(
		tui.WithText("────────────────────────────────────────"),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	current := model.ContextK
	if current == "" {
		current = i18n.Text(c.state.Language.Get(), i18n.KeyModelLimitEditUnknown)
	}
	if model.ContextOverridden {
		current = i18n.Format(c.state.Language.Get(), i18n.KeyModelLimitEditOverridden, current)
	}
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyModelLimitEditCurrent, current)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	input := mp.LimitEditInput
	if input == "" {
		input = "_"
	}
	outer.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyModelLimitEditInput, input)),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))
	outer.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyModelLimitEditHint)),
		tui.WithTextStyle(tui.NewStyle().Dim()),
	))
	if mp.LimitEditError != "" {
		outer.AddChild(tui.New(
			tui.WithText("  ✗ "+mp.LimitEditError),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Red)),
		))
	}
	return outer
}

// renderStatusBar creates the cost/context status line.
//
// Layout: [Mode badge] [Context meter] [provider dot] [session/feedback]
//
// The bar stays on one row. Complete low-priority segments are hidden as the
// terminal narrows; high-priority mode and context segments remain stable.
func (c *RootComponent) renderStatusBar(termWidth int) *tui.Element {
	const statusSegmentGap = 2
	if termWidth < 1 {
		termWidth = 1
	}
	if c.transcriptSelectionHintVisible.Get() {
		key := nativeSelectionHintKey(os.Getenv("TERM_PROGRAM"), os.Getenv("LC_TERMINAL"))
		return tui.New(
			tui.WithText(i18n.Text(c.state.Language.Get(), key)),
			tui.WithTextStyle(tui.NewStyle().Foreground(tui.Yellow)),
			tui.WithWidthPercent(100),
			tui.WithHeight(1),
			tui.WithTruncate(true),
		)
	}

	usage := c.state.ActiveSessionUsage()
	cumCost := usage.CumulativeCost
	costKnown := c.state.SessionCostKnown.Get()
	inTok := usage.InputTokens
	outTok := usage.OutputTokens
	cacheReadTok := usage.CacheReadTokens
	webSearchRequests := usage.WebSearchRequests
	usedTok := usage.UsedTokens
	maxTok := usage.MaxTokens
	provStatus := c.state.ProvStatus.Get()
	mode := c.state.Mode.Get()
	goalState := c.state.Goal.Get()

	type statusSegment struct {
		text       string
		style      tui.Style
		marginLeft int
		priority   int
	}
	var segments []statusSegment

	// --- Segment 1: Mode badge (fixed, high priority) ---
	modeColor := tui.Cyan
	switch mode {
	case ModeAutoEdit:
		modeColor = tui.Green
	case ModeAskEdit:
		modeColor = tui.Cyan
	case ModePlanEdit:
		modeColor = tui.Yellow
	}
	segments = append(segments, statusSegment{
		text:  mode.Badge() + " " + i18n.Format(c.state.Language.Get(), i18n.KeyModeLabel, localizedModeName(c.state.Language.Get(), mode)),
		style: tui.NewStyle().Foreground(modeColor).Bold(),
	})

	// --- Segment 2: Context usage meter (fixed, high priority) ---
	if maxTok > 0 && c.state.ContextMeasurement.Get() != presentation.ContextMeasurementUnknown {
		pct := float64(usedTok) / float64(maxTok) * 100
		if pct < 0 {
			pct = 0
		} else if pct > 100 {
			pct = 100
		}
		contextKey := i18n.KeyUsageContext
		compactContextKey := i18n.KeyUsageContextCompact
		switch c.state.ContextMeasurement.Get() {
		case presentation.ContextMeasurementLocalEstimate:
			contextKey = i18n.KeyUsageContextEstimated
			compactContextKey = i18n.KeyUsageContextEstimatedCompact
		case presentation.ContextMeasurementLocalLowerBound:
			contextKey = i18n.KeyUsageContextLowerBound
			compactContextKey = i18n.KeyUsageContextLowerBoundCompact
		}
		contextText := i18n.Format(c.state.Language.Get(), contextKey,
			"["+contextMeter(pct, 10)+"]", int(math.Round(pct)), fmtK(usedTok), fmtK(maxTok))
		if termWidth < 220 {
			contextText = i18n.Format(c.state.Language.Get(), compactContextKey,
				"["+contextMeter(pct, 10)+"]", int(math.Round(pct)))
		}
		segments = append(segments, statusSegment{
			text:       contextText,
			style:      contextUsageStyle(pct),
			marginLeft: statusSegmentGap,
		})
	}

	// --- Segment 3: Provider connection status ---
	if provStatus == StatusConnected {
		segments = append(segments, statusSegment{
			text:       "●",
			style:      tui.NewStyle().Foreground(tui.Green),
			marginLeft: statusSegmentGap,
			priority:   1,
		})
	}

	// --- Segment 4: Session and transient status information ---
	// Successful provider state is already shown as a compact green dot above.
	if provStatus != StatusUnknown && provStatus != StatusConnected {
		segments = append(segments, statusSegment{
			text: provStatus.Badge() + " " + i18n.RuntimeProviderStatusLabel(c.state.Language.Get(), provStatus.String()), style: tui.NewStyle().Dim(),
			marginLeft: statusSegmentGap,
		})
	}
	if goalText := formatGoalStatus(goalState, c.state.Language.Get()); goalText != "" {
		segments = append(segments, statusSegment{
			text:       goalText,
			style:      tui.NewStyle().Dim(),
			marginLeft: statusSegmentGap,
			priority:   2,
		})
	}
	if queued := c.state.QueuedInputCount.Get(); queued > 0 {
		segments = append(segments, statusSegment{
			text:       i18n.Format(c.state.Language.Get(), i18n.KeyTUIInputQueuedStatus, queued),
			style:      tui.NewStyle().Foreground(tui.Yellow),
			marginLeft: statusSegmentGap,
			priority:   1,
		})
	}
	var midParts []string

	usageKnown := c.state.SessionUsageKnown.Get() || inTok > 0 || outTok > 0 || cacheReadTok > 0 || cumCost > 0 || usedTok > 0 || maxTok > 0
	if usageKnown {
		sessionSummary := formatSessionUsageSummary(usage, costKnown, c.state.Language.Get(), c.state.ModelCostCurrency.Get())
		if termWidth < 220 {
			sessionSummary = formatSessionUsageCompactSummary(usage, costKnown, c.state.Language.Get(), c.state.ModelCostCurrency.Get())
		}
		if sessionSummary != "" {
			midParts = append(midParts, sessionSummary)
		}
	}
	if webSearchRequests > 0 {
		midParts = append(midParts, i18n.Format(c.state.Language.Get(), i18n.KeyWebSearchCount, webSearchRequests))
	}
	if c.state.TranscriptShowAll.Get() {
		midParts = append(midParts, i18n.Text(c.state.Language.Get(), i18n.KeyShowAllEvidence))
	}

	// Copy feedback toast (transient, shown for ~2 seconds)
	if fb := c.copyFeedback.Get(); fb != "" {
		midParts = append(midParts, fb)
	}

	for _, part := range midParts {
		segments = append(segments, statusSegment{
			text:       part,
			style:      tui.NewStyle().Dim(),
			marginLeft: statusSegmentGap,
			priority:   2,
		})
	}

	visible := make([]bool, len(segments))
	usedWidth := 0
	for priority := 0; priority <= 2; priority++ {
		for index, segment := range segments {
			if segment.priority != priority {
				continue
			}
			segmentWidth := terminalCellWidth(segment.text)
			marginLeft := 0
			if usedWidth > 0 {
				marginLeft = segment.marginLeft
			}
			if segmentWidth > termWidth || usedWidth+marginLeft+segmentWidth > termWidth {
				continue
			}
			visible[index] = true
			usedWidth += marginLeft + segmentWidth
		}
	}
	children := make([]*tui.Element, 0, len(segments))
	for index, segment := range segments {
		if !visible[index] {
			continue
		}
		marginLeft := segment.marginLeft
		if len(children) == 0 {
			marginLeft = 0
		}
		child := tui.New(
			tui.WithText(segment.text),
			tui.WithTextStyle(segment.style),
			tui.WithWidth(terminalCellWidth(segment.text)),
			tui.WithFlexShrink(0),
			tui.WithMarginTRBL(0, 0, 0, marginLeft),
		)
		children = append(children, child)
	}

	bar := tui.New(
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Row),
		tui.WithWidthPercent(100),
		tui.WithHeight(1),
	)
	bar.AddChild(children...)

	return bar
}

const goalStatusMaxCells = 40

func formatGoalStatus(goal *GoalViewState, lang i18n.Language) string {
	if goal == nil {
		return ""
	}
	prefix := i18n.Text(lang, i18n.KeyGoalPrefix)
	if goal.Status == "paused" {
		prefix = i18n.Text(lang, i18n.KeyGoalPausedPrefix)
	}
	objective := sanitizeGoalDisplayText(goal.Objective)
	text := prefix + objective
	if met, total := goalAcceptanceProgress(goal); total > 0 {
		text += i18n.Format(lang, i18n.KeyTUIGoalStatusProgress, met, total)
	}
	return truncateTerminalCells(text, goalStatusMaxCells)
}

func sanitizeGoalDisplayText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func goalAcceptanceProgress(current *GoalViewState) (met, total int) {
	if current == nil {
		return 0, 0
	}
	for _, criterion := range current.Criteria {
		if strings.TrimSpace(criterion.Text) == "" {
			continue
		}
		total++
		if criterion.Status == "met" {
			met++
		}
	}
	return met, total
}

func localizedModeName(lang i18n.Language, mode InteractionMode) string {
	switch mode {
	case ModeAutoEdit:
		return i18n.Text(lang, i18n.KeyModeAuto)
	case ModeAskEdit:
		return i18n.Text(lang, i18n.KeyModeAsk)
	case ModePlanEdit:
		return i18n.Text(lang, i18n.KeyModePlan)
	default:
		return mode.String()
	}
}

func truncateTerminalCells(text string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	if terminalCellWidth(text) <= maxCells {
		return text
	}
	const suffix = "…"
	remaining := maxCells - terminalCellWidth(suffix)
	if remaining <= 0 {
		return suffix
	}
	var truncated strings.Builder
	used := 0
	for _, r := range text {
		width := tui.RuneWidth(r)
		if used+width > remaining {
			break
		}
		truncated.WriteRune(r)
		used += width
	}
	truncated.WriteString(suffix)
	return truncated.String()
}

func contextMeter(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}

	const unitsPerCell = 8
	partialBlocks := [...]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	totalUnits := width * unitsPerCell
	filledUnits := int(pct*float64(totalUnits)/100 + 0.5)
	if pct > 0 && filledUnits == 0 {
		filledUnits = 1
	}
	// Reserve a visible empty sliver until the context is actually full.
	if pct < 100 && filledUnits >= totalUnits {
		filledUnits = totalUnits - 1
	}

	fullCells := filledUnits / unitsPerCell
	partialUnits := filledUnits % unitsPerCell
	occupiedCells := fullCells

	var meter strings.Builder
	meter.Grow(width * 3)
	meter.WriteString(strings.Repeat("█", fullCells))
	if partialUnits > 0 {
		meter.WriteString(partialBlocks[partialUnits])
		occupiedCells++
	}
	meter.WriteString(strings.Repeat("░", width-occupiedCells))
	return meter.String()
}

func contextUsageStyle(pct float64) tui.Style {
	switch {
	case pct >= 80:
		return tui.NewStyle().Foreground(tui.Red)
	case pct >= 50:
		return tui.NewStyle().Foreground(tui.Yellow)
	default:
		return tui.NewStyle().Dim()
	}
}

func formatSessionUsageSummary(usage SessionUsage, costKnown bool, lang i18n.Language, currency ...string) string {
	projection, ok := sessionUsageProjection(usage, costKnown, currency...)
	if !ok {
		return ""
	}
	return ui.FormatSessionUsage(lang, projection)
}

func sessionUsageProjection(usage SessionUsage, costKnown bool, currency ...string) (presentation.SessionUsageProjection, bool) {
	if usage.InputTokens <= 0 && usage.OutputTokens <= 0 && usage.CumulativeCost <= 0 {
		return presentation.SessionUsageProjection{}, false
	}
	displayInput, displayCache := usage.InputTokens, usage.CacheReadTokens
	if usage.HasCompacted && usage.CompactionBaselineKnown {
		displayInput = max(usage.InputTokens-usage.InputTokensAtCompact, 0)
		displayCache = max(usage.CacheReadTokens-usage.CacheReadAtCompact, 0)
	}
	cacheKnown := displayInput > 0 && displayCache >= 0 && displayCache <= displayInput
	cachePercent := 0
	if cacheKnown {
		cachePercent = sessionCachePercent(displayInput, displayCache)
	}
	costCurrency := "USD"
	if len(currency) > 0 && strings.TrimSpace(currency[0]) != "" {
		costCurrency = currency[0]
	}
	return presentation.SessionUsageProjection{
		Scope: presentation.UsageScopeCumulativeSession, Known: true, HasCompacted: usage.HasCompacted,
		BaselineKnown: usage.CompactionBaselineKnown,
		InputTokens:   displayInput, TotalInputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: displayCache, TotalCacheRead: usage.CacheReadTokens,
		CacheHitPercent: cachePercent, CacheHitKnown: cacheKnown,
		CostUSD: usage.CumulativeCost, CostCurrency: costCurrency, CostKnown: costKnown,
	}, true
}

func formatSessionUsageCompactSummary(usage SessionUsage, costKnown bool, lang i18n.Language, currency ...string) string {
	projection, ok := sessionUsageProjection(usage, costKnown, currency...)
	if !ok {
		return ""
	}
	return ui.FormatSessionUsageNarrow(lang, projection)
}

func sessionCachePercent(inputTok, cacheReadTok int) int {
	if inputTok <= 0 || cacheReadTok <= 0 || cacheReadTok > inputTok {
		return 0
	}
	if cacheReadTok == inputTok {
		return 100
	}
	cachePct := int(math.Round(float64(cacheReadTok) / float64(inputTok) * 100))
	if cachePct >= 100 {
		return 99
	}
	return cachePct
}

func slashCommandDisplayLabel(item SlashCommandEntry) string {
	if item.DisplayLabel != "" {
		return item.DisplayLabel
	}
	label := "/" + item.Name
	if len(item.Aliases) > 0 {
		label += "  (" + strings.Join(item.Aliases, ", ") + ")"
	}
	return label
}

func slashCommandColumnWidth(items []SlashCommandEntry) int {
	width := 0
	for _, item := range items {
		if itemWidth := terminalCellWidth(slashCommandDisplayLabel(item)); itemWidth > width {
			width = itemWidth
		}
	}
	return width
}

func terminalCellWidth(s string) int {
	width := 0
	for _, r := range s {
		width += tui.RuneWidth(r)
	}
	return width
}

func padRightCells(s string, width int) string {
	pad := width - terminalCellWidth(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

func (c *RootComponent) renderSlashSuggestions(state *slashSuggestionsState) *tui.Element {
	outer := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithBorder(tui.BorderRounded),
		tui.WithBorderStyle(tui.NewStyle().Foreground(tui.Cyan)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithWidthPercent(100),
	)
	header := i18n.Text(c.state.Language.Get(), i18n.KeySlashCommandsTitle)
	if isLanguageSubmenu(state) {
		header = i18n.Text(c.state.Language.Get(), i18n.KeyLanguageMenuTitle)
	}
	outer.AddChild(tui.New(
		tui.WithText(header),
		tui.WithTextStyle(tui.NewStyle().Foreground(tui.Cyan).Bold()),
	))

	start, visible := slashSuggestionsWindow(state)
	selected := state.Selected
	commandWidth := slashCommandColumnWidth(visible)

	for i, item := range visible {
		actualIdx := start + i
		marker := "  "
		style := tui.NewStyle()
		if actualIdx == selected {
			marker = "→ "
			style = style.Foreground(tui.Cyan).Bold()
		}

		label := padRightCells(slashCommandDisplayLabel(item), commandWidth)
		row := tui.New(
			tui.WithDisplay(tui.DisplayFlex),
			tui.WithDirection(tui.Row),
			tui.WithWidthPercent(100),
		)
		row.AddChild(tui.New(
			tui.WithText(marker),
			tui.WithTextStyle(style),
			tui.WithWidth(2),
		))
		row.AddChild(tui.New(
			tui.WithText(label),
			tui.WithTextStyle(style),
			tui.WithWidth(commandWidth),
		))
		if item.Description != "" || item.DescriptionKey != "" {
			row.AddChild(tui.New(
				tui.WithText("  "),
				tui.WithWidth(2),
			))
			row.AddChild(tui.New(
				tui.WithText(localizedSlashCommandDescription(c.state.Language.Get(), item)),
				tui.WithTextStyle(tui.NewStyle().Dim()),
				tui.WithFlexGrow(1),
			))
		}
		outer.AddChild(row)
	}

	if len(state.Items) > maxVisibleSlashSuggestions {
		outer.AddChild(tui.New(
			tui.WithText(fmt.Sprintf("  (%d/%d)", state.Selected+1, len(state.Items))),
			tui.WithTextStyle(tui.NewStyle().Dim()),
		))
	}

	return outer
}

func isLanguageSubmenu(state *slashSuggestionsState) bool {
	return state != nil && len(state.Items) > 0 && strings.HasPrefix(state.Items[0].Input, "/language ")
}

func permissionInputPreview(input map[string]any, maxRunes int) string {
	if len(input) == 0 {
		return ""
	}
	if maxRunes < 20 {
		maxRunes = 20
	}

	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		val := strings.Join(strings.Fields(fmt.Sprintf("%v", input[k])), " ")
		val = truncateRunes(val, 60, "...")
		parts = append(parts, fmt.Sprintf("%s=%s", k, val))
	}
	return truncateRunes(strings.Join(parts, " · "), maxRunes, "...")
}

func permissionActionElement(choice int, hotkey, label string, color tui.Color, selectedChoice int) *tui.Element {
	text := fmt.Sprintf(" %s %s ", hotkey, label)
	style := tui.NewStyle().Foreground(color)
	if selectedChoice == choice {
		text = fmt.Sprintf("> %s %s <", hotkey, label)
		style = tui.NewStyle().Foreground(tui.White).Bold()
	}
	return tui.New(
		tui.WithText(text),
		tui.WithTextStyle(style),
	)
}

func (c *RootComponent) activeDecisionRequest() *DecisionRequest {
	return c.state.DecisionReq.Get()
}

func decisionChoices(request *DecisionRequest) []string {
	if request != nil && len(request.Choices) > 0 {
		return request.Choices
	}
	return []string{"allow_once", "reject", "always_allow"}
}

func decisionChoiceLabel(lang i18n.Language, choice string) (string, string, tui.Color) {
	switch choice {
	case "allow_once":
		return "y", i18n.Text(lang, i18n.KeyPermissionAllowOnce), tui.Green
	case "always_allow":
		return "a", i18n.Text(lang, i18n.KeyPermissionAlwaysAllow), tui.Cyan
	case "execute":
		return "y", i18n.Text(lang, i18n.KeyPermissionExecute), tui.Green
	case "stay_in_plan":
		return "n", i18n.Text(lang, i18n.KeyPermissionStayInPlan), tui.Yellow
	default:
		return "n", i18n.Text(lang, i18n.KeyPermissionReject), tui.Red
	}
}

func (c *RootComponent) renderDecisionDialog(request *DecisionRequest) *tui.Element {
	prompt := *request
	riskLabel := i18n.Text(c.state.Language.Get(), i18n.KeyRuntimeRiskLow)
	riskColor := tui.Green
	if prompt.RiskLevel == 2 {
		riskLabel, riskColor = i18n.Text(c.state.Language.Get(), i18n.KeyRiskMedium), tui.Yellow
	} else if prompt.RiskLevel >= 3 {
		riskLabel, riskColor = i18n.Text(c.state.Language.Get(), i18n.KeyRiskHigh), tui.Red
	}
	title := i18n.Text(c.state.Language.Get(), i18n.KeyPermissionDecision)
	if prompt.Kind == permissions.PromptKindPlan {
		title = i18n.Text(c.state.Language.Get(), i18n.KeyPlanDecision)
	}
	borderColor := riskColor
	if c.decisionScrollTarget.Get() == decisionScrollTranscript {
		borderColor = tui.BrightBlack
	}
	container := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithBorder(tui.BorderDouble),
		tui.WithBorderStyle(tui.NewStyle().Foreground(borderColor)),
		tui.WithPaddingTRBL(0, 1, 0, 1),
		tui.WithHeight(c.decisionDialogHeight(request)),
		tui.WithWidthPercent(100),
	)
	container.AddChild(tui.New(tui.WithText(title), tui.WithTextStyle(tui.NewStyle().Foreground(riskColor).Bold())))
	details := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithFlexGrow(1),
		tui.WithWidthPercent(100),
		tui.WithScrollable(tui.ScrollVertical),
		tui.WithScrollOffset(0, c.decisionScroll.Get()),
	)
	// Keep a reference to this exact viewport. go-tui only routes a wheel event
	// to the deepest element under the pointer, which is normally one of these
	// text children rather than their scrollable parent.
	c.decisionRef.Set(details)
	details.AddChild(tui.New(
		tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyDecisionActor, prompt.ActorID, prompt.ActorType, prompt.WorkUnitID)),
		tui.WithWidthPercent(100),
	))
	if prompt.ExecutionSessionID != "" {
		details.AddChild(tui.New(
			tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyDecisionAgentSession)+prompt.ExecutionSessionID),
			tui.WithTextStyle(tui.NewStyle().Dim()),
			tui.WithWidthPercent(100),
		))
	}
	details.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyDecisionAction)+prompt.Action),
		tui.WithTextStyle(tui.NewStyle().Bold()),
		tui.WithWidthPercent(100),
	))
	details.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyDecisionTarget)+prompt.Target),
		tui.WithWidthPercent(100),
	))
	details.AddChild(tui.New(
		tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyDecisionImpact)+prompt.Impact),
		tui.WithWidthPercent(100),
	))
	details.AddChild(tui.New(
		tui.WithText(fmt.Sprintf("%s%s - %s", i18n.Text(c.state.Language.Get(), i18n.KeyDecisionRisk), riskLabel, prompt.RiskReason)),
		tui.WithTextStyle(tui.NewStyle().Foreground(riskColor)),
		tui.WithWidthPercent(100),
	))
	details.AddChild(tui.New(tui.WithText(i18n.Format(c.state.Language.Get(), i18n.KeyRuntimeDecisionScopeRule,
		i18n.Text(c.state.Language.Get(), i18n.KeyDecisionScope)+prompt.ApprovalScope, prompt.RuleSource)), tui.WithTextStyle(tui.NewStyle().Dim()), tui.WithWidthPercent(100)))
	if prompt.Body != "" {
		details.AddChild(tui.New(
			tui.WithText(prompt.Body),
			tui.WithWidthPercent(100),
		))
	} else if preview := permissionInputPreview(prompt.Input, c.termWidth-12); preview != "" {
		details.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyDecisionInput)+preview), tui.WithTextStyle(tui.NewStyle().Dim()), tui.WithWidthPercent(100)))
	}
	for _, detail := range prompt.ReviewDetails {
		details.AddChild(tui.New(tui.WithText(detail), tui.WithWidthPercent(100)))
	}
	if prompt.PostMode != "" {
		details.AddChild(tui.New(tui.WithText(i18n.Text(c.state.Language.Get(), i18n.KeyDecisionAfterApproval)+prompt.PostMode), tui.WithTextStyle(tui.NewStyle().Bold()), tui.WithWidthPercent(100)))
	}
	container.AddChild(details)
	selected := c.state.DecisionSelected.Get()
	direction := tui.Row
	if c.termWidth < 60 {
		direction = tui.Column
	}
	row := tui.New(tui.WithDisplay(tui.DisplayFlex), tui.WithDirection(direction), tui.WithWidthPercent(100))
	for i, choice := range decisionChoices(request) {
		if i > 0 && direction == tui.Row {
			row.AddChild(tui.New(tui.WithText("  ")))
		}
		hotkey, label, color := decisionChoiceLabel(c.state.Language.Get(), choice)
		row.AddChild(permissionActionElement(i, "["+hotkey+"]", label, color, selected))
	}
	container.AddChild(row)
	return container
}

// ---------------------------------------------------------------------------
// Copy to clipboard
// ---------------------------------------------------------------------------

func (c *RootComponent) copyInputSelection(cut bool) bool {
	text := c.input.SelectedText()
	if text == "" {
		return false
	}
	writer := c.clipboardWriter
	if writer == nil {
		writer = writeToClipboardInLanguage
	}
	if err := writer(c.state.Language.Get(), text); err != nil {
		c.copyFeedback.Set(i18n.Format(c.state.Language.Get(), i18n.KeyClipboardCopyFailed, err.Error()))
	} else {
		preview := truncateRunes(text, 40, "…")
		preview = strings.ReplaceAll(preview, "\n", "↵")
		c.copyFeedback.Set(i18n.Format(c.state.Language.Get(), i18n.KeyClipboardCopied, preview))
		if cut {
			c.input.DeleteSelection()
		}
	}
	c.scheduleCopyFeedbackClear()
	return true
}

// copyLastAssistant copies the most recent assistant message's text to the
// system clipboard and shows a brief feedback toast in the status bar.
func (c *RootComponent) copyLastAssistant() {
	text := c.state.LastAssistantText()
	if text == "" {
		c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyClipboardNothing))
		c.scheduleCopyFeedbackClear()
		return
	}
	if err := writeToClipboardInLanguage(c.state.Language.Get(), text); err != nil {
		c.copyFeedback.Set(i18n.Format(c.state.Language.Get(), i18n.KeyClipboardCopyFailed, err.Error()))
	} else {
		// Show preview of what was copied (truncated)
		preview := truncateRunes(text, 40, "…")
		preview = strings.ReplaceAll(preview, "\n", "↵")
		c.copyFeedback.Set(i18n.Format(c.state.Language.Get(), i18n.KeyClipboardCopied, preview))
	}
	c.scheduleCopyFeedbackClear()
}

// scheduleCopyFeedbackClear clears the copy feedback toast after 2 seconds.
// Uses a single resettable timer instead of spawning a goroutine per event.
// If called again before the previous timer fires, the timer is reset,
// so only one goroutine is ever in flight regardless of how fast the user
// triggers copy/paste operations.
func (c *RootComponent) scheduleCopyFeedbackClear() {
	if c.copyFeedbackTimer != nil {
		c.copyFeedbackTimer.Stop()
	}
	c.copyFeedbackTimer = time.AfterFunc(2*time.Second, func() {
		if c.app != nil {
			c.app.QueueUpdate(func() {
				c.copyFeedback.Set("")
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Image paste from clipboard
// ---------------------------------------------------------------------------

// handleImagePaste reads an image from the system clipboard and adds it as a
// pending attachment. The image will be sent with the next user message when
// they press Enter. Shows a toast with feedback.
//
// This runs clipboard commands (osascript/xclip/powershell) which may take
// up to 5 seconds, so it is dispatched in a goroutine and updates state via
// QueueUpdate to avoid blocking the TUI event loop.
func (c *RootComponent) handleImagePaste() {
	if !c.state.ModelCanSeeImages.Get() {
		c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyImageUnsupported))
		c.scheduleCopyFeedbackClear()
		return
	}

	// Show immediate feedback that we're checking the clipboard
	c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyImageCheckingClipboard))
	c.scheduleCopyFeedbackClear()

	go func() {
		base64Data, mediaType, err := input.GetClipboardImage()
		if c.app == nil {
			return
		}
		c.app.QueueUpdate(func() {
			if err != nil {
				c.copyFeedback.Set(i18n.Format(c.state.Language.Get(), i18n.KeyImageClipboardError, err.Error()))
				c.scheduleCopyFeedbackClear()
				return
			}
			if base64Data == "" {
				c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyImageClipboardEmpty))
				c.scheduleCopyFeedbackClear()
				return
			}
			if !c.state.ModelCanSeeImages.Get() {
				c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyImageUnsupported))
				c.scheduleCopyFeedbackClear()
				return
			}

			// Add to pending images and insert its atomic token at the cursor.
			id := c.attachPastedImage(base64Data, mediaType)

			// Show success feedback
			// Estimate size for display: base64 is ~4/3 of raw size
			rawBytes := len(base64Data) * 3 / 4
			sizeStr := formatBytes(rawBytes)
			c.copyFeedback.Set(i18n.Format(c.state.Language.Get(), i18n.KeyImagePasted, id, mediaType, sizeStr))
			c.scheduleCopyFeedbackClear()
		})
	}()
}

func (c *RootComponent) attachPastedImage(base64Data, mediaType string) int {
	id := c.state.AddPendingImage(base64Data, mediaType)
	placeholder := i18n.Format(c.state.Language.Get(), i18n.KeyTUIImageTag, id)
	c.input.InsertText(imageComposerPlaceholder(placeholder))
	return id
}

// formatBytes formats a byte count into a human-readable string.
func formatBytes(b int) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// ---------------------------------------------------------------------------
// Scroll helpers
// ---------------------------------------------------------------------------

// scrollBy adjusts the scroll position by delta rows.
// It clamps the result to [0, maxY] and updates stickToBottom accordingly:
// reaching the bottom re-enables auto-follow; scrolling up disables it.
//
// Note on Ref timing: contentRef points to the Element from the previous
// render frame. Use that element's actual offset as the baseline because
// deferred sticky-bottom layout can move it without updating scrollY.
func (c *RootComponent) scrollBy(delta int) {
	el := c.contentRef.El()
	if el == nil {
		return // first render hasn't happened yet
	}
	_, maxY := el.MaxScroll()
	curX, cur := el.ScrollOffset()
	// Guard: if scrollY somehow exceeds maxY (e.g. content shrank or was
	// cleared), snap it to maxY before applying delta.
	if cur > maxY {
		cur = maxY
	}
	newY := cur + delta
	if newY < 0 {
		newY = 0
	}
	if newY > maxY {
		newY = maxY
	}
	el.ScrollTo(curX, newY)
	c.scrollY.Set(newY)
	c.stickToBottom.Set(newY >= maxY)
}

// scrollByPage scrolls by a page-sized chunk based on actual viewport height.
// direction should be -1 (up) or +1 (down).
// Keeps 2 lines of overlap for reading context, matching industry convention.
func (c *RootComponent) scrollByPage(direction int) {
	el := c.contentRef.El()
	if el == nil {
		return
	}
	_, vpH := el.ViewportSize()
	delta := vpH - 2 // 2-line overlap for context continuity
	if delta < 1 {
		delta = 1
	}
	c.scrollBy(direction * delta)
}

// scrollToBottom re-enables sticky auto-follow and best-effort scrolls to
// the end of content. The real scroll-to-bottom is guaranteed by
// renderMessageArea which applies WithScrollToBottomOnLayout() whenever
// stickToBottom is true — that option defers the scroll until AFTER layout
// computes the true content height. The scrollY.Set(maxY) here is a
// best-effort for the current frame; it may be slightly stale if new
// content was just added (the Ref points to the previous frame's Element).
func (c *RootComponent) scrollToBottom() {
	el := c.contentRef.El()
	if el != nil {
		_, maxY := el.MaxScroll()
		c.scrollY.Set(maxY)
	} else {
		// Element not yet available (first render); set 0 and let the
		// OnChange watcher handle scroll-to-bottom on next message.
		c.scrollY.Set(0)
	}
	c.stickToBottom.Set(true)
}

// scrollToTop scrolls to the very beginning of content and disables
// sticky auto-follow.
func (c *RootComponent) scrollToTop() {
	c.scrollY.Set(0)
	c.stickToBottom.Set(false)
}

// scrollDecisionBy adjusts the permission-details viewport using the element's
// real laid-out bounds. Updating both the element and State keeps the current
// frame responsive and the next render at the same offset.
func (c *RootComponent) scrollDecisionBy(delta int) {
	el := c.decisionRef.El()
	if el == nil {
		return
	}
	_, maxY := el.MaxScroll()
	curX, curY := el.ScrollOffset()
	if curY > maxY {
		curY = maxY
	}
	next := curY + delta
	if next < 0 {
		next = 0
	}
	if next > maxY {
		next = maxY
	}
	el.ScrollTo(curX, next)
	c.decisionScroll.Set(next)
}

func (c *RootComponent) scrollDecisionByPage(direction int) {
	el := c.decisionRef.El()
	if el == nil {
		return
	}
	_, viewportHeight := el.ViewportSize()
	delta := viewportHeight - 2
	if delta < 1 {
		delta = 1
	}
	c.scrollDecisionBy(direction * delta)
}

func (c *RootComponent) scrollDecisionToBottom() {
	el := c.decisionRef.El()
	if el == nil {
		return
	}
	_, maxY := el.MaxScroll()
	c.scrollDecisionBy(maxY)
}

func (c *RootComponent) setDecisionScrollTarget(target decisionScrollTarget) {
	if c.decisionScrollTarget.Get() != target {
		c.decisionScrollTarget.Set(target)
	}
}

func (c *RootComponent) toggleDecisionScrollTarget() {
	if c.decisionScrollTarget.Get() == decisionScrollTranscript {
		c.setDecisionScrollTarget(decisionScrollDetails)
		return
	}
	c.setDecisionScrollTarget(decisionScrollTranscript)
}

func (c *RootComponent) scrollFocusedDecisionRegion(delta int) {
	if c.decisionScrollTarget.Get() == decisionScrollTranscript {
		c.scrollBy(delta)
		return
	}
	c.scrollDecisionBy(delta)
}

func (c *RootComponent) scrollFocusedDecisionRegionByPage(direction int) {
	if c.decisionScrollTarget.Get() == decisionScrollTranscript {
		c.pageTranscriptHistory(direction)
		c.scrollByPage(direction)
		return
	}
	c.scrollDecisionByPage(direction)
}

func (c *RootComponent) scrollFocusedDecisionRegionToTop() {
	if c.decisionScrollTarget.Get() == decisionScrollTranscript {
		c.setHistoryStart(0)
		c.scrollToTop()
		return
	}
	c.scrollDecisionBy(-c.decisionScroll.Get())
}

func (c *RootComponent) scrollFocusedDecisionRegionToBottom() {
	if c.decisionScrollTarget.Get() == decisionScrollTranscript {
		c.setHistoryStart(-1)
		c.scrollToBottom()
		return
	}
	c.scrollDecisionToBottom()
}

// HandleMouse implements tui.MouseListener for mouse wheel scrolling and
// interactive transcript controls.
func (c *RootComponent) HandleMouse(me tui.MouseEvent) bool {
	// Permission prompts leave both their details and the transcript readable.
	// Route the wheel by pointer position because go-tui's default deepest-node
	// hit test otherwise lands on a text child instead of either scrollable
	// ancestor. A click also changes the keyboard scroll target without
	// consuming the event, preserving native terminal text selection.
	if request := c.activeDecisionRequest(); request != nil {
		if request.Kind == permissions.PromptKindAskUser {
			return false
		}
		if me.Button == tui.MouseWheelUp || me.Button == tui.MouseWheelDown {
			delta := 3
			if me.Button == tui.MouseWheelUp {
				delta = -delta
			}
			if details := c.decisionRef.El(); details != nil && details.ContainsPoint(me.X, me.Y) {
				c.setDecisionScrollTarget(decisionScrollDetails)
				c.scrollDecisionBy(delta)
				return true
			}
			if transcript := c.contentRef.El(); transcript != nil && transcript.ContainsPoint(me.X, me.Y) {
				c.setDecisionScrollTarget(decisionScrollTranscript)
				c.scrollBy(delta)
				return true
			}
			return false
		}
		if me.Button == tui.MouseLeft && me.Action == tui.MousePress && me.Mod == tui.ModNone {
			if details := c.decisionRef.El(); details != nil && details.ContainsPoint(me.X, me.Y) {
				c.setDecisionScrollTarget(decisionScrollDetails)
				return false
			}
			if transcript := c.contentRef.El(); transcript != nil && transcript.ContainsPoint(me.X, me.Y) {
				c.setDecisionScrollTarget(decisionScrollTranscript)
				return false
			}
		}
		return false
	}

	// --- Mouse wheel scrolling ---
	switch me.Button {
	case tui.MouseWheelUp:
		c.scrollBy(-3)
		return true
	case tui.MouseWheelDown:
		c.scrollBy(3)
		return true
	}

	if me.Button == tui.MouseLeft && (me.Action == tui.MousePress || me.Action == tui.MouseRelease) {
		c.selectionHintShownForDrag = false
	}
	if me.Button == tui.MouseLeft && me.Action == tui.MouseDrag && me.Mod == tui.ModNone && !c.selectionHintShownForDrag && !c.hasPickerOverlay() {
		if transcript := c.contentRef.El(); transcript != nil && transcript.ContainsPoint(me.X, me.Y) {
			c.selectionHintShownForDrag = true
			c.showNativeSelectionHint()
		}
	}

	if me.Button == tui.MouseLeft && me.Action == tui.MousePress && me.Mod == tui.ModNone {
		if taskView := c.taskViewRef.El(); taskView != nil && taskView.ContainsPoint(me.X, me.Y) {
			if c.state.ExpandedView.Get() == "tasks" {
				c.state.SetExpandedView("")
			} else {
				c.state.SetExpandedView("tasks")
			}
			return true
		}
	}

	// Segment headers remain clickable while terminals handle native text
	// selection through their mouse-reporting bypass modifier.
	if me.Button == tui.MouseLeft && me.Action == tui.MousePress && me.Mod == tui.ModNone && c.toggleToolSegmentAt(me.X, me.Y) {
		return true
	}

	return false
}

const nativeSelectionHintDuration = 4 * time.Second

func nativeSelectionHintKey(termProgram, lcTerminal string) i18n.Key {
	terminalID := strings.ToLower(termProgram + " " + lcTerminal)
	if strings.Contains(terminalID, "iterm") {
		return i18n.KeyTranscriptSelectionHintOption
	}
	return i18n.KeyTranscriptSelectionHintGeneric
}

func (c *RootComponent) showNativeSelectionHint() {
	c.transcriptSelectionHintVisible.Set(true)
	c.selectionHintGeneration++
	generation := c.selectionHintGeneration
	if c.transcriptSelectionHintTimer != nil {
		c.transcriptSelectionHintTimer.Stop()
	}
	c.transcriptSelectionHintTimer = time.AfterFunc(nativeSelectionHintDuration, func() {
		if c.app != nil {
			c.app.QueueUpdate(func() {
				if c.selectionHintGeneration == generation {
					c.transcriptSelectionHintVisible.Set(false)
				}
			})
		}
	})
}

func (c *RootComponent) toggleToolSegmentAt(x, y int) bool {
	if c.segmentRefs == nil {
		return false
	}
	for id, element := range c.segmentRefs.All() {
		// ContainsPoint converts content-space layouts through every scrollable
		// ancestor and clips them to the visible viewport. This makes the whole
		// rendered header row clickable even though transcript children are laid
		// out in the message area's unscrolled content coordinate system.
		if element != nil && element.ContainsPoint(x, y) {
			return c.toggleToolSegmentByID(id)
		}
	}
	return false
}

func (c *RootComponent) toggleToolSegmentByID(id string) bool {
	if id == "" {
		return false
	}
	if activity, ok := c.subagentActivityBySegmentID(id); ok {
		ongoing := c.subagentProgressSegmentOngoing(activity)
		if !ongoing {
			expanded := c.subagentProgressSegmentExpanded(activity, c.state.TranscriptShowAll.Get())
			c.state.SetToolSegmentExpanded(id, !expanded)
		}
		if activity.Attention.Kind == ActivityAttentionReadyForReview && activity.Attention.Unread {
			_ = c.state.AcknowledgeActivity(activity.ID)
		}
		return true
	}
	focused := c.state.ActiveSessionInteraction().FocusedObservationID
	for _, item := range BuildTranscriptToolSegments(c.state.Messages.Get()) {
		if item.Segment == nil || item.Segment.ID != id {
			continue
		}
		if transcriptToolSegmentOngoing(c.state.Messages.Get(), *item.Segment) {
			return true
		}
		expanded := c.toolSegmentExpanded(*item.Segment, focused, c.state.TranscriptShowAll.Get())
		c.state.SetToolSegmentExpanded(id, !expanded)
		return true
	}
	return false
}

// toggleFocusedOrLatestToolSegment provides a keyboard path for the same
// group control. A focused member selects its group; otherwise the most recent
// segment is used, matching the transcript's reading direction.
func (c *RootComponent) toggleFocusedOrLatestToolSegment() bool {
	items := BuildTranscriptToolSegments(c.state.Messages.Get())
	focused := c.state.ActiveSessionInteraction().FocusedObservationID
	latest := ""
	latestEnd := -1
	for _, item := range items {
		if item.Segment == nil {
			continue
		}
		latest = item.Segment.ID
		latestEnd = item.End
		for _, message := range item.Segment.Messages {
			if focused != "" && message.ObservationID == focused {
				return c.toggleToolSegmentByID(item.Segment.ID)
			}
		}
	}
	if focused != "" {
		if observation, ok := c.state.GetObservation(focused); ok {
			if activity, found := c.subagentActivityForObservation(observation); found {
				return c.toggleToolSegmentByID(subagentProgressSegmentID(activity))
			}
		}
	}
	messages := c.state.Messages.Get()
	for index := len(messages) - 1; index >= 0 && index >= latestEnd; index-- {
		if messages[index].ObservationID == "" {
			continue
		}
		observation, ok := c.state.GetObservation(messages[index].ObservationID)
		if !ok {
			continue
		}
		if activity, found := c.subagentActivityForObservation(observation); found {
			return c.toggleToolSegmentByID(subagentProgressSegmentID(activity))
		}
	}
	return c.toggleToolSegmentByID(latest)
}

// KeyMap returns global key bindings.
//
// Input is registered via MountPersistent in Render(), so its KeyMap()
// (OnFocused bindings for AnyRune, Backspace, Enter, etc.) is automatically
// discovered by walkComponents/buildDispatchTable with correct focusCheck
// wiring. No manual merging needed here.
func (c *RootComponent) sendPermissionResponse(resp string) {
	request := c.state.DecisionReq.Get()
	if request == nil {
		return
	}
	choices := decisionChoices(request)
	choice := "reject"
	switch resp {
	case "y":
		choice = choices[0]
	case "a":
		for _, candidate := range choices {
			if candidate == "always_allow" {
				choice = candidate
			}
		}
	case "n":
		for _, candidate := range choices {
			if candidate == "reject" || candidate == "stay_in_plan" {
				choice = candidate
			}
		}
	}
	outcome := permissions.PromptOutcomeApproved
	if choice == "reject" || choice == "stay_in_plan" {
		outcome = permissions.PromptOutcomeRejected
	}
	select {
	case c.state.DecisionResp <- decisionResponse(request.DecisionID, outcome, choice):
	default:
	}
}

func (c *RootComponent) movePermissionSelection(delta int) {
	request := c.state.DecisionReq.Get()
	if request == nil {
		return
	}
	count := len(decisionChoices(request))
	selected := (c.state.DecisionSelected.Get() + delta) % count
	if selected < 0 {
		selected += count
	}
	c.state.DecisionSelected.Set(selected)
}

func (c *RootComponent) confirmPermissionSelection() {
	request := c.state.DecisionReq.Get()
	if request == nil {
		return
	}
	choices := decisionChoices(request)
	selected := c.state.DecisionSelected.Get()
	if selected < 0 || selected >= len(choices) {
		selected = 0
	}
	choice := choices[selected]
	outcome := permissions.PromptOutcomeApproved
	if choice == "reject" || choice == "stay_in_plan" {
		outcome = permissions.PromptOutcomeRejected
	}
	select {
	case c.state.DecisionResp <- decisionResponse(request.DecisionID, outcome, choice):
	default:
	}
}

func (c *RootComponent) escapeDecision() {
	request := c.state.DecisionReq.Get()
	if request == nil {
		return
	}
	select {
	case c.state.DecisionResp <- decisionResponse(request.DecisionID, permissions.PromptOutcomeEscaped, ""):
	default:
	}
}

// switchLanguage is the single commit boundary for every TUI language
// surface. SaveLanguage publishes the process-wide active language only after
// the preference file has been written, so publishing AppState afterwards
// keeps a failed transition entirely on the previous language.
func (c *RootComponent) switchLanguage(target i18n.Language) error {
	c.languageMu.Lock()
	saver := c.languageSaver
	if saver == nil {
		saver = i18n.SaveLanguage
	}
	if err := saver(target); err != nil {
		c.languageMu.Unlock()
		return err
	}
	c.state.Language.Set(target)
	_ = c.state.RelocalizeToolPresentations(target)
	observer := c.onLanguageSwitch
	c.languageMu.Unlock()

	if observer != nil {
		observer(target)
	}
	return nil
}

func (c *RootComponent) armExitConfirmation() {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	c.lastInterruptAt = now()
}

func (c *RootComponent) exitConfirmationArmed() bool {
	if c.lastInterruptAt.IsZero() {
		return false
	}
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	window := c.exitConfirmWindow
	if window <= 0 {
		window = 2 * time.Second
	}
	elapsed := now().Sub(c.lastInterruptAt)
	return elapsed >= 0 && elapsed <= window
}

func (c *RootComponent) exitApplication() {
	if c.onExit != nil {
		c.onExit()
		return
	}
	if c.app != nil {
		c.app.Stop()
	}
}

func (c *RootComponent) handleCtrlC(copySelection bool) {
	if copySelection && c.copyInputSelection(false) {
		return
	}
	if c.state.TryCancelQuery() {
		c.armExitConfirmation()
		c.state.AppendMessage(Message{
			Kind:      MsgInfo,
			Text:      i18n.Text(c.state.Language.Get(), i18n.KeyRuntimeQueryCancelled),
			Timestamp: time.Now(),
		})
		return
	}
	if c.exitConfirmationArmed() {
		c.lastInterruptAt = time.Time{}
		c.exitApplication()
		return
	}
	c.armExitConfirmation()
	c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyTUIExitConfirm))
	c.scheduleCopyFeedbackClear()
}

func (c *RootComponent) KeyMap() tui.KeyMap {
	var km tui.KeyMap
	if c.state.HasActiveQuery() && c.hasPickerOverlay() {
		km = append(km, tui.OnPreemptStop(tui.KeyCtrlC, func(ke tui.KeyEvent) {
			c.handleCtrlC(false)
		}))
	}

	// Exact /skills checklist keys. Enter selects the highlighted skill while
	// Space remains the explicit visibility toggle. Both precede AnyKey so they
	// can never leak into the input or be mistaken for filter text.
	if menu := c.state.SkillsMenu.Get(); menu != nil && menu.Visible {
		km = append(km,
			tui.OnPreemptStop(tui.KeyEscape, func(ke tui.KeyEvent) {
				c.state.SkillsMenu.Set(nil)
			}),
			tui.OnPreemptStop(tui.KeyUp, func(ke tui.KeyEvent) {
				current := c.state.SkillsMenu.Get()
				if current == nil {
					return
				}
				next := current.clone()
				next.Toggle.move(-1)
				c.state.SkillsMenu.Set(next)
			}),
			tui.OnPreemptStop(tui.KeyDown, func(ke tui.KeyEvent) {
				current := c.state.SkillsMenu.Get()
				if current == nil {
					return
				}
				next := current.clone()
				next.Toggle.move(1)
				c.state.SkillsMenu.Set(next)
			}),
			tui.OnPreemptStop(tui.KeyEnter, func(ke tui.KeyEvent) {
				c.selectHighlightedSkill()
			}),
			tui.OnPreemptStop(tui.KeyBackspace, func(ke tui.KeyEvent) {
				current := c.state.SkillsMenu.Get()
				if current == nil {
					return
				}
				next := current.clone()
				next.Toggle.backspaceQuery()
				c.state.SkillsMenu.Set(next)
			}),
			tui.OnPreemptStop(tui.Rune(' '), func(ke tui.KeyEvent) {
				current := c.state.SkillsMenu.Get()
				if current != nil {
					c.toggleSelectedSkill()
				}
			}),
			tui.OnPreemptStop(tui.AnyKey, func(ke tui.KeyEvent) {
				if ke.Rune == 0 {
					return
				}
				current := c.state.SkillsMenu.Get()
				if current == nil {
					return
				}
				next := current.clone()
				next.Toggle.appendQuery(ke.Rune)
				c.state.SkillsMenu.Set(next)
			}),
		)
		return km
	}

	// Fork picker keys
	if picker := c.state.ForkPicker.Get(); picker != nil && picker.Visible {
		km = append(km,
			tui.OnPreemptStop(tui.KeyEscape, func(ke tui.KeyEvent) {
				p := c.state.ForkPicker.Get()
				if p != nil && p.OnCancel != nil {
					p.OnCancel()
				}
				c.state.ForkPicker.Set(nil)
			}),
			tui.OnPreemptStop(tui.KeyUp, func(ke tui.KeyEvent) {
				p := c.state.ForkPicker.Get()
				if p == nil {
					return
				}
				p.Selected--
				p.clamp()
				c.state.ForkPicker.Set(p)
			}),
			tui.OnPreemptStop(tui.KeyDown, func(ke tui.KeyEvent) {
				p := c.state.ForkPicker.Get()
				if p == nil {
					return
				}
				p.Selected++
				p.clamp()
				c.state.ForkPicker.Set(p)
			}),
			tui.OnPreemptStop(tui.KeyEnter, func(ke tui.KeyEvent) {
				p := c.state.ForkPicker.Get()
				if p == nil {
					return
				}
				if p.OnSelect != nil && len(p.Entries) > 0 {
					p.OnSelect(p.Entries[p.Selected])
				}
				c.state.ForkPicker.Set(nil)
				if c.app != nil {
					c.app.RequestFullRedraw()
				}
			}),
			tui.OnPreemptStop(tui.AnyKey, func(ke tui.KeyEvent) {}),
		)
		return km
	}

	// Session picker keys
	if picker := c.state.SessionPicker.Get(); picker != nil && picker.Visible {
		km = append(km,
			tui.OnPreemptStop(tui.KeyEscape, func(ke tui.KeyEvent) {
				p := c.state.SessionPicker.Get()
				if p != nil && p.OnCancel != nil {
					p.OnCancel()
				}
				c.state.SessionPicker.Set(nil)
			}),
			tui.OnPreemptStop(tui.KeyUp, func(ke tui.KeyEvent) {
				p := c.state.SessionPicker.Get()
				if p == nil {
					return
				}
				p.Selected--
				p.clamp()
				c.state.SessionPicker.Set(p)
			}),
			tui.OnPreemptStop(tui.KeyDown, func(ke tui.KeyEvent) {
				p := c.state.SessionPicker.Get()
				if p == nil {
					return
				}
				p.Selected++
				p.clamp()
				c.state.SessionPicker.Set(p)
			}),
			tui.OnPreemptStop(tui.KeyEnter, func(ke tui.KeyEvent) {
				p := c.state.SessionPicker.Get()
				if p == nil {
					return
				}
				if p.OnSelect != nil && len(p.Entries) > 0 {
					p.OnSelect(p.Entries[p.Selected])
				}
				c.state.SessionPicker.Set(nil)
				if c.app != nil {
					c.app.RequestFullRedraw()
				}
			}),
			// Block other keypresses from leaking into the input while the picker is open.
			tui.OnPreemptStop(tui.AnyKey, func(ke tui.KeyEvent) {}),
		)
		return km
	}

	// Model picker keys — phase-aware cascading navigation (Provider → Model/Connect)
	if mp := c.state.ModelPicker.Get(); mp != nil && mp.Visible {
		km = append(km,
			tui.OnPreemptStop(tui.KeyEscape, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p == nil {
					return
				}
				switch p.Phase {
				case PickerPhaseReasoning:
					p.Phase = PickerPhaseModel
					c.state.ModelPicker.Set(p)
					if c.app != nil {
						c.app.RequestFullRedraw()
					}
				case PickerPhaseModel, PickerPhaseConnect, PickerPhaseDeleteConfirm:
					// Esc in model/connect phase → go back to provider phase
					p.GoBack()
					c.state.ModelPicker.Set(p)
					if c.app != nil {
						c.app.RequestFullRedraw()
					}
				default:
					// Esc in provider phase → close picker
					if p.OnCancel != nil {
						p.OnCancel()
					}
					c.state.ModelPicker.Set(nil)
					if c.app != nil {
						c.app.RequestFullRedraw()
					}
				}
			}),
			tui.OnPreemptStop(tui.KeyUp, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p == nil {
					return
				}
				switch p.Phase {
				case PickerPhaseProvider:
					p.ProviderSelected--
					p.clampProvider()
				case PickerPhaseModel:
					p.Selected--
					p.clamp()
				case PickerPhaseReasoning:
					p.ReasoningSelected--
					p.clampReasoning()
				case PickerPhaseConnect:
					// Navigate auth method selection
					if len(p.ConnectAuthMethods) > 1 {
						p.ConnectSelectedAuth--
						if p.ConnectSelectedAuth < 0 {
							p.ConnectSelectedAuth = len(p.ConnectAuthMethods) - 1
						}
					}
				}
				c.state.ModelPicker.Set(p)
			}),
			tui.OnPreemptStop(tui.KeyDown, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p == nil {
					return
				}
				switch p.Phase {
				case PickerPhaseProvider:
					p.ProviderSelected++
					p.clampProvider()
				case PickerPhaseModel:
					p.Selected++
					p.clamp()
				case PickerPhaseReasoning:
					p.ReasoningSelected++
					p.clampReasoning()
				case PickerPhaseConnect:
					// Navigate auth method selection
					if len(p.ConnectAuthMethods) > 1 {
						p.ConnectSelectedAuth++
						if p.ConnectSelectedAuth >= len(p.ConnectAuthMethods) {
							p.ConnectSelectedAuth = 0
						}
					}
				}
				c.state.ModelPicker.Set(p)
			}),
			tui.OnPreemptStop(tui.KeyLeft, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p != nil && p.Phase == PickerPhaseConnect && p.ConnectInputField == ConnectInputAPIStyle {
					p.changeAPIStyle(-1)
					p.ConnectError = ""
					c.state.ModelPicker.Set(p)
				}
			}),
			tui.OnPreemptStop(tui.KeyRight, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p != nil && p.Phase == PickerPhaseConnect && p.ConnectInputField == ConnectInputAPIStyle {
					p.changeAPIStyle(1)
					p.ConnectError = ""
					c.state.ModelPicker.Set(p)
				}
			}),
			tui.OnPreemptStop(tui.KeyEnter, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p == nil {
					return
				}
				switch p.Phase {
				case PickerPhaseProvider:
					if p.ProviderSelected < len(p.Providers) {
						prov := p.Providers[p.ProviderSelected]
						if prov.CanSelectModels && !prov.IsCreate {
							// Selectable provider → advance to model phase
							p.Phase = PickerPhaseModel
							p.SelectedProvider = prov.Name
							if p.EnterProvider != nil {
								p.EnterProvider(prov.Name)
							}
							if c.app != nil {
								c.app.RequestFullRedraw()
							}
						} else {
							// Unconfigured provider → enter connect/setup phase
							p.EnterProviderConnect(prov)
							c.state.ModelPicker.Set(p)
							if c.app != nil {
								c.app.RequestFullRedraw()
							}
						}
					}
				case PickerPhaseModel:
					// Enter in model phase → confirm selection
					if entry := p.selectedEntry(); entry != nil {
						if len(entry.ReasoningEfforts) > 0 {
							p.EnterReasoning(*entry)
							c.state.ModelPicker.Set(p)
							if c.app != nil {
								c.app.RequestFullRedraw()
							}
							return
						}
						if p.OnSelect != nil {
							p.OnSelect(*entry)
						}
					}
					c.state.ModelPicker.Set(nil)
					if c.app != nil {
						c.app.RequestFullRedraw()
					}
				case PickerPhaseReasoning:
					if p.OnSelect != nil {
						entry := p.ReasoningModel
						entry.ReasoningEffort = p.selectedReasoningEffort()
						p.OnSelect(entry)
					}
					c.state.ModelPicker.Set(nil)
					if c.app != nil {
						c.app.RequestFullRedraw()
					}
				case PickerPhaseConnect:
					// Enter in connect phase → submit connection
					if p.OnConnect != nil {
						// Determine selected auth method
						authMethod := "api_key" // default
						if len(p.ConnectAuthMethods) > 0 && p.ConnectSelectedAuth < len(p.ConnectAuthMethods) {
							authMethod = p.ConnectAuthMethods[p.ConnectSelectedAuth]
						} else if len(p.ConnectAuthMethods) == 0 {
							// Default to api_key if no auth methods specified
							authMethod = "api_key"
						}

						if authMethod == "api_key" {
							if p.ConnectAPIKeyInput == "" {
								p.ConnectError = i18n.Text(c.state.Language.Get(), i18n.KeyTUIConnectAPIKeyRequired)
								c.state.ModelPicker.Set(p)
								return
							}
							p.ConnectStatus = i18n.Text(c.state.Language.Get(), i18n.KeyTUIConnectSavingCredentials)
							p.ConnectError = ""
							c.state.ModelPicker.Set(p)
							p.OnConnect(ProviderConnectRequest{
								ProviderName: p.ConnectProvider,
								DisplayName:  strings.TrimSpace(p.ConnectProviderNameInput),
								AuthMethod:   authMethod,
								APIStyle:     p.selectedAPIStyle(),
								BaseURL:      strings.TrimSpace(p.ConnectBaseURLInput),
								APIKey:       p.ConnectAPIKeyInput,
								UserDefined:  p.ConnectUserDefined,
							})
						} else if authMethod == "oauth_pkce" || authMethod == "device_code" {
							// OAuth or Device auth — delegate to OnConnect which will
							// close the picker and run the flow in background
							p.OnConnect(ProviderConnectRequest{ProviderName: p.ConnectProvider, AuthMethod: authMethod})
						} else {
							if p.ConnectHint != "" {
								p.ConnectError = p.ConnectHint
							} else {
								p.ConnectError = i18n.Text(c.state.Language.Get(), i18n.KeyTUIConnectInlineUnavailable)
							}
							c.state.ModelPicker.Set(p)
						}
					}
				case PickerPhaseDeleteConfirm:
					c.confirmProviderDeletion(p)
				}
			}),
			tui.OnPreemptStop(tui.KeyBackspace, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p == nil {
					return
				}
				switch p.Phase {
				case PickerPhaseModel:
					if p.Query == "" {
						// Backspace with no query in model phase → go back to provider phase
						p.GoBack()
						if c.app != nil {
							c.app.RequestFullRedraw()
						}
					} else {
						p.backspaceQuery()
					}
					c.state.ModelPicker.Set(p)
				case PickerPhaseReasoning:
					p.Phase = PickerPhaseModel
					c.state.ModelPicker.Set(p)
					if c.app != nil {
						c.app.RequestFullRedraw()
					}
				case PickerPhaseConnect:
					activeInputEmpty := p.ConnectAPIKeyInput == ""
					if p.ConnectInputField == ConnectInputBaseURL {
						activeInputEmpty = p.ConnectBaseURLInput == ""
					} else if p.ConnectInputField == ConnectInputProviderName {
						activeInputEmpty = p.ConnectProviderNameInput == ""
					}
					if activeInputEmpty && p.ConnectAPIKeyInput == "" && p.ConnectBaseURLInput == "" && p.ConnectProviderNameInput == "" {
						// Backspace with no input → go back to provider phase
						p.GoBack()
						if c.app != nil {
							c.app.RequestFullRedraw()
						}
					} else {
						p.backspaceConnectInput()
					}
					c.state.ModelPicker.Set(p)
				}
				// In provider phase, backspace does nothing
			}),
			tui.OnPreemptStop(tui.KeyTab, func(ke tui.KeyEvent) {
				p := c.state.ModelPicker.Get()
				if p == nil || p.Phase != PickerPhaseConnect {
					return
				}
				authMethod := "api_key"
				if len(p.ConnectAuthMethods) > 0 && p.ConnectSelectedAuth < len(p.ConnectAuthMethods) {
					authMethod = p.ConnectAuthMethods[p.ConnectSelectedAuth]
				}
				if authMethod == "api_key" {
					p.toggleConnectInputField()
					p.ConnectError = ""
					c.state.ModelPicker.Set(p)
				}
			}),
			// Any printable rune → filter in model phase, API key input in connect phase
			tui.OnPreemptStop(tui.AnyKey, func(ke tui.KeyEvent) {
				if ke.Rune == 0 {
					return
				}
				p := c.state.ModelPicker.Get()
				if p == nil {
					return
				}
				switch p.Phase {
				case PickerPhaseProvider:
					if p.ProviderSelected < 0 || p.ProviderSelected >= len(p.Providers) {
						return
					}
					prov := p.Providers[p.ProviderSelected]
					switch unicode.ToLower(ke.Rune) {
					case 'd':
						if prov.UserDefined {
							p.EnterDeleteConfirm(prov.Name)
							c.state.ModelPicker.Set(p)
							if c.app != nil {
								c.app.RequestFullRedraw()
							}
						}
					case 'r':
						if prov.IsConnected || providerHasInteractiveConfig(prov) {
							if prov.IsConnected {
								p.EnterReconnect(prov)
							} else {
								p.EnterProviderConnect(prov)
							}
							c.state.ModelPicker.Set(p)
							if c.app != nil {
								c.app.RequestFullRedraw()
							}
						}
					}
				case PickerPhaseModel:
					p.appendQuery(ke.Rune)
					c.state.ModelPicker.Set(p)
				case PickerPhaseConnect:
					// Only accept typing for custom endpoint credentials.
					authMethod := "api_key"
					if len(p.ConnectAuthMethods) > 0 && p.ConnectSelectedAuth < len(p.ConnectAuthMethods) {
						authMethod = p.ConnectAuthMethods[p.ConnectSelectedAuth]
					}
					if authMethod == "api_key" {
						p.appendConnectInput(ke.Rune)
						p.ConnectError = "" // Clear error on new input
						c.state.ModelPicker.Set(p)
					}
				case PickerPhaseDeleteConfirm:
					if lowered := unicode.ToLower(ke.Rune); lowered == 'y' || lowered == 'd' {
						c.confirmProviderDeletion(p)
					}
				}
				// In provider phase, typing is ignored (selection only)
			}),
		)
		return km
	}

	// AskUser owns a separate transient editor so the conversation composer is
	// never mutated or submitted by question-dialog keystrokes.
	if c.activeAskUserRequest() != nil {
		km = append(km,
			tui.OnPreemptStop(tui.KeyUp, func(tui.KeyEvent) { c.moveAskUserSelection(-1) }),
			tui.OnPreemptStop(tui.KeyLeft, func(tui.KeyEvent) { c.moveAskUserSelection(-1) }),
			tui.OnPreemptStop(tui.KeyDown, func(tui.KeyEvent) { c.moveAskUserSelection(1) }),
			tui.OnPreemptStop(tui.KeyRight, func(tui.KeyEvent) { c.moveAskUserSelection(1) }),
			tui.OnPreemptStop(tui.KeyTab, func(tui.KeyEvent) { c.moveAskUserSelection(1) }),
			tui.OnPreemptStop(tui.Rune(' '), func(tui.KeyEvent) { c.toggleAskUserSelection() }),
			tui.OnPreemptStop(tui.Rune('n'), func(tui.KeyEvent) { c.beginOrAppendAskUserNotes() }),
			tui.OnPreemptStop(tui.KeyBackspace, func(tui.KeyEvent) { c.backspaceAskUserCustom() }),
			tui.OnPreemptStop(tui.KeyEnter, func(tui.KeyEvent) { c.confirmAskUserSelection() }),
			tui.OnPreemptStop(tui.KeyEscape, func(tui.KeyEvent) { c.escapeAskUser() }),
			tui.OnPreemptStop(tui.AnyRune, func(ke tui.KeyEvent) { c.appendAskUserCustom(ke.Rune) }),
			tui.OnPreemptStop(tui.AnyKey, func(tui.KeyEvent) {}),
		)
		return km
	}

	// Permission dialog keys (only active when dialog is shown)
	if c.activeDecisionRequest() != nil {
		km = append(km,
			tui.OnPreemptStop(tui.Rune('y'), func(ke tui.KeyEvent) {
				c.sendPermissionResponse("y")
			}),
			tui.OnPreemptStop(tui.Rune('n'), func(ke tui.KeyEvent) {
				c.sendPermissionResponse("n")
			}),
			tui.OnPreemptStop(tui.Rune('a'), func(ke tui.KeyEvent) {
				c.sendPermissionResponse("a")
			}),
			tui.OnPreemptStop(tui.KeyLeft, func(ke tui.KeyEvent) {
				c.movePermissionSelection(-1)
			}),
			tui.OnPreemptStop(tui.KeyUp, func(ke tui.KeyEvent) {
				c.scrollFocusedDecisionRegion(-1)
			}),
			tui.OnPreemptStop(tui.KeyRight, func(ke tui.KeyEvent) {
				c.movePermissionSelection(1)
			}),
			tui.OnPreemptStop(tui.KeyDown, func(ke tui.KeyEvent) {
				c.scrollFocusedDecisionRegion(1)
			}),
			tui.OnPreemptStop(tui.KeyTab, func(ke tui.KeyEvent) {
				c.toggleDecisionScrollTarget()
			}),
			tui.OnPreemptStop(tui.KeyTab.Shift(), func(ke tui.KeyEvent) {
				c.toggleDecisionScrollTarget()
			}),
			// Ctrl+PageUp/PageDown always address the transcript, even while
			// the permission details retain normal keyboard focus.
			tui.OnPreemptStop(tui.KeyPageUp.Ctrl(), func(ke tui.KeyEvent) {
				c.pageTranscriptHistory(-1)
				c.scrollByPage(-1)
			}),
			tui.OnPreemptStop(tui.KeyPageDown.Ctrl(), func(ke tui.KeyEvent) {
				c.pageTranscriptHistory(1)
				c.scrollByPage(1)
			}),
			tui.OnPreemptStop(tui.KeyPageUp, func(ke tui.KeyEvent) {
				c.scrollFocusedDecisionRegionByPage(-1)
			}),
			tui.OnPreemptStop(tui.KeyPageDown, func(ke tui.KeyEvent) {
				c.scrollFocusedDecisionRegionByPage(1)
			}),
			tui.OnPreemptStop(tui.KeyHome, func(ke tui.KeyEvent) {
				c.scrollFocusedDecisionRegionToTop()
			}),
			tui.OnPreemptStop(tui.KeyEnd, func(ke tui.KeyEvent) {
				c.scrollFocusedDecisionRegionToBottom()
			}),
			tui.OnPreemptStop(tui.KeyEnter, func(ke tui.KeyEvent) {
				// Enter only confirms while the decision pane owns focus. While
				// reading transcript history it is consumed as a no-op, preventing
				// an unrelated scrolling action from approving a tool call.
				if c.decisionScrollTarget.Get() == decisionScrollDetails {
					c.confirmPermissionSelection()
				}
			}),
			tui.OnPreemptStop(tui.KeyEscape, func(ke tui.KeyEvent) {
				c.escapeDecision()
			}),
			// Block all other keys when permission dialog is open
			tui.OnPreemptStop(tui.AnyKey, func(ke tui.KeyEvent) {}),
		)
		return km
	}

	// Global keys (outside permission dialog)
	km = append(km,
		tui.OnPreemptStop(tui.KeyTab, func(ke tui.KeyEvent) {
			if !c.hasSlashSuggestions() {
				return
			}
			c.acceptSlashSuggestion()
		}),
		tui.OnPreemptStop(tui.KeyEscape, func(ke tui.KeyEvent) {
			if c.hasSlashSuggestions() {
				c.dismissSlashSuggestions()
				return
			}
			if c.state.ExpandedView.Get() != "" {
				c.state.SetExpandedView("")
				return
			}
			if c.state.HasActiveQuery() && c.state.QueuedInputCount.Get() > 0 && c.onSteerQueued != nil {
				c.onSteerQueued()
				return
			}
			c.input.Focus()
		}),
		tui.OnPreemptStop(tui.KeyUp, func(ke tui.KeyEvent) {
			if c.state.ExpandedView.Get() == "activities" {
				c.moveActivityFocus(-1)
				return
			}
			if c.hasSlashSuggestions() {
				c.moveSlashSelection(-1)
				return
			}
			c.input.HandleEvent(ke)
		}),
		tui.OnPreemptStop(tui.KeyDown, func(ke tui.KeyEvent) {
			if c.state.ExpandedView.Get() == "activities" {
				c.moveActivityFocus(1)
				return
			}
			if c.hasSlashSuggestions() {
				c.moveSlashSelection(1)
				return
			}
			c.input.HandleEvent(ke)
		}),
		tui.OnPreemptStop(tui.KeyEnter, func(ke tui.KeyEvent) {
			if c.state.ExpandedView.Get() == "activities" {
				c.activateFocusedActivity()
				return
			}
			if !c.hasSlashSuggestions() {
				c.input.HandleEvent(ke)
				return
			}
			c.executeSlashSuggestion()
		}),
		tui.OnPreemptStop(tui.KeyBackspace, func(ke tui.KeyEvent) {
			c.input.HandleEvent(ke)
		}),
		tui.OnPreemptStop(tui.KeyDelete, func(ke tui.KeyEvent) {
			c.input.HandleEvent(ke)
		}),
		// Ctrl+C copies an active composer selection first. With no selection it
		// cancels active work. Idle exit always requires a second press within the
		// confirmation window.
		// Command+C is distinct and is handled below when the terminal reports the
		// Super modifier instead of consuming the shortcut itself.
		tui.On(tui.KeyCtrlC, func(ke tui.KeyEvent) {
			c.handleCtrlC(true)
		}),
		tui.On(tui.Rune('c').Super(), func(ke tui.KeyEvent) {
			c.copyInputSelection(false)
		}),
		tui.On(tui.Rune('x').Super(), func(ke tui.KeyEvent) {
			c.copyInputSelection(true)
		}),
		// Ctrl+D: exit (EOF) — always exits regardless of selection
		tui.On(tui.KeyCtrlD, func(ke tui.KeyEvent) {
			if c.app != nil {
				c.app.Stop()
			}
		}),

		// --- Copy to clipboard ---
		// Ctrl+G is the portable group toggle: every supported terminal encodes it
		// unambiguously. Keep Alt+G as an alias for terminals configured to send
		// Meta as ESC+g (or supporting the Kitty keyboard protocol). Both bindings
		// preempt and stop so a focused composer never receives the shortcut.
		tui.OnPreemptStop(tui.KeyCtrlG, func(ke tui.KeyEvent) {
			c.toggleFocusedOrLatestToolSegment()
		}),
		tui.OnPreemptStop(tui.Rune('g').Alt(), func(ke tui.KeyEvent) {
			c.toggleFocusedOrLatestToolSegment()
		}),
		// Ctrl+O cycles summary, detail, and exact evidence for the focused
		// observation (search and /detail establish that focus).
		tui.On(tui.Rune('o').Ctrl().Shift(), func(ke tui.KeyEvent) {
			c.toggleFocusedOrLatestToolSegment()
		}),
		tui.On(tui.Rune('o').Ctrl(), func(ke tui.KeyEvent) {
			focused := c.state.ActiveSessionInteraction().FocusedObservationID
			if focused == "" {
				return
			}
			_, _ = c.state.CycleObservationDisclosure(focused)
		}),
		// Alt+O toggles global evidence visibility without changing any
		// observation's local disclosure or pinned state.
		tui.On(tui.Rune('o').Alt(), func(ke tui.KeyEvent) {
			c.state.SetTranscriptShowAll(!c.state.TranscriptShowAll.Get())
		}),
		// Ctrl+Y: copy last assistant message to system clipboard.
		// Uses the platform clipboard command with OSC52 as a fallback.
		tui.On(tui.Rune('y').Ctrl(), func(ke tui.KeyEvent) { c.copyLastAssistant() }),
		// Ctrl+Shift+C copies an active input selection, otherwise the last
		// assistant message. It requires a terminal/protocol configuration that
		// distinguishes Ctrl+Shift+C from Ctrl+C.
		tui.On(tui.Rune('c').Ctrl().Shift(), func(ke tui.KeyEvent) {
			if c.copyInputSelection(false) {
				return
			}
			c.copyLastAssistant()
		}),

		// --- Image paste from clipboard ---
		// Ctrl+V on macOS/Linux, Alt+V on Windows — reads image from
		// clipboard and attaches to the next message. Text paste is
		// handled natively by the terminal (bracketed paste), so Ctrl+V
		// is free for image paste on macOS/Linux. On Windows, Ctrl+V is
		// the standard text paste key, so we use Alt+V instead.
		imagePasteBinding(func(ke tui.KeyEvent) {
			c.handleImagePaste()
		}),

		// --- Model picker (Alt+P / Meta+P) ---
		// Opens an overlay to switch provider/model from the ModelCatalog.
		// The picker is populated by the REPL layer that sets up OnSelect.
		tui.On(tui.Rune('p').Alt(), func(ke tui.KeyEvent) {
			if mp := c.state.ModelPicker.Get(); mp != nil && mp.Visible {
				return // already open
			}
			// Trigger is handled — the REPL layer sets up the picker entries
			// via AppState.ModelPicker. If no picker state is set (no catalog),
			// this is a no-op. We dispatch via a callback stored in the state.
			if c.openModelPicker != nil {
				c.openModelPicker()
			}
		}),

		// --- Language switching (Ctrl+L) ---
		// Cycles through supported display languages.
		tui.On(tui.KeyCtrlL, func(ke tui.KeyEvent) {
			if err := c.switchLanguage(c.state.Language.Get().Next()); err != nil {
				c.copyFeedback.Set(i18n.Text(c.state.Language.Get(), i18n.KeyLanguageUnavailable))
				c.scheduleCopyFeedbackClear()
			}
		}),

		// --- Mode switching (Shift+Tab) ---
		// Cycles through interaction modes: Auto → Ask → Plan → Auto.
		// Updates the mode state, triggers the callback that wires to
		// permissions.Checker and interaction.PlanState.
		tui.On(tui.KeyTab.Shift(), func(ke tui.KeyEvent) {
			cur := c.state.Mode.Get()
			next := cur.Next()
			c.state.Mode.Set(next)
			if c.onModeSwitch != nil {
				c.onModeSwitch(next)
			}
		}),

		// --- Scroll key bindings ---
		// Note: plain Up/Down are reserved for Phase 5 input history navigation.
		// Use Shift+Arrow for scrolling to avoid future conflicts.
		// Shift+Up / Shift+Down: scroll 1 line
		tui.On(tui.KeyUp.Shift(), func(ke tui.KeyEvent) { c.scrollBy(-1) }),
		tui.On(tui.KeyDown.Shift(), func(ke tui.KeyEvent) { c.scrollBy(1) }),
		// PageUp / PageDown: scroll by viewport height (minus 2 for overlap)
		tui.On(tui.KeyPageUp, func(ke tui.KeyEvent) {
			if c.state.ExpandedView.Get() == "activities" {
				c.moveActivityFocus(-7)
				return
			}
			c.pageTranscriptHistory(-1)
			c.scrollByPage(-1)
		}),
		tui.On(tui.KeyPageDown, func(ke tui.KeyEvent) {
			if c.state.ExpandedView.Get() == "activities" {
				c.moveActivityFocus(7)
				return
			}
			c.pageTranscriptHistory(1)
			c.scrollByPage(1)
		}),
		// Home: scroll to top
		tui.OnPreemptStop(tui.KeyHome.Ctrl(), func(ke tui.KeyEvent) {
			c.setHistoryStart(0)
			c.scrollToTop()
		}),
		tui.OnPreemptStop(tui.KeyEnd.Ctrl(), func(ke tui.KeyEvent) {
			c.setHistoryStart(-1)
			c.scrollToBottom()
		}),
		tui.On(tui.KeyHome, func(ke tui.KeyEvent) {
			c.setHistoryStart(0)
			c.scrollToTop()
		}),
		// End: scroll to bottom + re-enable follow
		tui.On(tui.KeyEnd, func(ke tui.KeyEvent) {
			c.setHistoryStart(-1)
			c.scrollToBottom()
		}),
	)
	if c.state.ExpandedView.Get() == "activities" {
		km = append(km,
			tui.OnPreemptStop(tui.Rune('c'), func(ke tui.KeyEvent) { c.runFocusedActivityAction(ActivityCancel) }),
			tui.OnPreemptStop(tui.Rune('d'), func(ke tui.KeyEvent) { c.runFocusedActivityAction(ActivityDetails) }),
			tui.OnPreemptStop(tui.Rune('g'), func(ke tui.KeyEvent) { c.runFocusedActivityAction(ActivityJump) }),
			tui.OnPreemptStop(tui.Rune('a'), func(ke tui.KeyEvent) { c.runFocusedActivityAction(ActivityAcknowledge) }),
		)
	}

	return km
}

func (c *RootComponent) moveActivityFocus(delta int) {
	activities := c.state.ActivitySnapshot().Activities
	if len(activities) == 0 {
		return
	}
	index := 0
	current := c.state.ActivityFocus.Get()
	for i := range activities {
		if activities[i].ID == current {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(activities) {
		index = len(activities) - 1
	}
	c.state.ActivityFocus.Set(activities[index].ID)
	offset := c.state.ActivityViewOffset.Get()
	if index < offset {
		offset = index
	} else if index >= offset+7 {
		offset = index - 6
	}
	c.state.ActivityViewOffset.Set(offset)
}

func (c *RootComponent) activateFocusedActivity() {
	activity, ok := c.state.GetActivity(c.state.ActivityFocus.Get())
	if !ok {
		return
	}
	for _, preferred := range []ActivityAction{ActivityDetails, ActivityJump} {
		for _, action := range activity.Actions {
			if action == preferred {
				c.runFocusedActivityAction(preferred)
				return
			}
		}
	}
}

func (c *RootComponent) runFocusedActivityAction(action ActivityAction) {
	if c.onActivityAction == nil {
		return
	}
	id := c.state.ActivityFocus.Get()
	activity, ok := c.state.GetActivity(id)
	if !ok {
		return
	}
	for _, available := range activity.Actions {
		if available == action {
			c.onActivityAction(id, action)
			return
		}
	}
}

func (c *RootComponent) pageTranscriptHistory(direction int) {
	messages := c.state.Messages.Get()
	if len(messages) == 0 {
		return
	}
	viewport := c.termHeight
	if viewport < 1 {
		viewport = 24
	}
	budget := viewport*4 + 32
	start := c.historyStart.Get()
	if start < 0 {
		start = len(messages) - budget
	}
	start += direction * (budget / 2)
	if start <= 0 {
		start = 0
	}
	if start+budget >= len(messages) {
		start = -1
	}
	c.setHistoryStart(start)
}

// Watchers returns background state observers.
func (c *RootComponent) Watchers() []tui.Watcher {
	return []tui.Watcher{
		// Match Codex's left-to-right working shimmer and refresh its wall-clock
		// duration while a model turn is active. Idle ticks do not enter the UI queue.
		llmWorkingShimmerWatcher{root: c},
		// Auto-scroll to bottom when new messages arrive (if following).
		// Uses scrollToBottom() which queries the real maxY from the Ref
		// element, avoiding the math.MaxInt sentinel problem.
		tui.OnChange(c.state.Messages, func(msgs []Message) {
			if c.stickToBottom.Get() {
				c.setHistoryStart(-1)
				c.scrollToBottom()
			}
		}),
		tui.OnChange(c.inputText, func(value string) {
			c.state.SetInteractionEditor(value, c.inputCursor.Get())
			c.refreshSlashSuggestions(value)
		}),
		tui.OnChange(c.inputCursor, func(cursor int) {
			c.state.SetInteractionCursor(cursor)
		}),
		tui.OnChange(c.scrollY, func(offset int) {
			c.state.SetInteractionScroll(offset)
		}),
		tui.OnChange(c.state.InteractionRevision, func(uint64) {
			c.syncSessionViewFromState()
		}),
		tui.OnChange(c.state.DecisionReq, func(request *DecisionRequest) {
			c.decisionScroll.Set(0)
			c.decisionScrollTarget.Set(decisionScrollDetails)
			if request == nil || request.Kind != permissions.PromptKindAskUser {
				c.state.AskUserDraft.Set(nil)
				return
			}
			if draft := c.state.AskUserDraft.Get(); draft != nil && draft.DecisionID != request.DecisionID {
				c.state.AskUserDraft.Set(nil)
			}
		}),
	}
}

func (c *RootComponent) hasSlashSuggestions() bool {
	state := c.slash.Get()
	return state != nil && len(state.Items) > 0
}

// hasPickerOverlay returns true when a modal overlay is open. It suppresses
// TextArea focus handlers so preemptive modal handlers receive navigation and
// confirmation keys first.
func (c *RootComponent) hasPickerOverlay() bool {
	if c.activeDecisionRequest() != nil {
		return true
	}
	if mp := c.state.ModelPicker.Get(); mp != nil && mp.Visible {
		return true
	}
	if sp := c.state.SessionPicker.Get(); sp != nil && sp.Visible {
		return true
	}
	if fp := c.state.ForkPicker.Get(); fp != nil && fp.Visible {
		return true
	}
	if menu := c.state.SkillsMenu.Get(); menu != nil && menu.Visible {
		return true
	}
	if c.state.ExpandedView.Get() == "activities" {
		return true
	}
	return false
}

func (c *RootComponent) refreshSlashSuggestions(value string) {
	if value != c.slashDismissedForInput {
		c.slashDismissedForInput = ""
	}
	if c.slashDismissedForInput != "" && value == c.slashDismissedForInput {
		return
	}
	c.slash.Set(computeSlashSuggestions(value, c.slashCommands, c.state.Language.Get()))
	c.persistSlashInteraction()
}

func (c *RootComponent) dismissSlashSuggestions() {
	c.slashDismissedForInput = c.inputText.Get()
	c.slash.Set(nil)
	c.persistSlashInteraction()
}

func (c *RootComponent) moveSlashSelection(delta int) {
	state := c.slash.Get()
	if state == nil || len(state.Items) == 0 {
		return
	}
	next := (state.Selected + delta) % len(state.Items)
	if next < 0 {
		next += len(state.Items)
	}
	state.Selected = next
	c.slash.Set(state)
	c.persistSlashInteraction()
}

func (c *RootComponent) persistSlashInteraction() {
	if c == nil || c.state == nil {
		return
	}
	selected, selectedSet := 0, false
	if suggestions := c.slash.Get(); suggestions != nil && len(suggestions.Items) > 0 {
		selected, selectedSet = suggestions.Selected, true
	}
	c.state.SetInteractionSlash(selected, selectedSet, c.slashDismissedForInput)
}

func (c *RootComponent) selectedSlashSuggestion() (SlashCommandEntry, bool) {
	state := c.slash.Get()
	if state == nil || len(state.Items) == 0 {
		return SlashCommandEntry{}, false
	}
	if state.Selected < 0 || state.Selected >= len(state.Items) {
		return SlashCommandEntry{}, false
	}
	return state.Items[state.Selected], true
}

func (c *RootComponent) acceptSlashSuggestion() {
	entry, ok := c.selectedSlashSuggestion()
	if !ok {
		return
	}
	if entry.OpensSubmenu {
		c.input.SetText("/" + entry.Name + " ")
		c.refreshSlashSuggestions(c.inputText.Get())
		return
	}
	c.input.SetText(slashCommandInput(entry))
	c.slash.Set(nil)
	c.slashDismissedForInput = ""
	c.persistSlashInteraction()
}

func (c *RootComponent) executeSlashSuggestion() {
	entry, ok := c.selectedSlashSuggestion()
	if !ok {
		return
	}
	if entry.OpensSubmenu {
		c.input.SetText("/" + entry.Name + " ")
		c.refreshSlashSuggestions(c.inputText.Get())
		return
	}
	input := slashCommandInput(entry)
	c.input.SetText(input)
	c.slash.Set(nil)
	c.slashDismissedForInput = ""
	c.persistSlashInteraction()
	c.submitInput(input)
}

func slashCommandInput(entry SlashCommandEntry) string {
	if entry.Input != "" {
		return entry.Input
	}
	return formatSlashCommandInput(entry.Name)
}
