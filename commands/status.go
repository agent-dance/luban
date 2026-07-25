package commands

import (
	"os"
	"strings"
	"time"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
)

// ---------------------------------------------------------------------------
// /status  (/st)
// ---------------------------------------------------------------------------

type statusCmd struct{}

func (c *statusCmd) Name() string      { return "status" }
func (c *statusCmd) Aliases() []string { return []string{"st"} }
func (c *statusCmd) Description() string {
	return builtinCommandDescription("status")
}

func (c *statusCmd) Execute(ctx *Context, _ string) error {
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	model := ctx.CurrentModel
	if model == "" {
		model = ctx.QueryLoop.Model()
	}
	if model == "" {
		model = i18n.Text(ctx.Language, i18n.KeyCommandStatusUnknown)
	}

	sessionID := ctx.SessionID
	if sessionID == "" {
		sessionID = i18n.Text(ctx.Language, i18n.KeyCommandStatusNone)
	}

	messages := ctx.QueryLoop.Messages()
	msgCount := len(messages)
	uniqueInputTokens := ctx.SessionUniqueInputTokens
	if uniqueInputTokens == 0 {
		uniqueInputTokens = estimateUniqueInputTokens(messages)
	}

	var sb strings.Builder
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandStatusReport, model, sessionID, cwd, msgCount, formatTokens(uniqueInputTokens)))
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandStatusAPIUsage, formatTokens(ctx.TotalInputTokens), formatTokens(ctx.TotalOutputTokens), formatTokens(ctx.TotalCacheReadTokens), formatTokens(ctx.TotalCacheCreationTokens)))
	if ctx.TotalWebSearchRequests > 0 {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandStatusWebSearches, formatTokens(ctx.TotalWebSearchRequests)))
	}
	if ctx.TotalCostUSD > 0 {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandStatusCost, provider.CostCurrencySymbol(ctx.CostCurrency), ctx.TotalCostUSD))
	} else if ctx.CostUnknown {
		sb.WriteString(i18n.Text(ctx.Language, i18n.KeyCommandStatusCostUnknown))
	}
	appendBuildDiagnostic(&sb, ctx.Language, ctx.BuildDiagnostic)
	ctx.OnEvent(sb.String())
	return nil
}

func appendBuildDiagnostic(builder *strings.Builder, lang i18n.Language, diagnostic buildinfo.Diagnostic) {
	fingerprint := diagnostic.Fingerprint
	if fingerprint.ProcessStart.IsZero() {
		return
	}
	unknown := i18n.Text(lang, i18n.KeyBuildValueUnknown)
	version := strings.TrimSpace(fingerprint.Version)
	if version == "" {
		version = unknown
	}
	revision := strings.TrimSpace(fingerprint.Revision)
	if revision == "" {
		revision = unknown
	}
	state := i18n.Text(lang, i18n.KeyBuildStateUnknown)
	if fingerprint.Dirty != nil {
		if *fingerprint.Dirty {
			state = i18n.Text(lang, i18n.KeyBuildStateDirty)
		} else {
			state = i18n.Text(lang, i18n.KeyBuildStateClean)
		}
	}
	buildTime := unknown
	if fingerprint.BuildTime != nil {
		buildTime = fingerprint.BuildTime.Format(time.RFC3339)
	}
	executable := strings.TrimSpace(fingerprint.Executable)
	if executable == "" {
		executable = unknown
	}
	head := i18n.Text(lang, i18n.KeyBuildHeadUnknown)
	switch diagnostic.RevisionStatus {
	case buildinfo.RevisionMatch:
		head = i18n.Text(lang, i18n.KeyBuildHeadMatch)
	case buildinfo.RevisionStale:
		head = i18n.Text(lang, i18n.KeyBuildHeadStale)
	}
	builder.WriteString(i18n.Format(lang, i18n.KeyBuildFingerprintDetail,
		version, revision, state, buildTime, fingerprint.ProcessStart.Format(time.RFC3339), executable, head))
}
