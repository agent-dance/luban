package types

import (
	"encoding/json"
	"testing"
)

func TestPresentationMetadataSurvivesMessageJSONRoundTrip(t *testing.T) {
	want := Message{
		ID:     "message-1",
		Role:   RoleUser,
		IsMeta: true,
		Content: []ContentBlock{ToolResultBlock{
			Type:      ContentTypeToolResult,
			ToolUseID: "toolu-1",
			Content:   "partial evidence",
			IsError:   true,
			Outcome:   ToolOutcomePartial,
		}},
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Message
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.IsMeta {
		t.Fatal("Message.IsMeta was lost across JSON round trip")
	}
	if len(got.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(got.Content))
	}
	result, ok := got.Content[0].(ToolResultBlock)
	if !ok || result.Outcome != ToolOutcomePartial {
		t.Fatalf("tool outcome round trip = %+v, want partial", got.Content)
	}
}
