package i18n

const (
	KeyLoopGoalEvaluatorProviderUnavailable Key = "loop.goal_evaluator.provider_unavailable"
	KeyLoopGoalEvaluatorProviderCallFailed  Key = "loop.goal_evaluator.provider_call_failed"
	KeyLoopGoalEvaluatorNilStream           Key = "loop.goal_evaluator.nil_stream"
	KeyLoopGoalEvaluatorStreamEnded         Key = "loop.goal_evaluator.stream_ended"
	KeyLoopGoalEvaluatorOutputLimit         Key = "loop.goal_evaluator.output_limit"
	KeyLoopGoalEvaluatorAttemptedTool       Key = "loop.goal_evaluator.attempted_tool"
	KeyLoopGoalEvaluatorStreamError         Key = "loop.goal_evaluator.stream_error"
	KeyLoopGoalEvaluatorStreamFailed        Key = "loop.goal_evaluator.stream_failed"
	KeyLoopGoalEvaluatorMarshalFailed       Key = "loop.goal_evaluator.marshal_failed"
	KeyLoopGoalEvaluatorEmptyResponse       Key = "loop.goal_evaluator.empty_response"
	KeyLoopGoalEvaluatorParseFailed         Key = "loop.goal_evaluator.parse_failed"
	KeyLoopGoalEvaluatorMissingMet          Key = "loop.goal_evaluator.missing_met"
	KeyLoopGoalEvaluatorMissingReason       Key = "loop.goal_evaluator.missing_reason"
	KeyLoopGoalEvaluatorReasonTooLong       Key = "loop.goal_evaluator.reason_too_long"
	KeyLoopGoalEvaluatorTrailingParseFailed Key = "loop.goal_evaluator.trailing_parse_failed"
	KeyLoopGoalEvaluatorMultipleJSON        Key = "loop.goal_evaluator.multiple_json"
)

var loopGoalEvaluatorKeys = [...]Key{
	KeyLoopGoalEvaluatorProviderUnavailable, KeyLoopGoalEvaluatorProviderCallFailed,
	KeyLoopGoalEvaluatorNilStream, KeyLoopGoalEvaluatorStreamEnded,
	KeyLoopGoalEvaluatorOutputLimit, KeyLoopGoalEvaluatorAttemptedTool,
	KeyLoopGoalEvaluatorStreamError, KeyLoopGoalEvaluatorStreamFailed,
	KeyLoopGoalEvaluatorMarshalFailed, KeyLoopGoalEvaluatorEmptyResponse,
	KeyLoopGoalEvaluatorParseFailed, KeyLoopGoalEvaluatorMissingMet,
	KeyLoopGoalEvaluatorMissingReason, KeyLoopGoalEvaluatorReasonTooLong,
	KeyLoopGoalEvaluatorTrailingParseFailed, KeyLoopGoalEvaluatorMultipleJSON,
}

func init() {
	entries := map[Key][6]string{
		KeyLoopGoalEvaluatorProviderUnavailable: {"goal evaluator provider is unavailable", "Goal evaluator provider 不可用", "Der Provider des Goal Evaluators ist nicht verfügbar", "Goal evaluator の provider を利用できません", "Goal evaluator provider를 사용할 수 없습니다", "Provider Goal evaluator недоступен"},
		KeyLoopGoalEvaluatorProviderCallFailed:  {"goal evaluator provider call: %v", "调用 Goal evaluator provider 失败：%v", "Aufruf des Goal-Evaluator-Providers fehlgeschlagen: %v", "Goal evaluator provider の呼び出しに失敗しました: %v", "Goal evaluator provider 호출에 실패했습니다: %v", "Ошибка вызова provider Goal evaluator: %v"},
		KeyLoopGoalEvaluatorNilStream:           {"goal evaluator provider returned a nil stream", "Goal evaluator provider 返回了 nil stream", "Der Goal-Evaluator-Provider hat einen nil-Stream zurückgegeben", "Goal evaluator provider が nil stream を返しました", "Goal evaluator provider가 nil stream을 반환했습니다", "Provider Goal evaluator вернул nil stream"},
		KeyLoopGoalEvaluatorStreamEnded:         {"goal evaluator stream ended before message_stop", "Goal evaluator stream 在 message_stop 前结束", "Der Goal-Evaluator-Stream endete vor message_stop", "Goal evaluator stream が message_stop より前に終了しました", "Goal evaluator stream이 message_stop 전에 종료되었습니다", "Поток Goal evaluator завершился до message_stop"},
		KeyLoopGoalEvaluatorOutputLimit:         {"goal evaluator response reached the output token limit", "Goal evaluator 响应达到了输出 token 上限", "Die Antwort des Goal Evaluators hat das Ausgabetoken-Limit erreicht", "Goal evaluator のレスポンスが出力 token 上限に達しました", "Goal evaluator 응답이 출력 token 한도에 도달했습니다", "Ответ Goal evaluator достиг лимита выходных token"},
		KeyLoopGoalEvaluatorAttemptedTool:       {"goal evaluator attempted to use a tool", "Goal evaluator 尝试使用 Tool", "Der Goal Evaluator hat versucht, ein Tool zu verwenden", "Goal evaluator が Tool を使用しようとしました", "Goal evaluator가 Tool 사용을 시도했습니다", "Goal evaluator попытался использовать Tool"},
		KeyLoopGoalEvaluatorStreamError:         {"goal evaluator stream: %v", "Goal evaluator stream 出错：%v", "Fehler im Goal-Evaluator-Stream: %v", "Goal evaluator stream でエラーが発生しました: %v", "Goal evaluator stream 오류: %v", "Ошибка потока Goal evaluator: %v"},
		KeyLoopGoalEvaluatorStreamFailed:        {"goal evaluator stream failed", "Goal evaluator stream 失败", "Der Goal-Evaluator-Stream ist fehlgeschlagen", "Goal evaluator stream に失敗しました", "Goal evaluator stream에 실패했습니다", "Поток Goal evaluator завершился ошибкой"},
		KeyLoopGoalEvaluatorMarshalFailed:       {"The goal evaluation transcript could not be encoded.", "无法编码 Goal evaluation 记录。", "Das Protokoll für die Goal-Bewertung konnte nicht codiert werden.", "Goal evaluation の記録をエンコードできませんでした。", "Goal evaluation 기록을 인코딩하지 못했습니다.", "Не удалось закодировать историю Goal evaluation."},
		KeyLoopGoalEvaluatorEmptyResponse:       {"goal evaluator returned an empty response", "Goal evaluator 返回了空响应", "Der Goal Evaluator hat eine leere Antwort zurückgegeben", "Goal evaluator から空のレスポンスが返されました", "Goal evaluator가 빈 응답을 반환했습니다", "Goal evaluator вернул пустой ответ"},
		KeyLoopGoalEvaluatorParseFailed:         {"parse goal evaluator response: %v", "解析 Goal evaluator 响应失败：%v", "Antwort des Goal Evaluators konnte nicht geparst werden: %v", "Goal evaluator のレスポンスを解析できませんでした: %v", "Goal evaluator 응답을 파싱하지 못했습니다: %v", "Не удалось разобрать ответ Goal evaluator: %v"},
		KeyLoopGoalEvaluatorMissingMet:          {"goal evaluator response omitted met", "Goal evaluator 响应缺少 met", "In der Antwort des Goal Evaluators fehlt met", "Goal evaluator のレスポンスに met がありません", "Goal evaluator 응답에 met가 없습니다", "В ответе Goal evaluator отсутствует met"},
		KeyLoopGoalEvaluatorMissingReason:       {"goal evaluator response omitted reason", "Goal evaluator 响应缺少 reason", "In der Antwort des Goal Evaluators fehlt reason", "Goal evaluator のレスポンスに reason がありません", "Goal evaluator 응답에 reason이 없습니다", "В ответе Goal evaluator отсутствует reason"},
		KeyLoopGoalEvaluatorReasonTooLong:       {"goal evaluator reason exceeds %d characters", "Goal evaluator 的 reason 超过 %d 个字符", "Die Begründung des Goal Evaluators überschreitet %d Zeichen", "Goal evaluator の reason が %d 文字を超えています", "Goal evaluator reason이 %d자를 초과합니다", "Причина Goal evaluator превышает %d символов"},
		KeyLoopGoalEvaluatorTrailingParseFailed: {"parse trailing goal evaluator response: %v", "解析 Goal evaluator 响应尾部失败：%v", "Nachlauf der Goal-Evaluator-Antwort konnte nicht geparst werden: %v", "Goal evaluator レスポンスの末尾を解析できませんでした: %v", "Goal evaluator 응답의 후행 데이터를 파싱하지 못했습니다: %v", "Не удалось разобрать остаток ответа Goal evaluator: %v"},
		KeyLoopGoalEvaluatorMultipleJSON:        {"goal evaluator returned multiple JSON values", "Goal evaluator 返回了多个 JSON 值", "Der Goal Evaluator hat mehrere JSON-Werte zurückgegeben", "Goal evaluator が複数の JSON 値を返しました", "Goal evaluator가 여러 JSON 값을 반환했습니다", "Goal evaluator вернул несколько значений JSON"},
	}
	for key, values := range entries {
		semanticTranslations[key] = map[Language]string{LangEN: values[0], LangZH: values[1], LangDE: values[2], LangJA: values[3], LangKO: values[4], LangRU: values[5]}
	}
}
