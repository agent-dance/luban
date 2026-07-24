package compact

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	// PostCompactMaxFiles is the maximum number of files to recover after compaction.
	PostCompactMaxFiles = 5

	// PostCompactTokenBudget is the total token budget for all recovered files.
	PostCompactTokenBudget = 50000

	// PostCompactPerFileBudget is the per-file token budget (matches TS POST_COMPACT_MAX_TOKENS_PER_FILE = 5000).
	PostCompactPerFileBudget = 5000

	// PostCompactBytesPerToken is the bytes-per-token ratio used for budget estimation.
	// Named "Bytes" (not "Chars") because Go's len(string) returns byte count.
	PostCompactBytesPerToken = 4

	// maxReadSize is the absolute upper bound for file reads (1MB) to prevent OOM.
	maxReadSize = 1 * 1024 * 1024
)

// toolNamesForFileRecovery is the set of tool names whose file_path inputs we track.
// Only "Read" is tracked (matching TS behavior where readFileState is only written by FileReadTool).
// Edit/Write files may have been modified since — recovering old content could confuse the model.
var toolNamesForFileRecovery = map[string]bool{
	"Read": true,
}

// excludedFilePatterns contains path patterns that should be excluded from recovery.
// These files are typically already present in the system prompt or have special handling.
var excludedFilePatterns = []string{
	"LUBAN.md",
	".luban-code/",
	"DEEPSEEK.md",
	"AGENTS.md",
	".deepseek-code/",
	"CLAUDE.md",
	"claude.md",
	".claude/",
	"session_memory.md",
}

// shouldExcludeFromRecovery checks if a file path should be excluded from
// post-compaction recovery (plan files, memory files, CLAUDE.md, etc.).
func shouldExcludeFromRecovery(path string) bool {
	for _, pattern := range excludedFilePatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}

// ExtractRecentFiles scans messages for recently and successfully read file
// paths. A candidate requires one uniquely paired Read ToolUseBlock and one
// later, non-error ToolResultBlock with an explicit succeeded outcome. An
// absent legacy outcome is not execution evidence and therefore fails closed.
// Returns unique paths in reverse chronological order (most recent first),
// capped at PostCompactMaxFiles entries.
// Paths that match excludedFilePatterns are skipped.
// The excludeAlreadyVisible set contains paths already present in preserved messages
// that should not be re-injected.
func ExtractRecentFiles(messages []types.Message, excludeAlreadyVisible map[string]bool) []string {
	type readUse struct {
		id, path string
		index    int
	}
	type readResult struct {
		index   int
		success bool
	}

	var uses []readUse
	useCounts := make(map[string]int)
	results := make(map[string][]readResult)
	for index, msg := range messages {
		if msg.Role == types.RoleAssistant && !msg.IsInternalRuntimeMessage() {
			for _, block := range msg.Content {
				tu, ok := block.(types.ToolUseBlock)
				if !ok || !toolNamesForFileRecovery[tu.Name] || tu.ID == "" {
					continue
				}
				path, ok := tu.Input["file_path"].(string)
				if !ok || strings.TrimSpace(path) == "" {
					continue
				}
				path = filepath.Clean(path)
				uses = append(uses, readUse{id: tu.ID, path: path, index: index})
				useCounts[tu.ID]++
			}
		}
		if msg.Role != types.RoleUser || msg.IsInternalRuntimeMessage() {
			continue
		}
		for _, block := range msg.Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok || result.ToolUseID == "" {
				continue
			}
			results[result.ToolUseID] = append(results[result.ToolUseID], readResult{
				index:   index,
				success: !result.IsError && result.Outcome == types.ToolOutcomeSucceeded,
			})
		}
	}

	visible := make(map[string]bool, len(excludeAlreadyVisible))
	for path, present := range excludeAlreadyVisible {
		if present && strings.TrimSpace(path) != "" {
			visible[filepath.Clean(path)] = true
		}
	}
	seenPaths := make(map[string]bool)
	recent := make([]string, 0, min(PostCompactMaxFiles, len(uses)))
	for index := len(uses) - 1; index >= 0; index-- {
		use := uses[index]
		if seenPaths[use.path] {
			continue
		}
		// The latest attempt owns the path verdict. Mark it seen before checking
		// evidence so a newer denial/failure cannot fall back to an older success.
		seenPaths[use.path] = true
		paired := results[use.id]
		if useCounts[use.id] != 1 || len(paired) != 1 || !paired[0].success || paired[0].index <= use.index {
			continue
		}
		if shouldExcludeFromRecovery(use.path) || visible[use.path] {
			continue
		}
		recent = append(recent, use.path)
		if len(recent) >= PostCompactMaxFiles {
			break
		}
	}
	return recent
}

// IsReinjectedAttachment reports whether msg is an automatic recovery
// attachment that should not be summarized again during a later compaction.
// The actual post-compact context may still keep recent automatic messages
// verbatim; this predicate is only for summarization input cleanup.
func IsReinjectedAttachment(msg types.Message) bool {
	if msg.Role != types.RoleUser || !msg.HasInternalControlProvenance() {
		return false
	}
	switch msg.InternalKind {
	case types.InternalMessageKindCompactFileRecovery:
		return msg.ID == postCompactFileRecoveryMessageID
	case types.InternalMessageKindCompactReminder:
		return isPostCompactReminderMessage(msg)
	default:
		return false
	}
}

// IsLegacyInvokedSkillsAttachment reports whether msg is the old name-only
// post-compact skill reminder. A name and tool-use ID are not evidence that a
// SKILL.md body remains model-visible, so the live catalog/body restoration
// path must never use this reminder to rebuild its loaded-body ledger.
func IsLegacyInvokedSkillsAttachment(msg types.Message) bool {
	return msg.Role == types.RoleUser && msg.HasInternalControlProvenance() &&
		isPostCompactReminderMessageFor(msg, i18n.KeyCompactAttachmentSkillsTitle)
}

// StripReinjectedAttachments removes automatic post-compact/listing
// attachments before summarization so compact summaries do not recursively
// describe context that will be regenerated by providers.
func StripReinjectedAttachments(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		if IsReinjectedAttachment(msg) {
			continue
		}
		out = append(out, msg)
	}
	if len(out) == len(messages) {
		return messages
	}
	return out
}

// extractFilePathsFromMessages extracts all file_path values from Read tool uses
// in the given messages. Used to find files already visible in preserved messages.
func extractFilePathsFromMessages(messages []types.Message) map[string]bool {
	paths := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role != types.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			tu, ok := block.(types.ToolUseBlock)
			if !ok {
				continue
			}
			if tu.Name != "Read" {
				continue
			}
			if pathVal, ok := tu.Input["file_path"]; ok {
				if path, ok := pathVal.(string); ok && path != "" {
					paths[path] = true
				}
			}
		}
	}
	return paths
}

// RecoverFiles reads the given file paths and returns user messages containing
// the file contents, respecting per-file and total byte budgets.
// Files that don't exist, can't be read, are too large, or escape every allowed
// root are silently skipped.
// Content is truncated at the nearest preceding newline to avoid breaking lines,
// and validated for UTF-8 safety.
// The ctx parameter allows cancellation during I/O.
// allowedDirs restricts recovery to paths inside one of the given directories.
// An empty or invalid allow-list fails closed and disables automatic recovery.
func RecoverFiles(ctx context.Context, paths []string, maxFiles, totalBudgetBytes, perFileBudgetBytes int, allowedDirs ...string) []types.Message {
	if len(allowedDirs) == 0 {
		return nil
	}
	lang := i18n.DetectOrLoadLanguage()
	var result []types.Message
	remainingTotal := totalBudgetBytes

	for _, path := range paths {
		if ctx.Err() != nil {
			break
		}
		if len(result) >= maxFiles {
			break
		}
		if remainingTotal <= 0 {
			break
		}

		// Open relative to an anchored os.Root. Root.Open follows only symlinks
		// that remain inside that root, closing the validation/reopen race.
		file, err := openRecoveryFile(path, allowedDirs)
		if err != nil {
			continue
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			_ = file.Close()
			continue
		}
		fileSize := info.Size()
		if fileSize == 0 || fileSize > maxReadSize {
			_ = file.Close()
			continue
		}

		// Read with size limit
		limit := perFileBudgetBytes
		if limit > remainingTotal {
			limit = remainingTotal
		}

		content, err := readFileLimited(file, limit)
		_ = file.Close()
		if err != nil || content == "" {
			continue
		}

		// Build the full message body and account for header overhead in budget.
		msg := newPostCompactFileRecoveryMessage(lang, path, content)
		result = append(result, msg)
		remainingTotal -= len(msg.GetText()) // account for header + content
	}

	return result
}

// readFileLimited reads up to limitBytes from the file, truncating at a
// newline boundary. Ensures the result is valid UTF-8.
func readFileLimited(f *os.File, limitBytes int) (string, error) {
	// Read at most limitBytes + a small margin for newline scan
	buf := make([]byte, min(limitBytes+1, maxReadSize))
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	data := buf[:n]

	// If we read more than the limit, truncate
	if len(data) > limitBytes {
		data = data[:limitBytes]
		// Truncate at nearest preceding newline
		if idx := lastIndexByte(data, '\n'); idx >= 0 {
			data = data[:idx+1]
		}
	}

	// Ensure valid UTF-8: trim trailing incomplete multi-byte sequence
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}

	return string(data), nil
}

// lastIndexByte returns the index of the last occurrence of c in s, or -1.
func lastIndexByte(s []byte, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// openRecoveryFile opens path through the first configured root that contains
// it lexically. os.Root then enforces that symlink traversal cannot escape the
// anchored directory while the returned file descriptor is in use.
func openRecoveryFile(path string, allowedDirs []string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, os.ErrPermission
	}
	path = filepath.Clean(path)
	for _, allowedDir := range allowedDirs {
		if strings.TrimSpace(allowedDir) == "" || !filepath.IsAbs(allowedDir) {
			continue
		}
		rootPath := filepath.Clean(allowedDir)
		relative, err := filepath.Rel(rootPath, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			continue
		}
		file, openErr := root.Open(relative)
		closeErr := root.Close()
		if openErr == nil && closeErr == nil {
			return file, nil
		}
		if file != nil {
			_ = file.Close()
		}
	}
	return nil, os.ErrPermission
}
