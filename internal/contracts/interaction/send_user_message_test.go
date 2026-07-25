package interaction

import (
	"encoding/json"
	"testing"
)

func TestSendUserMessageOutputWireShape(t *testing.T) {
	encoded, err := json.Marshal(SendUserMessageOutput{
		Message: "ready",
		Attachments: []SendUserMessageAttachment{{
			Path: "/tmp/result.txt", Size: 12, IsImage: false,
		}},
		SentAt: "2026-07-25T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"message":"ready","attachments":[{"path":"/tmp/result.txt","size":12,"isImage":false}],"sentAt":"2026-07-25T00:00:00Z"}`
	if string(encoded) != want {
		t.Fatalf("wire shape = %s, want %s", encoded, want)
	}
}
