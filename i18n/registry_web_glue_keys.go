package i18n

// Semantic errors produced by the root registry adapters before WebFetch and
// WebSearch wrap them in user-visible ToolResult content. Product names and
// protocol terms remain English; raw provider errors stay in the error chain.
const (
	KeyRegistryWebSearchProviderNilStream       Key = "registry.web_search.provider.nil_stream"
	KeyRegistryWebSearchProviderStreamFailed    Key = "registry.web_search.provider.stream_failed"
	KeyRegistryWebSearchResultMissingRawContent Key = "registry.web_search.result.missing_raw_content"
	KeyRegistryWebSearchDecodeResultBlock       Key = "registry.web_search.result.decode_block"
	KeyRegistryWebSearchDecodeHits              Key = "registry.web_search.result.decode_hits"
	KeyRegistryWebSearchDecodeError             Key = "registry.web_search.result.decode_error"

	KeyRegistryWebFetchSecondaryProviderUnavailable Key = "registry.web_fetch.secondary_provider.unavailable"
	KeyRegistryWebFetchSecondaryModelNilStream      Key = "registry.web_fetch.secondary_model.nil_stream"
	KeyRegistryWebFetchSecondaryModelStreamFailed   Key = "registry.web_fetch.secondary_model.stream_failed"
)

var registryWebGlueKeys = []Key{
	KeyRegistryWebSearchProviderNilStream,
	KeyRegistryWebSearchProviderStreamFailed,
	KeyRegistryWebSearchResultMissingRawContent,
	KeyRegistryWebSearchDecodeResultBlock,
	KeyRegistryWebSearchDecodeHits,
	KeyRegistryWebSearchDecodeError,
	KeyRegistryWebFetchSecondaryProviderUnavailable,
	KeyRegistryWebFetchSecondaryModelNilStream,
	KeyRegistryWebFetchSecondaryModelStreamFailed,
}

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

	add(KeyRegistryWebSearchProviderNilStream,
		"WebSearch provider returned a nil stream",
		"WebSearch provider 返回了 nil stream",
		"Der WebSearch-Provider hat einen nil-Stream zurückgegeben",
		"WebSearch provider から nil stream が返されました",
		"WebSearch provider가 nil stream을 반환했습니다",
		"Provider WebSearch вернул nil stream")
	add(KeyRegistryWebSearchProviderStreamFailed,
		"WebSearch provider stream failed",
		"WebSearch provider 的 stream 失败",
		"Der Stream des WebSearch-Providers ist fehlgeschlagen",
		"WebSearch provider の stream でエラーが発生しました",
		"WebSearch provider의 stream이 실패했습니다",
		"Stream provider WebSearch завершился с ошибкой")
	add(KeyRegistryWebSearchResultMissingRawContent,
		"WebSearch result block omitted raw content",
		"WebSearch result block 缺少 raw content",
		"Im WebSearch-Ergebnisblock fehlt raw content",
		"WebSearch result block に raw content がありません",
		"WebSearch result block에 raw content가 없습니다",
		"В result block WebSearch отсутствует raw content")
	add(KeyRegistryWebSearchDecodeResultBlock,
		"decode WebSearch result block: %v",
		"无法解码 WebSearch result block：%v",
		"WebSearch-Ergebnisblock konnte nicht decodiert werden: %v",
		"WebSearch result block をデコードできませんでした: %v",
		"WebSearch result block을 디코딩할 수 없습니다: %v",
		"Не удалось декодировать result block WebSearch: %v")
	add(KeyRegistryWebSearchDecodeHits,
		"decode WebSearch hits: %v",
		"无法解码 WebSearch hits：%v",
		"WebSearch-Treffer konnten nicht decodiert werden: %v",
		"WebSearch hits をデコードできませんでした: %v",
		"WebSearch hits를 디코딩할 수 없습니다: %v",
		"Не удалось декодировать hits WebSearch: %v")
	add(KeyRegistryWebSearchDecodeError,
		"decode WebSearch error: %v",
		"无法解码 WebSearch error：%v",
		"WebSearch-Fehler konnte nicht decodiert werden: %v",
		"WebSearch error をデコードできませんでした: %v",
		"WebSearch error를 디코딩할 수 없습니다: %v",
		"Не удалось декодировать error WebSearch: %v")
	add(KeyRegistryWebFetchSecondaryProviderUnavailable,
		"WebFetch secondary model provider is unavailable",
		"WebFetch secondary model provider 不可用",
		"Der Provider für das WebFetch secondary model ist nicht verfügbar",
		"WebFetch secondary model provider を利用できません",
		"WebFetch secondary model provider를 사용할 수 없습니다",
		"Provider secondary model для WebFetch недоступен")
	add(KeyRegistryWebFetchSecondaryModelNilStream,
		"WebFetch secondary model returned a nil stream",
		"WebFetch secondary model 返回了 nil stream",
		"Das WebFetch secondary model hat einen nil-Stream zurückgegeben",
		"WebFetch secondary model から nil stream が返されました",
		"WebFetch secondary model이 nil stream을 반환했습니다",
		"Secondary model WebFetch вернула nil stream")
	add(KeyRegistryWebFetchSecondaryModelStreamFailed,
		"WebFetch secondary model stream failed",
		"WebFetch secondary model 的 stream 失败",
		"Der Stream des WebFetch secondary model ist fehlgeschlagen",
		"WebFetch secondary model の stream でエラーが発生しました",
		"WebFetch secondary model의 stream이 실패했습니다",
		"Stream secondary model WebFetch завершился с ошибкой")
}
