package i18n

const (
	KeyCLIFlagResponsesWebSocket              Key = "cli.flag.responses_websocket"
	KeyCLIResponsesWebSocketRequiresResponses Key = "cli.responses_websocket.requires_responses"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyCLIFlagResponsesWebSocket,
		"Use the OpenAI public Responses WebSocket transport",
		"使用 OpenAI 公共 Responses WebSocket 传输",
		"Den öffentlichen OpenAI-Responses-WebSocket-Transport verwenden",
		"OpenAI 公開 Responses WebSocket トランスポートを使用します",
		"OpenAI 공개 Responses WebSocket 전송을 사용합니다",
		"Использовать публичный транспорт OpenAI Responses WebSocket")
	add(KeyCLIResponsesWebSocketRequiresResponses,
		"Responses WebSocket requires the responses API format",
		"Responses WebSocket 要求使用 responses API 格式",
		"Responses WebSocket erfordert das API-Format responses",
		"Responses WebSocket には responses API 形式が必要です",
		"Responses WebSocket에는 responses API 형식이 필요합니다",
		"Для Responses WebSocket требуется формат API responses")
}
