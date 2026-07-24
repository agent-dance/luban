package commands

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
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

// FormatInteractiveSkillsToggleResult maps the Manager's typed transaction
// truth to localized copy. Interactive surfaces must not infer outcomes from
// error strings or retain optimistic state after this receipt is returned.
func FormatInteractiveSkillsToggleResult(lang i18n.Language, result skills.ProjectVisibilityToggleResult) string {
	if err := result.Validate(); err != nil {
		return i18n.Format(lang, i18n.KeyCommandSkillsOperationFailed, i18n.Text(lang, i18n.KeyAuxSkillFailed))
	}
	id := string(result.RequestedSkillID)
	name, readOnlyReason := id, ""
	if result.Skill != nil {
		if strings.TrimSpace(result.Skill.Name) != "" {
			name = result.Skill.Name
		}
		readOnlyReason = result.Skill.ReadOnlyReason
	}
	readOnlyReason = skillReadOnlyReason(lang, readOnlyReason)

	if result.Outcome == skills.ProjectVisibilityToggleCommitted {
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleCommitted,
			name, id, i18n.RuntimeSkillVisibilityLabel(lang, string(result.Skill.Visibility)))
	}
	switch result.Reason {
	case skills.ProjectVisibilityToggleReasonStaleRevision:
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleStale, id)
	case skills.ProjectVisibilityToggleReasonUnknownSkill:
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleUnknown, id)
	case skills.ProjectVisibilityToggleReasonReadOnly:
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleReadOnly, name, id, readOnlyReason)
	case skills.ProjectVisibilityToggleReasonSessionOverride:
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleSession, name, id, id)
	case skills.ProjectVisibilityToggleReasonPersistenceFailed:
		return i18n.Format(lang, i18n.KeyCommandSkillsTogglePersistFailed, name, id)
	case skills.ProjectVisibilityToggleReasonLiveApplyRolledBack:
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleRolledBack, name, id)
	case skills.ProjectVisibilityToggleReasonRollbackFailed:
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleDegraded, name, id)
	case skills.ProjectVisibilityToggleReasonAuthoritativeRefresh:
		return i18n.Format(lang, i18n.KeyCommandSkillsToggleRefreshFailed, name, id)
	default:
		return i18n.Format(lang, i18n.KeyCommandSkillsOperationFailed,
			i18n.Text(lang, i18n.KeyAuxSkillFailed))
	}
}

// SkillsBackend is the live catalog surface needed by /skills. The runtime
// injects the same Manager used by SkillTool and the query-loop catalog.
type SkillsBackend interface {
	InteractiveSkillsBackend
	Resolve(sessionID, stableIDOrName string) (skills.ResolvedSkill, bool, error)
	SetVisibility(sessionID string, override skills.VisibilityOverride) (skills.CatalogSnapshot, error)
	ResetVisibility(sessionID string, scope skills.SkillScope, id skills.SkillID) (skills.CatalogSnapshot, error)
	RefreshSnapshot(sessionID string) (skills.CatalogSnapshot, error)

	// These adapters preserve the original current-session enable/disable/all
	// command semantics while callers migrate to scoped four-state operations.
	SetEnabled(sessionID, name string, enabled bool) (changed, found bool)
	SetAllEnabled(sessionID string, enabled bool) int
}

// SkillInvocationRequest is the surface-neutral explicit invocation contract.
// Arguments is a pointer so an omitted argument remains distinct from an
// explicitly supplied empty string. ExpectedRevision zero requests the latest
// effective state.
type SkillInvocationRequest struct {
	SessionID        string
	Selector         string
	ExpectedRevision skills.SkillRevision
	Arguments        *string
	Origin           skills.InvocationOrigin
}

// Validate applies the same selector, revision, and origin rules as the
// authoritative Manager execution boundary.
func (request SkillInvocationRequest) Validate() error {
	return (skills.SkillResolveRequest{
		SessionID:        request.SessionID,
		Selector:         request.Selector,
		ExpectedRevision: request.ExpectedRevision,
		Origin:           request.Origin,
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
	if invoke == nil {
		return types.ToolResult{}, i18n.NewError(i18n.KeyCommandSkillInvokerNotConfigured)
	}
	if err := request.Validate(); err != nil {
		return types.ToolResult{}, err
	}
	return invoke(ctx, request)
}

type skillsCmd struct {
	backend SkillsBackend
}

// NewSkillsCommand creates the /skills catalog and visibility command.
// Passing nil defers backend resolution to commands.Context at execution time.
func NewSkillsCommand(backend SkillsBackend) Command {
	return &skillsCmd{backend: backend}
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
	backend := c.backend
	if backend == nil {
		backend = ctx.SkillManager
	}
	if backend == nil {
		return fmt.Errorf("%s", i18n.Text(ctx.Language, i18n.KeyCommandSkillsUnavailable))
	}

	fields := strings.Fields(args)
	verb := "list"
	if len(fields) > 0 {
		verb = strings.ToLower(fields[0])
	}
	switch verb {
	case "", "list", "status":
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
	case "show", "get", "info":
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
	case "enable", "disable":
		if len(fields) != 2 {
			emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsToggleUsage, verb))
			reportCommandFailed(ctx)
			return nil
		}
		return toggleSkills(ctx, backend, fields[1], verb == "enable")
	case "refresh", "reload":
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
	case "help", "-h", "--help":
		emitSkills(ctx, skillsUsage(ctx.Language))
		reportCommandSucceeded(ctx)
		return nil
	default:
		return skillsUsageFailure(ctx)
	}
}

func showSkill(ctx *Context, backend SkillsBackend, selector string) error {
	snapshot, ok := readSkillsSnapshot(ctx, backend)
	if !ok {
		return nil
	}
	row, ok := selectSkill(ctx, snapshot, selector)
	if !ok {
		return nil
	}
	resolved, found, err := backend.Resolve(ctx.SessionID, string(row.ID))
	if err != nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	if !found || resolved.Skill == nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsNotFound, selector))
		reportCommandFailed(ctx)
		return nil
	}
	emitSkills(ctx, formatSkillDetails(ctx.Language, resolved, ctx.SessionID, snapshot.Revision))
	reportCommandSucceeded(ctx)
	return nil
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

func toggleSkills(ctx *Context, backend SkillsBackend, target string, enabled bool) error {
	target = strings.TrimSpace(target)
	action := i18n.Text(ctx.Language, i18n.KeyCommandSkillsStatusDisabled)
	state := action
	if enabled {
		action = i18n.Text(ctx.Language, i18n.KeyCommandSkillsStatusEnabled)
		state = action
	}
	if strings.EqualFold(target, "all") {
		changed := backend.SetAllEnabled(ctx.SessionID, enabled)
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsAllToggled,
			sentenceStart(action), changed, skillSessionLabel(ctx.Language, ctx.SessionID)))
		reportCommandSucceeded(ctx)
		return nil
	}

	snapshot, ok := readSkillsSnapshot(ctx, backend)
	if !ok {
		return nil
	}
	row, ok := selectSkill(ctx, snapshot, target)
	if !ok || !requireMutableSkill(ctx, row) {
		return nil
	}
	already := enabled && row.Visibility != skills.VisibilityOff || !enabled && row.Visibility == skills.VisibilityOff
	visibility := skills.VisibilityOff
	if enabled {
		visibility = skills.VisibilityAuto
	}
	if already {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsAlreadyToggled,
			row.Name, state, skillSessionLabel(ctx.Language, ctx.SessionID))+
			i18n.Format(ctx.Language, i18n.KeyCommandSkillsEffectiveState,
				i18n.RuntimeSkillVisibilityLabel(ctx.Language, string(row.Visibility)),
				i18n.RuntimeSkillScopeLabel(ctx.Language, string(row.VisibilitySource))))
		reportCommandSucceeded(ctx)
		return nil
	}

	next, err := backend.SetVisibility(ctx.SessionID, skills.VisibilityOverride{
		SkillID: row.ID, Scope: skills.SkillScopeSession, Visibility: visibility,
	})
	if errors.Is(err, skills.ErrSkillOverrideStoreMissing) && row.ShadowedBy == "" {
		changed, found := backend.SetEnabled(ctx.SessionID, row.Name, enabled)
		if !found {
			emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsNotFound, target))
			reportCommandFailed(ctx)
			return nil
		}
		if !changed {
			emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsAlreadyToggled,
				row.Name, state, skillSessionLabel(ctx.Language, ctx.SessionID))+
				i18n.Format(ctx.Language, i18n.KeyCommandSkillsEffectiveState,
					i18n.RuntimeSkillVisibilityLabel(ctx.Language, string(row.Visibility)),
					i18n.RuntimeSkillScopeLabel(ctx.Language, string(row.VisibilitySource))))
			reportCommandSucceeded(ctx)
			return nil
		}
		err = nil
		next, err = backend.Snapshot(ctx.SessionID)
	}
	if err != nil {
		emitSkills(ctx, i18n.Format(ctx.Language, i18n.KeyCommandSkillsOperationFailed, skills.UserFacingError(ctx.Language, err)))
		reportCommandFailed(ctx)
		return nil
	}
	output := i18n.Format(ctx.Language, i18n.KeyCommandSkillsToggled,
		sentenceStart(action), row.Name, skillSessionLabel(ctx.Language, ctx.SessionID))
	if updated, found := next.Find(row.ID); found {
		output += i18n.Format(ctx.Language, i18n.KeyCommandSkillsSetResult,
			updated.Name, updated.ID,
			i18n.RuntimeSkillVisibilityLabel(ctx.Language, string(visibility)),
			i18n.RuntimeSkillScopeLabel(ctx.Language, string(skills.SkillScopeSession)),
			i18n.RuntimeSkillVisibilityLabel(ctx.Language, string(updated.Visibility)),
			i18n.RuntimeSkillScopeLabel(ctx.Language, string(updated.VisibilitySource)))
	}
	emitSkills(ctx, output)
	reportCommandSucceeded(ctx)
	return nil
}

func parseSkillSet(fields []string) (string, skills.Visibility, skills.SkillScope, bool) {
	if len(fields) != 4 && len(fields) != 5 {
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
	if len(fields) != 3 && len(fields) != 4 {
		return "", "", false
	}
	scope, ok := parseSkillScope(fields[2:])
	return fields[1], scope, ok
}

func parseSkillScope(fields []string) (skills.SkillScope, bool) {
	var value string
	switch {
	case len(fields) == 2 && fields[0] == "--scope":
		value = fields[1]
	case len(fields) == 1 && strings.HasPrefix(fields[0], "--scope="):
		value = strings.TrimPrefix(fields[0], "--scope=")
	default:
		return "", false
	}
	scope := skills.SkillScope(strings.ToLower(value))
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

func sentenceStart(value string) string {
	first, size := utf8.DecodeRuneInString(value)
	if first == utf8.RuneError && size == 0 {
		return value
	}
	return string(unicode.ToUpper(first)) + value[size:]
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
	if skill == nil {
		return out.String()
	}
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
	if len(skill.Aliases) > 0 {
		out.WriteString(i18n.Format(lang, i18n.KeyCommandSkillsDetailAliases, strings.Join(skill.Aliases, ", ")))
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
		// Non-policy producers may supply an already presentable explanation.
		// Only the frozen CatalogPolicyReason codes are semantic UI tokens.
		return reason
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
