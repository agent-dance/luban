package shell

import (
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"mvdan.cc/sh/v3/syntax"
)

// destructiveWarningRule pairs a regex against a human-readable warning.
type destructiveWarningRule struct {
	pattern *regexp.Regexp
	key     i18n.Key
}

var destructiveWarningRules = []destructiveWarningRule{
	{regexp.MustCompile(`\brm\s+(?:-[a-zA-Z]*[rR][a-zA-Z]*\s+-[a-zA-Z]*[fF][a-zA-Z]*|-[a-zA-Z]*[fF][a-zA-Z]*\s+-[a-zA-Z]*[rR][a-zA-Z]*|-[a-zA-Z]*[rRfF]{2}[a-zA-Z]*)\b`), i18n.KeyToolRuntimeDestructiveRmRecursiveForce},
	{regexp.MustCompile(`\brm\s+(?:-[a-zA-Z]*[rR][a-zA-Z]*\s+).*\s+(/|\$HOME|~)\s*$`), i18n.KeyToolRuntimeDestructiveRmTopLevel},
	// rm long-flags: --recursive --force (any order, optional intermixed args).
	{regexp.MustCompile(`\brm\b(?:[^\n;&|]*\s)?--recursive\b(?:[^\n;&|]*\s)?--force\b`), i18n.KeyToolRuntimeDestructiveRmRecursiveForce},
	{regexp.MustCompile(`\brm\b(?:[^\n;&|]*\s)?--force\b(?:[^\n;&|]*\s)?--recursive\b`), i18n.KeyToolRuntimeDestructiveRmRecursiveForce},
	{regexp.MustCompile(`\bdd\s+`), i18n.KeyToolRuntimeDestructiveDd},
	{regexp.MustCompile(`\bmkfs(\.\w+)?\s`), i18n.KeyToolRuntimeDestructiveMkfs},
	{regexp.MustCompile(`\bshred\s`), i18n.KeyToolRuntimeDestructiveShred},
	{regexp.MustCompile(`\bwipe\s`), i18n.KeyToolRuntimeDestructiveWipe},
	{regexp.MustCompile(`>\s*/dev/sd[a-z]`), i18n.KeyToolRuntimeDestructiveRawDeviceRedirect},
	{regexp.MustCompile(`\bfind\b.*-(?:exec|execdir|ok|okdir)\s+rm\b`), i18n.KeyToolRuntimeDestructiveFindExecRm},
	{regexp.MustCompile(`\bfind\b.*-delete\b`), i18n.KeyToolRuntimeDestructiveFindDelete},
	// Git destructive operations (TS destructiveCommandWarning.ts).
	{regexp.MustCompile(`\bgit\s+push\b[^\n;&|]*\s--force\b`), i18n.KeyToolRuntimeDestructiveGitPushForce},
	{regexp.MustCompile(`\bgit\s+push\b[^\n;&|]*\s--force-with-lease\b`), i18n.KeyToolRuntimeDestructiveGitPushForceLease},
	{regexp.MustCompile(`\bgit\s+push\b[^\n;&|]*\s-f\b`), i18n.KeyToolRuntimeDestructiveGitPushForce},
	{regexp.MustCompile(`\bgit\s+reset\s+--hard\b`), i18n.KeyToolRuntimeDestructiveGitResetHard},
	{regexp.MustCompile(`\bgit\s+clean\s+(?:-[a-zA-Z]*f[a-zA-Z]*\b|--force\b)`), i18n.KeyToolRuntimeDestructiveGitCleanForce},
	{regexp.MustCompile(`\bgit\s+branch\s+(?:-D|--delete\s+--force)\b`), i18n.KeyToolRuntimeDestructiveGitBranchForceDelete},
	// SQL destructive operations.
	{regexp.MustCompile(`(?i)\bDROP\s+(?:TABLE|DATABASE|SCHEMA|INDEX|VIEW)\b`), i18n.KeyToolRuntimeDestructiveSQLDrop},
	{regexp.MustCompile(`(?i)\bTRUNCATE\s+TABLE\b`), i18n.KeyToolRuntimeDestructiveSQLTruncate},
	{regexp.MustCompile(`(?i)\bDELETE\s+FROM\b(?:[^;\n]*?)(?:;|$)`), i18n.KeyToolRuntimeDestructiveSQLDelete},
	// Kubernetes destructive operations.
	{regexp.MustCompile(`\bkubectl\s+delete\s+namespace\b`), i18n.KeyToolRuntimeDestructiveKubectlNamespace},
	{regexp.MustCompile(`\bkubectl\s+delete\b[^\n;&|]*\s--all\b`), i18n.KeyToolRuntimeDestructiveKubectlAll},
	{regexp.MustCompile(`\bkubectl\s+delete\s+(?:pv|persistentvolume|pvc|persistentvolumeclaim)\b`), i18n.KeyToolRuntimeDestructiveKubectlPersistentVolume},
	// Terraform destructive operations.
	{regexp.MustCompile(`\bterraform\s+destroy\b`), i18n.KeyToolRuntimeDestructiveTerraformDestroy},
	{regexp.MustCompile(`\bterraform\s+apply\b[^\n;&|]*\s-destroy\b`), i18n.KeyToolRuntimeDestructiveTerraformDestroy},
	// Helm destructive operations.
	{regexp.MustCompile(`\bhelm\s+(?:uninstall|delete)\b`), i18n.KeyToolRuntimeDestructiveHelmUninstall},
}

// destructiveCommandWarning evaluates `cmd` for destructive-but-not-blocked
// patterns and returns a warning string and a flag indicating whether the
// caller must confirm with the user before proceeding. An empty warning means
// the command does not require an extra confirmation step.
func destructiveCommandWarning(cmd string) (string, bool) {
	if strings.TrimSpace(cmd) == "" {
		return "", false
	}
	for _, rule := range destructiveWarningRules {
		if rule.pattern.MatchString(cmd) {
			return toolRuntimeText(rule.key), true
		}
	}
	// AST pass: catches `rm` invocations that don't match the regex but still
	// pass -r/-f via combined flags or appear inside a pipeline.
	if astWarning := destructiveASTCheck(cmd); astWarning != "" {
		return astWarning, true
	}
	return "", false
}

func destructiveASTCheck(cmd string) string {
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return ""
	}
	var warning string
	syntax.Walk(prog, func(node syntax.Node) bool {
		if warning != "" {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		name := cmdName(call)
		args := argLiterals(call)
		switch name {
		case "rm":
			if hasFlag(args, 'r') || hasFlag(args, 'R') {
				warning = toolRuntimeText(i18n.KeyToolRuntimeDestructiveRmRecursive)
				return false
			}
		case "rmdir":
			if hasFlag(args, 'p') || hasFlag(args, 'r') {
				warning = toolRuntimeText(i18n.KeyToolRuntimeDestructiveRmdirRecursive)
				return false
			}
		case "dd":
			for _, a := range args {
				if strings.HasPrefix(a, "of=/") {
					warning = toolRuntimeFormat(i18n.KeyToolRuntimeDestructiveDdPath, strings.TrimPrefix(a, "of="))
					return false
				}
			}
		}
		return true
	})
	return warning
}
