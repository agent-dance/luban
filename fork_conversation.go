package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/types"
)

func availableConversationForkEntries(messages []types.Message, lang i18n.Language, warning func(string)) []tui.ForkEntry {
	entries := buildConversationForkEntriesInLanguage(messages, lang)
	if len(entries) == 0 && warning != nil {
		warning(i18n.Text(lang, i18n.KeyForkNoConversationTurns))
	}
	return entries
}

func buildConversationForkEntries(messages []types.Message) []tui.ForkEntry {
	return buildConversationForkEntriesInLanguage(messages, i18n.DetectOrLoadLanguage())
}

func buildConversationForkEntriesInLanguage(messages []types.Message, lang i18n.Language) []tui.ForkEntry {
	type userPoint struct {
		index int
		text  string
	}
	points := make([]userPoint, 0)
	for index, message := range messages {
		if isSelectableForkUserMessage(message) {
			points = append(points, userPoint{index: index, text: forkMessageSummaryInLanguage(message, lang)})
		}
	}

	chronological := make([]tui.ForkEntry, 0, len(points))
	for index, point := range points {
		messageEnd := len(messages)
		if index+1 < len(points) {
			messageEnd = points[index+1].index
		}
		assistantText := ""
		for _, message := range messages[point.index+1 : messageEnd] {
			if message.Role == types.RoleAssistant {
				if text := strings.TrimSpace(message.GetText()); text != "" {
					assistantText = text
				}
			}
		}
		chronological = append(chronological, tui.ForkEntry{
			MessageEnd: messageEnd, UserText: point.text, AssistantText: assistantText,
		})
	}

	entries := make([]tui.ForkEntry, len(chronological))
	for i := range chronological {
		entries[i] = chronological[len(chronological)-1-i]
	}
	return entries
}

func isSelectableForkUserMessage(message types.Message) bool {
	if message.Role != types.RoleUser || message.IsInternalRuntimeMessage() || compact.IsCompactBoundaryMessage(message) || compact.IsCompactSummaryMessage(message) {
		return false
	}
	hasPromptContent := false
	for _, block := range message.Content {
		switch block.(type) {
		case types.ToolResultBlock:
			return false
		case types.TextBlock, types.ImageBlock, types.DocumentBlock:
			hasPromptContent = true
		}
	}
	if !hasPromptContent {
		return false
	}
	return true
}

func forkMessageSummary(message types.Message) string {
	return forkMessageSummaryInLanguage(message, i18n.DetectOrLoadLanguage())
}

func forkMessageSummaryInLanguage(message types.Message, lang i18n.Language) string {
	parts := make([]string, 0, len(message.Content))
	for _, block := range message.Content {
		switch value := block.(type) {
		case types.TextBlock:
			if text := strings.TrimSpace(value.Text); text != "" {
				parts = append(parts, text)
			}
		case types.ImageBlock:
			parts = append(parts, i18n.Text(lang, i18n.KeyRootForkImagePlaceholder))
		case types.DocumentBlock:
			parts = append(parts, i18n.Text(lang, i18n.KeyRootForkDocumentPlaceholder))
		}
	}
	if len(parts) == 0 {
		return i18n.Text(lang, i18n.KeyRootForkPromptPlaceholder)
	}
	return strings.Join(parts, " ")
}

func forkSessionFromSnapshot(ctx context.Context, cfg TUIREPLConfig, messages []types.Message, messageEnd int) (session.Ref, error) {
	return forkSessionFromSnapshotWithView(ctx, cfg, nil, messages, messageEnd)
}

func forkSessionFromSnapshotWithApp(ctx context.Context, cfg TUIREPLConfig, app tuiActivityApp, messages []types.Message, messageEnd int) (session.Ref, error) {
	if app == nil || messageEnd != len(messages) {
		return forkSessionFromSnapshotWithView(ctx, cfg, nil, messages, messageEnd)
	}
	if cfg.Repo == nil || cfg.SessionID == nil || cfg.SessionProjectDir == nil {
		return session.Ref{}, rootRuntimeError(i18n.KeyRootForkRepositoryUnavailable)
	}
	source := session.Ref{ID: strings.TrimSpace(*cfg.SessionID), ProjectDir: strings.TrimSpace(*cfg.SessionProjectDir)}
	if source.IsZero() {
		return session.Ref{}, rootRuntimeError(i18n.KeyRootForkIdentityIncomplete)
	}
	var capture tui.SessionViewCapture
	var captureErr error
	if !app.UpdateSync(func() {
		capture, captureErr = tui.CaptureSessionViewCheckpoint(app.State(), messages)
	}) {
		return session.Ref{}, i18n.NewError(i18n.KeyREPLTUIStopped)
	}
	if captureErr != nil {
		return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, captureErr, source.ID)
	}
	if err := tui.SaveSessionViewCapture(cfg.Repo.ArtifactsDir(source.ID, source.ProjectDir), capture); err != nil {
		return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, err, source.ID)
	}
	return forkSessionFromSnapshotWithView(ctx, cfg, nil, messages, messageEnd)
}

func forkSessionFromSnapshotWithView(ctx context.Context, cfg TUIREPLConfig, state *tui.AppState, messages []types.Message, messageEnd int) (session.Ref, error) {
	if cfg.Repo == nil || cfg.SessionID == nil || cfg.SessionProjectDir == nil {
		return session.Ref{}, rootRuntimeError(i18n.KeyRootForkRepositoryUnavailable)
	}
	if messageEnd <= 0 || messageEnd > len(messages) {
		return session.Ref{}, rootRuntimeError(i18n.KeyRootForkPointOutsideSnapshot, messageEnd, len(messages))
	}
	source := session.Ref{ID: strings.TrimSpace(*cfg.SessionID), ProjectDir: strings.TrimSpace(*cfg.SessionProjectDir)}
	if source.IsZero() {
		return session.Ref{}, rootRuntimeError(i18n.KeyRootForkIdentityIncomplete)
	}
	if state != nil && messageEnd == len(messages) {
		if err := tui.SaveSessionViewCheckpoint(cfg.Repo.ArtifactsDir(source.ID, source.ProjectDir), state, messages); err != nil {
			return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, err, source.ID)
		}
	}
	sourceIdentity, scopeErr := forkCheckpointSessionIdentity(cfg.Repo, source, 0)
	if scopeErr != nil {
		return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, scopeErr, source.ID)
	}
	fork, err := cfg.Repo.Fork(source, messages[:messageEnd])
	if err != nil {
		return session.Ref{}, err
	}

	cwd := currentCWD(cfg)
	meta := session.SessionMeta{CWD: cwd, GitBranch: detectGitBranch(cwd)}
	if cfg.Engine != nil {
		if currentProvider := cfg.Engine.Provider(); currentProvider != nil {
			meta.Provider = currentProvider.Name()
			meta.Model = currentProvider.ModelID()
		}
	}
	if cfg.CurrentModel != nil {
		if model := strings.TrimSpace(cfg.CurrentModel()); model != "" {
			meta.Model = model
		}
	}
	if cfg.RuntimeScope != nil {
		meta.Presentation = &session.SessionPresentationMeta{PermissionMode: cfg.RuntimeScope.PermissionMode()}
	}
	if err := cfg.Repo.SaveMeta(fork.ID, fork.ProjectDir, meta); err != nil {
		_ = cfg.Repo.Delete(fork.ID, fork.ProjectDir)
		return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, err, fork.ID)
	}
	targetMessages, loadErr := cfg.Repo.Load(fork)
	if loadErr != nil {
		_ = cfg.Repo.Delete(fork.ID, fork.ProjectDir)
		return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, loadErr, fork.ID)
	}
	targetIdentity, scopeErr := forkCheckpointSessionIdentity(cfg.Repo, fork, 1)
	if scopeErr != nil {
		_ = cfg.Repo.Delete(fork.ID, fork.ProjectDir)
		return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, scopeErr, fork.ID)
	}
	if err := tui.ForkSessionViewCheckpoint(
		cfg.Repo.ArtifactsDir(source.ID, source.ProjectDir),
		cfg.Repo.ArtifactsDir(fork.ID, fork.ProjectDir),
		sourceIdentity, targetIdentity,
		messages[:messageEnd], targetMessages,
	); err != nil {
		_ = cfg.Repo.Delete(fork.ID, fork.ProjectDir)
		return session.Ref{}, rootRuntimeWrap(i18n.KeyRootForkMetadataUpdate, err, fork.ID)
	}
	if cfg.OpenSessionTerminal == nil {
		return fork, rootRuntimeError(i18n.KeyRootForkTerminalUnavailable, fork.ID, brand.CommandName, fork.ID)
	}
	if err := cfg.OpenSessionTerminal(ctx, fork.ID, cwd, meta.Provider, meta.Model); err != nil {
		return fork, rootRuntimeWrap(i18n.KeyRootForkTerminalOpen, err, fork.ID, brand.CommandName, fork.ID)
	}
	return fork, nil
}

func forkCheckpointSessionIdentity(repo *session.Repository, ref session.Ref, epoch uint64) (tui.SessionIdentity, error) {
	if repo == nil || ref.IsZero() {
		return tui.SessionIdentity{}, session.ErrCorruptSessionHistory
	}
	scope, err := repo.StoreForProjectDir(ref.ProjectDir).MessageControlScope(ref.ID)
	if err != nil {
		return tui.SessionIdentity{}, err
	}
	expectedProjectScope, err := filepath.Abs(filepath.Clean(ref.ProjectDir))
	if err != nil {
		return tui.SessionIdentity{}, err
	}
	if !scope.Bound() || scope.SessionID() != ref.ID || scope.ProjectScope() != expectedProjectScope || scope.ContextGeneration() == 0 {
		return tui.SessionIdentity{}, session.ErrCorruptSessionHistory
	}
	identity := tui.SessionIdentity{Namespace: ref.ProjectDir, SessionID: ref.ID, Epoch: epoch}
	return identity.WithInternalControlScope(messagecontrol.Runtime(), scope), nil
}
