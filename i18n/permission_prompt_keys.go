package i18n

const (
	KeyPermissionPromptInline     Key = "permission.prompt.inline"
	KeyPermissionPromptTool       Key = "permission.prompt.tool"
	KeyPermissionPromptCall       Key = "permission.prompt.call"
	KeyPermissionPromptInfo       Key = "permission.prompt.info"
	KeyPermissionPromptRisk       Key = "permission.prompt.risk"
	KeyPermissionPromptAllow      Key = "permission.prompt.allow"
	KeyPermissionPromptRiskLow    Key = "permission.prompt.risk.low"
	KeyPermissionPromptRiskMedium Key = "permission.prompt.risk.medium"
	KeyPermissionPromptRiskHigh   Key = "permission.prompt.risk.high"
)

func init() {
	addPermissionPrompt(KeyPermissionPromptInline, "\nAllow %s? %s  [y/N/a(lways)]: ", "\n允许 %s？%s  [y/N/a（始终允许）]: ", "\n%s erlauben? %s  [y/N/a (immer)]: ", "\n%s を許可しますか？%s  [y/N/a（常に許可）]: ", "\n%s을(를) 허용할까요? %s  [y/N/a(항상 허용)]: ", "\nРазрешить %s? %s  [y/N/a (всегда)]: ")
	addPermissionPrompt(KeyPermissionPromptTool, "Tool:", "工具：", "Tool:", "ツール:", "도구:", "Инструмент:")
	addPermissionPrompt(KeyPermissionPromptCall, "Call:", "调用：", "Aufruf:", "呼び出し:", "호출:", "Вызов:")
	addPermissionPrompt(KeyPermissionPromptInfo, "Info:", "详情：", "Info:", "情報:", "정보:", "Сведения:")
	addPermissionPrompt(KeyPermissionPromptRisk, "Risk:", "风险：", "Risiko:", "リスク:", "위험:", "Риск:")
	addPermissionPrompt(KeyPermissionPromptAllow, "Allow? [y/N/a(lways)]:", "是否允许？[y/N/a（始终允许）]:", "Erlauben? [y/N/a (immer)]:", "許可しますか？[y/N/a（常に許可）]:", "허용할까요? [y/N/a(항상 허용)]:", "Разрешить? [y/N/a (всегда)]:")
	addPermissionPrompt(KeyPermissionPromptRiskLow, "🟢 Low", "🟢 低", "🟢 Niedrig", "🟢 低", "🟢 낮음", "🟢 Низкий")
	addPermissionPrompt(KeyPermissionPromptRiskMedium, "🟡 Medium", "🟡 中", "🟡 Mittel", "🟡 中", "🟡 보통", "🟡 Средний")
	addPermissionPrompt(KeyPermissionPromptRiskHigh, "🔴 High", "🔴 高", "🔴 Hoch", "🔴 高", "🔴 높음", "🔴 Высокий")
}

func addPermissionPrompt(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
