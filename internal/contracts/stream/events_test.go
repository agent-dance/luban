package stream

import (
	"encoding/json"
	"testing"
)

func TestEventMarshalJSONPreservesSurfaceNeutralPayload(t *testing.T) {
	event := Event{
		Type:      EventProgress,
		TurnCount: 3,
		Progress: &ProgressEvent{
			Stage:   "compact_start",
			Message: "compacting",
		},
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Type != event.Type || decoded.TurnCount != event.TurnCount || decoded.Progress == nil {
		t.Fatalf("decoded event = %#v", decoded)
	}
	if decoded.Progress.Stage != event.Progress.Stage || decoded.Progress.Message != event.Progress.Message {
		t.Fatalf("decoded progress = %#v", decoded.Progress)
	}
}

func TestSystemWarningMarshalJSONFailsClosed(t *testing.T) {
	if _, err := json.Marshal(Event{Type: EventSystemWarning}); err == nil {
		t.Fatal("system warning marshaled without an audience projection")
	}
}
