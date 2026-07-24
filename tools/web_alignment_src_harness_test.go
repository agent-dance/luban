package tools

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

type srcWebContractResult struct {
	Tool       string `json:"tool"`
	Input      any    `json:"input"`
	Validation struct {
		OK        bool   `json:"ok"`
		Message   string `json:"message"`
		ErrorCode int    `json:"errorCode"`
	} `json:"validation"`
	Semantics struct {
		ProviderNative         bool     `json:"providerNative"`
		PromptDrivenExtraction bool     `json:"promptDrivenExtraction"`
		HasDedicatedPrompt     bool     `json:"hasDedicatedPrompt"`
		PermissionAware        bool     `json:"permissionAware"`
		ProgressEvents         []string `json:"progressEvents,omitempty"`
		ResultShape            any      `json:"resultShape"`
	} `json:"semantics"`
	NormalizedResult json.RawMessage `json:"normalizedResult"`
}

func runSrcHarness(t *testing.T, tool, inputFile string) srcWebContractResult {
	t.Helper()
	script := filepath.Join("testdata", "web_alignment", "src_harness", "run_web_contract_check.js")
	input := filepath.Join("testdata", "web_alignment", "src_harness", inputFile)
	cmd := exec.Command(nodeExecutable(), script, tool, input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run src harness (%s %s): %v\nstderr: %s", tool, inputFile, err, stderr.String())
	}
	var out srcWebContractResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode src harness output: %v\noutput: %s", err, stdout.String())
	}
	return out
}

func nodeExecutable() string {
	if runtime.GOOS == "windows" {
		return "node.exe"
	}
	return "node"
}

func TestSrcHarnessWebFetchValid(t *testing.T) {
	got := runSrcHarness(t, "webfetch", "webfetch_valid_input.json")
	if got.Tool != "WebFetch" || !got.Validation.OK || !got.Semantics.ProviderNative || !got.Semantics.PromptDrivenExtraction {
		t.Fatalf("unexpected src harness result: %+v", got)
	}
}

func TestSrcHarnessWebFetchInvalidURL(t *testing.T) {
	got := runSrcHarness(t, "webfetch", "webfetch_invalid_url_input.json")
	if got.Validation.OK {
		t.Fatalf("expected invalid URL validation failure: %+v", got)
	}
}

func TestSrcHarnessWebSearchValid(t *testing.T) {
	got := runSrcHarness(t, "websearch", "websearch_valid_input.json")
	if got.Tool != "WebSearch" || !got.Validation.OK || !got.Semantics.ProviderNative || len(got.Semantics.ProgressEvents) == 0 {
		t.Fatalf("unexpected src harness result: %+v", got)
	}
}

func TestSrcHarnessWebSearchConflict(t *testing.T) {
	got := runSrcHarness(t, "websearch", "websearch_conflict_input.json")
	if got.Validation.OK {
		t.Fatalf("expected conflict validation failure: %+v", got)
	}
}

func TestSrcHarnessWebFetchMockExecutionNormalizedResult(t *testing.T) {
	got := runSrcHarness(t, "webfetch", "webfetch_mock_execution_input.json")
	if len(got.NormalizedResult) == 0 {
		t.Fatalf("expected normalized result payload")
	}
	var norm webFetchNormalizedResult
	if err := json.Unmarshal(got.NormalizedResult, &norm); err != nil {
		t.Fatalf("unmarshal normalized result: %v", err)
	}
	if norm.Execution.Method != "provider_native" {
		t.Fatalf("expected provider_native method, got %+v", norm)
	}
}

func TestSrcHarnessWebSearchMockExecutionNormalizedResult(t *testing.T) {
	got := runSrcHarness(t, "websearch", "websearch_mock_execution_input.json")
	if len(got.NormalizedResult) == 0 {
		t.Fatalf("expected normalized result payload")
	}
	var norm webSearchNormalizedResult
	if err := json.Unmarshal(got.NormalizedResult, &norm); err != nil {
		t.Fatalf("unmarshal normalized result: %v", err)
	}
	if norm.Execution.Method != "provider_native" || len(norm.Results) != 1 {
		t.Fatalf("unexpected normalized result: %+v", norm)
	}
}
