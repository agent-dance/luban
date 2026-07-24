package skills

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Budget constants for skill listing in system prompts.
// Aligns with TS prompt.ts in src/tools/SkillTool/prompt.ts.
const (
	// SkillBudgetContextPercent is the fraction of the context window allocated
	// to skill listing (1%).
	SkillBudgetContextPercent = 0.01

	// CharsPerToken is the average characters per token for budget calculation.
	CharsPerToken = 4

	// DefaultCharBudget is the fallback budget when no context window size
	// is provided: 1% of 200k tokens × 4 chars/token = 8000 chars.
	DefaultCharBudget = 8_000

	// MaxListingDescChars is the per-entry hard cap for skill descriptions.
	// The listing is for discovery only — the Skill tool loads full content on
	// invoke, so verbose whenToUse strings waste cache_creation tokens.
	MaxListingDescChars = 250

	// minDescLength is the minimum description length before falling back
	// to names-only mode for non-bundled skills.
	minDescLength = 20
)

// GetCharBudget returns the character budget for skill listing.
// If contextWindowTokens > 0, budget = tokens × CharsPerToken × SkillBudgetContextPercent.
// Otherwise returns DefaultCharBudget.
// Aligns with TS getCharBudget.
func GetCharBudget(contextWindowTokens int) int {
	if contextWindowTokens > 0 {
		return int(float64(contextWindowTokens) * CharsPerToken * SkillBudgetContextPercent)
	}
	return DefaultCharBudget
}

// getSkillDescription returns the description for prompt listing, combining
// description and whenToUse, truncated to MaxListingDescChars.
// Uses rune (character) count for truncation, aligning with TS
// desc.slice(0, MAX - 1) + '\u2026' which operates on UTF-16 code units.
// For BMP characters (ASCII, CJK, etc.) rune count == JS .length.
// Aligns with TS getCommandDescription.
func getSkillDescription(skill *Skill) string {
	desc := skill.EffectiveDescription()
	return truncateStr(desc, MaxListingDescChars)
}

// formatSkillEntry formats a single skill entry for the listing.
// Aligns with TS formatCommandDescription.
func formatSkillEntry(skill *Skill) string {
	return fmt.Sprintf("- %s: %s", skill.Name, getSkillDescription(skill))
}

// FormatSkillsWithinBudget formats skills for inclusion in system prompts,
// respecting a character budget. The strategy:
//
//  1. Try full descriptions for all skills
//  2. If over budget, partition into bundled (never truncated) and rest
//  3. Calculate max description length for non-bundled skills
//  4. If max < minDescLength, non-bundled go names-only
//
// Aligns with TS formatCommandsWithinBudget in src/tools/SkillTool/prompt.ts.
func FormatSkillsWithinBudget(skills []*Skill, contextWindowTokens int) string {
	if len(skills) == 0 {
		return ""
	}

	budget := GetCharBudget(contextWindowTokens)

	// Try full descriptions first
	type entry struct {
		skill *Skill
		full  string
	}
	entries := make([]entry, len(skills))
	fullTotal := 0
	for i, s := range skills {
		full := formatSkillEntry(s)
		entries[i] = entry{skill: s, full: full}
		fullTotal += runeLen(full)
	}
	// Add newline separators (N-1 for N entries)
	if len(entries) > 1 {
		fullTotal += len(entries) - 1
	}

	if fullTotal <= budget {
		lines := make([]string, len(entries))
		for i, e := range entries {
			lines[i] = e.full
		}
		return strings.Join(lines, "\n")
	}

	// Partition into bundled (never truncated) and rest
	bundledSet := make(map[int]bool)
	var restIndices []int
	for i, s := range skills {
		if s.Source == SourceBundled {
			bundledSet[i] = true
		} else {
			restIndices = append(restIndices, i)
		}
	}

	// Compute space used by bundled skills
	bundledChars := 0
	for i := range skills {
		if bundledSet[i] {
			bundledChars += runeLen(entries[i].full) + 1 // +1 for newline
		}
	}
	remainingBudget := budget - bundledChars

	if len(restIndices) == 0 {
		// Only bundled skills — show all full
		lines := make([]string, len(entries))
		for i, e := range entries {
			lines[i] = e.full
		}
		return strings.Join(lines, "\n")
	}

	// Calculate max description length for non-bundled
	restNameOverhead := 0
	for _, i := range restIndices {
		restNameOverhead += runeLen(skills[i].Name) + 4 // "- " + ": "
	}
	restNameOverhead += len(restIndices) - 1 // newlines between rest entries
	availableForDescs := remainingBudget - restNameOverhead
	maxDescLen := 0
	if len(restIndices) > 0 {
		maxDescLen = availableForDescs / len(restIndices)
	}

	if maxDescLen < minDescLength {
		// Extreme case: non-bundled go names-only, bundled keep descriptions
		lines := make([]string, len(skills))
		for i, s := range skills {
			if bundledSet[i] {
				lines[i] = entries[i].full
			} else {
				lines[i] = fmt.Sprintf("- %s", s.Name)
			}
		}
		return strings.Join(lines, "\n")
	}

	// Truncate non-bundled descriptions to fit within budget
	lines := make([]string, len(skills))
	for i, s := range skills {
		if bundledSet[i] {
			lines[i] = entries[i].full
		} else {
			desc := getSkillDescription(s)
			if runeLen(desc) > maxDescLen {
				desc = truncateStr(desc, maxDescLen)
			}
			lines[i] = fmt.Sprintf("- %s: %s", s.Name, desc)
		}
	}
	return strings.Join(lines, "\n")
}

// runeLen returns the number of runes (characters) in a string.
// This is used instead of len() for budget calculations to align with TS
// string .length which counts UTF-16 code units (same as rune count for BMP).
func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

// truncateStr truncates a string to at most maxLen characters (runes),
// appending "…" if truncation occurs. This aligns with TS
// desc.slice(0, maxLen - 1) + '\u2026' which operates on character indices.
// Using rune count ensures we never slice in the middle of a multi-byte
// UTF-8 character (e.g., CJK, emoji).
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// GetSkillToolPrompt returns the base prompt text for the SkillTool.
// Aligns with TS getPrompt in src/tools/SkillTool/prompt.ts.
func GetSkillToolPrompt() string {
	return `Execute a skill within the main conversation

When users ask you to perform tasks, check if any of the available skills match. Skills provide specialized capabilities and domain knowledge.

When users reference a "slash command" or "/<something>" (e.g., "/commit", "/review-pr"), they are referring to a skill. Use this tool to invoke it.

How to invoke:
- Use this tool with the skill name and optional arguments
- Examples:
  - skill: "pdf" - invoke the pdf skill
  - skill: "commit", args: "-m 'Fix bug'" - invoke with arguments
  - skill: "review-pr", args: "123" - invoke with arguments
  - skill: "ms-office-suite:pdf" - invoke using fully qualified name

Important:
- Available skills are listed in system-reminder messages in the conversation
- When a skill matches the user's request, this is a BLOCKING REQUIREMENT: invoke the relevant Skill tool BEFORE generating any other response about the task
- NEVER mention a skill without actually calling this tool
- Do not invoke a skill that is already running
- Do not use this tool for built-in CLI commands (like /help, /clear, etc.)
- If you see a <command-name> tag in the current conversation turn, the skill has ALREADY been loaded - follow the instructions directly instead of calling this tool again`
}

// FormatSkillListing creates a complete skill listing string suitable for
// injection into system-reminder blocks. It includes a header and the formatted
// skills within budget.
// Aligns with TS normalizeAttachmentForAPI case 'skill_listing' in messages.ts.
func FormatSkillListing(skills []*Skill, contextWindowTokens int) string {
	if len(skills) == 0 {
		return ""
	}

	formatted := FormatSkillsWithinBudget(skills, contextWindowTokens)
	if formatted == "" {
		return ""
	}

	return fmt.Sprintf("The following skills are available for use with the Skill tool:\n\n%s", formatted)
}

// WrapInSystemReminder wraps content in <system-reminder> tags.
// Aligns with TS normalizeAttachmentForAPI case 'skill_listing'.
func WrapInSystemReminder(content string) string {
	if content == "" {
		return ""
	}
	return "<system-reminder>\n" + content + "\n</system-reminder>"
}

// FilterModelInvocableSkills returns only skills that can be invoked by the model.
// Aligns with TS getSkillToolCommands which checks:
//   - !disableModelInvocation
//   - source !== 'builtin' (no SourceBuiltin in Go yet, but guarded for future)
//   - source is bundled/skills/commands_legacy/project/user/managed, OR
//     has hasUserSpecifiedDescription OR has whenToUse
//
// Skills from plugin/MCP sources without a user-specified description or
// whenToUse are excluded (they lack enough context for the model to invoke).
func FilterModelInvocableSkills(skills []*Skill) []*Skill {
	var result []*Skill
	for _, s := range skills {
		if IsModelInvocableSkill(s) {
			result = append(result, s)
		}
	}
	return result
}

// IsModelInvocableSkill reports whether one skill belongs in the model-facing
// catalog. Runtime session availability is intentionally evaluated by Manager
// callers so frontmatter policy and live /skills state remain separate layers.
func IsModelInvocableSkill(s *Skill) bool {
	if s == nil || s.DisableModelInvocation {
		return false
	}
	// TS: source !== 'builtin'
	// There's no "builtin" source for skills (builtin is for CLI commands).
	// Guard it anyway for forward-compatibility.
	if s.Source == "builtin" {
		return false
	}
	// TS: loadedFrom is bundled/skills/commands_DEPRECATED OR
	// has hasUserSpecifiedDescription OR has whenToUse
	switch s.Source {
	case SourceBundled, SourceProject, SourceUser, SourceManaged, SourceCommandsLegacy:
		return true
	default:
		// plugin/MCP sources need enough context for the model to invoke them.
		return s.HasUserSpecifiedDescription || s.WhenToUse != ""
	}
}

// FilterUserInvocableSkills returns only skills that can be invoked by users
// (via /slash-command).
// Aligns with TS getSlashCommandToolSkills.
func FilterUserInvocableSkills(skills []*Skill) []*Skill {
	var result []*Skill
	for _, s := range skills {
		if s.IsUserInvocable() {
			result = append(result, s)
		}
	}
	return result
}

// IsUserInvocable returns whether the skill can be invoked by users via /command.
// If UserInvocable is nil, the default depends on the source:
//   - commands_legacy: true
//   - everything else: true (skills/ default to user-invocable: true in TS)
func (s *Skill) IsUserInvocable() bool {
	if s.UserInvocable != nil {
		return *s.UserInvocable
	}
	// TS default: userInvocable = frontmatter['user-invocable'] === undefined ? true : ...
	return true
}
