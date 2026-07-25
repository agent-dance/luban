package i18n

// Semantic copy exposed through MCP tool descriptions, input schemas, and
// discovery metadata. MCP, OAuth, server names, tool names, and resource URIs
// remain untranslated protocol or external values.
const (
	KeyMCPListResourcesToolDescription    Key = "mcp.list_resources.description"
	KeyMCPListResourcesServerDescription  Key = "mcp.list_resources.input.server.description"
	KeyMCPReadResourceToolDescription     Key = "mcp.read_resource.description"
	KeyMCPReadResourceServerDescription   Key = "mcp.read_resource.input.server.description"
	KeyMCPReadResourceURIDescription      Key = "mcp.read_resource.input.uri.description"
	KeyMCPDynamicToolFallbackDescription  Key = "mcp.dynamic_tool.fallback_description"
	KeyMCPDynamicToolTruncatedDescription Key = "mcp.dynamic_tool.truncated_description"
	KeyMCPAuthToolDiscoveryHint           Key = "mcp.auth_tool.discovery_hint"
	KeyMCPAuthToolTransportLocation       Key = "mcp.auth_tool.transport_location"
)

var mcpToolPromptKeys = [...]Key{
	KeyMCPListResourcesToolDescription,
	KeyMCPListResourcesServerDescription,
	KeyMCPReadResourceToolDescription,
	KeyMCPReadResourceServerDescription,
	KeyMCPReadResourceURIDescription,
	KeyMCPDynamicToolFallbackDescription,
	KeyMCPDynamicToolTruncatedDescription,
	KeyMCPAuthToolDiscoveryHint,
	KeyMCPAuthToolTransportLocation,
}

func init() {
	addMCPToolPrompt := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	addMCPToolPrompt(KeyMCPListResourcesToolDescription,
		"Lists available resources from MCP servers.",
		"列出 MCP server 提供的可用资源。",
		"Listet verfügbare Ressourcen von MCP-Servern auf.",
		"MCP server から利用可能なリソースを一覧表示します。",
		"MCP server에서 사용 가능한 리소스를 나열합니다.",
		"Показывает доступные ресурсы MCP server.")
	addMCPToolPrompt(KeyMCPListResourcesServerDescription,
		"Optional MCP server name used to filter resources",
		"用于筛选资源的可选 MCP server 名称",
		"Optionaler Name des MCP-Servers zum Filtern der Ressourcen",
		"リソースの絞り込みに使用する任意の MCP server 名",
		"리소스 필터링에 사용할 선택적 MCP server 이름",
		"Необязательное имя MCP server для фильтрации ресурсов")
	addMCPToolPrompt(KeyMCPReadResourceToolDescription,
		"Reads a specific resource from an MCP server.",
		"从 MCP server 读取指定资源。",
		"Liest eine bestimmte Ressource von einem MCP-Server.",
		"MCP server から指定したリソースを読み取ります。",
		"MCP server에서 지정한 리소스를 읽습니다.",
		"Читает указанный ресурс с MCP server.")
	addMCPToolPrompt(KeyMCPReadResourceServerDescription,
		"Name of the MCP server",
		"MCP server 名称",
		"Name des MCP-Servers",
		"MCP server の名前",
		"MCP server 이름",
		"Имя MCP server")
	addMCPToolPrompt(KeyMCPReadResourceURIDescription,
		"URI of the resource to read",
		"要读取的资源 URI",
		"URI der zu lesenden Ressource",
		"読み取るリソースの URI",
		"읽을 리소스의 URI",
		"URI читаемого ресурса")
	addMCPToolPrompt(KeyMCPDynamicToolFallbackDescription,
		"Executes MCP tool %s on server %s.",
		"在 server %s 上执行 MCP 工具 %s。",
		"Führt das MCP-Tool %s auf Server %s aus.",
		"server %s で MCP ツール %s を実行します。",
		"server %s에서 MCP 도구 %s을(를) 실행합니다.",
		"Выполняет инструмент MCP %s на server %s.")
	addMCPToolPrompt(KeyMCPDynamicToolTruncatedDescription,
		"%s… [truncated]",
		"%s……[已截断]",
		"%s… [gekürzt]",
		"%s… [省略]",
		"%s… [잘림]",
		"%s… [обрезано]")
	addMCPToolPrompt(KeyMCPAuthToolDiscoveryHint,
		"MCP OAuth authentication authorization login connection required",
		"MCP OAuth 认证 授权 登录 连接 需要认证",
		"MCP OAuth Authentifizierung Autorisierung Anmeldung Verbindung erforderlich",
		"MCP OAuth 認証 承認 ログイン 接続 認証が必要",
		"MCP OAuth 인증 권한 부여 로그인 연결 인증 필요",
		"MCP OAuth аутентификация авторизация вход подключение требуется")
	addMCPToolPrompt(KeyMCPAuthToolTransportLocation,
		"%s at %s",
		"%s，地址为 %s",
		"%s unter %s",
		"%s（%s）",
		"%s(%s)",
		"%s по адресу %s")
}
