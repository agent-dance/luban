package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/grindlemire/go-tui"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/ui/theme"
	"github.com/agent-dance/luban/provider"
)

// App wraps go-tui's App and provides the complete TUI application lifecycle.
// It holds the state, root component, renderer, and provides Start/Stop.
type App struct {
	tuiApp    *tui.App
	state     *AppState
	root      *RootComponent
	renderer  *TuiRenderer
	running   atomic.Bool
	lifecycle sync.Mutex
}

var _ SkillsMenuLauncher = (*App)(nil)

// NewTUIAppWithAdmission requires synchronous acceptance before the composer
// is cleared. The interactive runtime uses it to reserve foreground work or
// explicitly admit the submission into its visible queue.
func NewTUIAppWithAdmission(trySubmit func(string) bool, providerName, model string, catalog *provider.ModelCatalog, slashCommands []SlashCommandEntry) (*App, error) {
	return newTUIApp(trySubmit, providerName, model, catalog, slashCommands)
}

func newTUIApp(trySubmit func(string) bool, providerName, model string, catalog *provider.ModelCatalog, slashCommands []SlashCommandEntry) (*App, error) {
	applyTerminalTheme()
	state := NewAppState()

	// Set initial banner values directly — safe before Run() starts.
	state.Provider.Set(providerName)
	state.Model.Set(model)

	// Enrich initial banner with catalog data if available
	if catalog != nil {
		if info, ok := catalog.ResolveForProvider(providerName, model); ok {
			if info.ContextWindow > 0 {
				state.ContextWindowK.Set(fmtContextWindow(info.ContextWindow))
				// Also set MaxTokens for the context bar so its initial value
				// reflects the actual model, not the hardcoded 200K default.
				state.MaxTokens.Set(modelContextCapacity(info))
			}
			state.ModelCostIn.Set(info.CostPer1MIn)
			state.ModelCostOut.Set(info.CostPer1MOut)
			state.ModelCostCurrency.Set(info.BillingCurrency())
			state.ModelCanSeeImages.Set(info.CanSeeImages)
		}
	}

	root := NewRootComponentWithAdmission(state, trySubmit, slashCommands)

	// Create go-tui app in fullscreen mode with mouse reporting enabled so
	// scrolling and mouse controls work out of the box. Terminal-native text
	// selection remains available through the terminal's modifier-drag gesture.
	// Set the detected system language
	state.Language.Set(i18n.DetectOrLoadLanguage())

	tuiApp, err := tui.NewApp(defaultTUIAppOptions(root)...)
	if err != nil {
		return nil, err
	}
	renderer := NewTuiRenderer(tuiApp, state, catalog)

	return &App{
		tuiApp:   tuiApp,
		state:    state,
		root:     root,
		renderer: renderer,
	}, nil
}

func modelContextCapacity(info provider.ModelInfo) int {
	return max(info.ContextWindow, 0)
}

func defaultTUIAppOptions(root *RootComponent) []tui.AppOption {
	return []tui.AppOption{
		tui.WithRootComponent(root),
		tui.WithMouse(),
	}
}

func applyTerminalTheme() {
	p := theme.Current()
	color := func(hex string) tui.Color {
		value, err := tui.HexColor(hex)
		if err != nil {
			return tui.DefaultColor()
		}
		return value
	}

	background := color(p.Background)
	foreground := color(p.Foreground)
	muted := color(p.Muted)
	accent := color(p.Accent)
	success := color(p.Success)
	warning := color(p.Warning)
	danger := color(p.Danger)
	tui.SetPalette(tui.Palette{
		Black: background, Red: danger, Green: success, Yellow: warning,
		Blue: accent, Magenta: accent, Cyan: accent, White: foreground,
		BrightBlack: muted, BrightRed: danger, BrightGreen: success, BrightYellow: warning,
		BrightBlue: accent, BrightMagenta: accent, BrightCyan: accent, BrightWhite: foreground,
	})
}

// Renderer returns the presentation.Renderer implementation backed by this TUI.
func (a *App) Renderer() *TuiRenderer {
	return a.renderer
}

// State returns the reactive application state.
func (a *App) State() *AppState {
	return a.state
}

// ApplySessionSnapshot publishes durable state and synchronizes Root-owned
// mirrors (input draft, history anchor, and scroll position) in one caller
// boundary. This also covers startup, where the snapshot is applied before
// OnChange watchers begin running.
func (a *App) ApplySessionSnapshot(snapshot SessionSnapshot) error {
	if a == nil || a.state == nil || a.root == nil {
		return i18n.NewError(i18n.KeyTUISessionSnapshotEmptySessionID)
	}
	if err := a.state.ApplySessionSnapshot(snapshot); err != nil {
		return err
	}
	a.root.invalidateSessionActionMap()
	a.root.syncSessionViewFromState()
	return nil
}

// GoTuiApp returns the underlying go-tui App for advanced usage.
func (a *App) GoTuiApp() *tui.App {
	return a.tuiApp
}

// SetOpenModelPicker registers a callback that fires when the user presses
// Alt+P. The callback should populate AppState.ModelPicker with the model
// catalog entries and show the picker overlay.
func (a *App) SetOpenModelPicker(fn func()) {
	a.root.openModelPicker = fn
}

// OpenSkillsMenu implements SkillsMenuLauncher. Exact /skills routing may run
// on a REPL worker goroutine, so publication is synchronized onto the TUI
// event loop. Opening the surface immediately starts its authoritative catalog
// read; there is no intermediate action menu.
func (a *App) OpenSkillsMenu(request SkillsMenuOpenRequest) error {
	if a == nil || a.state == nil || a.root == nil {
		return ErrSkillsMenuLauncherUnavailable
	}
	if !a.UpdateSync(func() {
		a.root.openSkillsMenu(newSkillsMenuState(request))
	}) {
		return ErrSkillsMenuLauncherUnavailable
	}
	return nil
}

// SetOnModeSwitch registers a callback that fires when the user presses
// Shift+Tab to cycle interaction modes. The callback receives the new mode
// and should wire it to permissions.Checker and interaction.PlanState.
func (a *App) SetOnModeSwitch(fn func(InteractionMode)) {
	a.root.onModeSwitch = fn
}

// SwitchLanguage durably changes the process-wide active language and then
// publishes the same language to AppState. A persistence failure leaves both
// values unchanged.
func (a *App) SwitchLanguage(lang i18n.Language) error {
	if a == nil || a.root == nil {
		return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLanguageUnavailable))
	}
	return a.root.switchLanguage(lang)
}

// SetOnLanguageSwitch registers an observer that fires after a successful
// language transaction. It is not responsible for persistence.
func (a *App) SetOnLanguageSwitch(fn func(i18n.Language)) {
	a.root.onLanguageSwitch = fn
}

func (a *App) SetOnActivityAction(fn func(string, ActivityAction)) {
	a.root.onActivityAction = fn
}

// SetOnSteerQueued registers the Escape action used while work and queued
// composer submissions are both present.
func (a *App) SetOnSteerQueued(fn func()) {
	a.root.onSteerQueued = fn
}

// SetBuildDiagnostic retains the immutable process identity and repository
// comparison without exposing either in the persistent terminal header.
func (a *App) SetBuildDiagnostic(diagnostic buildinfo.Diagnostic) bool {
	if a == nil || a.root == nil {
		return false
	}
	return a.UpdateSync(func() { a.root.build = diagnostic })
}

// Run starts the TUI event loop. Blocks until Stop() is called or Ctrl+C.
func (a *App) Run() error {
	a.lifecycle.Lock()
	a.running.Store(true)
	a.lifecycle.Unlock()
	defer func() {
		a.lifecycle.Lock()
		a.running.Store(false)
		a.lifecycle.Unlock()
	}()
	return a.tuiApp.Run()
}

type syncUpdateOutcome struct {
	state atomic.Uint32 // 0 pending, 1 callback owns mutation, 2 stop owns cancellation
	done  chan struct{}
}

func newSyncUpdateOutcome() *syncUpdateOutcome {
	return &syncUpdateOutcome{done: make(chan struct{})}
}

func (o *syncUpdateOutcome) run(fn func()) {
	if !o.state.CompareAndSwap(0, 1) {
		close(o.done)
		return
	}
	fn()
	close(o.done)
}

func (o *syncUpdateOutcome) stopOrWait() bool {
	if o.state.CompareAndSwap(0, 2) {
		return false
	}
	if o.state.Load() == 1 {
		<-o.done
		return true
	}
	return false
}

func (a *App) UpdateSync(fn func()) bool {
	if fn == nil {
		return true
	}
	a.lifecycle.Lock()
	if !a.running.Load() {
		defer a.lifecycle.Unlock()
		select {
		case <-a.tuiApp.StopCh():
			return false
		default:
		}
		a.tuiApp.Batch(fn)
		return true
	}
	a.lifecycle.Unlock()
	outcome := newSyncUpdateOutcome()
	if !a.tuiApp.QueueUpdateLossless(func() {
		outcome.run(func() {
			a.tuiApp.Batch(fn)
		})
	}) {
		return false
	}
	select {
	case <-outcome.done:
		return outcome.state.Load() == 1
	case <-a.tuiApp.StopCh():
		return outcome.stopOrWait()
	}
}

// Stop gracefully shuts down the TUI.
// Signals all blocked goroutines (e.g. PermissionRequest) before stopping.
func (a *App) Stop() {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	a.state.SignalStop()
	a.tuiApp.Stop()
}

// Close restores terminal state. Must be called after Run returns.
func (a *App) Close() error {
	return a.tuiApp.Close()
}

func (a *App) SetMouseEnabled(enabled bool) {
	a.tuiApp.SetMouseEnabled(enabled)
}

func (a *App) MouseEnabled() bool {
	return a.tuiApp.MouseEnabled()
}

// OpenFileInEditor releases fullscreen terminal ownership while the selected
// editor runs, then forces a full redraw on return.
func (a *App) OpenFileInEditor(path string) error {
	command, args, err := editorCommandInLanguage(a.state.Language.Get(), path)
	if err != nil {
		return err
	}
	return a.tuiApp.RunExternal(func() error {
		cmd := exec.Command(command, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	})
}

func editorCommandInLanguage(lang i18n.Language, path string) (string, []string, error) {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return "", nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyRuntimeEditorEnvMissing))
	}
	parts, err := splitEditorCommandInLanguage(lang, editor)
	if err != nil {
		return "", nil, err
	}
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyRuntimeEditorCommandEmpty))
	}
	return parts[0], append(parts[1:], path), nil
}

func splitEditorCommandInLanguage(lang i18n.Language, command string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped := false
	haveWord := false
	flush := func() {
		if haveWord {
			words = append(words, word.String())
			word.Reset()
			haveWord = false
		}
	}
	for _, char := range command {
		if escaped {
			if char != '\\' && char != '\'' && char != '"' && char != ' ' && char != '\t' && char != '\n' && char != '\r' {
				word.WriteRune('\\')
			}
			word.WriteRune(char)
			haveWord = true
			escaped = false
			continue
		}
		if quote == '\'' {
			if char == '\'' {
				quote = 0
			} else {
				word.WriteRune(char)
				haveWord = true
			}
			continue
		}
		if char == '\\' {
			escaped = true
			haveWord = true
			continue
		}
		if quote == '"' {
			if char == '"' {
				quote = 0
			} else {
				word.WriteRune(char)
				haveWord = true
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
			haveWord = true
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			word.WriteRune(char)
			haveWord = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyRuntimeEditorIncompleteEscape))
	}
	if quote != 0 {
		return nil, fmt.Errorf("%s", i18n.Text(lang, i18n.KeyRuntimeEditorUnclosedQuote))
	}
	flush()
	return words, nil
}
