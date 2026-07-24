package commands

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/session"
)

// ---------------------------------------------------------------------------
// /resume  (/r)
// ---------------------------------------------------------------------------

type resumeCmd struct{}

func (c *resumeCmd) Name() string        { return "resume" }
func (c *resumeCmd) Aliases() []string   { return []string{"r"} }
func (c *resumeCmd) Description() string { return builtinCommandDescription("resume") }

func (c *resumeCmd) Execute(ctx *Context, args string) error {
	if ctx.SessionStore == nil {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandResumeStoreUnavailable))
		reportCommandFailed(ctx)
		return nil
	}

	query := strings.TrimSpace(args)
	if query == "" {
		sessions, err := ctx.SessionStore.List()
		if err != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeListError, session.UserFacingError(ctx.Language, err)))
			reportCommandFailed(ctx)
			return nil
		}
		if len(sessions) == 0 {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandResumeNone))
			reportCommandSucceeded(ctx)
			return nil
		}
		limit := 20
		if len(sessions) < limit {
			limit = len(sessions)
		}
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandResumeRecent))
		for _, s := range sessions[:limit] {
			marker := "  "
			if s.ID == ctx.SessionID {
				marker = "→ "
			}
			title := s.Title
			if title == "" {
				title = s.ID
			}
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeEntry, marker, title, s.UpdatedAt.Format("2006-01-02 15:04"), s.MessageCount))
		}
		reportCommandSucceeded(ctx)
		return nil
	}

	matches, err := ctx.SessionStore.Search(query, ctx.CWD, true)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeSearchError, session.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	if len(matches) == 0 {
		matches = []SessionListEntry{{ID: query}}
	}
	if len(matches) > 1 {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeMultiple, len(matches), query))
		reportCommandFailed(ctx)
		return nil
	}

	target := matches[0]
	if ctx.ResumeSession != nil {
		if err := ctx.ResumeSession(target); err != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeLoadError, target.ID, session.UserFacingError(ctx.Language, err)))
			reportCommandFailed(ctx)
			return nil
		}
		title := target.Title
		if title == "" {
			title = target.ID
		}
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeLoaded, title))
		reportCommandSucceeded(ctx)
		return nil
	}
	msgs, err := ctx.SessionStore.Load(target.ID)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeLoadError, target.ID, session.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	ctx.QueryLoop.SetMessages(msgs)
	if ctx.SetSessionID != nil {
		ctx.SetSessionID(target.ID)
	}
	title := target.Title
	if title == "" {
		title = target.ID
	}
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandResumeLoadedMessages, title, len(msgs)))
	reportCommandSucceeded(ctx)
	return nil
}
