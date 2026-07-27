package provider

import (
	"encoding/json"
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/agent-dance/luban/types"
)

func TestAnthropicReasoningReplayRequiresAnthropicSignatureKind(t *testing.T) {
	tests := []struct {
		name      string
		kind      types.ThinkingSignatureKind
		signature string
		want      bool
	}{
		{name: "anthropic", kind: types.ThinkingSignatureAnthropic, signature: "anthropic-signature", want: true},
		{name: "openai", kind: types.ThinkingSignatureOpenAIEncryptedReasoning, signature: "openai-encrypted-cipher", want: false},
		{name: "untyped", signature: "legacy-untyped-signature", want: false},
		{name: "missing signature", kind: types.ThinkingSignatureAnthropic, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := convertToAnthropicMessagesForParams(Params{Messages: []types.Message{{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					types.ThinkingBlock{
						Type: types.ContentTypeThinking, Thinking: "visible thinking",
						Signature: test.signature, SignatureKind: test.kind,
					},
					types.TextBlock{Type: types.ContentTypeText, Text: "answer"},
				},
			}}})
			wire, err := json.Marshal(messages)
			if err != nil {
				t.Fatal(err)
			}
			got := string(wire)
			if strings.Contains(got, test.signature) != test.want && test.signature != "" {
				t.Fatalf("signature presence = %v, want %v: %s", strings.Contains(got, test.signature), test.want, got)
			}
			if strings.Contains(got, `"type":"thinking"`) != test.want {
				t.Fatalf("thinking block presence = %v, want %v: %s", strings.Contains(got, `"type":"thinking"`), test.want, got)
			}
		})
	}
}

func TestAnthropicReasoningOutputIsProtocolTagged(t *testing.T) {
	const raw = `{"type":"thinking","thinking":"deep thought","signature":"anthropic-signature"}`

	var streamBlock anthropic.ContentBlockStartEventContentBlockUnion
	if err := json.Unmarshal([]byte(raw), &streamBlock); err != nil {
		t.Fatal(err)
	}
	streamStart := anthropicStreamContentBlock(streamBlock)
	if streamStart.Type != types.ContentTypeThinking || streamStart.Signature != "anthropic-signature" || streamStart.SignatureKind != types.ThinkingSignatureAnthropic {
		t.Fatalf("stream reasoning block = %#v", streamStart)
	}

	var responseBlock anthropic.ContentBlockUnion
	if err := json.Unmarshal([]byte(raw), &responseBlock); err != nil {
		t.Fatal(err)
	}
	responseStart, responseDelta, ok := anthropicBlockToEvents(responseBlock)
	if !ok || responseStart == nil || responseDelta == nil {
		t.Fatalf("response reasoning conversion = %#v, %#v, %v", responseStart, responseDelta, ok)
	}
	if responseStart.Signature != "anthropic-signature" || responseStart.SignatureKind != types.ThinkingSignatureAnthropic {
		t.Fatalf("response reasoning block = %#v", responseStart)
	}
}
