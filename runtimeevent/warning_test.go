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

func TestWarningProjectionKeepsPrivateDiagnosticsOutOfDefaultAudiences(t *testing.T) {
	secret := "/Users/private/.ssh/id_ed25519 token=sk-warning-secret\x1b[2J"
	cause := errors.New(secret)
	event := NewWarningEvent(
		types.RuntimeIdentity{EventID: "warning-event", SessionID: "private-session", TurnID: "private-turn"},
		i18n.KeyRuntimeAutoCompactFailed,
		nil,
		cause,
		map[string]any{"authorization": "Bearer private-token", "project_root": "/private/project"},
	)
	if !errors.Is(event, cause) {
		t.Fatal("warning lost its private errors.Is chain")
	}

	projector := NewAudienceProjector()
	for _, audience := range []Audience{AudienceUser, AudienceModel, AudienceSDK} {
		projection, err := projector.Project(event, ProjectionOptions{
			Audience: audience, Redaction: RedactionStrict, Language: i18n.LangEN, LanguageSet: true,
		})
		if err != nil {
			t.Fatalf("%s projection: %v", audience, err)
		}
		if projection.Kind != types.RuntimeEventKindWarning || projection.Message != i18n.Text(i18n.LangEN, i18n.KeyRuntimeAutoCompactFailed) {
			t.Fatalf("%s projection = %#v", audience, projection)
		}
		encoded, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{secret, "sk-warning-secret", "private-token", "/private/project", "authorization", "project_root", "\x1b[2J", "private_cause", "private_metadata"} {
			if strings.Contains(string(encoded), private) {
				t.Fatalf("%s warning projection leaked %q: %s", audience, private, encoded)
			}
		}
	}

	raw, err := projector.Project(event, ProjectionOptions{
		Audience: AudienceAudit, Redaction: RedactionRaw, AllowRawAudit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if raw.PrivateCause != secret || raw.PrivateMetadata["project_root"] != "/private/project" {
		t.Fatalf("explicit raw audit lost private diagnostics: %#v", raw)
	}
}

func TestWarningProjectorIsConcurrentAndImmutable(t *testing.T) {
	cause := errors.New("private-race-cause token=sk-race\x1b[31m")
	event := NewWarningEvent(types.RuntimeIdentity{}, i18n.KeyRuntimePostCompactCleanupFailed, nil, cause, map[string]any{"path": "/private/race"})
	var wait sync.WaitGroup
	for range 100 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for _, audience := range []Audience{AudienceUser, AudienceModel, AudienceSDK} {
				projection, err := NewAudienceProjector().Project(event, ProjectionOptions{Audience: audience, Redaction: RedactionStrict})
				if err != nil || strings.Contains(projection.Message, cause.Error()) {
					t.Errorf("%s projection = %#v, err=%v", audience, projection, err)
				}
			}
		}()
	}
	wait.Wait()
	if !errors.Is(event, cause) || event.PrivateMetadata["path"] != "/private/race" {
		t.Fatalf("source warning mutated: %#v", event)
	}
}
