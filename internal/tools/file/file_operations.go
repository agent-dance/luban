package file

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

// checkAllowedPath verifies that the resolved path falls under one of the
// allowed directories. It resolves symlinks before checking to prevent
// symlink-based escapes.
func checkAllowedPath(path string, allowedDirs []string) error {
	if len(allowedDirs) == 0 {
		return nil // no restrictions
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return i18n.WrapError(i18n.KeyToolFileResolveFailed, err)
	}
	resolved, err := resolveAllowedPath(absPath)
	if err != nil {
		return err
	}
	resolved = toolbase.CanonicalPath(resolved)
	for _, dir := range allowedDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if rd, err := filepath.EvalSymlinks(absDir); err == nil {
			absDir = rd
		}
		absDir = toolbase.CanonicalPath(absDir)
		if strings.HasPrefix(resolved, absDir+string(os.PathSeparator)) || resolved == absDir {
			return nil
		}
	}
	return i18n.NewError(i18n.KeyToolPathOutsideAllowed, path)
}

func fileReadNotFoundRuntimeError(filePath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return i18n.NewError(i18n.KeyToolFileNotFound, filePath)
	}
	if suggestion := toolbase.SuggestNearbyPath(filePath, cwd); suggestion != "" {
		return i18n.NewError(i18n.KeyToolFileNotFoundSuggestion, cwd, suggestion)
	}
	return i18n.NewError(i18n.KeyToolFileNotFoundInCWD, cwd)
}

func fileTooLargeRuntimeError(size, maximum int64) error {
	return i18n.NewError(i18n.KeyToolFileTooLarge, formatReadSize(size), formatReadSize(maximum))
}

// resolveAllowedPath resolves every existing symlink component while allowing
// one or more missing trailing components. File creation commonly targets a
// not-yet-created nested directory, so resolving only the immediate parent
// incorrectly rejects safe paths such as <allowed>/.luban-code/settings.json.
func resolveAllowedPath(absPath string) (string, error) {
	current := filepath.Clean(absPath)
	missing := make([]string, 0, 4)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", i18n.WrapError(i18n.KeyToolFileResolveFailed, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", i18n.WrapError(i18n.KeyToolFileResolveFailed, err)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// ── FileReadTool ─────────────────────────────────────────────────────────────

// FileReadTool reads file contents with TOCTOU-safe fd verification.
//
// Mirrors src/tools/FileReadTool/FileReadTool.ts:
//   - ReadState is the shared read-file state map; eligible text/notebook reads
//     are recorded so Edit/Write can detect stale reads and exact repeats can
//     return file_unchanged. Image/PDF reads are intentionally not recorded.
//   - SkillManager (optional) auto-activates skills after a dedup miss and
//     before the file operation begins.
type FileReadTool struct {
	AllowedDirs []string
	// Runtime is sampled on each call so cwd, allowed directories, permission
	// rules, and active model follow session/worktree/provider changes.
	Runtime types.ToolRuntimeContextProvider
	// ReadState is the shared (path → entry) map. A nil state gives the
	// current Read invocation an isolated ledger.
	ReadState *ReadFileState

	// SkillManager is the optional skill registry that gets nearby skill
	// directories activated after a successful read. If nil, no skills are
	// activated.
	SkillManager FileReadSkillActivator

	// PreciseTokenCounter is sampled only when the cheap estimate crosses the
	// TS near-limit threshold. Provider-backed runtimes can wire their token API;
	// standalone tools fall back to the local tokenizer on error or absence.
	PreciseTokenCounter func(context.Context, string) (int, error)

	// ToolResultsDirProvider returns the persistent directory used for extracted
	// PDF page artifacts. It is sampled per call so worktree cwd changes apply.
	ToolResultsDirProvider func() string

	// digestAfterOpenForTest is a deterministic test seam for a pathname swap
	// after dedup opens its descriptor. Production constructors leave it nil.
	digestAfterOpenForTest func()
}

// FileReadSkillActivator narrows what FileReadTool needs from a skill
// manager so we can mock it in tests and avoid pulling the skills package
// into a circular import.
type FileReadSkillActivator interface {
	AddDirectoriesAtGeneration(uint64, []string) error
	ActivateConditionalForPathAtGeneration(uint64, string) error
}

func skillProjectGenerationFromContext(ctx context.Context) (uint64, bool) {
	if ctx == nil {
		return 0, false
	}
	exec, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return 0, false
	}
	generation, pinned := exec.SkillProjectGeneration()
	return generation, pinned
}

func addSkillDirectoriesForExecution(ctx context.Context, manager FileReadSkillActivator, dirs []string) {
	if manager == nil || len(dirs) == 0 {
		return
	}
	if generation, pinned := skillProjectGenerationFromContext(ctx); pinned {
		_ = manager.AddDirectoriesAtGeneration(generation, dirs)
	}
}

func activateConditionalPathForExecution(ctx context.Context, manager FileReadSkillActivator, absPath string) {
	if manager == nil || absPath == "" {
		return
	}
	if generation, pinned := skillProjectGenerationFromContext(ctx); pinned {
		_ = manager.ActivateConditionalForPathAtGeneration(generation, absPath)
	}
}

// readState returns the bound state or an invocation-local ledger.
func (t *FileReadTool) readState() *ReadFileState {
	if t == nil || t.ReadState == nil {
		return NewReadFileState()
	}
	return t.ReadState
}

func (t *FileReadTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileReadTool) Name() string        { return "Read" }
func (t *FileReadTool) Description() string { return toolPromptText(i18n.KeyToolFileReadDescription) }

func (t *FileReadTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: types.UnlimitedToolResultSize}
}

func (t *FileReadTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	rawPath, _ := input["file_path"].(string)
	baseDir := strings.TrimSpace(request.Runtime.ProjectRoot)
	if baseDir == "" {
		baseDir = t.readBaseDir()
	}
	path, err := expandReadPath(rawPath, baseDir)
	if err != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionText(i18n.KeyToolPermissionInvalidPath), Required: true}, nil
	}
	updated := cloneToolInput(input)
	updated["file_path"] = path
	if rule, ok := matchingReadPathRule(path, request.Runtime.DeniedRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorDeny,
			Message:     toolPermissionText(i18n.KeyToolPermissionReadDenied),
			BlockedPath: rule.RuleContent,
			Required:    true,
		}, nil
	}
	if rule, ok := matchingReadPathRule(path, request.Runtime.AskRules); ok {
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorAsk,
			Message:     toolPermissionFormat(i18n.KeyToolPermissionReadRequired, path),
			BlockedPath: rule.RuleContent,
			Required:    true,
		}, nil
	}
	// Do not stat/open UNC paths before the interactive permission lifecycle;
	// probing them can leak Windows credentials.
	if isUNCPath(path) || isUNCPath(strings.TrimSpace(rawPath)) {
		uncPath := path
		if raw := strings.TrimSpace(rawPath); isUNCPath(raw) {
			uncPath = raw
		}
		return types.ToolPermissionResult{
			Behavior:    types.PermissionBehaviorAsk,
			Message:     toolPermissionFormat(i18n.KeyToolPermissionReadUNC, uncPath),
			BlockedPath: uncPath,
			Required:    true,
		}, nil
	}
	ext := strings.ToLower(filepath.Ext(path))
	if hasBinaryExtension(path) && ext != ".pdf" && !imageExtensions[ext] {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionFormat(i18n.KeyToolPermissionReadBinary, ext), Required: true}, nil
	}
	if isBlockedDevicePath(filepath.Clean(path)) {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionFormat(i18n.KeyToolPermissionReadDevice, rawPath), Required: true}, nil
	}
	allowed := request.Runtime.AllowedDirs
	if allowed == nil {
		allowed = t.readAllowedDirs()
	}
	if len(allowed) == 0 || checkAllowedPath(path, allowed) == nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: updated}, nil
	}
	return outsideDirectoryPermission(path, "Read"), nil
}

func (t *FileReadTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolFileReadInputFilePathDescription),
			},
			"offset": toolbase.SemanticNumber(toolPromptText(i18n.KeyToolFileReadInputOffsetDescription), 1, true),
			"limit":  toolbase.SemanticNumber(toolPromptText(i18n.KeyToolFileReadInputLimitDescription), 0, true),
			"pages": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolFileReadInputPagesDescription),
			},
		},
		"file_path",
	)
}

func (t *FileReadTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, decodeErr := types.DecodeStrictToolInput[FileReadInput](input)
	if decodeErr != nil {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileInvalidInput, decodeErr)), nil
	}
	if in.Offset < 0 || in.Offset != float64(int(in.Offset)) {
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFileOffsetNonNegative)), nil
	}
	if _, specified := input["offset"]; specified && in.Offset == 0 {
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFileOffsetNonNegative)), nil
	}
	if in.Limit < 0 || in.Limit != float64(int(in.Limit)) {
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFileLimitNonNegative)), nil
	}
	if _, ok := input["limit"]; ok && int(in.Limit) == 0 {
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFileLimitPositive)), nil
	}

	// fr-pages-validation-pre-io: validate the pages selector BEFORE any
	// disk I/O so a malformed range surfaces "invalid pages parameter"
	// rather than "file not found" when the path is missing. Mirrors TS
	// validateInput in FileReadTool.
	if in.Pages != "" {
		set, ok := parsePDFPageSelector(in.Pages)
		if !ok {
			return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFilePagesInvalid, in.Pages)), nil
		}
		if set.OpenEnded || len(set.Pages) > pdfMaxPagesPerRead {
			return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFilePageRangeTooLarge, in.Pages, pdfMaxPagesPerRead)), nil
		}
	}

	// Expand and validate path without filesystem access. Permission rules and
	// UNC handling must run before any stat/open to avoid observable bypasses
	// and Windows credential probes.
	rawFilePath := in.FilePath
	filePath, pathErr := t.expandPath(rawFilePath)
	if pathErr != nil {
		return errorResponse(pathErr), nil
	}
	in.FilePath = filePath
	input = cloneToolInput(input)
	input["file_path"] = filePath
	runtimeSnapshot := t.runtimeSnapshot()
	if rule, denied := matchingReadPathRule(filePath, runtimeSnapshot.DeniedRules); denied {
		_ = rule
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFileDirectoryDenied)), nil
	}
	if isUNCPath(filePath) || isUNCPath(strings.TrimSpace(rawFilePath)) {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileUNCRequiresPermission, filePath)), nil
	}
	normalizedPath := filepath.Clean(filePath)
	if isBlockedDevicePath(normalizedPath) {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileDeviceBlocked, filePath)), nil
	}
	ext := strings.ToLower(filepath.Ext(normalizedPath))
	if hasBinaryExtension(normalizedPath) && ext != ".pdf" && !imageExtensions[ext] {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileBinaryUnsupported, ext)), nil
	}
	allowedDirs := t.readAllowedDirs()
	if err := checkAllowedPath(filePath, allowedDirs); err != nil {
		return errorResponse(err), nil
	}
	// The screenshot alternate path probe is safe only after permission checks.
	filePath = normalizeReadFilePath(filePath)
	normalizedPath = filepath.Clean(filePath)
	limits := resolveFileReadingLimits()

	// file_unchanged dedup. If a previous Read at the same offset/limit and
	// the file's mtime hasn't moved, return the stub immediately. The model
	// already has the content in context.
	requestedOffset := 1
	_, offsetSpecified := input["offset"]
	if offsetSpecified {
		requestedOffset = int(in.Offset)
	}
	requestedLimit := 0
	_, limitSpecified := input["limit"]
	if limitSpecified {
		requestedLimit = int(in.Limit)
	}
	state := t.readState()
	if ext == ".ipynb" || (!imageExtensions[ext] && ext != ".pdf") {
		if statInfo, err := os.Stat(filePath); err == nil && !statInfo.IsDir() {
			if entry, ok := state.GetForContext(ctx, normalizedPath); ok {
				if entry.LastTool == "Read" &&
					entry.DedupEligible &&
					!entry.IsPartialView &&
					entry.ContentDigest != "" &&
					readEntryMatchesFileIdentity(entry, statInfo) &&
					entry.Offset == requestedOffset &&
					entry.Limit == requestedLimit &&
					readEntryMatchesModTime(entry, statInfo.ModTime()) {
					currentDigest, _, digestErr := digestFileAtPathWithHook(filePath, statInfo, t.digestAfterOpenForTest)
					if digestErr == nil && currentDigest == entry.ContentDigest {
						dedup := newFileUnchangedResult(filePath)
						return dedup, nil
					}
				}
			}
		}
	}

	// TS performs skill discovery on a dedup miss before inner file work.
	t.discoverSkillsBeforeRead(ctx, normalizedPath)

	// Open file, then verify fd to close TOCTOU window
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return errorResponse(fileReadNotFoundRuntimeError(filePath)), nil
		}
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileOpenFailed, err)), nil
	}
	defer f.Close()

	if err := verifyOpenFd(f, allowedDirs); err != nil {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFilePathVerification, err)), nil
	}

	// Capture mtime once (post-open) for the state entry. We read mtime
	// from the open fd to close the TOCTOU window.
	openedInfo, statErr := f.Stat()
	if statErr != nil {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileReadFailed, statErr)), nil
	}
	if ext == ".ipynb" {
		return t.readNotebookFromOpenFile(
			ctx, f, openedInfo, filePath, normalizedPath, state, limits,
			requestedOffset, requestedLimit, offsetSpecified, limitSpecified,
		), nil
	}

	// Encoding detection: peek the first 8KB to classify text encoding.
	// For non-UTF-8 text files (UTF-16, Latin-1) we decode here and bypass
	// the line-streaming path, since the streaming reader assumes UTF-8.
	// Mirrors TS readFileSyncWithMetadata which threads encoding through
	// the result and lets FileWrite preserve it on overwrite.
	encoding := FileEncoding("")
	var encodingBOM []byte
	if !imageExtensions[ext] && ext != ".pdf" {
		if peek := peekOpenFileHead(f, 8192+utf8.UTFMax-1); len(peek) > 0 {
			det := detectFileEncoding(peek)
			encoding = det.Encoding
			encodingBOM = det.BOM
			// Non-UTF-8 text: full-read + decode path, then format like the
			// regular text path (numbered lines).
			if det.Encoding == EncodingUTF16LE || det.Encoding == EncodingUTF16BE || det.Encoding == EncodingLatin1 {
				return t.readNonUTF8TextFromOpenFile(ctx, f, openedInfo, filePath, normalizedPath, in, input, det, limits)
			}
		}
	}

	if richResult, handled, err := t.executeRichRead(ctx, filePath, in, limits); handled || err != nil {
		return richResult, err
	}

	startLine := requestedOffset
	zeroBasedOffset := startLine
	if startLine > 0 {
		zeroBasedOffset = startLine - 1
	}

	var limit *int
	if _, ok := input["limit"]; ok {
		value := int(in.Limit)
		limit = &value
	}

	var maxBytes *int64
	if limit == nil {
		limitValue := limits.MaxSizeBytes
		maxBytes = &limitValue
	}

	readResult, snapshotDigest, err := readFileInRangeFromOpenFile(
		ctx,
		f,
		openedInfo,
		filePath,
		zeroBasedOffset,
		limit,
		maxBytes,
		ReadFileRangeOptions{},
	)
	if err != nil {
		switch typed := err.(type) {
		case *FileTooLargeError:
			oversize := structuredReadSizeError(filePath, typed.SizeInBytes, typed.MaxSize)
			return oversize, nil
		default:
			if os.IsNotExist(err) {
				return errorResponse(fileReadNotFoundRuntimeError(filePath)), nil
			}
			return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileReadFailed, err)), nil
		}
	}
	if tokens, _ := t.tieredReadTokenCount(ctx, readResult.Content, limits.MaxTokens); tokens > limits.MaxTokens {
		return t.fileReadTokenLimitResult(ctx, filePath, readResult.Content, startLine, tokens, limits.MaxTokens), nil
	}

	if readResult.TotalLines == 0 {
		// Empty file is still a successful read; record state so the next
		// Read can dedup.
		state.RecordReadForContext(ctx, normalizedPath, ReadFileEntry{
			TimestampMs:      readResult.ModTimeMs,
			MtimeNs:          readResult.ModTimeNs,
			TotalBytes:       readResult.TotalBytes,
			ContentDigest:    snapshotDigest,
			FileIdentity:     openedInfo,
			Offset:           requestedOffset,
			Limit:            requestedLimit,
			OffsetSpecified:  offsetSpecified,
			LimitSpecified:   limitSpecified,
			CoverageComplete: true,
			FullSnapshot:     true,
			IsPartialView:    false,
			LastTool:         "Read",
			DedupEligible:    true,
		})
		return t.newTextReadResult(FileReadOutputFile{
			FilePath: filePath, Content: "", NumLines: 0, StartLine: startLine, TotalLines: 0,
		}), nil
	}
	if startLine > readResult.TotalLines {
		state.RecordReadForContext(ctx, normalizedPath, ReadFileEntry{
			TimestampMs: readResult.ModTimeMs, MtimeNs: readResult.ModTimeNs,
			TotalBytes: readResult.TotalBytes, TotalLines: readResult.TotalLines,
			ContentDigest: snapshotDigest,
			FileIdentity:  openedInfo,
			Offset:        requestedOffset, Limit: requestedLimit,
			OffsetSpecified: offsetSpecified, LimitSpecified: limitSpecified,
			IsPartialView: false,
			LastTool:      "Read", DedupEligible: true,
		})
		return t.newTextReadResult(FileReadOutputFile{
			FilePath: filePath, Content: "", NumLines: 0, StartLine: startLine, TotalLines: readResult.TotalLines,
		}), nil
	}

	coverage, coverageComplete := readObservationCoverage(startLine, readResult.LineCount, readResult.TotalLines)
	coverageComplete = coverageComplete && !readResult.TruncatedByBytes

	state.RecordReadForContext(ctx, normalizedPath, ReadFileEntry{
		TimestampMs:      readResult.ModTimeMs,
		MtimeNs:          readResult.ModTimeNs,
		TotalBytes:       readResult.TotalBytes,
		ContentDigest:    snapshotDigest,
		FileIdentity:     openedInfo,
		Offset:           requestedOffset,
		Limit:            requestedLimit,
		OffsetSpecified:  offsetSpecified,
		LimitSpecified:   limitSpecified,
		Coverage:         coverage,
		TotalLines:       readResult.TotalLines,
		CoverageComplete: coverageComplete,
		FullSnapshot:     coverageComplete,
		IsPartialView:    false,
		Content:          readResult.Content,
		LastTool:         "Read",
		DedupEligible:    true,
		Encoding:         encoding,
		BOM:              encodingBOM,
	})

	result := t.newTextReadResult(FileReadOutputFile{
		FilePath: filePath, Content: readResult.Content, NumLines: readResult.LineCount,
		StartLine: startLine, TotalLines: readResult.TotalLines,
	})
	return result, nil
}

// ── FileWriteTool ────────────────────────────────────────────────────────────

// FileWriteTool writes content to a file atomically:
//
//   - Plan mode rejects all writes.
//   - The path and any resolved symlink target must lie inside AllowedDirs.
//   - Existing files require a prior Read recorded in ReadState; if the
//     file has been modified since that Read, the write is rejected as
//     stale.
//   - Writes are atomic (temp + fsync + rename) and create missing parent
//     directories.
//   - The result reports create/update, the resolved path and content, a
//     structured patch, and the original content for updates.
type FileWriteTool struct {
	AllowedDirs []string
	PlanState   PlanMode
	// Runtime is sampled for each invocation so relative path expansion and
	// permission checks follow session/worktree cwd changes.
	Runtime types.ToolRuntimeContextProvider
	// ReadState tracks files read by the Read tool. If nil, the package
	// default is used. Tests should pass a fresh NewReadFileState() to
	// isolate state.
	ReadState *ReadFileState
}

func (t *FileWriteTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileWriteTool) Name() string { return "Write" }
func (t *FileWriteTool) Description() string {
	return toolPromptText(i18n.KeyToolFileWriteDescription)
}

func (t *FileWriteTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: 100_000}
}

func (t *FileWriteTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionText(i18n.KeyToolPermissionWritePlanMode), Required: true}, nil
	}
	return checkFileWriteToolPermission(t, input, request), nil
}

func (t *FileEditTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: 100_000}
}

func (t *FileEditTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionText(i18n.KeyToolPermissionEditPlanMode), Required: true}, nil
	}
	return checkFileWritePermission(permissionFilePath(input), input, request.Runtime.AllowedDirs, t.AllowedDirs, "Edit"), nil
}

func permissionFilePath(input map[string]any) string {
	if input == nil {
		return ""
	}
	path, _ := input["file_path"].(string)
	return strings.TrimSpace(path)
}

func checkFileWritePermission(path string, input map[string]any, runtimeAllowed, toolAllowed []string, toolName string) types.ToolPermissionResult {
	if path == "" {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorPassthrough}
	}
	allowed := runtimeAllowed
	if allowed == nil {
		allowed = toolAllowed
	}
	if len(allowed) > 0 && checkAllowedPath(path, allowed) != nil {
		return outsideDirectoryPermission(path, toolName)
	}
	absPath := path
	if resolved, err := filepath.Abs(path); err == nil {
		absPath = filepath.Clean(resolved)
	}
	return types.ToolPermissionResult{
		Behavior: types.PermissionBehaviorPassthrough,
		Message:  toolPermissionFormat(i18n.KeyToolPermissionModifyRequired, toolName, absPath),
		Suggestions: []types.PermissionUpdate{{
			Type:        types.PermissionUpdateAddRules,
			Destination: types.PermissionDestinationLocalSettings,
			Behavior:    types.PermissionBehaviorAllow,
			Rules: []types.PermissionRuleValue{{
				ToolName:    toolName,
				RuleContent: absPath,
			}},
		}},
		UpdatedInput: input,
	}
}

func outsideDirectoryPermission(path, toolName string) types.ToolPermissionResult {
	absPath := path
	if resolved, err := filepath.Abs(path); err == nil {
		absPath = filepath.Clean(resolved)
	}
	directory := absPath
	if filepath.Ext(absPath) != "" {
		directory = filepath.Dir(absPath)
	}
	return types.ToolPermissionResult{
		Behavior:    types.PermissionBehaviorAsk,
		Message:     toolPermissionFormat(i18n.KeyToolPermissionOutsideDirectories, absPath),
		BlockedPath: absPath,
		Required:    true,
		Suggestions: []types.PermissionUpdate{{
			Type:        types.PermissionUpdateAddDirectories,
			Destination: types.PermissionDestinationLocalSettings,
			Directories: []string{directory},
		}, {
			Type:        types.PermissionUpdateAddRules,
			Destination: types.PermissionDestinationLocalSettings,
			Behavior:    types.PermissionBehaviorAllow,
			Rules:       []types.PermissionRuleValue{{ToolName: toolName, RuleContent: absPath}},
		}},
	}
}

func (t *FileWriteTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolFileWriteInputFilePathDescription),
			},
			"content": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolFileWriteInputContentDescription),
			},
		},
		"file_path", "content",
	)
}

func (t *FileWriteTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFilePlanModeBlocked, "Write")), nil
	}

	in, toolErr := toolbase.ParseStrictInputOrError[FileWriteInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if strings.TrimSpace(in.FilePath) == "" {
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFilePathRequired)), nil
	}
	if _, present := input["content"]; !present {
		return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFileContentRequired)), nil
	}
	absPath, err := t.expandPath(in.FilePath)
	if err != nil {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileResolveFailed, err)), nil
	}
	content := in.Content

	// Team memory secret guard: writes targeting .luban-code/memory/team/* must
	// not contain credential-like patterns.
	if isTeamMemoryFilePath(absPath) {
		if hit := scanForTeamMemorySecrets(content); hit != "" {
			return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileTeamMemorySecret, absPath, hit)), nil
		}
	}

	allowedDirs := t.allowedDirs()
	if err := checkAllowedPath(absPath, allowedDirs); err != nil {
		return errorResponse(err), nil
	}
	state := t.ReadState
	if state == nil {
		state = NewReadFileState()
	}

	if dir := filepath.Dir(absPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileCreateDirectoryFailed, err)), nil
		}
	}

	target, err := inspectFileWriteTarget(absPath)
	if err != nil {
		return errorResponse(err), nil
	}
	unlock := lockFileEdit(target.TargetPath)
	defer unlock()
	// Re-inspect under the per-path lock so concurrent Edit/Write calls share
	// one complete read-check-write transaction.
	target, err = inspectFileWriteTarget(absPath)
	if err != nil {
		return errorResponse(err), nil
	}
	if target.Exists {
		entry, hasEntry := state.GetForContext(ctx, absPath)
		if !hasEntry {
			return structuredFileError(
				toolRuntimeText(i18n.KeyToolFileNotReadForWrite), fileErrorWriteReadRequired,
				absPath, true, nil, readFileRetry(absPath, 0, 0),
			), nil
		}
		if entry.IsPartialView || !readEntryCoverageComplete(entry) || !readEntryHasFullSnapshot(entry) {
			return structuredFileError(
				toolRuntimeText(i18n.KeyToolFilePartiallyReadForWrite), fileErrorWriteFullRead,
				absPath, true, readEntryToolCoverage(entry, nil), readFileRetry(absPath, 0, 0),
			), nil
		}
		if isFileStale(target.Info.ModTime(), entry) && target.Before != entry.Content {
			return errorResponsef("%s", toolRuntimeText(i18n.KeyToolFileChangedForWrite)), nil
		}
	}
	if err := recheckFileWriteTarget(target, allowedDirs); err != nil {
		return errorResponse(err), nil
	}
	writeBytes := encodeFileWriteBytes(content, target.Encoding, target.BOM)
	if err := secureio.AtomicWriteFile(target.TargetPath, writeBytes, target.Mode); err != nil {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileWriteFailed, err)), nil
	}

	var newMtimeMs, newMtimeNs int64
	var newFileIdentity os.FileInfo
	if newInfo, err := os.Stat(target.TargetPath); err == nil {
		newMtimeMs = newInfo.ModTime().UnixMilli()
		newMtimeNs = newInfo.ModTime().UnixNano()
		newFileIdentity = newInfo
	} else {
		now := time.Now()
		newMtimeMs, newMtimeNs = now.UnixMilli(), now.UnixNano()
	}
	totalLines := readStateTotalLines(content)
	coverage, _ := readObservationCoverage(1, totalLines, totalLines)
	state.SetForContext(ctx, absPath, ReadFileEntry{
		TimestampMs:      newMtimeMs,
		MtimeNs:          newMtimeNs,
		TotalBytes:       int64(len(writeBytes)),
		ContentDigest:    fileContentDigest(writeBytes),
		FileIdentity:     newFileIdentity,
		TotalLines:       totalLines,
		Coverage:         coverage,
		CoverageComplete: true,
		FullSnapshot:     true,
		Content:          content,
		IsPartialView:    false,
		LastTool:         "Write",
		DedupEligible:    false,
		Encoding:         target.Encoding,
		BOM:              append([]byte(nil), target.BOM...),
	})

	var patch []DiffHunk
	if target.Exists {
		patch = generateUnifiedHunks(target.Before, content, 3)
	}
	if patch == nil {
		patch = []DiffHunk{}
	}
	resultType := "create"
	var originalFile *string
	if target.Exists {
		resultType = "update"
		before := target.Before
		originalFile = &before
	}
	return fileWriteSuccessResponse(FileWriteResult{
		Type: resultType, FilePath: absPath, Content: content,
		StructuredPatch: patch, OriginalFile: originalFile,
	})
}

func peekOpenFileHead(file *os.File, maxBytes int) []byte {
	if file == nil || maxBytes <= 0 {
		return nil
	}
	buf := make([]byte, maxBytes)
	n, err := file.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) && n == 0 {
		return nil
	}
	return buf[:n]
}

func verifyOpenReadSnapshot(file *os.File, before os.FileInfo, path string) (os.FileInfo, error) {
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if before == nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, i18n.NewError(i18n.KeyToolFileHelperEditTargetChangedWhileRead)
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(after, current) {
		return nil, i18n.NewError(i18n.KeyToolFileHelperEditTargetChangedWhileRead)
	}
	return after, nil
}

func readRawSnapshotFromOpenFile(ctx context.Context, file *os.File, before os.FileInfo, path string) ([]byte, string, os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, err
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, "", nil, err
	}
	after, err := verifyOpenReadSnapshot(file, before, path)
	if err != nil {
		return nil, "", nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, err
	}
	return raw, fileContentDigest(raw), after, nil
}

// readFileInRangeFromOpenFile scans the already-authorized descriptor once.
// The selected visible lines and the full raw SHA-256 therefore belong to the
// same byte stream; path metadata is never used as a substitute for content.
func readFileInRangeFromOpenFile(
	ctx context.Context,
	file *os.File,
	before os.FileInfo,
	path string,
	offset int,
	maxLines *int,
	maxBytes *int64,
	options ReadFileRangeOptions,
) (ReadFileRangeResult, string, error) {
	if err := ctx.Err(); err != nil {
		return ReadFileRangeResult{}, "", err
	}
	if before == nil || !before.Mode().IsRegular() {
		return ReadFileRangeResult{}, "", i18n.NewError(i18n.KeyToolSourceSinkReadDirectory, path)
	}
	if !options.TruncateOnByteLimit && maxBytes != nil && before.Size() > *maxBytes {
		return ReadFileRangeResult{}, "", &FileTooLargeError{SizeInBytes: before.Size(), MaxSize: *maxBytes}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ReadFileRangeResult{}, "", err
	}

	reader := bufio.NewReaderSize(file, 64*1024)
	hash := sha256.New()
	endLine := int(^uint(0) >> 1)
	if maxLines != nil {
		endLine = offset + *maxLines
	}
	selectedLines := make([]string, 0)
	var selectedBytes, totalBytes int64
	truncatedByBytes := false
	lineIndex := 0
	firstChunk := true
	var partial []byte

	tryPush := func(line []byte) {
		if truncatedByBytes {
			return
		}
		if options.TruncateOnByteLimit && maxBytes != nil {
			separator := int64(0)
			if len(selectedLines) > 0 {
				separator = 1
			}
			if selectedBytes+separator+int64(len(line)) > *maxBytes {
				truncatedByBytes = true
				return
			}
			selectedBytes += separator + int64(len(line))
		}
		selectedLines = append(selectedLines, string(line))
	}
	processLine := func(line []byte) {
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if lineIndex >= offset && lineIndex < endLine {
			tryPush(line)
		}
		lineIndex++
	}

	for {
		if err := ctx.Err(); err != nil {
			return ReadFileRangeResult{}, "", err
		}
		chunk, readErr := reader.ReadSlice('\n')
		if len(chunk) > 0 {
			totalBytes += int64(len(chunk))
			_, _ = hash.Write(chunk)
			visibleChunk := chunk
			if firstChunk {
				firstChunk = false
				visibleChunk = bytes.TrimPrefix(visibleChunk, []byte{0xEF, 0xBB, 0xBF})
			}
			partial = append(partial, visibleChunk...)
		}
		switch {
		case errors.Is(readErr, bufio.ErrBufferFull):
			continue
		case errors.Is(readErr, io.EOF):
			if totalBytes > 0 {
				if len(partial) > 0 && partial[len(partial)-1] == '\n' {
					partial = partial[:len(partial)-1]
					processLine(partial)
					if lineIndex >= offset && lineIndex < endLine {
						tryPush(nil)
					}
					lineIndex++
				} else {
					processLine(partial)
				}
			}
			after, err := verifyOpenReadSnapshot(file, before, path)
			if err != nil {
				return ReadFileRangeResult{}, "", err
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return ReadFileRangeResult{}, "", err
			}
			content := stringsJoinWithNewlines(selectedLines)
			return ReadFileRangeResult{
				Content: content, LineCount: len(selectedLines), TotalLines: lineIndex,
				TotalBytes: totalBytes, ReadBytes: int64(len(content)),
				ModTimeMs: after.ModTime().UnixMilli(), ModTimeNs: after.ModTime().UnixNano(),
				TruncatedByBytes: truncatedByBytes,
			}, hex.EncodeToString(hash.Sum(nil)), nil
		case readErr != nil:
			return ReadFileRangeResult{}, "", readErr
		default:
			if len(partial) > 0 && partial[len(partial)-1] == '\n' {
				partial = partial[:len(partial)-1]
			}
			processLine(partial)
			partial = partial[:0]
		}
	}
}

func (t *FileReadTool) readNonUTF8TextFromOpenFile(
	ctx context.Context,
	file *os.File,
	before os.FileInfo,
	filePath, normalizedPath string,
	in FileReadInput,
	input map[string]any,
	det EncodingDetectResult,
	limits FileReadingLimits,
) (types.ToolResult, error) {
	raw, digest, snapshotInfo, err := readRawSnapshotFromOpenFile(ctx, file, before, filePath)
	if err != nil {
		return errorResponsef("%s", toolRuntimeFormat(i18n.KeyToolFileReadFailed, err)), nil
	}
	_, limitSpecified := input["limit"]
	_, offsetSpecified := input["offset"]
	if int64(len(raw)) > limits.MaxSizeBytes && !limitSpecified {
		return structuredReadSizeError(filePath, int64(len(raw)), limits.MaxSizeBytes), nil
	}
	decoded := decodeFileBytes(raw, det)
	requestedOffset := 1
	if offsetSpecified {
		requestedOffset = int(in.Offset)
	}
	requestedLimit := 0
	if limitSpecified {
		requestedLimit = int(in.Limit)
	}
	state := t.readState()
	baseEntry := ReadFileEntry{
		TimestampMs: snapshotInfo.ModTime().UnixMilli(), MtimeNs: snapshotInfo.ModTime().UnixNano(),
		TotalBytes: snapshotInfo.Size(), ContentDigest: digest, FileIdentity: snapshotInfo,
		Offset: requestedOffset, Limit: requestedLimit,
		OffsetSpecified: offsetSpecified, LimitSpecified: limitSpecified,
		IsPartialView: false, LastTool: "Read", DedupEligible: true,
		Encoding: det.Encoding, BOM: det.BOM,
	}
	if decoded == "" {
		baseEntry.CoverageComplete, baseEntry.FullSnapshot = true, true
		state.RecordReadForContext(ctx, normalizedPath, baseEntry)
		result := t.newTextReadResult(FileReadOutputFile{FilePath: filePath, StartLine: requestedOffset})
		result.Metadata["encoding"] = string(det.Encoding)
		return result, nil
	}
	allLines := strings.Split(decoded, "\n")
	totalLines := len(allLines)
	zeroBased := requestedOffset - 1
	baseEntry.TotalLines = totalLines
	if zeroBased >= totalLines {
		state.RecordReadForContext(ctx, normalizedPath, baseEntry)
		result := t.newTextReadResult(FileReadOutputFile{FilePath: filePath, StartLine: requestedOffset, TotalLines: totalLines})
		result.Metadata["encoding"] = string(det.Encoding)
		return result, nil
	}
	endLine := totalLines
	if requestedLimit > 0 && zeroBased+requestedLimit < endLine {
		endLine = zeroBased + requestedLimit
	}
	selected := strings.Join(allLines[zeroBased:endLine], "\n")
	if tokens, _ := t.tieredReadTokenCount(ctx, selected, limits.MaxTokens); tokens > limits.MaxTokens {
		return t.fileReadTokenLimitResult(ctx, filePath, selected, requestedOffset, tokens, limits.MaxTokens), nil
	}
	coverage, complete := readObservationCoverage(requestedOffset, endLine-zeroBased, totalLines)
	baseEntry.Coverage, baseEntry.CoverageComplete, baseEntry.FullSnapshot = coverage, complete, complete
	baseEntry.Content = selected
	state.RecordReadForContext(ctx, normalizedPath, baseEntry)
	result := t.newTextReadResult(FileReadOutputFile{
		FilePath: filePath, Content: selected, NumLines: endLine - zeroBased, StartLine: requestedOffset, TotalLines: totalLines,
	})
	result.Metadata["encoding"] = string(det.Encoding)
	return result, nil
}

// isFileStale reports whether the file's current mtime indicates a
// modification newer than the recorded read entry. Comparison is at
// millisecond precision to avoid false positives from sub-ms mtime jitter
// (and to match the TS Math.floor(mtimeMs) convention).
func isFileStale(mtime time.Time, entry ReadFileEntry) bool {
	return mtime.UnixNano() != entry.MtimeNs
}

// fileReadTokenLimitResult publishes a retry only after verifying that the
// proposed leading range passes the same token counter that rejected the
// current result. A single over-limit line therefore carries no impossible
// line-range retry instead of sending the model into a repeated failure loop.
func (t *FileReadTool) fileReadTokenLimitResult(ctx context.Context, filePath, content string, startLine, tokenCount, maxTokens int) types.ToolResult {
	result := fileReadTokenLimitError(filePath, tokenCount, maxTokens)
	lines := strings.Split(content, "\n")
	best := 0
	for low, high := 1, len(lines); low <= high; {
		middle := low + (high-low)/2
		candidate := strings.Join(lines[:middle], "\n")
		candidateTokens, _ := t.tieredReadTokenCount(ctx, candidate, maxTokens)
		if candidateTokens <= maxTokens {
			best = middle
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	data, ok := result.Data.(types.ToolErrorData)
	if !ok {
		return result
	}
	if best > 0 {
		data.Retry = readFileRetry(filePath, startLine, best)
	} else {
		data.Retry = nil
	}
	result.Data = data
	return result
}

func readEntryMatchesModTime(entry ReadFileEntry, mtime time.Time) bool {
	return entry.MtimeNs == mtime.UnixNano()
}
