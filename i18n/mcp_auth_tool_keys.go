package i18n

const (
	KeyMCPAuthToolDescription          Key = "mcp.auth_tool.description"
	KeyMCPAuthToolUninitialized        Key = "mcp.auth_tool.uninitialized"
	KeyMCPAuthToolUnsupportedTransport Key = "mcp.auth_tool.unsupported_transport"
	KeyMCPAuthToolStartFailed          Key = "mcp.auth_tool.start_failed"
	KeyMCPAuthToolAuthorizationURL     Key = "mcp.auth_tool.authorization_url"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyMCPAuthToolDescription,
		"The `%s` MCP server (%s) is installed but requires authentication. Call this tool to start the OAuth flow and get an authorization URL for the user.",
		"已安装 MCP server `%s`（%s），但仍需认证。调用此工具以启动 OAuth 流程，并获取供用户使用的授权 URL。",
		"Der MCP-Server `%s` (%s) ist installiert, muss aber authentifiziert werden. Rufe dieses Tool auf, um den OAuth-Ablauf zu starten und eine Autorisierungs-URL für den Benutzer abzurufen.",
		"MCP server `%s`（%s）はインストールされていますが、認証が必要です。このツールを呼び出して OAuth フローを開始し、ユーザー向けの認証 URL を取得してください。",
		"MCP server `%s`(%s)가 설치되어 있지만 인증이 필요합니다. 이 도구를 호출하여 OAuth 흐름을 시작하고 사용자에게 제공할 인증 URL을 받으세요.",
		"MCP server `%s` (%s) установлен, но требует аутентификации. Вызовите этот инструмент, чтобы запустить OAuth и получить URL авторизации для пользователя.")
	add(KeyMCPAuthToolUninitialized,
		"MCP auth tool is not initialized", "MCP 认证工具尚未初始化", "Das MCP-Authentifizierungstool ist nicht initialisiert", "MCP 認証ツールが初期化されていません", "MCP 인증 도구가 초기화되지 않았습니다", "Инструмент аутентификации MCP не инициализирован")
	add(KeyMCPAuthToolUnsupportedTransport,
		"Server %q uses %s transport, which does not support OAuth from this tool. Ask the user to run /mcp and authenticate manually.",
		"Server %q 使用 %s transport；此工具不支持通过该 transport 完成 OAuth。请让用户运行 /mcp 并手动认证。",
		"Server %q verwendet den Transport %s, der OAuth über dieses Tool nicht unterstützt. Bitte den Benutzer, /mcp auszuführen und sich manuell zu authentifizieren.",
		"Server %q は %s transport を使用しているため、このツールから OAuth を実行できません。ユーザーに /mcp を実行して手動で認証するよう案内してください。",
		"Server %q은(는) %s transport를 사용하므로 이 도구에서 OAuth를 지원하지 않습니다. 사용자에게 /mcp를 실행하여 수동으로 인증하도록 안내하세요.",
		"Server %q использует transport %s, для которого этот инструмент не поддерживает OAuth. Попросите пользователя выполнить /mcp и пройти аутентификацию вручную.")
	add(KeyMCPAuthToolStartFailed,
		"Failed to start OAuth flow for %s: %v. Ask the user to run /mcp and authenticate manually.",
		"无法为 %s 启动 OAuth 流程：%v。请让用户运行 /mcp 并手动认证。",
		"Der OAuth-Ablauf für %s konnte nicht gestartet werden: %v. Bitte den Benutzer, /mcp auszuführen und sich manuell zu authentifizieren.",
		"%s の OAuth フローを開始できませんでした: %v。ユーザーに /mcp を実行して手動で認証するよう案内してください。",
		"%s의 OAuth 흐름을 시작하지 못했습니다: %v. 사용자에게 /mcp를 실행하여 수동으로 인증하도록 안내하세요.",
		"Не удалось запустить OAuth для %s: %v. Попросите пользователя выполнить /mcp и пройти аутентификацию вручную.")
	add(KeyMCPAuthToolAuthorizationURL,
		"Ask the user to open this URL in their browser to authorize the %s MCP server:\n\n%s\n\nOnce they complete the flow, the server's tools will become available automatically.",
		"请让用户在浏览器中打开以下 URL，以授权 %s MCP server：\n\n%s\n\n完成后，该 Server 的工具会自动可用。",
		"Bitte den Benutzer, diese URL im Browser zu öffnen, um den MCP-Server %s zu autorisieren:\n\n%s\n\nNach Abschluss des Vorgangs werden die Tools des Servers automatisch verfügbar.",
		"ユーザーにブラウザで次の URL を開き、%s MCP server を認証するよう案内してください:\n\n%s\n\nフローが完了すると、Server のツールが自動的に利用可能になります。",
		"사용자에게 브라우저에서 다음 URL을 열어 %s MCP server를 인증하도록 안내하세요:\n\n%s\n\n흐름이 완료되면 Server의 도구를 자동으로 사용할 수 있습니다.",
		"Попросите пользователя открыть этот URL в браузере, чтобы авторизовать MCP server %s:\n\n%s\n\nПосле завершения процесса инструменты Server станут доступны автоматически.")
}
