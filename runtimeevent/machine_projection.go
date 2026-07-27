package runtimeevent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

// MachineEventSchemaVersion versions the persisted/SDK machine-event
// envelope independently from the internal model and runtime-event schemas.
// Version 2 remains content-free and adds allowlisted physical-child execution
// evidence. Raw tool inputs and results are still represented only by
// content-addressed references and aggregate/protocol metrics.
const MachineEventSchemaVersion = "machine-event/v2"

// ContentReference is a non-resolving, content-addressed description of a
// private payload. The digest is useful for correlation and de-duplication,
// while resolution remains an explicitly authorized audit capability.
type ContentReference struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
	Bytes     int    `json:"bytes"`
	Scope     string `json:"scope"`
}

// TokenUsageMetrics contains only numeric accounting data. It deliberately
// excludes provider metadata and raw server-tool payloads.
type TokenUsageMetrics struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	WebSearchRequests        int `json:"web_search_requests,omitempty"`
	WebFetchRequests         int `json:"web_fetch_requests,omitempty"`
}

// ToolEventMetrics is the allowlisted numeric shape emitted for tool events.
// Counts describe omitted values without copying keys, paths, commands, file
// contents, environment variables, or arbitrary metadata into machine logs.
type ToolEventMetrics struct {
	InputBytes                int                       `json:"input_bytes,omitempty"`
	InputFieldCount           int                       `json:"input_field_count,omitempty"`
	ContentBytes              int                       `json:"content_bytes,omitempty"`
	ContentBlockCount         int                       `json:"content_block_count,omitempty"`
	DataPresent               bool                      `json:"data_present,omitempty"`
	MetadataCount             int                       `json:"metadata_count,omitempty"`
	NewMessageCount           int                       `json:"new_message_count,omitempty"`
	LogicalExecutionCommitted bool                      `json:"logical_execution_committed,omitempty"`
	PhysicalChildOperations   int                       `json:"physical_child_operations,omitempty"`
	PhysicalSteps             []PhysicalToolStepMetrics `json:"physical_steps,omitempty"`
	RevisionSealDisposition   string                    `json:"revision_seal_disposition,omitempty"`
	Usage                     *TokenUsageMetrics        `json:"usage,omitempty"`
}

// PhysicalToolStepMetrics is one process that actually crossed exec.Start.
// It deliberately carries no model-authored step ID, command, path, output,
// or environment data. Offsets are relative to the logical tool execution.
type PhysicalToolStepMetrics struct {
	OperationID     string `json:"operation_id"`
	Ordinal         int    `json:"ordinal"`
	StartedOffsetMS int64  `json:"started_offset_ms"`
	EndedOffsetMS   int64  `json:"ended_offset_ms"`
	DurationMS      int64  `json:"duration_ms"`
	Outcome         string `json:"outcome"`
	StdoutBytes     int64  `json:"stdout_bytes"`
	StderrBytes     int64  `json:"stderr_bytes"`
}

// ToolExecutionEvidence is the private, content-free handoff implemented by a
// compound tool result. Step ordinals are execution-plan positions, never
// model-authored IDs. Providers must omit steps that did not cross exec.Start.
type ToolExecutionEvidence struct {
	LogicalExecutionCommitted bool
	RevisionSealDisposition   string
	PhysicalSteps             []PhysicalToolStepEvidence
}

type PhysicalToolStepEvidence struct {
	Ordinal         int
	StartedOffsetMS int64
	EndedOffsetMS   int64
	DurationMS      int64
	Outcome         string
	StdoutBytes     int64
	StderrBytes     int64
}

// ToolExecutionEvidenceProvider is intentionally narrow: the machine-event
// boundary accepts only typed, content-free process facts from compound tools.
type ToolExecutionEvidenceProvider interface {
	ToolExecutionEvidence() ToolExecutionEvidence
}

// AttachToolExecutionEvidence adds validated compound-tool evidence to an
// existing machine metric. Uncommitted logical executions and invalid child
// records fail closed and never contribute to physical-operation counts.
func AttachToolExecutionEvidence(metrics *ToolEventMetrics, toolUseID string, data any) {
	if metrics == nil || toolUseID == "" || data == nil {
		return
	}
	provider, ok := data.(ToolExecutionEvidenceProvider)
	if !ok {
		return
	}
	evidence := provider.ToolExecutionEvidence()
	if !evidence.LogicalExecutionCommitted {
		return
	}
	metrics.LogicalExecutionCommitted = true
	if validRevisionSealDisposition(evidence.RevisionSealDisposition) {
		metrics.RevisionSealDisposition = evidence.RevisionSealDisposition
	}
	seenOrdinals := make(map[int]struct{}, len(evidence.PhysicalSteps))
	for _, step := range evidence.PhysicalSteps {
		if step.Ordinal < 0 || step.StartedOffsetMS < 0 || step.EndedOffsetMS < step.StartedOffsetMS ||
			step.DurationMS < 0 || step.StdoutBytes < 0 || step.StderrBytes < 0 || !validPhysicalToolOutcome(step.Outcome) {
			continue
		}
		if _, duplicate := seenOrdinals[step.Ordinal]; duplicate {
			continue
		}
		seenOrdinals[step.Ordinal] = struct{}{}
		metrics.PhysicalSteps = append(metrics.PhysicalSteps, PhysicalToolStepMetrics{
			OperationID: physicalToolOperationID(toolUseID, step.Ordinal), Ordinal: step.Ordinal,
			StartedOffsetMS: step.StartedOffsetMS, EndedOffsetMS: step.EndedOffsetMS,
			DurationMS: step.DurationMS, Outcome: step.Outcome,
			StdoutBytes: step.StdoutBytes, StderrBytes: step.StderrBytes,
		})
	}
	metrics.PhysicalChildOperations = len(metrics.PhysicalSteps)
}

func physicalToolOperationID(toolUseID string, ordinal int) string {
	digest := sha256.Sum256([]byte(toolUseID + "\x00physical-child\x00" + strconv.Itoa(ordinal)))
	return hex.EncodeToString(digest[:])
}

func validPhysicalToolOutcome(value string) bool {
	switch value {
	case "succeeded", "failed", "timed_out", "cancelled":
		return true
	default:
		return false
	}
}

func validRevisionSealDisposition(value string) bool {
	switch value {
	case "revision_bound", "committed_unverified", "revision_mismatch":
		return true
	default:
		return false
	}
}

// ToolResultPrivatePayload is accepted only as input to the hashing boundary.
// Its fields are explicitly non-serializable so the helper cannot become a
// second raw machine-event wire format by accident.
type ToolResultPrivatePayload struct {
	Content       string `json:"-"`
	ContentBlocks any    `json:"-"`
	Data          any    `json:"-"`
	Metadata      any    `json:"-"`
	NewMessages   any    `json:"-"`
}

// NewToolInputReference returns a stable reference to the complete JSON tool
// input. Tool inputs originate from provider JSON, so the fallback is needed
// only for an invalid in-process value and still remains content-free.
func NewToolInputReference(input map[string]any) ContentReference {
	return referenceForJSON("tool_input", input, nil)
}

// NewToolResultContentReference returns a stable reference to the complete
// private result envelope. If an invalid in-process value cannot be encoded,
// it fails closed to a reference over the model-visible text instead of ever
// copying the marshal error or offending value to the external event.
func NewToolResultContentReference(payload ToolResultPrivatePayload) ContentReference {
	wire := struct {
		Content       string `json:"content"`
		ContentBlocks any    `json:"content_blocks,omitempty"`
		Data          any    `json:"data,omitempty"`
		Metadata      any    `json:"metadata,omitempty"`
		NewMessages   any    `json:"new_messages,omitempty"`
	}{
		Content: payload.Content, ContentBlocks: payload.ContentBlocks,
		Data: payload.Data, Metadata: payload.Metadata, NewMessages: payload.NewMessages,
	}
	return referenceForJSON("tool_result_envelope", wire, []byte(payload.Content))
}

func referenceForJSON(scope string, value any, fallback []byte) ContentReference {
	payload, err := json.Marshal(value)
	if err != nil {
		payload = append([]byte(nil), fallback...)
		scope += "_text_fallback"
	}
	digest := sha256.Sum256(payload)
	return ContentReference{
		Algorithm: "sha256",
		Digest:    hex.EncodeToString(digest[:]),
		Bytes:     len(payload),
		Scope:     scope,
	}
}
