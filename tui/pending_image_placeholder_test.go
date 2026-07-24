package tui

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestApplySessionSnapshotMigratesPendingImageIntoComposer(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity: SessionIdentity{SessionID: "session", Epoch: 1},
		DurableSessionView: DurableSessionView{
			Interaction:   SessionInteraction{InputDraft: "draft", InputCursor: 5, InputCursorSet: true},
			PendingImages: []ImageAttachment{{ID: 7, Base64: "image-data", MediaType: "image/png"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	images := state.PendingImages.Get()
	if len(images) != 1 || images[0].Placeholder != "[Image #7]" {
		t.Fatalf("migrated images = %+v, want localized placeholder", images)
	}
	interaction := state.ActiveSessionInteraction()
	if interaction.InputDraft != "draft [Image #7] " || interaction.InputCursor != len([]rune(interaction.InputDraft)) {
		t.Fatalf("migrated interaction = %+v, want inline image placeholder", interaction)
	}
}
