package main

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/tui"
)

func TestBuiltinTUISlashCommandWiringOpensLanguageSubmenu(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	entries := builtinTUISlashCommandEntries(registry)
	for _, entry := range entries {
		if entry.Name != "language" {
			continue
		}
		if !entry.OpensSubmenu || !slices.Contains(entry.Aliases, "lang") {
			t.Fatalf("language entry = %#v", entry)
		}
		return
	}
	t.Fatal("production slash registry omitted /language")
}

func TestBuiltinTUISlashCommandWiringExposesSkillsOnlyThroughSkillsEntry(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	entries := builtinTUISlashCommandEntries(registry)
	if len(entries) != len(registry.All()) {
		t.Fatalf("slash entries=%d, registered commands=%d", len(entries), len(registry.All()))
	}

	skillsEntries := 0
	for _, entry := range entries {
		if entry.Name == "skills" {
			skillsEntries++
		}
		if strings.HasPrefix(entry.Name, "skill:") {
			t.Fatalf("skill catalog entry leaked into slash commands: %#v", entry)
		}
	}
	if skillsEntries != 1 {
		t.Fatalf("/skills entries=%d, want exactly one catalog entry", skillsEntries)
	}
}

type task23TUILanguageSwitcher struct {
	state    *tui.AppState
	switchFn func(i18n.Language) error
}

func (s *task23TUILanguageSwitcher) State() *tui.AppState { return s.state }

func (s *task23TUILanguageSwitcher) SwitchLanguage(lang i18n.Language) error {
	return s.switchFn(lang)
}

func TestSwitchTUILanguageUsesAtomicAppTransaction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })
	if err := i18n.SaveLanguage(i18n.LangEN); err != nil {
		t.Fatalf("save starting language: %v", err)
	}
	state := tui.NewAppState()
	state.Language.Set(i18n.LangEN)
	switcher := &task23TUILanguageSwitcher{state: state}
	var calls int
	switcher.switchFn = func(target i18n.Language) error {
		calls++
		if got := state.Language.Get(); got != i18n.LangEN {
			t.Fatalf("command published AppState before the transaction: %v", got)
		}
		if err := i18n.SaveLanguage(target); err != nil {
			return err
		}
		state.Language.Set(target)
		return nil
	}

	got := switchTUILanguage(switcher, "ZH")

	if calls != 1 {
		t.Fatalf("SwitchLanguage calls = %d, want 1", calls)
	}
	if state.Language.Get() != i18n.LangZH || i18n.DetectOrLoadLanguage() != i18n.LangZH {
		t.Fatalf("successful transaction did not converge: state=%v active=%v", state.Language.Get(), i18n.DetectOrLoadLanguage())
	}
	if want := i18n.Format(i18n.LangZH, i18n.KeyLanguageSwitched, i18n.LangZH.String()); got != want {
		t.Fatalf("switch result = %q, want %q", got, want)
	}
}

func TestSwitchTUILanguageFailureKeepsPreviousLanguageAndLocalizesError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Cleanup(func() { _ = i18n.SaveLanguage(i18n.LangEN) })
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("save starting language: %v", err)
	}
	state := tui.NewAppState()
	state.Language.Set(i18n.LangZH)
	switcher := &task23TUILanguageSwitcher{
		state: state,
		switchFn: func(i18n.Language) error {
			return errors.New("read-only preference store")
		},
	}

	got := switchTUILanguage(switcher, "next")

	if state.Language.Get() != i18n.LangZH || i18n.DetectOrLoadLanguage() != i18n.LangZH {
		t.Fatalf("failed transaction changed language: state=%v active=%v", state.Language.Get(), i18n.DetectOrLoadLanguage())
	}
	if want := i18n.Text(i18n.LangZH, i18n.KeyLanguageUnavailable); got != want {
		t.Fatalf("failure result = %q, want localized %q", got, want)
	}
}
