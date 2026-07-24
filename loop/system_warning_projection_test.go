package loop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

func TestQuerySystemWarningCarriesOnlyRuntimeEventAcrossLoopBoundary(t *testing.T) {
	secret := "/Users/private/.config/provider token=sk-loop-warning\x1b[2J"
	cause := &types.APIError{Type: "overloaded_error", Message: secret, Status: 529}
	provider := &failNProvider{failUntil: 1, failErr: cause, response: textEvents("recovered")}
	query := New(provider, registry.New(), Config{
		MaxTurns: 2, MaxTokens: 1024, SessionID: "private-session", ProjectRoot: "/private/project",
	})
	var warnings []Event
	if err := query.Run(context.Background(), "continue", func(event Event) {
		if event.Type == EventSystemWarning {
			warnings = append(warnings, event)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Fatal("transient provider failure emitted no semantic warning")
	}
	for _, warning := range warnings {
		if warning.Text != "" || warning.Error != nil || warning.Metadata != nil || warning.ProjectRoot != "" || warning.RuntimeEvent == nil {
			t.Fatalf("warning crossed loop boundary through raw fields: %#v", warning)
		}
		authoritative := warning.SystemWarningRuntimeEvent()
		if !errors.Is(authoritative, cause) {
			t.Fatalf("warning lost private cause: %v", authoritative)
		}
		projection, err := runtimeevent.NewAudienceProjector().Project(authoritative, runtimeevent.ProjectionOptions{
			Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
			Language: i18n.LangEN, LanguageSet: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{secret, "sk-loop-warning", "/private/project", "private-session", "\x1b[2J"} {
			if strings.Contains(string(encoded), private) {
				t.Fatalf("strict loop warning leaked %q: %s", private, encoded)
			}
		}
	}
}

func TestGoalAndContextWarningConstructorsKeepCausePrivate(t *testing.T) {
	secret := "/private/goal/context token=sk-goal-warning\x1b[31m"
	cause := errors.New(secret)
	var events []Event
	emitGoalContinuationWarning(func(event Event) { events = append(events, event) }, 4, i18n.KeyRuntimeGoalLoadFailed, nil, cause)
	events = append(events, NewSystemWarningEvent(i18n.KeyRuntimeAutoCompactFailed, nil, cause, map[string]any{"path": "/private/context"}, 5))

	for _, event := range events {
		if event.Text != "" || event.Error != nil || event.Metadata != nil || event.ProjectRoot != "" {
			t.Fatalf("warning constructor populated legacy raw fields: %#v", event)
		}
		warning := event.SystemWarningRuntimeEvent()
		if !errors.Is(warning, cause) {
			t.Fatal("private warning cause was not retained")
		}
		projection, err := runtimeevent.NewAudienceProjector().Project(warning, runtimeevent.ProjectionOptions{
			Audience: runtimeevent.AudienceModel, Redaction: runtimeevent.RedactionStrict,
		})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(projection.Message, secret) || strings.Contains(projection.Message, "\x1b[31m") {
			t.Fatalf("model warning leaked private cause: %q", projection.Message)
		}
	}
}

func TestLegacyRawSystemWarningFailsClosed(t *testing.T) {
	secret := "/private/legacy token=sk-legacy-warning\x1b[H"
	apiCause := &types.APIError{Type: "private_warning", Message: secret}
	event := Event{
		Type: EventSystemWarning, Text: secret, ProjectRoot: "/private/project",
		Error:    apiCause,
		Metadata: map[string]any{"token": "private-token"},
	}
	if encoded, err := json.Marshal(event); err == nil || len(encoded) != 0 {
		t.Fatalf("generic warning JSON bypass succeeded: %s, err=%v", encoded, err)
	}
	warning := event.SystemWarningRuntimeEvent()
	if !errors.Is(warning, apiCause) {
		t.Fatal("legacy API warning cause lost errors.Is identity")
	}
	public, err := runtimeevent.NewAudienceProjector().Project(warning, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{secret, "sk-legacy-warning", "/private/project", "private-token", "\x1b[H"} {
		if strings.Contains(public.Message, private) {
			t.Fatalf("legacy warning leaked %q: %q", private, public.Message)
		}
	}
}
