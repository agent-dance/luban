package shell

import (
	"regexp"

	"github.com/agent-dance/luban/i18n"
)

// dangerousPattern pairs a compiled regex with a human-readable description.
type dangerousPattern struct {
	pattern *regexp.Regexp
	key     i18n.Key
}

// dangerousPatterns is the regex fast-path for DetectDangerousCommand.
// These catch the most obvious dangerous patterns without needing an AST parse.
// The AST deep-check (astDeepCheck) handles structural patterns that regexes miss.
var dangerousPatterns = []dangerousPattern{
	// Recursive force delete of root or system paths
	{
		pattern: regexp.MustCompile(`\brm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+-[a-zA-Z]*r|-[a-zA-Z]*r[a-zA-Z]*\s+-[a-zA-Z]*f|-[a-zA-Z]*rf[a-zA-Z]*|-[a-zA-Z]*fr[a-zA-Z]*)\b.*\s+/(\s|$|;|\|)`),
		key:     i18n.KeyToolRuntimeDangerousRootDelete,
	},
	// Filesystem format commands
	{
		pattern: regexp.MustCompile(`\bmkfs(\.\w+)?\s`),
		key:     i18n.KeyToolRuntimeDangerousFilesystemFormat,
	},
	// Direct disk write via dd
	{
		pattern: regexp.MustCompile(`\bdd\b.*\bof=/dev/[a-z]`),
		key:     i18n.KeyToolRuntimeDangerousDirectDiskWrite,
	},
	// Fork bomb patterns
	{
		pattern: regexp.MustCompile(`:\(\)\s*\{\s*:\|:\s*&\s*\}\s*;?\s*:`),
		key:     i18n.KeyToolRuntimeDangerousForkBomb,
	},
	{
		pattern: regexp.MustCompile(`\.\(\)\s*\{\s*\.\|\.\s*&\s*\}\s*;?\s*\.`),
		key:     i18n.KeyToolRuntimeDangerousForkBomb,
	},
	// curl/wget piped to shell (fast-path; AST also catches this)
	{
		pattern: regexp.MustCompile(`\b(curl|wget)\b.*\|\s*(bash|sh|zsh|sudo)\b`),
		key:     i18n.KeyToolRuntimeDangerousPipeToShell,
	},
	// base64 decode piped to shell
	{
		pattern: regexp.MustCompile(`\bbase64\s+-d\b.*\|\s*(bash|sh|zsh|sudo)\b`),
		key:     i18n.KeyToolRuntimeDangerousBase64ToShell,
	},
	// Reverse shell patterns
	{
		pattern: regexp.MustCompile(`\b(bash|sh|nc|ncat)\b.*(/dev/tcp/|/dev/udp/)`),
		key:     i18n.KeyToolRuntimeDangerousReverseShell,
	},
	// Python/Perl/Ruby destructive one-liners
	{
		pattern: regexp.MustCompile(`\b(python[23]?|perl|ruby)\s+-[a-zA-Z]*[ec][a-zA-Z]*\s+.*\b(rmtree|unlink|rm_rf|os\.system|shutil\.rmtree)\b`),
		key:     i18n.KeyToolRuntimeDangerousScriptingOneLiner,
	},
	// chmod 777 on root
	{
		pattern: regexp.MustCompile(`\bchmod\s+(-[a-zA-Z]*R[a-zA-Z]*\s+)?777\s+/(\s|$|;)`),
		key:     i18n.KeyToolRuntimeDangerousChmodRoot,
	},
}
