package i18n

const (
	KeyProviderOpenAIOAuthIDTokenEmpty                       Key = "provider.openai_oauth.id_token_empty"
	KeyProviderOpenAIOAuthIDTokenFormatInvalid               Key = "provider.openai_oauth.id_token_format_invalid"
	KeyProviderOpenAIOAuthIDTokenPayloadDecodeFailed         Key = "provider.openai_oauth.id_token_payload_decode_failed"
	KeyProviderOpenAIOAuthIDTokenPayloadParseFailed          Key = "provider.openai_oauth.id_token_payload_parse_failed"
	KeyProviderOpenAIOAuthAPIKeyExchangeRequestBuildFailed   Key = "provider.openai_oauth.api_key_exchange_request_build_failed"
	KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed        Key = "provider.openai_oauth.api_key_exchange_request_failed"
	KeyProviderOpenAIOAuthAPIKeyExchangeRejected             Key = "provider.openai_oauth.api_key_exchange_rejected"
	KeyProviderOpenAIOAuthAPIKeyExchangeResponseDecodeFailed Key = "provider.openai_oauth.api_key_exchange_response_decode_failed"
	KeyProviderOpenAIOAuthAPIKeyExchangeMissingAccessToken   Key = "provider.openai_oauth.api_key_exchange_missing_access_token"
)

var providerOpenAIOAuthKeys = []Key{
	KeyProviderOpenAIOAuthIDTokenEmpty,
	KeyProviderOpenAIOAuthIDTokenFormatInvalid,
	KeyProviderOpenAIOAuthIDTokenPayloadDecodeFailed,
	KeyProviderOpenAIOAuthIDTokenPayloadParseFailed,
	KeyProviderOpenAIOAuthAPIKeyExchangeRequestBuildFailed,
	KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed,
	KeyProviderOpenAIOAuthAPIKeyExchangeRejected,
	KeyProviderOpenAIOAuthAPIKeyExchangeResponseDecodeFailed,
	KeyProviderOpenAIOAuthAPIKeyExchangeMissingAccessToken,
}

func init() {
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthIDTokenEmpty,
		"openai oauth: id token is empty",
		"OpenAI OAuth：ID token 为空",
		"OpenAI OAuth: Das ID token ist leer",
		"OpenAI OAuth：ID token が空です",
		"OpenAI OAuth: ID token이 비어 있습니다",
		"OpenAI OAuth: ID token пуст")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthIDTokenFormatInvalid,
		"openai oauth: invalid id token format",
		"OpenAI OAuth：ID token 格式无效",
		"OpenAI OAuth: Das Format des ID token ist ungültig",
		"OpenAI OAuth：ID token の形式が無効です",
		"OpenAI OAuth: ID token 형식이 올바르지 않습니다",
		"OpenAI OAuth: недопустимый формат ID token")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthIDTokenPayloadDecodeFailed,
		"openai oauth: decode id token payload: %v",
		"OpenAI OAuth：解码 ID token payload 失败：%v",
		"OpenAI OAuth: Der Payload des ID token konnte nicht dekodiert werden: %v",
		"OpenAI OAuth：ID token の payload をデコードできませんでした：%v",
		"OpenAI OAuth: ID token payload를 디코딩하지 못했습니다: %v",
		"OpenAI OAuth: не удалось декодировать payload ID token: %v")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthIDTokenPayloadParseFailed,
		"openai oauth: parse id token payload: %v",
		"OpenAI OAuth：解析 ID token payload 失败：%v",
		"OpenAI OAuth: Der Payload des ID token konnte nicht geparst werden: %v",
		"OpenAI OAuth：ID token の payload を解析できませんでした：%v",
		"OpenAI OAuth: ID token payload를 파싱하지 못했습니다: %v",
		"OpenAI OAuth: не удалось разобрать payload ID token: %v")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthAPIKeyExchangeRequestBuildFailed,
		"openai oauth: build api-key exchange request: %v",
		"OpenAI OAuth：构建 API key 交换请求失败：%v",
		"OpenAI OAuth: Die Anfrage zum Austausch des API key konnte nicht erstellt werden: %v",
		"OpenAI OAuth：API key 交換リクエストを作成できませんでした：%v",
		"OpenAI OAuth: API key 교환 요청을 만들지 못했습니다: %v",
		"OpenAI OAuth: не удалось сформировать запрос обмена API key: %v")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed,
		"openai oauth: api-key exchange request: %v",
		"OpenAI OAuth：API key 交换请求失败：%v",
		"OpenAI OAuth: Die Anfrage zum Austausch des API key ist fehlgeschlagen: %v",
		"OpenAI OAuth：API key 交換リクエストに失敗しました：%v",
		"OpenAI OAuth: API key 교환 요청에 실패했습니다: %v",
		"OpenAI OAuth: запрос обмена API key завершился ошибкой: %v")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthAPIKeyExchangeRejected,
		"openai oauth: api-key exchange returned %d: %s",
		"OpenAI OAuth：API key 交换返回 HTTP %d：%s",
		"OpenAI OAuth: Der Austausch des API key hat HTTP %d zurückgegeben: %s",
		"OpenAI OAuth：API key 交換が HTTP %d を返しました：%s",
		"OpenAI OAuth: API key 교환이 HTTP %d을(를) 반환했습니다: %s",
		"OpenAI OAuth: обмен API key вернул HTTP %d: %s")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthAPIKeyExchangeResponseDecodeFailed,
		"openai oauth: decode api-key exchange response: %v",
		"OpenAI OAuth：解码 API key 交换响应失败：%v",
		"OpenAI OAuth: Die Antwort zum Austausch des API key konnte nicht dekodiert werden: %v",
		"OpenAI OAuth：API key 交換レスポンスをデコードできませんでした：%v",
		"OpenAI OAuth: API key 교환 응답을 디코딩하지 못했습니다: %v",
		"OpenAI OAuth: не удалось декодировать ответ обмена API key: %v")
	addProviderOpenAIOAuth(KeyProviderOpenAIOAuthAPIKeyExchangeMissingAccessToken,
		"openai oauth: api-key exchange response missing access_token",
		"OpenAI OAuth：API key 交换响应中缺少 access_token",
		"OpenAI OAuth: In der Antwort zum Austausch des API key fehlt access_token",
		"OpenAI OAuth：API key 交換レスポンスに access_token がありません",
		"OpenAI OAuth: API key 교환 응답에 access_token이 없습니다",
		"OpenAI OAuth: в ответе обмена API key отсутствует access_token")
}

func addProviderOpenAIOAuth(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
