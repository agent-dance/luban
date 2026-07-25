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
