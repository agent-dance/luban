package skill

import (
	"strings"

	"github.com/agent-dance/luban/skills"
)

// skillHasOnlySafeProperties reports whether the skill is safe to auto-allow
// in default permission mode. Returns true when no "unsafe" property is set
// to a meaningful value.
func skillHasOnlySafeProperties(s *skills.Skill) bool {
	if len(s.AllowedTools) > 0 {
		return false
	}
	if strings.TrimSpace(s.Shell) != "" {
		return false
	}
	return true
}
