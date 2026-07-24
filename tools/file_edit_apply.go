// Package tools — file_edit_apply.go implements the core string-replacement
// engine that backs the Edit tool. It mirrors the TS applyEdit + line-ending
// preservation logic from src/tools/FileEditTool/utils.ts.
//
// Two responsibilities live here:
//
//  1. ApplyEdit — replaces `oldString` with `newString` inside `content`,
//     enforcing the "ambiguous match" guard that TS surfaces (single
//     occurrence by default; replace_all required to rewrite many).
//
//  2. Line-ending preservation — TS detects whether a file is predominantly
//     CRLF or LF, normalises to LF for matching, then restores the original
//     style on write. Mixed files keep their dominant style. This module
//     exposes detectLineEnding / normaliseToLF / restoreLineEnding for the
//     Edit tool to compose.
package tools

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/types"
)

func readEntryCoversEdit(entry ReadFileEntry, content, oldString string, replaceAll bool) ([]types.ToolErrorRange, bool) {
	if !entry.CoverageKnown || readEntryCoverageComplete(entry) {
		return nil, true
	}
	matches := editMatchLineRanges(content, oldString)
	if !replaceAll && len(matches) > 1 {
		return mergeToolErrorRanges(matches), false
	}
	if len(matches) == 0 {
		return nil, false
	}

	// Return only the portions that have not yet been made model-visible.
	// Edit retries are iterative: a Read may be paged down by the token guard,
	// after which the next Edit must advance to the remaining suffix instead of
	// advertising the original (already partially covered) match again.
	uncovered := subtractObservedReadCoverage(mergeToolErrorRanges(matches), entry.Coverage)
	return uncovered, len(uncovered) == 0
}

// mergeToolErrorRanges normalizes 1-based inclusive edit requirements. Line
// evidence is the authorization unit, so overlapping or adjacent match ranges
// can be satisfied by one Read without weakening the anchor check.
func mergeToolErrorRanges(ranges []types.ToolErrorRange) []types.ToolErrorRange {
	normalized := make([]types.ToolErrorRange, 0, len(ranges))
	for _, value := range ranges {
		if value.StartLine < 1 {
			value.StartLine = 1
		}
		if value.EndLine < value.StartLine {
			continue
		}
		normalized = append(normalized, value)
	}
	if len(normalized) < 2 {
		return normalized
	}
	slices.SortFunc(normalized, func(a, b types.ToolErrorRange) int {
		if a.StartLine != b.StartLine {
			return cmp.Compare(a.StartLine, b.StartLine)
		}
		return cmp.Compare(a.EndLine, b.EndLine)
	})
	merged := normalized[:1]
	for _, current := range normalized[1:] {
		last := &merged[len(merged)-1]
		if current.StartLine <= last.EndLine+1 {
			if current.EndLine > last.EndLine {
				last.EndLine = current.EndLine
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

// subtractObservedReadCoverage subtracts half-open Read evidence from the
// inclusive edit requirements and returns sorted, disjoint missing ranges.
// It deliberately splits a partially observed requirement so token-limited
// Read retries make monotonic progress.
func subtractObservedReadCoverage(required []types.ToolErrorRange, observed []ReadLineRange) []types.ToolErrorRange {
	observed = mergeReadLineRanges(cloneReadLineRanges(observed))
	uncovered := make([]types.ToolErrorRange, 0, len(required))
	for _, target := range required {
		cursor := target.StartLine
		for _, evidence := range observed {
			evidenceEnd := evidence.EndLine - 1
			if evidenceEnd < cursor {
				continue
			}
			if evidence.StartLine > target.EndLine {
				break
			}
			if evidence.StartLine > cursor {
				uncovered = append(uncovered, types.ToolErrorRange{
					StartLine: cursor,
					EndLine:   min(target.EndLine, evidence.StartLine-1),
				})
			}
			if evidenceEnd >= cursor {
				cursor = evidenceEnd + 1
			}
			if cursor > target.EndLine {
				break
			}
		}
		if cursor <= target.EndLine {
			uncovered = append(uncovered, types.ToolErrorRange{StartLine: cursor, EndLine: target.EndLine})
		}
	}
	return mergeToolErrorRanges(uncovered)
}

func editMatchLineRanges(content, match string) []types.ToolErrorRange {
	if match == "" {
		return nil
	}
	var ranges []types.ToolErrorRange
	for searchFrom := 0; searchFrom <= len(content)-len(match); {
		relative := strings.Index(content[searchFrom:], match)
		if relative < 0 {
			break
		}
		index := searchFrom + relative
		startLine := strings.Count(content[:index], "\n") + 1
		lastLine := startLine + strings.Count(match, "\n")
		if strings.HasSuffix(match, "\n") && lastLine > startLine {
			lastLine--
		}
		ranges = append(ranges, types.ToolErrorRange{StartLine: startLine, EndLine: lastLine})
		searchFrom = index + len(match)
	}
	return ranges
}

// Sentinel errors so callers can distinguish failure modes without
// string-matching the message. Keep the user-facing message identical to TS
// for transcript parity.
var (
	// ErrEditOldStringMissing reports that `old_string` was not found in the
	// file. TS surfaces a slightly different message ("String not found in
	// file. Failed to apply edit.") which we mirror so prompts that grep
	// for it keep working.
	ErrEditOldStringMissing = errors.New("String not found in file. Failed to apply edit.")

	// ErrEditAmbiguousMatch reports more than one occurrence when the caller
	// did not pass replace_all=true. TS message: "Found N matches of the
	// string to replace, but replace_all is false. To replace all
	// occurrences, set replace_all to true. To replace only one occurrence,
	// please provide more context to uniquely identify the instance."
	ErrEditAmbiguousMatch = errors.New("ambiguous match: multiple occurrences without replace_all")

	// ErrEditIdenticalStrings reports that old_string and new_string are
	// identical. TS message: "No changes to make: old_string and new_string
	// are exactly the same."
	ErrEditIdenticalStrings = errors.New("No changes to make: old_string and new_string are exactly the same.")

	// ErrEditEmptyOldString reports that old_string was empty against a
	// non-empty file. Empty old_string is reserved for creating new files
	// in TS, but Edit (vs Write) must refuse it on existing content.
	ErrEditEmptyOldString = errors.New("old_string is empty: use the Write tool to create a new file")
)

// ApplyEdit performs the actual replacement. It mirrors TS applyEditToFile
// extended with the multi-occurrence guard that lives in TS validateInput
// (we fold them together because Go callers want one place to fail).
//
// Behaviour:
//   - replaceAll=false, exactly one match → replace once.
//   - replaceAll=false, more than one match → ErrEditAmbiguousMatch.
//   - replaceAll=true → replace every occurrence.
//   - oldString == newString → ErrEditIdenticalStrings.
//   - oldString == "" with non-empty content → ErrEditEmptyOldString.
//   - oldString not found → ErrEditOldStringMissing.
//
// The returned `occurrences` is the number of matches that were actually
// rewritten (1 in single mode, N in replace_all mode).
//
// Line endings are NOT touched here — callers should normalise via
// normaliseToLF before calling and restore via restoreLineEnding afterwards
// so the matching logic operates on a single canonical form.
func ApplyEdit(content, oldString, newString string, replaceAll bool) (string, int, error) {
	if oldString == newString {
		return "", 0, ErrEditIdenticalStrings
	}
	if oldString == "" {
		if content == "" {
			// Empty-file → Write semantics (write the new content directly).
			return newString, 1, nil
		}
		return "", 0, ErrEditEmptyOldString
	}

	rawCount := strings.Count(content, oldString)
	if rawCount == 0 {
		return "", 0, ErrEditOldStringMissing
	}
	if rawCount > 1 && !replaceAll {
		return "", rawCount, fmt.Errorf(
			"Found %d matches of the string to replace, but replace_all is false. "+
				"To replace all occurrences, set replace_all to true. To replace only "+
				"one occurrence, please provide more context to uniquely identify the instance",
			rawCount,
		)
	}

	// TS strips a single trailing newline when the user supplied
	// `oldString` without one but the file has the line followed by a newline
	// AND the replacement is empty. This avoids leaving a stray blank line
	// when the model "deletes a line" without explicitly including the
	// terminating newline in old_string.
	useTrailingNewlineHack := newString == "" &&
		!strings.HasSuffix(oldString, "\n") &&
		strings.Contains(content, oldString+"\n")

	search := oldString
	if useTrailingNewlineHack {
		search = oldString + "\n"
	}
	appliedCount := strings.Count(content, search)

	if replaceAll {
		out := strings.ReplaceAll(content, search, newString)
		return out, appliedCount, nil
	}
	out := strings.Replace(content, search, newString, 1)
	return out, 1, nil
}

// Curly quote constants mirror src/tools/FileEditTool/utils.ts. The TS Edit
// path accepts straight quotes from the model when the file contains curly
// quotes, then preserves the file's quote style in the replacement text.
const (
	leftSingleCurlyQuote  = '‘'
	rightSingleCurlyQuote = '’'
	leftDoubleCurlyQuote  = '“'
	rightDoubleCurlyQuote = '”'
)

func normalizeQuotes(s string) string {
	replacer := strings.NewReplacer(
		string(leftSingleCurlyQuote), "'",
		string(rightSingleCurlyQuote), "'",
		string(leftDoubleCurlyQuote), `"`,
		string(rightDoubleCurlyQuote), `"`,
	)
	return replacer.Replace(s)
}

// findActualString returns the exact substring present in fileContent that
// matches searchString after TS-style curly-quote normalisation.
func findActualString(fileContent, searchString string) (string, bool) {
	if strings.Contains(fileContent, searchString) {
		return searchString, true
	}
	normalizedSearch := normalizeQuotes(searchString)
	fileRunes := []rune(fileContent)
	searchLen := len([]rune(searchString))
	if searchLen == 0 {
		return "", true
	}
	if searchLen > len(fileRunes) {
		return "", false
	}
	for i := 0; i+searchLen <= len(fileRunes); i++ {
		actual := string(fileRunes[i : i+searchLen])
		if normalizeQuotes(actual) == normalizedSearch {
			return actual, true
		}
	}
	return "", false
}

func preserveQuoteStyle(oldString, actualOldString, newString string) string {
	if oldString == actualOldString {
		return newString
	}
	hasDouble := strings.ContainsRune(actualOldString, leftDoubleCurlyQuote) ||
		strings.ContainsRune(actualOldString, rightDoubleCurlyQuote)
	hasSingle := strings.ContainsRune(actualOldString, leftSingleCurlyQuote) ||
		strings.ContainsRune(actualOldString, rightSingleCurlyQuote)
	if !hasDouble && !hasSingle {
		return newString
	}
	result := newString
	if hasDouble {
		result = applyCurlyDoubleQuotes(result)
	}
	if hasSingle {
		result = applyCurlySingleQuotes(result)
	}
	return result
}

func isOpeningQuoteContext(chars []rune, index int) bool {
	if index == 0 {
		return true
	}
	switch chars[index-1] {
	case ' ', '\t', '\n', '\r', '(', '[', '{', '—', '–':
		return true
	default:
		return false
	}
}

func applyCurlyDoubleQuotes(s string) string {
	chars := []rune(s)
	for i, ch := range chars {
		if ch == '"' {
			if isOpeningQuoteContext(chars, i) {
				chars[i] = leftDoubleCurlyQuote
			} else {
				chars[i] = rightDoubleCurlyQuote
			}
		}
	}
	return string(chars)
}

func applyCurlySingleQuotes(s string) string {
	chars := []rune(s)
	for i, ch := range chars {
		if ch != '\'' {
			continue
		}
		var prevIsLetter, nextIsLetter bool
		if i > 0 {
			prevIsLetter = unicode.IsLetter(chars[i-1])
		}
		if i < len(chars)-1 {
			nextIsLetter = unicode.IsLetter(chars[i+1])
		}
		if prevIsLetter && nextIsLetter {
			chars[i] = rightSingleCurlyQuote
		} else if isOpeningQuoteContext(chars, i) {
			chars[i] = leftSingleCurlyQuote
		} else {
			chars[i] = rightSingleCurlyQuote
		}
	}
	return string(chars)
}

// detectLineEnding inspects content and returns "\r\n" if it is predominantly
// CRLF, otherwise "\n". This mirrors the TS detectLineEnding heuristic: count
// CRLF and treat the file as CRLF if at least one is present and they
// outnumber bare LFs.
func detectLineEnding(content string) string {
	if len(content) == 0 {
		return "\n"
	}
	crlf := strings.Count(content, "\r\n")
	if crlf == 0 {
		return "\n"
	}
	totalLF := strings.Count(content, "\n")
	bareLF := totalLF - crlf
	if crlf > bareLF {
		return "\r\n"
	}
	return "\n"
}

// normaliseToLF replaces every \r\n with \n. Used on file content and on the
// caller-supplied oldString/newString so matching is line-ending-agnostic.
func normaliseToLF(s string) string {
	if !strings.Contains(s, "\r\n") {
		return s
	}
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// restoreLineEnding replaces every standalone \n with the supplied ending.
// When ending is "\n" it returns the input unchanged.
func restoreLineEnding(s, ending string) string {
	if ending == "\n" || ending == "" {
		return s
	}
	// Defensive: collapse any \r\n already present so we don't double-up.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", ending)
}
