package compact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/securestore"
	"github.com/agent-dance/luban/types"
)

const (
	maxResultSizeChars          = 50000 // persist results larger than this
	ResultStoreDefaultThreshold = maxResultSizeChars
	previewSizeBytes            = 2000 // keep this much as preview
)

const (
	persistedOutputTag        = "<persisted-output>"
	persistedOutputClosingTag = "</persisted-output>"
)

// ResultStore persists oversized tool results to disk.
type ResultStore struct {
	dir                  string // directory to store results
	mu                   sync.Mutex
	rootIdentity         fs.FileInfo
	rootErr              error
	storageBeforePublish func()
}

// NewResultStore creates a store that persists to the given directory.
func NewResultStore(sessionDir string) *ResultStore {
	abs, absErr := filepath.Abs(sessionDir)
	if absErr != nil {
		return &ResultStore{dir: filepath.Join(sessionDir, "tool-results"), rootErr: absErr}
	}
	dir := filepath.Join(filepath.Clean(abs), "tool-results")
	rs := &ResultStore{dir: dir}
	parent, err := securestore.OpenUnowned(filepath.Clean(abs))
	if err != nil {
		rs.rootErr = err
		return rs
	}
	defer parent.Close()
	child, err := parent.OpenRootExclusive("tool-results")
	if errors.Is(err, fs.ErrExist) {
		child, err = parent.OpenRoot("tool-results", false)
	}
	if err != nil {
		rs.rootErr = err
		return rs
	}
	defer child.Close()
	rs.rootIdentity, rs.rootErr = child.Info()
	return rs
}

// PersistRawOutput writes a tool-owned raw output stream to the store using a
// collision-resistant filename. originalSize reports the untruncated byte
// count; maxBytes limits the persisted copy (zero means unlimited).
func (rs *ResultStore) PersistRawOutput(prefix string, content []byte, maxBytes int64) (path string, originalSize int64, err error) {
	if rs == nil {
		return "", int64(len(content)), i18n.NewError(i18n.KeyCompactResultStoreUnavailable)
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "tool"
	}
	for _, r := range prefix {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			prefix = "tool"
			break
		}
	}
	originalSize = int64(len(content))
	toWrite := content
	if maxBytes > 0 && int64(len(toWrite)) > maxBytes {
		toWrite = toWrite[:maxBytes]
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()
	root, err := rs.openRoot()
	if err != nil {
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCreateRawDirectory, err)
	}
	defer root.Close()
	file, relative, err := root.CreateTemp(".", prefix+"-*.txt")
	if err != nil {
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCreateRawFile, err)
	}
	relative = filepath.Base(relative)
	path = filepath.Join(rs.dir, relative)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = root.Remove(relative)
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCreateRawFile, err)
	}
	if n, err := file.Write(toWrite); err != nil {
		_ = file.Close()
		_ = root.Remove(relative)
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreWriteRawFile, err, path)
	} else if n != len(toWrite) {
		_ = file.Close()
		_ = root.Remove(relative)
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreWriteRawFile, io.ErrShortWrite, path)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = root.Remove(relative)
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreWriteRawFile, err, path)
	}
	writtenInfo, err := validateResultPrivateRegularFile(file, path)
	if err != nil {
		_ = file.Close()
		_ = root.Remove(relative)
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreWriteRawFile, err, path)
	}
	if err := file.Close(); err != nil {
		_ = root.Remove(relative)
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCloseRawFile, err, path)
	}
	if rs.storageBeforePublish != nil {
		rs.storageBeforePublish()
	}
	if err := root.Validate(); err != nil {
		_ = root.Remove(relative)
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCloseRawFile, err, path)
	}
	published, err := root.OpenFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCloseRawFile, err, path)
	}
	publishedInfo, validateErr := validateResultPrivateRegularFile(published, path)
	if validateErr == nil && !os.SameFile(writtenInfo, publishedInfo) {
		validateErr = fs.ErrInvalid
	}
	closeErr := published.Close()
	if validateErr != nil {
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCloseRawFile, validateErr, path)
	}
	if closeErr != nil {
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCloseRawFile, closeErr, path)
	}
	if err := root.Sync("."); err != nil {
		return "", originalSize, i18n.WrapError(i18n.KeyCompactResultStoreCloseRawFile, err, path)
	}
	return path, originalSize, nil
}

// BuildPersistedOutputMessage returns the model-facing wrapper used for a raw
// output file while keeping the preview bounded.
func BuildPersistedOutputMessage(path string, originalSize int64, previewSource string) string {
	preview, hasMore := generatePreview(previewSource, previewSizeBytes)
	return buildLargeToolResultMessage(path, int(originalSize), preview, hasMore)
}

// ProcessResult checks if a tool result is oversized and persists it if so.
// Returns the (possibly modified) result.
func (rs *ResultStore) ProcessResult(result types.ToolResultBlock) types.ToolResultBlock {
	processed, _ := rs.ProcessResultForTool(result, "")
	return processed
}

// ProcessResultForTool checks if a tool result should be normalized or
// persisted for the given tool. On persistence failure, it returns the original
// content unchanged with a filesystem error useful to logs/tests.
func (rs *ResultStore) ProcessResultForTool(result types.ToolResultBlock, toolName string) (types.ToolResultBlock, error) {
	if isToolResultContentEmpty(result) {
		return NormalizeEmptyToolResult(result, toolName), nil
	}
	if result.IsError {
		return result, nil // don't persist error results
	}
	if hasNonTextStructuredContent(result) {
		return result, nil
	}

	content, isJSON, ok, err := persistableContent(result)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, nil
	}

	threshold, persist := persistenceThreshold(result.Metadata)
	if !persist || len(content) <= threshold {
		return result, nil
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()
	root, err := rs.openRoot()
	if err != nil {
		return result, i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, rs.dir)
	}
	defer root.Close()
	if err := validateToolUseID(result.ToolUseID); err != nil {
		return result, i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, rs.dir)
	}

	ext := "txt"
	if isJSON {
		ext = "json"
	}
	path := filepath.Join(rs.dir, result.ToolUseID+"."+ext)
	if err := rs.writeCreateOnce(root, filepath.Base(path), path, []byte(content)); err != nil {
		return result, err
	}

	previewContent := content
	preview, hasMore := generatePreview(previewContent, previewSizeBytes)

	result.Content = buildLargeToolResultMessage(path, len(previewContent), preview, hasMore)
	result.ContentBlocks = nil

	return result, nil
}

// NormalizeEmptyToolResult gives tool results with no model-visible output the
// same marker as the original client. It is independent of ResultStore so
// runtimes without persistence, including subagents, share the same behavior.
func NormalizeEmptyToolResult(result types.ToolResultBlock, toolName string) types.ToolResultBlock {
	if !isToolResultContentEmpty(result) {
		return result
	}
	result.Content = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuxCompactEmptyToolResult, emptyMarkerToolName(toolName))
	result.ContentBlocks = nil
	return result
}

// PersistReplacement writes the full tool result content and returns the exact
// replacement text that should be shown to the model. It is used by the
// stateful aggregate per-message budget, which has already selected this
// result for replacement and therefore bypasses the per-result threshold.
func (rs *ResultStore) PersistReplacement(toolUseID, content string) (string, error) {
	if rs == nil {
		return "", i18n.NewError(i18n.KeyCompactResultStoreUnavailable)
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	root, err := rs.openRoot()
	if err != nil {
		return "", i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, rs.dir)
	}
	defer root.Close()
	if err := validateToolUseID(toolUseID); err != nil {
		return "", i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, rs.dir)
	}

	path := filepath.Join(rs.dir, toolUseID+".txt")
	if err := rs.writeCreateOnce(root, filepath.Base(path), path, []byte(content)); err != nil {
		return "", err
	}

	previewContent := content
	preview, hasMore := generatePreview(previewContent, previewSizeBytes)
	return buildLargeToolResultMessage(path, len(previewContent), preview, hasMore), nil
}

func (rs *ResultStore) writeCreateOnce(root *securestore.Root, name, path string, data []byte) error {
	if _, err := root.Lstat(name); err == nil {
		return verifyExistingPrivateResult(root, name, path, data)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}

	file, tmpName, err := root.CreateTemp(".", ".tool-result-*")
	if err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	tmpName = filepath.Base(tmpName)
	defer func() {
		_ = file.Close()
		_ = root.Remove(tmpName)
	}()
	if err := file.Chmod(0o600); err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	if n, err := file.Write(data); err != nil {
		_ = file.Close()
		return i18n.WrapError(i18n.KeyCompactResultStoreWriteResultFile, err, path)
	} else if n != len(data) {
		return i18n.WrapError(i18n.KeyCompactResultStoreWriteResultFile, io.ErrShortWrite, path)
	}
	if err := file.Sync(); err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreWriteResultFile, err, path)
	}
	tmpInfo, err := validateResultPrivateRegularFile(file, root.Path(tmpName))
	if err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreWriteResultFile, err, path)
	}
	if err := file.Close(); err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCloseResultFile, err, path)
	}
	if rs.storageBeforePublish != nil {
		rs.storageBeforePublish()
	}
	if err := root.Validate(); err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	publishedNew := false
	if err := root.Link(tmpName, name); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return verifyExistingPrivateResult(root, name, path, data)
		}
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	publishedNew = true
	published, err := root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	publishedInfo, statErr := published.Stat()
	closePublishedErr := published.Close()
	if statErr != nil || publishedInfo == nil || !publishedInfo.Mode().IsRegular() || !os.SameFile(tmpInfo, publishedInfo) {
		if statErr != nil {
			return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, statErr, path)
		}
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, fs.ErrInvalid, path)
	}
	if closePublishedErr != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCloseResultFile, closePublishedErr, path)
	}
	if err := root.Remove(tmpName); err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCloseResultFile, err, path)
	}
	if publishedNew {
		final, openErr := root.OpenFile(name, os.O_RDONLY, 0)
		if openErr != nil {
			return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, openErr, path)
		}
		finalInfo, validateErr := validateResultPrivateRegularFile(final, path)
		if validateErr == nil && !os.SameFile(tmpInfo, finalInfo) {
			validateErr = fs.ErrInvalid
		}
		closeErr := final.Close()
		if validateErr != nil {
			return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, validateErr, path)
		}
		if closeErr != nil {
			return i18n.WrapError(i18n.KeyCompactResultStoreCloseResultFile, closeErr, path)
		}
	}
	if err := root.Sync("."); err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCloseResultFile, err, path)
	}
	return nil
}

const maxToolUseIDBytes = 200

func validateToolUseID(id string) error {
	if id == "" || len(id) > maxToolUseIDBytes {
		return fs.ErrInvalid
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fs.ErrInvalid
	}
	return nil
}

func (rs *ResultStore) openRoot() (*securestore.Root, error) {
	if rs == nil || strings.TrimSpace(rs.dir) == "" {
		return nil, fs.ErrInvalid
	}
	if rs.rootErr != nil {
		return nil, rs.rootErr
	}
	root, err := securestore.Open(rs.dir, false)
	if err != nil {
		return nil, err
	}
	current, err := root.Info()
	if err != nil || rs.rootIdentity == nil || !os.SameFile(rs.rootIdentity, current) {
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		return nil, fs.ErrInvalid
	}
	return root, nil
}

func ensureResultPrivateDirectory(path string) error {
	if isResultVolumeRoot(path) {
		return fs.ErrInvalid
	}
	f, err := openResultPrivateDirectory(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		f, err = openResultPrivateDirectory(path)
	}
	if err != nil {
		return err
	}
	return f.Close()
}

func openResultPrivateDirectory(path string) (*os.File, error) {
	f, err := openResultPathNoFollow(path, os.O_RDONLY, 0, true)
	if err != nil {
		return nil, err
	}
	before, err := f.Stat()
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	if err := f.Chmod(0o700); err != nil {
		_ = f.Close()
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !after.IsDir() || !os.SameFile(before, after) {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return f, nil
}

func isResultVolumeRoot(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	volume := filepath.VolumeName(abs)
	return filepath.Clean(abs) == filepath.Clean(volume+string(os.PathSeparator))
}

func openResultPrivateRegularFile(path string) (*os.File, error) {
	f, err := openResultPathNoFollow(path, os.O_RDONLY, 0, false)
	if err != nil {
		return nil, err
	}
	before, err := f.Stat()
	if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	if err := validateResultPrivateRegularFileLinkCount(path, before); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	if err := validateResultPrivateRegularFileLinkCount(path, after); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func verifyExistingPrivateResult(root *securestore.Root, name, path string, want []byte) error {
	f, err := root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	defer f.Close()
	info, err := validateResultPrivateRegularFile(f, path)
	if err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	if info.Size() != int64(len(want)) {
		mismatch := &os.PathError{Op: "open", Path: path, Err: fs.ErrExist}
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, mismatch, path)
	}
	got, err := io.ReadAll(io.LimitReader(f, int64(len(want))+1))
	if err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	after, err := f.Stat()
	if err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	if !after.Mode().IsRegular() || !os.SameFile(info, after) {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}, path)
	}
	if err := validateResultPrivateRegularFileLinkCount(path, after); err != nil {
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, err, path)
	}
	if !bytes.Equal(got, want) {
		mismatch := &os.PathError{Op: "open", Path: path, Err: fs.ErrExist}
		return i18n.WrapError(i18n.KeyCompactResultStoreCreateResultFile, mismatch, path)
	}
	return nil
}

func validateResultPrivateRegularFile(file *os.File, path string) (fs.FileInfo, error) {
	before, err := file.Stat()
	if err != nil || before == nil || !before.Mode().IsRegular() {
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	if err := validateResultPrivateRegularFileLinkCount(path, before); err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || after == nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	if err := validateResultPrivateRegularFileLinkCount(path, after); err != nil {
		return nil, err
	}
	return after, nil
}

func syncResultPrivateDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func persistableContent(result types.ToolResultBlock) (content string, isJSON bool, ok bool, err error) {
	if !result.HasStructuredContent() {
		return result.Content, false, true, nil
	}
	b, err := json.MarshalIndent(result.ContentBlocks, "", "  ")
	if err != nil {
		return "", false, false, i18n.WrapError(i18n.KeyCompactResultStoreSerializeStructured, err, result.ToolUseID)
	}
	return string(b), true, true, nil
}

func hasNonTextStructuredContent(result types.ToolResultBlock) bool {
	if !result.HasStructuredContent() {
		return false
	}
	for _, block := range result.ContentBlocks {
		if block.GetType() != types.ContentTypeText {
			return true
		}
	}
	return false
}

func isToolResultContentEmpty(result types.ToolResultBlock) bool {
	if !result.HasStructuredContent() {
		return strings.TrimSpace(result.Content) == ""
	}
	if len(result.ContentBlocks) == 0 {
		return true
	}
	for _, block := range result.ContentBlocks {
		text, ok := block.(types.TextBlock)
		if !ok {
			return false
		}
		if strings.TrimSpace(text.Text) != "" {
			return false
		}
	}
	return true
}

func emptyMarkerToolName(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "tool"
	}
	return toolName
}

func persistenceThreshold(metadata map[string]string) (threshold int, persist bool) {
	threshold = maxResultSizeChars
	persist = true
	for _, key := range []string{
		"maxResultSizeChars",
		"max_result_size_chars",
		"persistenceThreshold",
		"persistThreshold",
		"toolResultPersistenceThreshold",
	} {
		raw, ok := metadata[key]
		if !ok {
			continue
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		switch strings.ToLower(raw) {
		case "inf", "+inf", "infinity", "+infinity", "unbounded", "max", "none", "off", "false", "disabled":
			return threshold, false
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			continue
		}
		if n > maxResultSizeChars {
			return maxResultSizeChars, true
		}
		return n, true
	}
	return threshold, true
}

func generatePreview(content string, maxBytes int) (string, bool) {
	if len(content) <= maxBytes {
		return content, false
	}
	truncated := content[:maxBytes]
	lastNewline := strings.LastIndex(truncated, "\n")
	cutPoint := maxBytes
	if lastNewline > maxBytes/2 {
		cutPoint = lastNewline
	} else if lastNewline == -1 && maxBytes > 256 {
		cutPoint = 256
	}
	return content[:cutPoint], true
}

func buildLargeToolResultMessage(path string, originalSize int, preview string, hasMore bool) string {
	lang := i18n.DetectOrLoadLanguage()
	var b strings.Builder
	b.WriteString(persistedOutputTag)
	b.WriteByte('\n')
	b.WriteString(i18n.Format(lang, i18n.KeyAuxCompactOutputTooLarge, formatFileSize(lang, originalSize), path))
	b.WriteString(i18n.Format(lang, i18n.KeyAuxCompactPreview, formatFileSize(lang, previewSizeBytes)))
	b.WriteString(preview)
	if hasMore {
		b.WriteString("\n...\n")
	} else {
		b.WriteByte('\n')
	}
	b.WriteString(persistedOutputClosingTag)
	return b.String()
}

func formatFileSize(lang i18n.Language, size int) string {
	kb := float64(size) / 1024
	if kb < 1 {
		return i18n.Format(lang, i18n.KeyAuxCompactBytes, size)
	}
	if kb < 1024 {
		return trimTrailingZero(kb) + "KB"
	}
	mb := kb / 1024
	if mb < 1024 {
		return trimTrailingZero(mb) + "MB"
	}
	gb := mb / 1024
	return trimTrailingZero(gb) + "GB"
}

func trimTrailingZero(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}
