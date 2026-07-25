// Package interaction implements interactive, model-facing tools. Their typed data
// is rendered locally while the model sees only a compact acknowledgement.
package interaction

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	interactioncontract "github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/types"
)

const (
	sendUserMessageName               = "SendUserMessage"
	sendUserMessageMaxResultSizeChars = 100_000
)

type sendUserMessageStatus string

const (
	sendUserMessageStatusNormal    sendUserMessageStatus = "normal"
	sendUserMessageStatusProactive sendUserMessageStatus = "proactive"
)

type sendUserMessageInput struct {
	Message     string                `json:"message"`
	Attachments []string              `json:"attachments,omitempty"`
	Status      sendUserMessageStatus `json:"status"`
}

// SendUserMessageTool implements the SendUserMessage tool.
type SendUserMessageTool struct {
	// workingDirectoryProvider is queried per execution so worktree/session cwd changes
	// are reflected without rebuilding the registry.
	workingDirectoryProvider func() string
}

func NewSendUserMessageTool(workingDirectory func() string) *SendUserMessageTool {
	return &SendUserMessageTool{workingDirectoryProvider: workingDirectory}
}

func (t *SendUserMessageTool) Name() string { return sendUserMessageName }

func (t *SendUserMessageTool) Description() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageDescription)
}

func (t *SendUserMessageTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"message": map[string]any{
			"type":        "string",
			"description": i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageInputMessageDescription),
		},
		"attachments": map[string]any{
			"type":        "array",
			"description": i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageInputAttachmentsDescription),
			"items":       map[string]any{"type": "string"},
		},
		"status": map[string]any{
			"type":        "string",
			"enum":        []string{string(sendUserMessageStatusNormal), string(sendUserMessageStatusProactive)},
			"description": i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageInputStatusDescription),
		},
	}, "message", "status")
}

func (t *SendUserMessageTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: sendUserMessageMaxResultSizeChars,
	}
}

func (t *SendUserMessageTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		AlwaysLoad: true,
		SearchHint: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageDiscoveryHint),
	}
}

// IsEnabled uses the runtime feature as the sole explicit opt-in.
func (t *SendUserMessageTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	return runtime.Features[types.ToolFeatureSendUserMessage]
}

func (t *SendUserMessageTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, err := types.DecodeStrictToolInput[sendUserMessageInput](input)
	if err != nil {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageInvalidInput, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	if in.Status != sendUserMessageStatusNormal && in.Status != sendUserMessageStatusProactive {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageInvalidStatus,
			in.Status, sendUserMessageStatusNormal, sendUserMessageStatusProactive), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	// z.string permits an empty message; attachments may carry the visible
	// content. Missing/non-string message values fail strict decoding above.
	if _, exists := input["message"]; !exists {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageMessageRequired), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	if raw, exists := input["attachments"]; exists && raw == nil {
		return types.ToolResult{Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageAttachmentsMustBeArray), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}

	attachments, validationErr := t.resolveAttachmentMetadata(ctx, in.Attachments)
	if validationErr != nil {
		return types.ToolResult{Content: validationErr.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	output := interactioncontract.SendUserMessageOutput{
		Message:     in.Message,
		Attachments: attachments,
		SentAt:      formatSendUserMessageSentAt(time.Now()),
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return types.ToolResult{}, i18n.WrapInternalError(i18n.KeyToolSendUserMessageEncodeFailed, err)
	}
	return types.ToolResult{
		Content: string(encoded),
		Data:    output,
		Outcome: types.ToolOutcomeSucceeded,
		Metadata: map[string]string{
			"messageStatus": string(in.Status),
			"toolName":      sendUserMessageName,
		},
	}, nil
}

func formatSendUserMessageSentAt(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000Z")
}

func (t *SendUserMessageTool) resolveAttachmentMetadata(ctx context.Context, rawPaths []string) ([]interactioncontract.SendUserMessageAttachment, error) {
	if len(rawPaths) == 0 {
		return nil, nil
	}
	cwd, err := t.workingDirectory()
	if err != nil {
		return nil, err
	}
	resolved := make([]interactioncontract.SendUserMessageAttachment, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fullPath, err := resolveSendUserMessageAttachmentPath(cwd, rawPath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageAttachmentMissing, rawPath, cwd))
			}
			if os.IsPermission(err) {
				return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageAttachmentPermissionDenied, rawPath))
			}
			return nil, i18n.WrapError(i18n.KeyToolSendUserMessageInspectAttachment, err, rawPath)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageAttachmentNotRegular, rawPath))
		}
		// Opening without reading proves the caller can access the file while
		// preserving the TS rule that contents are never inlined.
		file, err := os.Open(fullPath)
		if err != nil {
			if os.IsPermission(err) {
				return nil, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageAttachmentPermissionDenied, rawPath))
			}
			return nil, i18n.WrapError(i18n.KeyToolSendUserMessageOpenAttachment, err, rawPath)
		}
		if err := file.Close(); err != nil {
			return nil, i18n.WrapError(i18n.KeyToolSendUserMessageCloseAttachment, err, rawPath)
		}
		resolved = append(resolved, interactioncontract.SendUserMessageAttachment{
			Path:    fullPath,
			Size:    info.Size(),
			IsImage: isSendUserMessageImagePath(fullPath),
		})
	}

	return resolved, nil
}

func (t *SendUserMessageTool) workingDirectory() (string, error) {
	if t.workingDirectoryProvider == nil {
		return "", fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageEmptyWorkingDirectory))
	}
	cwd := strings.TrimSpace(t.workingDirectoryProvider())
	if cwd == "" {
		return "", fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageEmptyWorkingDirectory))
	}
	return filepath.Abs(cwd)
}

func resolveSendUserMessageAttachmentPath(cwd, rawPath string) (string, error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageEmptyAttachmentPath))
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", i18n.WrapError(i18n.KeyToolSendUserMessageExpandAttachment, err, rawPath)
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
		return "", i18n.WrapError(i18n.KeyToolSendUserMessageResolveAttachment, err, rawPath)
	}
	return filepath.Clean(abs), nil
}

func isSendUserMessageImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	default:
		return false
	}
}

func (t *SendUserMessageTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(interactioncontract.SendUserMessageOutput)
	if !ok {
		if pointer, pointerOK := data.(*interactioncontract.SendUserMessageOutput); pointerOK && pointer != nil {
			output = *pointer
			ok = true
		}
	}
	count := 0
	if ok {
		count = len(output.Attachments)
	}
	content := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageDelivered)
	if count == 1 {
		content += i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageOneAttachmentIncluded)
	} else if count > 1 {
		content += i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolSendUserMessageAttachmentsIncluded, count)
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   content,
		Data:      output,
		Outcome:   types.ToolOutcomeSucceeded,
		Metadata: map[string]string{
			"renderMode": "send_user_message",
			"toolName":   sendUserMessageName,
		},
	}
}
