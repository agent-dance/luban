package agent

import (
	"strings"

	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	"github.com/agent-dance/luban/skills"
)

func resolveProfileSkill(manager *skills.Manager, authority skillauthority.Authority, requested string) (*skills.Skill, bool, error) {
	requested = strings.TrimSpace(requested)
	if manager == nil || requested == "" {
		return nil, false, nil
	}
	snapshot, err := authority.Snapshot(manager)
	if err != nil {
		return nil, false, err
	}
	selected, found := resolveEffectiveProfileSkill(snapshot.Skills, requested)
	if !found {
		return nil, false, nil
	}
	var resolved *skills.Skill
	result, err := authority.ResolveLatest(manager, skills.SkillResolveRequest{
		Selector: string(selected.ID), ExpectedRevision: selected.Revision, Origin: skills.InvocationOriginModel,
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
	return skills.EffectiveSkill{}, false
}
