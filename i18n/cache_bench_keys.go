package i18n

const (
	KeyCacheBenchProviderInitFailed Key = "cache_bench.provider_init_failed"
	KeyCacheBenchProvider           Key = "cache_bench.provider"
	KeyCacheBenchHeader             Key = "cache_bench.header"
	KeyCacheBenchRoundFailed        Key = "cache_bench.round_failed"
	KeyCacheBenchNoUsage            Key = "cache_bench.no_usage"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyCacheBenchProviderInitFailed,
		"Could not initialize Provider: %v", "无法初始化 Provider：%v", "Provider konnte nicht initialisiert werden: %v", "Provider を初期化できませんでした: %v", "Provider를 초기화할 수 없습니다: %v", "Не удалось инициализировать Provider: %v")
	add(KeyCacheBenchProvider,
		"Provider: %s / %s", "Provider：%s / %s", "Provider: %s / %s", "Provider: %s / %s", "Provider: %s / %s", "Provider: %s / %s")
	add(KeyCacheBenchHeader,
		"Round        Input     Cached    Created   Uncached     Hit%", "轮次          输入       缓存       创建     未缓存     命中率", "Runde      Eingabe  Gecacht  Erstellt Ungecacht Treffer%", "回数          入力   キャッシュ      作成   未キャッシュ     ヒット率", "회차          입력       캐시       생성     미캐시     적중률", "Раунд         Ввод        Кэш     Создано  Без кэша  Попадание%")
	add(KeyCacheBenchRoundFailed,
		"Round %d failed: %v", "第 %d 轮失败：%v", "Runde %d fehlgeschlagen: %v", "第 %d 回に失敗しました: %v", "%d회차 실패: %v", "Раунд %d завершился ошибкой: %v")
	add(KeyCacheBenchNoUsage,
		"No usage data", "无用量数据", "Keine Nutzungsdaten", "使用量データなし", "사용량 데이터 없음", "Нет данных об использовании")
}
