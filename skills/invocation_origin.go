package skills

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// InvocationOrigin identifies the authority path requesting a skill. It is a
// transport-neutral contract shared by command surfaces and SkillTool; callers
// must not infer it from tool-use IDs or presentation state.
type InvocationOrigin string

const (
	InvocationOriginModel InvocationOrigin = "model"
	InvocationOriginUser  InvocationOrigin = "user"
)

// Validate rejects unknown origins so future values fail closed until every
// enforcement boundary understands them.
func (origin InvocationOrigin) Validate() error {
	switch origin {
	case InvocationOriginModel, InvocationOriginUser:
		return nil
	default:
		return fmt.Errorf("invalid skill invocation origin %q", origin)
	}
}

// SkillExecutionReceiptMetadataKey is the ToolResult metadata slot carrying a
// pending loaded-ledger commit. Execution emits the receipt; the query loop
// commits it only after the corresponding result enters visible history.
const SkillExecutionReceiptMetadataKey = "skillExecutionReceipt"

// SkillExecutionReceipt is transport-neutral evidence that one exact rendered
// skill envelope is ready to become visible in one context epoch.
type SkillExecutionReceipt struct {
	ContextEpoch            uint64                  `json:"context_epoch"`
	SkillID                 SkillID                 `json:"skill_id"`
	ContentDigest           SkillDigest             `json:"content_digest"`
	InvocationPayloadDigest InvocationPayloadDigest `json:"invocation_payload_digest"`
	InvocationEnvelopeKind  InvocationEnvelopeKind  `json:"invocation_envelope_kind"`
}

// Validate fails closed on incomplete identities, zero epochs, and unknown
// envelope kinds.
func (receipt SkillExecutionReceipt) Validate() error {
	if receipt.ContextEpoch == 0 {
		return errors.New("skill execution receipt context epoch is zero")
	}
	if err := receipt.SkillID.Validate(); err != nil {
		return err
	}
	if err := receipt.ContentDigest.Validate(); err != nil {
		return err
	}
	if err := receipt.InvocationPayloadDigest.Validate(); err != nil {
		return err
	}
	switch receipt.InvocationEnvelopeKind {
	case InvocationEnvelopeFull, InvocationEnvelopeAlreadyLoaded, InvocationEnvelopeSuperseding:
		return nil
	default:
		return fmt.Errorf("invalid skill invocation envelope kind %q", receipt.InvocationEnvelopeKind)
	}
}

// MarshalSkillExecutionReceipt produces the canonical JSON metadata value.
func MarshalSkillExecutionReceipt(receipt SkillExecutionReceipt) (string, error) {
	if err := receipt.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("marshal skill execution receipt: %w", err)
	}
	return string(encoded), nil
}

// UnmarshalSkillExecutionReceipt decodes one strict JSON metadata value and
// rejects unknown fields or trailing documents before validating the receipt.
func UnmarshalSkillExecutionReceipt(encoded string) (SkillExecutionReceipt, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(encoded))
	decoder.DisallowUnknownFields()
	var receipt SkillExecutionReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return SkillExecutionReceipt{}, fmt.Errorf("decode skill execution receipt: %w", err)
	}
	if err := ensureReceiptJSONEOF(decoder); err != nil {
		return SkillExecutionReceipt{}, err
	}
	if err := receipt.Validate(); err != nil {
		return SkillExecutionReceipt{}, err
	}
	return receipt, nil
}

// EncodeSkillExecutionReceiptMetadata returns a fresh metadata map suitable
// for merging into ToolResult.Metadata.
func EncodeSkillExecutionReceiptMetadata(receipt SkillExecutionReceipt) (map[string]string, error) {
	encoded, err := MarshalSkillExecutionReceipt(receipt)
	if err != nil {
		return nil, err
	}
	return map[string]string{SkillExecutionReceiptMetadataKey: encoded}, nil
}

// DecodeSkillExecutionReceiptMetadata extracts a receipt without treating an
// absent metadata key as an error.
func DecodeSkillExecutionReceiptMetadata(metadata map[string]string) (SkillExecutionReceipt, bool, error) {
	if len(metadata) == 0 {
		return SkillExecutionReceipt{}, false, nil
	}
	encoded, exists := metadata[SkillExecutionReceiptMetadataKey]
	if !exists {
		return SkillExecutionReceipt{}, false, nil
	}
	receipt, err := UnmarshalSkillExecutionReceipt(encoded)
	if err != nil {
		return SkillExecutionReceipt{}, true, err
	}
	return receipt, true, nil
}

func ensureReceiptJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode skill execution receipt: trailing JSON value")
		}
		return fmt.Errorf("decode skill execution receipt: trailing data: %w", err)
	}
	return nil
}
