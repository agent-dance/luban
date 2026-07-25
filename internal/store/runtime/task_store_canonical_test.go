package runtime

import (
	"testing"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

func TestDecodeRuntimeTaskRecordDoesNotBackfillHistoricalRunFields(t *testing.T) {
	record, err := decodeRuntimeTaskRecord([]byte(`{
		"id":"agent-1",
		"type":"local_agent",
		"status":"success",
		"runs":[{"run_id":"run-3","attempt":3,"status":"canceled"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if record.Attempt != 0 || record.CurrentRunID != "" || record.Outcome != "" {
		t.Fatalf("historical top-level fields were backfilled: %+v", record)
	}
	if len(record.Runs) != 1 || record.Runs[0].Outcome != "" {
		t.Fatalf("historical run outcome was backfilled: %+v", record.Runs)
	}
}

func TestDecodeRuntimeTaskRecordPreservesCanonicalRunFields(t *testing.T) {
	record, err := decodeRuntimeTaskRecord([]byte(`{
		"id":"agent-1",
		"type":"local_agent",
		"status":"completed",
		"current_run_id":"run-3",
		"attempt":3,
		"outcome":"succeeded",
		"runs":[{"run_id":"run-3","attempt":3,"status":"completed","outcome":"succeeded"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if record.Attempt != 3 || record.CurrentRunID != "run-3" || record.Outcome != agentcontract.RunOutcomeSucceeded {
		t.Fatalf("canonical top-level fields changed: %+v", record)
	}
	if len(record.Runs) != 1 || record.Runs[0].Outcome != agentcontract.RunOutcomeSucceeded {
		t.Fatalf("canonical run outcome changed: %+v", record.Runs)
	}
}
