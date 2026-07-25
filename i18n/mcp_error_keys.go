package i18n

// Services-layer MCP errors are localized at Error() time because connection
// failures can outlive the language that was active when the failure occurred.
// Protocol identifiers, server names, URLs, headers, status text, and remote
// response bodies are always supplied as formatting arguments.
const (
	KeyServicesMCPToolTimedOut                  Key = "services.mcp.tool.timed_out"
	KeyServicesMCPToolTimedOutNamed             Key = "services.mcp.tool.timed_out_named"
	KeyServicesMCPToolTimedOutOnServer          Key = "services.mcp.tool.timed_out_on_server"
	KeyServicesMCPToolReturnedError             Key = "services.mcp.tool.returned_error"
	KeyServicesMCPToolReturnedErrorMessage      Key = "services.mcp.tool.returned_error_message"
	KeyServicesMCPToolReturnedMessage           Key = "services.mcp.tool.returned_message"
	KeyServicesMCPToolReturnedServerMessage     Key = "services.mcp.tool.returned_server_message"
	KeyServicesMCPUnauthorized                  Key = "services.mcp.remote.unauthorized"
	KeyServicesMCPUnauthorizedASURI             Key = "services.mcp.remote.unauthorized_as_uri"
	KeyServicesMCPRemoteHTTPError               Key = "services.mcp.remote.http_error"
	KeyServicesMCPRemoteStatus                  Key = "services.mcp.remote.status"
	KeyServicesMCPRemoteStatusDetail            Key = "services.mcp.remote.status_detail"
	KeyServicesMCPSessionExpired                Key = "services.mcp.session.expired"
	KeyServicesMCPServerSessionExpired          Key = "services.mcp.session.server_expired"
	KeyServicesMCPJSONRPCError                  Key = "services.mcp.jsonrpc.error"
	KeyServicesMCPJSONRPCErrorCode              Key = "services.mcp.jsonrpc.error_code"
	KeyServicesMCPJSONRPCErrorDetail            Key = "services.mcp.jsonrpc.error_detail"
	KeyServicesMCPJSONRPCRequestMethodMissing   Key = "services.mcp.jsonrpc.request_method_missing"
	KeyServicesMCPJSONRPCNotifyMethodMissing    Key = "services.mcp.jsonrpc.notify_method_missing"
	KeyServicesMCPJSONRPCResultIDMissing        Key = "services.mcp.jsonrpc.result_id_missing"
	KeyServicesMCPJSONRPCErrorIDMissing         Key = "services.mcp.jsonrpc.error_id_missing"
	KeyServicesMCPJSONRPCEncodeRequestParams    Key = "services.mcp.jsonrpc.encode_request_params"
	KeyServicesMCPJSONRPCEncodeNotifyParams     Key = "services.mcp.jsonrpc.encode_notify_params"
	KeyServicesMCPJSONRPCEncodeResult           Key = "services.mcp.jsonrpc.encode_result"
	KeyServicesMCPJSONRPCEncodeErrorData        Key = "services.mcp.jsonrpc.encode_error_data"
	KeyServicesMCPTransportClosed               Key = "services.mcp.transport.closed"
	KeyServicesMCPTransportClosedReason         Key = "services.mcp.transport.closed_reason"
	KeyServicesMCPTransportClosedReasonCause    Key = "services.mcp.transport.closed_reason_cause"
	KeyServicesMCPTransportNotInitializedReason Key = "services.mcp.transport.reason.not_initialized"
	KeyServicesMCPTransportClosedStateReason    Key = "services.mcp.transport.reason.closed"
	KeyServicesMCPTransportEOFReason            Key = "services.mcp.transport.reason.eof"
	KeyServicesMCPTransportStreamFailedReason   Key = "services.mcp.transport.reason.stream_failed"
	KeyServicesMCPTransportPeerClosedReason     Key = "services.mcp.transport.reason.peer_closed"
	KeyServicesMCPTransportReceiveFailedReason  Key = "services.mcp.transport.reason.receive_failed"
	KeyServicesMCPTransportClientClosedReason   Key = "services.mcp.transport.reason.client_closed"
	KeyServicesMCPTransportCloseFrameReason     Key = "services.mcp.transport.reason.close_frame"
	KeyServicesMCPTransportWriteClosedReason    Key = "services.mcp.transport.reason.write_closed"
	KeyServicesMCPStdioOperationFailedReason    Key = "services.mcp.transport.reason.stdio_operation_failed"
	KeyServicesMCPStdioProcessExitedReason      Key = "services.mcp.transport.reason.process_exited"
	KeyServicesMCPStdioProcessExitDetailReason  Key = "services.mcp.transport.reason.process_exit_detail"
	KeyServicesMCPStdioStderrReason             Key = "services.mcp.transport.reason.stderr"
	KeyServicesMCPStdioPipeFailed               Key = "services.mcp.transport.stdio_pipe_failed"
	KeyServicesMCPStdioStartFailed              Key = "services.mcp.transport.stdio_start_failed"
	KeyServicesMCPResolveAuthToken              Key = "services.mcp.headers.resolve_auth_token"
	KeyServicesMCPNilManager                    Key = "services.mcp.manager.nil"
	KeyServicesMCPReadSettings                  Key = "services.mcp.manager.read_settings"
	KeyServicesMCPParseSettings                 Key = "services.mcp.manager.parse_settings"
	KeyServicesMCPServerNotConfigured           Key = "services.mcp.manager.server_not_configured"
	KeyServicesMCPNilClient                     Key = "services.mcp.client.nil"
	KeyServicesMCPNilTransport                  Key = "services.mcp.client.nil_transport"
	KeyServicesMCPInitializeNeedsTransport      Key = "services.mcp.client.initialize_needs_transport"
	KeyServicesMCPInitializeFailed              Key = "services.mcp.client.initialize_failed"
	KeyServicesMCPDecodeInitialize              Key = "services.mcp.client.decode_initialize"
	KeyServicesMCPInitializedNotification       Key = "services.mcp.client.initialized_notification"
	KeyServicesMCPClientNoTransport             Key = "services.mcp.client.no_transport"
	KeyServicesMCPNotifyNeedsTransport          Key = "services.mcp.client.notify_needs_transport"
	KeyServicesMCPMethodFailed                  Key = "services.mcp.client.method_failed"
	KeyServicesMCPNamedMethodFailed             Key = "services.mcp.client.named_method_failed"
	KeyServicesMCPLineOperationFailed           Key = "services.mcp.transport.line_operation_failed"
	KeyServicesMCPHTTPOperationFailed           Key = "services.mcp.transport.http_operation_failed"
	KeyServicesMCPSSENotInitialized             Key = "services.mcp.transport.sse_not_initialized"
	KeyServicesMCPSSEOperationFailed            Key = "services.mcp.transport.sse_operation_failed"
	KeyServicesMCPSSEContentType                Key = "services.mcp.transport.sse_content_type"
	KeyServicesMCPSSEEndpointMissingURL         Key = "services.mcp.transport.sse_endpoint_missing_url"
	KeyServicesMCPWebSocketOperationFailed      Key = "services.mcp.transport.websocket_operation_failed"
	KeyServicesMCPWebSocketHeaderInvalid        Key = "services.mcp.transport.websocket_header_invalid"
	KeyServicesMCPWebSocketMissingUpgrade       Key = "services.mcp.transport.websocket_missing_upgrade"
	KeyServicesMCPWebSocketMissingConnection    Key = "services.mcp.transport.websocket_missing_connection"
	KeyServicesMCPWebSocketAcceptMismatch       Key = "services.mcp.transport.websocket_accept_mismatch"
	KeyServicesMCPWebSocketSubprotocol          Key = "services.mcp.transport.websocket_subprotocol"
	KeyServicesMCPWebSocketContinuation         Key = "services.mcp.transport.websocket_continuation"
	KeyServicesMCPWebSocketOpcode               Key = "services.mcp.transport.websocket_opcode"
	KeyServicesMCPWebSocketMessageTooLarge      Key = "services.mcp.transport.websocket_message_too_large"
	KeyServicesMCPWebSocketFrameTooLarge        Key = "services.mcp.transport.websocket_frame_too_large"
	KeyServicesMCPPKCERandom                    Key = "services.mcp.oauth.pkce_random"
	KeyServicesMCPPKCEParseASURI                Key = "services.mcp.oauth.pkce_parse_as_uri"
	KeyServicesMCPOAuthErrorGeneric             Key = "services.mcp.oauth.remote_error_generic"
	KeyServicesMCPOAuthError                    Key = "services.mcp.oauth.remote_error"
	KeyServicesMCPOAuthErrorDetail              Key = "services.mcp.oauth.remote_error_detail"
	KeyServicesMCPOAuthRefreshClientIDMissing   Key = "services.mcp.oauth.refresh_client_id_missing"
	KeyServicesMCPOAuthMetadataCandidatesEmpty  Key = "services.mcp.oauth.metadata_candidates_empty"
	KeyServicesMCPProtectedMetadataServersEmpty Key = "services.mcp.oauth.protected_metadata_servers_empty"
	KeyServicesMCPOAuthMetadataEndpointsMissing Key = "services.mcp.oauth.metadata_endpoints_missing"
	KeyServicesMCPOAuthGETRejected              Key = "services.mcp.oauth.get_rejected"
	KeyServicesMCPOAuthDecodeJSON               Key = "services.mcp.oauth.decode_json"
	KeyServicesMCPOAuthTimeout                  Key = "services.mcp.oauth.timeout"
	KeyServicesMCPOAuthStateMismatch            Key = "services.mcp.oauth.state_mismatch"
	KeyServicesMCPOAuthCallbackCodeMissing      Key = "services.mcp.oauth.callback_code_missing"
	KeyServicesMCPOAuthRegistrationUnavailable  Key = "services.mcp.oauth.registration_unavailable"
	KeyServicesMCPOAuthRegistrationClientID     Key = "services.mcp.oauth.registration_client_id_missing"
	KeyServicesMCPOAuthPOSTRejected             Key = "services.mcp.oauth.post_rejected"
	KeyServicesMCPOAuthTokenEndpointRejected    Key = "services.mcp.oauth.token_endpoint_rejected"
	KeyServicesMCPOAuthAccessTokenMissing       Key = "services.mcp.oauth.access_token_missing"
	KeyServicesMCPOAuthTokenStorePathMissing    Key = "services.mcp.oauth.store_path_missing"
	KeyServicesMCPOAuthTokenStoreOperation      Key = "services.mcp.oauth.store_operation"
	KeyServicesMCPOAuthTokenStoreCreateDir      Key = "services.mcp.oauth.store_create_dir"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyServicesMCPToolTimedOut, "MCP: MCP tool timed out", "MCP：MCP tool 超时", "MCP: Zeitüberschreitung beim MCP-Tool", "MCP：MCP tool がタイムアウトしました", "MCP: MCP tool 시간이 초과되었습니다", "MCP: превышено время ожидания MCP tool")
	add(KeyServicesMCPToolTimedOutNamed, "MCP: MCP tool %q timed out after %ds", "MCP：MCP tool %q 在 %d 秒后超时", "MCP: Zeitüberschreitung beim MCP-Tool %q nach %d s", "MCP：MCP tool %q が %d 秒後にタイムアウトしました", "MCP: MCP tool %q의 시간이 %d초 후 초과되었습니다", "MCP: время ожидания MCP tool %q истекло через %d с")
	add(KeyServicesMCPToolTimedOutOnServer, "MCP: server %q tool %q timed out after %ds", "MCP：server %q 的 tool %q 在 %d 秒后超时", "MCP: Zeitüberschreitung bei Tool %q auf Server %q nach %d s", "MCP：server %q の tool %q が %d 秒後にタイムアウトしました", "MCP: server %q의 tool %q 시간이 %d초 후 초과되었습니다", "MCP: время ожидания tool %q на server %q истекло через %d с")
	add(KeyServicesMCPToolReturnedError, "MCP: MCP tool returned error", "MCP：MCP tool 返回了错误", "MCP: MCP-Tool hat einen Fehler zurückgegeben", "MCP：MCP tool がエラーを返しました", "MCP: MCP tool이 오류를 반환했습니다", "MCP: MCP tool вернул ошибку")
	add(KeyServicesMCPToolReturnedErrorMessage, "MCP tool returned error", "MCP tool 返回了错误", "MCP-Tool hat einen Fehler zurückgegeben", "MCP tool がエラーを返しました", "MCP tool이 오류를 반환했습니다", "MCP tool вернул ошибку")
	add(KeyServicesMCPToolReturnedMessage, "MCP: %s", "MCP：%s", "MCP: %s", "MCP：%s", "MCP: %s", "MCP: %s")
	add(KeyServicesMCPToolReturnedServerMessage, "MCP: server %q tool %q: %s", "MCP：server %q 的 tool %q：%s", "MCP: Server %q, Tool %q: %s", "MCP：server %q の tool %q：%s", "MCP: server %q의 tool %q: %s", "MCP: server %q, tool %q: %s")
	add(KeyServicesMCPUnauthorized, "MCP: unauthorized", "MCP：未获授权", "MCP: nicht autorisiert", "MCP：認証されていません", "MCP: 인증되지 않음", "MCP: нет авторизации")
	add(KeyServicesMCPUnauthorizedASURI, "MCP: unauthorized (as_uri=%s)", "MCP：未获授权（as_uri=%s）", "MCP: nicht autorisiert (as_uri=%s)", "MCP：認証されていません（as_uri=%s）", "MCP: 인증되지 않음(as_uri=%s)", "MCP: нет авторизации (as_uri=%s)")
	add(KeyServicesMCPRemoteHTTPError, "MCP: remote HTTP error", "MCP：远端 HTTP 错误", "MCP: Remote-HTTP-Fehler", "MCP：リモート HTTP エラー", "MCP: 원격 HTTP 오류", "MCP: удалённая ошибка HTTP")
	add(KeyServicesMCPRemoteStatus, "MCP: remote %s", "MCP：远端返回 %s", "MCP: Remote-Antwort %s", "MCP：リモート応答 %s", "MCP: 원격 응답 %s", "MCP: удалённый ответ %s")
	add(KeyServicesMCPRemoteStatusDetail, "MCP: remote %s: %s", "MCP：远端返回 %s：%s", "MCP: Remote-Antwort %s: %s", "MCP：リモート応答 %s：%s", "MCP: 원격 응답 %s: %s", "MCP: удалённый ответ %s: %s")
	add(KeyServicesMCPSessionExpired, "MCP: session expired", "MCP：session 已过期", "MCP: Session abgelaufen", "MCP：session の有効期限が切れました", "MCP: session이 만료되었습니다", "MCP: session истекла")
	add(KeyServicesMCPServerSessionExpired, "MCP: server %q session expired", "MCP：server %q 的 session 已过期", "MCP: Session von Server %q abgelaufen", "MCP：server %q の session の有効期限が切れました", "MCP: server %q의 session이 만료되었습니다", "MCP: session server %q истекла")
	add(KeyServicesMCPJSONRPCError, "MCP: JSON-RPC error", "MCP：JSON-RPC 错误", "MCP: JSON-RPC-Fehler", "MCP：JSON-RPC エラー", "MCP: JSON-RPC 오류", "MCP: ошибка JSON-RPC")
	add(KeyServicesMCPJSONRPCErrorCode, "MCP: JSON-RPC error %d", "MCP：JSON-RPC 错误 %d", "MCP: JSON-RPC-Fehler %d", "MCP：JSON-RPC エラー %d", "MCP: JSON-RPC 오류 %d", "MCP: ошибка JSON-RPC %d")
	add(KeyServicesMCPJSONRPCErrorDetail, "MCP: JSON-RPC error %d: %s", "MCP：JSON-RPC 错误 %d：%s", "MCP: JSON-RPC-Fehler %d: %s", "MCP：JSON-RPC エラー %d：%s", "MCP: JSON-RPC 오류 %d: %s", "MCP: ошибка JSON-RPC %d: %s")
	add(KeyServicesMCPJSONRPCRequestMethodMissing, "MCP: JSON-RPC request is missing a method", "MCP：JSON-RPC 请求缺少 method", "MCP: In der JSON-RPC-Anfrage fehlt eine Methode", "MCP：JSON-RPC request に method がありません", "MCP: JSON-RPC 요청에 method가 없습니다", "MCP: в запросе JSON-RPC отсутствует method")
	add(KeyServicesMCPJSONRPCNotifyMethodMissing, "MCP: JSON-RPC notification is missing a method", "MCP：JSON-RPC 通知缺少 method", "MCP: In der JSON-RPC-Benachrichtigung fehlt eine Methode", "MCP：JSON-RPC notification に method がありません", "MCP: JSON-RPC 알림에 method가 없습니다", "MCP: в уведомлении JSON-RPC отсутствует method")
	add(KeyServicesMCPJSONRPCResultIDMissing, "MCP: JSON-RPC result is missing an ID", "MCP：JSON-RPC result 缺少 ID", "MCP: Im JSON-RPC-Ergebnis fehlt eine ID", "MCP：JSON-RPC result に ID がありません", "MCP: JSON-RPC 결과에 ID가 없습니다", "MCP: в результате JSON-RPC отсутствует ID")
	add(KeyServicesMCPJSONRPCErrorIDMissing, "MCP: JSON-RPC error response is missing an ID", "MCP：JSON-RPC 错误响应缺少 ID", "MCP: In der JSON-RPC-Fehlerantwort fehlt eine ID", "MCP：JSON-RPC error response に ID がありません", "MCP: JSON-RPC 오류 응답에 ID가 없습니다", "MCP: в ответе с ошибкой JSON-RPC отсутствует ID")
	add(KeyServicesMCPJSONRPCEncodeRequestParams, "MCP: could not encode JSON-RPC request parameters", "MCP：无法编码 JSON-RPC 请求参数", "MCP: JSON-RPC-Anfrageparameter konnten nicht codiert werden", "MCP：JSON-RPC request parameters を encode できませんでした", "MCP: JSON-RPC 요청 매개변수를 인코딩하지 못했습니다", "MCP: не удалось закодировать параметры запроса JSON-RPC")
	add(KeyServicesMCPJSONRPCEncodeNotifyParams, "MCP: could not encode JSON-RPC notification parameters", "MCP：无法编码 JSON-RPC 通知参数", "MCP: JSON-RPC-Benachrichtigungsparameter konnten nicht codiert werden", "MCP：JSON-RPC notification parameters を encode できませんでした", "MCP: JSON-RPC 알림 매개변수를 인코딩하지 못했습니다", "MCP: не удалось закодировать параметры уведомления JSON-RPC")
	add(KeyServicesMCPJSONRPCEncodeResult, "MCP: could not encode the JSON-RPC result", "MCP：无法编码 JSON-RPC result", "MCP: Das JSON-RPC-Ergebnis konnte nicht codiert werden", "MCP：JSON-RPC result を encode できませんでした", "MCP: JSON-RPC 결과를 인코딩하지 못했습니다", "MCP: не удалось закодировать результат JSON-RPC")
	add(KeyServicesMCPJSONRPCEncodeErrorData, "MCP: could not encode JSON-RPC error data", "MCP：无法编码 JSON-RPC 错误数据", "MCP: JSON-RPC-Fehlerdaten konnten nicht codiert werden", "MCP：JSON-RPC error data を encode できませんでした", "MCP: JSON-RPC 오류 데이터를 인코딩하지 못했습니다", "MCP: не удалось закодировать данные ошибки JSON-RPC")
	add(KeyServicesMCPTransportClosed, "MCP: transport closed", "MCP：transport 已关闭", "MCP: Transport geschlossen", "MCP：transport は終了しました", "MCP: transport가 종료되었습니다", "MCP: transport закрыт")
	add(KeyServicesMCPTransportClosedReason, "MCP: transport closed: %s", "MCP：transport 已关闭：%s", "MCP: Transport geschlossen: %s", "MCP：transport は終了しました：%s", "MCP: transport가 종료되었습니다: %s", "MCP: transport закрыт: %s")
	add(KeyServicesMCPTransportClosedReasonCause, "MCP: transport closed: %s: %v", "MCP：transport 已关闭：%s：%v", "MCP: Transport geschlossen: %s: %v", "MCP：transport は終了しました：%s：%v", "MCP: transport가 종료되었습니다: %s: %v", "MCP: transport закрыт: %s: %v")
	add(KeyServicesMCPTransportNotInitializedReason, "%s transport not initialized", "%s transport 未初始化", "%s-Transport nicht initialisiert", "%s transport が初期化されていません", "%s transport가 초기화되지 않았습니다", "transport %s не инициализирован")
	add(KeyServicesMCPTransportClosedStateReason, "%s transport closed", "%s transport 已关闭", "%s-Transport geschlossen", "%s transport は終了しました", "%s transport가 종료되었습니다", "transport %s закрыт")
	add(KeyServicesMCPTransportEOFReason, "%s EOF", "%s EOF", "%s EOF", "%s EOF", "%s EOF", "%s EOF")
	add(KeyServicesMCPTransportStreamFailedReason, "%s stream failed", "%s stream 失败", "%s-Stream fehlgeschlagen", "%s stream に失敗しました", "%s stream에 실패했습니다", "сбой stream %s")
	add(KeyServicesMCPTransportPeerClosedReason, "%s peer closed", "%s peer 已关闭", "%s-Peer geschlossen", "%s peer は終了しました", "%s peer가 종료되었습니다", "peer %s закрыт")
	add(KeyServicesMCPTransportReceiveFailedReason, "transport receive failed", "transport receive 失败", "Transport-receive fehlgeschlagen", "transport receive に失敗しました", "transport receive에 실패했습니다", "сбой transport receive")
	add(KeyServicesMCPTransportClientClosedReason, "client closed", "client 已关闭", "Client geschlossen", "client は終了しました", "client가 종료되었습니다", "client закрыт")
	add(KeyServicesMCPTransportCloseFrameReason, "WebSocket close frame", "WebSocket close frame", "WebSocket-Close-Frame", "WebSocket close frame", "WebSocket close frame", "WebSocket close frame")
	add(KeyServicesMCPTransportWriteClosedReason, "WebSocket write closed", "WebSocket write 已关闭", "WebSocket-write geschlossen", "WebSocket write は終了しました", "WebSocket write가 종료되었습니다", "WebSocket write закрыт")
	add(KeyServicesMCPStdioOperationFailedReason, "stdio %s failed", "stdio %s 失败", "stdio-%s fehlgeschlagen", "stdio %s に失敗しました", "stdio %s에 실패했습니다", "сбой stdio %s")
	add(KeyServicesMCPStdioProcessExitedReason, "process exited", "process 已退出", "Prozess beendet", "process が終了しました", "process가 종료되었습니다", "process завершён")
	add(KeyServicesMCPStdioProcessExitDetailReason, "process exited: %v", "process 已退出：%v", "Prozess beendet: %v", "process が終了しました：%v", "process가 종료되었습니다: %v", "process завершён: %v")
	add(KeyServicesMCPStdioStderrReason, "stderr: %s", "stderr：%s", "stderr: %s", "stderr：%s", "stderr: %s", "stderr: %s")
	add(KeyServicesMCPStdioPipeFailed, "Could not open the MCP stdio %s pipe: %v", "无法打开 MCP stdio %s pipe：%v", "Die MCP-stdio-%s-Pipe konnte nicht geöffnet werden: %v", "MCP stdio の %s pipe を開けませんでした：%v", "MCP stdio %s pipe를 열 수 없습니다: %v", "Не удалось открыть pipe %s транспорта MCP stdio: %v")
	add(KeyServicesMCPStdioStartFailed, "Could not start MCP stdio server %q: %v", "无法启动 MCP stdio server %q：%v", "MCP-stdio-Server %q konnte nicht gestartet werden: %v", "MCP stdio server %q を起動できませんでした：%v", "MCP stdio server %q을(를) 시작할 수 없습니다: %v", "Не удалось запустить MCP stdio server %q: %v")
	add(KeyServicesMCPResolveAuthToken, "MCP: resolve auth token: %v", "MCP：解析 auth token 失败：%v", "MCP: Auth-Token konnte nicht aufgelöst werden: %v", "MCP：auth token を解決できませんでした：%v", "MCP: auth token을 확인하지 못했습니다: %v", "MCP: не удалось получить auth token: %v")
	add(KeyServicesMCPNilManager, "MCP: nil manager", "MCP：manager 为 nil", "MCP: manager ist nil", "MCP：manager が nil です", "MCP: manager가 nil입니다", "MCP: manager равен nil")
	add(KeyServicesMCPReadSettings, "MCP: read MCP settings %s: %v", "MCP：读取 MCP settings %s 失败：%v", "MCP: MCP-Einstellungen %s konnten nicht gelesen werden: %v", "MCP：MCP settings %s を読み取れませんでした：%v", "MCP: MCP settings %s을(를) 읽지 못했습니다: %v", "MCP: не удалось прочитать MCP settings %s: %v")
	add(KeyServicesMCPParseSettings, "MCP: parse MCP settings %s: %v", "MCP：解析 MCP settings %s 失败：%v", "MCP: MCP-Einstellungen %s konnten nicht geparst werden: %v", "MCP：MCP settings %s を parse できませんでした：%v", "MCP: MCP settings %s을(를) parse하지 못했습니다: %v", "MCP: не удалось разобрать MCP settings %s: %v")
	add(KeyServicesMCPServerNotConfigured, "MCP: MCP server %q not configured", "MCP：尚未配置 MCP server %q", "MCP: MCP-Server %q ist nicht konfiguriert", "MCP：MCP server %q は設定されていません", "MCP: MCP server %q이(가) 구성되지 않았습니다", "MCP: MCP server %q не настроен")
	add(KeyServicesMCPNilClient, "MCP: nil client", "MCP：client 为 nil", "MCP: client ist nil", "MCP：client が nil です", "MCP: client가 nil입니다", "MCP: client равен nil")
	add(KeyServicesMCPNilTransport, "MCP: nil transport", "MCP：transport 为 nil", "MCP: transport ist nil", "MCP：transport が nil です", "MCP: transport가 nil입니다", "MCP: transport равен nil")
	add(KeyServicesMCPInitializeNeedsTransport, "MCP: initialize requires a transport-backed client", "MCP：initialize 需要由 transport 支持的 client", "MCP: initialize erfordert einen transportgestützten Client", "MCP：initialize には transport 対応 client が必要です", "MCP: initialize에는 transport 기반 client가 필요합니다", "MCP: для initialize требуется client с transport")
	add(KeyServicesMCPInitializeFailed, "MCP: initialize: %v", "MCP：initialize 失败：%v", "MCP: initialize fehlgeschlagen: %v", "MCP：initialize に失敗しました：%v", "MCP: initialize에 실패했습니다: %v", "MCP: сбой initialize: %v")
	add(KeyServicesMCPDecodeInitialize, "MCP: decode initialize result: %v", "MCP：decode initialize result 失败：%v", "MCP: initialize-result konnte nicht decodiert werden: %v", "MCP：initialize result を decode できませんでした：%v", "MCP: initialize result를 decode하지 못했습니다: %v", "MCP: не удалось декодировать initialize result: %v")
	add(KeyServicesMCPInitializedNotification, "MCP: initialized notification: %v", "MCP：发送 initialized notification 失败：%v", "MCP: initialized-notification fehlgeschlagen: %v", "MCP：initialized notification に失敗しました：%v", "MCP: initialized notification에 실패했습니다: %v", "MCP: сбой initialized notification: %v")
	add(KeyServicesMCPClientNoTransport, "MCP: client has no transport", "MCP：client 没有 transport", "MCP: client hat keinen transport", "MCP：client に transport がありません", "MCP: client에 transport가 없습니다", "MCP: у client нет transport")
	add(KeyServicesMCPNotifyNeedsTransport, "MCP: notify requires a transport-backed client", "MCP：notify 需要由 transport 支持的 client", "MCP: notify erfordert einen transportgestützten Client", "MCP：notify には transport 対応 client が必要です", "MCP: notify에는 transport 기반 client가 필요합니다", "MCP: для notify требуется client с transport")
	add(KeyServicesMCPMethodFailed, "MCP: %s: %v", "MCP：%s 失败：%v", "MCP: %s fehlgeschlagen: %v", "MCP：%s に失敗しました：%v", "MCP: %s에 실패했습니다: %v", "MCP: сбой %s: %v")
	add(KeyServicesMCPNamedMethodFailed, "MCP: %s %q: %v", "MCP：%s %q 失败：%v", "MCP: %s %q fehlgeschlagen: %v", "MCP：%s %q に失敗しました：%v", "MCP: %s %q에 실패했습니다: %v", "MCP: сбой %s %q: %v")
	add(KeyServicesMCPLineOperationFailed, "MCP: line transport %s: %v", "MCP：line transport %s 失败：%v", "MCP: line-transport %s fehlgeschlagen: %v", "MCP：line transport %s に失敗しました：%v", "MCP: line transport %s에 실패했습니다: %v", "MCP: сбой line transport %s: %v")
	add(KeyServicesMCPHTTPOperationFailed, "MCP: HTTP transport %s: %v", "MCP：HTTP transport %s 失败：%v", "MCP: HTTP-Transport %s fehlgeschlagen: %v", "MCP：HTTP transport %s に失敗しました：%v", "MCP: HTTP transport %s에 실패했습니다: %v", "MCP: сбой HTTP transport %s: %v")
	add(KeyServicesMCPSSENotInitialized, "MCP: SSE transport not initialized", "MCP：SSE transport 未初始化", "MCP: SSE-Transport nicht initialisiert", "MCP：SSE transport が初期化されていません", "MCP: SSE transport가 초기화되지 않았습니다", "MCP: SSE transport не инициализирован")
	add(KeyServicesMCPSSEOperationFailed, "MCP: SSE %s: %v", "MCP：SSE %s 失败：%v", "MCP: SSE %s fehlgeschlagen: %v", "MCP：SSE %s に失敗しました：%v", "MCP: SSE %s에 실패했습니다: %v", "MCP: сбой SSE %s: %v")
	add(KeyServicesMCPSSEContentType, "MCP: SSE GET returned content-type %q", "MCP：SSE GET 返回了 content-type %q", "MCP: SSE GET hat content-type %q zurückgegeben", "MCP：SSE GET が content-type %q を返しました", "MCP: SSE GET이 content-type %q을(를) 반환했습니다", "MCP: SSE GET вернул content-type %q")
	add(KeyServicesMCPSSEEndpointMissingURL, "MCP: SSE endpoint event missing URL", "MCP：SSE endpoint event 缺少 URL", "MCP: Im SSE-endpoint-event fehlt die URL", "MCP：SSE endpoint event に URL がありません", "MCP: SSE endpoint event에 URL이 없습니다", "MCP: в SSE endpoint event отсутствует URL")
	add(KeyServicesMCPWebSocketOperationFailed, "MCP: WebSocket %s: %v", "MCP：WebSocket %s 失败：%v", "MCP: WebSocket %s fehlgeschlagen: %v", "MCP：WebSocket %s に失敗しました：%v", "MCP: WebSocket %s에 실패했습니다: %v", "MCP: сбой WebSocket %s: %v")
	add(KeyServicesMCPWebSocketHeaderInvalid, "MCP: WebSocket handshake header is invalid", "MCP：WebSocket 握手标头无效", "MCP: WebSocket-Handshake-Header ist ungültig", "MCP：WebSocket ハンドシェイクヘッダーが無効です", "MCP: WebSocket 핸드셰이크 헤더가 잘못되었습니다", "MCP: недопустимый заголовок рукопожатия WebSocket")
	add(KeyServicesMCPWebSocketMissingUpgrade, "MCP: WebSocket handshake missing Upgrade", "MCP：WebSocket handshake 缺少 Upgrade", "MCP: Beim WebSocket-handshake fehlt Upgrade", "MCP：WebSocket handshake に Upgrade がありません", "MCP: WebSocket handshake에 Upgrade가 없습니다", "MCP: в WebSocket handshake отсутствует Upgrade")
	add(KeyServicesMCPWebSocketMissingConnection, "MCP: WebSocket handshake missing Connection upgrade", "MCP：WebSocket handshake 缺少 Connection upgrade", "MCP: Beim WebSocket-handshake fehlt Connection upgrade", "MCP：WebSocket handshake に Connection upgrade がありません", "MCP: WebSocket handshake에 Connection upgrade가 없습니다", "MCP: в WebSocket handshake отсутствует Connection upgrade")
	add(KeyServicesMCPWebSocketAcceptMismatch, "MCP: WebSocket handshake accept mismatch", "MCP：WebSocket handshake accept 不匹配", "MCP: WebSocket-handshake-accept stimmt nicht überein", "MCP：WebSocket handshake accept が一致しません", "MCP: WebSocket handshake accept가 일치하지 않습니다", "MCP: значение accept в WebSocket handshake не совпадает")
	add(KeyServicesMCPWebSocketSubprotocol, "MCP: WebSocket subprotocol %q, want mcp", "MCP：WebSocket subprotocol 为 %q，需要 mcp", "MCP: WebSocket-subprotocol %q, erwartet wird mcp", "MCP：WebSocket subprotocol は %q ですが、mcp が必要です", "MCP: WebSocket subprotocol이 %q입니다. mcp가 필요합니다", "MCP: WebSocket subprotocol %q, требуется mcp")
	add(KeyServicesMCPWebSocketContinuation, "MCP: unexpected WebSocket continuation frame", "MCP：出现意外的 WebSocket continuation frame", "MCP: Unerwarteter WebSocket-continuation-frame", "MCP：予期しない WebSocket continuation frame です", "MCP: 예기치 않은 WebSocket continuation frame입니다", "MCP: неожиданный WebSocket continuation frame")
	add(KeyServicesMCPWebSocketOpcode, "MCP: unsupported WebSocket opcode %d", "MCP：不支持 WebSocket opcode %d", "MCP: Nicht unterstützter WebSocket-opcode %d", "MCP：WebSocket opcode %d はサポートされていません", "MCP: WebSocket opcode %d은(는) 지원되지 않습니다", "MCP: WebSocket opcode %d не поддерживается")
	add(KeyServicesMCPWebSocketMessageTooLarge, "MCP: WebSocket message too large", "MCP：WebSocket message 过大", "MCP: WebSocket-message ist zu groß", "MCP：WebSocket message が大きすぎます", "MCP: WebSocket message가 너무 큽니다", "MCP: WebSocket message слишком велико")
	add(KeyServicesMCPWebSocketFrameTooLarge, "MCP: WebSocket frame too large", "MCP：WebSocket frame 过大", "MCP: WebSocket-frame ist zu groß", "MCP：WebSocket frame が大きすぎます", "MCP: WebSocket frame이 너무 큽니다", "MCP: WebSocket frame слишком велик")
	add(KeyServicesMCPPKCERandom, "pkce: read random: %v", "pkce：读取随机数失败：%v", "pkce: Zufallsdaten konnten nicht gelesen werden: %v", "pkce：乱数を読み取れませんでした：%v", "pkce: 난수를 읽지 못했습니다: %v", "pkce: не удалось прочитать случайные данные: %v")
	add(KeyServicesMCPPKCEParseASURI, "pkce: parse as_uri: %v", "pkce：解析 as_uri 失败：%v", "pkce: as_uri konnte nicht geparst werden: %v", "pkce：as_uri を parse できませんでした：%v", "pkce: as_uri를 parse하지 못했습니다: %v", "pkce: не удалось разобрать as_uri: %v")
	add(KeyServicesMCPOAuthErrorGeneric, "oauth error", "OAuth 错误", "OAuth-Fehler", "OAuth エラー", "OAuth 오류", "ошибка OAuth")
	add(KeyServicesMCPOAuthError, "oauth error %s", "OAuth 错误 %s", "OAuth-Fehler %s", "OAuth エラー %s", "OAuth 오류 %s", "ошибка OAuth %s")
	add(KeyServicesMCPOAuthErrorDetail, "oauth error %s: %s", "OAuth 错误 %s：%s", "OAuth-Fehler %s: %s", "OAuth エラー %s：%s", "OAuth 오류 %s: %s", "ошибка OAuth %s: %s")
	add(KeyServicesMCPOAuthRefreshClientIDMissing, "MCP: no OAuth client_id for refresh", "MCP：OAuth refresh 缺少 client_id", "MCP: Für OAuth-refresh fehlt client_id", "MCP：OAuth refresh 用の client_id がありません", "MCP: OAuth refresh에 필요한 client_id가 없습니다", "MCP: для OAuth refresh отсутствует client_id")
	add(KeyServicesMCPOAuthMetadataCandidatesEmpty, "MCP: no authorization server metadata candidates", "MCP：没有可用的 authorization server metadata candidate", "MCP: Keine Kandidaten für authorization-server-metadata", "MCP：authorization server metadata の候補がありません", "MCP: authorization server metadata 후보가 없습니다", "MCP: нет кандидатов authorization server metadata")
	add(KeyServicesMCPProtectedMetadataServersEmpty, "MCP: protected resource metadata has no authorization_servers", "MCP：protected resource metadata 中没有 authorization_servers", "MCP: protected-resource-metadata enthält keine authorization_servers", "MCP：protected resource metadata に authorization_servers がありません", "MCP: protected resource metadata에 authorization_servers가 없습니다", "MCP: в protected resource metadata отсутствует authorization_servers")
	add(KeyServicesMCPOAuthMetadataEndpointsMissing, "MCP: OAuth metadata missing authorization_endpoint or token_endpoint", "MCP：OAuth metadata 缺少 authorization_endpoint 或 token_endpoint", "MCP: In den OAuth-metadata fehlt authorization_endpoint oder token_endpoint", "MCP：OAuth metadata に authorization_endpoint または token_endpoint がありません", "MCP: OAuth metadata에 authorization_endpoint 또는 token_endpoint가 없습니다", "MCP: в OAuth metadata отсутствует authorization_endpoint или token_endpoint")
	add(KeyServicesMCPOAuthGETRejected, "MCP: GET %s returned HTTP %d", "MCP：GET %s 返回 HTTP %d", "MCP: GET %s hat HTTP %d zurückgegeben", "MCP：GET %s が HTTP %d を返しました", "MCP: GET %s이(가) HTTP %d을(를) 반환했습니다", "MCP: GET %s вернул HTTP %d")
	add(KeyServicesMCPOAuthDecodeJSON, "MCP: decode OAuth JSON from %s: %v", "MCP：decode 来自 %s 的 OAuth JSON 失败：%v", "MCP: OAuth-JSON von %s konnte nicht decodiert werden: %v", "MCP：%s の OAuth JSON を decode できませんでした：%v", "MCP: %s의 OAuth JSON을 decode하지 못했습니다: %v", "MCP: не удалось декодировать OAuth JSON из %s: %v")
	add(KeyServicesMCPOAuthTimeout, "MCP: OAuth authentication timeout", "MCP：OAuth 认证超时", "MCP: Zeitüberschreitung bei der OAuth-Authentifizierung", "MCP：OAuth 認証がタイムアウトしました", "MCP: OAuth 인증 시간이 초과되었습니다", "MCP: превышено время ожидания OAuth-аутентификации")
	add(KeyServicesMCPOAuthStateMismatch, "MCP: OAuth state mismatch", "MCP：OAuth state 不匹配", "MCP: OAuth-state stimmt nicht überein", "MCP：OAuth state が一致しません", "MCP: OAuth state가 일치하지 않습니다", "MCP: OAuth state не совпадает")
	add(KeyServicesMCPOAuthCallbackCodeMissing, "MCP: OAuth callback missing code", "MCP：OAuth callback 缺少 code", "MCP: Im OAuth-callback fehlt code", "MCP：OAuth callback に code がありません", "MCP: OAuth callback에 code가 없습니다", "MCP: в OAuth callback отсутствует code")
	add(KeyServicesMCPOAuthRegistrationUnavailable, "MCP: OAuth metadata has no registration_endpoint and config has no clientId", "MCP：OAuth metadata 中没有 registration_endpoint，且 config 中没有 clientId", "MCP: OAuth-metadata enthält keinen registration_endpoint und die Konfiguration keine clientId", "MCP：OAuth metadata に registration_endpoint がなく、config にも clientId がありません", "MCP: OAuth metadata에 registration_endpoint가 없고 config에도 clientId가 없습니다", "MCP: в OAuth metadata нет registration_endpoint, а в config нет clientId")
	add(KeyServicesMCPOAuthRegistrationClientID, "MCP: OAuth client registration returned no client_id", "MCP：OAuth client registration 未返回 client_id", "MCP: OAuth-client-registration hat keine client_id zurückgegeben", "MCP：OAuth client registration が client_id を返しませんでした", "MCP: OAuth client registration이 client_id를 반환하지 않았습니다", "MCP: OAuth client registration не вернула client_id")
	add(KeyServicesMCPOAuthPOSTRejected, "MCP: POST %s returned HTTP %d", "MCP：POST %s 返回 HTTP %d", "MCP: POST %s hat HTTP %d zurückgegeben", "MCP：POST %s が HTTP %d を返しました", "MCP: POST %s이(가) HTTP %d을(를) 반환했습니다", "MCP: POST %s вернул HTTP %d")
	add(KeyServicesMCPOAuthTokenEndpointRejected, "MCP: token endpoint returned HTTP %d", "MCP：token endpoint 返回 HTTP %d", "MCP: token-endpoint hat HTTP %d zurückgegeben", "MCP：token endpoint が HTTP %d を返しました", "MCP: token endpoint가 HTTP %d을(를) 반환했습니다", "MCP: token endpoint вернул HTTP %d")
	add(KeyServicesMCPOAuthAccessTokenMissing, "MCP: token endpoint returned no access_token", "MCP：token endpoint 未返回 access_token", "MCP: token-endpoint hat kein access_token zurückgegeben", "MCP：token endpoint が access_token を返しませんでした", "MCP: token endpoint가 access_token을 반환하지 않았습니다", "MCP: token endpoint не вернул access_token")
	add(KeyServicesMCPOAuthTokenStorePathMissing, "MCP: file token store has no path", "MCP：file token store 没有 path", "MCP: file-token-store hat keinen path", "MCP：file token store に path がありません", "MCP: file token store에 path가 없습니다", "MCP: у file token store нет path")
	add(KeyServicesMCPOAuthTokenStoreOperation, "MCP: %s OAuth token store: %v", "MCP：%s OAuth token store 失败：%v", "MCP: %s des OAuth-token-store fehlgeschlagen: %v", "MCP：OAuth token store の %s に失敗しました：%v", "MCP: OAuth token store %s에 실패했습니다: %v", "MCP: сбой %s OAuth token store: %v")
	add(KeyServicesMCPOAuthTokenStoreCreateDir, "MCP: create OAuth token store dir: %v", "MCP：创建 OAuth token store 目录失败：%v", "MCP: Verzeichnis für den OAuth-token-store konnte nicht erstellt werden: %v", "MCP：OAuth token store のディレクトリを作成できませんでした：%v", "MCP: OAuth token store 디렉터리를 만들지 못했습니다: %v", "MCP: не удалось создать каталог OAuth token store: %v")
}
