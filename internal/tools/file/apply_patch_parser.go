package file

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	maxApplyPatchBytes = 16 << 20
	maxApplyPatchFiles = 256
	maxApplyPatchHunks = 4096
)

type applyPatchOperation string

const (
	applyPatchCreate applyPatchOperation = "create"
	applyPatchUpdate applyPatchOperation = "update"
	applyPatchDelete applyPatchOperation = "delete"
)

type applyPatchLine struct {
	Kind byte
	Text string
}

type applyPatchHunk struct {
	OldStart     int
	OldCount     int
	NewStart     int
	NewCount     int
	Positioned   bool
	Section      string
	AnchorEOF    bool
	Lines        []applyPatchLine
	OldNoNewline bool
	NewNoNewline bool
	NewlineKnown bool
}

type applyPatchFile struct {
	Path               string
	Operation          applyPatchOperation
	Hunks              []applyPatchHunk
	RequiresRead       bool
	CreateFinalNewline bool
}

type parsedApplyPatch struct {
	Files []applyPatchFile
}

type applyPatchParseFailure struct {
	Reason string
	Path   string
}

func (e *applyPatchParseFailure) Error() string {
	if e == nil {
		return ""
	}
	return e.Reason
}

var unifiedHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?:\s?(.*))?$`)

func parseApplyPatch(raw string) (parsedApplyPatch, *applyPatchParseFailure) {
	if len(raw) == 0 {
		return parsedApplyPatch{}, applyPatchParseError("empty", "")
	}
	if len(raw) > maxApplyPatchBytes {
		return parsedApplyPatch{}, applyPatchParseError("too_large", "")
	}
	if strings.IndexByte(raw, 0) >= 0 {
		return parsedApplyPatch{}, applyPatchParseError("nul_byte", "")
	}
	lines := applyPatchProtocolLines(raw)
	if len(lines) == 0 {
		return parsedApplyPatch{}, applyPatchParseError("empty", "")
	}
	var (
		parsed parsedApplyPatch
		err    *applyPatchParseFailure
	)
	if lines[0] == "*** Begin Patch" {
		parsed, err = parseApplyPatchEnvelope(lines)
	} else {
		parsed, err = parseUnifiedApplyPatch(lines)
	}
	if err != nil {
		return parsedApplyPatch{}, err
	}
	if len(parsed.Files) == 0 {
		return parsedApplyPatch{}, applyPatchParseError("no_files", "")
	}
	if len(parsed.Files) > maxApplyPatchFiles {
		return parsedApplyPatch{}, applyPatchParseError("too_many_files", "")
	}
	seen := make(map[string]struct{}, len(parsed.Files))
	totalHunks := 0
	for index := range parsed.Files {
		file := &parsed.Files[index]
		path, pathErr := normalizeApplyPatchPath(file.Path)
		if pathErr != nil {
			return parsedApplyPatch{}, pathErr
		}
		file.Path = path
		if _, exists := seen[path]; exists {
			return parsedApplyPatch{}, applyPatchParseError("duplicate_target", path)
		}
		seen[path] = struct{}{}
		totalHunks += len(file.Hunks)
		if totalHunks > maxApplyPatchHunks {
			return parsedApplyPatch{}, applyPatchParseError("too_many_hunks", path)
		}
		if file.Operation != applyPatchDelete && len(file.Hunks) == 0 {
			return parsedApplyPatch{}, applyPatchParseError("missing_hunk", path)
		}
		if hunkErr := validateApplyPatchHunks(*file); hunkErr != nil {
			return parsedApplyPatch{}, hunkErr
		}
	}
	return parsed, nil
}

func applyPatchProtocolLines(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	return lines
}

func parseApplyPatchEnvelope(lines []string) (parsedApplyPatch, *applyPatchParseFailure) {
	if len(lines) < 2 || lines[len(lines)-1] != "*** End Patch" {
		return parsedApplyPatch{}, applyPatchParseError("missing_end", "")
	}
	parsed := parsedApplyPatch{Files: make([]applyPatchFile, 0)}
	for index := 1; index < len(lines)-1; {
		line := lines[index]
		if line == "" {
			index++
			continue
		}
		if strings.HasPrefix(line, "*** Move to:") || strings.HasPrefix(line, "*** Rename") {
			return parsedApplyPatch{}, applyPatchParseError("rename_unsupported", "")
		}
		var file applyPatchFile
		switch {
		case strings.HasPrefix(line, "*** Add File:"):
			file.Path = strings.TrimSpace(strings.TrimPrefix(line, "*** Add File:"))
			file.Operation = applyPatchCreate
			file.CreateFinalNewline = true
			index++
			hunk := applyPatchHunk{OldStart: 0, NewStart: 1, Positioned: true}
			for index < len(lines)-1 && !isApplyPatchEnvelopeSection(lines[index]) {
				body := lines[index]
				if body == `\ No newline at end of file` {
					hunk.NewNoNewline = true
					hunk.NewlineKnown = true
					file.CreateFinalNewline = false
					index++
					continue
				}
				if !strings.HasPrefix(body, "+") {
					return parsedApplyPatch{}, applyPatchParseError("invalid_add_line", file.Path)
				}
				hunk.Lines = append(hunk.Lines, applyPatchLine{Kind: '+', Text: body[1:]})
				index++
			}
			hunk.NewCount = len(hunk.Lines)
			if hunk.NewCount == 0 {
				return parsedApplyPatch{}, applyPatchParseError("empty_create", file.Path)
			}
			file.Hunks = []applyPatchHunk{hunk}

		case strings.HasPrefix(line, "*** Update File:"):
			file.Path = strings.TrimSpace(strings.TrimPrefix(line, "*** Update File:"))
			file.Operation = applyPatchUpdate
			file.RequiresRead = true
			index++
			for index < len(lines)-1 && !isApplyPatchEnvelopeSection(lines[index]) {
				header := lines[index]
				if header == "*** End of File" {
					if len(file.Hunks) == 0 {
						return parsedApplyPatch{}, applyPatchParseError("end_without_hunk", file.Path)
					}
					file.Hunks[len(file.Hunks)-1].AnchorEOF = true
					index++
					continue
				}
				if !strings.HasPrefix(header, "@@") {
					return parsedApplyPatch{}, applyPatchParseError("missing_hunk_header", file.Path)
				}
				hunk, headerErr := parseApplyPatchHunkHeader(header)
				if headerErr != nil {
					return parsedApplyPatch{}, applyPatchParseError(headerErr.Reason, file.Path)
				}
				index++
				for index < len(lines)-1 && !strings.HasPrefix(lines[index], "@@") && !isApplyPatchEnvelopeSection(lines[index]) && lines[index] != "*** End of File" {
					body := lines[index]
					if body == `\ No newline at end of file` {
						markApplyPatchNoNewline(&hunk)
						index++
						continue
					}
					if body == "" || (body[0] != ' ' && body[0] != '+' && body[0] != '-') {
						return parsedApplyPatch{}, applyPatchParseError("invalid_hunk_line", file.Path)
					}
					hunk.Lines = append(hunk.Lines, applyPatchLine{Kind: body[0], Text: body[1:]})
					index++
				}
				if !hunk.Positioned {
					hunk.OldCount, hunk.NewCount = applyPatchHunkCounts(hunk)
				}
				file.Hunks = append(file.Hunks, hunk)
			}

		case strings.HasPrefix(line, "*** Delete File:"):
			file.Path = strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File:"))
			file.Operation = applyPatchDelete
			file.RequiresRead = true
			index++
			if index < len(lines)-1 && !isApplyPatchEnvelopeSection(lines[index]) {
				return parsedApplyPatch{}, applyPatchParseError("delete_has_body", file.Path)
			}

		default:
			return parsedApplyPatch{}, applyPatchParseError("unknown_section", "")
		}
		parsed.Files = append(parsed.Files, file)
	}
	return parsed, nil
}

func isApplyPatchEnvelopeSection(line string) bool {
	return strings.HasPrefix(line, "*** Add File:") ||
		strings.HasPrefix(line, "*** Update File:") ||
		strings.HasPrefix(line, "*** Delete File:") ||
		strings.HasPrefix(line, "*** Move to:") ||
		line == "*** End Patch"
}

func parseUnifiedApplyPatch(lines []string) (parsedApplyPatch, *applyPatchParseFailure) {
	parsed := parsedApplyPatch{Files: make([]applyPatchFile, 0)}
	for index := 0; index < len(lines); {
		line := lines[index]
		if line == "" || strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "new file mode ") || strings.HasPrefix(line, "deleted file mode ") ||
			strings.HasPrefix(line, "old mode ") || strings.HasPrefix(line, "new mode ") {
			index++
			continue
		}
		if strings.HasPrefix(line, "rename from ") || strings.HasPrefix(line, "rename to ") || strings.HasPrefix(line, "similarity index ") {
			return parsedApplyPatch{}, applyPatchParseError("rename_unsupported", "")
		}
		if !strings.HasPrefix(line, "--- ") {
			return parsedApplyPatch{}, applyPatchParseError("expected_old_header", "")
		}
		oldPath := unifiedApplyPatchPath(strings.TrimPrefix(line, "--- "))
		index++
		if index >= len(lines) || !strings.HasPrefix(lines[index], "+++ ") {
			return parsedApplyPatch{}, applyPatchParseError("expected_new_header", oldPath)
		}
		newPath := unifiedApplyPatchPath(strings.TrimPrefix(lines[index], "+++ "))
		index++
		file := applyPatchFile{}
		switch {
		case oldPath == "/dev/null" && newPath != "/dev/null":
			file.Path, file.Operation, file.CreateFinalNewline = newPath, applyPatchCreate, true
		case oldPath != "/dev/null" && newPath == "/dev/null":
			file.Path, file.Operation, file.RequiresRead = oldPath, applyPatchDelete, true
		case oldPath == "/dev/null" && newPath == "/dev/null":
			return parsedApplyPatch{}, applyPatchParseError("invalid_null_paths", "")
		default:
			if oldPath != newPath {
				return parsedApplyPatch{}, applyPatchParseError("rename_unsupported", oldPath)
			}
			file.Path, file.Operation, file.RequiresRead = oldPath, applyPatchUpdate, true
		}
		for index < len(lines) {
			if strings.HasPrefix(lines[index], "diff --git ") || strings.HasPrefix(lines[index], "--- ") {
				break
			}
			if lines[index] == "" {
				index++
				continue
			}
			if !strings.HasPrefix(lines[index], "@@") {
				return parsedApplyPatch{}, applyPatchParseError("expected_hunk", file.Path)
			}
			hunk, headerErr := parseApplyPatchHunkHeader(lines[index])
			if headerErr != nil || !hunk.Positioned {
				return parsedApplyPatch{}, applyPatchParseError("invalid_unified_hunk", file.Path)
			}
			index++
			oldSeen, newSeen := 0, 0
			for oldSeen < hunk.OldCount || newSeen < hunk.NewCount {
				if index >= len(lines) {
					return parsedApplyPatch{}, applyPatchParseError("truncated_hunk", file.Path)
				}
				body := lines[index]
				if body == `\ No newline at end of file` {
					markApplyPatchNoNewline(&hunk)
					index++
					continue
				}
				if body == "" || (body[0] != ' ' && body[0] != '+' && body[0] != '-') {
					return parsedApplyPatch{}, applyPatchParseError("invalid_hunk_line", file.Path)
				}
				patchLine := applyPatchLine{Kind: body[0], Text: body[1:]}
				switch patchLine.Kind {
				case ' ':
					oldSeen++
					newSeen++
				case '-':
					oldSeen++
				case '+':
					newSeen++
				}
				if oldSeen > hunk.OldCount || newSeen > hunk.NewCount {
					return parsedApplyPatch{}, applyPatchParseError("hunk_count_mismatch", file.Path)
				}
				hunk.Lines = append(hunk.Lines, patchLine)
				index++
			}
			for index < len(lines) && lines[index] == `\ No newline at end of file` {
				markApplyPatchNoNewline(&hunk)
				index++
			}
			file.Hunks = append(file.Hunks, hunk)
		}
		parsed.Files = append(parsed.Files, file)
	}
	return parsed, nil
}

func parseApplyPatchHunkHeader(header string) (applyPatchHunk, *applyPatchParseFailure) {
	if matches := unifiedHunkHeader.FindStringSubmatch(header); matches != nil {
		oldStart, oldErr := strconv.Atoi(matches[1])
		newStart, newErr := strconv.Atoi(matches[3])
		if oldErr != nil || newErr != nil {
			return applyPatchHunk{}, applyPatchParseError("invalid_hunk_range", "")
		}
		oldCount, countErr := applyPatchRangeCount(matches[2])
		if countErr != nil {
			return applyPatchHunk{}, countErr
		}
		newCount, countErr := applyPatchRangeCount(matches[4])
		if countErr != nil {
			return applyPatchHunk{}, countErr
		}
		return applyPatchHunk{
			OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount,
			Positioned: true, Section: strings.TrimSpace(matches[5]),
		}, nil
	}
	if !strings.HasPrefix(header, "@@") {
		return applyPatchHunk{}, applyPatchParseError("invalid_hunk_header", "")
	}
	section := strings.TrimSpace(strings.TrimPrefix(header, "@@"))
	section = strings.TrimSpace(strings.TrimSuffix(section, "@@"))
	return applyPatchHunk{Section: section}, nil
}

func applyPatchRangeCount(raw string) (int, *applyPatchParseFailure) {
	if raw == "" {
		return 1, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, applyPatchParseError("invalid_hunk_range", "")
	}
	return value, nil
}

func markApplyPatchNoNewline(hunk *applyPatchHunk) {
	if hunk == nil || len(hunk.Lines) == 0 {
		return
	}
	switch hunk.Lines[len(hunk.Lines)-1].Kind {
	case ' ':
		hunk.OldNoNewline = true
		hunk.NewNoNewline = true
		hunk.NewlineKnown = true
	case '-':
		hunk.OldNoNewline = true
		hunk.NewlineKnown = true
	case '+':
		hunk.NewNoNewline = true
		hunk.NewlineKnown = true
	}
}

func applyPatchHunkCounts(hunk applyPatchHunk) (oldCount, newCount int) {
	for _, line := range hunk.Lines {
		switch line.Kind {
		case ' ':
			oldCount++
			newCount++
		case '-':
			oldCount++
		case '+':
			newCount++
		}
	}
	return oldCount, newCount
}

func validateApplyPatchHunks(file applyPatchFile) *applyPatchParseFailure {
	changed := false
	for _, hunk := range file.Hunks {
		oldCount, newCount := applyPatchHunkCounts(hunk)
		if hunk.Positioned && (oldCount != hunk.OldCount || newCount != hunk.NewCount) {
			return applyPatchParseError("hunk_count_mismatch", file.Path)
		}
		if oldCount != newCount || applyPatchHunkHasReplacement(hunk) {
			changed = true
		}
		if !hunk.Positioned && oldCount == 0 {
			return applyPatchParseError("unanchored_insertion", file.Path)
		}
	}
	if file.Operation != applyPatchDelete && !changed {
		return applyPatchParseError("no_changes", file.Path)
	}
	return nil
}

func applyPatchHunkHasReplacement(hunk applyPatchHunk) bool {
	for _, line := range hunk.Lines {
		if line.Kind == '+' || line.Kind == '-' {
			return true
		}
	}
	return false
}

func unifiedApplyPatchPath(raw string) string {
	raw = strings.TrimSpace(strings.SplitN(raw, "\t", 2)[0])
	if raw == "/dev/null" {
		return raw
	}
	if strings.HasPrefix(raw, "a/") || strings.HasPrefix(raw, "b/") {
		raw = raw[2:]
	}
	return raw
}

func normalizeApplyPatchPath(raw string) (string, *applyPatchParseFailure) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/dev/null" || strings.HasPrefix(raw, `"`) || strings.IndexByte(raw, 0) >= 0 {
		return "", applyPatchParseError("invalid_path", raw)
	}
	normalized := strings.ReplaceAll(raw, `\`, "/")
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "//") ||
		(len(normalized) >= 2 && normalized[1] == ':') {
		return "", applyPatchParseError("absolute_path", raw)
	}
	parts := strings.Split(normalized, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", applyPatchParseError("path_traversal", raw)
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return "", applyPatchParseError("invalid_path", raw)
	}
	return strings.Join(cleaned, "/"), nil
}

func applyPatchParseError(reason, path string) *applyPatchParseFailure {
	return &applyPatchParseFailure{Reason: reason, Path: path}
}
