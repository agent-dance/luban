package skills

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterRegex matches YAML frontmatter between --- delimiters.
// Aligns with TS FRONTMATTER_REGEX in src/utils/frontmatterParser.ts.
var frontmatterRegex = regexp.MustCompile(`(?s)^---\s*\n(.*?)---\s*\n?`)

// rawFrontmatter holds the raw YAML fields parsed from a skill file.
// Matches TS FrontmatterData type in src/utils/frontmatterParser.ts.
type rawFrontmatter struct {
	Description            *string     `yaml:"description"`
	AllowedTools           yamlStrings `yaml:"allowed-tools"`
	ArgumentHint           *string     `yaml:"argument-hint"`
	Arguments              yamlStrings `yaml:"arguments"`
	WhenToUse              *string     `yaml:"when_to_use"`
	Version                *string     `yaml:"version"`
	Model                  *string     `yaml:"model"`
	DisableModelInvocation *string     `yaml:"disable-model-invocation"`
	UserInvocable          *string     `yaml:"user-invocable"`
	Hooks                  any         `yaml:"hooks"`
	Context                *string     `yaml:"context"`
	Agent                  *string     `yaml:"agent"`
	Effort                 *string     `yaml:"effort"`
	Paths                  yamlStrings `yaml:"paths"`
	Shell                  *string     `yaml:"shell"`
	Name                   *string     `yaml:"name"`
}

// yamlStrings supports both a single string ("a, b") and a YAML list (["a", "b"]).
type yamlStrings []string

func (s *yamlStrings) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		// Single string — split by comma (respecting braces for glob patterns)
		*s = splitCommaSafe(value.Value)
		return nil
	case yaml.SequenceNode:
		// YAML list
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*s = items
		return nil
	default:
		*s = nil
		return nil
	}
}

// splitCommaSafe splits by comma but not inside curly braces, matching the
// TS splitPathInFrontmatter logic for handling glob patterns like *.{ts,tsx}.
func splitCommaSafe(s string) []string {
	var parts []string
	var current strings.Builder
	braceDepth := 0

	for _, ch := range s {
		switch ch {
		case '{':
			braceDepth++
			current.WriteRune(ch)
		case '}':
			braceDepth--
			current.WriteRune(ch)
		case ',':
			if braceDepth == 0 {
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					parts = append(parts, trimmed)
				}
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	trimmed := strings.TrimSpace(current.String())
	if trimmed != "" {
		parts = append(parts, trimmed)
	}
	return parts
}

// yamlSpecialChars matches characters that need quoting in YAML values.
// Aligns with TS YAML_SPECIAL_CHARS regex.
var yamlSpecialChars = regexp.MustCompile(`[{}\[\]*&#!|>%@` + "`" + `]|: `)

// simpleKVLine matches a simple "key: value" line (not indented, not list).
var simpleKVLine = regexp.MustCompile(`^([a-zA-Z_-]+):\s+(.+)$`)

// quoteProblematicValues pre-processes frontmatter text to quote values
// containing special YAML characters. Aligns with TS quoteProblematicValues.
func quoteProblematicValues(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		m := simpleKVLine.FindStringSubmatch(line)
		if m != nil {
			key, value := m[1], m[2]

			// Skip if already quoted
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				result = append(result, line)
				continue
			}

			// Quote if contains special YAML characters
			if yamlSpecialChars.MatchString(value) {
				escaped := strings.ReplaceAll(value, `\`, `\\`)
				escaped = strings.ReplaceAll(escaped, `"`, `\"`)
				result = append(result, key+`: "`+escaped+`"`)
				continue
			}
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// ParsedMarkdown holds the result of parsing a markdown file with frontmatter.
type ParsedMarkdown struct {
	Frontmatter rawFrontmatter
	Content     string // markdown content with frontmatter stripped
}

// parseFrontmatter extracts YAML frontmatter from markdown content.
// Aligns with TS parseFrontmatter in src/utils/frontmatterParser.ts.
func parseFrontmatter(markdown, sourcePath string) ParsedMarkdown {
	match := frontmatterRegex.FindStringSubmatchIndex(markdown)
	if match == nil {
		return ParsedMarkdown{Content: markdown}
	}

	// match[2]:match[3] is the captured group (frontmatter text)
	fmText := markdown[match[2]:match[3]]
	// Content starts after the entire match
	content := markdown[match[1]:]

	var fm rawFrontmatter

	// Try parsing YAML directly
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		// Retry with problematic values quoted
		quoted := quoteProblematicValues(fmText)
		if err2 := yaml.Unmarshal([]byte(quoted), &fm); err2 != nil {
			// Both attempts failed — return content without frontmatter
			return ParsedMarkdown{Content: content}
		}
	}

	return ParsedMarkdown{
		Frontmatter: fm,
		Content:     content,
	}
}

// applyFrontmatter populates a Skill from parsed frontmatter fields.
func applyFrontmatter(skill *Skill, fm rawFrontmatter) {
	if fm.Name != nil && *fm.Name != "" {
		skill.Name = *fm.Name
	}
	if fm.Description != nil && *fm.Description != "" {
		skill.Description = *fm.Description
		skill.HasUserSpecifiedDescription = true
		skill.HasGeneratedDescription = false
	}
	if len(fm.AllowedTools) > 0 {
		skill.AllowedTools = fm.AllowedTools
	}
	if fm.ArgumentHint != nil {
		skill.ArgumentHint = *fm.ArgumentHint
	}
	if len(fm.Arguments) > 0 {
		skill.ArgNames = fm.Arguments
	}
	if fm.WhenToUse != nil {
		skill.WhenToUse = *fm.WhenToUse
	}
	if fm.Version != nil {
		skill.Version = *fm.Version
	}
	if fm.Model != nil {
		skill.Model = *fm.Model
	}
	if fm.DisableModelInvocation != nil {
		skill.DisableModelInvocation = parseBoolString(*fm.DisableModelInvocation)
	}
	if fm.UserInvocable != nil {
		v := parseBoolString(*fm.UserInvocable)
		skill.UserInvocable = &v
	}
	if fm.Context != nil {
		switch *fm.Context {
		case "fork":
			skill.Context = ContextFork
		default:
			skill.Context = ContextInline
		}
	}
	if fm.Agent != nil {
		skill.Agent = *fm.Agent
	}
	if fm.Effort != nil {
		skill.Effort = *fm.Effort
	}
	if len(fm.Paths) > 0 {
		skill.Paths = fm.Paths
	}
	if fm.Shell != nil {
		shell := strings.ToLower(strings.TrimSpace(*fm.Shell))
		if shell == "bash" || shell == "powershell" {
			skill.Shell = shell
		}
	}
}

// parseBoolString returns true for "true", false otherwise.
// Matches TS parseBooleanFrontmatter.
func parseBoolString(s string) bool {
	return strings.TrimSpace(strings.ToLower(s)) == "true"
}
