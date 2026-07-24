package provider

import (
	"context"
	"sync"
	"time"

	"github.com/agent-dance/luban/types"
)

// FakeProvider is a small provider test helper for deterministic stream tests.
// It intentionally lives in _test code so production provider behavior stays untouched.
type FakeProvider struct {
	mu        sync.Mutex
	NameValue string
	Model     string
	Turns     []FakeProviderTurn
	TurnIndex int
	Calls     []Params
}

type FakeProviderTurn struct {
	Events  []types.StreamEvent
	Error   error
	DelayMS int
}

func NewFakeProvider(turns ...FakeProviderTurn) *FakeProvider {
	return &FakeProvider{
		NameValue: "fake",
		Model:     "fake-model",
		Turns:     turns,
	}
}

func (p *FakeProvider) Name() string { return p.NameValue }

func (p *FakeProvider) ModelID() string { return p.Model }

func (p *FakeProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	idx := p.TurnIndex
	p.TurnIndex++
	p.Calls = append(p.Calls, params)
	p.mu.Unlock()

	if idx >= len(p.Turns) {
		ch := make(chan types.StreamEvent)
		close(ch)
		return ch, nil
	}
	turn := p.Turns[idx]
	if turn.Error != nil {
		return nil, turn.Error
	}
	ch := make(chan types.StreamEvent, max(1, len(turn.Events)))
	go func() {
		defer close(ch)
		if turn.DelayMS > 0 {
			timer := time.NewTimer(time.Duration(turn.DelayMS) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		for _, evt := range turn.Events {
			if ctx.Err() != nil {
				return
			}
			ch <- evt
		}
	}()
	return ch, nil
}
