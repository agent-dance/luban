package loop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestMessageHistoryLimitAllowsExactBound(t *testing.T) {
	messages := make([]types.Message, maxMessagesHardLimit)
	if err := enforceMessageHistoryLimit(messages); err != nil {
		t.Fatalf("enforceMessageHistoryLimit at exact bound: %v", err)
	}
}

func TestMessageHistoryLimitFailsClosedBeforeProviderWithoutDeletingPrompts(t *testing.T) {
	prov := &historyLimitProvider{}
	q := New(prov, registry.New(), Config{MaxTurns: 1})
	history := make([]types.Message, 0, maxMessagesHardLimit)
	for index := 0; index < maxMessagesHardLimit; index++ {
		history = append(history, types.UserMessage(fmt.Sprintf("ordinary-prompt-%03d", index)))
	}
	q.SetMessages(history)

	wantHistory := append(append([]types.Message(nil), history...), types.UserMessage("over-limit-prompt"))
	err := q.Run(context.Background(), "over-limit-prompt", func(Event) {})
	if err == nil {
		t.Fatal("Run succeeded with an over-limit history")
	}
	var limitErr *MessageHistoryLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error type = %T, want *MessageHistoryLimitError: %v", err, err)
	}
	if limitErr.MessageCount != maxMessagesHardLimit+1 || limitErr.Limit != maxMessagesHardLimit {
		t.Fatalf("limit error = %#v", limitErr)
	}
	if calls := prov.calls.Load(); calls != 0 {
		t.Fatalf("provider calls = %d, want 0", calls)
	}
	if got := q.Messages(); !reflect.DeepEqual(got, wantHistory) {
		t.Fatalf("over-limit history was rewritten:\n got: %#v\nwant: %#v", got, wantHistory)
	}
}

type historyLimitProvider struct {
	calls atomic.Int32
}

func (*historyLimitProvider) Name() string    { return "history-limit-test" }
func (*historyLimitProvider) ModelID() string { return "history-limit-test" }

func (p *historyLimitProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.calls.Add(1)
	stream := make(chan types.StreamEvent)
	close(stream)
	return stream, nil
}
