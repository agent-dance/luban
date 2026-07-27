package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/i18n"
)

const (
	diagnosticInputFixture  = "testdata/diagnostic-input.json"
	diagnosticLedgerFixture = "testdata/diagnostic-optimization-ledger.json"
)

func TestDiagnosticFixtureCompileAndRenderDeterministically(t *testing.T) {
	first, err := Compile(diagnosticInputFixture)
	if err != nil {
		t.Fatalf("Compile(first): %v", err)
	}
	second, err := Compile(diagnosticInputFixture)
	if err != nil {
		t.Fatalf("Compile(second): %v", err)
	}

	if !first.HasDiagnosticOnly || first.HasFormal || first.HasPilot {
		t.Fatalf("diagnostic evidence classification = diagnostic-only:%t formal:%t pilot:%t", first.HasDiagnosticOnly, first.HasFormal, first.HasPilot)
	}
	if first.Verdict.Status != "insufficient" {
		t.Fatalf("diagnostic verdict = %q, want insufficient", first.Verdict.Status)
	}
	if len(first.Experiments) == 0 {
		t.Fatal("diagnostic fixture compiled without experiments")
	}
	if first.PublicReferences[0].Score == nil || *first.PublicReferences[0].Score != float64(*first.PublicReferences[0].Passed)/float64(*first.PublicReferences[0].Total) {
		t.Fatal("public reference score was not derived exactly from its pinned pass counts")
	}
	computed := first.PublicReferences[0].Computed
	if computed == nil || computed.Provenance.SourceSHA256 != harness.DeepSWEGPT56SolXHighSourceSHA256 || computed.Provenance.ProjectionSHA256 != harness.DeepSWEGPT56SolXHighProjectionSHA256 || computed.RawRows != 452 || computed.ScoredRows != 451 || computed.ExcludedRows != 1 || computed.PassedRows != 319 {
		t.Fatalf("computed public reference provenance/counts = %#v", computed)
	}
	for _, experiment := range first.Experiments {
		if experiment.Class != ClassDiagnosticCanary {
			t.Fatalf("experiment %q class = %q, want %q", experiment.ID, experiment.Class, ClassDiagnosticCanary)
		}
	}

	firstHTML := renderReportForTest(t, first)
	secondHTML := renderReportForTest(t, second)
	if !bytes.Equal(firstHTML, secondHTML) {
		t.Fatal("two compilations of the same pinned fixture rendered different HTML")
	}

	rendered := string(firstHTML)
	if !strings.Contains(rendered, `<body class="diagnostic-only">`) {
		t.Fatal("diagnostic-only report is missing its document-level watermark class")
	}
	if !strings.Contains(rendered, `class="watermark"`) {
		t.Fatal("diagnostic-only report is missing a visible watermark")
	}
	if !first.DevelopmentContract || !strings.Contains(rendered, i18n.Text(i18n.LangZH, i18n.KeyAgenticReportDevelopmentWatermark)) {
		t.Fatal("pilot5 development contract is not visibly marked as optimization-contaminated and non-formal")
	}
	for _, required := range []string{"cost/raw_attempt", "trial_min/raw_attempt", harness.DeepSWEGPT56SolXHighSourceSHA256, harness.DeepSWEGPT56SolXHighProjectionSHA256, "raw/scored/excluded=452/451/1"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("rendered report omits computed public-reference evidence %q", required)
		}
	}
	if want := i18n.Text(i18n.LangZH, i18n.KeyAgenticReportVerdictInsufficient); !strings.Contains(rendered, want) {
		t.Fatalf("diagnostic report is missing insufficient-evidence verdict %q", want)
	}
	if forbidden := i18n.Text(i18n.LangZH, i18n.KeyAgenticReportVerdictExceeds); strings.Contains(rendered, forbidden) {
		t.Fatalf("diagnostic-only report rendered the formal victory verdict %q", forbidden)
	}
}

func TestValidateInputEnforcesContractSourceClassBoundary(t *testing.T) {
	var input Input
	if err := decodeStrictFile(diagnosticInputFixture, &input); err != nil {
		t.Fatal(err)
	}
	formalSource := ArtifactSource{
		ID: "formal", Label: "Formal", Class: ClassFormal, Root: "formal",
		LedgerFileSHA256: strings.Repeat("a", 64), Description: "formal source",
	}
	pilotSource := formalSource
	pilotSource.ID, pilotSource.Label, pilotSource.Class, pilotSource.Root = "pilot", "Pilot", ClassPilot, "pilot"

	formalWithoutSource := input
	formalWithoutSource.Report.BenchmarkContractID = BenchmarkContractDeepSWEV11Full113
	if err := ValidateInput(formalWithoutSource); err == nil || !strings.Contains(err.Error(), "exactly one formal source") {
		t.Fatalf("formal contract without formal source error = %v", err)
	}

	formal := input
	formal.Report.BenchmarkContractID = BenchmarkContractDeepSWEV11Full113
	formal.ArtifactSources = []ArtifactSource{formalSource}
	if err := ValidateInput(formal); err != nil {
		t.Fatalf("formal contract with one formal source: %v", err)
	}

	formalWithPilot := formal
	formalWithPilot.ArtifactSources = append(formalWithPilot.ArtifactSources, pilotSource)
	if err := ValidateInput(formalWithPilot); err == nil || !strings.Contains(err.Error(), "no pilot sources") {
		t.Fatalf("full113 accepted a pilot source: %v", err)
	}

	developmentWithFormal := input
	developmentWithFormal.ArtifactSources = []ArtifactSource{formalSource}
	if err := ValidateInput(developmentWithFormal); err == nil || !strings.Contains(err.Error(), "cannot contain a formal source") {
		t.Fatalf("pilot5 development contract accepted a formal source: %v", err)
	}

	development := input
	development.ArtifactSources = []ArtifactSource{pilotSource}
	if err := ValidateInput(development); err != nil {
		t.Fatalf("pilot5 development contract with pilot source: %v", err)
	}
}

func TestValidateInputRejectsInexactBenchmarkContractIDs(t *testing.T) {
	var input Input
	if err := decodeStrictFile(diagnosticInputFixture, &input); err != nil {
		t.Fatal(err)
	}
	for _, contractID := range []string{"", "DEEPSWE-V1.1-FULL113", "deepswe-v1.1-full113\u200b", "d\u0435epswe-v1.1-full113", "deepswe-v1.1-full112"} {
		t.Run(contractID, func(t *testing.T) {
			candidate := input
			candidate.Report.BenchmarkContractID = contractID
			if err := ValidateInput(candidate); err == nil || !strings.Contains(err.Error(), "benchmark_contract_id") {
				t.Fatalf("ValidateInput(%q) error = %v, want exact contract rejection", contractID, err)
			}
		})
	}
}

func TestDiagnosticMissingMetricsRemainUnknownAndRenderAsMissing(t *testing.T) {
	data, err := Compile(diagnosticInputFixture)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	run := diagnosticRunForTest(t, data, "diag-codex-fabric", "codex")
	if run.Metrics.ProviderRequests != nil || run.Metrics.ProviderRounds != nil || run.Metrics.ProviderErrors != nil {
		t.Fatalf("absent provider telemetry became values: requests=%v rounds=%v errors=%v", run.Metrics.ProviderRequests, run.Metrics.ProviderRounds, run.Metrics.ProviderErrors)
	}
	if run.Metrics.ToolInvocations != nil || run.Metrics.PhysicalToolOperations != nil || run.Metrics.RequestCacheHit != nil || run.Metrics.ProviderReportedCost != nil {
		t.Fatalf("absent telemetry became values: tools=%v physical=%v request-cache=%v provider-cost=%v",
			run.Metrics.ToolInvocations, run.Metrics.PhysicalToolOperations, run.Metrics.RequestCacheHit, run.Metrics.ProviderReportedCost)
	}

	rendered := string(renderReportForTest(t, data))
	row := tableRowContainingForTest(t, rendered, "codex-native-events")
	missing := i18n.Text(i18n.LangZH, i18n.KeyAgenticReportCoverageMissing)
	if count := strings.Count(row, missing); count < 3 {
		t.Fatalf("Codex row contains %d missing markers, want at least 3; row=%s", count, row)
	}
}

func TestRenderEscapesUntrustedReportValues(t *testing.T) {
	data, err := Compile(diagnosticInputFixture)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	payload := `<script>alert('agentic-report-xss')</script>`
	data.Meta.Title = payload
	data.Meta.Subtitle = payload
	data.Experiments[0].Description = payload
	data.Experiments[0].SourceNote = payload
	data.PublicReferences[0].Notes = payload
	data.PublicReferences[0].SourceURL = `javascript:alert('agentic-report-xss')`
	data.Optimizations[0].Entry.Summary = payload
	data.Limitations[0] = payload

	rendered := string(renderReportForTest(t, data))
	if strings.Contains(rendered, payload) || strings.Contains(rendered, "<script>") {
		t.Fatal("renderer emitted an unescaped script element")
	}
	if count := strings.Count(rendered, "&lt;script&gt;"); count < 6 {
		t.Fatalf("renderer escaped the injected text in only %d locations, want at least 6", count)
	}
	if !strings.Contains(rendered, `href="#ZgotmplZ"`) {
		t.Fatal("renderer did not reject a dangerous URL in an HTML URL context")
	}
}

func TestLoadInputRejectsUnknownFields(t *testing.T) {
	temporaryInput := copyDiagnosticFixturesForTest(t)
	contents, err := os.ReadFile(temporaryInput)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	mutated := bytes.Replace(contents, []byte(`"schema_version":`), []byte(`"unknown_top_level_field": true, "schema_version":`), 1)
	if bytes.Equal(mutated, contents) {
		t.Fatal("test mutation did not alter report input")
	}
	if err := os.WriteFile(temporaryInput, mutated, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err = LoadInput(temporaryInput)
	if err == nil {
		t.Fatal("LoadInput accepted an unknown field")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadInput error = %q, want unknown-field rejection", err)
	}
}

func TestLoadInputRejectsOptimizationLedgerHashMismatch(t *testing.T) {
	temporaryInput := copyDiagnosticFixturesForTest(t)
	contents, err := os.ReadFile(temporaryInput)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var input Input
	if err := json.Unmarshal(contents, &input); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	input.OptimizationLedger.SHA256 = strings.Repeat("0", 64)
	mutated, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(temporaryInput, mutated, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err = LoadInput(temporaryInput)
	if err == nil {
		t.Fatal("LoadInput accepted an optimization ledger with a mismatched pin")
	}
	if !strings.Contains(err.Error(), "SHA-256 does not match") {
		t.Fatalf("LoadInput error = %q, want ledger SHA-256 mismatch", err)
	}
}

func TestLoadInputRejectsHandFilledPublicMetrics(t *testing.T) {
	temporaryInput := copyDiagnosticFixturesForTest(t)
	contents, err := os.ReadFile(temporaryInput)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var input Input
	if err := json.Unmarshal(contents, &input); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	wrong := 0.5
	input.PublicReferences[0].Score = &wrong
	mutated, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(temporaryInput, mutated, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err = LoadInput(temporaryInput)
	if err == nil || !strings.Contains(err.Error(), "metrics must be computed from a registered local artifact") {
		t.Fatalf("LoadInput error = %v, want hand-filled public-metric rejection", err)
	}
}

func TestCompileRejectsOfficialReferenceAgentImpersonation(t *testing.T) {
	for _, agent := range []string{"Codex 0.145.0", "Luban"} {
		t.Run(agent, func(t *testing.T) {
			temporaryInput := copyDiagnosticFixturesForTest(t)
			contents, err := os.ReadFile(temporaryInput)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var input Input
			if err := json.Unmarshal(contents, &input); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			input.PublicReferences[0].Agent = agent
			mutated, err := json.MarshalIndent(input, "", "  ")
			if err != nil {
				t.Fatalf("MarshalIndent: %v", err)
			}
			if err := os.WriteFile(temporaryInput, mutated, 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			_, err = Compile(temporaryInput)
			if err == nil || !strings.Contains(err.Error(), "reference metadata differs from the computed artifact") {
				t.Fatalf("Compile error = %v, want official-reference impersonation rejection", err)
			}
		})
	}
}

func TestLoadInputRejectsPostHocStatisticsContract(t *testing.T) {
	temporaryInput := copyDiagnosticFixturesForTest(t)
	contents, err := os.ReadFile(temporaryInput)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var input Input
	if err := json.Unmarshal(contents, &input); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	input.Statistics.ConfidenceLevel = 0.51
	mutated, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(temporaryInput, mutated, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err = LoadInput(temporaryInput)
	if err == nil || !strings.Contains(err.Error(), "frozen comparative-inference contract") {
		t.Fatalf("LoadInput error = %v, want post-hoc statistics rejection", err)
	}
}

func renderReportForTest(t *testing.T, data Data) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := Render(&output, data); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return output.Bytes()
}

func diagnosticRunForTest(t *testing.T, data Data, experimentID, agentID string) RunData {
	t.Helper()
	for _, experiment := range data.Experiments {
		if experiment.ID != experimentID {
			continue
		}
		for _, run := range experiment.Runs {
			if run.AgentID == agentID {
				return run
			}
		}
	}
	t.Fatalf("run %s/%s not found", experimentID, agentID)
	return RunData{}
}

func tableRowContainingForTest(t *testing.T, document, marker string) string {
	t.Helper()
	markerIndex := strings.Index(document, marker)
	if markerIndex < 0 {
		t.Fatalf("rendered report does not contain marker %q", marker)
	}
	rowStart := strings.LastIndex(document[:markerIndex], "<tr")
	rowEndOffset := strings.Index(document[markerIndex:], "</tr>")
	if rowStart < 0 || rowEndOffset < 0 {
		t.Fatalf("marker %q is not enclosed in a table row", marker)
	}
	return document[rowStart : markerIndex+rowEndOffset+len("</tr>")]
}

func copyDiagnosticFixturesForTest(t *testing.T) string {
	t.Helper()
	temporaryDirectory := t.TempDir()
	for _, source := range []string{diagnosticInputFixture, diagnosticLedgerFixture} {
		contents, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", source, err)
		}
		destination := filepath.Join(temporaryDirectory, filepath.Base(source))
		if err := os.WriteFile(destination, contents, 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", destination, err)
		}
	}
	return filepath.Join(temporaryDirectory, filepath.Base(diagnosticInputFixture))
}
