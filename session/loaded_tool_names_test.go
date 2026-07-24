package session

import (
	"reflect"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestLoadedToolNamesPersistAsStableSessionLedger(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "loaded-tools-session"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta(sessionID, SessionMeta{
		LoadedToolNames: []string{"TaskUpdate", " TaskCreate ", "TaskUpdate", ""},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta(sessionID, SessionMeta{Title: "preserve loaded tools"}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"TaskCreate", "TaskUpdate"}; !reflect.DeepEqual(meta.LoadedToolNames, want) {
		t.Fatalf("loaded tool names = %v, want %v", meta.LoadedToolNames, want)
	}
}
