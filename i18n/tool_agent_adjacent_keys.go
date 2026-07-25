package i18n

// Semantic copy for Agent-adjacent MCP-readiness and plugin
// errors that reach the Agent tool-result boundary. Protocol identifiers,
// paths, HTTP status codes, raw response bodies, server names, and causes stay
// as format arguments so their canonical values are preserved.
const (
	KeyToolAgentMCPManagerMissingDetail         Key = "tool.agent.mcp.manager_missing_detail"
	KeyToolAgentMCPManagerNotConfigured         Key = "tool.agent.mcp.manager_not_configured"
	KeyToolAgentMCPServerNotConfiguredDetail    Key = "tool.agent.mcp.server_not_configured_detail"
	KeyToolAgentMCPRequiredServersNotConfigured Key = "tool.agent.mcp.required_servers_not_configured"
	KeyToolAgentMCPReadinessFailed              Key = "tool.agent.mcp.readiness_failed"
	KeyToolAgentMCPReadinessTimedOutWithCause   Key = "tool.agent.mcp.readiness_timed_out_with_cause"
	KeyToolAgentMCPReadinessTimedOut            Key = "tool.agent.mcp.readiness_timed_out"

	KeyToolAgentPluginConfigDirectoryUnavailable Key = "tool.agent.plugin.config_directory_unavailable"
)

var toolAgentAdjacentKeys = [...]Key{
	KeyToolAgentMCPManagerMissingDetail,
	KeyToolAgentMCPManagerNotConfigured,
	KeyToolAgentMCPServerNotConfiguredDetail,
	KeyToolAgentMCPRequiredServersNotConfigured,
	KeyToolAgentMCPReadinessFailed,
	KeyToolAgentMCPReadinessTimedOutWithCause,
	KeyToolAgentMCPReadinessTimedOut,
	KeyToolAgentPluginConfigDirectoryUnavailable,
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
}
