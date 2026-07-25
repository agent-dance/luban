package compact

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestBuildPostCompactMessagesSealsUsageIntoBoundary(t *testing.T) {
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: "manual", PreCompactTokenCount: 900})
	usage := &types.Usage{InputTokens: 41, OutputTokens: 13, CacheReadInputTokens: 17}
	result := &CompactionResult{
		BoundaryMarker:            &boundary,
		SummaryMessages:           []types.Message{trustedCompactSummaryForTest("summary")},
		MessagesToKeep:            []types.Message{types.UserMessage("tail")},
		PreCompactTokenCount:      900,
		PostCompactTokenCount:     240,
		TruePostCompactTokenCount: 255,
		CompactionUsage:           usage,
	}
	messages := BuildPostCompactMessages(result)
	metadata, ok := ParseCompactBoundaryMessage(messages[0])
	if !ok {
		t.Fatal("enriched boundary is not a trusted compact boundary")
	}
	if metadata.Trigger != "manual" || metadata.PreCompactTokenCount != 900 ||
		metadata.PostCompactTokenCount != 240 || metadata.TruePostCompactTokenCount != 255 {
		t.Fatalf("boundary counts = %+v", metadata)
	}
	if metadata.CompactionUsage == nil || metadata.CompactionUsage.InputTokens != 41 ||
		metadata.CompactionUsage.OutputTokens != 13 || metadata.CompactionUsage.CacheReadInputTokens != 17 {
		t.Fatalf("boundary usage = %+v", metadata.CompactionUsage)
	}
}
