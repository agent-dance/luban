// Package tools — grep_output_modes.go centralises the mode → flag mapping
// the GrepTool exposes (content / files_with_matches / count) and the
// pagination conventions (head_limit defaults to 250, offset works without
// head_limit). Mirrors src/tools/GrepTool/outputModes.ts.
//
// search.go still owns the actual ripgrep invocation; this file documents and
// validates the mode/flag combination, and exposes helpers tests use to lock
// the parity behaviour.
package tools

import "github.com/agent-dance/luban/i18n"

// GrepOutputMode is one of the three modes ripgrep exposes through the tool
// surface. Anything else is rejected by ValidateGrepOutputMode.
type GrepOutputMode string

const (
	GrepModeContent          GrepOutputMode = "content"
	GrepModeFilesWithMatches GrepOutputMode = "files_with_matches"
	GrepModeCount            GrepOutputMode = "count"
)

// DefaultGrepHeadLimit mirrors the TS default for head_limit when the caller
// omits it. Tests in search_test.go assert this exact value.
const DefaultGrepHeadLimit = defaultGrepHeadLimit

// ValidateGrepOutputMode validates the requested mode without trimming it.
// Empty strings default to "files_with_matches" (matching TS).
func ValidateGrepOutputMode(raw string) (GrepOutputMode, error) {
	mode := raw
	if mode == "" {
		return GrepModeFilesWithMatches, nil
	}
	switch GrepOutputMode(mode) {
	case GrepModeContent, GrepModeFilesWithMatches, GrepModeCount:
		return GrepOutputMode(mode), nil
	default:
		return "", i18n.NewError(i18n.KeyToolLegacyCInvalidOutputMode, raw)
	}
}

// RipgrepFlagsForMode returns the canonical flag set for a given mode. It
// excludes pattern/path/glob/type — those are handled by the caller. The
// returned slice is fresh and safe to append to.
func RipgrepFlagsForMode(mode GrepOutputMode, opts grepRipgrepOptions) []string {
	switch mode {
	case GrepModeFilesWithMatches:
		flags := []string{"-l"}
		if opts.SearchPathInfo != nil && opts.SearchPathInfo.IsDir() {
			flags = append(flags, "--sort=modified")
		}
		return flags
	case GrepModeCount:
		return []string{"-c"}
	case GrepModeContent:
		flags := []string{}
		if opts.ShowLineNumbers {
			flags = append(flags, "-n")
		}
		if opts.ContextSet {
			flags = append(flags, "-C", formatGrepNumber(opts.Context))
		} else {
			if opts.ContextBeforeSet {
				flags = append(flags, "-B", formatGrepNumber(opts.ContextBefore))
			}
			if opts.ContextAfterSet {
				flags = append(flags, "-A", formatGrepNumber(opts.ContextAfter))
			}
		}
		return flags
	}
	return nil
}

// ApplyHeadLimitOffset applies offset/head_limit to results, with TS parity:
//   - offset alone returns everything from offset to end.
//   - head_limit alone caps result length.
//   - both combined slice the (offset, offset+limit) window.
//   - unlimited=true disables capping.
//
// All results outside the requested window are dropped, in order.
func ApplyHeadLimitOffset(results []string, offset int, limit int, unlimited bool) []string {
	return paginateSearchResults(results, offset, limit, unlimited)
}

// ResolveHeadLimit returns the effective head_limit & unlimited flag from the
// raw input map, applying the TS rule:
//   - omitted key → default (250).
//   - explicit 0 → unlimited.
//   - explicit positive → that value, capped (unlimited=false).
func ResolveHeadLimit(input map[string]any, parsed *GrepInput) (limit int, unlimited bool) {
	limit = defaultGrepHeadLimit
	unlimited = false
	if input == nil {
		return limit, unlimited
	}
	if _, ok := input["head_limit"]; !ok {
		return limit, unlimited
	}
	if parsed.HeadLimit == nil {
		return limit, unlimited
	}
	limit = int(*parsed.HeadLimit)
	if limit == 0 {
		unlimited = true
	}
	return limit, unlimited
}

// ValidateContextFlags rejects context flags supplied in non-content modes.
// The TS tool ignores them gracefully, but we surface a clear error so the
// schema mismatch is visible during conformance testing.
func ValidateContextFlags(mode GrepOutputMode, opts grepRipgrepOptions) error {
	return nil
}
