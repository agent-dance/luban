package loop

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

type parityFakeProvider struct {
	mu        sync.Mutex
	name      string
	modelID   string
	turns     []parityProviderTurn
	turnIndex int
	Calls     []provider.Params
}

type parityProviderTurn struct {
	Events  []types.StreamEvent
	Error   *types.APIError
	DelayMS int
}

func newParityFakeProvider(turns []parityProviderTurn) *parityFakeProvider {
	return &parityFakeProvider{
		name:    "parity-fake",
		modelID: "parity-model",
		turns:   turns,
	}
}

func (p *parityFakeProvider) Name() string { return p.name }

func (p *parityFakeProvider) ModelID() string { return p.modelID }

func (p *parityFakeProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	idx := p.turnIndex
	p.turnIndex++
	p.Calls = append(p.Calls, params)
	p.mu.Unlock()

	if idx >= len(p.turns) {
		ch := make(chan types.StreamEvent)
		close(ch)
		return ch, nil
	}
	turn := p.turns[idx]
	if turn.Error != nil {
		return nil, turn.Error
	}
	events := attachTestProviderCommitReceipts(turn.Events)
	ch := make(chan types.StreamEvent, max(1, len(events)))
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
		for _, evt := range events {
			if ctx.Err() != nil {
				return
			}
			ch <- evt
		}
	}()
	return ch, nil
}

type parityTool struct {
	name           string
	description    string
	content        string
	echoPrefix     string
	isError        bool
	concurrentSafe bool
	newMessages    []types.Message
	contentBlocks  []types.ContentBlock
}

func (t parityTool) Name() string { return t.name }

func (t parityTool) Description() string {
	if t.description != "" {
		return t.description
	}
	return "parity fixture tool"
}

func (t parityTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}

func (t parityTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	content := t.content
	if t.echoPrefix != "" {
		text, _ := input["text"].(string)
		content = t.echoPrefix + text
	}
	return types.ToolResult{
		Content:       content,
		ContentBlocks: t.contentBlocks,
		IsError:       t.isError,
		NewMessages:   t.newMessages,
	}, nil
}

func (t parityTool) IsConcurrentSafe() bool { return t.concurrentSafe }

func newParityRegistry(t *testing.T, specs []parityToolFixture) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, spec := range specs {
		switch spec.Kind {
		case "echo":
			reg.Register(parityTool{name: spec.Name, echoPrefix: spec.EchoPrefix, concurrentSafe: true})
		case "static":
			reg.Register(parityTool{name: spec.Name, content: spec.Content, isError: spec.IsError, concurrentSafe: spec.ConcurrentSafe})
		case "attachment":
			reg.Register(parityTool{
				name:           spec.Name,
				content:        spec.Content,
				concurrentSafe: true,
				newMessages: []types.Message{{
					Role: types.RoleUser,
					Content: []types.ContentBlock{types.ImageBlock{
						Type: types.ContentTypeImage,
						Source: &types.ImageSource{
							Type:      "base64",
							MediaType: "image/png",
							Data:      "iVBORw0KGgo=",
						},
					}},
				}},
			})
		case "tool_search":
			reg.Register(parityTool{
				name:           spec.Name,
				content:        spec.Content,
				concurrentSafe: false,
				contentBlocks: []types.ContentBlock{types.ToolReferenceBlock{
					Type:     types.ContentTypeToolReference,
					ToolName: spec.ReferenceTool,
				}},
			})
		default:
			t.Fatalf("unknown parity tool kind %q for %q", spec.Kind, spec.Name)
		}
	}
	return reg
}

func parityTextEvents(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonEndTurn)},
		{Type: types.EventMessageStop},
	}
}

func stopReasonForParity(reason types.StopReason) *types.StopReason {
	return &reason
}

func hasString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func projectedSystemWarningText(event stream.Event) string {
	projection, err := runtimeevent.NewAudienceProjector().Project(runtimeevent.SystemWarningRuntimeEvent(event), runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
		Language: i18n.LangEN, LanguageSet: true,
	})
	if err != nil {
		return ""
	}
	return projection.Message
}

func joinedEventText(events []stream.Event) string {
	var b strings.Builder
	for _, evt := range events {
		if evt.Type == stream.EventText {
			b.WriteString(evt.Text)
		}
	}
	return b.String()
}
