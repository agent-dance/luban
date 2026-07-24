package tools

import (
	"strings"

	"github.com/agent-dance/luban/skills"
)

// resolveSkillModelOverride implements TS resolveSkillModelOverride: when a
// skill's frontmatter sets model: "sonnet" but the parent session is on
// "sonnet[1m]" (1M context tier), preserve the [1m] suffix so the override
// does not silently drop the user's 1M-context tier.
//
// Cases:
//   - skillModel == ""           → "" (no override)
//   - parent has [1m] suffix    → carry the suffix when bare model matches
//   - parent has no [1m] suffix → return skillModel as-is
//   - skillModel already has any suffix → respected exactly
func resolveSkillModelOverride(skillModel, parentModel string) string {
	skillModel = strings.TrimSpace(skillModel)
	parentModel = strings.TrimSpace(parentModel)
	if skillModel == "" {
		return ""
	}

	// If the skill author already specified a suffix, honor it verbatim.
	if strings.Contains(skillModel, "[") {
		return skillModel
	}

	// Extract the bracketed suffix from the parent model, if any.
	openIdx := strings.Index(parentModel, "[")
	if openIdx < 0 {
		return skillModel
	}
	closeIdx := strings.Index(parentModel[openIdx:], "]")
	if closeIdx < 0 {
		return skillModel
	}
	closeIdx += openIdx
	suffix := parentModel[openIdx : closeIdx+1] // e.g. "[1m]"

	// Carry the suffix.
	return skillModel + suffix
}

// safeSkillPropertyKeys mirrors TS SAFE_SKILL_PROPERTIES — properties the
// model may use without surfacing a permission prompt. Anything outside this
// set with a meaningful (non-empty) value forces a prompt.
//
// In TS this is a JS object key allowlist; in Go we model the same intent on
// the parsed Skill struct: every Skill field that is NOT in this list and has
// a non-zero value triggers permission.
//
// Fields considered safe (default-allow):
//
//	name, description, hasUserSpecifiedDescription, isHidden, aliases,
//	argumentHint, whenToUse, paths, version, disableModelInvocation,
//	userInvocable, source, contentLength, argNames, model, effort, context,
//	agent, filePath, skillDir, rawContent, content
//
// Fields that DO require permission when set:
//
//	allowedTools, hooks, shell
//
// Note: model and effort are SAFE in TS — they only adjust the next turn's
// model/effort, not what tools may run.
var unsafeSkillFieldsRequirePermission = []string{
	"allowedTools",
	"shell",
	// Hooks are not currently parsed into the Go Skill struct; if/when they
	// arrive (via FrontmatterData.hooks) the permission gate will need to be
	// reconfirmed. For now treat absence as safe.
}

// skillHasOnlySafeProperties reports whether the skill is safe to auto-allow
// in default permission mode. Returns true when no "unsafe" property is set
// to a meaningful value.
func skillHasOnlySafeProperties(s *skills.Skill) bool {
	if s == nil {
		return true
	}
	if len(s.AllowedTools) > 0 {
		return false
	}
	if strings.TrimSpace(s.Shell) != "" {
		return false
	}
	return true
}

// MatchSkillRule evaluates a single skill permission rule (e.g. "review-pr",
// "review:*", "/commit", "myplugin:*") against a skill name. It mirrors TS
// ruleMatches inside SkillTool.checkPermissions — both inputs are normalized
// by stripping a leading slash, and rules ending in ":*" are prefix matches.
func MatchSkillRule(rule, skillName string) bool {
	rule = strings.TrimSpace(rule)
	skillName = strings.TrimSpace(skillName)
	if rule == "" || skillName == "" {
		return false
	}

	// Normalize leading slash on both sides.
	rule = strings.TrimPrefix(rule, "/")
	skillName = strings.TrimPrefix(skillName, "/")

	// Exact match.
	if rule == skillName {
		return true
	}

	// Plugin-namespaced wildcard: "plugin:*" matches anything starting with
	// "plugin:" and also a bare "plugin" base prefix (TS keeps everything that
	// startsWith(prefix)).
	if strings.HasSuffix(rule, ":*") {
		prefix := strings.TrimSuffix(rule, ":*")
		if prefix == "" {
			// "*:*" or ":*" — refuse to match as "match all" since TS treats
			// the empty prefix as a no-op.
			return false
		}
		return strings.HasPrefix(skillName, prefix)
	}

	return false
}

// FirstMatchingSkillRule returns the first rule from rules that matches the
// given skill name, or "" if none. Useful for both deny and allow rule sets.
func FirstMatchingSkillRule(rules []string, skillName string) string {
	for _, r := range rules {
		if MatchSkillRule(r, skillName) {
			return r
		}
	}
	return ""
}
