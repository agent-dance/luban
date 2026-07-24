package skills

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestPresentedSummaryLocalizesOnlyGeneratedCopy(t *testing.T) {
	generated := EffectiveSkill{Name: "review", Summary: "Skill: review", SummaryGenerated: true, Source: SourceProject}
	if got := PresentedSummary(i18n.LangZH, generated); got != "技能：review" {
		t.Fatalf("generated summary = %q", got)
	}
	mcpGenerated := EffectiveSkill{Name: "server:review", Summary: "MCP skill: server:review", SummaryGenerated: true, Source: SourceMCP}
	if got := PresentedSummary(i18n.LangDE, mcpGenerated); got != "MCP-Skill: server:review" {
		t.Fatalf("generated MCP summary = %q", got)
	}
	authored := EffectiveSkill{Name: "review", Summary: "Keep this authored text", Source: SourceProject}
	if got := PresentedSummary(i18n.LangZH, authored); got != authored.Summary {
		t.Fatalf("authored summary changed: %q", got)
	}
}
