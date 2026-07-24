package i18n

const (
	KeyToolSendMessageStructuredObjectRequired      Key = "tool.send_message.structured.object_required"
	KeyToolSendMessageStructuredTypeUnsupported     Key = "tool.send_message.structured.type_unsupported"
	KeyToolSendMessageStructuredFieldUnsupported    Key = "tool.send_message.structured.field_unsupported"
	KeyToolSendMessageStructuredFieldStringRequired Key = "tool.send_message.structured.field_string_required"
	KeyToolSendMessageStructuredFieldRequired       Key = "tool.send_message.structured.field_required"
)

var toolSendMessageValidationKeys = []Key{
	KeyToolSendMessageStructuredObjectRequired,
	KeyToolSendMessageStructuredTypeUnsupported,
	KeyToolSendMessageStructuredFieldUnsupported,
	KeyToolSendMessageStructuredFieldStringRequired,
	KeyToolSendMessageStructuredFieldRequired,
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

	add(KeyToolSendMessageStructuredObjectRequired,
		"structured message must be an object",
		"结构化消息必须是对象",
		"Die strukturierte Nachricht muss ein Objekt sein",
		"構造化メッセージはオブジェクトである必要があります",
		"구조화된 메시지는 객체여야 합니다",
		"Структурированное сообщение должно быть объектом")
	add(KeyToolSendMessageStructuredTypeUnsupported,
		"unsupported structured message type: %s",
		"不支持的结构化消息类型：%s",
		"Nicht unterstützter Typ der strukturierten Nachricht: %s",
		"サポートされていない構造化メッセージタイプです: %s",
		"지원되지 않는 구조화 메시지 유형: %s",
		"Неподдерживаемый тип структурированного сообщения: %s")
	add(KeyToolSendMessageStructuredFieldUnsupported,
		"unsupported structured message field: %s",
		"不支持的结构化消息字段：%s",
		"Nicht unterstütztes Feld der strukturierten Nachricht: %s",
		"サポートされていない構造化メッセージフィールドです: %s",
		"지원되지 않는 구조화 메시지 필드: %s",
		"Неподдерживаемое поле структурированного сообщения: %s")
	add(KeyToolSendMessageStructuredFieldStringRequired,
		"%s must be a string",
		"字段 %s 必须是字符串",
		"Das Feld %s muss eine Zeichenfolge sein",
		"フィールド %s は文字列である必要があります",
		"필드 %s은(는) 문자열이어야 합니다",
		"Поле %s должно быть строкой")
	add(KeyToolSendMessageStructuredFieldRequired,
		"%s requires %s",
		"%s 需要字段 %s",
		"%s erfordert das Feld %s",
		"%s にはフィールド %s が必要です",
		"%s에는 %s 필드가 필요합니다",
		"Для %s требуется поле %s")
}
