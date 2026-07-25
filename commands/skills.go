package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// InteractiveSkillsBackend is the authoritative catalog surface used by the
// keyboard skills manager. A toggle is one Manager-owned transaction; callers
// must redraw from the returned snapshot instead of applying an optimistic
// local boolean change.
type InteractiveSkillsBackend interface {
	Snapshot(sessionID string) (skills.CatalogSnapshot, error)
	ToggleProjectVisibility(sessionID string, id skills.SkillID, expected skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error)
}

// SkillsBackend is the live catalog surface needed by /skills. The runtime
// injects the same Manager used by SkillTool and the query-loop catalog.
type SkillsBackend interface {
	InteractiveSkillsBackend
	SnapshotBinding(sessionID string) (skills.CatalogBinding, error)
	ResolveLatest(request skills.SkillResolveRequest, consume func(skills.ResolvedSkill) error) (skills.SkillResolveResult, error)
	SetVisibility(sessionID string, override skills.VisibilityOverride) (skills.CatalogSnapshot, error)
	ResetVisibility(sessionID string, scope skills.SkillScope, id skills.SkillID) (skills.CatalogSnapshot, error)
	RefreshSnapshot(sessionID string) (skills.CatalogSnapshot, error)
}

// SkillInvocationRequest is the surface-neutral explicit invocation contract.
// Arguments is a pointer so an omitted argument remains distinct from an
// explicitly supplied empty string. Project generation pins the workspace
// authority; ExpectedRevision pins the selected catalog row.
type SkillInvocationRequest struct {
	SessionID                 string
	Selector                  string
	ExpectedRevision          skills.SkillRevision
	ExpectedProjectGeneration skills.ProjectSourceGeneration
	Arguments                 *string
	Origin                    skills.InvocationOrigin
}

// Validate applies the same selector, revision, project-authority, and origin
// rules as the authoritative Manager execution boundary.
func (request SkillInvocationRequest) Validate() error {
	if err := request.ExpectedProjectGeneration.Validate(); err != nil {
		return err
	}
	return (skills.SkillResolveRequest{
		SessionID:                 request.SessionID,
		Selector:                  request.Selector,
		ExpectedRevision:          request.ExpectedRevision,
		ExpectedProjectGeneration: request.ExpectedProjectGeneration,
		Origin:                    request.Origin,
	}).Validate()
}

// SkillInvoker lets TUI and screen-reader surfaces share explicit invocation
// semantics without importing the concrete SkillTool implementation.
type SkillInvoker interface {
	InvokeSkill(context.Context, SkillInvocationRequest) (types.ToolResult, error)
}

// SkillInvokerFunc adapts a composition-root closure to SkillInvoker.
type SkillInvokerFunc func(context.Context, SkillInvocationRequest) (types.ToolResult, error)

func (invoke SkillInvokerFunc) InvokeSkill(ctx context.Context, request SkillInvocationRequest) (types.ToolResult, error) {
	if err := request.Validate(); err != nil {
		return types.ToolResult{}, err
	}
	return invoke(ctx, request)
}

type skillsCmd struct{}

// NewSkillsCommand creates the /skills catalog and visibility command.
func NewSkillsCommand() Command {
	return &skillsCmd{}
}

func (c *skillsCmd) Name() string      { return "skills" }
func (c *skillsCmd) Aliases() []string { return nil }
func (c *skillsCmd) Description() string {
	return builtinCommandDescription("skills")
}
func (c *skillsCmd) DescriptionKey() i18n.Key { return i18n.KeyCommandSkillsDescription }

func (c *skillsCmd) Execute(ctx *Context, args string) error {
	if ctx == nil {
		ctx = &Context{}
	}
	backend := ctx.SkillManager
	if backend == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyCommandSkillsUnavailable))
	}

	fields := strings.Fields(args)
	verb := "list"
	if len(fields) > 0 {
		verb = strings.ToLower(fields[0])
	}
	switch verb {
	case "list":
		if len(fields) > 1 {
			return skillsUsageFailure(ctx)
		}
		snapshot, ok := readSkillsSnapshot(ctx, backend)
		if !ok {
			return nil
		}
		emitSkills(ctx, formatSkillsList(ctx.Language, snapshot, ctx.SessionID))
		reportCommandSucceeded(ctx)
		return nil
	case "show":
		if len(fields) != 2 {
			emitSkills(ctx, i18n.Text(ctx.Language, i18n.KeyCommandSkillsShowUsage))
			reportCommandFailed(ctx)
			return nil
		}
		return showSkill(ctx, backend, fields[1])
	case "set":
		selector, visibility, scope, ok := parseSkillSet(fields)
		if !ok {
			emitSkills(ctx, i18n.Text(ctx.Language, i18n.KeyCommandSkillsSetUsage))
			reportCommandFailed(ctx)
			return nil
		}
		return setSkillVisibility(ctx, backend, selector, visibility, scope)
	case "reset":
		selector, scope, ok := parseSkillReset(fields)
		if !ok {
			emitSkills(ctx, i18n.Text(ctx.Language, i18n.KeyCommandSkillsResetUsage))
			reportCommandFailed(ctx)
			return nil
		}
		return resetSkillVisibility(ctx, backend, selector, scope)
	case "refresh":
		if len(fields) != 1 {
			emitSkills(ctx, i18n.Text(ctx.Language, i18n.KeyCommandSkillsRefreshUsage))
			reportCommandFailed(ctx)
			return nil
		}
		snapshot, err := backend.RefreshSnapshot(ctx.SessionID)
		if err != nil {
			emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
			reportCommandFailed(ctx)
			return nil
		}
		emitSkills(ctx, i18n.Text(ctx.Language, i18n.KeyCommandSkillsRefreshed)+formatSkillsList(ctx.Language, snapshot, ctx.SessionID))
		reportCommandSucceeded(ctx)
		return nil
	default:
		return skillsUsageFailure(ctx)
	}
}

func showSkill(ctx *Context, backend SkillsBackend, selector string) error {
	binding, ok := readSkillsBinding(ctx, backend)
	if !ok {
		return nil
	}
	row, ok := selectSkill(ctx, binding.Snapshot, selector)
	if !ok {
		return nil
	}
	result, err := backend.ResolveLatest(skills.SkillResolveRequest{
		SessionID:                 ctx.SessionID,
		Selector:                  string(row.ID),
		ExpectedRevision:          row.Revision,
		ExpectedProjectGeneration: binding.ProjectGeneration,
		Origin:                    skills.InvocationOriginUser,
	}, nil)
	if err != nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	if result.Outcome == skills.SkillResolveStale {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed,
			skills.UserFacingError(ctx.Language, skills.ErrInvalidSkillRevision)))
		reportCommandFailed(ctx)
		return nil
	}
	if result.Resolved == nil || result.Resolved.Skill == nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsNotFound, selector))
		reportCommandFailed(ctx)
		return nil
	}
	emitSkills(ctx, formatSkillDetails(ctx.Language, *result.Resolved, ctx.SessionID, result.CatalogRevision))
	reportCommandSucceeded(ctx)
	return nil
}

func readSkillsBinding(ctx *Context, backend SkillsBackend) (skills.CatalogBinding, bool) {
	binding, err := backend.SnapshotBinding(ctx.SessionID)
	if err != nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return skills.CatalogBinding{}, false
	}
	return binding, true
}

func setSkillVisibility(ctx *Context, backend SkillsBackend, selector string, visibility skills.Visibility, scope skills.SkillScope) error {
	snapshot, ok := readSkillsSnapshot(ctx, backend)
	if !ok {
		return nil
	}
	row, ok := selectSkill(ctx, snapshot, selector)
	if !ok || !requireMutableSkill(ctx, row) {
		return nil
	}
	next, err := backend.SetVisibility(ctx.SessionID, skills.VisibilityOverride{
		SkillID: row.ID, Scope: scope, Visibility: visibility,
	})
	if err != nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	updated, found := next.Find(row.ID)
	if !found {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsNotFound, selector))
		reportCommandFailed(ctx)
		return nil
	}
	emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsSetResult,
		updated.Name, updated.ID,
		i18n.RuntimeSkillVisibilityLabel(ctx.Language, string(visibility)),
		i18n.RuntimeSkillScopeLabel(ctx.Language, string(scope)),
		i18n.RuntimeSkillVisibilityLabel(ctx.Language, string(updated.Visibility)),
		i18n.RuntimeSkillScopeLabel(ctx.Language, string(updated.VisibilitySource))))
	reportCommandSucceeded(ctx)
	return nil
}

func resetSkillVisibility(ctx *Context, backend SkillsBackend, selector string, scope skills.SkillScope) error {
	snapshot, ok := readSkillsSnapshot(ctx, backend)
	if !ok {
		return nil
	}
	row, ok := selectSkill(ctx, snapshot, selector)
	if !ok || !requireMutableSkill(ctx, row) {
		return nil
	}
	next, err := backend.ResetVisibility(ctx.SessionID, scope, row.ID)
	if err != nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	updated, found := next.Find(row.ID)
	if !found {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsNotFound, selector))
		reportCommandFailed(ctx)
		return nil
	}
	emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsResetResult,
		updated.Name, updated.ID, i18n.RuntimeSkillScopeLabel(ctx.Language, string(scope)),
		i18n.RuntimeSkillVisibilityLabel(ctx.Language, string(updated.Visibility)),
		i18n.RuntimeSkillScopeLabel(ctx.Language, string(updated.VisibilitySource))))
	reportCommandSucceeded(ctx)
	return nil
}

func parseSkillSet(fields []string) (string, skills.Visibility, skills.SkillScope, bool) {
	if len(fields) != 5 {
		return "", "", "", false
	}
	visibility := skills.Visibility(strings.ToLower(fields[2]))
	if visibility.Validate() != nil {
		return "", "", "", false
	}
	scope, ok := parseSkillScope(fields[3:])
	return fields[1], visibility, scope, ok
}

func parseSkillReset(fields []string) (string, skills.SkillScope, bool) {
	if len(fields) != 4 {
		return "", "", false
	}
	scope, ok := parseSkillScope(fields[2:])
	return fields[1], scope, ok
}

func parseSkillScope(fields []string) (skills.SkillScope, bool) {
	if len(fields) != 2 || fields[0] != "--scope" {
		return "", false
	}
	scope := skills.SkillScope(strings.ToLower(fields[1]))
	switch scope {
	case skills.SkillScopeSession, skills.SkillScopeProject, skills.SkillScopeUser:
		return scope, true
	default:
		return "", false
	}
}

func readSkillsSnapshot(ctx *Context, backend SkillsBackend) (skills.CatalogSnapshot, bool) {
	snapshot, err := backend.Snapshot(ctx.SessionID)
	if err != nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return skills.CatalogSnapshot{}, false
	}
	return snapshot, true
}

func selectSkill(ctx *Context, snapshot skills.CatalogSnapshot, selector string) (skills.EffectiveSkill, bool) {
	selector = strings.TrimSpace(selector)
	if strings.HasPrefix(selector, "skill:") {
		id := skills.SkillID(selector)
		if err := id.Validate(); err != nil {
			emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsInvalidSelector, selector))
			reportCommandFailed(ctx)
			return skills.EffectiveSkill{}, false
		}
		if row, found := snapshot.Find(id); found {
			return row, true
		}
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsNotFound, selector))
		reportCommandFailed(ctx)
		return skills.EffectiveSkill{}, false
	}

	candidates := make([]skills.EffectiveSkill, 0, 1)
	for _, row := range snapshot.Skills {
		if row.Name == selector {
			candidates = append(candidates, row)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	if len(candidates) == 0 {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsNotFound, selector))
		reportCommandFailed(ctx)
		return skills.EffectiveSkill{}, false
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	var output strings.Builder
	output.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandSkillsAmbiguous, selector))
	for _, candidate := range candidates {
		output.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandSkillsAmbiguousCandidate,
			candidate.ID, i18n.RuntimeSkillSourceLabel(ctx.Language, string(candidate.Source)), candidate.Locator))
	}
	emitSkills(ctx, output.String())
	reportCommandFailed(ctx)
	return skills.EffectiveSkill{}, false
}

func requireMutableSkill(ctx *Context, row skills.EffectiveSkill) bool {
	if row.Mutable {
		return true
	}
	emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsReadOnly,
		row.Name, row.ID, skillReadOnlyReason(ctx.Language, row.ReadOnlyReason)))
	reportCommandFailed(ctx)
	return false
}

func formatSkillsList(lang i18n.Language, snapshot skills.CatalogSnapshot, sessionID string) string {
	enabled := 0
	for _, row := range snapshot.Skills {
		if row.Executable {
			enabled++
		}
	}

	var out strings.Builder
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListHeader,
		len(snapshot.Skills), enabled, len(snapshot.Skills)-enabled, skillSessionLabel(lang, sessionID)))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsCatalogRevision, snapshot.Revision))
	if len(snapshot.Skills) == 0 {
		out.WriteString(i18n.Text(lang, i18n.KeyCommandSkillsNone))
		return out.String()
	}
	for _, row := range snapshot.Skills {
		status := i18n.Text(lang, i18n.KeyCommandSkillsStatusEnabled)
		if !row.Executable {
			status = i18n.Text(lang, i18n.KeyCommandSkillsStatusDisabled)
		}
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListEntry, status, row.Name))
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListSummary, skillSummaryValue(lang, row)))
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListIdentity, row.ID, i18n.RuntimeSkillSourceLabel(lang, string(row.Source)), row.Locator))
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListRevision, row.Digest, row.Revision))
		readOnlyReason := skillReadOnlyReason(lang, row.ReadOnlyReason)
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListPolicy,
			i18n.RuntimeSkillVisibilityLabel(lang, string(row.Visibility)),
			i18n.RuntimeSkillScopeLabel(lang, string(row.VisibilitySource)), skillMutabilityLabel(lang, row.Mutable), readOnlyReason))
		if row.ShadowedBy != "" {
			out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListShadowed, row.ShadowedBy))
		}
	}
	return out.String()
}

func formatSkillDetails(lang i18n.Language, resolved skills.ResolvedSkill, sessionID string, catalogRevision skills.CatalogRevision) string {
	row := resolved.Effective
	status := i18n.Text(lang, i18n.KeyCommandSkillsStatusEnabled)
	if !row.Executable {
		status = i18n.Text(lang, i18n.KeyCommandSkillsStatusDisabled)
	}

	var out strings.Builder
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailHeader, row.Name))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailStatus, status, skillSessionLabel(lang, sessionID)))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailSummary, skillSummaryValue(lang, row)))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailIdentity, row.ID, i18n.RuntimeSkillSourceLabel(lang, string(row.Source)), row.Locator))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailRevision, row.Digest, row.Revision, catalogRevision))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailPolicy,
		i18n.RuntimeSkillVisibilityLabel(lang, string(row.Visibility)),
		i18n.RuntimeSkillScopeLabel(lang, string(row.VisibilitySource)), skillMutabilityLabel(lang, row.Mutable), skillReadOnlyReason(lang, row.ReadOnlyReason)))
	if row.ShadowedBy != "" {
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsListShadowed, row.ShadowedBy))
	}
	skill := resolved.Skill
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailPath, skillDisplayPath(lang, skill.FilePath)))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailDirectory, skillDisplayPath(lang, skill.SkillDir)))
	modelInvocation := i18n.Text(lang, i18n.KeyCommandSkillsStatusEnabled)
	if skill.DisableModelInvocation {
		modelInvocation = i18n.Text(lang, i18n.KeyCommandSkillsDisabledFrontmatter)
	} else if !row.ModelVisible {
		modelInvocation = i18n.Text(lang, i18n.KeyCommandSkillsStatusDisabled)
	}
	userInvocation := i18n.Text(lang, i18n.KeyCommandSkillsStatusEnabled)
	if !skill.IsUserInvocable() {
		userInvocation = i18n.Text(lang, i18n.KeyCommandSkillsDisabledFrontmatter)
	} else if !row.UserInvocable {
		userInvocation = i18n.Text(lang, i18n.KeyCommandSkillsStatusDisabled)
	}
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailModelInvoke, modelInvocation))
	out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailUserInvoke, userInvocation))
	if skill.Context != "" {
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailContext, i18n.RuntimeSkillContextLabel(lang, string(skill.Context))))
	}
	if skill.Model != "" {
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailModel, skill.Model))
	}
	if skill.Version != "" {
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailVersion, skill.Version))
	}
	if len(skill.AllowedTools) > 0 {
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailTools, strings.Join(skill.AllowedTools, ", ")))
	}
	return out.String()
}

func skillSummaryValue(lang i18n.Language, row skills.EffectiveSkill) string {
	value := skills.PresentedSummary(lang, row)
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return i18n.Text(lang, i18n.KeyCommandSkillsNoneValue)
	}
	const maxRunes = 180
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes-1]) + "…"
}

func skillDisplayPath(lang i18n.Language, value string) string {
	if strings.TrimSpace(value) == "" {
		return i18n.Text(lang, i18n.KeyCommandSkillsVirtualPath)
	}
	return value
}

func skillMutabilityLabel(lang i18n.Language, mutable bool) string {
	if mutable {
		return i18n.Text(lang, i18n.KeyCommandSkillsMutableYes)
	}
	return i18n.Text(lang, i18n.KeyCommandSkillsMutableNo)
}

func skillReadOnlyReason(lang i18n.Language, reason string) string {
	reason = strings.TrimSpace(reason)
	switch skills.CatalogPolicyReason(reason) {
	case "":
		return i18n.Text(lang, i18n.KeyCommandSkillsNoneValue)
	case skills.CatalogPolicyReasonManagedReadOnly:
		return i18n.Text(lang, i18n.KeyCommandSkillsReadOnlyManaged)
	case skills.CatalogPolicyReasonManagedDeny:
		return i18n.Text(lang, i18n.KeyCommandSkillsReadOnlyDenied)
	default:
		return i18n.Text(lang, i18n.KeyCommandSkillsNoneValue)
	}
}

func skillSessionLabel(lang i18n.Language, sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return i18n.Text(lang, i18n.KeyCommandSkillsSessionCurrent)
	}
	return i18n.Format(lang, i18n.KeyCommandSkillsSession, strings.TrimSpace(sessionID))
}

func emitSkills(ctx *Context, value string) {
	if ctx != nil && ctx.OnEvent != nil {
		ctx.OnEvent(value)
	}
}

func skillsUsageFailure(ctx *Context) error {
	emitSkills(ctx, skillsUsage(ctx.Language))
	reportCommandFailed(ctx)
	return nil
}

func skillsUsage(lang i18n.Language) string {
	return i18n.Text(lang, i18n.KeyCommandSkillsFullUsage)
}

var (
	_ SkillsBackend = (*skills.Manager)(nil)
	_ SkillInvoker  = SkillInvokerFunc(nil)
)
