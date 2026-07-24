package skills

import (
	"fmt"
	"slices"
)

// catalogModelProjection is the exact per-skill state whose replacement can
// affect the model-facing catalog. CatalogRevision is intentionally absent:
// it versions the whole registry and must not make every skill look changed.
//
// SkillRevision is included because it is the version marker carried by an
// upsert and distinguishes a remove/re-add lifecycle even when the metadata
// and digest return to their previous values.
type catalogModelProjection struct {
	id                 SkillID
	name               string
	summary            string
	source             SkillSource
	locator            SkillLocator
	digest             SkillDigest
	revision           SkillRevision
	visibility         Visibility
	modelVisible       bool
	descriptionVisible bool
	executable         bool
}

// DiffCatalog computes the append-only model projection between two complete
// immutable catalog snapshots. It never mutates either snapshot and returns a
// delta sorted by stable SkillID.
func DiffCatalog(previous, current CatalogSnapshot) (CatalogDelta, error) {
	if err := validateCatalogTransition(previous, current); err != nil {
		return CatalogDelta{}, err
	}

	upserts := make([]CatalogUpsert, 0)
	revokes := make([]CatalogRevoke, 0)
	previousIndex, currentIndex := 0, 0

	for previousIndex < len(previous.Skills) || currentIndex < len(current.Skills) {
		switch {
		case previousIndex >= len(previous.Skills):
			currentSkill := current.Skills[currentIndex]
			if catalogSkillIsAnnounceable(currentSkill) {
				upserts = append(upserts, CatalogUpsert{Skill: currentSkill, Reason: CatalogUpsertAdded})
			}
			currentIndex++

		case currentIndex >= len(current.Skills):
			previousSkill := previous.Skills[previousIndex]
			if catalogSkillIsAnnounceable(previousSkill) {
				revokes = append(revokes, catalogRevoke(previousSkill, CatalogRevokeDeleted))
			}
			previousIndex++

		default:
			previousSkill := previous.Skills[previousIndex]
			currentSkill := current.Skills[currentIndex]
			switch {
			case previousSkill.ID < currentSkill.ID:
				if catalogSkillIsAnnounceable(previousSkill) {
					revokes = append(revokes, catalogRevoke(previousSkill, CatalogRevokeDeleted))
				}
				previousIndex++
			case currentSkill.ID < previousSkill.ID:
				if catalogSkillIsAnnounceable(currentSkill) {
					upserts = append(upserts, CatalogUpsert{Skill: currentSkill, Reason: CatalogUpsertAdded})
				}
				currentIndex++
			default:
				previousAnnounceable := catalogSkillIsAnnounceable(previousSkill)
				currentAnnounceable := catalogSkillIsAnnounceable(currentSkill)
				switch {
				case currentAnnounceable && !previousAnnounceable:
					upserts = append(upserts, CatalogUpsert{Skill: currentSkill, Reason: CatalogUpsertReenabled})
				case currentAnnounceable && catalogModelState(previousSkill) != catalogModelState(currentSkill):
					upserts = append(upserts, CatalogUpsert{Skill: currentSkill, Reason: CatalogUpsertUpdated})
				case !currentAnnounceable && previousAnnounceable:
					revokes = append(revokes, catalogRevoke(currentSkill, catalogRevokeReason(currentSkill)))
				}
				previousIndex++
				currentIndex++
			}
		}
	}

	return NewCatalogDelta(previous.Revision, current.Revision, upserts, revokes)
}

// CoalesceCatalogSnapshots validates a sequence of authoritative snapshots and
// reduces all changes before a sampling boundary to one latest-state delta.
// Intermediate events are not concatenated: only the state in the final
// snapshot can be announced. This makes add-then-delete a no-op and
// delete/disable-then-restore one current re-enabled upsert.
func CoalesceCatalogSnapshots(base CatalogSnapshot, snapshots ...CatalogSnapshot) (CatalogDelta, error) {
	if err := base.Validate(); err != nil {
		return CatalogDelta{}, fmt.Errorf("base catalog snapshot: %w", err)
	}
	if len(snapshots) == 0 {
		return NewCatalogDelta(base.Revision, base.Revision, nil, nil)
	}

	type observedSkill struct {
		skill  EffectiveSkill
		absent bool
	}
	observed := make(map[SkillID]observedSkill, len(base.Skills))
	baseAnnounceable := make(map[SkillID]struct{}, len(base.Skills))
	for _, skill := range base.Skills {
		observed[skill.ID] = observedSkill{skill: skill}
		if catalogSkillIsAnnounceable(skill) {
			baseAnnounceable[skill.ID] = struct{}{}
		}
	}
	lostAnnouncement := make(map[SkillID]struct{})

	previous := base
	for index, current := range snapshots {
		if err := validateCatalogTransition(previous, current); err != nil {
			return CatalogDelta{}, fmt.Errorf("catalog snapshot %d: %w", index, err)
		}

		currentByID := make(map[SkillID]EffectiveSkill, len(current.Skills))
		for _, skill := range current.Skills {
			currentByID[skill.ID] = skill
			if prior, ok := observed[skill.ID]; ok {
				switch {
				case skill.Revision < prior.skill.Revision:
					return CatalogDelta{}, fmt.Errorf("skill %q: %w: revision %d precedes %d", skill.ID, ErrInvalidSkillRevision, skill.Revision, prior.skill.Revision)
				case prior.absent && skill.Revision == prior.skill.Revision:
					return CatalogDelta{}, fmt.Errorf("skill %q: %w: re-add must advance revision %d", skill.ID, ErrInvalidSkillRevision, skill.Revision)
				case !prior.absent && skill.Revision == prior.skill.Revision && skill != prior.skill:
					return CatalogDelta{}, fmt.Errorf("skill %q: %w: state changed without advancing revision %d", skill.ID, ErrInvalidSkillRevision, skill.Revision)
				}
			}
			observed[skill.ID] = observedSkill{skill: skill}
		}

		for id, prior := range observed {
			if _, exists := currentByID[id]; exists {
				continue
			}
			prior.absent = true
			observed[id] = prior
		}
		for id := range baseAnnounceable {
			skill, exists := currentByID[id]
			if !exists || !catalogSkillIsAnnounceable(skill) {
				lostAnnouncement[id] = struct{}{}
			}
		}

		previous = current
	}

	delta, err := DiffCatalog(base, previous)
	if err != nil {
		return CatalogDelta{}, err
	}
	for index := range delta.Upserts {
		if _, lost := lostAnnouncement[delta.Upserts[index].Skill.ID]; lost {
			delta.Upserts[index].Reason = CatalogUpsertReenabled
		}
	}
	return NewCatalogDelta(delta.FromRevision, delta.ToRevision, delta.Upserts, delta.Revokes)
}

func validateCatalogTransition(previous, current CatalogSnapshot) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("previous catalog snapshot: %w", err)
	}
	if err := current.Validate(); err != nil {
		return fmt.Errorf("current catalog snapshot: %w", err)
	}
	if current.Revision < previous.Revision {
		return fmt.Errorf("%w: current revision %d precedes previous revision %d", ErrInvalidCatalogRevision, current.Revision, previous.Revision)
	}
	if current.Revision == previous.Revision {
		if !slices.Equal(previous.Skills, current.Skills) {
			return fmt.Errorf("%w: revision %d has different catalog states", ErrInvalidCatalogRevision, current.Revision)
		}
		return nil
	}

	previousIndex, currentIndex := 0, 0
	for previousIndex < len(previous.Skills) && currentIndex < len(current.Skills) {
		previousSkill := previous.Skills[previousIndex]
		currentSkill := current.Skills[currentIndex]
		switch {
		case previousSkill.ID < currentSkill.ID:
			previousIndex++
		case currentSkill.ID < previousSkill.ID:
			currentIndex++
		default:
			switch {
			case currentSkill.Revision < previousSkill.Revision:
				return fmt.Errorf("skill %q: %w: current revision %d precedes previous revision %d", currentSkill.ID, ErrInvalidSkillRevision, currentSkill.Revision, previousSkill.Revision)
			case currentSkill.Revision == previousSkill.Revision && currentSkill != previousSkill:
				return fmt.Errorf("skill %q: %w: state changed without advancing revision %d", currentSkill.ID, ErrInvalidSkillRevision, currentSkill.Revision)
			}
			previousIndex++
			currentIndex++
		}
	}
	return nil
}

func catalogSkillIsAnnounceable(skill EffectiveSkill) bool {
	return skill.ModelVisible && skill.Executable && skill.ShadowedBy == ""
}

func catalogModelState(skill EffectiveSkill) catalogModelProjection {
	summary := skill.Summary
	if !skill.DescriptionVisible {
		summary = ""
	}
	return catalogModelProjection{
		id:                 skill.ID,
		name:               skill.Name,
		summary:            summary,
		source:             skill.Source,
		locator:            skill.Locator,
		digest:             skill.Digest,
		revision:           skill.Revision,
		visibility:         skill.Visibility,
		modelVisible:       skill.ModelVisible,
		descriptionVisible: skill.DescriptionVisible,
		executable:         skill.Executable,
	}
}

func catalogRevoke(skill EffectiveSkill, reason CatalogRevokeReason) CatalogRevoke {
	return CatalogRevoke{
		ID:       skill.ID,
		Name:     skill.Name,
		Source:   skill.Source,
		Locator:  skill.Locator,
		Revision: skill.Revision,
		Reason:   reason,
	}
}

func catalogRevokeReason(skill EffectiveSkill) CatalogRevokeReason {
	switch {
	case skill.ShadowedBy != "":
		return CatalogRevokeShadowed
	case skill.Visibility == VisibilityOff:
		return CatalogRevokeDisabled
	case !skill.Executable:
		return CatalogRevokePermissionLost
	default:
		return CatalogRevokeVisibility
	}
}
