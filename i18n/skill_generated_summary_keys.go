package i18n

const (
	KeySkillGeneratedSummary    Key = "skill.generated_summary"
	KeyMCPSkillGeneratedSummary Key = "skill.mcp.generated_summary"
)

var skillGeneratedSummaryKeys = []Key{
	KeySkillGeneratedSummary,
	KeyMCPSkillGeneratedSummary,
}

func init() {
	semanticTranslations[KeySkillGeneratedSummary] = map[Language]string{
		LangEN: "Skill: %s",
		LangZH: "技能：%s",
		LangDE: "Skill: %s",
		LangJA: "スキル: %s",
		LangKO: "스킬: %s",
		LangRU: "Навык: %s",
	}
	semanticTranslations[KeyMCPSkillGeneratedSummary] = map[Language]string{
		LangEN: "MCP skill: %s",
		LangZH: "MCP 技能：%s",
		LangDE: "MCP-Skill: %s",
		LangJA: "MCP スキル: %s",
		LangKO: "MCP 스킬: %s",
		LangRU: "Навык MCP: %s",
	}
}
