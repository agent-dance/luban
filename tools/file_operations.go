package tools

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
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/skills"
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
		return i18n.WrapError(i18n.KeyToolLegacyAFileResolveFailed, err)
	}
	resolved, err := resolveAllowedPath(absPath)
	if err != nil {
		return err
	}
	resolved = canonicalPathForComparison(resolved)
	for _, dir := range allowedDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if rd, err := filepath.EvalSymlinks(absDir); err == nil {
			absDir = rd
		}
		absDir = canonicalPathForComparison(absDir)
		if strings.HasPrefix(resolved, absDir+string(os.PathSeparator)) || resolved == absDir {
			return nil
		}
	}
	return i18n.NewError(i18n.KeyToolLegacyAFileOutsideAllowed, path)
}

func fileReadNotFoundRuntimeError(filePath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return i18n.NewError(i18n.KeyToolLegacyAFileNotFound, filePath)
	}
	if suggestion := suggestNearbyPath(filePath, cwd); suggestion != "" {
		return i18n.NewError(i18n.KeyToolLegacyAFileNotFoundSuggestion, cwd, suggestion)
	}
	return i18n.NewError(i18n.KeyToolLegacyAFileNotFoundInCWD, cwd)
}

func fileTooLargeRuntimeError(size, maximum int64) error {
	return i18n.NewError(i18n.KeyToolLegacyAFileTooLarge, formatReadSize(size), formatReadSize(maximum))
}

// resolveAllowedPath resolves every existing symlink component while allowing
// one or more missing trailing components. File creation commonly targets a
// not-yet-created nested directory, so resolving only the immediate parent
// incorrectly rejects safe paths such as <allowed>/.claude/settings.json.
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
			return "", i18n.WrapError(i18n.KeyToolLegacyAFileResolveFailed, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", i18n.WrapError(i18n.KeyToolLegacyAFileResolveFailed, err)
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
//   - TokenBudgetProvider (optional) returns the remaining model context
//     tokens; when set, image/PDF readers honour it.
//   - TS-compatible text listeners fire only for text. Separate legacy/rich
//     listeners remain available for Go consumers of all successful variants.
type FileReadTool struct {
	AllowedDirs []string
	// Runtime is sampled on each call so cwd, allowed directories, permission
	// rules, and active model follow session/worktree/provider changes.
	Runtime types.ToolRuntimeContextProvider
	// ModelProvider is an optional direct model source for embedders that do not
	// expose a full ToolRuntimeContextProvider.
	ModelProvider func() string

	// ReadState is the shared (path → entry) map. If nil, the package
	// default is used.
	ReadState *ReadFileState

	// SkillManager is the optional skill registry that gets nearby skill
	// directories activated after a successful read. If nil, no skills are
	// activated.
	SkillManager FileReadSkillActivator

	// TokenBudgetProvider returns the remaining model context tokens for the
	// current request. When nil, defaultReadTokenBudget is used.
	TokenBudgetProvider func() int
	// PreciseTokenCounter is sampled only when the cheap estimate crosses the
	// TS near-limit threshold. Provider-backed runtimes can wire their token API;
	// standalone tools fall back to the local tokenizer on error or absence.
	PreciseTokenCounter func(context.Context, string) (int, error)

	// DefaultReadingLimitsProvider supplies experiment/config defaults. The
	// CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS environment variable takes
	// precedence over this provider, matching TS limits.ts.
	DefaultReadingLimitsProvider func() FileReadingLimitsOverride
	// ReadingLimitsProvider supplies an optional session/per-call override and
	// therefore takes precedence over both env and defaults.
	ReadingLimitsProvider func(context.Context) *FileReadingLimitsOverride
	// ToolResultsDirProvider returns the persistent directory used for extracted
	// PDF page artifacts. It is sampled per call so worktree cwd changes apply.
	ToolResultsDirProvider func() string

	// AnalyticsHook receives event names ("tengu_file_read_dedup",
	// "tengu_session_file_read") with their payloads. Optional; when nil,
	// events are silently dropped.
	AnalyticsHook func(event string, payload map[string]any)

	// digestAfterOpenForTest is a deterministic test seam for a pathname swap
	// after dedup opens its descriptor. Production constructors leave it nil.
	digestAfterOpenForTest func()

	// listenerMu guards listeners; listeners is the set of registered
	// callbacks invoked after a successful read.
	listenerMu       sync.Mutex
	listeners        []FileReadListener
	textListeners    []FileReadTextListener
	richListeners    []FileReadListenerRich
	dynamicSkillDirs map[string]struct{}
}

// FileReadTextListener is the TS-compatible listener surface. It fires only
// for successful text reads with the resolved path and unnumbered content.
type FileReadTextListener = func(resolvedPath string, content string)

// FileReadListener is invoked after a successful read. mtimeMs is the
// file's mtime in milliseconds since epoch; isPartialView is true when the
// read covered only a portion of the file.
//
// Backward-compatible signature; richer metadata is delivered via
// FileReadListenerRich (registered separately via RegisterRichListener).
type FileReadListener func(absPath string, mtimeMs int64, isPartialView bool)

// FileReadListenerEvent carries the full metadata TS callbacks receive
// (path, content, variant, encoding, isPartial, mtime). Mirrors the TS
// fileReadListeners signature so future plugins can consume the same
// data Go does.
type FileReadListenerEvent struct {
	AbsPath       string
	MtimeMs       int64
	IsPartialView bool
	ByteCount     int64
	LineCount     int
	Variant       string
	Encoding      FileEncoding
}

// FileReadListenerRich is the rich-payload counterpart to FileReadListener.
// Both signatures fire on every successful read so plugins can pick the
// shape they need.
type FileReadListenerRich func(event FileReadListenerEvent)

// FileReadSkillActivator narrows what FileReadTool needs from a skill
// manager so we can mock it in tests and avoid pulling the skills package
// into a circular import.
type FileReadSkillActivator interface {
	AddDirectories(dirs []string)
	ActivateConditionalForPath(absPath string)
}

type fileReadNamedSkillActivator interface {
	ActivateConditionalSkill(name string)
}

type fileReadGenerationSkillActivator interface {
	AddDirectoriesAtGeneration(skills.ProjectSourceGeneration, []string) error
	ActivateConditionalForPathAtGeneration(skills.ProjectSourceGeneration, string) error
	ActivateConditionalSkillAtGeneration(skills.ProjectSourceGeneration, string) error
}

func skillProjectGenerationFromContext(ctx context.Context) (skills.ProjectSourceGeneration, bool) {
	if ctx == nil {
		return 0, false
	}
	exec, ok := loop.ToolExecutionContextFromContext(ctx)
	if !ok {
		return 0, false
	}
	return exec.SkillProjectGeneration()
}

func addSkillDirectoriesForExecution(ctx context.Context, manager FileReadSkillActivator, dirs []string) {
	if manager == nil || len(dirs) == 0 {
		return
	}
	if generation, pinned := skillProjectGenerationFromContext(ctx); pinned {
		if bound, ok := manager.(fileReadGenerationSkillActivator); ok {
			_ = bound.AddDirectoriesAtGeneration(generation, dirs)
		}
		return
	}
	manager.AddDirectories(dirs)
}

func activateConditionalPathForExecution(ctx context.Context, manager FileReadSkillActivator, absPath string) {
	if manager == nil || absPath == "" {
		return
	}
	if generation, pinned := skillProjectGenerationFromContext(ctx); pinned {
		if bound, ok := manager.(fileReadGenerationSkillActivator); ok {
			_ = bound.ActivateConditionalForPathAtGeneration(generation, absPath)
		}
		return
	}
	manager.ActivateConditionalForPath(absPath)
}

func activateConditionalNameForExecution(ctx context.Context, manager FileReadSkillActivator, name string) {
	if manager == nil || name == "" {
		return
	}
	if generation, pinned := skillProjectGenerationFromContext(ctx); pinned {
		if bound, ok := manager.(fileReadGenerationSkillActivator); ok {
			_ = bound.ActivateConditionalSkillAtGeneration(generation, name)
		}
		return
	}
	if named, ok := manager.(fileReadNamedSkillActivator); ok {
		named.ActivateConditionalSkill(name)
	}
}

// defaultReadTokenBudget is the fallback budget when no provider is set.
// Mirrors TS DEFAULT_REMAINING_TOKENS used by readImageWithTokenBudget.
const defaultReadTokenBudget = 200_000

// RegisterListener appends a listener that will be invoked after every
// successful read. Returns an unsubscribe function.
func (t *FileReadTool) RegisterListener(listener FileReadListener) func() {
	if t == nil || listener == nil {
		return func() {}
	}
	t.listenerMu.Lock()
	t.listeners = append(t.listeners, listener)
	idx := len(t.listeners) - 1
	t.listenerMu.Unlock()
	return func() {
		t.listenerMu.Lock()
		defer t.listenerMu.Unlock()
		if idx < len(t.listeners) {
			t.listeners[idx] = nil
		}
	}
}

// RegisterTextListener subscribes to TS-compatible text-read callbacks.
func (t *FileReadTool) RegisterTextListener(listener FileReadTextListener) func() {
	if t == nil || listener == nil {
		return func() {}
	}
	t.listenerMu.Lock()
	t.textListeners = append(t.textListeners, listener)
	idx := len(t.textListeners) - 1
	t.listenerMu.Unlock()
	return func() {
		t.listenerMu.Lock()
		defer t.listenerMu.Unlock()
		if idx < len(t.textListeners) {
			t.textListeners[idx] = nil
		}
	}
}

// RegisterRichListener appends a rich-payload listener invoked after
// every successful read. Mirrors the TS fileReadListeners signature
// (path, content, variant, encoding, isPartial, mtime).
func (t *FileReadTool) RegisterRichListener(listener FileReadListenerRich) func() {
	if t == nil || listener == nil {
		return func() {}
	}
	t.listenerMu.Lock()
	t.richListeners = append(t.richListeners, listener)
	idx := len(t.richListeners) - 1
	t.listenerMu.Unlock()
	return func() {
		t.listenerMu.Lock()
		defer t.listenerMu.Unlock()
		if idx < len(t.richListeners) {
			t.richListeners[idx] = nil
		}
	}
}

// invokeListeners notifies registered listeners. Errors from listeners are
// swallowed so a buggy listener cannot fail the read.
func (t *FileReadTool) invokeListeners(absPath string, mtimeMs int64, isPartialView bool) {
	if t == nil {
		return
	}
	t.listenerMu.Lock()
	listeners := append([]FileReadListener(nil), t.listeners...)
	t.listenerMu.Unlock()
	for _, l := range listeners {
		if l == nil {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			l(absPath, mtimeMs, isPartialView)
		}()
	}
}

func (t *FileReadTool) invokeTextListeners(absPath, content string) {
	if t == nil {
		return
	}
	t.listenerMu.Lock()
	listeners := append([]FileReadTextListener(nil), t.textListeners...)
	t.listenerMu.Unlock()
	for _, listener := range listeners {
		if listener == nil {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			listener(absPath, content)
		}()
	}
}

// readState returns the bound state, falling back to the package default.
func (t *FileReadTool) readState() *ReadFileState {
	if t == nil || t.ReadState == nil {
		return DefaultReadFileState()
	}
	return t.ReadState
}

// tokenBudget returns the current remaining-token budget, falling back to
// defaultReadTokenBudget when no provider is set.
func (t *FileReadTool) tokenBudget() int {
	if t == nil || t.TokenBudgetProvider == nil {
		return defaultReadTokenBudget
	}
	if v := t.TokenBudgetProvider(); v > 0 {
		return v
	}
	return defaultReadTokenBudget
}

// emitAnalytics dispatches an analytics event to the registered hook.
func (t *FileReadTool) emitAnalytics(event string, payload map[string]any) {
	if t == nil || t.AnalyticsHook == nil {
		return
	}
	defer func() { _ = recover() }()
	t.AnalyticsHook(event, payload)
}

func (t *FileReadTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileReadTool) Name() string        { return "Read" }
func (t *FileReadTool) Aliases() []string   { return []string{"FileRead"} }
func (t *FileReadTool) Description() string { return toolPromptText(i18n.KeyToolFileReadDescription) }

func (t *FileReadTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: types.UnlimitedToolResultSize}
}

func (t *FileReadTool) ToolContract() types.ToolContract {
	outputSchema := fileReadOutputSchema()
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: types.UnlimitedToolResultSize,
	}
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
			"offset": semanticNumber(toolPromptText(i18n.KeyToolFileReadInputOffsetDescription), 1, true),
			"limit":  semanticNumber(toolPromptText(i18n.KeyToolFileReadInputLimitDescription), 0, true),
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
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileInvalidInput, decodeErr)), nil
	}
	if in.Offset < 0 || in.Offset != float64(int(in.Offset)) {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAFileOffsetNonNegative)), nil
	}
	// Historical clients may still send offset=0 for the first line. Accept it
	// at execution time, but canonicalize every state/output/retry surface to
	// the documented 1-based coordinate system. The public schema advertises
	// only the canonical form.
	if _, specified := input["offset"]; specified && in.Offset == 0 {
		in.Offset = 1
	}
	if in.Limit < 0 || in.Limit != float64(int(in.Limit)) {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAFileLimitNonNegative)), nil
	}
	if _, ok := input["limit"]; ok && int(in.Limit) == 0 {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAFileLimitPositive)), nil
	}

	// fr-pages-validation-pre-io: validate the pages selector BEFORE any
	// disk I/O so a malformed range surfaces "invalid pages parameter"
	// rather than "file not found" when the path is missing. Mirrors TS
	// validateInput in FileReadTool.
	if in.Pages != "" {
		set, ok := parsePDFPageSelector(in.Pages)
		if !ok {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePagesInvalid, in.Pages)), nil
		}
		if set.OpenEnded || len(set.Pages) > PDFMaxPagesPerRead {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePageRangeTooLarge, in.Pages, PDFMaxPagesPerRead)), nil
		}
	}

	// Expand and validate path without filesystem access. Permission rules and
	// UNC handling must run before any stat/open to avoid observable bypasses
	// and Windows credential probes.
	rawFilePath := in.FilePath
	filePath, pathErr := t.expandPath(rawFilePath)
	if pathErr != nil {
		return ErrorResponse(pathErr), nil
	}
	in.FilePath = filePath
	input = cloneToolInput(input)
	input["file_path"] = filePath
	runtimeSnapshot := t.runtimeSnapshot()
	if rule, denied := matchingReadPathRule(filePath, runtimeSnapshot.DeniedRules); denied {
		_ = rule
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAFileDirectoryDenied)), nil
	}
	if isUNCPath(filePath) || isUNCPath(strings.TrimSpace(rawFilePath)) {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileUNCRequiresPermission, filePath)), nil
	}
	normalizedPath := filepath.Clean(filePath)
	if isBlockedDevicePath(normalizedPath) {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileDeviceBlocked, filePath)), nil
	}
	ext := strings.ToLower(filepath.Ext(normalizedPath))
	if hasBinaryExtension(normalizedPath) && ext != ".pdf" && !imageExtensions[ext] {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileBinaryUnsupported, ext)), nil
	}
	allowedDirs := t.readAllowedDirs()
	if err := checkAllowedPath(filePath, allowedDirs); err != nil {
		return ErrorResponse(err), nil
	}
	// The screenshot alternate path probe is safe only after permission checks.
	filePath = normalizeReadFilePath(filePath)
	normalizedPath = filepath.Clean(filePath)
	limits := t.resolveFileReadingLimits(ctx)

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
						payload := map[string]any{}
						if analyticsExt := fileExtensionForReadAnalytics(filePath); analyticsExt != "" {
							payload["ext"] = analyticsExt
						}
						t.emitAnalytics("tengu_file_read_dedup", payload)
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
			return ErrorResponse(fileReadNotFoundRuntimeError(filePath)), nil
		}
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileOpenFailed, err)), nil
	}
	defer f.Close()

	if err := verifyOpenFd(f, allowedDirs); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePathVerification, err)), nil
	}

	// Capture mtime once (post-open) for the state entry. We read mtime
	// from the open fd to close the TOCTOU window.
	openedInfo, statErr := f.Stat()
	if statErr != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileReadFailed, statErr)), nil
	}
	mtimeMs := openedInfo.ModTime().UnixMilli()

	// Encoding detection: peek the first 8KB to classify text encoding.
	// For non-UTF-8 text files (UTF-16, Latin-1) we decode here and bypass
	// the line-streaming path, since the streaming reader assumes UTF-8.
	// Mirrors TS readFileSyncWithMetadata which threads encoding through
	// the result and lets FileWrite preserve it on overwrite.
	encoding := FileEncoding("")
	var encodingBOM []byte
	if !imageExtensions[ext] && ext != ".pdf" && ext != ".ipynb" {
		if peek := peekOpenFileHead(f, 8192+utf8.UTFMax-1); len(peek) > 0 {
			det := detectFileEncoding(peek)
			encoding = det.Encoding
			encodingBOM = det.BOM
			// Non-UTF-8 text: full-read + decode path, then format like the
			// regular text path (numbered lines).
			if det.Encoding == EncodingUTF16LE || det.Encoding == EncodingUTF16BE || det.Encoding == EncodingLatin1 {
				return t.readNonUTF8TextFromOpenFile(ctx, f, openedInfo, filePath, normalizedPath, in, input, det, ext, limits)
			}
		}
	}

	if richResult, handled, err := t.executeRichRead(ctx, filePath, in, limits); handled || err != nil {
		variant := deriveRichReadVariant(ext, richReadVariantHints{IsError: richResult.IsError})
		if output, ok := asFileReadOutput(richResult.Data); ok {
			variant = output.Type
		}
		if err == nil && !richResult.IsError && richResult.Data != nil {
			// Notebook rendering currently reopens by path. Notebook Edit/Write
			// reject this format, so do not let a rich attachment masquerade as
			// same-FD mutation evidence. A future notebook-specific ledger must be
			// produced by the exact descriptor used by the renderer.
			t.afterSuccessfulRichRead(normalizedPath, mtimeMs, int64(len(richResult.Content)), 0, variant)
		}
		// Stamp the per-variant discriminator. The variant chosen here is
		// derived from the rich-read path — even error returns get tagged so
		// callers can switch on it (e.g. "large-pdf" for the > pdfAtMention
		// inline threshold).
		if richResult.Metadata == nil {
			richResult.Metadata = readVariantMetadata(variant)
		} else {
			for k, v := range readVariantMetadata(variant) {
				if _, exists := richResult.Metadata[k]; !exists {
					richResult.Metadata[k] = v
				}
			}
		}
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
			oversize.Metadata = readVariantMetadata(FileReadVariantOversize)
			return oversize, nil
		default:
			if os.IsNotExist(err) {
				return ErrorResponse(fileReadNotFoundRuntimeError(filePath)), nil
			}
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileReadFailed, err)), nil
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
			CoverageKnown:    true,
			CoverageComplete: true,
			FullSnapshot:     true,
			IsPartialView:    false,
			LastTool:         "Read",
			DedupEligible:    true,
		})
		t.afterSuccessfulTextRead(normalizedPath, "", readResult.ModTimeMs, false, fileReadSuccessMetrics{
			TotalLines: 0, ReadLines: 0, TotalBytes: readResult.TotalBytes, ReadBytes: 0,
			Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
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
			CoverageKnown: true, IsPartialView: false,
			LastTool: "Read", DedupEligible: true,
		})
		t.afterSuccessfulTextRead(normalizedPath, "", readResult.ModTimeMs, true, fileReadSuccessMetrics{
			TotalLines: readResult.TotalLines, ReadLines: 0, TotalBytes: readResult.TotalBytes, ReadBytes: 0,
			Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
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
		CoverageKnown:    true,
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

	t.afterSuccessfulTextRead(normalizedPath, readResult.Content, readResult.ModTimeMs, !coverageComplete, fileReadSuccessMetrics{
		TotalLines: readResult.TotalLines, ReadLines: readResult.LineCount,
		TotalBytes: readResult.TotalBytes, ReadBytes: readResult.ReadBytes,
		Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
	})

	result := t.newTextReadResult(FileReadOutputFile{
		FilePath: filePath, Content: readResult.Content, NumLines: readResult.LineCount,
		StartLine: startLine, TotalLines: readResult.TotalLines,
	})
	return result, nil
}

type fileReadSuccessMetrics struct {
	TotalLines int
	ReadLines  int
	TotalBytes int64
	ReadBytes  int64
	Offset     int
	Limit      *int
	Ext        string
	MessageID  string
}

func (t *FileReadTool) afterSuccessfulTextRead(absPath, content string, mtimeMs int64, isPartialView bool, metrics fileReadSuccessMetrics) {
	payload := map[string]any{
		"totalLines":            metrics.TotalLines,
		"readLines":             metrics.ReadLines,
		"totalBytes":            metrics.TotalBytes,
		"readBytes":             metrics.ReadBytes,
		"offset":                metrics.Offset,
		"is_session_memory":     detectSessionFileType(absPath) == SessionFileMemory,
		"is_session_transcript": detectSessionFileType(absPath) == SessionFileTranscript,
	}
	if metrics.Limit != nil {
		payload["limit"] = *metrics.Limit
	}
	if ext := fileExtensionForReadAnalytics(absPath); ext != "" {
		payload["ext"] = ext
	}
	if metrics.MessageID != "" {
		payload["messageID"] = metrics.MessageID
	}
	t.emitAnalytics("tengu_session_file_read", payload)
	t.invokeTextListeners(absPath, content)
	t.invokeListeners(absPath, mtimeMs, isPartialView)
	t.invokeRichListeners(FileReadListenerEvent{
		AbsPath: absPath, MtimeMs: mtimeMs, IsPartialView: isPartialView,
		ByteCount: metrics.ReadBytes, LineCount: metrics.ReadLines,
		Variant: string(FileReadVariantText),
	})
}

func (t *FileReadTool) afterSuccessfulRichRead(absPath string, mtimeMs int64, byteCount int64, lineCount int, variant FileReadVariant) {
	// Legacy lifecycle and rich listeners are retained as explicit Go
	// extensions. TS text listeners and tengu_session_file_read do not fire.
	t.invokeListeners(absPath, mtimeMs, false)
	t.invokeRichListeners(FileReadListenerEvent{
		AbsPath: absPath, MtimeMs: mtimeMs, ByteCount: byteCount,
		LineCount: lineCount, Variant: string(variant),
	})
}

// invokeRichListeners notifies registered rich-payload listeners.
func (t *FileReadTool) invokeRichListeners(event FileReadListenerEvent) {
	if t == nil {
		return
	}
	t.listenerMu.Lock()
	listeners := append([]FileReadListenerRich(nil), t.richListeners...)
	t.listenerMu.Unlock()
	for _, l := range listeners {
		if l == nil {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			l(event)
		}()
	}
}

// ── FileWriteTool ────────────────────────────────────────────────────────────

// FileWriteTool writes content to a file atomically. Mirrors the TS
// FileWriteTool behaviour:
//
//   - Plan mode rejects all writes.
//   - .ipynb files are rejected (callers must use NotebookEdit).
//   - The path must lie inside AllowedDirs (TOCTOU symlink-safe via
//     checkAllowedPath). Existing symlinks at the target are also refused
//     to prevent the rename from following the link to an attacker-chosen
//     destination.
//   - Existing files require a prior Read recorded in ReadState; if the
//     file has been modified since that Read, the write is rejected as
//     stale.
//   - Writes are atomic (temp + fsync + rename) and create missing parent
//     directories.
//   - The result payload mirrors the TS shape: {type, filePath, content,
//     isNew, bytes, lineCount, structuredPatch, originalFile, warning?}.
type FileWriteTool struct {
	AllowedDirs []string
	PlanState   *PlanState
	// Runtime is sampled for each invocation so relative path expansion and
	// permission checks follow session/worktree cwd changes.
	Runtime types.ToolRuntimeContextProvider
	// ReadState tracks files read by the Read tool. If nil, the package
	// default is used. Tests should pass a fresh NewReadFileState() to
	// isolate state.
	ReadState *ReadFileState

	// HistoryStore captures the pre-write version before the stale-write guard,
	// matching TS checkpoint timing. HistoryEnabled and HistoryCorrelationID
	// provide the runtime gate and parent-message UUID respectively.
	HistoryStore         *FileHistoryStore
	HistoryEnabled       func() bool
	HistoryCorrelationID func(context.Context) string

	// LSP — optional diagnostics provider. If nil, the result still
	// emits an empty diagnostics array. Failures here never block the
	// write.
	LSP LSPDiagnoser

	// DiagnosticsTracker — optional per-file delivered-diagnostics store
	// so duplicate diagnostics from prior writes are filtered. Mirrors the
	// TS clearDeliveredDiagnosticsForFile behaviour. nil disables tracking.
	DiagnosticsTracker DiagnosticsTracker
	PreparationTracker FileWritePreparationTracker

	// VSCodeNotifier — optional callback invoked after a successful write.
	// Mirrors TS notifyVscodeFileUpdated so a connected IDE can refresh
	// its diff view. Failures are swallowed.
	VSCodeNotifier FileWriteVSCodeNotifier

	// SkillManager is the same manager shared by Read/Edit/Agent/Skill.
	SkillManager FileReadSkillActivator

	// Remote diff is emitted only when both remote runtime and the feature gate
	// are active. The provider is injectable; failures never block Write.
	RemoteGitDiffEnabled func() bool
	GitDiffProvider      EditGitDiffProvider

	// ChangeListener receives non-model lifecycle metadata after commit.
	ChangeListener FileWriteChangeListener

	// AnalyticsHook receives event names ("tengu_write_claudemd",
	// "tengu_atomic_write_error") with their payloads. Optional.
	AnalyticsHook func(event string, payload map[string]any)

	// UserModified, when true, indicates the user altered the proposed
	// content before accepting (mirrors TS userModified flag).
	UserModified bool
}

// emitAnalytics dispatches an analytics event to the registered hook.
// Mirrors TS sendAnalyticsEvent for FileWriteTool.
func (t *FileWriteTool) emitAnalytics(event string, payload map[string]any) {
	if t == nil || t.AnalyticsHook == nil {
		return
	}
	defer func() { _ = recover() }()
	t.AnalyticsHook(event, payload)
}

func (t *FileWriteTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileWriteTool) Name() string      { return "Write" }
func (t *FileWriteTool) Aliases() []string { return []string{"FileWrite"} }
func (t *FileWriteTool) Description() string {
	return toolPromptText(i18n.KeyToolFileWriteDescription)
}

func (t *FileWriteTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true}
}

func (t *FileWriteTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: toolPermissionText(i18n.KeyToolPermissionWritePlanMode), Required: true}, nil
	}
	return checkFileWriteToolPermission(t, input, request), nil
}

func (t *FileEditTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true}
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
	if path, _ := input["file_path"].(string); strings.TrimSpace(path) != "" {
		return strings.TrimSpace(path)
	}
	path, _ := input["path"].(string)
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
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePlanModeBlocked, "Write")), nil
	}

	in, toolErr := parseStrictInputOrError[FileWriteInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if strings.TrimSpace(in.FilePath) == "" {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAFilePathRequired)), nil
	}
	if _, present := input["content"]; !present {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAFileContentRequired)), nil
	}
	absPath, err := t.expandPath(in.FilePath)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileResolveFailed, err)), nil
	}
	content := in.Content

	// Team memory secret guard: writes targeting .claude/memory/team/* must
	// not contain credential-like patterns. Mirrors TS checkTeamMemSecrets.
	if isTeamMemoryFilePath(absPath) {
		if hit := scanForTeamMemorySecrets(content); hit != "" {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileTeamMemorySecret, absPath, hit)), nil
		}
	}

	allowedDirs := t.allowedDirs()
	if err := checkAllowedPath(absPath, allowedDirs); err != nil {
		return ErrorResponse(err), nil
	}
	state := t.ReadState
	if state == nil {
		state = DefaultReadFileState()
	}

	t.beforeFileWrite(ctx, absPath)
	if dir := filepath.Dir(absPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileCreateDirectoryFailed, err)), nil
		}
	}

	target, err := inspectFileWriteTarget(absPath)
	if err != nil {
		return ErrorResponse(err), nil
	}
	unlock := lockFileEdit(target.TargetPath)
	defer unlock()
	// Re-inspect under the per-path lock so concurrent Edit/Write calls share
	// one complete read-check-write transaction.
	target, err = inspectFileWriteTarget(absPath)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if target.Exists {
		entry, hasEntry := state.GetForContext(ctx, absPath)
		if !hasEntry {
			return structuredFileError(
				toolRuntimeText(i18n.KeyToolLegacyAFileNotReadForWrite), fileErrorWriteReadRequired,
				absPath, true, nil, readFileRetry(absPath, 0, 0),
			), nil
		}
		if entry.IsPartialView || !readEntryCoverageComplete(entry) || !readEntryHasFullSnapshot(entry) {
			return structuredFileError(
				toolRuntimeText(i18n.KeyToolLegacyAFilePartiallyReadForWrite), fileErrorWriteFullRead,
				absPath, true, readEntryToolCoverage(entry, nil), readFileRetry(absPath, 0, 0),
			), nil
		}
		// TS checkpoints before this stale guard; an mtime-only change therefore
		// leaves an unused but valid pre-edit backup.
		t.trackPreWriteHistory(ctx, absPath, target.Before, content)
		if isFileStale(target.Info.ModTime(), entry) && target.Before != entry.Content {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyAFileChangedForWrite)), nil
		}
	} else {
		t.trackPreWriteHistory(ctx, absPath, "", content)
	}
	if err := recheckFileWriteTarget(target, allowedDirs); err != nil {
		return ErrorResponse(err), nil
	}
	writeBytes := encodeTSFileWriteBytes(content, target.Encoding, target.BOM)
	if err := atomicWriteFile(target.TargetPath, writeBytes, target.Mode); err != nil {
		t.emitAnalytics("tengu_atomic_write_error", map[string]any{})
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileWriteFailed, err)), nil
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
		CoverageKnown:    true,
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
	if target.Exists && target.Before != "" {
		patch = generateUnifiedHunks(target.Before, content, 3)
	}
	if patch == nil {
		patch = []DiffHunk{}
	}
	_, gitDiff := t.completeWriteLifecycle(ctx, absPath, target.Before, content, patch)
	resultType := "create"
	var originalFile *string
	if target.Before != "" {
		resultType = "update"
		before := target.Before
		originalFile = &before
	}
	return fileWriteSuccessResponse(FileWriteResult{
		Type: resultType, FilePath: absPath, Content: content,
		StructuredPatch: patch, OriginalFile: originalFile, GitDiff: gitDiff,
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
	ext string,
	limits FileReadingLimits,
) (types.ToolResult, error) {
	raw, digest, snapshotInfo, err := readRawSnapshotFromOpenFile(ctx, file, before, filePath)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileReadFailed, err)), nil
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
	var limit *int
	if limitSpecified {
		limit = &requestedLimit
	}
	state := t.readState()
	baseEntry := ReadFileEntry{
		TimestampMs: snapshotInfo.ModTime().UnixMilli(), MtimeNs: snapshotInfo.ModTime().UnixNano(),
		TotalBytes: snapshotInfo.Size(), ContentDigest: digest, FileIdentity: snapshotInfo,
		Offset: requestedOffset, Limit: requestedLimit,
		OffsetSpecified: offsetSpecified, LimitSpecified: limitSpecified,
		CoverageKnown: true, IsPartialView: false, LastTool: "Read", DedupEligible: true,
		Encoding: det.Encoding, BOM: det.BOM,
	}
	if decoded == "" {
		baseEntry.CoverageComplete, baseEntry.FullSnapshot = true, true
		state.RecordReadForContext(ctx, normalizedPath, baseEntry)
		t.afterSuccessfulTextRead(normalizedPath, "", baseEntry.TimestampMs, false, fileReadSuccessMetrics{
			TotalBytes: int64(len(raw)), Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
		})
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
		t.afterSuccessfulTextRead(normalizedPath, "", baseEntry.TimestampMs, true, fileReadSuccessMetrics{
			TotalLines: totalLines, TotalBytes: int64(len(raw)), Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
		})
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
	t.afterSuccessfulTextRead(normalizedPath, selected, baseEntry.TimestampMs, !complete, fileReadSuccessMetrics{
		TotalLines: totalLines, ReadLines: endLine - zeroBased, TotalBytes: int64(len(raw)), ReadBytes: int64(len(selected)),
		Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
	})
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
	if entry.MtimeNs != 0 {
		return mtime.UnixNano() != entry.MtimeNs
	}
	return mtime.UnixMilli() > entry.TimestampMs
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
	if entry.MtimeNs != 0 {
		return entry.MtimeNs == mtime.UnixNano()
	}
	return entry.TimestampMs == mtime.UnixMilli()
}

// writeLineCount returns the number of lines for a write payload, matching
// the convention used by the TS countLinesChanged helper:
//
//   - empty content     → 0
//   - "foo"             → 1 (final line w/o trailing newline still counts)
//   - "foo\n"           → 1
//   - "foo\nbar"        → 2
//   - "foo\nbar\n"      → 2
func writeLineCount(content string) int {
	if len(content) == 0 {
		return 0
	}
	count := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

// warnIfDocumentationCreation returns a non-empty warning when the absolute
// path looks like a freshly-created README/CHANGELOG or an arbitrary *.md
// documentation file. Mirrors the TS validateWrite soft-warning surface.
// Used only for new files (isNew=true) — editing an existing doc is fine.
func warnIfDocumentationCreation(absPath string) string {
	base := strings.ToLower(filepath.Base(absPath))
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, ext)
	if ext != ".md" && ext != ".markdown" {
		// README without extension is also documentation.
		if strings.HasPrefix(stem, "readme") || strings.HasPrefix(stem, "changelog") {
			return "Creating documentation file without explicit user request — verify this is intended."
		}
		return ""
	}
	if strings.HasPrefix(stem, "readme") || strings.HasPrefix(stem, "changelog") {
		return "Creating documentation file (README/CHANGELOG) without explicit user request — verify this is intended."
	}
	return "Creating new Markdown documentation file without explicit user request — verify this is intended."
}

// ── FileAppendTool ───────────────────────────────────────────────────────────

// FileAppendTool appends content to a file.
type FileAppendTool struct {
	AllowedDirs []string
	PlanState   *PlanState

	mutationLockRegisteredForTest func()
}

func (t *FileAppendTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileAppendTool) Name() string        { return "FileAppend" }
func (t *FileAppendTool) Description() string { return "Append content to the end of a file" }

func (t *FileAppendTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to append to",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to append",
			},
		},
		Required: []string{"file_path", "content"},
	}
}

func (t *FileAppendTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePlanModeBlocked, "FileAppend")), nil
	}

	filePath, err := MustGetStringField(input, "file_path")
	if err != nil {
		return ErrorResponse(err), nil
	}

	content, err := MustGetStringField(input, "content")
	if err != nil {
		return ErrorResponse(err), nil
	}

	// Share the same complete mutation transaction as Edit and Write. Lock
	// before permission/path revalidation and keep it through the fd write so
	// an append cannot land between Edit's final CAS verification and rename.
	unlock := lockFileEditWithRegisteredHook(filePath, t.mutationLockRegisteredForTest)
	defer unlock()

	if err := checkAllowedPath(filePath, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileOpenFailed, err)), nil
	}
	defer f.Close()

	// TOCTOU: verify the fd after open
	if err := verifyOpenFd(f, t.AllowedDirs); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePathVerification, err)), nil
	}

	if _, err := f.WriteString(content); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileAppendFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"status": "success",
		"path":   filePath,
	})
}

// ── FileDeleteTool ───────────────────────────────────────────────────────────

// FileDeleteTool deletes a file.
type FileDeleteTool struct {
	AllowedDirs []string
	PlanState   *PlanState

	mutationLockRegisteredForTest func()
}

func (t *FileDeleteTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileDeleteTool) Name() string        { return "FileDelete" }
func (t *FileDeleteTool) Description() string { return "Delete a file" }

func (t *FileDeleteTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to delete",
			},
		},
		Required: []string{"file_path"},
	}
}

func (t *FileDeleteTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePlanModeBlocked, "FileDelete")), nil
	}

	filePath, err := MustGetStringField(input, "file_path")
	if err != nil {
		return ErrorResponse(err), nil
	}

	// Deletion participates in the per-path mutation transaction; otherwise a
	// delete racing Edit's verify/rename window could be silently resurrected.
	unlock := lockFileEditWithRegisteredHook(filePath, t.mutationLockRegisteredForTest)
	defer unlock()

	if err := checkAllowedPath(filePath, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	if err := os.Remove(filePath); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileDeleteFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"status": "success",
		"path":   filePath,
	})
}

// ── FileListTool ─────────────────────────────────────────────────────────────

// FileListTool lists files in a directory.
type FileListTool struct {
	AllowedDirs []string
}

func (t *FileListTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileListTool) Name() string        { return "FileList" }
func (t *FileListTool) Description() string { return "List files and directories in a directory" }

func (t *FileListTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to the directory to list",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "If true, recursively list all files",
			},
		},
		Required: []string{"directory"},
	}
}

func (t *FileListTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir, err := MustGetStringField(input, "directory")
	if err != nil {
		return ErrorResponse(err), nil
	}

	if err := checkAllowedPath(dir, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	recursive := GetBoolField(input, "recursive", false)

	var results []map[string]any

	if recursive {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(dir, path)
			results = append(results, map[string]any{
				"name":     info.Name(),
				"path":     relPath,
				"is_dir":   info.IsDir(),
				"size":     info.Size(),
				"modified": info.ModTime().Unix(),
			})
			return nil
		})
	} else {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileListFailed, err)), nil
		}

		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			results = append(results, map[string]any{
				"name":     entry.Name(),
				"is_dir":   entry.IsDir(),
				"size":     info.Size(),
				"modified": info.ModTime().Unix(),
			})
		}
	}

	return ResponseJSON(map[string]any{
		"directory": dir,
		"entries":   results,
		"count":     len(results),
	})
}

// ── FileGlobTool ─────────────────────────────────────────────────────────────

// FileGlobTool finds files matching a pattern.
type FileGlobTool struct {
	AllowedDirs []string
}

func (t *FileGlobTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileGlobTool) Name() string        { return "FileGlob" }
func (t *FileGlobTool) Description() string { return "Find files matching a glob pattern" }

func (t *FileGlobTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern (e.g., '**/*.go')",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *FileGlobTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	pattern, err := MustGetStringField(input, "pattern")
	if err != nil {
		return ErrorResponse(err), nil
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileGlobInvalid, err)), nil
	}

	var results []map[string]any
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		results = append(results, map[string]any{
			"path":     match,
			"is_dir":   info.IsDir(),
			"size":     info.Size(),
			"modified": info.ModTime().Unix(),
		})
	}

	return ResponseJSON(map[string]any{
		"pattern": pattern,
		"matches": results,
		"count":   len(results),
	})
}

// ── FileMoveTool ─────────────────────────────────────────────────────────────

// FileMoveTool moves or renames a file.
type FileMoveTool struct {
	AllowedDirs []string
	PlanState   *PlanState

	mutationLockRegisteredForTest func()
}

func (t *FileMoveTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileMoveTool) Name() string        { return "FileMove" }
func (t *FileMoveTool) Description() string { return "Move or rename a file" }

func (t *FileMoveTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"source": map[string]any{
				"type":        "string",
				"description": "Current file path",
			},
			"destination": map[string]any{
				"type":        "string",
				"description": "New file path",
			},
		},
		Required: []string{"source", "destination"},
	}
}

func (t *FileMoveTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePlanModeBlocked, "FileMove")), nil
	}

	source, err := MustGetStringField(input, "source")
	if err != nil {
		return ErrorResponse(err), nil
	}

	dest, err := MustGetStringField(input, "destination")
	if err != nil {
		return ErrorResponse(err), nil
	}

	// A rename mutates both names. Acquire their canonical keys in sorted order
	// so Edit/Write/Append/Delete cannot interleave with either side and inverse
	// concurrent moves cannot deadlock.
	unlock := lockFileEditsWithRegisteredHook(t.mutationLockRegisteredForTest, source, dest)
	defer unlock()

	if err := checkAllowedPath(source, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	if err := checkAllowedPath(dest, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	if err := os.Rename(source, dest); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileMoveFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"status":      "success",
		"source":      source,
		"destination": dest,
	})
}

// ── FileSearchTool ───────────────────────────────────────────────────────────

// FileSearchTool searches for text in files.
type FileSearchTool struct {
	AllowedDirs []string
}

func (t *FileSearchTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileSearchTool) Name() string        { return "FileSearch" }
func (t *FileSearchTool) Description() string { return "Search for text in files matching a pattern" }

func (t *FileSearchTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern for files to search (e.g., '**/*.go')",
			},
			"search_text": map[string]any{
				"type":        "string",
				"description": "Text to search for",
			},
		},
		Required: []string{"pattern", "search_text"},
	}
}

func (t *FileSearchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	pattern, err := MustGetStringField(input, "pattern")
	if err != nil {
		return ErrorResponse(err), nil
	}

	searchText, err := MustGetStringField(input, "search_text")
	if err != nil {
		return ErrorResponse(err), nil
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileGlobInvalid, err)), nil
	}

	var results []map[string]any

	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || info.IsDir() {
			continue
		}

		content, err := os.ReadFile(match)
		if err != nil {
			continue
		}

		if strings.Contains(string(content), searchText) {
			lines := strings.Split(string(content), "\n")
			var lineNumbers []int
			for i, line := range lines {
				if strings.Contains(line, searchText) {
					lineNumbers = append(lineNumbers, i+1)
				}
			}

			results = append(results, map[string]any{
				"path":         match,
				"line_numbers": lineNumbers,
				"match_count":  len(lineNumbers),
			})
		}
	}

	return ResponseJSON(map[string]any{
		"pattern":     pattern,
		"search_text": searchText,
		"matches":     results,
		"total_files": len(results),
	})
}

// ── LinkTool ─────────────────────────────────────────────────────────────────

// LinkTool creates symbolic links.
type LinkTool struct {
	AllowedDirs []string
	PlanState   *PlanState
}

func (t *LinkTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *LinkTool) Name() string        { return "FileLink" }
func (t *LinkTool) Description() string { return "Create a symbolic link" }

func (t *LinkTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"target": map[string]any{
				"type":        "string",
				"description": "Path to the target file or directory",
			},
			"link_path": map[string]any{
				"type":        "string",
				"description": "Path where to create the symlink",
			},
		},
		Required: []string{"target", "link_path"},
	}
}

func (t *LinkTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t.PlanState != nil && t.PlanState.IsActive() {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFilePlanModeBlocked, "FileLink")), nil
	}

	target, err := MustGetStringField(input, "target")
	if err != nil {
		return ErrorResponse(err), nil
	}

	linkPath, err := MustGetStringField(input, "link_path")
	if err != nil {
		return ErrorResponse(err), nil
	}

	if err := checkAllowedPath(linkPath, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	if err := os.Symlink(target, linkPath); err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileSymlinkFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"status":    "success",
		"target":    target,
		"link_path": linkPath,
	})
}
