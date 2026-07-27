package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRootHelpDoesNotRequireConfiguration(t *testing.T) {
	t.Setenv("AGENTIC_BENCH_MANIFEST", "")
	t.Setenv("AGENTIC_BENCH_BACKEND_CONFIG", "")
	t.Setenv("AGENTIC_BENCH_WORK_DIR", "")
	for _, argument := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		if status := runMain(context.Background(), []string{argument}, &stdout, &stderr); status != 0 {
			t.Fatalf("%s status = %d, stderr = %q", argument, status, stderr.String())
		}
		if !strings.Contains(stdout.String(), "agenticbench") || stderr.Len() != 0 {
			t.Fatalf("%s stdout = %q, stderr = %q", argument, stdout.String(), stderr.String())
		}
	}
}
