package i18n

const (
	KeyProviderOpenAIStreamChunkParseFailed  Key = "provider.openai.stream_chunk_parse_failed"
	KeyProviderResponsesCompletedParseFailed Key = "provider.responses.completed_parse_failed"
	KeyProviderResponsesFailedParseFailed    Key = "provider.responses.failed_parse_failed"
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
	add(KeyProviderResponsesFailedParseFailed,
		"failed to parse response.failed: %v",
		"解析 response.failed 失败：%v",
		"response.failed konnte nicht geparst werden: %v",
		"response.failed の解析に失敗しました: %v",
		"response.failed 파싱에 실패했습니다: %v",
		"Не удалось разобрать response.failed: %v")
}
