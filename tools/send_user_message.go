// Package tools implements the canonical SendUserMessage (legacy alias:
// Brief) tool. The tool's typed data is rendered locally while the model sees
// only a compact delivery acknowledgement, matching BriefTool in the TS
// baseline.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const (
	SendUserMessageName               = "SendUserMessage"
	SendUserMessageAlias              = "Brief"
	SendUserMessageMaxResultSizeChars = 100_000
)

type SendUserMessageStatus string

const (
	SendUserMessageStatusNormal    SendUserMessageStatus = "normal"
	SendUserMessageStatusProactive SendUserMessageStatus = "proactive"
)

// SendUserMessageInput is the exact TS BriefTool input contract.
type SendUserMessageInput struct {
	Message     string                `json:"message"`
	Attachments []string              `json:"attachments,omitempty"`
	Status      SendUserMessageStatus `json:"status"`
}

type SendUserMessageAttachment = types.SendUserMessageAttachment
type SendUserMessageResult = types.SendUserMessageOutput

// BriefAttachmentUploader is an optional bridge capability. Upload is
// best-effort: callers still receive local path metadata on every failure.
type BriefAttachmentUploader interface {
	UploadBriefAttachment(context.Context, string, int64) (string, error)
}

// SendUserMessageTool is the sole implementation registered under both
// SendUserMessage and Brief.
type SendUserMessageTool struct {
	// WorkingDirectory is queried per execution so worktree/session cwd changes
	// are reflected without rebuilding the registry.
	WorkingDirectory func() string
	Uploader         BriefAttachmentUploader
	Now              func() time.Time

	// The Go build has no Bun build flags or GrowthBook runtime. These seams
	// represent the entitlement and refreshed kill-switch decisions for hosts
	// that do provide them. Nil means the bundled capability is available.
	EntitlementGate func(types.ToolRuntimeContext) bool
	KillSwitchGate  func(types.ToolRuntimeContext) bool

	// Analytics receives the TS-equivalent proactive/count fields after input
	// validation. It is deliberately optional and has no effect on delivery.
	Analytics func(proactive bool, attachmentCount int)
}

func NewSendUserMessageTool() *SendUserMessageTool { return &SendUserMessageTool{} }

func (t *SendUserMessageTool) Name() string      { return SendUserMessageName }
func (t *SendUserMessageTool) Aliases() []string { return []string{SendUserMessageAlias} }

func (t *SendUserMessageTool) Description() string { return "Send a message to the user" }

func (t *SendUserMessageTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"message": map[string]any{
			"type":        "string",
			"description": "The message for the user. Supports markdown formatting.",
		},
		"attachments": map[string]any{
			"type":        "array",
			"description": "Optional file paths (absolute or relative to cwd) to attach.",
			"items":       map[string]any{"type": "string"},
		},
		"status": map[string]any{
			"type":        "string",
			"enum":        []string{string(SendUserMessageStatusNormal), string(SendUserMessageStatusProactive)},
			"description": "Use proactive for unsolicited updates and normal for replies.",
		},
	}, "message", "status")
}

func sendUserMessageOutputSchema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"message": map[string]any{"type": "string"},
			"attachments": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":      map[string]any{"type": "string"},
						"size":      map[string]any{"type": "number"},
						"isImage":   map[string]any{"type": "boolean"},
						"file_uuid": map[string]any{"type": "string"},
					},
					"required": []string{"path", "size", "isImage"},
				},
			},
			"sentAt": map[string]any{"type": "string"},
		},
		Required: []string{"message"},
	}
}

func (t *SendUserMessageTool) ToolContract() types.ToolContract {
	output := sendUserMessageOutputSchema()
	return types.ToolContract{
		OutputSchema:       &output,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: SendUserMessageMaxResultSizeChars,
	}
}

func (t *SendUserMessageTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: SendUserMessageMaxResultSizeChars,
	}
}

func (t *SendUserMessageTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{
		AlwaysLoad: true,
		SearchHint: "send a message to the user - your primary visible output channel",
	}
}

func (t *SendUserMessageTool) IsConcurrentSafe() bool { return true }
func (t *SendUserMessageTool) IsReadOnly() bool       { return true }

func (t *SendUserMessageTool) ToAutoClassifierInput(input map[string]any) string {
	message, _ := input["message"].(string)
	return message
}

// IsEnabled uses the runtime brief feature as Go's explicit opt-in. The env
// flag force-enables development/testing, while injected entitlement and kill
// switch gates remain authoritative. A missing feature map preserves legacy
// embedders that have no RuntimeScope; SetupRegistry always supplies one.
func (t *SendUserMessageTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	if t.EntitlementGate != nil && !t.EntitlementGate(runtime) {
		return false
	}
	if t.KillSwitchGate != nil && !t.KillSwitchGate(runtime) {
		return false
	}
	if isBriefEnvTruthy(os.Getenv("CLAUDE_CODE_BRIEF")) {
		return true
	}
	if runtime.Features == nil {
		return true
	}
	return runtime.Features[types.ToolFeatureBrief]
}

func isBriefEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (t *SendUserMessageTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, err := types.DecodeStrictToolInput[SendUserMessageInput](input)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCInvalidSendUserMessage, err)), nil
	}
	if in.Status != SendUserMessageStatusNormal && in.Status != SendUserMessageStatusProactive {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCInvalidStatus,
			in.Status, SendUserMessageStatusNormal, SendUserMessageStatusProactive)), nil
	}
	// z.string permits an empty message; attachments may carry the visible
	// content. Missing/non-string message values fail strict decoding above.
	if _, exists := input["message"]; !exists {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCMessageRequired)), nil
	}
	if raw, exists := input["attachments"]; exists && raw == nil {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCAttachmentsMustBeArray)), nil
	}

	attachments, validationErr := t.resolveAttachmentMetadata(ctx, in.Attachments)
	if validationErr != nil {
		return ErrorResponsef("%s", validationErr), nil
	}
	if t.Analytics != nil {
		t.Analytics(in.Status == SendUserMessageStatusProactive, len(in.Attachments))
	}

	now := time.Now
	if t.Now != nil {
		now = t.Now
	}
	output := types.SendUserMessageOutput{
		Message:     in.Message,
		Attachments: attachments,
		SentAt:      formatBriefSentAt(now()),
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return types.ToolResult{}, fmt.Errorf("%s: %w", toolRuntimeText(i18n.KeyToolLegacyCEncodeSendUserMessageFailed), err)
	}
	return types.ToolResult{
		Content: string(encoded),
		Data:    output,
		Metadata: map[string]string{
			"briefStatus": string(in.Status),
			"toolName":    SendUserMessageName,
		},
	}, nil
}

func formatBriefSentAt(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000Z")
}

func (t *SendUserMessageTool) resolveAttachmentMetadata(ctx context.Context, rawPaths []string) ([]types.SendUserMessageAttachment, error) {
	if len(rawPaths) == 0 {
		return nil, nil
	}
	cwd, err := t.workingDirectory()
	if err != nil {
		return nil, err
	}
	resolved := make([]types.SendUserMessageAttachment, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fullPath, err := resolveBriefAttachmentPath(cwd, rawPath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCAttachmentMissing, rawPath, cwd))
			}
			if os.IsPermission(err) {
				return nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCAttachmentPermissionDenied, rawPath))
			}
			return nil, fmt.Errorf("%s: %w", toolRuntimeFormat(i18n.KeyToolLegacyCInspectAttachment, rawPath), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCAttachmentNotRegular, rawPath))
		}
		// Opening without reading proves the caller can access the file while
		// preserving the TS rule that contents are never inlined.
		file, err := os.Open(fullPath)
		if err != nil {
			if os.IsPermission(err) {
				return nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCAttachmentPermissionDenied, rawPath))
			}
			return nil, fmt.Errorf("%s: %w", toolRuntimeFormat(i18n.KeyToolLegacyCOpenAttachment, rawPath), err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("%s: %w", toolRuntimeFormat(i18n.KeyToolLegacyCCloseAttachment, rawPath), err)
		}
		resolved = append(resolved, types.SendUserMessageAttachment{
			Path:    fullPath,
			Size:    info.Size(),
			IsImage: isBriefImagePath(fullPath),
		})
	}

	if t.Uploader == nil {
		return resolved, nil
	}
	// Validation/stat completes for every path before any upload starts. Uploads
	// then run in parallel, preserving input order and degrading independently.
	var wg sync.WaitGroup
	for i := range resolved {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			uuid, uploadErr := t.Uploader.UploadBriefAttachment(ctx, resolved[index].Path, resolved[index].Size)
			if uploadErr == nil {
				resolved[index].FileUUID = strings.TrimSpace(uuid)
			}
		}(i)
	}
	wg.Wait()
	return resolved, nil
}

func (t *SendUserMessageTool) workingDirectory() (string, error) {
	if t.WorkingDirectory != nil {
		cwd := strings.TrimSpace(t.WorkingDirectory())
		if cwd == "" {
			return "", fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolLegacyCEmptyWorkingDirectory))
		}
		return filepath.Abs(cwd)
	}
	return os.Getwd()
}

func resolveBriefAttachmentPath(cwd, rawPath string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolLegacyCEmptyAttachmentPath))
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%s: %w", toolRuntimeFormat(i18n.KeyToolLegacyCExpandAttachment, rawPath), err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w", toolRuntimeFormat(i18n.KeyToolLegacyCResolveAttachment, rawPath), err)
	}
	return filepath.Clean(abs), nil
}

func isBriefImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func (t *SendUserMessageTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(types.SendUserMessageOutput)
	if !ok {
		if pointer, pointerOK := data.(*types.SendUserMessageOutput); pointerOK && pointer != nil {
			output = *pointer
			ok = true
		}
	}
	count := 0
	if ok {
		count = len(output.Attachments)
	}
	content := toolRuntimeText(i18n.KeyToolLegacyCMessageDelivered)
	if count == 1 {
		content += toolRuntimeText(i18n.KeyToolLegacyCOneAttachmentIncluded)
	} else if count > 1 {
		content += toolRuntimeFormat(i18n.KeyToolLegacyCAttachmentsIncluded, count)
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   content,
		Metadata: map[string]string{
			"renderMode": "send_user_message",
			"toolName":   SendUserMessageName,
		},
	}
}
