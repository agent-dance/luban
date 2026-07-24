package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agent-dance/luban/auth"
	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/sdk"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

// configureSkillRuntime binds the one registry-owned SkillTool to dynamic
// conversation state after the engine exists. The returned adapter is shared
// by both interactive transports and always enters through the explicit-user
// authorization path.
func configureSkillRuntime(deps *RegistryDeps, eng *engine.CoreEngine) commands.SkillInvoker {
	if deps == nil || deps.SkillTool == nil || eng == nil {
		return commands.SkillInvokerFunc(func(context.Context, commands.SkillInvocationRequest) (types.ToolResult, error) {
			return types.ToolResult{}, fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeySkillToolUnavailable))
		})
	}
	deps.SkillTool.LanguageResolver = func(context.Context) i18n.Language {
		return i18n.DetectOrLoadLanguage()
	}
	deps.SkillTool.SessionIDResolver = func(ctx context.Context) string {
		if exec, ok := loop.ToolExecutionContextFromContext(ctx); ok {
			return strings.TrimSpace(exec.SessionID)
		}
		return deps.CurrentSessionID()
	}
	deps.SkillTool.LoadedLedgerResolver = func(ctx context.Context, sessionID string, id skills.SkillID) tools.SkillLoadedLedgerState {
		if exec, ok := loop.ToolExecutionContextFromContext(ctx); ok {
			// A model execution must resolve against the QueryLoop that created
			// the immutable tool context. Falling through to CoreEngine here would
			// accidentally route Agent/Team child loops through a same-ID parent
			// conversation (or report every child invocation as a full body).
			if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(exec.SessionID) != strings.TrimSpace(sessionID) {
				return tools.SkillLoadedLedgerState{}
			}
			state, resolved := exec.ResolveSkillLoadedLedger(id)
			if !resolved {
				return tools.SkillLoadedLedgerState{}
			}
			return tools.SkillLoadedLedgerState{
				ContextEpoch:       state.ContextEpoch,
				LoadedContextEpoch: state.LoadedContextEpoch,
				ContentDigest:      state.ContentDigest,
				PayloadDigest:      state.PayloadDigest,
			}
		}
		// Explicit-user UI invocations have no tool execution context and may
		// consult only an idle, exact CoreEngine conversation.
		state := eng.ResolveSkillLoadedLedger(ctx, sessionID, id)
		return tools.SkillLoadedLedgerState{
			ContextEpoch:       state.ContextEpoch,
			LoadedContextEpoch: state.LoadedContextEpoch,
			ContentDigest:      state.ContentDigest,
			PayloadDigest:      state.PayloadDigest,
		}
	}
	return commands.SkillInvokerFunc(func(ctx context.Context, request commands.SkillInvocationRequest) (types.ToolResult, error) {
		return deps.SkillTool.Invoke(tools.WithSkillSessionID(ctx, request.SessionID), tools.SkillInvocationRequest{
			SessionID: request.SessionID, Selector: request.Selector,
			ExpectedRevision: request.ExpectedRevision, Origin: skills.InvocationOriginUser,
			Arguments: request.Arguments,
		})
	})
}

// prepareInitialRegistryRuntime is the startup gate for all staged workspace
// dependencies, including skill policy documents. main must not construct an
// Engine when this fails: doing so would leave the fixed Skill tool registered
// without its authoritative policy store.
func prepareInitialRegistryRuntime(deps *RegistryDeps, cwd string, allowedDirs []string) error {
	if deps == nil {
		return skills.ErrSkillOverrideStoreMissing
	}
	return deps.UpdateSessionContext(cwd, allowedDirs)
}

// loadHooks merges Claude, DeepSeek Code, and LUBAN Code hooks in migration
// order, with newer product settings taking precedence. Errors are non-fatal so the
// CLI works without any hooks.
func loadHooks(cwd string) *hooks.Runner {
	brandDir := filepath.Join(cwd, brand.ConfigDirName)
	legacyDeepSeekDir := filepath.Join(cwd, brand.LegacyDeepSeekConfigDirName)
	legacyDir := filepath.Join(cwd, brand.LegacyConfigDirName)
	brandSettings, _ := hooks.LoadFromSettings(filepath.Join(brandDir, "settings.json"))
	brandHooks, _ := hooks.LoadFromDir(filepath.Join(brandDir, "hooks"))
	legacyDeepSeekSettings, _ := hooks.LoadFromSettings(filepath.Join(legacyDeepSeekDir, "settings.json"))
	legacyDeepSeekHooks, _ := hooks.LoadFromDir(filepath.Join(legacyDeepSeekDir, "hooks"))
	legacySettings, _ := hooks.LoadFromSettings(filepath.Join(legacyDir, "settings.json"))
	legacyHooks, _ := hooks.LoadFromDir(filepath.Join(legacyDir, "hooks"))
	return legacySettings.Merge(legacyHooks).
		Merge(legacyDeepSeekSettings).Merge(legacyDeepSeekHooks).
		Merge(brandSettings).Merge(brandHooks)
}

// splitAndTrim splits s by sep and trims whitespace from each element,
// discarding empty strings.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func newOutputRenderer(opts cli.Options, output io.Writer) (ui.Renderer, error) {
	switch strings.ToLower(strings.TrimSpace(opts.OutputFormat)) {
	case "", "text":
		if opts.Quiet {
			return ui.NewQuietRenderer(output), nil
		}
		return ui.NewTermRenderer(output), nil
	case "json", "stream-json":
		return ui.NewJSONRenderer(output), nil
	default:
		return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyStartupUnsupportedOutput, opts.OutputFormat))
	}
}

func main() {
	opts := cli.Parse()
	lang := i18n.DetectOrLoadLanguage()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CLAUDE_CODE_SCREEN_READER")), "true") || os.Getenv("CLAUDE_CODE_SCREEN_READER") == "1" {
		opts.ScreenReader = true
	}
	stdinTerminal := cli.IsStdinTerminal()
	if err := cli.ValidateInputMode(opts, stdinTerminal); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		os.Exit(2)
	}

	// Pass --api flag to provider layer via env var (before provider init).
	if opts.API != "" {
		os.Setenv("OPENAI_API", opts.API)
	}

	// Obtain shared singletons for multi-provider support (Phase 4).
	providerRegistry := provider.DefaultRegistry()
	credStore, credErr := provider.DefaultCredentialStore()
	if credErr != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupCredentialStoreWarning, credErr))
	}
	// Wire credential store into the registry so Available() sees stored keys.
	if credStore != nil {
		providerRegistry.SetCredentialStore(credStore)
	}
	if oauthStore, oauthErr := auth.NewStore(); oauthErr == nil {
		providerRegistry.SetOAuthHook(auth.NewOAuthHookAdapter(oauthStore, auth.AnthropicOAuthConfig()))
	} else {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupOAuthStoreWarning, oauthErr))
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupWorkingDirectoryFatal, err))
		os.Exit(1)
	}
	if err := configureTerminalTheme(cwd); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupWarning, err))
	}
	startupModelSettings, settingsErr := loadStartupModelSettings(cwd)
	if settingsErr != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupWarning, settingsErr))
	} else {
		provider.SetRuntimeModelOverrides(startupModelSettings.ModelOverrides)
		providerRegistry.ApplyModelOverrides(startupModelSettings.ModelOverrides)
		applyStartupModelSettings(&opts, startupModelSettings)
	}

	// Create provider from environment, with CLI overrides for provider/model.
	p, err := provider.NewFromEnvWithOverrides(opts.Provider, opts.Model)
	if err != nil {
		if !opts.Print && !opts.SDK && isBootstrappableProviderError(providerRegistry, opts.Provider, err) {
			providerName, modelName := bootstrapProviderSelection(providerRegistry, opts.Provider, opts.Model)
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupWarning, err))
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupNoActiveModel, providerName, providerName))
			p = provider.NewDisconnectedProvider(providerName, modelName, err.Error())
		} else {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
			os.Exit(1)
		}
	}

	// Wrap in a ProviderRef for runtime hot-swap support. All downstream
	// consumers (Engine, AgentTool, TeamManager) receive pRef, so a
	// Swap() propagates everywhere automatically.
	pRef := provider.NewProviderRef(p)
	if opts.DebugFile != "" {
		debugFile, debugErr := provider.OpenDebugFile(opts.DebugFile)
		if debugErr != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, debugErr))
			os.Exit(1)
		}
		defer debugFile.Close()
		_, _ = fmt.Fprintln(debugFile, i18n.Format(lang, i18n.KeyLogDebugSessionStarted, time.Now().UTC().Format(time.RFC3339Nano)))
		pRef.SetDebugObserver(provider.NewDebugWriterObserver(debugFile))
	}

	sessionRepo := session.DefaultRepository()
	resolvedSession, resolveSessionErr := ResolveSession(opts.SessionID, opts.Resume, sessionRepo, cwd, os.Stderr)
	if resolveSessionErr != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupSessionFatal, resolveSessionErr))
		os.Exit(1)
	}
	sessionID := resolvedSession.Ref.ID
	sessionProjectDir := resolvedSession.Ref.ProjectDir
	if resolvedSession.Resumed {
		if resumeCWD := strings.TrimSpace(resolvedSession.SessionCWD); resumeCWD != "" {
			if info, statErr := os.Stat(resumeCWD); statErr == nil && info.IsDir() {
				if chdirErr := os.Chdir(resumeCWD); chdirErr == nil {
					cwd = resumeCWD
				}
			}
		}
	}
	allowedDirs := allowedDirsForSession(cwd, opts.AllowedDirs)

	// Validate environment safety for --allow-all mode.
	if opts.AllowAll {
		if err := permissions.ValidateEnvironmentForBypass(); err != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
			os.Exit(1)
		}
	}

	// Set up sandbox backend.
	var sb sandbox.Backend
	if opts.Sandbox {
		sb = sandbox.Detect()
		if sb.Name() == "none" {
			fmt.Fprint(os.Stderr, i18n.Text(lang, i18n.KeyStartupSandboxUnavailable))
		}
	}

	// Setup tool registry
	var webDomains *WebDomainConfig
	if opts.AllowedDomains != "" || opts.DisallowedDomains != "" {
		webDomains = &WebDomainConfig{}
		if opts.AllowedDomains != "" {
			webDomains.AllowedDomains = splitAndTrim(opts.AllowedDomains, ",")
		}
		if opts.DisallowedDomains != "" {
			webDomains.DisallowedDomains = splitAndTrim(opts.DisallowedDomains, ",")
		}
	}
	effectivePrint := opts.Print || !stdinTerminal
	interactive := !effectivePrint && !opts.SDK
	if interactive {
		_, restoreDiagnosticLogger := installInteractiveDiagnosticLogger()
		defer restoreDiagnosticLogger()
	}
	deps := SetupRegistry(pRef, cwd, allowedDirs, sb, webDomains, interactive)
	if err := prepareInitialRegistryRuntime(deps, cwd, allowedDirs); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		os.Exit(1)
	}
	deps.BindSessionIdentity(sessionID)
	deps.SetGoalRuntime(newDynamicSessionGoalRuntime(
		sessionRepo,
		func() string { return sessionID },
		func() string { return sessionProjectDir },
	))
	defer deps.CronStore.Stop()
	defer deps.StopWebFetchCache()
	defer deps.StopMCPRuntimeBridge()
	if opts.AllowedTools != "" {
		deps.RuntimeScope.SetAllowedTools(splitAndTrim(opts.AllowedTools, ","))
	}
	if opts.DisallowedTools != "" {
		deps.RuntimeScope.SetDeniedTools(splitAndTrim(opts.DisallowedTools, ","))
	}
	if err := deps.AgentTool.SetInlineProfilesFromJSON(opts.Agents); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		os.Exit(1)
	}

	// Build system prompt
	systemPrompt := buildSystemPromptForCWD(opts.SystemPrompt, deps.Registry, cwd)

	// Wire system prompt into Agent and Team tools
	deps.AgentTool.System = systemPrompt
	deps.TeamManager.System = systemPrompt

	// Effective max turns
	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 100
	}

	// Create hooks and engine
	hookRunner := loadHooks(cwd)
	deps.SetHookRunner(hookRunner)
	reasoningEffort := resolveStartupReasoningEffort(
		os.Getenv("OPENAI_REASONING_EFFORT"),
		startupModelSettings,
		pRef.Name(),
		pRef.ModelID(),
		providerRegistry.Catalog(),
	)

	// Build permission handler: --allow-all skips prompts; default is interactive.
	// When a real sandbox backend is active, SandboxAwarePermissionHandler binds
	// permission decisions to that executable capability. Auto-approval remains
	// disabled unless the backend also proves protected-path enforcement.
	// Both modes use permissions.Checker so that --allowed-tools / --disallowed-tools
	// whitelist/blacklist is enforced regardless of mode (bypass-immune).
	var permHandler engine.PermissionHandler
	var checker *permissions.Checker

	if opts.AllowAll {
		checker = permissions.NewChecker(permissions.ModeAllowAll, nil)
	} else {
		checker = permissions.NewChecker(permissions.ModeAskAlways, nil)
		checker.SetPromptFunc(permissions.NewRichPrompt(os.Stderr, os.Stdin).PromptFunc())
	}
	if err := bindPlanModePermissionDispatcher(deps.RuntimeScope, deps.PlanState, checker); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		os.Exit(1)
	}

	// Apply tool whitelist/blacklist (bypass-immune — works in all modes).
	if opts.AllowedTools != "" {
		checker.SetAllowedTools(splitAndTrim(opts.AllowedTools, ","))
	}
	if opts.DisallowedTools != "" {
		checker.SetDisallowedTools(splitAndTrim(opts.DisallowedTools, ","))
	}

	cliHandler := permissions.NewCLIPermissionHandler(checker)
	if sandbox.IsRealBackend(sb) && !opts.AllowAll {
		permHandler = permissions.NewSandboxAwarePermissionHandler(sb, cliHandler)
	} else {
		permHandler = cliHandler
	}
	childPermissionHandler := engine.AsLoopPermissionHandler(permHandler)
	deps.AgentTool.PermissionHandler = childPermissionHandler
	deps.TeamManager.PermissionHandler = childPermissionHandler

	// Inject safety dependencies: wire tools package functions into permissions package
	// to avoid circular imports. This must happen before any permission checks.
	permissions.SetSafetyConfig(permissions.SafetyConfig{
		ShellPolicyAnalyzer: tools.AnalyzeShellCommand,
	})

	eng, err := engine.New(engine.Config{
		Provider:              p,
		ProviderRef:           pRef,
		Registry:              deps.Registry,
		Sessions:              engine.NewRepositorySessionManager(sessionRepo, func() string { return sessionProjectDir }),
		SystemPrompt:          systemPrompt,
		HookRunner:            hookRunner,
		AllowedDirs:           allowedDirs,
		MaxTurns:              maxTurns,
		MaxTokens:             16384,
		MaxContextTokens:      200000,
		ReasoningEffort:       reasoningEffort,
		Permission:            permHandler,
		ProjectRoot:           cwd,
		CWD:                   cwd,
		SkillManager:          deps.SkillManager,
		SkillSessionOverrides: deps.SkillSessionOverrides,
		PlanState:             deps.PlanState,
		InvokedSkills:         deps.SkillTool,
		BackgroundTasks:       deps.BackgroundTasks,
		MCPState:              deps.MCPManager,
		AgentDefinitions:      deps.AgentTool,
	})
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		os.Exit(1)
	}
	defer eng.Shutdown(context.Background())
	skillInvoker := configureSkillRuntime(deps, eng)
	configureWorktreeSessionRuntime(deps, eng, &cwd, &hookRunner, opts.SystemPrompt, opts.AllowedDirs)

	// ── TTY detection (Task 2b) ─────────────────────────────────────────────
	// Piped stdin → auto-enable non-interactive print mode.
	if !stdinTerminal && !opts.Print {
		opts.Print = true
	}
	// Piped stdout → disable colors.
	if !cli.IsStdoutTerminal() {
		os.Setenv("NO_COLOR", "1")
	}

	// ── Print mode (-p) ─────────────────────────────────────────────────────
	if opts.Print {
		renderer, rendererErr := newOutputRenderer(opts, os.Stdout)
		if rendererErr != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, rendererErr))
			os.Exit(2)
		}
		exitCode := RunPrintMode(eng, renderer, strings.Join(opts.Args, " "), opts.Verbose)
		if err := eng.Shutdown(context.Background()); err != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupShutdownWarning, err))
		}
		os.Exit(exitCode)
	}

	// ── SDK mode (--sdk) ────────────────────────────────────────────────────
	if opts.SDK {
		sdkCtx, sdkCancel := context.WithCancel(context.Background())
		defer sdkCancel()
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			sdkCancel()
		}()
		srv := sdk.NewSDKServer(eng, os.Stdin, os.Stdout)
		if err := srv.Serve(sdkCtx); err != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupSDKError, err))
			_ = eng.Shutdown(context.Background())
			os.Exit(1)
		}
		return
	}

	// Construct the interactive input owner only after the print and SDK
	// transports have been routed away. This prevents multiple scanners from
	// ever reading stdin in the same process.
	var screenReader *ui.ScreenReaderRenderer
	if opts.ScreenReader {
		screenReader = ui.NewScreenReaderRenderer(os.Stdout, os.Stdin)
	}

	// ── Common session setup ───────────────────────────────────────────────
	// Resume the session in the engine if requested.
	if resolvedSession.Resumed {
		count, resumeErr := eng.Resume(context.Background(), sessionID)
		if resumeErr != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupResumeWarning, sessionID, resumeErr))
		} else {
			if saver, ok := eng.Sessions().(engine.SessionContextSaver); ok {
				_ = saver.SaveSessionContext(sessionID, cwd, detectGitBranch(cwd))
			}
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupResumed, sessionID, count))
			if meta, _, metaErr := sessionRepo.GetMeta(sessionID, sessionProjectDir); metaErr == nil && meta.Provider != "" {
				currentProvider := pRef.Name()
				if meta.Provider != currentProvider {
					fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupProviderMismatch,
						meta.Provider, meta.Model, currentProvider, pRef.ModelID()))
				}
			}
		}
	}
	deps.TeamManager.SetProjectRoot(cwd)
	deps.RuntimeScope.SetTeamNameFunc(deps.TeamManager.CurrentTeamName)
	switcher := &sessionSwitcher{
		repo:                 sessionRepo,
		deps:                 deps,
		eng:                  eng,
		sessionID:            &sessionID,
		sessionProjectDir:    &sessionProjectDir,
		cwd:                  &cwd,
		hookRunnerRef:        &hookRunner,
		systemPromptOverride: opts.SystemPrompt,
		extraAllowedDirs:     opts.AllowedDirs,
	}

	// Signal handling
	globalCtx, globalCancel := context.WithCancel(context.Background())
	defer globalCancel()

	sigHandler := NewSignalHandler(globalCancel)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go sigHandler.Listen(sigCh)
	if screenReader != nil {
		checker.SetStructuredPromptFunc(screenReader.DecisionRequest)
		if readerErr := RunScreenReaderREPL(globalCtx, TUIREPLConfig{
			Engine: eng, Repo: sessionRepo, SessionID: &sessionID, SessionProjectDir: &sessionProjectDir,
			CWD: &cwd, HookRunnerRef: &hookRunner, SwitchSession: switcher.switchTo, PublishSessionID: deps.PublishSessionID,
			ProviderRef: pRef, ProviderRegistry: providerRegistry, CredentialStore: credStore,
			PermChecker: checker, PlanState: deps.PlanState, AskUserQuestionTool: deps.AskUserQuestionTool, RuntimeScope: deps.RuntimeScope,
			TaskCreateTool: deps.TaskCreateTool, AgentTool: deps.AgentTool, BackgroundTasks: deps.BackgroundTasks, MCPBackend: deps.ServiceMCP,
			SkillManager: deps.SkillManager, SkillInvoker: skillInvoker,
			ReasoningEffort: reasoningEffort,
			BuildDiagnostic: buildinfo.Current,
			OpenSessionTerminal: func(ctx context.Context, forkID, forkCWD, providerName, modelID string) error {
				return openForkSessionTerminal(ctx, opts, providerName, modelID, forkID, forkCWD)
			},
		}, screenReader, sigHandler); readerErr != nil {
			fmt.Fprint(os.Stderr, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyStartupScreenReaderError, readerErr))
			_ = eng.Shutdown(context.Background())
			os.Exit(1)
		}
		return
	}

	// ── Interactive TUI mode (default) ──────────────────────────────────────
	if tuiErr := RunTUIREPL(globalCtx, TUIREPLConfig{
		Engine:              eng,
		Repo:                sessionRepo,
		SessionID:           &sessionID,
		SessionProjectDir:   &sessionProjectDir,
		CWD:                 &cwd,
		HookRunnerRef:       &hookRunner,
		SwitchSession:       switcher.switchTo,
		PublishSessionID:    deps.PublishSessionID,
		ProviderRef:         pRef,
		ProviderRegistry:    providerRegistry,
		CredentialStore:     credStore,
		PermChecker:         checker,
		PlanState:           deps.PlanState,
		AskUserQuestionTool: deps.AskUserQuestionTool,
		RuntimeScope:        deps.RuntimeScope,
		TaskCreateTool:      deps.TaskCreateTool,
		AgentTool:           deps.AgentTool,
		BackgroundTasks:     deps.BackgroundTasks,
		MCPBackend:          deps.ServiceMCP,
		SkillManager:        deps.SkillManager,
		SkillInvoker:        skillInvoker,
		ReasoningEffort:     reasoningEffort,
		BuildDiagnostic:     buildinfo.Current,
		OpenSessionTerminal: func(ctx context.Context, forkID, forkCWD, providerName, modelID string) error {
			return openForkSessionTerminal(ctx, opts, providerName, modelID, forkID, forkCWD)
		},
	}, sigHandler); tuiErr != nil {
		fmt.Fprint(os.Stderr, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyStartupTUIError, tuiErr))
		_ = eng.Shutdown(context.Background())
		os.Exit(1)
	}
}

func isBootstrappableProviderError(registry *provider.ProviderRegistry, providerName string, err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "unknown provider") {
		return false
	}
	if providerName == "" {
		return true
	}
	normalized := provider.CanonicalProviderName(providerName)
	_, ok := registry.Get(normalized)
	return ok
}

func bootstrapProviderSelection(registry *provider.ProviderRegistry, providerName, modelName string) (string, string) {
	name := strings.ToLower(strings.TrimSpace(providerName))
	if name == "" {
		name = os.Getenv("PROVIDER")
	}
	if name == "" && os.Getenv("CLAUDE_CODE_USE_BEDROCK") == "1" {
		name = "bedrock"
	}
	if name == "" && os.Getenv("CLAUDE_CODE_USE_VERTEX") == "1" {
		name = "vertex"
	}
	if name == "" && os.Getenv("DEEPSEEK_API_KEY") != "" {
		name = brand.DeepSeekProvider
	}
	if name == "" && os.Getenv("OPENAI_API_KEY") != "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		name = "openai"
	}
	if name == "" && os.Getenv("ANTHROPIC_API_KEY") != "" {
		name = "anthropic"
	}
	if name == "" && os.Getenv("OAUTH_ACCESS_TOKEN") != "" {
		name = "anthropic"
	}
	if name == "" {
		name = brand.DefaultProvider
	}
	name = provider.CanonicalProviderName(name)

	model := strings.TrimSpace(modelName)
	if info, ok := registry.Get(name); ok {
		if model == "" {
			model = info.DefaultModel
		}
		return name, model
	}
	if model == "" {
		model = provider.CatalogDefaultModel(name, brand.DeepSeekDefaultModel)
	}
	return name, model
}
