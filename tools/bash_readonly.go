package tools

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// IsReadOnlyCommand reports whether `cmd` performs only read operations and
// can therefore bypass permission gates. The semantic classification is
// computed once and reused; callers may pass a pre-computed value to avoid
// re-parsing.
func IsReadOnlyCommand(cmd string, semantics CommandSemantic) bool {
	if strings.TrimSpace(cmd) == "" {
		return false
	}
	if containsDynamicReadOnlyExpansion(cmd) {
		return false
	}
	if semantics == SemanticUnknown {
		semantics = ClassifyCommand(cmd)
	}
	if semantics != SemanticRead {
		return false
	}
	// Walk the AST and confirm there are no write redirects, no command
	// substitutions to non-readable commands, and no process substitutions
	// that could execute side-effecting code.
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return false
	}
	safe := true
	syntax.Walk(prog, func(node syntax.Node) bool {
		if !safe {
			return false
		}
		switch n := node.(type) {
		case *syntax.Stmt:
			for _, redir := range n.Redirs {
				switch redir.Op {
				case syntax.RdrOut, syntax.AppOut, syntax.RdrAll:
					target := wordToString(redir.Word)
					if !isReadOnlyRedirTarget(target) {
						safe = false
						return false
					}
				}
			}
		case *syntax.ProcSubst:
			// Process substitution can execute arbitrary commands; only
			// honour <(...) when the inner command is itself read-only.
			for _, stmt := range n.Stmts {
				if !stmtIsReadOnly(stmt) {
					safe = false
					return false
				}
			}
		}
		return true
	})
	return safe
}

// containsDynamicReadOnlyExpansion rejects shell words whose runtime value can
// become a flag, command, or path that was not visible to static validation.
// Expansions inside single quotes and globs inside either quote form are
// literal and remain eligible for the read-only path.
func containsDynamicReadOnlyExpansion(command string) bool {
	var inSingle, inDouble, escaped bool
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle {
			continue
		}
		if ch == '$' || ch == '`' {
			return true
		}
		if !inDouble && strings.ContainsRune("?*[]", rune(ch)) {
			return true
		}
	}
	return false
}

// isReadOnlyRedirTarget treats /dev/null and stdio devices as effectively
// read-only — output is discarded, so the command is still side-effect free.
func isReadOnlyRedirTarget(target string) bool {
	switch target {
	case "/dev/null", "/dev/stdout", "/dev/stderr":
		return true
	}
	return false
}

// stmtIsReadOnly recursively checks whether a single statement is read-only.
func stmtIsReadOnly(stmt *syntax.Stmt) bool {
	if stmt == nil || stmt.Cmd == nil {
		return true
	}
	if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
		name := cmdName(call)
		if name == "" {
			return false
		}
		args := argLiterals(call)
		sem := classifyExternal(name, args)
		if sem == SemanticUnknown {
			sem = classifyBuiltin(name)
		}
		return sem == SemanticRead
	}
	return false
}

// ShouldUseSandbox reports whether a sandboxed invocation is required for the
// given command. Read-only commands skip sandboxing for performance; network
// or write/destructive commands always sandbox. Callers can override via the
// existing dangerouslyDisableSandbox flag.
func ShouldUseSandbox(cmd string, semantics CommandSemantic) bool {
	if strings.TrimSpace(cmd) == "" {
		return true
	}
	if semantics == SemanticUnknown {
		semantics = ClassifyCommand(cmd)
	}
	if !IsReadOnlyCommand(cmd, semantics) {
		return true
	}
	// Read-only commands that pull from network must still sandbox.
	if semantics == SemanticNetwork {
		return true
	}
	return false
}
