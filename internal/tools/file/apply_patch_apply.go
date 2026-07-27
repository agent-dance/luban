package file

import (
	"sort"
	"strings"
)

// applyPatchVisibleEvidenceRequirements replays hunk placement while tracking
// each live line back to the original file. Existing old/context lines must be
// visible; a pure insertion must be adjacent to a visible original anchor.
func applyPatchVisibleEvidenceRequirements(file applyPatchFile, before string) ([]ReadLineRange, bool, *applyPatchParseFailure) {
	text := splitApplyPatchText(before)
	if file.Operation == applyPatchDelete && len(file.Hunks) == 0 {
		return nil, true, nil
	}
	origins := make([]int, len(text.Lines))
	for index := range origins {
		origins[index] = index + 1
	}
	required := make(map[int]struct{})
	lineDelta, lastEnd := 0, 0
	for _, hunk := range file.Hunks {
		oldLines, newLines := applyPatchHunkSides(hunk)
		index, reason := locateApplyPatchHunkDetailed(text.Lines, oldLines, hunk, lineDelta, lastEnd)
		if reason != "" {
			return nil, false, applyPatchParseError(reason, file.Path)
		}
		if len(oldLines) == 0 {
			anchor := 0
			if index > 0 {
				anchor = nearestOriginalLine(origins, index-1, -1)
			}
			if anchor == 0 && index < len(origins) {
				anchor = nearestOriginalLine(origins, index, 1)
			}
			if anchor == 0 {
				return nil, true, nil
			}
			required[anchor] = struct{}{}
		} else {
			for offset := range oldLines {
				if origin := origins[index+offset]; origin > 0 {
					required[origin] = struct{}{}
				}
			}
		}

		newOrigins := make([]int, 0, len(newLines))
		oldOffset := 0
		for _, line := range hunk.Lines {
			switch line.Kind {
			case ' ':
				newOrigins = append(newOrigins, origins[index+oldOffset])
				oldOffset++
			case '-':
				oldOffset++
			case '+':
				newOrigins = append(newOrigins, 0)
			}
		}
		updatedOrigins := make([]int, 0, len(origins)-len(oldLines)+len(newOrigins))
		updatedOrigins = append(updatedOrigins, origins[:index]...)
		updatedOrigins = append(updatedOrigins, newOrigins...)
		updatedOrigins = append(updatedOrigins, origins[index+len(oldLines):]...)
		origins = updatedOrigins

		updatedLines := make([]string, 0, len(text.Lines)-len(oldLines)+len(newLines))
		updatedLines = append(updatedLines, text.Lines[:index]...)
		updatedLines = append(updatedLines, newLines...)
		updatedLines = append(updatedLines, text.Lines[index+len(oldLines):]...)
		text.Lines = updatedLines
		lineDelta += len(newLines) - len(oldLines)
		lastEnd = index + len(newLines)
	}
	lines := make([]int, 0, len(required))
	for line := range required {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	ranges := make([]ReadLineRange, 0, len(lines))
	for _, line := range lines {
		ranges = append(ranges, ReadLineRange{StartLine: line, EndLine: line + 1})
	}
	return mergeReadLineRanges(ranges), false, nil
}

func nearestOriginalLine(origins []int, start, step int) int {
	for index := start; index >= 0 && index < len(origins); index += step {
		if origins[index] > 0 {
			return origins[index]
		}
	}
	return 0
}

func readEntryCoversApplyPatchLines(entry ReadFileEntry, required []ReadLineRange) bool {
	if len(required) == 0 {
		return true
	}
	observed := mergeReadLineRanges(cloneReadLineRanges(entry.Coverage))
	for _, target := range required {
		covered := false
		for _, evidence := range observed {
			if evidence.StartLine <= target.StartLine && evidence.EndLine >= target.EndLine {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

type applyPatchText struct {
	Lines        []string
	FinalNewline bool
}

func splitApplyPatchText(content string) applyPatchText {
	if content == "" {
		return applyPatchText{}
	}
	finalNewline := strings.HasSuffix(content, "\n")
	if finalNewline {
		content = strings.TrimSuffix(content, "\n")
	}
	return applyPatchText{Lines: strings.Split(content, "\n"), FinalNewline: finalNewline}
}

func joinApplyPatchText(text applyPatchText) string {
	if len(text.Lines) == 0 {
		return ""
	}
	content := strings.Join(text.Lines, "\n")
	if text.FinalNewline {
		content += "\n"
	}
	return content
}

func applyParsedFilePatch(file applyPatchFile, before string) (string, *applyPatchParseFailure) {
	text := splitApplyPatchText(before)
	if file.Operation == applyPatchCreate {
		text = applyPatchText{FinalNewline: file.CreateFinalNewline}
	}
	lineDelta := 0
	lastEnd := 0
	for _, hunk := range file.Hunks {
		oldLines, newLines := applyPatchHunkSides(hunk)
		index, reason := locateApplyPatchHunkDetailed(text.Lines, oldLines, hunk, lineDelta, lastEnd)
		if reason != "" {
			return "", applyPatchParseError(reason, file.Path)
		}
		updated := make([]string, 0, len(text.Lines)-len(oldLines)+len(newLines))
		updated = append(updated, text.Lines[:index]...)
		updated = append(updated, newLines...)
		updated = append(updated, text.Lines[index+len(oldLines):]...)
		text.Lines = updated
		lineDelta += len(newLines) - len(oldLines)
		lastEnd = index + len(newLines)
		if hunk.NewlineKnown {
			switch {
			case hunk.NewNoNewline:
				text.FinalNewline = false
			case hunk.OldNoNewline:
				text.FinalNewline = true
			}
		}
	}
	if len(text.Lines) == 0 {
		text.FinalNewline = false
	}
	after := joinApplyPatchText(text)
	switch file.Operation {
	case applyPatchCreate:
		if after == "" {
			return "", applyPatchParseError("empty_create", file.Path)
		}
	case applyPatchUpdate:
		if after == before {
			return "", applyPatchParseError("no_changes", file.Path)
		}
	case applyPatchDelete:
		if len(file.Hunks) > 0 && after != "" {
			return "", applyPatchParseError("delete_not_empty", file.Path)
		}
		return "", nil
	}
	return after, nil
}

func applyPatchHunkSides(hunk applyPatchHunk) (oldLines, newLines []string) {
	oldLines = make([]string, 0, hunk.OldCount)
	newLines = make([]string, 0, hunk.NewCount)
	for _, line := range hunk.Lines {
		switch line.Kind {
		case ' ':
			oldLines = append(oldLines, line.Text)
			newLines = append(newLines, line.Text)
		case '-':
			oldLines = append(oldLines, line.Text)
		case '+':
			newLines = append(newLines, line.Text)
		}
	}
	return oldLines, newLines
}

func locateApplyPatchHunk(content, oldLines []string, hunk applyPatchHunk, lineDelta, lastEnd int) (int, bool) {
	index, reason := locateApplyPatchHunkDetailed(content, oldLines, hunk, lineDelta, lastEnd)
	return index, reason == ""
}

func locateApplyPatchHunkDetailed(content, oldLines []string, hunk applyPatchHunk, lineDelta, lastEnd int) (int, string) {
	if len(oldLines) == 0 {
		if !hunk.Positioned {
			return 0, "position_mismatch"
		}
		// In unified diff, -N,0 identifies the gap after original line N.
		// The old one-based-to-zero-based decrement applies only when the
		// hunk consumes old lines; applying it here inserts one line too early.
		index := hunk.OldStart
		index += lineDelta
		if index < lastEnd || index < 0 || index > len(content) {
			return 0, "position_mismatch"
		}
		if hunk.AnchorEOF && index != len(content) {
			return 0, "eof_mismatch"
		}
		return index, ""
	}
	positionInvalid := false
	if hunk.Positioned {
		expected := hunk.OldStart
		if expected > 0 {
			expected--
		}
		expected += lineDelta
		positionInvalid = expected < lastEnd || expected < 0 || expected+len(oldLines) > len(content)
		if !positionInvalid && applyPatchLinesEqualAt(content, oldLines, expected) &&
			(!hunk.AnchorEOF || expected+len(oldLines) == len(content)) {
			return expected, ""
		}
	}
	candidates := applyPatchLineMatches(content, oldLines, lastEnd, hunk.AnchorEOF)
	if len(candidates) == 1 {
		return candidates[0], ""
	}
	if len(candidates) > 1 && strings.TrimSpace(hunk.Section) != "" {
		if selected, ok := selectApplyPatchSectionCandidate(content, candidates, hunk.Section); ok {
			return selected, ""
		}
	}
	if len(candidates) > 1 {
		return 0, "anchor_ambiguous"
	}
	if hunk.AnchorEOF && len(applyPatchLineMatches(content, oldLines, lastEnd, false)) > 0 {
		return 0, "eof_mismatch"
	}
	if positionInvalid {
		return 0, "position_mismatch"
	}
	return 0, "anchor_missing"
}

func applyPatchLinesEqualAt(content, expected []string, index int) bool {
	if index < 0 || index+len(expected) > len(content) {
		return false
	}
	for offset := range expected {
		if content[index+offset] != expected[offset] {
			return false
		}
	}
	return true
}

func applyPatchLineMatches(content, expected []string, minimum int, anchorEOF bool) []int {
	if minimum < 0 {
		minimum = 0
	}
	matches := make([]int, 0, 2)
	for index := minimum; index+len(expected) <= len(content); index++ {
		if anchorEOF && index+len(expected) != len(content) {
			continue
		}
		if applyPatchLinesEqualAt(content, expected, index) {
			matches = append(matches, index)
		}
	}
	return matches
}

func selectApplyPatchSectionCandidate(content []string, candidates []int, section string) (int, bool) {
	section = strings.TrimSpace(section)
	anchors := make([]int, 0, 2)
	for index, line := range content {
		trimmed := strings.TrimSpace(line)
		if trimmed == section || strings.Contains(trimmed, section) {
			anchors = append(anchors, index)
		}
	}
	if len(anchors) != 1 {
		return 0, false
	}
	selected := -1
	for _, candidate := range candidates {
		if candidate < anchors[0] {
			continue
		}
		if selected >= 0 {
			return 0, false
		}
		selected = candidate
	}
	return selected, selected >= 0
}

func applyPatchDiffCounts(file applyPatchFile) (additions, deletions int) {
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			switch line.Kind {
			case '+':
				additions++
			case '-':
				deletions++
			}
		}
	}
	return additions, deletions
}
