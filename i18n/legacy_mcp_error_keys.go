package i18n

// Legacy MCP errors are raised by the original stdio/SSE client and lifecycle
// manager. Protocol names, server/tool names, process identifiers, status
// codes, response bodies, and remote tool output remain formatting arguments
// rather than being treated as translatable product copy.
const (
	KeyLegacyMCPStdinPipe             Key = "legacy_mcp.stdio.stdin_pipe"
	KeyLegacyMCPStdoutPipe            Key = "legacy_mcp.stdio.stdout_pipe"
	KeyLegacyMCPStartServer           Key = "legacy_mcp.stdio.start_server"
	KeyLegacyMCPInitialize            Key = "legacy_mcp.client.initialize"
	KeyLegacyMCPToolReturnedError     Key = "legacy_mcp.tool.returned_error"
	KeyLegacyMCPNilClient             Key = "legacy_mcp.client.nil"
	KeyLegacyMCPSSEInitialize         Key = "legacy_mcp.sse.initialize"
	KeyLegacyMCPHTTPPost              Key = "legacy_mcp.sse.http_post"
	KeyLegacyMCPHTTPStatus            Key = "legacy_mcp.sse.http_status"
	KeyLegacyMCPSSEClientClosed       Key = "legacy_mcp.sse.client_closed"
	KeyLegacyMCPSSEGetStatus          Key = "legacy_mcp.sse.get_status"
	KeyLegacyMCPDecodeRPCEnvelope     Key = "legacy_mcp.rpc.decode_envelope"
	KeyLegacyMCPRPCError              Key = "legacy_mcp.rpc.remote_error"
	KeyLegacyMCPServerAlreadyRunning  Key = "legacy_mcp.lifecycle.already_running"
	KeyLegacyMCPStartNamedServer      Key = "legacy_mcp.lifecycle.start_server"
	KeyLegacyMCPServerNotFound        Key = "legacy_mcp.lifecycle.server_not_found"
	KeyLegacyMCPStopDuringRestart     Key = "legacy_mcp.lifecycle.stop_during_restart"
	KeyLegacyMCPStartDuringRestart    Key = "legacy_mcp.lifecycle.start_during_restart"
	KeyLegacyMCPNamedServerError      Key = "legacy_mcp.lifecycle.named_server_error"
	KeyLegacyMCPHealthServerNotFound  Key = "legacy_mcp.health.server_not_found"
	KeyLegacyMCPServerNotRunning      Key = "legacy_mcp.health.server_not_running"
	KeyLegacyMCPProcessNotAlive       Key = "legacy_mcp.health.process_not_alive"
	KeyLegacyMCPHealthCheckFailed     Key = "legacy_mcp.health.check_failed"
	KeyLegacyMCPReconnectAttempt      Key = "legacy_mcp.reconnect.attempt_failed"
	KeyLegacyMCPServerDisappeared     Key = "legacy_mcp.reconnect.server_disappeared"
	KeyLegacyMCPProcessUnexpectedExit Key = "legacy_mcp.reconnect.process_unexpected_exit"
)

var legacyMCPErrorKeys = []Key{
	KeyLegacyMCPStdinPipe,
	KeyLegacyMCPStdoutPipe,
	KeyLegacyMCPStartServer,
	KeyLegacyMCPInitialize,
	KeyLegacyMCPToolReturnedError,
	KeyLegacyMCPNilClient,
	KeyLegacyMCPSSEInitialize,
	KeyLegacyMCPHTTPPost,
	KeyLegacyMCPHTTPStatus,
	KeyLegacyMCPSSEClientClosed,
	KeyLegacyMCPSSEGetStatus,
	KeyLegacyMCPDecodeRPCEnvelope,
	KeyLegacyMCPRPCError,
	KeyLegacyMCPServerAlreadyRunning,
	KeyLegacyMCPStartNamedServer,
	KeyLegacyMCPServerNotFound,
	KeyLegacyMCPStopDuringRestart,
	KeyLegacyMCPStartDuringRestart,
	KeyLegacyMCPNamedServerError,
	KeyLegacyMCPHealthServerNotFound,
	KeyLegacyMCPServerNotRunning,
	KeyLegacyMCPProcessNotAlive,
	KeyLegacyMCPHealthCheckFailed,
	KeyLegacyMCPReconnectAttempt,
	KeyLegacyMCPServerDisappeared,
	KeyLegacyMCPProcessUnexpectedExit,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyLegacyMCPStdinPipe,
		"stdin pipe: %v",
		"创建 stdin pipe 失败：%v",
		"stdin-Pipe konnte nicht erstellt werden: %v",
		"stdin pipe を作成できませんでした：%v",
		"stdin pipe를 만들지 못했습니다: %v",
		"Не удалось создать stdin pipe: %v")
	add(KeyLegacyMCPStdoutPipe,
		"stdout pipe: %v",
		"创建 stdout pipe 失败：%v",
		"stdout-Pipe konnte nicht erstellt werden: %v",
		"stdout pipe を作成できませんでした：%v",
		"stdout pipe를 만들지 못했습니다: %v",
		"Не удалось создать stdout pipe: %v")
	add(KeyLegacyMCPStartServer,
		"start MCP server: %v",
		"启动 MCP server 失败：%v",
		"MCP-Server konnte nicht gestartet werden: %v",
		"MCP server を起動できませんでした：%v",
		"MCP server를 시작하지 못했습니다: %v",
		"Не удалось запустить MCP server: %v")
	add(KeyLegacyMCPInitialize,
		"MCP initialize: %v",
		"MCP 初始化失败：%v",
		"MCP-Initialisierung fehlgeschlagen: %v",
		"MCP の初期化に失敗しました：%v",
		"MCP 초기화에 실패했습니다: %v",
		"Не удалось инициализировать MCP: %v")
	add(KeyLegacyMCPToolReturnedError,
		"tool error: %s",
		"tool 返回错误：%s",
		"Tool-Fehler: %s",
		"tool エラー：%s",
		"tool 오류: %s",
		"Ошибка tool: %s")
	add(KeyLegacyMCPNilClient,
		"mcp: nil client",
		"mcp：client 为 nil",
		"mcp: Client ist nil",
		"mcp：client が nil です",
		"mcp: client가 nil입니다",
		"mcp: client равен nil")
	add(KeyLegacyMCPSSEInitialize,
		"SSE MCP initialize: %v",
		"SSE MCP 初始化失败：%v",
		"SSE-MCP-Initialisierung fehlgeschlagen: %v",
		"SSE MCP の初期化に失敗しました：%v",
		"SSE MCP 초기화에 실패했습니다: %v",
		"Не удалось инициализировать SSE MCP: %v")
	add(KeyLegacyMCPHTTPPost,
		"HTTP POST: %v",
		"HTTP POST 失败：%v",
		"HTTP POST fehlgeschlagen: %v",
		"HTTP POST に失敗しました：%v",
		"HTTP POST에 실패했습니다: %v",
		"Ошибка HTTP POST: %v")
	add(KeyLegacyMCPHTTPStatus,
		"HTTP %d: %s",
		"HTTP %d：%s",
		"HTTP %d: %s",
		"HTTP %d：%s",
		"HTTP %d: %s",
		"HTTP %d: %s")
	add(KeyLegacyMCPSSEClientClosed,
		"SSE client closed",
		"SSE client 已关闭",
		"SSE-Client wurde geschlossen",
		"SSE client は終了しました",
		"SSE client가 종료되었습니다",
		"SSE client закрыт")
	add(KeyLegacyMCPSSEGetStatus,
		"SSE GET returned %d",
		"SSE GET 返回 %d",
		"SSE GET gab %d zurück",
		"SSE GET が %d を返しました",
		"SSE GET이(가) %d을(를) 반환했습니다",
		"SSE GET вернул %d")
	add(KeyLegacyMCPDecodeRPCEnvelope,
		"decode RPC envelope: %v",
		"解析 RPC envelope 失败：%v",
		"RPC-Envelope konnte nicht dekodiert werden: %v",
		"RPC envelope を解析できませんでした：%v",
		"RPC envelope를 디코딩하지 못했습니다: %v",
		"Не удалось декодировать RPC envelope: %v")
	add(KeyLegacyMCPRPCError,
		"RPC error %d: %s",
		"RPC 错误 %d：%s",
		"RPC-Fehler %d: %s",
		"RPC エラー %d：%s",
		"RPC 오류 %d: %s",
		"Ошибка RPC %d: %s")
	add(KeyLegacyMCPServerAlreadyRunning,
		"MCP server %q is already running (pid %d)",
		"MCP server %q 已在运行（pid %d）",
		"MCP-Server %q läuft bereits (pid %d)",
		"MCP server %q はすでに実行中です（pid %d）",
		"MCP server %q이(가) 이미 실행 중입니다(pid %d)",
		"MCP server %q уже запущен (pid %d)")
	add(KeyLegacyMCPStartNamedServer,
		"start MCP server %q: %v",
		"启动 MCP server %q 失败：%v",
		"MCP-Server %q konnte nicht gestartet werden: %v",
		"MCP server %q を起動できませんでした：%v",
		"MCP server %q을(를) 시작하지 못했습니다: %v",
		"Не удалось запустить MCP server %q: %v")
	add(KeyLegacyMCPServerNotFound,
		"MCP server %q not found",
		"未找到 MCP server %q",
		"MCP-Server %q wurde nicht gefunden",
		"MCP server %q が見つかりません",
		"MCP server %q을(를) 찾을 수 없습니다",
		"MCP server %q не найден")
	add(KeyLegacyMCPStopDuringRestart,
		"stop during restart of %q: %v",
		"重启 %q 时停止失败：%v",
		"%q konnte beim Neustart nicht gestoppt werden: %v",
		"%q の再起動中に停止できませんでした：%v",
		"%q을(를) 다시 시작하는 중에 중지하지 못했습니다: %v",
		"Не удалось остановить %q во время перезапуска: %v")
	add(KeyLegacyMCPStartDuringRestart,
		"start during restart of %q: %v",
		"重启 %q 时启动失败：%v",
		"%q konnte beim Neustart nicht gestartet werden: %v",
		"%q の再起動中に起動できませんでした：%v",
		"%q을(를) 다시 시작하는 중에 시작하지 못했습니다: %v",
		"Не удалось запустить %q во время перезапуска: %v")
	add(KeyLegacyMCPNamedServerError,
		"%s: %v",
		"%s：%v",
		"%s: %v",
		"%s：%v",
		"%s: %v",
		"%s: %v")
	add(KeyLegacyMCPHealthServerNotFound,
		"server %q not found",
		"未找到 server %q",
		"Server %q wurde nicht gefunden",
		"server %q が見つかりません",
		"server %q을(를) 찾을 수 없습니다",
		"Server %q не найден")
	add(KeyLegacyMCPServerNotRunning,
		"server %q is not running",
		"server %q 未在运行",
		"Server %q läuft nicht",
		"server %q は実行されていません",
		"server %q이(가) 실행 중이 아닙니다",
		"Server %q не запущен")
	add(KeyLegacyMCPProcessNotAlive,
		"server %q process not alive: %v",
		"server %q 的进程已停止：%v",
		"Prozess von Server %q ist nicht aktiv: %v",
		"server %q のプロセスが動作していません：%v",
		"server %q의 프로세스가 실행 중이 아닙니다: %v",
		"Процесс server %q не работает: %v")
	add(KeyLegacyMCPHealthCheckFailed,
		"health check failed after %d consecutive pings",
		"连续 %d 次 ping 失败，health check 未通过",
		"Health Check nach %d aufeinanderfolgenden Pings fehlgeschlagen",
		"ping が %d 回連続で失敗したため、health check に失敗しました",
		"ping이 %d회 연속 실패하여 health check에 실패했습니다",
		"Health check завершился ошибкой после %d последовательных ping")
	add(KeyLegacyMCPReconnectAttempt,
		"reconnect attempt %d: %v",
		"第 %d 次重连失败：%v",
		"Wiederverbindungsversuch %d fehlgeschlagen: %v",
		"%d 回目の再接続に失敗しました：%v",
		"%d번째 재연결에 실패했습니다: %v",
		"Попытка переподключения %d завершилась ошибкой: %v")
	add(KeyLegacyMCPServerDisappeared,
		"server %q disappeared",
		"server %q 已消失",
		"Server %q ist nicht mehr vorhanden",
		"server %q が見つからなくなりました",
		"server %q이(가) 사라졌습니다",
		"Server %q исчез")
	add(KeyLegacyMCPProcessUnexpectedExit,
		"process exited unexpectedly",
		"进程意外退出",
		"Prozess wurde unerwartet beendet",
		"プロセスが予期せず終了しました",
		"프로세스가 예기치 않게 종료되었습니다",
		"Процесс неожиданно завершился")
}
