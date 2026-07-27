package pierbackend

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestScrubClientExecutionProjectionRemovesEveryTraceOwnedField(t *testing.T) {
	duration, failed, outputBytes, traceBytes := int64(17), true, int64(23), int64(29)
	physical, critical, total, queue := 3, int64(11), int64(19), int64(2)
	round := harness.ProviderRoundEvidence{
		ToolCalls: []harness.ToolCallEvidence{{
			ID: "provider-owned-id", Name: "Run", InputBytes: 31,
			DurationMS: &duration, Error: &failed, OutputBytes: &outputBytes,
			AgentTraceOutputBytes: &traceBytes, TraceMatch: "id", TraceKind: "tool_result",
		}},
		PhysicalToolOperations: &physical,
		ToolCriticalPathMS:     &critical,
		ToolTotalLatencyMS:     &total,
		ToolQueueMS:            &queue,
	}

	got := scrubClientExecutionProjection(round)
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].ID != "provider-owned-id" || got.ToolCalls[0].Name != "Run" || got.ToolCalls[0].InputBytes != 31 {
		t.Fatalf("provider-owned tool projection changed: %#v", got.ToolCalls)
	}
	call := got.ToolCalls[0]
	if call.DurationMS != nil || call.Error != nil || call.OutputBytes != nil || call.AgentTraceOutputBytes != nil || call.TraceMatch != "" || call.TraceKind != "" {
		t.Fatalf("client execution fields survived projection scrub: %#v", call)
	}
	if got.PhysicalToolOperations != nil || got.ToolCriticalPathMS != nil || got.ToolTotalLatencyMS != nil || got.ToolQueueMS != nil {
		t.Fatalf("client round metrics survived projection scrub: %#v", got)
	}
	if round.ToolCalls[0].OutputBytes == nil || *round.ToolCalls[0].OutputBytes != outputBytes {
		t.Fatal("projection scrub mutated its caller")
	}
}

func TestValidateArchivedProviderProjectionRequiresFinalCodexBinding(t *testing.T) {
	raw := []byte("{}\n")
	digest := sha256.Sum256(raw)
	err := ValidateArchivedProviderProjection(
		raw,
		hex.EncodeToString(digest[:]),
		nil,
		harness.AgentSpec{ID: "codex"},
		strings.Repeat("1", 64),
		"",
		harness.ProviderEndpointSpec{},
	)
	if err == nil || !strings.Contains(err.Error(), "final service-tier canonicalization binding") {
		t.Fatalf("missing final Codex binding error = %v", err)
	}
}
