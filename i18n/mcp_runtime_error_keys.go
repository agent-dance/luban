package i18n

const (
	KeyMCPValidationInvalidJSON           Key = "mcp.validation.invalid_json"
	KeyMCPValidationMissingEnv            Key = "mcp.validation.missing_environment"
	KeyMCPValidationSetEnv                Key = "mcp.validation.set_environment"
	KeyMCPValidationCommandEmpty          Key = "mcp.validation.command_empty"
	KeyMCPValidationSetField              Key = "mcp.validation.set_field"
	KeyMCPValidationIDENameEmpty          Key = "mcp.validation.ide_name_empty"
	KeyMCPValidationNameEmpty             Key = "mcp.validation.name_empty"
	KeyMCPValidationIDEmpty               Key = "mcp.validation.id_empty"
	KeyMCPValidationTransportInvalid      Key = "mcp.validation.transport_invalid"
	KeyMCPValidationUseTransport          Key = "mcp.validation.use_transport"
	KeyMCPValidationServerNameEmpty       Key = "mcp.validation.server_name_empty"
	KeyMCPValidationServerNameReserved    Key = "mcp.validation.server_name_reserved"
	KeyMCPValidationServerNameInvalid     Key = "mcp.validation.server_name_invalid"
	KeyMCPValidationCallbackPort          Key = "mcp.validation.callback_port"
	KeyMCPValidationSetCallbackPort       Key = "mcp.validation.set_callback_port"
	KeyMCPValidationMetadataURL           Key = "mcp.validation.metadata_url"
	KeyMCPValidationSetMetadataURL        Key = "mcp.validation.set_metadata_url"
	KeyMCPValidationMetadataHTTPS         Key = "mcp.validation.metadata_https"
	KeyMCPValidationUseMetadataHTTPS      Key = "mcp.validation.use_metadata_https"
	KeyMCPValidationURLEmpty              Key = "mcp.validation.url_empty"
	KeyMCPValidationURLInvalid            Key = "mcp.validation.url_invalid"
	KeyMCPValidationSetURL                Key = "mcp.validation.set_url"
	KeyMCPIntegrationUnsupported          Key = "mcp.connection.integration_unsupported"
	KeyMCPIntegrationUnsupportedTransport Key = "mcp.connection.integration_unsupported_transport"
	KeyMCPIntegrationUnsupportedServer    Key = "mcp.connection.integration_unsupported_server"
	KeyMCPIntegrationUnavailable          Key = "mcp.connection.integration_unavailable"
	KeyMCPIntegrationUnavailableInBuild   Key = "mcp.connection.integration_unavailable_in_build"
	KeyMCPIntegrationFactoryNil           Key = "mcp.connection.integration_factory_nil"
	KeyMCPSDKBridgeMissing                Key = "mcp.connection.sdk_bridge_missing"
	KeyMCPNotIDETransport                 Key = "mcp.connection.not_ide_transport"
	KeyMCPTransportUnsupported            Key = "mcp.connection.transport_unsupported"
	KeyMCPStdioCommandRequired            Key = "mcp.connection.stdio_command_required"
	KeyMCPStdioStartFailed                Key = "mcp.connection.stdio_start_failed"
	KeyMCPStdioPipeFailed                 Key = "mcp.connection.stdio_pipe_failed"
	KeyMCPSSEBaseURLRequired              Key = "mcp.connection.sse_base_url_required"
	KeyMCPHTTPBaseURLRequired             Key = "mcp.connection.http_base_url_required"
	KeyMCPHTTPURLInvalid                  Key = "mcp.connection.http_url_invalid"
	KeyMCPHTTPSchemeInvalid               Key = "mcp.connection.http_scheme_invalid"
	KeyMCPWebSocketURLRequired            Key = "mcp.connection.websocket_url_required"
	KeyMCPWebSocketURLInvalid             Key = "mcp.connection.websocket_url_invalid"
	KeyMCPWebSocketSchemeInvalid          Key = "mcp.connection.websocket_scheme_invalid"
	KeyMCPOAuthMetadataURLInvalid         Key = "mcp.oauth.metadata_url_invalid"
	KeyMCPOAuthMetadataHTTPS              Key = "mcp.oauth.metadata_https"
	KeyMCPOAuthUnsupportedTransport       Key = "mcp.oauth.transport_unsupported"
	KeyMCPOAuthServerURLRequired          Key = "mcp.oauth.server_url_required"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyMCPValidationInvalidJSON, "MCP config is not valid JSON", "MCP 配置不是有效的 JSON", "Die MCP-Konfiguration ist kein gültiges JSON", "MCP 設定が有効な JSON ではありません", "MCP 구성이 올바른 JSON이 아닙니다", "Конфигурация MCP не является допустимым JSON")
	add(KeyMCPValidationMissingEnv, "Missing environment variables: %s", "缺少环境变量：%s", "Fehlende Umgebungsvariablen: %s", "環境変数がありません: %s", "누락된 환경 변수: %s", "Отсутствуют переменные среды: %s")
	add(KeyMCPValidationSetEnv, "Set the following environment variables: %s", "请设置以下环境变量：%s", "Setze die folgenden Umgebungsvariablen: %s", "次の環境変数を設定してください: %s", "다음 환경 변수를 설정하세요: %s", "Задайте следующие переменные среды: %s")
	add(KeyMCPValidationCommandEmpty, "Command cannot be empty", "Command 不能为空", "Command darf nicht leer sein", "Command は空にできません", "Command는 비워 둘 수 없습니다", "Command не может быть пустым")
	add(KeyMCPValidationSetField, "Set %s", "请设置 %s", "%s festlegen", "%s を設定してください", "%s을(를) 설정하세요", "Задайте %s")
	add(KeyMCPValidationIDENameEmpty, "ideName cannot be empty", "ideName 不能为空", "ideName darf nicht leer sein", "ideName は空にできません", "ideName은 비워 둘 수 없습니다", "ideName не может быть пустым")
	add(KeyMCPValidationNameEmpty, "name cannot be empty", "name 不能为空", "name darf nicht leer sein", "name は空にできません", "name은 비워 둘 수 없습니다", "name не может быть пустым")
	add(KeyMCPValidationIDEmpty, "id cannot be empty", "id 不能为空", "id darf nicht leer sein", "id は空にできません", "id는 비워 둘 수 없습니다", "id не может быть пустым")
	add(KeyMCPValidationTransportInvalid, "Invalid transport type: %s", "无效的 transport 类型：%s", "Ungültiger Transporttyp: %s", "無効な transport タイプ: %s", "잘못된 transport 유형: %s", "Недопустимый тип transport: %s")
	add(KeyMCPValidationUseTransport, "Use one of: stdio, sse, sse-ide, http, ws, ws-ide, sdk, claudeai-proxy", "请使用以下类型之一：stdio、sse、sse-ide、http、ws、ws-ide、sdk、claudeai-proxy", "Verwende einen der folgenden Typen: stdio, sse, sse-ide, http, ws, ws-ide, sdk, claudeai-proxy", "次のいずれかを使用してください: stdio、sse、sse-ide、http、ws、ws-ide、sdk、claudeai-proxy", "다음 중 하나를 사용하세요: stdio, sse, sse-ide, http, ws, ws-ide, sdk, claudeai-proxy", "Используйте один из вариантов: stdio, sse, sse-ide, http, ws, ws-ide, sdk, claudeai-proxy")
	add(KeyMCPValidationServerNameEmpty, "Invalid name. Names can only contain letters, numbers, hyphens, and underscores.", "名称无效。名称只能包含字母、数字、连字符和下划线。", "Ungültiger Name. Namen dürfen nur Buchstaben, Zahlen, Bindestriche und Unterstriche enthalten.", "名前が無効です。使用できるのは英字、数字、ハイフン、アンダースコアだけです。", "잘못된 이름입니다. 이름에는 문자, 숫자, 하이픈, 밑줄만 사용할 수 있습니다.", "Недопустимое имя. Допустимы только буквы, цифры, дефисы и подчёркивания.")
	add(KeyMCPValidationServerNameReserved, "Cannot add MCP server %q: this name is reserved.", "无法添加 MCP server %q：该名称已保留。", "MCP-Server %q kann nicht hinzugefügt werden: Dieser Name ist reserviert.", "MCP server %q を追加できません。この名前は予約されています。", "MCP server %q을(를) 추가할 수 없습니다. 예약된 이름입니다.", "Нельзя добавить MCP server %q: это имя зарезервировано.")
	add(KeyMCPValidationServerNameInvalid, "Invalid name %s. Names can only contain letters, numbers, hyphens, and underscores.", "名称 %s 无效。名称只能包含字母、数字、连字符和下划线。", "Ungültiger Name %s. Namen dürfen nur Buchstaben, Zahlen, Bindestriche und Unterstriche enthalten.", "名前 %s が無効です。使用できるのは英字、数字、ハイフン、アンダースコアだけです。", "잘못된 이름 %s입니다. 이름에는 문자, 숫자, 하이픈, 밑줄만 사용할 수 있습니다.", "Недопустимое имя %s. Допустимы только буквы, цифры, дефисы и подчёркивания.")
	add(KeyMCPValidationCallbackPort, "callbackPort must be a positive integer", "callbackPort 必须是正整数", "callbackPort muss eine positive Ganzzahl sein", "callbackPort は正の整数にしてください", "callbackPort는 양의 정수여야 합니다", "callbackPort должен быть положительным целым числом")
	add(KeyMCPValidationSetCallbackPort, "Set a positive OAuth callback port", "请设置一个正数的 OAuth callback port", "Lege einen positiven OAuth-Callback-Port fest", "正の OAuth callback port を設定してください", "양의 OAuth callback port를 설정하세요", "Задайте положительный callback port OAuth")
	add(KeyMCPValidationMetadataURL, "Invalid authServerMetadataUrl", "authServerMetadataUrl 无效", "Ungültige authServerMetadataUrl", "authServerMetadataUrl が無効です", "잘못된 authServerMetadataUrl입니다", "Недопустимый authServerMetadataUrl")
	add(KeyMCPValidationSetMetadataURL, "Set oauth.authServerMetadataUrl to a valid https URL", "请将 oauth.authServerMetadataUrl 设置为有效的 https URL", "Setze oauth.authServerMetadataUrl auf eine gültige https-URL", "oauth.authServerMetadataUrl に有効な https URL を設定してください", "oauth.authServerMetadataUrl을 올바른 https URL로 설정하세요", "Задайте для oauth.authServerMetadataUrl допустимый URL https")
	add(KeyMCPValidationMetadataHTTPS, "authServerMetadataUrl must use https://", "authServerMetadataUrl 必须使用 https://", "authServerMetadataUrl muss https:// verwenden", "authServerMetadataUrl には https:// を使用してください", "authServerMetadataUrl은 https://를 사용해야 합니다", "authServerMetadataUrl должен использовать https://")
	add(KeyMCPValidationUseMetadataHTTPS, "Use an https:// OAuth metadata URL", "请使用 https:// OAuth metadata URL", "Verwende eine OAuth-Metadaten-URL mit https://", "https:// の OAuth metadata URL を使用してください", "https:// OAuth metadata URL을 사용하세요", "Используйте URL метаданных OAuth с https://")
	add(KeyMCPValidationURLEmpty, "Invalid URL: URL cannot be empty", "URL 无效：URL 不能为空", "Ungültige URL: Die URL darf nicht leer sein", "無効な URL: URL は空にできません", "잘못된 URL: URL은 비워 둘 수 없습니다", "Недопустимый URL: URL не может быть пустым")
	add(KeyMCPValidationURLInvalid, "Invalid URL: %s", "URL 无效：%s", "Ungültige URL: %s", "無効な URL: %s", "잘못된 URL: %s", "Недопустимый URL: %s")
	add(KeyMCPValidationSetURL, "Set a valid URL for %s", "请为 %s 设置有效的 URL", "Lege eine gültige URL für %s fest", "%s に有効な URL を設定してください", "%s에 올바른 URL을 설정하세요", "Задайте допустимый URL для %s")
	add(KeyMCPIntegrationUnsupported, "MCP integration is not supported", "不支持该 MCP 集成", "MCP-Integration wird nicht unterstützt", "MCP 統合はサポートされていません", "MCP 통합이 지원되지 않습니다", "Интеграция MCP не поддерживается")
	add(KeyMCPIntegrationUnsupportedTransport, "MCP integration %q is not supported: %s", "不支持 MCP 集成 %q：%s", "MCP-Integration %q wird nicht unterstützt: %s", "MCP 統合 %q はサポートされていません: %s", "MCP 통합 %q은(는) 지원되지 않습니다: %s", "Интеграция MCP %q не поддерживается: %s")
	add(KeyMCPIntegrationUnsupportedServer, "MCP server %q does not support integration %q: %s", "MCP server %q 不支持集成 %q：%s", "MCP-Server %q unterstützt die Integration %q nicht: %s", "MCP server %q は統合 %q をサポートしていません: %s", "MCP server %q은(는) 통합 %q을(를) 지원하지 않습니다: %s", "MCP server %q не поддерживает интеграцию %q: %s")
	add(KeyMCPIntegrationUnavailable, "Integration is not available", "该集成不可用", "Integration ist nicht verfügbar", "統合を利用できません", "통합을 사용할 수 없습니다", "Интеграция недоступна")
	add(KeyMCPIntegrationUnavailableInBuild, "The bundled in-process server is only available in the TypeScript/native build", "捆绑的进程内 server 仅在 TypeScript/native 构建中可用", "Der gebündelte In-Process-Server ist nur im TypeScript-/Native-Build verfügbar", "組み込みのプロセス内 server は TypeScript/native ビルドでのみ利用できます", "번들 인프로세스 server는 TypeScript/native 빌드에서만 사용할 수 있습니다", "Встроенный внутрипроцессный server доступен только в сборке TypeScript/native")
	add(KeyMCPIntegrationFactoryNil, "The registered in-process server factory returned nil", "已注册的进程内 server factory 返回了 nil", "Die registrierte In-Process-Server-Factory hat nil zurückgegeben", "登録済みのプロセス内 server factory が nil を返しました", "등록된 인프로세스 server factory가 nil을 반환했습니다", "Зарегистрированная factory внутрипроцессного server вернула nil")
	add(KeyMCPSDKBridgeMissing, "No SDK MCP control bridge is registered in this process", "当前进程中未注册 SDK MCP control bridge", "In diesem Prozess ist keine SDK-MCP-Control-Bridge registriert", "このプロセスには SDK MCP control bridge が登録されていません", "이 프로세스에 SDK MCP control bridge가 등록되지 않았습니다", "В этом процессе не зарегистрирован control bridge SDK MCP")
	add(KeyMCPNotIDETransport, "This is not an IDE MCP transport", "这不是 IDE MCP transport", "Dies ist kein IDE-MCP-Transport", "IDE MCP transport ではありません", "IDE MCP transport가 아닙니다", "Это не transport IDE MCP")
	add(KeyMCPTransportUnsupported, "Unsupported MCP transport type %q", "不支持的 MCP transport 类型 %q", "Nicht unterstützter MCP-Transporttyp %q", "サポートされていない MCP transport タイプ %q", "지원되지 않는 MCP transport 유형 %q", "Неподдерживаемый тип transport MCP %q")
	add(KeyMCPStdioCommandRequired, "The MCP stdio transport requires a command", "MCP stdio transport 需要 command", "Der MCP-stdio-Transport benötigt einen Befehl", "MCP stdio transport には command が必要です", "MCP stdio transport에는 command가 필요합니다", "Для transport MCP stdio требуется command")
	add(KeyMCPStdioStartFailed, "Could not start MCP stdio server %q", "无法启动 MCP stdio server %q", "MCP-stdio-Server %q konnte nicht gestartet werden", "MCP stdio server %q を起動できませんでした", "MCP stdio server %q을(를) 시작할 수 없습니다", "Не удалось запустить MCP stdio server %q")
	add(KeyMCPStdioPipeFailed, "Could not open the MCP stdio %s pipe", "无法打开 MCP stdio %s pipe", "Die MCP-stdio-%s-Pipe konnte nicht geöffnet werden", "MCP stdio の %s pipe を開けませんでした", "MCP stdio %s pipe를 열 수 없습니다", "Не удалось открыть pipe %s транспорта MCP stdio")
	add(KeyMCPSSEBaseURLRequired, "The MCP SSE transport requires a Base URL", "MCP SSE transport 需要 Base URL", "Der MCP-SSE-Transport benötigt eine Base URL", "MCP SSE transport には Base URL が必要です", "MCP SSE transport에는 Base URL이 필요합니다", "Для transport MCP SSE требуется Base URL")
	add(KeyMCPHTTPBaseURLRequired, "The MCP HTTP transport requires a Base URL", "MCP HTTP transport 需要 Base URL", "Der MCP-HTTP-Transport benötigt eine Base URL", "MCP HTTP transport には Base URL が必要です", "MCP HTTP transport에는 Base URL이 필요합니다", "Для transport MCP HTTP требуется Base URL")
	add(KeyMCPHTTPURLInvalid, "Invalid MCP HTTP transport URL %q", "MCP HTTP transport URL %q 无效", "Ungültige MCP-HTTP-Transport-URL %q", "MCP HTTP transport URL %q が無効です", "잘못된 MCP HTTP transport URL %q입니다", "Недопустимый URL transport MCP HTTP %q")
	add(KeyMCPHTTPSchemeInvalid, "Invalid MCP HTTP transport scheme %q", "MCP HTTP transport scheme %q 无效", "Ungültiges MCP-HTTP-Transport-Schema %q", "MCP HTTP transport scheme %q が無効です", "잘못된 MCP HTTP transport scheme %q입니다", "Недопустимая scheme transport MCP HTTP %q")
	add(KeyMCPWebSocketURLRequired, "The MCP WebSocket transport requires a URL", "MCP WebSocket transport 需要 URL", "Der MCP-WebSocket-Transport benötigt eine URL", "MCP WebSocket transport には URL が必要です", "MCP WebSocket transport에는 URL이 필요합니다", "Для transport MCP WebSocket требуется URL")
	add(KeyMCPWebSocketURLInvalid, "Invalid MCP WebSocket URL %q", "MCP WebSocket URL %q 无效", "Ungültige MCP-WebSocket-URL %q", "MCP WebSocket URL %q が無効です", "잘못된 MCP WebSocket URL %q입니다", "Недопустимый URL MCP WebSocket %q")
	add(KeyMCPWebSocketSchemeInvalid, "Invalid MCP WebSocket scheme %q", "MCP WebSocket scheme %q 无效", "Ungültiges MCP-WebSocket-Schema %q", "MCP WebSocket scheme %q が無効です", "잘못된 MCP WebSocket scheme %q입니다", "Недопустимая scheme MCP WebSocket %q")
	add(KeyMCPOAuthMetadataURLInvalid, "Invalid OAuth metadata URL %q", "OAuth metadata URL %q 无效", "Ungültige OAuth-Metadaten-URL %q", "OAuth metadata URL %q が無効です", "잘못된 OAuth metadata URL %q입니다", "Недопустимый URL метаданных OAuth %q")
	add(KeyMCPOAuthMetadataHTTPS, "OAuth metadata URL must use https://", "OAuth metadata URL 必须使用 https://", "Die OAuth-Metadaten-URL muss https:// verwenden", "OAuth metadata URL には https:// を使用してください", "OAuth metadata URL은 https://를 사용해야 합니다", "URL метаданных OAuth должен использовать https://")
	add(KeyMCPOAuthUnsupportedTransport, "MCP transport %s does not support OAuth", "MCP transport %s 不支持 OAuth", "MCP-Transport %s unterstützt OAuth nicht", "MCP transport %s は OAuth に対応していません", "MCP transport %s은(는) OAuth를 지원하지 않습니다", "Transport MCP %s не поддерживает OAuth")
	add(KeyMCPOAuthServerURLRequired, "The MCP server URL is required for OAuth", "OAuth 需要 MCP server URL", "Für OAuth ist die MCP-Server-URL erforderlich", "OAuth には MCP server URL が必要です", "OAuth에는 MCP server URL이 필요합니다", "Для OAuth требуется URL MCP server")
}
