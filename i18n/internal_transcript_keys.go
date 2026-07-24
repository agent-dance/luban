package i18n

// Semantic copy carried by model-visible runtime messages, transcript
// placeholders, SDK event summaries, and presentation fallbacks. Protocol
// tags, field names, paths, identifiers, counts, and raw tool output remain
// formatting arguments or stable metadata.
const (
	KeyCompactMicrocompactResultCleared  Key = "compact.microcompact.result_cleared"
	KeyToolReadFileUnchanged             Key = "tool.read.file_unchanged"
	KeyToolReadEmptyFileWarning          Key = "tool.read.empty_file_warning"
	KeyToolReadOffsetBeyondEndWarning    Key = "tool.read.offset_beyond_end_warning"
	KeyToolAgentForkStarted              Key = "tool.agent.fork.started"
	KeyLoopFallbackTombstoneSummary      Key = "loop.fallback.tombstone_summary"
	KeyToolBackgroundRuntimeInterrupted  Key = "tool.background.runtime_interrupted"
	KeyToolBackgroundFollowUpInstruction Key = "tool.background.follow_up_instruction"
	KeyPresentationCronNextUnknown       Key = "presentation.cron.next_unknown"
	KeyPresentationCronTimezoneLocal     Key = "presentation.cron.timezone_local"
)

var internalTranscriptKeys = [...]Key{
	KeyCompactMicrocompactResultCleared,
	KeyToolReadFileUnchanged,
	KeyToolReadEmptyFileWarning,
	KeyToolReadOffsetBeyondEndWarning,
	KeyToolAgentForkStarted,
	KeyLoopFallbackTombstoneSummary,
	KeyToolBackgroundRuntimeInterrupted,
	KeyToolBackgroundFollowUpInstruction,
	KeyPresentationCronNextUnknown,
	KeyPresentationCronTimezoneLocal,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyCompactMicrocompactResultCleared,
		"[Old tool result content cleared]",
		"[旧工具结果内容已清除]",
		"[Inhalt des alten Tool-Ergebnisses gelöscht]",
		"[古い Tool 結果の内容を消去しました]",
		"[이전 Tool 결과 내용 삭제됨]",
		"[Содержимое прежнего результата Tool очищено]")
	add(KeyToolReadFileUnchanged,
		"File unchanged since last read. The content from the earlier Read tool_result in this conversation is still current — refer to that instead of re-reading.",
		"文件自上次读取后未发生变化。本次对话中先前 Read tool_result 的内容仍然有效，请直接参考该内容，无需重新读取。",
		"Die Datei ist seit dem letzten Lesen unverändert. Der Inhalt des früheren Read-tool_result in dieser Unterhaltung ist weiterhin aktuell; verwende ihn, statt die Datei erneut zu lesen.",
		"前回の読み取り以降、ファイルは変更されていません。この会話にある以前の Read tool_result はまだ最新です。再読み取りせず、そちらを参照してください。",
		"마지막으로 읽은 뒤 파일이 변경되지 않았습니다. 이 대화의 이전 Read tool_result 내용이 여전히 최신이므로 다시 읽지 말고 해당 내용을 참조하세요.",
		"Файл не изменился после последнего чтения. Содержимое предыдущего Read tool_result в этой беседе по-прежнему актуально; используйте его вместо повторного чтения.")
	add(KeyToolReadEmptyFileWarning,
		"<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>",
		"<system-reminder>警告：文件存在，但内容为空。</system-reminder>",
		"<system-reminder>Warnung: Die Datei ist vorhanden, aber leer.</system-reminder>",
		"<system-reminder>警告: ファイルは存在しますが、内容は空です。</system-reminder>",
		"<system-reminder>경고: 파일은 존재하지만 내용이 비어 있습니다.</system-reminder>",
		"<system-reminder>Предупреждение: файл существует, но не содержит данных.</system-reminder>")
	add(KeyToolReadOffsetBeyondEndWarning,
		"<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>",
		"<system-reminder>警告：文件存在，但长度未达到指定的 offset（%d）。该文件共有 %d 行。</system-reminder>",
		"<system-reminder>Warnung: Die Datei ist vorhanden, aber kürzer als der angegebene Offset (%d). Sie hat %d Zeilen.</system-reminder>",
		"<system-reminder>警告: ファイルは存在しますが、指定した offset（%d）より短く、全 %d 行です。</system-reminder>",
		"<system-reminder>경고: 파일은 존재하지만 지정한 offset(%d)보다 짧습니다. 파일은 %d줄입니다.</system-reminder>",
		"<system-reminder>Предупреждение: файл существует, но короче указанного offset (%d). В файле %d строк.</system-reminder>")
	add(KeyToolAgentForkStarted,
		"Fork started — processing in background",
		"Fork 已启动，正在后台处理",
		"Fork gestartet — Verarbeitung im Hintergrund",
		"Fork を開始しました。バックグラウンドで処理しています",
		"Fork가 시작되어 백그라운드에서 처리 중입니다",
		"Fork запущен — обработка выполняется в фоне")
	add(KeyLoopFallbackTombstoneSummary,
		"Assistant message replaced by fallback retry",
		"Assistant 消息已由 fallback 重试替换",
		"Assistant-Nachricht durch einen Fallback-Wiederholungsversuch ersetzt",
		"Assistant メッセージを fallback リトライで置き換えました",
		"Assistant 메시지가 fallback 재시도로 대체되었습니다",
		"Сообщение Assistant заменено повторной попыткой fallback")
	add(KeyToolBackgroundRuntimeInterrupted,
		"Agent run was interrupted when its owning runtime exited",
		"所属 runtime 退出时，Agent 运行被中断",
		"Der Agent-Lauf wurde beim Beenden der zugehörigen Runtime unterbrochen",
		"所有元の runtime が終了したため、Agent の実行が中断されました",
		"소유 runtime이 종료되어 Agent 실행이 중단되었습니다",
		"Запуск Agent прерван при завершении владеющей им runtime")
	add(KeyToolBackgroundFollowUpInstruction,
		"A background task has completed. Continue the user's request using this result. Do not expose internal task identifiers unless they are needed to explain an error.",
		"后台任务已完成。请使用该结果继续处理用户的请求。除非解释错误时确有必要，否则不要暴露内部任务标识符。",
		"Eine Hintergrundaufgabe ist abgeschlossen. Setze die Anfrage des Benutzers mit diesem Ergebnis fort. Gib interne Aufgabenkennungen nur an, wenn sie zur Erklärung eines Fehlers erforderlich sind.",
		"バックグラウンドタスクが完了しました。この結果を使ってユーザーの依頼を続行してください。エラーの説明に必要な場合を除き、内部タスク ID は公開しないでください。",
		"백그라운드 작업이 완료되었습니다. 이 결과를 사용해 사용자의 요청을 계속 처리하세요. 오류 설명에 필요한 경우가 아니면 내부 작업 식별자를 노출하지 마세요.",
		"Фоновая задача завершена. Продолжите выполнение запроса пользователя с учётом этого результата. Не раскрывайте внутренние идентификаторы задач, если они не нужны для объяснения ошибки.")
	add(KeyPresentationCronNextUnknown,
		"unknown",
		"未知",
		"unbekannt",
		"不明",
		"알 수 없음",
		"неизвестно")
	add(KeyPresentationCronTimezoneLocal,
		"local time",
		"本地时间",
		"Ortszeit",
		"現地時刻",
		"현지 시간",
		"местное время")
}
