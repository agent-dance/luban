package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// InvocationEnvelopeVersion is the wire version of model-visible skill body
// envelopes. Bump it only for an incompatible representation change.
const InvocationEnvelopeVersion = 1

// InvocationEnvelopeKind describes why an invocation envelope was emitted.
type InvocationEnvelopeKind string

const (
	// InvocationEnvelopeFull carries the complete rendered skill body.
	InvocationEnvelopeFull InvocationEnvelopeKind = "full"
	// InvocationEnvelopeAlreadyLoaded acknowledges that the exact rendered
	// payload is already present in the current visible context epoch.
	InvocationEnvelopeAlreadyLoaded InvocationEnvelopeKind = "already_loaded"
	// InvocationEnvelopeSuperseding carries a new body version and identifies
	// the previously loaded skill digest that it supersedes.
	InvocationEnvelopeSuperseding InvocationEnvelopeKind = "superseding"
)

// InvocationPayloadDigest fingerprints the exact text rendered for one
// invocation after argument and runtime-variable substitution. It is distinct
// from SkillDigest: the latter versions stable SKILL.md content, while two
// invocations of the same SkillDigest may render differently when their
// arguments differ.
type InvocationPayloadDigest string

// Validate reports whether digest uses the canonical SHA-256 representation.
func (digest InvocationPayloadDigest) Validate() error {
	if err := SkillDigest(digest).Validate(); err != nil {
		return fmt.Errorf("invalid invocation payload digest: %w", err)
	}
	return nil
}

// DigestInvocationPayload returns the digest of the exact model-visible body.
func DigestInvocationPayload(body string) InvocationPayloadDigest {
	sum := sha256.Sum256([]byte(body))
	return InvocationPayloadDigest("sha256:" + hex.EncodeToString(sum[:]))
}

// InvocationArguments preserves the difference between omitted arguments and
// an explicitly provided empty string. Value is JSON-encoded by the envelope
// renderer and is never interpolated into envelope syntax.
type InvocationArguments struct {
	Provided bool   `json:"provided"`
	Value    string `json:"value,omitempty"`
}

// NewInvocationArguments converts the pointer convention used by
// PrepareSkillContent into the stable envelope representation.
func NewInvocationArguments(args *string) InvocationArguments {
	if args == nil {
		return InvocationArguments{}
	}
	return InvocationArguments{Provided: true, Value: *args}
}

func (args InvocationArguments) validate() error {
	if !args.Provided && args.Value != "" {
		return errors.New("omitted invocation arguments cannot carry a value")
	}
	if !utf8.ValidString(args.Value) {
		return errors.New("invocation arguments are not valid UTF-8")
	}
	return nil
}

type invocationEnvelopeSkill struct {
	ID       SkillID       `json:"id"`
	Name     string        `json:"name"`
	Revision SkillRevision `json:"revision"`
	Digest   SkillDigest   `json:"digest"`
	Source   SkillSource   `json:"source"`
	Locator  SkillLocator  `json:"locator"`
}

type invocationEnvelopeWire struct {
	Type           string                  `json:"type"`
	Version        int                     `json:"version"`
	Kind           InvocationEnvelopeKind  `json:"kind"`
	Skill          invocationEnvelopeSkill `json:"skill"`
	Arguments      InvocationArguments     `json:"arguments"`
	PayloadDigest  InvocationPayloadDigest `json:"payload_digest"`
	PreviousDigest SkillDigest             `json:"previous_digest,omitempty"`
	Body           *string                 `json:"body,omitempty"`
}

// RenderFullInvocationEnvelope renders a complete, versioned body envelope.
// body must be the exact post-substitution text that will be shown to the
// model; its payload digest is computed by this function rather than trusted
// from a caller.
func RenderFullInvocationEnvelope(skill EffectiveSkill, body string, args InvocationArguments) (string, error) {
	return renderInvocationEnvelope(skill, InvocationEnvelopeFull, "", bodyPointer(body), "", args)
}

// RenderLoadedDigestAcknowledgement renders a short envelope without the
// skill body. Callers must pass the loaded SkillDigest and payload digest from
// the current visible context epoch together with the body prepared for this
// invocation. Both stable and rendered digests are checked before the body is
// omitted, so different arguments cannot accidentally reuse an old rendering.
func RenderLoadedDigestAcknowledgement(
	skill EffectiveSkill,
	loadedDigest SkillDigest,
	loadedPayloadDigest InvocationPayloadDigest,
	currentBody string,
	args InvocationArguments,
) (string, error) {
	if err := loadedDigest.Validate(); err != nil {
		return "", fmt.Errorf("invalid loaded skill digest: %w", err)
	}
	if loadedDigest != skill.Digest {
		return "", errors.New("loaded skill digest does not match current skill digest")
	}
	if err := loadedPayloadDigest.Validate(); err != nil {
		return "", err
	}
	if !utf8.ValidString(currentBody) {
		return "", errors.New("invocation body is not valid UTF-8")
	}
	currentPayloadDigest := DigestInvocationPayload(currentBody)
	if loadedPayloadDigest != currentPayloadDigest {
		return "", errors.New("loaded invocation payload digest does not match current rendered body")
	}
	return renderInvocationEnvelope(skill, InvocationEnvelopeAlreadyLoaded, "", nil, currentPayloadDigest, args)
}

// RenderSupersedingInvocationEnvelope renders a complete body envelope that
// explicitly replaces a previously loaded version of the same stable skill.
func RenderSupersedingInvocationEnvelope(
	skill EffectiveSkill,
	previousDigest SkillDigest,
	body string,
	args InvocationArguments,
) (string, error) {
	if err := previousDigest.Validate(); err != nil {
		return "", fmt.Errorf("invalid previous skill digest: %w", err)
	}
	if previousDigest == skill.Digest {
		return "", errors.New("superseding envelope requires a changed skill digest")
	}
	return renderInvocationEnvelope(skill, InvocationEnvelopeSuperseding, previousDigest, bodyPointer(body), "", args)
}

func renderInvocationEnvelope(
	skill EffectiveSkill,
	kind InvocationEnvelopeKind,
	previousDigest SkillDigest,
	body *string,
	payloadDigest InvocationPayloadDigest,
	args InvocationArguments,
) (string, error) {
	if err := skill.Validate(); err != nil {
		return "", fmt.Errorf("invalid invocation skill: %w", err)
	}
	if err := args.validate(); err != nil {
		return "", err
	}
	if body != nil {
		if !utf8.ValidString(*body) {
			return "", errors.New("invocation body is not valid UTF-8")
		}
		payloadDigest = DigestInvocationPayload(*body)
	}

	switch kind {
	case InvocationEnvelopeFull:
		if body == nil || previousDigest != "" {
			return "", errors.New("full invocation envelope requires a body and no previous digest")
		}
	case InvocationEnvelopeAlreadyLoaded:
		if body != nil || previousDigest != "" {
			return "", errors.New("already-loaded invocation envelope cannot carry a body or previous digest")
		}
		if err := payloadDigest.Validate(); err != nil {
			return "", err
		}
	case InvocationEnvelopeSuperseding:
		if body == nil || previousDigest == "" {
			return "", errors.New("superseding invocation envelope requires a body and previous digest")
		}
	default:
		return "", fmt.Errorf("unknown invocation envelope kind %q", kind)
	}

	wire := invocationEnvelopeWire{
		Type:    "skill_invocation",
		Version: InvocationEnvelopeVersion,
		Kind:    kind,
		Skill: invocationEnvelopeSkill{
			ID:       skill.ID,
			Name:     skill.Name,
			Revision: skill.Revision,
			Digest:   skill.Digest,
			Source:   skill.Source,
			Locator:  skill.Locator,
		},
		Arguments:      args,
		PayloadDigest:  payloadDigest,
		PreviousDigest: previousDigest,
		Body:           body,
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return "", fmt.Errorf("marshal invocation envelope: %w", err)
	}
	return string(encoded), nil
}

func bodyPointer(body string) *string { return &body }
