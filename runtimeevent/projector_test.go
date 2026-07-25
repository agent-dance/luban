package runtimeevent

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func projectionFixture(private error) types.RuntimeEvent {
	event := types.NewRuntimeEvent(
		types.RuntimeEventKindError,
		types.RuntimeIdentity{
			EventID: "event-17", SessionID: "session-private", Epoch: 3,
			ContextGeneration: 11, TurnID: "turn-private", ToolUseID: "tool-private",
			WorkUnitID: "work-private", ActorID: "actor-private", ActorType: "reviewer",
		},
		types.ToolOutcomeFailed,
		i18n.KeyRuntimeErrorPublicSummary,
		nil,
		"runtime.operation_failed",
		private,
	)
	event.PrivateMetadata = map[string]any{"authorization": "private-token"}
	event.EvidenceRef = &types.RuntimeEvidenceRef{ID: "evidence-private", Digest: "digest-private"}
	return event
}

func TestAudienceRedactionMatrix(t *testing.T) {
	private := errors.New("private-cause-secret")
	event := projectionFixture(private)
	projector := NewAudienceProjector()
	tests := []struct {
		name         string
		options      ProjectionOptions
		wantIdentity bool
		wantOutcome  bool
		wantEvidence bool
	}{
		{"user-strict", ProjectionOptions{Audience: AudienceUser, Redaction: RedactionStrict}, false, true, false},
		{"model-strict", ProjectionOptions{Audience: AudienceModel, Redaction: RedactionStrict}, false, true, false},
		{"sdk-strict", ProjectionOptions{Audience: AudienceSDK, Redaction: RedactionStrict}, true, true, false},
		{"sdk-diagnostic", ProjectionOptions{Audience: AudienceSDK, Redaction: RedactionDiagnostic}, true, true, true},
		{"audit-strict", ProjectionOptions{Audience: AudienceAudit, Redaction: RedactionStrict}, true, true, false},
		{"audit-diagnostic", ProjectionOptions{Audience: AudienceAudit, Redaction: RedactionDiagnostic}, true, true, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection, err := projector.Project(event, test.options)
			if err != nil {
				t.Fatal(err)
			}
			if (projection.EventID != "") != test.wantIdentity {
				t.Fatalf("identity disclosure = %v, want %v: %#v", projection.EventID != "", test.wantIdentity, projection)
			}
			if (projection.Outcome != "") != test.wantOutcome {
				t.Fatalf("outcome disclosure = %v, want %v", projection.Outcome != "", test.wantOutcome)
			}
			if (projection.EvidenceRef != nil) != test.wantEvidence {
				t.Fatalf("evidence disclosure = %v, want %v", projection.EvidenceRef != nil, test.wantEvidence)
			}
			encoded, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{private.Error(), "private-token"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("%s projection leaked %q: %s", test.name, secret, encoded)
				}
			}
		})
	}
}

func TestRawAuditRequiresExplicitOptIn(t *testing.T) {
	private := errors.New("raw-private-cause")
	event := projectionFixture(private)
	projector := NewAudienceProjector()

	_, err := projector.Project(event, ProjectionOptions{Audience: AudienceAudit, Redaction: RedactionRaw})
	if !errors.Is(err, ErrRawAuditNotEnabled) {
		t.Fatalf("raw audit without opt-in error = %v", err)
	}
	_, err = projector.Project(event, ProjectionOptions{Audience: AudienceSDK, Redaction: RedactionRaw, AllowRawAudit: true})
	if !errors.Is(err, ErrRawNonAuditAudience) {
		t.Fatalf("raw SDK projection error = %v", err)
	}

	projection, err := projector.Project(event, ProjectionOptions{Audience: AudienceAudit, Redaction: RedactionRaw, AllowRawAudit: true})
	if err != nil {
		t.Fatal(err)
	}
	if projection.PrivateCause != private.Error() || projection.PrivateMetadata["authorization"] != "private-token" {
		t.Fatalf("explicit raw projection omitted private audit data: %#v", projection)
	}
}

func TestProjectionRendersMessageAtFinalLanguageBoundary(t *testing.T) {
	event := projectionFixture(errors.New("private"))
	projector := NewAudienceProjector()
	for _, lang := range i18n.AllLanguages() {
		projection, err := projector.Project(event, ProjectionOptions{
			Audience: AudienceUser, Redaction: RedactionStrict, Language: lang, LanguageSet: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if want := i18n.Text(lang, i18n.KeyRuntimeErrorPublicSummary); projection.Message != want {
			t.Fatalf("%s message = %q, want %q", lang.Code(), projection.Message, want)
		}
	}
}

func TestProjectionRequiresExplicitAudienceAndRedaction(t *testing.T) {
	event := projectionFixture(errors.New("private"))
	projector := NewAudienceProjector()

	_, err := projector.Project(event, ProjectionOptions{Redaction: RedactionStrict})
	if !errors.Is(err, ErrInvalidAudience) {
		t.Fatalf("missing audience error = %v", err)
	}
	_, err = projector.Project(event, ProjectionOptions{Audience: AudienceUser})
	if !errors.Is(err, ErrInvalidRedaction) {
		t.Fatalf("missing redaction error = %v", err)
	}
}

func TestSDKProjectionPreservesStableIdentityAndSchema(t *testing.T) {
	event := projectionFixture(errors.New("private"))
	projection, err := NewAudienceProjector().Project(event, ProjectionOptions{
		Audience: AudienceSDK, Redaction: RedactionDiagnostic, Language: i18n.LangEN, LanguageSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != types.RuntimeEventSchemaVersion || projection.EventID != "event-17" ||
		projection.SessionID != "session-private" || projection.Epoch != 3 || projection.ContextGeneration != 11 ||
		projection.TurnID != "turn-private" || projection.ToolUseID != "tool-private" ||
		projection.WorkUnitID != "work-private" || projection.ActorID != "actor-private" || projection.ActorType != "reviewer" {
		t.Fatalf("identity/schema drift: %#v", projection)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"schema_version":"runtime-event/v2"`, `"audience":"sdk"`, `"redaction_level":"diagnostic"`, `"context_generation":11`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("schema field %s missing from %s", field, encoded)
		}
	}
	for _, forbidden := range []string{"private_cause", "private_metadata", "private-token"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("SDK schema leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestProjectionRejectsMissingAuthoritativeOutcome(t *testing.T) {
	event := types.NewToolResultRuntimeEvent(
		types.RuntimeIdentity{EventID: "event-incomplete"},
		types.ToolResultBlock{ToolUseID: "tool-incomplete", IsError: true, Content: `{"error":true}`},
		i18n.KeyRuntimeErrorPublicSummary,
		nil,
	)
	_, err := NewAudienceProjector().Project(event, ProjectionOptions{Audience: AudienceSDK, Redaction: RedactionStrict})
	if !errors.Is(err, ErrMissingOutcome) {
		t.Fatalf("missing outcome error = %v", err)
	}
}

func TestAudienceProjectorIsConcurrentAndDoesNotMutateEvent(t *testing.T) {
	event := projectionFixture(errors.New("private"))
	projector := NewAudienceProjector()
	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			projection, err := projector.Project(event, ProjectionOptions{Audience: AudienceSDK, Redaction: RedactionStrict})
			if err != nil || projection.ContextGeneration != 11 {
				t.Errorf("projection = %#v, err = %v", projection, err)
			}
		}()
	}
	wait.Wait()
	if event.PrivateMetadata["authorization"] != "private-token" || event.EvidenceRef.ID != "evidence-private" {
		t.Fatalf("source event mutated: %#v", event)
	}
}
