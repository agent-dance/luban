package commands

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// /language  (/lang)
// ---------------------------------------------------------------------------

type languageCmd struct{}

func (c *languageCmd) Name() string        { return "language" }
func (c *languageCmd) Aliases() []string   { return []string{"lang"} }
func (c *languageCmd) Description() string { return builtinCommandDescription("language") }

func (c *languageCmd) Execute(ctx *Context, args string) error {
	code := strings.TrimSpace(args)
	if ctx.SwitchLanguage == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyLanguageUnavailable))
	}
	if code == "" {
		code = "show"
	}
	switch strings.ToLower(code) {
	case "show", "next", "en", "zh", "de", "ja", "ko", "ru":
	default:
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyLanguageUnsupported))
	}
	result := ctx.SwitchLanguage(code)
	if ctx.OnEvent != nil {
		ctx.OnEvent(result + "\n")
	}
	reportCommandSucceeded(ctx)
	return nil
}
