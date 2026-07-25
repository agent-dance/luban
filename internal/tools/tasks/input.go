package tasktools

// Field names are protocol identifiers from the Task tool JSON contracts.
const (
	taskIDField     = "taskId"
	activeFormField = "activeForm"
)

type taskCreateInput struct {
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type taskUpdateInput struct {
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

type taskGetInput struct {
	TaskID string `json:"taskId"`
}

type taskStopInput struct {
	TaskID string `json:"task_id"`
}

type taskOutputInput struct {
	TaskID  string  `json:"task_id"`
	Block   bool    `json:"block"`
	Timeout float64 `json:"timeout"`
}
