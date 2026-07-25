package shell

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// CommandResultInterpretation is a model-friendly explanation of a non-zero
// exit code that takes the command name into account. POSIX exit codes are
// generic ("exit 1: general error"), but many UNIX tools use exit 1 to
// communicate a benign result (grep "no matches", diff "files differ",
// find "some inaccessible dirs", test "condition false"). Surfacing those
// per-command meanings keeps the model from retrying pointlessly or treating
// a successful empty search as a failure.
type CommandResultInterpretation struct {
	// Severity is "ok", "warn" or "error".
	//   ok     — the command completed and the exit code is informational.
	//   warn   — partial result (e.g. find: some directories unreadable).
	//   error  — the command actually failed.
	Severity string
	// Message is a human-readable line for ToolResult.Metadata.
	Message string
	// Treat as success when the model should NOT retry / not surface as error.
	TreatAsSuccess bool
}

// interpretCommandResult returns a CommandResultInterpretation for a known
// (commandName, exitCode) pair. When the command name is not recognised, the
// caller should fall back to the generic interpretReturnCode.
func interpretCommandResult(commandName string, exitCode int) (CommandResultInterpretation, bool) {
	if exitCode == 0 {
		return CommandResultInterpretation{
			Severity:       "ok",
			Message:        toolRuntimeText(i18n.KeyToolRuntimeReturnSuccess),
			TreatAsSuccess: true,
		}, true
	}
	name := commandBaseName(commandName)
	switch name {
	case "grep", "egrep", "fgrep", "rg", "ack", "ag":
		switch exitCode {
		case 1:
			return CommandResultInterpretation{
				Severity:       "ok",
				Message:        toolRuntimeFormat(i18n.KeyToolRuntimeBashNoMatches, name),
				TreatAsSuccess: true,
			}, true
		case 2:
			return CommandResultInterpretation{
				Severity: "error",
				Message:  toolRuntimeFormat(i18n.KeyToolRuntimeBashInvalidPattern, name),
			}, true
		}
	case "find":
		if exitCode == 1 {
			return CommandResultInterpretation{
				Severity:       "warn",
				Message:        toolRuntimeText(i18n.KeyToolRuntimeBashFindPartial),
				TreatAsSuccess: true,
			}, true
		}
	case "diff", "cmp":
		switch exitCode {
		case 1:
			return CommandResultInterpretation{
				Severity:       "ok",
				Message:        toolRuntimeFormat(i18n.KeyToolRuntimeBashFilesDiffer, name),
				TreatAsSuccess: true,
			}, true
		case 2:
			return CommandResultInterpretation{
				Severity: "error",
				Message:  toolRuntimeFormat(i18n.KeyToolRuntimeBashDiffTrouble, name),
			}, true
		}
	case "test", "[", "[[":
		if exitCode == 1 {
			return CommandResultInterpretation{
				Severity:       "ok",
				Message:        toolRuntimeFormat(i18n.KeyToolRuntimeBashConditionFalse, name),
				TreatAsSuccess: true,
			}, true
		}
	case "git":
		if exitCode == 1 {
			return CommandResultInterpretation{
				Severity:       "warn",
				Message:        toolRuntimeText(i18n.KeyToolRuntimeBashGitNonFatal),
				TreatAsSuccess: false,
			}, true
		}
	case "make":
		if exitCode == 2 {
			return CommandResultInterpretation{
				Severity: "error",
				Message:  toolRuntimeText(i18n.KeyToolRuntimeBashMakeFailed),
			}, true
		}
	case "tput":
		if exitCode == 1 {
			return CommandResultInterpretation{
				Severity:       "ok",
				Message:        toolRuntimeText(i18n.KeyToolRuntimeBashTputUnsupported),
				TreatAsSuccess: true,
			}, true
		}
	case "wc":
		// wc returns 0 even on empty input; non-zero is a real error.
	case "jq", "yq":
		if exitCode == 1 {
			return CommandResultInterpretation{
				Severity:       "ok",
				Message:        toolRuntimeFormat(i18n.KeyToolRuntimeBashNoFilterMatches, name),
				TreatAsSuccess: true,
			}, true
		}
	}
	return CommandResultInterpretation{}, false
}

// commandBaseName returns the leading command name from a command line.
// Strips path components and stops at the first whitespace so "rg --hidden -n"
// returns "rg" and "/usr/bin/grep" returns "grep".
func commandBaseName(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	// Take up to the first whitespace.
	if idx := strings.IndexAny(cmd, " \t\n"); idx >= 0 {
		cmd = cmd[:idx]
	}
	if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
		cmd = cmd[idx+1:]
	}
	return cmd
}

// interpretReturnCodeWithCommand augments interpretReturnCode by consulting
// per-command exit-code semantics when the generic POSIX explanation would be
// misleading. The command argument is the full command line as executed.
func interpretReturnCodeWithCommand(command string, exitCode int, interrupted bool) string {
	if interrupted {
		return interpretReturnCode(exitCode, true)
	}
	first := firstSegmentCommand(command)
	if interp, ok := interpretCommandResult(first, exitCode); ok {
		return interp.Message
	}
	return interpretReturnCode(exitCode, false)
}

// firstSegmentCommand walks the command up to the first separator and returns
// the first invoked command. We stop at `|`, `&&`, `||`, `;` so "grep foo |
// head" reports "grep" and not "head" — exit codes from pipefail propagate
// from the rightmost segment but the model usually anchors on the leftmost
// command's expectations.
func firstSegmentCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	// Strip leading env-var assignments like `FOO=bar grep ...`.
	for {
		idx := strings.IndexFunc(cmd, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '='
		})
		if idx <= 0 || cmd[idx] != '=' {
			break
		}
		// Skip "VAR=value " — find next space.
		spaceIdx := strings.IndexAny(cmd, " \t")
		if spaceIdx < 0 {
			return ""
		}
		cmd = strings.TrimLeft(cmd[spaceIdx:], " \t")
	}
	// Stop at the first top-level pipe/operator.
	for _, sep := range []string{"&&", "||", "|", ";"} {
		if idx := strings.Index(cmd, sep); idx >= 0 {
			cmd = cmd[:idx]
		}
	}
	return commandBaseName(cmd)
}
