// Package file — file_edit_diff.go produces unified-diff hunks for the Edit
// result payload. We don't pull in a third-party diff library: the format is
// fixed, callers only consume our own output, and a 100-line LCS-based diff
// keeps the build minimal.
//
// The shape mirrors TS createPatch from the `diff` npm package. Each hunk
// carries old/new line numbers (1-based, inclusive) and lines prefixed with
// "+", "-", or " ". Three lines of context are emitted around each change.
package file

import (
	"strings"
)

// DiffHunk describes one contiguous patch hunk. JSON keys mirror the TS
// `Hunk` shape: oldStart, oldLines, newStart, newLines, lines.
type DiffHunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

// generateUnifiedHunks returns the unified-diff hunks for a → b with the
// given context size (typically 3 to match TS). The output is empty when the
// inputs are identical.
func generateUnifiedHunks(a, b string, context int) []DiffHunk {
	if context < 0 {
		context = 0
	}
	aLines := splitForDiff(a)
	bLines := splitForDiff(b)

	ops := diffOps(aLines, bLines)
	return buildHunks(aLines, bLines, ops, context)
}

func convertLeadingTabsForDiff(content string) string {
	if !strings.Contains(content, "\t") {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		count := 0
		for count < len(line) && line[count] == '\t' {
			count++
		}
		if count > 0 {
			lines[i] = strings.Repeat("  ", count) + line[count:]
		}
	}
	return strings.Join(lines, "\n")
}

// splitForDiff splits content into lines for diffing. We strip the synthetic
// trailing empty element produced by `strings.Split` for content that ends in
// "\n" so a no-op edit does not surface a phantom hunk.
func splitForDiff(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if last := len(lines) - 1; last >= 0 && lines[last] == "" {
		lines = lines[:last]
	}
	return lines
}

// op describes one element of an LCS-derived diff: equal, delete, insert.
type op struct {
	kind byte // '=', '-', '+'
	line string
}

// diffOps computes a deterministic edit script. Small changed regions use LCS
// for a minimal patch; large regions are split on unique patience anchors so
// memory remains linear instead of exploding at FileEditTool's 1 GiB limit.
func diffOps(a, b []string) []op {
	var prefix []op
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		prefix = append(prefix, op{kind: '=', line: a[0]})
		a = a[1:]
		b = b[1:]
	}

	suffixLen := 0
	for suffixLen < len(a) && suffixLen < len(b) && a[len(a)-1-suffixLen] == b[len(b)-1-suffixLen] {
		suffixLen++
	}
	aMiddle := a[:len(a)-suffixLen]
	bMiddle := b[:len(b)-suffixLen]

	middle := diffOpsMiddle(aMiddle, bMiddle)
	out := make([]op, 0, len(prefix)+len(middle)+suffixLen)
	out = append(out, prefix...)
	out = append(out, middle...)
	for i := suffixLen; i > 0; i-- {
		out = append(out, op{kind: '=', line: a[len(a)-i]})
	}
	return out
}

const maxLCSDiffCells = 4_000_000

func diffOpsMiddle(a, b []string) []op {
	if len(a) == 0 {
		out := make([]op, 0, len(b))
		for _, line := range b {
			out = append(out, op{kind: '+', line: line})
		}
		return out
	}
	if len(b) == 0 {
		out := make([]op, 0, len(a))
		for _, line := range a {
			out = append(out, op{kind: '-', line: line})
		}
		return out
	}
	if len(a) <= maxLCSDiffCells/len(b) {
		return diffOpsLCS(a, b)
	}

	anchors := patienceAnchors(a, b)
	if len(anchors) == 0 {
		out := make([]op, 0, len(a)+len(b))
		for _, line := range a {
			out = append(out, op{kind: '-', line: line})
		}
		for _, line := range b {
			out = append(out, op{kind: '+', line: line})
		}
		return out
	}

	out := make([]op, 0, len(a)+len(b))
	oldCursor, newCursor := 0, 0
	for _, anchor := range anchors {
		out = append(out, diffOps(a[oldCursor:anchor.old], b[newCursor:anchor.new])...)
		out = append(out, op{kind: '=', line: a[anchor.old]})
		oldCursor = anchor.old + 1
		newCursor = anchor.new + 1
	}
	out = append(out, diffOps(a[oldCursor:], b[newCursor:])...)
	return out
}

func diffOpsLCS(a, b []string) []op {
	n, m := len(a), len(b)
	// dp[i][j] = LCS length of a[:i] and b[:j].
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	var rev []op
	i, j := n, m
	for i > 0 && j > 0 {
		switch {
		case a[i-1] == b[j-1]:
			rev = append(rev, op{kind: '=', line: a[i-1]})
			i--
			j--
		case dp[i-1][j] >= dp[i][j-1]:
			rev = append(rev, op{kind: '-', line: a[i-1]})
			i--
		default:
			rev = append(rev, op{kind: '+', line: b[j-1]})
			j--
		}
	}
	for i > 0 {
		rev = append(rev, op{kind: '-', line: a[i-1]})
		i--
	}
	for j > 0 {
		rev = append(rev, op{kind: '+', line: b[j-1]})
		j--
	}
	// Reverse in place.
	for l, r := 0, len(rev)-1; l < r; l, r = l+1, r-1 {
		rev[l], rev[r] = rev[r], rev[l]
	}
	return rev
}

type patienceAnchor struct {
	old int
	new int
}

func patienceAnchors(a, b []string) []patienceAnchor {
	oldCount := make(map[string]int, len(a))
	newCount := make(map[string]int, len(b))
	newIndex := make(map[string]int, len(b))
	for _, line := range a {
		oldCount[line]++
	}
	for i, line := range b {
		newCount[line]++
		newIndex[line] = i
	}
	candidates := make([]patienceAnchor, 0)
	for i, line := range a {
		if oldCount[line] == 1 && newCount[line] == 1 {
			candidates = append(candidates, patienceAnchor{old: i, new: newIndex[line]})
		}
	}
	if len(candidates) < 2 {
		return candidates
	}

	// Longest increasing subsequence over new-line positions preserves order
	// on both sides and yields stable anchors.
	tails := make([]int, 0, len(candidates))
	previous := make([]int, len(candidates))
	for i := range previous {
		previous[i] = -1
	}
	for i, candidate := range candidates {
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := lo + (hi-lo)/2
			if candidates[tails[mid]].new < candidate.new {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo > 0 {
			previous[i] = tails[lo-1]
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}

	anchors := make([]patienceAnchor, len(tails))
	index := tails[len(tails)-1]
	for i := len(anchors) - 1; i >= 0; i-- {
		anchors[i] = candidates[index]
		index = previous[index]
	}
	return anchors
}

// buildHunks groups change ops with their surrounding context lines into the
// unified-diff hunk format expected by TS consumers.
func buildHunks(a, b []string, ops []op, context int) []DiffHunk {
	if len(ops) == 0 {
		return nil
	}

	// Identify change runs (sequences of '+'/'-' interleaved with '='). We
	// emit one hunk per run, expanded by `context` lines on either side.
	type window struct {
		start int // index into ops
		end   int // exclusive
	}
	var windows []window

	i := 0
	for i < len(ops) {
		// Skip equal ops outside any change run.
		if ops[i].kind == '=' {
			i++
			continue
		}
		start := i
		for i < len(ops) && ops[i].kind != '=' {
			i++
		}
		end := i

		// Extend the run by up to `context` equal ops on each side. Adjacent
		// runs whose contexts overlap merge into a single window.
		for back := 0; back < context && start > 0 && ops[start-1].kind == '='; back++ {
			start--
		}
		for fwd := 0; fwd < context && end < len(ops) && ops[end].kind == '='; fwd++ {
			end++
		}
		// Merge with previous window if they touch or overlap.
		if len(windows) > 0 && windows[len(windows)-1].end >= start {
			windows[len(windows)-1].end = end
		} else {
			windows = append(windows, window{start: start, end: end})
		}
	}

	if len(windows) == 0 {
		return nil
	}

	// Convert windows to DiffHunks with absolute line numbers. Line numbers
	// follow TS conventions: 1-based inclusive over the original (a) and new
	// (b) content, counting only ops that contribute to that side.
	hunks := make([]DiffHunk, 0, len(windows))
	oldLine, newLine := 1, 1
	cursor := 0
	for _, w := range windows {
		// Advance past ops before this window.
		for cursor < w.start {
			switch ops[cursor].kind {
			case '=':
				oldLine++
				newLine++
			case '-':
				oldLine++
			case '+':
				newLine++
			}
			cursor++
		}
		hunkOldStart := oldLine
		hunkNewStart := newLine
		var lines []string
		oldCount, newCount := 0, 0
		for cursor < w.end {
			o := ops[cursor]
			switch o.kind {
			case '=':
				lines = append(lines, " "+o.line)
				oldLine++
				newLine++
				oldCount++
				newCount++
			case '-':
				lines = append(lines, "-"+o.line)
				oldLine++
				oldCount++
			case '+':
				lines = append(lines, "+"+o.line)
				newLine++
				newCount++
			}
			cursor++
		}

		// Empty-side hunks (file-level adds/deletes) follow the standard
		// unified-diff convention of reporting a 0-length range with start
		// equal to the line *before* the change; if either side has no
		// lines, snap the start to 0 to match TS createPatch.
		if oldCount == 0 {
			hunkOldStart = hunkOldStart - 1
			if hunkOldStart < 0 {
				hunkOldStart = 0
			}
		}
		if newCount == 0 {
			hunkNewStart = hunkNewStart - 1
			if hunkNewStart < 0 {
				hunkNewStart = 0
			}
		}

		hunks = append(hunks, DiffHunk{
			OldStart: hunkOldStart,
			OldLines: oldCount,
			NewStart: hunkNewStart,
			NewLines: newCount,
			Lines:    lines,
		})
	}
	return hunks
}
