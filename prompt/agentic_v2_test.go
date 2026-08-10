package prompt

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func agenticV2TestTools() []types.Tool {
	return []types.Tool{
		&mockTool{name: "Inspect", desc: "inspect-description-sentinel"},
		&mockTool{name: "ApplyPatch", desc: "apply-patch-description-sentinel"},
		&mockTool{name: "Run", desc: "run-description-sentinel"},
		&mockTool{name: "Read", desc: "read-description-sentinel"},
		&mockTool{name: "Edit", desc: "edit-description-sentinel"},
		&mockTool{name: "Write", desc: "write-description-sentinel"},
		&mockTool{name: "Glob", desc: "glob-description-sentinel"},
		&mockTool{name: "Grep", desc: "grep-description-sentinel"},
		&mockTool{name: "Bash", desc: "bash-description-sentinel"},
	}
}

func TestAgenticV2RequiresAllThreeCoreTools(t *testing.T) {
	all := agenticV2TestTools()

	for _, missing := range []string{"Inspect", "ApplyPatch", "Run"} {
		t.Run(missing, func(t *testing.T) {
			var tools []types.Tool
			for _, tool := range all {
				if tool.Name() != missing {
					tools = append(tools, tool)
				}
			}
			got := BuildSystemPrompt(tools, Config{CWD: "/repo"})
			if strings.Contains(got, "ApplyPatch is the only file writer") {
				t.Fatalf("v2 activated without %s", missing)
			}
			for _, legacy := range []string{"To read files use Read instead of cat", "To edit files use Edit instead of sed", "Reserve Bash for system commands"} {
				if strings.Contains(got, legacy) {
					t.Fatalf("incomplete coding kernel fell back to legacy guidance %q", legacy)
				}
			}
		})
	}
}

func TestAgenticV2ReplacesLegacyGuidanceWithCoreWorkflow(t *testing.T) {
	blocks := BuildSystemPromptBlocks(agenticV2TestTools(), Config{CWD: "/repo"})

	if len(blocks) != 2 {
		t.Fatalf("expected one static and one dynamic block, got %d", len(blocks))
	}
	if !blocks[0].Cache || blocks[0].Name != "static" {
		t.Fatalf("expected v2 to remain in the cache-eligible static block, got %#v", blocks[0])
	}
	if blocks[1].Cache {
		t.Fatalf("expected runtime block to remain uncached, got %#v", blocks[1])
	}

	got := blocks.JoinedText()
	for _, want := range []string{
		"derive concrete acceptance criteria",
		"root-cause and implementation hypothesis",
		"Inspect is the only repository read, search, and file-discovery tool",
		"high-information inspections",
		"never a fixed round count",
		"Do not reread unchanged evidence",
		"ApplyPatch is the only file writer",
		"one cohesive multi-file, multi-hunk transaction",
		"Run is the only terminal and verification tool",
		"cheapest focused test",
		"Broaden only for risk signals",
		"one Run graph",
		"requires_patch_commit=true",
		"adjacent omission is fused only within that response",
		"ApplyPatch cannot issue a receipt for a no-op",
		"missing adoption or sealing authority",
		"criticize the complete patch or resulting diff",
		"both sides of compatibility changes",
		"A passing command does not by itself prove semantic correctness",
		"Never repeat an unchanged deterministic failure fingerprint",
		"Do not weaken meaningful tests merely to make them pass",
		"Stop as soon as the criteria are satisfied",
		"The complete visible catalog is Inspect, ApplyPatch, and Run",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected v2 prompt to contain %q", want)
		}
	}

	for _, legacy := range []string{
		"To read files use Read instead of cat",
		"To edit files use Edit instead of sed",
		"Reserve Bash for system commands",
		"Use Read, Grep, or Glob only",
		"use Edit or Write only",
		"use Bash only",
	} {
		if strings.Contains(got, legacy) {
			t.Fatalf("v2 should replace legacy guidance %q", legacy)
		}
	}
	if count := strings.Count(got, "# Coding contract"); count != 1 {
		t.Fatalf("expected exactly one tool-guidance section, got %d", count)
	}
	for _, schemaLeak := range []string{
		"description-sentinel",
		`"type":"object"`,
		"## Inspect",
		"## ApplyPatch",
		"## Run",
	} {
		if strings.Contains(got, schemaLeak) {
			t.Fatalf("system prompt duplicated tool schema or description %q", schemaLeak)
		}
	}
}

func TestAgenticV2StaticPrefixIsLeanAndRuntimeConfigIsLate(t *testing.T) {
	first := BuildSystemPromptBlocks(agenticV2TestTools(), Config{
		CWD: "/repo-a", Language: "japanese", OutputStyle: "concise", ToolDescriptions: "late tool notes",
	})
	second := BuildSystemPromptBlocks(agenticV2TestTools(), Config{
		CWD: "/repo-b", Language: "german", OutputStyle: "explanatory", ToolDescriptions: "different notes",
	})
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("prompt blocks = %d and %d, want static+dynamic", len(first), len(second))
	}
	if first[0].Text != second[0].Text || !first[0].Cache || !second[0].Cache {
		t.Fatal("runtime configuration changed the cache-eligible Agentic V2 prefix")
	}
	for _, dynamicOnly := range []string{"japanese", "concise", "late tool notes"} {
		if strings.Contains(first[0].Text, dynamicOnly) || !strings.Contains(first[1].Text, dynamicOnly) {
			t.Fatalf("late runtime value %q crossed the static cache boundary", dynamicOnly)
		}
	}
	if strings.Contains(first[0].Text, "/repo-a") || !strings.Contains(first[1].Text, "/repo-a") {
		t.Fatal("working directory crossed the static cache boundary")
	}

	const previousBytes = 9924
	const previousEstimatedTokens = 2481
	currentBytes := len(first[0].Text)
	currentEstimatedTokens := (currentBytes + 3) / 4
	if currentBytes >= previousBytes || currentEstimatedTokens >= previousEstimatedTokens {
		t.Fatalf("Agentic V2 static prefix did not shrink: before=%dB/~%dt after=%dB/~%dt",
			previousBytes, previousEstimatedTokens, currentBytes, currentEstimatedTokens)
	}
	if currentBytes > 5000 {
		t.Fatalf("Agentic V2 static prefix = %d bytes, want <= 5000", currentBytes)
	}
	t.Logf("Agentic V2 static prefix: before=%d bytes (~%d tokens), after=%d bytes (~%d tokens)",
		previousBytes, previousEstimatedTokens, currentBytes, currentEstimatedTokens)
}

func TestAgenticV2CannotLoseKernelThroughSimpleMode(t *testing.T) {
	t.Setenv("LUBAN_CODE_SIMPLE", "true")
	blocks := BuildSystemPromptBlocks(agenticV2TestTools(), Config{CWD: "/repo"})
	if len(blocks) != 2 || !blocks[0].Cache || !strings.Contains(blocks[0].Text, "# Coding contract") {
		t.Fatalf("simple mode erased Agentic V2 correctness kernel: %#v", blocks)
	}
}
