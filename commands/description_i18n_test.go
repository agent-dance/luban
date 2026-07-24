package commands

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

type externalDescriptionCommand struct {
	name string
	key  i18n.Key
}

func (c *externalDescriptionCommand) Name() string                 { return c.name }
func (*externalDescriptionCommand) Aliases() []string              { return nil }
func (*externalDescriptionCommand) Description() string            { return "Extension-provided copy" }
func (*externalDescriptionCommand) Execute(*Context, string) error { return nil }
func (c *externalDescriptionCommand) DescriptionKey() i18n.Key     { return c.key }

func TestBuiltinsHaveSemanticDescriptionKeys(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry)

	for _, command := range registry.All() {
		key, ok := CommandDescriptionKey(command)
		if !ok {
			t.Errorf("/%s has no semantic description key", command.Name())
			continue
		}
		for _, lang := range i18n.AllLanguages() {
			if got := LocalizedCommandDescription(lang, command); got == "" || got[0] == '[' {
				t.Errorf("/%s description in %s = %q (key %q)", command.Name(), lang.Code(), got, key)
			}
		}
	}
}

func TestDescriptionProviderSurvivesRegistryPresentationWrapper(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&externalDescriptionCommand{name: "semantic-extension", key: i18n.KeyCommandHelpDescription})

	command := registry.Find("semantic-extension")
	if got, want := LocalizedCommandDescription(i18n.LangZH, command), "列出所有可用命令"; got != want {
		t.Fatalf("localized extension description = %q, want %q", got, want)
	}
}

func TestUnknownExtensionDescriptionFallsBackToRawCopy(t *testing.T) {
	command := &externalDescriptionCommand{name: "raw-extension"}
	if got, want := LocalizedCommandDescription(i18n.LangZH, command), command.Description(); got != want {
		t.Fatalf("fallback description = %q, want %q", got, want)
	}
}

func TestHelpUsesActiveLanguageForBuiltInDescriptions(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry)

	var output string
	ctx := &Context{Language: i18n.LangZH, OnEvent: func(text string) { output += text }}
	if err := registry.Find("help").Execute(ctx, ""); err != nil {
		t.Fatalf("help Execute: %v", err)
	}
	for _, want := range []string{"/help", "列出所有可用命令", "选择显示语言", "管理 MCP 服务器"} {
		if !strings.Contains(output, want) {
			t.Errorf("localized help omitted %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "List all available commands") {
		t.Fatalf("localized help leaked the English description:\n%s", output)
	}
}
