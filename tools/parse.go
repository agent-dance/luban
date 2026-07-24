package tools

import (
	"bytes"
	"encoding/json"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// parseInput unmarshals a map[string]any tool input into a typed struct.
// It round-trips through JSON to leverage struct tags and type checking,
// replacing dozens of manual type assertions with a single call.
func parseInput[T any](input map[string]any) (T, error) {
	var result T
	data, err := json.Marshal(input)
	if err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseMarshal, err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseDecode, err)
	}
	return result, nil
}

// parseInputOrError is a convenience wrapper that returns a ToolResult error
// on parse failure, suitable for direct use in Execute methods.
func parseInputOrError[T any](input map[string]any) (T, *types.ToolResult) {
	result, err := parseInput[T](input)
	if err != nil {
		return result, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, err), IsError: true}
	}
	return result, nil
}

// parseStrictInput is the z.strictObject equivalent for tools whose TS input
// contract rejects unknown fields. It is intentionally opt-in so incremental
// tool migrations do not change legacy schemas.
func parseStrictInput[T any](input map[string]any) (T, error) {
	var result T
	data, err := json.Marshal(input)
	if err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseMarshal, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, i18n.WrapError(i18n.KeyToolSourceSinkParseDecode, err)
	}
	return result, nil
}

func parseStrictInputOrError[T any](input map[string]any) (T, *types.ToolResult) {
	result, err := parseStrictInput[T](input)
	if err != nil {
		return result, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, err), IsError: true}
	}
	return result, nil
}

// --- Typed input structs for each tool ---

// BashInput is the typed input for BashTool
type BashInput struct {
	Command                   string  `json:"command"`
	Timeout                   float64 `json:"timeout,omitempty"`
	Description               string  `json:"description,omitempty"`
	RunInBackground           bool    `json:"run_in_background,omitempty"`
	DangerouslyDisableSandbox bool    `json:"dangerouslyDisableSandbox,omitempty"`
}

// PowerShellInput is the typed input for PowerShellTool.
type PowerShellInput struct {
	Command         string  `json:"command"`
	Timeout         float64 `json:"timeout,omitempty"`
	Description     string  `json:"description,omitempty"`
	RunInBackground bool    `json:"run_in_background,omitempty"`
}

// FileReadInput is the typed input for FileReadTool
type FileReadInput struct {
	FilePath string  `json:"file_path"`
	Offset   float64 `json:"offset,omitempty"`
	Limit    float64 `json:"limit,omitempty"`
	Pages    string  `json:"pages,omitempty"`
}

// FileWriteInput is the typed input for FileWriteTool
type FileWriteInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// FileEditInput is the typed input for FileEditTool
type FileEditInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// GlobInput is the typed input for GlobTool
type GlobInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// GrepInput is the typed input for GrepTool
type GrepInput struct {
	Pattern         string   `json:"pattern"`
	Path            string   `json:"path,omitempty"`
	Glob            string   `json:"glob,omitempty"`
	OutputMode      string   `json:"output_mode,omitempty"`
	CaseInsensitive bool     `json:"-i,omitempty"`
	ShowLineNumbers *bool    `json:"-n,omitempty"`
	ContextBefore   *float64 `json:"-B,omitempty"`
	ContextAfter    *float64 `json:"-A,omitempty"`
	ContextC        *float64 `json:"-C,omitempty"`
	Context         *float64 `json:"context,omitempty"`
	Type            string   `json:"type,omitempty"`
	HeadLimit       *float64 `json:"head_limit,omitempty"`
	Offset          *float64 `json:"offset,omitempty"`
	Multiline       bool     `json:"multiline,omitempty"`
}

// AgentInput is the typed input for AgentTool
type AgentInput struct {
	Description     string `json:"description"`
	Prompt          string `json:"prompt"`
	SubagentType    string `json:"subagent_type,omitempty"`
	Model           string `json:"model,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
	Name            string `json:"name,omitempty"`
	TeamName        string `json:"team_name,omitempty"`
	Isolation       string `json:"isolation,omitempty"`
	CWD             string `json:"cwd,omitempty"`
	Color           string `json:"color,omitempty"`
}

// normalizeAgentInputCompatibility treats an explicit false/null isolation as
// omission. Isolation is an optional enum, but models sometimes encode an
// unused optional mode as false instead of leaving the field out.
func normalizeAgentInputCompatibility(input map[string]any) map[string]any {
	isolation, exists := input["isolation"]
	if !exists {
		return input
	}
	omit := isolation == nil
	if flag, ok := isolation.(bool); ok && !flag {
		omit = true
	}
	if !omit {
		return input
	}
	normalized := make(map[string]any, len(input)-1)
	for key, value := range input {
		if key != "isolation" {
			normalized[key] = value
		}
	}
	return normalized
}

// TaskCreateInput is the typed input for TaskCreateTool
type TaskCreateInput struct {
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TaskUpdateInput is the typed input for TaskUpdateTool
type TaskUpdateInput struct {
	TaskID       string         `json:"taskId"`
	Subject      *string        `json:"subject,omitempty"`
	Description  *string        `json:"description,omitempty"`
	ActiveForm   *string        `json:"activeForm,omitempty"`
	Status       *string        `json:"status,omitempty"`
	AddBlocks    []string       `json:"addBlocks,omitempty"`
	AddBlockedBy []string       `json:"addBlockedBy,omitempty"`
	Owner        *string        `json:"owner,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// TodoWriteInput is the typed input for TodoWriteTool
type TodoWriteInput struct {
	Todos []TodoItem `json:"todos"`
}

// TodoItem represents a single item in a TodoWrite call
type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
}

// WebFetchInput is the typed input for WebFetchTool
type WebFetchInput struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

// WebSearchInput is the typed input for WebSearchTool
type WebSearchInput struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains"`
	BlockedDomains []string `json:"blocked_domains"`
}

// ConfigInput is the typed input for ConfigTool
type ConfigInput struct {
	Action  string `json:"action"`
	Key     string `json:"key"`
	Setting string `json:"setting"`
	Value   any    `json:"value"`
}

// CronCreateInput is the typed input for CronCreateTool
type CronCreateInput struct {
	Cron      string `json:"cron"`
	Prompt    string `json:"prompt"`
	Recurring *bool  `json:"recurring"`
	Durable   *bool  `json:"durable"`
}

// CronDeleteInput is the typed input for CronDeleteTool
type CronDeleteInput struct {
	ID string `json:"id"`
}

// CronListInput is the typed input for CronListTool.
type CronListInput struct{}

// AskUserQuestionInput is the typed input for AskUserQuestionTool
type AskUserQuestionInput struct {
	Questions []QuestionSpec `json:"questions"`
}

// QuestionSpec describes a single question with its options
type QuestionSpec struct {
	Question    string       `json:"question"`
	Header      string       `json:"header"`
	Options     []OptionSpec `json:"options"`
	MultiSelect bool         `json:"multiSelect"`
}

// OptionSpec describes a single selectable option
type OptionSpec struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Preview     string `json:"preview,omitempty"`
}

// EnterWorktreeInput is the typed input for EnterWorktreeTool
type EnterWorktreeInput struct {
	Name string `json:"name,omitempty"`
}

// ExitWorktreeInput is the typed input for ExitWorktreeTool
type ExitWorktreeInput struct {
	Action         string `json:"action"`
	DiscardChanges bool   `json:"discard_changes"`
}

// TaskGetInput is the typed input for TaskGetTool
type TaskGetInput struct {
	TaskID string `json:"taskId"`
}

// TaskStopInput is the typed input for TaskStopTool
type TaskStopInput struct {
	TaskID  string `json:"task_id,omitempty"`
	ShellID string `json:"shell_id,omitempty"`
}

// TaskOutputInput is the typed input for TaskOutputTool
type TaskOutputInput struct {
	TaskID  string  `json:"task_id"`
	Block   bool    `json:"block"`
	Timeout float64 `json:"timeout"`
}

// SendMessageInput is the typed input for SendMessageTool
type SendMessageInput struct {
	To      string `json:"to"`
	Summary string `json:"summary,omitempty"`
	Message any    `json:"message"`
}

// TeamCreateInput is the typed input for TeamCreateTool
type TeamCreateInput struct {
	TeamName    string `json:"team_name,omitempty"`
	Description string `json:"description,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
}

// TeamAgentSpec describes a single agent in a TeamCreate call
type TeamAgentSpec struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

// TeamDeleteInput is the typed input for TeamDeleteTool
type TeamDeleteInput struct {
}

// TeamDispatchInput is the typed input for TeamDispatchTool
type TeamDispatchInput struct {
	TeamID string         `json:"team_id"`
	Tasks  []TeamTaskSpec `json:"tasks"`
}

// TeamTaskSpec describes a single task to dispatch
type TeamTaskSpec struct {
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

// ReceiveMessagesInput is the typed input for ReceiveMessagesTool
type ReceiveMessagesInput struct {
	AgentID string `json:"agent_id"`
}

// LSPInput is the typed input for LSPTool
type LSPInput struct {
	Operation string `json:"operation"`
	FilePath  string `json:"filePath"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

// MCPToolInput is the typed input for MCPTool
type MCPToolInput struct {
	ServerName string         `json:"server_name"`
	ToolName   string         `json:"tool_name"`
	Arguments  map[string]any `json:"arguments"`
}

// ListMcpResourcesInput is the typed input for ListMcpResourcesTool
type ListMcpResourcesInput struct {
	Server string `json:"server"`
}

// ReadMcpResourceInput is the typed input for ReadMcpResourceTool
type ReadMcpResourceInput struct {
	Server string `json:"server"`
	URI    string `json:"uri"`
}

// SkillInput is the typed input for SkillTool
type SkillInput struct {
	Skill string `json:"skill"`
	Args  string `json:"args"`
}

// TeamDispatchTaskSpec describes a single task in a TeamDispatch call
type TeamDispatchTaskSpec struct {
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}
