package i18n

const (
	KeyLogMCPStartFailed          Key = "log.mcp.start_failed"
	KeyLogMCPStarted              Key = "log.mcp.started"
	KeyLogMCPSigtermFailed        Key = "log.mcp.sigterm_failed"
	KeyLogMCPShutdownTimeout      Key = "log.mcp.shutdown_timeout"
	KeyLogMCPStopped              Key = "log.mcp.stopped"
	KeyLogMCPRestarted            Key = "log.mcp.restarted"
	KeyLogMCPServerNotFound       Key = "log.mcp.server_not_found"
	KeyLogMCPHealthStopped        Key = "log.mcp.health_stopped"
	KeyLogMCPHealthLoopStarted    Key = "log.mcp.health_loop_started"
	KeyLogMCPHealthLoopStopped    Key = "log.mcp.health_loop_stopped"
	KeyLogMCPHealthyAgain         Key = "log.mcp.healthy_again"
	KeyLogMCPPingFailed           Key = "log.mcp.ping_failed"
	KeyLogMCPMarkedUnhealthy      Key = "log.mcp.marked_unhealthy"
	KeyLogMCPReconnectNotFound    Key = "log.mcp.reconnect_not_found"
	KeyLogMCPReconnectLoopStarted Key = "log.mcp.reconnect_loop_started"
	KeyLogMCPReconnectLoopStopped Key = "log.mcp.reconnect_loop_stopped"
	KeyLogMCPUnexpectedExit       Key = "log.mcp.unexpected_exit"
	KeyLogMCPStableReset          Key = "log.mcp.stable_reset"
	KeyLogMCPRestartAttempt       Key = "log.mcp.restart_attempt"
	KeyLogMCPRestartSucceeded     Key = "log.mcp.restart_succeeded"
	KeyLogMCPRestartFailed        Key = "log.mcp.restart_failed"
	KeyLogMCPReconnectExhausted   Key = "log.mcp.reconnect_exhausted"
	KeyLogMCPStreamReconnect      Key = "log.mcp.stream_reconnect"
	KeyLogMCPUnparseableEvent     Key = "log.mcp.unparseable_event"
	KeyLogHookUnknownEvent        Key = "log.hook.unknown_event"
	KeyLogTmuxBorderStatusFailed  Key = "log.tmux.border_status_failed"
	KeyLogTmuxBorderFormatFailed  Key = "log.tmux.border_format_failed"
	KeyLogSDKSessionStatFailed    Key = "log.sdk.session_stat_failed"
	KeyLogSDKSessionPartialRead   Key = "log.sdk.session_partial_read"
	KeyLogSDKSessionDeleted       Key = "log.sdk.session_deleted"
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
	add(KeyLogMCPStartFailed, "MCP server failed to start", "MCP server 启动失败", "MCP-Server konnte nicht gestartet werden", "MCP server の起動に失敗しました", "MCP server 시작 실패", "Не удалось запустить MCP server")
	add(KeyLogMCPStarted, "MCP server started", "MCP server 已启动", "MCP-Server gestartet", "MCP server を起動しました", "MCP server 시작됨", "MCP server запущен")
	add(KeyLogMCPSigtermFailed, "Could not send SIGTERM to MCP server", "无法向 MCP server 发送 SIGTERM", "SIGTERM konnte nicht an den MCP-Server gesendet werden", "MCP server に SIGTERM を送信できませんでした", "MCP server에 SIGTERM을 보낼 수 없음", "Не удалось отправить SIGTERM MCP server")
	add(KeyLogMCPShutdownTimeout, "MCP server did not stop gracefully; killing it", "MCP server 未能正常停止；将强制终止", "MCP-Server wurde nicht ordnungsgemäß beendet und wird erzwungen gestoppt", "MCP server が正常に停止しなかったため強制終了します", "MCP server가 정상 종료되지 않아 강제 종료함", "MCP server не завершился корректно; выполняется принудительная остановка")
	add(KeyLogMCPStopped, "MCP server stopped", "MCP server 已停止", "MCP-Server gestoppt", "MCP server を停止しました", "MCP server 중지됨", "MCP server остановлен")
	add(KeyLogMCPRestarted, "MCP server restarted", "MCP server 已重启", "MCP-Server neu gestartet", "MCP server を再起動しました", "MCP server 다시 시작됨", "MCP server перезапущен")
	add(KeyLogMCPServerNotFound, "MCP server not found", "未找到 MCP server", "MCP-Server nicht gefunden", "MCP server が見つかりません", "MCP server를 찾을 수 없음", "MCP server не найден")
	add(KeyLogMCPHealthStopped, "MCP health check stopped", "MCP 健康检查已停止", "MCP-Zustandsprüfung gestoppt", "MCP のヘルスチェックを停止しました", "MCP 상태 검사 중지됨", "Проверка состояния MCP остановлена")
	add(KeyLogMCPHealthLoopStarted, "MCP health-check loop started", "MCP 健康检查循环已启动", "MCP-Zustandsprüfungsschleife gestartet", "MCP ヘルスチェックループを開始しました", "MCP 상태 검사 루프 시작됨", "Цикл проверки состояния MCP запущен")
	add(KeyLogMCPHealthLoopStopped, "MCP health-check loop stopped", "MCP 健康检查循环已停止", "MCP-Zustandsprüfungsschleife gestoppt", "MCP ヘルスチェックループを停止しました", "MCP 상태 검사 루프 중지됨", "Цикл проверки состояния MCP остановлен")
	add(KeyLogMCPHealthyAgain, "MCP server is healthy again", "MCP server 已恢复健康", "MCP-Server ist wieder funktionsfähig", "MCP server は正常な状態に戻りました", "MCP server가 다시 정상 상태임", "MCP server снова исправен")
	add(KeyLogMCPPingFailed, "MCP health-check ping failed", "MCP 健康检查 ping 失败", "MCP-Ping zur Zustandsprüfung fehlgeschlagen", "MCP ヘルスチェックの ping に失敗しました", "MCP 상태 검사 ping 실패", "Ping проверки состояния MCP завершился ошибкой")
	add(KeyLogMCPMarkedUnhealthy, "MCP server marked unhealthy; reconnecting", "MCP server 已标记为异常；正在重新连接", "MCP-Server als fehlerhaft markiert; erneute Verbindung", "MCP server を異常と判断し、再接続します", "MCP server가 비정상으로 표시되어 다시 연결함", "MCP server признан неисправным; выполняется переподключение")
	add(KeyLogMCPReconnectNotFound, "MCP server not found; reconnect was not enabled", "未找到 MCP server；未启用重新连接", "MCP-Server nicht gefunden; erneute Verbindung wurde nicht aktiviert", "MCP server が見つからないため再接続を有効にできませんでした", "MCP server를 찾을 수 없어 재연결을 활성화하지 못함", "MCP server не найден; переподключение не включено")
	add(KeyLogMCPReconnectLoopStarted, "MCP reconnect loop started", "MCP 重连循环已启动", "MCP-Schleife zur erneuten Verbindung gestartet", "MCP 再接続ループを開始しました", "MCP 재연결 루프 시작됨", "Цикл переподключения MCP запущен")
	add(KeyLogMCPReconnectLoopStopped, "MCP reconnect loop stopped", "MCP 重连循环已停止", "MCP-Schleife zur erneuten Verbindung gestoppt", "MCP 再接続ループを停止しました", "MCP 재연결 루프 중지됨", "Цикл переподключения MCP остановлен")
	add(KeyLogMCPUnexpectedExit, "MCP server exited unexpectedly; reconnecting", "MCP server 意外退出；正在重新连接", "MCP-Server wurde unerwartet beendet; erneute Verbindung", "MCP server が予期せず終了したため再接続します", "MCP server가 예기치 않게 종료되어 다시 연결함", "MCP server неожиданно завершился; выполняется переподключение")
	add(KeyLogMCPStableReset, "MCP server remained stable; resetting reconnect backoff", "MCP server 保持稳定；正在重置重连退避", "MCP-Server blieb stabil; Wartezeit für erneute Verbindung wird zurückgesetzt", "MCP server が安定していたため再接続の待機時間をリセットします", "MCP server가 안정적으로 유지되어 재연결 backoff를 초기화함", "MCP server работал стабильно; задержка переподключения сброшена")
	add(KeyLogMCPRestartAttempt, "Attempting to restart MCP server", "正在尝试重启 MCP server", "MCP-Server wird neu gestartet", "MCP server の再起動を試みます", "MCP server 다시 시작 시도 중", "Попытка перезапустить MCP server")
	add(KeyLogMCPRestartSucceeded, "MCP server restart succeeded", "MCP server 重启成功", "MCP-Server erfolgreich neu gestartet", "MCP server の再起動に成功しました", "MCP server 다시 시작 성공", "MCP server успешно перезапущен")
	add(KeyLogMCPRestartFailed, "MCP server restart failed", "MCP server 重启失败", "Neustart des MCP-Servers fehlgeschlagen", "MCP server の再起動に失敗しました", "MCP server 다시 시작 실패", "Не удалось перезапустить MCP server")
	add(KeyLogMCPReconnectExhausted, "MCP reconnect attempts exhausted; giving up", "MCP 重连次数已用尽；停止尝试", "MCP-Verbindungsversuche ausgeschöpft; Abbruch", "MCP の再接続試行回数を使い切ったため中止します", "MCP 재연결 시도 횟수를 모두 사용하여 중단함", "Попытки переподключения MCP исчерпаны; прекращение")
	add(KeyLogMCPStreamReconnect, "MCP SSE stream failed; reconnecting", "MCP SSE stream 失败；正在重新连接", "MCP-SSE-Stream fehlgeschlagen; erneute Verbindung", "MCP SSE stream に失敗したため再接続します", "MCP SSE stream 실패로 다시 연결함", "Ошибка MCP SSE stream; выполняется переподключение")
	add(KeyLogMCPUnparseableEvent, "Ignoring an MCP SSE event that could not be parsed", "已忽略无法解析的 MCP SSE 事件", "Nicht parsebares MCP-SSE-Ereignis wird ignoriert", "解析できない MCP SSE イベントを無視します", "파싱할 수 없는 MCP SSE 이벤트 무시", "Неразбираемое событие MCP SSE проигнорировано")
	add(KeyLogHookUnknownEvent, "Unknown hook event type in configuration; skipping it", "配置中存在未知 hook 事件类型；已跳过", "Unbekannter Hook-Ereignistyp in der Konfiguration; wird übersprungen", "設定に不明な hook イベントタイプがあるためスキップします", "구성에 알 수 없는 hook 이벤트 유형이 있어 건너뜀", "Неизвестный тип события hook в конфигурации; пропуск")
	add(KeyLogTmuxBorderStatusFailed, "Could not set tmux pane border status; continuing", "无法设置 tmux pane 边框状态；将继续", "Status des tmux-Pane-Rahmens konnte nicht gesetzt werden; Fortsetzung", "tmux pane の境界ステータスを設定できませんでしたが続行します", "tmux pane 테두리 상태를 설정할 수 없어 계속함", "Не удалось задать состояние границы pane tmux; продолжение")
	add(KeyLogTmuxBorderFormatFailed, "Could not set tmux pane border format; continuing", "无法设置 tmux pane 边框格式；将继续", "Format des tmux-Pane-Rahmens konnte nicht gesetzt werden; Fortsetzung", "tmux pane の境界形式を設定できませんでしたが続行します", "tmux pane 테두리 형식을 설정할 수 없어 계속함", "Не удалось задать формат границы pane tmux; продолжение")
	add(KeyLogSDKSessionStatFailed, "Could not inspect an SDK session entry", "无法检查 SDK session 条目", "SDK-Sitzungseintrag konnte nicht geprüft werden", "SDK session エントリを確認できませんでした", "SDK session 항목을 확인할 수 없음", "Не удалось проверить запись session SDK")
	add(KeyLogSDKSessionPartialRead, "SDK session messages were only partially read", "仅部分读取了 SDK session 消息", "SDK-Sitzungsnachrichten wurden nur teilweise gelesen", "SDK session メッセージの一部のみ読み込みました", "SDK session 메시지를 일부만 읽음", "Сообщения session SDK прочитаны не полностью")
	add(KeyLogSDKSessionDeleted, "SDK session deleted", "SDK session 已删除", "SDK-Sitzung gelöscht", "SDK session を削除しました", "SDK session 삭제됨", "Session SDK удалён")
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
