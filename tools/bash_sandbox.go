package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SandboxPolicy is the merged set of sandbox knobs. Mirrors the TS settings
// schema: sandbox.enabled / sandbox.allowUnsandboxedCommands /
// sandbox.excludedCommands plus the `dangerouslyDisableSandbox` request flag.
type SandboxPolicy struct {
	// Enabled toggles the entire sandbox layer. False is "no sandbox at all".
	Enabled bool
	// AllowUnsandboxedCommands lets explicitly excluded commands bypass the
	// sandbox even when Enabled is true. Mirrors the TS
	// areUnsandboxedCommandsAllowed switch.
	AllowUnsandboxedCommands bool
	// ExcludedCommands carries the operator's exclude patterns from settings.
	// Each pattern matches a command's first non-env, non-wrapper token.
	ExcludedCommands []string
	// DangerouslyDisableSandbox is the per-call flag from the tool input.
	// When set, the entire gate evaluates to "do NOT sandbox" regardless of
	// other knobs.
	DangerouslyDisableSandbox bool
}

// ShouldUseSandboxDecision is the result of the gate. Reason is included to
// keep the model decision auditable in metadata['sandboxDecision'].
type ShouldUseSandboxDecision struct {
	UseSandbox bool
	Reason     string
}

// envWrapperRe matches a leading FOO=bar (single env-var assignment with no
// whitespace inside the value) so we can strip it before classifying the
// invoker. Mirrors the TS containsExcludedCommand fixed-point loop.
var envWrapperRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=[^\s]*\s+`)

// commonCommandWrappers are passthrough invokers that should NOT count as the
// real command. We unwrap them iteratively (timeout 30 bazel build → bazel)
// so settings.sandbox.excludedCommands can target the underlying tool.
var commonCommandWrappers = map[string]bool{
	"timeout":  true,
	"nice":     true,
	"ionice":   true,
	"stdbuf":   true,
	"unbuffer": true,
	"env":      true,
	"sudo":     true,
	"doas":     true,
	"taskset":  true,
	"chrt":     true,
	"nohup":    true,
}

// stripEnvAndWrappers walks the command head and removes leading env-var
// assignments and common process wrappers (timeout, nice, ...) until the
// remaining first token is the real command. Stops at a fixed point.
func stripEnvAndWrappers(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	for {
		old := cmd
		// Strip leading env assignments: FOO=bar BAZ=qux ...
		for {
			loc := envWrapperRe.FindStringIndex(cmd)
			if loc == nil || loc[0] != 0 {
				break
			}
			cmd = strings.TrimSpace(cmd[loc[1]:])
		}
		// Strip a wrapper command and its first numeric/dash arg if applicable.
		first := commandFirstToken(cmd)
		if !commonCommandWrappers[first] {
			break
		}
		// Eat the wrapper word.
		idx := strings.IndexAny(cmd, " \t")
		if idx < 0 {
			break
		}
		rest := strings.TrimSpace(cmd[idx:])
		// timeout/nice/ionice take a leading numeric argument; keep stripping
		// numeric flags / values like `timeout 30` or `nice -n 10`.
		switch first {
		case "timeout", "nice", "ionice", "chrt", "taskset":
			// Eat one or more flag tokens that look numeric or start with `-`.
			for {
				next := commandFirstToken(rest)
				if next == "" {
					break
				}
				if !strings.HasPrefix(next, "-") && !isNumericToken(next) {
					break
				}
				j := strings.IndexAny(rest, " \t")
				if j < 0 {
					rest = ""
					break
				}
				rest = strings.TrimSpace(rest[j:])
			}
		case "env":
			// `env FOO=bar baz` — env's args may be VAR=val pairs handled above.
		}
		cmd = rest
		if cmd == old {
			break
		}
	}
	return cmd
}

func isNumericToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func commandFirstToken(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	idx := strings.IndexAny(cmd, " \t\n")
	if idx < 0 {
		idx = len(cmd)
	}
	tok := cmd[:idx]
	// Strip any path component so /usr/local/bin/bazel matches "bazel".
	if i := strings.LastIndex(tok, "/"); i >= 0 {
		tok = tok[i+1:]
	}
	if i := strings.LastIndex(tok, "\\"); i >= 0 {
		tok = tok[i+1:]
	}
	return tok
}

// containsExcludedCommand reports whether the command's leading invocation
// (after env/wrapper stripping) matches any pattern in `patterns`. Patterns
// support glob semantics via filepath.Match; a bare name is also accepted.
func containsExcludedCommand(cmd string, patterns []string) (bool, string) {
	stripped := stripEnvAndWrappers(cmd)
	first := commandFirstToken(stripped)
	if first == "" {
		return false, ""
	}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == first {
			return true, p
		}
		if matched, err := filepath.Match(p, first); err == nil && matched {
			return true, p
		}
	}
	return false, ""
}

// ShouldUseSandbox is the consolidated gate that combines:
//   - DangerouslyDisableSandbox flag
//   - SandboxPolicy.Enabled toggle
//   - AllowUnsandboxedCommands + ExcludedCommands
//
// The gate's purpose is to decide whether to build a sandboxed exec.Cmd.
//
// Replaces the legacy ShouldUseSandbox(command, semantics) helper for callers
// that have a SandboxPolicy; that helper is kept as a thin wrapper below.
func ShouldUseSandboxWithPolicy(command string, policy SandboxPolicy) ShouldUseSandboxDecision {
	if policy.DangerouslyDisableSandbox {
		return ShouldUseSandboxDecision{UseSandbox: false, Reason: "dangerouslyDisableSandbox=true"}
	}
	if !policy.Enabled {
		return ShouldUseSandboxDecision{UseSandbox: false, Reason: "sandbox disabled by settings"}
	}
	if policy.AllowUnsandboxedCommands {
		if matched, pat := containsExcludedCommand(command, policy.ExcludedCommands); matched {
			return ShouldUseSandboxDecision{
				UseSandbox: false,
				Reason:     "command matches excludedCommands pattern: " + pat,
			}
		}
	}
	return ShouldUseSandboxDecision{UseSandbox: true, Reason: "sandbox required by policy"}
}

// LoadSandboxPolicyFromEnv reads the same env-var knobs used by tests and the
// runtime entry-point so a caller can populate SandboxPolicy without
// hand-wiring every field. Settings not present default conservatively
// (Enabled=false on platforms without a backend, AllowUnsandboxedCommands=true
// on macOS/Linux to match TS defaults).
func LoadSandboxPolicyFromEnv() SandboxPolicy {
	enabled := strings.EqualFold(os.Getenv("CLAUDE_CODE_SANDBOX_ENABLED"), "true")
	allowUnsandboxed := !strings.EqualFold(os.Getenv("CLAUDE_CODE_SANDBOX_DISALLOW_UNSANDBOXED"), "true")
	excludeRaw := os.Getenv("CLAUDE_CODE_SANDBOX_EXCLUDED_COMMANDS")
	excluded := splitListEnv(excludeRaw)
	return SandboxPolicy{
		Enabled:                  enabled,
		AllowUnsandboxedCommands: allowUnsandboxed,
		ExcludedCommands:         excluded,
	}
}

func splitListEnv(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
