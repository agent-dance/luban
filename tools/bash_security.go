package tools

import (
	"regexp"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// BashSecuritySeverity classifies the impact of a security-rule match.
type BashSecuritySeverity int

const (
	// SeverityWarn is informational; a caller may surface but still execute.
	SeverityWarn BashSecuritySeverity = iota + 1
	// SeverityBlock should prevent execution.
	SeverityBlock
)

// String returns a stable label for use in reports.
func (s BashSecuritySeverity) String() string {
	switch s {
	case SeverityWarn:
		return "warn"
	case SeverityBlock:
		return "block"
	}
	return "unknown"
}

// BashSecurityRule pairs a regex against a description and severity.
type BashSecurityRule struct {
	Name       string
	Pattern    *regexp.Regexp
	Severity   BashSecuritySeverity
	ReasonKey  i18n.Key
	ReasonArgs []any
	// Reason is retained for source compatibility. First-party findings expose
	// the semantic key string here; user-visible copy is resolved from ReasonKey.
	Reason string
}

// BashSecurityFinding describes one matched rule.
type BashSecurityFinding struct {
	Rule     BashSecurityRule
	Match    string // the matched substring
	Severity BashSecuritySeverity
}

// bashSecurityRules is the table of injection / dangerous-shell-trick rules.
// Each entry names the rule, the pattern, and its severity. The table is
// independent of dangerousPatterns so it can grow without disturbing the
// existing block list.
var bashSecurityRules = []BashSecurityRule{
	{
		Name:      "pipe-curl-to-shell",
		Pattern:   regexp.MustCompile(`\b(?:curl|wget|fetch)\b[^|]*\|\s*(?:bash|sh|zsh|ksh|dash|sudo)\b`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityRemotePipeShell,
	},
	{
		Name:      "base64-decode-to-shell",
		Pattern:   regexp.MustCompile(`\bbase64\s+(?:-d|--decode|-D)\b[^|]*\|\s*(?:bash|sh|zsh|sudo)\b`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityEncodedPipeShell,
	},
	{
		Name:      "subshell-base64-decode",
		Pattern:   regexp.MustCompile(`\$\(\s*echo\s+[^)]*\|\s*base64\s+(?:-d|--decode)[^)]*\)`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityEncodedSubstitution,
	},
	{
		Name:      "eval-substitution",
		Pattern:   regexp.MustCompile(`\beval\s+\$(?:\(|{)`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityDynamicEval,
	},
	{
		Name:      "exec-tcp-redirect",
		Pattern:   regexp.MustCompile(`\bexec\s+\d*\s*<>?\s*/dev/(?:tcp|udp)/`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityReverseShell,
	},
	{
		Name:      "bash-tcp-redirect",
		Pattern:   regexp.MustCompile(`\b(?:bash|sh|zsh)\b[^\n]*\s/dev/(?:tcp|udp)/`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityReverseShell,
	},
	{
		Name:       "perl-eval-payload",
		Pattern:    regexp.MustCompile(`\bperl\s+-[a-zA-Z]*e[a-zA-Z]*\s+['"][^'"]*(?:exec|system|` + "`" + `)`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityScriptPayload,
		ReasonArgs: []any{"perl"},
	},
	{
		Name:       "python-eval-payload",
		Pattern:    regexp.MustCompile(`\bpython[23]?\s+-[a-zA-Z]*c[a-zA-Z]*\s+['"][^'"]*(?:os\.system|subprocess|exec\()`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityScriptPayload,
		ReasonArgs: []any{"python"},
	},
	{
		Name:       "ruby-eval-payload",
		Pattern:    regexp.MustCompile(`\bruby\s+-[a-zA-Z]*e[a-zA-Z]*\s+['"][^'"]*(?:system|exec|` + "`" + `)`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityScriptPayload,
		ReasonArgs: []any{"ruby"},
	},
	{
		Name:       "history-clear",
		Pattern:    regexp.MustCompile(`\bhistory\s+-c\b`),
		Severity:   SeverityWarn,
		ReasonKey:  i18n.KeyBashSecurityHistoryTampering,
		ReasonArgs: []any{"history -c"},
	},
	{
		Name:       "unset-history",
		Pattern:    regexp.MustCompile(`\bunset\s+HISTFILE\b`),
		Severity:   SeverityWarn,
		ReasonKey:  i18n.KeyBashSecurityHistoryTampering,
		ReasonArgs: []any{"unset HISTFILE"},
	},
	{
		Name:      "obfuscated-hex-payload",
		Pattern:   regexp.MustCompile(`\$'\\\\x[0-9A-Fa-f]{2}`),
		Severity:  SeverityWarn,
		ReasonKey: i18n.KeyBashSecurityObfuscatedPayload,
	},
	{
		Name:      "ssh-execute-arbitrary",
		Pattern:   regexp.MustCompile(`\bssh\b[^\n]*\b(?:bash|sh|zsh)\s*-c\b`),
		Severity:  SeverityWarn,
		ReasonKey: i18n.KeyBashSecuritySSHInlineShell,
	},
	{
		Name:       "rm-rf-glob-root",
		Pattern:    regexp.MustCompile(`\brm\s+(?:-[a-zA-Z]*[rR][fF][a-zA-Z]*|-[a-zA-Z]*[fF][rR][a-zA-Z]*)\s+/[* ]?(?:\s|$|;)`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityRecursiveDelete,
		ReasonArgs: []any{"/"},
	},
	{
		Name:       "rm-rf-home",
		Pattern:    regexp.MustCompile(`\brm\s+(?:-[a-zA-Z]*[rR][fF][a-zA-Z]*|-[a-zA-Z]*[fF][rR][a-zA-Z]*)\s+(?:\$HOME|~)\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityRecursiveDelete,
		ReasonArgs: []any{"$HOME"},
	},
	{
		Name:      "fork-bomb",
		Pattern:   regexp.MustCompile(`:\(\)\s*\{\s*:\|:\s*&\s*\}\s*;?\s*:`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityForkBomb,
	},
	{
		Name:      "chmod-suid-root",
		Pattern:   regexp.MustCompile(`\bchmod\s+(?:[+]?[u]?[+]?s|4[0-7][0-7][0-7])\s+/`),
		Severity:  SeverityWarn,
		ReasonKey: i18n.KeyBashSecurityChmodSetuid,
	},
	{
		Name:      "chmod-world-write-root",
		Pattern:   regexp.MustCompile(`\bchmod\s+(?:-R\s+)?77[67]\s+/(?:\s|$)`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityChmodWorldWritable,
	},
	{
		Name:      "encoded-eval",
		Pattern:   regexp.MustCompile(`\beval\s+\$\(\s*echo\s+[^)]*\|\s*base64\s+(?:-d|--decode)`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityDynamicEval,
	},
	{
		Name:      "wget-pipe-to-bash",
		Pattern:   regexp.MustCompile(`\bwget\s+-qO?-?\s+[^|]+\|\s*(?:bash|sh|zsh|sudo)\b`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityRemotePipeShell,
	},
	{
		Name:      "curl-pipe-to-bash",
		Pattern:   regexp.MustCompile(`\bcurl\s+(?:-fsSL|-sL|-Ls|-L)\s+[^|]+\|\s*(?:bash|sh|zsh|sudo)\b`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityRemotePipeShell,
	},
	{
		Name:      "subshell-curl-bash",
		Pattern:   regexp.MustCompile(`\$\(\s*curl\s+[^)]+\)`),
		Severity:  SeverityWarn,
		ReasonKey: i18n.KeyBashSecurityDownloadSubstitution,
	},
	// Backtick command substitution invoking shell pipelines.
	{
		Name:      "backtick-curl-pipe-shell",
		Pattern:   regexp.MustCompile("`[^`]*\\b(?:curl|wget|fetch)\\b[^`]*\\|\\s*(?:bash|sh|zsh|sudo)\\b[^`]*`"),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityRemotePipeShell,
	},
	// ZSH_DANGEROUS: rm -rf on system paths (/usr, /etc, /var, /bin, /lib, /sbin, /opt, /boot, /root).
	{
		Name:       "rm-rf-system-path",
		Pattern:    regexp.MustCompile(`\brm\s+(?:-[a-zA-Z]*[rR][fF][a-zA-Z]*|-[a-zA-Z]*[fF][rR][a-zA-Z]*)\s+/(?:usr|etc|var|bin|sbin|lib|lib32|lib64|opt|boot|root|sys|proc)(?:/|\s|$|;)`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityRecursiveDelete,
		ReasonArgs: []any{"a system path"},
	},
	// rm with long flags --recursive --force.
	{
		Name:       "rm-long-flags-recursive-force",
		Pattern:    regexp.MustCompile(`\brm\b(?:[^\n;&|]*\s)?--recursive\b(?:[^\n;&|]*\s)?--force\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityRecursiveDelete,
		ReasonArgs: []any{"files"},
	},
	{
		Name:       "rm-long-flags-force-recursive",
		Pattern:    regexp.MustCompile(`\brm\b(?:[^\n;&|]*\s)?--force\b(?:[^\n;&|]*\s)?--recursive\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityRecursiveDelete,
		ReasonArgs: []any{"files"},
	},
	// dd writing to a raw block device.
	{
		Name:      "dd-raw-disk",
		Pattern:   regexp.MustCompile(`\bdd\b[^\n;&|]*\bof=/dev/(?:sd[a-z]|nvme\d+n\d+|hd[a-z]|vd[a-z]|mmcblk\d+|xvd[a-z]|disk\d+)`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityRawDiskWrite,
	},
	// mkfs on a real partition.
	{
		Name:      "mkfs-partition",
		Pattern:   regexp.MustCompile(`\bmkfs(?:\.\w+)?\s+(?:[^\n;&|]*\s)?/dev/(?:sd[a-z]\d*|nvme\d+n\d+(?:p\d+)?|hd[a-z]\d*|vd[a-z]\d*|mmcblk\d+(?:p\d+)?|xvd[a-z]\d*)`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityFilesystemFormat,
	},
	// System power state.
	{
		Name:       "shutdown",
		Pattern:    regexp.MustCompile(`\bshutdown\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityPowerOperation,
		ReasonArgs: []any{"shutdown"},
	},
	{
		Name:       "reboot",
		Pattern:    regexp.MustCompile(`\breboot\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityPowerOperation,
		ReasonArgs: []any{"reboot"},
	},
	{
		Name:       "halt",
		Pattern:    regexp.MustCompile(`\bhalt\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityPowerOperation,
		ReasonArgs: []any{"halt"},
	},
	{
		Name:       "poweroff",
		Pattern:    regexp.MustCompile(`\bpoweroff\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityPowerOperation,
		ReasonArgs: []any{"poweroff"},
	},
	{
		Name:       "init-runlevel",
		Pattern:    regexp.MustCompile(`\binit\s+[06]\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityPowerOperation,
		ReasonArgs: []any{"init 0/6"},
	},
	// User crontab purge / dangerous edits.
	{
		Name:      "crontab-purge",
		Pattern:   regexp.MustCompile(`\bcrontab\s+(?:-[a-zA-Z]*r[a-zA-Z]*|--remove)\b`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityCrontabRemoval,
	},
	// Firewall flush.
	{
		Name:       "iptables-flush",
		Pattern:    regexp.MustCompile(`\biptables\b[^\n;&|]*\s(?:-F|--flush)\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityFirewallFlush,
		ReasonArgs: []any{"iptables -F"},
	},
	{
		Name:       "ip6tables-flush",
		Pattern:    regexp.MustCompile(`\bip6tables\b[^\n;&|]*\s(?:-F|--flush)\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityFirewallFlush,
		ReasonArgs: []any{"ip6tables -F"},
	},
	{
		Name:       "nftables-flush",
		Pattern:    regexp.MustCompile(`\bnft\s+flush\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityFirewallFlush,
		ReasonArgs: []any{"nft flush"},
	},
	// chmod 000 lockout on a system path.
	{
		Name:      "chmod-000-system-path",
		Pattern:   regexp.MustCompile(`\bchmod\s+(?:-R\s+)?0?000\s+/`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityPermissionLockout,
	},
	// killing critical services.
	{
		Name:       "killall-critical-service",
		Pattern:    regexp.MustCompile(`\bkillall\b[^\n;&|]*\s(?:sshd|systemd|init|launchd|dbus|networkmanager|cron|atd)\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityCriticalService,
		ReasonArgs: []any{"killall"},
	},
	{
		Name:       "pkill-critical-service",
		Pattern:    regexp.MustCompile(`\bpkill\b[^\n;&|]*\s(?:sshd|systemd|init|launchd|dbus|cron|atd)\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityCriticalService,
		ReasonArgs: []any{"pkill"},
	},
	// systemctl stop/disable on critical services.
	{
		Name:       "systemctl-disable-ssh",
		Pattern:    regexp.MustCompile(`\bsystemctl\s+(?:stop|disable|mask)\s+(?:sshd?|networkmanager|systemd-networkd)\b`),
		Severity:   SeverityBlock,
		ReasonKey:  i18n.KeyBashSecurityCriticalService,
		ReasonArgs: []any{"systemctl"},
	},
	// userdel / passwd / shadow disruption.
	{
		Name:      "userdel-root",
		Pattern:   regexp.MustCompile(`\buserdel\s+(?:-r\s+)?(?:root|admin)\b`),
		Severity:  SeverityBlock,
		ReasonKey: i18n.KeyBashSecurityPrivilegedUserDelete,
	},
	// Disk partitioning destructive moves.
	{
		Name:      "fdisk-write",
		Pattern:   regexp.MustCompile(`\bfdisk\s+/dev/(?:sd[a-z]|nvme\d+n\d+|hd[a-z]|vd[a-z])`),
		Severity:  SeverityWarn,
		ReasonKey: i18n.KeyBashSecurityDiskRepartition,
	},
}

// EvaluateBashSecurity scans `cmd` against the security rule table and
// returns every matched finding (in declaration order). An empty slice means
// the command did not trigger any rule.
func EvaluateBashSecurity(cmd string) []BashSecurityFinding {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	var findings []BashSecurityFinding
	for _, rule := range bashSecurityRules {
		if rule.Pattern == nil {
			continue
		}
		if m := rule.Pattern.FindString(cmd); m != "" {
			if rule.Reason == "" && rule.ReasonKey != "" {
				rule.Reason = string(rule.ReasonKey)
			}
			findings = append(findings, BashSecurityFinding{
				Rule:     rule,
				Match:    m,
				Severity: rule.Severity,
			})
		}
	}
	return findings
}

// HighestSeverity returns the worst severity in the findings slice.
func HighestSeverity(findings []BashSecurityFinding) BashSecuritySeverity {
	worst := BashSecuritySeverity(0)
	for _, f := range findings {
		if f.Severity > worst {
			worst = f.Severity
		}
	}
	return worst
}

// FindingReasons returns the reasons of all findings joined for messaging.
func FindingReasons(findings []BashSecurityFinding) string {
	reasons := make([]string, 0, len(findings))
	for _, f := range findings {
		if f.Rule.ReasonKey != "" {
			reasons = append(reasons, toolPermissionFormat(f.Rule.ReasonKey, f.Rule.ReasonArgs...))
		} else if f.Rule.Reason != "" {
			reasons = append(reasons, f.Rule.Reason)
		}
	}
	return strings.Join(reasons, "; ")
}

// SegmentSeparator describes how two adjacent shell segments are joined.
type SegmentSeparator int

const (
	// SegSeparatorNone marks the very first segment (no preceding separator).
	SegSeparatorNone SegmentSeparator = iota
	// SegSeparatorAndOr is "&&" or "||" — sequenced and cwd-preserving.
	SegSeparatorAndOr
	// SegSeparatorSemi is ";" — sequenced and cwd-preserving.
	SegSeparatorSemi
	// SegSeparatorPipe is "|" — separate subshell, cwd does NOT propagate.
	SegSeparatorPipe
	// SegSeparatorBackground is "&" — backgrounded.
	SegSeparatorBackground
)

func (s SegmentSeparator) String() string {
	switch s {
	case SegSeparatorAndOr:
		return "&&||"
	case SegSeparatorSemi:
		return ";"
	case SegSeparatorPipe:
		return "|"
	case SegSeparatorBackground:
		return "&"
	}
	return "none"
}

// BashSegment is a single command from a compound shell expression. It carries
// the segment text, the separator that introduced it, and a copy of the text
// with file-redirection clauses stripped (used for permission/allow-rule
// matching, since `ls > /etc/passwd` should still match a `Bash(ls:*)` rule).
type BashSegment struct {
	// Raw is the exact substring of the original command for this segment.
	Raw string
	// Stripped has its `> file`, `>> file`, `2> file`, `&> file` redirections
	// removed. Use this for allow/deny-rule matching.
	Stripped string
	// RedirectionTargets are file paths captured from output redirections that
	// must be checked separately as write operations.
	RedirectionTargets []string
	// Separator is the operator that introduced this segment.
	Separator SegmentSeparator
}

// shellSegmentSplitRe matches the operator that closes a segment. We split on
// the literals at the top level (not inside quotes/heredocs/braces); for the
// rules below the simple regex is sufficient because we operate on a flat
// string, callers wishing precise boundaries should drop into the syntax
// package and walk the AST.
var shellSegmentSplitRe = regexp.MustCompile(`(\|\||&&|;|\||&)`)

// outputRedirectionRe matches `>file`, `>>file`, `2>file`, `2>>file`, `&>file`
// and similar forms together with a trailing whitespace-delimited target. The
// result of FindAllStringSubmatchIndex pinpoints the operator and the target
// so that `SplitBashSegments` can excise both pieces.
var outputRedirectionRe = regexp.MustCompile(
	`(?:^|\s)(?:[12]?>>?|&>>?|>&)\s*([^\s|;&<>]+)`,
)

// SplitBashSegments splits a compound shell expression into its top-level
// segments along `&&`, `||`, `;`, `|`, `&`. It does not attempt to handle
// nested subshells perfectly — those are returned as one opaque segment with
// SegSeparatorNone for any internal operators. The caller is responsible for
// further parsing if required.
func SplitBashSegments(cmd string) []BashSegment {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	var segments []BashSegment
	idxs := shellSegmentSplitRe.FindAllStringSubmatchIndex(cmd, -1)
	prev := 0
	prevSep := SegSeparatorNone
	for _, m := range idxs {
		opStart, opEnd := m[0], m[1]
		// Account for whitespace before the operator: shellSegmentSplitRe
		// captures the operator only, so extract it directly.
		op := cmd[opStart:opEnd]
		raw := strings.TrimSpace(cmd[prev:opStart])
		if raw != "" {
			seg := buildBashSegment(raw, prevSep)
			segments = append(segments, seg)
		}
		switch op {
		case "&&", "||":
			prevSep = SegSeparatorAndOr
		case ";":
			prevSep = SegSeparatorSemi
		case "|":
			prevSep = SegSeparatorPipe
		case "&":
			prevSep = SegSeparatorBackground
		}
		prev = opEnd
	}
	tail := strings.TrimSpace(cmd[prev:])
	if tail != "" {
		segments = append(segments, buildBashSegment(tail, prevSep))
	}
	return segments
}

// buildBashSegment captures the redirection targets of a raw segment and
// returns the segment with those redirections stripped from its body.
func buildBashSegment(raw string, sep SegmentSeparator) BashSegment {
	stripped := raw
	var targets []string
	matches := outputRedirectionRe.FindAllStringSubmatchIndex(raw, -1)
	if len(matches) > 0 {
		// Walk in reverse so index shifts stay consistent.
		for i := len(matches) - 1; i >= 0; i-- {
			m := matches[i]
			start, end := m[0], m[1]
			tgtStart, tgtEnd := m[2], m[3]
			if tgtStart >= 0 && tgtEnd <= len(raw) {
				targets = append([]string{raw[tgtStart:tgtEnd]}, targets...)
			}
			stripped = stripped[:start] + " " + stripped[end:]
		}
		stripped = strings.Join(strings.Fields(stripped), " ")
	}
	return BashSegment{
		Raw:                raw,
		Stripped:           stripped,
		RedirectionTargets: targets,
		Separator:          sep,
	}
}

// CompoundCdViolation describes a multi-cd or cross-pipe-cd pattern that the
// Bash permission system MUST reject. Two distinct cases are flagged:
//
//  1. More than one `cd` in a single compound expression. Each subsequent `cd`
//     can change the cwd for subsequent destructive segments under what looks
//     like an allowed prefix.
//  2. A `cd` in segment N that is followed (segment N+1) by a destructive
//     write whose introducer is `|` rather than `&&`/`;`. Pipes start a
//     subshell so the cd does not propagate, but the write target is computed
//     against the cd'd path — the model can use this to evade per-segment cwd
//     reasoning that the allow-list relies on.
type CompoundCdViolation struct {
	// ReasonKey is resolved only when the violation is shown to a user.
	ReasonKey i18n.Key
	// Reason is the stable semantic code retained for source compatibility.
	Reason string
	// SegmentIndex is the offending segment (or the second cd's index).
	SegmentIndex int
	// Segment is the offending segment text.
	Segment string
}

var cdCallRe = regexp.MustCompile(`^\s*cd(?:\s|$)`)
var writeIntroRe = regexp.MustCompile(`^\s*(?:tee|sh|bash|zsh|cat|cp|mv|rm|dd|truncate|chmod|chown|touch|ln|install)(?:\s|$)`)

// FindCompoundCdViolations inspects the segment list of `cmd` and returns the
// cd-related violations to reject. Returns an empty slice when none are found.
func FindCompoundCdViolations(cmd string) []CompoundCdViolation {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	segments := SplitBashSegments(cmd)
	if len(segments) < 2 {
		return nil
	}
	var violations []CompoundCdViolation
	cdSeen := 0
	for i, seg := range segments {
		if cdCallRe.MatchString(seg.Stripped) {
			cdSeen++
			if cdSeen > 1 {
				violations = append(violations, CompoundCdViolation{
					ReasonKey:    i18n.KeyBashSecurityCompoundMultipleCD,
					Reason:       string(i18n.KeyBashSecurityCompoundMultipleCD),
					SegmentIndex: i,
					Segment:      seg.Raw,
				})
			}
			// cd then cross-pipe write.
			if i+1 < len(segments) && segments[i+1].Separator == SegSeparatorPipe {
				next := segments[i+1]
				if writeIntroRe.MatchString(next.Stripped) {
					violations = append(violations, CompoundCdViolation{
						ReasonKey:    i18n.KeyBashSecurityCompoundCrossPipeWrite,
						Reason:       string(i18n.KeyBashSecurityCompoundCrossPipeWrite),
						SegmentIndex: i + 1,
						Segment:      next.Raw,
					})
				}
			}
		}
	}
	return violations
}

// CompoundCdViolationReasons resolves detection codes at the display boundary.
func CompoundCdViolationReasons(violations []CompoundCdViolation) string {
	reasons := make([]string, 0, len(violations))
	for _, violation := range violations {
		reasons = append(reasons, toolPermissionText(violation.ReasonKey))
	}
	return strings.Join(reasons, "; ")
}

// BuildSegmentWithoutRedirections is the public alias TS code base names
// `buildSegmentWithoutRedirections`. Strips `>`/`>>`/`2>` redirections from
// `segment` and returns the cleaned text along with the captured target list.
// Callers should match the cleaned text against allow/deny rules and check
// the captured targets separately as write operations.
func BuildSegmentWithoutRedirections(segment string) (string, []string) {
	seg := buildBashSegment(strings.TrimSpace(segment), SegSeparatorNone)
	return seg.Stripped, seg.RedirectionTargets
}
