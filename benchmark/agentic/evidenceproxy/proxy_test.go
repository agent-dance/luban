package evidenceproxy

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testRunIdentity = "1111111111111111111111111111111111111111111111111111111111111111"

func codexStaticProofConfig() Config {
	return Config{
		AgentID:                            "codex",
		RegisteredBinarySHA256:             strings.Repeat("1", 64),
		FrozenBundleManifestSHA256:         strings.Repeat("2", 64),
		FrozenBundleTreeSHA256:             strings.Repeat("3", 64),
		FrozenCanonicalCanaryReceiptSHA256: strings.Repeat("4", 64),
		AdapterSHA256:                      strings.Repeat("5", 64),
		AdapterVersion:                     "2.3.0",
		SourceCommandArgvSHA256:            strings.Repeat("6", 64),
	}
}

func configureCodexStaticProof(t *testing.T, config Config) Config {
	t.Helper()
	proofInputs := codexStaticProofConfig()
	config.AgentID = proofInputs.AgentID
	config.RegisteredBinarySHA256 = proofInputs.RegisteredBinarySHA256
	config.FrozenBundleManifestSHA256 = proofInputs.FrozenBundleManifestSHA256
	config.FrozenBundleTreeSHA256 = proofInputs.FrozenBundleTreeSHA256
	config.FrozenCanonicalCanaryReceiptSHA256 = proofInputs.FrozenCanonicalCanaryReceiptSHA256
	config.AdapterSHA256 = proofInputs.AdapterSHA256
	config.AdapterVersion = proofInputs.AdapterVersion
	config.SourceCommandArgvSHA256 = proofInputs.SourceCommandArgvSHA256
	proof, err := ServiceTierCanonicalizationStaticProof(config)
	if err != nil {
		t.Fatal(err)
	}
	config.ClientCanonicalizationStaticProofSHA256 = proof
	return config
}

// withDefaultServiceTier keeps ordinary fixtures on the formal cost contract.
// Tests that exercise omitted/auto/priority tiers pass those explicit values,
// which this helper deliberately preserves.
func withDefaultServiceTier(t *testing.T, body string) string {
	t.Helper()
	var request map[string]any
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		t.Fatalf("decode test Responses request: %v", err)
	}
	if _, present := request["service_tier"]; !present {
		request["service_tier"] = "default"
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("encode test Responses request: %v", err)
	}
	return string(encoded)
}

func withDefaultResponseServiceTier(stream string) string {
	lines := strings.SplitAfter(stream, "\n")
	for index, framed := range lines {
		line := strings.TrimSuffix(framed, "\n")
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(data), &event) != nil || event["type"] != "response.completed" {
			continue
		}
		response, ok := event["response"].(map[string]any)
		if !ok {
			continue
		}
		if _, present := response["service_tier"]; !present {
			response["service_tier"] = "default"
		}
		encoded, err := json.Marshal(event)
		if err == nil {
			lines[index] = "data: " + string(encoded) + "\n"
		}
	}
	return strings.Join(lines, "")
}

func setContinuationHeaders(request *http.Request, lineage string, epoch uint64, reset bool) {
	request.Header.Set("X-Luban-Stateless-Lineage", lineage)
	request.Header.Set("X-Luban-Stateless-Epoch", fmt.Sprintf("%d", epoch))
	if reset {
		request.Header.Set("X-Luban-Stateless-Reset", "1")
	}
}

func TestProxyInjectsCredentialAndRecordsContentFreeStreamingEvidence(t *testing.T) {
	var upstreamAuthorization string
	var upstreamContinuationHeaders http.Header
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamAuthorization = request.Header.Get("Authorization")
		upstreamContinuationHeaders = request.Header.Clone()
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("x-request-id", "req-server-1")
		flusher := writer.(http.Flusher)
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-1\",\"model\":\"gpt-5.6-sol\"}}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(writer, withDefaultResponseServiceTier("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-1\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":100,\"input_tokens_details\":{\"cached_tokens\":80,\"cache_write_tokens\":7},\"output_tokens\":20,\"output_tokens_details\":{\"reasoning_tokens\":12}},\"output\":[{\"type\":\"reasoning\",\"id\":\"rs-1\",\"summary\":[],\"encrypted_content\":\"ENCRYPTED-REASONING-SECRET\"},{\"type\":\"function_call\",\"call_id\":\"call-1\",\"name\":\"Bash\",\"arguments\":\"{\\\"cmd\\\":\\\"pwd\\\"}\"}]}}\n\n"))
		flusher.Flush()
	}))
	defer upstream.Close()
	evidencePath := filepath.Join(t.TempDir(), "provider-http.jsonl")
	accessPath := "/" + strings.Repeat("a", 48)
	handler, err := NewHandler(Config{
		Upstream: upstream.URL, EvidencePath: evidencePath, Transport: upstream.Client().Transport,
		AccessPath: accessPath, Credential: "REAL-UPSTREAM-SECRET",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	body := withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"prompt_cache_key":"cache-secret-key","prompt_cache_options":{"mode":"implicit","ttl":"30m"},"tools":[{"type":"function","name":"Bash","description":"Run a local command","strict":true,"parameters":{"type":"object","properties":{"cmd":{"type":"string"}},"required":["cmd"],"additionalProperties":false}}],"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"TOP-SECRET-SYSTEM","prompt_cache_breakpoint":{"mode":"explicit"}}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"TOP-SECRET-PROMPT"}]}]}`)
	request, err := http.NewRequest(http.MethodPost, proxy.URL+accessPath+"/v1/responses", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer DUMMY-AGENT-TOKEN")
	setContinuationHeaders(request, "lineage-main-test", 1, false)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if upstreamAuthorization != "Bearer REAL-UPSTREAM-SECRET" {
		t.Fatalf("proxy did not replace the agent credential")
	}
	for _, name := range []string{"X-Luban-Stateless-Lineage", "X-Luban-Stateless-Epoch", "X-Luban-Stateless-Reset"} {
		if got := upstreamContinuationHeaders.Get(name); got != "" {
			t.Fatalf("private continuation header %s reached upstream: %q", name, got)
		}
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"TOP-SECRET", "ENCRYPTED-REASONING-SECRET", "REAL-UPSTREAM", "DUMMY-AGENT", "req-server-1", "resp-1", "prev-secret-id", "cache-secret-key", accessPath} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("evidence persisted forbidden content %q: %s", forbidden, raw)
		}
	}
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != "agentic-bench/provider-http-v6" || record.RunIdentity != testRunIdentity || !record.ProtocolValid || record.Round != 0 || record.Path != "/v1/responses" || record.RequestedModel != "gpt-5.6-sol" || record.RequestedReasoningEffort != "xhigh" || record.RequestedReasoningContext != "all_turns" || record.UpstreamRequestIDHash == "" || record.ResponseIDHash == "" || record.ResponseModel != "gpt-5.6-sol" || !record.ResponseCompleted || record.ResponseStatus != "completed" {
		t.Fatalf("identity evidence is incomplete: %#v", record)
	}
	if !record.StoreSpecified || record.Store || record.PreviousResponseIDPresent || record.PreviousResponseIDHash != "" || !record.PromptCacheKeyPresent || record.PromptCacheKeyHash == "" {
		t.Fatalf("storage/cache request metadata is incomplete: %#v", record)
	}
	if !record.CachePolicyObserved || !record.PromptCacheOptionsPresent || record.PromptCacheOptionsMode != "implicit" || record.PromptCacheTTLSeconds == nil || *record.PromptCacheTTLSeconds != 1800 || record.PromptCacheRetentionPresent || record.CacheBreakpointCount != 1 || len(record.CacheBreakpointPositionHashes) != 1 || !isLowerHex64(record.CacheBreakpointPositionHashes[0]) {
		t.Fatalf("cache policy evidence is incomplete: %#v", record)
	}
	if len(record.ToolResults) != 0 {
		t.Fatalf("unexpected tool-result projection: %#v", record.ToolResults)
	}
	if record.RequestBytes != int64(len(body)) || record.ResponseBytes == 0 || record.UpstreamHeadersAt.IsZero() || record.FirstResponseByteAt.IsZero() || record.FinishedAt.IsZero() {
		t.Fatalf("transport timing or bytes are incomplete: %#v", record)
	}
	if !record.UsagePresent || record.InputTokens == nil || *record.InputTokens != 100 || record.CachedInputTokens == nil || *record.CachedInputTokens != 80 || record.CacheWriteInputTokens == nil || *record.CacheWriteInputTokens != 7 || record.OutputTokens == nil || *record.OutputTokens != 20 || record.ReasoningOutputTokens == nil || *record.ReasoningOutputTokens != 12 {
		t.Fatalf("usage evidence is wrong: %#v", record)
	}
	if len(record.ToolCalls) != 1 || record.ToolCalls[0].IDHash == "" || record.ToolCalls[0].Name != "Bash" {
		t.Fatalf("tool evidence is wrong: %#v", record.ToolCalls)
	}
	if !record.EncryptedReasoningRequested || record.EncryptedReasoningItemCount != 1 || len(record.EncryptedReasoningHashes) != 1 || record.EncryptedReasoningHashes[0] == "" || record.EncryptedReasoningReplayBound {
		t.Fatalf("encrypted reasoning evidence is wrong: %#v", record)
	}
	if !record.ContinuationLineagePresent || record.ContinuationLineageHash == "" || record.ContinuationEpoch != 1 || record.ContinuationReset || record.ResponseOutputItemCount != 2 || len(record.ResponseOutputItemHashes) != 2 || record.ReplayOutputItemCount != 0 || record.ReplayOutputItemsBound {
		t.Fatalf("continuation evidence is wrong: %#v", record)
	}
}

func TestProxySealsStructuredContextFailureWithoutDiagnosticText(t *testing.T) {
	const privateMessage = "private provider context diagnostic"
	failureEvent := `{"type":"response.failed","response":{"id":"resp-context","model":"gpt-5.6-sol","status":"failed","error":{"code":"context_length_exceeded","message":"` + privateMessage + `"}}}`
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("x-request-id", "req-context")
		_, _ = io.WriteString(writer, "data: "+failureEvent+"\n\n")
	}))
	defer upstream.Close()

	evidencePath := filepath.Join(t.TempDir(), "provider-http.jsonl")
	accessPath := "/" + strings.Repeat("k", 48)
	handler, err := NewHandler(Config{
		Upstream: upstream.URL, EvidencePath: evidencePath, Transport: upstream.Client().Transport,
		AccessPath: accessPath, Credential: "upstream-secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	body := `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"service_tier":"default","include":["reasoning.encrypted_content"],"store":false,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"secret prompt"}]}]}`
	request, err := http.NewRequest(http.MethodPost, proxy.URL+accessPath+"/v1/responses", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer agent-secret")
	setContinuationHeaders(request, "lineage-context-test", 1, false)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err := handler.SealEvidence(); err != nil {
		t.Fatal(err)
	}
	seal, err := ValidateEvidenceSeal(evidencePath, testRunIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if seal.RecordCount != 1 || seal.Fatal {
		t.Fatalf("context evidence seal = %#v", seal)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 {
		t.Fatalf("context records = %#v", records)
	}
	record := records[0]
	if record.ProtocolValid || record.Disposition != "agent_context_failure" ||
		record.ErrorCode != "provider_context_failure" ||
		record.ResponseFailureCode != "context_length_exceeded" ||
		record.ResponseFailureEventSHA256 != hashRawBytes([]byte(failureEvent)) ||
		record.RunIdentity != testRunIdentity || record.ProviderAttemptKind != "inference" {
		t.Fatalf("structured context receipt = %#v", record)
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{privateMessage, "secret prompt", "upstream-secret", "agent-secret", "resp-context", "req-context"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("context receipt leaked %q: %s", forbidden, raw)
		}
	}
}

func TestStreamCollectorRejectsMissingUnknownAndDuplicateResponseFailureCodes(t *testing.T) {
	for name, events := range map[string][]string{
		"missing":   {`{"type":"response.failed","response":{"error":{"message":"context_length_exceeded"}}}`},
		"unknown":   {`{"type":"response.failed","response":{"error":{"code":"future_failure"}}}`},
		"duplicate": {`{"type":"response.failed","response":{"error":{"code":"context_length_exceeded"}}}`, `{"type":"response.failed","response":{"error":{"code":"context_length_exceeded"}}}`},
	} {
		t.Run(name, func(t *testing.T) {
			record := Record{}
			collector := newStreamCollector(&record, func(any) string { return strings.Repeat("a", 64) })
			for _, event := range events {
				collector.consume([]byte(event))
			}
			if record.Disposition != "experiment_invalid" || record.ErrorCode == "" {
				t.Fatalf("untyped provider failure was accepted: %#v", record)
			}
		})
	}
}

func TestProxyFailsClosedBeforeUpstreamOnPinMismatch(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamCalls++ }))
	defer upstream.Close()
	evidencePath := filepath.Join(t.TempDir(), "provider-http.jsonl")
	accessPath := "/" + strings.Repeat("b", 48)
	handler, err := NewHandler(Config{Upstream: upstream.URL, EvidencePath: evidencePath, Transport: upstream.Client().Transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"high","context":"all_turns"},"store":false}`)))
	setContinuationHeaders(request, "lineage-pin-test", 1, false)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || upstreamCalls != 0 {
		t.Fatalf("pin mismatch status=%d upstreamCalls=%d", response.Code, upstreamCalls)
	}
	var record Record
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatal(err)
	}
	if record.ErrorCode != "pinned_request_mismatch" || record.ProtocolValid {
		t.Fatalf("pin mismatch evidence = %#v", record)
	}
}

func TestProxyRejectsMissingTrueOrNonBooleanStoreBeforeUpstream(t *testing.T) {
	upstreamCalls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		return nil, nil
	})
	accessPath := "/" + strings.Repeat("d", 48)
	for _, body := range []string{
		`{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"}}`,
		`{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"store":true}`,
		`{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"store":"false"}`,
	} {
		evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
		handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, body)))
		setContinuationHeaders(request, "lineage-store-test", 1, false)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("store gate status=%d body=%s", response.Code, body)
		}
		var record Record
		raw, err := os.ReadFile(evidencePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		if record.ErrorCode != "response_storage_not_disabled" || record.ProtocolValid {
			t.Fatalf("store gate evidence = %#v", record)
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("store gate reached upstream %d times", upstreamCalls)
	}
}

func TestReasoningModeAdmissionDefaultsToStandardAndRejectsProUnknownOrNonString(t *testing.T) {
	tests := []struct {
		name          string
		reasoning     string
		wantStatus    int
		wantCanonical string
	}{
		{name: "absent", reasoning: `{"effort":"xhigh"}`, wantStatus: http.StatusOK, wantCanonical: "standard"},
		{name: "explicit standard", reasoning: `{"effort":"xhigh","mode":"standard"}`, wantStatus: http.StatusOK, wantCanonical: "standard"},
		{name: "explicit default", reasoning: `{"effort":"xhigh","mode":"default"}`, wantStatus: http.StatusOK, wantCanonical: "standard"},
		{name: "pro", reasoning: `{"effort":"xhigh","mode":"pro"}`, wantStatus: http.StatusBadRequest, wantCanonical: "pro"},
		{name: "unknown", reasoning: `{"effort":"xhigh","mode":"turbo"}`, wantStatus: http.StatusBadRequest, wantCanonical: "unknown"},
		{name: "non string", reasoning: `{"effort":"xhigh","mode":7}`, wantStatus: http.StatusBadRequest, wantCanonical: "invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstreamCalls := 0
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				upstreamCalls++
				body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-mode\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n"
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-mode"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
			})
			evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
			accessPath := "/" + strings.Repeat("m", 48)
			handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
			if err != nil {
				t.Fatal(err)
			}
			body := `{"model":"gpt-5.6-sol","reasoning":` + test.reasoning + `,"text":{"verbosity":"high"},"max_output_tokens":4096,"store":false,"input":[]}`
			request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, body)))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d", response.Code, test.wantStatus)
			}
			record := readEvidenceRecords(t, evidencePath)[0]
			if record.RequestedReasoningModeCanonical != test.wantCanonical || record.RequestedTextVerbosity != "high" || !record.MaxOutputTokensSpecified || record.MaxOutputTokens == nil || *record.MaxOutputTokens != 4096 {
				t.Fatalf("request strategy evidence = %#v", record)
			}
			if test.wantStatus == http.StatusOK {
				if upstreamCalls != 1 || !record.ProtocolValid {
					t.Fatalf("accepted mode calls=%d record=%#v", upstreamCalls, record)
				}
			} else if upstreamCalls != 0 || record.ErrorCode != "reasoning_mode_not_comparable" || record.Disposition != "experiment_invalid" {
				t.Fatalf("rejected mode calls=%d record=%#v", upstreamCalls, record)
			}
		})
	}
}

func TestProxyBindsEncryptedReasoningReplayAndRejectsUnknownCiphertext(t *testing.T) {
	upstreamCalls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		ciphertext := "cipher-from-round-one"
		responseID := "response-round-one"
		if upstreamCalls == 2 {
			ciphertext = "cipher-from-round-two"
			responseID = "response-round-two"
		}
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"" + responseID + "\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"input_tokens_details\":{\"cached_tokens\":4},\"output_tokens\":3},\"output\":[{\"type\":\"reasoning\",\"id\":\"reasoning-" + responseID + "\",\"summary\":[],\"encrypted_content\":\"" + ciphertext + "\"}]}}\n\n"
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"private-upstream-request"}},
			Body:       io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body))),
		}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("r", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}

	send := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, body)))
		setContinuationHeaders(request, "lineage-replay-test", 1, false)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	base := `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"input":[]}`
	if response := send(base); response.Code != http.StatusOK {
		t.Fatalf("first round status = %d", response.Code)
	}
	if response := send(base); response.Code != http.StatusBadRequest {
		t.Fatalf("missing full-history replay status = %d", response.Code)
	}
	replay := `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"input":[{"type":"reasoning","id":"reasoning-response-round-one","summary":[],"encrypted_content":"cipher-from-round-one"}]}`
	if response := send(replay); response.Code != http.StatusOK {
		t.Fatalf("bound replay status = %d", response.Code)
	}
	unknown := `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"input":[{"type":"reasoning","id":"unbound","summary":[],"encrypted_content":"unknown-ciphertext"}]}`
	if response := send(unknown); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown replay status = %d", response.Code)
	}
	if upstreamCalls != 2 {
		t.Fatalf("upstream calls = %d, want 2", upstreamCalls)
	}

	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 4 {
		t.Fatalf("records = %d, want 4", len(records))
	}
	if !records[0].ProtocolValid || records[0].EncryptedReasoningReplayBound || len(records[0].EncryptedReasoningHashes) != 1 {
		t.Fatalf("first reasoning receipt = %#v", records[0])
	}
	if records[1].ProtocolValid || records[1].ErrorCode != "unbound_stateless_output_replay" || records[1].EncryptedReasoningReplayCount != 0 {
		t.Fatalf("missing replay receipt = %#v", records[1])
	}
	if !records[2].ProtocolValid || !records[2].EncryptedReasoningReplayBound || records[2].EncryptedReasoningReplayCount != 1 || records[2].EncryptedReasoningReplayHashes[0] != records[0].EncryptedReasoningHashes[0] {
		t.Fatalf("bound replay receipt = %#v", records[2])
	}
	if records[3].ProtocolValid || records[3].ErrorCode != "unbound_stateless_output_replay" {
		t.Fatalf("unknown replay receipt = %#v", records[3])
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"cipher-from-round-one", "cipher-from-round-two", "unknown-ciphertext", "private-upstream-request"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("evidence leaked %q", secret)
		}
	}
}

func TestHeaderlessCodexAndHeaderedLubanAcceptSameBoundReplay(t *testing.T) {
	for _, test := range []struct {
		name          string
		headered      bool
		lineageSource string
	}{
		{name: "codex controller lineage", lineageSource: "controller_default"},
		{name: "luban agent lineage", headered: true, lineageSource: "agent_header"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				output := `[]`
				if calls == 1 {
					output = `[{"type":"reasoning","id":"shared-reasoning","summary":[],"encrypted_content":"shared-private-cipher"}]`
				}
				body := fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"shared-response-%d\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"input_tokens_details\":{\"cached_tokens\":1},\"output_tokens\":1},\"output\":%s}}\n\n", calls, output)
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{fmt.Sprintf("shared-request-%d", calls)}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
			})
			evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
			accessPath := "/" + strings.Repeat("q", 48)
			handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
			if err != nil {
				t.Fatal(err)
			}
			send := func(input string) *httptest.ResponseRecorder {
				body := fmt.Sprintf(`{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"input":%s}`, input)
				request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, body)))
				if test.headered {
					setContinuationHeaders(request, "lineage-shared-canary", 1, false)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				return response
			}
			if response := send(`[]`); response.Code != http.StatusOK {
				t.Fatalf("first status = %d", response.Code)
			}
			if response := send(`[{"type":"reasoning","id":"shared-reasoning","summary":[],"encrypted_content":"shared-private-cipher"}]`); response.Code != http.StatusOK {
				t.Fatalf("bound replay status = %d", response.Code)
			}
			records := readEvidenceRecords(t, evidencePath)
			if len(records) != 2 || !records[0].ProtocolValid || !records[1].ProtocolValid || records[1].ContinuationLineageSource != test.lineageSource || !records[1].ReplayOutputItemsBound || !records[1].EncryptedReasoningReplayBound {
				t.Fatalf("dual-agent replay receipts = %#v", records)
			}
		})
	}
}

func TestToolResultPayloadHMACRejectsSameOutputSemanticStatusMutation(t *testing.T) {
	callItem := map[string]any{
		"type": "function_call", "id": "function-item-immutable", "call_id": "call-immutable",
		"name": "Bash", "arguments": `{"cmd":"pwd"}`, "status": "completed",
	}
	upstreamCalls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		output := []any{}
		if upstreamCalls == 1 {
			output = []any{callItem}
		}
		payload, err := json.Marshal(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id": fmt.Sprintf("response-tool-result-%d", upstreamCalls), "model": "gpt-5.6-sol", "status": "completed",
				"usage":  map[string]any{"input_tokens": 3, "input_tokens_details": map[string]any{"cached_tokens": 1}, "output_tokens": 1},
				"output": output,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{fmt.Sprintf("request-tool-result-%d", upstreamCalls)}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier("data: " + string(payload) + "\n\n")))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("y", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	send := func(input []any) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{
			"model": "gpt-5.6-sol", "reasoning": map[string]any{"effort": "xhigh"}, "store": false,
			"tools": []any{map[string]any{
				"type": "function", "name": "Bash", "description": "Run a local command", "strict": true,
				"parameters": map[string]any{"type": "object", "properties": map[string]any{"cmd": map[string]any{"type": "string"}}, "required": []any{"cmd"}, "additionalProperties": false},
			}},
			"input": input,
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, string(body))))
		setContinuationHeaders(request, "lineage-tool-result-hmac", 1, false)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := send([]any{}); response.Code != http.StatusOK {
		t.Fatalf("first status = %d", response.Code)
	}
	validHistory := []any{callItem, map[string]any{"type": "function_call_output", "call_id": "call-immutable", "status": "completed", "output": "AA"}}
	if response := send(validHistory); response.Code != http.StatusOK {
		t.Fatalf("first result status = %d", response.Code)
	}
	bitFlipped := []any{callItem, map[string]any{"type": "function_call_output", "call_id": "call-immutable", "status": "failed", "output": "AA"}}
	if response := send(bitFlipped); response.Code != http.StatusBadRequest {
		t.Fatalf("same-output semantic status mutation = %d", response.Code)
	}
	if upstreamCalls != 2 {
		t.Fatalf("upstream calls = %d, want 2", upstreamCalls)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 3 || !records[1].ProtocolValid || !records[1].ToolResultHistoryValid || len(records[1].ToolResults) != 1 || records[1].ToolResults[0].PayloadHash == "" || records[1].ToolResults[0].OutputBytes != 2 {
		t.Fatalf("valid tool-result receipt = %#v", records)
	}
	if records[2].ProtocolValid || records[2].ToolResultHistoryValid || records[2].ErrorCode != "unbound_stateless_output_replay" || records[2].Disposition != "experiment_invalid" || records[2].ToolResults[0].PayloadHash == records[1].ToolResults[0].PayloadHash {
		t.Fatalf("bit-flip receipt = %#v", records[2])
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"AA"`) || strings.Contains(string(raw), `"BB"`) {
		t.Fatalf("tool result payload leaked: %s", raw)
	}
}

func TestProxyAcceptsOrderedLineageResetAndRejectsReorderedOrReusedHistory(t *testing.T) {
	itemA := map[string]any{"type": "reasoning", "id": "reasoning-a", "summary": []any{}, "encrypted_content": "cipher-a"}
	itemB := map[string]any{"type": "reasoning", "id": "reasoning-b", "summary": []any{}, "encrypted_content": "cipher-b"}
	itemC := map[string]any{"type": "reasoning", "id": "reasoning-c", "summary": []any{}, "encrypted_content": "cipher-c"}
	responses := [][]any{{itemA, itemB}, {itemC}, {}}
	var upstreamHeaders []http.Header
	upstreamCalls := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamHeaders = append(upstreamHeaders, request.Header.Clone())
		index := upstreamCalls
		upstreamCalls++
		if index >= len(responses) {
			t.Fatalf("unexpected upstream call %d", upstreamCalls)
		}
		payload, err := json.Marshal(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":     fmt.Sprintf("response-lineage-%d", upstreamCalls),
				"model":  "gpt-5.6-sol",
				"status": "completed",
				"usage": map[string]any{
					"input_tokens": 10, "input_tokens_details": map[string]any{"cached_tokens": 4}, "output_tokens": 3,
				},
				"output": responses[index],
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{fmt.Sprintf("request-lineage-%d", upstreamCalls)}},
			Body:       io.NopCloser(strings.NewReader(withDefaultResponseServiceTier("data: " + string(payload) + "\n\n"))),
		}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("l", 48)
	handler, err := NewHandler(Config{
		Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport,
		AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	send := func(input []any, epoch uint64, reset bool) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"model": "gpt-5.6-sol", "reasoning": map[string]any{"effort": "xhigh", "context": "all_turns"},
			"include": []any{"reasoning.encrypted_content"}, "store": false, "input": input,
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, string(body))))
		setContinuationHeaders(request, "lineage-reset-test", epoch, reset)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := send([]any{}, 1, false); response.Code != http.StatusOK {
		t.Fatalf("initial lineage status = %d", response.Code)
	}
	if response := send([]any{itemB}, 2, true); response.Code != http.StatusOK {
		t.Fatalf("ordered compaction reset status = %d", response.Code)
	}
	if response := send([]any{itemC, itemB}, 3, true); response.Code != http.StatusBadRequest {
		t.Fatalf("reordered reset status = %d", response.Code)
	}
	if response := send([]any{itemA}, 2, false); response.Code != http.StatusBadRequest {
		t.Fatalf("reused pre-reset item status = %d", response.Code)
	}
	if response := send([]any{itemB, itemC}, 2, false); response.Code != http.StatusOK {
		t.Fatalf("post-reset exact replay status = %d", response.Code)
	}
	if upstreamCalls != 3 {
		t.Fatalf("upstream calls = %d, want 3", upstreamCalls)
	}
	for call, headers := range upstreamHeaders {
		for _, name := range []string{"X-Luban-Stateless-Lineage", "X-Luban-Stateless-Epoch", "X-Luban-Stateless-Reset"} {
			if got := headers.Get(name); got != "" {
				t.Fatalf("upstream call %d received private header %s=%q", call+1, name, got)
			}
		}
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 5 {
		t.Fatalf("records = %d, want 5", len(records))
	}
	if !records[0].ProtocolValid || records[0].ResponseOutputItemCount != 2 {
		t.Fatalf("initial lineage receipt = %#v", records[0])
	}
	if !records[1].ProtocolValid || !records[1].ContinuationReset || !records[1].ContinuationResetAccepted || !records[1].ReplayOutputItemsBound || !records[1].EncryptedReasoningReplayBound || records[1].ReplayOutputItemCount != 1 {
		t.Fatalf("accepted reset receipt = %#v", records[1])
	}
	if records[1].ReplayOutputItemHashes[0] != records[0].ResponseOutputItemHashes[1] || records[1].EncryptedReasoningReplayHashes[0] != records[0].EncryptedReasoningHashes[1] {
		t.Fatalf("reset did not bind the exact retained item: before=%#v reset=%#v", records[0], records[1])
	}
	for _, index := range []int{2, 3} {
		if records[index].ProtocolValid || records[index].ErrorCode != "unbound_stateless_output_replay" {
			t.Fatalf("rejected lineage receipt %d = %#v", index, records[index])
		}
	}
	if !records[4].ProtocolValid || !records[4].ReplayOutputItemsBound || records[4].ReplayOutputItemCount != 2 || records[4].ContinuationEpoch != 2 {
		t.Fatalf("post-reset replay receipt = %#v", records[4])
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"cipher-a", "cipher-b", "cipher-c", "lineage-reset-test"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("lineage evidence leaked %q", secret)
		}
	}
}

func TestProxyRequiresAuthenticResponseCompletedEvent(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"fake-completed-id\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"fake-request"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("f", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"input":[]}`)))
	setContinuationHeaders(request, "lineage-completed-proof", 1, false)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("forwarded status = %d", response.Code)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 || records[0].ProtocolValid || records[0].ResponseCompleted || records[0].UsagePresent || records[0].ResponseIDHash != "" || records[0].ErrorCode != "provider_response_not_completed" {
		t.Fatalf("forged completion receipt = %#v", records)
	}
}

func TestProxyMarksServedModelMismatchAsExperimentInvalid(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-wrong-model\",\"model\":\"gpt-5.6-mini\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-wrong-model"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("m", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	records := readEvidenceRecords(t, evidencePath)
	if response.Code != http.StatusOK || len(records) != 1 || records[0].ProtocolValid || records[0].ErrorCode != "served_model_mismatch" || records[0].Disposition != "experiment_invalid" || records[0].ResponseModel != "gpt-5.6-mini" {
		t.Fatalf("served-model mismatch receipt = %#v", records)
	}
}

func TestServedModelMismatchOverridesConcurrentProtocolFailures(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		streamBody string
	}{
		{
			name:       "missing usage",
			status:     http.StatusOK,
			streamBody: "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"wrong-missing-usage\",\"model\":\"gpt-5.6-mini\",\"status\":\"completed\",\"output\":[]}}\n\n",
		},
		{
			name:       "incomplete response",
			status:     http.StatusOK,
			streamBody: "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"wrong-incomplete\",\"model\":\"gpt-5.6-mini\",\"status\":\"incomplete\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n",
		},
		{
			name:       "provider http error",
			status:     http.StatusInternalServerError,
			streamBody: "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"wrong-http\",\"model\":\"gpt-5.6-mini\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n",
		},
		{
			name:       "truncated after created",
			status:     http.StatusOK,
			streamBody: "data: {\"type\":\"response.created\",\"response\":{\"id\":\"wrong-created\",\"model\":\"gpt-5.6-mini\",\"status\":\"in_progress\"}}\n\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-wrong-matrix"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(test.streamBody)))}, nil
			})
			evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
			accessPath := "/" + strings.Repeat("n", 48)
			handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			records := readEvidenceRecords(t, evidencePath)
			if len(records) != 1 || records[0].ProtocolValid || records[0].ErrorCode != "served_model_mismatch" || records[0].Disposition != "experiment_invalid" {
				t.Fatalf("wrong-model precedence receipt = %#v", records)
			}
		})
	}
}

func TestProxyRecordsAndEnforcesComparableServiceTier(t *testing.T) {
	value := func(source string) *string { return &source }
	for _, test := range []struct {
		name            string
		requestTier     *string
		responseTier    *string
		wantStatus      int
		wantUpstream    int
		wantValid       bool
		wantError       string
		wantDisposition string
	}{
		{name: "explicit default", requestTier: value("default"), responseTier: value("default"), wantStatus: http.StatusOK, wantUpstream: 1, wantValid: true, wantDisposition: "valid"},
		{name: "omitted request", responseTier: value("default"), wantStatus: http.StatusBadRequest, wantError: "service_tier_request_not_comparable", wantDisposition: "experiment_invalid"},
		{name: "auto request", requestTier: value("auto"), responseTier: value("default"), wantStatus: http.StatusBadRequest, wantError: "service_tier_request_not_comparable", wantDisposition: "experiment_invalid"},
		{name: "priority request", requestTier: value("priority"), responseTier: value("priority"), wantStatus: http.StatusBadRequest, wantError: "service_tier_request_not_comparable", wantDisposition: "experiment_invalid"},
		{name: "actual tier drift", requestTier: value("default"), responseTier: value("flex"), wantStatus: http.StatusOK, wantUpstream: 1, wantError: "service_tier_mismatch", wantDisposition: "experiment_invalid"},
		{name: "priority actual tier drift", requestTier: value("default"), responseTier: value("priority"), wantStatus: http.StatusOK, wantUpstream: 1, wantError: "service_tier_mismatch", wantDisposition: "experiment_invalid"},
		{name: "missing actual tier", requestTier: value("default"), wantStatus: http.StatusOK, wantUpstream: 1, wantError: "service_tier_mismatch", wantDisposition: "experiment_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstreamCalls := 0
			transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
				upstreamCalls++
				completed := map[string]any{
					"id": "response-tier", "model": "gpt-5.6-sol", "status": "completed",
					"usage": map[string]any{"input_tokens": 1, "input_tokens_details": map[string]any{"cached_tokens": 0}, "output_tokens": 1}, "output": []any{},
				}
				if test.responseTier != nil {
					completed["service_tier"] = *test.responseTier
				}
				encoded, _ := json.Marshal(map[string]any{"type": "response.completed", "response": completed})
				body := "data: " + string(encoded) + "\n\n"
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-tier"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
			accessPath := "/" + strings.Repeat("t", 48)
			handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
			if err != nil {
				t.Fatal(err)
			}
			requestBody := map[string]any{"model": "gpt-5.6-sol", "reasoning": map[string]any{"effort": "xhigh"}, "store": false, "input": []any{}}
			if test.requestTier != nil {
				requestBody["service_tier"] = *test.requestTier
			}
			encoded, _ := json.Marshal(requestBody)
			request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", bytes.NewReader(encoded))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			records := readEvidenceRecords(t, evidencePath)
			if response.Code != test.wantStatus || upstreamCalls != test.wantUpstream || len(records) != 1 {
				t.Fatalf("status=%d upstream=%d records=%#v", response.Code, upstreamCalls, records)
			}
			record := records[0]
			wantRequestTier, wantResponseTier := "", ""
			if test.requestTier != nil {
				wantRequestTier = *test.requestTier
			}
			if test.responseTier != nil && test.wantUpstream > 0 {
				wantResponseTier = *test.responseTier
			}
			if record.ProtocolValid != test.wantValid || record.ErrorCode != test.wantError || record.Disposition != test.wantDisposition || record.RequestedServiceTier != wantRequestTier || record.ResponseServiceTier != wantResponseTier || record.ServiceTierComparable != test.wantValid {
				t.Fatalf("service tier receipt = %#v", record)
			}
		})
	}
}

func TestServiceTierCanonicalizationStaticProofBindsEveryFrozenInput(t *testing.T) {
	base := codexStaticProofConfig()
	proof, err := ServiceTierCanonicalizationStaticProof(base)
	if err != nil {
		t.Fatal(err)
	}
	if !isLowerHex64(proof) {
		t.Fatalf("static proof = %q", proof)
	}
	mutations := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "registered binary", mutate: func(value *Config) { value.RegisteredBinarySHA256 = strings.Repeat("a", 64) }},
		{name: "bundle manifest", mutate: func(value *Config) { value.FrozenBundleManifestSHA256 = strings.Repeat("a", 64) }},
		{name: "bundle tree", mutate: func(value *Config) { value.FrozenBundleTreeSHA256 = strings.Repeat("a", 64) }},
		{name: "canonical canary", mutate: func(value *Config) { value.FrozenCanonicalCanaryReceiptSHA256 = strings.Repeat("a", 64) }},
		{name: "adapter", mutate: func(value *Config) { value.AdapterSHA256 = strings.Repeat("a", 64) }},
		{name: "adapter version", mutate: func(value *Config) { value.AdapterVersion = "2.3.1" }},
		{name: "source command argv", mutate: func(value *Config) { value.SourceCommandArgvSHA256 = strings.Repeat("a", 64) }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			mutated := base
			test.mutate(&mutated)
			got, err := ServiceTierCanonicalizationStaticProof(mutated)
			if err != nil {
				t.Fatal(err)
			}
			if got == proof || !isLowerHex64(got) {
				t.Fatalf("mutated proof = %q, original = %q", got, proof)
			}
		})
	}
	invalid := base
	invalid.FrozenBundleTreeSHA256 = strings.ToUpper(strings.Repeat("a", 64))
	if _, err := ServiceTierCanonicalizationStaticProof(invalid); err == nil {
		t.Fatal("uppercase frozen digest unexpectedly produced a static proof")
	}
	if proof, err := ServiceTierCanonicalizationStaticProof(Config{AgentID: "luban"}); err != nil || proof != "" {
		t.Fatalf("Luban static proof = %q, %v", proof, err)
	}
}

func TestCodexHandlerRequiresExternallyComputedMatchingStaticProof(t *testing.T) {
	upstreamCalls := 0
	base := codexStaticProofConfig()
	base.Upstream = "https://api.openai.com"
	base.EvidencePath = filepath.Join(t.TempDir(), "evidence.jsonl")
	base.AccessPath = "/" + strings.Repeat("p", 48)
	base.Credential = "secret"
	base.ExpectedModel = "gpt-5.6-sol"
	base.ExpectedEffort = "xhigh"
	base.RunIdentity = testRunIdentity
	base.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		return nil, errors.New("unexpected upstream call")
	})
	if _, err := NewHandler(base); err == nil {
		t.Fatal("Codex handler accepted a missing pre-run static proof")
	}
	if upstreamCalls != 0 {
		t.Fatalf("missing proof reached upstream %d times", upstreamCalls)
	}
	proof, err := ServiceTierCanonicalizationStaticProof(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ClientCanonicalizationStaticProofSHA256 = strings.Repeat("f", 64)
	if base.ClientCanonicalizationStaticProofSHA256 == proof {
		t.Fatal("mismatched proof fixture unexpectedly equals computed proof")
	}
	if _, err := NewHandler(base); err == nil {
		t.Fatal("Codex handler accepted a mismatched pre-run static proof")
	}
	base.ClientCanonicalizationStaticProofSHA256 = strings.ToUpper(proof)
	if _, err := NewHandler(base); err == nil {
		t.Fatal("Codex handler accepted an uppercase pre-run static proof")
	}
	base.ClientCanonicalizationStaticProofSHA256 = proof
	if _, err := NewHandler(base); err != nil {
		t.Fatalf("Codex handler rejected the externally computed proof: %v", err)
	}
}

func TestServiceTierControllerTransformationIsExactAndAgentSpecific(t *testing.T) {
	for _, test := range []struct {
		name               string
		agentID            string
		body               string
		wantTransformation string
		wantOriginalTier   bool
		wantBytesUnchanged bool
	}{
		{
			name: "Codex frozen omission is injected", agentID: "codex",
			body:               " {\n  \"model\":\"gpt-5.6-sol\",\"reasoning\":{\"effort\":\"xhigh\"},\"store\":false,\"input\":[],\"sentinel\":9007199254740993\n } ",
			wantTransformation: serviceTierTransformationInjectDefault,
		},
		{
			name: "Luban explicit default is byte exact", agentID: "luban",
			body:               " {\n  \"service_tier\" : \"default\", \"model\":\"gpt-5.6-sol\",\"reasoning\":{\"effort\":\"xhigh\"},\"store\":false,\"input\":[],\"sentinel\":9007199254740993\n } ",
			wantTransformation: serviceTierTransformationNone, wantOriginalTier: true, wantBytesUnchanged: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var upstreamBody []byte
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				var err error
				upstreamBody, err = io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				stream := withDefaultResponseServiceTier("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-tier-transform\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n")
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-tier-transform"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
			})
			evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
			accessPath := "/" + strings.Repeat("q", 48)
			config := Config{
				Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport,
				AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh",
				AgentID: test.agentID, RunIdentity: testRunIdentity,
			}
			if test.agentID == "codex" {
				config = configureCodexStaticProof(t, config)
			}
			handler, err := NewHandler(config)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(test.body)))
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			originalObject := decodeTestJSONObject(t, []byte(test.body))
			forwardedObject := decodeTestJSONObject(t, upstreamBody)
			if forwardedObject["service_tier"] != "default" {
				t.Fatalf("upstream service tier = %#v", forwardedObject["service_tier"])
			}
			delete(forwardedObject, "service_tier")
			delete(originalObject, "service_tier")
			if !reflect.DeepEqual(originalObject, forwardedObject) {
				t.Fatalf("non-tier request semantics changed:\noriginal=%#v\nforwarded=%#v", originalObject, forwardedObject)
			}
			if test.wantBytesUnchanged && !bytes.Equal([]byte(test.body), upstreamBody) {
				t.Fatalf("Luban body changed:\noriginal=%q\nforwarded=%q", test.body, upstreamBody)
			}
			records := readEvidenceRecords(t, evidencePath)
			if len(records) != 1 {
				t.Fatalf("records=%#v", records)
			}
			record := records[0]
			if !record.ProtocolValid || !record.ServiceTierComparable || record.ServiceTierTransformation != test.wantTransformation ||
				!record.ServiceTierTransformationExactDiff || !isLowerHex64(record.ServiceTierTransformationProofSHA256) ||
				record.OriginalServiceTierPresent != test.wantOriginalTier || !record.ForwardedServiceTierPresent || record.ForwardedServiceTier != "default" ||
				record.OriginalRequestBodySHA256 != stableSHA256Bytes([]byte(test.body)) ||
				record.ForwardedRequestBodySHA256 != stableSHA256Bytes(upstreamBody) ||
				record.OriginalRequestWithoutServiceTierSHA256 != record.ForwardedRequestWithoutServiceTierSHA256 {
				t.Fatalf("service-tier transformation evidence = %#v", record)
			}
			if test.agentID == "codex" {
				if record.RequestedServiceTierPresent || record.RequestedServiceTier != "" || record.RequestedServiceTierCanonical != "default" ||
					record.RequestedServiceTierRepresentation != "client_canonicalized_default" || !isLowerHex64(record.ClientCanonicalizationStaticProofSHA256) ||
					record.OriginalRequestBodySHA256 == record.ForwardedRequestBodySHA256 {
					t.Fatalf("Codex omission evidence = %#v", record)
				}
			} else if record.ClientCanonicalizationStaticProofSHA256 != "" || record.OriginalRequestBodySHA256 != record.ForwardedRequestBodySHA256 ||
				record.OriginalRequestCanonicalSHA256 != record.ForwardedRequestCanonicalSHA256 {
				t.Fatalf("Luban no-op evidence = %#v", record)
			}
		})
	}
}

func TestServiceTierControllerRejectsEveryNonContractShapeBeforeUpstream(t *testing.T) {
	for _, test := range []struct {
		name      string
		agentID   string
		body      string
		wantError string
	}{
		{
			name: "Codex explicit default is not the frozen omission", agentID: "codex",
			body:      `{"model":"gpt-5.6-sol","service_tier":"default","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`,
			wantError: "service_tier_request_not_comparable",
		},
		{
			name: "Codex explicit nondefault is forbidden", agentID: "codex",
			body:      `{"model":"gpt-5.6-sol","service_tier":"auto","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`,
			wantError: "service_tier_request_not_comparable",
		},
		{
			name: "Codex duplicate tier is ambiguous", agentID: "codex",
			body:      `{"model":"gpt-5.6-sol","service_tier":"default","service_tier":"default","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`,
			wantError: "request_body_transformation_invalid",
		},
		{
			name: "Luban omission is forbidden", agentID: "luban",
			body:      `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`,
			wantError: "service_tier_request_not_comparable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstreamCalls := 0
			evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
			accessPath := "/" + strings.Repeat("r", 48)
			config := Config{
				Upstream: "https://api.openai.com", EvidencePath: evidencePath,
				AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh",
				AgentID: test.agentID, RunIdentity: testRunIdentity,
				Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					upstreamCalls++
					return nil, errors.New("forbidden upstream call")
				}),
			}
			if test.agentID == "codex" {
				config = configureCodexStaticProof(t, config)
			}
			handler, err := NewHandler(config)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(test.body)))
			if response.Code != http.StatusBadRequest || upstreamCalls != 0 {
				t.Fatalf("status=%d upstream_calls=%d body=%s", response.Code, upstreamCalls, response.Body.String())
			}
			records := readEvidenceRecords(t, evidencePath)
			if len(records) != 1 || records[0].ErrorCode != test.wantError || records[0].Disposition != "experiment_invalid" || records[0].ProviderAttemptStarted {
				t.Fatalf("rejected transformation records = %#v", records)
			}
		})
	}
}

func decodeTestJSONObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	decoded, err := decodeUniqueJSONObject(body)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestBindingHashesArePerHandlerKeyedReceipts(t *testing.T) {
	newHandler := func(marker string) *Handler {
		t.Helper()
		handler, err := NewHandler(Config{
			Upstream: "https://api.openai.com", EvidencePath: filepath.Join(t.TempDir(), marker+".jsonl"),
			AccessPath: "/" + strings.Repeat(marker, 48), Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
		})
		if err != nil {
			t.Fatal(err)
		}
		return handler
	}
	left := newHandler("h")
	right := newHandler("j")
	const secret = "same-provider-private-value"
	leftHash := left.hashBindingValue(secret)
	if leftHash == "" || leftHash != left.hashBindingValue(secret) {
		t.Fatal("binding receipt must be non-empty and deterministic within one handler")
	}
	if leftHash == right.hashBindingValue(secret) || leftHash == hashOpaque(secret) {
		t.Fatal("binding receipt must be per-handler HMAC, not a stable raw-value digest")
	}
	requestReceipt := inspectRequest([]byte(`{"input":[{"type":"message","id":"message-large-number","role":"assistant","status":"completed","content":[],"future_counter":9007199254740993}]}`), left.hashBindingValue)
	var responseItem map[string]any
	decoder := json.NewDecoder(strings.NewReader(`{"type":"message","id":"message-large-number","role":"assistant","status":"completed","content":[],"future_counter":9007199254740993}`))
	decoder.UseNumber()
	if err := decoder.Decode(&responseItem); err != nil {
		t.Fatal(err)
	}
	if len(requestReceipt.ReplayOutputItemHashes) != 1 || requestReceipt.ReplayOutputItemHashes[0] != left.hashBindingValue(responseItem) {
		t.Fatal("request and response canonicalization changed a large integer output item")
	}
}

func TestProxyCapturesCodex0145ApplyPatchAndShellCommittedItems(t *testing.T) {
	operation := map[string]any{"type": "update_file", "path": "private-target.go", "diff": "PRIVATE-DIFF"}
	action := map[string]any{"commands": []any{"go test ./...", "git diff --check"}, "timeout_ms": json.Number("120000"), "max_output_length": json.Number("4096")}
	responseOutputs := [][]any{
		{
			map[string]any{"id": "ap-item-1", "type": "apply_patch_call", "status": "completed", "call_id": "ap-call-1", "operation": operation},
			map[string]any{"id": "sh-item-1", "type": "shell_call", "status": "completed", "call_id": "sh-call-1", "action": action},
		},
		{},
	}
	upstreamCalls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		output := responseOutputs[upstreamCalls]
		upstreamCalls++
		payload, err := json.Marshal(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id": fmt.Sprintf("response-specialized-%d", upstreamCalls), "model": "gpt-5.6-sol", "status": "completed",
				"usage":  map[string]any{"input_tokens": 20, "input_tokens_details": map[string]any{"cached_tokens": 10}, "output_tokens": 5},
				"output": output,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{fmt.Sprintf("request-specialized-%d", upstreamCalls)}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier("data: " + string(payload) + "\n\n")))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("s", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	parameters := map[string]any{"type": "object", "properties": map[string]any{"cmd": map[string]any{"type": "string"}}, "required": []any{"cmd"}, "additionalProperties": false}
	tools := []any{
		map[string]any{"type": "apply_patch"},
		map[string]any{"type": "shell", "environment": map[string]any{"type": "local"}},
		map[string]any{"type": "function", "name": "Bash", "strict": true, "parameters": parameters},
	}
	send := func() *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{
			"model": "gpt-5.6-sol", "reasoning": map[string]any{"effort": "xhigh"}, "store": false, "tools": tools, "input": []any{},
		})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, string(body))))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := send(); response.Code != http.StatusOK {
		t.Fatalf("first specialized response status = %d", response.Code)
	}
	if response := send(); response.Code != http.StatusOK {
		t.Fatalf("headerless no-replay strategy status = %d", response.Code)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 2 || !records[0].ProtocolValid || !records[1].ProtocolValid {
		t.Fatalf("specialized records = %#v", records)
	}
	first := records[0]
	if first.ContinuationLineagePresent || first.ContinuationLineageSource != "controller_default" || len(first.ToolCalls) != 2 {
		t.Fatalf("specialized lineage/calls = %#v", first)
	}
	operationBytes, _ := toolPayloadBytes(operation, true)
	actionBytes, _ := toolPayloadBytes(action, true)
	if first.ToolCalls[0].Kind != "apply_patch_call" || first.ToolCalls[0].Name != "apply_patch" || first.ToolCalls[0].IDHash != hashOpaque("ap-call-1") || first.ToolCalls[0].InputBytes != operationBytes {
		t.Fatalf("apply_patch evidence = %#v", first.ToolCalls[0])
	}
	if first.ToolCalls[1].Kind != "shell_call" || first.ToolCalls[1].Name != "shell" || first.ToolCalls[1].IDHash != hashOpaque("sh-call-1") || first.ToolCalls[1].InputBytes != actionBytes {
		t.Fatalf("shell evidence = %#v", first.ToolCalls[1])
	}
	if first.ToolDefinitionCount != 3 || len(first.ToolDefinitions) != 3 || first.ToolDefinitions[0].Type != "apply_patch" || first.ToolDefinitions[0].SchemaHash != "" || first.ToolDefinitions[1].Type != "shell" || first.ToolDefinitions[2].Name != "Bash" || first.ToolDefinitions[2].Strict == nil || !*first.ToolDefinitions[2].Strict || first.ToolDefinitions[2].SchemaHash == "" {
		t.Fatalf("tool catalog evidence = %#v", first.ToolDefinitions)
	}
	canonicalSchema, _ := canonicalBindingValue(parameters)
	if first.ToolDefinitions[2].SchemaBytes != int64(len(canonicalSchema)) || first.ToolCatalogHash == "" || first.ToolCatalogCompared {
		t.Fatalf("tool catalog receipt = %#v", first)
	}
	if !records[1].ToolCatalogCompared || !records[1].ToolCatalogStable || !records[1].ContinuationResetUnknown {
		t.Fatalf("second-round strategy/catalog receipt = %#v", records[1])
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATE-DIFF", "private-target.go", "go test ./..."} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("specialized tool evidence leaked %q", secret)
		}
	}
}

func TestProxyRejectsUnknownTerminalOutputItemInsteadOfUndercounting(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-unknown-tool\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[{\"type\":\"future_magic_tool_call\",\"id\":\"future-item\",\"call_id\":\"future-call\",\"status\":\"completed\",\"payload\":{\"secret\":true}}]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-unknown-tool"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("z", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	records := readEvidenceRecords(t, evidencePath)
	if response.Code != http.StatusOK || len(records) != 1 || records[0].ProtocolValid || records[0].ErrorCode != "response_output_item_unknown" || len(records[0].ToolCalls) != 0 || records[0].ResponseOutputItemCount != 1 {
		t.Fatalf("unknown output receipt = %#v", records)
	}
}

func TestStreamCollectorRejectsOutputItemDoneAndCompletedSourceMismatch(t *testing.T) {
	record := Record{}
	handler, err := NewHandler(Config{
		Upstream: "https://api.openai.com", EvidencePath: filepath.Join(t.TempDir(), "evidence.jsonl"),
		AccessPath: "/" + strings.Repeat("o", 48), Credential: "secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	collector := newStreamCollector(&record, handler.hashBindingValue)
	collector.consume([]byte(`{"type":"response.output_item.done","item":{"type":"apply_patch_call","id":"item-one","call_id":"call-one","status":"completed","operation":{"type":"update_file","path":"a.go","diff":"old"}}}`))
	collector.consume([]byte(`{"type":"response.completed","response":{"id":"response-one","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens":1},"output":[{"type":"apply_patch_call","id":"item-one","call_id":"call-one","status":"completed","operation":{"type":"update_file","path":"a.go","diff":"new"}}]}}`))
	collector.Close()
	if record.ErrorCode != "response_output_item_source_mismatch" || record.ResponseOutputItemCount != 1 || len(record.ToolCalls) != 1 {
		t.Fatalf("mismatched output sources = %#v", record)
	}
}

func TestProxyMeasuresMissingReasoningContextAndEncryptedIncludeWithoutBias(t *testing.T) {
	upstreamCalls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-measured-strategy\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-measured-strategy"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("e", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || upstreamCalls != 1 {
		t.Fatalf("status=%d upstreamCalls=%d", response.Code, upstreamCalls)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 || !records[0].ProtocolValid || records[0].EncryptedReasoningRequested || records[0].RequestedReasoningContext != "" || records[0].ContinuationLineagePresent || records[0].ContinuationLineageSource != "controller_default" {
		t.Fatalf("measured strategy receipt = %#v", records)
	}
}

func TestProxyRejectsPreviousResponseIDInStatelessMode(t *testing.T) {
	upstreamCalls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		return nil, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("p", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"previous_response_id":"stored-chain-id","input":[]}`)))
	setContinuationHeaders(request, "lineage-previous-test", 1, false)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || upstreamCalls != 0 {
		t.Fatalf("status=%d upstreamCalls=%d", response.Code, upstreamCalls)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 || records[0].ErrorCode != "previous_response_id_forbidden_stateless" || !records[0].PreviousResponseIDPresent || records[0].PreviousResponseIDHash == "" || records[0].ProtocolValid {
		t.Fatalf("previous-response receipt = %#v", records)
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "stored-chain-id") {
		t.Fatalf("previous response ID leaked: %s", raw)
	}
}

func TestProxyUsageReceiptDoesNotCoerceMissingToZero(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-no-cache-field\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"input_tokens_details\":{},\"output_tokens\":0,\"output_tokens_details\":{\"reasoning_tokens\":0}},\"output\":[]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Request-Id": []string{"request-no-cache-field"}, "Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("u", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh","context":"all_turns"},"include":["reasoning.encrypted_content"],"store":false,"input":[]}`)))
	setContinuationHeaders(request, "lineage-usage-test", 1, false)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("forwarded status = %d", response.Code)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 {
		t.Fatalf("records = %d", len(records))
	}
	record := records[0]
	if record.UsagePresent || record.InputTokens != nil || record.CachedInputTokens != nil || record.CacheWriteInputTokens != nil || record.OutputTokens != nil || record.ProtocolValid || record.ErrorCode != "provider_usage_receipt_incomplete" || record.Disposition != "provider_infra_exclusion" {
		t.Fatalf("partial usage was not rejected atomically: %#v", record)
	}
	if record.ReasoningOutputTokens == nil || *record.ReasoningOutputTokens != 0 {
		t.Fatalf("explicit zero reasoning tokens lost: %#v", record.ReasoningOutputTokens)
	}
}

func TestCompleted2xxWithoutRequestIDIsHardNonScoreable(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-no-request-id\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("i", 48)
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	records := readEvidenceRecords(t, evidencePath)
	if response.Code != http.StatusOK || len(records) != 1 || records[0].ProtocolValid || records[0].ErrorCode != "incomplete_server_evidence" || records[0].Disposition != "provider_infra_exclusion" {
		t.Fatalf("missing request-id receipt = %#v", records)
	}
}

func TestRequestStartJournalFailurePreventsAnyUpstreamAttempt(t *testing.T) {
	upstreamCalls := 0
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	if err := os.Mkdir(AttemptJournalPath(evidencePath), 0o700); err != nil {
		t.Fatal(err)
	}
	accessPath := "/" + strings.Repeat("w", 48)
	handler, err := NewHandler(Config{
		Upstream: "https://api.openai.com", EvidencePath: evidencePath,
		Transport:  roundTripFunc(func(*http.Request) (*http.Response, error) { upstreamCalls++; return nil, nil }),
		AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || upstreamCalls != 0 || handler.PersistenceError() == nil {
		t.Fatalf("journal fail-closed status=%d upstream=%d persistence=%v", response.Code, upstreamCalls, handler.PersistenceError())
	}
}

func TestRecoveryNeverReplaysStartedAttemptWhoseFinalReceiptWasLost(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("k", 48)
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		// The request-start WAL is already fsynced when RoundTrip begins. Turn the
		// final receipt target into a directory to emulate an ENOSPC/write failure
		// after the provider accepted the request.
		if err := os.Mkdir(evidencePath, 0o700); err != nil {
			t.Fatal(err)
		}
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-lost-receipt\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":9,\"input_tokens_details\":{\"cached_tokens\":3},\"output_tokens\":2},\"output\":[]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-lost-receipt"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || handler.PersistenceError() == nil {
		t.Fatalf("lost-receipt response=%d persistence=%v", response.Code, handler.PersistenceError())
	}
	if err := os.Remove(evidencePath); err != nil {
		t.Fatal(err)
	}
	state, err := InspectAttemptRecoveryState(evidencePath, testRunIdentity, 0)
	if err != nil || state != AttemptRecoveryStartedUnsealed {
		t.Fatalf("recovery state=%q err=%v", state, err)
	}
	if err := handler.SealEvidence(); err == nil {
		t.Fatal("incomplete provider attempt unexpectedly produced a valid seal")
	}
	rawSeal, err := os.ReadFile(EvidenceSealPath(evidencePath))
	if err != nil {
		t.Fatal(err)
	}
	var seal EvidenceSeal
	if err := json.Unmarshal(rawSeal, &seal); err != nil {
		t.Fatal(err)
	}
	if !seal.Fatal || seal.StartedAttemptCount != 1 || seal.PersistedAttemptCount != 0 {
		t.Fatalf("fatal seal = %#v", seal)
	}
	zeroState, err := InspectAttemptRecoveryState(filepath.Join(t.TempDir(), "never-started.jsonl"), testRunIdentity, 0)
	if err != nil || zeroState != AttemptRecoveryZeroEvidence {
		t.Fatalf("zero-evidence recovery state=%q err=%v", zeroState, err)
	}
}

func TestEvidenceSealValidatesFsyncedHashChainAndAttemptCounts(t *testing.T) {
	evidencePath := filepath.Join(t.TempDir(), "evidence.jsonl")
	accessPath := "/" + strings.Repeat("v", 48)
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-sealed\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":0},\"output\":[]}}\n\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"request-sealed"}}, Body: io.NopCloser(strings.NewReader(withDefaultResponseServiceTier(body)))}, nil
	})
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: evidencePath, Transport: transport, AccessPath: accessPath, Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	state, err := InspectAttemptRecoveryState(evidencePath, testRunIdentity, 0)
	if err != nil || state != AttemptRecoverySealed {
		t.Fatalf("attempt state=%q err=%v", state, err)
	}
	if err := handler.SealEvidence(); err != nil {
		t.Fatal(err)
	}
	seal, err := ValidateEvidenceSeal(evidencePath, testRunIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if seal.Fatal || seal.RecordCount != 1 || seal.StartedAttemptCount != 1 || seal.PersistedAttemptCount != 1 || seal.LastEvidenceHash == "" {
		t.Fatalf("validated seal = %#v", seal)
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 || records[0].EvidenceSequence != 0 || records[0].EvidenceHash == "" || !records[0].ProviderAttemptStarted {
		t.Fatalf("hash-chain record = %#v", records)
	}

	rawEvidence, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	tamperedEvidence := bytes.Replace(rawEvidence, []byte(`"input_tokens":1`), []byte(`"input_tokens":2`), 1)
	if bytes.Equal(tamperedEvidence, rawEvidence) {
		t.Fatal("test fixture did not contain the token receipt to tamper with")
	}
	if err := os.WriteFile(evidencePath, tamperedEvidence, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEvidenceSeal(evidencePath, testRunIdentity); err == nil {
		t.Fatal("modified token receipt unexpectedly validated against the original seal")
	}
	if err := os.WriteFile(evidencePath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEvidenceSeal(evidencePath, testRunIdentity); err == nil {
		t.Fatal("deleted terminal evidence record unexpectedly validated against the original seal")
	}
	if err := os.WriteFile(evidencePath, rawEvidence, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(EvidenceSealPath(evidencePath)); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEvidenceSeal(evidencePath, testRunIdentity); err == nil {
		t.Fatal("missing evidence seal unexpectedly validated")
	}
}

func readEvidenceRecords(t *testing.T, path string) []Record {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []Record
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestProxyRejectsRequestsOutsideUnguessableResponsesPath(t *testing.T) {
	handler, err := NewHandler(Config{Upstream: "https://api.openai.com", EvidencePath: filepath.Join(t.TempDir(), "evidence.jsonl"), AccessPath: "/" + strings.Repeat("c", 48), Credential: "secret", ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("health status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unguarded provider path status = %d", response.Code)
	}
}

func TestHTTPRoundTripRecordsVerifiedTLSIdentityForApprovedOrigin(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("x-request-id", "tls-request-id")
		_, _ = io.WriteString(writer, withDefaultResponseServiceTier("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"tls-response-id\",\"model\":\"gpt-5.6-sol\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"input_tokens_details\":{\"cached_tokens\":0},\"output_tokens\":1},\"output\":[]}}\n\n"))
	}))
	defer upstream.Close()

	const approvedOrigin = "https://example.com"
	const semantics = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	evidencePath := filepath.Join(t.TempDir(), "provider-http.jsonl")
	accessPath := "/" + strings.Repeat("t", 48)
	handler, err := NewHandler(Config{
		Upstream: approvedOrigin, ApprovedOrigin: approvedOrigin,
		EndpointSemanticsSHA256: semantics, RequireTLSPeerEvidence: true,
		EvidencePath: evidencePath, Transport: upstream.Client().Transport,
		AccessPath: accessPath, Credential: "provider-secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)
	request := httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("proxy status=%d body=%s", response.Code, response.Body.String())
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 {
		t.Fatalf("record count=%d", len(records))
	}
	record := records[0]
	leaf := upstream.Certificate()
	if record.ApprovedOrigin != approvedOrigin || record.SemanticsSHA256 != semantics ||
		record.TLSServerName != "example.com" || !record.TLSVerified || record.TLSObservedAt.IsZero() || record.TLSObservedAt.Location() != time.UTC ||
		record.TLSPeerLeafCertSHA256 != hashOpaque(string(leaf.Raw)) ||
		record.TLSPeerSPKISHA256 != hashOpaque(string(leaf.RawSubjectPublicKeyInfo)) || !record.ProtocolValid {
		t.Fatalf("TLS provider evidence=%#v", record)
	}
	if err := handler.SealEvidence(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEvidenceSeal(evidencePath, testRunIdentity); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte(record.TLSPeerLeafCertSHA256), []byte(strings.Repeat("f", 64)), 1)
	if bytes.Equal(tampered, raw) {
		t.Fatal("TLS leaf fixture was not present in the sealed record")
	}
	if err := os.WriteFile(evidencePath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEvidenceSeal(evidencePath, testRunIdentity); err == nil {
		t.Fatal("tampered TLS peer evidence unexpectedly validated")
	}
}

func TestRequiredHTTPPeerEvidenceRejectsResponseWithoutTLSState(t *testing.T) {
	const approvedOrigin = "https://example.com"
	const semantics = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	evidencePath := filepath.Join(t.TempDir(), "provider-http.jsonl")
	accessPath := "/" + strings.Repeat("u", 48)
	handler, err := NewHandler(Config{
		Upstream: approvedOrigin, ApprovedOrigin: approvedOrigin,
		EndpointSemanticsSHA256: semantics, RequireTLSPeerEvidence: true,
		EvidencePath: evidencePath, AccessPath: accessPath, Credential: "provider-secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("unused"))}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	body := withDefaultServiceTier(t, `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"},"store":false,"input":[]}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, accessPath+"/v1/responses", strings.NewReader(body)))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	records := readEvidenceRecords(t, evidencePath)
	if len(records) != 1 || records[0].ErrorCode != "upstream_tls_peer_evidence" ||
		records[0].Disposition != "experiment_invalid" || records[0].TLSVerified ||
		records[0].ApprovedOrigin != approvedOrigin || records[0].SemanticsSHA256 != semantics {
		t.Fatalf("missing-TLS record=%#v", records)
	}
}

func TestDefaultHTTPTransportIsOwnedDirectAndTLS12Minimum(t *testing.T) {
	handler, err := NewHandler(Config{
		Upstream: "https://example.com", EvidencePath: filepath.Join(t.TempDir(), "evidence.jsonl"),
		AccessPath: "/" + strings.Repeat("v", 48), Credential: "provider-secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := handler.transport.(*http.Transport)
	if !ok || transport == http.DefaultTransport || transport.Proxy != nil || transport.TLSClientConfig == nil ||
		transport.TLSClientConfig.MinVersion < tls.VersionTLS12 || transport.TLSClientConfig.ServerName != "example.com" {
		t.Fatalf("default provider transport=%#v", handler.transport)
	}
}

func TestRequiredTLSPeerProjectionRejectsIncompleteOrUnboundState(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer upstream.Close()
	const approvedOrigin = "https://example.com"
	handler, err := NewHandler(Config{
		Upstream: approvedOrigin, ApprovedOrigin: approvedOrigin,
		EndpointSemanticsSHA256: strings.Repeat("d", 64), RequireTLSPeerEvidence: true,
		EvidencePath: filepath.Join(t.TempDir(), "evidence.jsonl"), Transport: upstream.Client().Transport,
		AccessPath: "/" + strings.Repeat("x", 48), Credential: "provider-secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaf := upstream.Certificate()
	valid := tls.ConnectionState{
		Version: tls.VersionTLS13, HandshakeComplete: true, ServerName: "example.com",
		PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}},
	}
	if evidence, err := handler.projectTLSPeerEvidence(&valid); err != nil || !evidence.TLSVerified {
		t.Fatalf("valid TLS projection evidence=%#v err=%v", evidence, err)
	}
	tests := map[string]func(*tls.ConnectionState){
		"handshake incomplete": func(state *tls.ConnectionState) { state.HandshakeComplete = false },
		"old TLS version":      func(state *tls.ConnectionState) { state.Version = tls.VersionTLS11 },
		"server name drift":    func(state *tls.ConnectionState) { state.ServerName = "other.invalid" },
		"peer absent":          func(state *tls.ConnectionState) { state.PeerCertificates = nil },
		"chain absent":         func(state *tls.ConnectionState) { state.VerifiedChains = nil },
		"chain leaf drift": func(state *tls.ConnectionState) {
			other := *leaf
			other.Raw = append([]byte(nil), leaf.Raw...)
			other.Raw[0] ^= 0xff
			state.VerifiedChains = [][]*x509.Certificate{{&other}}
		},
		"hostname mismatch": func(state *tls.ConnectionState) {
			other := *leaf
			other.DNSNames = []string{"other.invalid"}
			other.IPAddresses = nil
			state.PeerCertificates = []*x509.Certificate{&other}
			state.VerifiedChains = [][]*x509.Certificate{{&other}}
		},
		"leaf DER absent": func(state *tls.ConnectionState) {
			other := *leaf
			other.Raw = nil
			state.PeerCertificates = []*x509.Certificate{&other}
			state.VerifiedChains = [][]*x509.Certificate{{&other}}
		},
		"leaf SPKI absent": func(state *tls.ConnectionState) {
			other := *leaf
			other.RawSubjectPublicKeyInfo = nil
			state.PeerCertificates = []*x509.Certificate{&other}
			state.VerifiedChains = [][]*x509.Certificate{{&other}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if _, err := handler.projectTLSPeerEvidence(&candidate); err == nil {
				t.Fatal("invalid TLS state unexpectedly projected")
			}
		})
	}
	if _, err := handler.projectTLSPeerEvidence(nil); err == nil {
		t.Fatal("missing TLS state unexpectedly projected")
	}
}

func TestRequiredTLSPeerEvidenceRejectsEndpointAuthorityDrift(t *testing.T) {
	base := Config{
		Upstream: "https://example.com", ApprovedOrigin: "https://example.com",
		EndpointSemanticsSHA256: strings.Repeat("e", 64), RequireTLSPeerEvidence: true,
		EvidencePath: filepath.Join(t.TempDir(), "evidence.jsonl"),
		AccessPath:   "/" + strings.Repeat("y", 48), Credential: "provider-secret",
		ExpectedModel: "gpt-5.6-sol", ExpectedEffort: "xhigh", RunIdentity: testRunIdentity,
	}
	tests := map[string]func(*Config){
		"origin mismatch":        func(value *Config) { value.ApprovedOrigin = "https://other.invalid" },
		"upstream path":          func(value *Config) { value.Upstream += "/base" },
		"upstream query":         func(value *Config) { value.Upstream += "?region=other" },
		"approved userinfo":      func(value *Config) { value.ApprovedOrigin = "https://user@example.com" },
		"semantics uppercase":    func(value *Config) { value.EndpointSemanticsSHA256 = strings.Repeat("A", 64) },
		"semantics missing":      func(value *Config) { value.EndpointSemanticsSHA256 = "" },
		"approved origin absent": func(value *Config) { value.ApprovedOrigin = "" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if _, err := NewHandler(candidate); err == nil {
				t.Fatal("endpoint authority drift unexpectedly accepted")
			}
		})
	}
}
