package skills

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// SubstituteArguments replaces $ARGUMENTS placeholders in skill content with
// actual argument values. This aligns with TS substituteArguments in
// src/utils/argumentSubstitution.ts.
//
// Supported placeholders:
//   - $ARGUMENTS — replaced with the full arguments string
//   - $ARGUMENTS[0], $ARGUMENTS[1], etc. — replaced with individual indexed arguments
//   - $0, $1, etc. — shorthand for $ARGUMENTS[0], $ARGUMENTS[1]
//   - Named arguments (e.g., $foo, $bar) — when argument names are defined in frontmatter
//
// If args is nil (pointer), the content is returned unchanged.
// If args points to an empty string, placeholders are replaced with empty strings.
// If appendIfNoPlaceholder is true and no placeholders were found, "ARGUMENTS: {args}"
// is appended to the content (only if args is non-empty).
func SubstituteArguments(content string, args *string, appendIfNoPlaceholder bool, argNames []string) string {
	// nil means no args provided — return content unchanged.
	// Empty string is valid input that should replace placeholders with empty.
	if args == nil {
		return content
	}

	argsStr := *args
	parsedArgs := ParseArguments(argsStr)
	originalContent := content

	// Step 1: Replace named arguments (e.g., $foo, $bar) with their values.
	// Named arguments map to positions: argNames[0] -> parsedArgs[0], etc.
	for i, name := range argNames {
		if name == "" {
			continue
		}
		// Match $name but not $name[...] or $nameXxx (word boundary).
		// TS regex: \$name(?![\[\w])
		content = replaceWithLookahead(content, name, parsedArgs, i)
	}

	// Step 2: Replace indexed arguments $ARGUMENTS[N]
	content = indexedArgRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := indexedArgRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		idx, err := strconv.Atoi(sub[1])
		if err != nil {
			return match
		}
		if idx < len(parsedArgs) {
			return parsedArgs[idx]
		}
		return ""
	})

	// Step 3: Replace shorthand indexed arguments $N (but not $Nword).
	// TS regex: \$(\d+)(?!\w) — uses negative lookahead.
	// Go doesn't support lookahead, so we use manual scanning to avoid
	// consuming the trailing character (which would break adjacent $N$M patterns).
	content = replaceShorthandArgs(content, parsedArgs)

	// Step 4: Replace $ARGUMENTS with the full arguments string
	content = strings.ReplaceAll(content, "$ARGUMENTS", argsStr)

	// Step 5: If no placeholders were found and appendIfNoPlaceholder is true,
	// append "ARGUMENTS: {args}" (only if args is non-empty).
	if content == originalContent && appendIfNoPlaceholder && argsStr != "" {
		content = content + "\n\nARGUMENTS: " + argsStr
	}

	return content
}

// Pre-compiled regex patterns for argument substitution.
var (
	// $ARGUMENTS[0], $ARGUMENTS[1], etc.
	indexedArgRe = regexp.MustCompile(`\$ARGUMENTS\[(\d+)\]`)
)

// replaceWithLookahead replaces $name in content with the corresponding parsed
// argument value, ensuring the match is not followed by [ or word characters.
// This mimics the TS regex \$name(?![\[\w]).
func replaceWithLookahead(content, name string, parsedArgs []string, idx int) string {
	target := "$" + name
	replacement := ""
	if idx < len(parsedArgs) {
		replacement = parsedArgs[idx]
	}

	var result strings.Builder
	i := 0
	for i < len(content) {
		// Find next occurrence of $name
		pos := strings.Index(content[i:], target)
		if pos == -1 {
			result.WriteString(content[i:])
			break
		}

		// Write everything before the match
		result.WriteString(content[i : i+pos])

		afterPos := i + pos + len(target)

		// Check the character after $name — must NOT be [ or \w
		if afterPos < len(content) {
			ch := content[afterPos]
			if ch == '[' || isWordChar(ch) {
				// Not a valid match — write the target literally and continue
				result.WriteString(target)
				i = afterPos
				continue
			}
		}

		// Valid match — substitute
		result.WriteString(replacement)
		i = afterPos
	}

	return result.String()
}

// isWordChar returns true if ch is a word character [a-zA-Z0-9_].
func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_'
}

// replaceShorthandArgs replaces $N shorthand placeholders with the corresponding
// parsed argument value. This mimics the TS regex \$(\d+)(?!\w) using manual
// scanning, which correctly handles adjacent patterns like "$0$1" where $0 is
// followed by $ (not a word char), so both should be replaced.
func replaceShorthandArgs(content string, parsedArgs []string) string {
	var result strings.Builder
	i := 0
	for i < len(content) {
		if content[i] != '$' {
			result.WriteByte(content[i])
			i++
			continue
		}

		// Found '$' — try to parse digits after it
		j := i + 1
		for j < len(content) && content[j] >= '0' && content[j] <= '9' {
			j++
		}

		// No digits after '$' — write '$' literally
		if j == i+1 {
			result.WriteByte('$')
			i++
			continue
		}

		// Check negative lookahead: next char must NOT be a word character.
		// If it IS a word char, this is not a match (e.g., $0abc).
		if j < len(content) && isWordChar(content[j]) {
			// Not a match — write the "$N..." literally
			result.WriteString(content[i:j])
			i = j
			continue
		}

		// Valid match: parse the index and replace
		idx, err := strconv.Atoi(content[i+1 : j])
		if err != nil {
			// Shouldn't happen since we validated digits, but be safe
			result.WriteString(content[i:j])
			i = j
			continue
		}

		if idx < len(parsedArgs) {
			result.WriteString(parsedArgs[idx])
		}
		// Out-of-range index → replace with empty string (TS behavior)
		i = j
	}
	return result.String()
}

// ParseArguments parses an arguments string into individual arguments.
// Uses shell-like quoting: quoted strings ("..." or '...') are kept as
// one token with quotes stripped, backslash escapes are supported outside
// single quotes (matching TS shell-quote behavior). Unquoted text is
// split by whitespace.
// Aligns with TS parseArguments in src/utils/argumentSubstitution.ts.
func ParseArguments(args string) []string {
	args = strings.TrimSpace(args)
	if args == "" {
		return nil
	}

	var result []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(args); i++ {
		ch := args[i]

		switch {
		case ch == '\\' && !inSingle:
			// Backslash escape: consume next character literally.
			// Inside single quotes, backslash is literal (POSIX behavior).
			if i+1 < len(args) {
				i++
				current.WriteByte(args[i])
			} else {
				current.WriteByte(ch)
			}
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case (ch == ' ' || ch == '\t') && !inSingle && !inDouble:
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// ParseArgumentNames parses argument names from the frontmatter 'arguments' field.
// Filters out empty strings and numeric-only names (which conflict with $0, $1 shorthand).
// Aligns with TS parseArgumentNames.
func ParseArgumentNames(names []string) []string {
	var result []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Skip numeric-only names (conflict with $0, $1 shorthand)
		if _, err := strconv.Atoi(name); err == nil {
			continue
		}
		result = append(result, name)
	}
	return result
}

// GenerateProgressiveArgumentHint generates an argument hint showing remaining
// unfilled args. Returns empty string if all arguments are filled.
// Aligns with TS generateProgressiveArgumentHint.
func GenerateProgressiveArgumentHint(argNames []string, typedArgs []string) string {
	if len(typedArgs) >= len(argNames) {
		return ""
	}
	remaining := argNames[len(typedArgs):]
	parts := make([]string, len(remaining))
	for i, name := range remaining {
		parts[i] = "[" + name + "]"
	}
	return strings.Join(parts, " ")
}

// SubstituteVariables replaces template variables in skill content:
//   - ${CLAUDE_SKILL_DIR} → skill directory path
//   - ${CLAUDE_SESSION_ID} → current session ID
//
// On Windows, backslashes in skillDir are normalized to forward slashes.
// Aligns with TS getPromptForCommand in src/skills/loadSkillsDir.ts.
func SubstituteVariables(content, skillDir, sessionID string) string {
	// Replace ${CLAUDE_SKILL_DIR}
	if skillDir != "" {
		dir := skillDir
		if runtime.GOOS == "windows" {
			dir = strings.ReplaceAll(dir, `\`, "/")
		}
		content = strings.ReplaceAll(content, "${CLAUDE_SKILL_DIR}", dir)
	}

	// Replace ${CLAUDE_SESSION_ID}
	if sessionID != "" {
		content = strings.ReplaceAll(content, "${CLAUDE_SESSION_ID}", sessionID)
	}

	return content
}

// PrepareSkillContent applies the full substitution pipeline to skill content.
// This is the Go equivalent of the TS getPromptForCommand callback:
//
//  1. Prepend base-dir header (if skillDir is set)
//  2. SubstituteArguments (named, indexed, shorthand, $ARGUMENTS, fallback append)
//  3. SubstituteVariables (${CLAUDE_SKILL_DIR}, ${CLAUDE_SESSION_ID})
//
// Shell command execution (``!`cmd` `` and ``` ```! cmd ``` ```) is NOT performed
// here — it requires integration with the permission system and BashTool, which
// is handled at the tool execution layer.
func PrepareSkillContent(skill *Skill, args *string, sessionID string) string {
	content := skill.Content

	// Step 1: Prepend base directory header
	if skill.SkillDir != "" {
		content = fmt.Sprintf("Base directory for this skill: %s\n\n%s", skill.SkillDir, content)
	}

	// Step 2: Substitute arguments
	argNames := ParseArgumentNames(skill.ArgNames)
	content = SubstituteArguments(content, args, true, argNames)

	// Step 3: Substitute variables
	content = SubstituteVariables(content, skill.SkillDir, sessionID)

	return content
}

// Shell command patterns for future implementation.
// These match the TS BLOCK_PATTERN and INLINE_PATTERN in promptShellExecution.ts.
var (
	// ```! command ``` — code block shell command
	ShellBlockPattern = regexp.MustCompile("(?s)```!\\s*\n?([\\s\\S]*?)\n?```")

	// !`command` — inline shell command (preceded by whitespace or start of line)
	ShellInlinePattern = regexp.MustCompile("(?m)(?:^|\\s)!`([^`]+)`")
)

// HasShellCommands reports whether the content contains embedded shell commands
// (``!`cmd` `` or ``` ```! cmd ``` ```). This can be used to check whether shell
// execution is needed before performing the expensive permission/execution flow.
func HasShellCommands(content string) bool {
	// Fast path: check for !` substring (93% of skills have none)
	if !strings.Contains(content, "!`") && !strings.Contains(content, "```!") {
		return false
	}
	return ShellBlockPattern.MatchString(content) || ShellInlinePattern.MatchString(content)
}
