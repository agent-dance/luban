package report

import (
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestSelectionTaskCountKeepsPilotSubsetSeparateFromUniverse(t *testing.T) {
	tests := []struct {
		selection harness.SelectionSpec
		want      int
	}{
		{selection: harness.SelectionSpec{Mode: "full", ExpectedTaskCount: 113}, want: 113},
		{selection: harness.SelectionSpec{Mode: "tasks", TaskIDs: []string{"a", "b", "c", "d", "e"}, ExpectedTaskCount: 113}, want: 5},
		{selection: harness.SelectionSpec{Mode: "sample", SampleCount: 5, ExpectedTaskCount: 113}, want: 5},
	}
	for _, test := range tests {
		if got := selectionTaskCount(test.selection); got != test.want {
			t.Fatalf("selectionTaskCount(%+v) = %d, want %d", test.selection, got, test.want)
		}
	}
}
