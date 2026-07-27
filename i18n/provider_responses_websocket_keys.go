package i18n

const (
	KeyProviderResponsesWebSocketProtocolInvalid    Key = "provider.responses_websocket.protocol_invalid"
	KeyProviderResponsesWebSocketCapacity           Key = "provider.responses_websocket.capacity"
	KeyProviderResponsesWebSocketEndpointInvalid    Key = "provider.responses_websocket.endpoint_invalid"
	KeyProviderResponsesWebSocketProfileUnsupported Key = "provider.responses_websocket.profile_unsupported"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyProviderResponsesWebSocketProtocolInvalid,
		"The Responses WebSocket returned an invalid protocol event",
		"Responses WebSocket 返回了无效的协议事件",
		"Der Responses-WebSocket hat ein ungültiges Protokollereignis zurückgegeben",
		"Responses WebSocket から無効なプロトコルイベントが返されました",
		"Responses WebSocket에서 잘못된 프로토콜 이벤트를 반환했습니다",
		"Responses WebSocket вернул недопустимое событие протокола")
	add(KeyProviderResponsesWebSocketCapacity,
		"No isolated Responses WebSocket connection slot is available",
		"当前没有可用的隔离 Responses WebSocket 连接槽位",
		"Es ist kein isolierter Responses-WebSocket-Verbindungsplatz verfügbar",
		"分離された Responses WebSocket 接続スロットに空きがありません",
		"격리된 Responses WebSocket 연결 슬롯을 사용할 수 없습니다",
		"Нет свободного изолированного слота подключения Responses WebSocket")
	add(KeyProviderResponsesWebSocketEndpointInvalid,
		"The Responses WebSocket endpoint is invalid",
		"Responses WebSocket endpoint 无效",
		"Der Responses-WebSocket-Endpunkt ist ungültig",
		"Responses WebSocket endpoint が無効です",
		"Responses WebSocket endpoint가 올바르지 않습니다",
		"Недопустимый endpoint Responses WebSocket")
	add(KeyProviderResponsesWebSocketProfileUnsupported,
		"Responses WebSocket requires the public OpenAI Responses API profile",
		"Responses WebSocket 要求使用 OpenAI 公共 Responses API 配置",
		"Responses WebSocket erfordert das öffentliche OpenAI-Responses-API-Profil",
		"Responses WebSocket には OpenAI 公開 Responses API プロファイルが必要です",
		"Responses WebSocket에는 OpenAI 공개 Responses API 프로필이 필요합니다",
		"Для Responses WebSocket требуется профиль публичного OpenAI Responses API")
}
