package shell

import "strings"

// extractPathsFromPowerShellCommand returns literal path-like tokens without
// executing PowerShell. It handles quoted paths, redirections, cmdlet
// arguments, and compact assignments such as $target=C:\work\file.txt.
func extractPathsFromPowerShellCommand(command string) []string {
	seen := make(map[string]struct{})
	var paths []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		if value == "" || !looksLikePath(value) {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		paths = append(paths, value)
	}
	for _, token := range tokenizePowerShellLiterals(command) {
		add(token)
		if index := strings.IndexByte(token, '='); index >= 0 && index+1 < len(token) {
			add(token[index+1:])
		}
	}
	return paths
}

// dynamicPowerShellPathReference reports path-shaped variable expansion that
// cannot be resolved safely before process launch. Scoped child agents reject
// these references instead of treating them as relative literal paths.
func dynamicPowerShellPathReference(command string) string {
	for _, token := range tokenizePowerShellLiterals(command) {
		candidate := token
		if index := strings.IndexByte(candidate, '='); index >= 0 && index+1 < len(candidate) {
			candidate = candidate[index+1:]
		}
		if strings.Contains(candidate, "$") {
			return candidate
		}
	}
	return ""
}

func tokenizePowerShellLiterals(command string) []string {
	var tokens []string
	var current strings.Builder
	var quote byte
	escaped := false
	flush := func() {
		if token := strings.TrimSpace(current.String()); token != "" {
			tokens = append(tokens, token)
		}
		current.Reset()
	}
	for i := 0; i < len(command); i++ {
		char := command[i]
		if escaped {
			current.WriteByte(char)
			escaped = false
			continue
		}
		if char == '`' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				if quote == '\'' && i+1 < len(command) && command[i+1] == '\'' {
					current.WriteByte(char)
					i++
					continue
				}
				quote = 0
				continue
			}
			current.WriteByte(char)
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case ' ', '\t', '\r', '\n', ';', '|', '>', '<', '(', ')', '{', '}', '[', ']', ',':
			flush()
		default:
			current.WriteByte(char)
		}
	}
	if escaped {
		current.WriteByte('`')
	}
	flush()
	return tokens
}
