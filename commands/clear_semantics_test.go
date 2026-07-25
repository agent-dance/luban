package commands_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/types"
)

// Target API assumptions locked by this file:
//   - Context.ClearView clears only the presentation projection.
//   - Context.ClearConversation performs the staged engine/session/TUI switch.
//   - /clear view delegates only to ClearView.
//   - /clear and /clear conversation delegate only to ClearConversation and do
//     not fall back to mutating QueryLoop messages in place.

func TestClearViewDoesNotChangeModelContext(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Find("clear")
	if cmd == nil {
		t.Fatal("/clear must be registered")
	}

	modelMessages := []types.Message{types.UserMessage("keep in model context"), types.AssistantMessage("keep response")}
	ql := &stubQL{messages: append([]types.Message(nil), modelMessages...)}
	viewClears := 0
	conversationClears := 0
	ctx := &commands.Context{
		QueryLoop: ql,
		SessionID: "session-a",
		OnEvent:   func(string) {},
		ClearView: func() error {
			viewClears++
			return nil
		},
		ClearConversation: func() error {
			conversationClears++
			return nil
		},
	}

	if err := cmd.Execute(ctx, "view"); err != nil {
		t.Fatalf("/clear view: %v", err)
	}
	if viewClears != 1 || conversationClears != 0 {
		t.Fatalf("clear callbacks = (view=%d conversation=%d), want (1, 0)", viewClears, conversationClears)
	}
	if !messagesEqual(ql.Messages(), modelMessages) {
		t.Fatalf("/clear view changed model context: got %#v want %#v", ql.Messages(), modelMessages)
	}
}

func TestClearConversationCreatesNewSessionAndPreservesOldAudit(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Find("clear")
	if cmd == nil {
		t.Fatal("/clear must be registered")
	}

	oldID := "session-old"
	newID := "session-new"
	oldMessages := []types.Message{types.UserMessage("audited request"), types.AssistantMessage("audited response")}
	store := &clearAuditStore{messages: map[string][]types.Message{oldID: append([]types.Message(nil), oldMessages...)}}
	ql := &stubQL{messages: append([]types.Message(nil), oldMessages...)}
	activeSession := oldID
	conversationClears := 0
	ctx := &commands.Context{
		QueryLoop:    ql,
		SessionID:    oldID,
		SessionStore: store,
		OnEvent:      func(string) {},
		ClearConversation: func() error {
			conversationClears++
			// The application callback owns the atomic switch. The command must not
			// erase or overwrite the old session in order to implement clear.
			activeSession = newID
			ql.SetMessagesPreservingToolUseLedger(nil)
			return nil
		},
	}

	if err := cmd.Execute(ctx, "conversation"); err != nil {
		t.Fatalf("/clear conversation: %v", err)
	}
	if conversationClears != 1 {
		t.Fatalf("ClearConversation called %d times, want 1", conversationClears)
	}
	if activeSession == oldID || activeSession == "" {
		t.Fatalf("clear conversation kept old/empty active session ID %q", activeSession)
	}
	if got := ql.Messages(); len(got) != 0 {
		t.Fatalf("new model context contains %d messages, want empty", len(got))
	}
	loaded, err := store.Load(oldID)
	if err != nil {
		t.Fatalf("load preserved old audit: %v", err)
	}
	if !messagesEqual(loaded, oldMessages) {
		t.Fatalf("old session audit changed: got %#v want %#v", loaded, oldMessages)
	}
}

func TestClearDefaultsToConversationCallback(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Find("clear")
	if cmd == nil {
		t.Fatal("/clear must be registered")
	}

	conversationClears := 0
	ctx := &commands.Context{
		QueryLoop: &stubQL{},
		SessionID: "session-old",
		OnEvent:   func(string) {},
		ClearConversation: func() error {
			conversationClears++
			return nil
		},
	}
	if err := cmd.Execute(ctx, ""); err != nil {
		t.Fatalf("/clear: %v", err)
	}
	if conversationClears != 1 {
		t.Fatalf("default /clear called ClearConversation %d times, want 1", conversationClears)
	}
}

func TestClearConversationRequiresAtomicCallback(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Find("clear")
	if cmd == nil {
		t.Fatal("/clear must be registered")
	}

	modelMessages := []types.Message{types.UserMessage("must survive failed clear")}
	ql := &stubQL{messages: append([]types.Message(nil), modelMessages...)}
	ctx := &commands.Context{QueryLoop: ql, SessionID: "session-old", OnEvent: func(string) {}}
	if err := cmd.Execute(ctx, ""); err == nil {
		t.Fatal("/clear without ClearConversation callback succeeded with non-atomic in-place semantics")
	}
	if !messagesEqual(ql.Messages(), modelMessages) {
		t.Fatalf("failed clear changed model context: got %#v want %#v", ql.Messages(), modelMessages)
	}
}

func TestClearCallbackFailureLeavesConversationUntouched(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Find("clear")
	if cmd == nil {
		t.Fatal("/clear must be registered")
	}

	modelMessages := []types.Message{types.UserMessage("retain on failure")}
	ql := &stubQL{messages: append([]types.Message(nil), modelMessages...)}
	wantErr := errors.New("prepare new session")
	ctx := &commands.Context{
		QueryLoop: ql,
		SessionID: "session-old",
		OnEvent:   func(string) {},
		ClearConversation: func() error {
			return wantErr
		},
	}

	if err := cmd.Execute(ctx, ""); !errors.Is(err, wantErr) {
		t.Fatalf("/clear error = %v, want %v", err, wantErr)
	}
	if !messagesEqual(ql.Messages(), modelMessages) {
		t.Fatalf("failed clear changed model context: got %#v want %#v", ql.Messages(), modelMessages)
	}
}

type clearAuditStore struct {
	messages map[string][]types.Message
}

func (s *clearAuditStore) Save(id string, messages []types.Message) error {
	if s.messages == nil {
		s.messages = make(map[string][]types.Message)
	}
	s.messages[id] = append([]types.Message(nil), messages...)
	return nil
}

func (s *clearAuditStore) Load(id string) ([]types.Message, error) {
	messages, ok := s.messages[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return append([]types.Message(nil), messages...), nil
}

func (s *clearAuditStore) List() ([]commands.SessionListEntry, error) { return nil, nil }

func (s *clearAuditStore) Search(query, _ string, _ bool) ([]commands.SessionListEntry, error) {
	for id := range s.messages {
		if strings.EqualFold(id, query) {
			return []commands.SessionListEntry{{ID: id}}, nil
		}
	}
	return nil, nil
}

func (s *clearAuditStore) Rename(_, _ string) error { return nil }

func messagesEqual(left, right []types.Message) bool {
	return reflect.DeepEqual(left, right)
}
