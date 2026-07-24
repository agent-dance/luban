package i18n

const (
	KeySubagentSegmentMessages          Key = "tui.subagent_segment.messages"
	KeySubagentSegmentTurns             Key = "tui.subagent_segment.turns"
	KeySubagentSegmentWaiting           Key = "tui.subagent_segment.waiting"
	KeySubagentSegmentResultPendingView Key = "tui.subagent_segment.result_pending_view"
	KeySubagentSegmentResultSummary     Key = "tui.subagent_segment.result_summary"
	KeySubagentSegmentUsage             Key = "tui.subagent_segment.usage"
	KeySubagentSegmentUsageUnknownCost  Key = "tui.subagent_segment.usage_unknown_cost"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}
	add(KeySubagentSegmentMessages,
		"Messages: %d",
		"消息：%d",
		"Nachrichten: %d",
		"メッセージ：%d",
		"메시지: %d",
		"Сообщения: %d",
	)
	add(KeySubagentSegmentTurns,
		"Turns: %d",
		"轮次：%d",
		"Runden: %d",
		"ターン：%d",
		"턴: %d",
		"Ходов: %d",
	)
	add(KeySubagentSegmentWaiting,
		"Waiting for Agent output…",
		"等待 Agent 输出…",
		"Warte auf Agent-Ausgabe…",
		"Agent の出力を待機中…",
		"Agent 출력을 기다리는 중…",
		"Ожидание вывода Agent…",
	)
	add(KeySubagentSegmentResultPendingView,
		"Result ready to view",
		"结果待查看",
		"Ergebnis kann angesehen werden",
		"結果を確認できます",
		"결과 확인 가능",
		"результат доступен для просмотра",
	)
	add(KeySubagentSegmentResultSummary,
		"Result summary",
		"结果摘要",
		"Ergebnisübersicht",
		"結果の概要",
		"결과 요약",
		"сводка результата",
	)
	add(KeySubagentSegmentUsage,
		"in %s · %d%% cached · out %s · $%.4f",
		"输入 %s · 缓存 %d%% · 输出 %s · $%.4f",
		"%s ein · %d%% im Cache · %s aus · $%.4f",
		"入力 %s · キャッシュ %d%% · 出力 %s · $%.4f",
		"입력 %s · 캐시 %d%% · 출력 %s · $%.4f",
		"вход %s · %d%% из кэша · выход %s · $%.4f",
	)
	add(KeySubagentSegmentUsageUnknownCost,
		"in %s · %d%% cached · out %s · cost unknown",
		"输入 %s · 缓存 %d%% · 输出 %s · 费用未知",
		"%s ein · %d%% im Cache · %s aus · Kosten unbekannt",
		"入力 %s · キャッシュ %d%% · 出力 %s · 料金不明",
		"입력 %s · 캐시 %d%% · 출력 %s · 비용 알 수 없음",
		"вход %s · %d%% из кэша · выход %s · стоимость неизвестна",
	)
}
