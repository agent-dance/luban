package session

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestSessionActivityProvisionalMetadataRoundTripsWithVersion(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := store.Save("activity-provisional", []types.Message{types.UserMessage("start")}); err != nil {
		t.Fatal(err)
	}
	want := SessionActivityMeta{
		Version: SessionActivityMetaVersionProvisional, ID: "tool:read-1", Kind: "tool", Name: "Read",
		State: "failed", Lifecycle: "failed", Outcome: "failed", Provisional: true,
		ProgressMessage: "generic runtime error", JumpTarget: "runtime-error:event-1", LastSequence: 3,
	}
	if err := store.SaveMeta("activity-provisional", SessionMeta{Activities: []SessionActivityMeta{want}}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.GetMeta("activity-provisional")
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Activities) != 1 || !reflect.DeepEqual(meta.Activities[0], want) {
		t.Fatalf("provisional activity metadata = %+v, want %+v", meta.Activities, want)
	}
	encoded, err := json.Marshal(meta.Activities[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["version"] != float64(SessionActivityMetaVersionProvisional) || fields["provisional"] != true {
		t.Fatalf("typed provisional fields not persisted: %s", encoded)
	}
}
