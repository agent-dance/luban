package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/benchmark/agentic/pilot"
	"github.com/agent-dance/luban/benchmark/agentic/report"
	"github.com/agent-dance/luban/i18n"
)

func TestFrozenDevelopmentPricingMatchesCheckedInPilotManifest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "manifests", "deepswe-v1.1-pilot.template.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest harness.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest.Pricing, frozenDevelopmentPricing) {
		t.Fatalf("development pricing drifted from checked-in pilot manifest:\nreport=%#v\nmanifest=%#v", frozenDevelopmentPricing, manifest.Pricing)
	}
}

func TestRunMainGeneratesFiveTaskNonFormalDevelopmentReport(t *testing.T) {
	paths := writePilotLedgerForTest(t, nil)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runMain([]string{
		"--ledger", paths.ledger,
		"--output", paths.input,
		"--html", paths.html,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runMain exit code = %d, stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 || !strings.Contains(stdout.String(), paths.html) {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	ledgerRaw, err := os.ReadFile(paths.ledger)
	if err != nil {
		t.Fatal(err)
	}
	var ledgerTop map[string]json.RawMessage
	if err := json.Unmarshal(ledgerRaw, &ledgerTop); err != nil {
		t.Fatal(err)
	}
	if got := string(ledgerTop["formal_compatible"]); got != "false" {
		t.Fatalf("source ledger formal_compatible = %s, want false", got)
	}

	var input report.Input
	decodeJSONFileForTest(t, paths.input, &input)
	if input.Report.BenchmarkContractID != report.BenchmarkContractDeepSWEV11Pilot5 {
		t.Fatalf("benchmark_contract_id = %q, want %q", input.Report.BenchmarkContractID, report.BenchmarkContractDeepSWEV11Pilot5)
	}
	if len(input.ArtifactSources) != 0 || len(input.PublicReferences) != 0 {
		t.Fatalf("development report invoked a formal source/scorer: artifact_sources=%d public_references=%d", len(input.ArtifactSources), len(input.PublicReferences))
	}
	if len(input.DiagnosticExperiments) != 1 || input.DiagnosticExperiments[0].Class != report.ClassDiagnosticCanary {
		t.Fatalf("diagnostic experiments = %#v", input.DiagnosticExperiments)
	}
	assertExactPilotMatrixForTest(t, input.DiagnosticExperiments[0].Runs)
	for _, run := range input.DiagnosticExperiments[0].Runs {
		if run.Metrics.ComparableCost == nil || run.Metrics.ComparableCostBasis != comparableCostBasis {
			t.Fatalf("run %s/%s comparable cost = %v (%q)", run.TaskID, run.AgentID, run.Metrics.ComparableCost, run.Metrics.ComparableCostBasis)
		}
		if run.Metrics.WallTimeSeconds == nil || *run.Metrics.WallTimeSeconds != 16 || run.Metrics.TrialDurationSeconds == nil || *run.Metrics.TrialDurationSeconds != 23 {
			t.Fatalf("run %s/%s timing does not come from execution timestamps: wall=%v trial=%v", run.TaskID, run.AgentID, run.Metrics.WallTimeSeconds, run.Metrics.TrialDurationSeconds)
		}
		if run.Metrics.CacheWriteInputTokens != nil {
			t.Fatalf("run %s/%s fabricated missing cache-write tokens: %d", run.TaskID, run.AgentID, *run.Metrics.CacheWriteInputTokens)
		}
		if run.Metrics.PhysicalToolOperations != nil || run.Metrics.ToolCriticalPathMS != nil || run.Metrics.ToolTotalLatencyMS != nil || run.Metrics.ToolQueueMS != nil || run.Metrics.ReasoningOutputTokens != nil || run.Metrics.ToolErrors != nil {
			t.Fatalf("run %s/%s fabricated unobserved optional telemetry: %#v", run.TaskID, run.AgentID, run.Metrics)
		}
		if run.Metrics.LLMCallsStarted == nil || *run.Metrics.LLMCallsStarted != 1 || run.Metrics.ToolInvocations == nil || *run.Metrics.ToolInvocations != 0 {
			t.Fatalf("run %s/%s lost evidence-derived structural counters: llm=%v tools=%v", run.TaskID, run.AgentID, run.Metrics.LLMCallsStarted, run.Metrics.ToolInvocations)
		}
	}

	htmlRaw, err := os.ReadFile(paths.html)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(htmlRaw)
	for _, required := range []string{
		"<!doctype html>",
		`<body class="diagnostic-only">`,
		`class="watermark"`,
		"formal_compatible=false",
		report.BenchmarkContractDeepSWEV11Pilot5,
		"codex",
		"luban",
		pilot.ExactTaskIDs[0],
		"llm_calls_started",
		comparableCostBasis,
		"$0.009472",
		"<style>",
	} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("rendered report is missing %q", required)
		}
	}
	for _, forbidden := range []string{`<script src=`, `<link rel="stylesheet"`} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered report has an external runtime dependency marker %q", forbidden)
		}
	}
	for _, language := range i18n.AllLanguages() {
		if label := i18n.Text(language, i18n.KeyAgenticReportGateProjectionIntegrity); strings.Contains(rendered, label) {
			t.Fatalf("development report rendered the formal projection gate %q", label)
		}
	}
}

func TestDevelopmentComparableCostDoesNotRequireCacheWriteCoverage(t *testing.T) {
	metrics := &harness.UsageMetrics{
		TransportAttempts:                       2,
		CostReceiptTotal:                        2,
		AllExecutedUsageObservations:            2,
		UnknownCostAttempts:                     2,
		AllExecutedUnreportedCacheWriteAttempts: 2,
		// The sealed lower bound already contains the per-request long-context
		// multipliers. The known surcharge is removed from the non-billing view.
		KnownCatalogCostLowerBound: 2.75,
		KnownCacheWriteSurcharge:   0.25,
	}
	cost := developmentComparableCost(metrics)
	if cost == nil || *cost != 2.5 {
		t.Fatalf("development comparable cost = %v, want 2.5", cost)
	}
}

func TestDevelopmentComparableCostPreservesFrozenPerRequestLongContextTier(t *testing.T) {
	metrics := &harness.UsageMetrics{
		TransportAttempts:            2,
		CostReceiptTotal:             2,
		AllExecutedUsageObservations: 2,
		AllExecutedInputTokens:       544001,
		AllExecutedOutputTokens:      3000,
		// This is the frozen per-request projection for requests at 272000
		// and 272001 input tokens. Only the latter receives the long-context
		// multipliers; aggregation must not re-tier their combined token count.
		KnownCatalogCostLowerBound: 2.1550125,
		KnownCacheWriteSurcharge:   0.2050025,
	}
	cost := developmentComparableCost(metrics)
	if cost == nil || math.Abs(*cost-1.95001) > 1e-12 {
		t.Fatalf("long-context development comparable cost = %v, want 1.95001", cost)
	}
}

func TestDevelopmentComparableCostRequiresAllExecutedUsage(t *testing.T) {
	metrics := &harness.UsageMetrics{
		TransportAttempts:            2,
		CostReceiptTotal:             2,
		AllExecutedUsageObservations: 1,
		KnownCatalogCostLowerBound:   1,
	}
	if cost := developmentComparableCost(metrics); cost != nil {
		t.Fatalf("incomplete usage produced development comparable cost %v", *cost)
	}
}

func TestRunMainRejectsInvalidDevelopmentLedgers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*pilot.Ledger)
		want   string
	}{
		{
			name: "formal compatible",
			mutate: func(ledger *pilot.Ledger) {
				ledger.FormalCompatible = true
			},
			want: "ledger_identity_or_completion_invalid",
		},
		{
			name: "not exact five task matrix",
			mutate: func(ledger *pilot.Ledger) {
				for key, run := range ledger.Runs {
					run.Entry.TaskID = "unregistered-sixth-task"
					ledger.Runs[key] = run
					break
				}
			},
			want: "run_matrix_invalid",
		},
		{
			name: "unsealed run",
			mutate: func(ledger *pilot.Ledger) {
				for key, run := range ledger.Runs {
					run.State = "reserved"
					ledger.Runs[key] = run
					break
				}
			},
			want: "run_not_sealed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := writePilotLedgerForTest(t, test.mutate)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := runMain([]string{
				"--ledger", paths.ledger,
				"--output", paths.input,
				"--html", paths.html,
			}, &stdout, &stderr)
			if code != 1 {
				t.Fatalf("runMain exit code = %d, want 1; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr=%q, want error containing %q", stderr.String(), test.want)
			}
			if _, err := os.Stat(paths.input); !os.IsNotExist(err) {
				t.Fatalf("rejected ledger produced report input: stat error=%v", err)
			}
			if _, err := os.Stat(paths.html); !os.IsNotExist(err) {
				t.Fatalf("rejected ledger produced HTML: stat error=%v", err)
			}
		})
	}
}

func TestRunMainRejectsTamperedDevelopmentReceiptsAndEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *pilot.Ledger)
		want   string
	}{
		{
			name: "duplicate receipt key with refreshed digest",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				key, run := firstPilotRunForTest(t, ledger)
				raw := readFileForTest(t, run.ReceiptPath)
				raw = append([]byte(`{"schema_version":"shadow",`), raw[1:]...)
				writeFileForTest(t, run.ReceiptPath, raw)
				run.ReceiptSHA256 = hashFileForTest(t, run.ReceiptPath)
				ledger.Runs[key] = run
			},
			want: "attempt_receipt_duplicate_key",
		},
		{
			name: "receipt raw bytes changed without refreshed digest",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				_, run := firstPilotRunForTest(t, ledger)
				raw := append(readFileForTest(t, run.ReceiptPath), ' ')
				writeFileForTest(t, run.ReceiptPath, raw)
			},
			want: "run_receipt_digest_invalid",
		},
		{
			name: "unknown receipt field with refreshed digest",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				key, run := firstPilotRunForTest(t, ledger)
				raw := readFileForTest(t, run.ReceiptPath)
				raw = append([]byte(`{"future_receipt_field":true,`), raw[1:]...)
				writeFileForTest(t, run.ReceiptPath, raw)
				run.ReceiptSHA256 = hashFileForTest(t, run.ReceiptPath)
				ledger.Runs[key] = run
			},
			want: "attempt_receipt_decode_invalid",
		},
		{
			name: "trailing receipt value with refreshed digest",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				key, run := firstPilotRunForTest(t, ledger)
				raw := append(readFileForTest(t, run.ReceiptPath), []byte("{}\n")...)
				writeFileForTest(t, run.ReceiptPath, raw)
				run.ReceiptSHA256 = hashFileForTest(t, run.ReceiptPath)
				ledger.Runs[key] = run
			},
			want: "attempt_receipt_trailing_json",
		},
		{
			name: "receipt metrics disagree with ledger",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				key, run := firstPilotRunForTest(t, ledger)
				receipt := readAttemptReceiptForTest(t, run.ReceiptPath)
				receipt.Metrics.LLMCallsStarted++
				writeJSONFileForTest(t, run.ReceiptPath, receipt)
				run.ReceiptSHA256 = hashFileForTest(t, run.ReceiptPath)
				ledger.Runs[key] = run
			},
			want: "run_receipt_ledger_mismatch",
		},
		{
			name: "evidence usage changed after sealing",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				_, run := firstPilotRunForTest(t, ledger)
				round := readEvidenceRoundForTest(t, run.NormalizedEvidencePath)
				changed := int64(4096)
				round.InputTokens = &changed
				writeJSONLineForTest(t, run.NormalizedEvidencePath, round)
			},
			want: "run_evidence_metrics_mismatch",
		},
		{
			name: "duplicate evidence key",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				_, run := firstPilotRunForTest(t, ledger)
				raw := readFileForTest(t, run.NormalizedEvidencePath)
				raw = append([]byte(`{"schema_version":"shadow",`), raw[1:]...)
				writeFileForTest(t, run.NormalizedEvidencePath, raw)
			},
			want: "provider_evidence_duplicate_key",
		},
		{
			name: "unknown evidence field",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				_, run := firstPilotRunForTest(t, ledger)
				raw := readFileForTest(t, run.NormalizedEvidencePath)
				raw = append([]byte(`{"future_evidence_field":true,`), raw[1:]...)
				writeFileForTest(t, run.NormalizedEvidencePath, raw)
			},
			want: "provider_evidence_decode_invalid",
		},
		{
			name: "trailing evidence value on line",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				_, run := firstPilotRunForTest(t, ledger)
				raw := bytes.TrimSpace(readFileForTest(t, run.NormalizedEvidencePath))
				raw = append(raw, []byte("{}\n")...)
				writeFileForTest(t, run.NormalizedEvidencePath, raw)
			},
			want: "provider_evidence_trailing_json",
		},
		{
			name: "invalid execution timing bound in both copies",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				key, run := firstPilotRunForTest(t, ledger)
				receipt := readAttemptReceiptForTest(t, run.ReceiptPath)
				receipt.Execution.FinishedAt = receipt.Execution.StartedAt.Add(-time.Second)
				run.Execution = &receipt.Execution
				writeJSONFileForTest(t, run.ReceiptPath, receipt)
				run.ReceiptSHA256 = hashFileForTest(t, run.ReceiptPath)
				ledger.Runs[key] = run
			},
			want: "run_execution_timing_invalid",
		},
		{
			name: "receipt symlink",
			mutate: func(t *testing.T, ledger *pilot.Ledger) {
				_, run := firstPilotRunForTest(t, ledger)
				target := run.ReceiptPath + ".original"
				if err := os.Rename(run.ReceiptPath, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, run.ReceiptPath); err != nil {
					t.Fatal(err)
				}
			},
			want: "artifact_path_symlink",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			ledger := validPilotLedgerForTest(t, root)
			test.mutate(t, &ledger)
			paths := writePilotLedgerValueForTest(t, root, ledger)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := runMain([]string{"--ledger", paths.ledger, "--output", paths.input, "--html", paths.html}, &stdout, &stderr)
			if code != 1 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("runMain exit=%d stdout=%q stderr=%q, want rejection containing %q", code, stdout.String(), stderr.String(), test.want)
			}
			if _, err := os.Stat(paths.input); !os.IsNotExist(err) {
				t.Fatalf("tampered run produced report input: stat error=%v", err)
			}
		})
	}
}

type developmentReportPaths struct {
	ledger string
	input  string
	html   string
}

func writePilotLedgerForTest(t *testing.T, mutate func(*pilot.Ledger)) developmentReportPaths {
	t.Helper()
	root := t.TempDir()
	ledger := validPilotLedgerForTest(t, root)
	if mutate != nil {
		mutate(&ledger)
	}
	return writePilotLedgerValueForTest(t, root, ledger)
}

func writePilotLedgerValueForTest(t *testing.T, root string, ledger pilot.Ledger) developmentReportPaths {
	t.Helper()
	paths := developmentReportPaths{
		ledger: filepath.Join(root, pilot.LedgerRelativePath),
		input:  filepath.Join(root, "report-input.json"),
		html:   filepath.Join(root, "development-report.html"),
	}
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ledger, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return paths
}

func validPilotLedgerForTest(t *testing.T, root string) pilot.Ledger {
	t.Helper()
	started := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	completed := started.Add(10 * time.Minute)
	ledger := pilot.Ledger{
		SchemaVersion:    pilot.LedgerSchemaVersion,
		FormalCompatible: false,
		Status:           "complete",
		ManifestSHA256:   strings.Repeat("a", 64),
		PlanSHA256:       strings.Repeat("b", 64),
		StartedAt:        started,
		UpdatedAt:        completed,
		CompletedAt:      &completed,
		Storage:          pilot.StoragePaths{FormalCompatible: false},
		Oracle:           map[string]pilot.OracleRecord{},
		Runs:             make(map[string]pilot.RunRecord, len(pilot.ExactTaskIDs)*2),
	}
	ordinal := 0
	for taskIndex, taskID := range pilot.ExactTaskIDs {
		for _, agentID := range []string{"codex", "luban"} {
			entry := harness.PlanEntry{
				Ordinal: ordinal, PairID: "pair-" + taskID, TaskID: taskID,
				AgentID: agentID, Repetition: 0,
			}
			reserved := started.Add(time.Duration(taskIndex*2+ordinal%2) * time.Minute)
			sealed := reserved.Add(30 * time.Second)
			artifactDir := filepath.Join(root, "runs", taskID, agentID)
			receiptPath := filepath.Join(artifactDir, pilot.AttemptReceiptName)
			evidencePath := filepath.Join(artifactDir, "metrics", "provider-requests.jsonl")
			if err := os.MkdirAll(filepath.Dir(evidencePath), 0o700); err != nil {
				t.Fatal(err)
			}
			model, definitions := developmentModelForTest(agentID)
			round := developmentEvidenceRoundForTest(agentID, model, definitions, reserved.Add(5*time.Second))
			evidenceRaw, err := json.Marshal(round)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(evidencePath, append(evidenceRaw, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			metrics, err := harness.ValidateAndAggregateEvidence([]harness.ProviderRoundEvidence{round}, model, frozenDevelopmentPricing)
			if err != nil {
				t.Fatal(err)
			}
			verification := harness.VerificationResult{ProtocolValid: true, Reward: float64((taskIndex + ordinal) % 2)}
			execution := harness.AgentExecution{
				Lifecycle: harness.AttemptLifecycle{
					SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: round.RunIdentity,
					ControllerStartedAt: reserved.Add(time.Second), ControllerFinishedAt: reserved.Add(26 * time.Second),
					ProviderAttemptState: "provider_attempt_sealed", ProviderAttemptCount: 1,
				},
				ExitClass: "completed", StartedAt: reserved.Add(4 * time.Second), FinishedAt: reserved.Add(20 * time.Second),
				TrialStartedAt: reserved.Add(2 * time.Second), TrialFinishedAt: reserved.Add(25 * time.Second),
				EvidencePath: evidencePath, EvidenceRunIdentity: round.RunIdentity,
				ProviderEvidence: harness.ProviderEvidenceSeal{
					StartedAttemptCount: 1, PersistedAttemptCount: 1, RecordCount: 1, LastEvidenceHash: round.EvidenceHash,
				},
				Verification: &verification,
			}
			receipt := pilot.AttemptReceipt{
				SchemaVersion: pilot.AttemptReceiptSchemaVersion, FormalCompatible: false,
				ManifestSHA256: ledger.ManifestSHA256, PlanSHA256: ledger.PlanSHA256,
				RunKey: harness.RunKey(entry), Entry: entry, Model: model, SealedAt: sealed,
				Execution: execution, Verification: verification, Metrics: metrics,
			}
			receiptRaw, err := json.MarshalIndent(receipt, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(receiptPath, append(receiptRaw, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			receiptSHA, err := harness.HashFile(receiptPath)
			if err != nil {
				t.Fatal(err)
			}
			ledger.Runs[harness.RunKey(entry)] = pilot.RunRecord{
				Entry: entry, State: "sealed", AttemptNumber: 1, ReservedAt: reserved, SealedAt: sealed,
				ArtifactDir: artifactDir, ReceiptPath: receiptPath, ReceiptSHA256: receiptSHA,
				Model: model, Execution: &execution, Verification: &verification,
				NormalizedEvidencePath: evidencePath, Metrics: &metrics,
			}
			ordinal++
		}
	}
	return ledger
}

func developmentModelForTest(agentID string) (harness.ModelRequestSpec, []harness.ToolDefinitionEvidence) {
	identities := []harness.ToolIdentitySpec{
		{Type: "function", Name: "Inspect", DefinitionSHA256: strings.Repeat("a", 64)},
		{Type: "function", Name: "ApplyPatch", DefinitionSHA256: strings.Repeat("b", 64)},
		{Type: "function", Name: "Run", DefinitionSHA256: strings.Repeat("c", 64)},
	}
	encoding := harness.ServiceTierEncodingExplicitDefault
	if agentID == "codex" {
		encoding = harness.ServiceTierEncodingClientCanonical
		identities = []harness.ToolIdentitySpec{
			{Type: "custom", Name: "exec", DefinitionSHA256: strings.Repeat("a", 64)},
			{Type: "function", Name: "wait", DefinitionSHA256: strings.Repeat("b", 64)},
			{Type: "function", Name: "request_user_input", DefinitionSHA256: strings.Repeat("c", 64)},
		}
	}
	definitions := make([]harness.ToolDefinitionEvidence, 0, len(identities))
	for index, identity := range identities {
		definitions = append(definitions, harness.ToolDefinitionEvidence{
			Type: identity.Type, Name: identity.Name, BillingOwner: "client",
			SchemaHash: strings.Repeat(string(rune('1'+index)), 64), SchemaSHA256: strings.Repeat(string(rune('4'+index)), 64), SchemaBytes: 1,
			DefinitionSHA256: identity.DefinitionSHA256, DefinitionBytes: 1,
		})
	}
	model := harness.ModelRequestSpec{
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh",
		ServiceTier: harness.FormalServiceTier, ServiceTierRequestEncoding: encoding,
		TransportRequirement: harness.TransportRequirementHTTPInference,
		ToolCatalog: harness.ToolCatalogSpec{
			SchemaVersion:  harness.FormalToolCatalogSchemaVersion,
			SemanticSHA256: harness.StableToolCatalogSHA256(definitions), Tools: identities,
		},
	}
	return model, definitions
}

func developmentEvidenceRoundForTest(agentID string, model harness.ModelRequestSpec, definitions []harness.ToolDefinitionEvidence, started time.Time) harness.ProviderRoundEvidence {
	input, cached, output := int64(2048), int64(1024), int64(128)
	round := harness.ProviderRoundEvidence{
		SchemaVersion: "agentic-bench/provider-round-v2", EvidenceSequence: 0,
		EvidenceHash: strings.Repeat("8", 64), Round: 0, RunIdentity: strings.Repeat("f", 64),
		ProviderAttemptStarted: true, Transport: "http_sse", ProviderAttemptKind: "inference",
		WebSocketChainBound: true, TransportDisposition: "valid", Outcome: "success",
		RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
		StartedAt: started, UpstreamHeadersAt: started.Add(100 * time.Millisecond),
		FirstResponseByteAt: started.Add(200 * time.Millisecond), FinishedAt: started.Add(time.Second),
		Provider: model.Provider, Model: model.Model, ReasoningEffort: model.ReasoningEffort,
		RequestedReasoningContext: "all_turns", RequestedReasoningModeCanonical: "standard",
		StoreSpecified: true, EncryptedReasoningRequested: true,
		ContinuationLineagePresent: true, ContinuationLineageHash: strings.Repeat("e", 64),
		ContinuationLineageSource: "agent_header", ContinuationEpoch: 1,
		RequestedServiceTierCanonical:      harness.FormalServiceTier,
		RequestedServiceTierRepresentation: model.ServiceTierRequestEncoding, ClientAgentID: agentID,
		OriginalRequestBodySHA256: strings.Repeat("1", 64), OriginalRequestCanonicalSHA256: strings.Repeat("2", 64),
		OriginalRequestWithoutServiceTierSHA256: strings.Repeat("3", 64), ForwardedRequestWithoutServiceTierSHA256: strings.Repeat("3", 64),
		ForwardedServiceTierPresent: true, ForwardedServiceTier: harness.FormalServiceTier,
		ServiceTierTransformationExactDiff: true, ServiceTierTransformationProofSHA256: strings.Repeat("6", 64),
		ResponseServiceTierRaw: harness.FormalServiceTier, ResponseServiceTierCanonical: harness.FormalServiceTier, ServiceTierComparable: true,
		ToolDefinitionCount: len(definitions), ToolDefinitions: definitions, ToolCatalogHash: strings.Repeat("c", 64),
		ToolCatalogSemanticSHA256: harness.StableToolCatalogSHA256(definitions), ToolCatalogCanonicalBytes: int64(len(definitions)),
		ToolCatalogStable: true, ToolResultHistoryValid: true,
		ResponseModel: model.Model, ResponseCompleted: true, ResponseStatus: "completed",
		HTTPStatus: 200, RequestBytes: 1000, ResponseBytes: 100,
		UsagePresent: true, InputTokens: &input, CachedInputTokens: &cached, OutputTokens: &output,
	}
	if agentID == "codex" {
		round.ClientCanonicalizationProofSHA256 = strings.Repeat("7", 64)
		round.ForwardedRequestBodySHA256 = strings.Repeat("4", 64)
		round.ForwardedRequestCanonicalSHA256 = strings.Repeat("5", 64)
		round.ForwardedRequestBytes = round.RequestBytes + 1
		round.ServiceTierTransformation = "inject_explicit_default"
	} else {
		round.RequestedServiceTierRaw = harness.FormalServiceTier
		round.RequestedServiceTierPresent = true
		round.OriginalServiceTierPresent = true
		round.OriginalServiceTier = harness.FormalServiceTier
		round.ForwardedRequestBodySHA256 = round.OriginalRequestBodySHA256
		round.ForwardedRequestCanonicalSHA256 = round.OriginalRequestCanonicalSHA256
		round.ForwardedRequestBytes = round.RequestBytes
		round.ServiceTierTransformation = "none"
	}
	return round
}

func firstPilotRunForTest(t *testing.T, ledger *pilot.Ledger) (string, pilot.RunRecord) {
	t.Helper()
	keys := make([]string, 0, len(ledger.Runs))
	for key := range ledger.Runs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		t.Fatal("pilot ledger has no runs")
	}
	return keys[0], ledger.Runs[keys[0]]
}

func readFileForTest(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeFileForTest(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hashFileForTest(t *testing.T, path string) string {
	t.Helper()
	digest, err := harness.HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func readAttemptReceiptForTest(t *testing.T, path string) pilot.AttemptReceipt {
	t.Helper()
	var receipt pilot.AttemptReceipt
	if err := json.Unmarshal(readFileForTest(t, path), &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func readEvidenceRoundForTest(t *testing.T, path string) harness.ProviderRoundEvidence {
	t.Helper()
	var round harness.ProviderRoundEvidence
	if err := json.Unmarshal(readFileForTest(t, path), &round); err != nil {
		t.Fatal(err)
	}
	return round
}

func writeJSONFileForTest(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, path, append(raw, '\n'))
}

func writeJSONLineForTest(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFileForTest(t, path, append(raw, '\n'))
}

func decodeJSONFileForTest(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}

func assertExactPilotMatrixForTest(t *testing.T, runs []report.DiagnosticRun) {
	t.Helper()
	if len(runs) != len(pilot.ExactTaskIDs)*2 {
		t.Fatalf("development runs = %d, want %d", len(runs), len(pilot.ExactTaskIDs)*2)
	}
	seen := make(map[string]map[string]bool, len(pilot.ExactTaskIDs))
	for _, run := range runs {
		if seen[run.TaskID] == nil {
			seen[run.TaskID] = map[string]bool{}
		}
		seen[run.TaskID][run.AgentID] = true
	}
	if len(seen) != len(pilot.ExactTaskIDs) {
		t.Fatalf("unique development tasks = %d, want %d: %v", len(seen), len(pilot.ExactTaskIDs), seen)
	}
	for _, taskID := range pilot.ExactTaskIDs {
		if !slices.Equal(sortedAgentsForTest(seen[taskID]), []string{"codex", "luban"}) {
			t.Fatalf("task %q agents = %v, want [codex luban]", taskID, sortedAgentsForTest(seen[taskID]))
		}
	}
}

func sortedAgentsForTest(agents map[string]bool) []string {
	result := make([]string, 0, len(agents))
	for agent := range agents {
		result = append(result, agent)
	}
	slices.Sort(result)
	return result
}
