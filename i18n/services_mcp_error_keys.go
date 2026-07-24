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
	KeyServicesMCPResolveDynamicHeaders         Key = "services.mcp.headers.resolve_dynamic_headers"
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
	KeyServicesMCPServerNotConnected            Key = "services.mcp.prompt.server_not_connected"
	KeyServicesMCPServerPromptsUnsupported      Key = "services.mcp.prompt.prompts_unsupported"
	KeyServicesMCPLineOperationFailed           Key = "services.mcp.transport.line_operation_failed"
	KeyServicesMCPHTTPOperationFailed           Key = "services.mcp.transport.http_operation_failed"
	KeyServicesMCPSSENotInitialized             Key = "services.mcp.transport.sse_not_initialized"
	KeyServicesMCPSSEOperationFailed            Key = "services.mcp.transport.sse_operation_failed"
	KeyServicesMCPSSEContentType                Key = "services.mcp.transport.sse_content_type"
	KeyServicesMCPSSEEndpointMissingURL         Key = "services.mcp.transport.sse_endpoint_missing_url"
	KeyServicesMCPWebSocketOperationFailed      Key = "services.mcp.transport.websocket_operation_failed"
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
	KeyServicesMCPClaudeAIProxyServerID         Key = "services.mcp.claude_ai_proxy.server_id_missing"
	KeyServicesMCPClaudeAIProxyURL              Key = "services.mcp.claude_ai_proxy.url_invalid"
	KeyServicesMCPNilSDKSender                  Key = "services.mcp.sdk.nil_sender"
	KeyServicesMCPSDKControlSend                Key = "services.mcp.sdk.control_send"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyServicesMCPToolTimedOut, "services/mcp: MCP tool timed out", "services/mcp：MCP tool 超时", "services/mcp: Zeitüberschreitung beim MCP-Tool", "services/mcp：MCP tool がタイムアウトしました", "services/mcp: MCP tool 시간이 초과되었습니다", "services/mcp: превышено время ожидания MCP tool")
	add(KeyServicesMCPToolTimedOutNamed, "services/mcp: MCP tool %q timed out after %ds", "services/mcp：MCP tool %q 在 %d 秒后超时", "services/mcp: Zeitüberschreitung beim MCP-Tool %q nach %d s", "services/mcp：MCP tool %q が %d 秒後にタイムアウトしました", "services/mcp: MCP tool %q의 시간이 %d초 후 초과되었습니다", "services/mcp: время ожидания MCP tool %q истекло через %d с")
	add(KeyServicesMCPToolTimedOutOnServer, "services/mcp: server %q tool %q timed out after %ds", "services/mcp：server %q 的 tool %q 在 %d 秒后超时", "services/mcp: Zeitüberschreitung bei Tool %q auf Server %q nach %d s", "services/mcp：server %q の tool %q が %d 秒後にタイムアウトしました", "services/mcp: server %q의 tool %q 시간이 %d초 후 초과되었습니다", "services/mcp: время ожидания tool %q на server %q истекло через %d с")
	add(KeyServicesMCPToolReturnedError, "services/mcp: MCP tool returned error", "services/mcp：MCP tool 返回了错误", "services/mcp: MCP-Tool hat einen Fehler zurückgegeben", "services/mcp：MCP tool がエラーを返しました", "services/mcp: MCP tool이 오류를 반환했습니다", "services/mcp: MCP tool вернул ошибку")
	add(KeyServicesMCPToolReturnedErrorMessage, "MCP tool returned error", "MCP tool 返回了错误", "MCP-Tool hat einen Fehler zurückgegeben", "MCP tool がエラーを返しました", "MCP tool이 오류를 반환했습니다", "MCP tool вернул ошибку")
	add(KeyServicesMCPToolReturnedMessage, "services/mcp: %s", "services/mcp：%s", "services/mcp: %s", "services/mcp：%s", "services/mcp: %s", "services/mcp: %s")
	add(KeyServicesMCPToolReturnedServerMessage, "services/mcp: server %q tool %q: %s", "services/mcp：server %q 的 tool %q：%s", "services/mcp: Server %q, Tool %q: %s", "services/mcp：server %q の tool %q：%s", "services/mcp: server %q의 tool %q: %s", "services/mcp: server %q, tool %q: %s")
	add(KeyServicesMCPUnauthorized, "services/mcp: unauthorized", "services/mcp：未获授权", "services/mcp: nicht autorisiert", "services/mcp：認証されていません", "services/mcp: 인증되지 않음", "services/mcp: нет авторизации")
	add(KeyServicesMCPUnauthorizedASURI, "services/mcp: unauthorized (as_uri=%s)", "services/mcp：未获授权（as_uri=%s）", "services/mcp: nicht autorisiert (as_uri=%s)", "services/mcp：認証されていません（as_uri=%s）", "services/mcp: 인증되지 않음(as_uri=%s)", "services/mcp: нет авторизации (as_uri=%s)")
	add(KeyServicesMCPRemoteHTTPError, "services/mcp: remote HTTP error", "services/mcp：远端 HTTP 错误", "services/mcp: Remote-HTTP-Fehler", "services/mcp：リモート HTTP エラー", "services/mcp: 원격 HTTP 오류", "services/mcp: удалённая ошибка HTTP")
	add(KeyServicesMCPRemoteStatus, "services/mcp: remote %s", "services/mcp：远端返回 %s", "services/mcp: Remote-Antwort %s", "services/mcp：リモート応答 %s", "services/mcp: 원격 응답 %s", "services/mcp: удалённый ответ %s")
	add(KeyServicesMCPRemoteStatusDetail, "services/mcp: remote %s: %s", "services/mcp：远端返回 %s：%s", "services/mcp: Remote-Antwort %s: %s", "services/mcp：リモート応答 %s：%s", "services/mcp: 원격 응답 %s: %s", "services/mcp: удалённый ответ %s: %s")
	add(KeyServicesMCPSessionExpired, "services/mcp: session expired", "services/mcp：session 已过期", "services/mcp: Session abgelaufen", "services/mcp：session の有効期限が切れました", "services/mcp: session이 만료되었습니다", "services/mcp: session истекла")
	add(KeyServicesMCPServerSessionExpired, "services/mcp: server %q session expired", "services/mcp：server %q 的 session 已过期", "services/mcp: Session von Server %q abgelaufen", "services/mcp：server %q の session の有効期限が切れました", "services/mcp: server %q의 session이 만료되었습니다", "services/mcp: session server %q истекла")
	add(KeyServicesMCPJSONRPCError, "services/mcp: JSON-RPC error", "services/mcp：JSON-RPC 错误", "services/mcp: JSON-RPC-Fehler", "services/mcp：JSON-RPC エラー", "services/mcp: JSON-RPC 오류", "services/mcp: ошибка JSON-RPC")
	add(KeyServicesMCPJSONRPCErrorCode, "services/mcp: JSON-RPC error %d", "services/mcp：JSON-RPC 错误 %d", "services/mcp: JSON-RPC-Fehler %d", "services/mcp：JSON-RPC エラー %d", "services/mcp: JSON-RPC 오류 %d", "services/mcp: ошибка JSON-RPC %d")
	add(KeyServicesMCPJSONRPCErrorDetail, "services/mcp: JSON-RPC error %d: %s", "services/mcp：JSON-RPC 错误 %d：%s", "services/mcp: JSON-RPC-Fehler %d: %s", "services/mcp：JSON-RPC エラー %d：%s", "services/mcp: JSON-RPC 오류 %d: %s", "services/mcp: ошибка JSON-RPC %d: %s")
	add(KeyServicesMCPTransportClosed, "services/mcp: transport closed", "services/mcp：transport 已关闭", "services/mcp: Transport geschlossen", "services/mcp：transport は終了しました", "services/mcp: transport가 종료되었습니다", "services/mcp: transport закрыт")
	add(KeyServicesMCPTransportClosedReason, "services/mcp: transport closed: %s", "services/mcp：transport 已关闭：%s", "services/mcp: Transport geschlossen: %s", "services/mcp：transport は終了しました：%s", "services/mcp: transport가 종료되었습니다: %s", "services/mcp: transport закрыт: %s")
	add(KeyServicesMCPTransportClosedReasonCause, "services/mcp: transport closed: %s: %v", "services/mcp：transport 已关闭：%s：%v", "services/mcp: Transport geschlossen: %s: %v", "services/mcp：transport は終了しました：%s：%v", "services/mcp: transport가 종료되었습니다: %s: %v", "services/mcp: transport закрыт: %s: %v")
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
	add(KeyServicesMCPResolveAuthToken, "services/mcp: resolve auth token: %v", "services/mcp：解析 auth token 失败：%v", "services/mcp: Auth-Token konnte nicht aufgelöst werden: %v", "services/mcp：auth token を解決できませんでした：%v", "services/mcp: auth token을 확인하지 못했습니다: %v", "services/mcp: не удалось получить auth token: %v")
	add(KeyServicesMCPResolveDynamicHeaders, "services/mcp: resolve dynamic headers: %v", "services/mcp：解析动态 headers 失败：%v", "services/mcp: Dynamische Header konnten nicht aufgelöst werden: %v", "services/mcp：動的 headers を解決できませんでした：%v", "services/mcp: 동적 headers를 확인하지 못했습니다: %v", "services/mcp: не удалось получить динамические headers: %v")
	add(KeyServicesMCPNilManager, "services/mcp: nil manager", "services/mcp：manager 为 nil", "services/mcp: manager ist nil", "services/mcp：manager が nil です", "services/mcp: manager가 nil입니다", "services/mcp: manager равен nil")
	add(KeyServicesMCPReadSettings, "services/mcp: read MCP settings %s: %v", "services/mcp：读取 MCP settings %s 失败：%v", "services/mcp: MCP-Einstellungen %s konnten nicht gelesen werden: %v", "services/mcp：MCP settings %s を読み取れませんでした：%v", "services/mcp: MCP settings %s을(를) 읽지 못했습니다: %v", "services/mcp: не удалось прочитать MCP settings %s: %v")
	add(KeyServicesMCPParseSettings, "services/mcp: parse MCP settings %s: %v", "services/mcp：解析 MCP settings %s 失败：%v", "services/mcp: MCP-Einstellungen %s konnten nicht geparst werden: %v", "services/mcp：MCP settings %s を parse できませんでした：%v", "services/mcp: MCP settings %s을(를) parse하지 못했습니다: %v", "services/mcp: не удалось разобрать MCP settings %s: %v")
	add(KeyServicesMCPServerNotConfigured, "services/mcp: MCP server %q not configured", "services/mcp：尚未配置 MCP server %q", "services/mcp: MCP-Server %q ist nicht konfiguriert", "services/mcp：MCP server %q は設定されていません", "services/mcp: MCP server %q이(가) 구성되지 않았습니다", "services/mcp: MCP server %q не настроен")
	add(KeyServicesMCPNilClient, "services/mcp: nil client", "services/mcp：client 为 nil", "services/mcp: client ist nil", "services/mcp：client が nil です", "services/mcp: client가 nil입니다", "services/mcp: client равен nil")
	add(KeyServicesMCPNilTransport, "services/mcp: nil transport", "services/mcp：transport 为 nil", "services/mcp: transport ist nil", "services/mcp：transport が nil です", "services/mcp: transport가 nil입니다", "services/mcp: transport равен nil")
	add(KeyServicesMCPInitializeNeedsTransport, "services/mcp: initialize requires a transport-backed client", "services/mcp：initialize 需要由 transport 支持的 client", "services/mcp: initialize erfordert einen transportgestützten Client", "services/mcp：initialize には transport 対応 client が必要です", "services/mcp: initialize에는 transport 기반 client가 필요합니다", "services/mcp: для initialize требуется client с transport")
	add(KeyServicesMCPInitializeFailed, "services/mcp: initialize: %v", "services/mcp：initialize 失败：%v", "services/mcp: initialize fehlgeschlagen: %v", "services/mcp：initialize に失敗しました：%v", "services/mcp: initialize에 실패했습니다: %v", "services/mcp: сбой initialize: %v")
	add(KeyServicesMCPDecodeInitialize, "services/mcp: decode initialize result: %v", "services/mcp：decode initialize result 失败：%v", "services/mcp: initialize-result konnte nicht decodiert werden: %v", "services/mcp：initialize result を decode できませんでした：%v", "services/mcp: initialize result를 decode하지 못했습니다: %v", "services/mcp: не удалось декодировать initialize result: %v")
	add(KeyServicesMCPInitializedNotification, "services/mcp: initialized notification: %v", "services/mcp：发送 initialized notification 失败：%v", "services/mcp: initialized-notification fehlgeschlagen: %v", "services/mcp：initialized notification に失敗しました：%v", "services/mcp: initialized notification에 실패했습니다: %v", "services/mcp: сбой initialized notification: %v")
	add(KeyServicesMCPClientNoTransport, "services/mcp: client has no transport", "services/mcp：client 没有 transport", "services/mcp: client hat keinen transport", "services/mcp：client に transport がありません", "services/mcp: client에 transport가 없습니다", "services/mcp: у client нет transport")
	add(KeyServicesMCPNotifyNeedsTransport, "services/mcp: notify requires a transport-backed client", "services/mcp：notify 需要由 transport 支持的 client", "services/mcp: notify erfordert einen transportgestützten Client", "services/mcp：notify には transport 対応 client が必要です", "services/mcp: notify에는 transport 기반 client가 필요합니다", "services/mcp: для notify требуется client с transport")
	add(KeyServicesMCPMethodFailed, "services/mcp: %s: %v", "services/mcp：%s 失败：%v", "services/mcp: %s fehlgeschlagen: %v", "services/mcp：%s に失敗しました：%v", "services/mcp: %s에 실패했습니다: %v", "services/mcp: сбой %s: %v")
	add(KeyServicesMCPNamedMethodFailed, "services/mcp: %s %q: %v", "services/mcp：%s %q 失败：%v", "services/mcp: %s %q fehlgeschlagen: %v", "services/mcp：%s %q に失敗しました：%v", "services/mcp: %s %q에 실패했습니다: %v", "services/mcp: сбой %s %q: %v")
	add(KeyServicesMCPServerNotConnected, "services/mcp: MCP server %q is %s, not connected", "services/mcp：MCP server %q 当前为 %s，尚未连接", "services/mcp: MCP-Server %q hat den Status %s und ist nicht verbunden", "services/mcp：MCP server %q は %s のため接続されていません", "services/mcp: MCP server %q의 상태가 %s이며 연결되지 않았습니다", "services/mcp: MCP server %q находится в состоянии %s и не подключён")
	add(KeyServicesMCPServerPromptsUnsupported, "services/mcp: MCP server %q does not advertise prompts", "services/mcp：MCP server %q 未声明 prompts capability", "services/mcp: MCP-Server %q kündigt keine prompts-Fähigkeit an", "services/mcp：MCP server %q は prompts capability を通知していません", "services/mcp: MCP server %q이(가) prompts capability를 알리지 않았습니다", "services/mcp: MCP server %q не объявляет capability prompts")
	add(KeyServicesMCPLineOperationFailed, "services/mcp: line transport %s: %v", "services/mcp：line transport %s 失败：%v", "services/mcp: line-transport %s fehlgeschlagen: %v", "services/mcp：line transport %s に失敗しました：%v", "services/mcp: line transport %s에 실패했습니다: %v", "services/mcp: сбой line transport %s: %v")
	add(KeyServicesMCPHTTPOperationFailed, "services/mcp: HTTP transport %s: %v", "services/mcp：HTTP transport %s 失败：%v", "services/mcp: HTTP-Transport %s fehlgeschlagen: %v", "services/mcp：HTTP transport %s に失敗しました：%v", "services/mcp: HTTP transport %s에 실패했습니다: %v", "services/mcp: сбой HTTP transport %s: %v")
	add(KeyServicesMCPSSENotInitialized, "services/mcp: SSE transport not initialized", "services/mcp：SSE transport 未初始化", "services/mcp: SSE-Transport nicht initialisiert", "services/mcp：SSE transport が初期化されていません", "services/mcp: SSE transport가 초기화되지 않았습니다", "services/mcp: SSE transport не инициализирован")
	add(KeyServicesMCPSSEOperationFailed, "services/mcp: SSE %s: %v", "services/mcp：SSE %s 失败：%v", "services/mcp: SSE %s fehlgeschlagen: %v", "services/mcp：SSE %s に失敗しました：%v", "services/mcp: SSE %s에 실패했습니다: %v", "services/mcp: сбой SSE %s: %v")
	add(KeyServicesMCPSSEContentType, "services/mcp: SSE GET returned content-type %q", "services/mcp：SSE GET 返回了 content-type %q", "services/mcp: SSE GET hat content-type %q zurückgegeben", "services/mcp：SSE GET が content-type %q を返しました", "services/mcp: SSE GET이 content-type %q을(를) 반환했습니다", "services/mcp: SSE GET вернул content-type %q")
	add(KeyServicesMCPSSEEndpointMissingURL, "services/mcp: SSE endpoint event missing URL", "services/mcp：SSE endpoint event 缺少 URL", "services/mcp: Im SSE-endpoint-event fehlt die URL", "services/mcp：SSE endpoint event に URL がありません", "services/mcp: SSE endpoint event에 URL이 없습니다", "services/mcp: в SSE endpoint event отсутствует URL")
	add(KeyServicesMCPWebSocketOperationFailed, "services/mcp: WebSocket %s: %v", "services/mcp：WebSocket %s 失败：%v", "services/mcp: WebSocket %s fehlgeschlagen: %v", "services/mcp：WebSocket %s に失敗しました：%v", "services/mcp: WebSocket %s에 실패했습니다: %v", "services/mcp: сбой WebSocket %s: %v")
	add(KeyServicesMCPWebSocketMissingUpgrade, "services/mcp: WebSocket handshake missing Upgrade", "services/mcp：WebSocket handshake 缺少 Upgrade", "services/mcp: Beim WebSocket-handshake fehlt Upgrade", "services/mcp：WebSocket handshake に Upgrade がありません", "services/mcp: WebSocket handshake에 Upgrade가 없습니다", "services/mcp: в WebSocket handshake отсутствует Upgrade")
	add(KeyServicesMCPWebSocketMissingConnection, "services/mcp: WebSocket handshake missing Connection upgrade", "services/mcp：WebSocket handshake 缺少 Connection upgrade", "services/mcp: Beim WebSocket-handshake fehlt Connection upgrade", "services/mcp：WebSocket handshake に Connection upgrade がありません", "services/mcp: WebSocket handshake에 Connection upgrade가 없습니다", "services/mcp: в WebSocket handshake отсутствует Connection upgrade")
	add(KeyServicesMCPWebSocketAcceptMismatch, "services/mcp: WebSocket handshake accept mismatch", "services/mcp：WebSocket handshake accept 不匹配", "services/mcp: WebSocket-handshake-accept stimmt nicht überein", "services/mcp：WebSocket handshake accept が一致しません", "services/mcp: WebSocket handshake accept가 일치하지 않습니다", "services/mcp: значение accept в WebSocket handshake не совпадает")
	add(KeyServicesMCPWebSocketSubprotocol, "services/mcp: WebSocket subprotocol %q, want mcp", "services/mcp：WebSocket subprotocol 为 %q，需要 mcp", "services/mcp: WebSocket-subprotocol %q, erwartet wird mcp", "services/mcp：WebSocket subprotocol は %q ですが、mcp が必要です", "services/mcp: WebSocket subprotocol이 %q입니다. mcp가 필요합니다", "services/mcp: WebSocket subprotocol %q, требуется mcp")
	add(KeyServicesMCPWebSocketContinuation, "services/mcp: unexpected WebSocket continuation frame", "services/mcp：出现意外的 WebSocket continuation frame", "services/mcp: Unerwarteter WebSocket-continuation-frame", "services/mcp：予期しない WebSocket continuation frame です", "services/mcp: 예기치 않은 WebSocket continuation frame입니다", "services/mcp: неожиданный WebSocket continuation frame")
	add(KeyServicesMCPWebSocketOpcode, "services/mcp: unsupported WebSocket opcode %d", "services/mcp：不支持 WebSocket opcode %d", "services/mcp: Nicht unterstützter WebSocket-opcode %d", "services/mcp：WebSocket opcode %d はサポートされていません", "services/mcp: WebSocket opcode %d은(는) 지원되지 않습니다", "services/mcp: WebSocket opcode %d не поддерживается")
	add(KeyServicesMCPWebSocketMessageTooLarge, "services/mcp: WebSocket message too large", "services/mcp：WebSocket message 过大", "services/mcp: WebSocket-message ist zu groß", "services/mcp：WebSocket message が大きすぎます", "services/mcp: WebSocket message가 너무 큽니다", "services/mcp: WebSocket message слишком велико")
	add(KeyServicesMCPWebSocketFrameTooLarge, "services/mcp: WebSocket frame too large", "services/mcp：WebSocket frame 过大", "services/mcp: WebSocket-frame ist zu groß", "services/mcp：WebSocket frame が大きすぎます", "services/mcp: WebSocket frame이 너무 큽니다", "services/mcp: WebSocket frame слишком велик")
	add(KeyServicesMCPPKCERandom, "pkce: read random: %v", "pkce：读取随机数失败：%v", "pkce: Zufallsdaten konnten nicht gelesen werden: %v", "pkce：乱数を読み取れませんでした：%v", "pkce: 난수를 읽지 못했습니다: %v", "pkce: не удалось прочитать случайные данные: %v")
	add(KeyServicesMCPPKCEParseASURI, "pkce: parse as_uri: %v", "pkce：解析 as_uri 失败：%v", "pkce: as_uri konnte nicht geparst werden: %v", "pkce：as_uri を parse できませんでした：%v", "pkce: as_uri를 parse하지 못했습니다: %v", "pkce: не удалось разобрать as_uri: %v")
	add(KeyServicesMCPOAuthErrorGeneric, "oauth error", "OAuth 错误", "OAuth-Fehler", "OAuth エラー", "OAuth 오류", "ошибка OAuth")
	add(KeyServicesMCPOAuthError, "oauth error %s", "OAuth 错误 %s", "OAuth-Fehler %s", "OAuth エラー %s", "OAuth 오류 %s", "ошибка OAuth %s")
	add(KeyServicesMCPOAuthErrorDetail, "oauth error %s: %s", "OAuth 错误 %s：%s", "OAuth-Fehler %s: %s", "OAuth エラー %s：%s", "OAuth 오류 %s: %s", "ошибка OAuth %s: %s")
	add(KeyServicesMCPOAuthRefreshClientIDMissing, "services/mcp: no OAuth client_id for refresh", "services/mcp：OAuth refresh 缺少 client_id", "services/mcp: Für OAuth-refresh fehlt client_id", "services/mcp：OAuth refresh 用の client_id がありません", "services/mcp: OAuth refresh에 필요한 client_id가 없습니다", "services/mcp: для OAuth refresh отсутствует client_id")
	add(KeyServicesMCPOAuthMetadataCandidatesEmpty, "services/mcp: no authorization server metadata candidates", "services/mcp：没有可用的 authorization server metadata candidate", "services/mcp: Keine Kandidaten für authorization-server-metadata", "services/mcp：authorization server metadata の候補がありません", "services/mcp: authorization server metadata 후보가 없습니다", "services/mcp: нет кандидатов authorization server metadata")
	add(KeyServicesMCPProtectedMetadataServersEmpty, "services/mcp: protected resource metadata has no authorization_servers", "services/mcp：protected resource metadata 中没有 authorization_servers", "services/mcp: protected-resource-metadata enthält keine authorization_servers", "services/mcp：protected resource metadata に authorization_servers がありません", "services/mcp: protected resource metadata에 authorization_servers가 없습니다", "services/mcp: в protected resource metadata отсутствует authorization_servers")
	add(KeyServicesMCPOAuthMetadataEndpointsMissing, "services/mcp: OAuth metadata missing authorization_endpoint or token_endpoint", "services/mcp：OAuth metadata 缺少 authorization_endpoint 或 token_endpoint", "services/mcp: In den OAuth-metadata fehlt authorization_endpoint oder token_endpoint", "services/mcp：OAuth metadata に authorization_endpoint または token_endpoint がありません", "services/mcp: OAuth metadata에 authorization_endpoint 또는 token_endpoint가 없습니다", "services/mcp: в OAuth metadata отсутствует authorization_endpoint или token_endpoint")
	add(KeyServicesMCPOAuthGETRejected, "services/mcp: GET %s returned HTTP %d", "services/mcp：GET %s 返回 HTTP %d", "services/mcp: GET %s hat HTTP %d zurückgegeben", "services/mcp：GET %s が HTTP %d を返しました", "services/mcp: GET %s이(가) HTTP %d을(를) 반환했습니다", "services/mcp: GET %s вернул HTTP %d")
	add(KeyServicesMCPOAuthDecodeJSON, "services/mcp: decode OAuth JSON from %s: %v", "services/mcp：decode 来自 %s 的 OAuth JSON 失败：%v", "services/mcp: OAuth-JSON von %s konnte nicht decodiert werden: %v", "services/mcp：%s の OAuth JSON を decode できませんでした：%v", "services/mcp: %s의 OAuth JSON을 decode하지 못했습니다: %v", "services/mcp: не удалось декодировать OAuth JSON из %s: %v")
	add(KeyServicesMCPOAuthTimeout, "services/mcp: OAuth authentication timeout", "services/mcp：OAuth 认证超时", "services/mcp: Zeitüberschreitung bei der OAuth-Authentifizierung", "services/mcp：OAuth 認証がタイムアウトしました", "services/mcp: OAuth 인증 시간이 초과되었습니다", "services/mcp: превышено время ожидания OAuth-аутентификации")
	add(KeyServicesMCPOAuthStateMismatch, "services/mcp: OAuth state mismatch", "services/mcp：OAuth state 不匹配", "services/mcp: OAuth-state stimmt nicht überein", "services/mcp：OAuth state が一致しません", "services/mcp: OAuth state가 일치하지 않습니다", "services/mcp: OAuth state не совпадает")
	add(KeyServicesMCPOAuthCallbackCodeMissing, "services/mcp: OAuth callback missing code", "services/mcp：OAuth callback 缺少 code", "services/mcp: Im OAuth-callback fehlt code", "services/mcp：OAuth callback に code がありません", "services/mcp: OAuth callback에 code가 없습니다", "services/mcp: в OAuth callback отсутствует code")
	add(KeyServicesMCPOAuthRegistrationUnavailable, "services/mcp: OAuth metadata has no registration_endpoint and config has no clientId", "services/mcp：OAuth metadata 中没有 registration_endpoint，且 config 中没有 clientId", "services/mcp: OAuth-metadata enthält keinen registration_endpoint und die Konfiguration keine clientId", "services/mcp：OAuth metadata に registration_endpoint がなく、config にも clientId がありません", "services/mcp: OAuth metadata에 registration_endpoint가 없고 config에도 clientId가 없습니다", "services/mcp: в OAuth metadata нет registration_endpoint, а в config нет clientId")
	add(KeyServicesMCPOAuthRegistrationClientID, "services/mcp: OAuth client registration returned no client_id", "services/mcp：OAuth client registration 未返回 client_id", "services/mcp: OAuth-client-registration hat keine client_id zurückgegeben", "services/mcp：OAuth client registration が client_id を返しませんでした", "services/mcp: OAuth client registration이 client_id를 반환하지 않았습니다", "services/mcp: OAuth client registration не вернула client_id")
	add(KeyServicesMCPOAuthPOSTRejected, "services/mcp: POST %s returned HTTP %d", "services/mcp：POST %s 返回 HTTP %d", "services/mcp: POST %s hat HTTP %d zurückgegeben", "services/mcp：POST %s が HTTP %d を返しました", "services/mcp: POST %s이(가) HTTP %d을(를) 반환했습니다", "services/mcp: POST %s вернул HTTP %d")
	add(KeyServicesMCPOAuthTokenEndpointRejected, "services/mcp: token endpoint returned HTTP %d", "services/mcp：token endpoint 返回 HTTP %d", "services/mcp: token-endpoint hat HTTP %d zurückgegeben", "services/mcp：token endpoint が HTTP %d を返しました", "services/mcp: token endpoint가 HTTP %d을(를) 반환했습니다", "services/mcp: token endpoint вернул HTTP %d")
	add(KeyServicesMCPOAuthAccessTokenMissing, "services/mcp: token endpoint returned no access_token", "services/mcp：token endpoint 未返回 access_token", "services/mcp: token-endpoint hat kein access_token zurückgegeben", "services/mcp：token endpoint が access_token を返しませんでした", "services/mcp: token endpoint가 access_token을 반환하지 않았습니다", "services/mcp: token endpoint не вернул access_token")
	add(KeyServicesMCPOAuthTokenStorePathMissing, "services/mcp: file token store has no path", "services/mcp：file token store 没有 path", "services/mcp: file-token-store hat keinen path", "services/mcp：file token store に path がありません", "services/mcp: file token store에 path가 없습니다", "services/mcp: у file token store нет path")
	add(KeyServicesMCPOAuthTokenStoreOperation, "services/mcp: %s OAuth token store: %v", "services/mcp：%s OAuth token store 失败：%v", "services/mcp: %s des OAuth-token-store fehlgeschlagen: %v", "services/mcp：OAuth token store の %s に失敗しました：%v", "services/mcp: OAuth token store %s에 실패했습니다: %v", "services/mcp: сбой %s OAuth token store: %v")
	add(KeyServicesMCPOAuthTokenStoreCreateDir, "services/mcp: create OAuth token store dir: %v", "services/mcp：创建 OAuth token store 目录失败：%v", "services/mcp: Verzeichnis für den OAuth-token-store konnte nicht erstellt werden: %v", "services/mcp：OAuth token store のディレクトリを作成できませんでした：%v", "services/mcp: OAuth token store 디렉터리를 만들지 못했습니다: %v", "services/mcp: не удалось создать каталог OAuth token store: %v")
	add(KeyServicesMCPClaudeAIProxyServerID, "services/mcp: claudeai-proxy server id is required", "services/mcp：claudeai-proxy 需要 server id", "services/mcp: claudeai-proxy benötigt eine server id", "services/mcp：claudeai-proxy には server id が必要です", "services/mcp: claudeai-proxy에는 server id가 필요합니다", "services/mcp: для claudeai-proxy требуется server id")
	add(KeyServicesMCPClaudeAIProxyURL, "services/mcp: invalid claudeai-proxy URL %q", "services/mcp：claudeai-proxy URL %q 无效", "services/mcp: Ungültige claudeai-proxy-URL %q", "services/mcp：claudeai-proxy URL %q が無効です", "services/mcp: 잘못된 claudeai-proxy URL %q입니다", "services/mcp: недопустимый URL claudeai-proxy %q")
	add(KeyServicesMCPNilSDKSender, "services/mcp: nil SDK MCP sender for %q", "services/mcp：%q 的 SDK MCP sender 为 nil", "services/mcp: SDK-MCP-sender für %q ist nil", "services/mcp：%q の SDK MCP sender が nil です", "services/mcp: %q의 SDK MCP sender가 nil입니다", "services/mcp: SDK MCP sender для %q равен nil")
	add(KeyServicesMCPSDKControlSend, "services/mcp: SDK control send %q: %v", "services/mcp：SDK control 向 %q 发送失败：%v", "services/mcp: SDK-control-send an %q fehlgeschlagen: %v", "services/mcp：SDK control から %q への send に失敗しました：%v", "services/mcp: SDK control에서 %q(으)로 send하지 못했습니다: %v", "services/mcp: сбой SDK control send для %q: %v")
}
