package i18n

const (
	KeyLogHookUnknownEvent        Key = "log.hook.unknown_event"
	KeyLogSDKPermissionMode       Key = "log.sdk.permission_mode_changed"
	KeyLogDebugSessionStarted     Key = "log.debug.session_started"
	KeyLogDebugUnknownPhase       Key = "log.debug.unknown_phase"
	KeyLogDebugPayloadNil         Key = "log.debug.payload_nil"
	KeyLogDebugMarshalFailed      Key = "log.debug.marshal_failed"
	KeyLogAnthropicRequestError   Key = "log.anthropic.request_error"
	KeyLogAnthropicNormalizeError Key = "log.anthropic.normalize_error"
	KeyLogAnthropicBodyOmitted    Key = "log.anthropic.body_omitted"
	KeyLogAnthropicBodySniff      Key = "log.anthropic.body_sniff"
	KeyLogAnthropicNormalizedGzip Key = "log.anthropic.normalized_gzip"
	KeyLogAnthropicNormalizedZlib Key = "log.anthropic.normalized_zlib"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyLogHookUnknownEvent, "Unknown hook event type in configuration; skipping it", "配置中存在未知 hook 事件类型；已跳过", "Unbekannter Hook-Ereignistyp in der Konfiguration; wird übersprungen", "設定に不明な hook イベントタイプがあるためスキップします", "구성에 알 수 없는 hook 이벤트 유형이 있어 건너뜀", "Неизвестный тип события hook в конфигурации; пропуск")
	add(KeyLogSDKPermissionMode, "SDK permission mode changed", "SDK 权限模式已更改", "SDK-Berechtigungsmodus geändert", "SDK の権限モードを変更しました", "SDK 권한 모드 변경됨", "Режим разрешений SDK изменён")
	add(KeyLogDebugSessionStarted, "[Debug] session started %s", "[Debug] session 已启动 %s", "[Debug] Sitzung gestartet %s", "[Debug] session を開始しました %s", "[Debug] session 시작됨 %s", "[Debug] session запущена %s")
	add(KeyLogDebugUnknownPhase, "unknown debug phase %q", "未知的 debug phase %q", "Unbekannte Debug-Phase %q", "不明な debug phase %q", "알 수 없는 debug phase %q", "Неизвестная debug phase %q")
	add(KeyLogDebugPayloadNil, "debug %s payload is nil", "debug %s payload 为 nil", "Debug-Payload für %s ist nil", "debug %s の payload が nil です", "debug %s payload가 nil입니다", "Payload debug %s равен nil")
	add(KeyLogDebugMarshalFailed, "marshal debug %s: %v", "序列化 debug %s 失败：%v", "Debug-Daten für %s konnten nicht serialisiert werden: %v", "debug %s をシリアライズできませんでした: %v", "debug %s을(를) 직렬화하지 못했습니다: %v", "Не удалось сериализовать debug %s: %v")
	add(KeyLogAnthropicRequestError, "[anthropic-debug] request error: %v", "[anthropic-debug] 请求错误：%v", "[anthropic-debug] Anfragefehler: %v", "[anthropic-debug] リクエストエラー: %v", "[anthropic-debug] 요청 오류: %v", "[anthropic-debug] ошибка запроса: %v")
	add(KeyLogAnthropicNormalizeError, "[anthropic-debug] normalize error: %v", "[anthropic-debug] 规范化错误：%v", "[anthropic-debug] Normalisierungsfehler: %v", "[anthropic-debug] 正規化エラー: %v", "[anthropic-debug] 정규화 오류: %v", "[anthropic-debug] ошибка нормализации: %v")
	add(KeyLogAnthropicBodyOmitted, "[anthropic-debug] body omitted: content-type=%q content-encoding=%q", "[anthropic-debug] 已省略 body：content-type=%q content-encoding=%q", "[anthropic-debug] Body ausgelassen: content-type=%q content-encoding=%q", "[anthropic-debug] body を省略しました: content-type=%q content-encoding=%q", "[anthropic-debug] body 생략됨: content-type=%q content-encoding=%q", "[anthropic-debug] body пропущен: content-type=%q content-encoding=%q")
	add(KeyLogAnthropicBodySniff, "[anthropic-debug] body sniff bytes: % x", "[anthropic-debug] body 检测字节：% x", "[anthropic-debug] Erkannte Body-Bytes: % x", "[anthropic-debug] body 判定バイト: % x", "[anthropic-debug] body 감지 바이트: % x", "[anthropic-debug] контрольные байты body: % x")
	add(KeyLogAnthropicNormalizedGzip, "[anthropic-debug] normalized gzip-compressed SSE body without Content-Encoding header", "[anthropic-debug] 已规范化缺少 Content-Encoding header 的 gzip 压缩 SSE body", "[anthropic-debug] gzip-komprimierter SSE-Body ohne Content-Encoding-Header normalisiert", "[anthropic-debug] Content-Encoding header のない gzip 圧縮 SSE body を正規化しました", "[anthropic-debug] Content-Encoding header가 없는 gzip 압축 SSE body를 정규화했습니다", "[anthropic-debug] нормализован сжатый gzip SSE body без header Content-Encoding")
	add(KeyLogAnthropicNormalizedZlib, "[anthropic-debug] normalized deflate-compressed SSE body without Content-Encoding header", "[anthropic-debug] 已规范化缺少 Content-Encoding header 的 deflate 压缩 SSE body", "[anthropic-debug] deflate-komprimierter SSE-Body ohne Content-Encoding-Header normalisiert", "[anthropic-debug] Content-Encoding header のない deflate 圧縮 SSE body を正規化しました", "[anthropic-debug] Content-Encoding header가 없는 deflate 압축 SSE body를 정규화했습니다", "[anthropic-debug] нормализован сжатый deflate SSE body без header Content-Encoding")
}
