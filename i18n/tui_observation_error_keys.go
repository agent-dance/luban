package i18n

// Semantic errors emitted while the TUI correlates tool calls with retained
// observation evidence. Tool names, tool use IDs, match counts, and storage or
// encoding causes remain raw parameters.
const (
	KeyTUIObservationStoreNotFound          Key = "tui.observation.error.not_found"
	KeyTUIObservationMissingToolUseID       Key = "tui.observation.error.missing_tool_use_id"
	KeyTUIObservationToolUseIDConflict      Key = "tui.observation.error.tool_use_id_conflict"
	KeyTUIObservationToolCallMissingID      Key = "tui.observation.error.tool_call_missing_id"
	KeyTUIObservationToolCallIDConflict     Key = "tui.observation.error.tool_call_id_conflict"
	KeyTUIObservationRetainResultEvidence   Key = "tui.observation.error.retain_result_evidence"
	KeyTUIObservationEncodeStructuredResult Key = "tui.observation.error.encode_structured_result"
	KeyTUIObservationRetainStructuredResult Key = "tui.observation.error.retain_structured_result"
	KeyTUIObservationToolResultMissingID    Key = "tui.observation.error.tool_result_missing_id"
	KeyTUIObservationToolResultMatchCount   Key = "tui.observation.error.tool_result_match_count"
)

var tuiObservationErrorKeys = [...]Key{
	KeyTUIObservationStoreNotFound,
	KeyTUIObservationMissingToolUseID,
	KeyTUIObservationToolUseIDConflict,
	KeyTUIObservationToolCallMissingID,
	KeyTUIObservationToolCallIDConflict,
	KeyTUIObservationRetainResultEvidence,
	KeyTUIObservationEncodeStructuredResult,
	KeyTUIObservationRetainStructuredResult,
	KeyTUIObservationToolResultMissingID,
	KeyTUIObservationToolResultMatchCount,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyTUIObservationStoreNotFound,
		"observation not found",
		"找不到观察记录",
		"Beobachtung nicht gefunden",
		"観測が見つかりません",
		"관찰을 찾을 수 없습니다",
		"Наблюдение не найдено")
	add(KeyTUIObservationMissingToolUseID,
		"missing tool use ID",
		"缺少 tool use ID",
		"tool use ID fehlt",
		"tool use ID がありません",
		"tool use ID가 없습니다",
		"Отсутствует tool use ID")
	add(KeyTUIObservationToolUseIDConflict,
		"tool use ID conflict",
		"tool use ID 冲突",
		"Konflikt bei der tool use ID",
		"tool use ID が競合しています",
		"tool use ID가 충돌합니다",
		"Конфликт tool use ID")
	add(KeyTUIObservationToolCallMissingID,
		"tool call %q: %v",
		"工具调用 %q：%v",
		"Tool-Aufruf %q: %v",
		"Tool 呼び出し %q：%v",
		"Tool 호출 %q: %v",
		"Вызов Tool %q: %v")
	add(KeyTUIObservationToolCallIDConflict,
		"tool call %q (%s): %v",
		"工具调用 %q（%s）：%v",
		"Tool-Aufruf %q (%s): %v",
		"Tool 呼び出し %q（%s）：%v",
		"Tool 호출 %q(%s): %v",
		"Вызов Tool %q (%s): %v")
	add(KeyTUIObservationRetainResultEvidence,
		"retain tool result evidence: %v",
		"保留工具结果证据失败：%v",
		"Tool-Ergebnisbeleg konnte nicht gespeichert werden: %v",
		"Tool 結果の証拠を保存できませんでした：%v",
		"Tool 결과 증거를 보관하지 못했습니다: %v",
		"Не удалось сохранить доказательства результата Tool: %v")
	add(KeyTUIObservationEncodeStructuredResult,
		"encode structured tool result evidence: %v",
		"编码结构化工具结果证据失败：%v",
		"Strukturierter Tool-Ergebnisbeleg konnte nicht kodiert werden: %v",
		"構造化された Tool 結果の証拠をエンコードできませんでした：%v",
		"구조화된 Tool 결과 증거를 인코딩하지 못했습니다: %v",
		"Не удалось закодировать структурированные доказательства результата Tool: %v")
	add(KeyTUIObservationRetainStructuredResult,
		"retain structured tool result evidence: %v",
		"保留结构化工具结果证据失败：%v",
		"Strukturierter Tool-Ergebnisbeleg konnte nicht gespeichert werden: %v",
		"構造化された Tool 結果の証拠を保存できませんでした：%v",
		"구조화된 Tool 결과 증거를 보관하지 못했습니다: %v",
		"Не удалось сохранить структурированные доказательства результата Tool: %v")
	add(KeyTUIObservationToolResultMissingID,
		"tool result: %v",
		"工具结果：%v",
		"Tool-Ergebnis: %v",
		"Tool 結果：%v",
		"Tool 결과: %v",
		"Результат Tool: %v")
	add(KeyTUIObservationToolResultMatchCount,
		"tool result %s has %d matching calls: %v",
		"工具结果 %s 匹配到 %d 个调用：%v",
		"Zum Tool-Ergebnis %s passen %d Aufrufe: %v",
		"Tool 結果 %s に一致する呼び出しは %d 件あります：%v",
		"Tool 결과 %s에 일치하는 호출이 %d개 있습니다: %v",
		"Результату Tool %s соответствуют вызовы (%d): %v")
}
