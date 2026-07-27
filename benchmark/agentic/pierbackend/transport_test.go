package pierbackend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const fixtureRunIdentity = "1111111111111111111111111111111111111111111111111111111111111111"

func TestNormalizeCodexAndLubanToolEvidenceParityWithoutContent(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		directory := t.TempDir()
		rawPath := filepath.Join(directory, "provider.jsonl")
		streamPath := filepath.Join(directory, "codex.jsonl")
		outputPath := filepath.Join(directory, "normalized.jsonl")
		calls := []evidenceproxy.ToolCall{
			{IDHash: hashID("call-shell"), Kind: "custom_tool_call", Name: "exec", InputBytes: 17},
			{IDHash: hashID("call-patch"), Kind: "custom_tool_call", Name: "exec", InputBytes: 29},
		}
		records := providerFixtureRecords("codex", calls, []evidenceproxy.ToolResult{
			{IDHash: hashID("call-shell"), OutputBytes: 23},
			{IDHash: hashID("call-patch"), OutputBytes: 31},
		})
		writeJSONLines(t, rawPath, records)
		const commandSecret = "SECRET command output"
		const pathSecret = "/private/SECRET/file.go"
		stream := strings.Join([]string{
			`{"type":"item.completed","item":{"id":"call-shell","type":"command_execution","aggregated_output":"` + commandSecret + `","exit_code":1,"status":"failed"}}`,
			`{"type":"item.completed","item":{"id":"item_1","type":"file_change","changes":[{"path":"` + pathSecret + `","kind":"update"}],"status":"completed"}}`,
		}, "\n") + "\n"
		if err := os.WriteFile(streamPath, []byte(stream), 0o600); err != nil {
			t.Fatal(err)
		}
		agent := fixtureAgent("codex")
		if err := normalizeProviderEvidenceUnsealed(rawPath, streamPath, outputPath, agent, fixtureRunIdentity); err != nil {
			t.Fatal(err)
		}
		rounds, err := harness.ReadJSONLines[harness.ProviderRoundEvidence](outputPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(rounds) != 2 || rounds[0].Round != 1 || rounds[1].Round != 0 || len(rounds[1].ToolCalls) != 2 {
			t.Fatalf("normalized rounds = %#v", rounds)
		}
		if rounds[1].RunIdentity != fixtureRunIdentity || rounds[1].CacheWriteInputTokens == nil || *rounds[1].CacheWriteInputTokens != 0 || rounds[1].RequestedReasoningContext != "all_turns" || !rounds[1].ResponseCompleted {
			t.Fatalf("v5 request identity/usage/commit evidence = %#v", rounds[1])
		}
		shell, patch := rounds[1].ToolCalls[0], rounds[1].ToolCalls[1]
		if shell.Error == nil || !*shell.Error || shell.DurationMS != nil || shell.OutputBytes == nil || *shell.OutputBytes != 23 || shell.AgentTraceOutputBytes == nil || shell.TraceMatch != "id" {
			t.Fatalf("Codex command evidence = %#v", shell)
		}
		if patch.Error == nil || *patch.Error || patch.DurationMS != nil || patch.OutputBytes == nil || *patch.OutputBytes != 31 || patch.AgentTraceOutputBytes == nil || patch.TraceMatch != "ordered_kind" {
			t.Fatalf("Codex file-change evidence = %#v", patch)
		}
		assertNormalizedContentFree(t, outputPath, commandSecret, pathSecret)
		assertCoverage(t, rounds, 2, 0, 2, 1, 1)
	})

	t.Run("luban", func(t *testing.T) {
		directory := t.TempDir()
		rawPath := filepath.Join(directory, "provider.jsonl")
		streamPath := filepath.Join(directory, "luban.jsonl")
		outputPath := filepath.Join(directory, "normalized.jsonl")
		calls := []evidenceproxy.ToolCall{{IDHash: hashID("call-run"), Name: "Run", InputBytes: 41}}
		records := providerFixtureRecords("luban", calls, []evidenceproxy.ToolResult{{IDHash: hashID("call-run"), OutputBytes: 47}})
		writeJSONLines(t, rawPath, records)
		const secret = "SECRET tool result must not be copied"
		stream := strings.Join([]string{
			`{"type":"tool_result","tool_use_id":"call-run","is_error":false,"content_ref":{"digest":"` + hashID(secret) + `"},"metrics":{"content_bytes":37}}`,
			`{"type":"agentic_metrics","metric":"tool_round","tool_round":{"logical_model_visible_calls":1,"physical_child_operations":3,"critical_path_ms":19,"total_child_latency_ms":41,"queue_ms":2}}`,
		}, "\n") + "\n"
		if err := os.WriteFile(streamPath, []byte(stream), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := normalizeProviderEvidenceUnsealed(rawPath, streamPath, outputPath, fixtureAgent("luban"), fixtureRunIdentity); err != nil {
			t.Fatal(err)
		}
		rounds, err := harness.ReadJSONLines[harness.ProviderRoundEvidence](outputPath)
		if err != nil {
			t.Fatal(err)
		}
		if rounds[0].Round != 1 || rounds[1].Round != 0 {
			t.Fatalf("normalized completion order = %#v", rounds)
		}
		call := rounds[1].ToolCalls[0]
		if call.Error == nil || *call.Error || call.DurationMS != nil || call.OutputBytes == nil || *call.OutputBytes != 47 || call.AgentTraceOutputBytes == nil || *call.AgentTraceOutputBytes != 37 || call.TraceMatch != "id" {
			t.Fatalf("Luban tool evidence = %#v", call)
		}
		if rounds[1].PhysicalToolOperations == nil || *rounds[1].PhysicalToolOperations != 3 ||
			rounds[1].ToolCriticalPathMS == nil || *rounds[1].ToolCriticalPathMS != 19 ||
			rounds[1].ToolTotalLatencyMS == nil || *rounds[1].ToolTotalLatencyMS != 41 ||
			rounds[1].ToolQueueMS == nil || *rounds[1].ToolQueueMS != 2 {
			t.Fatalf("Luban physical scheduling evidence = %#v", rounds[1])
		}
		assertNormalizedContentFree(t, outputPath, secret)
		assertCoverage(t, rounds, 1, 0, 1, 1, 0)
	})
}

func TestCorrelateCodexExecUsesContentFreeOrderedPolymorphicBinding(t *testing.T) {
	shellID := hashID("provider-shell")
	patchID := hashID("provider-patch")
	rounds := []harness.ProviderRoundEvidence{{ToolCalls: []harness.ToolCallEvidence{
		{ID: shellID, Kind: "custom_tool_call", Name: "exec"},
		{ID: patchID, Kind: "custom_tool_call", Name: "exec"},
	}}}
	results := map[string]providerToolResultReceipt{
		shellID: {Kind: "custom_tool_call_output", OutputBytes: 11},
		patchID: {Kind: "custom_tool_call_output", OutputBytes: 13},
	}
	trace := parsedTrace{byID: map[string]int{}}
	trace.addTool(tracedTool{IDHash: hashID("item_0"), Kind: "command_execution", Error: boolPointer(false)})
	trace.addTool(tracedTool{IDHash: hashID("item_1"), Kind: "file_change", Error: boolPointer(false)})

	if err := correlateToolEvidence(rounds, results, trace); err != nil {
		t.Fatal(err)
	}
	shell, patch := rounds[0].ToolCalls[0], rounds[0].ToolCalls[1]
	if shell.TraceMatch != "ordered_kind" || shell.TraceKind != "command_execution" || shell.OutputBytes == nil || *shell.OutputBytes != 11 {
		t.Fatalf("ordered command binding = %#v", shell)
	}
	if patch.TraceMatch != "ordered_kind" || patch.TraceKind != "file_change" || patch.OutputBytes == nil || *patch.OutputBytes != 13 {
		t.Fatalf("ordered file-change binding = %#v", patch)
	}
}

func TestCorrelateCodexExecFailsClosedOnAmbiguousResidualCardinality(t *testing.T) {
	firstID := hashID("provider-first")
	secondID := hashID("provider-second")
	rounds := []harness.ProviderRoundEvidence{{ToolCalls: []harness.ToolCallEvidence{
		{ID: firstID, Kind: "custom_tool_call", Name: "exec"},
		{ID: secondID, Kind: "custom_tool_call", Name: "exec"},
	}}}
	results := map[string]providerToolResultReceipt{
		firstID:  {Kind: "custom_tool_call_output", OutputBytes: 17},
		secondID: {Kind: "custom_tool_call_output", OutputBytes: 19},
	}
	trace := parsedTrace{byID: map[string]int{}}
	trace.addTool(tracedTool{IDHash: hashID("item_0"), Kind: "file_change", Error: boolPointer(false)})

	if err := correlateToolEvidence(rounds, results, trace); err != nil {
		t.Fatal(err)
	}
	for index, call := range rounds[0].ToolCalls {
		if call.TraceMatch != "" || call.TraceKind != "" || call.Error != nil || call.AgentTraceOutputBytes != nil {
			t.Fatalf("ambiguous call %d was attributed: %#v", index, call)
		}
	}
}

func TestCorrelateCodexExecFailsClosedWhenOrderedFallbackCrossesIDAnchor(t *testing.T) {
	anchoredID := hashID("provider-anchored")
	residualID := hashID("provider-residual")
	rounds := []harness.ProviderRoundEvidence{{ToolCalls: []harness.ToolCallEvidence{
		{ID: anchoredID, Kind: "custom_tool_call", Name: "exec"},
		{ID: residualID, Kind: "custom_tool_call", Name: "exec"},
	}}}
	results := map[string]providerToolResultReceipt{
		anchoredID: {Kind: "custom_tool_call_output", OutputBytes: 23},
		residualID: {Kind: "custom_tool_call_output", OutputBytes: 29},
	}
	trace := parsedTrace{byID: map[string]int{}}
	trace.addTool(tracedTool{IDHash: hashID("item_0"), Kind: "file_change", Error: boolPointer(false)})
	trace.addTool(tracedTool{IDHash: anchoredID, Kind: "command_execution", Error: boolPointer(false)})

	if err := correlateToolEvidence(rounds, results, trace); err != nil {
		t.Fatal(err)
	}
	if rounds[0].ToolCalls[0].TraceMatch != "id" || rounds[0].ToolCalls[0].TraceKind != "command_execution" {
		t.Fatalf("exact ID anchor was not retained: %#v", rounds[0].ToolCalls[0])
	}
	if call := rounds[0].ToolCalls[1]; call.TraceMatch != "" || call.TraceKind != "" || call.Error != nil || call.AgentTraceOutputBytes != nil {
		t.Fatalf("order-conflicting residual was attributed: %#v", call)
	}
}

func TestCorrelateCodexExecRejectsIncompatibleExactTraceKind(t *testing.T) {
	callID := hashID("provider-exec")
	rounds := []harness.ProviderRoundEvidence{{ToolCalls: []harness.ToolCallEvidence{{
		ID: callID, Kind: "custom_tool_call", Name: "exec",
	}}}}
	results := map[string]providerToolResultReceipt{
		callID: {Kind: "custom_tool_call_output", OutputBytes: 31},
	}
	trace := parsedTrace{byID: map[string]int{}}
	trace.addTool(tracedTool{IDHash: callID, Kind: "mcp_tool_call", Error: boolPointer(false)})

	if err := correlateToolEvidence(rounds, results, trace); err == nil || !strings.Contains(err.Error(), "incompatible agent trace kind") {
		t.Fatalf("incompatible exact trace was accepted: %v", err)
	}
}

func TestCorrelateCodexExecRequiresProviderVisibleResultForOrderedBinding(t *testing.T) {
	callID := hashID("provider-aborted")
	rounds := []harness.ProviderRoundEvidence{{ToolCalls: []harness.ToolCallEvidence{{
		ID: callID, Kind: "custom_tool_call", Name: "exec",
	}}}}
	trace := parsedTrace{byID: map[string]int{}}
	trace.addTool(tracedTool{IDHash: hashID("item_0"), Kind: "command_execution", Error: boolPointer(false)})

	if err := correlateToolEvidence(rounds, nil, trace); err != nil {
		t.Fatal(err)
	}
	if call := rounds[0].ToolCalls[0]; call.TraceMatch != "" || call.TraceKind != "" || call.Error != nil {
		t.Fatalf("proposal without provider result consumed a later trace: %#v", call)
	}
}

func TestClassifyTrialInfrastructureUsesStructuredTerminalEvidenceOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.jsonl")
	writeJSONLines(t, path, []evidenceproxy.Record{{
		SchemaVersion: "agentic-bench/provider-http-v6", Round: 0, RunIdentity: fixtureRunIdentity, HTTPStatus: 400,
		ErrorCode: "provider_http_error",
	}})
	if category, ok := classifyTrialInfrastructure(path, sanitizedTrialResult{ExceptionType: "logs mention throttle and 429"}); ok || category != "" {
		t.Fatalf("non-retryable HTTP 400 was excluded from scoring: %q %v", category, ok)
	}

	path = filepath.Join(t.TempDir(), "provider.jsonl")
	writeJSONLines(t, path, []evidenceproxy.Record{{
		SchemaVersion: "agentic-bench/provider-http-v6", Round: 0, RunIdentity: fixtureRunIdentity, HTTPStatus: 429,
		ErrorCode: "provider_http_error",
	}})
	if category, ok := classifyTrialInfrastructure(path, sanitizedTrialResult{}); !ok || category != harness.DeepSWEFailureProviderInfrastructure {
		t.Fatalf("structured retryable provider failure = %q %v", category, ok)
	}

	if category, ok := classifyTrialInfrastructure(filepath.Join(t.TempDir(), "absent"), sanitizedTrialResult{VerifierStarted: time.Now()}); !ok || category != harness.DeepSWEFailureVerifierInfrastructure {
		t.Fatalf("started no-reward verifier = %q %v", category, ok)
	}
}

func TestDeriveTerminalEvidenceUsesStructuredCodexAndLubanCodes(t *testing.T) {
	tests := []struct {
		name      string
		agentKind string
		event     string
	}{
		{
			name:      "codex turn failure",
			agentKind: "codex",
			event:     `{"type":"turn.failed","error":{"code":"context_length_exceeded","message":"not authority"}}`,
		},
		{
			name:      "luban semantic runtime failure",
			agentKind: "luban",
			event:     `{"type":"error","schema_version":"runtime-event/v2","kind":"runtime_error","outcome":"failed","code":"context_length_exceeded","message":"redacted"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exitReceipt := []byte("exit receipt\n")
			evidence, err := deriveTerminalEvidence(test.agentKind, []byte(test.event+"\n"), exitReceipt, 1)
			if err != nil {
				t.Fatal(err)
			}
			if evidence.SchemaVersion != "agentic-bench/terminal-evidence-v1" || evidence.Source != "provider_event" || evidence.Code != "context_length_exceeded" || evidence.EvidenceSHA256 != sha256Hex([]byte(test.event)) {
				t.Fatalf("terminal evidence = %#v", evidence)
			}
		})
	}
}

func TestDeriveTerminalEvidenceRejectsUnstructuredUnknownAndConflictingEvents(t *testing.T) {
	for name, stream := range map[string]string{
		"message only": `{"type":"turn.failed","error":{"message":"context_length_exceeded"}}` + "\n",
		"unknown code": `{"type":"error","error":{"code":"future_provider_failure"}}` + "\n",
		"untyped":      `{"message":"context_length_exceeded"}` + "\n",
		"duplicate":    `{"type":"text","type":"turn.completed"}` + "\n",
		"non-finite":   `{"type":"text","value":NaN}` + "\n",
		"bare CR":      `{"type":"text"}\r{"type":"text"}` + "\n",
		"malformed":    "not-json\n",
		"conflicting":  `{"type":"turn.failed","error":{"code":"context_length_exceeded"}}` + "\n" + `{"type":"turn.completed","usage":{}}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveTerminalEvidence("codex", []byte(stream), []byte("exit\n"), 1); err == nil {
				t.Fatal("untrusted terminal stream was accepted")
			}
		})
	}
}

func TestReadAndValidateTerminalEvidenceBindsAdapterReceiptToRawEvent(t *testing.T) {
	directory := t.TempDir()
	streamPath := filepath.Join(directory, "stream.jsonl")
	receiptPath := filepath.Join(directory, "terminal-evidence.json")
	event := []byte(`{"type":"response.failed","response":{"error":{"code":"context_length_exceeded"}}}`)
	if err := os.WriteFile(streamPath, append(append([]byte(nil), event...), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt := harness.AgentTerminalEvidence{
		SchemaVersion: "agentic-bench/terminal-evidence-v1", Source: "provider_event",
		Code: "context_length_exceeded", EvidenceSHA256: sha256Hex(event),
	}
	if err := harness.WriteJSONAtomic(receiptPath, receipt, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readAndValidateTerminalEvidence(receiptPath, streamPath, []byte("exit\n"), 1, "luban")
	if err != nil {
		t.Fatal(err)
	}
	if got != receipt {
		t.Fatalf("terminal evidence = %#v, want %#v", got, receipt)
	}

	receipt.EvidenceSHA256 = strings.Repeat("f", 64)
	if err := harness.WriteJSONAtomic(receiptPath, receipt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readAndValidateTerminalEvidence(receiptPath, streamPath, []byte("exit\n"), 1, "luban"); err == nil {
		t.Fatal("forged adapter terminal digest was accepted")
	}
}

func TestDeriveProviderContextTerminalRequiresLastSealedInferenceRound(t *testing.T) {
	digest := strings.Repeat("d", 64)
	records := func() []evidenceproxy.Record {
		return []evidenceproxy.Record{
			{
				SchemaVersion: "agentic-bench/provider-http-v6", Round: 1, RunIdentity: fixtureRunIdentity,
				ProviderAttemptKind: "inference", ProviderAttemptStarted: true,
				Disposition: "agent_context_failure", ErrorCode: "provider_context_failure",
				ResponseStatus: "failed", ResponseFailureCode: "context_length_exceeded", ResponseFailureEventSHA256: digest,
			},
			{
				SchemaVersion: "agentic-bench/provider-http-v6", Round: 0, RunIdentity: fixtureRunIdentity,
				ProviderAttemptKind: "prewarm", ProviderAttemptStarted: true, Disposition: "prewarm_transport", ProtocolValid: true,
			},
		}
	}

	evidence, found, err := deriveProviderContextTerminal(records(), fixtureRunIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if !found || evidence.Source != "provider_event" || evidence.Code != "context_length_exceeded" || evidence.EvidenceSHA256 != digest {
		t.Fatalf("provider terminal evidence = %#v, found=%v", evidence, found)
	}

	withLaterPrewarm := append(records(), evidenceproxy.Record{
		SchemaVersion: "agentic-bench/provider-http-v6", Round: 2, RunIdentity: fixtureRunIdentity,
		ProviderAttemptKind: "prewarm", ProviderAttemptStarted: true, Disposition: "prewarm_transport", ProtocolValid: true,
	})
	if _, found, err := deriveProviderContextTerminal(withLaterPrewarm, fixtureRunIdentity); err != nil || !found {
		t.Fatalf("later transport-only prewarm obscured the terminal inference: found=%v err=%v", found, err)
	}

	withLaterSuccess := append(records(), evidenceproxy.Record{
		SchemaVersion: "agentic-bench/provider-http-v6", Round: 2, RunIdentity: fixtureRunIdentity,
		ProviderAttemptKind: "inference", ProviderAttemptStarted: true, Disposition: "valid", ProtocolValid: true,
	})
	if evidence, found, err := deriveProviderContextTerminal(withLaterSuccess, fixtureRunIdentity); err != nil || found {
		t.Fatalf("nonterminal context failure was accepted: evidence=%#v found=%v err=%v", evidence, found, err)
	}

	for name, mutate := range map[string]func([]evidenceproxy.Record){
		"other run": func(values []evidenceproxy.Record) { values[0].RunIdentity = strings.Repeat("e", 64) },
		"unknown code": func(values []evidenceproxy.Record) {
			values[0].ResponseFailureCode = "future_failure"
		},
		"missing digest": func(values []evidenceproxy.Record) { values[0].ResponseFailureEventSHA256 = "" },
		"wrong disposition": func(values []evidenceproxy.Record) {
			values[0].Disposition = "provider_infra_exclusion"
		},
		"not started":        func(values []evidenceproxy.Record) { values[0].ProviderAttemptStarted = false },
		"completed response": func(values []evidenceproxy.Record) { values[0].ResponseCompleted = true },
		"unknown attempt kind": func(values []evidenceproxy.Record) {
			values[1].ProviderAttemptKind = "future_attempt"
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := records()
			mutate(invalid)
			if _, _, err := deriveProviderContextTerminal(invalid, fixtureRunIdentity); err == nil {
				t.Fatal("invalid provider context terminal evidence was accepted")
			}
		})
	}
}

func TestValidateCodexProxyContextStreamNeverClassifiesMessageText(t *testing.T) {
	for name, message := range map[string]string{
		"context prose":   "context_length_exceeded",
		"unrelated prose": "the provider stopped without a public structured code",
		"empty prose":     "",
	} {
		t.Run(name, func(t *testing.T) {
			encodedMessage, err := json.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			stream := `{"type":"turn.failed","error":{"message":` + string(encodedMessage) + `}}` + "\n"
			if err := validateCodexProxyContextStream([]byte(stream), 1); err != nil {
				t.Fatalf("sealed proxy context was made dependent on message prose: %v", err)
			}
		})
	}

	for name, stream := range map[string]string{
		"unknown structured code": `{"type":"turn.failed","error":{"code":"future_failure","message":"context_length_exceeded"}}` + "\n",
		"conflicting completion":  `{"type":"turn.failed","error":{"message":"failed"}}` + "\n" + `{"type":"turn.completed","usage":{}}` + "\n",
		"two failed turns":        `{"type":"turn.failed","error":{"message":"first"}}` + "\n" + `{"type":"turn.failed","error":{"message":"second"}}` + "\n",
		"untyped":                 `{"message":"context_length_exceeded"}` + "\n",
		"malformed":               "not-json\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateCodexProxyContextStream([]byte(stream), 1); err == nil {
				t.Fatal("ambiguous Codex terminal stream was accepted")
			}
		})
	}
	if err := validateCodexProxyContextStream([]byte(`{"type":"turn.failed","error":{"message":"failed"}}`+"\n"), 0); err == nil {
		t.Fatal("zero Codex exit was accepted as a failed terminal turn")
	}
}

func TestNormalizeProviderEvidenceCarriesContentFreeContextFailureReceipt(t *testing.T) {
	directory := t.TempDir()
	rawPath := filepath.Join(directory, "provider.jsonl")
	streamPath := filepath.Join(directory, "codex.jsonl")
	outputPath := filepath.Join(directory, "normalized.jsonl")
	records := providerFixtureRecords("codex", nil, nil)
	digest := strings.Repeat("d", 64)
	for index := range records {
		if records[index].Round != 1 {
			continue
		}
		records[index].ProtocolValid = false
		records[index].Disposition = "agent_context_failure"
		records[index].ErrorCode = "provider_context_failure"
		records[index].ResponseCompleted = false
		records[index].ResponseStatus = "failed"
		records[index].ResponseFailureCode = "context_length_exceeded"
		records[index].ResponseFailureEventSHA256 = digest
	}
	writeJSONLines(t, rawPath, records)
	if err := os.WriteFile(streamPath, []byte(`{"type":"turn.failed","error":{"message":"not classification authority"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := normalizeProviderEvidenceUnsealed(rawPath, streamPath, outputPath, fixtureAgent("codex"), fixtureRunIdentity); err != nil {
		t.Fatal(err)
	}
	rounds, err := harness.ReadJSONLines[harness.ProviderRoundEvidence](outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 2 || rounds[0].Round != 1 || rounds[0].TransportDisposition != "agent_context_failure" || rounds[0].Outcome != "error" ||
		rounds[0].ResponseFailureCode != "context_length_exceeded" || rounds[0].ResponseFailureEventSHA256 != digest {
		t.Fatalf("normalized context receipt = %#v", rounds)
	}
	metrics, err := harness.ValidateAndAggregateEvidence(
		rounds,
		fixtureAgent("codex").Model,
		harness.PricingCatalog{UnitTokens: 1_000_000, Rates: []harness.PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ProviderRequests != 2 || metrics.ProviderRounds != 1 || metrics.ProviderErrors != 1 {
		t.Fatalf("normalized context metrics = %#v", metrics)
	}
}

func providerFixtureRecords(agentID string, calls []evidenceproxy.ToolCall, results []evidenceproxy.ToolResult) []evidenceproxy.Record {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	makeRecord := func(round int) evidenceproxy.Record {
		started := now.Add(time.Duration(round*2) * time.Second)
		return evidenceproxy.Record{
			SchemaVersion: "agentic-bench/provider-http-v6", EvidenceSequence: uint64(round), EvidenceHash: strings.Repeat(string(rune('1'+round)), 64), Round: round, RunIdentity: fixtureRunIdentity, ProviderAttemptStarted: true,
			Transport: "http_sse", ProviderAttemptKind: "inference", WebSocketChainBound: true,
			StartedAt: started, UpstreamHeadersAt: started.Add(10 * time.Millisecond), FirstResponseByteAt: started.Add(20 * time.Millisecond), FinishedAt: started.Add(30 * time.Millisecond),
			Method: "POST", Path: "/v1/responses", RequestBytes: 100, ResponseBytes: 100,
			RequestedModel: "gpt-5.6-sol", RequestedReasoningEffort: "xhigh", RequestedReasoningContext: "all_turns", RequestedReasoningModeCanonical: "standard", RequestedServiceTierCanonical: "default", ClientAgentID: agentID, StoreSpecified: true,
			EncryptedReasoningRequested: true,
			ContinuationLineagePresent:  true, ContinuationLineageHash: strings.Repeat("e", 64), ContinuationLineageSource: "agent_header", ContinuationEpoch: 1,
			ToolCatalogHash: strings.Repeat("9", 64), ToolCatalogSemanticSHA256: "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945", ToolCatalogStable: true, ToolResultHistoryValid: true,
			HTTPStatus: 200, UpstreamRequestIDHash: strings.Repeat(string(rune('a'+round*2)), 64), ResponseIDHash: strings.Repeat(string(rune('b'+round*2)), 64),
			ResponseModel: "gpt-5.6-sol", ResponseServiceTier: "default", ResponseServiceTierCanonical: "default", ServiceTierComparable: true, ResponseCompleted: true, ResponseStatus: "completed",
			UsagePresent: true, InputTokens: int64Pointer(100), CachedInputTokens: int64Pointer(80), CacheWriteInputTokens: int64Pointer(0), OutputTokens: int64Pointer(10), ProtocolValid: true,
			Disposition: "valid",
		}
	}
	first, second := makeRecord(0), makeRecord(1)
	definitions, semanticSHA, canonicalBytes := fixtureProviderToolCatalog(agentID)
	for _, record := range []*evidenceproxy.Record{&first, &second} {
		record.ToolDefinitions = append([]evidenceproxy.ToolDefinitionEvidence(nil), definitions...)
		record.ToolDefinitionCount = len(definitions)
		record.ToolCatalogSemanticSHA256 = semanticSHA
		record.ToolCatalogCanonicalBytes = canonicalBytes
		if agentID == "codex" {
			record.RequestedServiceTierRepresentation = "client_canonicalized_default"
			record.ClientCanonicalizationStaticProofSHA256 = strings.Repeat("7", 64)
			record.OriginalRequestBodySHA256 = strings.Repeat("1", 64)
			record.ForwardedRequestBodySHA256 = strings.Repeat("2", 64)
			record.OriginalRequestCanonicalSHA256 = strings.Repeat("3", 64)
			record.ForwardedRequestCanonicalSHA256 = strings.Repeat("4", 64)
			record.OriginalRequestWithoutServiceTierSHA256 = strings.Repeat("5", 64)
			record.ForwardedRequestWithoutServiceTierSHA256 = record.OriginalRequestWithoutServiceTierSHA256
			record.ForwardedServiceTierPresent = true
			record.ForwardedServiceTier = "default"
			record.ForwardedRequestBytes = 120
			record.ServiceTierTransformation = "inject_explicit_default"
		} else {
			record.RequestedServiceTier = "default"
			record.RequestedServiceTierPresent = true
			record.RequestedServiceTierRepresentation = "explicit_default"
			record.OriginalRequestBodySHA256 = strings.Repeat("1", 64)
			record.ForwardedRequestBodySHA256 = record.OriginalRequestBodySHA256
			record.OriginalRequestCanonicalSHA256 = strings.Repeat("2", 64)
			record.ForwardedRequestCanonicalSHA256 = record.OriginalRequestCanonicalSHA256
			record.OriginalRequestWithoutServiceTierSHA256 = strings.Repeat("3", 64)
			record.ForwardedRequestWithoutServiceTierSHA256 = record.OriginalRequestWithoutServiceTierSHA256
			record.OriginalServiceTierPresent = true
			record.OriginalServiceTier = "default"
			record.ForwardedServiceTierPresent = true
			record.ForwardedServiceTier = "default"
			record.ForwardedRequestBytes = record.RequestBytes
			record.ServiceTierTransformation = "none"
		}
		record.ServiceTierTransformationExactDiff = true
		record.ServiceTierTransformationProofSHA256 = strings.Repeat("6", 64)
	}
	second.EvidenceSequence = 0
	second.PreviousEvidenceHash = ""
	first.EvidenceSequence = 1
	first.PreviousEvidenceHash = second.EvidenceHash
	for index := range calls {
		if calls[index].Kind == "" {
			calls[index].Kind = "function_call"
		}
	}
	for index := range results {
		if results[index].Kind == "" {
			results[index].Kind = "function_call_output"
			if agentID == "codex" {
				results[index].Kind = "custom_tool_call_output"
			}
		}
		if results[index].PayloadHash == "" {
			results[index].PayloadHash = strings.Repeat(string(rune('4'+index)), 64)
		}
	}
	first.ToolCalls = calls
	second.ToolResults = results
	// Evidence is appended when requests finish, so deliberately supply reverse
	// completion order. The normalizer must preserve this immutable hash-chain
	// order while correlating tools through a temporary start-order view.
	return []evidenceproxy.Record{second, first}
}

func fixtureProviderToolCatalog(agentID string) ([]evidenceproxy.ToolDefinitionEvidence, string, int64) {
	identities := [][2]string{{"function", "Inspect"}, {"function", "ApplyPatch"}, {"function", "Run"}}
	if agentID == "codex" {
		identities = [][2]string{{"custom", "exec"}, {"function", "wait"}, {"function", "request_user_input"}}
	}
	strict := true
	definitions := make([]evidenceproxy.ToolDefinitionEvidence, 0, len(identities))
	normalized := make([]harness.ToolDefinitionEvidence, 0, len(identities))
	canonicalBytes := int64(0)
	for index, identity := range identities {
		definition := evidenceproxy.ToolDefinitionEvidence{
			Type: identity[0], Name: identity[1], BillingOwner: "client",
			DefinitionSHA256: strings.Repeat(string(rune('a'+index)), 64), DefinitionBytes: int64(index + 1),
		}
		if identity[0] == "function" {
			definition.Strict = &strict
			definition.SchemaHash = strings.Repeat(string(rune('4'+index)), 64)
			definition.SchemaSHA256 = strings.Repeat(string(rune('7'+index)), 64)
			definition.SchemaBytes = int64(index + 1)
		}
		definitions = append(definitions, definition)
		normalized = append(normalized, harness.ToolDefinitionEvidence{
			Type: definition.Type, Name: definition.Name, BillingOwner: definition.BillingOwner, Strict: definition.Strict,
			SchemaHash: definition.SchemaHash, SchemaSHA256: definition.SchemaSHA256, SchemaBytes: definition.SchemaBytes,
			DescriptionSHA256: definition.DescriptionSHA256, DescriptionBytes: definition.DescriptionBytes,
			DefinitionSHA256: definition.DefinitionSHA256, DefinitionBytes: definition.DefinitionBytes,
		})
		canonicalBytes += definition.DefinitionBytes
	}
	return definitions, harness.StableToolCatalogSHA256(normalized), canonicalBytes
}

func fixtureAgent(id string) harness.AgentSpec {
	encoding := harness.ServiceTierEncodingExplicitDefault
	if id == "codex" {
		encoding = harness.ServiceTierEncodingClientCanonical
	}
	definitions, semanticSHA, _ := fixtureProviderToolCatalog(id)
	tools := make([]harness.ToolIdentitySpec, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, harness.ToolIdentitySpec{
			Type: definition.Type, Name: definition.Name, DefinitionSHA256: definition.DefinitionSHA256,
		})
	}
	return harness.AgentSpec{ID: id, Model: harness.ModelRequestSpec{
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: "default",
		ServiceTierRequestEncoding: encoding, TransportRequirement: harness.TransportRequirementHTTPInference,
		ToolCatalog: harness.ToolCatalogSpec{
			SchemaVersion: harness.FormalToolCatalogSchemaVersion, SemanticSHA256: semanticSHA, Tools: tools,
		},
	}}
}

func writeJSONLines[T any](t *testing.T, path string, values []T) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, value := range values {
		if err := encoder.Encode(value); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertNormalizedContentFree(t *testing.T, path string, forbidden ...string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range forbidden {
		if strings.Contains(string(raw), value) {
			t.Fatalf("normalized evidence leaked %q: %s", value, raw)
		}
	}
}

func assertCoverage(t *testing.T, rounds []harness.ProviderRoundEvidence, errors, durations, outputs, idMatches, orderedMatches int) {
	t.Helper()
	if len(rounds) == 0 {
		t.Fatal("normalized evidence is empty")
	}
	metrics, err := harness.ValidateAndAggregateEvidence(
		rounds,
		fixtureAgent(rounds[0].ClientAgentID).Model,
		harness.PricingCatalog{UnitTokens: 1_000_000, Rates: []harness.PricingRate{{Provider: "openai", Model: "gpt-5.6-sol"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.ToolErrorObservations != errors || metrics.ToolDurationObservations != durations || metrics.ToolOutputObservations != outputs || metrics.ToolTraceIDMatches != idMatches || metrics.ToolTraceOrderedMatches != orderedMatches {
		t.Fatalf("observability coverage = %#v", metrics)
	}
}
