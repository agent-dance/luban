package i18n

const (
	KeyRuntimeErrorPublicSummary         Key = "runtime.error.public_summary"
	KeyRuntimeErrorProviderAPISuggestion Key = "runtime.error.provider.api_suggestion"
	KeyRuntimeErrorProviderAPIsExhausted Key = "runtime.error.provider.apis_exhausted"
)

func init() {
	semanticTranslations[KeyRuntimeErrorPublicSummary] = map[Language]string{
		LangEN: "A runtime operation failed. Retry the operation or open diagnostics if the problem continues.",
		LangZH: "运行操作失败。请重试；如果问题持续存在，可打开诊断信息。",
		LangDE: "Ein Laufzeitvorgang ist fehlgeschlagen. Wiederhole den Vorgang oder öffne die Diagnose, falls das Problem bestehen bleibt.",
		LangJA: "実行時の処理に失敗しました。再試行し、問題が続く場合は診断情報を開いてください。",
		LangKO: "런타임 작업에 실패했습니다. 다시 시도하고 문제가 계속되면 진단 정보를 여세요.",
		LangRU: "Операция выполнения завершилась с ошибкой. Повторите её, а если проблема сохраняется — откройте диагностику.",
	}
	semanticTranslations[KeyRuntimeErrorProviderAPISuggestion] = map[Language]string{
		LangEN: "The endpoint rejected %s. Retry with --api %s, or configure an endpoint that supports %s.",
		LangZH: "该 endpoint 拒绝了 %s。请使用 --api %s 重试，或配置支持 %s 的 endpoint。",
		LangDE: "Der Endpoint hat %s abgelehnt. Versuche es erneut mit --api %s oder konfiguriere einen Endpoint, der %s unterstützt.",
		LangJA: "endpoint が %s を拒否しました。--api %s で再試行するか、%s 対応の endpoint を設定してください。",
		LangKO: "endpoint가 %s을(를) 거부했습니다. --api %s(으)로 다시 시도하거나 %s을(를) 지원하는 endpoint를 구성하세요.",
		LangRU: "Endpoint отклонил %s. Повторите попытку с --api %s или настройте endpoint с поддержкой %s.",
	}
	semanticTranslations[KeyRuntimeErrorProviderAPIsExhausted] = map[Language]string{
		LangEN: "Requests using %s and %s both failed. Verify the base URL, credentials, and endpoint protocol support.",
		LangZH: "使用 %s 和 %s 的请求均失败。请检查 base URL、凭据以及 endpoint 的协议支持。",
		LangDE: "Anfragen mit %s und %s sind beide fehlgeschlagen. Prüfe Base URL, Zugangsdaten und die Protokollunterstützung des Endpoints.",
		LangJA: "%s と %s の両方のリクエストが失敗しました。base URL、認証情報、endpoint のプロトコル対応を確認してください。",
		LangKO: "%s 및 %s 요청이 모두 실패했습니다. base URL, 자격 증명, endpoint 프로토콜 지원을 확인하세요.",
		LangRU: "Запросы с %s и %s завершились ошибкой. Проверьте base URL, учётные данные и поддержку протоколов endpoint.",
	}
}
