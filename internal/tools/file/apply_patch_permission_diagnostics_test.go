package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestApplyPatchMalformedInputIsReportedAsParseFailureNotPermissionDenial(t *testing.T) {
	root := t.TempDir()
	tool := newApplyPatchTestTool(root)
	tests := []struct {
		name   string
		input  map[string]any
		reason string
		path   string
	}{
		{
			name:   "invalid strict input",
			input:  map[string]any{"patch": 42},
			reason: "invalid_input",
		},
		{
			name: "invalid hunk line",
			input: map[string]any{"patch": strings.Join([]string{
				"*** Begin Patch",
				"*** Update File: target.txt",
				"@@",
				"old",
				"+new",
				"*** End Patch",
			}, "\n")},
			reason: "invalid_hunk_line",
			path:   "target.txt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			permission, err := tool.CheckPermissions(context.Background(), test.input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
				ProjectRoot: root,
				AllowedDirs: []string{root},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if permission.Behavior != types.PermissionBehaviorAllow || permission.Required {
				t.Fatalf("permission = %+v, want allow so Execute can return a parse error", permission)
			}
			if permission.Message != "" {
				t.Fatalf("malformed input produced a pre-execution permission message: %q", permission.Message)
			}

			result, err := tool.Execute(context.Background(), permission.UpdatedInput)
			if err != nil {
				t.Fatal(err)
			}
			requireApplyPatchErrorCode(t, result, fileErrorApplyPatchParse)
			data, ok := result.Data.(types.ToolErrorData)
			if !ok {
				t.Fatalf("parse result Data = %T, want types.ToolErrorData", result.Data)
			}
			if data.Schema != "tool_error/v1" || data.Retryable || data.Path != test.path {
				t.Fatalf("parse tool error = %+v", data)
			}
			if result.Metadata["apply_patch.failure_reason"] != test.reason {
				t.Fatalf("failure reason = %q, want %q", result.Metadata["apply_patch.failure_reason"], test.reason)
			}
			if want := toolRuntimeFormat(i18n.KeyToolApplyPatchParseFailed, test.reason, test.path); result.Content != want {
				t.Fatalf("parse diagnostic = %q, want %q", result.Content, want)
			}

			mapped := types.MapToolResult(tool, result, "parse-call")
			if !strings.Contains(mapped.Content, "<tool_error>") ||
				!strings.Contains(mapped.Content, `"code":"file.apply_patch.parse"`) {
				t.Fatalf("model-visible parse diagnostic lost structured tool_error: %q", mapped.Content)
			}
		})
	}
}

func TestApplyPatchMalformedInputCannotBypassPathPolicyOrWrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "blocked.txt")
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget := filepath.Join(resolvedRoot, "blocked.txt")
	tool := newApplyPatchTestTool(root)
	malformed := map[string]any{"patch": strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: blocked.txt",
		"not-an-added-line",
		"*** End Patch",
	}, "\n")}
	request := types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root,
		AllowedDirs: []string{root},
		DeniedRules: []types.PermissionRuleValue{{ToolName: "ApplyPatch", RuleContent: target}},
	}}

	permission, err := tool.CheckPermissions(context.Background(), malformed, request)
	if err != nil || permission.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("malformed permission = %+v, err=%v", permission, err)
	}
	result, err := tool.Execute(context.Background(), permission.UpdatedInput)
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchParse)
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("malformed patch reached a denied write target: %v", err)
	}

	valid := map[string]any{"patch": "*** Begin Patch\n*** Add File: blocked.txt\n+content\n*** End Patch"}
	permission, err = tool.CheckPermissions(context.Background(), valid, request)
	if err != nil {
		t.Fatal(err)
	}
	if permission.Behavior != types.PermissionBehaviorDeny || permission.BlockedPath == "" ||
		permission.Message != toolPermissionFormat(i18n.KeyToolApplyPatchPermissionDenied, resolvedTarget, "denied_rule") {
		t.Fatalf("valid denied target lost path-policy enforcement: %+v", permission)
	}
}

func TestApplyPatchClientPreflightRejectsDuplicateTargetBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	tool := newApplyPatchTestTool(root)
	input := map[string]any{"patch": strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: duplicate.txt",
		"+first",
		"*** Add File: duplicate.txt",
		"+second",
		"*** End Patch",
	}, "\n")}
	permission, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits",
	}})
	if err != nil || permission.Behavior != types.PermissionBehaviorAllow || permission.Required || permission.Message != "" {
		t.Fatalf("deterministic parse preflight entered permission UI: %+v, err=%v", permission, err)
	}
	result, err := tool.Execute(context.Background(), permission.UpdatedInput)
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchParse)
	data := result.Data.(types.ToolErrorData)
	if data.Schema != "tool_error/v1" || data.Retryable || data.Path != "duplicate.txt" ||
		result.Metadata["apply_patch.failure_reason"] != "duplicate_target" {
		t.Fatalf("duplicate-target tool contract = result:%+v data:%+v", result, data)
	}
	if _, statErr := os.Lstat(filepath.Join(root, "duplicate.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("duplicate target reached filesystem mutation: %v", statErr)
	}
}

func TestApplyPatchClientPreflightRequiresInspectBeforeContextFreeDelete(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "delete.txt")
	writeApplyPatchFixture(t, target, "preserve until inspected\n")
	tool := newApplyPatchTestTool(root)
	input := map[string]any{"patch": "*** Begin Patch\n*** Delete File: delete.txt\n*** End Patch"}
	permission, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits",
	}})
	if err != nil || permission.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("read-required permission preflight = %+v, err=%v", permission, err)
	}
	result, err := tool.Execute(context.Background(), permission.UpdatedInput)
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchReadRequired)
	data := result.Data.(types.ToolErrorData)
	if data.Schema != "tool_error/v1" || !data.Retryable || data.Path != "delete.txt" || data.Retry == nil ||
		data.Retry.Tool != "Inspect" || data.Retry.Action != "inspect_batch" || len(data.Retry.Requests) != 1 {
		t.Fatalf("read-required tool contract = %+v", data)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "preserve until inspected\n" {
		t.Fatalf("read-required preflight mutated target: %q, err=%v", got, readErr)
	}
}
