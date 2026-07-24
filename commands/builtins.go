package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/brand"
	compactpkg "github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/cost"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/session"
)

// RegisterBuiltins adds all built-in slash commands to r.
func RegisterBuiltins(r *Registry) {
	r.Register(&exitCmd{})
	r.Register(&helpCmd{registry: r})
	r.Register(&clearCmd{})
	r.Register(&goalCmd{})
	r.Register(&searchTranscriptCmd{})
	r.Register(&exportTranscriptCmd{})
	r.Register(&editorTranscriptCmd{})
	r.Register(&mouseCaptureCmd{})
	r.Register(&activityCmd{})
	r.Register(&detailCmd{})
	r.Register(&compactCmd{})
	r.Register(&modelCmd{})
	r.Register(&sessionCmd{})
	r.Register(&configCmd{})
	r.Register(&statusCmd{})
	r.Register(&contextCmd{})
	r.Register(&initCmd{})
	r.Register(&resumeCmd{})
	r.Register(&forkCmd{})
	r.Register(&reviewCmd{})
	r.Register(&doctorCmd{})
	r.Register(&languageCmd{})
	r.Register(NewSkillsCommand(nil))
	RegisterMCPCommand(r, nil)
}

type activityCmd struct{}

func (*activityCmd) Name() string        { return "activity" }
func (*activityCmd) Aliases() []string   { return []string{"activities"} }
func (*activityCmd) Description() string { return builtinCommandDescription("activity") }
func (*activityCmd) Execute(ctx *Context, args string) error {
	fields := strings.Fields(args)
	if len(fields) == 0 || fields[0] == "list" {
		if ctx.OpenActivityView == nil {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinActivityViewUnavailable))
		}
		if result := ctx.OpenActivityView(); result != "" && ctx.OnEvent != nil {
			ctx.OnEvent(result)
		}
		return nil
	}
	if len(fields) == 1 && fields[0] == "close" {
		if ctx.CloseActivityView == nil {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinActivityViewUnavailable))
		}
		if result := ctx.CloseActivityView(); result != "" && ctx.OnEvent != nil {
			ctx.OnEvent(result)
		}
		return nil
	}
	if ctx.ActivityAction == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinActivityActionsUnavailable))
	}
	if len(fields) != 2 {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinActivityUsage))
	}
	result, err := ctx.ActivityAction(fields[0], fields[1])
	if err != nil {
		message := i18n.Format(ctx.Language, i18n.KeyRuntimeCommandActivityActionFailed, i18n.RuntimeActivityActionLabel(ctx.Language, fields[1]), fields[0])
		return fmt.Errorf("%s: %w", message, err)
	}
	if result != "" && ctx.OnEvent != nil {
		ctx.OnEvent(result)
	}
	return nil
}

type detailCmd struct{}

func (*detailCmd) Name() string        { return "detail" }
func (*detailCmd) Aliases() []string   { return nil }
func (*detailCmd) Description() string { return builtinCommandDescription("detail") }
func (*detailCmd) Execute(ctx *Context, args string) error {
	fields := strings.Fields(args)
	if len(fields) < 1 || len(fields) > 2 || ctx.SetDisclosure == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinDetailUsage))
	}
	level := "next"
	if len(fields) == 2 {
		level = fields[1]
	}
	result, err := ctx.SetDisclosure(fields[0], level)
	if err != nil {
		message := i18n.Format(ctx.Language, i18n.KeyRuntimeCommandDetailFailed, fields[0])
		return fmt.Errorf("%s: %w", message, err)
	}
	if result != "" && ctx.OnEvent != nil {
		ctx.OnEvent(result)
	}
	return nil
}

type searchTranscriptCmd struct{}

func (*searchTranscriptCmd) Name() string        { return "search" }
func (*searchTranscriptCmd) Aliases() []string   { return nil }
func (*searchTranscriptCmd) Description() string { return builtinCommandDescription("search") }
func (*searchTranscriptCmd) Execute(ctx *Context, args string) error {
	query := strings.TrimSpace(args)
	if query == "" {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinSearchUsage))
	}
	if ctx.SearchTranscript == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinSearchUnavailable))
	}
	result, err := ctx.SearchTranscript(query)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Text(ctx.Language, i18n.KeyRuntimeCommandSearchFailed), err)
	}
	if result != "" && ctx.OnEvent != nil {
		ctx.OnEvent(result)
	}
	return nil
}

type exportTranscriptCmd struct{}

func (*exportTranscriptCmd) Name() string        { return "export" }
func (*exportTranscriptCmd) Aliases() []string   { return nil }
func (*exportTranscriptCmd) Description() string { return builtinCommandDescription("export") }
func (*exportTranscriptCmd) Execute(ctx *Context, args string) error {
	if ctx.ExportTranscript == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinExportUnavailable))
	}
	path, err := ctx.ExportTranscript(strings.TrimSpace(args))
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Text(ctx.Language, i18n.KeyRuntimeCommandExportFailed), err)
	}
	if path != "" && ctx.OnEvent != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinExported, path))
	}
	return nil
}

type editorTranscriptCmd struct{}

func (*editorTranscriptCmd) Name() string      { return "editor" }
func (*editorTranscriptCmd) Aliases() []string { return nil }
func (*editorTranscriptCmd) Description() string {
	return builtinCommandDescription("editor")
}
func (*editorTranscriptCmd) Execute(ctx *Context, args string) error {
	fields := strings.Fields(args)
	if len(fields) > 0 && fields[0] == "detail" {
		if len(fields) != 2 || ctx.OpenDetailEditor == nil {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinEditorDetailUsage))
		}
		if err := ctx.OpenDetailEditor(fields[1]); err != nil {
			return i18n.WrapInternalErrorInLanguage(ctx.Language, i18n.KeyRuntimeCommandEditorFailed, err)
		}
		return nil
	}
	if ctx.OpenTranscriptEditor == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinEditorUnavailable))
	}
	if err := ctx.OpenTranscriptEditor(strings.TrimSpace(args)); err != nil {
		return i18n.WrapInternalErrorInLanguage(ctx.Language, i18n.KeyRuntimeCommandEditorFailed, err)
	}
	return nil
}

type mouseCaptureCmd struct{}

func (*mouseCaptureCmd) Name() string        { return "mouse" }
func (*mouseCaptureCmd) Aliases() []string   { return nil }
func (*mouseCaptureCmd) Description() string { return builtinCommandDescription("mouse") }
func (*mouseCaptureCmd) Execute(ctx *Context, args string) error {
	if ctx.SetMouseCapture == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinMouseUnavailable))
	}
	mode := strings.ToLower(strings.TrimSpace(args))
	if mode == "" {
		mode = "toggle"
	}
	if mode != "toggle" && mode != "on" && mode != "off" {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinMouseUsage))
	}
	enabled, err := ctx.SetMouseCapture(mode)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.Text(ctx.Language, i18n.KeyRuntimeCommandMouseFailed), err)
	}
	if ctx.OnEvent != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinMouseState, enabled))
	}
	return nil
}

// ---------------------------------------------------------------------------
// /help
// ---------------------------------------------------------------------------

type helpCmd struct {
	registry *Registry
}

func (c *helpCmd) Name() string        { return "help" }
func (c *helpCmd) Aliases() []string   { return nil }
func (c *helpCmd) Description() string { return builtinCommandDescription("help") }

func (c *helpCmd) Execute(ctx *Context, _ string) error {
	var sb strings.Builder
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinHelpCommands))
	for _, cmd := range c.registry.All() {
		line := fmt.Sprintf("  /%s", cmd.Name())
		if aliases := cmd.Aliases(); len(aliases) > 0 {
			parts := make([]string, len(aliases))
			for i, a := range aliases {
				parts[i] = "/" + a
			}
			line += " (" + strings.Join(parts, ", ") + ")"
		}
		line += "  — " + LocalizedCommandDescription(ctx.Language, cmd)
		sb.WriteString(line + "\n")
	}
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinHelpShortcuts))
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinHelpShortcutCycle))
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinHelpShortcutToggle))
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinHelpShortcutJump))
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinHelpShortcutScroll))
	sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinHelpShortcutClose))
	ctx.OnEvent(sb.String())
	return nil
}

// ---------------------------------------------------------------------------
// /exit  (/quit)
// ---------------------------------------------------------------------------

type exitCmd struct{}

func (c *exitCmd) Name() string        { return "exit" }
func (c *exitCmd) Aliases() []string   { return []string{"quit"} }
func (c *exitCmd) Description() string { return builtinCommandDescription("exit") }

func (c *exitCmd) Execute(_ *Context, _ string) error {
	return ErrExit
}

// ---------------------------------------------------------------------------
// /clear
// ---------------------------------------------------------------------------

type clearCmd struct{}

func (c *clearCmd) Name() string        { return "clear" }
func (c *clearCmd) Aliases() []string   { return nil }
func (c *clearCmd) Description() string { return builtinCommandDescription("clear") }

func (c *clearCmd) Execute(ctx *Context, args string) error {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "view":
		if ctx.ClearView == nil {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinClearViewUnavailable))
		}
		return ctx.ClearView()
	case "", "conversation":
		if ctx.ClearConversation == nil {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinClearConversationUnavailable))
		}
		return ctx.ClearConversation()
	default:
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinClearUsage))
	}
}

// ---------------------------------------------------------------------------
// /compact
// ---------------------------------------------------------------------------

type compactCmd struct{}

func (c *compactCmd) Name() string        { return "compact" }
func (c *compactCmd) Aliases() []string   { return nil }
func (c *compactCmd) Description() string { return builtinCommandDescription("compact") }

func (c *compactCmd) Execute(ctx *Context, args string) error {
	msgs := ctx.QueryLoop.Messages()
	if len(msgs) == 0 {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyBuiltinCompactEmpty))
		return nil
	}

	if ctx.CompactFunc == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinCompactUnavailable))
	}

	ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyBuiltinCompactRunning))
	if err := ctx.CompactFunc(strings.TrimSpace(args)); err != nil {
		return fmt.Errorf("%s", compactpkg.FormatCompactUserError(ctx.Language, err))
	}
	after := ctx.QueryLoop.Messages()
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinCompactComplete, len(msgs), len(after)))
	return nil
}

// ---------------------------------------------------------------------------
// /model
// ---------------------------------------------------------------------------

type modelCmd struct{}

func (c *modelCmd) Name() string        { return "model" }
func (c *modelCmd) Aliases() []string   { return nil }
func (c *modelCmd) Description() string { return builtinCommandDescription("model") }

func (c *modelCmd) Execute(ctx *Context, args string) error {
	if args == "" {
		if ctx.OpenModelPicker != nil {
			return ctx.OpenModelPicker()
		}
		return c.showModels(ctx)
	}
	return c.switchModel(ctx, args)
}

// showModels lists the current model and all available models from the catalog.
func (c *modelCmd) showModels(ctx *Context) error {
	var sb strings.Builder

	currentProvider := ctx.CurrentProvider
	currentModel := ctx.QueryLoop.Model()

	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinModelCurrent, currentProvider, currentModel))

	// If we have a ProviderRegistry, list available models grouped by provider.
	if ctx.ProviderRegistry != nil {
		sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinModelAvailable))
		catalog := ctx.ProviderRegistry.Catalog()
		available := ctx.ProviderRegistry.Available()

		for _, pInfo := range available {
			models := catalog.ListByProvider(pInfo.Name)
			if len(models) == 0 {
				continue
			}
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinModelProvider, pInfo.DisplayName, pInfo.Name))
			for _, m := range models {
				marker := "  "
				if m.Provider == currentProvider && m.ID == currentModel {
					marker = "→ "
				}
				extra := ""
				if m.CanSeeImages {
					extra += i18n.Text(ctx.Language, i18n.KeyBuiltinModelVision)
				} else {
					extra += i18n.Text(ctx.Language, i18n.KeyBuiltinModelText)
				}
				if len(m.ReasoningEfforts) > 0 {
					extra += i18n.Format(ctx.Language, i18n.KeyBuiltinModelReasoningEfforts, strings.Join(m.ReasoningEfforts, "/"))
				} else if m.CanReason {
					extra += i18n.Text(ctx.Language, i18n.KeyBuiltinModelThinking)
				}
				if m.IsDefault {
					extra += i18n.Text(ctx.Language, i18n.KeyBuiltinModelDefault)
				}
				ctxK := ""
				if m.ContextWindow > 0 {
					ctxK = provider.FormatContextWindow(m.ContextWindow)
				}
				costStr := ""
				if m.CostPer1MIn > 0 {
					costStr = formatModelPricePair(m.CostPer1MIn, m.CostPer1MOut, m.BillingCurrency())
				}

				line := fmt.Sprintf("    %s%s/%s", marker, pInfo.Name, m.ID)
				if ctxK != "" || costStr != "" {
					line += "  ["
					if ctxK != "" {
						line += ctxK
					}
					if costStr != "" {
						if ctxK != "" {
							line += ", "
						}
						line += costStr
					}
					line += "]"
				}
				line += extra
				sb.WriteString(line + "\n")
			}
		}
	}

	ctx.OnEvent(sb.String())
	return nil
}

// switchModel handles model switching with optional provider/model format.
func (c *modelCmd) switchModel(ctx *Context, args string) error {
	// Parse "provider/model" format
	providerName, modelName := parseProviderModel(args)
	if providerName != "" {
		providerName = provider.CanonicalProviderName(providerName)
	}

	if providerName == "" {
		// Simple model switch within current provider.
		ctx.QueryLoop.SetModel(modelName)
		if path, err := persistProjectSetting(ctx, "model", modelName); err != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelSwitchedPersistError, modelName, err))
			reportCommandDomainResult(ctx, CommandOutcomeWarning, "", i18n.Text(ctx.Language, i18n.KeyCommandPresentationModelSaveWarning))
		} else if path != "" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelSwitchedSaved, modelName, path))
		} else {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelSwitched, modelName))
		}
		return nil
	}

	// Cross-provider switch: need Registry and ProviderRef.
	if ctx.ProviderRegistry == nil || ctx.ProviderRef == nil {
		// Fallback: just switch the model name without provider change.
		ctx.QueryLoop.SetModel(modelName)
		if path, err := persistProjectSettings(ctx, map[string]interface{}{"provider": providerName, "model": modelName}); err != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelFallbackPersistError, modelName, err))
			reportCommandDomainResult(ctx, CommandOutcomeWarning, "", i18n.Text(ctx.Language, i18n.KeyCommandPresentationModelSaveWarning))
		} else if path != "" {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelFallbackSaved, modelName, path))
		} else {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelFallback, modelName))
		}
		return nil
	}

	// Check if the provider exists in the registry.
	pInfo, ok := ctx.ProviderRegistry.Get(providerName)
	if !ok {
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyBuiltinModelUnknownProvider,
			providerName, strings.Join(ctx.ProviderRegistry.VisibleNames(), ", ")))
	}

	// Check whether the provider is model-selectable.
	connection := ctx.ProviderRegistry.ConnectionState(pInfo.Name).Localized(ctx.Language)
	if !connection.CanSelectModels {
		if connection.SetupHint != "" {
			return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyBuiltinModelProviderNotReady, providerName, connection.SetupHint))
		}
		return fmt.Errorf("%s", i18n.Format(ctx.Language, i18n.KeyBuiltinModelProviderNotReady, providerName, connection.Detail))
	}

	// Build a Config from the credential store for the target provider.
	cfg, err := provider.ResolveCredentialConfig(ctx.ProviderRegistry, providerName)
	if err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyBuiltinModelLoadCredentialsError), providerName, err)
	}
	// Also check env var fallback.
	if cfg.APIKey == "" && pInfo.EnvKey != "" {
		cfg.APIKey = os.Getenv(pInfo.EnvKey)
	}

	// Create the new provider via the registry factory.
	newP, err := ctx.ProviderRegistry.Create(providerName, cfg, modelName)
	if err != nil {
		return fmt.Errorf(i18n.Text(ctx.Language, i18n.KeyBuiltinModelCreateProviderError), providerName, err)
	}

	// Atomically swap the provider and update the model.
	ctx.QueryLoop.SetProvider(newP)
	ctx.QueryLoop.SetModel(modelName)

	// Build info string.
	extra := ""
	catalog := ctx.ProviderRegistry.Catalog()
	if mInfo, found := catalog.ResolveForProvider(providerName, modelName); found {
		parts := []string{}
		if mInfo.ContextWindow > 0 {
			parts = append(parts, i18n.Format(ctx.Language, i18n.KeyBuiltinModelContext, provider.FormatContextWindow(mInfo.ContextWindow)))
		}
		if mInfo.CanSeeImages {
			parts = append(parts, i18n.Text(ctx.Language, i18n.KeyBuiltinModelVisionBare))
		} else {
			parts = append(parts, i18n.Text(ctx.Language, i18n.KeyBuiltinModelTextOnly))
		}
		if mInfo.CanReason {
			parts = append(parts, i18n.Text(ctx.Language, i18n.KeyBuiltinModelReasoningBare))
		}
		if len(parts) > 0 {
			extra = " (" + strings.Join(parts, ", ") + ")"
		}
	}
	if path, err := persistProjectSettings(ctx, map[string]interface{}{"provider": providerName, "model": modelName}); err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelProviderSwitchedPersistError, providerName, modelName, extra, err))
		reportCommandDomainResult(ctx, CommandOutcomeWarning, "", i18n.Text(ctx.Language, i18n.KeyCommandPresentationProviderWarning))
	} else if path != "" {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelProviderSwitchedSaved, providerName, modelName, extra, path))
	} else {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinModelProviderSwitched, providerName, modelName, extra))
	}
	return nil
}

// parseProviderModel splits "provider/model" into (provider, model).
// If there's no slash, returns ("", input).
func parseProviderModel(input string) (string, string) {
	input = strings.TrimSpace(input)
	idx := strings.Index(input, "/")
	if idx <= 0 {
		return "", input
	}
	return input[:idx], input[idx+1:]
}

func formatModelPricePair(in, out float64, currency string) string {
	symbol := provider.CostCurrencySymbol(currency)
	return fmt.Sprintf("%s%.2f/%s%.2f", symbol, in, symbol, out)
}

// ---------------------------------------------------------------------------
// /cost
// ---------------------------------------------------------------------------

type costCmd struct{}

func (c *costCmd) Name() string        { return "cost" }
func (c *costCmd) Aliases() []string   { return nil }
func (c *costCmd) Description() string { return builtinCommandDescription("cost") }

func (c *costCmd) Execute(ctx *Context, _ string) error {
	usage := cost.TokenUsage{
		InputTokens:              ctx.TotalInputTokens,
		OutputTokens:             ctx.TotalOutputTokens,
		CacheReadInputTokens:     ctx.TotalCacheReadTokens,
		CacheCreationInputTokens: ctx.TotalCacheCreationTokens,
		WebSearchRequests:        ctx.TotalWebSearchRequests,
	}
	totalTokens := usage.InputTokens + usage.OutputTokens

	model := ctx.CurrentModel
	breakdown, ok := cost.CalculateCost(model, usage)

	// If we have catalog-based per-model data, try to recalculate total from that
	if ctx.ProviderRegistry != nil {
		catalog := ctx.ProviderRegistry.Catalog()
		if info, found := catalog.ResolveForProvider(ctx.CurrentProvider, model); found && info.BillingCurrency() == "USD" && (info.CostPer1MIn > 0 || info.CostPer1MOut > 0) {
			cacheReadRate := info.CacheReadPer1M
			if cacheReadRate <= 0 {
				cacheReadRate = info.CostPer1MIn
			}
			cacheCreationRate := info.CacheCreatePer1M
			if cacheCreationRate <= 0 {
				cacheCreationRate = info.CostPer1MIn
			}
			webSearchRate := 0.0
			switch provider.CanonicalProviderName(ctx.CurrentProvider) {
			case "anthropic", "bedrock", "vertex", "oauth":
				webSearchRate = cost.WebSearchRequestPriceUSD
			}
			pricing := cost.ModelPricing{
				InputPerMtok:         info.CostPer1MIn,
				OutputPerMtok:        info.CostPer1MOut,
				CacheReadPerMtok:     cacheReadRate,
				CacheCreationPerMtok: cacheCreationRate,
				WebSearchPerRequest:  webSearchRate,
			}
			breakdown = cost.CalculateCostFromPricing(pricing, usage)
			ok = true
		}
	}
	if ctx.SessionCostBreakdown != nil {
		breakdown = *ctx.SessionCostBreakdown
		ok = true
	}
	if ctx.CostUnknown {
		ok = false
	}

	// Use TotalCostUSD if available (it's accumulated across model switches)
	sessionCost := ctx.TotalCostUSD
	if sessionCost == 0 && ok {
		sessionCost = breakdown.TotalUSD
	}

	var sb strings.Builder
	if ok {
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostSession, cost.FormatUSD(sessionCost)))
		showBucketCosts := ctx.SessionCostBreakdown != nil || ctx.TotalCostUSD == 0
		if showBucketCosts {
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostInputWithCost, formatTokens(usage.InputTokens), cost.FormatUSD(breakdown.InputUSD)))
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostOutputWithCost, formatTokens(usage.OutputTokens), cost.FormatUSD(breakdown.OutputUSD)))
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostCacheReadWithCost, formatTokens(usage.CacheReadInputTokens), cost.FormatUSD(breakdown.CacheReadUSD)))
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostCacheCreationWithCost, formatTokens(usage.CacheCreationInputTokens), cost.FormatUSD(breakdown.CacheCreationUSD)))
		} else {
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostInput, formatTokens(usage.InputTokens)))
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostOutput, formatTokens(usage.OutputTokens)))
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostCacheRead, formatTokens(usage.CacheReadInputTokens)))
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostCacheCreation, formatTokens(usage.CacheCreationInputTokens)))
		}
		if usage.WebSearchRequests > 0 {
			if showBucketCosts {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostWebSearchWithCost, formatTokens(usage.WebSearchRequests), cost.FormatUSD(breakdown.WebSearchUSD)))
			} else {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostWebSearch, formatTokens(usage.WebSearchRequests)))
			}
		}
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostTotal, formatTokens(totalTokens)))
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostModel, model))
	} else {
		// Unknown model — show tokens only.
		sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostUsageOnly,
			formatTokens(usage.InputTokens), formatTokens(usage.OutputTokens), formatTokens(totalTokens)))
		if model != "" {
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostModelUnknown, model))
		}
	}

	// Per-model breakdown (multi-model sessions)
	if len(ctx.PerModelCosts) > 1 {
		sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinCostPerModel))
		for _, mc := range ctx.PerModelCosts {
			webSearch := ""
			if mc.WebSearchRequests > 0 {
				webSearch = i18n.Format(ctx.Language, i18n.KeyBuiltinCostPerModelWebSearch, mc.WebSearchRequests)
			}
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinCostPerModelRow,
				mc.Model, formatTokens(mc.InputTokens), formatTokens(mc.OutputTokens), webSearch, mc.TurnCount, cost.FormatUSD(mc.CostUSD)))
		}
	}

	ctx.OnEvent(sb.String())
	return nil
}

// formatTokens formats an integer token count with comma separators.
func formatTokens(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(ch))
	}
	return string(result)
}

// ---------------------------------------------------------------------------
// /version
// ---------------------------------------------------------------------------

type versionCmd struct{}

func (c *versionCmd) Name() string        { return "version" }
func (c *versionCmd) Aliases() []string   { return nil }
func (c *versionCmd) Description() string { return builtinCommandDescription("version") }

func (c *versionCmd) Execute(ctx *Context, _ string) error {
	v := ctx.AppVersion
	if v == "" {
		v = "dev"
	}
	ctx.OnEvent(fmt.Sprintf("%s %s\n", brand.RuntimeName, v))
	return nil
}

// ---------------------------------------------------------------------------
// /session
// ---------------------------------------------------------------------------

type sessionCmd struct{}

func (c *sessionCmd) Name() string      { return "session" }
func (c *sessionCmd) Aliases() []string { return nil }
func (c *sessionCmd) Description() string {
	return builtinCommandDescription("session")
}

func (c *sessionCmd) Execute(ctx *Context, args string) error {
	parts := strings.Fields(args)

	if len(parts) == 0 || parts[0] == "current" {
		if ctx.SessionStore == nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionCurrentID, ctx.SessionID))
			reportCommandSucceeded(ctx)
			return nil
		}
		sessions, err := ctx.SessionStore.List()
		if err != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionCurrentID, ctx.SessionID))
			reportCommandFailed(ctx)
			return nil
		}
		for _, s := range sessions {
			if s.ID != ctx.SessionID {
				continue
			}
			title := s.Title
			if title == "" {
				title = s.ID
			}
			var sb strings.Builder
			sb.WriteString(i18n.Text(ctx.Language, i18n.KeyBuiltinSessionCurrent))
			sb.WriteString(strings.Repeat("─", 42) + "\n")
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionID, s.ID))
			sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionTitle, title))
			if !s.CreatedAt.IsZero() {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionCreated, s.CreatedAt.Format("2006-01-02 15:04")))
			}
			if !s.UpdatedAt.IsZero() {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionUpdated, s.UpdatedAt.Format("2006-01-02 15:04")))
			}
			if s.MessageCount > 0 {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionMessages, s.MessageCount))
			}
			if s.GitBranch != "" {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionGitBranch, s.GitBranch))
			}
			if s.CWD != "" {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionDirectory, s.CWD))
			}
			if s.PreviewText != "" {
				sb.WriteString(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionPreview, s.PreviewText))
			}
			ctx.OnEvent(sb.String())
			reportCommandSucceeded(ctx)
			return nil
		}
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionCurrentID, ctx.SessionID))
		reportCommandSucceeded(ctx)
		return nil
	}

	switch parts[0] {
	case "list":
		if ctx.SessionStore == nil {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyBuiltinSessionStoreUnavailable))
			reportCommandFailed(ctx)
			return nil
		}
		query := ""
		if len(parts) > 1 {
			query = strings.Join(parts[1:], " ")
		}
		sessions, err := ctx.SessionStore.Search(query, ctx.CWD, true)
		if err != nil {
			ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyBuiltinSessionListError, session.UserFacingError(ctx.Language, err)))
			reportCommandFailed(ctx)
			return nil
		}
		if len(sessions) == 0 {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyBuiltinSessionNone))
			reportCommandSucceeded(ctx)
			return nil
		}
		limit := 15
		if len(sessions) < limit {
			limit = len(sessions)
		}
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyBuiltinSessionRecent))
		for _, s := range sessions[:limit] {
			marker := "  "
			if s.ID == ctx.SessionID {
				marker = "→ "
			}
			title := s.Title
			if title == "" {
				title = s.ID
			}
			line := fmt.Sprintf("%s%s  (%s", marker, title, s.UpdatedAt.Format("2006-01-02 15:04"))
			if s.MessageCount > 0 {
				line += i18n.Format(ctx.Language, i18n.KeyBuiltinSessionListMessages, s.MessageCount)
			}
			if s.Provider != "" {
				pm := s.Provider
				if s.Model != "" {
					pm += "/" + s.Model
				}
				line += fmt.Sprintf(", %s", pm)
			}
			if s.GitBranch != "" {
				line += fmt.Sprintf(", %s", s.GitBranch)
			}
			line += ")\n"
			ctx.OnEvent(line)
			if s.PreviewText != "" {
				ctx.OnEvent(fmt.Sprintf("    %s\n", s.PreviewText))
			}
		}
		reportCommandSucceeded(ctx)
		return nil

	case "load":
		if len(parts) < 2 {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyBuiltinSessionLoadUsage))
			reportCommandFailed(ctx)
			return nil
		}
		resume := &resumeCmd{}
		return resume.Execute(ctx, strings.Join(parts[1:], " "))

	case "rename":
		rename := &renameCmd{}
		return rename.Execute(ctx, strings.Join(parts[1:], " "))

	case "delete":
		if len(parts) != 2 {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinSessionDeleteUsage))
		}
		if ctx.DeleteHistory == nil {
			return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyBuiltinSessionDeleteUnavailable))
		}
		if err := ctx.DeleteHistory(parts[1]); err != nil {
			message := i18n.Format(ctx.Language, i18n.KeyRuntimeCommandSessionDeleteFailed, parts[1])
			return fmt.Errorf("%s: %w", message, err)
		}
		reportCommandSucceeded(ctx)
		return nil

	default:
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyBuiltinSessionUsage))
		reportCommandFailed(ctx)
		return nil
	}
}
