package i18n

const (
	KeyProviderOpenAIStreamChunkParseFailed   Key = "provider.openai.stream_chunk_parse_failed"
	KeyProviderResponsesCompletedParseFailed  Key = "provider.responses.completed_parse_failed"
	KeyProviderResponsesIncompleteParseFailed Key = "provider.responses.incomplete_parse_failed"
	KeyProviderResponsesFailedParseFailed     Key = "provider.responses.failed_parse_failed"
	KeyProviderResponsesContinuationInvalid   Key = "provider.responses.continuation_invalid"
	KeyProviderResponsesCustomToolCallInvalid Key = "provider.responses.custom_tool_call_invalid"
	KeyProviderResponsesKnownEventParseFailed Key = "provider.responses.known_event_parse_failed"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyProviderOpenAIStreamChunkParseFailed,
		"failed to parse stream chunk: %v",
		"解析 stream chunk 失败：%v",
		"Stream-Chunk konnte nicht geparst werden: %v",
		"stream chunk の解析に失敗しました: %v",
		"stream chunk 파싱에 실패했습니다: %v",
		"Не удалось разобрать stream chunk: %v")
	add(KeyProviderResponsesCompletedParseFailed,
		"failed to parse response.completed: %v",
		"解析 response.completed 失败：%v",
		"response.completed konnte nicht geparst werden: %v",
		"response.completed の解析に失敗しました: %v",
		"response.completed 파싱에 실패했습니다: %v",
		"Не удалось разобрать response.completed: %v")
	add(KeyProviderResponsesIncompleteParseFailed,
		"failed to parse response.incomplete: %v",
		"解析 response.incomplete 失败：%v",
		"response.incomplete konnte nicht geparst werden: %v",
		"response.incomplete の解析に失敗しました: %v",
		"response.incomplete 파싱에 실패했습니다: %v",
		"Не удалось разобрать response.incomplete: %v")
	add(KeyProviderResponsesFailedParseFailed,
		"failed to parse response.failed: %v",
		"解析 response.failed 失败：%v",
		"response.failed konnte nicht geparst werden: %v",
		"response.failed の解析に失敗しました: %v",
		"response.failed 파싱에 실패했습니다: %v",
		"Не удалось разобрать response.failed: %v")
	add(KeyProviderResponsesContinuationInvalid,
		"The Responses continuation state is invalid.",
		"Responses 延续状态无效。",
		"Der Responses-Fortsetzungsstatus ist ungültig.",
		"Responses の継続状態が無効です。",
		"Responses 연속 상태가 올바르지 않습니다.",
		"Состояние продолжения Responses недействительно.")
	add(KeyProviderResponsesCustomToolCallInvalid,
		"The Responses custom tool call was incomplete or violated the declared protocol.",
		"Responses custom 工具调用不完整，或违反了声明的协议。",
		"Der Responses-Custom-Tool-Aufruf war unvollständig oder verletzte das deklarierte Protokoll.",
		"Responses custom ツール呼び出しが不完全か、宣言されたプロトコルに違反しています。",
		"Responses custom 도구 호출이 불완전하거나 선언된 프로토콜을 위반했습니다.",
		"Вызов custom-инструмента Responses был неполным или нарушил заявленный протокол.")
	add(KeyProviderResponsesKnownEventParseFailed,
		"The Responses event %s could not be decoded.",
		"无法解码 Responses 事件 %s。",
		"Das Responses-Ereignis %s konnte nicht dekodiert werden.",
		"Responses イベント %s をデコードできませんでした。",
		"Responses 이벤트 %s을(를) 디코딩할 수 없습니다.",
		"Не удалось декодировать событие Responses %s.")
}
