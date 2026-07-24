package commands

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// forkCmd opens a runtime-owned history picker. Session snapshotting and
// terminal launch stay outside commands so this package remains renderer- and
// platform-neutral.
type forkCmd struct{}

func (*forkCmd) Name() string        { return "fork" }
func (*forkCmd) Aliases() []string   { return nil }
func (*forkCmd) Description() string { return builtinCommandDescription("fork") }

func (*forkCmd) Execute(ctx *Context, args string) error {
	lang := i18n.DetectOrLoadLanguage()
	if ctx != nil {
		lang = ctx.Language
	}
	if strings.TrimSpace(args) != "" {
		return fmt.Errorf("%s", i18n.Text(lang, i18n.KeyCommandForkUsage))
	}
	if ctx == nil || ctx.OpenForkPicker == nil {
		return fmt.Errorf("%s", i18n.Text(lang, i18n.KeyCommandForkPickerUnavailable))
	}
	return ctx.OpenForkPicker()
}
