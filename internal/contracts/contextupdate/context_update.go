// Package contextupdate defines the model-to-runtime proposal protocol for
// progressive context handling. Decisions are untrusted until the runtime
// resolves the referenced tool result and validates its configured policy.
package contextupdate

const SchemaVersion = "context-update/v3"

type Action string

const (
	ActionKeep    Action = "KEEP"
	ActionRewrite Action = "REWRITE"
	ActionIndex   Action = "INDEX"
	ActionDrop    Action = "DROP"
)

type Decision struct {
	Schema      string  `json:"schema"`
	TargetIndex int     `json:"target_index"`
	TargetTool  string  `json:"target_tool"`
	Action      Action  `json:"action"`
	ReasonCode  string  `json:"reason_code"`
	Confidence  float64 `json:"confidence"`
}

type Provider interface {
	ContextUpdateDecision() Decision
}
