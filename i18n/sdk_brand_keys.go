package i18n

const (
	KeyBrandTagline                          Key = "brand.tagline"
	KeySDKReady                              Key = "sdk.ready"
	KeySDKSystemPromptReceived               Key = "sdk.system_prompt_received"
	KeySDKStdinReadError                     Key = "sdk.stdin_read_error"
	KeySDKInvalidJSONEnvelope                Key = "sdk.invalid_json_envelope"
	KeySDKUnknownMessageType                 Key = "sdk.unknown_message_type"
	KeySDKParseUserMessage                   Key = "sdk.parse_user_message"
	KeySDKExtractMessageText                 Key = "sdk.extract_message_text"
	KeySDKStreamEndedWithoutFinalEvent       Key = "sdk.stream_ended_without_final_event"
	KeySDKParseControlRequest                Key = "sdk.parse_control_request"
	KeySDKParseRequestSubtype                Key = "sdk.parse_request_subtype"
	KeySDKUnsupportedControlSubtype          Key = "sdk.unsupported_control_subtype"
	KeySDKParseControlPayload                Key = "sdk.parse_control_payload"
	KeySDKMarshalInitializeResponse          Key = "sdk.marshal_initialize_response"
	KeySDKMarshalResumeResponse              Key = "sdk.marshal_resume_response"
	KeySDKMarshalContextUsage                Key = "sdk.marshal_context_usage"
	KeySDKParseControlResponse               Key = "sdk.parse_control_response"
	KeySDKUnrecognizedControlResponsePayload Key = "sdk.unrecognized_control_response_payload"
	KeySDKMarshalOutput                      Key = "sdk.marshal_output"
)

func init() {
	addSDKBrand := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}

	addSDKBrand(KeyBrandTagline,
		"Agentic coding in your terminal",
		"让智能体在终端中编程",
		"KI-gestütztes Programmieren im Terminal",
		"ターミナルでエージェント型コーディング",
		"터미널에서 에이전트 기반 코딩",
		"Агентное программирование в терминале")
	addSDKBrand(KeySDKReady,
		"ready",
		"就绪",
		"bereit",
		"準備完了",
		"준비됨",
		"готово")
	addSDKBrand(KeySDKSystemPromptReceived,
		"system_prompt_received",
		"已收到系统提示词",
		"System-Prompt empfangen",
		"システムプロンプトを受信しました",
		"시스템 프롬프트를 받았습니다",
		"Системный промпт получен")
	addSDKBrand(KeySDKStdinReadError,
		"sdk: stdin read error: %v",
		"SDK：读取 stdin 失败：%v",
		"SDK: Fehler beim Lesen von stdin: %v",
		"SDK: stdin の読み取りに失敗しました: %v",
		"SDK: stdin 읽기 실패: %v",
		"SDK: ошибка чтения stdin: %v")
	addSDKBrand(KeySDKInvalidJSONEnvelope,
		"sdk: invalid JSON envelope: %v",
		"SDK：JSON 封装无效：%v",
		"SDK: ungültiges JSON-Envelope: %v",
		"SDK: JSON エンベロープが無効です: %v",
		"SDK: 잘못된 JSON 엔벌로프: %v",
		"SDK: недопустимый JSON-конверт: %v")
	addSDKBrand(KeySDKUnknownMessageType,
		"sdk: unknown message type %q",
		"SDK：未知的消息类型 %q",
		"SDK: unbekannter Nachrichtentyp %q",
		"SDK: 不明なメッセージタイプ %q",
		"SDK: 알 수 없는 메시지 유형 %q",
		"SDK: неизвестный тип сообщения %q")
	addSDKBrand(KeySDKParseUserMessage,
		"sdk: parse user message: %v",
		"SDK：解析用户消息失败：%v",
		"SDK: Benutzernachricht konnte nicht analysiert werden: %v",
		"SDK: ユーザーメッセージを解析できませんでした: %v",
		"SDK: 사용자 메시지 파싱 실패: %v",
		"SDK: не удалось разобрать сообщение пользователя: %v")
	addSDKBrand(KeySDKExtractMessageText,
		"sdk: extract message text: %v",
		"SDK：提取消息文本失败：%v",
		"SDK: Nachrichtentext konnte nicht extrahiert werden: %v",
		"SDK: メッセージ本文を抽出できませんでした: %v",
		"SDK: 메시지 텍스트 추출 실패: %v",
		"SDK: не удалось извлечь текст сообщения: %v")
	addSDKBrand(KeySDKStreamEndedWithoutFinalEvent,
		"stream ended without final event",
		"流已结束，但未收到最终事件",
		"Stream wurde ohne Abschlussereignis beendet",
		"最終イベントがないままストリームが終了しました",
		"최종 이벤트 없이 스트림이 종료되었습니다",
		"Поток завершился без финального события")
	addSDKBrand(KeySDKParseControlRequest,
		"sdk: parse control_request: %v",
		"SDK：解析 control_request 失败：%v",
		"SDK: control_request konnte nicht analysiert werden: %v",
		"SDK: control_request を解析できませんでした: %v",
		"SDK: control_request 파싱 실패: %v",
		"SDK: не удалось разобрать control_request: %v")
	addSDKBrand(KeySDKParseRequestSubtype,
		"parse request subtype: %v",
		"解析请求 subtype 失败：%v",
		"Anfrage-subtype konnte nicht analysiert werden: %v",
		"リクエストの subtype を解析できませんでした: %v",
		"요청 subtype 파싱 실패: %v",
		"Не удалось разобрать subtype запроса: %v")
	addSDKBrand(KeySDKUnsupportedControlSubtype,
		"unsupported control subtype %q",
		"不支持的控制消息 subtype %q",
		"Nicht unterstützter Steuerungs-subtype %q",
		"未対応の制御 subtype %q",
		"지원하지 않는 제어 subtype %q",
		"Неподдерживаемый subtype управляющего сообщения %q")
	addSDKBrand(KeySDKParseControlPayload,
		"parse %s: %v",
		"解析 %s 失败：%v",
		"%s konnte nicht analysiert werden: %v",
		"%s を解析できませんでした: %v",
		"%s 파싱 실패: %v",
		"Не удалось разобрать %s: %v")
	addSDKBrand(KeySDKMarshalInitializeResponse,
		"marshal initialize response: %v",
		"序列化 initialize 响应失败：%v",
		"initialize-Antwort konnte nicht serialisiert werden: %v",
		"initialize レスポンスをシリアライズできませんでした: %v",
		"initialize 응답 직렬화 실패: %v",
		"Не удалось сериализовать ответ initialize: %v")
	addSDKBrand(KeySDKMarshalResumeResponse,
		"marshal resume response: %v",
		"序列化 resume 响应失败：%v",
		"resume-Antwort konnte nicht serialisiert werden: %v",
		"resume レスポンスをシリアライズできませんでした: %v",
		"resume 응답 직렬화 실패: %v",
		"Не удалось сериализовать ответ resume: %v")
	addSDKBrand(KeySDKMarshalContextUsage,
		"marshal context usage: %v",
		"序列化上下文用量失败：%v",
		"Kontextnutzung konnte nicht serialisiert werden: %v",
		"コンテキスト使用量をシリアライズできませんでした: %v",
		"컨텍스트 사용량 직렬화 실패: %v",
		"Не удалось сериализовать использование контекста: %v")
	addSDKBrand(KeySDKParseControlResponse,
		"sdk: parse control_response: %v",
		"SDK：解析 control_response 失败：%v",
		"SDK: control_response konnte nicht analysiert werden: %v",
		"SDK: control_response を解析できませんでした: %v",
		"SDK: control_response 파싱 실패: %v",
		"SDK: не удалось разобрать control_response: %v")
	addSDKBrand(KeySDKUnrecognizedControlResponsePayload,
		"sdk: unrecognized control_response payload: %s",
		"SDK：无法识别 control_response 的 payload：%s",
		"SDK: nicht erkannte control_response-Nutzlast: %s",
		"SDK: control_response の payload を認識できません: %s",
		"SDK: 인식할 수 없는 control_response payload: %s",
		"SDK: нераспознанная полезная нагрузка control_response: %s")
	addSDKBrand(KeySDKMarshalOutput,
		"sdk: marshal output: %v",
		"SDK：序列化输出失败：%v",
		"SDK: Ausgabe konnte nicht serialisiert werden: %v",
		"SDK: 出力をシリアライズできませんでした: %v",
		"SDK: 출력 직렬화 실패: %v",
		"SDK: не удалось сериализовать вывод: %v")
}
