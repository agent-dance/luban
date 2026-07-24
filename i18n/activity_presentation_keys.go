package i18n

const (
	KeyActivityResultsPendingView Key = "tui.activity.results_pending_view"
)

func init() {
	semanticTranslations[KeyActivityResultsPendingView] = map[Language]string{
		LangEN: "%d results ready to view",
		LangZH: "%d 项结果待查看",
		LangDE: "%d Ergebnisse zur Ansicht",
		LangJA: "確認可能な結果 %d 件",
		LangKO: "확인할 결과 %d개",
		LangRU: "%d результатов для просмотра",
	}
}
