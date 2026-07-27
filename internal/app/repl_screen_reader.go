package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/runtimeevent"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

func RunScreenReaderREPL(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, sigHandler *SignalHandler) (runErr error) {
	if cfg.Engine == nil || cfg.SessionID == nil || renderer == nil {
		return replError(i18n.KeyREPLErrorScreenReaderNotConfigured)
	}
	defer func() {
		if closeErr := renderer.Close(); runErr == nil && closeErr != nil {
			runErr = replWrap(i18n.KeyREPLErrorCloseScreenReaderInput, closeErr)
		}
	}()
	if cfg.SessionTransitionMu == nil {
		cfg.SessionTransitionMu = &sync.Mutex{}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if cfg.AskUserQuestionTool != nil {
		cfg.AskUserQuestionTool.SetInteractionRequester(renderer)
		defer cfg.AskUserQuestionTool.SetInteractionRequester(nil)
	}
	if hookRunner := currentHookRunner(cfg); hookRunner != nil {
		hookContext := newREPLHookContext(*cfg.SessionID, hooks.HookSessionStart, "repl", "system")
		runObservedREPLHooks(ctx, hookRunner, renderer, hooks.HookSessionStart, hooks.HookInput{}, hookContext)
	}
	defer func() {
		if hookRunner := currentHookRunner(cfg); hookRunner != nil {
			hookContext := newREPLHookContext(*cfg.SessionID, hooks.HookSessionEnd, "repl", "system")
			runObservedREPLHooks(context.Background(), hookRunner, renderer, hooks.HookSessionEnd, hooks.HookInput{}, hookContext)
		}
	}()

	tracker := ui.NewCostTracker(cfg.Engine.Provider().ModelID())
	tracker.SetProvider(cfg.Engine.Provider().Name())
	if catalog := getCatalog(cfg); catalog != nil {
		tracker.SetCatalog(catalog)
	}
	if err := restoreScreenReaderLifecycle(cfg, tracker); err != nil {
		return err
	}
	visibleBackgroundRenderer := screenReaderSessionInfoRenderer{ScreenReaderRenderer: renderer, sessionID: cfg.SessionID}
	unbindBackgroundNotifications := installTUIBackgroundNotifications(cfg.BackgroundTasks, visibleBackgroundRenderer, func(_ context.Context, notification agentcontract.RuntimeNotification) error {
		return runScreenReaderBackgroundFollowUp(ctx, cfg, renderer, tracker, notification)
	})
	defer unbindBackgroundNotifications()
	renderer.SetDecisionRecorder(screenReaderDecisionRecorder(cfg))
	defer renderer.SetDecisionRecorder(nil)
	renderer.SetSessionIdentityResolver(func() string {
		if cfg.SessionID == nil {
			return ""
		}
		cfg.SessionTransitionMu.Lock()
		defer cfg.SessionTransitionMu.Unlock()
		return *cfg.SessionID
	})
	defer renderer.SetSessionIdentityResolver(nil)
	defer func() {
		if err := persistScreenReaderLifecycle(cfg, tracker); err != nil {
			renderer.Warning(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLTUIErrorPrefix, err))
		}
	}()
	renderer.Banner(cfg.Engine.Provider().Name(), cfg.Engine.Provider().ModelID())
	renderer.SessionInfo(*cfg.SessionID, cfg.Engine.Tools())
	if messages, err := cfg.Engine.Sessions().Load(*cfg.SessionID); err == nil && len(messages) > 0 {
		renderScreenReaderTranscript(renderer, messages, presentation.ToolEventContext{SessionID: *cfg.SessionID, ProjectRoot: currentRuntimeProjectRoot(cfg)}, screenReaderControlScope(cfg, *cfg.SessionID))
	}

	for {
		input, err := renderer.ReadCommand(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				renderer.Goodbye()
				return nil
			}
			return err
		}
		trimmedInput := strings.TrimSpace(input)
		if trimmedInput == "" {
			continue
		}
		// Preserve trailing whitespace for slash skills: `/name` means omitted
		// arguments while `/name ` is an explicitly supplied empty argument.
		// Ordinary prompts use the canonical fully-trimmed input form.
		if strings.HasPrefix(strings.TrimLeftFunc(input, unicode.IsSpace), "/") {
			input = strings.TrimLeftFunc(input, unicode.IsSpace)
		} else {
			input = trimmedInput
		}
		if handled, exit, commandErr := handleScreenReaderCommand(ctx, cfg, renderer, tracker, input, sigHandler); handled {
			if commandErr != nil && !isScreenReaderPresentedCommandError(commandErr) {
				renderer.Error(commandErr.Error())
			}
			if exit {
				renderer.Goodbye()
				return nil
			}
			continue
		}
		if hookRunner := currentHookRunner(cfg); hookRunner != nil {
			hookContext := newREPLHookContext(*cfg.SessionID, hooks.HookUserPromptSubmit, "user", "local")
			result := runObservedREPLHooks(ctx, hookRunner, renderer, hooks.HookUserPromptSubmit, hooks.HookInput{UserInput: input}, hookContext)
			if result.Blocked {
				renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLInputBlocked, result.Reason))
				continue
			}
		}

		projectRoot := currentRuntimeProjectRoot(cfg)
		req, _ := buildGoalActivationQueryRequest(*cfg.SessionID, input, currentCWD(cfg), projectRoot)
		req.SessionProjectDir = currentProjectDir(cfg)
		runScreenReaderQuery(ctx, cfg, renderer, tracker, req, sigHandler)
	}
}

type screenReaderPresentedCommandError struct{ err error }

func (e screenReaderPresentedCommandError) Error() string { return e.err.Error() }
func (e screenReaderPresentedCommandError) Unwrap() error { return e.err }

func isScreenReaderPresentedCommandError(err error) bool {
	var presented screenReaderPresentedCommandError
	return errors.As(err, &presented)
}

type screenReaderSessionInfoRenderer struct {
	*ui.ScreenReaderRenderer
	sessionID *string
}

func (r screenReaderSessionInfoRenderer) VisibleSessionID() string {
	if r.sessionID == nil {
		return ""
	}
	return *r.sessionID
}

func runScreenReaderBackgroundFollowUp(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, tracker *ui.CostTracker, notification agentcontract.RuntimeNotification) error {
	followUpEngine, ok := cfg.Engine.(engine.FollowUpEngine)
	if !ok {
		return replError(i18n.KeyREPLErrorFollowUpUnsupported)
	}
	if cfg.BackgroundTasks == nil || renderer == nil {
		return replError(i18n.KeyREPLErrorFollowUpUnavailable)
	}
	target, ok := cfg.BackgroundTasks.NotificationFollowUpTarget(notification)
	if !ok {
		return replError(i18n.KeyREPLErrorFollowUpTaskUnresolved, notification.TaskID)
	}
	if !backgroundFollowUpProjectMatches(cfg.BackgroundTasks, target.ProjectRoot) {
		return replError(i18n.KeyREPLErrorFollowUpUnavailable)
	}
	renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLBackgroundStarted, target.SessionID, notification.TaskID))
	ch, err := followUpEngine.QueryFollowUp(ctx, engine.QueryRequest{
		SessionID: target.SessionID, SessionProjectDir: target.SessionProjectDir,
		Message: target.Message, CWD: target.ProjectRoot, ProjectRoot: target.ProjectRoot,
		RuntimeEventID: notification.ID, InternalControlCapability: messagecontrol.Runtime(),
	})
	if err != nil {
		if errors.Is(err, engine.ErrSessionDeleted) {
			renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLBackgroundDiscarded, notification.TaskID))
			return nil
		}
		renderer.Error(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLBackgroundFailed, target.SessionID, err))
		return err
	}
	eventTracker := tracker
	if cfg.SessionID == nil || target.SessionID != *cfg.SessionID {
		eventTracker = nil
	}
	handler := makeScreenReaderEventHandler(renderer, eventTracker, func() (int, int) {
		usage, usageErr := cfg.Engine.ContextUsage(target.SessionID)
		if usageErr != nil || usage == nil {
			return 0, 0
		}
		return usage.TotalTokens, usage.UsedTokens
	}, presentation.ToolEventContext{
		SessionID: target.SessionID, ProjectRoot: target.ProjectRoot, ActorID: "background", ActorType: "background", WorkUnitID: notification.TaskID,
	})
	var runErr error
	for event := range ch {
		if event.Final {
			runErr = event.Error
			continue
		}
		handler(event.Inner)
	}
	renderer.Newline()
	if errors.Is(runErr, engine.ErrSessionDeleted) {
		renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLBackgroundDiscarded, notification.TaskID))
		return nil
	}
	if runErr != nil {
		renderer.Error(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLBackgroundFailed, target.SessionID, runErr))
		return runErr
	}
	if usageEvent, ok := backgroundNotificationUsageEvent(notification); ok && eventTracker != nil {
		handler(usageEvent)
	}
	renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLBackgroundCompleted, target.SessionID))
	return nil
}

func handleScreenReaderCommand(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, tracker *ui.CostTracker, input string, signalHandlers ...*SignalHandler) (handled, exit bool, err error) {
	fields := strings.Fields(input)
	command := strings.ToLower(fields[0])
	switch command {
	case "/exit", "/quit":
		if err := runScreenReaderCommandOperation(renderer, "exit", "exit", "", commands.CommandOutcomeExitRequested, nil); err != nil {
			return true, false, err
		}
		return true, true, nil
	case "/help":
		if err := runScreenReaderDiagnosticCommand(cfg, renderer, input); err != nil {
			return true, false, err
		}
		renderer.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLHelpScreenReader))
		return true, false, nil
	case "/goal":
		var objective string
		if err := runScreenReaderDiagnosticCommand(cfg, renderer, input, func(value string) { objective = value }); err != nil {
			return true, false, err
		}
		if cfg.Engine != nil {
			req, ok := buildGoalActivationQueryRequest(*cfg.SessionID, objective, currentCWD(cfg), currentRuntimeProjectRoot(cfg))
			if ok {
				req.SessionProjectDir = currentProjectDir(cfg)
				var sigHandler *SignalHandler
				if len(signalHandlers) > 0 {
					sigHandler = signalHandlers[0]
				}
				runScreenReaderQuery(ctx, cfg, renderer, tracker, req, sigHandler)
			}
		}
		return true, false, nil
	case "/skills", "/mcp", "/doctor", "/diagnose":
		return true, false, runScreenReaderDiagnosticCommand(cfg, renderer, input)
	case "/compact":
		return true, false, runScreenReaderCommandOperation(renderer, "compact", "compact", "", commands.CommandOutcomeSucceeded, func() error {
			projectRoot := currentRuntimeProjectRoot(cfg)
			handle := makeScreenReaderEventHandler(renderer, tracker, func() (int, int) {
				usage, usageErr := cfg.Engine.ContextUsage(*cfg.SessionID)
				if usageErr != nil || usage == nil {
					return 0, 0
				}
				return usage.TotalTokens, usage.UsedTokens
			}, presentation.ToolEventContext{SessionID: *cfg.SessionID, ProjectRoot: projectRoot})
			return runManualCompactionEventsInLanguage(ctx, cfg.Engine, *cfg.SessionID, "", i18n.DetectOrLoadLanguage(), handle)
		})
	case "/export":
		target := ""
		if len(fields) == 2 {
			target = fields[1]
		}
		return true, false, runScreenReaderCommandOperation(renderer, "export", "export", target, commands.CommandOutcomeSucceeded, func() error {
			if len(fields) != 2 {
				return replError(i18n.KeyREPLErrorUsage, "/export PATH")
			}
			return exportScreenReaderSession(cfg, *cfg.SessionID, fields[1], renderer)
		})
	case "/clear":
		action := "conversation"
		if len(fields) == 2 && strings.EqualFold(fields[1], "view") {
			action = "view"
		}
		return true, false, runScreenReaderCommandOperation(renderer, "clear", action, "", commands.CommandOutcomeSucceeded, func() error {
			if action == "view" {
				renderer.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLClearViewReceipt))
				return nil
			}
			if len(fields) != 1 {
				return replError(i18n.KeyREPLErrorUsage, "/clear [view]")
			}
			return clearScreenReaderConversation(ctx, cfg, renderer, tracker)
		})
	case "/resume", "/r":
		target := ""
		if len(fields) == 2 {
			target = fields[1]
		}
		return true, false, runScreenReaderCommandOperation(renderer, "resume", "load", target, commands.CommandOutcomeSucceeded, func() error {
			if len(fields) != 2 {
				return replError(i18n.KeyREPLErrorUsage, "/resume SESSION_ID")
			}
			return resumeScreenReaderSession(ctx, cfg, renderer, tracker, fields[1])
		})
	case "/session":
		if len(fields) < 2 || !strings.EqualFold(fields[1], "load") {
			return true, false, runScreenReaderDiagnosticCommand(cfg, renderer, input)
		}
		target := ""
		if len(fields) == 3 {
			target = fields[2]
		}
		return true, false, runScreenReaderCommandOperation(renderer, "session", "load", target, commands.CommandOutcomeSucceeded, func() error {
			if len(fields) != 3 {
				return replError(i18n.KeyREPLErrorUsage, "/session load SESSION_ID")
			}
			return resumeScreenReaderSession(ctx, cfg, renderer, tracker, target)
		})
	case "/fork":
		action, target := "list", ""
		if len(fields) > 1 {
			action, target = "select", fields[1]
		}
		return true, false, runScreenReaderCommandOperation(renderer, "fork", action, target, commands.CommandOutcomeSucceeded, func() error {
			return forkScreenReaderSession(ctx, cfg, renderer, fields)
		})
	case "/delete-history":
		if len(fields) != 2 {
			return true, false, replError(i18n.KeyREPLErrorUsage, "/delete-history SESSION_ID")
		}
		return true, false, deleteScreenReaderSessionHistory(ctx, cfg, renderer, fields[1])
	case "/mode":
		if len(fields) != 2 {
			return true, false, replError(i18n.KeyREPLErrorUsage, "/mode auto|ask|plan")
		}
		mode := tui.ModeAskEdit
		switch strings.ToLower(fields[1]) {
		case "auto":
			mode = tui.ModeAutoEdit
		case "ask":
			mode = tui.ModeAskEdit
		case "plan":
			mode = tui.ModePlanEdit
		default:
			return true, false, replError(i18n.KeyREPLErrorUsage, "/mode auto|ask|plan")
		}
		if err := applyTUISessionPermissionMode(cfg, mode); err != nil {
			return true, false, err
		}
		if err := persistScreenReaderLifecycle(cfg, tracker); err != nil {
			return true, false, err
		}
		renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLModeReceipt, strings.ToLower(mode.String())))
		return true, false, nil
	default:
		if strings.HasPrefix(command, "/") {
			registry := commands.NewRegistry()
			commands.RegisterBuiltins(registry)
			if registered, _ := registry.Parse(input); registered != nil {
				return true, false, runScreenReaderDiagnosticCommand(cfg, renderer, input)
			}
			submission := tui.InvokeUserSkillSlash(
				ctx,
				cfg.SkillManager,
				cfg.SkillInvoker,
				screenReaderSessionID(cfg),
				input,
			)
			if !submission.Successful() {
				renderer.Error(formatScreenReaderSkillSlashFailure(i18n.DetectOrLoadLanguage(), submission))
				return true, false, nil
			}

			// ModelContent is the versioned user-role SKILL.md envelope. It is
			// deliberately never presented through Info/Error or appended as a
			// human transcript message; only the model receives it.
			var sigHandler *SignalHandler
			if len(signalHandlers) > 0 {
				sigHandler = signalHandlers[0]
			}
			projectRoot := currentRuntimeProjectRoot(cfg)
			runScreenReaderQuery(ctx, cfg, renderer, tracker, engine.QueryRequest{
				SessionID:                 screenReaderSessionID(cfg),
				SessionProjectDir:         currentProjectDir(cfg),
				Message:                   submission.ModelContent,
				InternalKind:              types.InternalMessageKindSkillInvocation,
				InternalControlCapability: messagecontrol.Runtime(),
				CWD:                       currentCWD(cfg),
				ProjectRoot:               projectRoot,
			}, sigHandler)
			return true, false, nil
		}
		return false, false, nil
	}
}

func screenReaderSessionID(cfg TUIREPLConfig) string {
	if cfg.SessionID == nil {
		return ""
	}
	return strings.TrimSpace(*cfg.SessionID)
}

func formatScreenReaderSkillSlashFailure(lang i18n.Language, submission tui.SkillSlashSubmission) string {
	selector := strings.TrimSpace(submission.RequestedSelector)
	command := "/" + selector
	if selector == "" {
		command = "/"
	}
	switch submission.Outcome {
	case tui.SkillSlashInvalidInput:
		return i18n.Format(lang, i18n.KeyScreenReaderSkillInvalidSelector, command)
	case tui.SkillSlashBackendUnavailable:
		return i18n.Text(lang, i18n.KeyScreenReaderSkillCatalogUnavailable)
	case tui.SkillSlashSnapshotFailed:
		return i18n.Format(lang, i18n.KeyScreenReaderSkillLookupFailed, submission.Err)
	case tui.SkillSlashNotFound:
		return i18n.Format(lang, i18n.KeyScreenReaderSkillNotFound, command)
	case tui.SkillSlashAmbiguous:
		ids := make([]string, len(submission.Candidates))
		for index, id := range submission.Candidates {
			ids[index] = string(id)
		}
		return i18n.Format(lang, i18n.KeyScreenReaderSkillAmbiguous, command, strings.Join(ids, ", "))
	case tui.SkillSlashPolicyDenied:
		return i18n.Format(lang, i18n.KeyScreenReaderSkillUnavailable, command)
	case tui.SkillSlashInvokerUnavailable:
		return i18n.Text(lang, i18n.KeyScreenReaderSkillInvokerUnavailable)
	case tui.SkillSlashInvocationFailed:
		return i18n.Format(lang, i18n.KeyScreenReaderSkillInvocationFailed, command, submission.Err)
	case tui.SkillSlashInvocationRejected:
		if content := strings.TrimSpace(submission.ToolResult.TextContent()); content != "" {
			return content
		}
		return i18n.Format(lang, i18n.KeyScreenReaderSkillInvocationRejected, command)
	case tui.SkillSlashEmptyEnvelope:
		return i18n.Format(lang, i18n.KeyScreenReaderSkillEmptyEnvelope, command)
	default:
		return i18n.Format(lang, i18n.KeyScreenReaderSkillInvocationRejected, command)
	}
}

// runScreenReaderCommandOperation emits one typed lifecycle around an
// append-only screen-reader operation. Session transitions and terminal I/O
// remain owned by the line-mode runtime so their ordering stays observable.
func runScreenReaderCommandOperation(renderer *ui.ScreenReaderRenderer, command, action, target string, success commands.CommandOutcome, run func() error) error {
	contract, _ := commands.LookupCommandPresentationContract(command)
	risk := contract.Risk
	if command == "clear" && action == "view" {
		risk = commands.CommandRiskLow
	}
	base := commands.CommandPresentation{
		Version: 2, Command: command, Family: contract.Family, Action: action,
		Target: commands.RedactCommandPresentationText(target, 160), Summary: "/" + command + " " + action,
		Display: contract.Display, Risk: risk, OutcomeReliable: true,
	}
	running := base
	running.State = commands.CommandStateRunning
	running.Outcome = commands.CommandOutcomeUnknown
	renderScreenReaderCommandPresentation(renderer, running)

	var err error
	if run != nil {
		err = run()
	}
	terminal := base
	terminal.State = commands.CommandStateCompleted
	terminal.Outcome = success
	lang := i18n.DetectOrLoadLanguage()
	terminal.NextAction = commands.LocalizedCommandNextAction(lang, contract, false)
	if err != nil {
		terminal.Outcome = commands.CommandOutcomeForError(err)
		terminal.Result = commands.RedactCommandPresentationText(err.Error(), 1200)
		terminal.NextAction = commands.LocalizedCommandNextAction(lang, contract, true)
	}
	renderScreenReaderCommandPresentation(renderer, terminal)
	if err != nil {
		return screenReaderPresentedCommandError{err: err}
	}
	return nil
}

func forkScreenReaderSession(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, fields []string) error {
	if cfg.Engine == nil || cfg.SessionID == nil || renderer == nil {
		return replError(i18n.KeyREPLErrorForkUnavailable)
	}
	messages, err := cfg.Engine.Sessions().Load(*cfg.SessionID)
	if err != nil {
		return replWrap(i18n.KeyREPLErrorForkLoadConversation, err)
	}
	entries := availableConversationForkEntries(messages, i18n.DetectOrLoadLanguage(), renderer.Warning)
	if len(entries) == 0 {
		return nil
	}
	if len(fields) == 1 {
		var listing strings.Builder
		listing.WriteString(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLForkListIntro) + "\n")
		for index, entry := range entries {
			text := strings.Join(strings.Fields(entry.UserText), " ")
			runes := []rune(text)
			if len(runes) > 120 {
				text = string(runes[:117]) + "..."
			}
			fmt.Fprintf(&listing, "%d. %s\n", index+1, text)
		}
		renderer.Info(strings.TrimSpace(listing.String()))
		return nil
	}
	if len(fields) != 2 {
		return replError(i18n.KeyREPLErrorUsage, "/fork [NUMBER]")
	}
	selection, err := strconv.Atoi(fields[1])
	if err != nil || selection < 1 || selection > len(entries) {
		return replError(i18n.KeyREPLErrorForkSelectionRange, len(entries))
	}
	if cfg.SessionTransitionMu == nil {
		return replError(i18n.KeyREPLErrorSessionTransitionLockMissing)
	}
	cfg.SessionTransitionMu.Lock()
	defer cfg.SessionTransitionMu.Unlock()
	fork, err := forkSessionFromSnapshot(ctx, cfg, messages, entries[selection-1].MessageEnd)
	if err != nil {
		return err
	}
	renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLForkOpened, fork.ID))
	return nil
}

func runScreenReaderDiagnosticCommand(cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, input string, onGoalActivated ...func(string)) error {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	cmd, args := registry.Parse(input)
	if cmd == nil {
		return replError(i18n.KeyREPLErrorUnknownCommand, strings.Fields(input)[0])
	}
	sessionID := ""
	if cfg.SessionID != nil {
		sessionID = *cfg.SessionID
	}
	projectDir := currentProjectDir(cfg)

	commandCtx := &commands.Context{
		Language: i18n.DetectOrLoadLanguage(),
		OnEvent: func(value string) {
			renderer.Info(commands.RedactCommandPresentationText(value, 0))
		},
		OnCommandPresentation: func(presentation commands.CommandPresentation) {
			renderScreenReaderCommandPresentation(renderer, presentation)
		},
		CWD:                      currentCWD(cfg),
		CurrentProjectDir:        projectDir,
		SessionID:                sessionID,
		GoalRuntime:              newSessionGoalRuntime(cfg, sessionID, projectDir),
		BuildDiagnostic:          currentBuildDiagnostic(cfg),
		MCPBackend:               cfg.MCPBackend,
		SkillManager:             cfg.SkillManager,
		SkillInvoker:             cfg.SkillInvoker,
		ProviderRegistry:         cfg.ProviderRegistry,
		CredentialStore:          cfg.CredentialStore,
		ProviderRuntimeOverrides: cfg.ProviderRuntimeOverrides,
	}
	if cfg.Repo != nil {
		commandCtx.SessionStore = &sessionStoreAdapter{repo: cfg.Repo, currentProjectDir: func() string { return currentProjectDir(cfg) }}
	}
	commandCtx.SwitchLanguage = switchScreenReaderLanguage
	if len(onGoalActivated) > 0 {
		commandCtx.OnGoalActivated = onGoalActivated[0]
	}
	if cfg.Engine != nil {
		providerName, modelID := "", ""
		if current := cfg.Engine.Provider(); current != nil {
			providerName, modelID = current.Name(), current.ModelID()
		}
		commandCtx.CurrentProvider = providerName
		commandCtx.CurrentModel = modelID
		commandCtx.QueryLoop = &engineQueryLooper{
			eng: cfg.Engine,
			sessionID: func() string {
				if cfg.SessionID == nil {
					return ""
				}
				return *cfg.SessionID
			},
			model: modelID,
		}
	}
	if cmd.Name() == "doctor" && commandCtx.QueryLoop == nil {
		return replError(i18n.KeyREPLErrorDoctorEngineRequired)
	}
	if err := cmd.Execute(commandCtx, args); err != nil {
		return screenReaderPresentedCommandError{err: err}
	}
	return nil
}

func switchScreenReaderLanguage(code string) string {
	current := i18n.DetectOrLoadLanguage()
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" || code == "show" {
		return i18n.Format(current, i18n.KeyLanguageCurrent, current.String())
	}
	target := current
	if code == "next" {
		target = current.Next()
	} else {
		found := false
		for _, candidate := range i18n.AllLanguages() {
			if candidate.Code() == code {
				target, found = candidate, true
				break
			}
		}
		if !found {
			return i18n.Text(current, i18n.KeyLanguageUnsupported)
		}
	}
	if err := i18n.SaveLanguage(target); err != nil {
		return i18n.Format(current, i18n.KeyREPLTUIErrorPrefix, err)
	}
	return i18n.Format(current, i18n.KeyLanguageSwitched, target.String())
}

func runScreenReaderQuery(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, tracker *ui.CostTracker, req engine.QueryRequest, sigHandler *SignalHandler) {
	queryCtx, queryCancel := context.WithCancel(ctx)
	if sigHandler != nil {
		sigHandler.SetQueryCancel(queryCancel)
	}
	ch, queryErr := cfg.Engine.Query(queryCtx, req)
	if queryErr != nil {
		queryCancel()
		if sigHandler != nil {
			sigHandler.ClearQueryCancel()
		}
		lang := i18n.DetectOrLoadLanguage()
		renderer.Error(i18n.Format(lang, i18n.KeyREPLTUIQueryStartFailed, engine.UserFacingError(lang, queryErr)))
		return
	}
	handler := makeScreenReaderEventHandler(renderer, tracker, func() (int, int) {
		usage, usageErr := cfg.Engine.ContextUsage(req.SessionID)
		if usageErr != nil || usage == nil {
			return 0, 0
		}
		return usage.TotalTokens, usage.UsedTokens
	}, presentation.ToolEventContext{SessionID: req.SessionID, ProjectRoot: req.ProjectRoot})
	var runErr error
	for event := range ch {
		if event.Final {
			runErr = event.Error
			continue
		}
		handler(event.Inner)
	}
	queryCancel()
	if sigHandler != nil {
		sigHandler.ClearQueryCancel()
	}
	renderer.Newline()
	switch {
	case runErr == nil:
		renderer.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLQueryCompleted))
	case errors.Is(runErr, context.Canceled):
		renderer.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLQueryCancelled))
	default:
		lang := i18n.DetectOrLoadLanguage()
		renderer.Error(i18n.Format(lang, i18n.KeyREPLQueryFailed, engine.UserFacingError(lang, runErr)))
	}
}

func deleteScreenReaderSessionHistory(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, targetID string) error {
	targetID = strings.TrimSpace(targetID)
	if cfg.Repo == nil {
		return replError(i18n.KeyREPLErrorSessionRepositoryUnavailable)
	}
	if cfg.SessionTransitionMu == nil || cfg.SessionID == nil || renderer == nil {
		return replError(i18n.KeyREPLErrorDeletionBoundaryUnavailable)
	}
	if targetID == "" {
		return replError(i18n.KeyREPLErrorSessionIDRequired)
	}

	// Resolve and guard the target under the transition boundary, but never
	// hold that mutex while DecisionRequest persists its audit record: the
	// screen-reader recorder uses the same boundary.
	cfg.SessionTransitionMu.Lock()
	decisionSessionID := strings.TrimSpace(*cfg.SessionID)
	if targetID == decisionSessionID {
		cfg.SessionTransitionMu.Unlock()
		return replError(i18n.KeyREPLErrorDeleteActiveSessionGuidance)
	}
	_, targetRef, resolveErr := cfg.Repo.GetMeta(targetID, currentProjectDir(cfg))
	cfg.SessionTransitionMu.Unlock()
	if resolveErr != nil {
		return resolveErr
	}

	response := renderer.DecisionRequest(ctx, permissions.PromptRequest{
		DecisionID: "decision:delete-history:" + targetID,
		SessionID:  decisionSessionID,
		ActorID:    "user", ActorType: "local", WorkUnitID: "session-management",
		Kind:   permissions.PromptKindPermission,
		Action: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryAction), Target: targetID,
		Impact:    i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryImpact),
		RiskLevel: 3, RiskReason: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryRisk),
		RuleSource: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryRule), ApprovalScope: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryScope),
		Choices: []string{"allow_once", "reject"},
		Body:    i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryBody),
		Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyDeleteHistoryMessage),
	})
	if response.Outcome != permissions.PromptOutcomeApproved {
		renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLDeleteHistoryRejected, response.Outcome, targetID))
		return nil
	}

	cfg.SessionTransitionMu.Lock()
	defer cfg.SessionTransitionMu.Unlock()
	activeID := strings.TrimSpace(*cfg.SessionID)
	if activeID != decisionSessionID {
		return replError(i18n.KeyREPLErrorActiveSessionChanged)
	}
	if targetID == activeID {
		return replError(i18n.KeyREPLErrorDeleteActiveSession)
	}
	deleter, _ := cfg.Engine.(engine.SessionHistoryDeleter)
	if deleter != nil {
		if err := deleter.DeleteSessionHistory(ctx, targetID, targetRef.ProjectDir); err != nil {
			return err
		}
	} else if err := cfg.Repo.Delete(targetID, targetRef.ProjectDir); err != nil {
		return err
	}
	renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLDeleteHistoryCompleted, targetID))
	return nil
}

func clearScreenReaderConversation(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, tracker *ui.CostTracker) error {
	cfg.SessionTransitionMu.Lock()
	defer cfg.SessionTransitionMu.Unlock()
	if err := persistScreenReaderLifecycle(cfg, tracker); err != nil {
		return replWrap(i18n.KeyREPLErrorSaveCurrentLifecycle, err)
	}
	oldID := *cfg.SessionID
	oldMode := currentTUISessionPermissionMode(cfg, tui.ModeAskEdit)
	newID := uuid.NewString()
	_, prepared, err := prepareEmptyTUISession(ctx, cfg, newID, 1)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			prepared.Abort()
			_ = cfg.Engine.Sessions().Delete(newID)
		}
	}()
	if err := applyTUISessionPermissionMode(cfg, tui.ModeAutoEdit); err != nil {
		return replWrap(i18n.KeyREPLErrorResetPermissionMode, err)
	}
	if err := commitPreparedRuntimeResume(ctx, prepared); err != nil {
		modeErr := applyTUISessionPermissionMode(cfg, oldMode)
		if modeErr != nil {
			actual := currentTUISessionPermissionMode(cfg, tui.ModeAutoEdit)
			fatalErr := replWrapWithDetail(i18n.KeyREPLErrorActivateEmptySession, err, i18n.KeyREPLErrorRollbackModeFailedClosed, modeErr, actual.String())
			renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLClearFailedClosed, actual.String()))
			if cfg.FailClosed != nil {
				cfg.FailClosed(fatalErr)
			}
			return fatalErr
		}
		return replWrapWithDetail(i18n.KeyREPLErrorActivateEmptySession, err, i18n.KeyREPLErrorRollbackMode, modeErr)
	}
	committed = true
	*cfg.SessionID = newID
	if cfg.PublishSessionID != nil {
		cfg.PublishSessionID(newID)
	}
	if cfg.PermChecker != nil {
		cfg.PermChecker.ResetSession()
	}
	tracker.SetProviderAndModel(cfg.Engine.Provider().Name(), cfg.Engine.Provider().ModelID())
	tracker.Reset(cfg.Engine.Provider().ModelID())
	renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLClearConversation, newID, oldID))
	return nil
}

func resumeScreenReaderSession(ctx context.Context, cfg TUIREPLConfig, renderer *ui.ScreenReaderRenderer, tracker *ui.CostTracker, targetID string) error {
	if cfg.SwitchSession == nil {
		return replError(i18n.KeyREPLErrorSessionSwitchUnavailable)
	}
	cfg.SessionTransitionMu.Lock()
	defer cfg.SessionTransitionMu.Unlock()
	if err := persistScreenReaderLifecycle(cfg, tracker); err != nil {
		return replWrap(i18n.KeyREPLErrorSaveCurrentLifecycle, err)
	}
	oldEntry := commands.SessionListEntry{ID: *cfg.SessionID, ProjectDir: currentProjectDir(cfg), CWD: currentCWD(cfg)}
	oldMode := tui.ModeAskEdit
	if cfg.RuntimeScope != nil {
		oldMode = interactionModeFromSessionMeta(cfg.RuntimeScope.PermissionMode())
	}
	entry, messages, targetMeta, err := resolveScreenReaderSession(cfg, targetID)
	if err != nil {
		return err
	}
	targetMode := tui.ModeAutoEdit
	if targetMeta.Presentation != nil {
		targetMode = interactionModeFromSessionMeta(targetMeta.Presentation.PermissionMode)
	}
	if err := cfg.SwitchSession(ctx, entry); err != nil {
		return err
	}
	if err := applyTUISessionPermissionMode(cfg, targetMode); err != nil {
		rollbackErr := cfg.SwitchSession(ctx, oldEntry)
		if rollbackErr == nil {
			modeErr := applyTUISessionPermissionMode(cfg, oldMode)
			if modeErr != nil {
				actual := currentTUISessionPermissionMode(cfg, oldMode)
				fatalErr := replWrapWithDetail(i18n.KeyREPLErrorRestorePermissionMode, err, i18n.KeyREPLErrorRollbackModeFailedClosed, modeErr, actual.String())
				renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLResumeFailedClosed, actual.String()))
				if cfg.FailClosed != nil {
					cfg.FailClosed(fatalErr)
				}
				return fatalErr
			}
			return replWrapWithDetail(i18n.KeyREPLErrorRestorePermissionMode, err, i18n.KeyREPLErrorRollbackMode, modeErr)
		}
		// The atomic switcher remains on target when rollback fails. Complete
		// the target-facing linear projection instead of claiming the old ID.
		if cfg.PermChecker != nil {
			cfg.PermChecker.ResetSession()
		}
		tracker.SetProviderAndModel(cfg.Engine.Provider().Name(), cfg.Engine.Provider().ModelID())
		tracker.Reset(cfg.Engine.Provider().ModelID())
		if targetMeta.Usage != nil {
			usage := targetMeta.Usage
			tracker.RestoreSession(cfg.Engine.Provider().ModelID(), usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheCreateTokens, usage.WebSearchRequests, usage.CumulativeCost, usage.CostKnown)
			tracker.RestoreCompactionBaselineState(usage.HasCompacted, usage.CompactionBaselineKnown, usage.InputTokensAtCompact, usage.CacheReadAtCompact)
			restoreTrackerConversationUsageFromMeta(tracker, usage)
		}
		renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLResumeDegraded, targetID))
		renderScreenReaderTranscript(renderer, messages, presentation.ToolEventContext{SessionID: *cfg.SessionID, ProjectRoot: currentRuntimeProjectRoot(cfg)}, screenReaderControlScope(cfg, *cfg.SessionID))
		return replWrapWithDetail(i18n.KeyREPLErrorRestorePermissionMode, err, i18n.KeyREPLErrorRollbackSessionTargetRetained, rollbackErr, currentTUISessionPermissionMode(cfg, targetMode).String())
	}
	if cfg.PermChecker != nil {
		cfg.PermChecker.ResetSession()
	}
	tracker.SetProviderAndModel(cfg.Engine.Provider().Name(), cfg.Engine.Provider().ModelID())
	tracker.Reset(cfg.Engine.Provider().ModelID())
	if targetMeta.Usage != nil {
		targetUsage := targetMeta.Usage
		tracker.RestoreSession(cfg.Engine.Provider().ModelID(), targetUsage.InputTokens, targetUsage.OutputTokens, targetUsage.CacheReadTokens, targetUsage.CacheCreateTokens, targetUsage.WebSearchRequests, targetUsage.CumulativeCost, targetUsage.CostKnown)
		tracker.RestoreCompactionBaselineState(targetUsage.HasCompacted, targetUsage.CompactionBaselineKnown, targetUsage.InputTokensAtCompact, targetUsage.CacheReadAtCompact)
		restoreTrackerConversationUsageFromMeta(tracker, targetUsage)
	}
	renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLResumeCompleted, targetID))
	renderScreenReaderTranscript(renderer, messages, presentation.ToolEventContext{SessionID: *cfg.SessionID, ProjectRoot: currentRuntimeProjectRoot(cfg)}, screenReaderControlScope(cfg, *cfg.SessionID))
	return nil
}

func resolveScreenReaderSession(cfg TUIREPLConfig, targetID string) (commands.SessionListEntry, []types.Message, session.SessionMeta, error) {
	entry := commands.SessionListEntry{ID: targetID, ProjectDir: currentProjectDir(cfg), CWD: currentCWD(cfg)}
	if cfg.Repo == nil {
		messages, err := cfg.Engine.Sessions().Load(targetID)
		return entry, messages, session.SessionMeta{}, err
	}
	meta, ref, err := cfg.Repo.GetMeta(targetID, currentProjectDir(cfg))
	if err != nil {
		return commands.SessionListEntry{}, nil, session.SessionMeta{}, err
	}
	messages, err := cfg.Repo.Load(ref)
	if err != nil {
		return commands.SessionListEntry{}, nil, session.SessionMeta{}, err
	}
	entry.ProjectDir = ref.ProjectDir
	entry.CWD = strings.TrimSpace(meta.CWD)
	return entry, messages, meta, nil
}

func restoreScreenReaderLifecycle(cfg TUIREPLConfig, tracker *ui.CostTracker) error {
	if cfg.Repo == nil || cfg.SessionID == nil {
		return nil
	}
	meta, _, err := cfg.Repo.GetMeta(*cfg.SessionID, currentProjectDir(cfg))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return replWrap(i18n.KeyREPLErrorLoadScreenReaderMetadata, err)
		}
		return nil
	}
	if meta.Presentation != nil {
		if err := applyTUISessionPermissionMode(cfg, interactionModeFromSessionMeta(meta.Presentation.PermissionMode)); err != nil {
			return replWrap(i18n.KeyREPLErrorRestoreScreenReaderMode, err)
		}
	}
	if meta.Usage != nil {
		tracker.RestoreSession(cfg.Engine.Provider().ModelID(), meta.Usage.InputTokens, meta.Usage.OutputTokens, meta.Usage.CacheReadTokens, meta.Usage.CacheCreateTokens, meta.Usage.WebSearchRequests, meta.Usage.CumulativeCost, meta.Usage.CostKnown)
		tracker.RestoreCompactionBaselineState(meta.Usage.HasCompacted, meta.Usage.CompactionBaselineKnown, meta.Usage.InputTokensAtCompact, meta.Usage.CacheReadAtCompact)
		restoreTrackerConversationUsageFromMeta(tracker, meta.Usage)
	}
	return nil
}

func persistScreenReaderLifecycle(cfg TUIREPLConfig, tracker *ui.CostTracker) error {
	if cfg.Repo == nil || cfg.SessionID == nil || strings.TrimSpace(*cfg.SessionID) == "" || tracker == nil {
		return nil
	}
	projectDir := currentProjectDir(cfg)
	currentMeta, _, currentMetaErr := cfg.Repo.GetMeta(*cfg.SessionID, projectDir)
	snapshot := tracker.Snapshot()
	usage := &session.SessionUsageMeta{
		InputTokens: snapshot.SessionInput, OutputTokens: snapshot.SessionOutput,
		CacheReadTokens: snapshot.SessionCacheRead, CacheCreateTokens: snapshot.SessionCacheCreate,
		WebSearchRequests: snapshot.SessionWebSearchRequests, CumulativeCost: snapshot.SessionCost, CostKnown: snapshot.CostKnown,
		HasCompacted: snapshot.HasCompacted, CompactionBaselineKnown: snapshot.CompactionBaselineKnown,
		InputTokensAtCompact: snapshot.InputAtCompact, CacheReadAtCompact: snapshot.CacheReadAtCompact,
	}
	conversation := snapshot.Conversation
	if !conversation.Known && currentMetaErr == nil && currentMeta.Usage != nil {
		restored := currentMeta.Usage
		conversation = ui.ConversationUsage{
			Known: restored.RoundUsageKnown, CompactionCount: restored.CompactionCount,
			CompletedInputTokens: restored.CompletedRoundInputTokens, CompletedOutputTokens: restored.CompletedRoundOutputTokens,
			LastInputTokens: restored.LastInputTokens, LastOutputTokens: restored.LastOutputTokens,
			LastCacheReadTokens: restored.LastCacheReadTokens, LastCacheMakeTokens: restored.LastCacheCreateTokens,
		}
	}
	usage.RoundUsageKnown = conversation.Known
	usage.CompactionCount = conversation.CompactionCount
	usage.CompletedRoundInputTokens = conversation.CompletedInputTokens
	usage.CompletedRoundOutputTokens = conversation.CompletedOutputTokens
	usage.LastInputTokens = conversation.LastInputTokens
	usage.LastOutputTokens = conversation.LastOutputTokens
	usage.LastCacheReadTokens = conversation.LastCacheReadTokens
	usage.LastCacheCreateTokens = conversation.LastCacheMakeTokens
	if current, err := cfg.Engine.ContextUsage(*cfg.SessionID); err == nil && current != nil {
		usage.UsedTokens, usage.MaxTokens = current.UsedTokens, current.TotalTokens
	}
	mode := tui.ModeAskEdit
	if cfg.RuntimeScope != nil {
		mode = interactionModeFromSessionMeta(cfg.RuntimeScope.PermissionMode())
	} else if cfg.PermChecker != nil && cfg.PermChecker.Mode() == permissions.ModeAllowAll {
		mode = tui.ModeAutoEdit
	}
	presentation := session.SessionPresentationMeta{}
	if currentMetaErr == nil && currentMeta.Presentation != nil {
		presentation = *currentMeta.Presentation
	}
	presentation.PermissionMode = mode.Code()
	return saveTUISessionMeta(cfg, *cfg.SessionID, projectDir, session.SessionMeta{Usage: usage, Presentation: &presentation})
}

func screenReaderDecisionRecorder(cfg TUIREPLConfig) func(ui.ScreenReaderDecisionRecord) error {
	return func(record ui.ScreenReaderDecisionRecord) error {
		if cfg.SessionTransitionMu == nil {
			return replError(i18n.KeyREPLErrorSessionTransitionBoundary)
		}
		cfg.SessionTransitionMu.Lock()
		defer cfg.SessionTransitionMu.Unlock()
		if cfg.SessionID == nil {
			return replError(i18n.KeyREPLErrorActiveSessionIdentityMissing)
		}
		activeSessionID := strings.TrimSpace(*cfg.SessionID)
		requestSessionID := strings.TrimSpace(record.Prompt.SessionID)
		if requestSessionID == "" {
			return replError(i18n.KeyREPLErrorDecisionSessionMissing, record.Prompt.DecisionID)
		}
		if requestSessionID != activeSessionID {
			return replError(i18n.KeyREPLErrorDecisionWrongSession, record.Prompt.DecisionID, requestSessionID, activeSessionID)
		}
		return persistScreenReaderDecisionForSession(cfg, requestSessionID, record)
	}
}

func persistScreenReaderDecisionForSession(cfg TUIREPLConfig, sessionID string, record ui.ScreenReaderDecisionRecord) error {
	if cfg.Repo == nil {
		return nil
	}
	if strings.TrimSpace(sessionID) == "" {
		return replError(i18n.KeyREPLErrorDecisionSessionEmpty)
	}
	meta, _, err := cfg.Repo.GetMeta(sessionID, currentProjectDir(cfg))
	if err != nil {
		return err
	}
	executionDecision := int(record.Response.Decision)
	meta.Decisions = append(meta.Decisions, session.SessionDecisionMeta{
		DecisionID: record.Prompt.DecisionID, ExecutionSessionID: record.Prompt.ExecutionSessionID,
		TurnID: record.Prompt.TurnID, ToolUseID: record.Prompt.ToolUseID,
		ToolName: record.Prompt.ToolName, Input: record.Prompt.Input, RiskLevel: record.Prompt.RiskLevel, Message: record.Prompt.Message,
		ActorID: record.Prompt.ActorID, ActorType: record.Prompt.ActorType, WorkUnitID: record.Prompt.WorkUnitID,
		Kind: string(record.Prompt.Kind), Action: record.Prompt.Action, Target: record.Prompt.Target,
		Impact: record.Prompt.Impact, RiskReason: record.Prompt.RiskReason, RuleSource: record.Prompt.RuleSource,
		ApprovalScope: record.Prompt.ApprovalScope, Choices: append([]string(nil), record.Prompt.Choices...), Body: record.Prompt.Body,
		ReviewDetails: append([]string(nil), record.Prompt.ReviewDetails...), PostMode: record.Prompt.PostMode,
		Outcome: string(record.Response.Outcome), Choice: record.Response.Choice, Decision: &executionDecision, ResolvedAt: record.ResolvedAt,
	})
	return saveTUISessionMeta(cfg, sessionID, currentProjectDir(cfg), session.SessionMeta{Decisions: meta.Decisions})
}

func exportScreenReaderSession(cfg TUIREPLConfig, sessionID, path string, renderer *ui.ScreenReaderRenderer) error {
	messages, err := cfg.Engine.Sessions().Load(sessionID)
	if err != nil {
		return err
	}
	visible := make([]types.Message, 0, len(messages))
	scope := screenReaderControlScope(cfg, sessionID)
	scopeSet := scope.Bound()
	for _, message := range messages {
		if !screenReaderInternalMessageForScope(message, scope, scopeSet) {
			visible = append(visible, message)
		}
	}
	data, err := json.MarshalIndent(visible, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	renderer.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyREPLExportCompleted, len(visible), path))
	return nil
}

func renderScreenReaderTranscript(renderer *ui.ScreenReaderRenderer, messages []types.Message, ctx presentation.ToolEventContext, scopes ...messagecontrol.Scope) {
	if len(messages) == 0 {
		return
	}
	scope := messagecontrol.Scope{}
	if len(scopes) == 1 {
		scope = scopes[0]
	}
	scopeSet := scope.Bound()
	renderer.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLTranscriptBegins))
	for _, message := range messages {
		// Only a content-bound runtime capability can hide model-only context.
		// Public Role/metadata descriptors remain visible ordinary evidence.
		if screenReaderInternalMessageForScope(message, scope, scopeSet) {
			continue
		}
		if summary, ok := screenReaderSkillInvocationSummary(i18n.DetectOrLoadLanguage(), message, scope, scopeSet); ok {
			renderer.Info(i18n.TranscriptRoleLabel(i18n.DetectOrLoadLanguage(), string(types.RoleUser)) + ": " + summary)
			continue
		}
		displayRole := message.Role
		if displayRole == types.RoleDeveloper && !screenReaderTrustedDeveloperForScope(message, scope, scopeSet) {
			displayRole = types.RoleUser
		}
		for _, block := range message.Content {
			switch value := block.(type) {
			case types.TextBlock:
				renderer.Info(i18n.TranscriptRoleLabel(i18n.DetectOrLoadLanguage(), string(displayRole)) + ": " + value.Text)
			case types.ToolUseBlock:
				renderer.RenderToolCall(ctx, value)
			case types.ToolResultBlock:
				renderer.RenderToolResult(ctx, value)
			}
		}
	}
	renderer.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLTranscriptEnds))
}

// screenReaderSkillInvocationSummary delegates strict v1 recognition to the
// shared TUI projection contract, then adds only screen-reader-owned copy. The
// returned command contains the original user selector/arguments, never the
// versioned body or registry metadata.
func screenReaderSkillInvocationSummary(lang i18n.Language, message types.Message, scope messagecontrol.Scope, scopeSet bool) (string, bool) {
	trusted := message.IsTrustedSkillInvocationMessageForScope(messagecontrol.Scope{})
	if scopeSet {
		trusted = message.IsTrustedSkillInvocationMessageForScope(scope)
	}
	if !trusted || len(message.Content) != 1 {
		return "", false
	}
	block, ok := message.Content[0].(types.TextBlock)
	if !ok || block.Type != types.ContentTypeText {
		return "", false
	}
	command, ok := tui.SkillInvocationTranscriptCommand(block.Text)
	if !ok {
		return "", false
	}
	argumentState := i18n.Text(lang, i18n.KeyScreenReaderSkillArgumentsOmitted)
	if separator := strings.IndexFunc(command, unicode.IsSpace); separator >= 0 {
		command = command[:separator]
		argumentState = i18n.Text(lang, i18n.KeyScreenReaderSkillArgumentsProvided)
	}
	return i18n.Format(lang, i18n.KeyScreenReaderSkillTranscriptInvoke, command, argumentState), true
}

func screenReaderControlScope(cfg TUIREPLConfig, sessionID string) messagecontrol.Scope {
	if cfg.Repo == nil || strings.TrimSpace(sessionID) == "" {
		return messagecontrol.Scope{}
	}
	scope, err := cfg.Repo.StoreForProjectDir(currentProjectDir(cfg)).MessageControlScope(sessionID)
	if err != nil || !scope.Bound() {
		return messagecontrol.Scope{}
	}
	return scope
}

func screenReaderInternalMessageForScope(message types.Message, scope messagecontrol.Scope, scopeSet bool) bool {
	if scopeSet {
		return message.IsInternalRuntimeMessageForScope(scope)
	}
	return message.IsInternalRuntimeMessageForScope(messagecontrol.Scope{})
}

func screenReaderTrustedDeveloperForScope(message types.Message, scope messagecontrol.Scope, scopeSet bool) bool {
	if scopeSet {
		return message.IsTrustedDeveloperMessageForScope(scope)
	}
	return message.IsTrustedDeveloperMessageForScope(messagecontrol.Scope{})
}

func makeScreenReaderEventHandler(renderer *ui.ScreenReaderRenderer, tracker *ui.CostTracker, getContextUsage func() (int, int), base presentation.ToolEventContext) func(stream.Event) {
	turnStart := time.Now()
	compactionReceipts := make(map[string]string)
	compactionBoundaries := make(map[string]struct{})
	toolCalls := make(map[string]types.ToolUseBlock)
	semanticGroups := newSemanticToolAggregationBuffer()
	return func(event stream.Event) {
		ctx := base
		if event.TurnID != "" {
			ctx.TurnID = event.TurnID
		} else if event.TurnCount > 0 {
			ctx.TurnID = fmt.Sprintf("%s:turn-%d", ctx.SessionID, event.TurnCount)
		}
		if event.ActorID != "" {
			ctx.ActorID = event.ActorID
		}
		if event.ActorType != "" {
			ctx.ActorType = event.ActorType
		}
		if event.WorkUnitID != "" {
			ctx.WorkUnitID = event.WorkUnitID
		}
		if event.Type != stream.EventSystemWarning && event.ProjectRoot != "" {
			ctx.ProjectRoot = event.ProjectRoot
		}
		switch event.Type {
		case stream.EventText:
			// Append-only accessibility output cannot retroactively replace text.
			// Emit every text event immediately so the first feedback is timely
			// and later tool events retain their source order.
			renderer.Text(event.Text)
		case stream.EventToolUse:
			if event.ToolUse != nil {
				call := *event.ToolUse
				if call.ID != "" {
					toolCalls[call.ID] = call
				}
				if call.Name != "SendUserMessage" {
					if semanticGroups.Start(ctx, call) {
						renderer.RenderToolPresentation(semanticToolCallPresentation(ctx, call))
					}
				}
			}
		case stream.EventToolResult:
			if event.ToolResult != nil {
				result := *event.ToolResult
				if presentation.IsSendUserMessageResult(result) {
					presentation.DispatchToolResultEvent(renderer, ctx, result)
					delete(toolCalls, result.ToolUseID)
					break
				}
				call := toolCalls[result.ToolUseID]
				if call.ID == "" {
					call = types.ToolUseBlock{ID: result.ToolUseID, Name: "Tool"}
				}
				presentation := semanticToolResultPresentation(ctx, call, result)
				if semanticGroups.Complete(ctx, call, result, presentation) {
					renderer.RenderToolPresentation(presentation)
				}
				delete(toolCalls, result.ToolUseID)
			}
		case stream.EventHookSummary:
			if event.HookSummary != nil {
				renderer.RenderHookSummary(ctx, presentation.HookSummary{ExecutionID: event.HookSummary.HookExecutionID, ToolUseID: event.HookSummary.ToolUseID, Name: event.HookSummary.HookName, Status: event.HookSummary.Status, Summary: event.HookSummary.Summary, Metadata: event.HookSummary.Metadata})
			}
		case stream.EventGoalEvaluation, stream.EventProviderUsage:
			recordAuxiliaryUsageEvent(tracker, event)
		case stream.EventTurnEnd:
			for _, presentation := range semanticGroups.Flush() {
				renderer.RenderToolPresentation(presentation)
			}
			recorded := recordTurnUsageEvent(tracker, event, time.Since(turnStart))
			maxTokens, usedTokens := 0, 0
			if getContextUsage != nil {
				maxTokens, usedTokens = getContextUsage()
			}
			if recorded {
				renderer.UsageSemantics(ui.BuildUsageSemanticsSnapshot(event.Usage, tracker, usedTokens, maxTokens))
			} else {
				renderer.Usage(event.Usage)
				if maxTokens > 0 && event.Usage != nil {
					renderer.ContextBar(usedTokens, maxTokens)
				}
			}
			turnStart = time.Now()
		case stream.EventError:
			for _, presentation := range semanticGroups.Flush() {
				renderer.RenderToolPresentation(presentation)
			}
			renderer.RuntimeErrorEvent(ctx, event.ToolUseID, event.Text, event.Error, event.Metadata)
		case stream.EventSystemWarning:
			presentation.DispatchRuntimeWarningEvent(renderer, runtimeevent.SystemWarningRuntimeEvent(event), i18n.DetectOrLoadLanguage(), true)
		case stream.EventUserInterruption:
			for _, presentation := range semanticGroups.Flush() {
				renderer.RenderToolPresentation(presentation)
			}
			renderer.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyREPLTurnInterrupted))
		case stream.EventProgress:
			if event.Progress == nil {
				break
			}
			key, state, receipt := screenReaderCompactionProgressReceipt(ctx, *event.Progress)
			if receipt != "" && compactionReceipts[key] != state {
				compactionReceipts[key] = state
				renderer.Info(receipt)
			}
		case stream.EventCompactBoundary:
			if event.Compact == nil {
				break
			}
			key := presentation.CompactionBoundaryIdentity(ctx, *event.Compact)
			if _, seen := compactionBoundaries[key]; seen {
				break
			}
			compactionBoundaries[key] = struct{}{}
			if tracker != nil && !tracker.MarkCompactionBoundary(key) {
				break
			}
			renderer.Info(screenReaderCompactionBoundaryReceipt(*event.Compact))
			retained := event.Compact.TruePostCompactTokenCount
			if retained == 0 {
				retained = event.Compact.PostCompactTokenCount
			}
			if getContextUsage != nil && retained > 0 {
				maxTokens, _ := getContextUsage()
				if maxTokens > 0 {
					renderer.ModelContext(presentation.ModelContextProjection{
						Scope: presentation.UsageScopeModelContext, Known: true,
						UsedTokens: retained, CapacityTokens: maxTokens,
						PercentUsed: modelContextPercent(retained, maxTokens),
						Measurement: presentation.ContextMeasurementLocalEstimate,
					})
					break
				}
			}
			renderer.ModelContext(presentation.ModelContextProjection{
				Scope: presentation.UsageScopeModelContext, Measurement: presentation.ContextMeasurementUnknown,
			})
		}
	}
}

func screenReaderCompactionProgressReceipt(ctx presentation.ToolEventContext, progress stream.ProgressEvent) (key, state, receipt string) {
	stage := strings.ToLower(strings.TrimSpace(progress.Stage))
	trigger := compactionTrigger(progress.Metadata)
	lang := i18n.DetectOrLoadLanguage()
	triggerLabel := i18n.TUICompactionTriggerLabel(lang, trigger)
	key = screenReaderCompactionKey(ctx, trigger, progress.Metadata)
	switch stage {
	case "compact_start":
		return key, "started", i18n.Format(lang, i18n.KeyREPLCompactionStarted, triggerLabel)
	case "compact_end":
		return key, "completed", i18n.Format(lang, i18n.KeyREPLCompactionCompleted, triggerLabel)
	case "compact_failed":
		return key, "failed", i18n.Format(lang, i18n.KeyREPLCompactionFailed, triggerLabel, compactionProgressReason(lang, progress))
	case "compact_cancelled":
		return key, "cancelled", i18n.Format(lang, i18n.KeyREPLCompactionCancelled, triggerLabel, compactionProgressReason(lang, progress))
	default:
		return key, "", ""
	}
}

func screenReaderCompactionKey(ctx presentation.ToolEventContext, trigger string, metadata map[string]any) string {
	if id, ok := metadata["compaction_id"].(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	turnID := ctx.TurnID
	if turnID == "" {
		turnID = ctx.SessionID + ":turn"
	}
	return compactionActivityID(ctx.SessionID, turnID, strings.ToLower(strings.TrimSpace(trigger)))
}

func compactionProgressReason(lang i18n.Language, progress stream.ProgressEvent) string {
	if reason, ok := progress.Metadata["error"].(string); ok && strings.TrimSpace(reason) != "" {
		if strings.EqualFold(strings.TrimSpace(progress.Stage), "compact_cancelled") && strings.TrimSpace(reason) == context.Canceled.Error() {
			return ""
		}
		return i18n.Format(lang, i18n.KeyREPLCompactionReason, strings.TrimSpace(reason))
	}
	message := strings.TrimSpace(progress.Message)
	if message == "" || message == "failed" || message == "cancelled" || message == "idle" || message == "compacting" {
		return ""
	}
	return i18n.Format(lang, i18n.KeyREPLCompactionReason, message)
}

func screenReaderCompactionBoundaryReceipt(boundary stream.CompactBoundaryEvent) string {
	retained := boundary.TruePostCompactTokenCount
	if retained == 0 {
		retained = boundary.PostCompactTokenCount
	}
	discarded := boundary.PreCompactTokenCount - retained
	if discarded < 0 {
		discarded = 0
	}
	lang := i18n.DetectOrLoadLanguage()
	return i18n.Format(lang, i18n.KeyREPLCompactionBoundary, i18n.TUICompactionTriggerLabel(lang, boundary.Trigger), boundary.PreCompactTokenCount, retained, retained, discarded)
}
