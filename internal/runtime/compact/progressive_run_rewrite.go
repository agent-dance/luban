package compact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/types"
)

const (
	progressiveRunRewriteSchema = "progressive-run-rewrite/v1"
	progressiveRunExcerptBytes  = 768
)

type progressiveRunRewriteEnvelope struct {
	Schema        string          `json:"schema"`
	OriginalBytes int             `json:"original_bytes"`
	OriginalSHA   string          `json:"original_sha256"`
	Head          string          `json:"head,omitempty"`
	Tail          string          `json:"tail,omitempty"`
	Truncated     bool            `json:"truncated"`
	Proof         json.RawMessage `json:"proof"`
}

// progressiveRunRewriteContent is intentionally limited to successful,
// logically committed executions. Failures, cancellations, timeouts,
// revision mismatches, and partial schedules remain lossless diagnostics.
func progressiveRunRewriteContent(result types.ToolResultBlock, original string) (string, bool) {
	if result.IsError || result.Outcome != types.ToolOutcomeSucceeded || strings.TrimSpace(original) == "" {
		return "", false
	}
	provider, ok := result.Data.(compactproof.Provider)
	if !ok {
		return "", false
	}
	proof := provider.CompactionProof()
	if proof.Run == nil || !safeProgressiveRunProof(*proof.Run, result.Metadata) {
		return "", false
	}
	proofContent, valid := agenticV2ProofContent("Run", result)
	if !valid || !json.Valid([]byte(proofContent)) {
		return "", false
	}
	digest := sha256.Sum256([]byte(original))
	head, tail, truncated := progressiveRunExcerpt(original)
	envelope := progressiveRunRewriteEnvelope{
		Schema: progressiveRunRewriteSchema, OriginalBytes: len(original), OriginalSHA: hex.EncodeToString(digest[:]),
		Head: head, Tail: tail, Truncated: truncated, Proof: json.RawMessage(proofContent),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) >= len(original) {
		return "", false
	}
	return string(encoded), true
}

func safeProgressiveRunProof(proof compactproof.RunProof, metadata map[string]string) bool {
	if !proof.LogicalExecutionCommitted || len(proof.Steps) == 0 {
		return false
	}
	for _, step := range proof.Steps {
		if !step.Invoked || step.ExitCode != 0 || step.Status != "succeeded" {
			return false
		}
	}
	for _, value := range []string{
		proof.ScheduleReason, metadata["schedule.reason"], metadata["error.code"],
	} {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	switch proof.VerificationStatus {
	case "", "passed", "succeeded", "not_requested":
		return true
	default:
		return false
	}
}

func progressiveRunExcerpt(original string) (head, tail string, truncated bool) {
	if len(original) <= progressiveRunExcerptBytes*2 {
		return original, "", false
	}
	head = validUTF8Prefix(original, progressiveRunExcerptBytes)
	tail = validUTF8Suffix(original, progressiveRunExcerptBytes)
	return head, tail, true
}

func validUTF8Prefix(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}

func validUTF8Suffix(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[len(value)-limit:]
	for !utf8.ValidString(value) && len(value) > 0 {
		_, size := utf8.DecodeRuneInString(value)
		value = value[size:]
	}
	return value
}
