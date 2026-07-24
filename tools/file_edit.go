// Package tools — file_edit.go provides the FileEditTool, the Go runtime
// counterpart of src/tools/FileEditTool/FileEditTool.ts. It composes the
// helpers from file_edit_apply.go (replacement engine + line-ending
// preservation) and file_edit_diff.go (unified-diff hunk generation), gates
// the operation behind plan-mode / allowed-dirs / Read-state / settings
// validation, atomically writes the new content to disk, refreshes the read
// state, records a file-history entry and (best-effort) consults LSP
// diagnostics for the touched file.
//
// The serialized result mirrors the TS FileEditOutput shape:
//
//	{
//	  filePath:        string,
//	  oldString:       string,
//	  newString:       string,
//	  originalFile:    string,
//	  structuredPatch: DiffHunk[],
//	  userModified:    boolean,
//	  replaceAll:      boolean,
//	  occurrences:     number,   // Go-only convenience field
//	  durationMs:      number,   // Go-only convenience field
//	  status:          "success" // mirrors other Go file ops
//	  diagnostics?:    LSPDiagnostic[],
//	}
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// MaxEditFileSize is the byte-level guard against editing huge files.
// Mirrors MAX_EDIT_FILE_SIZE in FileEditTool.ts (1 GiB).
const MaxEditFileSize int64 = 1024 * 1024 * 1024

// FileEditTool replaces strings inside a file. It enforces Read-before-Edit,
// stale-edit detection, allowed-dirs sandboxing, and produces a structured
// diff payload suitable for downstream UI rendering or transcript replay.
type FileEditTool struct {
	AllowedDirs []string
	// Runtime supplies the active project root for path expansion so hooks,
	// permission checks, normalization, and execution resolve the same target.
	Runtime types.ToolRuntimeContextProvider

	// PlanState — when active, every Edit is rejected with the plan-mode
	// message (plan mode is read-only).
	PlanState *PlanState

	// ReadState is the per-session shared store of read timestamps. If nil,
	// the package-wide DefaultReadFileState is used.
	ReadState *ReadFileState

	// HistoryStore — optional file-history sink. When non-nil, every
	// successful edit is appended to .claude/file-history/<sha1>.jsonl via
	// the supplied store. nil disables history tracking entirely.
	HistoryStore *FileHistoryStore

	// LSP — optional diagnostics provider. If nil, the result has no
	// `diagnostics` field. Failures here never block the edit.
	//
	// Implementations may also satisfy LSPDocumentSync to receive the
	// didChange/didSave pair after each successful edit, mirroring the TS
	// lspManager.changeFile + lspManager.saveFile sequence so subsequent
	// rename / refactor RPCs on the same file see the post-edit text.
	LSP LSPDiagnoser

	// DiagnosticsTracker — optional per-file delivered-diagnostics store.
	// When set, FileEditTool clears the tracker before each edit and
	// filters previously-delivered diagnostics out of the result so a
	// regression that re-introduces a fixed diagnostic still surfaces.
	DiagnosticsTracker DiagnosticsTracker

	// VSCodeNotifier — optional hook invoked after a successful edit so
	// the IDE diff view refreshes in real time. Errors are non-fatal.
	VSCodeNotifier VSCodeNotifier

	// UserModified, when true, indicates the user altered the proposed edit
	// before accepting (mirrors TS userModified flag).
	UserModified bool

	// SkillManager receives nearby skill directories and conditional-path
	// activation after a successful edit.
	SkillManager FileReadSkillActivator

	// GitDiffProvider computes the optional TS-shaped remote gitDiff payload.
	// nil uses the package default, which is gated by remote diff settings.
	GitDiffProvider EditGitDiffProvider

	// AnalyticsHook receives Edit lifecycle events without coupling this tool
	// to a concrete analytics backend.
	AnalyticsHook func(event string, payload map[string]any)

	// ChangeListener observes the committed changed-file payload. It runs after
	// the atomic write and cannot veto the edit.
	ChangeListener EditChangeListener

	// afterPrecommitVerifyForTest is a deterministic test seam for exercising a
	// cooperating mutator exactly between Edit's final CAS check and rename.
	// Production constructors leave it nil.
	afterPrecommitVerifyForTest func()
}

// SetAllowedDirs replaces the allowed-dirs sandbox set.
func (t *FileEditTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *FileEditTool) Name() string      { return "Edit" }
func (t *FileEditTool) Aliases() []string { return []string{"FileEdit"} }
func (t *FileEditTool) Description() string {
	return toolPromptText(i18n.KeyToolFileEditDescription)
}

func (t *FileEditTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolFileEditInputFilePathDescription),
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolFileEditInputOldStringDescription),
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyToolFileEditInputNewStringDescription),
			},
			"replace_all": semanticBoolean(toolPromptText(i18n.KeyToolFileEditInputReplaceAllDescription), false),
		},
		"file_path", "old_string", "new_string",
	)
}

// EditResultMetadata bundles the tool-internal counters (occurrences,
// duration) under a sub-object on EditResult, mirroring the TS shape that
// keeps the operational metrics separate from the user-facing diff payload.
type EditResultMetadata struct {
	Occurrences int   `json:"occurrences"`
	DurationMs  int64 `json:"durationMs"`
}

// EditResult is the typed payload returned by FileEditTool. JSON keys mirror
// FileEditOutput from src/tools/FileEditTool/types.ts; Go-only convenience
// fields use camelCase to match the rest of the Go tool corpus.
//
// Occurrences and DurationMs remain as direct Go fields for in-process
// callers (and existing conformance tests). On the wire they are emitted
// under a "metadata" sub-object — handled by the custom MarshalJSON /
// UnmarshalJSON pair below — so the surface matches the TS layout.
type EditResult struct {
	FilePath        string          `json:"filePath"`
	OldString       string          `json:"oldString"`
	NewString       string          `json:"newString"`
	OriginalFile    string          `json:"originalFile"`
	StructuredPatch []DiffHunk      `json:"structuredPatch"`
	UserModified    bool            `json:"userModified"`
	ReplaceAll      bool            `json:"replaceAll"`
	GitDiff         *EditGitDiff    `json:"gitDiff,omitempty"`
	Occurrences     int             `json:"-"`
	DurationMs      int64           `json:"-"`
	ReplaceAllUsed  bool            `json:"-"`
	Status          string          `json:"status"`
	Diagnostics     []LSPDiagnostic `json:"diagnostics"`
	Warning         string          `json:"warning,omitempty"`
}

// editResultWire is the JSON wire shape for EditResult. The wire emits
// occurrences/durationMs under a metadata sub-object (alignment requirement)
// while keeping them as flat Go fields on EditResult for legacy callers.
type editResultWire struct {
	FilePath        string             `json:"filePath"`
	OldString       string             `json:"oldString"`
	NewString       string             `json:"newString"`
	OriginalFile    string             `json:"originalFile"`
	StructuredPatch []DiffHunk         `json:"structuredPatch"`
	UserModified    bool               `json:"userModified"`
	ReplaceAll      bool               `json:"replaceAll"`
	GitDiff         *EditGitDiff       `json:"gitDiff,omitempty"`
	Metadata        EditResultMetadata `json:"metadata"`
	Status          string             `json:"status"`
	Diagnostics     []LSPDiagnostic    `json:"diagnostics"`
	Warning         string             `json:"warning,omitempty"`
}

// MarshalJSON renders the result with occurrences/durationMs under metadata.
func (r EditResult) MarshalJSON() ([]byte, error) {
	diags := r.Diagnostics
	if diags == nil {
		diags = []LSPDiagnostic{}
	}
	return json.Marshal(editResultWire{
		FilePath:        r.FilePath,
		OldString:       r.OldString,
		NewString:       r.NewString,
		OriginalFile:    r.OriginalFile,
		StructuredPatch: r.StructuredPatch,
		UserModified:    r.UserModified,
		ReplaceAll:      r.ReplaceAll,
		GitDiff:         r.GitDiff,
		Metadata: EditResultMetadata{
			Occurrences: r.Occurrences,
			DurationMs:  r.DurationMs,
		},
		Status:      r.Status,
		Diagnostics: diags,
		Warning:     r.Warning,
	})
}

// UnmarshalJSON accepts either the new metadata-grouped shape or the legacy
// flat shape (where occurrences/durationMs sat at the top level), so older
// fixtures and conformance tests continue to deserialise.
func (r *EditResult) UnmarshalJSON(data []byte) error {
	var wire editResultWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	r.FilePath = wire.FilePath
	r.OldString = wire.OldString
	r.NewString = wire.NewString
	r.OriginalFile = wire.OriginalFile
	r.StructuredPatch = wire.StructuredPatch
	r.UserModified = wire.UserModified
	r.ReplaceAll = wire.ReplaceAll
	r.GitDiff = wire.GitDiff
	r.ReplaceAllUsed = wire.ReplaceAll
	r.Status = wire.Status
	r.Diagnostics = wire.Diagnostics
	r.Warning = wire.Warning
	r.Occurrences = wire.Metadata.Occurrences
	r.DurationMs = wire.Metadata.DurationMs

	// Backwards compatibility: accept top-level occurrences/durationMs too.
	var legacy struct {
		Occurrences *int   `json:"occurrences"`
		DurationMs  *int64 `json:"durationMs"`
	}
	if err := json.Unmarshal(data, &legacy); err == nil {
		if legacy.Occurrences != nil && r.Occurrences == 0 {
			r.Occurrences = *legacy.Occurrences
		}
		if legacy.DurationMs != nil && r.DurationMs == 0 {
			r.DurationMs = *legacy.DurationMs
		}
	}
	return nil
}

// readState resolves the ReadFileState the Edit should operate against.
func (t *FileEditTool) readState() *ReadFileState {
	if t.ReadState != nil {
		return t.ReadState
	}
	return DefaultReadFileState()
}

func (t *FileEditTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	start := time.Now()

	if t.PlanState != nil && t.PlanState.IsActive() {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeEditPlanMode)), nil
	}

	in, toolErr := decodeFileEditInput(input)
	if toolErr != nil {
		return *toolErr, nil
	}
	filePath := strings.TrimSpace(in.FilePath)
	if filePath == "" {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, "file_path")), nil
	}
	if _, hasOld := input["old_string"]; !hasOld {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, "old_string")), nil
	}
	if _, hasNew := input["new_string"]; !hasNew {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, "new_string")), nil
	}

	oldString := in.OldString
	newString := in.NewString
	replaceAll := in.ReplaceAll

	if oldString == newString {
		return types.ToolResult{
			Content:  toolRuntimeText(i18n.KeyToolRuntimeEditNoChanges),
			Outcome:  types.ToolOutcomeSucceeded,
			Metadata: map[string]string{"semanticCategory": "unchanged"},
		}, nil
	}

	// Reject Jupyter notebooks early — caller must use NotebookEdit.
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".ipynb" {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeEditNotebook)), nil
	}

	// Allowed-dirs / symlink TOCTOU boundary. checkAllowedPath resolves
	// symlinks before checking, so a path that escapes via symlink is
	// rejected here. For not-yet-existing files (creation), checkAllowedPath
	// still validates the parent directory.
	if err := checkAllowedPath(filePath, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}

	absPath, err := t.expandPath(filePath)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeResolvePathFailed, err)), nil
	}
	absPath = filepath.Clean(absPath)
	unlock := lockFileEdit(absPath)
	defer unlock()

	// TS rejects the proposed new_string before touching team-memory files so
	// secret-shaped values never reach history, diagnostics, or disk.
	if isTeamMemoryFilePath(absPath) {
		if secret := scanForTeamMemorySecrets(newString); secret != "" {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeEditTeamMemorySecret, secret)), nil
		}
	}

	state := t.readState()

	// Lstat (not Stat) so we can refuse to follow symlinks at the target.
	info, statErr := os.Lstat(absPath)
	fileExists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeStatFileFailed, statErr)), nil
	}

	// File creation path: empty old_string + nonexistent file.
	if !fileExists {
		if oldString != "" {
			cwd := currentWorkingDirOrEmpty()
			suggestions := suggestSimilarPath(cwd, filePath)
			hint := formatDidYouMean(filePath, suggestions)
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeFileMissing, cwd, hint)), nil
		}
		return t.handleNewFileCreation(ctx, absPath, filePath, newString, replaceAll, start)
	}

	// File exists. Validate kind and refuse symlinks (TOCTOU mitigation —
	// rename onto a symlink would replace the link itself, but if an
	// attacker swapped it we could write to an arbitrary location).
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeEditSymlink, filePath)), nil
	}
	if info.IsDir() {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimePathIsDirectory, filePath)), nil
	}

	// File-size guard: prevent OOM on multi-GB files.
	if info.Size() > MaxEditFileSize {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeEditFileTooLarge, info.Size(), MaxEditFileSize)), nil
	}

	// Read-state lookup. Edits without a prior Read are rejected — the model
	// would otherwise be applying changes to a file it has never seen.
	entry, hasEntry := state.GetForContext(ctx, absPath)
	if !hasEntry {
		return structuredFileError(
			toolRuntimeText(i18n.KeyToolRuntimeFileNotRead), fileErrorReadRequired,
			filePath, true, nil, readFileRetry(filePath, 0, 0),
		), nil
	}
	if entry.IsPartialView {
		return structuredFileError(
			toolRuntimeText(i18n.KeyToolRuntimeFileViewTransformed), fileErrorViewTransformed,
			filePath, true, readEntryToolCoverage(entry, nil), readFileRetry(filePath, 0, 0),
		), nil
	}

	// Read fresh content for the edit. Decode through the same metadata path
	// Read/Write use so UTF-16, Latin-1, and UTF-8 BOM files round-trip
	// without byte corruption.
	targetSnapshot, err := readEditTarget(absPath, info)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeReadFileFailed, err)), nil
	}
	rawContent := targetSnapshot.Raw
	encoding := detectFileEncoding(rawContent)
	originalContent := decodeFileBytes(rawContent, encoding)

	if entry.ContentDigest == "" || entry.FileIdentity == nil ||
		!readEntryMatchesFileIdentity(entry, targetSnapshot.Info) || targetSnapshot.ContentDigest != entry.ContentDigest {
		return structuredFileError(
			toolRuntimeText(i18n.KeyToolRuntimeFileChangedSinceRead), fileErrorSnapshotStale,
			filePath, true, readEntryToolCoverage(entry, nil), readFileRetry(filePath, 0, 0),
		), nil
	}

	// Detect the file's predominant line ending and normalise to LF for
	// matching, restoring the original ending on write. This mirrors the TS
	// readFileSyncWithMetadata + writeTextContent pair.
	ending := detectLineEnding(originalContent)
	normalisedContent := normaliseToLF(originalContent)
	normalisedOld := normaliseToLF(oldString)
	normalisedNew := normaliseToLF(newString)
	normalisedOld, normalisedNew = desanitizeFileEditPair(normalisedContent, normalisedOld, normalisedNew)

	actualOldString, ok := findActualString(normalisedContent, normalisedOld)
	if !ok {
		return structuredFileError(
			toolRuntimeFormat(i18n.KeyToolRuntimeEditStringMissing, oldString), fileErrorAnchorMissing,
			filePath, true, readEntryToolCoverage(entry, nil), readFileRetry(filePath, 0, 0),
		), nil
	}
	actualNewString := preserveQuoteStyle(normalisedOld, actualOldString, normalisedNew)

	updated, occurrences, applyErr := ApplyEdit(normalisedContent, actualOldString, actualNewString, replaceAll)
	if applyErr != nil {
		if occurrences > 1 {
			return structuredFileError(
				toolRuntimeFormat(i18n.KeyToolRuntimeEditAmbiguousMatch, occurrences), fileErrorAnchorAmbiguous, filePath, true,
				readEntryToolCoverage(entry, nil), nil,
			), nil
		}
		return mapApplyError(applyErr, oldString), nil
	}
	if uncovered, covered := readEntryCoversEdit(entry, normalisedContent, actualOldString, replaceAll); !covered {
		// readEntryCoversEdit returns only missing evidence. Publishing that
		// same set in coverage.required guarantees a structured retry never
		// asks the model to reread an already observed match or page prefix.
		next := uncovered[0]
		limit := next.EndLine - next.StartLine + 1
		return structuredFileError(
			toolRuntimeFormat(i18n.KeyToolRuntimeEditRangeNotObserved, next.StartLine, next.EndLine),
			fileErrorAnchorUnobserved, filePath, true,
			readEntryToolCoverage(entry, uncovered), readFileRetry(filePath, next.StartLine, limit),
		), nil
	}

	// Settings.json validation: refuse edits that would corrupt the
	// .claude settings file (invalid JSON, removed required keys).
	settingsWarning, validErr := validateSettingsEdit(absPath, updated, originalContent)
	if validErr != nil {
		return ErrorResponse(validErr), nil
	}

	// Restore the file's native line ending before writing to disk.
	finalContent := restoreLineEnding(updated, ending)

	// Atomic write: temp + fsync + rename, preserves file mode.
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	writeBytes := encodeWriteBytes(finalContent, encoding.Encoding, encoding.BOM)
	if err := atomicWriteFileWithEditCAS(absPath, writeBytes, mode, func() error {
		if err := t.recheckEditTarget(absPath, targetSnapshot.Info, targetSnapshot.ContentDigest); err != nil {
			return err
		}
		if t.afterPrecommitVerifyForTest != nil {
			t.afterPrecommitVerifyForTest()
		}
		return nil
	}); err != nil {
		if errors.Is(err, errEditSnapshotCASMismatch) {
			return structuredFileError(
				toolRuntimeText(i18n.KeyToolRuntimeFileChangedSinceRead), fileErrorSnapshotStale,
				filePath, true, readEntryToolCoverage(entry, nil), readFileRetry(filePath, 0, 0),
			), nil
		}
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeWriteFileFailed, err)), nil
	}

	// Structured patch generation (LF-normalised on both sides for stable
	// hunk numbering). Mirrors TS getPatchForEdit + structuredPatch output.
	patch := generateUnifiedHunks(convertLeadingTabsForDiff(normalisedContent), convertLeadingTabsForDiff(updated), 3)

	return t.completeSuccessfulEdit(ctx, editCompletion{
		AbsPath:            absPath,
		DisplayPath:        filePath,
		Before:             originalContent,
		After:              finalContent,
		OldString:          actualOldString,
		AnalyticsOldString: oldString,
		NewString:          newString,
		Patch:              patch,
		Occurrences:        occurrences,
		ReplaceAll:         replaceAll,
		StartedAt:          start,
		Encoding:           encoding.Encoding,
		BOM:                encoding.BOM,
		SettingsWarning:    settingsWarning,
		ContentDigest:      fileContentDigest(writeBytes),
	})
}

// handleNewFileCreation supports the TS "empty old_string on nonexistent
// file = create new file" semantics. The ReadState requirement does not
// apply to brand-new files.
func (t *FileEditTool) handleNewFileCreation(ctx context.Context, absPath, filePath, newString string, replaceAll bool, start time.Time) (types.ToolResult, error) {
	settingsWarning, validationErr := validateSettingsEdit(absPath, newString, "")
	if validationErr != nil {
		return ErrorResponse(validationErr), nil
	}
	if dir := filepath.Dir(absPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeCreateDirectoryFailed, err)), nil
		}
	}
	if err := checkAllowedPath(absPath, t.AllowedDirs); err != nil {
		return ErrorResponse(err), nil
	}
	if _, err := os.Lstat(absPath); err == nil {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeEditTargetAppeared)), nil
	} else if !os.IsNotExist(err) {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeEditRecheckTargetFailed, err)), nil
	}
	finalContent := normaliseToLF(newString)
	if err := atomicCreateFile(absPath, []byte(finalContent), 0o644); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeEditTargetAppeared)), nil
		}
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeWriteFileFailed, err)), nil
	}

	patch := generateUnifiedHunks("", convertLeadingTabsForDiff(finalContent), 3)
	return t.completeSuccessfulEdit(ctx, editCompletion{
		AbsPath:            absPath,
		DisplayPath:        filePath,
		Before:             "",
		After:              finalContent,
		OldString:          "",
		AnalyticsOldString: "",
		NewString:          newString,
		Patch:              patch,
		Occurrences:        1,
		ReplaceAll:         replaceAll,
		StartedAt:          start,
		Encoding:           EncodingUTF8,
		SettingsWarning:    settingsWarning,
		ContentDigest:      fileContentDigest([]byte(finalContent)),
	})
}

// mapApplyError translates a sentinel error from ApplyEdit into a
// user-facing ToolResult with the TS-matching message.
func mapApplyError(err error, oldString string) types.ToolResult {
	switch {
	case errors.Is(err, ErrEditOldStringMissing):
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolRuntimeEditStringMissing, oldString))
	case errors.Is(err, ErrEditIdenticalStrings):
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeEditNoChanges))
	case errors.Is(err, ErrEditEmptyOldString):
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeEditCreateExisting))
	default:
		// ErrEditAmbiguousMatch falls through here because the wrapped
		// error already contains the multi-match TS message.
		return ErrorResponse(err)
	}
}

// currentWorkingDirOrEmpty returns os.Getwd() or "" on failure. Used to
// embed the cwd in the "File does not exist" message (matching TS).
func currentWorkingDirOrEmpty() string {
	if cwd, err := os.Getwd(); err == nil {
		return displayPathForUser(cwd)
	}
	return ""
}

// String renders a one-line user-facing summary suitable for transcript
// display. Mirrors the TS mapToolResultToToolResultBlockParam content text.
func (r EditResult) String() string {
	if r.ReplaceAll {
		if r.UserModified {
			return toolRuntimeFormat(i18n.KeyToolRuntimeEditSummaryReplaceAllModified, r.FilePath, r.Occurrences)
		}
		return toolRuntimeFormat(i18n.KeyToolRuntimeEditSummaryReplaceAll, r.FilePath, r.Occurrences)
	}
	if r.UserModified {
		return toolRuntimeFormat(i18n.KeyToolRuntimeEditSummaryModified, r.FilePath)
	}
	return toolRuntimeFormat(i18n.KeyToolRuntimeEditSummary, r.FilePath)
}
