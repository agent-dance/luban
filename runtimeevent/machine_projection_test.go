package runtimeevent

import (
	"encoding/json"
	"strings"
	"testing"
)

type machineExecutionEvidenceFixture struct {
	evidence ToolExecutionEvidence
}

func (fixture machineExecutionEvidenceFixture) ToolExecutionEvidence() ToolExecutionEvidence {
	return fixture.evidence
}

func TestMachineContentReferencesNeverSerializePrivatePayload(t *testing.T) {
	const secret = "token=sk-machine-projection-secret"
	private := ToolResultPrivatePayload{
		Content: secret,
		ContentBlocks: []any{map[string]any{
			"text":   secret,
			"nested": []any{map[string]any{"authorization": secret}},
		}},
		Data: map[string]any{
			"originalFile": secret,
			"nested":       map[string]any{"environment": []any{secret}},
		},
		Metadata:    map[string]string{"secret-shaped-key": secret},
		NewMessages: []any{map[string]any{"content": secret}},
	}

	first := NewToolResultContentReference(private)
	second := NewToolResultContentReference(private)
	if first != second || first.Algorithm != "sha256" || len(first.Digest) != 64 || first.Bytes == 0 || first.Scope != "tool_result_envelope" {
		t.Fatalf("content reference = %#v, second = %#v", first, second)
	}

	wire, err := json.Marshal(struct {
		SchemaVersion string                   `json:"schema_version"`
		ContentRef    ContentReference         `json:"content_ref"`
		Metrics       ToolEventMetrics         `json:"metrics"`
		Private       ToolResultPrivatePayload `json:"private"`
	}{
		SchemaVersion: MachineEventSchemaVersion,
		ContentRef:    first,
		Metrics: ToolEventMetrics{
			ContentBytes: len(secret), ContentBlockCount: 1, DataPresent: true,
			MetadataCount: 1, NewMessageCount: 1,
		},
		Private: private,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{secret, "sk-machine-projection-secret", "originalFile", "authorization", "environment", "secret-shaped-key"} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("machine projection leaked %q: %s", forbidden, wire)
		}
	}
}

func TestToolInputReferenceIsStableAndContentFree(t *testing.T) {
	const secret = "Bearer sk-machine-input-secret"
	input := map[string]any{
		"command": "env API_TOKEN=" + secret,
		"nested":  map[string]any{"secrets": []any{secret}},
	}
	first := NewToolInputReference(input)
	second := NewToolInputReference(input)
	if first != second || first.Scope != "tool_input" || first.Bytes == 0 {
		t.Fatalf("input references = %#v / %#v", first, second)
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), secret) || strings.Contains(string(wire), "sk-machine-input-secret") {
		t.Fatalf("input reference leaked private input: %s", wire)
	}
}

func TestAttachToolExecutionEvidenceCountsOnlyCommittedValidPhysicalChildren(t *testing.T) {
	evidence := machineExecutionEvidenceFixture{evidence: ToolExecutionEvidence{
		LogicalExecutionCommitted: true,
		RevisionSealDisposition:   "revision_bound",
		PhysicalSteps: []PhysicalToolStepEvidence{
			{Ordinal: 0, StartedOffsetMS: 2, EndedOffsetMS: 7, DurationMS: 5, Outcome: "succeeded", StdoutBytes: 3},
			{Ordinal: 1, StartedOffsetMS: 8, EndedOffsetMS: 7, DurationMS: 0, Outcome: "failed"},
			{Ordinal: 0, StartedOffsetMS: 9, EndedOffsetMS: 10, DurationMS: 1, Outcome: "failed"},
			{Ordinal: 3, StartedOffsetMS: 11, EndedOffsetMS: 13, DurationMS: 2, Outcome: "cancelled", StderrBytes: 4},
		},
	}}
	first := ToolEventMetrics{}
	AttachToolExecutionEvidence(&first, "toolu-private-parent", evidence)
	second := ToolEventMetrics{}
	AttachToolExecutionEvidence(&second, "toolu-private-parent", evidence)
	if !first.LogicalExecutionCommitted || first.PhysicalChildOperations != 2 || len(first.PhysicalSteps) != 2 ||
		first.RevisionSealDisposition != "revision_bound" || first.PhysicalSteps[0].OperationID != second.PhysicalSteps[0].OperationID ||
		len(first.PhysicalSteps[0].OperationID) != 64 {
		t.Fatalf("physical execution projection = %#v / %#v", first, second)
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "toolu-private-parent") {
		t.Fatalf("physical operation ID exposed its parent ID: %s", wire)
	}

	uncommitted := ToolEventMetrics{}
	AttachToolExecutionEvidence(&uncommitted, "toolu-aborted", machineExecutionEvidenceFixture{evidence: ToolExecutionEvidence{
		PhysicalSteps: []PhysicalToolStepEvidence{{Ordinal: 0, Outcome: "succeeded"}},
	}})
	if uncommitted.LogicalExecutionCommitted || uncommitted.PhysicalChildOperations != 0 || len(uncommitted.PhysicalSteps) != 0 {
		t.Fatalf("uncommitted execution contributed metrics: %#v", uncommitted)
	}
}
