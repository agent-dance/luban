package compact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/types"
)

const maxCompactionErrorSummaryBytes = 256

var agenticV2ProofTools = map[string]struct{}{
	"Inspect":    {},
	"Run":        {},
	"ApplyPatch": {},
}

type agenticV2CompactionEnvelope struct {
	Schema        string                     `json:"schema"`
	Tool          string                     `json:"tool"`
	Outcome       types.ToolOutcome          `json:"outcome"`
	IsError       bool                       `json:"is_error"`
	ContentSHA256 string                     `json:"content_sha256"`
	Error         *compactionErrorProof      `json:"error,omitempty"`
	Permission    *compactionPermissionProof `json:"permission,omitempty"`
	Proof         *compactproof.Proof        `json:"proof,omitempty"`
}

type compactionErrorProof struct {
	Code             string `json:"code"`
	Retryable        bool   `json:"retryable,omitempty"`
	Summary          string `json:"summary,omitempty"`
	SummaryTruncated bool   `json:"summary_truncated,omitempty"`
}

type compactionPermissionProof struct {
	Decision      string           `json:"decision"`
	Authoritative bool             `json:"authoritative"`
	PolicyCode    string           `json:"policy_code,omitempty"`
	RuleSource    string           `json:"rule_source,omitempty"`
	Risk          types.PolicyRisk `json:"risk,omitempty"`
}

type cachedAgenticV2ProofLedgerEnvelope struct {
	Schema  string                      `json:"schema"`
	Entries []cachedAgenticV2ProofEntry `json:"entries"`
}

type cachedAgenticV2ProofEntry struct {
	ToolUseID string          `json:"tool_use_id"`
	Proof     json.RawMessage `json:"proof"`
}

func isAgenticV2ProofTool(name string) bool {
	_, ok := agenticV2ProofTools[name]
	return ok
}

func agenticV2ProofContent(toolName string, result types.ToolResultBlock) (string, bool) {
	if !isAgenticV2ProofTool(toolName) {
		return "", false
	}
	original := result.TextContent()
	digest := sha256.Sum256([]byte(original))
	envelope := agenticV2CompactionEnvelope{
		Schema: compactproof.SchemaVersion, Tool: toolName,
		Outcome: result.Outcome, IsError: result.IsError,
		ContentSHA256: hex.EncodeToString(digest[:]),
	}

	if provider, ok := result.Data.(compactproof.Provider); ok {
		proof := provider.CompactionProof()
		envelope.Proof = &proof
	}
	mergeRunMetadata(&envelope, result.Metadata)
	errorCode, retryable := compactErrorCode(result)
	if result.IsError || result.Outcome == types.ToolOutcomeFailed || result.Outcome == types.ToolOutcomeDenied ||
		result.Outcome == types.ToolOutcomeCancelled || result.Outcome == types.ToolOutcomeTimedOut {
		if errorCode == "" {
			errorCode = fallbackCompactionErrorCode(result.Outcome)
		}
		envelope.Error = &compactionErrorProof{Code: errorCode, Retryable: retryable}
		if toolName != "Run" || envelope.Proof == nil || envelope.Proof.Run == nil || len(envelope.Proof.Run.Steps) == 0 {
			envelope.Error.Summary, envelope.Error.SummaryTruncated = boundedCompactionSummary(original)
		}
	}
	mergePermissionProof(&envelope, result, errorCode)
	mergePatchDisposition(&envelope, result, errorCode)

	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func compactErrorCode(result types.ToolResultBlock) (string, bool) {
	switch data := result.Data.(type) {
	case types.ToolErrorData:
		return data.Code, data.Retryable
	case *types.ToolErrorData:
		if data != nil {
			return data.Code, data.Retryable
		}
	case types.PolicyDecision:
		return data.Code, false
	case *types.PolicyDecision:
		if data != nil {
			return data.Code, false
		}
	}
	if result.Metadata != nil {
		if reason := result.Metadata["schedule.reason"]; reason != "" {
			return reason, false
		}
		if status := result.Metadata["verification.status"]; status == "revision_mismatch" || status == "patch_commit_required" {
			return status, false
		}
	}
	return "", false
}

func fallbackCompactionErrorCode(outcome types.ToolOutcome) string {
	switch outcome {
	case types.ToolOutcomeDenied:
		return "permission_denied"
	case types.ToolOutcomeCancelled:
		return "cancelled"
	case types.ToolOutcomeTimedOut:
		return "timed_out"
	case types.ToolOutcomePartial:
		return "partial"
	default:
		return "tool_result_failed"
	}
}

func mergePermissionProof(envelope *agenticV2CompactionEnvelope, result types.ToolResultBlock, errorCode string) {
	if envelope == nil {
		return
	}
	permission := compactionPermissionProof{}
	switch data := result.Data.(type) {
	case types.PolicyDecision:
		permission = permissionFromPolicyDecision(data)
	case *types.PolicyDecision:
		if data != nil {
			permission = permissionFromPolicyDecision(*data)
		}
	}
	permissionFailure := result.Outcome == types.ToolOutcomeDenied || strings.Contains(strings.ToLower(errorCode), "permission")
	if permissionFailure {
		permission.Decision = "denied"
		permission.Authoritative = true
	}
	if permission.Decision != "" {
		envelope.Permission = &permission
	}
}

func permissionFromPolicyDecision(decision types.PolicyDecision) compactionPermissionProof {
	value := compactionPermissionProof{
		Decision: string(decision.Disposition), Authoritative: true,
		PolicyCode: decision.Code, RuleSource: decision.RuleSource, Risk: decision.Risk,
	}
	if decision.Disposition == types.PolicyBlock {
		value.Decision = "denied"
	}
	return value
}

func mergePatchDisposition(envelope *agenticV2CompactionEnvelope, result types.ToolResultBlock, errorCode string) {
	if envelope == nil || envelope.Tool != "ApplyPatch" {
		return
	}
	if envelope.Proof == nil {
		envelope.Proof = &compactproof.Proof{}
	}
	if envelope.Proof.Patch == nil {
		envelope.Proof.Patch = &compactproof.PatchProof{Status: string(result.Outcome)}
	}
	patch := envelope.Proof.Patch
	if patch.Status == "" {
		patch.Status = string(result.Outcome)
	}
	if patch.FailureReason == "" && result.Metadata != nil {
		patch.FailureReason = result.Metadata["apply_patch.failure_reason"]
	}
	if patch.FailureReason == "" && errorCode != "" {
		patch.FailureReason = errorCode
	}
	if patch.CAS == "" {
		lowerCode := strings.ToLower(errorCode)
		switch {
		case result.Outcome == types.ToolOutcomeSucceeded:
			patch.CAS = "committed"
		case result.Outcome == types.ToolOutcomePartial && strings.Contains(lowerCode, "commit"):
			patch.CAS = "committed_revision_unsealed"
		case strings.Contains(lowerCode, "conflict") || strings.Contains(lowerCode, "cas"):
			patch.CAS = "rejected"
		case result.Outcome == types.ToolOutcomeDenied || strings.Contains(lowerCode, "permission"):
			patch.CAS = "not_authorized"
		case strings.Contains(lowerCode, "commit"):
			// A failed commit can have either rolled back completely or left a
			// partial mutation after rollback failure. The compact proof must
			// not strengthen that uncertainty into "not committed".
			patch.CAS = "commit_state_unknown"
		default:
			patch.CAS = "not_committed"
		}
	}
	if envelope.Proof.Revision == nil {
		status := "not_issued"
		if patch.CAS == "committed_revision_unsealed" {
			status = "receipt_failed"
		}
		envelope.Proof.Revision = &compactproof.RevisionProof{Status: status}
	}
}

func mergeRunMetadata(envelope *agenticV2CompactionEnvelope, metadata map[string]string) {
	if envelope == nil || envelope.Tool != "Run" || len(metadata) == 0 {
		return
	}
	if envelope.Proof == nil {
		envelope.Proof = &compactproof.Proof{}
	}
	if envelope.Proof.Run == nil {
		envelope.Proof.Run = &compactproof.RunProof{}
	}
	run := envelope.Proof.Run
	run.VerificationStatus = metadata["verification.status"]
	run.VerificationKind = metadata["verification.kind"]
	run.VerificationConfigDigest = metadata["verification.config_digest"]
	run.ScheduleStatus = metadata["schedule.status"]
	run.ScheduleReason = metadata["schedule.reason"]
	if envelope.Proof.Revision == nil {
		epoch, _ := strconv.ParseUint(metadata["verification.revision_epoch"], 10, 64)
		digest := metadata["verification.revision_digest"]
		if epoch != 0 || digest != "" || run.VerificationStatus != "" {
			envelope.Proof.Revision = &compactproof.RevisionProof{
				Status: run.VerificationStatus, Epoch: epoch, Digest: digest,
			}
		}
	}
}

func boundedCompactionSummary(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) <= maxCompactionErrorSummaryBytes {
		return value, false
	}
	value = value[:maxCompactionErrorSummaryBytes]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value), true
}

func cachedAgenticV2ProofLedger(messages []types.Message, deleted []string) (string, int, int) {
	if len(deleted) == 0 {
		return "", 0, 0
	}
	toolNames := toolNamesByUseID(messages)
	results := toolResultsByUseID(messages)
	entries := make([]cachedAgenticV2ProofEntry, 0, len(deleted))
	reclaimedBytes := 0
	for _, id := range deleted {
		result, ok := results[id]
		if !ok {
			continue
		}
		reclaimedBytes += len(result.TextContent())
		name := toolNames[id]
		if !isAgenticV2ProofTool(name) {
			continue
		}
		proof, proofOK := agenticV2ProofContent(name, result)
		if !proofOK || len(proof) >= len(result.TextContent()) {
			continue
		}
		entries = append(entries, cachedAgenticV2ProofEntry{ToolUseID: id, Proof: json.RawMessage(proof)})
	}
	if len(entries) == 0 {
		return "", reclaimedBytes, 0
	}
	// Deleted order is already stable, but sorting by tool-use ID makes the
	// ledger canonical even if a future provider returns cache edits reordered.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ToolUseID < entries[j].ToolUseID })
	encoded, err := json.Marshal(cachedAgenticV2ProofLedgerEnvelope{
		Schema: "agentic-v2-cache-proof-ledger/v1", Entries: entries,
	})
	if err != nil {
		return "", reclaimedBytes, 0
	}
	return string(encoded), reclaimedBytes, len(encoded)
}

func toolNamesByUseID(messages []types.Message) map[string]string {
	names := make(map[string]string)
	for _, message := range messages {
		if effectiveCompactionRole(message) != types.RoleAssistant {
			continue
		}
		for _, block := range message.Content {
			if use, ok := block.(types.ToolUseBlock); ok && use.ID != "" {
				names[use.ID] = use.Name
			}
		}
	}
	return names
}

func toolResultsByUseID(messages []types.Message) map[string]types.ToolResultBlock {
	results := make(map[string]types.ToolResultBlock)
	for _, message := range messages {
		if effectiveCompactionRole(message) != types.RoleUser {
			continue
		}
		for _, block := range message.Content {
			if result, ok := block.(types.ToolResultBlock); ok && result.ToolUseID != "" {
				results[result.ToolUseID] = result
			}
		}
	}
	return results
}
