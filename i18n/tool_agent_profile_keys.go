package i18n

const (
	KeyToolAgentProfileDescriptionMissing Key = "tool.agent.profile.description_missing"
	KeyToolAgentProfileLine               Key = "tool.agent.profile.line"
	KeyToolAgentProfileNoTools            Key = "tool.agent.profile.no_tools"
	KeyToolAgentProfileAllToolsExcept     Key = "tool.agent.profile.all_tools_except"
	KeyToolAgentProfileAllTools           Key = "tool.agent.profile.all_tools"
)

func init() {
	semanticTranslations[KeyToolAgentProfileDescriptionMissing] = map[Language]string{
		LangEN: "No description provided.",
		LangZH: "未提供描述。",
		LangDE: "Keine Beschreibung angegeben.",
		LangJA: "説明がありません。",
		LangKO: "설명이 제공되지 않았습니다.",
		LangRU: "Описание не указано.",
	}
	semanticTranslations[KeyToolAgentProfileLine] = map[Language]string{
		LangEN: "- %s: %s (Tools: %s)",
		LangZH: "- %s：%s（工具：%s）",
		LangDE: "- %s: %s (Tools: %s)",
		LangJA: "- %s: %s（ツール: %s）",
		LangKO: "- %s: %s (도구: %s)",
		LangRU: "- %s: %s (Инструменты: %s)",
	}
	semanticTranslations[KeyToolAgentProfileNoTools] = map[Language]string{
		LangEN: "No tools",
		LangZH: "无工具",
		LangDE: "Keine Tools",
		LangJA: "ツールなし",
		LangKO: "도구 없음",
		LangRU: "Нет инструментов",
	}
	semanticTranslations[KeyToolAgentProfileAllToolsExcept] = map[Language]string{
		LangEN: "All tools except %s",
		LangZH: "除 %s 外的所有工具",
		LangDE: "Alle Tools außer %s",
		LangJA: "%s 以外のすべてのツール",
		LangKO: "%s을(를) 제외한 모든 도구",
		LangRU: "Все инструменты, кроме %s",
	}
	semanticTranslations[KeyToolAgentProfileAllTools] = map[Language]string{
		LangEN: "All tools",
		LangZH: "所有工具",
		LangDE: "Alle Tools",
		LangJA: "すべてのツール",
		LangKO: "모든 도구",
		LangRU: "Все инструменты",
	}
}
