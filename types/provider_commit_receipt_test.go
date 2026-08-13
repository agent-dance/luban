package types

import (
	"strings"
	"testing"
)

func TestProviderCommitReceiptCommitsOnlyToFramedToolPayloads(t *testing.T) {
	first := NewProviderCommitReceipt("deepseek", "responses", "completed", [][]byte{[]byte("ab"), []byte("c")})
	second := NewProviderCommitReceipt("deepseek", "responses", "completed", [][]byte{[]byte("a"), []byte("bc")})
	if first.SchemaVersion != ProviderCommitReceiptSchema || !first.ToolsAuthorized || first.ToolCalls != 2 || first.ToolBatchBytes != 3 {
		t.Fatalf("receipt = %#v", first)
	}
	if first.ToolBatchDigest == second.ToolBatchDigest || !strings.HasPrefix(first.ToolBatchDigest, "sha256:") {
		t.Fatalf("framed digests collided: first=%q second=%q", first.ToolBatchDigest, second.ToolBatchDigest)
	}
	incomplete := NewProviderCommitReceipt("deepseek", "responses", "incomplete", [][]byte{[]byte("secret")})
	if incomplete.ToolsAuthorized {
		t.Fatalf("incomplete receipt authorized tools: %#v", incomplete)
	}
}

func TestProviderToolCommitReceiptBindsIdentityAndRawInput(t *testing.T) {
	base := []ProviderToolCallCommit{{
		OutputIndex: 2, ToolType: ToolDefinitionTypeFunction, ProviderItemID: "item-1",
		CallID: "call-1", Name: "Run", RawInput: `{"steps":[]}`,
	}}
	receipt := NewProviderToolCommitReceipt("deepseek", "responses", "completed", base)
	changed := append([]ProviderToolCallCommit(nil), base...)
	changed[0].CallID = "call-2"
	if receipt.ToolBatchDigest == NewProviderToolCommitReceipt("deepseek", "responses", "completed", changed).ToolBatchDigest {
		t.Fatal("tool identity change did not alter batch digest")
	}
	changed = append([]ProviderToolCallCommit(nil), base...)
	changed[0].RawInput += " "
	if receipt.ToolBatchDigest == NewProviderToolCommitReceipt("deepseek", "responses", "completed", changed).ToolBatchDigest {
		t.Fatal("raw input change did not alter batch digest")
	}
	if strings.Contains(receipt.ToolBatchDigest, "steps") {
		t.Fatalf("receipt retained raw input: %#v", receipt)
	}
}
