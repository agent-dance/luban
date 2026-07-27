package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMainRequiresInputAndOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMain(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("runMain exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 || strings.TrimSpace(stderr.String()) == "" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunMainGeneratesDiagnosticReport(t *testing.T) {
	input, err := filepath.Abs(filepath.Join("..", "..", "testdata", "diagnostic-input.json"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "diagnostic.html")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runMain([]string{"--input", input, "--output", output}, &stdout, &stderr); code != 0 {
		t.Fatalf("runMain exit code = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), output) {
		t.Fatalf("success output %q does not contain %q", stdout.String(), output)
	}
}
