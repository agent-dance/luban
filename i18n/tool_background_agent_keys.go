package i18n

// Semantic copy emitted by background task and Agent result lifecycles. Task
// and Agent identifiers, durations, statuses, paths, and raw causes stay as
// format arguments so protocol values are never translated.
const (
	KeyToolBackgroundTaskCanceled           Key = "tool.background_agent.task.canceled"
	KeyToolBackgroundCommandTimedOut        Key = "tool.background_agent.command.timed_out"
	KeyToolBackgroundCommandTimedOutAfter   Key = "tool.background_agent.command.timed_out_after"
	KeyToolBackgroundOutputOpenFailed       Key = "tool.background_agent.output.open_failed"
	KeyToolBackgroundAgentEmptyOutput       Key = "tool.background_agent.result.empty_output"
	KeyToolBackgroundAgentFailed            Key = "tool.background_agent.result.failed"
	KeyToolBackgroundAgentCanceledWithCause Key = "tool.background_agent.result.canceled_with_cause"
	KeyToolBackgroundAgentTimedOutWithCause Key = "tool.background_agent.result.timed_out_with_cause"
)

var toolBackgroundAgentKeys = [...]Key{
	KeyToolBackgroundTaskCanceled,
	KeyToolBackgroundCommandTimedOut,
	KeyToolBackgroundCommandTimedOutAfter,
	KeyToolBackgroundOutputOpenFailed,
	KeyToolBackgroundAgentEmptyOutput,
	KeyToolBackgroundAgentFailed,
	KeyToolBackgroundAgentCanceledWithCause,
	KeyToolBackgroundAgentTimedOutWithCause,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolBackgroundTaskCanceled,
		"background task canceled",
		"后台任务已取消",
		"Hintergrundaufgabe abgebrochen",
		"バックグラウンドタスクはキャンセルされました",
		"백그라운드 작업이 취소되었습니다",
		"Фоновая задача отменена")
	add(KeyToolBackgroundCommandTimedOut,
		"background command timed out",
		"后台命令超时",
		"Zeitüberschreitung beim Hintergrundbefehl",
		"バックグラウンドコマンドがタイムアウトしました",
		"백그라운드 명령 시간이 초과되었습니다",
		"Превышено время ожидания фоновой команды")
	add(KeyToolBackgroundCommandTimedOutAfter,
		"background command timed out after %s",
		"后台命令在 %s 后超时",
		"Zeitüberschreitung beim Hintergrundbefehl nach %s",
		"バックグラウンドコマンドは %s 後にタイムアウトしました",
		"백그라운드 명령이 %s 후 시간 초과되었습니다",
		"Время ожидания фоновой команды истекло через %s")
	add(KeyToolBackgroundOutputOpenFailed,
		"open background output file: %v",
		"无法打开后台输出文件：%v",
		"Hintergrund-Ausgabedatei konnte nicht geöffnet werden: %v",
		"バックグラウンド出力ファイルを開けませんでした: %v",
		"백그라운드 출력 파일을 열 수 없습니다: %v",
		"Не удалось открыть файл фонового вывода: %v")
	add(KeyToolBackgroundAgentEmptyOutput,
		"(Subagent completed but returned no output.)",
		"（Subagent 已完成，但未返回任何输出。）",
		"(Subagent wurde abgeschlossen, hat aber keine Ausgabe zurückgegeben.)",
		"（Subagent は完了しましたが、出力はありませんでした。）",
		"(Subagent가 완료되었지만 출력이 없습니다.)",
		"(Subagent завершён, но не вернул вывод.)")
	add(KeyToolBackgroundAgentFailed,
		"agent failed",
		"Agent 执行失败",
		"Agent fehlgeschlagen",
		"Agent が失敗しました",
		"Agent가 실패했습니다",
		"Ошибка Agent")
	add(KeyToolBackgroundAgentCanceledWithCause,
		"%v",
		"Agent 已取消：%v",
		"Agent abgebrochen: %v",
		"Agent はキャンセルされました: %v",
		"Agent가 취소되었습니다: %v",
		"Agent отменён: %v")
	add(KeyToolBackgroundAgentTimedOutWithCause,
		"%v",
		"Agent 超时：%v",
		"Zeitüberschreitung beim Agent: %v",
		"Agent がタイムアウトしました: %v",
		"Agent 시간이 초과되었습니다: %v",
		"Превышено время ожидания Agent: %v")
}
