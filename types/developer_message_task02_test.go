package types_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func TestDeveloperMessageConstructor(t *testing.T) {
	metadata := types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 7,
	}
	message := types.DeveloperMessage("available skills", metadata)

	if message.Role != types.RoleDeveloper {
		t.Fatalf("role = %q, want %q", message.Role, types.RoleDeveloper)
	}
	if !message.IsMeta {
		t.Fatal("developer catalog message must be marked as internal metadata")
	}
	if got := message.GetText(); got != "available skills" {
		t.Fatalf("text = %q, want %q", got, "available skills")
	}
	if message.DeveloperMetadata == nil || *message.DeveloperMetadata != metadata {
		t.Fatalf("developer metadata = %#v, want %#v", message.DeveloperMetadata, metadata)
	}
}

func TestDeveloperMessageJSONRoundTrip(t *testing.T) {
	want := types.DeveloperMessage("revoke skill-1", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogDelta,
		Revision: 12,
	})

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got types.Message
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestDeveloperMessageSessionPersistenceRoundTrip(t *testing.T) {
	store := session.NewFileStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	want := types.DeveloperMessage("current catalog", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 41,
	})

	if err := store.Save("developer-message", []types.Message{want}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load("developer-message")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 1 || !reflect.DeepEqual(loaded[0], want) {
		t.Fatalf("loaded messages = %#v, want %#v", loaded, []types.Message{want})
	}
}

func TestAssistantMessageJSONDoesNotRequireDeveloperMetadata(t *testing.T) {
	encoded := []byte(`{"role":"assistant","content":[{"type":"text","text":"assistant"}]}`)

	var message types.Message
	if err := json.Unmarshal(encoded, &message); err != nil {
		t.Fatalf("Unmarshal assistant message: %v", err)
	}
	if message.Role != types.RoleAssistant || message.GetText() != "assistant" {
		t.Fatalf("assistant message = %#v", message)
	}
	if message.DeveloperMetadata != nil {
		t.Fatalf("assistant developer metadata = %#v, want nil", message.DeveloperMetadata)
	}
}

func TestOrdinaryMessageJSONOmitsOptionalMetadata(t *testing.T) {
	tests := []struct {
		name    string
		message types.Message
		want    string
	}{
		{
			name:    "user",
			message: types.UserMessage("hello"),
			want:    `{"role":"user","content":[{"type":"text","text":"hello"}]}`,
		},
		{
			name:    "assistant",
			message: types.AssistantMessage("world"),
			want:    `{"role":"assistant","content":[{"type":"text","text":"world"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.message)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if got := string(encoded); got != test.want {
				t.Fatalf("encoding = %s, want %s", got, test.want)
			}
		})
	}
}
