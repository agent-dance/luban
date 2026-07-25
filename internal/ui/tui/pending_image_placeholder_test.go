package tui

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestApplySessionSnapshotRejectsIncompletePendingImage(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	err := state.ApplySessionSnapshot(SessionSnapshot{
		Identity: SessionIdentity{SessionID: "session", Epoch: 1},
		DurableSessionView: DurableSessionView{
			Interaction:   SessionInteraction{InputDraft: "draft", InputCursor: 5, InputCursorSet: true},
			PendingImages: []ImageAttachment{{ID: 7, Base64: "image-data", MediaType: "image/png"}},
		},
	})
	if err == nil {
		t.Fatal("incomplete pending image snapshot was accepted")
	}
}
