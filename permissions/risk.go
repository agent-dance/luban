package permissions

import (
	"os"
	"path/filepath"
	"strings"
)

// RiskLevel indicates how dangerous a tool call is.
type RiskLevel int

const (
	RiskLow    RiskLevel = iota // read-only operations
	RiskMedium                  // writes within CWD or non-destructive ops
	RiskHigh                    // destructive, external, or out-of-CWD writes
)

// String returns a human-readable risk level name.
func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "Low"
	case RiskMedium:
		return "Medium"
	case RiskHigh:
		return "High"
	}
	return "Unknown"
}

// lowRiskTools are read-only tools that never modify state.
var lowRiskTools = map[string]bool{
	"Read": true, "Glob": true, "Grep": true,
	// LSP variants
	"LSP":                       true,
	"lsp_diagnostics":           true,
	"lsp_diagnostics_directory": true,
	"lsp_find_references":       true,
	"lsp_goto_definition":       true,
	"lsp_hover":                 true,
	"lsp_document_symbols":      true,
	"lsp_workspace_symbols":     true,
	"lsp_servers":               true,
}

// mediumRiskTools make network requests but don't write local state.
var mediumRiskTools = map[string]bool{
	"HttpGet":  true,
	"HttpPost": true,
	"Ping":     true,
}

// highRiskBashPatterns are substrings that escalate a Bash command to High.
var highRiskBashPatterns = []string{
	"rm -rf",
	"sudo ",
	"chmod 777",
	"> /dev/",
	">/dev/",
	"| sh",
	"|sh",
	"| bash",
	"|bash",
	"| zsh",
	"|zsh",
	"mkfs",
	"dd if=",
	":(){",  // fork bomb
	"curl ", // network fetch
	"curl\t",
	"wget ", // network fetch
	"wget\t",
	"nc ", // netcat
	"ncat ",
	"netcat ",
	"ssh ",   // remote shell
	"scp ",   // remote copy
	"rsync ", // potentially remote
	"eval ",  // code injection risk
	"exec ",  // exec replacement
}

// lowRiskBashCommands are shell commands whose first token is always Low risk.
var lowRiskBashCommands = map[string]bool{
	"ls": true, "cat": true, "echo": true, "pwd": true,
	"which": true, "whoami": true, "uname": true,
	"head": true, "tail": true, "wc": true, "grep": true,
	"sort": true, "uniq": true, "diff": true,
	"stat": true, "file": true, "du": true, "df": true,
	"ps": true, "id": true, "hostname": true, "uptime": true,
	"type": true, "readlink": true, "realpath": true,
	"basename": true, "dirname": true, "cut": true, "paste": true,
	"tr": true, "column": true, "tac": true, "rev": true,
	"fold": true, "expand": true, "unexpand": true, "fmt": true,
	"comm": true, "cmp": true, "numfmt": true, "groups": true,
	"nproc": true, "true": true, "false": true, "getconf": true,
	"seq": true, "tsort": true, "pr": true,
}

// gitReadSubcommands are git sub-commands that are purely read-only.
var gitReadSubcommands = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true,
	"branch": true, "remote": true, "stash": true, "tag": true,
	"describe": true, "shortlog": true, "blame": true,
	"ls-files": true, "ls-tree": true, "rev-parse": true,
	"reflog": true, "config": true, "help": true, "version": true,
}

// mediumRiskBashCommands are shell commands that write but stay within CWD.
var mediumRiskBashCommands = map[string]bool{
	"mkdir": true, "cp": true, "mv": true, "touch": true,
	"ln": true, "tar": true, "zip": true, "unzip": true,
	"chmod": true, "chown": true,
	"npm": true, "yarn": true, "pnpm": true,
	"go": true, "python": true, "python3": true, "pip": true, "pip3": true,
	"make": true, "cmake": true, "cargo": true, "rustc": true,
	"node": true, "deno": true, "bun": true,
	"git": true, // git write ops (push, commit, etc.) are medium by default
	"tee": true,
}

var highRiskPowerShellPatterns = []string{
	"remove-item",
	"set-content",
	"add-content",
	"clear-content",
	"new-item",
	"copy-item",
	"move-item",
	"rename-item",
	"invoke-expression",
	"start-process",
	"stop-process",
	"set-executionpolicy",
	"out-file",
	"invoke-webrequest",
	"invoke-restmethod",
	"rm ",
	"del ",
	"erase ",
	"rmdir ",
	"iex ",
	"iwr ",
	"irm ",
	"curl ",
	"wget ",
}

var lowRiskPowerShellCommands = map[string]bool{
	"get-childitem": true, "get-content": true, "get-location": true,
	"get-item": true, "get-process": true, "get-service": true,
	"resolve-path": true, "test-path": true, "select-string": true,
	"select-object": true, "where-object": true, "measure-object": true,
	"sort-object": true, "format-table": true, "format-list": true,
	"out-string": true, "write-output": true, "write-host": true,
	"git": true,
}

var mediumRiskPowerShellCommands = map[string]bool{
	"mkdir": true, "go": true, "npm": true, "yarn": true,
	"pnpm": true, "node": true, "python": true, "python3": true,
	"pip": true, "pip3": true, "git": true,
}

// ClassifyRisk returns the risk level for a tool call.
func ClassifyRisk(toolName string, input map[string]any) RiskLevel {
	if lowRiskTools[toolName] {
		return RiskLow
	}
	if mediumRiskTools[toolName] {
		return RiskMedium
	}
	switch toolName {
	case "Write", "Edit":
		return classifyFilePath(input)
	case "Bash":
		cmd, _ := input["command"].(string)
		return classifyBashCommand(cmd)
	case "PowerShell":
		cmd, _ := input["command"].(string)
		return classifyPowerShellCommand(cmd)
	case "SendMessage":
		if _, plain := input["message"].(string); plain {
			return RiskLow
		}
		return RiskMedium
	}
	// Unknown tools default to Medium (conservative but not blocking).
	return RiskMedium
}

// classifyFilePath checks if the file_path is inside CWD (Medium) or outside (High).
func classifyFilePath(input map[string]any) RiskLevel {
	fp, ok := input["file_path"].(string)
	if !ok || fp == "" {
		return RiskMedium
	}
	cwd, err := os.Getwd()
	if err != nil {
		return RiskMedium
	}
	abs, err := filepath.Abs(fp)
	if err != nil {
		return RiskMedium
	}
	// abs must be cwd or a descendant of cwd
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return RiskHigh
	}
	if strings.HasPrefix(rel, "..") {
		return RiskHigh
	}
	return RiskMedium
}

// containsShellChaining returns true if cmd contains shell metacharacters that
// can chain or redirect commands outside of what is described by the first
// token alone (e.g. ";", "&&", "||", pipes, command substitution).
// Single-quoted and double-quoted regions are excluded because:
// - Single quotes disable all metacharacter interpretation in POSIX shells.
// - Double quotes disable ;, &&, ||, and pipes (only $, `, \, and ! are special).
// W3 fix: added double-quote region skipping to prevent false positives
// like `bash -c "ls; echo hi"` being flagged as shell chaining.
func containsShellChaining(cmd string) bool {
	dangerousPatterns := []string{";", "&&", "||", "$(", "`", "| "}
	inSingleQuote := false
	inDoubleQuote := false
	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]

		// Handle escape sequences in double-quoted regions
		if inDoubleQuote && ch == '\\' && i+1 < len(cmd) {
			i++ // skip escaped character
			continue
		}

		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if inSingleQuote || inDoubleQuote {
			continue
		}
		for _, p := range dangerousPatterns {
			if strings.HasPrefix(cmd[i:], p) {
				return true
			}
		}
	}
	return false
}

// classifyBashCommand analyses a shell command string for risk.
func classifyBashCommand(cmd string) RiskLevel {
	if cmd == "" {
		return RiskMedium
	}
	lower := strings.ToLower(cmd)

	// Check for shell metacharacters that can chain commands — this must be
	// evaluated before the per-token classification so that a command like
	// "ls; curl evil.com" is caught even though "ls" is low risk.
	if containsShellChaining(cmd) {
		return RiskHigh
	}

	// Check high-risk substrings first — order matters.
	for _, pat := range highRiskBashPatterns {
		if strings.Contains(lower, pat) {
			return RiskHigh
		}
	}

	// Redirect to an absolute path outside common safe directories.
	if containsAbsoluteRedirect(cmd) {
		return RiskHigh
	}
	if IsReadOnlyBashCommand(cmd) {
		return RiskLow
	}

	// Determine first token (strip any path prefix like /usr/bin/ls → ls).
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return RiskMedium
	}
	first := filepath.Base(parts[0])

	// git: read-only sub-commands are Low; everything else is Medium.
	if first == "git" {
		if len(parts) > 1 && gitReadSubcommands[parts[1]] {
			return RiskLow
		}
		return RiskMedium
	}

	if lowRiskBashCommands[first] {
		return RiskLow
	}
	if mediumRiskBashCommands[first] {
		return RiskMedium
	}
	// rm without -rf: medium; rm -rf already caught by pattern above.
	if first == "rm" {
		return RiskMedium
	}

	// Anything else unrecognised → High (fail safe).
	return RiskHigh
}

func classifyPowerShellCommand(cmd string) RiskLevel {
	if strings.TrimSpace(cmd) == "" {
		return RiskMedium
	}
	lower := strings.ToLower(cmd)
	for _, pat := range highRiskPowerShellPatterns {
		if strings.Contains(lower, pat) {
			return RiskHigh
		}
	}
	if containsPowerShellRedirect(cmd) {
		return RiskHigh
	}
	if IsReadOnlyPowerShellCommand(cmd) {
		return RiskLow
	}

	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return RiskMedium
	}
	first := normalizePowerShellCommand(parts[0])
	if first == "git" {
		if len(parts) > 1 && gitReadSubcommands[parts[1]] {
			return RiskLow
		}
		return RiskMedium
	}
	if lowRiskPowerShellCommands[first] {
		return RiskLow
	}
	if mediumRiskPowerShellCommands[first] {
		return RiskMedium
	}
	return RiskHigh
}

func IsReadOnlyPowerShellCommand(cmd string) bool {
	segments, ok := splitPowerShellReadOnlySegments(cmd)
	if !ok || len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		argv := strings.Fields(strings.TrimSpace(segment))
		if len(argv) == 0 {
			return false
		}
		first := normalizePowerShellCommand(argv[0])
		switch first {
		case "git":
			subcommand := gitReadOnlySubcommand(argv[1:])
			if subcommand == "" || !gitReadSubcommands[subcommand] {
				return false
			}
		default:
			if !lowRiskPowerShellCommands[first] {
				return false
			}
		}
	}
	return true
}

func normalizePowerShellCommand(token string) string {
	token = strings.ToLower(strings.Trim(token, `'"`))
	token = strings.TrimSuffix(token, ".exe")
	switch token {
	case "ls", "dir", "gci":
		return "get-childitem"
	case "cat", "type", "gc":
		return "get-content"
	case "pwd", "gl":
		return "get-location"
	case "sls":
		return "select-string"
	case "select":
		return "select-object"
	case "where", "?":
		return "where-object"
	case "measure":
		return "measure-object"
	case "sort":
		return "sort-object"
	case "ft":
		return "format-table"
	case "fl":
		return "format-list"
	default:
		return token
	}
}

func splitPowerShellReadOnlySegments(command string) ([]string, bool) {
	var segments []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '`' {
			current.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
			continue
		}
		if !inSingleQuote && !inDoubleQuote {
			if ch == '>' || ch == '<' || ch == '&' {
				return nil, false
			}
			if ch == '|' || ch == ';' {
				segment := strings.TrimSpace(current.String())
				if segment != "" {
					segments = append(segments, segment)
				}
				current.Reset()
				continue
			}
		}
		current.WriteByte(ch)
	}
	if inSingleQuote || inDoubleQuote {
		return nil, false
	}
	if segment := strings.TrimSpace(current.String()); segment != "" {
		segments = append(segments, segment)
	}
	return segments, true
}

func containsPowerShellRedirect(command string) bool {
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '`' {
			escaped = true
			continue
		}
		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if !inSingleQuote && !inDoubleQuote && (ch == '>' || ch == '<') {
			return true
		}
	}
	return false
}

// IsReadOnlyBashCommand returns true for commands that are treated as
// read-only for permission auto-allow purposes.
func IsReadOnlyBashCommand(cmd string) bool {
	if containsUnquotedExpansion(cmd) {
		return false
	}
	argv := bashReadOnlyArgv(cmd)
	if len(argv) == 0 {
		return false
	}

	first := filepath.Base(argv[0])
	switch first {
	case "git":
		subcommand := gitReadOnlySubcommand(argv[1:])
		return subcommand != "" && gitReadSubcommands[subcommand]
	case "sed":
		return sedReadOnlyArgs(argv[1:])
	case "jq":
		return jqReadOnlyArgs(argv[1:])
	case "rg", "ripgrep":
		return rgReadOnlyArgs(argv[1:])
	case "tree":
		return treeReadOnlyArgs(argv[1:])
	case "history":
		return historyReadOnlyArgs(argv[1:])
	case "alias":
		return len(argv) == 1
	case "find":
		return findReadOnlyArgs(argv[1:])
	}

	return lowRiskBashCommands[first]
}

func containsUnquotedExpansion(command string) bool {
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && !inSingleQuote {
			escaped = true
			continue
		}
		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if inSingleQuote {
			continue
		}
		if ch == '$' && i+1 < len(command) {
			next := command[i+1]
			if (next >= 'A' && next <= 'Z') ||
				(next >= 'a' && next <= 'z') ||
				next == '_' || next == '@' || next == '*' || next == '#' ||
				next == '?' || next == '!' || next == '$' || next == '-' ||
				(next >= '0' && next <= '9') {
				return true
			}
		}
		if inDoubleQuote {
			continue
		}
		switch ch {
		case '?', '*', '[', ']':
			return true
		}
	}
	return false
}

func bashReadOnlyArgv(cmd string) []string {
	if cmd == "" || containsShellChaining(cmd) {
		return nil
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil
	}

	for len(parts) > 0 && looksLikeEnvAssignment(parts[0]) {
		parts = parts[1:]
	}
	for len(parts) > 0 {
		switch parts[0] {
		case "env":
			parts = parts[1:]
			for len(parts) > 0 && (looksLikeEnvAssignment(parts[0]) || parts[0] == "-i") {
				parts = parts[1:]
			}
		case "command", "builtin":
			parts = parts[1:]
		default:
			return parts
		}
	}
	return nil
}

func looksLikeEnvAssignment(token string) bool {
	if token == "" || strings.HasPrefix(token, "=") {
		return false
	}
	return strings.Contains(token, "=") && !strings.HasPrefix(token, "-")
}

func gitReadOnlySubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}
		if arg == "--" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
		switch arg {
		case "-C", "-c", "--exec-path", "--git-dir", "--work-tree", "--namespace", "--config-env":
			i++
		default:
			if strings.Contains(arg, "=") {
				continue
			}
		}
	}
	return ""
}

func sedReadOnlyArgs(args []string) bool {
	for _, arg := range args {
		if arg == "-i" || arg == "--in-place" || strings.HasPrefix(arg, "--in-place=") {
			return false
		}
		if strings.HasPrefix(arg, "-i") && arg != "-i" {
			return false
		}
	}
	return true
}

func jqReadOnlyArgs(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-f", "--from-file", "--rawfile", "--slurpfile", "--run-tests", "-L", "--library-path":
			return false
		}
		if strings.HasPrefix(arg, "--from-file=") ||
			strings.HasPrefix(arg, "--rawfile=") ||
			strings.HasPrefix(arg, "--slurpfile=") ||
			strings.HasPrefix(arg, "--library-path=") {
			return false
		}
	}
	return true
}

func rgReadOnlyArgs(args []string) bool {
	for _, arg := range args {
		if arg == "--pre" || arg == "--pre-glob" || strings.HasPrefix(arg, "--pre=") || strings.HasPrefix(arg, "--pre-glob=") {
			return false
		}
	}
	return true
}

func treeReadOnlyArgs(args []string) bool {
	for _, arg := range args {
		if arg == "-o" || arg == "--output" || strings.HasPrefix(arg, "--output=") {
			return false
		}
	}
	return true
}

func historyReadOnlyArgs(args []string) bool {
	if len(args) == 0 {
		return true
	}
	if len(args) != 1 {
		return false
	}
	for _, ch := range args[0] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func findReadOnlyArgs(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fls", "-fprintf":
			return false
		}
	}
	return true
}

// containsAbsoluteRedirect returns true when the command redirects stdout to
// an absolute path that looks system-level (not a project-relative write).
// Single and double quotes around the path are stripped before the check so
// that "> '/etc/passwd'" is not bypassed.
func containsAbsoluteRedirect(cmd string) bool {
	for i, ch := range cmd {
		if ch != '>' {
			continue
		}
		rest := strings.TrimLeft(cmd[i+1:], " \t>")
		// Strip surrounding single or double quotes from the path token.
		if len(rest) >= 2 && (rest[0] == '\'' || rest[0] == '"') {
			quote := rest[0]
			if end := strings.IndexByte(rest[1:], quote); end >= 0 {
				rest = rest[1 : end+1]
			}
		}
		if strings.HasPrefix(rest, "/") {
			// Allow redirects to /tmp as medium
			if strings.HasPrefix(rest, "/tmp/") {
				continue
			}
			return true
		}
	}
	return false
}
