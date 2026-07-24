package commands

import (
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/types"
)

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// ---------------------------------------------------------------------------
// /rename
// ---------------------------------------------------------------------------

type renameCmd struct{}

func (c *renameCmd) Name() string      { return "rename" }
func (c *renameCmd) Aliases() []string { return nil }
func (c *renameCmd) Description() string {
	return builtinCommandDescription("rename")
}

func (c *renameCmd) Execute(ctx *Context, args string) error {
	if ctx.SessionStore == nil {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandRenameStoreUnavailable))
		reportCommandFailed(ctx)
		return nil
	}

	name := strings.TrimSpace(args)
	if name == "" {
		name = generateSessionName(ctx.QueryLoop.Messages())
		if name == "" {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandRenameGenerateFailed))
			reportCommandFailed(ctx)
			return nil
		}
	}

	if err := ctx.SessionStore.Rename(ctx.SessionID, name); err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandRenameError, session.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandRenameSucceeded, name))
	reportCommandSucceeded(ctx)
	return nil
}

func generateSessionName(msgs []types.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].IsInternalRuntimeMessage() {
			continue
		}
		text := strings.TrimSpace(msgs[i].GetText())
		if text == "" {
			continue
		}
		text = strings.ToLower(text)
		text = nonSlugChars.ReplaceAllString(text, "-")
		text = strings.Trim(text, "-")
		parts := strings.Split(text, "-")
		kept := make([]string, 0, 4)
		for _, p := range parts {
			if p == "" || len(p) <= 2 {
				continue
			}
			kept = append(kept, p)
			if len(kept) == 4 {
				break
			}
		}
		if len(kept) == 0 {
			continue
		}
		return strings.Join(kept, "-")
	}
	return ""
}
