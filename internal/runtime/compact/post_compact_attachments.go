package compact

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const postCompactAttachmentReadLimit = 64 * 1024

const (
	// PostCompactMaxSkillBodies bounds full/superseding invocation envelopes
	// reattached after one history replacement. Bodies already present in the
	// preserved tail do not consume this reattachment count.
	PostCompactMaxSkillBodies = 5

	// PostCompactSkillBodyBudgetBytes is the aggregate encoded-envelope budget
	// for reattached skill bodies. Selection is deterministic and skips an
	// oversized candidate instead of allowing one body to crowd out all others.
	PostCompactSkillBodyBudgetBytes = 64 * 1024
)

// PostCompactAttachmentState is the compact-time context passed to optional
// attachment providers. Message slices must be treated as read-only.
type PostCompactAttachmentState struct {
	OriginalMessages      []types.Message
	MessagesAfterBoundary []types.Message
	PreservedTail         []types.Message
	SessionID             string
	CWD                   string
	ContextWindowTokens   int
}

// PostCompactAttachmentProvider restores compacted-away runtime context as
// model-visible user messages. Providers must be best-effort and return nil on
// missing state; compaction must not fail because optional context is absent.
type PostCompactAttachmentProvider interface {
	PostCompactAttachments(ctx context.Context, state PostCompactAttachmentState) []types.Message
}

// SkillCatalogPostCompactProvider rebuilds the live developer catalog and
// exact invocation-body projection. Implementations derive body evidence from
// model-visible messages.
type SkillCatalogPostCompactProvider interface {
	PostCompactSkillAttachments(ctx context.Context, state PostCompactAttachmentState) []types.Message
}

type PlanStateProvider interface {
	IsActive() bool
	PlanFile() string
}

type BackgroundTaskSnapshot struct {
	ID          string
	Type        string
	Status      string
	Description string
	Command     string
	Prompt      string
	Error       string
	Result      string
}

type BackgroundTaskProvider interface {
	PostCompactBackgroundTasks() []BackgroundTaskSnapshot
}

type MCPServerSnapshot struct {
	Name         string
	Tools        []string
	Instructions string
}

type MCPStateProvider interface {
	PostCompactMCPServers() []MCPServerSnapshot
}

type AgentDefinitionSnapshot struct {
	Name      string
	WhenToUse string
	Source    string
}

type AgentDefinitionProvider interface {
	PostCompactAgentDefinitions(cwd string) []AgentDefinitionSnapshot
}

// RuntimeAttachmentProvider converts Go runtime state that has a real product
// surface into TS-style post-compact attachments.
type RuntimeAttachmentProvider struct {
	PlanState        PlanStateProvider
	SkillCatalog     SkillCatalogPostCompactProvider
	BackgroundTasks  BackgroundTaskProvider
	MCPState         MCPStateProvider
	AgentDefinitions AgentDefinitionProvider

	SessionID string
	CWD       string

	DeferredToolNames func() []string
	LoadedToolNames   func() []string
}

func (p *RuntimeAttachmentProvider) PostCompactAttachments(ctx context.Context, state PostCompactAttachmentState) []types.Message {
	return p.postCompactAttachmentsForLanguage(ctx, state, i18n.DetectOrLoadLanguage())
}

func (p *RuntimeAttachmentProvider) postCompactAttachmentsForLanguage(ctx context.Context, state PostCompactAttachmentState, lang i18n.Language) []types.Message {
	if p == nil {
		return nil
	}
	_ = ctx
	if state.SessionID == "" {
		state.SessionID = p.SessionID
	}
	if state.CWD == "" {
		state.CWD = p.CWD
	}

	var out []types.Message
	if msg := p.planAttachment(lang); msg != nil {
		out = append(out, *msg)
	}
	if msg := p.planModeAttachment(lang); msg != nil {
		out = append(out, *msg)
	}
	if p.SkillCatalog != nil {
		out = append(out, p.SkillCatalog.PostCompactSkillAttachments(ctx, state)...)
	}
	if msg := p.backgroundTasksAttachment(lang); msg != nil {
		out = append(out, *msg)
	}
	if msg := p.deferredToolsAttachment(lang); msg != nil {
		out = append(out, *msg)
	}
	if msg := p.agentListingAttachment(lang, state.CWD); msg != nil {
		out = append(out, *msg)
	}
	if msg := p.mcpAttachment(lang); msg != nil {
		out = append(out, *msg)
	}
	return out
}

func (p *RuntimeAttachmentProvider) planAttachment(lang i18n.Language) *types.Message {
	if p.PlanState == nil || !p.PlanState.IsActive() {
		return nil
	}
	planFile := strings.TrimSpace(p.PlanState.PlanFile())
	if planFile == "" {
		return nil
	}
	content := readSmallTextFile(planFile)
	if content == "" {
		return nil
	}
	body := i18n.Format(lang, i18n.KeyCompactAttachmentPlanFile, planFile)
	body += "\n\n" + content
	return newPostCompactReminderMessage(lang, i18n.KeyCompactAttachmentPlanTitle, body)
}

func (p *RuntimeAttachmentProvider) planModeAttachment(lang i18n.Language) *types.Message {
	if p.PlanState == nil || !p.PlanState.IsActive() {
		return nil
	}
	return newPostCompactReminderMessage(
		lang,
		i18n.KeyCompactAttachmentPlanModeTitle,
		i18n.Text(lang, i18n.KeyCompactAttachmentPlanModeBody),
	)
}

func (p *RuntimeAttachmentProvider) backgroundTasksAttachment(lang i18n.Language) *types.Message {
	if p.BackgroundTasks == nil {
		return nil
	}
	tasks := p.BackgroundTasks.PostCompactBackgroundTasks()
	if len(tasks) == 0 {
		return nil
	}
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		label := firstNonEmpty(task.Description, task.Command, task.Prompt, task.Result)
		line := fmt.Sprintf("- %s [%s]", id, firstNonEmpty(task.Status, i18n.Text(lang, i18n.KeyCompactAttachmentUnknownStatus)))
		if task.Type != "" {
			line += i18n.Format(lang, i18n.KeyCompactAttachmentTypeLabel, task.Type)
		}
		if label != "" {
			line += " " + label
		}
		if task.Error != "" {
			line += i18n.Format(lang, i18n.KeyCompactAttachmentErrorLabel, task.Error)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}
	return newPostCompactReminderMessage(lang, i18n.KeyCompactAttachmentBackgroundTitle, strings.Join(lines, "\n"))
}

func (p *RuntimeAttachmentProvider) deferredToolsAttachment(lang i18n.Language) *types.Message {
	if p.DeferredToolNames == nil && p.LoadedToolNames == nil {
		return nil
	}
	deferred := uniqueSorted(callStringSlice(p.DeferredToolNames))
	loaded := uniqueSorted(callStringSlice(p.LoadedToolNames))
	if len(deferred) == 0 && len(loaded) == 0 {
		return nil
	}
	var lines []string
	if len(loaded) > 0 {
		lines = append(lines, i18n.Format(lang, i18n.KeyCompactAttachmentLoadedTools, strings.Join(loaded, ", ")))
	}
	if len(deferred) > 0 {
		lines = append(lines, i18n.Format(lang, i18n.KeyCompactAttachmentDeferredPool, strings.Join(deferred, ", ")))
	}
	return newPostCompactReminderMessage(lang, i18n.KeyCompactAttachmentDeferredTitle, strings.Join(lines, "\n"))
}

func (p *RuntimeAttachmentProvider) agentListingAttachment(lang i18n.Language, cwd string) *types.Message {
	if p.AgentDefinitions == nil {
		return nil
	}
	defs := p.AgentDefinitions.PostCompactAgentDefinitions(cwd)
	if len(defs) == 0 {
		return nil
	}
	lines := make([]string, 0, len(defs))
	for _, def := range defs {
		name := strings.TrimSpace(def.Name)
		if name == "" {
			continue
		}
		line := "- " + name
		if def.Source != "" {
			line += " (" + def.Source + ")"
		}
		if def.WhenToUse != "" {
			line += ": " + truncateLine(def.WhenToUse, 240)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}
	return newPostCompactReminderMessage(lang, i18n.KeyCompactAttachmentAgentTitle, strings.Join(lines, "\n"))
}

func (p *RuntimeAttachmentProvider) mcpAttachment(lang i18n.Language) *types.Message {
	if p.MCPState == nil {
		return nil
	}
	servers := p.MCPState.PostCompactMCPServers()
	if len(servers) == 0 {
		return nil
	}
	lines := make([]string, 0, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		tools := uniqueSorted(server.Tools)
		line := "- " + name
		if len(tools) > 0 {
			line += i18n.Format(lang, i18n.KeyCompactAttachmentMCPToolsLabel, strings.Join(tools, ", "))
		}
		if strings.TrimSpace(server.Instructions) != "" {
			line += i18n.Format(lang, i18n.KeyCompactAttachmentMCPInstructionsLabel, truncateLine(server.Instructions, 500))
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}
	return newPostCompactReminderMessage(lang, i18n.KeyCompactAttachmentMCPTitle, strings.Join(lines, "\n"))
}

func readSmallTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > postCompactAttachmentReadLimit {
		data = data[:postCompactAttachmentReadLimit]
	}
	return strings.TrimSpace(string(data))
}

func callStringSlice(fn func() []string) []string {
	if fn == nil {
		return nil
	}
	return fn()
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
