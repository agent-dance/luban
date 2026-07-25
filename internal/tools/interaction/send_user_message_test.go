package interaction

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	interactioncontract "github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func newSendUserMessageTestTool(t *testing.T) *SendUserMessageTool {
	t.Helper()
	root := t.TempDir()
	return NewSendUserMessageTool(func() string { return root })
}

func TestSendUserMessageAlignment(t *testing.T) {
	tool := newSendUserMessageTestTool(t)
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

func task24Output(t *testing.T, result types.ToolResult) interactioncontract.SendUserMessageOutput {
	t.Helper()
	output, ok := result.Data.(interactioncontract.SendUserMessageOutput)
	if !ok {
		t.Fatalf("SendUserMessage Data = %T, want interactioncontract.SendUserMessageOutput", result.Data)
	}
	return output
}

func TestSendUserMessageSchemaStrictStatusAndAttachmentShape(t *testing.T) {
	tool := newSendUserMessageTestTool(t)
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
	if !definition.Strict {
		t.Fatal("definition must derive strictness from the input schema")
	}
}

func TestSendUserMessageStatusStrictValidation(t *testing.T) {
	tool := newSendUserMessageTestTool(t)
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
	tool := newSendUserMessageTestTool(t)
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

func TestSendUserMessageOutputAndToolResultAcknowledgement(t *testing.T) {
	tool := newSendUserMessageTestTool(t)
	startedAt := time.Now().UTC().Add(-time.Second)
	result, err := tool.Execute(context.Background(), map[string]any{
		"message": "Build is green.",
		"status":  "normal",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%#v err=%v", result, err)
	}
	if result.Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("Execute outcome = %q", result.Outcome)
	}
	output := task24Output(t, result)
	if output.Message != "Build is green." || output.Attachments != nil {
		t.Fatalf("output = %#v", output)
	}
	sentAt, err := time.Parse("2006-01-02T15:04:05.000Z", output.SentAt)
	if err != nil || sentAt.Before(startedAt) || sentAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("sentAt = %q parsed:%v err:%v", output.SentAt, sentAt, err)
	}
	if result.Metadata["messageStatus"] != string(sendUserMessageStatusNormal) {
		t.Fatalf("message status metadata = %#v", result.Metadata)
	}
	block := types.MapToolResult(tool, result, "toolu_message")
	if block.ToolUseID != "toolu_message" || block.Content != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageDelivered) {
		t.Fatalf("mapped block = %#v", block)
	}
	if block.Outcome != types.ToolOutcomeSucceeded || !reflect.DeepEqual(block.Data, output) {
		t.Fatalf("mapped authority = outcome:%q data:%#v", block.Outcome, block.Data)
	}
	if strings.Contains(block.Content, output.Message) {
		t.Fatalf("model acknowledgement leaked user-facing message: %q", block.Content)
	}
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
	tool := NewSendUserMessageTool(func() string { return root })
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
	if output.Attachments[0].Path != textPath || output.Attachments[0].Size != int64(len("secret body")) || output.Attachments[0].IsImage {
		t.Fatalf("text attachment = %#v", output.Attachments[0])
	}
	if output.Attachments[1].Path != imagePath || !output.Attachments[1].IsImage {
		t.Fatalf("image attachment = %#v", output.Attachments[1])
	}
	block := types.MapToolResult(tool, result, "toolu_attachments")
	wantAcknowledgement := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageDelivered) +
		i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageAttachmentsIncluded, 2)
	if block.Content != wantAcknowledgement {
		t.Fatalf("ack = %q", block.Content)
	}
	if strings.Contains(result.Content, "secret body") || len(result.ContentBlocks) != 0 {
		t.Fatalf("attachment contents must not be inlined: content=%q blocks=%#v", result.Content, result.ContentBlocks)
	}
}

func TestSendUserMessageAttachmentCountAcknowledgement(t *testing.T) {
	tool := newSendUserMessageTestTool(t)
	lang := i18n.DetectOrLoadLanguage()
	base := i18n.Text(lang, i18n.KeyToolSendUserMessageDelivered)
	for count, want := range map[int]string{
		0: base,
		1: base + i18n.Text(lang, i18n.KeyToolSendUserMessageOneAttachmentIncluded),
		2: base + i18n.Format(lang, i18n.KeyToolSendUserMessageAttachmentsIncluded, 2),
	} {
		attachments := make([]interactioncontract.SendUserMessageAttachment, count)
		block := tool.MapToolResultToToolResultBlock(interactioncontract.SendUserMessageOutput{Attachments: attachments}, "toolu_count")
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
	tool := NewSendUserMessageTool(func() string { return root })
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

func TestSendUserMessageAttachmentRequiresInjectedWorkingDirectory(t *testing.T) {
	tool := NewSendUserMessageTool(nil)
	result, err := tool.Execute(context.Background(), map[string]any{
		"message": "attachment", "status": "normal", "attachments": []any{"file.txt"},
	})
	if err != nil || !result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
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
	result, err := newSendUserMessageTestTool(t).Execute(context.Background(), map[string]any{
		"message": "private", "status": "normal", "attachments": []any{path},
	})
	if err != nil || !result.IsError || !strings.Contains(strings.ToLower(result.Content), "permission") {
		t.Fatalf("permission result=%#v err=%v", result, err)
	}
}

func TestSendUserMessageEnabledRequiresRuntimeFeature(t *testing.T) {
	tool := newSendUserMessageTestTool(t)
	if tool.IsEnabled(types.ToolRuntimeContext{}) {
		t.Fatal("missing runtime feature state must not enable SendUserMessage")
	}
	runtime := types.ToolRuntimeContext{Features: map[string]bool{types.ToolFeatureSendUserMessage: false}}
	if tool.IsEnabled(runtime) {
		t.Fatal("explicit runtime opt-out must disable SendUserMessage")
	}
	t.Setenv("LUBAN_CODE_SEND_USER_MESSAGE", "true")
	if tool.IsEnabled(runtime) {
		t.Fatal("tool-local environment fallback must not override runtime feature state")
	}
	runtime.Features[types.ToolFeatureSendUserMessage] = true
	if !tool.IsEnabled(runtime) {
		t.Fatal("runtime feature opt-in must enable SendUserMessage")
	}
}

func TestSendUserMessageConcurrentReadOnlyVisibilityAndClassifier(t *testing.T) {
	tool := newSendUserMessageTestTool(t)
	metadata := tool.ToolMetadata(nil)
	if !metadata.ConcurrencySafe || !metadata.ReadOnly {
		t.Fatalf("tool metadata = %#v", metadata)
	}
	if !metadata.ReadOnly || !metadata.ConcurrencySafe || metadata.MaxResultSizeChars != 100_000 {
		t.Fatalf("metadata = %#v", metadata)
	}
	discovery := tool.ToolDiscoveryMetadata()
	if !discovery.AlwaysLoad || discovery.SearchHint != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageDiscoveryHint) {
		t.Fatalf("discovery = %#v", discovery)
	}
	reg := registry.New()
	reg.Register(tool)
	if got := reg.ToolMetadata("SendUserMessage", nil); !got.ReadOnly || !got.ConcurrencySafe {
		t.Fatalf("registry metadata = %#v", got)
	}
}
