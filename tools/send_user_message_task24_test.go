package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestSendUserMessageAlignment(t *testing.T) {
	tool := NewSendUserMessageTool()
	if !reflect.DeepEqual(tool.Aliases(), []string{"Brief"}) {
		t.Fatalf("aliases = %v", tool.Aliases())
	}
	result, err := tool.Execute(context.Background(), map[string]any{
		"message": "aligned", "status": "normal",
	})
	if err != nil || result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var wire map[string]any
	if err := json.Unmarshal([]byte(result.Content), &wire); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	for _, goOnly := range []string{"delivered", "messageId", "status", "content", "media_type", "bytes"} {
		if _, exists := wire[goOnly]; exists {
			t.Fatalf("TS-compatible output contains Go-only field %q: %#v", goOnly, wire)
		}
	}
	if wire["message"] != "aligned" || wire["sentAt"] == "" {
		t.Fatalf("output wire shape = %#v", wire)
	}
}

func task24Output(t *testing.T, result types.ToolResult) types.SendUserMessageOutput {
	t.Helper()
	output, ok := result.Data.(types.SendUserMessageOutput)
	if !ok {
		t.Fatalf("SendUserMessage Data = %T, want types.SendUserMessageOutput", result.Data)
	}
	return output
}

func TestSendUserMessageSchemaStrictStatusAndAttachmentShape(t *testing.T) {
	tool := NewSendUserMessageTool()
	schema := tool.Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatal("SendUserMessage input schema must be a strict object")
	}
	if !reflect.DeepEqual(schema.Required, []string{"message", "status"}) {
		t.Fatalf("required = %v, want [message status]", schema.Required)
	}
	status := schema.Properties["status"].(map[string]any)
	if !reflect.DeepEqual(status["enum"], []string{"normal", "proactive"}) {
		t.Fatalf("status enum = %#v", status["enum"])
	}
	attachments := schema.Properties["attachments"].(map[string]any)
	items := attachments["items"].(map[string]any)
	if items["type"] != "string" {
		t.Fatalf("attachment item type = %#v, want string", items["type"])
	}
	definition := types.ToDefinition(tool)
	if !definition.Strict || definition.OutputSchema == nil {
		t.Fatalf("definition strict/output schema = %v/%v", definition.Strict, definition.OutputSchema)
	}
}

func TestSendUserMessageStatusStrictCompatibility(t *testing.T) {
	tool := NewSendUserMessageTool()
	for _, status := range []string{"normal", "proactive"} {
		t.Run("valid_"+status, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), map[string]any{"message": "hello", "status": status})
			if err != nil || result.IsError {
				t.Fatalf("valid status %q: result=%#v err=%v", status, result, err)
			}
		})
	}
	for _, status := range []string{"", "info", "success", "warning", "error", "NORMAL"} {
		t.Run("invalid_"+status, func(t *testing.T) {
			input := map[string]any{"message": "hello", "status": status}
			if status == "" {
				delete(input, "status")
			}
			result, err := tool.Execute(context.Background(), input)
			if err != nil || !result.IsError {
				t.Fatalf("invalid status %q: result=%#v err=%v", status, result, err)
			}
		})
	}
}

func TestSendUserMessageStrictRejectsUnknownAndObjectAttachmentShape(t *testing.T) {
	tool := NewSendUserMessageTool()
	tests := []map[string]any{
		{"message": "hello", "status": "normal", "extra": true},
		{"message": "hello", "status": "normal", "attachments": []any{map[string]any{"path": "x"}}},
	}
	for _, input := range tests {
		result, err := tool.Execute(context.Background(), input)
		if err != nil || !result.IsError {
			t.Fatalf("input %#v: result=%#v err=%v", input, result, err)
		}
	}
}

func TestSendUserMessageStrictRegistryDispatcherAndAlias(t *testing.T) {
	reg := registry.New()
	reg.Register(NewSendUserMessageTool())
	bad := reg.ExecuteTool(context.Background(), "Brief", map[string]any{
		"message": "hello", "status": "normal", "unexpected": true,
	})
	if !bad.IsError || !strings.Contains(bad.Content, "unexpected") {
		t.Fatalf("strict alias dispatch = %#v", bad)
	}
	good := reg.ExecuteTool(context.Background(), "Brief", map[string]any{
		"message": "hello", "status": "normal",
	})
	if good.IsError || good.Content != "Message delivered to user." {
		t.Fatalf("alias dispatch = %#v", good)
	}
}

func TestSendUserMessageOutputAndToolResultAcknowledgement(t *testing.T) {
	fixed := time.Date(2026, 7, 11, 12, 34, 56, 789_000_000, time.FixedZone("offset", 8*60*60))
	tool := NewSendUserMessageTool()
	tool.Now = func() time.Time { return fixed }
	result, err := tool.Execute(context.Background(), map[string]any{
		"message": "Build is green.",
		"status":  "normal",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%#v err=%v", result, err)
	}
	output := task24Output(t, result)
	if output.Message != "Build is green." || output.SentAt != "2026-07-11T04:34:56.789Z" || output.Attachments != nil {
		t.Fatalf("output = %#v", output)
	}
	block := types.MapToolResult(tool, result, "toolu_brief")
	if block.ToolUseID != "toolu_brief" || block.Content != "Message delivered to user." {
		t.Fatalf("mapped block = %#v", block)
	}
	if strings.Contains(block.Content, output.Message) {
		t.Fatalf("model acknowledgement leaked user-facing message: %q", block.Content)
	}
}

type task24Uploader func(context.Context, string, int64) (string, error)

func (f task24Uploader) UploadBriefAttachment(ctx context.Context, path string, size int64) (string, error) {
	return f(ctx, path, size)
}

func TestSendUserMessageAttachmentPathMetadataAndCount(t *testing.T) {
	root := t.TempDir()
	textPath := filepath.Join(root, "report.log")
	imagePath := filepath.Join(root, "screen.PNG")
	if err := os.WriteFile(textPath, []byte("secret body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("not decoded or inlined"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewSendUserMessageTool()
	tool.WorkingDirectory = func() string { return root }
	tool.Uploader = task24Uploader(func(_ context.Context, path string, _ int64) (string, error) {
		if path == imagePath {
			return "file_uuid_image", nil
		}
		return "", errors.New("best effort upload failed")
	})
	result, err := tool.Execute(context.Background(), map[string]any{
		"message":     "Artifacts attached.",
		"status":      "proactive",
		"attachments": []any{"report.log", imagePath},
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%#v err=%v", result, err)
	}
	output := task24Output(t, result)
	if len(output.Attachments) != 2 {
		t.Fatalf("attachments = %#v", output.Attachments)
	}
	if output.Attachments[0].Path != textPath || output.Attachments[0].Size != int64(len("secret body")) || output.Attachments[0].IsImage || output.Attachments[0].FileUUID != "" {
		t.Fatalf("text attachment = %#v", output.Attachments[0])
	}
	if output.Attachments[1].Path != imagePath || !output.Attachments[1].IsImage || output.Attachments[1].FileUUID != "file_uuid_image" {
		t.Fatalf("image attachment = %#v", output.Attachments[1])
	}
	block := types.MapToolResult(tool, result, "toolu_attachments")
	if block.Content != "Message delivered to user. (2 attachments included)" {
		t.Fatalf("ack = %q", block.Content)
	}
	if strings.Contains(result.Content, "secret body") || len(result.ContentBlocks) != 0 {
		t.Fatalf("attachment contents must not be inlined: content=%q blocks=%#v", result.Content, result.ContentBlocks)
	}
}

func TestSendUserMessageAttachmentCountAcknowledgement(t *testing.T) {
	tool := NewSendUserMessageTool()
	for count, want := range map[int]string{
		0: "Message delivered to user.",
		1: "Message delivered to user. (1 attachment included)",
		2: "Message delivered to user. (2 attachments included)",
	} {
		attachments := make([]types.SendUserMessageAttachment, count)
		block := tool.MapToolResultToToolResultBlock(types.SendUserMessageOutput{Attachments: attachments}, "toolu_count")
		if block.Content != want {
			t.Fatalf("count %d acknowledgement = %q, want %q", count, block.Content, want)
		}
	}
}

func TestSendUserMessageAttachmentFileValidation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := NewSendUserMessageTool()
	tool.WorkingDirectory = func() string { return root }
	for name, path := range map[string]string{"missing": "missing.txt", "directory": dir} {
		t.Run(name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), map[string]any{
				"message":     "attachment",
				"status":      "normal",
				"attachments": []any{path},
			})
			if err != nil || !result.IsError {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}

func TestSendUserMessageAttachmentPermissionDenied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Skipf("cannot make attachment inaccessible: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if file, err := os.Open(path); err == nil {
		_ = file.Close()
		t.Skip("test process can bypass file permission bits")
	}
	result, err := NewSendUserMessageTool().Execute(context.Background(), map[string]any{
		"message": "private", "status": "normal", "attachments": []any{path},
	})
	if err != nil || !result.IsError || !strings.Contains(strings.ToLower(result.Content), "permission") {
		t.Fatalf("permission result=%#v err=%v", result, err)
	}
}

func TestSendUserMessageAttachmentValidationPrecedesUpload(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.txt")
	if err := os.WriteFile(valid, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	uploads := 0
	tool := NewSendUserMessageTool()
	tool.Uploader = task24Uploader(func(context.Context, string, int64) (string, error) {
		uploads++
		return "should_not_upload", nil
	})
	result, err := tool.Execute(context.Background(), map[string]any{
		"message": "attachments", "status": "normal", "attachments": []any{valid, filepath.Join(root, "missing.txt")},
	})
	if err != nil || !result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if uploads != 0 {
		t.Fatalf("uploads began before all attachment paths validated: %d", uploads)
	}
}

func TestSendUserMessageUploadUnavailableIsNoop(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "artifact.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewSendUserMessageTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"message": "attached", "status": "normal", "attachments": []any{path},
	})
	if err != nil || result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if got := task24Output(t, result).Attachments[0].FileUUID; got != "" {
		t.Fatalf("file_uuid = %q, want absent", got)
	}
}

func TestSendUserMessageEnabledEnvironmentAndInjectableGates(t *testing.T) {
	tool := NewSendUserMessageTool()
	runtime := types.ToolRuntimeContext{Features: map[string]bool{types.ToolFeatureBrief: false}}
	if tool.IsEnabled(runtime) {
		t.Fatal("explicit runtime opt-out must disable SendUserMessage")
	}
	t.Setenv("CLAUDE_CODE_BRIEF", "true")
	if !tool.IsEnabled(runtime) {
		t.Fatal("CLAUDE_CODE_BRIEF must opt in")
	}
	tool.KillSwitchGate = func(types.ToolRuntimeContext) bool { return false }
	if tool.IsEnabled(runtime) {
		t.Fatal("injectable kill switch must override env opt-in")
	}
	tool.KillSwitchGate = nil
	tool.EntitlementGate = func(types.ToolRuntimeContext) bool { return false }
	if tool.IsEnabled(runtime) {
		t.Fatal("injectable entitlement gate must override env opt-in")
	}
}

func TestSendUserMessageConcurrentReadOnlyVisibilityAndClassifier(t *testing.T) {
	tool := NewSendUserMessageTool()
	if !tool.IsConcurrentSafe() || !tool.IsReadOnly() {
		t.Fatalf("concurrent/read-only = %v/%v", tool.IsConcurrentSafe(), tool.IsReadOnly())
	}
	metadata := tool.ToolMetadata(nil)
	if !metadata.ReadOnly || !metadata.ConcurrencySafe || metadata.MaxResultSizeChars != 100_000 {
		t.Fatalf("metadata = %#v", metadata)
	}
	discovery := tool.ToolDiscoveryMetadata()
	if !discovery.AlwaysLoad || !strings.Contains(discovery.SearchHint, "primary visible output channel") {
		t.Fatalf("discovery = %#v", discovery)
	}
	if got := tool.ToAutoClassifierInput(map[string]any{"message": "Need your input", "status": "normal"}); got != "Need your input" {
		t.Fatalf("classifier input = %q", got)
	}
	reg := registry.New()
	reg.Register(tool)
	if got := reg.ToolMetadata("SendUserMessage", nil); !got.ReadOnly || !got.ConcurrencySafe {
		t.Fatalf("registry metadata = %#v", got)
	}
}
