package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	shell "github.com/agent-dance/luban/internal/tools/shell"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func containsOutsideAllowedPath(content, path string) bool {
	return strings.Contains(content, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolPathOutsideAllowed, path))
}

type agentCWDScopeRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

func (p agentCWDScopeRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return p.runtime
}

func TestAgentRuntimeContextRestrictsAllowedDirsToChildCWD(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parent := registry.New()
	parent.SetRuntimeContextProvider(agentCWDScopeRuntimeProvider{runtime: types.ToolRuntimeContext{
		ProjectRoot: parentRoot,
		AllowedDirs: []string{parentRoot},
		DeniedRules: []types.PermissionRuleValue{{ToolName: "Read", RuleContent: "**/.env"}},
	}})

	runtime := (agentRuntimeContextProvider{snapshot: parent.RuntimeContext(), cwd: childRoot}).ToolRuntimeContext()
	if runtime.ProjectRoot != childRoot {
		t.Fatalf("child project root = %q, want %q", runtime.ProjectRoot, childRoot)
	}
	if len(runtime.AllowedDirs) != 1 || filepath.Clean(runtime.AllowedDirs[0]) != filepath.Clean(childRoot) {
		t.Fatalf("child allowed dirs = %v, want only %q", runtime.AllowedDirs, childRoot)
	}
	if len(runtime.DeniedRules) != 1 || runtime.DeniedRules[0].ToolName != "Read" {
		t.Fatalf("child runtime lost inherited permission rules: %+v", runtime.DeniedRules)
	}
}

func TestPinnedAgentRegistryDoesNotFollowParentSessionRuntime(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	parentRuntime := &agentCWDScopeRuntimeProvider{runtime: types.ToolRuntimeContext{
		SessionID: "old-session", ProjectRoot: oldRoot, AllowedDirs: []string{oldRoot}, PermissionMode: "default",
	}}
	parent := registry.New()
	parent.SetRuntimeContextProvider(parentRuntime)
	parentBash := &shell.BashTool{CWD: oldRoot, AllowedDirs: []string{oldRoot}}
	parentRead := &toolfile.FileReadTool{AllowedDirs: []string{oldRoot}, Runtime: parentRuntime, ReadState: toolfile.NewReadFileState()}
	parent.Register(parentBash)
	parent.Register(parentRead)

	child := parent.Clone()
	snapshot := parent.RuntimeContext()
	provider := agentRuntimeContextProvider{snapshot: snapshot, agentID: "child"}
	pinRegistryForAgentRuntime(child, provider, snapshot)
	child.SetRuntimeContextProvider(provider)

	parentRuntime.runtime = types.ToolRuntimeContext{SessionID: "new-session", ProjectRoot: newRoot, AllowedDirs: []string{newRoot}, PermissionMode: "bypassPermissions"}
	parentBash.CWD = newRoot
	parentBash.AllowedDirs = []string{newRoot}

	childRuntime := child.RuntimeContext()
	if childRuntime.SessionID != "old-session" || childRuntime.ProjectRoot != oldRoot || childRuntime.PermissionMode != "default" {
		t.Fatalf("child runtime followed parent switch: %+v", childRuntime)
	}
	childBash, ok := child.Get("Bash").(*shell.BashTool)
	if !ok || childBash == parentBash || childBash.CWD != oldRoot || len(childBash.AllowedDirs) != 1 || childBash.AllowedDirs[0] != oldRoot {
		t.Fatalf("child Bash was not pinned: child=%+v parent=%+v", childBash, parentBash)
	}
	childRead, ok := child.Get("Read").(*toolfile.FileReadTool)
	if !ok || childRead == parentRead || len(childRead.AllowedDirs) != 1 || childRead.AllowedDirs[0] != oldRoot {
		t.Fatalf("child Read was not pinned: child=%+v parent=%+v", childRead, parentRead)
	}
}

func TestAgentCWDReadWriteUsePrivateChildScope(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(parentRoot, "parent.txt"), []byte("parent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childRoot, "child.txt"), []byte("child"), 0o644); err != nil {
		t.Fatal(err)
	}

	parentRuntime := agentCWDScopeRuntimeProvider{runtime: types.ToolRuntimeContext{
		ProjectRoot: parentRoot,
		AllowedDirs: []string{parentRoot},
	}}
	parentRegistry := registry.New()
	parentRegistry.SetRuntimeContextProvider(parentRuntime)
	readState := toolfile.NewReadFileState()
	parentRead := &toolfile.FileReadTool{AllowedDirs: []string{parentRoot}, Runtime: parentRuntime, ReadState: readState}
	parentWrite := &toolfile.FileWriteTool{AllowedDirs: []string{parentRoot}, Runtime: parentRuntime, ReadState: readState}
	childRegistry := registry.New()
	childRegistry.Register(parentRead)
	childRegistry.Register(parentWrite)

	wrapRegistryForAgentCWD(childRegistry, childRoot)
	childRegistry.SetRuntimeContextProvider(agentRuntimeContextProvider{snapshot: parentRegistry.RuntimeContext(), cwd: childRoot})

	readResult, err := childRegistry.Get("Read").Execute(context.Background(), map[string]any{"file_path": "child.txt"})
	if err != nil || readResult.IsError || !strings.Contains(readResult.Content, "child") {
		t.Fatalf("child Read result=%+v err=%v", readResult, err)
	}
	readResult, err = childRegistry.Get("Read").Execute(context.Background(), map[string]any{"file_path": filepath.Join(parentRoot, "parent.txt")})
	if err != nil || !readResult.IsError || !containsOutsideAllowedPath(readResult.Content, filepath.Join(parentRoot, "parent.txt")) {
		t.Fatalf("parent checkout Read result=%+v err=%v", readResult, err)
	}

	childWritePath := filepath.Join(childRoot, "created.txt")
	writeResult, err := childRegistry.Get("Write").Execute(context.Background(), map[string]any{
		"file_path": "created.txt",
		"content":   "from child",
	})
	if err != nil || writeResult.IsError {
		t.Fatalf("child Write result=%+v err=%v", writeResult, err)
	}
	if content, err := os.ReadFile(childWritePath); err != nil || string(content) != "from child" {
		t.Fatalf("child Write content=%q err=%v", content, err)
	}
	parentWritePath := filepath.Join(parentRoot, "must-not-change.txt")
	writeResult, err = childRegistry.Get("Write").Execute(context.Background(), map[string]any{
		"file_path": parentWritePath,
		"content":   "escaped",
	})
	if err != nil || !writeResult.IsError || !containsOutsideAllowedPath(writeResult.Content, parentWritePath) {
		t.Fatalf("parent checkout Write result=%+v err=%v", writeResult, err)
	}
	if _, err := os.Stat(parentWritePath); !os.IsNotExist(err) {
		t.Fatalf("parent checkout was modified, stat err=%v", err)
	}

	if len(parentRead.AllowedDirs) != 1 || parentRead.AllowedDirs[0] != parentRoot {
		t.Fatalf("parent Read scope mutated: %v", parentRead.AllowedDirs)
	}
	if len(parentWrite.AllowedDirs) != 1 || parentWrite.AllowedDirs[0] != parentRoot {
		t.Fatalf("parent Write scope mutated: %v", parentWrite.AllowedDirs)
	}
}

func TestAgentCWDBashRejectsParentCheckoutAbsolutePath(t *testing.T) {
	requireBashAvailable(t)
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parentPath := filepath.Join(parentRoot, "parent.txt")
	if err := os.WriteFile(parentPath, []byte("parent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childRoot, "child.txt"), []byte("child"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := &task07SandboxBackend{}
	parentBash := &shell.BashTool{CWD: parentRoot, AllowedDirs: []string{parentRoot}, Sandbox: backend}
	childRegistry := registry.New()
	childRegistry.Register(parentBash)
	wrapRegistryForAgentCWD(childRegistry, childRoot)
	wrapper, ok := childRegistry.Get("Bash").(*agentCWDBashToolWrapper)
	if !ok {
		t.Fatalf("wrapped Bash = %T", childRegistry.Get("Bash"))
	}
	if len(wrapper.AllowedDirs) != 1 || filepath.Clean(wrapper.AllowedDirs[0]) != filepath.Clean(childRoot) {
		t.Fatalf("child Bash allowed dirs = %v, want only %q", wrapper.AllowedDirs, childRoot)
	}
	if !wrapper.ForceSandbox {
		t.Fatal("isolated child Bash did not force the filesystem sandbox")
	}

	insideResult, err := executeApprovedRegistryToolForTest(t, childRegistry, "Bash", map[string]any{"command": `cat "child.txt"`})
	if err != nil || insideResult.IsError || insideResult.Content != "child" {
		t.Fatalf("child Bash result=%+v err=%v", insideResult, err)
	}
	outsideResult, err := executeApprovedRegistryToolForTest(t, childRegistry, "Bash", map[string]any{"command": "printf escaped > " + parentPath})
	if err != nil || !outsideResult.IsError || !containsOutsideAllowedPath(outsideResult.Content, parentPath) {
		t.Fatalf("parent checkout Bash result=%+v err=%v", outsideResult, err)
	}
	if content, err := os.ReadFile(parentPath); err != nil || string(content) != "parent" {
		t.Fatalf("parent checkout Bash changed file to %q, err=%v", content, err)
	}
	if len(parentBash.AllowedDirs) != 1 || parentBash.AllowedDirs[0] != parentRoot {
		t.Fatalf("parent Bash scope mutated: %v", parentBash.AllowedDirs)
	}
}

func TestAgentCWDOmitsBashWithoutRealFilesystemSandbox(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parentBash := &shell.BashTool{CWD: parentRoot}
	childRegistry := registry.New()
	childRegistry.Register(parentBash)
	wrapRegistryForAgentCWD(childRegistry, childRoot)
	if childRegistry.Get("Bash") != nil {
		t.Fatalf("isolated child retained unsandboxed Bash: %T", childRegistry.Get("Bash"))
	}
	if parentBash.CWD != parentRoot || parentBash.ForceSandbox {
		t.Fatalf("parent Bash mutated: cwd %q force=%v", parentBash.CWD, parentBash.ForceSandbox)
	}
}

func TestAgentCWDRewrapScopesNestedAgentToNewestDirectory(t *testing.T) {
	parentRoot := t.TempDir()
	teammateRoot := t.TempDir()
	nestedRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(teammateRoot, "scope.txt"), []byte("teammate"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedRoot, "scope.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtimeProvider := agentCWDScopeRuntimeProvider{runtime: types.ToolRuntimeContext{ProjectRoot: parentRoot, AllowedDirs: []string{parentRoot}}}
	reg := registry.New()
	reg.SetRuntimeContextProvider(runtimeProvider)
	reg.Register(&toolfile.FileReadTool{AllowedDirs: []string{parentRoot}, Runtime: runtimeProvider})
	reg.Register(&toolfile.FileWriteTool{AllowedDirs: []string{parentRoot}, Runtime: runtimeProvider})
	reg.Register(&shell.BashTool{CWD: parentRoot, AllowedDirs: []string{parentRoot}, Sandbox: &task07SandboxBackend{}})

	wrapRegistryForAgentCWD(reg, teammateRoot)
	wrapRegistryForAgentCWD(reg, nestedRoot)

	readResult, err := reg.Get("Read").Execute(context.Background(), map[string]any{"file_path": "scope.txt"})
	if err != nil || readResult.IsError || !strings.Contains(readResult.Content, "nested") {
		t.Fatalf("nested Read result=%+v err=%v", readResult, err)
	}
	readResult, err = reg.Get("Read").Execute(context.Background(), map[string]any{"file_path": filepath.Join(teammateRoot, "scope.txt")})
	if err != nil || !readResult.IsError || !containsOutsideAllowedPath(readResult.Content, filepath.Join(teammateRoot, "scope.txt")) {
		t.Fatalf("teammate escape Read result=%+v err=%v", readResult, err)
	}

	writeResult, err := reg.Get("Write").Execute(context.Background(), map[string]any{"file_path": "nested-write.txt", "content": "nested"})
	if err != nil || writeResult.IsError {
		t.Fatalf("nested Write result=%+v err=%v", writeResult, err)
	}
	writeResult, err = reg.Get("Write").Execute(context.Background(), map[string]any{"file_path": filepath.Join(teammateRoot, "escaped.txt"), "content": "escaped"})
	if err != nil || !writeResult.IsError || !containsOutsideAllowedPath(writeResult.Content, filepath.Join(teammateRoot, "escaped.txt")) {
		t.Fatalf("teammate escape Write result=%+v err=%v", writeResult, err)
	}

	bash, ok := reg.Get("Bash").(*agentCWDBashToolWrapper)
	if !ok || bash.CWD != nestedRoot || !bash.ForceSandbox {
		t.Fatalf("nested Bash scope = %T %#v", reg.Get("Bash"), bash)
	}
	outside, err := executeApprovedRegistryToolForTest(t, reg, "Bash", map[string]any{"command": "cat " + filepath.Join(teammateRoot, "scope.txt")})
	if err != nil || !outside.IsError || !containsOutsideAllowedPath(outside.Content, filepath.Join(teammateRoot, "scope.txt")) {
		t.Fatalf("teammate escape Bash result=%+v err=%v", outside, err)
	}
}

func TestAgentCWDOmitsUnsandboxedPowerShell(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	parentPowerShell := &shell.PowerShellTool{CWD: parentRoot}
	childRegistry := registry.New()
	childRegistry.Register(parentPowerShell)
	wrapRegistryForAgentCWD(childRegistry, childRoot)

	if childRegistry.Get("PowerShell") != nil {
		t.Fatalf("isolated child retained unsandboxed PowerShell: %T", childRegistry.Get("PowerShell"))
	}
	if parentPowerShell.CWD != parentRoot || len(parentPowerShell.AllowedDirs) != 0 {
		t.Fatalf("parent PowerShell scope mutated: cwd %q allowed %v", parentPowerShell.CWD, parentPowerShell.AllowedDirs)
	}
}

func TestAgentCWDStillDelegatesToParentPermissionHandler(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := filepath.Join(parentRoot, "child")
	if err := os.MkdirAll(childRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childRoot, "note.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	parentRuntime := agentCWDScopeRuntimeProvider{runtime: types.ToolRuntimeContext{
		ProjectRoot: parentRoot,
		AllowedDirs: []string{parentRoot},
	}}
	reg := registry.New()
	reg.SetRuntimeContextProvider(parentRuntime)
	reg.Register(&toolfile.FileReadTool{AllowedDirs: []string{parentRoot}, Runtime: parentRuntime})
	provider := &agentCWDProvider{}
	tool := &AgentTool{
		Provider:          provider,
		Registry:          reg,
		PermissionHandler: denyToolPermissionHandler{tool: "Read"},
	}

	_, err := tool.runSubAgentWithOptions(context.Background(), "agent-child-permission", agentcontract.Input{
		Prompt: "read note",
		CWD:    childRoot,
	}, nil, agentLoopOptions{})
	if err != nil {
		t.Fatalf("runSubAgent: %v", err)
	}
	want := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimePermissionDenied, "Read")
	if !strings.Contains(provider.toolResultSeen, want) {
		t.Fatalf("child bypassed parent permission handler: %q", provider.toolResultSeen)
	}
}
