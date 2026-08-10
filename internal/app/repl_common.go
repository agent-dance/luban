package app

import (
	"context"
	"encoding/base64"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// sessionStoreAdapter bridges session.Repository to commands.SessionStore.
type sessionStoreAdapter struct {
	repo              *session.Repository
	currentProjectDir func() string
}

func (a *sessionStoreAdapter) Save(id string, msgs []types.Message) error {
	return a.repo.Save(id, a.currentProjectDir(), msgs)
}

func (a *sessionStoreAdapter) Load(id string) ([]types.Message, error) {
	msgs, _, err := a.repo.LoadByID(id, a.currentProjectDir())
	return msgs, err
}

func (a *sessionStoreAdapter) LoadEntry(entry commands.SessionListEntry) ([]types.Message, error) {
	projectDir := strings.TrimSpace(entry.ProjectDir)
	if projectDir == "" {
		projectDir = a.currentProjectDir()
	}
	return a.repo.Load(session.Ref{ID: entry.ID, ProjectDir: projectDir})
}

func (a *sessionStoreAdapter) Current(id string) (commands.SessionListEntry, error) {
	projectDir := strings.TrimSpace(a.currentProjectDir())
	if projectDir == "" {
		meta, ref, err := a.repo.GetMeta(id, "")
		if err != nil {
			return commands.SessionListEntry{}, err
		}
		return sessionListEntryFromMeta(ref, meta), nil
	}
	meta, err := a.repo.StoreForProjectDir(projectDir).GetMeta(id)
	if err != nil {
		return commands.SessionListEntry{}, err
	}
	ref := session.Ref{ID: id, ProjectDir: projectDir}
	return sessionListEntryFromMeta(ref, meta), nil
}

func sessionListEntryFromMeta(ref session.Ref, meta session.SessionMeta) commands.SessionListEntry {
	return commands.SessionListEntry{
		ID:           ref.ID,
		ProjectDir:   ref.ProjectDir,
		Title:        meta.Title,
		UpdatedAt:    meta.UpdatedAt,
		CreatedAt:    meta.CreatedAt,
		MessageCount: meta.MessageCount,
		CWD:          meta.CWD,
		GitBranch:    meta.GitBranch,
		PreviewText:  meta.PreviewText,
		Provider:     meta.Provider,
		Model:        meta.Model,
	}
}

func (a *sessionStoreAdapter) List() ([]commands.SessionListEntry, error) {
	infos, err := a.repo.Search(session.SearchOptions{
		AllProjects: true,
	})
	if err != nil {
		return nil, err
	}
	out := make([]commands.SessionListEntry, len(infos))
	for i, info := range infos {
		out[i] = commands.SessionListEntry{
			ID:           info.ID,
			ProjectDir:   info.ProjectDir,
			Title:        info.Title,
			UpdatedAt:    info.UpdatedAt,
			CreatedAt:    info.CreatedAt,
			MessageCount: info.MessageCount,
			CWD:          info.CWD,
			GitBranch:    info.GitBranch,
			PreviewText:  info.PreviewText,
			Provider:     info.Provider,
			Model:        info.Model,
		}
	}
	return out, nil
}

func (a *sessionStoreAdapter) Search(query, currentCWD string, allProjects bool) ([]commands.SessionListEntry, error) {
	infos, err := a.repo.Search(session.SearchOptions{
		Query:             query,
		CurrentCWD:        currentCWD,
		CurrentProjectDir: a.currentProjectDir(),
		AllProjects:       allProjects,
	})
	if err != nil {
		return nil, err
	}
	out := make([]commands.SessionListEntry, len(infos))
	for i, info := range infos {
		out[i] = commands.SessionListEntry{
			ID:           info.ID,
			ProjectDir:   info.ProjectDir,
			Title:        info.Title,
			UpdatedAt:    info.UpdatedAt,
			CreatedAt:    info.CreatedAt,
			MessageCount: info.MessageCount,
			CWD:          info.CWD,
			GitBranch:    info.GitBranch,
			PreviewText:  info.PreviewText,
			Provider:     info.Provider,
			Model:        info.Model,
		}
	}
	return out, nil
}

func (a *sessionStoreAdapter) Rename(id, title string) error {
	return a.repo.SaveMeta(id, a.currentProjectDir(), session.SessionMeta{Title: title})
}

// engineQueryLooper adapts engine.Engine to the commands.QueryLooper interface.
type engineQueryLooper struct {
	eng             engine.Engine
	sessionID       func() string
	model           string
	reasoningEffort string
	modelSaver      func(providerName, modelID string, reasoningEffort ...string) error
}

func (a *engineQueryLooper) Messages() []types.Message {
	msgs, _ := a.eng.Sessions().Load(a.sessionID())
	return msgs
}

func (a *engineQueryLooper) SetMessages(msgs []types.Message) {
	sid := a.sessionID()
	if err := a.eng.Sessions().Save(sid, msgs); err != nil {
		return
	}
	_, _ = a.eng.Resume(context.Background(), sid)
}

// SetMessagesPreservingToolUseLedger is the explicit same-session mutation
// surface used by MCP prompt commands. SessionManager.Save preserves the
// existing metadata sidecar and Resume reloads its durable identity ledger.
func (a *engineQueryLooper) SetMessagesPreservingToolUseLedger(msgs []types.Message) {
	a.SetMessages(msgs)
}

func (a *engineQueryLooper) Model() string {
	return a.model
}

func (a *engineQueryLooper) SetModel(m string) {
	a.model = m
	_ = a.eng.SetModel(a.sessionID(), m)
	if a.modelSaver != nil {
		providerName := ""
		if p := a.eng.Provider(); p != nil {
			providerName = p.Name()
		}
		_ = a.modelSaver(providerName, m)
	}
}

func (a *engineQueryLooper) ContextUsage() (maxTokens, usedTokens int) {
	maxTokens, usage := a.ContextUsageDetail()
	if usage.Measurement == compact.ContextUsageUnknown {
		return 0, 0
	}
	return maxTokens, usage.UsedTokens
}

func (a *engineQueryLooper) ContextUsageDetail() (int, compact.ContextInputUsage) {
	info, err := a.eng.ContextUsage(a.sessionID())
	if err != nil || info == nil {
		return 0, compact.ContextInputUsage{Measurement: compact.ContextUsageUnknown}
	}
	measurement := compact.ContextUsageMeasurement(info.Measurement)
	switch measurement {
	case compact.ContextUsageProviderReported, compact.ContextUsageLocalEstimate, compact.ContextUsageLocalLowerBound:
	default:
		measurement = compact.ContextUsageUnknown
	}
	return info.TotalTokens, compact.ContextInputUsage{
		UsedTokens:  info.UsedTokens,
		Measurement: measurement,
	}
}

func (a *engineQueryLooper) SetReasoningEffort(effort string) {
	a.reasoningEffort = effort
	if err := a.eng.SetReasoningEffort(a.sessionID(), effort); err != nil {
		return
	}
	if a.modelSaver != nil {
		providerName := ""
		if p := a.eng.Provider(); p != nil {
			providerName = p.Name()
		}
		_ = a.modelSaver(providerName, a.model, effort)
	}
}

func (a *engineQueryLooper) ReasoningEffort() string {
	return a.reasoningEffort
}

func (a *engineQueryLooper) SetProvider(p provider.Provider) {
	a.eng.SetProvider(p)
	a.model = p.ModelID()
}

// buildQueryRequest constructs an engine.QueryRequest from raw user input,
// parsing any image directives (/image <path> or @<path.ext>) into multimodal
// content blocks.
func buildQueryRequest(sessionID, rawInput string) (engine.QueryRequest, error) {
	content, text, err := parseImageInput(rawInput)
	if err != nil {
		return engine.QueryRequest{}, err
	}
	if len(content) == 0 {
		return engine.QueryRequest{
			SessionID: sessionID,
			Message:   rawInput,
		}, nil
	}
	text = strings.TrimSpace(text)
	if text != "" {
		content = append([]types.ContentBlock{types.TextBlock{
			Type: types.ContentTypeText,
			Text: text,
		}}, content...)
	}
	return engine.QueryRequest{
		SessionID: sessionID,
		Content:   content,
	}, nil
}

func buildGoalActivationQueryRequest(sessionID, objective, cwd, projectRoot string) (engine.QueryRequest, bool) {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return engine.QueryRequest{}, false
	}
	return engine.QueryRequest{
		SessionID:   strings.TrimSpace(sessionID),
		Message:     objective,
		CWD:         strings.TrimSpace(cwd),
		ProjectRoot: strings.TrimSpace(projectRoot),
	}, true
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true, ".tiff": true, ".tif": true,
}

func parseImageInput(rawInput string) ([]types.ContentBlock, string, error) {
	var blocks []types.ContentBlock
	remaining := rawInput

	for {
		idx := strings.Index(remaining, "/image ")
		if idx == -1 {
			break
		}
		end := strings.IndexByte(remaining[idx+7:], ' ')
		var path, after string
		if end == -1 {
			path = strings.TrimSpace(remaining[idx+7:])
			after = remaining[:idx]
		} else {
			path = strings.TrimSpace(remaining[idx+7 : idx+7+end])
			after = remaining[:idx] + remaining[idx+7+end:]
		}
		if path == "" {
			break
		}
		block, err := imageBlockFromFile(path)
		if err != nil {
			return nil, "", err
		}
		blocks = append(blocks, block)
		remaining = after
	}

	words := strings.Fields(remaining)
	var kept []string
	for _, w := range words {
		if strings.HasPrefix(w, "@") {
			candidate := w[1:]
			if imageExtensions[strings.ToLower(filepath.Ext(candidate))] {
				block, err := imageBlockFromFile(candidate)
				if err != nil {
					return nil, "", err
				}
				blocks = append(blocks, block)
				continue
			}
		}
		kept = append(kept, w)
	}
	remaining = strings.Join(kept, " ")

	return blocks, remaining, nil
}

func imageBlockFromFile(path string) (types.ContentBlock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, rootRuntimeWrap(i18n.KeyRootImageRead, err, path)
	}
	mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mediaType == "" {
		mediaType = "image/png"
	}
	if i := strings.IndexByte(mediaType, ';'); i != -1 {
		mediaType = strings.TrimSpace(mediaType[:i])
	}
	return types.ImageBlock{
		Type: types.ContentTypeImage,
		Source: &types.ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		},
	}, nil
}
