package shell

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// CommandSemantic classifies a shell command by its expected effect.
// Higher ordinal values indicate higher impact; pipelines and chained
// commands take the worst (highest) classification of any element.
type CommandSemantic int

const (
	// SemanticUnknown is returned when a command cannot be classified
	// (e.g. unparseable, or invokes an unknown external program).
	SemanticUnknown CommandSemantic = iota
	// SemanticRead is a command that only reads from the filesystem
	// or environment (cat, ls, grep, git status, jq '.', etc.).
	SemanticRead
	// SemanticProcess inspects or controls processes (ps, kill, jobs).
	SemanticProcess
	// SemanticWrite mutates the filesystem in a non-destructive way
	// (mkdir, touch, cp, mv, sed -i, npm install, build/test runners).
	SemanticWrite
	// SemanticNetwork performs network I/O (curl, wget, ssh, scp, rsync).
	SemanticNetwork
	// SemanticDestructive performs irreversible wipe/destroy operations
	// (rm -rf, dd, mkfs, shred, format).
	SemanticDestructive
)

// String returns a stable label for use in metadata/analytics.
func (c CommandSemantic) String() string {
	switch c {
	case SemanticRead:
		return "read"
	case SemanticProcess:
		return "process"
	case SemanticWrite:
		return "write"
	case SemanticNetwork:
		return "network"
	case SemanticDestructive:
		return "destructive"
	default:
		return "unknown"
	}
}

// readCommands lists external programs that only read state.
// git is handled specially via classifyGit because subcommands matter.
var readCommands = map[string]bool{
	"cat": true, "ls": true, "head": true, "tail": true, "wc": true,
	"stat": true, "file": true, "find": true, "tree": true, "du": true,
	"df": true, "echo": true, "printf": true, "true": true, "false": true,
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	"pwd": true, "whoami": true, "id": true, "uname": true, "hostname": true,
	"date": true, "uptime": true, "env": true, "printenv": true, "type": true,
	"which": true, "command": true, "test": true, "[": true, "[[": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ack": true,
	"jq": true, "yq": true, "xmllint": true,
	"awk": true, "sort": true, "uniq": true, "cut": true, "tr": true,
	"comm": true, "diff": true, "cmp": true, "md5sum": true, "sha256sum": true,
	"sha1sum": true, "sha512sum": true, "od": true, "hexdump": true,
	"xxd": true, "less": true, "more": true, "tac": true, "rev": true,
	"column": true, "fold": true, "expand": true, "unexpand": true,
	"nl": true, "fmt": true, "pr": true, "paste": true, "join": true,
	"tput": true, "tty": true, "groups": true, "users": true, "lsof": true,
	"netstat": true, "ss": true, "info": true, "man": true, "help": true,
	"history": true, "alias": true, "set": true, "shopt": true,
	// TS COMMAND_ALLOWLIST extras (readOnlyValidation.ts).
	"nproc": true, "getconf": true, "locale": true,
	"go": true, // go without subcommands like "go list" — refined below
}

// processCommands list those that interact with the process table.
var processCommands = map[string]bool{
	"ps": true, "pgrep": true, "pidof": true, "top": true, "htop": true,
	"jobs": true, "fg": true, "bg": true, "wait": true,
}

// processControlCommands escalate to write/destructive (kill, killall, etc.).
var processControlCommands = map[string]bool{
	"kill": true, "killall": true, "pkill": true,
}

// networkCommands perform remote I/O.
var networkCommands = map[string]bool{
	"curl": true, "wget": true, "nc": true, "ncat": true, "ssh": true,
	"scp": true, "rsync": true, "sftp": true, "telnet": true,
	"ping": true, "ping6": true, "traceroute": true, "dig": true,
	"nslookup": true, "host": true, "whois": true, "ftp": true,
}

// destructiveCommands wipe data irreversibly.
var destructiveCommands = map[string]bool{
	"rm": true, "rmdir": true, "shred": true, "wipe": true, "srm": true,
	"dd": true, "mkfs": true, "format": true, "fdisk": true, "parted": true,
	"mkswap": true, "swapoff": true, "swapon": true,
}

// writeCommands mutate the local filesystem (non-destructive in a normal flow).
var writeCommands = map[string]bool{
	"mkdir": true, "touch": true, "cp": true, "mv": true, "ln": true,
	"chmod": true, "chown": true, "chgrp": true, "tee": true, "install": true,
	"truncate": true, "patch": true, "tar": true, "zip": true, "unzip": true,
	"gzip": true, "gunzip": true, "bzip2": true, "bunzip2": true,
	"xz": true, "unxz": true, "zstd": true, "unzstd": true, "7z": true,
	"npm": true, "pnpm": true, "yarn": true, "bun": true, "pip": true,
	"pip3": true, "poetry": true, "uv": true, "cargo": true, "make": true,
	"gradle": true, "mvn": true, "dotnet": true, "rustc": true, "cc": true,
	"gcc": true, "clang": true, "javac": true, "ld": true,
	"docker": true, "podman": true, "kubectl": true, "helm": true,
	"terraform": true, "pulumi": true, "ansible": true,
}

// readBuiltins are bash builtins that have no side-effects beyond the shell.
var readBuiltins = map[string]bool{
	"cd": true, "pwd": true, "echo": true, "printf": true, "true": true,
	"false": true, "test": true, "[": true, "[[": true, "let": true,
	"local": true, "declare": true, "typeset": true, "readonly": true,
	"export": true, "unset": true, "shift": true, "return": true,
	"exit": true, ":": true, "read": true, "type": true, "alias": true,
	"history": true, "set": true, "shopt": true, "umask": true,
}

// classifyExternal maps an external command name to its semantic.
// Returns SemanticUnknown if the name is not in any allow-list.
func classifyExternal(name string, args []string) CommandSemantic {
	if name == "" {
		return SemanticUnknown
	}

	switch name {
	case "git":
		return classifyGit(args)
	case "go":
		return classifyGo(args)
	case "sed":
		return classifySed(args)
	case "xxd":
		return classifyXxd(args)
	case "find":
		return classifyFind(args)
	case "awk":
		return classifyAwk(args)
	case "xargs":
		return classifyXargs(args)
	case "date":
		return classifyDate(args)
	case "hostname":
		return classifyHostname(args)
	case "info":
		return classifyInfo(args)
	case "sort":
		return classifyOutputFlagCommand(args, "-o", "--output", "--compress-program")
	case "tree":
		return classifyOutputFlagCommand(args, "-o", "--output", "-R")
	case "env", "command", "builtin", "exec", "nice", "nohup", "time", "timeout", "stdbuf", "sudo", "doas":
		return classifyWrappedCommand(name, args)
	case "ssh", "scp", "rsync":
		return SemanticNetwork
	}

	if destructiveCommands[name] {
		return SemanticDestructive
	}
	if networkCommands[name] {
		return SemanticNetwork
	}
	if writeCommands[name] {
		return SemanticWrite
	}
	if processControlCommands[name] {
		return SemanticDestructive
	}
	if processCommands[name] {
		return SemanticProcess
	}
	if readCommands[name] {
		return SemanticRead
	}
	return SemanticUnknown
}

// classifyBuiltin handles bash builtins.
func classifyBuiltin(name string) CommandSemantic {
	if readBuiltins[name] {
		return SemanticRead
	}
	return SemanticUnknown
}

// classifyGit returns the worst semantic of a git invocation.
func classifyGit(args []string) CommandSemantic {
	sub := firstNonFlag(args)
	switch sub {
	case "config":
		return classifyGitConfig(argsAfter(args, sub))
	case "branch":
		return classifyGitBranch(argsAfter(args, sub))
	case "tag":
		return classifyGitTag(argsAfter(args, sub))
	case "remote":
		return classifyGitRemote(argsAfter(args, sub))
	case "status", "diff", "log", "show", "blame", "reflog", "shortlog", "whatchanged":
		// These commands can enter configured fsmonitor, external-diff,
		// textconv/filter, or pager processes even though their primary result
		// is observational.
		return SemanticWrite
	case "verify-tag", "verify-commit":
		// Signature verification delegates to the configured verifier.
		return SemanticWrite
	case "cat-file":
		if hasAnyLiteralFlag(argsAfter(args, sub), "--filters", "--textconv") {
			return SemanticWrite
		}
		return SemanticRead
	case "ls-remote":
		return SemanticNetwork
	case "describe", "rev-parse", "rev-list", "ls-tree",
		"for-each-ref", "fsck", "verify-pack", "name-rev":
		return SemanticRead
	case "ls-files":
		// Index refresh can invoke core.fsmonitor.
		return SemanticWrite
	case "stash":
		// stash with "list"/"show" is read; with "pop"/"drop" is write.
		// Its display path is pager-capable, so both branches require a
		// non-read-only execution boundary.
		next := nextNonFlag(args, sub)
		switch next {
		case "pop", "drop", "clear", "apply", "save", "push", "create", "store", "branch":
			return SemanticWrite
		}
		return SemanticWrite
	case "fetch", "pull", "push", "clone", "submodule":
		return SemanticNetwork
	case "":
		return SemanticRead
	}
	return SemanticWrite
}

// classifyGo returns the worst semantic of a "go" subcommand.
func classifyGo(args []string) CommandSemantic {
	sub := firstNonFlag(args)
	switch sub {
	case "version", "doc", "vet":
		return SemanticRead
	case "list":
		if classifyGoListMayExecuteOrWrite(argsAfter(args, sub)) {
			return SemanticWrite
		}
		return SemanticRead
	case "env":
		for _, arg := range argsAfter(args, sub) {
			if arg == "-w" || arg == "-u" || strings.HasPrefix(arg, "-w=") || strings.HasPrefix(arg, "-u=") {
				return SemanticWrite
			}
		}
		return SemanticRead
	case "build", "install", "generate", "mod", "work", "tool", "fmt", "fix":
		return SemanticWrite
	case "test", "run":
		return SemanticWrite
	case "get":
		return SemanticNetwork
	}
	return SemanticUnknown
}

func classifyGoListMayExecuteOrWrite(args []string) bool {
	for _, argument := range args {
		for _, prefix := range []string{
			"-a", "-asan", "-asmflags", "-buildmode", "-buildvcs", "-compiled",
			"-compiler", "-export", "-gccgoflags", "-gcflags", "-installsuffix",
			"-ldflags", "-linkshared", "-mod", "-modcacherw", "-modfile", "-msan",
			"-overlay", "-pgo", "-pkgdir", "-race", "-tags", "-test", "-toolexec",
			"-trimpath", "-work", "-x",
		} {
			if argument == prefix || strings.HasPrefix(argument, prefix+"=") {
				return true
			}
		}
	}
	return false
}

func hasAnyLiteralFlag(args []string, flags ...string) bool {
	for _, argument := range args {
		for _, flag := range flags {
			if argument == flag || strings.HasPrefix(argument, flag+"=") {
				return true
			}
		}
	}
	return false
}

func classifyXxd(args []string) CommandSemantic {
	_, output, complete := shellPolicyXxdOutputTarget(args)
	if !complete {
		return SemanticUnknown
	}
	if output {
		return SemanticWrite
	}
	return SemanticRead
}

func classifyDate(args []string) CommandSemantic {
	flagsWithValues := map[string]bool{
		"-d": true, "--date": true, "-r": true, "--reference": true,
		"--iso-8601": true, "--rfc-3339": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-s" || arg == "--set" || arg == "-f" || arg == "--file":
			return SemanticWrite
		case strings.HasPrefix(arg, "--set=") || strings.HasPrefix(arg, "--file="):
			return SemanticWrite
		case strings.HasPrefix(arg, "-"):
			if flagsWithValues[arg] && i+1 < len(args) {
				i++
			}
		case !strings.HasPrefix(arg, "+"):
			return SemanticWrite
		}
	}
	return SemanticRead
}

func classifyHostname(args []string) CommandSemantic {
	unsafe := map[string]bool{
		"-F": true, "--file": true, "-b": true, "--boot": true,
		"-y": true, "--yp": true, "--nis": true,
	}
	for _, arg := range args {
		if unsafe[arg] || strings.HasPrefix(arg, "--file=") {
			return SemanticWrite
		}
		if !strings.HasPrefix(arg, "-") {
			return SemanticWrite
		}
	}
	return SemanticRead
}

func classifyInfo(args []string) CommandSemantic {
	return classifyOutputFlagCommand(args, "-o", "--output", "--dribble", "--init-file", "--restore")
}

func classifyOutputFlagCommand(args []string, unsafeFlags ...string) CommandSemantic {
	for _, arg := range args {
		for _, flag := range unsafeFlags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return SemanticWrite
			}
		}
	}
	return SemanticRead
}

func classifyWrappedCommand(wrapper string, args []string) CommandSemantic {
	idx := 0
	switch wrapper {
	case "env":
		for idx < len(args) {
			arg := args[idx]
			if strings.Contains(arg, "=") && !strings.HasPrefix(arg, "-") {
				idx++
				continue
			}
			if arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir" {
				idx += 2
				continue
			}
			if strings.HasPrefix(arg, "-") {
				idx++
				continue
			}
			break
		}
	case "command", "builtin", "exec":
		for idx < len(args) && strings.HasPrefix(args[idx], "-") {
			idx++
		}
	case "nice", "timeout", "stdbuf", "sudo", "doas":
		for idx < len(args) && strings.HasPrefix(args[idx], "-") {
			flag := args[idx]
			idx++
			if (flag == "-n" || flag == "-u" || flag == "-g" || flag == "-C") && idx < len(args) {
				idx++
			}
		}
		if wrapper == "timeout" && idx < len(args) {
			idx++
		}
	}
	if idx >= len(args) {
		if wrapper == "env" || wrapper == "command" || wrapper == "builtin" {
			return SemanticRead
		}
		return SemanticUnknown
	}
	name := strings.TrimSpace(args[idx])
	if slash := strings.LastIndexAny(name, `/\\`); slash >= 0 {
		name = name[slash+1:]
	}
	sem := classifyExternal(name, args[idx+1:])
	if sem == SemanticUnknown {
		sem = classifyBuiltin(name)
	}
	return sem
}

func classifyGitConfig(args []string) CommandSemantic {
	positional := 0
	readMode := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--get" || arg == "--get-all" || arg == "--get-regexp" || arg == "--get-urlmatch" ||
			arg == "--list" || arg == "-l" || arg == "--get-color" || arg == "--get-colorbool":
			readMode = true
		case arg == "--unset" || arg == "--unset-all" || arg == "--rename-section" || arg == "--remove-section" ||
			arg == "--add" || arg == "--replace-all" || arg == "--edit" || arg == "-e":
			return SemanticWrite
		case strings.HasPrefix(arg, "-"):
			// Scope/output type flags do not themselves mutate.
		default:
			positional++
		}
	}
	if readMode || positional <= 1 {
		return SemanticRead
	}
	return SemanticWrite
}

func classifyGitBranch(args []string) CommandSemantic {
	listMode := false
	positional := 0
	for _, arg := range args {
		switch {
		case arg == "--list" || arg == "-l" || arg == "-a" || arg == "--all" || arg == "-r" || arg == "--remotes":
			listMode = true
		case arg == "-d" || arg == "-D" || arg == "--delete" || arg == "-m" || arg == "-M" || arg == "--move" ||
			arg == "-c" || arg == "-C" || arg == "--copy" || arg == "--set-upstream-to" || arg == "--unset-upstream" ||
			arg == "--edit-description" || arg == "--create-reflog":
			return SemanticWrite
		case strings.HasPrefix(arg, "-"):
		default:
			positional++
		}
	}
	if positional == 0 || listMode {
		return SemanticRead
	}
	return SemanticWrite
}

func classifyGitTag(args []string) CommandSemantic {
	listMode := false
	positional := 0
	for _, arg := range args {
		switch {
		case arg == "--list" || arg == "-l":
			listMode = true
		case arg == "-d" || arg == "--delete" || arg == "-a" || arg == "--annotate" || arg == "-s" || arg == "--sign" ||
			arg == "-v" || arg == "--verify" ||
			arg == "-u" || arg == "--local-user" || arg == "-m" || arg == "-F" || arg == "--file" || arg == "--create-reflog":
			return SemanticWrite
		case strings.HasPrefix(arg, "-"):
		default:
			positional++
		}
	}
	if positional == 0 || listMode {
		return SemanticRead
	}
	return SemanticWrite
}

func classifyGitRemote(args []string) CommandSemantic {
	if len(args) == 0 {
		return SemanticRead
	}
	sub := firstNonFlag(args)
	switch sub {
	case "", "show", "get-url":
		return SemanticRead
	case "update", "prune":
		return SemanticNetwork
	default:
		return SemanticWrite
	}
}

func argsAfter(args []string, token string) []string {
	for i, arg := range args {
		if arg == token {
			return args[i+1:]
		}
	}
	return nil
}

// classifySed treats sed as read-only unless -i/--in-place is present.
func classifySed(args []string) CommandSemantic {
	for _, a := range args {
		if a == "--in-place" || strings.HasPrefix(a, "--in-place=") {
			return SemanticWrite
		}
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			if strings.ContainsRune(a, 'i') {
				return SemanticWrite
			}
		}
	}
	return SemanticRead
}

// classifyFind treats find as read-only unless -delete or -exec is present.
func classifyFind(args []string) CommandSemantic {
	for i, a := range args {
		switch a {
		case "-delete":
			return SemanticDestructive
		case "-fprint", "-fprint0", "-fprintf", "-fls":
			return SemanticWrite
		case "-exec", "-execdir", "-ok", "-okdir":
			// Look at the immediate command name to refine.
			if i+1 < len(args) {
				cmd := args[i+1]
				if cmd == "rm" || cmd == "shred" {
					return SemanticDestructive
				}
				if writeCommands[cmd] || destructiveCommands[cmd] {
					return SemanticWrite
				}
			}
			return SemanticWrite
		}
	}
	return SemanticRead
}

var (
	awkSystemPattern      = regexp.MustCompile(`(?i)\bsystem[[:space:]]*\(`)
	awkOutputWritePattern = regexp.MustCompile(`(?i)\b(?:print|printf)\b[^;\n]*(?:>>|>)[[:space:]]*(?:["'$[:alnum:]_(])`)
)

func classifyAwk(args []string) CommandSemantic {
	for i, arg := range args {
		if arg == "-f" || arg == "--file" || strings.HasPrefix(arg, "--file=") {
			return SemanticUnknown
		}
		if strings.HasPrefix(arg, "-") && i == 0 {
			continue
		}
		if awkSystemPattern.MatchString(arg) || awkOutputWritePattern.MatchString(arg) {
			return SemanticWrite
		}
	}
	return SemanticRead
}

var safeXargsTargets = map[string]bool{
	"echo": true, "printf": true, "wc": true,
	"grep": true, "head": true, "tail": true,
}

// classifyXargs only auto-allows the narrow target set used by the TS
// validator. After xargs identifies its target, all remaining arguments are
// passed to that executable, so targets with write/code/network flags cannot
// be inferred safe from the outer xargs flags alone.
func classifyXargs(args []string) CommandSemantic {
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				target = args[i+1]
			}
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			target = arg
			break
		}

		switch arg {
		case "-0", "-t", "-r", "-x":
			continue
		case "-I", "-n", "-P", "-L", "-s", "-E", "-d":
			i++
			if i >= len(args) {
				return SemanticUnknown
			}
			continue
		case "-i", "-e":
			return SemanticUnknown
		}

		if isSafeAttachedXargsFlag(arg) || isSafeCombinedXargsFlag(arg) {
			continue
		}
		return SemanticUnknown
	}

	if target == "" {
		return SemanticRead // xargs defaults to echo
	}
	target = filepathBase(target)
	if safeXargsTargets[target] {
		return SemanticRead
	}
	sem := classifyExternal(target, nil)
	if sem == SemanticDestructive || sem == SemanticNetwork || sem == SemanticWrite {
		return sem
	}
	return SemanticUnknown
}

func isSafeAttachedXargsFlag(arg string) bool {
	for _, prefix := range []string{"-I", "-n", "-P", "-L", "-s", "-E", "-d"} {
		if strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
			return true
		}
	}
	return false
}

func isSafeCombinedXargsFlag(arg string) bool {
	if len(arg) < 3 || !strings.HasPrefix(arg, "-") {
		return false
	}
	for _, flag := range arg[1:] {
		if !strings.ContainsRune("0trx", flag) {
			return false
		}
	}
	return true
}

func filepathBase(name string) string {
	if slash := strings.LastIndexAny(name, `/\\`); slash >= 0 {
		return name[slash+1:]
	}
	return name
}

// firstNonFlag returns the first argument that doesn't start with "-".
func firstNonFlag(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// nextNonFlag returns the next non-flag argument after `after`.
func nextNonFlag(args []string, after string) string {
	found := false
	for _, a := range args {
		if !found {
			if a == after {
				found = true
			}
			continue
		}
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// classifyTopLevel walks a parsed command and returns the worst semantic
// across every CallExpr in the AST (handles &&, ||, ;, |, subshells).
func classifyTopLevel(prog *syntax.File) CommandSemantic {
	worst := SemanticUnknown
	var hasAny, hasUnknown bool
	syntax.Walk(prog, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			hasAny = true
			name := cmdName(n)
			if name == "" {
				hasUnknown = true
				return true
			}
			args := argLiterals(n)
			sem := classifyExternal(name, args)
			if sem == SemanticUnknown {
				sem = classifyBuiltin(name)
			}
			if sem == SemanticUnknown {
				hasUnknown = true
			}
			if sem > worst {
				worst = sem
			}
		}
		return true
	})
	if !hasAny {
		return SemanticUnknown
	}
	if hasUnknown && worst <= SemanticRead {
		return SemanticUnknown
	}
	return worst
}

// ClassifyCommand parses the bash command and returns its overall semantic
// classification. Pipelines and chained commands are classified by the worst
// (highest-ordinal) semantic of any element. Unparseable commands return
// SemanticUnknown.
func ClassifyCommand(cmd string) CommandSemantic {
	if strings.TrimSpace(cmd) == "" {
		return SemanticUnknown
	}
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return SemanticUnknown
	}
	sem := classifyTopLevel(prog)
	// Output redirects (> file, >> file) imply write semantics regardless of
	// the originating command (e.g. `echo hi > file.txt`).
	if sem == SemanticRead || sem == SemanticUnknown {
		hasWriteRedir := false
		syntax.Walk(prog, func(node syntax.Node) bool {
			if stmt, ok := node.(*syntax.Stmt); ok {
				for _, redir := range stmt.Redirs {
					switch redir.Op {
					case syntax.RdrOut, syntax.AppOut, syntax.RdrAll:
						target := wordToString(redir.Word)
						if target != "/dev/null" && target != "/dev/stderr" && target != "/dev/stdout" {
							hasWriteRedir = true
							return false
						}
					}
				}
			}
			return true
		})
		if hasWriteRedir && sem < SemanticWrite {
			return SemanticWrite
		}
	}
	return sem
}
