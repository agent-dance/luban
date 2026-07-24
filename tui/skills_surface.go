package tui

import (
	"context"
	"errors"
	"sort"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// SkillsSnapshotBackend is the read boundary shared by the interactive catalog
// and explicit user invocation. Implementations must return the current
// effective registry view; prompt history is never an authorization source.
type SkillsSnapshotBackend interface {
	Snapshot(sessionID string) (skills.CatalogSnapshot, error)
}

// SkillsManagementBackend is the minimal mutation boundary needed by the
// interactive skills surface. The concrete checklist is intentionally
// owned elsewhere.
type SkillsManagementBackend interface {
	SkillsSnapshotBackend
	ToggleProjectVisibility(sessionID string, id skills.SkillID, expected skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error)
}

// SkillsMenuOpenRequest gives the menu everything it needs without coupling
// the REPL to its concrete state or widget types. Resolvers are
// live so a long-lived launcher cannot accidentally retain a stale session or
// language after a transition.
type SkillsMenuOpenRequest struct {
	SessionID func() string
	Language  func() i18n.Language
	Backend   SkillsManagementBackend
}

// SkillsMenuLauncher is the small boundary used by exact /skills routing.
// Task-specific menu state remains behind this interface.
type SkillsMenuLauncher interface {
	OpenSkillsMenu(SkillsMenuOpenRequest) error
}

// SkillsMenuLauncherFunc adapts a function for composition and tests.
type SkillsMenuLauncherFunc func(SkillsMenuOpenRequest) error

func (open SkillsMenuLauncherFunc) OpenSkillsMenu(request SkillsMenuOpenRequest) error {
	if open == nil {
		return ErrSkillsMenuLauncherUnavailable
	}
	return open(request)
}

// ErrSkillsMenuLauncherUnavailable is an internal routing sentinel. Surface
// code maps it to semantic i18n copy before presenting it.
var ErrSkillsMenuLauncherUnavailable = errors.New("skills menu launcher unavailable")

// RouteExactSkillsMenu owns the exact-command split. /skills subcommands are
// deliberately not handled here and continue through the shared command.
func RouteExactSkillsMenu(input string, launcher SkillsMenuLauncher, request SkillsMenuOpenRequest) (bool, error) {
	if strings.TrimSpace(input) != "/skills" {
		return false, nil
	}
	if launcher == nil {
		return true, ErrSkillsMenuLauncherUnavailable
	}
	return true, launcher.OpenSkillsMenu(request)
}

// SkillSlashOutcome is a transport-neutral result of resolving and invoking
// an unknown slash name as an explicit user skill. Presentation surfaces own
// their wording; Skill tool error content is retained as raw shared output.
type SkillSlashOutcome string

const (
	SkillSlashResolved           SkillSlashOutcome = "resolved"
	SkillSlashInvalidInput       SkillSlashOutcome = "invalid-input"
	SkillSlashBackendUnavailable SkillSlashOutcome = "backend-unavailable"
	SkillSlashSnapshotFailed     SkillSlashOutcome = "snapshot-failed"
	SkillSlashNotFound           SkillSlashOutcome = "not-found"
	SkillSlashAmbiguous          SkillSlashOutcome = "ambiguous"
	SkillSlashPolicyDenied       SkillSlashOutcome = "policy-denied"
	SkillSlashInvokerUnavailable SkillSlashOutcome = "invoker-unavailable"
	SkillSlashInvocationFailed   SkillSlashOutcome = "invocation-failed"
	SkillSlashInvocationRejected SkillSlashOutcome = "invocation-rejected"
	SkillSlashEmptyEnvelope      SkillSlashOutcome = "empty-envelope"
)

// SkillSlashSubmission contains no presentational copy. ModelContent is the
// versioned SKILL.md envelope and must be sent only to the model; it must never
// be rendered as a normal transcript/info message.
type SkillSlashSubmission struct {
	Outcome           SkillSlashOutcome
	RequestedSelector string
	Skill             *skills.EffectiveSkill
	Candidates        []skills.SkillID
	Arguments         *string
	ModelContent      string
	ToolResult        types.ToolResult
	Err               error
}

func (submission SkillSlashSubmission) Successful() bool {
	return submission.Outcome == SkillSlashResolved
}

// InvokeUserSkillSlash resolves a slash selector from the latest authoritative
// snapshot, converts it to a stable ID plus observed skill revision, and then
// invokes through the origin-aware boundary. The invoker remains authoritative
// and revalidates under the Manager transaction lock, closing the refresh race.
func InvokeUserSkillSlash(
	ctx context.Context,
	backend SkillsSnapshotBackend,
	invoker commands.SkillInvoker,
	sessionID string,
	input string,
) SkillSlashSubmission {
	selector, arguments, ok := parseUserSkillSlash(input)
	result := SkillSlashSubmission{RequestedSelector: selector, Arguments: arguments}
	if !ok {
		result.Outcome = SkillSlashInvalidInput
		return result
	}
	if backend == nil {
		result.Outcome = SkillSlashBackendUnavailable
		return result
	}

	snapshot, err := backend.Snapshot(sessionID)
	if err != nil {
		result.Outcome, result.Err = SkillSlashSnapshotFailed, err
		return result
	}
	if err := snapshot.Validate(); err != nil {
		result.Outcome, result.Err = SkillSlashSnapshotFailed, err
		return result
	}

	selected, candidates, outcome := selectUserInvocableSkill(snapshot, selector)
	result.Candidates = candidates
	if outcome != SkillSlashResolved {
		result.Outcome = outcome
		return result
	}
	selectedCopy := selected
	result.Skill = &selectedCopy
	if !selected.Executable || !selected.UserInvocable || selected.Visibility == skills.VisibilityOff || selected.ShadowedBy != "" {
		result.Outcome = SkillSlashPolicyDenied
		return result
	}
	if invoker == nil {
		result.Outcome = SkillSlashInvokerUnavailable
		return result
	}

	toolResult, invokeErr := invoker.InvokeSkill(ctx, commands.SkillInvocationRequest{
		SessionID:        sessionID,
		Selector:         string(selected.ID),
		ExpectedRevision: selected.Revision,
		Arguments:        arguments,
		Origin:           skills.InvocationOriginUser,
	})
	result.ToolResult = toolResult
	if invokeErr != nil {
		result.Outcome, result.Err = SkillSlashInvocationFailed, invokeErr
		return result
	}
	if toolResult.IsError {
		result.Outcome = SkillSlashInvocationRejected
		return result
	}
	content := toolResult.TextContent()
	if strings.TrimSpace(content) == "" {
		result.Outcome = SkillSlashEmptyEnvelope
		return result
	}
	result.Outcome = SkillSlashResolved
	result.ModelContent = content
	return result
}

func parseUserSkillSlash(input string) (string, *string, bool) {
	trimmed := strings.TrimLeftFunc(input, unicode.IsSpace)
	if len(trimmed) < 2 || trimmed[0] != '/' || strings.ContainsAny(trimmed, "\r\n") {
		return "", nil, false
	}
	body := strings.TrimPrefix(trimmed, "/")
	separator := strings.IndexFunc(body, unicode.IsSpace)
	if separator < 0 {
		if body == "" {
			return "", nil, false
		}
		return body, nil, true
	}
	selector := body[:separator]
	argumentText := strings.TrimSpace(body[separator:])
	if selector == "" {
		return "", nil, false
	}
	return selector, &argumentText, true
}

func selectUserInvocableSkill(snapshot skills.CatalogSnapshot, selector string) (skills.EffectiveSkill, []skills.SkillID, SkillSlashOutcome) {
	id := skills.SkillID(selector)
	if id.IsValid() {
		row, found := snapshot.Find(id)
		if !found {
			return skills.EffectiveSkill{}, nil, SkillSlashNotFound
		}
		return row, []skills.SkillID{id}, SkillSlashResolved
	}
	if strings.HasPrefix(selector, "skill:") {
		return skills.EffectiveSkill{}, nil, SkillSlashInvalidInput
	}

	matches := make([]skills.EffectiveSkill, 0, 1)
	for _, row := range snapshot.Skills {
		if row.Name == selector {
			matches = append(matches, row)
		}
	}
	if len(matches) == 0 {
		return skills.EffectiveSkill{}, nil, SkillSlashNotFound
	}
	candidates := make([]skills.SkillID, len(matches))
	for index, row := range matches {
		candidates[index] = row.ID
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i] < candidates[j] })
	if len(matches) != 1 {
		return skills.EffectiveSkill{}, candidates, SkillSlashAmbiguous
	}
	return matches[0], candidates, SkillSlashResolved
}

// FormatTUISkillSlashFailure maps typed routing outcomes to this surface's
// semantic copy. A SkillTool rejection is already localized by the runtime and
// is returned verbatim rather than reinterpreted from prose.
func FormatTUISkillSlashFailure(lang i18n.Language, submission SkillSlashSubmission) string {
	selector := submission.RequestedSelector
	switch submission.Outcome {
	case SkillSlashInvalidInput:
		return i18n.Format(lang, i18n.KeyTUISkillsInvalidSelector, selector)
	case SkillSlashBackendUnavailable:
		return i18n.Text(lang, i18n.KeyTUISkillsBackendUnavailable)
	case SkillSlashSnapshotFailed:
		return i18n.Format(lang, i18n.KeyTUISkillsSnapshotFailed, submission.Err)
	case SkillSlashNotFound:
		return i18n.Format(lang, i18n.KeyTUISkillsNotFound, selector)
	case SkillSlashAmbiguous:
		ids := make([]string, len(submission.Candidates))
		for index, id := range submission.Candidates {
			ids[index] = string(id)
		}
		return i18n.Format(lang, i18n.KeyTUISkillsAmbiguous, selector, strings.Join(ids, ", "))
	case SkillSlashPolicyDenied:
		return i18n.Format(lang, i18n.KeyTUISkillsUnavailable, selector)
	case SkillSlashInvokerUnavailable:
		return i18n.Text(lang, i18n.KeyTUISkillsInvokerUnavailable)
	case SkillSlashInvocationFailed:
		return i18n.Format(lang, i18n.KeyTUISkillsInvocationFailed, selector, submission.Err)
	case SkillSlashInvocationRejected:
		if content := strings.TrimSpace(submission.ToolResult.TextContent()); content != "" {
			return content
		}
		return i18n.Format(lang, i18n.KeyTUISkillsInvocationRejected, selector)
	case SkillSlashEmptyEnvelope:
		return i18n.Format(lang, i18n.KeyTUISkillsEmptyEnvelope, selector)
	default:
		return ""
	}
}

var (
	_ SkillsSnapshotBackend   = commands.SkillsBackend(nil)
	_ SkillsManagementBackend = commands.InteractiveSkillsBackend(nil)
)
