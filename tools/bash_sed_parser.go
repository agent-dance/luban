package tools

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// SedEditOp models a single sed operation (s/d/i/a/p/c/y).
type SedEditOp struct {
	Op          string // "s" | "d" | "i" | "a" | "c" | "p" | "y" | "other"
	AddressFrom string // optional address (line/regex)
	AddressTo   string // optional address range
	Pattern     string // search pattern (s/y)
	Replacement string // replacement (s/y)
	Flags       string // s flags (g, i, etc.)
	Delimiter   byte   // delimiter character used in the script
	Raw         string // raw program text
}

// SedEditPlan describes the in-place edits a sed invocation would perform.
// Returned by ParseSedEdit only when the command actually mutates files
// (i.e. the -i / --in-place flag is present).
type SedEditPlan struct {
	FilePaths []string
	Edits     []SedEditOp
	BackupExt string // "" if no backup, otherwise the suffix (e.g. ".bak")
}

// FilePath returns the first target file (helper for callers that expect
// a single target — sed -i with multiple files modifies each in turn).
func (p *SedEditPlan) FilePath() string {
	if p == nil || len(p.FilePaths) == 0 {
		return ""
	}
	return p.FilePaths[0]
}

// ParseSedEdit returns a non-nil plan when `cmd` invokes sed with -i (in-place
// editing). The returned plan lists every file sed will mutate and the parsed
// edit operations. Returns (nil, false) for read-only sed invocations or for
// commands that do not look like sed at all.
//
// Handles:
//   - -i and -i.bak (with optional backup extension)
//   - --in-place / --in-place=.bak
//   - multiple -e expressions
//   - -f script-file (treated as opaque; FilePaths captured but Edits empty)
//   - semicolon-separated programs
//   - address ranges and addresses (1,5d, /pattern/d, etc.)
func ParseSedEdit(cmd string) (*SedEditPlan, bool) {
	plans := parseSedEditPlans(cmd)
	if len(plans) == 0 {
		return nil, false
	}
	return plans[0], true
}

// parseSedEditPlans returns every statically recognized sed -i invocation in
// a compound shell program. Validation and mutation locking must cover the
// union; the public compatibility helper above continues to return the first.
func parseSedEditPlans(cmd string) []*SedEditPlan {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil
	}

	var plans []*SedEditPlan
	syntax.Walk(prog, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		if cmdName(call) != "sed" {
			return true
		}
		args := argLiterals(call)
		if p := parseSedArgs(args); p != nil {
			plans = append(plans, p)
		}
		return true
	})
	return plans
}

// parseSedArgs walks through `args` (already stripped of the "sed" command name)
// and produces a SedEditPlan when -i is detected.
func parseSedArgs(args []string) *SedEditPlan {
	var (
		inPlace       bool
		backupExt     string
		scripts       []string
		files         []string
		seenScript    bool
		hasScriptFile bool
	)

	skipNext := false
	for i, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		switch {
		case a == "-i":
			inPlace = true
			if i+1 < len(args) && (args[i+1] == "" || strings.HasPrefix(args[i+1], ".")) {
				backupExt = args[i+1]
				skipNext = true
			}
		case strings.HasPrefix(a, "-i") && !strings.HasPrefix(a, "--"):
			inPlace = true
			ext := strings.TrimPrefix(a, "-i")
			if ext != "" {
				backupExt = ext
			}
		case a == "--in-place":
			inPlace = true
		case strings.HasPrefix(a, "--in-place="):
			inPlace = true
			backupExt = strings.TrimPrefix(a, "--in-place=")
		case a == "-e" || a == "--expression":
			if i+1 < len(args) {
				scripts = append(scripts, args[i+1])
				skipNext = true
			}
			seenScript = true
		case strings.HasPrefix(a, "-e") && !strings.HasPrefix(a, "--"):
			scripts = append(scripts, strings.TrimPrefix(a, "-e"))
			seenScript = true
		case strings.HasPrefix(a, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(a, "--expression="))
			seenScript = true
		case a == "-f" || a == "--file":
			hasScriptFile = true
			seenScript = true
			if i+1 < len(args) {
				skipNext = true
			}
		case strings.HasPrefix(a, "-f") && !strings.HasPrefix(a, "--"):
			hasScriptFile = true
			seenScript = true
		case strings.HasPrefix(a, "--file="):
			hasScriptFile = true
			seenScript = true
		case strings.HasPrefix(a, "-"):
			// Unknown sed flag; ignore.
		default:
			// First non-flag without explicit -e is the inline script.
			if !seenScript && looksLikeSedProgram(a) {
				scripts = append(scripts, a)
				seenScript = true
				continue
			}
			files = append(files, a)
		}
	}

	if !inPlace {
		return nil
	}
	plan := &SedEditPlan{
		FilePaths: files,
		BackupExt: backupExt,
	}
	if !hasScriptFile {
		for _, s := range scripts {
			plan.Edits = append(plan.Edits, parseSedScript(s)...)
		}
	}
	return plan
}

func looksLikeSedProgram(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	for _, op := range parseSedScript(s) {
		if op.Op != "" && op.Op != "other" {
			return true
		}
	}
	return false
}

// parseSedScript splits a sed program on `;` and `\n` and parses each piece.
func parseSedScript(prog string) []SedEditOp {
	pieces := splitSedProgram(prog)
	out := make([]SedEditOp, 0, len(pieces))
	for _, p := range pieces {
		op := parseSedPiece(p)
		op.Raw = p
		if op.Op != "" {
			out = append(out, op)
		}
	}
	return out
}

// splitSedProgram splits a sed program into pieces on `;` while respecting
// substitution delimiters and backslash escapes.
func splitSedProgram(s string) []string {
	var pieces []string
	var cur strings.Builder
	var (
		delim          byte
		delimitersLeft int
		escaped        bool
		inAddress      bool
		commandStarted bool
	)
	flush := func() {
		if piece := strings.TrimSpace(cur.String()); piece != "" {
			pieces = append(pieces, piece)
		}
		cur.Reset()
		delim = 0
		delimitersLeft = 0
		escaped = false
		inAddress = false
		commandStarted = false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			cur.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			cur.WriteByte(c)
			escaped = true
			continue
		}
		if delim != 0 {
			cur.WriteByte(c)
			if c == delim {
				delimitersLeft--
				if delimitersLeft == 0 {
					delim = 0
				}
			}
			continue
		}
		if inAddress {
			cur.WriteByte(c)
			if c == '/' {
				inAddress = false
			}
			continue
		}
		switch c {
		case ';', '\n':
			flush()
		case '/':
			cur.WriteByte(c)
			if !commandStarted {
				inAddress = true
			}
		default:
			cur.WriteByte(c)
			if !commandStarted && !isSedAddressPrefixByte(c) {
				commandStarted = true
				if (c == 's' || c == 'y') && i+1 < len(s) && isSedDelimiter(s[i+1]) {
					delim = s[i+1]
					delimitersLeft = 2
					cur.WriteByte(s[i+1])
					i++
				}
			}
		}
	}
	flush()
	return pieces
}

func isSedAddressPrefixByte(c byte) bool {
	return (c >= '0' && c <= '9') || c == '$' || c == ',' || c == '!' || c == ' ' || c == '\t'
}

func isSedDelimiter(c byte) bool {
	switch c {
	case '/', '|', '#', ',', '@', ':':
		return true
	}
	return false
}

// parseSedPiece extracts a single SedEditOp from a piece of the program.
func parseSedPiece(p string) SedEditOp {
	op := SedEditOp{Op: "", Raw: p}
	if p == "" {
		return op
	}

	// Strip optional address(es): N, N,M, /regex/, /regex/,/regex/
	rest := p
	addrFrom, after, hasAddr := splitAddress(rest)
	if hasAddr {
		op.AddressFrom = addrFrom
		// Look for second part of a range.
		if strings.HasPrefix(after, ",") {
			second := strings.TrimPrefix(after, ",")
			to, after2, _ := splitAddress(second)
			op.AddressTo = to
			rest = strings.TrimSpace(after2)
		} else {
			rest = strings.TrimSpace(after)
		}
	}

	if rest == "" {
		return op
	}

	switch rest[0] {
	case 's', 'y':
		if len(rest) < 4 {
			return op
		}
		delim := rest[1]
		op.Op = string(rest[0])
		op.Delimiter = delim
		body := rest[2:]
		// pattern/replacement/flags
		parts := splitSedSubstBody(body, delim)
		if len(parts) >= 1 {
			op.Pattern = parts[0]
		}
		if len(parts) >= 2 {
			op.Replacement = parts[1]
		}
		if len(parts) >= 3 {
			op.Flags = parts[2]
		}
	case 'd':
		op.Op = "d"
	case 'p':
		op.Op = "p"
	case 'i', 'a', 'c':
		op.Op = string(rest[0])
		// Insert/append/change uses backslash-newline syntax.
		op.Replacement = strings.TrimSpace(rest[1:])
	default:
		op.Op = "other"
	}
	return op
}

// splitAddress consumes a leading address: digits, $, /regex/.
// Returns the address text and the remaining script.
func splitAddress(s string) (addr, rest string, ok bool) {
	if s == "" {
		return "", s, false
	}
	switch {
	case s[0] >= '0' && s[0] <= '9':
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		return s[:i], s[i:], true
	case s[0] == '$':
		return "$", s[1:], true
	case s[0] == '/':
		// /pattern/ — find the closing /, respecting escape.
		i := 1
		for i < len(s) {
			if s[i] == '\\' && i+1 < len(s) {
				i += 2
				continue
			}
			if s[i] == '/' {
				return s[:i+1], s[i+1:], true
			}
			i++
		}
		return s, "", true
	}
	return "", s, false
}

// splitSedSubstBody splits "pat<delim>repl<delim>flags" respecting escaping.
func splitSedSubstBody(body string, delim byte) []string {
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\\' && i+1 < len(body) {
			cur.WriteByte(c)
			cur.WriteByte(body[i+1])
			i++
			continue
		}
		if c == delim {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}
