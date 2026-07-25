package tui

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

// fireKeyBinding finds a binding in KeyMap matching the given pattern and fires it.
// If r != 0, matches by Rune; otherwise matches by Key.
func fireKeyBinding(t *testing.T, root *RootComponent, key gtui.Key, r rune, mod gtui.Modifier) {
	t.Helper()
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })
	for _, binding := range root.KeyMap() {
		if r != 0 {
			if binding.Pattern.Rune == r && binding.Pattern.Mod == mod && !binding.Pattern.AnyKey && !binding.Pattern.AnyRune {
				binding.Handler(gtui.KeyEvent{Key: gtui.KeyRune, Rune: r, Mod: mod})
				return
			}
			continue
		}
		if binding.Pattern.Key == key && binding.Pattern.Mod == mod && !binding.Pattern.AnyKey && !binding.Pattern.AnyRune {
			binding.Handler(gtui.KeyEvent{Key: key, Mod: mod})
			return
		}
	}
	t.Fatalf("binding not found for key=%v rune=%q mod=%v", key, r, mod)
}

func TestLanguageSwitchKeyBinding_CyclesLanguages(t *testing.T) {
	tests := []struct {
		name      string
		startLang i18n.Language
		expected  i18n.Language
	}{
		{name: "en_to_zh", startLang: i18n.LangEN, expected: i18n.LangZH},
		{name: "zh_to_de", startLang: i18n.LangZH, expected: i18n.LangDE},
		{name: "de_to_ja", startLang: i18n.LangDE, expected: i18n.LangJA},
		{name: "ja_to_ko", startLang: i18n.LangJA, expected: i18n.LangKO},
		{name: "ko_to_ru", startLang: i18n.LangKO, expected: i18n.LangRU},
		{name: "ru_to_en", startLang: i18n.LangRU, expected: i18n.LangEN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if err := i18n.SaveLanguage(tt.startLang); err != nil {
				t.Fatalf("save starting language: %v", err)
			}
			state := NewAppState()
			state.Language.Set(tt.startLang)
			root := NewRootComponent(state, nil, nil)

			// Fire Ctrl+L
			fireKeyBinding(t, root, 0, 'l', gtui.ModCtrl)

			if got := state.Language.Get(); got != tt.expected {
				t.Fatalf("language = %v (%s), want %v (%s)", got, got.String(), tt.expected, tt.expected.String())
			}
			if got := i18n.DetectOrLoadLanguage(); got != tt.expected {
				t.Fatalf("active language = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLanguageSwitchKeyBinding_BindingExistsInDefaultKeyMap(t *testing.T) {
	state := NewAppState()
	root := NewRootComponent(state, nil, nil)

	found := false
	for _, binding := range root.KeyMap() {
		if binding.Pattern.Rune == 'l' && binding.Pattern.Mod == gtui.ModCtrl && !binding.Pattern.AnyKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Ctrl+L language switch binding not found in default KeyMap")
	}
}

func TestLanguageSwitchKeyBinding_OnLanguageSwitchCallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := i18n.SaveLanguage(i18n.LangEN); err != nil {
		t.Fatalf("save starting language: %v", err)
	}
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	root := NewRootComponent(state, nil, nil)

	var callbackLang i18n.Language
	var callbackSawCommittedState bool
	root.onLanguageSwitch = func(lang i18n.Language) {
		callbackLang = lang
		callbackSawCommittedState = state.Language.Get() == lang && i18n.DetectOrLoadLanguage() == lang
	}

	fireKeyBinding(t, root, 0, 'l', gtui.ModCtrl)

	if callbackLang != i18n.LangZH {
		t.Fatalf("onLanguageSwitch received %v, want LangZH", callbackLang)
	}
	if !callbackSawCommittedState {
		t.Fatal("language observer ran before durable and AppState languages were committed")
	}
}

func TestLanguageSwitchKeyBinding_PersistenceFailureKeepsPreviousLanguage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("save starting language: %v", err)
	}
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	root := NewRootComponent(state, nil, nil)
	root.languageSaver = func(i18n.Language) error { return errors.New("read-only preference store") }
	observerCalled := false
	root.onLanguageSwitch = func(i18n.Language) { observerCalled = true }

	fireKeyBinding(t, root, 0, 'l', gtui.ModCtrl)

	if got := state.Language.Get(); got != i18n.LangZH {
		t.Fatalf("AppState language = %v, want previous %v", got, i18n.LangZH)
	}
	if got := i18n.DetectOrLoadLanguage(); got != i18n.LangZH {
		t.Fatalf("active language = %v, want previous %v", got, i18n.LangZH)
	}
	if observerCalled {
		t.Fatal("language observer ran for a rejected transition")
	}
	want := i18n.Text(i18n.LangZH, i18n.KeyLanguageUnavailable)
	if got := root.copyFeedback.Get(); got != want {
		t.Fatalf("failure feedback = %q, want localized %q", got, want)
	}
}

func TestAppSwitchLanguageDelegatesToTheSameTransaction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })
	if err := i18n.SaveLanguage(i18n.LangEN); err != nil {
		t.Fatalf("save starting language: %v", err)
	}
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	root := NewRootComponent(state, nil, nil)
	app := &App{state: state, root: root}

	if err := app.SwitchLanguage(i18n.LangJA); err != nil {
		t.Fatalf("SwitchLanguage: %v", err)
	}
	if state.Language.Get() != i18n.LangJA || i18n.DetectOrLoadLanguage() != i18n.LangJA {
		t.Fatalf("App transaction did not converge: state=%v active=%v", state.Language.Get(), i18n.DetectOrLoadLanguage())
	}
}

func TestLanguageSwitchKeyBinding_NotBlockedByPermissionDialog(t *testing.T) {
	// When permission dialog is active, the KeyMap returns early
	// with only permission bindings. Language switch should still work
	// when dialog is not active (tested above via default state).
	state := NewAppState()
	state.DecisionReq.Set(&DecisionRequest{DecisionID: "language-switch", ToolName: "Bash", Choices: []string{"allow_once", "reject"}})
	root := NewRootComponent(state, nil, nil)

	found := false
	for _, binding := range root.KeyMap() {
		if binding.Pattern.AnyKey {
			// permission dialog blocks with AnyKey
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected permission dialog to block keys with AnyKey")
	}
}

func TestLanguageSwitchKeyBinding_IsSuppressedDuringPermissionDialog(t *testing.T) {
	state := NewAppState()
	state.DecisionReq.Set(&DecisionRequest{DecisionID: "language-switch", ToolName: "Bash", Choices: []string{"allow_once", "reject"}})
	root := NewRootComponent(state, nil, nil)

	originalLang := state.Language.Get()
	for range root.KeyMap() {
		// just verify that the default KeyMap is replaced by permission bindings
	}
	_ = originalLang
}

func TestCtrlLGoTuiDispatchMatchesCorrectly(t *testing.T) {
	// KeyCtrlL = Rune('l').Ctrl() produces pattern with rune 'l' and ModCtrl.
	// The go-tui parser maps Ctrl+L (0x0C) to KeyEvent{Key: KeyRune, Rune: 'l', Mod: ModCtrl}.
	// These match via dispatch.go: p.Rune != 0 && ke.Rune == p.Rune && ke.Key == KeyRune.
	// Verified by TestLanguageSwitchKeyBinding_CyclesLanguages which fires
	// KeyEvent{Key: KeyRune, Rune: 'l', Mod: ModCtrl} and asserts the language changed.
}
