package i18n

const (
	KeyProviderStreamInitialIdleTimeout Key = "provider.stream.initial_idle_timeout"
	KeyProviderStreamActiveIdleTimeout  Key = "provider.stream.active_idle_timeout"
)

func init() {
	semanticTranslations[KeyProviderStreamInitialIdleTimeout] = map[Language]string{
		LangEN: "The provider stream made no byte progress for %s before output began",
		LangZH: "Provider stream 在输出开始前已连续 %s 没有字节进展",
		LangDE: "Der Provider-Stream zeigte vor Beginn der Ausgabe %s lang keinen Byte-Fortschritt",
		LangJA: "出力開始前の Provider stream で %s の間バイト進行がありませんでした",
		LangKO: "출력 시작 전 Provider stream에서 %s 동안 바이트 진행이 없었습니다",
		LangRU: "В stream провайдера не было байтового прогресса в течение %s до начала вывода",
	}
	semanticTranslations[KeyProviderStreamActiveIdleTimeout] = map[Language]string{
		LangEN: "The provider stream made no byte progress for %s after output began",
		LangZH: "Provider stream 在输出开始后已连续 %s 没有字节进展",
		LangDE: "Der Provider-Stream zeigte nach Beginn der Ausgabe %s lang keinen Byte-Fortschritt",
		LangJA: "出力開始後の Provider stream で %s の間バイト進行がありませんでした",
		LangKO: "출력 시작 후 Provider stream에서 %s 동안 바이트 진행이 없었습니다",
		LangRU: "В stream провайдера не было байтового прогресса в течение %s после начала вывода",
	}
}
