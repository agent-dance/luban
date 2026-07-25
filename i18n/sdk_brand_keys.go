package i18n

const (
	KeyBrandTagline                          Key = "brand.tagline"
	KeySDKReady                              Key = "sdk.ready"
	KeySDKSystemPromptReceived               Key = "sdk.system_prompt_received"
	KeySDKStdinReadError                     Key = "sdk.stdin_read_error"
	KeySDKInvalidJSONEnvelope                Key = "sdk.invalid_json_envelope"
	KeySDKUnknownMessageType                 Key = "sdk.unknown_message_type"
	KeySDKParseUserMessage                   Key = "sdk.parse_user_message"
	KeySDKUserUUIDRequired                   Key = "sdk.user_uuid_required"
	KeySDKExtractMessageText                 Key = "sdk.extract_message_text"
	KeySDKUnsupportedMessageContent          Key = "sdk.unsupported_message_content"
	KeySDKStreamEndedWithoutFinalEvent       Key = "sdk.stream_ended_without_final_event"
	KeySDKQueryCancelled                     Key = "sdk.query_cancelled"
	KeySDKQueryAlreadyActive                 Key = "sdk.query_already_active"
	KeySDKParseControlRequest                Key = "sdk.parse_control_request"
	KeySDKControlRequestIDRequired           Key = "sdk.control_request_id_required"
	KeySDKControlRequestIDConflict           Key = "sdk.control_request_id_conflict"
	KeySDKParseRequestSubtype                Key = "sdk.parse_request_subtype"
	KeySDKUnsupportedControlSubtype          Key = "sdk.unsupported_control_subtype"
	KeySDKControlUnavailableDuringQuery      Key = "sdk.control_unavailable_during_query"
	KeySDKParseControlPayload                Key = "sdk.parse_control_payload"
	KeySDKMarshalInitializeResponse          Key = "sdk.marshal_initialize_response"
	KeySDKMarshalResumeResponse              Key = "sdk.marshal_resume_response"
	KeySDKMarshalCompactResponse             Key = "sdk.marshal_compact_response"
	KeySDKMarshalContextUsage                Key = "sdk.marshal_context_usage"
	KeySDKParseControlResponse               Key = "sdk.parse_control_response"
	KeySDKUnrecognizedControlResponsePayload Key = "sdk.unrecognized_control_response_payload"
	KeySDKMarshalOutput                      Key = "sdk.marshal_output"
	KeySDKWriteOutput                        Key = "sdk.write_output"
	KeySDKServeAlreadyStarted                Key = "sdk.serve_already_started"
	KeySDKPermissionDuplicateRequestID       Key = "sdk.permission.duplicate_request_id"
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
	addSDKBrand(KeySDKUserUUIDRequired,
		"user message uuid is required",
		"用户消息必须包含 uuid",
		"Die Benutzernachricht benötigt eine uuid",
		"ユーザーメッセージには uuid が必要です",
		"사용자 메시지에는 uuid가 필요합니다",
		"Для сообщения пользователя требуется uuid")
	addSDKBrand(KeySDKExtractMessageText,
		"sdk: extract message text: %v",
		"SDK：提取消息文本失败：%v",
		"SDK: Nachrichtentext konnte nicht extrahiert werden: %v",
		"SDK: メッセージ本文を抽出できませんでした: %v",
		"SDK: 메시지 텍스트 추출 실패: %v",
		"SDK: не удалось извлечь текст сообщения: %v")
	addSDKBrand(KeySDKUnsupportedMessageContent,
		"unsupported user message content",
		"不支持的用户消息内容",
		"Nicht unterstützter Inhalt der Benutzernachricht",
		"未対応のユーザーメッセージ内容です",
		"지원하지 않는 사용자 메시지 콘텐츠",
		"Неподдерживаемое содержимое сообщения пользователя")
	addSDKBrand(KeySDKStreamEndedWithoutFinalEvent,
		"stream ended without final event",
		"流已结束，但未收到最终事件",
		"Stream wurde ohne Abschlussereignis beendet",
		"最終イベントがないままストリームが終了しました",
		"최종 이벤트 없이 스트림이 종료되었습니다",
		"Поток завершился без финального события")
	addSDKBrand(KeySDKQueryCancelled,
		"query cancelled",
		"查询已取消",
		"Abfrage abgebrochen",
		"クエリはキャンセルされました",
		"쿼리가 취소되었습니다",
		"Запрос отменён")
	addSDKBrand(KeySDKQueryAlreadyActive,
		"another query is already running",
		"已有另一个查询正在运行",
		"Eine andere Abfrage wird bereits ausgeführt",
		"別のクエリがすでに実行中です",
		"다른 쿼리가 이미 실행 중입니다",
		"Другой запрос уже выполняется")
	addSDKBrand(KeySDKParseControlRequest,
		"sdk: parse control_request: %v",
		"SDK：解析 control_request 失败：%v",
		"SDK: control_request konnte nicht analysiert werden: %v",
		"SDK: control_request を解析できませんでした: %v",
		"SDK: control_request 파싱 실패: %v",
		"SDK: не удалось разобрать control_request: %v")
	addSDKBrand(KeySDKControlRequestIDRequired,
		"control request_id is required",
		"控制消息必须包含 request_id",
		"Die Steuerungsanfrage benötigt eine request_id",
		"制御リクエストには request_id が必要です",
		"제어 요청에는 request_id가 필요합니다",
		"Для управляющего запроса требуется request_id")
	addSDKBrand(KeySDKControlRequestIDConflict,
		"control request ID %q was reused with a different payload",
		"控制请求 ID %q 被用于不同的 payload",
		"Die Steuerungsanfrage-ID %q wurde mit einer anderen Nutzlast wiederverwendet",
		"制御リクエスト ID %q が異なる payload で再利用されました",
		"제어 요청 ID %q이(가) 다른 payload에 재사용되었습니다",
		"ID управляющего запроса %q повторно использован с другой полезной нагрузкой")
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
	addSDKBrand(KeySDKControlUnavailableDuringQuery,
		"control subtype %q is unavailable while a query is running",
		"查询运行期间无法使用控制消息 subtype %q",
		"Steuerungs-subtype %q ist während einer laufenden Abfrage nicht verfügbar",
		"クエリの実行中は制御 subtype %q を使用できません",
		"쿼리 실행 중에는 제어 subtype %q을(를) 사용할 수 없습니다",
		"Управляющий subtype %q недоступен во время выполнения запроса")
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
	addSDKBrand(KeySDKMarshalCompactResponse,
		"marshal compact response: %v",
		"序列化 compact 响应失败：%v",
		"compact-Antwort konnte nicht serialisiert werden: %v",
		"compact レスポンスをシリアライズできませんでした: %v",
		"compact 응답 직렬화 실패: %v",
		"Не удалось сериализовать ответ compact: %v")
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
	addSDKBrand(KeySDKWriteOutput,
		"sdk: write output: %v",
		"SDK：写入输出失败：%v",
		"SDK: Ausgabe konnte nicht geschrieben werden: %v",
		"SDK: 出力の書き込みに失敗しました: %v",
		"SDK: 출력 쓰기 실패: %v",
		"SDK: не удалось записать вывод: %v")
	addSDKBrand(KeySDKServeAlreadyStarted,
		"sdk server can only be served once",
		"SDK 服务器只能启动一次",
		"Der SDK-Server kann nur einmal gestartet werden",
		"SDK サーバーは一度だけ起動できます",
		"SDK 서버는 한 번만 시작할 수 있습니다",
		"SDK-сервер можно запустить только один раз")
	addSDKBrand(KeySDKPermissionDuplicateRequestID,
		"sdk: permission request ID %q is already pending",
		"SDK：权限请求 ID %q 已在等待处理",
		"SDK: Berechtigungsanfrage-ID %q ist bereits ausstehend",
		"SDK: 権限リクエスト ID %q はすでに処理待ちです",
		"SDK: 권한 요청 ID %q이(가) 이미 대기 중입니다",
		"SDK: запрос разрешения с ID %q уже ожидает ответа")
}
