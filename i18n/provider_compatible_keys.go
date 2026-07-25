package i18n

const (
	KeyProviderCompatibleBaseURLRequired          Key = "provider.compatible.base_url_required"
	KeyProviderCompatibleBaseURLInvalid           Key = "provider.compatible.base_url_invalid"
	KeyProviderCompatibleCatalogUnavailable       Key = "provider.compatible.catalog_unavailable"
	KeyProviderCompatibleModelsRequestBuildFailed Key = "provider.compatible.models.request_build_failed"
	KeyProviderCompatibleModelsRequestFailed      Key = "provider.compatible.models.request_failed"
	KeyProviderCompatibleModelsReadFailed         Key = "provider.compatible.models.read_failed"
	KeyProviderCompatibleModelsHTTPFailed         Key = "provider.compatible.models.http_failed"
	KeyProviderCompatibleModelsDecodeFailed       Key = "provider.compatible.models.decode_failed"
	KeyProviderCompatibleModelsEmpty              Key = "provider.compatible.models.empty"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyProviderCompatibleBaseURLRequired,
		"This provider requires a Base URL", "此供应商必须设置 Base URL", "Dieser Anbieter benötigt eine Base URL", "このプロバイダーには Base URL が必要です", "이 공급자에는 Base URL이 필요합니다", "Для этого поставщика требуется Base URL")
	add(KeyProviderCompatibleBaseURLInvalid,
		"The Base URL is invalid", "Base URL 无效", "Die Base URL ist ungültig", "Base URL が無効です", "Base URL이 올바르지 않습니다", "Недопустимый Base URL")
	add(KeyProviderCompatibleCatalogUnavailable,
		"The model catalog is unavailable", "模型目录不可用", "Der Modellkatalog ist nicht verfügbar", "モデルカタログを利用できません", "모델 카탈로그를 사용할 수 없습니다", "Каталог моделей недоступен")
	add(KeyProviderCompatibleModelsRequestBuildFailed,
		"Could not build the model-list request: %v", "无法构建模型列表请求：%v", "Die Anfrage für die Modellliste konnte nicht erstellt werden: %v", "モデル一覧リクエストを作成できませんでした: %v", "모델 목록 요청을 만들 수 없습니다: %v", "Не удалось сформировать запрос списка моделей: %v")
	add(KeyProviderCompatibleModelsRequestFailed,
		"Could not fetch the model list: %v", "无法获取模型列表：%v", "Die Modellliste konnte nicht abgerufen werden: %v", "モデル一覧を取得できませんでした: %v", "모델 목록을 가져올 수 없습니다: %v", "Не удалось получить список моделей: %v")
	add(KeyProviderCompatibleModelsReadFailed,
		"Could not read the model-list response: %v", "无法读取模型列表响应：%v", "Die Antwort mit der Modellliste konnte nicht gelesen werden: %v", "モデル一覧の応答を読み取れませんでした: %v", "모델 목록 응답을 읽을 수 없습니다: %v", "Не удалось прочитать ответ со списком моделей: %v")
	add(KeyProviderCompatibleModelsHTTPFailed,
		"The model-list endpoint returned HTTP %d: %s", "模型列表接口返回 HTTP %d：%s", "Der Endpunkt für die Modellliste gab HTTP %d zurück: %s", "モデル一覧エンドポイントが HTTP %d を返しました: %s", "모델 목록 엔드포인트가 HTTP %d을(를) 반환했습니다: %s", "Конечная точка списка моделей вернула HTTP %d: %s")
	add(KeyProviderCompatibleModelsDecodeFailed,
		"Could not decode the model list: %v", "无法解析模型列表：%v", "Die Modellliste konnte nicht dekodiert werden: %v", "モデル一覧を解析できませんでした: %v", "모델 목록을 디코딩할 수 없습니다: %v", "Не удалось декодировать список моделей: %v")
	add(KeyProviderCompatibleModelsEmpty,
		"The endpoint returned no models", "接口未返回任何模型", "Der Endpunkt hat keine Modelle zurückgegeben", "エンドポイントからモデルが返されませんでした", "엔드포인트에서 모델을 반환하지 않았습니다", "Конечная точка не вернула моделей")
}
