package tools

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type toolSkillAuthority struct {
	sessionID         string
	sessionProjectDir string
	projectRoot       string
	cwd               string
	generation        skills.ProjectSourceGeneration
	pinned            bool
	active            bool
}

// withGenerationLease revalidates the pinned authority and keeps a project
// retarget from crossing one short, in-memory registration. Compatibility
// callers without a loop-owned generation retain their existing behavior.
func (authority toolSkillAuthority) withGenerationLease(manager *skills.Manager, commit func() error) error {
	if commit == nil {
		return nil
	}
	if manager == nil || !authority.pinned {
		return commit()
	}
	return manager.WithProjectGenerationLease(authority.generation, commit)
}

// validateToolSkillAuthority rejects a stale loop-owned tool execution before
// it can read a retargeted child runtime or perform external side effects.
// Direct embedders without a private QueryLoop capability retain their legacy
// behavior; real model executions always carry the private capability.
func validateToolSkillAuthority(ctx context.Context, manager *skills.Manager) (toolSkillAuthority, error) {
	// The generation capability exists only when the skill subsystem is wired.
	// Agent/Team embedders that intentionally omit a Manager have no mutable
	// skill authority to fence and retain their legacy runtime behavior.
	if manager == nil {
		return toolSkillAuthority{}, nil
	}
	exec, ok := loop.ToolExecutionContextFromContext(ctx)
	if !ok {
		return toolSkillAuthority{}, nil
	}
	if !exec.IsLoopOwned() {
		return toolSkillAuthority{}, nil
	}
	sessionID, sessionProjectDir, projectRoot, cwd, active := exec.ActiveRuntimeOwnerIdentity()
	if !active {
		return toolSkillAuthority{}, i18n.WrapInternalError(
			i18n.KeyLoopQueryValidateSkillGenerationFailed,
			skills.ErrSkillProjectGenerationChanged,
		)
	}
	authority := toolSkillAuthority{
		sessionID: strings.TrimSpace(sessionID), sessionProjectDir: strings.TrimSpace(sessionProjectDir),
		projectRoot: projectRoot, cwd: cwd, active: true,
	}
	generation, pinned := exec.SkillProjectGeneration()
	if !pinned {
		return toolSkillAuthority{}, i18n.WrapInternalError(
			i18n.KeyLoopQueryValidateSkillGenerationFailed,
			skills.ErrSkillProjectGenerationChanged,
		)
	}
	if err := manager.ValidateProjectGeneration(generation); err != nil {
		return toolSkillAuthority{}, i18n.WrapInternalError(i18n.KeyLoopQueryValidateSkillGenerationFailed, err)
	}
	authority.generation = generation
	authority.pinned = true
	return authority, nil
}

func (authority toolSkillAuthority) validateRuntime(runtime types.ToolRuntimeContext) error {
	if !authority.active {
		return nil
	}
	if strings.TrimSpace(runtime.SessionID) != authority.sessionID ||
		!sameToolRuntimePath(runtime.ProjectRoot, authority.projectRoot) {
		return i18n.WrapInternalError(
			i18n.KeyLoopQueryValidateSkillGenerationFailed,
			skills.ErrSkillProjectGenerationChanged,
		)
	}
	return nil
}

func sameToolRuntimePath(left, right string) bool {
	canonical := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		if abs, err := filepath.Abs(value); err == nil {
			value = abs
		}
		value = filepath.Clean(value)
		if resolved, err := filepath.EvalSymlinks(value); err == nil {
			value = filepath.Clean(resolved)
		}
		return value
	}
	return canonical(left) != "" && canonical(left) == canonical(right)
}

// resolveProfileSkill resolves a profile-declared preload through the same
// session policy and project generation as the parent model run. It first maps
// legacy/plugin aliases using the authoritative effective snapshot, then asks
// ResolveLatest to revalidate identity, revision, visibility and execution
// policy at the body-read boundary.
func resolveProfileSkill(manager *skills.Manager, authority toolSkillAuthority, requested string) (*skills.Skill, bool, error) {
	requested = strings.TrimSpace(requested)
	if manager == nil || requested == "" {
		return nil, false, nil
	}
	if !authority.pinned {
		skill, found := ResolveSkillName(manager, requested)
		return skill, found, nil
	}

	snapshot, err := manager.SnapshotAtGeneration(authority.sessionID, authority.generation)
	if err != nil {
		return nil, false, err
	}
	selected, found := resolveEffectiveProfileSkill(snapshot.Skills, requested)
	if !found {
		return nil, false, nil
	}
	var resolved *skills.Skill
	result, err := manager.ResolveLatest(skills.SkillResolveRequest{
		SessionID:                 authority.sessionID,
		Selector:                  string(selected.ID),
		ExpectedRevision:          selected.Revision,
		ExpectedProjectGeneration: authority.generation,
		Origin:                    skills.InvocationOriginModel,
	}, func(current skills.ResolvedSkill) error {
		resolved = current.Skill
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if result.Outcome != skills.SkillResolveResolved || resolved == nil {
		return nil, false, nil
	}
	return resolved, true, nil
}

func resolveEffectiveProfileSkill(rows []skills.EffectiveSkill, requested string) (skills.EffectiveSkill, bool) {
	for _, row := range rows {
		if row.ShadowedBy == "" && row.Name == requested {
			return row, true
		}
	}
	for _, row := range rows {
		if row.ShadowedBy != "" {
			continue
		}
		_, suffix, found := strings.Cut(strings.TrimSpace(row.Name), ":")
		if found && strings.EqualFold(suffix, requested) {
			return row, true
		}
	}
	lowered := strings.ToLower(requested)
	for _, row := range rows {
		if row.ShadowedBy != "" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(row.Name))
		if strings.HasSuffix(name, ":"+lowered) || strings.HasSuffix(name, "/"+lowered) {
			return row, true
		}
	}
	return skills.EffectiveSkill{}, false
}
