package i18n

// Semantic failures for the prompt/provider visible-tool envelope. Internal
// schema names, hashes, and registry diagnostics remain wrapped causes and are
// not interpolated into user-visible copy.
const (
	KeyRootVisibleToolCatalogInvalid      Key = "root.visible_tool_catalog.invalid"
	KeyLoopQueryVisibleToolCatalogInvalid Key = "loop.query.visible_tool_catalog.invalid"
	KeyLoopQueryToolOutsideVisibleCatalog Key = "loop.query.tool_outside_visible_catalog"
)

var visibleToolCatalogKeys = [...]Key{
	KeyRootVisibleToolCatalogInvalid,
	KeyLoopQueryVisibleToolCatalogInvalid,
	KeyLoopQueryToolOutsideVisibleCatalog,
}

func init() {
	entries := map[Key][6]string{
		KeyRootVisibleToolCatalogInvalid: {
			"The model-visible tool catalog could not be prepared.",
			"无法准备模型可见的工具目录。",
			"Der für das Modell sichtbare Werkzeugkatalog konnte nicht vorbereitet werden.",
			"モデルに表示するツールカタログを準備できませんでした。",
			"모델에 표시할 도구 카탈로그를 준비하지 못했습니다.",
			"Не удалось подготовить видимый модели каталог инструментов.",
		},
		KeyLoopQueryVisibleToolCatalogInvalid: {
			"The model-visible tool catalog changed inconsistently while preparing the request.",
			"准备请求时，模型可见的工具目录发生了不一致的变化。",
			"Der für das Modell sichtbare Werkzeugkatalog wurde während der Anfragevorbereitung inkonsistent geändert.",
			"リクエストの準備中に、モデルに表示するツールカタログが一貫しない状態で変更されました。",
			"요청을 준비하는 동안 모델에 표시할 도구 카탈로그가 일관되지 않게 변경되었습니다.",
			"При подготовке запроса видимый модели каталог инструментов изменился несогласованно.",
		},
		KeyLoopQueryToolOutsideVisibleCatalog: {
			"The model requested tool %s outside the catalog supplied for this turn.",
			"模型请求了本轮所提供目录之外的工具 %s。",
			"Das Modell hat das Werkzeug %s angefordert, das nicht im für diese Runde bereitgestellten Katalog enthalten ist.",
			"モデルが、このターンに提供されたカタログにないツール %s を要求しました。",
			"모델이 이번 턴에 제공된 카탈로그에 없는 도구 %s을(를) 요청했습니다.",
			"Модель запросила инструмент %s, отсутствующий в каталоге, переданном для этого хода.",
		},
	}
	for key, translations := range entries {
		semanticTranslations[key] = map[Language]string{
			LangEN: translations[0],
			LangZH: translations[1],
			LangDE: translations[2],
			LangJA: translations[3],
			LangKO: translations[4],
			LangRU: translations[5],
		}
	}
}
