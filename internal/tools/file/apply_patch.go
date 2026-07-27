package file

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

const (
	fileErrorApplyPatchParse           = "file.apply_patch.parse"
	fileErrorApplyPatchConflict        = "file.apply_patch.conflict"
	fileErrorApplyPatchReadRequired    = "file.apply_patch.read_required"
	fileErrorApplyPatchPermission      = "file.apply_patch.permission"
	fileErrorApplyPatchCommit          = "file.apply_patch.commit"
	fileErrorApplyPatchAnchorMissing   = "file.apply_patch.anchor_missing"
	fileErrorApplyPatchAnchorAmbiguous = "file.apply_patch.anchor_ambiguous"
	fileErrorApplyPatchPosition        = "file.apply_patch.position_mismatch"
	fileErrorApplyPatchEOF             = "file.apply_patch.eof_mismatch"
)

type ApplyPatchInput struct {
	Patch string `json:"patch"`
}

// ApplyPatchTool applies one validated transaction containing multiple files
// and hunks. Existing content is protected by an exact snapshot digest and a
// compare-and-swap check immediately before each commit.
type ApplyPatchTool struct {
	AllowedDirs  []string
	Runtime      types.ToolRuntimeContextProvider
	PlanState    PlanMode
	ReadState    *ReadFileState
	SkillManager FileReadSkillActivator
	// CustomToolInput enables the documented OpenAI Responses freeform wire
	// contract. It is set only by the Agentic V2 experimental profile; the
	// ordinary function schema remains the fail-safe default.
	CustomToolInput bool
	// WorkspaceRevisions is set only for the Agentic V2 profile. It issues the
	// post-commit receipt consumed by an adjacent Run in the same response.
	WorkspaceRevisions *workspacerevision.Ledger

	// Test seams exercise an uncooperative writer before CAS and a failure after
	// a committed file. Production construction leaves both nil.
	beforeCommitForTest func(index int, path string)
	afterCommitForTest  func(index int, path string) error
}

type ApplyPatchFileStat struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Hunks     int    `json:"hunks"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type ApplyPatchSummary struct {
	Files     int `json:"files"`
	Hunks     int `json:"hunks"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// ApplyPatchResult deliberately contains no source snapshots or patch body.
type ApplyPatchResult struct {
	Status       string               `json:"status"`
	ChangedPaths []string             `json:"changedPaths"`
	Files        []ApplyPatchFileStat `json:"files"`
	Summary      ApplyPatchSummary    `json:"summary"`

	revision workspacerevision.Receipt
}

// CompactionProof retains the committed mutation totals, CAS disposition, and
// opaque workspace revision identity without paths, hunks, or patch content.
func (r ApplyPatchResult) CompactionProof() compactproof.Proof {
	proof := compactproof.Proof{Patch: &compactproof.PatchProof{
		Status: r.Status, CAS: "committed",
		Files: r.Summary.Files, Hunks: r.Summary.Hunks,
		Additions: r.Summary.Additions, Deletions: r.Summary.Deletions,
	}}
	if receipt, ok := r.WorkspaceRevisionReceipt(); ok {
		proof.Revision = &compactproof.RevisionProof{
			Status: "committed", Epoch: uint64(receipt.Epoch()), Digest: string(receipt.Digest()),
		}
	}
	return proof
}

func (r ApplyPatchResult) WorkspaceRevisionReceipt() (workspacerevision.Receipt, bool) {
	return r.revision, r.revision.Valid()
}

func (t *ApplyPatchTool) ProvidesWorkspaceRevisionBarrier() bool {
	return t != nil && t.WorkspaceRevisions != nil
}

type applyPatchPlan struct {
	Spec             applyPatchFile
	AbsPath          string
	Exists           bool
	Info             os.FileInfo
	BeforeRaw        []byte
	BeforeDigest     string
	Before           string
	AfterRaw         []byte
	After            string
	AfterDigest      string
	Mode             os.FileMode
	Encoding         FileEncoding
	BOM              []byte
	StagePath        string
	BackupPath       string
	KeepBackup       bool
	Committed        bool
	Additions        int
	Deletions        int
	PriorFullVisible bool
}

type applyPatchTargetFailure struct {
	Code     string
	Path     string
	Required []ReadLineRange
}

func (e *applyPatchTargetFailure) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (t *ApplyPatchTool) SetAllowedDirs(dirs []string) {
	t.AllowedDirs = append([]string(nil), dirs...)
}

func (t *ApplyPatchTool) Name() string { return "ApplyPatch" }

func (t *ApplyPatchTool) Description() string {
	if t != nil && t.CustomToolInput {
		return toolPromptText(i18n.KeyToolApplyPatchCustomDescription)
	}
	return toolPromptText(i18n.KeyToolApplyPatchDescription)
}

func (t *ApplyPatchTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"patch": map[string]any{
			"type":        "string",
			"description": toolPromptText(i18n.KeyToolApplyPatchInputPatchDescription),
		},
	}, "patch")
}

func (t *ApplyPatchTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: 100_000}
}

func (t *ApplyPatchTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{AlwaysLoad: true, SearchHint: "multi file unified diff patch create update delete atomic"}
}

func (t *ApplyPatchTool) runtimeSnapshot() types.ToolRuntimeContext {
	if t != nil && t.Runtime != nil {
		return t.Runtime.ToolRuntimeContext()
	}
	return types.ToolRuntimeContext{}
}

func (t *ApplyPatchTool) baseDir() string {
	if root := strings.TrimSpace(t.runtimeSnapshot().ProjectRoot); root != "" {
		return root
	}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return "."
}

func (t *ApplyPatchTool) allowedDirs() []string {
	if runtimeDirs := t.runtimeSnapshot().AllowedDirs; runtimeDirs != nil {
		return append([]string(nil), runtimeDirs...)
	}
	if t == nil {
		return nil
	}
	return append([]string(nil), t.AllowedDirs...)
}

func (t *ApplyPatchTool) readState() *ReadFileState {
	if t != nil && t.ReadState != nil {
		return t.ReadState
	}
	return NewReadFileState()
}

func (t *ApplyPatchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, err
	}
	if t.PlanState != nil && t.PlanState.IsActive() {
		return applyPatchToolError(i18n.KeyToolApplyPatchPlanMode, fileErrorApplyPatchPermission, "", false, ""), nil
	}
	in, err := types.DecodeStrictToolInput[ApplyPatchInput](input)
	if err != nil {
		return applyPatchParseResult("invalid_input", ""), nil
	}
	parsed, parseErr := parseApplyPatch(in.Patch)
	if parseErr != nil {
		return applyPatchParseResult(parseErr.Reason, parseErr.Path), nil
	}
	root, targets, targetErr := t.resolveTargets(parsed)
	if targetErr != nil {
		return applyPatchPermissionResult(targetErr.Code, targetErr.Path), nil
	}
	if relationErr := validateApplyPatchTargetRelations(targets); relationErr != nil {
		return applyPatchConflictResult(relationErr.Path), nil
	}

	lockPaths := make([]string, 0, len(targets)*2)
	for _, target := range targets {
		lockPaths = append(lockPaths, target.AbsPath, filepath.Dir(target.AbsPath))
	}
	unlock := lockFileEditsWithRegisteredHook(nil, lockPaths...)
	defer unlock()

	// Repeat path and symlink validation after acquiring every sorted lock.
	_, lockedTargets, lockedErr := t.resolveTargets(parsed)
	if lockedErr != nil {
		return applyPatchPermissionResult(lockedErr.Code, lockedErr.Path), nil
	}
	for index := range targets {
		if toolbase.CanonicalPath(targets[index].AbsPath) != toolbase.CanonicalPath(lockedTargets[index].AbsPath) {
			return applyPatchConflictResult(targets[index].Spec.Path), nil
		}
	}
	targets = lockedTargets

	for index := range targets {
		if err := ctx.Err(); err != nil {
			return types.ToolResult{}, err
		}
		prepared, prepareErr := t.prepareApplyPatchPlan(ctx, targets[index])
		if prepareErr != nil {
			switch prepareErr.Code {
			case fileErrorApplyPatchReadRequired:
				return applyPatchReadRequiredResult(prepareErr.Path, prepareErr.Required), nil
			case fileErrorApplyPatchPermission:
				return applyPatchPermissionResult("unsafe_target", prepareErr.Path), nil
			case fileErrorApplyPatchAnchorMissing, fileErrorApplyPatchAnchorAmbiguous,
				fileErrorApplyPatchPosition, fileErrorApplyPatchEOF:
				return applyPatchPlacementResult(prepareErr.Path, prepareErr.Code), nil
			default:
				return applyPatchConflictResult(prepareErr.Path), nil
			}
		}
		targets[index] = prepared
	}

	createdDirs, directoryErr := createApplyPatchDirectories(root, targets)
	if directoryErr != nil {
		return applyPatchCommitResult(false), nil
	}
	committed := false
	defer func() {
		cleanupApplyPatchTemps(targets)
		if !committed {
			removeEmptyApplyPatchDirectories(createdDirs)
		}
	}()

	for index := range targets {
		if err := t.verifyApplyPatchSnapshot(targets[index]); err != nil {
			return applyPatchConflictResult(targets[index].Spec.Path), nil
		}
	}
	for index := range targets {
		if err := stageApplyPatchPlan(&targets[index]); err != nil {
			return applyPatchCommitResult(false), nil
		}
	}
	for index := range targets {
		if err := t.verifyApplyPatchSnapshot(targets[index]); err != nil {
			return applyPatchConflictResult(targets[index].Spec.Path), nil
		}
	}

	ordered := make([]*applyPatchPlan, 0, len(targets))
	for index := range targets {
		ordered = append(ordered, &targets[index])
	}
	sort.Slice(ordered, func(i, j int) bool {
		return toolbase.CanonicalPath(ordered[i].AbsPath) < toolbase.CanonicalPath(ordered[j].AbsPath)
	})
	commitErr := error(nil)
	commitConflict := false
	commitConflictPath := ""
	for index, plan := range ordered {
		if err := ctx.Err(); err != nil {
			commitErr = err
			break
		}
		if t.beforeCommitForTest != nil {
			t.beforeCommitForTest(index, plan.AbsPath)
		}
		if err := t.verifyApplyPatchSnapshot(*plan); err != nil {
			commitErr = err
			commitConflict = true
			commitConflictPath = plan.Spec.Path
			break
		}
		if err := commitApplyPatchPlan(plan); err != nil {
			commitErr = err
			break
		}
		if t.afterCommitForTest != nil {
			if err := t.afterCommitForTest(index, plan.AbsPath); err != nil {
				commitErr = err
				break
			}
		}
	}
	if commitErr != nil {
		rolledBack := rollbackApplyPatchPlans(ordered)
		if errors.Is(commitErr, context.Canceled) || errors.Is(commitErr, context.DeadlineExceeded) {
			return types.ToolResult{}, commitErr
		}
		if commitConflict && rolledBack {
			return applyPatchConflictResult(commitConflictPath), nil
		}
		return applyPatchCommitResult(!rolledBack), nil
	}
	committed = true

	for index := range targets {
		t.recordApplyPatchResult(ctx, targets[index])
		t.activateSkillsForPatchedPath(ctx, targets[index].AbsPath)
	}
	var revision workspacerevision.Receipt
	if t.WorkspaceRevisions != nil {
		paths := make([]string, 0, len(targets))
		for _, target := range targets {
			paths = append(paths, target.AbsPath)
		}
		var revisionErr error
		revision, revisionErr = t.WorkspaceRevisions.Commit(root, paths)
		if revisionErr != nil {
			return applyPatchRevisionReceiptResult(), nil
		}
	}
	return applyPatchSuccessResult(targets, revision)
}

func (t *ApplyPatchTool) resolveTargets(parsed parsedApplyPatch) (string, []applyPatchPlan, *applyPatchTargetFailure) {
	base, err := filepath.Abs(t.baseDir())
	if err != nil {
		return "", nil, &applyPatchTargetFailure{Code: "invalid_root"}
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", nil, &applyPatchTargetFailure{Code: "invalid_root"}
	}
	rootInfo, err := os.Stat(base)
	if err != nil || !rootInfo.IsDir() {
		return "", nil, &applyPatchTargetFailure{Code: "invalid_root"}
	}
	allowedDirs := t.allowedDirs()
	targets := make([]applyPatchPlan, 0, len(parsed.Files))
	for _, file := range parsed.Files {
		absPath := filepath.Clean(filepath.Join(base, filepath.FromSlash(file.Path)))
		if err := checkAllowedPath(absPath, allowedDirs); err != nil {
			return "", nil, &applyPatchTargetFailure{Code: "outside_allowed", Path: file.Path}
		}
		if err := rejectApplyPatchSymlinkPath(base, absPath); err != nil {
			return "", nil, &applyPatchTargetFailure{Code: err.Code, Path: file.Path}
		}
		targets = append(targets, applyPatchPlan{Spec: file, AbsPath: absPath})
	}
	return base, targets, nil
}

func rejectApplyPatchSymlinkPath(root, target string) *applyPatchTargetFailure {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return &applyPatchTargetFailure{Code: "outside_root"}
	}
	current := root
	parts := strings.Split(relative, string(os.PathSeparator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				return nil
			}
			return &applyPatchTargetFailure{Code: "stat_failed"}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &applyPatchTargetFailure{Code: "symlink"}
		}
		if index < len(parts)-1 && !info.IsDir() {
			return &applyPatchTargetFailure{Code: "parent_not_directory"}
		}
	}
	return nil
}

func validateApplyPatchTargetRelations(targets []applyPatchPlan) *applyPatchTargetFailure {
	type targetPath struct {
		canonical string
		display   string
	}
	paths := make([]targetPath, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, targetPath{canonical: toolbase.CanonicalPath(target.AbsPath), display: target.Spec.Path})
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i].canonical < paths[j].canonical })
	for index := 1; index < len(paths); index++ {
		if strings.HasPrefix(paths[index].canonical, paths[index-1].canonical+string(os.PathSeparator)) {
			return &applyPatchTargetFailure{Code: "nested_target", Path: paths[index].display}
		}
	}
	return nil
}

func (t *ApplyPatchTool) prepareApplyPatchPlan(ctx context.Context, plan applyPatchPlan) (applyPatchPlan, *applyPatchTargetFailure) {
	info, err := os.Lstat(plan.AbsPath)
	if err != nil && !os.IsNotExist(err) {
		return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchConflict, Path: plan.Spec.Path}
	}
	plan.Exists = err == nil
	if plan.Exists {
		if info.Mode()&os.ModeSymlink != 0 {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchPermission, Path: plan.Spec.Path}
		}
		if !info.Mode().IsRegular() || info.Size() > MaxEditFileSize {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchConflict, Path: plan.Spec.Path}
		}
		if plan.Spec.Operation == applyPatchCreate {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchConflict, Path: plan.Spec.Path}
		}
		snapshot, readErr := readEditTarget(plan.AbsPath, info)
		if readErr != nil {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchConflict, Path: plan.Spec.Path}
		}
		plan.Info = snapshot.Info
		plan.BeforeRaw = snapshot.Raw
		plan.BeforeDigest = snapshot.ContentDigest
		detected := detectFileEncoding(snapshot.Raw)
		plan.Encoding = detected.Encoding
		plan.BOM = append([]byte(nil), detected.BOM...)
		plan.Before = decodeFileBytes(snapshot.Raw, detected)
		plan.Mode = snapshot.Info.Mode().Perm()
		if plan.Mode == 0 {
			plan.Mode = 0o644
		}
	} else {
		if plan.Spec.Operation != applyPatchCreate {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchConflict, Path: plan.Spec.Path}
		}
		plan.Encoding = EncodingUTF8
		plan.Mode = 0o644
	}
	beforeLF := normaliseToLF(plan.Before)
	if plan.Spec.RequiresRead {
		entry, found := t.readState().GetForContext(ctx, plan.AbsPath)
		if !found || entry.IsPartialView || !mutationReadStateIsFresh(plan.AbsPath, plan.Info, entry) {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchReadRequired, Path: plan.Spec.Path}
		}
		required, requireFull, evidenceErr := applyPatchVisibleEvidenceRequirements(plan.Spec, beforeLF)
		if evidenceErr != nil {
			return plan, &applyPatchTargetFailure{Code: applyPatchPlacementCode(evidenceErr.Reason), Path: plan.Spec.Path}
		}
		if requireFull {
			if !readEntryCoverageComplete(entry) || !readEntryHasFullSnapshot(entry) {
				return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchReadRequired, Path: plan.Spec.Path}
			}
		} else if !readEntryCoversApplyPatchLines(entry, required) {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchReadRequired, Path: plan.Spec.Path, Required: required}
		}
		plan.PriorFullVisible = readEntryCoverageComplete(entry) && readEntryHasFullSnapshot(entry)
	}
	afterLF, patchErr := applyParsedFilePatch(plan.Spec, beforeLF)
	if patchErr != nil {
		return plan, &applyPatchTargetFailure{Code: applyPatchPlacementCode(patchErr.Reason), Path: plan.Spec.Path}
	}
	if plan.Spec.Operation != applyPatchDelete {
		ending := detectLineEnding(plan.Before)
		if !plan.Exists {
			ending = "\n"
		}
		plan.After = restoreLineEnding(afterLF, ending)
		if isTeamMemoryFilePath(plan.AbsPath) {
			if secret := scanForTeamMemorySecrets(plan.After); secret != "" {
				return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchPermission, Path: plan.Spec.Path}
			}
		}
		plan.AfterRaw = encodeWriteBytes(plan.After, plan.Encoding, plan.BOM)
		plan.AfterDigest = fileContentDigest(plan.AfterRaw)
		if plan.Exists && plan.AfterDigest == plan.BeforeDigest {
			return plan, &applyPatchTargetFailure{Code: fileErrorApplyPatchConflict, Path: plan.Spec.Path}
		}
	}
	plan.Additions, plan.Deletions = applyPatchDiffCounts(plan.Spec)
	if plan.Spec.Operation == applyPatchDelete && len(plan.Spec.Hunks) == 0 {
		plan.Deletions = len(splitApplyPatchText(beforeLF).Lines)
	}
	return plan, nil
}

func (t *ApplyPatchTool) verifyApplyPatchSnapshot(plan applyPatchPlan) error {
	if err := checkAllowedPath(plan.AbsPath, t.allowedDirs()); err != nil {
		return err
	}
	if err := rejectApplyPatchSymlinkPath(filepath.Clean(t.resolvedBaseDir()), plan.AbsPath); err != nil {
		return err
	}
	if !plan.Exists {
		if _, err := os.Lstat(plan.AbsPath); err == nil {
			return fs.ErrExist
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	info, err := os.Lstat(plan.AbsPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(plan.Info, info) {
		return errEditSnapshotCASMismatch
	}
	snapshot, err := readEditTarget(plan.AbsPath, plan.Info)
	if err != nil || snapshot.ContentDigest != plan.BeforeDigest {
		return errEditSnapshotCASMismatch
	}
	return nil
}

func (t *ApplyPatchTool) resolvedBaseDir() string {
	base, err := filepath.Abs(t.baseDir())
	if err != nil {
		return filepath.Clean(t.baseDir())
	}
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		return resolved
	}
	return base
}

func (t *ApplyPatchTool) recordApplyPatchResult(ctx context.Context, plan applyPatchPlan) {
	state := t.readState()
	if plan.Spec.Operation == applyPatchDelete {
		state.ClearForContext(ctx, plan.AbsPath)
		return
	}
	if plan.Spec.Operation != applyPatchCreate && !plan.PriorFullVisible {
		state.ClearForContext(ctx, plan.AbsPath)
		return
	}
	info, err := os.Stat(plan.AbsPath)
	if err != nil {
		state.ClearForContext(ctx, plan.AbsPath)
		return
	}
	totalLines := readStateTotalLines(plan.After)
	coverage, _ := readObservationCoverage(1, totalLines, totalLines)
	state.SetForContext(ctx, plan.AbsPath, ReadFileEntry{
		TimestampMs: info.ModTime().UnixMilli(), MtimeNs: info.ModTime().UnixNano(),
		TotalBytes: info.Size(), ContentDigest: plan.AfterDigest, FileIdentity: info,
		TotalLines: totalLines, Coverage: coverage, CoverageComplete: true, FullSnapshot: true,
		Content: plan.After, LastTool: "ApplyPatch", Encoding: plan.Encoding,
		BOM: append([]byte(nil), plan.BOM...),
	})
}

func (t *ApplyPatchTool) activateSkillsForPatchedPath(ctx context.Context, absPath string) {
	if t == nil || t.SkillManager == nil || fileReadSimpleMode() {
		return
	}
	dirs := DiscoverSkillDirsForPaths([]string{absPath})
	if len(dirs) > 0 {
		addSkillDirectoriesForExecution(ctx, t.SkillManager, dirs)
	}
	activateConditionalPathForExecution(ctx, t.SkillManager, absPath)
}

func applyPatchParseResult(reason, path string) types.ToolResult {
	return applyPatchToolError(i18n.KeyToolApplyPatchParseFailed, fileErrorApplyPatchParse, path, false, reason)
}

func applyPatchConflictResult(path string) types.ToolResult {
	result := applyPatchToolError(i18n.KeyToolApplyPatchConflict, fileErrorApplyPatchConflict, path, true, "")
	if data, ok := result.Data.(types.ToolErrorData); ok && path != "" {
		data.Retry = inspectFileRetry(path, nil)
		result.Data = data
	}
	return result
}

func applyPatchReadRequiredResult(path string, required []ReadLineRange) types.ToolResult {
	result := applyPatchToolError(i18n.KeyToolApplyPatchReadRequired, fileErrorApplyPatchReadRequired, path, true, "")
	if data, ok := result.Data.(types.ToolErrorData); ok {
		data.Retry = inspectFileRetry(path, required)
		result.Data = data
	}
	return result
}

func applyPatchPlacementResult(path, code string) types.ToolResult {
	result := applyPatchToolError(i18n.KeyToolApplyPatchConflict, code, path, true, "")
	if data, ok := result.Data.(types.ToolErrorData); ok && path != "" {
		data.Retry = inspectFileRetry(path, nil)
		result.Data = data
	}
	return result
}

func applyPatchPlacementCode(reason string) string {
	switch reason {
	case "anchor_missing":
		return fileErrorApplyPatchAnchorMissing
	case "anchor_ambiguous":
		return fileErrorApplyPatchAnchorAmbiguous
	case "position_mismatch":
		return fileErrorApplyPatchPosition
	case "eof_mismatch":
		return fileErrorApplyPatchEOF
	default:
		return fileErrorApplyPatchConflict
	}
}

func applyPatchPermissionResult(reason, path string) types.ToolResult {
	return applyPatchToolError(i18n.KeyToolApplyPatchPermissionDenied, fileErrorApplyPatchPermission, path, false, reason)
}

func applyPatchCommitResult(rollbackFailed bool) types.ToolResult {
	reason := "commit_failed"
	if rollbackFailed {
		reason = "rollback_failed"
	}
	return applyPatchToolError(i18n.KeyToolApplyPatchCommitFailed, fileErrorApplyPatchCommit, "", false, reason)
}

func applyPatchToolError(key i18n.Key, code, path string, retryable bool, reason string) types.ToolResult {
	var content string
	switch key {
	case i18n.KeyToolApplyPatchPlanMode:
		content = toolRuntimeText(key)
	case i18n.KeyToolApplyPatchParseFailed:
		content = toolRuntimeFormat(key, reason, path)
	case i18n.KeyToolApplyPatchConflict, i18n.KeyToolApplyPatchReadRequired:
		content = toolRuntimeFormat(key, path)
	case i18n.KeyToolApplyPatchPermissionDenied:
		content = toolRuntimeFormat(key, path, reason)
	case i18n.KeyToolApplyPatchCommitFailed:
		content = toolRuntimeFormat(key, reason)
	default:
		content = toolRuntimeText(key)
	}
	failureReason := reason
	if failureReason == "" {
		failureReason = code
	}
	return types.ToolResult{
		Content: content,
		Data: types.ToolErrorData{
			Schema: "tool_error/v1", Code: code, Retryable: retryable, Path: path,
		},
		// Stable protocol reason used by proof-preserving microcompaction. It
		// deliberately excludes the target path and localized public copy.
		Metadata: map[string]string{"apply_patch.failure_reason": failureReason},
		IsError:  true, Outcome: types.ToolOutcomeFailed,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete},
	}
}

func applyPatchRevisionReceiptResult() types.ToolResult {
	return types.ToolResult{
		Content: toolRuntimeText(i18n.KeyToolApplyPatchRevisionReceiptFailed),
		Data: types.ToolErrorData{
			Schema: "tool_error/v1", Code: fileErrorApplyPatchCommit, Retryable: true,
		},
		Metadata: map[string]string{"apply_patch.failure_reason": "revision_receipt_failed"},
		IsError:  true, Outcome: types.ToolOutcomePartial,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete},
	}
}

func applyPatchSuccessResult(plans []applyPatchPlan, revision workspacerevision.Receipt) (types.ToolResult, error) {
	result := ApplyPatchResult{
		Status:       "success",
		ChangedPaths: make([]string, 0, len(plans)),
		Files:        make([]ApplyPatchFileStat, 0, len(plans)),
		Summary:      ApplyPatchSummary{Files: len(plans)},
		revision:     revision,
	}
	for _, plan := range plans {
		stat := ApplyPatchFileStat{
			Path: plan.Spec.Path, Operation: string(plan.Spec.Operation), Hunks: len(plan.Spec.Hunks),
			Additions: plan.Additions, Deletions: plan.Deletions,
		}
		result.ChangedPaths = append(result.ChangedPaths, plan.Spec.Path)
		result.Files = append(result.Files, stat)
		result.Summary.Hunks += stat.Hunks
		result.Summary.Additions += stat.Additions
		result.Summary.Deletions += stat.Deletions
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return types.ToolResult{}, err
	}
	return types.ToolResult{
		Content: string(raw), Data: result, Outcome: types.ToolOutcomeSucceeded,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete},
	}, nil
}

func (t *ApplyPatchTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := data.(ApplyPatchResult)
	if !ok {
		return types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
			Content: toolRuntimeText(i18n.KeyToolApplyPatchInvalidResult), IsError: true,
		}
	}
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
		Content: toolRuntimeFormat(
			i18n.KeyToolApplyPatchSuccess,
			result.Summary.Files, result.Summary.Additions, result.Summary.Deletions,
		),
		Data: result,
	}
}

func commitApplyPatchPlan(plan *applyPatchPlan) error {
	if plan == nil {
		return fs.ErrInvalid
	}
	switch plan.Spec.Operation {
	case applyPatchCreate:
		if err := secureio.PublishFileAtomicallyNoReplace(plan.StagePath, plan.AbsPath); err != nil {
			return err
		}
	case applyPatchUpdate:
		if err := secureio.ReplaceFileAtomically(plan.StagePath, plan.AbsPath); err != nil {
			return err
		}
		plan.StagePath = ""
	case applyPatchDelete:
		if err := os.Remove(plan.AbsPath); err != nil {
			return err
		}
	default:
		return fs.ErrInvalid
	}
	plan.Committed = true
	if err := secureio.SyncRuntimeDirectory(filepath.Dir(plan.AbsPath)); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return err
	}
	return nil
}

func stageApplyPatchPlan(plan *applyPatchPlan) error {
	if plan == nil {
		return fs.ErrInvalid
	}
	if plan.Spec.Operation != applyPatchDelete {
		stage, err := writeApplyPatchTemp(filepath.Dir(plan.AbsPath), ".luban-patch-stage-*", plan.AfterRaw, plan.Mode)
		if err != nil {
			return err
		}
		plan.StagePath = stage
	}
	if plan.Exists {
		backup, err := writeApplyPatchTemp(filepath.Dir(plan.AbsPath), ".luban-patch-backup-*", plan.BeforeRaw, plan.Mode)
		if err != nil {
			return err
		}
		plan.BackupPath = backup
	}
	return nil
}

func writeApplyPatchTemp(dir, pattern string, content []byte, mode os.FileMode) (string, error) {
	temporary, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := temporary.Name()
	success := false
	defer func() {
		if !success {
			_ = temporary.Close()
			_ = os.Remove(path)
		}
	}()
	if err := secureio.WriteAll(temporary, content); err != nil {
		return "", err
	}
	if err := temporary.Chmod(mode); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	success = true
	return path, nil
}

func rollbackApplyPatchPlans(plans []*applyPatchPlan) bool {
	succeeded := true
	for index := len(plans) - 1; index >= 0; index-- {
		plan := plans[index]
		if plan == nil || !plan.Committed {
			continue
		}
		var err error
		switch plan.Spec.Operation {
		case applyPatchCreate:
			if snapshot, readErr := readEditTarget(plan.AbsPath, nil); readErr != nil || snapshot.ContentDigest != plan.AfterDigest {
				err = errEditSnapshotCASMismatch
			} else {
				err = os.Remove(plan.AbsPath)
			}
		case applyPatchUpdate:
			if snapshot, readErr := readEditTarget(plan.AbsPath, nil); readErr != nil || snapshot.ContentDigest != plan.AfterDigest {
				err = errEditSnapshotCASMismatch
			} else {
				err = secureio.ReplaceFileAtomically(plan.BackupPath, plan.AbsPath)
				if err == nil {
					plan.BackupPath = ""
				}
			}
		case applyPatchDelete:
			if _, statErr := os.Lstat(plan.AbsPath); statErr == nil || !os.IsNotExist(statErr) {
				err = errEditSnapshotCASMismatch
			} else {
				err = secureio.PublishFileAtomicallyNoReplace(plan.BackupPath, plan.AbsPath)
			}
		}
		if err != nil {
			succeeded = false
			if plan.BackupPath != "" {
				plan.KeepBackup = true
			}
		} else if syncErr := secureio.SyncRuntimeDirectory(filepath.Dir(plan.AbsPath)); syncErr != nil && !errors.Is(syncErr, fs.ErrInvalid) {
			succeeded = false
		}
		plan.Committed = false
	}
	return succeeded
}

func cleanupApplyPatchTemps(plans []applyPatchPlan) {
	for _, plan := range plans {
		if plan.StagePath != "" {
			_ = os.Remove(plan.StagePath)
		}
		if plan.BackupPath != "" && !plan.KeepBackup {
			_ = os.Remove(plan.BackupPath)
		}
	}
}

func createApplyPatchDirectories(root string, plans []applyPatchPlan) ([]string, error) {
	missing := make(map[string]struct{})
	for _, plan := range plans {
		current := filepath.Dir(plan.AbsPath)
		for toolbase.CanonicalPath(current) != toolbase.CanonicalPath(root) {
			info, err := os.Lstat(current)
			if err == nil {
				if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
					return nil, fs.ErrInvalid
				}
				break
			}
			if !os.IsNotExist(err) {
				return nil, err
			}
			missing[current] = struct{}{}
			parent := filepath.Dir(current)
			if parent == current {
				return nil, fs.ErrInvalid
			}
			current = parent
		}
	}
	directories := make([]string, 0, len(missing))
	for directory := range missing {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(i, j int) bool {
		depthI := strings.Count(filepath.Clean(directories[i]), string(os.PathSeparator))
		depthJ := strings.Count(filepath.Clean(directories[j]), string(os.PathSeparator))
		if depthI == depthJ {
			return directories[i] < directories[j]
		}
		return depthI < depthJ
	})
	created := make([]string, 0, len(directories))
	for _, directory := range directories {
		if err := os.Mkdir(directory, 0o755); err != nil {
			if !os.IsExist(err) {
				removeEmptyApplyPatchDirectories(created)
				return nil, err
			}
			info, statErr := os.Lstat(directory)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				removeEmptyApplyPatchDirectories(created)
				return nil, fs.ErrInvalid
			}
			continue
		}
		created = append(created, directory)
	}
	return created, nil
}

func removeEmptyApplyPatchDirectories(directories []string) {
	for index := len(directories) - 1; index >= 0; index-- {
		_ = os.Remove(directories[index])
	}
}
