package i18n

const KeyRuntimeErrorPublicSummary Key = "runtime.error.public_summary"

func init() {
	semanticTranslations[KeyRuntimeErrorPublicSummary] = map[Language]string{
		LangEN: "A runtime operation failed. Retry the operation or open diagnostics if the problem continues.",
		LangZH: "运行操作失败。请重试；如果问题持续存在，可打开诊断信息。",
		LangDE: "Ein Laufzeitvorgang ist fehlgeschlagen. Wiederhole den Vorgang oder öffne die Diagnose, falls das Problem bestehen bleibt.",
		LangJA: "実行時の処理に失敗しました。再試行し、問題が続く場合は診断情報を開いてください。",
		LangKO: "런타임 작업에 실패했습니다. 다시 시도하고 문제가 계속되면 진단 정보를 여세요.",
		LangRU: "Операция выполнения завершилась с ошибкой. Повторите её, а если проблема сохраняется — откройте диагностику.",
	}
}
