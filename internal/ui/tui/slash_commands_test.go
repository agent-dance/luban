package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func TestComputeSlashSuggestionsEmptyQueryPreservesOrder(t *testing.T) {
	commands := []SlashCommandEntry{
		{Name: "exit"},
		{Name: "compact"},
		{Name: "model"},
	}

	state := computeSlashSuggestions("/", commands, i18n.LangEN)
	if state == nil {
		t.Fatal("expected suggestions for bare slash")
	}
	if len(state.Items) != len(commands) {
		t.Fatalf("len(items) = %d, want %d", len(state.Items), len(commands))
	}
	for i, cmd := range commands {
		if got := state.Items[i].Name; got != cmd.Name {
			t.Fatalf("items[%d] = %q, want %q", i, got, cmd.Name)
		}
	}
}

func TestComputeSlashSuggestionsRanksPrefixAndAliasMatches(t *testing.T) {
	commands := []SlashCommandEntry{
		{Name: "compact", Description: "compact context"},
		{Name: "config", Description: "configuration"},
		{Name: "exit", Aliases: []string{"quit"}, Description: "leave"},
	}

	state := computeSlashSuggestions("/qu", commands, i18n.LangEN)
	if state == nil {
		t.Fatal("expected suggestions for alias match")
	}
	if got := state.Items[0].Name; got != "exit" {
		t.Fatalf("first suggestion = %q, want %q", got, "exit")
	}

	state = computeSlashSuggestions("/comp", commands, i18n.LangEN)
	if state == nil {
		t.Fatal("expected suggestions for prefix match")
	}
	if got := state.Items[0].Name; got != "compact" {
		t.Fatalf("first suggestion = %q, want %q", got, "compact")
	}
}

func TestComputeSlashSuggestionsIncludesRegisteredGoalCommand(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)

	entries := make([]SlashCommandEntry, 0, len(registry.All()))
	for _, command := range registry.All() {
		entries = append(entries, SlashCommandEntry{
			Name:        command.Name(),
			Aliases:     command.Aliases(),
			Description: command.Description(),
		})
	}

	suggestions := computeSlashSuggestions("/go", entries, i18n.LangEN)
	if suggestions == nil || len(suggestions.Items) == 0 {
		t.Fatal("registered /goal command was absent from slash suggestions")
	}
	got := suggestions.Items[0]
	if got.Name != "goal" {
		t.Fatalf("first /go suggestion = %q, want goal", got.Name)
	}
	if got.Description == "" {
		t.Fatal("registered /goal suggestion has an empty description")
	}
}

func TestComputeSlashSuggestionsHideWhenArgumentsStart(t *testing.T) {
	commands := []SlashCommandEntry{{Name: "model"}}
	if got := computeSlashSuggestions("/model sonnet", commands, i18n.LangEN); got != nil {
		t.Fatal("expected no suggestions once arguments are present")
	}
	if got := computeSlashSuggestions("/model ", commands, i18n.LangEN); got != nil {
		t.Fatal("expected no suggestions after trailing space")
	}
}

func TestLanguageSubmenuUsesEachLanguageNativeName(t *testing.T) {
	state := computeSlashSuggestions("/language ", nil, i18n.LangDE)
	if state == nil || len(state.Items) != len(i18n.AllLanguages()) {
		t.Fatalf("language items = %#v", state)
	}

	for i, language := range i18n.AllLanguages() {
		item := state.Items[i]
		if got := slashCommandDisplayLabel(item); got != language.String() {
			t.Fatalf("item[%d] = %q, want native name %q", i, got, language.String())
		}
		if want := "/language " + language.Code(); item.Input != want {
			t.Fatalf("item[%d] input = %q, want %q", i, item.Input, want)
		}
	}
}

func TestLanguageCommandSelectionOpensSubmenu(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, []SlashCommandEntry{{Name: "language", OpensSubmenu: true}})
	root.slash.Set(&slashSuggestionsState{Items: []SlashCommandEntry{{Name: "language", OpensSubmenu: true}}})

	root.executeSlashSuggestion()

	if got := root.inputText.Get(); got != "/language " {
		t.Fatalf("input = %q, want language submenu trigger", got)
	}
	if suggestions := root.slash.Get(); suggestions == nil || len(suggestions.Items) != len(i18n.AllLanguages()) {
		t.Fatalf("language submenu = %#v", suggestions)
	}
}

func TestLanguageSubmenuSelectionSubmitsLanguageCommand(t *testing.T) {
	var submitted string
	root := NewRootComponent(NewAppState(), func(input string) {
		submitted = input
	}, nil)
	root.slash.Set(computeLanguageSuggestions("zh"))

	root.executeSlashSuggestion()

	if submitted != "/language zh" {
		t.Fatalf("submitted = %q, want /language zh", submitted)
	}
	if got := root.slash.Get(); got != nil {
		t.Fatalf("slash state = %#v, want nil after selection", got)
	}
}

func TestLanguageSubmenuHasLocalizedNavigationAndNativeOptions(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	root := NewRootComponent(state, nil, nil)
	menu := computeSlashSuggestions("/language ", nil, i18n.LangZH)

	text := collectElementText(root.renderSlashSuggestions(menu))
	if !strings.Contains(text, "显示语言") {
		t.Fatalf("language submenu header = %q, want Chinese navigation text", text)
	}
	for _, want := range []string{"English", "中文", "Deutsch", "日本語", "한국어"} {
		if !strings.Contains(text, want) {
			t.Fatalf("language submenu = %q, missing native option %q", text, want)
		}
	}
	menu.Selected = len(menu.Items) - 1
	if text := collectElementText(root.renderSlashSuggestions(menu)); !strings.Contains(text, "Русский") {
		t.Fatalf("scrolled language submenu = %q, missing native option Русский", text)
	}
}

func TestExecuteSlashSuggestionSubmitsFormattedCommand(t *testing.T) {
	var submitted string
	root := NewRootComponent(NewAppState(), func(input string) {
		submitted = input
	}, []SlashCommandEntry{{Name: "compact"}})

	root.slash.Set(&slashSuggestionsState{
		Items:    []SlashCommandEntry{{Name: "compact"}},
		Selected: 0,
	})

	root.executeSlashSuggestion()

	if submitted != "/compact " {
		t.Fatalf("submitted = %q, want %q", submitted, "/compact ")
	}
	if got := root.inputText.Get(); got != "" {
		t.Fatalf("input text = %q, want cleared", got)
	}
	if got := root.slash.Get(); got != nil {
		t.Fatalf("slash state = %#v, want nil", got)
	}
}

func TestAcceptSlashSuggestionCompletesInputWithoutSubmitting(t *testing.T) {
	var submitted string
	root := NewRootComponent(NewAppState(), func(input string) {
		submitted = input
	}, []SlashCommandEntry{{Name: "compact"}})

	root.slash.Set(&slashSuggestionsState{
		Items:    []SlashCommandEntry{{Name: "compact"}},
		Selected: 0,
	})

	root.acceptSlashSuggestion()

	if submitted != "" {
		t.Fatalf("submitted = %q, want empty", submitted)
	}
	if got := root.inputText.Get(); got != "/compact " {
		t.Fatalf("input text = %q, want %q", got, "/compact ")
	}
}

func TestMoveSlashSelectionWrapsWithArrowNavigation(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, []SlashCommandEntry{
		{Name: "compact"},
		{Name: "config"},
		{Name: "exit"},
	})

	root.slash.Set(&slashSuggestionsState{
		Items: []SlashCommandEntry{
			{Name: "compact"},
			{Name: "config"},
			{Name: "exit"},
		},
		Selected: 0,
	})

	root.moveSlashSelection(1)
	if got := root.slash.Get().Selected; got != 1 {
		t.Fatalf("selected after down = %d, want 1", got)
	}

	root.moveSlashSelection(1)
	if got := root.slash.Get().Selected; got != 2 {
		t.Fatalf("selected after second down = %d, want 2", got)
	}

	root.moveSlashSelection(1)
	if got := root.slash.Get().Selected; got != 0 {
		t.Fatalf("selected after wrap-down = %d, want 0", got)
	}

	root.moveSlashSelection(-1)
	if got := root.slash.Get().Selected; got != 2 {
		t.Fatalf("selected after wrap-up = %d, want 2", got)
	}
}

func TestDismissSlashSuggestionsStaysDismissedUntilInputChanges(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, []SlashCommandEntry{{Name: "compact"}})

	root.refreshSlashSuggestions("/")
	if !root.hasSlashSuggestions() {
		t.Fatal("expected suggestions before dismiss")
	}

	root.inputText.Set("/")
	root.dismissSlashSuggestions()
	if root.hasSlashSuggestions() {
		t.Fatal("expected suggestions to be dismissed")
	}

	root.refreshSlashSuggestions("/")
	if root.hasSlashSuggestions() {
		t.Fatal("expected dismissed suggestions to stay hidden for same input")
	}

	root.refreshSlashSuggestions("/c")
	if !root.hasSlashSuggestions() {
		t.Fatal("expected suggestions to return after input changes")
	}
}

func TestSlashAwareTextAreaArrowKeysMoveSuggestions(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, []SlashCommandEntry{
		{Name: "compact"},
		{Name: "config"},
		{Name: "exit"},
	})

	root.slash.Set(&slashSuggestionsState{
		Items: []SlashCommandEntry{
			{Name: "compact"},
			{Name: "config"},
			{Name: "exit"},
		},
		Selected: 0,
	})
	root.input.Focus()

	if stopped := root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyDown}); !stopped {
		t.Fatal("expected KeyDown to be consumed")
	}
	if got := root.slash.Get().Selected; got != 1 {
		t.Fatalf("selected after down = %d, want 1", got)
	}

	if stopped := root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyUp}); !stopped {
		t.Fatal("expected KeyUp to be consumed")
	}
	if got := root.slash.Get().Selected; got != 0 {
		t.Fatalf("selected after up = %d, want 0", got)
	}
}

func TestSlashAwareTextAreaEnterExecutesSuggestion(t *testing.T) {
	var submitted string
	root := NewRootComponent(NewAppState(), func(input string) {
		submitted = input
	}, []SlashCommandEntry{{Name: "compact"}})

	root.slash.Set(&slashSuggestionsState{
		Items:    []SlashCommandEntry{{Name: "compact"}},
		Selected: 0,
	})
	root.input.Focus()

	if stopped := root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEnter}); !stopped {
		t.Fatal("expected KeyEnter to be consumed")
	}
	if submitted != "/compact " {
		t.Fatalf("submitted = %q, want %q", submitted, "/compact ")
	}
	if got := root.slash.Get(); got != nil {
		t.Fatalf("slash state = %#v, want nil", got)
	}
}

func TestSlashAwareTextAreaEscapeDismissesSuggestions(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, []SlashCommandEntry{{Name: "compact"}})

	root.inputText.Set("/")
	root.slash.Set(&slashSuggestionsState{
		Items:    []SlashCommandEntry{{Name: "compact"}},
		Selected: 0,
	})
	root.input.Focus()

	if stopped := root.input.HandleEvent(gtui.KeyEvent{Key: gtui.KeyEscape}); !stopped {
		t.Fatal("expected KeyEscape to be consumed")
	}
	if root.hasSlashSuggestions() {
		t.Fatal("expected suggestions to be dismissed")
	}
	if got := root.slashDismissedForInput; got != "/" {
		t.Fatalf("dismissedForInput = %q, want %q", got, "/")
	}
}

func TestSlashSuggestionsWindowScrollsAroundSelection(t *testing.T) {
	state := &slashSuggestionsState{
		Items: []SlashCommandEntry{
			{Name: "cmd0"},
			{Name: "cmd1"},
			{Name: "cmd2"},
			{Name: "cmd3"},
			{Name: "cmd4"},
			{Name: "cmd5"},
			{Name: "cmd6"},
		},
		Selected: 6,
	}

	start, visible := slashSuggestionsWindow(state)
	if start != 2 {
		t.Fatalf("start = %d, want 2", start)
	}
	if len(visible) != maxVisibleSlashSuggestions {
		t.Fatalf("len(visible) = %d, want %d", len(visible), maxVisibleSlashSuggestions)
	}
	if got := visible[len(visible)-1].Name; got != "cmd6" {
		t.Fatalf("last visible = %q, want cmd6", got)
	}
}

func TestSlashCommandColumnWidthUsesLongestVisibleLabel(t *testing.T) {
	items := []SlashCommandEntry{
		{Name: "x", Aliases: []string{"short"}},
		{Name: "much-longer-command"},
	}

	width := slashCommandColumnWidth(items)
	want := terminalCellWidth("/much-longer-command")
	if width != want {
		t.Fatalf("column width = %d, want %d", width, want)
	}
	if got := terminalCellWidth(padRightCells("/x  (short)", width)); got != width {
		t.Fatalf("padded width = %d, want %d", got, width)
	}
}
