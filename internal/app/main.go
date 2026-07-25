package app

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

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/auth"
	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	toolshell "github.com/agent-dance/luban/internal/tools/shell"
	toolskill "github.com/agent-dance/luban/internal/tools/skill"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/sdk"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
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
	deps.SkillTool.LoadedLedgerResolver = func(ctx context.Context, sessionID string, id skills.SkillID) toolskill.SkillLoadedLedgerState {
		if exec, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
			// A model execution must resolve against the QueryLoop that created
			// the immutable tool context. Falling through to CoreEngine here would
			// accidentally route Agent/Team child loops through a same-ID parent
			// conversation (or report every child invocation as a full body).
			if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(exec.SessionID) != strings.TrimSpace(sessionID) {
				return toolskill.SkillLoadedLedgerState{}
			}
			state, resolved := exec.ResolveSkillLoadedLedger(string(id))
			if !resolved {
				return toolskill.SkillLoadedLedgerState{}
			}
			return toolskill.SkillLoadedLedgerState{
				ContextEpoch:       state.ContextEpoch,
				LoadedContextEpoch: state.LoadedContextEpoch,
				ContentDigest:      skills.SkillDigest(state.ContentDigest),
				PayloadDigest:      skills.InvocationPayloadDigest(state.PayloadDigest),
			}
		}
		// Explicit-user UI invocations have no tool execution context and may
		// consult only an idle, exact CoreEngine conversation.
		state := eng.ResolveSkillLoadedLedger(ctx, sessionID, id)
		return toolskill.SkillLoadedLedgerState{
			ContextEpoch:       state.ContextEpoch,
			LoadedContextEpoch: state.LoadedContextEpoch,
			ContentDigest:      state.ContentDigest,
			PayloadDigest:      state.PayloadDigest,
		}
	}
	return commands.SkillInvokerFunc(func(ctx context.Context, request commands.SkillInvocationRequest) (types.ToolResult, error) {
		return deps.SkillTool.Invoke(ctx, toolskill.SkillInvocationRequest{
			SessionID: request.SessionID, Selector: request.Selector,
			ExpectedRevision:          request.ExpectedRevision,
			ExpectedProjectGeneration: request.ExpectedProjectGeneration,
			Origin:                    skills.InvocationOriginUser,
			Arguments:                 request.Arguments,
		})
	})
}

// prepareInitialRegistryRuntime is the startup gate for all staged workspace
// dependencies, including MCP settings and skill policy documents. main must
// not construct an Engine when this fails: doing so would expose tools without
// their validated workspace configuration.
func prepareInitialRegistryRuntime(deps *RegistryDeps, cwd string, allowedDirs []string) error {
	if deps == nil {
		return skills.ErrSkillOverrideStoreMissing
	}
	return deps.UpdateSessionContext(cwd, allowedDirs)
}

// loadHooks loads project hooks. Errors are non-fatal so the CLI works without
// any hooks.
func loadHooks(cwd string) *hooks.Runner {
	brandDir := filepath.Join(cwd, brand.ConfigDirName)
	brandSettings, _ := hooks.LoadFromSettings(filepath.Join(brandDir, "settings.json"))
	brandHooks, _ := hooks.LoadFromDir(filepath.Join(brandDir, "hooks"))
	return brandSettings.Merge(brandHooks)
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

func newOutputRenderer(opts cli.Options, output io.Writer) (presentation.Renderer, error) {
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

// Run executes the luban-code application and returns the process exit code.
// The command entrypoint is the sole owner of process termination.
func Run() (exitCode int) {
	invocation, parseErr := cli.ParseInvocation(os.Args[1:])
	if parseErr != nil {
		fmt.Fprint(os.Stderr, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCLIError, parseErr))
		return 2
	}
	if invocation.Action != cli.EarlyActionNone {
		return cli.RunEarlyAction(invocation, os.Stdout, os.Stderr)
	}
	opts := invocation.Options
	lang := i18n.DetectOrLoadLanguage()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LUBAN_CODE_SCREEN_READER")), "true") || os.Getenv("LUBAN_CODE_SCREEN_READER") == "1" {
		opts.ScreenReader = true
	}
	stdinTerminal := cli.IsStdinTerminal()
	if err := cli.ValidateInputMode(opts, stdinTerminal); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		return 2
	}
	if err := prepareInputTransport(&opts, stdinTerminal, os.Stdin); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		return 1
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
		return 1
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
			return 1
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
			return 1
		}
		defer func() {
			if closeErr := debugFile.Close(); closeErr != nil {
				issue := i18n.WrapInternalError(i18n.KeyStartupShutdownDebugFile, closeErr)
				fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupShutdownWarning, issue))
				exitCode = applicationExitCodeAfterShutdown(exitCode, []error{issue})
			}
		}()
		_, _ = fmt.Fprintln(debugFile, i18n.Format(lang, i18n.KeyLogDebugSessionStarted, time.Now().UTC().Format(time.RFC3339Nano)))
		pRef.SetDebugObserver(provider.NewDebugWriterObserver(debugFile))
	}

	sessionRepo := session.DefaultRepository()
	resolvedSession, resolveSessionErr := ResolveSession(opts.SessionID, opts.Resume, sessionRepo, cwd, os.Stderr)
	if resolveSessionErr != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupSessionFatal, resolveSessionErr))
		return 1
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
			return 1
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
	interactive := !opts.Print && !opts.SDK
	if interactive {
		_, restoreDiagnosticLogger := installInteractiveDiagnosticLogger()
		defer restoreDiagnosticLogger()
	}
	deps := SetupRegistry(pRef, cwd, allowedDirs, sb, webDomains, interactive)
	var eng *engine.CoreEngine
	// The lifecycle owner is installed immediately after composition. This also
	// covers failures while preparing the initial workspace, after SetupRegistry
	// has already created background cache and MCP resources.
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), applicationShutdownTimeout)
		defer shutdownCancel()
		issues := shutdownApplicationRuntime(shutdownCtx, deps, eng)
		if shutdownErr := joinApplicationShutdownIssues(issues); shutdownErr != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupShutdownWarning, shutdownErr))
		}
		exitCode = applicationExitCodeAfterShutdown(exitCode, issues)
	}()
	if err := prepareInitialRegistryRuntime(deps, cwd, allowedDirs); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		return 1
	}
	deps.BindSessionIdentity(sessionID)
	deps.SetGoalRuntime(newDynamicSessionGoalRuntime(
		sessionRepo,
		func() string { return sessionID },
		func() string { return sessionProjectDir },
	))
	if opts.AllowedTools != "" {
		deps.RuntimeScope.SetAllowedTools(splitAndTrim(opts.AllowedTools, ","))
	}
	if opts.DisallowedTools != "" {
		deps.RuntimeScope.SetDeniedTools(splitAndTrim(opts.DisallowedTools, ","))
	}
	if err := deps.AgentTool.SetInlineProfilesFromJSON(opts.Agents); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		return 1
	}

	// Build system prompt
	systemPrompt := buildSystemPromptForCWD(opts.SystemPrompt, deps.Registry, cwd)

	// Wire the system prompt into the agent runtime.
	deps.AgentTool.System = systemPrompt

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

	// Build permission handler. Interactive sessions start in Auto mode; print
	// and SDK transports retain their permission bridge unless --allow-all was
	// explicitly requested.
	// When a real sandbox backend is active, SandboxAwarePermissionHandler binds
	// permission decisions to that executable capability. Auto-approval remains
	// disabled unless the backend also proves protected-path enforcement.
	// Both modes use permissions.Checker so that --allowed-tools / --disallowed-tools
	// whitelist/blacklist is enforced regardless of mode (bypass-immune).
	var permHandler permission.PermissionHandler
	var checker *permissions.Checker

	checkerMode := initialPermissionCheckerMode(opts)
	checker = permissions.NewChecker(checkerMode, nil)
	if checkerMode != permissions.ModeAllowAll {
		checker.SetStructuredPromptFunc(permissions.NewRichPrompt(os.Stderr, os.Stdin).DecisionRequest)
	}
	if err := bindPlanModePermissionDispatcher(deps.RuntimeScope, deps.PlanState, checker); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		return 1
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
	deps.AgentTool.PermissionHandler = permHandler

	// Inject safety dependencies: wire tools package functions into permissions package
	// to avoid circular imports. This must happen before any permission checks.
	permissions.SetSafetyConfig(permissions.SafetyConfig{
		ShellPolicyAnalyzer: toolshell.AnalyzeShellCommand,
	})

	eng, err = engine.New(engine.Config{
		Provider:              pRef,
		Registry:              deps.Registry,
		Sessions:              engine.NewRepositorySessionManager(sessionRepo, func() string { return sessionProjectDir }),
		SystemPrompt:          systemPrompt,
		HookRunner:            hookRunner,
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
		BackgroundTasks:       appBackgroundTaskCompactAdapter{source: deps.BackgroundTasks},
		MCPState:              deps,
		AgentDefinitions:      appAgentDefinitionCompactAdapter{source: deps.AgentTool},
	})
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		return 1
	}
	skillInvoker := configureSkillRuntime(deps, eng)
	configureWorktreeSessionRuntime(deps, eng, &cwd, &hookRunner, opts.SystemPrompt, opts.AllowedDirs)
	if err := deps.StartSchedule(context.Background()); err != nil {
		fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, err))
		return 1
	}

	// ── TTY detection (Task 2b) ─────────────────────────────────────────────
	// Piped stdout → disable colors.
	if !cli.IsStdoutTerminal() {
		os.Setenv("NO_COLOR", "1")
	}

	// ── Print mode (-p) ─────────────────────────────────────────────────────
	if opts.Print {
		renderer, rendererErr := newOutputRenderer(opts, os.Stdout)
		if rendererErr != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupFatal, rendererErr))
			return 2
		}
		exitCode := RunPrintMode(eng, renderer, strings.Join(opts.Args, " "), opts.Verbose)
		return exitCode
	}

	// ── SDK mode (--sdk) ────────────────────────────────────────────────────
	if opts.SDK {
		sdkCtx, stopSDKSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopSDKSignals()
		srv := sdk.NewSDKServer(newSDKRuntime(eng), os.Stdin, os.Stdout, initialSDKPermissionMode(opts))
		if err := srv.Serve(sdkCtx); err != nil {
			fmt.Fprint(os.Stderr, i18n.Format(lang, i18n.KeyStartupSDKError, err))
			return 1
		}
		return 0
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
	sigHandler.Start(globalCtx, sigCh)
	defer func() {
		signal.Stop(sigCh)
		sigHandler.StopAndWait()
	}()
	if screenReader != nil {
		checker.SetStructuredPromptFunc(screenReader.DecisionRequest)
		if readerErr := RunScreenReaderREPL(globalCtx, TUIREPLConfig{
			Engine: eng, Repo: sessionRepo, SessionID: &sessionID, SessionProjectDir: &sessionProjectDir,
			CWD: &cwd, HookRunnerRef: &hookRunner, SwitchSession: switcher.switchTo, PublishSessionID: deps.PublishSessionID,
			ProviderRef: pRef, ProviderRegistry: providerRegistry, CredentialStore: credStore,
			PermChecker: checker, PlanState: deps.PlanState, AskUserQuestionTool: deps.AskUserQuestionTool, RuntimeScope: deps.RuntimeScope,
			TaskCreateTool: deps.TaskCreateTool, AgentTool: deps.AgentTool, BackgroundTasks: agentBackgroundPresentationPort(deps.BackgroundTasks), MCPBackend: deps.ServiceMCP,
			SkillManager: deps.SkillManager, SkillInvoker: skillInvoker,
			ReasoningEffort: reasoningEffort,
			BuildDiagnostic: buildinfo.Current,
			OpenSessionTerminal: func(ctx context.Context, forkID, forkCWD, providerName, modelID string) error {
				return openForkSessionTerminal(ctx, opts, providerName, modelID, forkID, forkCWD)
			},
		}, screenReader, sigHandler); readerErr != nil {
			fmt.Fprint(os.Stderr, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyStartupScreenReaderError, readerErr))
			return 1
		}
		return 0
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
		BackgroundTasks:     agentBackgroundPresentationPort(deps.BackgroundTasks),
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
		return 1
	}
	return 0
}

func initialSDKPermissionMode(opts cli.Options) sdk.InitialPermissionMode {
	if opts.AllowAll {
		return sdk.InitialPermissionFullAuto
	}
	return sdk.InitialPermissionBridge
}

func initialPermissionCheckerMode(opts cli.Options) permissions.Mode {
	if opts.AllowAll || (!opts.Print && !opts.SDK) {
		return permissions.ModeAllowAll
	}
	return permissions.ModeAskAlways
}

// applyInputTransportDefaults selects print mode only for an otherwise
// unclaimed pipe. Its return value records that print mode was implicit, which
// is the only mode allowed to consume a prompt from stdin.
func applyInputTransportDefaults(opts *cli.Options, stdinTerminal bool) bool {
	if opts == nil || stdinTerminal || opts.Print || opts.SDK {
		return false
	}
	opts.Print = true
	return true
}

func prepareInputTransport(opts *cli.Options, stdinTerminal bool, input io.Reader) error {
	implicitPrint := applyInputTransportDefaults(opts, stdinTerminal)
	if !implicitPrint || len(opts.Args) > 0 {
		return nil
	}
	prompt, err := cli.ReadPipedPrompt(input)
	if err != nil {
		return err
	}
	opts.Args = []string{prompt}
	return nil
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
	if name == "" && os.Getenv("LUBAN_CODE_USE_BEDROCK") == "1" {
		name = "bedrock"
	}
	if name == "" && os.Getenv("LUBAN_CODE_USE_VERTEX") == "1" {
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
