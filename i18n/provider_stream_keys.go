package i18n

const (
	KeyProviderStreamIdleTimeout        Key = "provider.stream.idle_timeout"
	KeyProviderStreamInitialIdleTimeout Key = "provider.stream.initial_idle_timeout"
	KeyProviderStreamActiveIdleTimeout  Key = "provider.stream.active_idle_timeout"
)

func init() {
	semanticTranslations[KeyProviderStreamIdleTimeout] = map[Language]string{
		LangEN: "The provider stream had no activity for %s",
		LangZH: "Provider stream 已连续 %s 没有活动",
		LangDE: "Der Provider-Stream zeigte %s lang keine Aktivität",
		LangJA: "Provider stream で %s の間アクティビティがありませんでした",
		LangKO: "Provider stream에서 %s 동안 활동이 없었습니다",
		LangRU: "В stream провайдера не было активности в течение %s",
	}
	semanticTranslations[KeyProviderStreamInitialIdleTimeout] = map[Language]string{
		LangEN: "The provider stream had no activity for %s before output began",
		LangZH: "Provider stream 在输出开始前已连续 %s 没有活动",
		LangDE: "Der Provider-Stream zeigte vor Beginn der Ausgabe %s lang keine Aktivität",
		LangJA: "出力開始前の Provider stream で %s の間アクティビティがありませんでした",
		LangKO: "출력 시작 전 Provider stream에서 %s 동안 활동이 없었습니다",
		LangRU: "В stream провайдера не было активности в течение %s до начала вывода",
	}
	semanticTranslations[KeyProviderStreamActiveIdleTimeout] = map[Language]string{
		LangEN: "The provider stream had no activity for %s after output began",
		LangZH: "Provider stream 在输出开始后已连续 %s 没有活动",
		LangDE: "Der Provider-Stream zeigte nach Beginn der Ausgabe %s lang keine Aktivität",
		LangJA: "出力開始後の Provider stream で %s の間アクティビティがありませんでした",
		LangKO: "출력 시작 후 Provider stream에서 %s 동안 활동이 없었습니다",
		LangRU: "В stream провайдера не было активности в течение %s после начала вывода",
	}
}
