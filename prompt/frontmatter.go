package prompt

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var memoryFrontmatterRegex = regexp.MustCompile(`(?s)^---\s*\n(.*?)---\s*\n?`)

type memoryFrontmatter struct {
	Paths yamlStringList `yaml:"paths"`
}

type yamlStringList []string

func (s *yamlStringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = splitFrontmatterPaths(value.Value)
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*s = items
	default:
		*s = nil
	}
	return nil
}

func parseMemoryFrontmatterPaths(raw string) (content string, paths []string) {
	match := memoryFrontmatterRegex.FindStringSubmatchIndex(raw)
	if match == nil {
		return raw, nil
	}

	fmText := raw[match[2]:match[3]]
	content = raw[match[1]:]

	var fm memoryFrontmatter
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		if err := yaml.Unmarshal([]byte(quoteMemoryFrontmatterValues(fmText)), &fm); err != nil {
			return content, nil
		}
	}

	for _, pattern := range fm.Paths {
		pattern = strings.TrimSpace(pattern)
		pattern = strings.TrimSuffix(pattern, "/**")
		if pattern != "" {
			paths = append(paths, pattern)
		}
	}
	if len(paths) == 0 {
		return content, nil
	}
	allMatchAll := true
	for _, pattern := range paths {
		if pattern != "**" {
			allMatchAll = false
			break
		}
	}
	if allMatchAll {
		return content, nil
	}
	return content, paths
}

func splitFrontmatterPaths(s string) []string {
	var parts []string
	var current strings.Builder
	braceDepth := 0
	for _, ch := range s {
		switch ch {
		case '{':
			braceDepth++
			current.WriteRune(ch)
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
			current.WriteRune(ch)
		case ',':
			if braceDepth == 0 {
				if trimmed := strings.TrimSpace(current.String()); trimmed != "" {
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
	if trimmed := strings.TrimSpace(current.String()); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return parts
}

var memoryYAMLSpecialChars = regexp.MustCompile(`[{}\[\]*&#!|>%@` + "`" + `]|: `)
var memorySimpleKVLine = regexp.MustCompile(`^([a-zA-Z_-]+):\s+(.+)$`)

func quoteMemoryFrontmatterValues(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		m := memorySimpleKVLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value := m[2]
		if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
			(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
			continue
		}
		if memoryYAMLSpecialChars.MatchString(value) {
			escaped := strings.ReplaceAll(value, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			lines[i] = m[1] + `: "` + escaped + `"`
		}
	}
	return strings.Join(lines, "\n")
}
