package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func p0SedCommand(script, target string) string {
	if runtime.GOOS == "darwin" {
		return "sed -i '' '" + script + "' " + target
	}
	return "sed -i '" + script + "' " + target
}

func TestP0BSedCompoundBindsPhysicalEffectiveCWD(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "victim.txt")
	p0bWriteFixture(t, target, "alpha\n")

	direct := p0SedCommand("s/alpha/beta/", "victim.txt")
	commands := map[string]string{
		"cd-and":   "cd sub && " + direct,
		"subshell": "(cd sub && " + direct + ")",
		"env-c":    "env -C sub " + direct,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			execution := analyzeSedEditExecution(command, root)
			if !execution.HasInPlace || !execution.EvidenceSafe || len(execution.Invocations) != 1 {
				t.Fatalf("execution plan is not evidence-safe: %+v", execution)
			}
			invocation := execution.Invocations[0]
			if invocation.EffectiveCWD != canonicalPathForComparison(sub) || len(invocation.Targets) != 1 ||
				invocation.Targets[0] != filepath.Clean(target) {
				t.Fatalf("wrong effective-CWD binding: %+v", invocation)
			}
			locks := sedEditExecutionMutationTargets(execution)
			if len(locks) != 1 || locks[0] != canonicalFileEditLockPath(target) {
				t.Fatalf("wrong canonical lock targets: %v", locks)
			}
			if err := validateSedEditExecutionReadState(context.Background(), execution, NewReadFileState()); err == nil ||
				!strings.Contains(err.Error(), displayPathForUser(target)) {
				t.Fatalf("unread physical target did not fail closed: %v", err)
			}
		})
	}
}

func TestP0BSedEffectiveCWDRejectsUnreadRealTarget(t *testing.T) {
	requireBashAvailable(t)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "victim.txt")
	p0bWriteFixture(t, target, "alpha\n")
	state := NewReadFileState()
	tool := &BashTool{
		CWD: root, AllowedDirs: []string{root}, ReadFileState: state, SedValidationEnabled: true,
	}
	command := "cd sub && " + p0SedCommand("s/alpha/forged/", "victim.txt")
	result, err := tool.Execute(context.Background(), map[string]any{"command": command})
	if err != nil || !result.IsError {
		t.Fatalf("unread effective target was not rejected: result=%+v err=%v", result, err)
	}
	raw, readErr := os.ReadFile(target)
	if readErr != nil || string(raw) != "alpha\n" {
		t.Fatalf("rejected command changed real target: content=%q err=%v", raw, readErr)
	}
	if _, exists := state.Get(target); exists {
		t.Fatal("rejected command published evidence for unread target")
	}
}

func TestP0BSedUnprovableSequenceIsApprovalOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	static := p0SedCommand("s/alpha/beta/", "victim.txt")
	commands := map[string]string{
		"unconditional-cd": "cd sub; " + static,
		"preceding-copy":   "cp other.txt victim.txt; " + static,
		"glob-target":      p0SedCommand("s/alpha/beta/", "*.txt"),
		"script-file":      "sed -i -f edits.sed victim.txt",
		"conditional":      "if true; then " + static + "; fi",
		"asynchronous":     static + " &",
	}
	policy := (&BashTool{CWD: root, AllowedDirs: []string{root}}).shellPolicyContext(types.ToolRuntimeContext{}, true)
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			decision, execution := analyzeBashCommandWithSedEvidencePolicy(command, policy)
			if !execution.HasInPlace || execution.EvidenceSafe || decision.Disposition != types.PolicyRequiredAsk {
				t.Fatalf("unprovable sed sequence failed open: decision=%+v execution=%+v", decision, execution)
			}
			if targets := sedEditExecutionMutationTargets(execution); len(targets) != 0 {
				t.Fatalf("unprovable sed sequence published lock/evidence targets: %v", targets)
			}
		})
	}
}

func TestP0BSedEffectiveCWDLockCoversValidationExecutionAndRefresh(t *testing.T) {
	requireBashAvailable(t)
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "victim.txt")
	decoy := filepath.Join(root, "victim.txt")
	cdpathRoot := filepath.Join(root, "cdpath")
	cdpathSub := filepath.Join(cdpathRoot, "sub")
	if err := os.MkdirAll(cdpathSub, 0o700); err != nil {
		t.Fatal(err)
	}
	cdpathVictim := filepath.Join(cdpathSub, "victim.txt")
	p0bWriteFixture(t, target, "alpha\n")
	p0bWriteFixture(t, decoy, "decoy\n")
	p0bWriteFixture(t, cdpathVictim, "cdpath\n")
	t.Setenv("CDPATH", cdpathRoot)
	state := NewReadFileState()
	p0bReadEvidence(t, root, target, state)
	p0bReadEvidence(t, root, decoy, state)
	_, releaseEdit, editResult := p0bStartEditAtPrecommitBarrier(t, root, target, "alpha", "beta", state)

	registered := make(chan struct{})
	var signal sync.Once
	tool := &BashTool{
		CWD: root, AllowedDirs: []string{root}, ReadFileState: state, SedValidationEnabled: true,
		sedLockRegisteredForTest: func() { signal.Do(func() { close(registered) }) },
	}
	command := "cd sub && " + p0SedCommand("s/beta/gamma/", "victim.txt")
	bashResult := make(chan types.ToolResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		result, err := tool.Execute(ctx, map[string]any{"command": command})
		if err != nil {
			result = ErrorResponse(err)
		}
		bashResult <- result
	}()
	select {
	case <-registered:
	case <-ctx.Done():
		t.Fatalf("sed did not register the effective-target lock: %v", ctx.Err())
	}
	select {
	case result := <-bashResult:
		t.Fatalf("sed bypassed the lock held by Edit: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseEdit)
	if result := p0bWaitResult(t, editResult); result.IsError {
		t.Fatalf("Edit failed: %+v", result)
	}
	if result := p0bWaitResult(t, bashResult); result.IsError {
		t.Fatalf("sed failed after acquiring effective-target lock: %+v", result)
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != "gamma\n" {
		t.Fatalf("serialized effective target content=%q err=%v", raw, err)
	}
	decoyRaw, err := os.ReadFile(decoy)
	if err != nil || string(decoyRaw) != "decoy\n" {
		t.Fatalf("initial-CWD decoy was mutated: content=%q err=%v", decoyRaw, err)
	}
	cdpathRaw, err := os.ReadFile(cdpathVictim)
	if err != nil || string(cdpathRaw) != "cdpath\n" {
		t.Fatalf("inherited CDPATH redirected the proven CWD: content=%q err=%v", cdpathRaw, err)
	}
	entry, ok := state.Get(target)
	if !ok || entry.LastTool != "Bash" || entry.ContentDigest != fileContentDigest(raw) || entry.FileIdentity == nil {
		t.Fatalf("effective target did not receive strong post-sed evidence: %+v", entry)
	}
}

func TestP0BSedPrecedingMutationRequiresApprovalAndPublishesNoEvidence(t *testing.T) {
	requireBashAvailable(t)
	root := t.TempDir()
	victim := filepath.Join(root, "victim.txt")
	other := filepath.Join(root, "other.txt")
	p0bWriteFixture(t, victim, "alpha\n")
	p0bWriteFixture(t, other, "bravo\n")
	fixed := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(victim, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(other, fixed, fixed); err != nil {
		t.Fatal(err)
	}
	state := NewReadFileState()
	p0bReadEvidence(t, root, victim, state)
	before, ok := state.Get(victim)
	if !ok || before.ContentDigest == "" || before.FileIdentity == nil {
		t.Fatalf("missing strong initial evidence: %+v", before)
	}
	command := "cp -p other.txt victim.txt; " + p0SedCommand("s/bravo/charl/", "victim.txt")
	policyContext := (&BashTool{CWD: root, AllowedDirs: []string{root}}).shellPolicyContext(types.ToolRuntimeContext{}, true)
	decision, execution := analyzeBashCommandWithSedEvidencePolicy(command, policyContext)
	if decision.Disposition != types.PolicyRequiredAsk || decision.Risk != types.PolicyRiskUnrestrictedCode ||
		!execution.HasInPlace || execution.EvidenceSafe {
		t.Fatalf("preceding replacement was not approval-only: decision=%+v execution=%+v", decision, execution)
	}

	tool := &BashTool{
		CWD: root, AllowedDirs: []string{root}, ReadFileState: state, SedValidationEnabled: true,
	}
	direct, err := tool.Execute(context.Background(), map[string]any{"command": command})
	if err != nil || !direct.IsError {
		t.Fatalf("direct execution bypassed RequiredAsk: result=%+v err=%v", direct, err)
	}
	if raw, readErr := os.ReadFile(victim); readErr != nil || string(raw) != "alpha\n" {
		t.Fatalf("denied command changed victim: content=%q err=%v", raw, readErr)
	}

	reg := registry.New()
	reg.Register(tool)
	input := map[string]any{"command": command}
	request := types.ToolPermissionRequest{
		SessionID: "sed-session", TurnID: "sed-turn", ToolUseID: "sed-tool", ApprovalEpoch: "sed-epoch",
	}
	preflight, err := reg.CheckToolPermissions(context.Background(), "Bash", input, request)
	if err != nil || preflight.Behavior != types.PermissionBehaviorAsk || !preflight.Required || preflight.PermissionGrant == "" {
		t.Fatalf("missing mandatory sed approval: result=%+v err=%v", preflight, err)
	}
	token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, "Bash", input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	if token == "" {
		t.Fatal("approved sed command did not receive an execution grant")
	}
	approved := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	result, err := reg.ExecuteToolWithError(approved, "Bash", input)
	if err != nil || result.IsError {
		t.Fatalf("approved sed command failed: result=%+v err=%v", result, err)
	}
	raw, readErr := os.ReadFile(victim)
	if readErr != nil || string(raw) != "charl\n" {
		t.Fatalf("approved compound result=%q err=%v", raw, readErr)
	}
	after, ok := state.Get(victim)
	if !ok || after.LastTool == "Bash" || after.ContentDigest != before.ContentDigest ||
		after.FileIdentity == nil || !os.SameFile(after.FileIdentity, before.FileIdentity) {
		t.Fatalf("approval-only command published or rewrote Read evidence: before=%+v after=%+v", before, after)
	}
	edit := &FileEditTool{AllowedDirs: []string{root}, ReadState: state}
	editResult, err := edit.Execute(context.Background(), map[string]any{
		"file_path": victim, "old_string": "charl", "new_string": "forged",
	})
	data, dataOK := editResult.Data.(types.ToolErrorData)
	if err != nil || !editResult.IsError || !dataOK || data.Code != fileErrorSnapshotStale {
		t.Fatalf("same-stat compound result authorized Edit: result=%+v data=%+v err=%v", editResult, editResult.Data, err)
	}
}

func TestP0BSedCompatibilityTrackerRejectsSameStatDigestChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same-stat.txt")
	p0bWriteFixture(t, path, "alpha\n")
	tracker := NewSedReadStateTracker()
	tracker.RecordRead(path)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	p0bWriteFixture(t, path, "bravo\n")
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Skip("filesystem cannot construct a same-size/same-mtime fixture")
	}
	if tracker.HasFresh(path) {
		t.Fatal("mtime-compatible digest change retained weak sed authority")
	}
}
