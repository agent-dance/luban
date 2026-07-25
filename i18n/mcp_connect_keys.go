package i18n

// Semantic copy used by the MCP and provider-connection command surfaces.
const (
	KeyMCPUsage                    Key = "mcp.usage"
	KeyMCPUsageReconnect           Key = "mcp.usage.reconnect"
	KeyMCPUsageAuthenticate        Key = "mcp.usage.authenticate"
	KeyMCPUsageAddJSON             Key = "mcp.usage.add_json"
	KeyMCPUsageRemove              Key = "mcp.usage.remove"
	KeyMCPServerNotFound           Key = "mcp.server_not_found"
	KeyMCPNoServers                Key = "mcp.no_servers"
	KeyMCPServersSummary           Key = "mcp.servers_summary"
	KeyMCPDiagnostics              Key = "mcp.diagnostics"
	KeyMCPEnableFailed             Key = "mcp.enable.failed"
	KeyMCPDisableFailed            Key = "mcp.disable.failed"
	KeyMCPNoneEnabled              Key = "mcp.enable.none"
	KeyMCPNoneDisabled             Key = "mcp.disable.none"
	KeyMCPEnabledPersistFailed     Key = "mcp.enable.persist_failed"
	KeyMCPDisabledPersistFailed    Key = "mcp.disable.persist_failed"
	KeyMCPEnabledSaved             Key = "mcp.enable.saved"
	KeyMCPDisabledSaved            Key = "mcp.disable.saved"
	KeyMCPEnabled                  Key = "mcp.enable.completed"
	KeyMCPDisabled                 Key = "mcp.disable.completed"
	KeyMCPReconnectFailed          Key = "mcp.reconnect.failed"
	KeyMCPReconnectSuccess         Key = "mcp.reconnect.success"
	KeyMCPReconnectNeedsAuth       Key = "mcp.reconnect.needs_auth"
	KeyMCPReconnectDisabled        Key = "mcp.reconnect.disabled"
	KeyMCPReconnectStateFailed     Key = "mcp.reconnect.state_failed"
	KeyMCPReconnectUnexpectedState Key = "mcp.reconnect.unexpected_state"
	KeyMCPAuthOpenURL              Key = "mcp.auth.open_url"
	KeyMCPAuthUnsupportedTransport Key = "mcp.auth.unsupported_transport"
	KeyMCPInvalidServerJSON        Key = "mcp.invalid_server_json"
	KeyMCPInvalidConfig            Key = "mcp.invalid_config"
	KeyMCPInvalidConfigError       Key = "mcp.invalid_config_error"
	KeyMCPSaveServerFailed         Key = "mcp.save_server_failed"
	KeyMCPServerAdded              Key = "mcp.server_added"
	KeyMCPRemoveServerFailed       Key = "mcp.remove_server_failed"
	KeyMCPServerRemoved            Key = "mcp.server_removed"
	KeyMCPServerNotWritable        Key = "mcp.server_not_writable"
	KeyMCPDiagnosticMarkDisabled   Key = "mcp.diagnostic.mark_disabled"
	KeyMCPDiagnosticCommandMissing Key = "mcp.diagnostic.command_missing"
	KeyMCPDiagnosticCommandNotPath Key = "mcp.diagnostic.command_not_path"
	KeyMCPNeedsOAuthScope          Key = "mcp.needs_oauth_scope"
	KeyMCPResourceMetadata         Key = "mcp.resource_metadata"
	KeyMCPServerRow                Key = "mcp.server_row"
	KeyMCPReconnectProgress        Key = "mcp.reconnect_progress"
	KeyMCPConfigSource             Key = "mcp.config_source"
	KeyMCPLastError                Key = "mcp.last_error"
	KeyMCPWarning                  Key = "mcp.warning"
	KeyMCPDetailState              Key = "mcp.detail.state"
	KeyMCPDetailTransport          Key = "mcp.detail.transport"
	KeyMCPDetailScope              Key = "mcp.detail.scope"
	KeyMCPDetailSource             Key = "mcp.detail.source"
	KeyMCPDetailAuth               Key = "mcp.detail.auth"
	KeyMCPDetailTools              Key = "mcp.detail.tools"
	KeyMCPDetailResources          Key = "mcp.detail.resources"
	KeyMCPDetailPrompts            Key = "mcp.detail.prompts"
	KeyMCPDetailCapabilities       Key = "mcp.detail.capabilities"
	KeyMCPDetailServer             Key = "mcp.detail.server"
	KeyMCPDetailLastError          Key = "mcp.detail.last_error"
	KeyMCPDetailReconnectAttempts  Key = "mcp.detail.reconnect_attempts"
	KeyMCPDiagnosticCounts         Key = "mcp.diagnostic.counts"
	KeyMCPDiagnosticServerState    Key = "mcp.diagnostic.server_state"
	KeyMCPStatePending             Key = "mcp.state.pending"
	KeyMCPStateConnected           Key = "mcp.state.connected"
	KeyMCPStateFailed              Key = "mcp.state.failed"
	KeyMCPStateNeedsAuth           Key = "mcp.state.needs_auth"
	KeyMCPStateDisabled            Key = "mcp.state.disabled"
	KeyMCPAuthStatusNeedsAuth      Key = "mcp.auth_status.needs_auth"
	KeyMCPAuthStatusAuthenticated  Key = "mcp.auth_status.authenticated"
	KeyMCPAuthStatusConfigured     Key = "mcp.auth_status.configured"
	KeyMCPAuthStatusUnknown        Key = "mcp.auth_status.unknown"
	KeyMCPAuthStatusNotApplicable  Key = "mcp.auth_status.not_applicable"
	KeyMCPScopeLocal               Key = "mcp.scope.local"
	KeyMCPScopeUser                Key = "mcp.scope.user"
	KeyMCPScopeProject             Key = "mcp.scope.project"
	KeyMCPDoctorNoServers          Key = "mcp.doctor.no_servers"
	KeyMCPDoctorFailed             Key = "mcp.doctor.failed"
	KeyMCPDoctorDiagnostics        Key = "mcp.doctor.diagnostics"
	KeyMCPDoctorConfigured         Key = "mcp.doctor.configured"

	KeyConnectUnknownProvider            Key = "connect.unknown_provider"
	KeyConnectCredentialStoreUnavailable Key = "connect.credential_store_unavailable"
	KeyConnectSaveCredentialsFailed      Key = "connect.save_credentials_failed"
	KeyConnectModelHint                  Key = "connect.model_hint"
	KeyConnectOAuthUnsupported           Key = "connect.oauth_unsupported"
	KeyConnectOAuthConfigUnavailable     Key = "connect.oauth_config_unavailable"
	KeyConnectOAuthStarting              Key = "connect.oauth_starting"
	KeyConnectBrowserOpening             Key = "connect.browser_opening"
	KeyConnectOpenURL                    Key = "connect.open_url"
	KeyConnectWaiting                    Key = "connect.waiting"
	KeyConnectOAuthTimedOut              Key = "connect.oauth_timed_out"
	KeyConnectOAuthFailed                Key = "connect.oauth_failed"
	KeyConnectSaveOAuthCredentialsFailed Key = "connect.save_oauth_credentials_failed"
	KeyConnectOAuthSuccess               Key = "connect.oauth_success"
	KeyConnectOpenAITokensWithAPIKey     Key = "connect.openai.tokens_with_api_key"
	KeyConnectOpenAITokensCodex          Key = "connect.openai.tokens_codex"
	KeyConnectOpenAIAPIKeyUnavailable    Key = "connect.openai.api_key_unavailable"
	KeyConnectDeviceUnavailable          Key = "connect.device_unavailable"
	KeyConnectDeviceStarting             Key = "connect.device_starting"
	KeyConnectUserCode                   Key = "connect.user_code"
	KeyConnectOpen                       Key = "connect.open"
	KeyConnectEnterCode                  Key = "connect.enter_code"
	KeyConnectDeviceFailed               Key = "connect.device_failed"
	KeyConnectDeviceSuccess              Key = "connect.device_success"
	KeyConnectUnsupportedOS              Key = "connect.unsupported_os"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyMCPUsage,
		"Usage: /mcp [list|status|diagnostics]\n       /mcp get <server>\n       /mcp enable [server|all]\n       /mcp disable [server|all]\n       /mcp reconnect <server>\n       /mcp authenticate <server>\n       /mcp add-json <server> '<json config>'\n       /mcp remove <server>\n",
		"用法：/mcp [list|status|diagnostics]\n       /mcp get <server>\n       /mcp enable [server|all]\n       /mcp disable [server|all]\n       /mcp reconnect <server>\n       /mcp authenticate <server>\n       /mcp add-json <server> '<json config>'\n       /mcp remove <server>\n",
		"Verwendung: /mcp [list|status|diagnostics]\n       /mcp get <server>\n       /mcp enable [server|all]\n       /mcp disable [server|all]\n       /mcp reconnect <server>\n       /mcp authenticate <server>\n       /mcp add-json <server> '<json config>'\n       /mcp remove <server>\n",
		"使い方：/mcp [list|status|diagnostics]\n       /mcp get <server>\n       /mcp enable [server|all]\n       /mcp disable [server|all]\n       /mcp reconnect <server>\n       /mcp authenticate <server>\n       /mcp add-json <server> '<json config>'\n       /mcp remove <server>\n",
		"사용법: /mcp [list|status|diagnostics]\n       /mcp get <server>\n       /mcp enable [server|all]\n       /mcp disable [server|all]\n       /mcp reconnect <server>\n       /mcp authenticate <server>\n       /mcp add-json <server> '<json config>'\n       /mcp remove <server>\n",
		"Использование: /mcp [list|status|diagnostics]\n       /mcp get <server>\n       /mcp enable [server|all]\n       /mcp disable [server|all]\n       /mcp reconnect <server>\n       /mcp authenticate <server>\n       /mcp add-json <server> '<json config>'\n       /mcp remove <server>\n")
	add(KeyMCPUsageReconnect, "Usage: /mcp reconnect <server>\n", "用法：/mcp reconnect <server>\n", "Verwendung: /mcp reconnect <server>\n", "使い方：/mcp reconnect <server>\n", "사용법: /mcp reconnect <server>\n", "Использование: /mcp reconnect <server>\n")
	add(KeyMCPUsageAuthenticate, "Usage: /mcp authenticate <server>\n", "用法：/mcp authenticate <server>\n", "Verwendung: /mcp authenticate <server>\n", "使い方：/mcp authenticate <server>\n", "사용법: /mcp authenticate <server>\n", "Использование: /mcp authenticate <server>\n")
	add(KeyMCPUsageAddJSON, "Usage: /mcp add-json <server> '<json config>'\n", "用法：/mcp add-json <server> '<json config>'\n", "Verwendung: /mcp add-json <server> '<json config>'\n", "使い方：/mcp add-json <server> '<json config>'\n", "사용법: /mcp add-json <server> '<json config>'\n", "Использование: /mcp add-json <server> '<json config>'\n")
	add(KeyMCPUsageRemove, "Usage: /mcp remove <server>\n", "用法：/mcp remove <server>\n", "Verwendung: /mcp remove <server>\n", "使い方：/mcp remove <server>\n", "사용법: /mcp remove <server>\n", "Использование: /mcp remove <server>\n")
	add(KeyMCPServerNotFound, "MCP server %q not found\n", "未找到 MCP 服务器 %q\n", "MCP-Server %q wurde nicht gefunden\n", "MCP サーバー %q が見つかりません\n", "MCP 서버 %q을(를) 찾을 수 없습니다\n", "MCP-сервер %q не найден\n")
	add(KeyMCPNoServers, "No MCP servers configured. Use `luban-code mcp add-json` to add one.\n", "尚未配置 MCP 服务器。可使用 `luban-code mcp add-json` 添加。\n", "Keine MCP-Server konfiguriert. Füge mit `luban-code mcp add-json` einen hinzu.\n", "MCP サーバーが設定されていません。`luban-code mcp add-json` で追加できます。\n", "구성된 MCP 서버가 없습니다. `luban-code mcp add-json`으로 추가하세요.\n", "Серверы MCP не настроены. Добавьте сервер командой `luban-code mcp add-json`.\n")
	add(KeyMCPServersSummary, "MCP servers: %d total (%d connected, %d pending, %d failed, %d needs auth, %d disabled)\n", "MCP 服务器：共 %d 个（已连接 %d、等待中 %d、失败 %d、需要认证 %d、已禁用 %d）\n", "MCP-Server: %d insgesamt (%d verbunden, %d ausstehend, %d fehlgeschlagen, %d benötigen Authentifizierung, %d deaktiviert)\n", "MCP サーバー：合計 %d（接続済み %d、保留中 %d、失敗 %d、要認証 %d、無効 %d）\n", "MCP 서버: 총 %d개(연결됨 %d, 대기 중 %d, 실패 %d, 인증 필요 %d, 비활성화 %d)\n", "Серверы MCP: всего %d (подключено %d, ожидает %d, с ошибками %d, нужна авторизация %d, отключено %d)\n")
	add(KeyMCPDiagnostics, "Diagnostics:\n", "MCP 诊断：\n", "MCP-Diagnose:\n", "MCP 診断：\n", "MCP 진단:\n", "Диагностика MCP:\n")
	add(KeyMCPEnableFailed, "Failed to enable MCP server %q: %v\n", "启用 MCP 服务器 %q 失败：%v\n", "MCP-Server %q konnte nicht aktiviert werden: %v\n", "MCP サーバー %q を有効にできませんでした：%v\n", "MCP 서버 %q을(를) 활성화하지 못했습니다: %v\n", "Не удалось включить сервер MCP %q: %v\n")
	add(KeyMCPDisableFailed, "Failed to disable MCP server %q: %v\n", "禁用 MCP 服务器 %q 失败：%v\n", "MCP-Server %q konnte nicht deaktiviert werden: %v\n", "MCP サーバー %q を無効にできませんでした：%v\n", "MCP 서버 %q을(를) 비활성화하지 못했습니다: %v\n", "Не удалось отключить сервер MCP %q: %v\n")
	add(KeyMCPNoneEnabled, "No MCP servers were enabled.\n", "没有启用任何 MCP 服务器。\n", "Es wurden keine MCP-Server aktiviert.\n", "有効にした MCP サーバーはありません。\n", "활성화된 MCP 서버가 없습니다.\n", "Ни один сервер MCP не был включён.\n")
	add(KeyMCPNoneDisabled, "No MCP servers were disabled.\n", "没有禁用任何 MCP 服务器。\n", "Es wurden keine MCP-Server deaktiviert.\n", "無効にした MCP サーバーはありません。\n", "비활성화된 MCP 서버가 없습니다.\n", "Ни один сервер MCP не был отключён.\n")
	add(KeyMCPEnabledPersistFailed, "Enabled %d MCP server(s), but failed to save settings: %v\n", "已启用 %d 个 MCP 服务器，但保存设置失败：%v\n", "%d MCP-Server aktiviert, aber die Einstellungen konnten nicht gespeichert werden: %v\n", "%d 台の MCP サーバーを有効にしましたが、設定を保存できませんでした：%v\n", "MCP 서버 %d개를 활성화했지만 설정을 저장하지 못했습니다: %v\n", "Серверов MCP включено: %d, но сохранить настройки не удалось: %v\n")
	add(KeyMCPDisabledPersistFailed, "Disabled %d MCP server(s), but failed to save settings: %v\n", "已禁用 %d 个 MCP 服务器，但保存设置失败：%v\n", "%d MCP-Server deaktiviert, aber die Einstellungen konnten nicht gespeichert werden: %v\n", "%d 台の MCP サーバーを無効にしましたが、設定を保存できませんでした：%v\n", "MCP 서버 %d개를 비활성화했지만 설정을 저장하지 못했습니다: %v\n", "Серверов MCP отключено: %d, но сохранить настройки не удалось: %v\n")
	add(KeyMCPEnabledSaved, "Enabled %d MCP server(s). State saved in %s\n", "已启用 %d 个 MCP 服务器。状态已保存到 %s\n", "%d MCP-Server aktiviert. Status gespeichert in %s\n", "%d 台の MCP サーバーを有効にしました。状態を %s に保存しました\n", "MCP 서버 %d개를 활성화했습니다. 상태를 %s에 저장했습니다\n", "Серверов MCP включено: %d. Состояние сохранено в %s\n")
	add(KeyMCPDisabledSaved, "Disabled %d MCP server(s). State saved in %s\n", "已禁用 %d 个 MCP 服务器。状态已保存到 %s\n", "%d MCP-Server deaktiviert. Status gespeichert in %s\n", "%d 台の MCP サーバーを無効にしました。状態を %s に保存しました\n", "MCP 서버 %d개를 비활성화했습니다. 상태를 %s에 저장했습니다\n", "Серверов MCP отключено: %d. Состояние сохранено в %s\n")
	add(KeyMCPEnabled, "Enabled %d MCP server(s).\n", "已启用 %d 个 MCP 服务器。\n", "%d MCP-Server aktiviert.\n", "%d 台の MCP サーバーを有効にしました。\n", "MCP 서버 %d개를 활성화했습니다.\n", "Серверов MCP включено: %d.\n")
	add(KeyMCPDisabled, "Disabled %d MCP server(s).\n", "已禁用 %d 个 MCP 服务器。\n", "%d MCP-Server deaktiviert.\n", "%d 台の MCP サーバーを無効にしました。\n", "MCP 서버 %d개를 비활성화했습니다.\n", "Серверов MCP отключено: %d.\n")
	add(KeyMCPReconnectFailed, "Failed to reconnect MCP server %q: %v\n", "重新连接 MCP 服务器 %q 失败：%v\n", "MCP-Server %q konnte nicht erneut verbunden werden: %v\n", "MCP サーバー %q に再接続できませんでした：%v\n", "MCP 서버 %q에 다시 연결하지 못했습니다: %v\n", "Не удалось повторно подключить сервер MCP %q: %v\n")
	add(KeyMCPReconnectSuccess, "Successfully reconnected to %s (%d tools, %d resources, %d prompts).\n", "已重新连接到 %s（%d 个工具、%d 个资源、%d 个提示）。\n", "Erneut mit %s verbunden (%d Tools, %d Ressourcen, %d Prompts).\n", "%s に再接続しました（ツール %d、リソース %d、プロンプト %d）。\n", "%s에 다시 연결했습니다(도구 %d개, 리소스 %d개, 프롬프트 %d개).\n", "Повторное подключение к %s выполнено (инструментов: %d, ресурсов: %d, промптов: %d).\n")
	add(KeyMCPReconnectNeedsAuth, "%s requires authentication. Run /mcp authenticate %s.\n", "%s 需要认证。请运行 /mcp authenticate %s。\n", "%s erfordert eine Authentifizierung. Führe /mcp authenticate %s aus.\n", "%s には認証が必要です。/mcp authenticate %s を実行してください。\n", "%s에 인증이 필요합니다. /mcp authenticate %s를 실행하세요.\n", "%s требует авторизации. Выполните /mcp authenticate %s.\n")
	add(KeyMCPReconnectDisabled, "%s is disabled. Run /mcp enable %s first.\n", "%s 已禁用。请先运行 /mcp enable %s。\n", "%s ist deaktiviert. Führe zuerst /mcp enable %s aus.\n", "%s は無効です。先に /mcp enable %s を実行してください。\n", "%s은(는) 비활성화되어 있습니다. 먼저 /mcp enable %s를 실행하세요.\n", "%s отключён. Сначала выполните /mcp enable %s.\n")
	add(KeyMCPReconnectStateFailed, "Failed to reconnect to %s: %s\n", "%s 重新连接失败：%s\n", "Erneute Verbindung mit %s fehlgeschlagen: %s\n", "%s への再接続に失敗しました：%s\n", "%s에 다시 연결하지 못했습니다: %s\n", "Не удалось повторно подключиться к %s: %s\n")
	add(KeyMCPReconnectUnexpectedState, "After reconnecting, %s is in state %s.\n", "重新连接后，%s 处于“%s”状态。\n", "Nach der erneuten Verbindung befindet sich %s im Status %s.\n", "再接続後、%s は %s 状態です。\n", "다시 연결한 후 %s의 상태는 %s입니다.\n", "После повторного подключения %s находится в состоянии %s.\n")
	add(KeyMCPAuthOpenURL, "Open this URL in your browser to authenticate MCP server %q:", "请在浏览器中打开此 URL，以认证 MCP 服务器 %q：", "Öffne diese URL im Browser, um den MCP-Server %q zu authentifizieren:", "MCP サーバー %q を認証するには、この URL をブラウザーで開いてください：", "MCP 서버 %q을(를) 인증하려면 브라우저에서 이 URL을 여세요:", "Откройте этот URL в браузере, чтобы авторизовать сервер MCP %q:")
	add(KeyMCPAuthUnsupportedTransport, "MCP server %q uses the %s transport, which does not support OAuth authentication.", "MCP 服务器 %q 使用 %s transport，不支持 OAuth 认证。", "MCP-Server %q verwendet den Transport %s, der keine OAuth-Authentifizierung unterstützt.", "MCP サーバー %q は %s transport を使用しており、OAuth 認証には対応していません。", "MCP 서버 %q은(는) OAuth 인증을 지원하지 않는 %s transport를 사용합니다.", "Сервер MCP %q использует transport %s, который не поддерживает авторизацию OAuth.")
	add(KeyMCPInvalidServerJSON, "Invalid MCP server JSON: %v\n", "MCP 服务器 JSON 无效：%v\n", "Ungültiges JSON für den MCP-Server: %v\n", "MCP サーバーの JSON が無効です：%v\n", "MCP 서버 JSON이 잘못되었습니다: %v\n", "Недопустимый JSON сервера MCP: %v\n")
	add(KeyMCPInvalidConfig, "Invalid MCP configuration:\n", "MCP 配置无效：\n", "Ungültige MCP-Konfiguration:\n", "MCP 設定が無効です：\n", "MCP 구성이 잘못되었습니다:\n", "Недопустимая конфигурация MCP:\n")
	add(KeyMCPInvalidConfigError, "Invalid MCP configuration: %v\n", "MCP 配置无效：%v\n", "Ungültige MCP-Konfiguration: %v\n", "MCP 設定が無効です：%v\n", "MCP 구성이 잘못되었습니다: %v\n", "Недопустимая конфигурация MCP: %v\n")
	add(KeyMCPSaveServerFailed, "Failed to save MCP server %q: %v\n", "保存 MCP 服务器 %q 失败：%v\n", "MCP-Server %q konnte nicht gespeichert werden: %v\n", "MCP サーバー %q を保存できませんでした：%v\n", "MCP 서버 %q을(를) 저장하지 못했습니다: %v\n", "Не удалось сохранить сервер MCP %q: %v\n")
	add(KeyMCPServerAdded, "Added MCP server %q to %s\n", "已将 MCP 服务器 %q 添加到 %s\n", "MCP-Server %q wurde zu %s hinzugefügt\n", "MCP サーバー %q を %s に追加しました\n", "MCP 서버 %q을(를) %s에 추가했습니다\n", "Сервер MCP %q добавлен в %s\n")
	add(KeyMCPRemoveServerFailed, "Failed to remove MCP server %q: %v\n", "删除 MCP 服务器 %q 失败：%v\n", "MCP-Server %q konnte nicht entfernt werden: %v\n", "MCP サーバー %q を削除できませんでした：%v\n", "MCP 서버 %q을(를) 삭제하지 못했습니다: %v\n", "Не удалось удалить сервер MCP %q: %v\n")
	add(KeyMCPServerRemoved, "Removed MCP server %q from %s\n", "已从 %[2]s 中删除 MCP 服务器 %[1]q\n", "MCP-Server %q wurde aus %s entfernt\n", "MCP サーバー %q を %s から削除しました\n", "MCP 서버 %q을(를) %s에서 삭제했습니다\n", "Сервер MCP %q удалён из %s\n")
	add(KeyMCPServerNotWritable, "MCP server %q is not present in the writable project settings.\n", "可写的项目设置中不存在 MCP 服务器 %q。\n", "MCP-Server %q ist in den beschreibbaren Projekteinstellungen nicht vorhanden.\n", "書き込み可能なプロジェクト設定に MCP サーバー %q がありません。\n", "쓰기 가능한 프로젝트 설정에 MCP 서버 %q이(가) 없습니다.\n", "Сервер MCP %q отсутствует в доступных для записи настройках проекта.\n")
	add(KeyMCPDiagnosticMarkDisabled, "%s: failed to mark as disabled: %v", "%s：标记为已禁用失败：%v", "%s: Konnte nicht als deaktiviert markiert werden: %v", "%s：無効としてマークできませんでした：%v", "%s: 비활성화로 표시하지 못했습니다: %v", "%s: не удалось пометить как отключённый: %v")
	add(KeyMCPDiagnosticCommandMissing, "%s: command %q not found", "%s：未找到命令 %q", "%s: Befehl %q wurde nicht gefunden", "%s：コマンド %q が見つかりません", "%s: 명령 %q을(를) 찾을 수 없습니다", "%s: команда %q не найдена")
	add(KeyMCPDiagnosticCommandNotPath, "%s: command %q not found in PATH", "%s：在 PATH 中未找到命令 %q", "%s: Befehl %q wurde nicht in PATH gefunden", "%s：PATH にコマンド %q が見つかりません", "%s: PATH에서 명령 %q을(를) 찾을 수 없습니다", "%s: команда %q не найдена в PATH")
	add(KeyMCPNeedsOAuthScope, "needs OAuth scope: %s", "需要 OAuth scope：%s", "Benötigt OAuth-Scope: %s", "OAuth scope が必要です：%s", "OAuth scope가 필요합니다: %s", "Требуется OAuth scope: %s")
	add(KeyMCPResourceMetadata, "resource metadata: %s", "资源 metadata：%s", "Ressourcen-Metadaten: %s", "リソース metadata：%s", "리소스 metadata: %s", "Метаданные ресурса: %s")
	add(KeyMCPServerRow, "- %s  state=%s transport=%s scope=%s tools=%d resources=%d prompts=%d auth=%s", "- %s  状态=%s transport=%s 范围=%s 工具=%d 资源=%d 提示=%d 认证=%s", "- %s  Status=%s Transport=%s Bereich=%s Tools=%d Ressourcen=%d Prompts=%d Authentifizierung=%s", "- %s  状態=%s transport=%s スコープ=%s ツール=%d リソース=%d プロンプト=%d 認証=%s", "- %s  상태=%s transport=%s 범위=%s 도구=%d 리소스=%d 프롬프트=%d 인증=%s", "- %s  состояние=%s transport=%s область=%s инструменты=%d ресурсы=%d промпты=%d авторизация=%s")
	add(KeyMCPReconnectProgress, " reconnect=%d/%d", " 重连=%d/%d", " Wiederverbindung=%d/%d", " 再接続=%d/%d", " 재연결=%d/%d", " переподключение=%d/%d")
	add(KeyMCPConfigSource, " source=%s", " 来源=%s", " Quelle=%s", " ソース=%s", " 소스=%s", " источник=%s")
	add(KeyMCPLastError, "  error: %s\n", "  错误：%s\n", "  Fehler: %s\n", "  エラー：%s\n", "  오류: %s\n", "  ошибка: %s\n")
	add(KeyMCPWarning, "  warning: %s\n", "  警告：%s\n", "  Warnung: %s\n", "  警告：%s\n", "  경고: %s\n", "  предупреждение: %s\n")
	add(KeyMCPDetailState, "  State: %s\n", "  状态：%s\n", "  Status: %s\n", "  状態：%s\n", "  상태: %s\n", "  Состояние: %s\n")
	add(KeyMCPDetailTransport, "  Transport: %s\n", "  Transport：%s\n", "  Transport: %s\n", "  Transport：%s\n", "  Transport: %s\n", "  Transport: %s\n")
	add(KeyMCPDetailScope, "  Scope: %s\n", "  范围：%s\n", "  Bereich: %s\n", "  スコープ：%s\n", "  범위: %s\n", "  Область: %s\n")
	add(KeyMCPDetailSource, "  Source: %s\n", "  来源：%s\n", "  Quelle: %s\n", "  ソース：%s\n", "  소스: %s\n", "  Источник: %s\n")
	add(KeyMCPDetailAuth, "  Authentication: %s\n", "  认证：%s\n", "  Authentifizierung: %s\n", "  認証：%s\n", "  인증: %s\n", "  Авторизация: %s\n")
	add(KeyMCPDetailTools, "  Tools: %d\n", "  工具：%d\n", "  Tools: %d\n", "  ツール：%d\n", "  도구: %d\n", "  Инструменты: %d\n")
	add(KeyMCPDetailResources, "  Resources: %d\n", "  资源：%d\n", "  Ressourcen: %d\n", "  リソース：%d\n", "  리소스: %d\n", "  Ресурсы: %d\n")
	add(KeyMCPDetailPrompts, "  Prompts: %d\n", "  提示：%d\n", "  Prompts: %d\n", "  プロンプト：%d\n", "  프롬프트: %d\n", "  Промпты: %d\n")
	add(KeyMCPDetailCapabilities, "  Capabilities: %s\n", "  能力：%s\n", "  Fähigkeiten: %s\n", "  機能：%s\n", "  기능: %s\n", "  Возможности: %s\n")
	add(KeyMCPDetailServer, "  Server: %s %s\n", "  服务器：%s %s\n", "  Server: %s %s\n", "  サーバー：%s %s\n", "  서버: %s %s\n", "  Сервер: %s %s\n")
	add(KeyMCPDetailLastError, "  Last error: %s\n", "  最近错误：%s\n", "  Letzter Fehler: %s\n", "  最後のエラー：%s\n", "  마지막 오류: %s\n", "  Последняя ошибка: %s\n")
	add(KeyMCPDetailReconnectAttempts, "  Reconnect attempts: %d/%d\n", "  重连次数：%d/%d\n", "  Wiederverbindungsversuche: %d/%d\n", "  再接続試行：%d/%d\n", "  재연결 시도: %d/%d\n", "  Попытки переподключения: %d/%d\n")
	add(KeyMCPDiagnosticCounts, "  connected=%d pending=%d failed=%d needs-auth=%d disabled=%d\n", "  已连接=%d 等待中=%d 失败=%d 需要认证=%d 已禁用=%d\n", "  verbunden=%d ausstehend=%d fehlgeschlagen=%d Authentifizierung-nötig=%d deaktiviert=%d\n", "  接続済み=%d 保留中=%d 失敗=%d 要認証=%d 無効=%d\n", "  연결됨=%d 대기 중=%d 실패=%d 인증 필요=%d 비활성화=%d\n", "  подключено=%d ожидает=%d с-ошибками=%d нужна-авторизация=%d отключено=%d\n")
	add(KeyMCPDiagnosticServerState, "  %s: %s", "  %s：%s", "  %s: %s", "  %s：%s", "  %s: %s", "  %s: %s")
	add(KeyMCPStatePending, "pending", "等待中", "ausstehend", "保留中", "대기 중", "ожидает")
	add(KeyMCPStateConnected, "connected", "已连接", "verbunden", "接続済み", "연결됨", "подключён")
	add(KeyMCPStateFailed, "failed", "失败", "fehlgeschlagen", "失敗", "실패", "ошибка")
	add(KeyMCPStateNeedsAuth, "needs-auth", "需要认证", "Authentifizierung nötig", "要認証", "인증 필요", "нужна авторизация")
	add(KeyMCPStateDisabled, "disabled", "已禁用", "deaktiviert", "無効", "비활성화", "отключён")
	add(KeyMCPAuthStatusNeedsAuth, "needs-auth", "需要认证", "Authentifizierung nötig", "要認証", "인증 필요", "нужна авторизация")
	add(KeyMCPAuthStatusAuthenticated, "authenticated", "已认证", "authentifiziert", "認証済み", "인증됨", "авторизован")
	add(KeyMCPAuthStatusConfigured, "oauth-configured", "已配置 OAuth", "OAuth konfiguriert", "OAuth 設定済み", "OAuth 구성됨", "OAuth настроен")
	add(KeyMCPAuthStatusUnknown, "unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно")
	add(KeyMCPAuthStatusNotApplicable, "not-applicable", "不适用", "nicht zutreffend", "該当なし", "해당 없음", "не применимо")
	add(KeyMCPScopeLocal, "local", "本地", "lokal", "ローカル", "로컬", "локальная")
	add(KeyMCPScopeUser, "user", "用户", "Benutzer", "ユーザー", "사용자", "пользовательская")
	add(KeyMCPScopeProject, "project", "项目", "Projekt", "プロジェクト", "프로젝트", "проектная")
	add(KeyMCPDoctorNoServers, "no MCP servers configured", "未配置 MCP 服务器", "keine MCP-Server konfiguriert", "MCP サーバー未設定", "구성된 MCP 서버 없음", "серверы MCP не настроены")
	add(KeyMCPDoctorFailed, "%d server(s): %d failed, %d need authentication, %d pending, %d disabled", "%d 个服务器：%d 个失败、%d 个需要认证、%d 个等待中、%d 个已禁用", "%d Server: %d fehlgeschlagen, %d benötigen Authentifizierung, %d ausstehend, %d deaktiviert", "サーバー %d 台：失敗 %d、要認証 %d、保留中 %d、無効 %d", "서버 %d개: 실패 %d, 인증 필요 %d, 대기 중 %d, 비활성화 %d", "Серверов: %d; с ошибками: %d, нужна авторизация: %d, ожидает: %d, отключено: %d")
	add(KeyMCPDoctorDiagnostics, "%d server(s) configured; diagnostics: %s", "%d 个服务器已配置；诊断：%s", "%d Server konfiguriert; Diagnose: %s", "サーバー %d 台を設定済み；診断：%s", "서버 %d개 구성됨; 진단: %s", "Серверов настроено: %d; диагностика: %s")
	add(KeyMCPDoctorConfigured, "%d server(s) configured: %d connected, %d pending, %d disabled", "%d 个服务器已配置：%d 个已连接、%d 个等待中、%d 个已禁用", "%d Server konfiguriert: %d verbunden, %d ausstehend, %d deaktiviert", "サーバー %d 台を設定済み：接続済み %d、保留中 %d、無効 %d", "서버 %d개 구성됨: 연결됨 %d, 대기 중 %d, 비활성화 %d", "Серверов настроено: %d; подключено: %d, ожидает: %d, отключено: %d")

	add(KeyConnectUnknownProvider, "Unknown Provider %q", "未知 Provider %q", "Unbekannter Provider %q", "不明な Provider %q", "알 수 없는 Provider %q", "Неизвестный Provider %q")
	add(KeyConnectCredentialStoreUnavailable, "Credential storage is unavailable.", "凭据存储不可用。", "Der Zugangsdaten-Speicher ist nicht verfügbar.", "認証情報ストアを利用できません。", "자격 증명 저장소를 사용할 수 없습니다.", "Хранилище учётных данных недоступно.")
	add(KeyConnectSaveCredentialsFailed, "Failed to save credentials:", "保存凭据失败：", "Zugangsdaten konnten nicht gespeichert werden:", "認証情報を保存できませんでした：", "자격 증명을 저장하지 못했습니다:", "Не удалось сохранить учётные данные:")
	add(KeyConnectModelHint, "   Switch with '/model %s/<model>'.\n", "   可使用 '/model %s/<model>' 切换。\n", "   Wechsle mit '/model %s/<model>'.\n", "   '/model %s/<model>' で切り替えられます。\n", "   '/model %s/<model>'로 전환하세요.\n", "   Для переключения используйте '/model %s/<model>'.\n")
	add(KeyConnectOAuthUnsupported, "%s does not support OAuth PKCE authentication.", "%s 不支持 OAuth PKCE 认证。", "%s unterstützt keine OAuth-PKCE-Authentifizierung.", "%s は OAuth PKCE 認証に対応していません。", "%s은(는) OAuth PKCE 인증을 지원하지 않습니다.", "%s не поддерживает авторизацию OAuth PKCE.")
	add(KeyConnectOAuthConfigUnavailable, "OAuth configuration is unavailable for %s.", "%s 的 OAuth 配置不可用。", "Für %s ist keine OAuth-Konfiguration verfügbar.", "%s の OAuth 設定を利用できません。", "%s의 OAuth 구성을 사용할 수 없습니다.", "Конфигурация OAuth для %s недоступна.")
	add(KeyConnectOAuthStarting, "🔑 Starting OAuth PKCE for %s...\n", "🔑 正在为 %s 启动 OAuth PKCE…\n", "🔑 OAuth PKCE für %s wird gestartet …\n", "🔑 %s の OAuth PKCE を開始しています…\n", "🔑 %s의 OAuth PKCE를 시작합니다…\n", "🔑 Запуск OAuth PKCE для %s…\n")
	add(KeyConnectBrowserOpening, "   A browser window will open for authorization.\n\n", "   将打开浏览器窗口进行授权。\n\n", "   Zur Autorisierung wird ein Browserfenster geöffnet.\n\n", "   認証のためにブラウザーウィンドウを開きます。\n\n", "   인증을 위해 브라우저 창이 열립니다.\n\n", "   Для авторизации откроется окно браузера.\n\n")
	add(KeyConnectOpenURL, "   If the browser does not open, use this URL:\n   %s\n\n", "   如果浏览器没有打开，请使用此 URL：\n   %s\n\n", "   Falls sich der Browser nicht öffnet, verwende diese URL:\n   %s\n\n", "   ブラウザーが開かない場合は、次の URL を使用してください：\n   %s\n\n", "   브라우저가 열리지 않으면 다음 URL을 사용하세요:\n   %s\n\n", "   Если браузер не открылся, используйте этот URL:\n   %s\n\n")
	add(KeyConnectWaiting, "   Waiting for authorization...\n", "   正在等待授权…\n", "   Warten auf Autorisierung …\n", "   認証を待っています…\n", "   인증을 기다리는 중…\n", "   Ожидание авторизации…\n")
	add(KeyConnectOAuthTimedOut, "OAuth authorization timed out.", "OAuth 授权超时。", "Zeitüberschreitung bei der OAuth-Autorisierung.", "OAuth 認証がタイムアウトしました。", "OAuth 인증 시간이 초과되었습니다.", "Время ожидания авторизации OAuth истекло.")
	add(KeyConnectOAuthFailed, "OAuth authorization failed:", "OAuth 授权失败：", "OAuth-Autorisierung fehlgeschlagen:", "OAuth 認証に失敗しました：", "OAuth 인증에 실패했습니다:", "Ошибка авторизации OAuth:")
	add(KeyConnectSaveOAuthCredentialsFailed, "Failed to save OAuth credentials:", "保存 OAuth 凭据失败：", "OAuth-Zugangsdaten konnten nicht gespeichert werden:", "OAuth 認証情報を保存できませんでした：", "OAuth 자격 증명을 저장하지 못했습니다:", "Не удалось сохранить учётные данные OAuth:")
	add(KeyConnectOAuthSuccess, "\n✅ OAuth authentication succeeded for %s.\n", "\n✅ %s 的 OAuth 认证成功。\n", "\n✅ OAuth-Authentifizierung für %s erfolgreich.\n", "\n✅ %s の OAuth 認証に成功しました。\n", "\n✅ %s의 OAuth 인증에 성공했습니다.\n", "\n✅ Авторизация OAuth для %s выполнена.\n")
	add(KeyConnectOpenAITokensWithAPIKey, "   ChatGPT OAuth tokens were saved, and the API key exchange succeeded.\n", "   已保存 ChatGPT OAuth token，且 API key 交换成功。\n", "   ChatGPT-OAuth-Token wurden gespeichert; der API-Schlüsselaustausch war erfolgreich.\n", "   ChatGPT OAuth token を保存し、API キーの交換にも成功しました。\n", "   ChatGPT OAuth token을 저장했으며 API 키 교환도 성공했습니다.\n", "   Token OAuth ChatGPT сохранены; обмен на API-ключ выполнен.\n")
	add(KeyConnectOpenAITokensCodex, "   ChatGPT OAuth tokens were saved. Requests will use the ChatGPT Codex backend.\n", "   已保存 ChatGPT OAuth token。请求将使用 ChatGPT Codex backend。\n", "   ChatGPT-OAuth-Token wurden gespeichert. Anfragen verwenden das ChatGPT-Codex-Backend.\n", "   ChatGPT OAuth token を保存しました。リクエストには ChatGPT Codex backend を使用します。\n", "   ChatGPT OAuth token을 저장했습니다. 요청은 ChatGPT Codex backend를 사용합니다.\n", "   Token OAuth ChatGPT сохранены. Запросы будут использовать backend ChatGPT Codex.\n")
	add(KeyConnectOpenAIAPIKeyUnavailable, "   API key exchange was unavailable; ChatGPT OAuth requests do not use it.\n", "   API key 交换不可用；ChatGPT OAuth 请求不使用该 key。\n", "   Der API-Schlüsselaustausch war nicht verfügbar; ChatGPT-OAuth-Anfragen verwenden ihn nicht.\n", "   API キーを交換できませんでしたが、ChatGPT OAuth リクエストでは使用しません。\n", "   API 키 교환을 사용할 수 없었지만 ChatGPT OAuth 요청에는 필요하지 않습니다.\n", "   Обмен на API-ключ недоступен; запросы ChatGPT OAuth его не используют.\n")
	add(KeyConnectDeviceUnavailable, "Device Authorization is unavailable for %s.", "%s 不支持 Device Authorization。", "Device Authorization ist für %s nicht verfügbar.", "%s では Device Authorization を利用できません。", "%s에서 Device Authorization을 사용할 수 없습니다.", "Device Authorization для %s недоступна.")
	add(KeyConnectDeviceStarting, "🔑 Starting Device Authorization for %s...\n\n", "🔑 正在为 %s 启动 Device Authorization…\n\n", "🔑 Device Authorization für %s wird gestartet …\n\n", "🔑 %s の Device Authorization を開始しています…\n\n", "🔑 %s의 Device Authorization을 시작합니다…\n\n", "🔑 Запуск Device Authorization для %s…\n\n")
	add(KeyConnectUserCode, "   User code: %s\n", "   用户代码：%s\n", "   Benutzercode: %s\n", "   ユーザーコード：%s\n", "   사용자 코드: %s\n", "   Код пользователя: %s\n")
	add(KeyConnectOpen, "   Open: %s\n", "   打开：%s\n", "   Öffnen: %s\n", "   開く：%s\n", "   열기: %s\n", "   Откройте: %s\n")
	add(KeyConnectEnterCode, "   Enter the code: %s\n", "   输入代码：%s\n", "   Code eingeben: %s\n", "   コードを入力：%s\n", "   코드 입력: %s\n", "   Введите код: %s\n")
	add(KeyConnectDeviceFailed, "Device Authorization failed:", "Device Authorization 失败：", "Device Authorization fehlgeschlagen:", "Device Authorization に失敗しました：", "Device Authorization에 실패했습니다:", "Ошибка Device Authorization:")
	add(KeyConnectDeviceSuccess, "\n✅ Device Authorization succeeded for %s.\n", "\n✅ %s 的 Device Authorization 成功。\n", "\n✅ Device Authorization für %s erfolgreich.\n", "\n✅ %s の Device Authorization に成功しました。\n", "\n✅ %s의 Device Authorization에 성공했습니다.\n", "\n✅ Device Authorization для %s выполнена.\n")
	add(KeyConnectUnsupportedOS, "Opening a browser is not supported on %s.", "%s 暂不支持打开浏览器。", "Das Öffnen eines Browsers wird unter %s nicht unterstützt.", "%s ではブラウザーを開けません。", "%s에서는 브라우저 열기를 지원하지 않습니다.", "Открытие браузера не поддерживается в %s.")
}
