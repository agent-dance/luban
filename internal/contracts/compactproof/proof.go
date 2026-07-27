// Package compactproof defines the content-free, tool-owned facts that survive
// Agentic V2 microcompaction. It contains no rendering or execution policy.
package compactproof

const SchemaVersion = "agentic-v2-tool-proof/v1"

// Provider is implemented by typed tool result data that can expose a bounded
// proof without returning source content, commands, process output, or paths.
type Provider interface {
	CompactionProof() Proof
}

type Proof struct {
	Revision *RevisionProof `json:"revision,omitempty"`
	Inspect  *InspectProof  `json:"inspect,omitempty"`
	Run      *RunProof      `json:"run,omitempty"`
	Patch    *PatchProof    `json:"patch,omitempty"`
}

type RevisionProof struct {
	Status     string `json:"status,omitempty"`
	Generation string `json:"generation,omitempty"`
	Epoch      uint64 `json:"epoch,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

type InspectProof struct {
	Requests           int      `json:"requests"`
	Files              int      `json:"files"`
	Matches            int      `json:"matches"`
	Snippets           int      `json:"snippets"`
	Items              int      `json:"items"`
	HasMoreView        bool     `json:"has_more_view"`
	SourceTruncated    bool     `json:"source_truncated"`
	OmittedRequests    int      `json:"omitted_requests,omitempty"`
	ErrorCodes         []string `json:"error_codes,omitempty"`
	PartialReasonCodes []string `json:"partial_reason_codes,omitempty"`
}

type RunProof struct {
	LogicalExecutionCommitted bool           `json:"logical_execution_committed"`
	RevisionSealDisposition   string         `json:"revision_seal_disposition,omitempty"`
	TotalDurationMS           int64          `json:"total_duration_ms"`
	Steps                     []RunStepProof `json:"steps,omitempty"`
	VerificationStatus        string         `json:"verification_status,omitempty"`
	VerificationKind          string         `json:"verification_kind,omitempty"`
	VerificationConfigDigest  string         `json:"verification_config_digest,omitempty"`
	ScheduleStatus            string         `json:"schedule_status,omitempty"`
	ScheduleReason            string         `json:"schedule_reason,omitempty"`
}

type RunStepProof struct {
	Ordinal    int    `json:"ordinal"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Invoked    bool   `json:"invoked"`
	Truncated  bool   `json:"truncated,omitempty"`
}

type PatchProof struct {
	Status        string `json:"status"`
	CAS           string `json:"cas"`
	FailureReason string `json:"failure_reason,omitempty"`
	Files         int    `json:"files"`
	Hunks         int    `json:"hunks"`
	Additions     int    `json:"additions"`
	Deletions     int    `json:"deletions"`
}
