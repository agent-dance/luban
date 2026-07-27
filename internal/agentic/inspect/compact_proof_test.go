package inspect

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestInspectCompactionProofIsContentFreeAndDeterministic(t *testing.T) {
	result := Result{
		Generation: "generation-proof",
		Requests: []RequestResult{
			{ID: "private-request", Path: "private/source.go", Files: []string{"private/source.go"},
				Matches: []Match{{Path: "private/source.go", Line: 7, Text: "private match text"}},
				Errors:  []RequestError{{Code: "zeta", Message: "private error detail"}, {Code: "alpha"}}, PartialReason: "budget"},
			{Errors: []RequestError{{Code: "alpha"}}, PartialReason: "source"},
		},
		Snippets:    []Snippet{{ID: "private-snippet", Path: "private/source.go", Content: "private source body"}},
		HasMoreView: true, SourceTruncated: true, OmittedRequestIDs: []string{"private-omitted"}, Cursor: "private-cursor",
		Stats: ResultStats{Requests: 2, Files: 3, Matches: 4, Snippets: 5, Items: 6},
	}
	proof := result.CompactionProof()
	if proof.Revision == nil || proof.Revision.Generation != "generation-proof" || proof.Inspect == nil {
		t.Fatalf("proof = %#v", proof)
	}
	if !reflect.DeepEqual(proof.Inspect.ErrorCodes, []string{"alpha", "zeta"}) ||
		!reflect.DeepEqual(proof.Inspect.PartialReasonCodes, []string{"budget", "source"}) || proof.Inspect.OmittedRequests != 1 {
		t.Fatalf("inspect proof = %#v", proof.Inspect)
	}
	encoded, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-request", "private/source.go", "private match text", "private error detail", "private source body", "private-cursor", "private-omitted"} {
		if !strings.Contains(string(encoded), secret) {
			continue
		}
		t.Fatalf("Inspect proof leaked request identity: %s", encoded)
	}
}
