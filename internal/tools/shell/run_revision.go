package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
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
	for index := range plan.steps {
		step := &plan.steps[index]
		step.formatterWrites = declaredGofmtWrites(*step, workspaceRoot)
		switch {
		case len(step.formatterWrites) > 0:
			if seenVerification {
				return
			}
			plan.formatterCount++
		case step.verificationKind != runVerificationNone:
			seenVerification = true
			step.verificationSafe = true
		default:
			return
		}
		step.managedRoot = managedRoot
	}
	if !seenVerification {
		return
	}
	if plan.formatterCount > 0 {
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

	for index := 0; index < plan.formatterCount; index++ {
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
		if beforeErr != nil || afterErr != nil || changeErr != nil || !runWritesWithinDeclaration(actual, step.formatterWrites) {
			markUnsafe("formatter_write_set_exceeded")
		}
		for _, path := range actual {
			changed[path] = struct{}{}
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
	for index := plan.formatterCount; index < len(plan.steps); index++ {
		step := plan.steps[index]
		if failed {
			executions[index] = skippedRunStep(step)
			continue
		}
		executions[index] = executeRunStep(ctx, scope, step, plan.headLines, plan.tailLines, stepBudget, logicalStarted)
		verificationRan = true
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
	entries := make(map[string][sha256.Size]byte)
	err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
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
		if relative == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		h := sha256.New()
		_, _ = h.Write([]byte(info.Mode().String()))
		switch {
		case info.Mode().IsRegular():
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			current, statErr := os.Lstat(path)
			if statErr != nil || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) || current.Mode() != info.Mode() {
				return workspacerevision.ErrRevisionChanged
			}
			_, _ = h.Write(content)
		case info.Mode()&os.ModeSymlink != 0:
			target, readErr := os.Readlink(path)
			if readErr != nil {
				return readErr
			}
			_, _ = h.Write([]byte(target))
		case info.IsDir():
		default:
			return workspacerevision.ErrRevisionChanged
		}
		var digest [sha256.Size]byte
		copy(digest[:], h.Sum(nil))
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
