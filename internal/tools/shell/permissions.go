package shell

import (
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
	"mvdan.cc/sh/v3/syntax"
)

// BashRulePattern is the parsed shape of a "Bash(<name>[:<args>][!])" rule.
//
// Grammar:
//
//	Bash(<name>)            — match the literal command name (first token)
//	Bash(<name>:<args>)     — also match the joined remainder
//	Bash(<name>:*)          — match any args after <name>
//	Bash(*)                 — match any command (catch-all)
//	Trailing "!"            — flips the decision to Deny
//
// `Args` follows shell-glob semantics via filepath.Match. A pattern of "*"
// matches any args (including none). A trailing "!" forces a Deny decision
// regardless of the rule's stored Decision.
type BashRulePattern struct {
	Name    string // command literal or "*" for any
	Args    string // optional args glob ("" means args must be empty)
	Deny    bool   // trailing "!" — overrides the rule decision
	HasArgs bool   // true when ":<args>" was present
}

// parseBashPattern parses a permission Pattern that comes from a permissions.Rule
// whose Tool field is "Bash". The pattern is the body of "Bash(...)" — i.e. the
// caller has already stripped the "Bash(" prefix and ")" suffix when needed.
//
// Accepts both the bare body ("npm test") and the wrapped form ("Bash(npm test)").
func parseBashPattern(pattern string) (BashRulePattern, bool) {
	out := BashRulePattern{}
	p := strings.TrimSpace(pattern)
	if p == "" {
		return out, false
	}
	// Allow "Bash(...)" wrapper for convenience.
	if strings.HasPrefix(p, "Bash(") && strings.HasSuffix(p, ")") {
		p = strings.TrimSuffix(strings.TrimPrefix(p, "Bash("), ")")
		p = strings.TrimSpace(p)
	}
	if p == "" {
		return out, false
	}
	// Trailing "!" => deny override.
	if strings.HasSuffix(p, "!") {
		out.Deny = true
		p = strings.TrimSpace(strings.TrimSuffix(p, "!"))
		if p == "" {
			return out, false
		}
	}
	// Catch-all "*"
	if p == "*" {
		out.Name = "*"
		return out, true
	}
	// Split on the first ":" — everything after is the args glob.
	if idx := strings.Index(p, ":"); idx >= 0 {
		out.Name = strings.TrimSpace(p[:idx])
		out.Args = strings.TrimSpace(p[idx+1:])
		out.HasArgs = true
		if out.Deny && out.Args == "" {
			out.Args = "*"
		}
		if out.Name == "" {
			return out, false
		}
		return out, true
	}
	// No colon — the whole pattern is the command (possibly "name args literal").
	// We accept either a single word OR "<name> <args>" (literal match).
	parts := strings.Fields(p)
	out.Name = parts[0]
	if len(parts) > 1 {
		out.Args = strings.Join(parts[1:], " ")
		out.HasArgs = true
	}
	return out, true
}

// matchesBashPattern reports whether `pattern` matches the parsed command
// (firstName + remainder).
func (p BashRulePattern) matches(firstName, remainder string, stripExecutablePath bool) bool {
	// Catch-all "*"
	if p.Name == "*" && !p.HasArgs {
		return true
	}
	// Deny/Ask rules intentionally cover alternate executable paths. Allow
	// rules preserve the path so `git` cannot authorize `/tmp/evil/git`.
	stripped := firstName
	if stripExecutablePath {
		if idx := strings.LastIndexAny(stripped, `/\\`); idx >= 0 {
			stripped = stripped[idx+1:]
		}
	}
	nameMatched := p.Name == stripped
	if !nameMatched && strings.ContainsAny(p.Name, "*?[") {
		nameMatched, _ = filepath.Match(p.Name, stripped)
	}
	if !nameMatched {
		return false
	}
	if !p.HasArgs {
		return true
	}
	if p.Args == "*" {
		return true
	}
	if p.Args == "" {
		return strings.TrimSpace(remainder) == ""
	}
	if p.Args == strings.TrimSpace(remainder) {
		return true
	}
	matched, err := filepath.Match(p.Args, strings.TrimSpace(remainder))
	if err == nil && matched {
		return true
	}
	return strings.HasPrefix(strings.TrimSpace(remainder), p.Args+" ")
}

func executableBase(name string) string {
	stripped := name
	if idx := strings.LastIndexAny(stripped, `/\\`); idx >= 0 {
		stripped = stripped[idx+1:]
	}
	return stripped
}

// firstCallToken extracts the first call's command name and its remainder
// (the joined args). Returns ("", "") when the command cannot be parsed.
func firstCallToken(cmd string) (string, string) {
	if strings.TrimSpace(cmd) == "" {
		return "", ""
	}
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return "", ""
	}
	var name, remainder string
	syntax.Walk(prog, func(node syntax.Node) bool {
		if name != "" {
			return false
		}
		if call, ok := node.(*syntax.CallExpr); ok {
			n := cmdName(call)
			if n == "" {
				return true
			}
			name = n
			remainder = strings.Join(argLiterals(call), " ")
			return false
		}
		return true
	})
	return name, remainder
}

func matchBashRuleDetailed(cmd string, rules []permissions.Rule) (permissions.Decision, *permissions.Rule, bool) {
	if strings.TrimSpace(cmd) == "" {
		return permissions.DecisionAsk, nil, false
	}
	invocations := bashRuleInvocations(cmd)
	if len(invocations) == 0 {
		return permissions.DecisionAsk, nil, false
	}

	var firstAllow, firstAsk *permissions.Rule
	unmatched := false
	for _, invocation := range invocations {
		decision, matched := matchBashInvocation(invocation, rules)
		if decision == permissions.DecisionDeny {
			return decision, matched, false
		}
		if matched == nil {
			// A compound command is only auto-allowed when every executable
			// segment is covered. A partial match must fall back to Ask.
			unmatched = true
			continue
		}
		if decision == permissions.DecisionAsk {
			if firstAsk == nil {
				firstAsk = matched
			}
			continue
		}
		if firstAllow == nil {
			firstAllow = matched
		}
	}
	if unmatched {
		if firstAsk != nil {
			return permissions.DecisionAsk, firstAsk, true
		}
		if firstAllow != nil {
			return permissions.DecisionAsk, firstAllow, true
		}
	}
	if firstAsk != nil {
		return permissions.DecisionAsk, firstAsk, false
	}
	if firstAllow != nil {
		return permissions.DecisionAllow, firstAllow, false
	}
	return permissions.DecisionAsk, nil, false
}

type bashRuleInvocation struct {
	rawName         string
	rawRemainder    string
	name            string
	remainder       string
	allowRaw        bool
	allowNormalized bool
	restrictiveOnly bool
}

func bashRuleInvocations(cmd string) []bashRuleInvocation {
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil
	}
	var invocations []bashRuleInvocation
	syntax.Walk(prog, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		rawName := bashRuleExecutableLiteral(call)
		if rawName == "" {
			return true
		}
		name, args, _, _ := unwrapShellPolicyCall(call, map[string]shellPolicyValue{}, DefaultShellPolicyContext())
		if name == "" {
			name = executableBase(rawName)
			args = call.Args[1:]
		}
		invocation := bashRuleInvocation{
			rawName: rawName, rawRemainder: strings.Join(argLiterals(call), " "),
			name: name, remainder: strings.Join(shellRuleWordLiterals(args), " "),
			allowRaw:        len(call.Assigns) == 0,
			allowNormalized: bashRuleAllowTransparent(call),
		}
		invocations = append(invocations, invocation)
		for _, nested := range bashRuleNestedInvocations(name, args) {
			nested.restrictiveOnly = true
			nested.allowRaw = false
			nested.allowNormalized = false
			invocations = append(invocations, nested)
		}
		return true
	})
	return invocations
}

func matchBashInvocation(invocation bashRuleInvocation, rules []permissions.Rule) (permissions.Decision, *permissions.Rule) {
	var firstAllow, firstAsk *permissions.Rule
	for i := range rules {
		rule := &rules[i]
		if !ruleAppliesToBash(rule.Tool) {
			continue
		}
		pat, ok := parseBashPattern(rule.Pattern)
		if !ok {
			continue
		}
		effectiveDecision := rule.Decision
		if pat.Deny {
			effectiveDecision = permissions.DecisionDeny
		}
		restrictive := effectiveDecision == permissions.DecisionDeny || effectiveDecision == permissions.DecisionAsk
		rawMatch := (!invocation.restrictiveOnly || restrictive) &&
			(restrictive || invocation.allowRaw) && pat.matches(invocation.rawName, invocation.rawRemainder, restrictive)
		normalizedMatch := (restrictive || invocation.allowNormalized) && pat.matches(invocation.name, invocation.remainder, restrictive)
		if !rawMatch && !normalizedMatch {
			continue
		}
		if effectiveDecision == permissions.DecisionDeny {
			return permissions.DecisionDeny, rule
		}
		switch effectiveDecision {
		case permissions.DecisionAllow, permissions.DecisionAllowOnce:
			if firstAllow == nil {
				firstAllow = rule
			}
		case permissions.DecisionAsk:
			if firstAsk == nil {
				firstAsk = rule
			}
		}
	}
	if firstAsk != nil {
		return permissions.DecisionAsk, firstAsk
	}
	if firstAllow != nil {
		return permissions.DecisionAllow, firstAllow
	}
	return permissions.DecisionAsk, nil
}

func bashRuleExecutableLiteral(call *syntax.CallExpr) string {
	if call == nil || len(call.Args) == 0 {
		return ""
	}
	return wordToString(call.Args[0])
}

func shellRuleWordLiterals(words []*syntax.Word) []string {
	values := make([]string, 0, len(words))
	for _, word := range words {
		values = append(values, wordToString(word))
	}
	return values
}

func bashRuleAllowTransparent(call *syntax.CallExpr) bool {
	if call == nil || len(call.Assigns) != 0 {
		return false
	}
	words := call.Args
	for len(words) > 0 {
		executable := wordToString(words[0])
		if executable == "" {
			return false
		}
		switch executableBase(executable) {
		case "command":
			words = skipCommandWrapper(words[1:])
		case "builtin":
			words = skipBuiltinWrapper(words[1:])
		case "exec":
			words = skipExecWrapper(words[1:])
		case "nohup":
			words = skipNohupWrapper(words[1:])
		case "time":
			var decision types.PolicyDecision
			words, decision = skipTimeWrapper(words[1:], map[string]shellPolicyValue{}, DefaultShellPolicyContext())
			if decision.Disposition != types.PolicyAllow {
				return false
			}
		case "timeout":
			words = skipTimeoutWrapper(words[1:])
		case "nice":
			words = skipNiceWrapper(words[1:])
		case "stdbuf":
			words = skipStdbufWrapper(words[1:])
		case "env", "sudo", "doas", "ionice", "unbuffer", "taskset", "chrt", "busybox", "toybox", "bash", "sh", "zsh", "dash", "ash", "ksh", "eval", "xargs", "find":
			return false
		default:
			return !strings.ContainsAny(executable, `/\\`)
		}
		if words == nil {
			return false
		}
	}
	return false
}

func bashRuleNestedInvocations(name string, args []*syntax.Word) []bashRuleInvocation {
	var commands []string
	switch name {
	case "eval":
		payload := evalPayloadWords(args)
		for index := range payload {
			if command, ok := shellRuleEvalCommand(payload[index:]); ok {
				commands = append(commands, command)
			}
		}
	case "bash", "sh", "zsh", "dash", "ash", "ksh":
		for index, word := range args {
			flag := wordToString(word)
			if (flag == "--command" || strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && strings.ContainsRune(strings.TrimPrefix(flag, "-"), 'c')) && index+1 < len(args) {
				if command := wordToString(args[index+1]); command != "" {
					commands = append(commands, command)
				}
				break
			}
		}
	case "busybox", "toybox":
		if command, ok := shellRuleLiteralCommand(args); ok {
			commands = append(commands, command)
		}
	case "xargs":
		for index := range args {
			if command, ok := shellRuleLiteralCommand(args[index:]); ok {
				commands = append(commands, command)
			}
		}
	case "find":
		commands = append(commands, shellRuleFindCommands(args)...)
	}
	var nested []bashRuleInvocation
	for _, command := range commands {
		nested = append(nested, bashRuleInvocations(command)...)
	}
	return nested
}

func shellRuleEvalCommand(words []*syntax.Word) (string, bool) {
	values := shellRuleWordLiterals(words)
	for index, value := range values {
		if value == "" && len(words[index].Parts) > 0 {
			return "", false
		}
	}
	command := strings.Join(values, " ")
	return command, strings.TrimSpace(command) != ""
}

func shellRuleLiteralCommand(words []*syntax.Word) (string, bool) {
	var command strings.Builder
	for index, word := range words {
		value := wordToString(word)
		if value == "" && len(word.Parts) > 0 {
			return "", false
		}
		if index > 0 {
			command.WriteByte(' ')
		}
		command.WriteString(shellPolicyQuoteLiteral(value))
	}
	return command.String(), command.Len() > 0
}

func shellRuleFindCommands(words []*syntax.Word) []string {
	var commands []string
	for index := 0; index < len(words); index++ {
		operator := wordToString(words[index])
		if operator != "-exec" && operator != "-execdir" && operator != "-ok" && operator != "-okdir" {
			continue
		}
		start, end := index+1, index+1
		for end < len(words) && wordToString(words[end]) != ";" && wordToString(words[end]) != "+" {
			end++
		}
		if command, ok := shellRuleLiteralCommand(words[start:end]); ok {
			commands = append(commands, command)
		}
		index = end
	}
	return commands
}

// ruleAppliesToBash reports whether `tool` targets the Bash tool.
func ruleAppliesToBash(tool string) bool {
	switch strings.TrimSpace(tool) {
	case "Bash", "bash", "*":
		return true
	}
	return false
}
