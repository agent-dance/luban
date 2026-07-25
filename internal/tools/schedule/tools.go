package schedule

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
	"github.com/rivo/uniseg"
)

type createInput struct {
	Cron      string `json:"cron"`
	Prompt    string `json:"prompt"`
	Recurring *bool  `json:"recurring,omitempty"`
	Durable   *bool  `json:"durable,omitempty"`
}

type deleteInput struct {
	ID string `json:"id"`
}

type createOutput struct {
	ID        string `json:"id"`
	Cron      string `json:"cron"`
	NextFire  string `json:"next_fire"`
	Recurring bool   `json:"recurring"`
	Durable   bool   `json:"durable"`
}

type deleteOutput struct {
	ID string `json:"id"`
}

type listOutput struct {
	Jobs []listJobOutput `json:"jobs"`
}

type listJobOutput struct {
	ID        string `json:"id"`
	Cron      string `json:"cron"`
	Prompt    string `json:"prompt"`
	NextFire  string `json:"next_fire"`
	Recurring bool   `json:"recurring"`
	Durable   bool   `json:"durable"`
}

type createTool struct {
	service     *Service
	agentID     string
	projectRoot string
}

var _ agentcontract.ScopedTool = (*createTool)(nil)

// NewCreateTool returns the CronCreate tool.
func NewCreateTool(service *Service) types.Tool {
	return &createTool{service: service}
}

func (t *createTool) BindAgentScope(agentID, projectRoot string) types.Tool {
	cloned := *t
	cloned.agentID = strings.TrimSpace(agentID)
	cloned.projectRoot = strings.TrimSpace(projectRoot)
	return &cloned
}

func (t *createTool) Name() string { return "CronCreate" }

func (t *createTool) Description() string {
	return text(i18n.KeyToolScheduleCreateDescription)
}

func (t *createTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: 100_000}
}

func (t *createTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"cron":      map[string]any{"type": "string", "description": text(i18n.KeyToolScheduleSchemaCron)},
		"prompt":    map[string]any{"type": "string", "description": text(i18n.KeyToolScheduleSchemaPrompt)},
		"recurring": map[string]any{"type": "boolean", "description": text(i18n.KeyToolScheduleSchemaRecurring), "default": true},
		"durable":   map[string]any{"type": "boolean", "description": text(i18n.KeyToolScheduleSchemaDurable), "default": false},
	}, "cron", "prompt")
}

func (t *createTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if t == nil || t.service == nil {
		return failedResult(i18n.KeyToolScheduleExecutorUnavailable, "executor_unavailable"), nil
	}
	parsed, err := toolbase.ParseStrictInput[createInput](input)
	if err != nil {
		return failedResult(i18n.KeyToolScheduleInvalidInput, "invalid_input", err), nil
	}
	parsed.Cron = strings.TrimSpace(parsed.Cron)
	if parsed.Cron == "" {
		return failedResult(i18n.KeyToolScheduleInvalidExpression, "invalid_expression", parsed.Cron, fs.ErrInvalid), nil
	}
	expression, err := parseExpression(parsed.Cron)
	if err != nil {
		return failedResult(i18n.KeyToolScheduleInvalidExpression, "invalid_expression", parsed.Cron, err), nil
	}
	if strings.TrimSpace(parsed.Prompt) == "" {
		return failedResult(i18n.KeyToolScheduleInvalidInput, "invalid_input", fs.ErrInvalid), nil
	}
	now := t.service.clock().UTC()
	if _, _, ok := nextRun(expression, now, t.service.location, ""); !ok {
		return failedResult(i18n.KeyToolScheduleNoFutureFire, "no_future_fire", parsed.Cron), nil
	}
	recurring := true
	if parsed.Recurring != nil {
		recurring = *parsed.Recurring
	}
	durable := parsed.Durable != nil && *parsed.Durable
	job, err := t.service.create(parsed.Cron, parsed.Prompt, recurring, durable, t.currentAgentID(), t.currentProjectRoot(), now)
	if err != nil {
		return toolErrorResult(err, parsed.Cron, ""), nil
	}
	next, _, ok, err := t.service.nextScheduled(job)
	if err != nil {
		return toolErrorResult(err, parsed.Cron, job.ID), nil
	}
	nextFire := text(i18n.KeyToolScheduleNextFireUnknown)
	if ok {
		nextFire = next.In(t.service.location).Format(time.RFC3339Nano)
	}
	output := createOutput{
		ID: job.ID, Cron: job.Expression, NextFire: nextFire,
		Recurring: job.Recurring, Durable: job.Durable,
	}
	contentKey := i18n.KeyToolScheduleCreatedOneShot
	if recurring {
		contentKey = i18n.KeyToolScheduleCreatedRecurring
	}
	content := format(contentKey, job.ID, nextFire)
	if durable {
		content += " " + format(i18n.KeyToolScheduleCreatedPersisted, job.ID)
	} else {
		content += " " + format(i18n.KeyToolScheduleCreatedSession, job.ID)
	}
	return types.ToolResult{
		Content: content,
		Data:    output,
		Metadata: map[string]string{
			"id": job.ID, "cron": job.Expression, "next_fire": nextFire,
			"recurring": fmt.Sprintf("%t", recurring), "durable": fmt.Sprintf("%t", durable),
		},
	}, nil
}

func (t *createTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(createOutput)
	if !ok {
		return invalidResultBlock(data, toolUseID)
	}
	key := i18n.KeyToolScheduleCreatedOneShot
	if output.Recurring {
		key = i18n.KeyToolScheduleCreatedRecurring
	}
	content := format(key, output.ID, output.NextFire)
	if output.Durable {
		content += " " + format(i18n.KeyToolScheduleCreatedPersisted, output.ID)
	} else {
		content += " " + format(i18n.KeyToolScheduleCreatedSession, output.ID)
	}
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
		Content: content,
	}
}

func (t *createTool) currentAgentID() string {
	if t.agentID != "" {
		return t.agentID
	}
	return t.service.agentID()
}

func (t *createTool) currentProjectRoot() string {
	return t.projectRoot
}

type deleteTool struct {
	service *Service
	agentID string
}

var _ agentcontract.ScopedTool = (*deleteTool)(nil)

// NewDeleteTool returns the CronDelete tool.
func NewDeleteTool(service *Service) types.Tool { return &deleteTool{service: service} }

func (t *deleteTool) BindAgentScope(agentID, _ string) types.Tool {
	cloned := *t
	cloned.agentID = strings.TrimSpace(agentID)
	return &cloned
}

func (t *deleteTool) Name() string { return "CronDelete" }

func (t *deleteTool) Description() string { return text(i18n.KeyToolScheduleDeleteDescription) }

func (t *deleteTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: 100_000}
}

func (t *deleteTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"id": map[string]any{"type": "string", "description": text(i18n.KeyToolScheduleSchemaID)},
	}, "id")
}

func (t *deleteTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if t == nil || t.service == nil {
		return failedResult(i18n.KeyToolScheduleExecutorUnavailable, "executor_unavailable"), nil
	}
	parsed, err := toolbase.ParseStrictInput[deleteInput](input)
	if err != nil {
		return failedResult(i18n.KeyToolScheduleInvalidInput, "invalid_input", err), nil
	}
	parsed.ID = strings.TrimSpace(parsed.ID)
	if parsed.ID == "" {
		return failedResult(i18n.KeyToolScheduleInvalidInput, "invalid_input", fs.ErrInvalid), nil
	}
	if err := t.service.delete(parsed.ID, t.currentAgentID()); err != nil {
		return toolErrorResult(err, "", parsed.ID), nil
	}
	output := deleteOutput(parsed)
	return types.ToolResult{
		Content: format(i18n.KeyToolScheduleCancelled, parsed.ID), Data: output,
		Metadata: map[string]string{"id": parsed.ID},
	}, nil
}

func (t *deleteTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(deleteOutput)
	if !ok {
		return invalidResultBlock(data, toolUseID)
	}
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
		Content: format(i18n.KeyToolScheduleCancelled, output.ID),
	}
}

func (t *deleteTool) currentAgentID() string {
	if t.agentID != "" {
		return t.agentID
	}
	return t.service.agentID()
}

type listTool struct {
	service *Service
	agentID string
}

var _ agentcontract.ScopedTool = (*listTool)(nil)

// NewListTool returns the CronList tool.
func NewListTool(service *Service) types.Tool { return &listTool{service: service} }

func (t *listTool) BindAgentScope(agentID, _ string) types.Tool {
	cloned := *t
	cloned.agentID = strings.TrimSpace(agentID)
	return &cloned
}

func (t *listTool) Name() string { return "CronList" }

func (t *listTool) Description() string { return text(i18n.KeyToolScheduleListDescription) }

func (t *listTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *listTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (t *listTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if t == nil || t.service == nil {
		return failedResult(i18n.KeyToolScheduleExecutorUnavailable, "executor_unavailable"), nil
	}
	if _, err := toolbase.ParseStrictInput[struct{}](input); err != nil {
		return failedResult(i18n.KeyToolScheduleInvalidInput, "invalid_input", err), nil
	}
	jobs, err := t.service.list(t.currentAgentID())
	if err != nil {
		return toolErrorResult(err, "", ""), nil
	}
	output := listOutput{Jobs: make([]listJobOutput, 0, len(jobs))}
	for _, job := range jobs {
		nextFire := text(i18n.KeyToolScheduleNextFireUnknown)
		if job.Pending != nil {
			nextFire = job.Pending.ScheduledAt.In(t.service.location).Format(time.RFC3339Nano)
		} else if next, _, ok, nextErr := t.service.nextScheduled(job); nextErr == nil && ok {
			nextFire = next.In(t.service.location).Format(time.RFC3339Nano)
		}
		output.Jobs = append(output.Jobs, listJobOutput{
			ID: job.ID, Cron: job.Expression, Prompt: job.Prompt, NextFire: nextFire,
			Recurring: job.Recurring, Durable: job.Durable,
		})
	}
	return types.ToolResult{Content: listContent(output), Data: output}, nil
}

func (t *listTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(listOutput)
	if !ok {
		return invalidResultBlock(data, toolUseID)
	}
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: listContent(output),
	}
}

func (t *listTool) currentAgentID() string {
	if t.agentID != "" {
		return t.agentID
	}
	return t.service.agentID()
}

func listContent(output listOutput) string {
	if len(output.Jobs) == 0 {
		return text(i18n.KeyToolScheduleNoJobs)
	}
	var builder strings.Builder
	for index, job := range output.Jobs {
		if index > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(format(i18n.KeyToolScheduleListRow,
			job.ID, job.Cron, job.NextFire, truncatePrompt(job.Prompt)))
	}
	return builder.String()
}

func truncatePrompt(prompt string) string {
	const maximumWidth = 80
	if newline := strings.IndexByte(prompt, '\n'); newline >= 0 {
		prompt = prompt[:newline]
	}
	if uniseg.StringWidth(prompt) <= maximumWidth {
		return prompt
	}
	var builder strings.Builder
	width := 0
	graphemes := uniseg.NewGraphemes(prompt)
	for graphemes.Next() {
		segmentWidth := graphemes.Width()
		if width+segmentWidth > maximumWidth-1 {
			break
		}
		builder.WriteString(graphemes.Str())
		width += segmentWidth
	}
	builder.WriteRune('…')
	return builder.String()
}

func invalidResultBlock(data any, toolUseID string) types.ToolResultBlock {
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
		Content: format(i18n.KeyToolScheduleInvalidTypedResult, fmt.Sprintf("%T", data)), IsError: true,
	}
}

func toolErrorResult(err error, expression, id string) types.ToolResult {
	switch domainErrorKind(err) {
	case errorKindTooMany:
		return failedResult(i18n.KeyToolScheduleTooMany, "too_many_jobs", maxJobs)
	case errorKindNotFound:
		return failedResult(i18n.KeyToolScheduleNotFound, "not_found", id)
	case errorKindOwnerDenied:
		return failedResult(i18n.KeyToolScheduleOwnerDenied, "owner_denied", id)
	case errorKindDurableDenied:
		return failedResult(i18n.KeyToolScheduleAgentDenied, "durable_agent_denied")
	case errorKindStoreRead:
		if errors.Is(err, fs.ErrInvalid) || errors.Is(err, fs.ErrPermission) {
			return failedWrappedResult(i18n.KeyToolScheduleStoreSecurity, "store_security", err)
		}
		return failedWrappedResult(i18n.KeyToolScheduleStoreReadFailed, "store_read_failed", err)
	case errorKindStoreWrite:
		if errors.Is(err, fs.ErrInvalid) || errors.Is(err, fs.ErrPermission) {
			return failedWrappedResult(i18n.KeyToolScheduleStoreSecurity, "store_security", err)
		}
		return failedWrappedResult(i18n.KeyToolScheduleStoreWriteFailed, "store_write_failed", err)
	case errorKindStoreVersion:
		var version schemaVersionError
		if errors.As(err, &version) {
			return failedResult(i18n.KeyToolScheduleStoreVersion, "store_version", int(version))
		}
		return failedResult(i18n.KeyToolScheduleStoreVersion, "store_version", 0)
	case errorKindID:
		return failedResult(i18n.KeyToolScheduleRandomIDFailed, "random_id_failed")
	case errorKindStoreInvalid:
		if expression != "" {
			return failedWrappedResult(i18n.KeyToolScheduleInvalidExpression, "invalid_expression", err, expression)
		}
		return failedWrappedResult(i18n.KeyToolScheduleStoreCorrupt, "store_corrupt", err)
	default:
		return failedResult(i18n.KeyToolScheduleInvalidInput, "schedule_failed", err)
	}
}
