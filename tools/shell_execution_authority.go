package tools

import (
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
	"mvdan.cc/sh/v3/syntax"
)

// shellPolicySafeCommands is intentionally narrower than the metadata
// classifier. Entries here have no option that executes another program or
// routes output to an arbitrary file. Output redirections remain covered by
// analyzeShellRedirectPolicy.
var shellPolicySafeCommands = map[string]bool{
	"basename": true, "cat": true, "column": true, "comm": true,
	"cmp": true, "cut": true, "df": true, "diff": true, "dirname": true,
	"du": true, "echo": true, "egrep": true, "expand": true,
	"false": true, "fgrep": true, "fmt": true, "fold": true,
	"getconf": true, "grep": true, "groups": true, "head": true,
	"hexdump": true, "host": true, "id": true, "join": true,
	"jq": true, "locale": true, "ls": true, "md5sum": true,
	"netstat": true, "nl": true, "nproc": true, "od": true,
	"paste": true, "pidof": true, "printenv": true,
	"printf": true, "pr": true, "ps": true, "pwd": true,
	"readlink": true, "realpath": true, "rev": true, "sha1sum": true,
	"sha256sum": true, "sha512sum": true, "sleep": true, "stat": true,
	"tac": true, "tail": true, "test": true, "tr": true,
	"true": true, "tty": true, "uname": true, "unexpand": true,
	"uniq": true, "uptime": true, "users": true, "wc": true,
	"which": true, "whoami": true,
}

// These commands have target grammars covered by the unified analyzer. The
// list must not be expanded merely because CommandSemantic says "write": a
// mutator belongs here only after all output-routing options are modeled.
var shellPolicyModeledMutators = map[string]bool{
	"chmod":    true,
	"chown":    true,
	"cp":       true,
	"dd":       true,
	"install":  true,
	"ln":       true,
	"mkdir":    true,
	"mv":       true,
	"rm":       true,
	"tee":      true,
	"touch":    true,
	"truncate": true,
}

var shellPolicyUnrestrictedInterpreters = map[string]bool{
	"ash": true, "bash": true, "bun": true, "dash": true,
	"deno": true, "ksh": true, "lua": true, "node": true,
	"perl": true, "php": true, "python": true, "ruby": true,
	"sh": true, "zsh": true,
}

// analyzeShellExecutionAuthority is the completeness floor for PolicyAllow.
// Pattern checks answer whether a known operation is blocked; this check
// answers the separate question of whether the executable's possible effects
// are completely represented by those patterns. Unknown programs, language
// runtimes, build/package hooks, network tools, and unmodeled routing flags all
// require a fresh invocation-scoped decision.
func analyzeShellExecutionAuthority(name string, args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	executable := filepath.Base(strings.TrimSpace(name))
	if executable == "" {
		return unrestrictedCodePolicyDecision("?")
	}
	if shellPolicyInterpreterName(executable) || executable == "eval" {
		return unrestrictedCodePolicyDecision(executable)
	}
	if readBuiltins[executable] {
		return allowShellPolicyDecision()
	}
	if shellPolicySafeCommands[executable] {
		return allowShellPolicyDecision()
	}

	// These have dedicated recursive/target analyzers. Their nested actions are
	// independently subjected to this same completeness floor.
	switch executable {
	case "find", "xargs":
		return allowShellPolicyDecision()
	case "mktemp":
		return analyzeModeledMktempAuthority(args, known, policy)
	case "git":
		if shellPolicySafeGitInvocation(args, known, policy) {
			return allowShellPolicyDecision()
		}
		return unrestrictedCodePolicyDecision(executable)
	case "go":
		if shellPolicySafeGoInvocation(args, known, policy) {
			return allowShellPolicyDecision()
		}
		return unrestrictedCodePolicyDecision(executable)
	case "xxd":
		return analyzeModeledXxdAuthority(args, known, policy)
	case "date", "hostname", "info", "sort", "tree":
		literals, ok := shellPolicyStaticArguments(args, known, policy)
		if ok && classifyExternal(executable, literals) == SemanticRead {
			return allowShellPolicyDecision()
		}
		return unrestrictedCodePolicyDecision(executable)
	case "file":
		literals, ok := shellPolicyStaticArguments(args, known, policy)
		if ok && !hasAnyShellPolicyFlag(literals, "-C", "--compile") {
			return allowShellPolicyDecision()
		}
		return unrestrictedCodePolicyDecision(executable)
	case "rg":
		literals, ok := shellPolicyStaticArguments(args, known, policy)
		if ok && !hasAnyShellPolicyFlag(literals, "--pre", "--hostname-bin") {
			return allowShellPolicyDecision()
		}
		return unrestrictedCodePolicyDecision(executable)
	case "ss":
		literals, ok := shellPolicyStaticArguments(args, known, policy)
		if ok && !hasAnyShellPolicyFlag(literals, "-K", "--kill") {
			return allowShellPolicyDecision()
		}
		return unrestrictedCodePolicyDecision(executable)
	case "sed":
		if shellPolicySafeSedInvocation(args, known, policy) {
			return allowShellPolicyDecision()
		}
		return unrestrictedCodePolicyDecision(executable)
	}

	if shellPolicyModeledMutators[executable] {
		// Except for rm, whose expansion-aware parser explicitly handles dynamic
		// argv, the target analyzers require every word to resolve exactly.
		if executable == "rm" {
			return allowShellPolicyDecision()
		}
		if _, ok := shellPolicyStaticArguments(args, known, policy); ok {
			return allowShellPolicyDecision()
		}
	}
	return unrestrictedCodePolicyDecision(executable)
}

func shellPolicyInterpreterName(executable string) bool {
	if shellPolicyUnrestrictedInterpreters[executable] {
		return true
	}
	for _, prefix := range []string{"python", "ruby", "perl", "node", "php", "lua"} {
		if strings.HasPrefix(executable, prefix) && len(executable) > len(prefix) {
			suffix := executable[len(prefix):]
			if suffix[0] >= '0' && suffix[0] <= '9' {
				return true
			}
		}
	}
	return false
}

func shellPolicyStaticArguments(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) ([]string, bool) {
	literals := make([]string, 0, len(args))
	for _, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" || shellPolicyWordHasUnquotedExpansion(word) {
			return nil, false
		}
		literals = append(literals, value.literal)
	}
	return literals, true
}

func hasAnyShellPolicyFlag(args []string, flags ...string) bool {
	for _, argument := range args {
		for _, flag := range flags {
			if argument == flag || strings.HasPrefix(argument, flag+"=") {
				return true
			}
		}
	}
	return false
}

func shellPolicySafeGoInvocation(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) bool {
	literals, ok := shellPolicyStaticArguments(args, known, policy)
	if !ok {
		return false
	}
	subcommand, tail, ok := shellPolicyGoSubcommand(literals)
	if !ok {
		return false
	}
	switch subcommand {
	case "version", "doc":
		return true
	case "list":
		return shellPolicySafeGoListFlags(tail)
	case "env":
		return classifyGo(literals) == SemanticRead
	default:
		return false
	}
}

// shellPolicyGoSubcommand recognizes the only process-wide flag accepted by
// the go driver. Treating an arbitrary flag value as the subcommand would let
// an execution-bearing option move the parser out of sync.
func shellPolicyGoSubcommand(args []string) (string, []string, bool) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "-C":
			index++
			if index >= len(args) || strings.TrimSpace(args[index]) == "" {
				return "", nil, false
			}
		case strings.HasPrefix(argument, "-C="):
			if strings.TrimSpace(strings.TrimPrefix(argument, "-C=")) == "" {
				return "", nil, false
			}
		case strings.HasPrefix(argument, "-"):
			return "", nil, false
		default:
			return argument, args[index+1:], true
		}
	}
	return "", nil, false
}

// go list normally performs package metadata discovery, but a subset of its
// build flags enters the compiler/toolchain path, routes mutable module state,
// or substitutes source through an overlay. Only the documented metadata-only
// grammar is eligible for automatic approval.
func shellPolicySafeGoListFlags(args []string) bool {
	valueFlags := map[string]bool{
		"-f": true, "-reuse": true,
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			return true
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			continue
		}
		switch argument {
		case "-deps", "-e", "-find", "-json", "-m", "-retracted":
			continue
		}
		if strings.HasPrefix(argument, "-f=") || strings.HasPrefix(argument, "-reuse=") {
			continue
		}
		if valueFlags[argument] {
			index++
			if index >= len(args) {
				return false
			}
			continue
		}
		// This includes -export/-compiled, -toolexec, -overlay, build
		// instrumentation flags, module/cache mutators, and future flags whose
		// execution/output semantics have not been modeled yet.
		return false
	}
	return true
}

func shellPolicySafeSedInvocation(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) bool {
	literals, ok := shellPolicyStaticArguments(args, known, policy)
	if !ok {
		return false
	}
	scripts := make([]string, 0, 2)
	seenScript := false
	for index := 0; index < len(literals); index++ {
		argument := literals[index]
		switch {
		case argument == "-f" || argument == "--file" || strings.HasPrefix(argument, "-f") && argument != "-f" || strings.HasPrefix(argument, "--file="):
			// The file can contain e/w commands and is not immutable evidence at
			// the policy boundary.
			return false
		case argument == "-e" || argument == "--expression":
			index++
			if index >= len(literals) {
				return false
			}
			scripts = append(scripts, literals[index])
			seenScript = true
		case strings.HasPrefix(argument, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(argument, "--expression="))
			seenScript = true
		case strings.HasPrefix(argument, "-e") && len(argument) > 2:
			scripts = append(scripts, strings.TrimPrefix(argument, "-e"))
			seenScript = true
		case argument == "-i":
			// BSD sed accepts a separate empty/dot-prefixed backup suffix.
			if index+1 < len(literals) && (literals[index+1] == "" || strings.HasPrefix(literals[index+1], ".")) {
				index++
			}
		case strings.HasPrefix(argument, "-i") || argument == "--in-place" || strings.HasPrefix(argument, "--in-place="):
			// Output targets are the file operands already modeled by
			// analyzeUnwrappedWritePolicy and the sed mutation lifecycle.
		case argument == "-l" || argument == "--line-length":
			index++
			if index >= len(literals) {
				return false
			}
		case argument == "-n" || argument == "--quiet" || argument == "--silent" ||
			argument == "-E" || argument == "-r" || argument == "--regexp-extended" ||
			argument == "-u" || argument == "--unbuffered" || argument == "-s" ||
			argument == "--separate" || argument == "--posix" || argument == "-z" || argument == "--null-data" ||
			strings.HasPrefix(argument, "--line-length="):
		case strings.HasPrefix(argument, "-"):
			// Combined flags are accepted only when they are made entirely of
			// no-value, side-effect-free switches.
			if !shellPolicySafeCombinedSedFlags(argument) {
				return false
			}
		case !seenScript:
			scripts = append(scripts, argument)
			seenScript = true
		}
	}
	if len(scripts) == 0 {
		return true
	}
	for _, script := range scripts {
		operations := parseSedScript(script)
		if len(operations) == 0 {
			return false
		}
		for _, operation := range operations {
			switch operation.Op {
			case "s", "y":
				if !shellPolicySafeSedSubstitutionFlags(operation.Flags) {
					return false
				}
			case "d", "p", "i", "a", "c":
			default:
				// Includes sed's e, w/W, and any grammar the focused parser does
				// not prove safe.
				return false
			}
		}
	}
	return true
}

func shellPolicySafeCombinedSedFlags(argument string) bool {
	if len(argument) < 2 || !strings.HasPrefix(argument, "-") || strings.HasPrefix(argument, "--") {
		return false
	}
	for _, flag := range argument[1:] {
		if !strings.ContainsRune("nErsuz", flag) {
			return false
		}
	}
	return true
}

func shellPolicySafeSedSubstitutionFlags(flags string) bool {
	for _, flag := range strings.TrimSpace(flags) {
		if flag >= '0' && flag <= '9' || strings.ContainsRune("gIpM", flag) || flag == ' ' || flag == '\t' {
			continue
		}
		// GNU/BSD e executes code; w/W routes output to a second target.
		return false
	}
	return true
}

func shellPolicySafeGitInvocation(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) bool {
	literals, ok := shellPolicyStaticArguments(args, known, policy)
	if !ok {
		return false
	}
	subcommand, tail, safe := shellPolicyGitSubcommand(literals)
	if !safe {
		return false
	}
	switch subcommand {
	case "rev-parse", "rev-list", "ls-tree", "name-rev":
		return true
	case "cat-file":
		// --filters and --textconv execute repository-configured filter or
		// diff-driver programs. Their optional path forms use both separate
		// and equals arguments, so reject either spelling.
		return !hasAnyShellPolicyFlag(tail, "--filters", "--textconv")
	case "config":
		return classifyGitConfig(tail) == SemanticRead
	default:
		// status/ls-files/diff can invoke core.fsmonitor, diff/textconv and
		// filter processes; verify-* invokes a signature helper; porcelain
		// display commands can invoke a configured pager. None of those
		// ambient repository authorities is represented by path analysis.
		return false
	}
}

func shellPolicyGitSubcommand(args []string) (string, []string, bool) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			if index+1 >= len(args) {
				return "", nil, true
			}
			return args[index+1], args[index+2:], true
		}
		if argument == "-c" || argument == "--config-env" || argument == "--exec-path" ||
			argument == "-p" || argument == "--paginate" ||
			strings.HasPrefix(argument, "-c=") || strings.HasPrefix(argument, "-c") && len(argument) > 2 ||
			strings.HasPrefix(argument, "--config-env=") || strings.HasPrefix(argument, "--exec-path=") {
			return "", nil, false
		}
		if argument == "-C" || argument == "--git-dir" || argument == "--work-tree" || argument == "--namespace" {
			index++
			if index >= len(args) {
				return "", nil, false
			}
			continue
		}
		if strings.HasPrefix(argument, "--git-dir=") || strings.HasPrefix(argument, "--work-tree=") || strings.HasPrefix(argument, "--namespace=") {
			continue
		}
		if strings.HasPrefix(argument, "-") {
			switch argument {
			case "--no-pager", "--no-replace-objects", "--bare", "--no-optional-locks",
				"--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs":
				continue
			default:
				return "", nil, false
			}
		}
		return argument, args[index+1:], true
	}
	return "", nil, true
}

func analyzeModeledXxdAuthority(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	targetWord, target, hasOutput, ok := shellPolicyXxdOutputWord(args, known, policy)
	if !ok {
		return unrestrictedCodePolicyDecision("xxd")
	}
	if !hasOutput {
		return allowShellPolicyDecision()
	}
	possible := target.literal
	if target.possibleLiteral != "" {
		possible = target.possibleLiteral
	}
	// Protection is checked before filesystem resolution so a not-yet-created
	// .env or git metadata file is still an unconditional Block.
	if possible != "" && permissions.IsProtectedPath(possible) {
		return blockShellPolicyDecision(
			"shell.policy.block.protected", i18n.KeyShellPolicyBlockProtected, possible,
		)
	}
	if target.dynamic || target.commandSub || target.possibleLiteral != "" || shellPolicyWordHasUnquotedExpansion(targetWord) {
		return unrestrictedCodePolicyDecision("xxd")
	}
	if target.literal == "-" || target.literal == "/dev/null" {
		return allowShellPolicyDecision()
	}
	return classifyDestructivePathWithGlob(
		target.literal, policy, shellPolicyWordHasActiveUnquotedGlob(targetWord, target),
	)
}

func shellPolicyXxdOutputWord(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) (*syntax.Word, shellPolicyValue, bool, bool) {
	operands := make([]*syntax.Word, 0, 2)
	endFlags := false
	valueFlags := map[string]bool{
		"-c": true, "-cols": true, "-g": true, "-groupsize": true,
		"-l": true, "-len": true, "-n": true, "-name": true,
		"-o": true, "-offset": true, "-R": true, "-seek": true, "-s": true,
	}
	noValueFlags := map[string]bool{
		"-a": true, "-autoskip": true, "-b": true, "-bits": true,
		"-C": true, "-capitalize": true, "-d": true, "-E": true,
		"-ebcdic": true, "-e": true, "-little-endian": true,
		"-h": true, "-help": true, "-i": true, "-include": true,
		"-ps": true, "-postscript": true, "-plain": true,
		"-r": true, "-revert": true, "-u": true, "-upper": true,
		"-v": true, "-version": true,
	}
	for index := 0; index < len(args); index++ {
		word := args[index]
		value := shellPolicyWordValue(word, known, policy)
		if !endFlags && (value.dynamic || value.commandSub || value.possibleLiteral != "" || shellPolicyWordHasUnquotedExpansion(word)) {
			return nil, shellPolicyValue{}, false, false
		}
		argument := value.literal
		if !endFlags && argument == "--" {
			endFlags = true
			continue
		}
		if !endFlags && strings.HasPrefix(argument, "-") && argument != "-" {
			if noValueFlags[argument] {
				continue
			}
			if valueFlags[argument] {
				index++
				if index >= len(args) {
					return nil, shellPolicyValue{}, false, false
				}
				continue
			}
			if shellPolicyXxdAttachedValueFlag(argument) {
				continue
			}
			return nil, shellPolicyValue{}, false, false
		}
		operands = append(operands, word)
		if len(operands) > 2 {
			return nil, shellPolicyValue{}, false, false
		}
	}
	if len(operands) < 2 {
		return nil, shellPolicyValue{}, false, true
	}
	return operands[1], shellPolicyWordValue(operands[1], known, policy), true, true
}

// shellPolicyXxdOutputTarget parses xxd's [infile [outfile]] grammar. xxd can
// write an outfile in both normal and reverse mode; -r is not the property
// that makes it a mutator. Unknown options fail closed because accepting one
// without its arity would let its value be mistaken for an input operand.
func shellPolicyXxdOutputTarget(args []string) (target string, hasOutput bool, complete bool) {
	operands := make([]string, 0, 2)
	endFlags := false
	valueFlags := map[string]bool{
		"-c": true, "-cols": true, "-g": true, "-groupsize": true,
		"-l": true, "-len": true, "-n": true, "-name": true,
		"-o": true, "-offset": true, "-R": true, "-seek": true, "-s": true,
	}
	noValueFlags := map[string]bool{
		"-a": true, "-autoskip": true, "-b": true, "-bits": true,
		"-C": true, "-capitalize": true, "-d": true, "-E": true,
		"-ebcdic": true, "-e": true, "-little-endian": true,
		"-h": true, "-help": true, "-i": true, "-include": true,
		"-ps": true, "-postscript": true, "-plain": true,
		"-r": true, "-revert": true, "-u": true, "-upper": true,
		"-v": true, "-version": true,
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !endFlags && argument == "--" {
			endFlags = true
			continue
		}
		if !endFlags && strings.HasPrefix(argument, "-") && argument != "-" {
			if noValueFlags[argument] {
				continue
			}
			if valueFlags[argument] {
				index++
				if index >= len(args) {
					return "", false, false
				}
				continue
			}
			if shellPolicyXxdAttachedValueFlag(argument) {
				continue
			}
			return "", false, false
		}
		operands = append(operands, argument)
		if len(operands) > 2 {
			return "", false, false
		}
	}
	if len(operands) < 2 {
		return "", false, true
	}
	return operands[1], true, true
}

func shellPolicyXxdAttachedValueFlag(argument string) bool {
	for _, flag := range []string{
		"-c", "-cols", "-g", "-groupsize", "-l", "-len", "-n", "-name",
		"-o", "-offset", "-R", "-seek", "-s",
	} {
		if strings.HasPrefix(argument, flag+"=") && len(argument) > len(flag)+1 {
			return true
		}
		// The short flags accept attached values (for example -c16 and -s+4).
		if len(flag) == 2 && strings.HasPrefix(argument, flag) && len(argument) > len(flag) {
			return true
		}
	}
	return false
}

func analyzeModeledMktempAuthority(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	call := &syntax.CallExpr{Args: append([]*syntax.Word{{Parts: []syntax.WordPart{&syntax.Lit{Value: "mktemp"}}}}, args...)}
	substitution := &syntax.CmdSubst{Stmts: []*syntax.Stmt{{Cmd: call}}}
	root, ok := shellPolicyCmdSubstMktempDir(substitution, known, policy)
	if !ok {
		return unrestrictedCodePolicyDecision("mktemp")
	}
	resolvedRoot, err := resolveExistingPolicyPath(root)
	if err == nil {
		for _, trustedRoot := range policy.TrustedTempRoots {
			if validTrustedTempRoot(trustedRoot) && policyPathWithin(resolvedRoot, trustedRoot) {
				return allowShellPolicyDecision()
			}
		}
	}
	if decision := classifyDestructivePath(root, policy); decision.Disposition == types.PolicyBlock {
		return decision
	}
	return unrestrictedCodePolicyDecision("mktemp")
}
