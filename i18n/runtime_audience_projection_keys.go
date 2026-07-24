package i18n

const (
	KeyRuntimeEventProjectionRejected Key = "runtime.event.projection_rejected"
	KeyRuntimeEventInvalid            Key = "runtime.event.invalid"
	KeyRuntimeToolResultPublicSummary Key = "runtime.tool_result.public_summary"
)

var runtimeAudienceProjectionKeys = [...]Key{
	KeyRuntimeEventProjectionRejected,
	KeyRuntimeEventInvalid,
	KeyRuntimeToolResultPublicSummary,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyRuntimeEventProjectionRejected,
		"The runtime event projection was rejected.",
		"运行事件投影已被拒绝。",
		"Die Projektion des Laufzeitereignisses wurde abgelehnt.",
		"実行時イベントの投影が拒否されました。",
		"런타임 이벤트 프로젝션이 거부되었습니다.",
		"Проекция события выполнения отклонена.")
	add(KeyRuntimeEventInvalid,
		"The runtime event is invalid.",
		"运行事件无效。",
		"Das Laufzeitereignis ist ungültig.",
		"実行時イベントが無効です。",
		"런타임 이벤트가 올바르지 않습니다.",
		"Событие выполнения недопустимо.")
	add(KeyRuntimeToolResultPublicSummary,
		"Tool execution finished.",
		"工具执行已结束。",
		"Die Tool-Ausführung ist beendet.",
		"ツールの実行が終了しました。",
		"도구 실행이 끝났습니다.",
		"Выполнение инструмента завершено.")
}
