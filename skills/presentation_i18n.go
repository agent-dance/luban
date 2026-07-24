package skills

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// PresentedSummary returns authored copy unchanged and re-renders only the
// first-party generated fallback in the active surface language.
func PresentedSummary(lang i18n.Language, skill EffectiveSkill) string {
	if !skill.SummaryGenerated {
		return strings.TrimSpace(skill.Summary)
	}
	key := i18n.KeySkillGeneratedSummary
	if skill.Source == SourceMCP {
		key = i18n.KeyMCPSkillGeneratedSummary
	}
	return i18n.Format(lang, key, skill.Name)
}
