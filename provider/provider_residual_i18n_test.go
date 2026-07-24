package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestResponsesParseErrorsUseActiveRuntimeLanguage(t *testing.T) {
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("SaveLanguage(LangZH): %v", err)
	}
	t.Cleanup(func() {
		if err := i18n.SaveLanguage(i18n.LangEN); err != nil {
			t.Errorf("restore language: %v", err)
		}
	})

	tests := []struct {
		eventType  string
		wantPrefix string
	}{
		{"response.completed", "解析 response.completed 失败："},
		{"response.failed", "解析 response.failed 失败："},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			body := strings.NewReader(buildSSEStream([]sseEvent{{Type: tt.eventType, Data: "not-json"}}))
			ch := make(chan types.StreamEvent, 4)
			p := &ResponsesProvider{}
			p.processResponsesStream(context.Background(), body, ch)
			close(ch)

			var got *types.APIError
			for event := range ch {
				if event.Type == types.EventError {
					got = event.Error
					break
				}
			}
			if got == nil {
				t.Fatal("expected parse error event")
			}
			if !strings.HasPrefix(got.Message, tt.wantPrefix) {
				t.Fatalf("APIError.Message = %q, want prefix %q", got.Message, tt.wantPrefix)
			}
			if !strings.Contains(got.Message, "invalid character") {
				t.Fatalf("APIError.Message = %q; raw JSON diagnostic was not preserved", got.Message)
			}
		})
	}
}
