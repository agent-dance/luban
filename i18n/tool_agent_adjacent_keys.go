package i18n

// Semantic copy for Agent-adjacent remote-runtime, MCP-readiness, and plugin
// errors that reach the Agent tool-result boundary. Protocol identifiers,
// paths, HTTP status codes, raw response bodies, server names, and causes stay
// as format arguments so their canonical values are preserved.
const (
	KeyToolAgentRemoteParentPermissionSnapshotRequired  Key = "tool.agent.remote.parent_permission_snapshot_required"
	KeyToolAgentRemoteProfileRestrictionsRequired       Key = "tool.agent.remote.profile_restrictions_required"
	KeyToolAgentRemoteFailClosedPromptsRequired         Key = "tool.agent.remote.fail_closed_prompts_required"
	KeyToolAgentRemoteAuthenticationRequired            Key = "tool.agent.remote.authentication_required"
	KeyToolAgentRemoteEncodeSpawnFailed                 Key = "tool.agent.remote.encode_spawn_failed"
	KeyToolAgentRemoteBuildSpawnRequestFailed           Key = "tool.agent.remote.build_spawn_request_failed"
	KeyToolAgentRemoteSpawnRequestFailed                Key = "tool.agent.remote.spawn_request_failed"
	KeyToolAgentRemoteSpawnRejected                     Key = "tool.agent.remote.spawn_rejected"
	KeyToolAgentRemoteReadSpawnResponseFailed           Key = "tool.agent.remote.read_spawn_response_failed"
	KeyToolAgentRemoteDecodeSpawnResponseFailed         Key = "tool.agent.remote.decode_spawn_response_failed"
	KeyToolAgentRemoteTaskIDMissing                     Key = "tool.agent.remote.task_id_missing"
	KeyToolAgentRemotePermissionSnapshotUnacknowledged  Key = "tool.agent.remote.permission_snapshot_unacknowledged"
	KeyToolAgentRemotePromptRoutingUnacknowledged       Key = "tool.agent.remote.prompt_routing_unacknowledged"
	KeyToolAgentRemoteProfileRestrictionsUnacknowledged Key = "tool.agent.remote.profile_restrictions_unacknowledged"

	KeyToolAgentMCPManagerMissingDetail         Key = "tool.agent.mcp.manager_missing_detail"
	KeyToolAgentMCPManagerNotConfigured         Key = "tool.agent.mcp.manager_not_configured"
	KeyToolAgentMCPServerNotConfiguredDetail    Key = "tool.agent.mcp.server_not_configured_detail"
	KeyToolAgentMCPRequiredServersNotConfigured Key = "tool.agent.mcp.required_servers_not_configured"
	KeyToolAgentMCPReadinessFailed              Key = "tool.agent.mcp.readiness_failed"
	KeyToolAgentMCPReadinessTimedOutWithCause   Key = "tool.agent.mcp.readiness_timed_out_with_cause"
	KeyToolAgentMCPReadinessTimedOut            Key = "tool.agent.mcp.readiness_timed_out"

	KeyToolAgentPluginConfigDirectoryUnavailable Key = "tool.agent.plugin.config_directory_unavailable"
	KeyToolAgentPluginPermissionModeUnsupported  Key = "tool.agent.plugin.permission_mode_unsupported"
)

var toolAgentAdjacentKeys = [...]Key{
	KeyToolAgentRemoteParentPermissionSnapshotRequired,
	KeyToolAgentRemoteProfileRestrictionsRequired,
	KeyToolAgentRemoteFailClosedPromptsRequired,
	KeyToolAgentRemoteAuthenticationRequired,
	KeyToolAgentRemoteEncodeSpawnFailed,
	KeyToolAgentRemoteBuildSpawnRequestFailed,
	KeyToolAgentRemoteSpawnRequestFailed,
	KeyToolAgentRemoteSpawnRejected,
	KeyToolAgentRemoteReadSpawnResponseFailed,
	KeyToolAgentRemoteDecodeSpawnResponseFailed,
	KeyToolAgentRemoteTaskIDMissing,
	KeyToolAgentRemotePermissionSnapshotUnacknowledged,
	KeyToolAgentRemotePromptRoutingUnacknowledged,
	KeyToolAgentRemoteProfileRestrictionsUnacknowledged,
	KeyToolAgentMCPManagerMissingDetail,
	KeyToolAgentMCPManagerNotConfigured,
	KeyToolAgentMCPServerNotConfiguredDetail,
	KeyToolAgentMCPRequiredServersNotConfigured,
	KeyToolAgentMCPReadinessFailed,
	KeyToolAgentMCPReadinessTimedOutWithCause,
	KeyToolAgentMCPReadinessTimedOut,
	KeyToolAgentPluginConfigDirectoryUnavailable,
	KeyToolAgentPluginPermissionModeUnsupported,
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

	add(KeyToolAgentRemoteParentPermissionSnapshotRequired,
		"Agent error: remote runtime must explicitly declare and enforce the parent permission snapshot",
		"Agent 错误：remote runtime 必须明确声明并强制执行父级权限快照",
		"Agent-Fehler: Die Remote Runtime muss den Berechtigungs-Snapshot des Elternprozesses ausdrücklich deklarieren und durchsetzen",
		"Agent エラー: remote runtime は親の権限スナップショットを明示的に宣言し、適用する必要があります",
		"Agent 오류: remote runtime은 상위 권한 스냅샷을 명시적으로 선언하고 적용해야 합니다",
		"Ошибка Agent: remote runtime должен явно заявлять и обеспечивать применение снимка родительских разрешений")
	add(KeyToolAgentRemoteProfileRestrictionsRequired,
		"Agent error: remote runtime must explicitly declare and enforce resolved profile restrictions",
		"Agent 错误：remote runtime 必须明确声明并强制执行已解析的 profile 限制",
		"Agent-Fehler: Die Remote Runtime muss die aufgelösten Profileinschränkungen ausdrücklich deklarieren und durchsetzen",
		"Agent エラー: remote runtime は解決済みの profile 制限を明示的に宣言し、適用する必要があります",
		"Agent 오류: remote runtime은 확인된 profile 제한을 명시적으로 선언하고 적용해야 합니다",
		"Ошибка Agent: remote runtime должен явно заявлять и обеспечивать применение разрешённых ограничений profile")
	add(KeyToolAgentRemoteFailClosedPromptsRequired,
		"Agent error: remote runtime must explicitly declare fail-closed permission prompt handling",
		"Agent 错误：remote runtime 必须明确声明采用 fail-closed 的权限提示处理方式",
		"Agent-Fehler: Die Remote Runtime muss ausdrücklich eine Fail-Closed-Behandlung von Berechtigungsabfragen deklarieren",
		"Agent エラー: remote runtime は権限確認を fail-closed で処理することを明示的に宣言する必要があります",
		"Agent 오류: remote runtime은 권한 요청을 fail-closed 방식으로 처리한다고 명시적으로 선언해야 합니다",
		"Ошибка Agent: remote runtime должен явно заявлять об обработке запросов разрешений по принципу fail-closed")
	add(KeyToolAgentRemoteAuthenticationRequired,
		`Agent error: isolation="remote" requires an authenticated claude.ai session`,
		`Agent 错误：isolation="remote" 需要已通过身份验证的 claude.ai session`,
		`Agent-Fehler: isolation="remote" erfordert eine authentifizierte claude.ai-Sitzung`,
		`Agent エラー: isolation="remote" には認証済みの claude.ai session が必要です`,
		`Agent 오류: isolation="remote"에는 인증된 claude.ai session이 필요합니다`,
		`Ошибка Agent: для isolation="remote" требуется аутентифицированная session claude.ai`)
	add(KeyToolAgentRemoteEncodeSpawnFailed,
		"encode remote agent spawn: %v",
		"编码 remote Agent 启动请求失败：%v",
		"Startanforderung für Remote-Agent konnte nicht codiert werden: %v",
		"remote Agent の起動リクエストをエンコードできませんでした: %v",
		"remote Agent 시작 요청을 인코딩하지 못했습니다: %v",
		"Не удалось закодировать запрос на запуск remote Agent: %v")
	add(KeyToolAgentRemoteBuildSpawnRequestFailed,
		"build remote agent spawn request: %v",
		"构建 remote Agent 启动请求失败：%v",
		"Startanforderung für Remote-Agent konnte nicht erstellt werden: %v",
		"remote Agent の起動リクエストを作成できませんでした: %v",
		"remote Agent 시작 요청을 만들지 못했습니다: %v",
		"Не удалось сформировать запрос на запуск remote Agent: %v")
	add(KeyToolAgentRemoteSpawnRequestFailed,
		"remote agent spawn failed: %v",
		"remote Agent 启动请求失败：%v",
		"Startanforderung für Remote-Agent fehlgeschlagen: %v",
		"remote Agent の起動リクエストに失敗しました: %v",
		"remote Agent 시작 요청에 실패했습니다: %v",
		"Сбой запроса на запуск remote Agent: %v")
	add(KeyToolAgentRemoteSpawnRejected,
		"remote agent spawn returned %d: %s",
		"remote Agent 启动请求返回 %d：%s",
		"Startanforderung für Remote-Agent gab %d zurück: %s",
		"remote Agent の起動リクエストから %d が返されました: %s",
		"remote Agent 시작 요청이 %d을(를) 반환했습니다: %s",
		"Запрос на запуск remote Agent вернул %d: %s")
	add(KeyToolAgentRemoteReadSpawnResponseFailed,
		"read remote agent spawn response: %v",
		"读取 remote Agent 启动响应失败：%v",
		"Startantwort des Remote-Agent konnte nicht gelesen werden: %v",
		"remote Agent の起動レスポンスを読み取れませんでした: %v",
		"remote Agent 시작 응답을 읽지 못했습니다: %v",
		"Не удалось прочитать ответ запуска remote Agent: %v")
	add(KeyToolAgentRemoteDecodeSpawnResponseFailed,
		"decode remote agent spawn response: %v",
		"解码 remote Agent 启动响应失败：%v",
		"Startantwort des Remote-Agent konnte nicht decodiert werden: %v",
		"remote Agent の起動レスポンスをデコードできませんでした: %v",
		"remote Agent 시작 응답을 디코딩하지 못했습니다: %v",
		"Не удалось декодировать ответ запуска remote Agent: %v")
	add(KeyToolAgentRemoteTaskIDMissing,
		"remote agent spawn returned no taskId",
		"remote Agent 启动响应未返回 taskId",
		"Die Startantwort des Remote-Agent enthielt keine taskId",
		"remote Agent の起動レスポンスに taskId がありません",
		"remote Agent 시작 응답에 taskId가 없습니다",
		"В ответе запуска remote Agent отсутствует taskId")
	add(KeyToolAgentRemotePermissionSnapshotUnacknowledged,
		"remote agent runtime did not acknowledge parent permission snapshot enforcement",
		"remote Agent runtime 未确认会强制执行父级权限快照",
		"Die Remote-Agent-Runtime hat die Durchsetzung des übergeordneten Berechtigungs-Snapshots nicht bestätigt",
		"remote Agent runtime は親の権限スナップショットの適用を確認しませんでした",
		"remote Agent runtime이 상위 권한 스냅샷 적용을 확인하지 않았습니다",
		"Среда выполнения remote Agent не подтвердила применение снимка родительских разрешений")
	add(KeyToolAgentRemotePromptRoutingUnacknowledged,
		"remote agent runtime did not acknowledge fail-closed prompt routing",
		"remote Agent runtime 未确认采用 fail-closed 的 prompt 路由",
		"Die Remote-Agent-Runtime hat das Fail-Closed-Routing von Prompts nicht bestätigt",
		"remote Agent runtime は fail-closed の prompt routing を確認しませんでした",
		"remote Agent runtime이 fail-closed prompt routing을 확인하지 않았습니다",
		"Среда выполнения remote Agent не подтвердила маршрутизацию prompt по принципу fail-closed")
	add(KeyToolAgentRemoteProfileRestrictionsUnacknowledged,
		"remote agent runtime did not acknowledge profile restriction enforcement",
		"remote Agent runtime 未确认会强制执行 profile 限制",
		"Die Remote-Agent-Runtime hat die Durchsetzung der Profileinschränkungen nicht bestätigt",
		"remote Agent runtime は profile 制限の適用を確認しませんでした",
		"remote Agent runtime이 profile 제한 적용을 확인하지 않았습니다",
		"Среда выполнения remote Agent не подтвердила применение ограничений profile")

	add(KeyToolAgentMCPManagerMissingDetail,
		"no MCP manager configured",
		"未配置 MCP manager",
		"Kein MCP-Manager konfiguriert",
		"MCP manager が設定されていません",
		"MCP manager가 구성되지 않았습니다",
		"MCP manager не настроен")
	add(KeyToolAgentMCPManagerNotConfigured,
		"WaitForMCPReadiness: no MCP manager configured",
		"WaitForMCPReadiness：未配置 MCP manager",
		"WaitForMCPReadiness: Kein MCP-Manager konfiguriert",
		"WaitForMCPReadiness: MCP manager が設定されていません",
		"WaitForMCPReadiness: MCP manager가 구성되지 않았습니다",
		"WaitForMCPReadiness: MCP manager не настроен")
	add(KeyToolAgentMCPServerNotConfiguredDetail,
		"server is not configured",
		"server 未配置",
		"Server ist nicht konfiguriert",
		"server が設定されていません",
		"server가 구성되지 않았습니다",
		"server не настроен")
	add(KeyToolAgentMCPRequiredServersNotConfigured,
		"Agent error: required MCP servers are not configured: %s",
		"Agent 错误：尚未配置必需的 MCP server：%s",
		"Agent-Fehler: Erforderliche MCP-Server sind nicht konfiguriert: %s",
		"Agent エラー: 必須の MCP server が設定されていません: %s",
		"Agent 오류: 필수 MCP server가 구성되지 않았습니다: %s",
		"Ошибка Agent: обязательные MCP server не настроены: %s")
	add(KeyToolAgentMCPReadinessFailed,
		"Agent error: MCP readiness failed for: %s",
		"Agent 错误：以下 MCP server 未就绪：%s",
		"Agent-Fehler: MCP-Bereitschaft fehlgeschlagen für: %s",
		"Agent エラー: 次の MCP server の readiness check に失敗しました: %s",
		"Agent 오류: 다음 MCP server의 readiness check에 실패했습니다: %s",
		"Ошибка Agent: проверка готовности MCP завершилась ошибкой для: %s")
	add(KeyToolAgentMCPReadinessTimedOutWithCause,
		"readiness timed out: %v",
		"readiness check 超时：%v",
		"Zeitüberschreitung bei der Bereitschaftsprüfung: %v",
		"readiness check がタイムアウトしました: %v",
		"readiness check 시간이 초과되었습니다: %v",
		"Истекло время ожидания проверки готовности: %v")
	add(KeyToolAgentMCPReadinessTimedOut,
		"readiness timed out",
		"readiness check 超时",
		"Zeitüberschreitung bei der Bereitschaftsprüfung",
		"readiness check がタイムアウトしました",
		"readiness check 시간이 초과되었습니다",
		"Истекло время ожидания проверки готовности")

	add(KeyToolAgentPluginConfigDirectoryUnavailable,
		"could not resolve LUBAN Code config directory for plugin agents",
		"无法确定 plugin Agent 的 LUBAN Code 配置目录",
		"Das LUBAN Code-Konfigurationsverzeichnis für Plugin-Agents konnte nicht ermittelt werden",
		"plugin Agent 用の LUBAN Code 設定ディレクトリを特定できませんでした",
		"plugin Agent용 LUBAN Code 구성 디렉터리를 확인할 수 없습니다",
		"Не удалось определить каталог конфигурации LUBAN Code для plugin Agent")
	add(KeyToolAgentPluginPermissionModeUnsupported,
		"plugin agent %q: %v",
		"plugin Agent %q：%v",
		"Plugin-Agent %q: %v",
		"plugin Agent %q: %v",
		"plugin Agent %q: %v",
		"Plugin Agent %q: %v")
}
