package pierbackend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

// ValidateArchivedProviderProjection proves that every provider-owned field in
// normalized evidence is the exact projection of one immutable raw-v6 ledger
// snapshot. Client execution fields are deliberately outside this comparison;
// they are independently closed by the tool-trace coverage contract.
func ValidateArchivedProviderProjection(
	raw []byte,
	expectedRawSHA256 string,
	normalized []harness.ProviderRoundEvidence,
	agent harness.AgentSpec,
	expectedRunIdentity string,
	canonicalizationBindingSHA string,
	endpoint harness.ProviderEndpointSpec,
) error {
	if !lowerHexSHA256(expectedRawSHA256) {
		return errors.New("archived provider projection lacks the frozen raw-ledger digest")
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != expectedRawSHA256 {
		return errors.New("archived provider projection raw-ledger digest mismatch")
	}
	// The exported archive validator is a formal boundary. Codex's raw
	// service-tier proof is only provisional until the controller seals the
	// final binding receipt, so an archive without that receipt can never be
	// accepted here. The projector itself also serves isolated, deliberately
	// unsealed parser fixtures; those retain their raw static proof instead.
	if agent.ID == "codex" && !lowerHexSHA256(canonicalizationBindingSHA) {
		return errors.New("Codex archived raw projection lacks its final service-tier canonicalization binding")
	}
	if agent.ID != "codex" && canonicalizationBindingSHA != "" {
		return errors.New("non-Codex archived raw projection has a Codex canonicalization binding")
	}
	records, err := decodeStrictProviderRecords(raw)
	if err != nil {
		return err
	}
	if err := ValidateArchivedProviderTLSRecords(records, expectedRunIdentity, endpoint); err != nil {
		return err
	}
	for _, record := range records {
		if err := evidenceproxy.ValidateServiceTierTransformationProof(record); err != nil {
			return fmt.Errorf("provider round %d has an invalid service-tier transformation proof: %w", record.Round, err)
		}
	}
	projected, resultReceipts, err := projectProviderRecords(records, agent, expectedRunIdentity, canonicalizationBindingSHA)
	if err != nil {
		return err
	}
	// Tool-result output sizes are provider-owned raw evidence. Correlate them
	// without any client trace before scrubbing client-only annotations.
	slices.SortFunc(projected, func(left, right harness.ProviderRoundEvidence) int { return left.Round - right.Round })
	if err := correlateToolEvidence(projected, resultReceipts, parsedTrace{}); err != nil {
		return err
	}
	slices.SortFunc(projected, func(left, right harness.ProviderRoundEvidence) int {
		if left.EvidenceSequence < right.EvidenceSequence {
			return -1
		}
		if left.EvidenceSequence > right.EvidenceSequence {
			return 1
		}
		return 0
	})
	if len(projected) != len(normalized) {
		return fmt.Errorf("normalized provider projection has %d rounds, raw authority has %d", len(normalized), len(projected))
	}
	for index := range projected {
		expected := scrubClientExecutionProjection(projected[index])
		actual := scrubClientExecutionProjection(normalized[index])
		if !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("normalized provider evidence sequence %d differs from its raw-v6 projection", index)
		}
	}
	return nil
}

func decodeStrictProviderRecords(raw []byte) ([]evidenceproxy.Record, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return nil, errors.New("archived provider projection is not newline-terminated canonical JSONL")
	}
	lines := bytes.Split(raw[:len(raw)-1], []byte{'\n'})
	records := make([]evidenceproxy.Record, 0, len(lines))
	for index, line := range lines {
		if len(line) == 0 {
			return nil, fmt.Errorf("raw provider JSONL line %d is empty", index+1)
		}
		if err := rejectDuplicateProviderJSONKeys(line); err != nil {
			return nil, fmt.Errorf("decode raw provider JSONL line %d: %w", index+1, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		var record evidenceproxy.Record
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("decode raw provider JSONL line %d: %w", index+1, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if err == nil {
				return nil, fmt.Errorf("decode raw provider JSONL line %d: trailing JSON value", index+1)
			}
			return nil, fmt.Errorf("decode raw provider JSONL line %d trailer: %w", index+1, err)
		}
		canonical, err := json.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("canonicalize raw provider JSONL line %d: %w", index+1, err)
		}
		if !bytes.Equal(line, canonical) {
			return nil, fmt.Errorf("raw provider JSONL line %d is not exact canonical JSON", index+1)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, errors.New("archived provider projection contains no raw records")
	}
	return records, nil
}

func rejectDuplicateProviderJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var consume func() error
	consume = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, composite := token.(json.Delim)
		if !composite {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("JSON object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if err := consume(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return errors.New("JSON object is not closed")
			}
		case '[':
			for decoder.More() {
				if err := consume(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return errors.New("JSON array is not closed")
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		return nil
	}
	if err := consume(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func scrubClientExecutionProjection(round harness.ProviderRoundEvidence) harness.ProviderRoundEvidence {
	round.ToolCalls = append([]harness.ToolCallEvidence(nil), round.ToolCalls...)
	for index := range round.ToolCalls {
		call := &round.ToolCalls[index]
		call.DurationMS = nil
		call.Error = nil
		call.OutputBytes = nil
		call.AgentTraceOutputBytes = nil
		call.TraceMatch = ""
		call.TraceKind = ""
	}
	round.PhysicalToolOperations = nil
	round.ToolCriticalPathMS = nil
	round.ToolTotalLatencyMS = nil
	round.ToolQueueMS = nil
	return round
}

func projectProviderRecords(
	records []evidenceproxy.Record,
	agent harness.AgentSpec,
	expectedRunIdentity string,
	canonicalizationBindingSHA string,
) ([]harness.ProviderRoundEvidence, map[string]providerToolResultReceipt, error) {
	if !lowerHexSHA256(expectedRunIdentity) {
		return nil, nil, errors.New("provider evidence run identity is invalid")
	}
	if agent.ID != "codex" && canonicalizationBindingSHA != "" {
		return nil, nil, errors.New("non-Codex raw projection has a Codex canonicalization binding")
	}
	rounds := make([]harness.ProviderRoundEvidence, 0, len(records))
	resultReceipts := map[string]providerToolResultReceipt{}
	startOrdinals := make(map[int]struct{}, len(records))
	for _, record := range records {
		if record.SchemaVersion != "agentic-bench/provider-http-v6" {
			return nil, nil, fmt.Errorf("provider transport round %d has unsupported evidence schema %q", record.Round, record.SchemaVersion)
		}
		if record.RunIdentity != expectedRunIdentity {
			return nil, nil, fmt.Errorf("provider transport round %d belongs to another run identity", record.Round)
		}
		if record.ProviderAttemptStarted {
			switch agent.Model.TransportRequirement {
			case harness.TransportRequirementHTTPInference:
				if record.Transport != "http_sse" || record.ProviderAttemptKind != "inference" ||
					record.WebSocketConnectionHash != "" || record.WebSocketRequestSequence != 0 || record.WebSocketConnectionReused ||
					record.WebSocketHandshakeStatus != 0 || record.WebSocketHandshakeModel != "" {
					return nil, nil, fmt.Errorf("formal v8 provider round %d violates the HTTP-only inference contract", record.Round)
				}
			case harness.TransportRequirementWebSocket:
				if record.Transport != "websocket" || record.WebSocketHandshakeStatus != http.StatusSwitchingProtocols ||
					record.WebSocketHandshakeModel != agent.Model.Model || !record.WebSocketChainBound {
					return nil, nil, fmt.Errorf("formal v8 provider round %d lacks a successful pinned WebSocket handshake", record.Round)
				}
			default:
				return nil, nil, fmt.Errorf("formal v8 provider round %d has an unknown transport requirement", record.Round)
			}
		}
		if record.Disposition == "experiment_invalid" {
			return nil, nil, fmt.Errorf("provider transport round %d invalidates the experiment: %s", record.Round, record.ErrorCode)
		}
		if record.Disposition != "valid" && record.Disposition != "prewarm_transport" && record.Disposition != "provider_infra_exclusion" && record.Disposition != "agent_context_failure" {
			return nil, nil, fmt.Errorf("provider transport round %d has unknown disposition %q", record.Round, record.Disposition)
		}
		if record.Round < 0 {
			return nil, nil, fmt.Errorf("provider transport round %d has a negative start ordinal", record.Round)
		}
		if _, duplicate := startOrdinals[record.Round]; duplicate {
			return nil, nil, fmt.Errorf("provider transport start ordinal %d is duplicated", record.Round)
		}
		startOrdinals[record.Round] = struct{}{}
		if record.Transport == "http_sse" && record.ProviderAttemptStarted && record.HTTPStatus >= 200 && record.HTTPStatus < 300 && !record.ProtocolValid && record.Disposition != "agent_context_failure" {
			return nil, nil, fmt.Errorf("provider transport round %d has an incomplete 2xx protocol receipt", record.Round)
		}
		outcome := "success"
		if record.Disposition == "prewarm_transport" {
			if record.ProviderAttemptKind != "prewarm" || !record.ProtocolValid || record.ErrorCode != "" {
				return nil, nil, fmt.Errorf("provider transport round %d has invalid prewarm evidence", record.Round)
			}
			outcome = "prewarm"
		} else if record.Disposition == "agent_context_failure" {
			if record.ProtocolValid || record.ProviderAttemptKind != "inference" || record.ErrorCode != "provider_context_failure" ||
				record.ResponseFailureCode != "context_length_exceeded" || !lowerHexSHA256(record.ResponseFailureEventSHA256) ||
				record.ResponseCompleted || (record.ResponseStatus != "" && record.ResponseStatus != "failed") {
				return nil, nil, fmt.Errorf("provider transport round %d has invalid context-failure evidence", record.Round)
			}
			outcome = "error"
		} else if record.ResponseFailureCode != "" || record.ResponseFailureEventSHA256 != "" {
			return nil, nil, fmt.Errorf("provider transport round %d has response-failure evidence under the wrong disposition", record.Round)
		} else if !record.ProtocolValid || record.ErrorCode != "" {
			if record.Disposition != "provider_infra_exclusion" || !admissibleProviderError(record.ErrorCode) {
				return nil, nil, fmt.Errorf("provider transport round %d is invalid: %s", record.Round, record.ErrorCode)
			}
			outcome = "error"
		}
		if (outcome == "success" || outcome == "prewarm") && ((record.ResponseCreatedModel != "" && record.ResponseCreatedModel != agent.Model.Model) || record.ResponseModel != agent.Model.Model) {
			return nil, nil, fmt.Errorf("provider transport round %d returned created/completed model %q/%q", record.Round, record.ResponseCreatedModel, record.ResponseModel)
		}
		reasoningModeCanonical := record.RequestedReasoningModeCanonical
		if record.RequestedReasoningMode == "" && reasoningModeCanonical == "" {
			reasoningModeCanonical = "standard"
		}
		canonicalizationProof := record.ClientCanonicalizationStaticProofSHA256
		if canonicalizationBindingSHA != "" {
			canonicalizationProof = canonicalizationBindingSHA
		}
		round := harness.ProviderRoundEvidence{
			SchemaVersion: "agentic-bench/provider-round-v2", EvidenceSequence: record.EvidenceSequence,
			PreviousEvidenceHash: record.PreviousEvidenceHash, EvidenceHash: record.EvidenceHash,
			Round: record.Round, RunIdentity: record.RunIdentity, ProviderAttemptStarted: record.ProviderAttemptStarted,
			Transport: record.Transport, ProviderAttemptKind: record.ProviderAttemptKind,
			WebSocketConnectionHash: record.WebSocketConnectionHash, WebSocketRequestSequence: record.WebSocketRequestSequence,
			WebSocketConnectionReused: record.WebSocketConnectionReused, WebSocketHandshakeStatus: record.WebSocketHandshakeStatus,
			WebSocketHandshakeModel: record.WebSocketHandshakeModel, WebSocketChainBound: record.WebSocketChainBound,
			GenerateSpecified: record.GenerateSpecified, Generate: record.Generate,
			TransportDisposition: record.Disposition, Outcome: outcome, ErrorCode: record.ErrorCode,
			RequestID: record.UpstreamRequestIDHash, ResponseIDHash: record.ResponseIDHash,
			StartedAt: record.StartedAt, UpstreamHeadersAt: record.UpstreamHeadersAt,
			FirstResponseByteAt: record.FirstResponseByteAt, FinishedAt: record.FinishedAt,
			Provider: agent.Model.Provider, Model: record.RequestedModel, ReasoningEffort: record.RequestedReasoningEffort,
			RequestedReasoningContext: record.RequestedReasoningContext,
			RequestedReasoningMode:    record.RequestedReasoningMode, RequestedReasoningModeCanonical: reasoningModeCanonical,
			RequestedTextVerbosity:   record.RequestedTextVerbosity,
			MaxOutputTokensSpecified: record.MaxOutputTokensSpecified, MaxOutputTokens: record.MaxOutputTokens,
			StoreSpecified: record.StoreSpecified, Store: record.Store,
			PreviousResponseIDPresent: record.PreviousResponseIDPresent, PreviousResponseIDHash: record.PreviousResponseIDHash,
			PromptCacheKeyPresent: record.PromptCacheKeyPresent, PromptCacheKeyHash: record.PromptCacheKeyHash,
			CachePolicyObserved: record.CachePolicyObserved, PromptCacheOptionsPresent: record.PromptCacheOptionsPresent,
			PromptCacheOptionsMode: record.PromptCacheOptionsMode, PromptCacheTTLSeconds: record.PromptCacheTTLSeconds,
			PromptCacheRetentionPresent: record.PromptCacheRetentionPresent, PromptCacheRetention: record.PromptCacheRetention,
			CacheBreakpointCount:          record.CacheBreakpointCount,
			CacheBreakpointPositionHashes: append([]string(nil), record.CacheBreakpointPositionHashes...),
			EncryptedReasoningRequested:   record.EncryptedReasoningRequested, EncryptedReasoningItemCount: record.EncryptedReasoningItemCount,
			EncryptedReasoningHashes:       append([]string(nil), record.EncryptedReasoningHashes...),
			EncryptedReasoningReplayCount:  record.EncryptedReasoningReplayCount,
			EncryptedReasoningReplayHashes: append([]string(nil), record.EncryptedReasoningReplayHashes...),
			EncryptedReasoningReplayBound:  record.EncryptedReasoningReplayBound,
			ReplayOutputItemCount:          record.ReplayOutputItemCount, ReplayOutputItemHashes: append([]string(nil), record.ReplayOutputItemHashes...),
			ReplayOutputItemsBound:  record.ReplayOutputItemsBound,
			ResponseOutputItemCount: record.ResponseOutputItemCount, ResponseOutputItemHashes: append([]string(nil), record.ResponseOutputItemHashes...),
			ContinuationLineagePresent: record.ContinuationLineagePresent, ContinuationLineageHash: record.ContinuationLineageHash,
			ContinuationEpoch: record.ContinuationEpoch, ContinuationReset: record.ContinuationReset,
			ContinuationResetAccepted: record.ContinuationResetAccepted, ContinuationLineageSource: record.ContinuationLineageSource,
			ContinuationResetUnknown: record.ContinuationResetUnknown,
			RequestedServiceTierRaw:  record.RequestedServiceTier, RequestedServiceTierPresent: record.RequestedServiceTierPresent,
			RequestedServiceTierCanonical:      record.RequestedServiceTierCanonical,
			RequestedServiceTierRepresentation: record.RequestedServiceTierRepresentation,
			ClientCanonicalizationProofSHA256:  canonicalizationProof, ClientAgentID: record.ClientAgentID,
			OriginalRequestBodySHA256: record.OriginalRequestBodySHA256, ForwardedRequestBodySHA256: record.ForwardedRequestBodySHA256,
			OriginalRequestCanonicalSHA256: record.OriginalRequestCanonicalSHA256, ForwardedRequestCanonicalSHA256: record.ForwardedRequestCanonicalSHA256,
			OriginalRequestWithoutServiceTierSHA256:  record.OriginalRequestWithoutServiceTierSHA256,
			ForwardedRequestWithoutServiceTierSHA256: record.ForwardedRequestWithoutServiceTierSHA256,
			OriginalServiceTierPresent:               record.OriginalServiceTierPresent, OriginalServiceTier: record.OriginalServiceTier,
			ForwardedServiceTierPresent: record.ForwardedServiceTierPresent, ForwardedServiceTier: record.ForwardedServiceTier,
			ForwardedRequestBytes: record.ForwardedRequestBytes, ServiceTierTransformation: record.ServiceTierTransformation,
			ServiceTierTransformationExactDiff:   record.ServiceTierTransformationExactDiff,
			ServiceTierTransformationProofSHA256: record.ServiceTierTransformationProofSHA256,
			ResponseServiceTierRaw:               record.ResponseServiceTier, ResponseServiceTierCanonical: record.ResponseServiceTierCanonical,
			ServiceTierComparable: record.ServiceTierComparable,
			ToolDefinitionCount:   record.ToolDefinitionCount, ToolCatalogHash: record.ToolCatalogHash,
			ToolCatalogSemanticSHA256: record.ToolCatalogSemanticSHA256, ToolCatalogCanonicalBytes: record.ToolCatalogCanonicalBytes,
			ToolCatalogCompared: record.ToolCatalogCompared, ToolCatalogStable: record.ToolCatalogStable,
			ToolResultHistoryValid: record.ToolResultHistoryValid,
			ResponseCreatedModel:   record.ResponseCreatedModel, ResponseModel: record.ResponseModel,
			ResponseCompleted: record.ResponseCompleted, ResponseStatus: record.ResponseStatus,
			ResponseFailureCode: record.ResponseFailureCode, ResponseFailureEventSHA256: record.ResponseFailureEventSHA256,
			HTTPStatus: record.HTTPStatus, RequestBytes: record.RequestBytes, ResponseBytes: record.ResponseBytes,
			UsagePresent: record.UsagePresent, InputTokens: record.InputTokens, CachedInputTokens: record.CachedInputTokens,
			CacheWriteInputTokens: record.CacheWriteInputTokens, OutputTokens: record.OutputTokens,
			ReasoningOutputTokens: record.ReasoningOutputTokens,
		}
		for _, definition := range record.ToolDefinitions {
			round.ToolDefinitions = append(round.ToolDefinitions, harness.ToolDefinitionEvidence{
				Type: definition.Type, Name: definition.Name, BillingOwner: definition.BillingOwner, Strict: definition.Strict,
				SchemaHash: definition.SchemaHash, SchemaSHA256: definition.SchemaSHA256, SchemaBytes: definition.SchemaBytes,
				DescriptionSHA256: definition.DescriptionSHA256, DescriptionBytes: definition.DescriptionBytes,
				DefinitionSHA256: definition.DefinitionSHA256, DefinitionBytes: definition.DefinitionBytes,
			})
		}
		for _, result := range record.ToolResults {
			round.ToolResultPayloadBytes += result.OutputBytes
			receipt := providerToolResultReceipt{Kind: result.Kind, PayloadHash: result.PayloadHash, OutputBytes: result.OutputBytes}
			if result.IDHash == "" || receipt.Kind == "" || !lowerHexSHA256(receipt.PayloadHash) || receipt.OutputBytes < 0 {
				return nil, nil, fmt.Errorf("provider-visible result in round %d is invalid", record.Round)
			}
			if previous, exists := resultReceipts[result.IDHash]; exists && previous != receipt {
				return nil, nil, fmt.Errorf("provider-visible result %s changed across requests", result.IDHash)
			}
			resultReceipts[result.IDHash] = receipt
		}
		for _, call := range record.ToolCalls {
			round.ToolCalls = append(round.ToolCalls, harness.ToolCallEvidence{ID: call.IDHash, Kind: call.Kind, Name: call.Name, InputBytes: call.InputBytes})
		}
		rounds = append(rounds, round)
	}
	for ordinal := 0; ordinal < len(records); ordinal++ {
		if _, present := startOrdinals[ordinal]; !present {
			return nil, nil, fmt.Errorf("provider transport start-order view is missing round %d", ordinal)
		}
	}
	slices.SortFunc(rounds, func(left, right harness.ProviderRoundEvidence) int {
		if left.EvidenceSequence < right.EvidenceSequence {
			return -1
		}
		if left.EvidenceSequence > right.EvidenceSequence {
			return 1
		}
		return 0
	})
	return rounds, resultReceipts, nil
}
