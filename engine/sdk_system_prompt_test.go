package engine_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sdk"
	"github.com/agent-dance/luban/types"
)

type sdkPromptProvider struct {
	mu     sync.Mutex
	params provider.Params
}

func (p *sdkPromptProvider) Name() string    { return "sdk-prompt" }
func (p *sdkPromptProvider) ModelID() string { return "sdk-prompt-model" }
func (p *sdkPromptProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.params = params
	p.mu.Unlock()

	ch := make(chan types.StreamEvent, 4)
	ch <- types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}}
	ch <- types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "ok"}}
	ch <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
	ch <- types.StreamEvent{Type: types.EventMessageStop}
	close(ch)
	return ch, nil
}

func TestSDKInitializeSystemPromptChangesNextQueryPrompt(t *testing.T) {
	prov := &sdkPromptProvider{}
	eng, err := engine.New(engine.Config{
		Provider:     prov,
		Sessions:     newSDKPromptMemorySessions(),
		SystemPrompt: "default system prompt",
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer outW.Close()

	srv := sdk.NewSDKServer(eng, inR, outW)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx) }()

	scanCh := startSDKPromptScanner(outR)
	expectSDKPromptMessage(t, scanCh, "system", "init")

	initPayload, _ := json.Marshal(sdk.InitializeRequest{
		Subtype:      "initialize",
		SystemPrompt: "custom sdk prompt",
	})
	writeSDKPromptLine(t, inW, sdk.SDKControlRequest{
		Type:      "control_request",
		RequestID: "init-1",
		Request:   initPayload,
	})
	expectSDKPromptMessage(t, scanCh, "system", "status")
	expectSDKPromptMessage(t, scanCh, "control_response", "")

	writeSDKPromptLine(t, inW, sdk.SDKUserMessage{
		Type:      "user",
		SessionID: "sdk-system-prompt-session",
		Message:   json.RawMessage(`"hello"`),
	})
	expectSDKPromptMessage(t, scanCh, "streamlined_text", "")
	expectSDKPromptMessage(t, scanCh, "result", "success")

	inW.Close()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for SDK server shutdown")
	}

	prov.mu.Lock()
	got := prov.params.JoinedSystemPrompt()
	blocks := len(prov.params.SystemBlocks)
	prov.mu.Unlock()
	if got != "custom sdk prompt" {
		t.Fatalf("JoinedSystemPrompt = %q, want custom SDK prompt", got)
	}
	if blocks != 1 {
		t.Fatalf("SystemBlocks = %d, want 1 custom block", blocks)
	}
}

type sdkPromptMemorySessions struct {
	mu       sync.Mutex
	messages map[string][]types.Message
}

func newSDKPromptMemorySessions() *sdkPromptMemorySessions {
	return &sdkPromptMemorySessions{messages: make(map[string][]types.Message)}
}

func (s *sdkPromptMemorySessions) Save(id string, messages []types.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[id] = append([]types.Message(nil), messages...)
	return nil
}

func (s *sdkPromptMemorySessions) Load(id string) ([]types.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages, ok := s.messages[id]
	if !ok {
		return nil, engine.ErrSessionNotFound
	}
	return append([]types.Message(nil), messages...), nil
}

func (s *sdkPromptMemorySessions) List() ([]engine.SessionInfo, error) { return nil, nil }
func (s *sdkPromptMemorySessions) Latest() (string, error)             { return "", engine.ErrSessionNotFound }
func (s *sdkPromptMemorySessions) Delete(string) error                 { return nil }

func writeSDKPromptLine(t *testing.T, w io.Writer, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal SDK line: %v", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", data); err != nil {
		t.Fatalf("write SDK line: %v", err)
	}
}

func startSDKPromptScanner(r io.Reader) chan map[string]any {
	ch := make(chan map[string]any, 16)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			var m map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &m); err == nil {
				ch <- m
			}
		}
		close(ch)
	}()
	return ch
}

func expectSDKPromptMessage(t *testing.T, ch chan map[string]any, typ, subtype string) map[string]any {
	t.Helper()
	select {
	case m, ok := <-ch:
		if !ok {
			t.Fatalf("expected %s/%s message, got closed channel", typ, subtype)
		}
		if m["type"] != typ {
			t.Fatalf("message type = %v, want %s (message %#v)", m["type"], typ, m)
		}
		if subtype != "" && m["subtype"] != subtype {
			t.Fatalf("message subtype = %v, want %s (message %#v)", m["subtype"], subtype, m)
		}
		return m
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s/%s message", typ, subtype)
		return nil
	}
}
