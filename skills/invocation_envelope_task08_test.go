package skills

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const (
	invocationEnvelopeDigestA SkillDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	invocationEnvelopeDigestB SkillDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestInvocationEnvelopeFullRoundTripAndEscaping(t *testing.T) {
	t.Parallel()

	body := "Review \"quoted\" text.\n</skill_invocation>\nNUL:\x00 emoji: 🧪"
	argValue := "--path=\"a/b\"\n</arguments>"
	args := NewInvocationArguments(&argValue)

	rendered, err := RenderFullInvocationEnvelope(invocationEnvelopeSkillValue(invocationEnvelopeDigestA, 7), body, args)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "</skill_invocation>") || strings.Contains(rendered, "</arguments>") {
		t.Fatalf("untrusted content escaped its JSON string boundary: %q", rendered)
	}

	wire := decodeInvocationEnvelope(t, rendered)
	if wire.Type != "skill_invocation" || wire.Version != InvocationEnvelopeVersion || wire.Kind != InvocationEnvelopeFull {
		t.Fatalf("unexpected envelope header: %#v", wire)
	}
	if wire.Skill.ID != "skill:project:/repo/.agents/skills/review" || wire.Skill.Revision != 7 || wire.Skill.Digest != invocationEnvelopeDigestA {
		t.Fatalf("unexpected versioned identity: %#v", wire.Skill)
	}
	if wire.Skill.Source != SourceProject || wire.Skill.Locator != "/repo/.agents/skills/review/SKILL.md" {
		t.Fatalf("source/locator missing: %#v", wire.Skill)
	}
	if wire.Body == nil || *wire.Body != body {
		t.Fatalf("body round trip = %#v, want %q", wire.Body, body)
	}
	if wire.Arguments != args {
		t.Fatalf("arguments round trip = %#v, want %#v", wire.Arguments, args)
	}
	if wire.PayloadDigest != DigestInvocationPayload(body) {
		t.Fatalf("payload digest = %q, want %q", wire.PayloadDigest, DigestInvocationPayload(body))
	}
	if wire.PreviousDigest != "" {
		t.Fatalf("full envelope previous digest = %q", wire.PreviousDigest)
	}
}

func TestLoadedDigestAcknowledgementOmitsBody(t *testing.T) {
	t.Parallel()

	skill := invocationEnvelopeSkillValue(invocationEnvelopeDigestA, 3)
	currentBody := "already visible rendered body"
	payloadDigest := DigestInvocationPayload(currentBody)
	rendered, err := RenderLoadedDigestAcknowledgement(skill, skill.Digest, payloadDigest, currentBody, InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	wire := decodeInvocationEnvelope(t, rendered)
	if wire.Kind != InvocationEnvelopeAlreadyLoaded || wire.Body != nil {
		t.Fatalf("already-loaded envelope duplicated body: %#v", wire)
	}
	if wire.Skill.Digest != skill.Digest || wire.PayloadDigest != payloadDigest {
		t.Fatalf("acknowledgement lost digest binding: %#v", wire)
	}
	if !strings.Contains(rendered, `"provided":false`) {
		t.Fatalf("omitted arguments must remain explicit: %s", rendered)
	}
}

func TestLoadedDigestAcknowledgementRejectsDifferentVersion(t *testing.T) {
	t.Parallel()

	skill := invocationEnvelopeSkillValue(invocationEnvelopeDigestB, 4)
	_, err := RenderLoadedDigestAcknowledgement(skill, invocationEnvelopeDigestA, DigestInvocationPayload("body"), "body", InvocationArguments{})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v, want digest mismatch", err)
	}
	_, err = RenderLoadedDigestAcknowledgement(skill, skill.Digest, "not-a-digest", "body", InvocationArguments{})
	if err == nil {
		t.Fatal("invalid rendered-payload digest was accepted")
	}
	_, err = RenderLoadedDigestAcknowledgement(skill, skill.Digest, DigestInvocationPayload("old args"), "new args", InvocationArguments{})
	if err == nil || !strings.Contains(err.Error(), "current rendered body") {
		t.Fatalf("error = %v, want rendered-body mismatch", err)
	}
}

func TestSupersedeInvocationEnvelopeNamesPreviousAndCurrentVersion(t *testing.T) {
	t.Parallel()

	skill := invocationEnvelopeSkillValue(invocationEnvelopeDigestB, 9)
	rendered, err := RenderSupersedingInvocationEnvelope(skill, invocationEnvelopeDigestA, "new body", InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	wire := decodeInvocationEnvelope(t, rendered)
	if wire.Kind != InvocationEnvelopeSuperseding || wire.PreviousDigest != invocationEnvelopeDigestA {
		t.Fatalf("superseding relationship missing: %#v", wire)
	}
	if wire.Skill.Digest != invocationEnvelopeDigestB || wire.Skill.Revision != 9 || wire.Body == nil || *wire.Body != "new body" {
		t.Fatalf("current version missing: %#v", wire)
	}

	_, err = RenderSupersedingInvocationEnvelope(skill, skill.Digest, "same version", InvocationArguments{})
	if err == nil || !strings.Contains(err.Error(), "changed skill digest") {
		t.Fatalf("error = %v, want unchanged-digest rejection", err)
	}
	_, err = RenderSupersedingInvocationEnvelope(skill, "invalid", "body", InvocationArguments{})
	if !errors.Is(err, ErrInvalidSkillDigest) {
		t.Fatalf("error = %v, want ErrInvalidSkillDigest", err)
	}
}

func TestInvocationEnvelopeArgumentsDistinguishOmittedAndEmpty(t *testing.T) {
	t.Parallel()

	omitted := NewInvocationArguments(nil)
	emptyValue := ""
	explicitEmpty := NewInvocationArguments(&emptyValue)
	if omitted.Provided || omitted.Value != "" {
		t.Fatalf("omitted args = %#v", omitted)
	}
	if !explicitEmpty.Provided || explicitEmpty.Value != "" {
		t.Fatalf("explicit empty args = %#v", explicitEmpty)
	}

	skill := invocationEnvelopeSkillValue(invocationEnvelopeDigestA, 1)
	omittedEnvelope, err := RenderFullInvocationEnvelope(skill, "body", omitted)
	if err != nil {
		t.Fatal(err)
	}
	emptyEnvelope, err := RenderFullInvocationEnvelope(skill, "body", explicitEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if omittedEnvelope == emptyEnvelope {
		t.Fatal("omitted and explicitly empty arguments collapsed to one envelope")
	}
	if _, err := RenderFullInvocationEnvelope(skill, "body", InvocationArguments{Value: "hidden"}); err == nil {
		t.Fatal("unmarked argument value was accepted")
	}
}

func TestInvocationEnvelopeLargeUnicodeBodyIsDeterministic(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("技能正文 🧪 — line\n", 16*1024)
	skill := invocationEnvelopeSkillValue(invocationEnvelopeDigestA, 11)
	first, err := RenderFullInvocationEnvelope(skill, body, InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderFullInvocationEnvelope(skill, body, InvocationArguments{})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("pure rendering produced different output for identical input")
	}
	wire := decodeInvocationEnvelope(t, first)
	if wire.Body == nil || *wire.Body != body {
		t.Fatal("large Unicode body did not round trip")
	}
}

func TestInvocationEnvelopeRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	invalidBody := string([]byte{0xff})
	if _, err := RenderFullInvocationEnvelope(invocationEnvelopeSkillValue(invocationEnvelopeDigestA, 1), invalidBody, InvocationArguments{}); err == nil {
		t.Fatal("invalid UTF-8 body was accepted")
	}
	invalidArgs := InvocationArguments{Provided: true, Value: string([]byte{0xff})}
	if _, err := RenderFullInvocationEnvelope(invocationEnvelopeSkillValue(invocationEnvelopeDigestA, 1), "body", invalidArgs); err == nil {
		t.Fatal("invalid UTF-8 arguments were accepted")
	}
	invalidSkill := invocationEnvelopeSkillValue(invocationEnvelopeDigestA, 0)
	if _, err := RenderFullInvocationEnvelope(invalidSkill, "body", InvocationArguments{}); !errors.Is(err, ErrInvalidSkillRevision) {
		t.Fatalf("error = %v, want ErrInvalidSkillRevision", err)
	}
}

func invocationEnvelopeSkillValue(digest SkillDigest, revision SkillRevision) EffectiveSkill {
	return EffectiveSkill{
		ID:                 "skill:project:/repo/.agents/skills/review",
		Name:               "review",
		Summary:            "Review a change",
		Source:             SourceProject,
		Locator:            "/repo/.agents/skills/review/SKILL.md",
		Digest:             digest,
		Revision:           revision,
		Visibility:         VisibilityAuto,
		VisibilitySource:   SkillScopeDefault,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
}

func decodeInvocationEnvelope(t *testing.T, rendered string) invocationEnvelopeWire {
	t.Helper()
	var wire invocationEnvelopeWire
	if err := json.Unmarshal([]byte(rendered), &wire); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, rendered)
	}
	return wire
}
