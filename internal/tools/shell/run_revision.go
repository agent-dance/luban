package shell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

type runRevisionSafeResult struct {
	result            types.ToolResult
	receipt           workspacerevision.Receipt
	certified         bool
	mutationCommitted bool
	verificationRan   bool
	reason            string
}

type runSourceSnapshot struct {
	root    string
	entries map[string][sha256.Size]byte
}

func configureRunRevisionSafety(plan *compiledRunPlan, workspaceRoot, configuredRoot string) {
	if plan == nil || len(plan.steps) == 0 {
		return
	}
	managedRoot := normalizedRunManagedRoot(workspaceRoot, configuredRoot)
	if managedRoot == "" {
		return
	}
	seenVerification := false
	formatterCount := 0
	for index := range plan.steps {
		step := &plan.steps[index]
		step.formatterWrites = declaredGofmtWrites(*step, workspaceRoot)
		switch {
		case len(step.formatterWrites) > 0:
			if seenVerification {
				return
			}
			formatterCount++
		case step.verificationKind != runVerificationNone:
			seenVerification = true
		case step.readOnly:
			// Read-only observations may share a revision-safe graph with the
			// classified verifier. The whole workspace is snapshotted around the
			// graph, so an incorrect command classification still fails closed.
		default:
			return
		}
	}
	if !seenVerification {
		return
	}
	if formatterCount > 0 {
		for index, step := range plan.steps {
			if index == 0 {
				if len(step.dependsOn) != 0 {
					return
				}
				continue
			}
			if len(step.dependsOn) != 1 || step.dependsOn[0] != index-1 {
				return
			}
		}
	}
	for index := range plan.steps {
		plan.steps[index].verificationSafe = plan.steps[index].verificationKind != runVerificationNone
		plan.steps[index].managedRoot = managedRoot
	}
	plan.formatterCount = formatterCount
	plan.revisionSafe = true
}

func normalizedRunManagedRoot(workspaceRoot, configuredRoot string) string {
	if strings.TrimSpace(configuredRoot) == "" {
		return ""
	}
	workspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return ""
	}
	base, err := filepath.Abs(configuredRoot)
	if err != nil || pathWithin(base, workspace) {
		return ""
	}
	digest := sha256.Sum256([]byte(filepath.Clean(workspace)))
	root := filepath.Join(filepath.Clean(base), "workspace-"+hex.EncodeToString(digest[:8]))
	if pathWithin(root, workspace) {
		return ""
	}
	return root
}

func declaredGofmtWrites(step compiledRunStep, workspaceRoot string) []string {
	if step.useShell || len(step.argv) < 3 || step.argv[0] != "gofmt" {
		return nil
	}
	write := false
	paths := make([]string, 0, len(step.argv)-2)
	for _, argument := range step.argv[1:] {
		switch argument {
		case "-w":
			write = true
		case "-s":
			continue
		default:
			if strings.HasPrefix(argument, "-") {
				return nil
			}
			absolute := argument
			if !filepath.IsAbs(absolute) {
				absolute = filepath.Join(step.cwd, argument)
			}
			absolute, _ = filepath.Abs(absolute)
			absolute = filepath.Clean(absolute)
			if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
				absolute = filepath.Clean(resolved)
			}
			info, err := os.Stat(absolute)
			if err != nil || !info.Mode().IsRegular() || !pathWithin(absolute, workspaceRoot) {
				return nil
			}
			paths = append(paths, absolute)
		}
	}
	if !write || len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	return paths
}

func executeRevisionSafeRunPlan(ctx context.Context, scope bashExecutionScope, plan *compiledRunPlan, receipt workspacerevision.Receipt) runRevisionSafeResult {
	logicalStarted := time.Now()
	if plan == nil || !plan.revisionSafe || scope.workspaceRevisions == nil || !receipt.Valid() {
		return runRevisionSafeResult{result: executeRunPlan(ctx, scope, plan)}
	}
	if plan.formatterCount == 0 {
		before, err := captureRunSourceSnapshot(scope.cwd)
		if err != nil {
			return runRevisionSafeResult{result: executeRunPlan(ctx, scope, plan), reason: "source_snapshot_failed"}
		}
		result := executeRunPlan(ctx, scope, plan)
		after, afterErr := captureRunSourceSnapshot(scope.cwd)
		changes, changeErr := changedRunSourcePaths(before, after)
		certified := afterErr == nil && changeErr == nil && len(changes) == 0 && scope.workspaceRevisions.Validate(receipt) == nil
		reason := ""
		if !certified {
			reason = "source_changed_or_revision_invalid"
		}
		return runRevisionSafeResult{
			result: result, receipt: receipt, certified: certified,
			verificationRan: runResultExecutedVerification(plan, result),
			reason:          reason,
		}
	}
	return executeFormatterVerificationChain(ctx, scope, plan, receipt, logicalStarted)
}

func executeFormatterVerificationChain(ctx context.Context, scope bashExecutionScope, plan *compiledRunPlan, receipt workspacerevision.Receipt, logicalStarted time.Time) runRevisionSafeResult {
	executions := make([]runStepExecution, len(plan.steps))
	changed := make(map[string]struct{})
	unsafeWrite := false
	reason := ""
	markUnsafe := func(value string) {
		unsafeWrite = true
		if reason == "" {
			reason = value
		}
	}
	failed := false
	stepBudget := max(1, plan.maxChars/len(plan.steps))

	verificationStart := len(plan.steps)
	for index, step := range plan.steps {
		if step.verificationKind != runVerificationNone {
			verificationStart = index
			break
		}
	}
	for index := 0; index < verificationStart; index++ {
		step := plan.steps[index]
		if failed {
			executions[index] = skippedRunStep(step)
			continue
		}
		before, beforeErr := captureRunSourceSnapshot(scope.cwd)
		if beforeErr != nil {
			markUnsafe("source_snapshot_failed")
		}
		executions[index] = executeRunStep(ctx, scope, step, plan.headLines, plan.tailLines, stepBudget, logicalStarted)
		after, afterErr := captureRunSourceSnapshot(scope.cwd)
		actual, changeErr := changedRunSourcePaths(before, after)
		snapshotFailed := beforeErr != nil || afterErr != nil || changeErr != nil
		switch {
		case len(step.formatterWrites) > 0:
			if snapshotFailed || !runWritesWithinDeclaration(actual, step.formatterWrites) {
				markUnsafe("formatter_write_set_exceeded")
			}
			for _, path := range actual {
				changed[path] = struct{}{}
			}
		case snapshotFailed || len(actual) != 0:
			// A step classified as a read-only pre-verification observation is
			// allowed in this phase, but it may not manufacture source changes.
			// Keep the graph fail-closed if classification and reality diverge.
			markUnsafe("observation_changed_source")
		}
		if runStepFailed(executions[index].output.Status) {
			failed = true
		}
	}

	changedPaths := make([]string, 0, len(changed))
	for path := range changed {
		changedPaths = append(changedPaths, path)
	}
	sort.Strings(changedPaths)
	mutationCommitted := false
	if len(changedPaths) > 0 && !unsafeWrite {
		var err error
		receipt, err = scope.workspaceRevisions.Commit(scope.cwd, changedPaths)
		if err != nil {
			markUnsafe("formatter_revision_commit_failed")
		} else {
			mutationCommitted = true
		}
	}

	verificationBefore, snapshotErr := captureRunSourceSnapshot(scope.cwd)
	if snapshotErr != nil {
		markUnsafe("verification_snapshot_failed")
	}
	verificationRan := false
	for index := verificationStart; index < len(plan.steps); index++ {
		step := plan.steps[index]
		if failed {
			executions[index] = skippedRunStep(step)
			continue
		}
		executions[index] = executeRunStep(ctx, scope, step, plan.headLines, plan.tailLines, stepBudget, logicalStarted)
		if step.verificationKind != runVerificationNone {
			verificationRan = true
		}
		if runStepFailed(executions[index].output.Status) {
			failed = true
		}
	}
	verificationAfter, afterErr := captureRunSourceSnapshot(scope.cwd)
	verificationChanges, changeErr := changedRunSourcePaths(verificationBefore, verificationAfter)
	if afterErr != nil || changeErr != nil || len(verificationChanges) != 0 {
		markUnsafe("verification_changed_source")
	}
	result := buildRunResult(executions)
	revisionValid := scope.workspaceRevisions.Validate(receipt) == nil
	if !revisionValid {
		markUnsafe("final_revision_invalid")
	}
	certified := !unsafeWrite && revisionValid
	return runRevisionSafeResult{
		result: result, receipt: receipt, certified: certified,
		mutationCommitted: mutationCommitted, verificationRan: verificationRan,
		reason: reason,
	}
}

func runResultExecutedVerification(plan *compiledRunPlan, result types.ToolResult) bool {
	output, ok := result.Data.(*RunOutput)
	if !ok || output == nil {
		return false
	}
	for index, step := range plan.steps {
		if step.verificationKind != runVerificationNone && index < len(output.Steps) && output.Steps[index].Status != runStatusSkipped {
			return true
		}
	}
	return false
}

func runWritesWithinDeclaration(actual, declared []string) bool {
	allowed := make(map[string]struct{}, len(declared))
	for _, path := range declared {
		allowed[filepath.Clean(path)] = struct{}{}
	}
	for _, path := range actual {
		if _, ok := allowed[filepath.Clean(path)]; !ok {
			return false
		}
	}
	return true
}

func captureRunSourceSnapshot(root string) (runSourceSnapshot, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || strings.TrimSpace(root) == "" {
		return runSourceSnapshot{}, errors.New("invalid workspace")
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		absolute = filepath.Clean(resolved)
	}
	if paths, gitBacked, gitErr := gitRunSnapshotPaths(absolute); gitBacked {
		if gitErr != nil {
			return runSourceSnapshot{}, gitErr
		}
		return captureSelectedRunSourcePaths(absolute, paths)
	}
	return captureWalkedRunSourceSnapshot(absolute)
}

func gitRunSnapshotPaths(root string) ([]string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gitArgs := []string{"-C", root, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "submodule.recurse=false"}
	rootCommand := exec.CommandContext(ctx, "git", append(gitArgs, "rev-parse", "--show-toplevel")...)
	rootCommand.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	repositoryOutput, err := rootCommand.Output()
	if err != nil {
		return nil, false, nil
	}
	repositoryRoot := filepath.Clean(strings.TrimSpace(string(repositoryOutput)))
	if resolved, resolveErr := filepath.EvalSymlinks(repositoryRoot); resolveErr == nil {
		repositoryRoot = filepath.Clean(resolved)
	}
	if !pathWithin(root, repositoryRoot) {
		return nil, true, workspacerevision.ErrRevisionChanged
	}
	relativeRoot, err := filepath.Rel(repositoryRoot, root)
	if err != nil {
		return nil, true, err
	}
	pathspec := filepath.ToSlash(relativeRoot)
	if pathspec == "" {
		pathspec = "."
	}
	list := func(mode ...string) ([]byte, error) {
		listArgs := append(append([]string{}, gitArgs...), "--literal-pathspecs", "ls-files", "-z")
		listArgs = append(listArgs, mode...)
		listArgs = append(listArgs, "--full-name", "--", pathspec)
		listCommand := exec.CommandContext(ctx, "git", listArgs...)
		listCommand.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
		return listCommand.Output()
	}
	tracked, err := list("--cached")
	if err != nil {
		return nil, true, err
	}
	untracked, err := list("--others", "--exclude-standard")
	if err != nil {
		return nil, true, err
	}
	paths := make([]string, 0, bytes.Count(tracked, []byte{0})+bytes.Count(untracked, []byte{0}))
	seen := make(map[string]struct{})
	appendPaths := func(output []byte, excludeGenerated bool) error {
		for _, raw := range bytes.Split(output, []byte{0}) {
			if len(raw) == 0 {
				continue
			}
			path := filepath.Clean(filepath.Join(repositoryRoot, filepath.FromSlash(string(raw))))
			if !pathWithin(path, root) {
				return workspacerevision.ErrRevisionChanged
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			relative = filepath.ToSlash(relative)
			if excludeGenerated && isRunGeneratedPath(relative) {
				continue
			}
			if _, ok := seen[relative]; ok {
				continue
			}
			seen[relative] = struct{}{}
			paths = append(paths, relative)
		}
		return nil
	}
	if err := appendPaths(tracked, false); err != nil {
		return nil, true, err
	}
	if err := appendPaths(untracked, true); err != nil {
		return nil, true, err
	}
	sort.Strings(paths)
	return paths, true, nil
}

var runGeneratedTopLevelDirectories = map[string]struct{}{
	".gradle": {}, ".luban-build": {}, ".next": {}, "build": {}, "coverage": {},
	"dist": {}, "node_modules": {}, "out": {}, "target": {},
}

func isRunGeneratedPath(relative string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(relative), "/")
	_, generated := runGeneratedTopLevelDirectories[first]
	return generated || strings.HasPrefix(first, "build-") || strings.HasPrefix(first, "build_") ||
		strings.HasPrefix(first, ".luban-build-")
}

func captureSelectedRunSourcePaths(root string, paths []string) (runSourceSnapshot, error) {
	entries := make(map[string][sha256.Size]byte, len(paths))
	for _, relative := range paths {
		digest, err := digestRunSourcePath(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return runSourceSnapshot{}, err
		}
		entries[relative] = digest
	}
	return runSourceSnapshot{root: filepath.Clean(root), entries: entries}, nil
}

func digestRunSourcePath(path string) ([sha256.Size]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return sha256.Sum256([]byte("missing")), nil
	}
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(info.Mode().String()))
	switch {
	case info.Mode().IsRegular():
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return [sha256.Size]byte{}, readErr
		}
		current, statErr := os.Lstat(path)
		if statErr != nil || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) || current.Mode() != info.Mode() {
			return [sha256.Size]byte{}, workspacerevision.ErrRevisionChanged
		}
		_, _ = h.Write(content)
	case info.Mode()&os.ModeSymlink != 0:
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return [sha256.Size]byte{}, readErr
		}
		_, _ = h.Write([]byte(target))
	case info.IsDir():
	default:
		return [sha256.Size]byte{}, workspacerevision.ErrRevisionChanged
	}
	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	return digest, nil
}

func captureWalkedRunSourceSnapshot(absolute string) (runSourceSnapshot, error) {
	entries := make(map[string][sha256.Size]byte)
	err := filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == absolute {
			return nil
		}
		relative, relErr := filepath.Rel(absolute, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() && (relative == ".git" || isRunGeneratedPath(filepath.ToSlash(relative))) {
			return filepath.SkipDir
		}
		digest, digestErr := digestRunSourcePath(path)
		if digestErr != nil {
			return digestErr
		}
		entries[filepath.ToSlash(relative)] = digest
		return nil
	})
	if err != nil {
		return runSourceSnapshot{}, err
	}
	return runSourceSnapshot{root: filepath.Clean(absolute), entries: entries}, nil
}

func changedRunSourcePaths(before, after runSourceSnapshot) ([]string, error) {
	if before.root == "" || before.root != after.root || before.entries == nil || after.entries == nil {
		return nil, workspacerevision.ErrRevisionChanged
	}
	changed := make(map[string]struct{})
	for path, digest := range before.entries {
		if current, ok := after.entries[path]; !ok || current != digest {
			changed[path] = struct{}{}
		}
	}
	for path, digest := range after.entries {
		if previous, ok := before.entries[path]; !ok || previous != digest {
			changed[path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(changed))
	for relative := range changed {
		paths = append(paths, filepath.Join(before.root, filepath.FromSlash(relative)))
	}
	sort.Strings(paths)
	return paths, nil
}

func runVerificationEnvironment(scope bashExecutionScope, step compiledRunStep) (sandbox.EnvironmentSnapshot, string, error) {
	if step.managedRoot == "" {
		return scope.environment, "", nil
	}
	directories := map[string]string{
		"cache": filepath.Join(step.managedRoot, "cache"),
		"tmp":   filepath.Join(step.managedRoot, "tmp"),
		"go":    filepath.Join(step.managedRoot, "go-build"),
		"mod":   filepath.Join(step.managedRoot, "go-mod"),
		"cargo": filepath.Join(step.managedRoot, "cargo-target"),
		"py":    filepath.Join(step.managedRoot, "python-cache"),
	}
	for _, directory := range directories {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return sandbox.EnvironmentSnapshot{}, "", err
		}
	}
	overrides := map[string]string{
		"GOCACHE": directories["go"], "GOMODCACHE": directories["mod"], "GOTMPDIR": directories["tmp"],
		"CARGO_TARGET_DIR": directories["cargo"], "PYTHONPYCACHEPREFIX": directories["py"],
		"PYTEST_ADDOPTS": "-p no:cacheprovider", "TMPDIR": directories["tmp"], "TMP": directories["tmp"], "TEMP": directories["tmp"],
		"XDG_CACHE_HOME": directories["cache"], "NPM_CONFIG_CACHE": filepath.Join(directories["cache"], "npm"),
	}
	return scope.environment.WithOverrides(overrides), step.managedRoot, nil
}
