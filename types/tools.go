package types

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// JSONSchema represents the JSON Schema subset used by built-in tool input
// and output contracts. Properties remain open-ended so nested schemas can
// use the full JSON Schema vocabulary without another Go type hierarchy.
type JSONSchema struct {
	Type                 string         `json:"type"`
	Properties           map[string]any `json:"properties"`
	Required             []string       `json:"required,omitempty"`
	Description          string         `json:"description,omitempty"`
	AdditionalProperties any            `json:"additionalProperties,omitempty"`
	Items                any            `json:"items,omitempty"`
	Enum                 []any          `json:"enum,omitempty"`
	OneOf                []any          `json:"oneOf,omitempty"`
	AnyOf                []any          `json:"anyOf,omitempty"`
	AllOf                []any          `json:"allOf,omitempty"`
	Defs                 map[string]any `json:"$defs,omitempty"`
}

// StrictObjectSchema constructs the JSON Schema equivalent of z.strictObject.
func StrictObjectSchema(properties map[string]any, required ...string) JSONSchema {
	filteredRequired := make([]string, 0, len(required))
	for _, field := range required {
		if field != "" {
			filteredRequired = append(filteredRequired, field)
		}
	}
	return JSONSchema{
		Type:                 "object",
		Properties:           properties,
		Required:             filteredRequired,
		AdditionalProperties: false,
	}
}

// RejectsUnknownFields reports whether the schema closes the root object to
// properties not explicitly declared in Properties.
func (s JSONSchema) RejectsUnknownFields() bool {
	if s.Type != "object" {
		return false
	}
	switch value := s.AdditionalProperties.(type) {
	case bool:
		return !value
	case *bool:
		return value != nil && !*value
	default:
		return false
	}
}

// ToolMetadata is scheduling and result-budget metadata shared by providers,
// permission logic, and the execution loop.
type ToolMetadata struct {
	ReadOnly           bool `json:"read_only"`
	Search             bool `json:"search"`
	Write              bool `json:"write"`
	Destructive        bool `json:"destructive"`
	ConcurrencySafe    bool `json:"concurrency_safe"`
	MaxResultSizeChars int  `json:"max_result_size_chars,omitempty"`
}

// UnlimitedToolResultSize mirrors TS Infinity for tools that self-bound their
// output and must never have results persisted by the shared result store.
const UnlimitedToolResultSize = -1

type ToolResultMapper interface {
	MapToolResultToToolResultBlock(data any, toolUseID string) ToolResultBlock
}

// ToolPermissionRejectionMapper lets tools with a typed approval lifecycle
// preserve domain-specific rejection feedback without executing the tool. The
// loop calls it only after an interactive permission handler denies an ask.
type ToolPermissionRejectionMapper interface {
	MapToolPermissionRejection(input map[string]any, toolUseID, message string) ToolResultBlock
}

// ToolDefinitionType identifies the provider wire contract for a client tool.
// The zero value is the ordinary JSON function contract so existing providers
// cannot accidentally opt into freeform input.
type ToolDefinitionType string

const (
	ToolDefinitionTypeFunction ToolDefinitionType = ""
	ToolDefinitionTypeCustom   ToolDefinitionType = "custom"
)

// ToolInputFormat constrains freeform custom-tool input on providers that
// explicitly implement the contract. Grammar is currently the only supported
// format because an unconstrained text tool would weaken local validation.
type ToolInputFormat struct {
	Type       string `json:"type"`
	Syntax     string `json:"syntax"`
	Definition string `json:"definition"`
}

// CustomToolDefinitionProvider is an opt-in contract implemented by tools
// whose raw provider input must bypass JSON argument encoding. Returning false
// keeps the tool on the ordinary function surface.
type CustomToolDefinitionProvider interface {
	CustomToolInputFormat() (ToolInputFormat, bool)
}

// CustomToolInputDecoder projects exact freeform provider input into the
// ordinary local execution map. Permission checks, hooks, schema validation,
// and Execute must all receive this single canonical projection.
type CustomToolInputDecoder interface {
	DecodeCustomToolInput(raw string) (map[string]any, error)
}

// ToolDefinition is the provider-facing tool contract. Strict is derived from
// the input schema instead of a second independently maintained declaration.
// InputSchema remains present for local validation even when Type is custom;
// providers must serialize only Format for custom definitions.
type ToolDefinition struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	InputSchema JSONSchema         `json:"input_schema"`
	Strict      bool               `json:"strict,omitempty"`
	Type        ToolDefinitionType `json:"type,omitempty"`
	Format      *ToolInputFormat   `json:"format,omitempty"`
}

func (d ToolDefinition) IsCustom() bool { return d.Type == ToolDefinitionTypeCustom }

// ServerToolDefinition is an API-hosted tool schema. It is intentionally
// separate from ToolDefinition so ordinary Go client tools can never be
// serialized as provider-defined server tools by name alone.
type ServerToolDefinition struct {
	Type           string   `json:"type"`
	Name           string   `json:"name"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
	MaxUses        int      `json:"max_uses,omitempty"`
}

// Tool is the interface that all tools must implement
type Tool interface {
	// Name returns the tool's unique identifier
	Name() string

	// Description returns a human-readable description of the tool
	Description() string

	// Schema returns the JSON Schema for the tool's input parameters
	Schema() JSONSchema

	// Execute runs the tool with the given input and returns the result.
	//
	// Error semantics:
	//   - Return (ToolResult{IsError: true}, nil) for business/tool-level errors
	//     that the LLM should see and reason about (e.g., file not found, invalid
	//     pattern, command failed). These are normal tool outcomes.
	//   - Return (ToolResult{}, err) for infrastructure failures that should
	//     interrupt the loop (e.g., context cancelled, unrecoverable system error).
	//     The error propagates up and may abort the conversation turn.
	Execute(ctx context.Context, input map[string]any) (ToolResult, error)
}

// PermissionBehavior is a tool-specific pre-execution permission outcome.
// Passthrough delegates the final decision to the runtime's general
// permission handler; Ask carries the prompt and optional persistent-rule
// suggestions needed by interactive and SDK frontends.
type PermissionBehavior string

const (
	PermissionBehaviorAllow       PermissionBehavior = "allow"
	PermissionBehaviorDeny        PermissionBehavior = "deny"
	PermissionBehaviorAsk         PermissionBehavior = "ask"
	PermissionBehaviorPassthrough PermissionBehavior = "passthrough"
)

type PermissionUpdateType string

const (
	PermissionUpdateAddRules       PermissionUpdateType = "addRules"
	PermissionUpdateAddDirectories PermissionUpdateType = "addDirectories"
)

type PermissionUpdateDestination string

const (
	PermissionDestinationLocalSettings PermissionUpdateDestination = "localSettings"
	PermissionDestinationSession       PermissionUpdateDestination = "session"
)

// PermissionRuleValue mirrors the model-visible value stored in permissions
// settings. RuleContent is omitted for blanket tool rules.
type PermissionRuleValue struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionUpdate represents both in-memory and persistent changes proposed
// by a tool-specific permission check. The fields used depend on Type.
type PermissionUpdate struct {
	Type        PermissionUpdateType        `json:"type"`
	Destination PermissionUpdateDestination `json:"destination"`
	Behavior    PermissionBehavior          `json:"behavior,omitempty"`
	Rules       []PermissionRuleValue       `json:"rules,omitempty"`
	Directories []string                    `json:"directories,omitempty"`
	Mode        string                      `json:"mode,omitempty"`
}

const (
	ToolFeatureTeams           = "teams"
	ToolFeatureRemoteTrigger   = "remote_trigger"
	ToolFeatureCron            = "cron"
	ToolFeatureWebSearch       = "web_search"
	ToolFeatureToolSearch      = "tool_search"
	ToolFeaturePlanMode        = "plan_mode"
	ToolFeatureWorktree        = "worktree"
	ToolFeatureSendUserMessage = "send_user_message"
)

// ToolRuntimeContext is the immutable-by-convention session snapshot used by
// both visibility and permission checks. Providers must return defensive map
// and slice copies because checks may run concurrently.
type ToolRuntimeContext struct {
	SessionID      string                `json:"sessionId,omitempty"`
	ProjectRoot    string                `json:"projectRoot,omitempty"`
	AllowedDirs    []string              `json:"allowedDirs,omitempty"`
	Interactive    bool                  `json:"interactive,omitempty"`
	AgentID        string                `json:"agentId,omitempty"`
	PermissionMode string                `json:"permissionMode,omitempty"`
	Provider       string                `json:"provider,omitempty"`
	Model          string                `json:"model,omitempty"`
	Features       map[string]bool       `json:"features,omitempty"`
	AllowedTools   map[string]bool       `json:"allowedTools,omitempty"`
	DeniedTools    map[string]bool       `json:"deniedTools,omitempty"`
	AllowedRules   []PermissionRuleValue `json:"allowedRules,omitempty"`
	DeniedRules    []PermissionRuleValue `json:"deniedRules,omitempty"`
	AskRules       []PermissionRuleValue `json:"askRules,omitempty"`
}

func (c ToolRuntimeContext) FeatureEnabled(name string) bool {
	return c.Features != nil && c.Features[name]
}

// ToolRuntimeContextProvider supplies the latest session snapshot without
// rebuilding a registry when cwd, provider, or feature state changes.
type ToolRuntimeContextProvider interface {
	ToolRuntimeContext() ToolRuntimeContext
}

type ToolEnabledProvider interface {
	IsEnabled(runtime ToolRuntimeContext) bool
}

type ToolPermissionRequest struct {
	SessionID     string
	TurnID        string
	ToolUseID     string
	ApprovalEpoch string
	Mode          string
	AvoidPrompts  bool
	Runtime       ToolRuntimeContext
}

// ToolPermissionBinding identifies one exact permission-to-execution handoff.
// ApprovalEpoch is the owning loop's private active-run token; empty identity
// remains supported for direct Registry dispatch of commands that do not ask.
type ToolPermissionBinding struct {
	SessionID string
	// PolicyOwnerSessionID independently identifies the session that owns the
	// runtime-policy snapshot. It may differ from SessionID for a child execution
	// inheriting parent policy; both identities are bound and revalidated at
	// promotion and consumption.
	PolicyOwnerSessionID string
	TurnID               string
	ToolUseID            string
	ApprovalEpoch        string
	PolicySnapshotDigest string
	// PolicyRisk binds the one-time grant to the typed analyzer risk observed at
	// preflight. It is independently checked in addition to the policy-code
	// fingerprint and exact input digest.
	PolicyRisk PolicyRisk
	// SandboxCapability binds a shell permission grant to the immutable
	// executable authority observed during tool preflight.
	SandboxCapability string
}

func (r ToolPermissionRequest) Binding() ToolPermissionBinding {
	return ToolPermissionBinding{
		SessionID: r.SessionID, TurnID: r.TurnID, ToolUseID: r.ToolUseID,
		ApprovalEpoch: r.ApprovalEpoch,
	}
}

type ToolPermissionResult struct {
	Behavior       PermissionBehavior
	Message        string
	UpdatedInput   map[string]any
	Suggestions    []PermissionUpdate
	BlockedPath    string
	PolicyDecision *PolicyDecision
	// ExecutionPolicyCode binds a permission handoff to the exact internal
	// analyzer verdict even when no user-facing PolicyDecision is required.
	ExecutionPolicyCode string
	PermissionGrant     string
	PermissionBinding   ToolPermissionBinding
	// Required marks bypass-immune asks such as safety and explicit ask rules.
	Required bool
	// ToolLocalReadOnlyAllow records that this exact invocation passed a
	// read-only tool's own scope checks before its final decision was delegated
	// to the runtime permission policy. It must not override required asks or
	// explicit ask/deny rules.
	ToolLocalReadOnlyAllow bool
	// Sandboxed is the tool-check snapshot proving that this exact invocation
	// was evaluated with a real OS sandbox capability. Permission handlers may
	// use sandbox auto-approval only when this signed handoff bit is true.
	Sandboxed bool
	// SandboxCapability is the immutable capability ID corresponding to
	// Sandboxed. An empty ID can never authorize sandbox auto-approval.
	SandboxCapability string
	// RuntimeSnapshot and ToolMetadata are detached, process-local presentation
	// context captured by Registry during the permission check. They describe
	// the basis shown to a reviewer and never grant or widen execution authority.
	RuntimeSnapshot *ToolRuntimeContext `json:"-"`
	ToolMetadata    ToolMetadata        `json:"-"`
}

// ToolPermissionChecker lets a tool make its content-specific decision before
// Execute. General policy and UI prompting remain owned by the dispatcher.
type ToolPermissionChecker interface {
	CheckPermissions(ctx context.Context, input map[string]any, request ToolPermissionRequest) (ToolPermissionResult, error)
}

// ToolMetadataProvider supports tools whose classification depends on input,
// for example Bash commands and ExitWorktree(action=remove).
type ToolMetadataProvider interface {
	ToolMetadata(input map[string]any) ToolMetadata
}

// ToolResult represents the output of a tool execution.
//
// IsError indicates a tool-level business error (the tool ran but the operation
// failed in an expected way). This is distinct from the Go error return value,
// which indicates infrastructure failure. See Tool.Execute documentation.
type ToolResult struct {
	Content       string         `json:"content"`
	ContentBlocks []ContentBlock `json:"-"`
	// Data is the typed programmatic result. It is deliberately excluded from
	// provider JSON; ToolResultMapper owns the model-visible representation.
	Data        any       `json:"-"`
	IsError     bool      `json:"is_error,omitempty"`
	NewMessages []Message `json:"-"`
	// Usage accounts for nested provider calls made by a tool, such as
	// WebSearch's provider-native server-tool request.
	Usage *Usage `json:"-"`

	// Metadata carries tool-specific key/value annotations (e.g. wasReadOnly,
	// semanticCategory). Optional; consumers must tolerate nil maps and
	// missing keys.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Outcome is the deterministic local execution state. It is excluded from
	// provider JSON and copied to ToolResultBlock for presentation/audit.
	Outcome      ToolOutcome            `json:"-"`
	Completeness ToolResultCompleteness `json:"-"`
}

// ToolErrorRange is a 1-based inclusive range exposed in the stable,
// language-independent tool error recovery protocol.
type ToolErrorRange struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

// ToolErrorCoverage describes what file range evidence was available when a
// mutation precondition failed. It deliberately excludes file contents.
type ToolErrorCoverage struct {
	Complete   bool             `json:"complete"`
	TotalLines int              `json:"total_lines,omitempty"`
	Observed   []ToolErrorRange `json:"observed,omitempty"`
	Required   []ToolErrorRange `json:"required,omitempty"`
}

// ToolErrorRetry is an allowlisted recovery action. It cannot carry arbitrary
// tool input, raw causes, or file content into the model-facing envelope.
type ToolErrorRetry struct {
	Action   string                    `json:"action"`
	Tool     string                    `json:"tool"`
	FilePath string                    `json:"file_path,omitempty"`
	Offset   int                       `json:"offset,omitempty"`
	Limit    int                       `json:"limit,omitempty"`
	Requests []ToolErrorInspectRequest `json:"requests,omitempty"`
}

// ToolErrorInspectRequest is the allowlisted subset of Inspect's batch input
// used by mutation recovery. It cannot smuggle arbitrary tool arguments.
type ToolErrorInspectRequest struct {
	ID     string           `json:"id"`
	Kind   string           `json:"kind"`
	Path   string           `json:"path"`
	Ranges []ToolErrorRange `json:"ranges,omitempty"`
}

// ToolErrorData is the typed P0 file-tool error contract. Local renderers use
// the localized ToolResult.Content; providers additionally receive a compact
// allowlisted envelope so recovery does not depend on parsing that language.
type ToolErrorData struct {
	Schema    string             `json:"schema"`
	Code      string             `json:"code"`
	Retryable bool               `json:"retryable"`
	Path      string             `json:"path,omitempty"`
	Coverage  *ToolErrorCoverage `json:"coverage,omitempty"`
	Retry     *ToolErrorRetry    `json:"retry,omitempty"`
}

func (r ToolResult) HasStructuredContent() bool {
	return len(r.ContentBlocks) > 0
}

func (r ToolResult) TextContent() string {
	return toolResultTextContent(r.Content, r.ContentBlocks)
}

func (r ToolResult) HasMediaContent() bool {
	return toolResultHasMediaContent(r.ContentBlocks)
}

// DecodeStrictToolInput decodes into a typed Go input while rejecting fields
// that are not represented by the destination struct.
func DecodeStrictToolInput[T any](input map[string]any) (T, error) {
	var result T
	data, err := json.Marshal(input)
	if err != nil {
		return result, fmt.Errorf("marshal tool input: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode tool input: %w", err)
	}
	return result, nil
}

// ToolInputValidationError is returned before Execute when a strict schema
// receives unsupported root properties.
type ToolInputValidationError struct {
	ToolName         string
	UnexpectedFields []string
}

func (e *ToolInputValidationError) Error() string {
	return e.LocalizedToolInputValidation(i18n.DetectOrLoadLanguage())
}

// LocalizedToolInputValidation renders product copy in lang while preserving
// the tool identifier and schema field names as raw formatting parameters.
func (e *ToolInputValidationError) LocalizedToolInputValidation(lang i18n.Language) string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, len(e.UnexpectedFields))
	for _, field := range e.UnexpectedFields {
		parts = append(parts, i18n.Format(lang, i18n.KeyToolInputValidationUnexpectedParameter, field))
	}
	key := i18n.KeyToolInputValidationFailedPlural
	if len(e.UnexpectedFields) == 1 {
		key = i18n.KeyToolInputValidationFailedSingle
	}
	return i18n.Format(lang, key, e.ToolName, strings.Join(parts, "\n"))
}

// ValidateToolInput enforces the root strict-object boundary advertised by a
// tool schema. Value and type validation remain owned by the individual tool.
func ValidateToolInput(t Tool, input map[string]any) error {
	if t == nil {
		return nil
	}
	schema := t.Schema()
	if !schema.RejectsUnknownFields() {
		return nil
	}
	unexpected := make([]string, 0)
	for field := range input {
		if _, ok := schema.Properties[field]; !ok {
			unexpected = append(unexpected, field)
		}
	}
	if len(unexpected) == 0 {
		return nil
	}
	sort.Strings(unexpected)
	return &ToolInputValidationError{ToolName: t.Name(), UnexpectedFields: unexpected}
}

// MapToolResult separates typed result data from provider-visible content.
// Tools with typed Data may provide a model-facing mapper; explicit Content
// and ContentBlocks remain authoritative for tools without typed Data.
func MapToolResult(t Tool, result ToolResult, toolUseID string) ToolResultBlock {
	block := ToolResultBlock{
		Type:          ContentTypeToolResult,
		ToolUseID:     toolUseID,
		Content:       result.Content,
		ContentBlocks: result.ContentBlocks,
		Data:          result.Data,
		IsError:       result.IsError,
		NewMessages:   result.NewMessages,
		Usage:         result.Usage,
		Metadata:      result.Metadata,
		Outcome:       result.Outcome,
		Completeness:  result.Completeness.Clone(),
	}
	if result.IsError {
		var structured *ToolErrorData
		switch value := result.Data.(type) {
		case ToolErrorData:
			copy := value
			structured = &copy
		case *ToolErrorData:
			structured = value
		}
		if structured != nil {
			if payload, err := json.Marshal(structured); err == nil {
				// i18n:allow display-literal protocol -- Stable model-facing recovery envelope; localized copy remains in result.Content.
				block.Content = strings.TrimSpace(result.Content) + "\n<tool_error>" + string(payload) + "</tool_error>"
			}
			return applyToolResultContract(t, block)
		}
		// Error Data is local typed diagnostic state, never an input to a
		// success mapper. This prevents arbitrary maps or internal causes from
		// replacing the localized public error text.
		return applyToolResultContract(t, block)
	}
	mapper, ok := t.(ToolResultMapper)
	if !ok || result.Data == nil {
		return applyToolResultContract(t, block)
	}

	mapped := mapper.MapToolResultToToolResultBlock(result.Data, toolUseID)
	if mapped.Type == "" {
		mapped.Type = ContentTypeToolResult
	}
	if mapped.ToolUseID == "" {
		mapped.ToolUseID = toolUseID
	}
	mapped.Data = result.Data
	mapped.Usage = result.Usage
	if !result.Completeness.IsZero() {
		mapped.Completeness = result.Completeness.Clone()
	}
	mapped.IsError = mapped.IsError || result.IsError
	if mapped.Outcome == "" {
		mapped.Outcome = result.Outcome
	}
	if len(result.NewMessages) > 0 {
		mapped.NewMessages = append(append([]Message(nil), result.NewMessages...), mapped.NewMessages...)
	}
	if len(result.Metadata) > 0 {
		metadata := make(map[string]string, len(result.Metadata)+len(mapped.Metadata))
		for key, value := range result.Metadata {
			metadata[key] = value
		}
		for key, value := range mapped.Metadata {
			metadata[key] = value
		}
		mapped.Metadata = metadata
	}
	return applyToolResultContract(t, mapped)
}

func applyToolResultContract(t Tool, block ToolResultBlock) ToolResultBlock {
	provider, ok := t.(ToolMetadataProvider)
	if !ok {
		return block
	}
	maxResultSizeChars := provider.ToolMetadata(nil).MaxResultSizeChars
	if maxResultSizeChars == 0 {
		return block
	}
	metadata := make(map[string]string, len(block.Metadata)+1)
	for key, value := range block.Metadata {
		metadata[key] = value
	}
	if maxResultSizeChars == UnlimitedToolResultSize {
		metadata["maxResultSizeChars"] = "inf"
	} else if maxResultSizeChars > 0 {
		metadata["maxResultSizeChars"] = strconv.Itoa(maxResultSizeChars)
	}
	block.Metadata = metadata
	return block
}

// ToDefinition converts a Tool interface to an API ToolDefinition
func ToDefinition(t Tool) ToolDefinition {
	schema := t.Schema()
	definition := ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: schema,
		Strict:      schema.RejectsUnknownFields(),
	}
	if custom, ok := t.(CustomToolDefinitionProvider); ok {
		if format, enabled := custom.CustomToolInputFormat(); enabled {
			definition.Type = ToolDefinitionTypeCustom
			definition.Format = &format
			definition.Strict = false
		}
	}
	return definition
}

// ToDefinitions converts a slice of Tools to API ToolDefinitions
func ToDefinitions(tools []Tool) []ToolDefinition {
	defs := make([]ToolDefinition, len(tools))
	for i, t := range tools {
		defs[i] = ToDefinition(t)
	}
	return defs
}

// MarshalJSON for Message to handle ContentBlock interface serialization
func (m Message) MarshalJSON() ([]byte, error) {
	type messageJSON struct {
		ID                string                    `json:"id,omitempty"`
		Role              Role                      `json:"role"`
		Content           []json.RawMessage         `json:"content"`
		IsMeta            bool                      `json:"is_meta,omitempty"`
		InternalKind      InternalMessageKind       `json:"internal_kind,omitempty"`
		DeveloperMetadata *DeveloperMessageMetadata `json:"developer_metadata,omitempty"`
	}

	msg := messageJSON{
		ID:                m.ID,
		Role:              m.Role,
		IsMeta:            m.IsMeta,
		InternalKind:      m.InternalKind,
		DeveloperMetadata: m.DeveloperMetadata,
	}
	// Ensure content serializes as [] not null when empty.
	msg.Content = make([]json.RawMessage, 0, len(m.Content))
	for _, block := range m.Content {
		// UnknownBlock already carries raw JSON — re-emit it verbatim.
		if ub, ok := block.(UnknownBlock); ok {
			msg.Content = append(msg.Content, ub.Raw)
			continue
		}
		data, err := json.Marshal(block)
		if err != nil {
			return nil, err
		}
		msg.Content = append(msg.Content, data)
	}
	return json.Marshal(msg)
}

func (b ToolResultBlock) MarshalJSON() ([]byte, error) {
	type toolResultJSON struct {
		Type      ContentType        `json:"type"`
		ToolUseID string             `json:"tool_use_id"`
		Content   any                `json:"content"`
		IsError   bool               `json:"is_error,omitempty"`
		Outcome   ToolOutcome        `json:"outcome,omitempty"`
		ToolType  ToolDefinitionType `json:"tool_type,omitempty"`
	}

	content := any(b.Content)
	if len(b.ContentBlocks) > 0 {
		content = b.ContentBlocks
	}

	return json.Marshal(toolResultJSON{
		Type:      ContentTypeToolResult,
		ToolUseID: b.ToolUseID,
		Content:   content,
		IsError:   b.IsError,
		Outcome:   b.Outcome,
		ToolType:  b.ToolType,
	})
}

func (b *ToolResultBlock) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type      ContentType        `json:"type"`
		ToolUseID string             `json:"tool_use_id"`
		Content   json.RawMessage    `json:"content"`
		IsError   bool               `json:"is_error,omitempty"`
		Outcome   ToolOutcome        `json:"outcome,omitempty"`
		ToolType  ToolDefinitionType `json:"tool_type,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	b.Type = raw.Type
	if b.Type == "" {
		b.Type = ContentTypeToolResult
	}
	b.ToolUseID = raw.ToolUseID
	b.IsError = raw.IsError
	b.Outcome = raw.Outcome
	b.ToolType = raw.ToolType
	b.Content = ""
	b.ContentBlocks = nil
	b.Data = nil

	content := bytes.TrimSpace(raw.Content)
	if len(content) == 0 || bytes.Equal(content, []byte("null")) {
		return nil
	}
	if content[0] == '[' {
		var rawBlocks []json.RawMessage
		if err := json.Unmarshal(content, &rawBlocks); err != nil {
			return err
		}
		blocks, err := decodeContentBlocks(rawBlocks)
		if err != nil {
			return err
		}
		b.ContentBlocks = blocks
		return nil
	}
	return json.Unmarshal(content, &b.Content)
}

func decodeContentBlocks(rawBlocks []json.RawMessage) ([]ContentBlock, error) {
	blocks := make([]ContentBlock, 0, len(rawBlocks))
	for _, rawBlock := range rawBlocks {
		var typeHolder struct {
			Type ContentType `json:"type"`
		}
		if err := json.Unmarshal(rawBlock, &typeHolder); err != nil {
			return nil, err
		}

		var block ContentBlock
		switch typeHolder.Type {
		case ContentTypeText:
			var b TextBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		case ContentTypeToolUse:
			var b ToolUseBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		case ContentTypeToolResult:
			var b ToolResultBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		case ContentTypeToolReference:
			var b ToolReferenceBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		case ContentTypeThinking:
			var b ThinkingBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		case ContentTypeImage:
			var b ImageBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		case ContentTypeDocument:
			var b DocumentBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		case ContentTypeReplacement:
			var b ContentReplacementBlock
			if err := json.Unmarshal(rawBlock, &b); err != nil {
				return nil, err
			}
			block = b
		default:
			block = UnknownBlock{
				Type: typeHolder.Type,
				Raw:  json.RawMessage(append([]byte(nil), rawBlock...)),
			}
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// UnmarshalJSON for Message to handle ContentBlock interface deserialization
func (m *Message) UnmarshalJSON(data []byte) error {
	type messageRaw struct {
		ID                string                    `json:"id,omitempty"`
		Role              Role                      `json:"role"`
		Content           []json.RawMessage         `json:"content"`
		IsMeta            bool                      `json:"is_meta,omitempty"`
		InternalKind      InternalMessageKind       `json:"internal_kind,omitempty"`
		DeveloperMetadata *DeveloperMessageMetadata `json:"developer_metadata,omitempty"`
	}

	var raw messageRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// JSON is an external description, never a runtime capability. Clear any
	// authority even when UnmarshalJSON reuses an existing Message value.
	m.clearInternalControlProvenance()
	m.ClearProviderContinuation()
	m.ID = raw.ID
	m.Role = raw.Role
	m.IsMeta = raw.IsMeta
	m.InternalKind = raw.InternalKind
	m.DeveloperMetadata = raw.DeveloperMetadata
	m.Content = make([]ContentBlock, 0, len(raw.Content))

	blocks, err := decodeContentBlocks(raw.Content)
	if err != nil {
		return err
	}
	m.Content = append(m.Content, blocks...)

	return nil
}
